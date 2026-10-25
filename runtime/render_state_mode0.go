//go:build mtile_mode0

package runtime

// framebufferInVRAM is false: this renderer's software framebuffer is RAM.
const framebufferInVRAM = false

// renderState is the entire Mode 0 renderer allocation. framebuffer is a nil
// compatibility slice so common, statically unreachable Mode 4 primitives can
// still compile; unlike Mode 4 it contributes no 38,400-byte EWRAM backing
// store. mode0Screen mirrors two adjacent 32x32 regular-screen blocks.
type renderState struct {
	mode0SheetUploadState
	framebuffer []byte
	mode0ObjectState

	mode0Screen    mode0ScreenStorage
	mode0TileStage [64]byte
	mode0TileDirty mode0TileDirtyStorage

	mode0Clear        byte
	mode0ClsCalled    bool
	mode0DrawActive   bool
	mode0MapCalls     int
	mode0MapSeen      uint8
	mode0MapReady     [mode0LayerStorageCount]bool
	mode0MapDirty     [mode0LayerStorageCount]bool
	mode0MapSigSet    [mode0LayerStorageCount]bool
	mode0MapFullCache [mode0LayerStorageCount]bool
	mode0MapCellX     [mode0LayerStorageCount]int
	mode0MapCellY     [mode0LayerStorageCount]int
	mode0MapTileX     [mode0LayerStorageCount]int
	mode0MapTileY     [mode0LayerStorageCount]int
	mode0MapCellsW    [mode0LayerStorageCount]int
	mode0MapCellsH    [mode0LayerStorageCount]int
	mode0MapLayer     [mode0LayerStorageCount]byte
	mode0MapSource    [mode0LayerStorageCount]uint8
	mode0ScrollX      [mode0LayerStorageCount]uint16
	mode0ScrollY      [mode0LayerStorageCount]uint16
	mode0ColdCycles   uint32
	mode0UploadStats  RendererUploadStats
}

func (c *Context) resetRendererState() {
	c.renderState = renderState{}
}

func (c *Context) sheetMutated(x, y int) {
	if c == nil || c.cart == nil || x < 0 || y < 0 {
		return
	}
	id := (y/8)*c.cart.SpriteCols + x/8
	c.invalidateMode0TileUploads()
	c.dirtyMode0PackedObject(id)
	c.mode0ObjectTileMutated(id)
	if id < 0 || id >= mode0TileLimit {
		return
	}
	c.mode0TileDirty[id>>3] |= byte(1 << uint(id&7))
}

func (c *Context) mapMutated(x, y int, id uint16) {
	c.mapLayerMutated(0, x, y, id)
}

func (c *Context) drawPaletteMutated() {
	c.paletteDirty = true
}

func (c *Context) rendererPaletteIndex(index int) byte {
	return c.displayPal[c.drawPal[index]]
}

func (c *Context) psprOnHardwareOBJ() bool { return true }

func (c *Context) RendererUploadStats() RendererUploadStats {
	if c == nil {
		return RendererUploadStats{}
	}
	return c.mode0UploadStats
}

// Mode 0 has no software framebuffer; these keep the common Mode 4
// primitives compiling and are statically unreachable.
func (c *Context) ensureFramebuffer()          {}
func (c *Context) writableFramebuffer() []byte { return c.framebuffer }
func (c *Context) clearTarget() []byte         { return c.framebuffer }
