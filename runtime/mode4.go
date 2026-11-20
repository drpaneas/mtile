//go:build !mtile_mode0 && !mtile_tilecanvas

package runtime

import (
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

// The public API's Mode 0/Mode 4 compatibility contract lives in internal/transpiler/render_modes.go and
// docs/render-modes.md; CPU telemetry must never switch this hardware state
// adaptively because tile and bitmap VRAM do not preserve equivalent state.

const (
	mode4Display = 0x0404 // MODE_4 | BG2_ON; OBJ remains disabled
	forceBlank   = 0x0080
	pageBit      = 0x0010
)

// InitHardware configures affine BG2 as an identity transform, fills the
// visible page with index 255, loads all palette entries, and shows page A.
// The logical image lives on hidden page B, so pixels drawn before this call
// survive it. Page B's slot overlaps OBJ character memory, so OBJ is never
// enabled by this runtime.
func (c *Context) InitHardware() {
	hw.SetReg16(hw.DISPCNT, forceBlank)
	hw.SetReg16(hw.BG2PA, 0x0100)
	hw.SetReg16(hw.BG2PB, 0x0000)
	hw.SetReg16(hw.BG2PC, 0x0000)
	hw.SetReg16(hw.BG2PD, 0x0100)
	hw.SetReg32(hw.BG2X, 0)
	hw.SetReg32(hw.BG2Y, 0)
	c.ensureFramebuffer()
	hidden := c.mode4Page(1)
	if &c.framebuffer[0] != &hidden[0] {
		copyMode4Page(hidden, c.framebuffer)
	}
	c.framebuffer, c.pageFront, c.pageStale = hidden, 0, false
	fillMode4Page(c.mode4Page(0), 0xFF)
	// The first present must show the logical image even if the first tick
	// draws nothing, as the former copy-and-flip presentation did.
	c.pageDirty = true
	c.paletteDirty = true
	c.ApplyDisplayPalette()
	hw.SetReg16(hw.DISPCNT, mode4Display)
}

// ensureFramebuffer binds a fresh Context to hidden page B with the all-zero
// logical image of a newly initialized framebuffer.
func (c *Context) ensureFramebuffer() {
	if c.framebuffer != nil {
		return
	}
	c.framebuffer = c.mode4Page(1)
	fillMode4Page(c.framebuffer, 0)
	c.pageFront, c.pageStale, c.pageDirty = 0, false, false
}

// writableFramebuffer returns the hidden page for drawing. Every pixel writer
// calls it first. After a flip the hidden page holds the image from two ticks
// earlier, so the first write copies the visible page across to keep the
// framebuffer persistent; cls avoids that copy through clearTarget.
func (c *Context) writableFramebuffer() []byte {
	if c.pageStale || c.framebuffer == nil {
		c.syncHiddenPage()
	}
	c.pageDirty = true
	return c.framebuffer
}

//go:noinline
func (c *Context) syncHiddenPage() {
	c.ensureFramebuffer()
	if !c.pageStale {
		return
	}
	hidden := c.mode4Page(1 - int(c.pageFront))
	if c.profileEnabled {
		cycleStart := startShortCycleMeasurement()
		copyMode4Page(hidden, c.framebuffer)
		c.lastBlitCycles = stopShortCycleMeasurement(cycleStart)
	} else {
		copyMode4Page(hidden, c.framebuffer)
	}
	c.framebuffer, c.pageStale = hidden, false
}

// clearTarget returns the hidden page for a full-page clear, which replaces
// every pixel, so a stale page needs no copy first.
func (c *Context) clearTarget() []byte {
	c.ensureFramebuffer()
	if c.pageStale {
		c.framebuffer, c.pageStale = c.mode4Page(1-int(c.pageFront)), false
	}
	c.pageDirty = true
	return c.framebuffer
}

func (c *Context) clearRender(color int) {
	c.clearMode4(color)
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

// PrepareHiddenPage is the generated loop's pre-present hook. Drawing already
// happened on the hidden page, so nothing is copied; it records the VCOUNT
// window for diagnostics. LastBlitCycles reports the most recent stale-page
// copy instead, the only presentation copy Mode 4 still performs.
func (c *Context) PrepareHiddenPage() {
	now := hw.GetReg16(hw.VCOUNT)
	c.lastBlitStart, c.lastBlitEnd = now, now
}

// BeginRenderFrame, PrepareRender, and PresentRender are the only renderer
// lifecycle calls emitted by generated games. They deliberately resolve to
// concrete methods at compile time, so selecting Mode 0 cannot add a renderer
// interface dispatch or per-frame mode branch to Mode 4.
func (c *Context) BeginRenderFrame() {}

func (c *Context) PrepareRender() {
	c.PrepareHiddenPage()
}

func (c *Context) PresentRender() {
	c.ensurePaletteState()
	if !c.paletteDirty && !c.pageDirty {
		return
	}
	// A page flip fits in one complete scanline. Publishing all 256 palette
	// entries and flipping takes up to seven lines in the native map/sprite
	// parity fixture. Allow ten for changes in code layout and bus timing.
	reserve := uint32(cyclesPerScanline)
	if c.paletteDirty {
		reserve = 10 * cyclesPerScanline
	}
	if c.nativeAudio != nil {
		reserve += audio.NativeServiceCycleBudget
	}
	waitForPresentCycles(reserve)
	c.ApplyDisplayPalette()
	c.FlipPage()
}

// BlitToHiddenPage is the VBlank-gated diagnostic present used by drawspike.
// It refuses to proceed outside VBlank; the hidden page already holds the
// frame, so success means the caller may flip.
func (c *Context) BlitToHiddenPage() bool {
	start := hw.GetReg16(hw.VCOUNT)
	c.lastBlitStart, c.lastBlitEnd = start, start
	if start < ScreenHeight {
		c.frameSkips++
		return false
	}
	return true
}

// FlipPage shows the hidden page when anything was drawn since the last flip.
// Otherwise the visible page already holds the logical image. The flipped-in
// page stays the logical image until the next write or clear.
func (c *Context) FlipPage() {
	c.ensureFramebuffer()
	if !c.pageDirty {
		return
	}
	c.pageFront ^= 1
	hw.SetReg16(hw.DISPCNT, hw.GetReg16(hw.DISPCNT)&^pageBit|uint16(c.pageFront)<<4)
	c.pageStale, c.pageDirty = true, false
}
