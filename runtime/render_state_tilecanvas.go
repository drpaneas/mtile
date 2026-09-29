//go:build mtile_tilecanvas && !mtile_mode0

package runtime

// Generic private hybrid: ordinary tiled backgrounds plus a sparse OBJ canvas.
// Resource limits come from the hardware/memory plan, never a game identity.
type renderState struct {
	framebuffer                     [FramebufferSize]byte
	canvasTiles                     [objectCanvasTileBytes]byte
	canvasOAM                       [objectCanvasOAMBytes]byte
	canvasCoverage                  [objectCanvasCoverageWords]uint32
	canvasCandidates                [objectCanvasRows]uint16
	canvasRuns                      [objectCanvasRunLimit]uint16
	canvasResult                    objectCanvasPackResult
	canvasRunCount                  int
	canvasLastBG                    int
	mode0MapFullCache               [4]bool
	canvasDrawn, canvasHasPixels    bool
	canvasUploadEstimate            uint32
	canvasCommittedObjects          uint32
	mapRasterReady, mapRasterFailed bool
	mapRasterBG, mapRasterSpanCount int
	mode0Screen                     [4][4096]byte
	mode0TileStage                  [64]byte
	mode0TileDirty                  [120]byte

	mode0Clear       byte
	mode0ClsCalled   bool
	mode0DrawActive  bool
	mode0MapCalls    int
	mode0MapSeen     uint8
	mode0MapReady    [4]bool
	mode0MapDirty    [4]bool
	mode0MapSigSet   [4]bool
	mode0MapCellX    [4]int
	mode0MapCellY    [4]int
	mode0MapTileX    [4]int
	mode0MapTileY    [4]int
	mode0MapCellsW   [4]int
	mode0MapCellsH   [4]int
	mode0MapLayer    [4]byte
	mode0MapSource   [4]uint8
	mode0ScrollX     [4]uint16
	mode0ScrollY     [4]uint16
	mode0ColdCycles  uint32
	mode0UploadStats RendererUploadStats
}

func (c *Context) resetRendererState() { c.renderState = renderState{} }
func (c *Context) sheetMutated(x, y int) {
	runtimePanic("tile canvas requires an immutable spritesheet; use Mode 4")
}
func (c *Context) mapMutated(x, y int, id uint16)      { c.mapLayerMutated(0, x, y, id) }
func (c *Context) drawPaletteMutated()                 {}
func (c *Context) rendererPaletteIndex(index int) byte { return c.displayPal[index] }
func (c *Context) renderSprite(id, x, y, w, h int, flipX, flipY bool) {
	c.canvasMarkSprite(id, x, y, w, h, flipX, flipY)
	c.drawSprite(id, x, y, w, h, flipX, flipY)
}
func (c *Context) hardwarePrint(s, x, y, color Value) (int, bool)       { return 0, false }
func (c *Context) hardwarePrintStatic(s, x, y, color Value) (int, bool) { return 0, false }
func (c *Context) psprOnHardwareOBJ() bool                              { return false }
func (c *Context) mode0PSprPart(id, x, y int, flipX, flipY bool)        {}
func (c *Context) mode0PSpr(site, windowBase, id, x, y int, flipX, flipY bool) {
}
func (c *Context) takeMode0PsprSite(base int) int { return base }
func (c *Context) renderSSpr(sx, sy, sw, sh, dx, dy, dw, dh int, flipX, flipY bool) bool {
	c.canvasMarkSSpr(sx, sy, sw, sh, dx, dy, dw, dh, flipX, flipY)
	return false
}
func (c *Context) RendererUploadStats() RendererUploadStats { return c.mode0UploadStats }
func (c *Context) TileCanvasDMARuns() uint32                { return uint32(c.canvasRunCount) }
func (c *Context) TileCanvasOBJCount() uint32               { return c.canvasCommittedObjects }
