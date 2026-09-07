//go:build !mtile_mode0 && !mtile_tilecanvas

package runtime

// renderState is embedded in Context. Keeping the Mode 4 framebuffer behind a
// build tag makes renderer selection a link-time memory decision, not a hot
// runtime branch.
type renderState struct {
	framebuffer        [FramebufferSize]byte
	mapRasterReady     bool
	mapRasterFailed    bool
	mapRasterBG        int
	mapRasterSpanCount int
}

func (c *Context) resetRendererState()                        { c.invalidateMapRaster() }
func (c *Context) sheetMutated(x, y int)                      { c.invalidateMapRaster() }
func (c *Context) mapMutated(x, y int, id uint16)             { c.invalidateMapRaster() }
func (c *Context) mapLayerMutated(layer, x, y int, id uint16) { c.invalidateMapRaster() }
func (c *Context) flagsMutated(id int)                        {}
func (c *Context) drawPaletteMutated()                        {}

func (c *Context) rendererPaletteIndex(index int) byte {
	return c.displayPal[index]
}

func (c *Context) renderSprite(id, x, y, w, h int, flipX, flipY bool) {
	c.drawSprite(id, x, y, w, h, flipX, flipY)
}

func (c *Context) hardwarePrint(s, x, y, color Value) (int, bool) {
	return 0, false
}

func (c *Context) hardwarePrintStatic(s, x, y, color Value) (int, bool) {
	return 0, false
}

func (c *Context) renderSSpr(sx, sy, sw, sh, dx, dy, dw, dh int, flipX, flipY bool) bool {
	return false
}

func (c *Context) RendererUploadStats() RendererUploadStats {
	return RendererUploadStats{}
}
