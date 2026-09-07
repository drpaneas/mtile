//go:build mtile_mode0 || mtile_tilecanvas

package runtime

import "unsafe"

type mapLayerPatchView struct {
	screen                 *uint16
	dirty, signature       *bool
	minX, maxX, minY, maxY int
	tileBiasX, tileBiasY   int
	flags                  byte
	snapshot               bool
}

type mapLayerViewRenderState struct {
	patches  [mode0LayerCount]mapLayerPatchView
	count    int
	flags    *byte
	fallback bool
}

func (view *MapLayerView) borrowRenderState(c *Context, layer int) {
	if len(c.cart.Flags) < c.cart.SpriteN {
		view.fallback = true
		return
	}
	if len(c.cart.Flags) > 0 {
		view.flags = &c.cart.Flags[0]
	}
	for slot := 0; slot < mode0LayerCount; slot++ {
		if int(c.mode0MapSource[slot]) != layer || !c.mode0MapSigSet[slot] {
			continue
		}
		base := unsafe.Pointer(&c.mode0Screen[slot][0])
		if !littleEndianNumberWords || uintptr(base)&1 != 0 {
			// Normal shadows are aligned. Retain the byte-oriented hook if an
			// alternate renderer/storage layout ever supplies an odd address.
			view.fallback = true
			return
		}
		patch := &view.patches[view.count]
		view.count++
		patch.screen, patch.dirty, patch.signature = (*uint16)(base), &c.mode0MapDirty[slot], &c.mode0MapSigSet[slot]
		patch.snapshot = c.mode0DrawActive && c.mode0MapSeen&(1<<uint(slot)) != 0
		patch.flags = c.mode0MapLayer[slot]
		cellX, cellY := c.mode0MapCellX[slot], c.mode0MapCellY[slot]
		tileX, tileY := c.mode0MapTileX[slot], c.mode0MapTileY[slot]
		patch.minX, patch.maxX = cellX, cellX+c.mode0MapCellsW[slot]
		patch.minY, patch.maxY = cellY, cellY+c.mode0MapCellsH[slot]
		patch.tileBiasX, patch.tileBiasY = tileX-cellX, tileY-cellY
		if !c.mode0MapFullCache[slot] {
			// Existing overlap conditions are screenX+8>clipX0 and
			// screenX<=clipX1. Arithmetic shifts give their exact tile bounds,
			// including negative positions and partially visible guard cells.
			x0 := cellX + ((c.clipX0 + int(c.mode0ScrollX[slot])) >> 3) - tileX
			x1 := cellX + ((c.clipX1 + int(c.mode0ScrollX[slot])) >> 3) - tileX + 1
			y0 := cellY + ((c.clipY0 + int(c.mode0ScrollY[slot])) >> 3) - tileY
			y1 := cellY + ((c.clipY1 + int(c.mode0ScrollY[slot])) >> 3) - tileY + 1
			patch.minX, patch.maxX = max(patch.minX, x0), min(patch.maxX, x1)
			patch.minY, patch.maxY = max(patch.minY, y0), min(patch.maxY, y1)
		}
	}
}

// The strict loop proof keeps these rectangle/binding snapshots stable. Flag
// values and signature bits stay live because fset may change them mid-loop.
// This helper exists only in Mode0 builds, leaving Mode4's IWRAM unchanged.
//
//go:noinline
//go:section .iwram
func (view *MapLayerView) mapLayerViewChanged(x, y int, id uint16) {
	if view.fallback {
		view.context.mapLayerMutated(view.layer, x, y, id)
		return
	}
	for i := 0; i < view.count; i++ {
		patch := &view.patches[i]
		if patch.snapshot {
			*patch.signature = false
			continue
		}
		if !*patch.signature || x < patch.minX || x >= patch.maxX || y < patch.minY || y >= patch.maxY {
			continue
		}
		value := id
		if id == 0 || id >= mode0TileLimit || patch.flags != 0 &&
			*(*byte)(unsafe.Add(unsafe.Pointer(view.flags), uintptr(id)))&patch.flags != patch.flags {
			value = 0
		}
		entry := mode0ScreenEntry(x+patch.tileBiasX, y+patch.tileBiasY)
		pointer := (*uint16)(unsafe.Add(unsafe.Pointer(patch.screen), entry*2))
		if *pointer != value {
			*pointer = value
			*patch.dirty = true
		}
	}
}
