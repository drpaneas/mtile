package runtime

import "github.com/drpaneas/mtile/runtime/hw"

const (
	cyclesPerScanline = 1232
	scanlinesPerFrame = 228
	cyclesPerFrame    = cyclesPerScanline * scanlinesPerFrame
)

// BlitKind is a diagnostic compatibility setting from the former Mode 4
// framebuffer copy. Renderers keep it so benchmark code can still record it;
// no renderer copies a framebuffer to present any more.
type BlitKind int

const (
	BlitDMA BlitKind = iota
	BlitCPU
	BlitIWRAM
)

// RendererUploadStats exposes the renderer's last materialization work to
// tests and benchmarks without adding Mode 0-only counters to the Mode 4
// Context allocation.
type RendererUploadStats struct {
	ColdTileBytes    uint32
	LastTileBytes    uint32
	LastMapBytes     uint32
	LastOAMBytes     uint32
	LastOBJTileBytes uint32
}

// Only complete scanlines after VCOUNT are available to finish visible writes.
// A late caller can keep the current image for one more scan and commit in
// the next VBlank.
func presentWindowFits(line uint16, cycles uint32) bool {
	if line < ScreenHeight || line >= scanlinesPerFrame {
		return false
	}
	completeLines := scanlinesPerFrame - int(line) - 1
	return cycles <= uint32(completeLines*cyclesPerScanline)
}

func waitForPresentCycles(cycles uint32) {
	if !presentWindowFits(hw.GetReg16(hw.VCOUNT), cycles) {
		hw.WaitVDraw()
		hw.WaitForPresentWindow()
	}
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

// WaitVDraw consumes the current VBlank by observing the next LCD frame event.
// An audio IRQ may return after that frame's visible interval; it must not make
// the game loop wait an extra refresh. VRAM guards use hw.WaitVDraw instead
// when they require an actual visible scanline followed by a fresh VBlank.
func (c *Context) WaitVDraw() {
	hw.WaitFrameStart()
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
