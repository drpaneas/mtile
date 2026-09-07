//go:build !mtile_mode0 && !mtile_tilecanvas

package runtime

// Map draws map cells as 8x8 sprite cells from the native uint16 map and
// 8bpp sheet. layer is a flag bitmask: 0 draws every sprite, a nonzero
// value draws a cell only when all bits in layer are present in its flags.
// Sprite 0 is the empty map cell. cellsW or cellsH <= 0
// means "cover the current clip" (30x20 cells on the default 240x160 clip).
// This is a conditional Mode 0 candidate: a tile backend must preserve map
// offsets, layers, streaming, palette, transparency, and call order exactly;
// see docs/render-modes.md.
//
// Full-screen calls can visit 600 cells. mapLayer clips the visible range
// once before walking it.
//
//go:noinline
//go:section .iwram
func (c *Context) Map(cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	c.mapLayer(0, cellX, cellY, screenX, screenY, cellsW, cellsH, layer)
}

// MapBG draws one authored logical background plane. BG0 is the back plane
// and BG3 is the front plane; valid transpiled programs issue explicit calls
// in that order so Mode 4 painter order matches Mode 0 hardware priority.
func (c *Context) MapBG(bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	c.mapLayer(bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer)
}

// MapBGAt is shared with generated code for both renderers. Mode 4 uses
// painter order, so the densely allocated hardware slot is intentionally
// ignored here.
func (c *Context) MapBGAt(_ int, bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	c.mapLayer(bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer)
}

// MapAll draws only build-proven non-empty logical planes, back to front.
func (c *Context) MapAll(mask uint8) {
	for bg := 0; bg < 4; bg++ {
		if mask&(1<<uint(bg)) != 0 {
			c.MapBG(bg, 0, 0, 0, 0, 0, 0, 0)
		}
	}
}

// mapVisibleCellRange intersects a requested cell span with the map and
// screen clip before walking it. A camera far into a large map must not make
// every frame visit all the off-screen cells preceding the visible ones.
func mapVisibleCellRange(cellStart, cells, mapSize, origin, clipStart, clipEnd int) (int, int) {
	first, end := 0, cells
	if first < -cellStart {
		first = -cellStart
	}
	if end > mapSize-cellStart {
		end = mapSize - cellStart
	}
	// Signed shifts round down, including when the first cell starts to the
	// right of the clip. The end uses ceil so a partly visible cell survives.
	if visible := (clipStart - origin) >> 3; first < visible {
		first = visible
	}
	if visible := (clipEnd - origin + 7) >> 3; end > visible {
		end = visible
	}
	return first, end
}

func (c *Context) mapLayer(bg, cellX, cellY, screenX, screenY, cellsW, cellsH, layer int) {
	if c == nil || c.cart == nil {
		return
	}
	if bg < 0 || bg >= c.cart.MapLayers {
		return
	}
	storageBG, ok := mapPlaneSlot(c.cart.MapPlaneMask, bg, c.cart.MapLayers)
	if !ok {
		return
	}
	c.ensureDrawState()
	if c.clipX0 >= c.clipX1 || c.clipY0 >= c.clipY1 {
		return
	}
	if cellsW <= 0 {
		cellsW = (c.clipX1 - c.clipX0 + 7) / 8
		// Account for every cell hidden before the clip, not just a single
		// sub-tile offset, so the default extent still reaches the right edge.
		if hidden := c.clipX0 - (screenX + cellX*8 - c.camX); hidden > 0 {
			cellsW += (hidden + 7) / 8
		}
	}
	if cellsH <= 0 {
		cellsH = (c.clipY1 - c.clipY0 + 7) / 8
		if hidden := c.clipY0 - (screenY + cellY*8 - c.camY); hidden > 0 {
			cellsH += (hidden + 7) / 8
		}
	}
	if cellsW <= 0 || cellsH <= 0 {
		return
	}
	mapW, mapH := c.cart.MapW, c.cart.MapH
	clipX0, clipY0, clipX1, clipY1 := c.clipX0, c.clipY0, c.clipX1, c.clipY1
	baseXOrigin := screenX + cellX*8 - c.camX
	baseYOrigin := screenY + cellY*8 - c.camY
	mx0, mx1 := mapVisibleCellRange(cellX, cellsW, mapW, baseXOrigin, clipX0, clipX1)
	my0, my1 := mapVisibleCellRange(cellY, cellsH, mapH, baseYOrigin, clipY0, clipY1)
	if mx0 >= mx1 || my0 >= my1 {
		return
	}
	mapPlaneN := mapW * mapH
	mapData := c.cart.Map
	mapStreaming := mapStorageStreaming || !mapStorageFixed && c.cart.MapStreaming
	spriteN := c.cart.SpriteN
	flags := c.cart.Flags
	var mask byte
	if layer > 0 && layer <= 255 {
		mask = byte(layer)
	}

	sheet := c.cart.Sheet
	sheetW := c.cart.SheetW
	sheetH := c.cart.SheetH
	cols := c.cart.SpriteCols
	if cols <= 0 {
		return
	}
	if mask == 0 && c.drawPalIdentity && c.transpKey0 &&
		c.drawMapRaster(bg, storageBG, cellX, cellY, mx0, mx1, my0, my1, baseXOrigin, baseYOrigin) {
		return
	}
	// Hoist the id -> cell divide out of the 600-cell loop: one reciprocal
	// multiply per cell instead of two software divides (see spriteCellRecip).
	colsRecip := spriteCellRecip(cols)
	fb := c.framebuffer[:]
	pal := &c.drawPal
	transp := &c.transp
	identity := c.drawPalIdentity
	key0 := c.transpKey0
	canPack := identity && key0 && sheetW&3 == 0 && baseXOrigin&3 == 0

	for my := my0; my < my1; my++ {
		mapY := cellY + my
		baseY := baseYOrigin + my*8
		rowY0 := 0
		rowY1 := 8
		if baseY < clipY0 {
			rowY0 = clipY0 - baseY
		}
		if baseY+8 > clipY1 {
			rowY1 = clipY1 - baseY
		}
		mapRowBase := storageBG*mapPlaneN + mapY*mapW
		streamChunkX := -1
		streamSlot := -1

		for mx := mx0; mx < mx1; mx++ {
			mapX := cellX + mx
			baseX := baseXOrigin + mx*8
			var id uint16
			if mapStreaming {
				// A visible row crosses at most two 32-cell chunks. Resolve
				// each chunk once, then read its cache storage directly. Calling
				// mapStreamingGet here used to scan/touch the cache and probe the
				// edit overlay for every one of ~600 visible cells, making a
				// streamed sparse map about twice as expensive as the same EWRAM
				// map. Loaded chunks already include edits, and Mset updates a
				// resident chunk, so the direct value is authoritative.
				chunkX := mapX / MapChunkWidth
				if chunkX != streamChunkX {
					streamChunkX = chunkX
					streamSlot = c.mapLayerChunkSlot(bg, chunkX, mapY/MapChunkHeight)
					if streamSlot < 0 {
						streamSlot = c.loadMapLayerChunk(bg, chunkX, mapY/MapChunkHeight)
					} else {
						c.touchMapChunk(streamSlot)
					}
				}
				if streamSlot < 0 {
					continue
				}
				localX := mapX % MapChunkWidth
				localY := mapY % MapChunkHeight
				id = mapData[streamSlot*MapChunkCells+localY*MapChunkWidth+localX]
			} else {
				id = mapData[mapRowBase+mapX]
			}
			if id == 0 || int(id) >= spriteN {
				continue
			}
			if mask != 0 && flags[id]&mask != mask {
				continue
			}
			srcX0, srcY0 := 0, 0
			blitSheet := sheet
			blitSheetW := sheetW
			if sheetStorageReadOnly || c.cart.SheetReadOnly {
				if int(id) >= len(c.cart.SheetIndex) {
					continue
				}
				slot := c.cart.SheetIndex[id]
				if slot == 0 {
					if key0 {
						continue
					}
					blitSheet = emptySpriteCell[:]
					blitSheetW = 8
					srcY0 = 0
				} else {
					blitSheetW = 8
					srcY0 = (int(slot) - 1) * 8
				}
			} else {
				srcCellX, srcCellY := spriteCellSplit(int(id), cols, colsRecip)
				srcX0 = srcCellX * 8
				srcY0 = srcCellY * 8
				if srcX0+8 > sheetW || srcY0+8 > sheetH {
					continue
				}
			}
			colX0, colX1 := 0, 8
			if baseX < clipX0 {
				colX0 = clipX0 - baseX
			}
			if baseX+8 > clipX1 {
				colX1 = clipX1 - baseX
			}
			packed := canPack && colX0 == 0 && colX1 == 8
			blitUnflippedCell8(blitSheet, fb, pal, transp, blitSheetW, srcX0, srcY0, baseX, baseY,
				rowY0, rowY1, colX0, colX1, identity, key0, packed)
		}
	}
}
