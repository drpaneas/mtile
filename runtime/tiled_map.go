//go:build mtile_mode0 || mtile_tilecanvas

package runtime

import "github.com/drpaneas/mtile/runtime/hw"

func mode0LayerScreenBase(layer int) uintptr {
	return hw.VRAM + uintptr((30-layer*2)*2048)
}

func mode0LayerControl(layer int) uint16 {
	// Later map() calls are in front, matching Mode 4 painter order.
	priority := mode0LayerCount - 1 - layer
	return uint16(priority) | 0x0080 | uint16(30-layer*2)<<8 | 0x4000
}

func (c *Context) stageMode0Tile(id int) bool {
	if c == nil || c.cart == nil || id < 0 || id >= c.cart.SpriteN {
		return false
	}
	if sheetStorageReadOnly || c.cart.SheetReadOnly {
		if id >= len(c.cart.SheetIndex) {
			return false
		}
		slot := int(c.cart.SheetIndex[id])
		if slot == 0 {
			for i := range c.mode0TileStage {
				c.mode0TileStage[i] = 0
			}
			return false
		}
		start := (slot - 1) * 64
		if start < 0 || start+64 > len(c.cart.Sheet) {
			return false
		}
		copy(c.mode0TileStage[:], c.cart.Sheet[start:start+64])
		return true
	}
	cols := c.cart.SpriteCols
	if cols <= 0 {
		return false
	}
	cellX, cellY := id%cols, id/cols
	startX, startY := cellX*8, cellY*8
	if startX+8 > c.cart.SheetW || startY+8 > c.cart.SheetH {
		return false
	}
	for row := 0; row < 8; row++ {
		src := (startY+row)*c.cart.SheetW + startX
		copy(c.mode0TileStage[row*8:row*8+8], c.cart.Sheet[src:src+8])
	}
	return true
}

func positiveMod(value, modulus int) int {
	result := value % modulus
	if result < 0 {
		result += modulus
	}
	return result
}

// mode0ScreenEntry implements the GBA's 64x32 regular-background block
// layout: the right half lives in the second 32x32 screen block.
func mode0ScreenEntry(x, y int) int {
	// Both dimensions are powers of two: masking is positive modulo even
	// for negative tile positions, without signed remainder correction.
	return ((x & 32) << 5) + ((y & 31) << 5) + (x & 31)
}

// Map preserves the Mode 4 finite-map contract. Each statically proven call
// owns one hardware BG. Safe finite rectangles are cached in full so camera
// movement only updates scroll registers; displaced windows use a viewport cache.
func (c *Context) Map(cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	if c == nil {
		return
	}
	mapIndex := c.mode0MapCalls
	c.mapLayerAt(mapIndex, 0, cellX, cellY, screenX, screenY, cellsW, cellsH, layer)
	c.drawTileLinks(0, cellX, cellY, cellsW, cellsH, screenX, screenY, layer)
}

// MapBG binds one authored plane to its matching logical hardware BG slot.
// The transpiler proves stable BG IDs and ordering before selecting Mode 0.
func (c *Context) MapBG(bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	c.mapLayerAt(bg, bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer)
	c.drawTileLinks(bg, cellX, cellY, cellsW, cellsH, screenX, screenY, layer)
}

// MapBGAt binds a stable logical source plane to a densely allocated hardware
// BG slot. Empty gaps in BG0..BG3 therefore consume neither a screen shadow
// nor a hardware background.
func (c *Context) MapBGAt(slot, bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	if slot < 0 {
		return
	}
	c.mapLayerAt(slot, bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer)
	c.drawTileLinks(bg, cellX, cellY, cellsW, cellsH, screenX, screenY, layer)
}

// MapAll materializes the selected non-empty logical planes in BG order while
// packing them into consecutive hardware slots.
func (c *Context) MapAll(mask uint8) {
	slot := 0
	for bg := 0; bg < 4; bg++ {
		if mask&(1<<uint(bg)) == 0 {
			continue
		}
		c.MapBGAt(slot, bg, 0, 0, 0, 0, 0, 0, 0)
		slot++
	}
}

func (c *Context) mapLayerAt(mapIndex, sourceBG, cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	if c == nil || c.cart == nil {
		return
	}
	if tileCanvasExperiment {
		c.checkCanvasMap(mapIndex, sourceBG)
	}
	if sourceBG < 0 || sourceBG >= c.cart.MapLayers {
		return
	}
	c.ensureDrawState()
	// A size of zero covers the clip. Mode 4 counts every cell hidden before
	// the clip too; that window outgrows the 64x32 hardware screen once the
	// camera moves far into a wide or tall map. Start at the first visible
	// cell instead when needed. Cells keep their positions, because cell N is
	// always drawn at screen + N*8, so the visible image is unchanged.
	if cellsW <= 0 {
		cellsW = (c.clipX1 - c.clipX0 + 7) / 8
		if hidden := c.clipX0 - (screenX + cellX*8 - c.camX); hidden > 0 {
			cellsW += (hidden + 7) / 8
			if cellsW > 64 {
				cellX += hidden >> 3
				cellsW = (c.clipX1 - c.clipX0 + hidden&7 + 7) / 8
			}
		}
	}
	if cellsH <= 0 {
		cellsH = (c.clipY1 - c.clipY0 + 7) / 8
		if hidden := c.clipY0 - (screenY + cellY*8 - c.camY); hidden > 0 {
			cellsH += (hidden + 7) / 8
			if cellsH > 32 {
				cellY += hidden >> 3
				cellsH = (c.clipY1 - c.clipY0 + hidden&7 + 7) / 8
			}
		}
	}
	if cellsW <= 0 || cellsH <= 0 || cellsW > 64 || cellsH > 32 {
		if tileCanvasExperiment && (cellsW > 64 || cellsH > 32) {
			runtimePanic("tile canvas map rectangle exceeds 64x32 cells; use Mode 4")
		}
		return
	}
	if mapIndex >= mode0LayerCount {
		return
	}
	if c.tryMode0FullMap(mapIndex, sourceBG, cellX, cellY, screenX, screenY, cellsW, cellsH, layer) {
		return
	}
	c.mode0MapCalls++
	c.mode0MapSeen |= 1 << uint(mapIndex)

	originX := screenX + cellX*8 - c.camX
	originY := screenY + cellY*8 - c.camY
	scrollX := positiveMod(-originX, 8)
	scrollY := positiveMod(-originY, 8)
	tileX := (originX + scrollX) / 8
	tileY := (originY + scrollY) / 8
	c.mode0ScrollX[mapIndex] = uint16(scrollX)
	c.mode0ScrollY[mapIndex] = uint16(scrollY)

	var mask byte
	if layer > 0 && layer <= 255 {
		mask = byte(layer)
	}
	if c.mode0MapSigSet[mapIndex] && c.mode0MapCellX[mapIndex] == cellX && c.mode0MapCellY[mapIndex] == cellY &&
		c.mode0MapTileX[mapIndex] == tileX && c.mode0MapTileY[mapIndex] == tileY &&
		c.mode0MapCellsW[mapIndex] == cellsW && c.mode0MapCellsH[mapIndex] == cellsH &&
		c.mode0MapLayer[mapIndex] == mask && c.mode0MapSource[mapIndex] == uint8(sourceBG) {
		return
	}

	screen := c.mode0Screen[mapIndex][:]
	// The Context screen shadows are aligned. DMA3 replaces a 4 KiB byte
	// clear every time a scrolling background crosses a tile boundary.
	hw.Fill32(screen, 0, len(screen)/4)
	mapW, mapH := c.cart.MapW, c.cart.MapH
	storageLayer, present := mapPlaneSlot(c.cart.MapPlaneMask, sourceBG, c.cart.MapLayers)
	streaming := mapStorageStreaming || !mapStorageFixed && c.cart.MapStreaming
	mapData, flags, spriteN := c.cart.Map, c.cart.Flags, c.cart.SpriteN
	linkOf := c.cart.TileLinkOf
	mapPlaneOffset := storageLayer * mapW * mapH
	// Keep the cell exactly on the right/bottom clip boundary as a guard.
	// A one-pixel pan can reveal it without changing the cached tile placement.
	// Signed shifts implement floor division, including negative origins.
	mx0 := max(0, -cellX, (c.clipX0-originX)>>3)
	my0 := max(0, -cellY, (c.clipY0-originY)>>3)
	mx1 := min(cellsW, mapW-cellX, ((c.clipX1-originX)>>3)+1)
	my1 := min(cellsH, mapH-cellY, ((c.clipY1-originY)>>3)+1)
	if present {
		cur := c.newMapCursor(sourceBG)
		for my := my0; my < my1; my++ {
			mapY := cellY + my
			mapRow := mapPlaneOffset + mapY*mapW + cellX
			screenRow := ((tileY + my) & 31) << 5
			for mx := mx0; mx < mx1; mx++ {
				var id uint16
				if streaming {
					id = c.mapCursorCell(&cur, cellX+mx, mapY)
				} else {
					id = mapData[mapRow+mx]
				}
				if id == 0 || int(id) >= spriteN || id >= mode0TileLimit || int(id) < len(linkOf) && linkOf[id] != 0 {
					continue
				}
				if mask != 0 && flags[id]&mask != mask {
					continue
				}
				x := tileX + mx
				entry := (((x & 32) << 5) + screenRow + (x & 31)) * 2
				screen[entry], screen[entry+1] = byte(id), byte(id>>8)
			}
		}
	}
	c.mode0MapCellX[mapIndex], c.mode0MapCellY[mapIndex] = cellX, cellY
	c.mode0MapTileX[mapIndex], c.mode0MapTileY[mapIndex] = tileX, tileY
	c.mode0MapCellsW[mapIndex], c.mode0MapCellsH[mapIndex] = cellsW, cellsH
	c.mode0MapLayer[mapIndex] = mask
	c.mode0MapSource[mapIndex] = uint8(sourceBG)
	c.mode0MapSigSet[mapIndex] = true
	c.mode0MapReady[mapIndex] = true
	c.mode0MapDirty[mapIndex] = true
}
