//go:build !mtile_tilecanvas

package runtime

// The diagnostic renderer is deliberately absent from public selection. This
// constant removes its map interception entirely from ordinary Mode 4 builds.
const tileCanvasExperiment = false

func (c *Context) TileCanvasDMARuns() uint32 { return 0 }

func (c *Context) markCanvasRect(x0, y0, x1, y1 int) {}

func (c *Context) markCanvasPixel(x, y int) {}

// Mode 0 shares tiled_map.go with the diagnostic tile canvas. Its guarded
// call still needs a declaration when the tile-canvas implementation is absent.
func (c *Context) checkCanvasMap(slot, bg int) {} //nolint:unused // Used under mtile_mode0.
