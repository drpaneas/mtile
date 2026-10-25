package runtime

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

// mapCursor reads cells of one logical plane during a draw. Fixed maps read
// storage directly; streamed maps resolve each 32x32 chunk once, as mapLayer
// does, instead of paying a full cache lookup for every cell.
type mapCursor struct {
	bg             int
	planeBase      int
	chunkX, chunkY int
	slot           int
}

func (c *Context) newMapCursor(bg int) mapCursor {
	cur := mapCursor{bg: bg, planeBase: -1, chunkX: -1, chunkY: -1, slot: -1}
	if !mapStorageStreaming && (mapStorageFixed || !c.cart.MapStreaming) {
		if storageBG, ok := mapPlaneSlot(c.cart.MapPlaneMask, bg, c.cart.MapLayers); ok {
			cur.planeBase = storageBG * c.cart.MapW * c.cart.MapH
		}
	}
	return cur
}

// mapCursorCell returns the cell at an in-map coordinate, or 0 when its
// plane has no storage or its chunk cannot load.
func (c *Context) mapCursorCell(cur *mapCursor, x, y int) uint16 {
	if cur.planeBase >= 0 {
		return c.cart.Map[cur.planeBase+y*c.cart.MapW+x]
	}
	if !mapStorageStreaming && (mapStorageFixed || !c.cart.MapStreaming) {
		return 0
	}
	chunkX, chunkY := x/MapChunkWidth, y/MapChunkHeight
	if chunkX != cur.chunkX || chunkY != cur.chunkY {
		cur.chunkX, cur.chunkY = chunkX, chunkY
		cur.slot = c.mapLayerChunkSlot(cur.bg, chunkX, chunkY)
		if cur.slot < 0 {
			cur.slot = c.loadMapLayerChunk(cur.bg, chunkX, chunkY)
		} else {
			c.touchMapChunk(cur.slot)
		}
	}
	if cur.slot < 0 {
		return 0
	}
	// Loaded chunks include sparse edits, and Mset updates resident chunks.
	return c.cart.Map[cur.slot*MapChunkCells+(y%MapChunkHeight)*MapChunkWidth+x%MapChunkWidth]
}
