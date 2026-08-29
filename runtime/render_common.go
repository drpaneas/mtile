package runtime

import "github.com/drpaneas/mtile/runtime/hw"

const (
	cyclesPerScanline = 1232
	scanlinesPerFrame = 228
	cyclesPerFrame    = cyclesPerScanline * scanlinesPerFrame
)

// BlitKind selects the diagnostic Mode 4 framebuffer copy. It remains part of
// the common profiling surface so benchmark code can report the selected
// renderer without changing Context's shape at runtime.
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

func (c *Context) FrameSkips() uint32 {
	return c.frameSkips
}

func (c *Context) LastBlitWindow() (start, end uint16) {
	return c.lastBlitStart, c.lastBlitEnd
}

func (c *Context) LastBlitCycles() uint32 {
	return c.lastBlitCycles
}
