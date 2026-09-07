//go:build !mtile_tilecanvas

package runtime

// The diagnostic renderer is deliberately absent from public selection. This
// constant removes its map interception entirely from ordinary Mode 4 builds.
const tileCanvasExperiment = false

func (c *Context) TileCanvasDMARuns() uint32 { return 0 }

func (c *Context) refreshCanvasSpriteFlags() {}

func (c *Context) checkCanvasMap(slot, bg int)      {}
func (c *Context) markCanvasPixelOffset(offset int) {}

func (c *Context) tileCanvasMap(bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {}

func (c *Context) markCanvasRect(x0, y0, x1, y1 int)        {}
func (c *Context) canvasDirectRear() bool                   { return false }
func (c *Context) canvasRearSpan(y, x0, x1 int, color byte) {}
func (c *Context) canvasRearPixel(x, y int, color byte)     {}

func (c *Context) markCanvasPixel(x, y int) {}
