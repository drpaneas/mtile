//go:build mtile_mode0 && mtile_mode0_obj && mtile_mode0_objpack

package runtime

import (
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

const (
	mode0PackedObjectTileBase     = 448
	mode0PackedObjectEntries      = 8
	mode0PackedObjectTileCapacity = 32
	mode0PackedObjectTileEnd      = mode0PackedObjectTileBase + mode0PackedObjectTileCapacity
)

type mode0ObjectPackState struct {
	// Only objpack builds reserve this 2 KiB aligned staging buffer. The
	// thirty-two cell cache is rearranged before VBlank, then each dirty
	// rectangle transfers as one contiguous DMA block.
	mode0PackedObjectStage    [mode0PackedObjectTileCapacity * 16]uint32
	mode0PackedObjectSources  [mode0PackedObjectEntries]uint16
	mode0PackedObjectCells    [mode0PackedObjectEntries]uint8
	mode0PackedObjectCount    uint8
	mode0PackedObjectTiles    uint8
	mode0PackedObjectDirty    uint8
	mode0PackedObjectPrepared uint8
}

func (c *Context) hasMode0PackedUploads() bool { return c.mode0PackedObjectDirty != 0 }

func mode0PackedObjectReservedSource(id int) bool {
	return id >= mode0PackedObjectTileBase && id < mode0PackedObjectTileEnd
}

func mode0PackedObjectWindowReserved(base, count int) bool {
	return count > 0 && base < mode0PackedObjectTileEnd && base+count > mode0PackedObjectTileBase
}

func (c *Context) mode0PackedObjectTile(id, cells int) (int, bool) {
	if c == nil || c.cart == nil || id < 0 || c.cart.SpriteCols <= 0 || cells != 2 && cells != 4 {
		return 0, false
	}
	cols := c.cart.SpriteCols
	lastID := id + (cells-1)*cols + cells - 1
	if id%cols+cells > cols || lastID >= c.cart.SpriteN || lastID >= mode0PackedObjectTileBase {
		return 0, false
	}
	tileOffset := 0
	for slot := 0; slot < int(c.mode0PackedObjectCount); slot++ {
		cachedCells := int(c.mode0PackedObjectCells[slot])
		if int(c.mode0PackedObjectSources[slot]) == id && cachedCells == cells {
			return mode0PackedObjectTileBase + tileOffset, true
		}
		tileOffset += cachedCells * cachedCells
	}
	tiles := cells * cells
	if c.mode0PackedObjectCount >= mode0PackedObjectEntries ||
		int(c.mode0PackedObjectTiles)+tiles > mode0PackedObjectTileCapacity {
		return 0, false
	}
	slot := int(c.mode0PackedObjectCount)
	c.mode0PackedObjectSources[slot] = uint16(id)
	c.mode0PackedObjectCells[slot] = uint8(cells)
	c.mode0PackedObjectCount++
	c.mode0PackedObjectTiles += uint8(tiles)
	c.mode0PackedObjectDirty |= 1 << uint(slot)
	c.mode0PackedObjectPrepared &^= 1 << uint(slot)
	return mode0PackedObjectTileBase + tileOffset, true
}

func (c *Context) dirtyMode0PackedObject(id int) {
	if c == nil || c.cart == nil || id < 0 {
		return
	}
	cols := c.cart.SpriteCols
	for slot := 0; slot < int(c.mode0PackedObjectCount); slot++ {
		source := int(c.mode0PackedObjectSources[slot])
		cells := int(c.mode0PackedObjectCells[slot])
		for row := 0; row < cells; row++ {
			rowStart := source + row*cols
			if id >= rowStart && id < rowStart+cells {
				c.mode0PackedObjectDirty |= 1 << uint(slot)
				c.mode0PackedObjectPrepared &^= 1 << uint(slot)
				break
			}
		}
	}
}

func (c *Context) packedMode0StageTile(index int) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(&c.mode0PackedObjectStage[index*16])), 64)
}

func (c *Context) prepareMode0PackedObjects() {
	if c == nil || c.cart == nil || c.mode0PackedObjectDirty == 0 {
		return
	}
	cols, tileOffset := c.cart.SpriteCols, 0
	for slot := 0; slot < int(c.mode0PackedObjectCount); slot++ {
		cells := int(c.mode0PackedObjectCells[slot])
		mask := uint8(1 << uint(slot))
		if c.mode0PackedObjectDirty&mask != 0 {
			source := int(c.mode0PackedObjectSources[slot])
			for row := 0; row < cells; row++ {
				for col := 0; col < cells; col++ {
					tile := c.packedMode0StageTile(tileOffset + row*cells + col)
					if c.stageMode0Tile(source + row*cols + col) {
						copy(tile, c.mode0TileStage[:])
					} else {
						clear(tile)
					}
				}
			}
			c.mode0PackedObjectPrepared |= mask
		}
		tileOffset += cells * cells
	}
}

func (c *Context) presentMode0PackedObjects() uint32 {
	if c == nil || c.cart == nil || c.mode0PackedObjectDirty == 0 {
		return 0
	}
	var uploaded uint32
	cols := c.cart.SpriteCols
	tileOffset := 0
	for slot := 0; slot < int(c.mode0PackedObjectCount); slot++ {
		cells := int(c.mode0PackedObjectCells[slot])
		mask := uint8(1 << uint(slot))
		if c.mode0PackedObjectDirty&mask != 0 {
			if c.mode0PackedObjectPrepared&mask != 0 {
				start, words := tileOffset*16, cells*cells*16
				hw.CopyWordsToVRAM32DMA(c.mode0PackedObjectStage[start:start+words],
					hw.OBJVRAM+uintptr((mode0PackedObjectTileBase+tileOffset)*64), words)
				uploaded += uint32(words * 4)
				c.mode0PackedObjectDirty &^= mask
				c.mode0PackedObjectPrepared &^= mask
				tileOffset += cells * cells
				continue
			}
			// Direct calls without PrepareRender, or source mutations after
			// preparation, retain the exact original per-cell upload path.
			source := int(c.mode0PackedObjectSources[slot])
			for row := 0; row < cells; row++ {
				for col := 0; col < cells; col++ {
					sourceID := source + row*cols + col
					if !c.stageMode0Tile(sourceID) {
						for index := range c.mode0TileStage {
							c.mode0TileStage[index] = 0
						}
					}
					destination := mode0PackedObjectTileBase + tileOffset + row*cells + col
					hw.CopyToVRAM32DMA(c.mode0TileStage[:], hw.OBJVRAM+uintptr(destination*64), 16)
					uploaded += 64
				}
			}
			c.mode0PackedObjectDirty &^= mask
		}
		tileOffset += cells * cells
	}
	return uploaded
}
