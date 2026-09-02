//go:build mtile_mode0

package runtime

// renderState is the entire Mode 0 renderer allocation. framebuffer is a nil
// compatibility slice so common, statically unreachable Mode 4 primitives can
// still compile; unlike Mode 4 it contributes no 38,400-byte EWRAM backing
// store. mode0Screen mirrors two adjacent 32x32 regular-screen blocks.
type renderState struct {
	framebuffer []byte
	mode0ObjectState

	mode0Screen    mode0ScreenStorage
	mode0TileStage [64]byte
	mode0TileDirty mode0TileDirtyStorage

	mode0Clear       byte
	mode0ClsCalled   bool
	mode0DrawActive  bool
	mode0MapCalls    int
	mode0MapSeen     uint8
	mode0MapReady    [mode0LayerStorageCount]bool
	mode0MapDirty    [mode0LayerStorageCount]bool
	mode0MapSigSet   [mode0LayerStorageCount]bool
	mode0MapCellX    [mode0LayerStorageCount]int
	mode0MapCellY    [mode0LayerStorageCount]int
	mode0MapTileX    [mode0LayerStorageCount]int
	mode0MapTileY    [mode0LayerStorageCount]int
	mode0MapCellsW   [mode0LayerStorageCount]int
	mode0MapCellsH   [mode0LayerStorageCount]int
	mode0MapLayer    [mode0LayerStorageCount]byte
	mode0MapSource   [mode0LayerStorageCount]uint8
	mode0ScrollX     [mode0LayerStorageCount]uint16
	mode0ScrollY     [mode0LayerStorageCount]uint16
	mode0ColdCycles  uint32
	mode0UploadStats RendererUploadStats
}

func (c *Context) resetRendererState() {
	c.renderState = renderState{}
}

func (c *Context) sheetMutated(x, y int) {
	if c == nil || c.cart == nil || x < 0 || y < 0 {
		return
	}
	id := (y/8)*c.cart.SpriteCols + x/8
	c.dirtyMode0PackedObject(id)
	c.mode0ObjectTileMutated(id)
	if id < 0 || id >= mode0TileLimit {
		return
	}
	c.mode0TileDirty[id>>3] |= byte(1 << uint(id&7))
}

func (c *Context) mapMutated(x, y int) {
	c.mapLayerMutated(0, x, y)
}

func (c *Context) mapLayerMutated(sourceBG, x, y int) {
	if c == nil || c.cart == nil {
		return
	}
	id := c.mapGetLayer(sourceBG, x, y)
	for layer := 0; layer < mode0LayerCount; layer++ {
		if int(c.mode0MapSource[layer]) != sourceBG {
			continue
		}
		if c.mode0DrawActive && c.mode0MapSeen&(1<<uint(layer)) != 0 {
			// Mode 4 has already rasterized this layer for the pending frame.
			// Keep that snapshot and force the next map() to rematerialize.
			c.mode0MapSigSet[layer] = false
			continue
		}
		c.patchMode0MapCell(layer, x, y, id)
	}
}

func (c *Context) flagsMutated(id int) {
	if c == nil || c.cart == nil || id <= 0 || id >= c.cart.SpriteN {
		return
	}
	for layer := 0; layer < mode0LayerCount; layer++ {
		if !c.mode0MapSigSet[layer] || c.mode0MapLayer[layer] == 0 {
			continue
		}
		if c.mode0DrawActive && c.mode0MapSeen&(1<<uint(layer)) != 0 {
			// As with mset(), a draw-time flag change belongs to the next
			// frame once this layer has already been materialized.
			c.mode0MapSigSet[layer] = false
			continue
		}
		for my := 0; my < c.mode0MapCellsH[layer]; my++ {
			mapY := c.mode0MapCellY[layer] + my
			if mapY < 0 || mapY >= c.cart.MapH {
				continue
			}
			for mx := 0; mx < c.mode0MapCellsW[layer]; mx++ {
				mapX := c.mode0MapCellX[layer] + mx
				if mapX < 0 || mapX >= c.cart.MapW || int(c.mapGetLayer(int(c.mode0MapSource[layer]), mapX, mapY)) != id {
					continue
				}
				c.patchMode0MapCell(layer, mapX, mapY, uint16(id))
			}
		}
	}
}

// patchMode0MapCell updates one already-materialized BG entry. Mutations no
// longer invalidate the entire 64x32 shadow: a later map() still rebuilds when
// its viewport/signature changes, while stable visible windows upload only
// when the affected entry's composed tile value actually changes.
func (c *Context) patchMode0MapCell(layer, mapX, mapY int, id uint16) {
	if layer < 0 || layer >= mode0LayerCount || !c.mode0MapSigSet[layer] {
		return
	}
	mx := mapX - c.mode0MapCellX[layer]
	my := mapY - c.mode0MapCellY[layer]
	if mx < 0 || my < 0 || mx >= c.mode0MapCellsW[layer] || my >= c.mode0MapCellsH[layer] {
		return
	}
	screenX := (c.mode0MapTileX[layer]+mx)*8 - int(c.mode0ScrollX[layer])
	screenY := (c.mode0MapTileY[layer]+my)*8 - int(c.mode0ScrollY[layer])
	// Boundary guard cells can become visible after a sub-tile scroll without
	// rebuilding the cached map, so update-phase mutations must patch them too.
	if screenX+8 <= c.clipX0 || screenX > c.clipX1 || screenY+8 <= c.clipY0 || screenY > c.clipY1 {
		return
	}
	value := id
	mask := c.mode0MapLayer[layer]
	if id == 0 || int(id) >= c.cart.SpriteN || id >= mode0TileLimit || mask != 0 && c.cart.Flags[id]&mask != mask {
		value = 0
	}
	entry := mode0ScreenEntry(c.mode0MapTileX[layer]+mx, c.mode0MapTileY[layer]+my) * 2
	before := uint16(c.mode0Screen[layer][entry]) | uint16(c.mode0Screen[layer][entry+1])<<8
	if before == value {
		return
	}
	c.mode0Screen[layer][entry] = byte(value)
	c.mode0Screen[layer][entry+1] = byte(value >> 8)
	c.mode0MapDirty[layer] = true
}

func (c *Context) drawPaletteMutated() {
	c.paletteDirty = true
}

func (c *Context) rendererPaletteIndex(index int) byte {
	return c.displayPal[c.drawPal[index]]
}

func (c *Context) RendererUploadStats() RendererUploadStats {
	if c == nil {
		return RendererUploadStats{}
	}
	return c.mode0UploadStats
}
