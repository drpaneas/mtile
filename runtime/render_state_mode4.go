//go:build !mtile_mode0 && !mtile_tilecanvas

package runtime

// framebufferInVRAM selects VRAM-safe pixel stores: Mode 4 draws directly
// into the hidden display page.
const framebufferInVRAM = true

// renderState is embedded in Context. Keeping the Mode 4 page state behind a
// build tag makes renderer selection a link-time memory decision, not a hot
// runtime branch.
type renderState struct {
	mapPackedReady     [4]bool
	mapPackedFailed    [4]bool
	mapPackedSpanCount [4]int
	// framebuffer views the page that holds the logical image. Drawing goes
	// to the hidden page; after a flip the logical image is the visible page
	// (pageStale) until the next write copies it back or cls replaces it.
	framebuffer        []byte
	pages              mode4PageStorage
	pageFront          uint8
	pageStale          bool
	pageDirty          bool
	mapRasterReady     bool
	mapRasterFailed    bool
	mapRasterBG        int
	mapRasterSpanCount int
}

func (c *Context) resetRendererState()            { c.invalidateMapRaster(); c.invalidateMapPacked(-1) }
func (c *Context) sheetMutated(x, y int)          { c.invalidateMapRaster(); c.invalidateMapPacked(-1) }
func (c *Context) mapMutated(x, y int, id uint16) { c.invalidateMapRaster(); c.invalidateMapPacked(0) }
func (c *Context) mapLayerMutated(layer, x, y int, id uint16) {
	c.invalidateMapRaster()
	c.invalidateMapPacked(layer)
}
func (c *Context) flagsMutated(id int) {}
func (c *Context) drawPaletteMutated() {}

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

func (c *Context) psprOnHardwareOBJ() bool { return false }

func (c *Context) mode0PSprPart(id, x, y int, flipX, flipY bool) {}

func (c *Context) mode0PSpr(site, windowBase, id, x, y int, flipX, flipY bool) {}

func (c *Context) takeMode0PsprSite(base int) int { return base }

func (c *Context) renderSSpr(sx, sy, sw, sh, dx, dy, dw, dh int, flipX, flipY bool) bool {
	return false
}

func (c *Context) RendererUploadStats() RendererUploadStats {
	return RendererUploadStats{}
}
