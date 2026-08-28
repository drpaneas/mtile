package runtime

import "github.com/drpaneas/mtile/runtime/hw"

const (
	mode4Display = 0x0404 // MODE_4 | BG2_ON; OBJ remains disabled
	forceBlank   = 0x0080
	pageBit      = 0x0010

	cyclesPerScanline = 1232
	scanlinesPerFrame = 228
	// cyclesPerFrame backstops the VCOUNT-based deadline check below: a copy
	// slow enough to wrap one or more full 228-line frames can land VCOUNT
	// back in the 160-227 range by coincidence, aliasing as "still in
	// VBlank" even though it overran by a full frame or more.
	cyclesPerFrame = cyclesPerScanline * scanlinesPerFrame
)

// BlitKind selects the VBlank copy used by BlitToHiddenPage. DMA is the
// production default. CPU and IWRAM exist so the presentation gate can
// record VCOUNT and cycle counts for each path.
type BlitKind int

const (
	BlitDMA BlitKind = iota
	BlitCPU
	BlitIWRAM
)

func clearPage(base uintptr, value byte) {
	word := uint32(value) | uint32(value)<<8 | uint32(value)<<16 | uint32(value)<<24
	for offset := 0; offset < hw.PageStride; offset += 4 {
		hw.SetVRAM32(base+uintptr(offset), word)
	}
}

// InitHardware configures affine BG2 as an identity transform, clears both
// full Mode 4 page slots with 32-bit writes, loads all palette entries, and
// shows page A. Page B's slot overlaps OBJ character memory, so OBJ is never
// enabled by this runtime.
func (c *Context) InitHardware() {
	hw.SetReg16(hw.DISPCNT, forceBlank)
	hw.SetReg16(hw.BG2PA, 0x0100)
	hw.SetReg16(hw.BG2PB, 0x0000)
	hw.SetReg16(hw.BG2PC, 0x0000)
	hw.SetReg16(hw.BG2PD, 0x0100)
	hw.SetReg32(hw.BG2X, 0)
	hw.SetReg32(hw.BG2Y, 0)
	clearPage(hw.Page0, 0xFF)
	clearPage(hw.Page1, 0xFF)
	c.paletteDirty = true
	c.ApplyDisplayPalette()
	hw.SetReg16(hw.DISPCNT, mode4Display)
}

// WaitForPresentWindow uses the current VBlank if the caller is already in
// it. This is deliberately different from helpers that always wait for the
// next VBlank and would waste the remaining presentation window.
func (c *Context) WaitForPresentWindow() {
	if c != nil && c.profileEnabled {
		c.lastPresentStart = hw.GetReg16(hw.VCOUNT)
	}
	hw.WaitForPresentWindow()
	if c != nil && c.profileEnabled {
		c.lastPresentEnd = hw.GetReg16(hw.VCOUNT)
	}
}

// WaitVDraw consumes the current VBlank so each generated-loop iteration is
// locked to exactly one display interval.
func (c *Context) WaitVDraw() {
	hw.WaitVDraw()
}

func (c *Context) hiddenPage() uintptr {
	if hw.GetReg16(hw.DISPCNT)&pageBit == 0 {
		return hw.Page1
	}
	return hw.Page0
}

func (c *Context) SetBlitKind(kind BlitKind) {
	if kind < BlitDMA || kind > BlitIWRAM {
		return
	}
	c.blitKind = kind
}

func (c *Context) BlitKind() BlitKind {
	return c.blitKind
}

func (c *Context) copyFramebuffer(dest uintptr) {
	words := FramebufferSize / 4
	src := c.framebuffer[:]
	switch c.blitKind {
	case BlitCPU:
		hw.CopyToVRAM32CPU(src, dest, words)
	case BlitIWRAM:
		hw.CopyToVRAM32IWRAM(src, dest, words)
	default:
		hw.CopyToVRAM32DMA(src, dest, words)
	}
}

// PrepareHiddenPage copies the logical framebuffer into the hidden VRAM
// page immediately, without waiting for VBlank first. This is the
// production presentation path used by the generated loop: it is always
// safe to write to the hidden page at any VCOUNT, because that page is by
// definition not the one currently being scanned out for display, so the
// write cannot produce a visible tear. Only the page flip itself
// (FlipPage) is display-order-sensitive and must still happen during
// VBlank; ApplyDisplayPalette and FlipPage handle that.
//
// Calling this as soon as Draw finishes - rather than waiting for VBlank
// first, as the diagnostic BlitToHiddenPage below does - gives the copy
// the entire ~197,120-cycle VDraw window to run in instead of sharing the
// ~83,776-cycle VBlank window with the palette write and the flip. The
// production DMA copy measures ~76,970 cycles (see BlitToHiddenPage's doc
// comment), which used ~92% of the old VBlank-only budget; against the
// VDraw window it uses under 40%, removing the deadline risk entirely for
// any per-frame workload that was already fitting before.
func (c *Context) PrepareHiddenPage() {
	start := hw.GetReg16(hw.VCOUNT)
	base := c.hiddenPage()
	if c.profileEnabled {
		cycleStart := startShortCycleMeasurement()
		c.copyFramebuffer(base)
		c.lastBlitCycles = stopShortCycleMeasurement(cycleStart)
	} else {
		c.copyFramebuffer(base)
		c.lastBlitCycles = 0
	}
	c.lastBlitStart = start
	c.lastBlitEnd = hw.GetReg16(hw.VCOUNT)
}

// BlitToHiddenPage copies the logical framebuffer as packed 32-bit Mode 4
// indices. It refuses to flip if VBlank is already over or wraps before the
// final volatile VCOUNT read. Cycle count covers only the copy.
//
// This VBlank-gated form is kept for the drawspike DMA/CPU/IWRAM comparison,
// which needs to know whether a copy fits inside one VBlank window, and for
// any caller that still wants that guarantee. The generated production loop
// calls PrepareHiddenPage right after Draw and FlipPage once VBlank arrives.
func (c *Context) BlitToHiddenPage() bool {
	start := hw.GetReg16(hw.VCOUNT)
	c.lastBlitStart = start
	c.lastBlitCycles = 0
	if start < ScreenHeight {
		c.frameSkips++
		c.lastBlitEnd = start
		return false
	}

	base := c.hiddenPage()
	cycleStart := startShortCycleMeasurement()
	c.copyFramebuffer(base)
	c.lastBlitCycles = stopShortCycleMeasurement(cycleStart)

	end := hw.GetReg16(hw.VCOUNT)
	c.lastBlitEnd = end
	if c.lastBlitCycles >= cyclesPerFrame || end < ScreenHeight || end < start {
		c.frameSkips++
		return false
	}
	return true
}

func (c *Context) FlipPage() {
	hw.SetReg16(hw.DISPCNT, hw.GetReg16(hw.DISPCNT)^pageBit)
}

func (c *Context) FrameSkips() uint32 {
	return c.frameSkips
}

func (c *Context) LastBlitWindow() (start, end uint16) {
	return c.lastBlitStart, c.lastBlitEnd
}

func (c *Context) LastBlitCycles() uint32 {
	return c.lastBlitCycles
}
