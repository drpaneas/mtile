//go:build mtile_mode0 && !mtile_mode0_sheet_immutable

package runtime

import (
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

const mode0SheetImmutable = false

// The renderer proof admits at most sixteen sset calls per update. Store
// their row-major-to-tile conversion before VBlank, leaving only DMA in the
// visible commit. This storage is absent from immutable-sheet cartridges.
const mode0PreparedTileCapacity = 16

type mode0SheetUploadState struct {
	mode0PreparedTileWords [mode0PreparedTileCapacity][16]uint32
	mode0PreparedTileIDs   [mode0PreparedTileCapacity]uint16
	mode0PreparedTileCount int
	mode0TilesPrepared     bool
	mode0TilesOverflow     bool
}

func (c *Context) preparedMode0Tile(index int) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(&c.mode0PreparedTileWords[index])), 64)
}

func (c *Context) invalidateMode0TileUploads() {
	c.mode0TilesPrepared = false
}

func (c *Context) prepareMode0TileUploads() {
	if c == nil || c.cart == nil {
		return
	}
	c.mode0PreparedTileCount = 0
	c.mode0TilesOverflow = false
	limit := min(c.cart.SpriteN, mode0TileLimit)
	for id := 0; id < limit; id++ {
		if c.mode0TileDirty[id>>3]&(1<<uint(id&7)) == 0 {
			continue
		}
		if c.mode0PreparedTileCount == mode0PreparedTileCapacity {
			// Direct runtime users and force-blank initialization can exceed
			// the source proof. Preserve their uploads through the old path.
			c.mode0TilesOverflow = true
			break
		}
		index := c.mode0PreparedTileCount
		c.mode0PreparedTileIDs[index] = uint16(id)
		tile := c.preparedMode0Tile(index)
		if c.stageMode0Tile(id) {
			copy(tile, c.mode0TileStage[:])
		} else {
			clear(tile)
		}
		c.mode0PreparedTileCount++
	}
	c.mode0TilesPrepared = true
}

func (c *Context) uploadMode0DirtyTiles() (uint32, uint32) {
	if c == nil || c.cart == nil {
		return 0, 0
	}
	if !c.mode0TilesPrepared {
		return c.uploadMode0DirtyTilesUnprepared()
	}
	var bgUploaded, objUploaded uint32
	for index := 0; index < c.mode0PreparedTileCount; index++ {
		id := int(c.mode0PreparedTileIDs[index])
		tile := c.preparedMode0Tile(index)
		c.mode0TileDirty[id>>3] &^= 1 << uint(id&7)
		if id != 0 {
			hw.CopyToVRAM32DMA(tile, hw.VRAM+uintptr(id*64), 16)
			bgUploaded += 64
		}
		objUploaded += c.uploadMode0ObjectTile(id, tile)
	}
	c.mode0TilesPrepared = false
	c.mode0PreparedTileCount = 0
	if c.mode0TilesOverflow {
		bg, obj := c.uploadMode0DirtyTilesUnprepared()
		bgUploaded += bg
		objUploaded += obj
	}
	return bgUploaded, objUploaded
}

// Initialization runs force-blanked and can modify the complete sheet. Keep
// this exact unbounded path for it, direct PresentRender calls without a
// prepare phase, and mutations occurring after a prepared snapshot.
func (c *Context) uploadMode0DirtyTilesUnprepared() (uint32, uint32) {
	var bgUploaded, objUploaded uint32
	limit := min(c.cart.SpriteN, mode0TileLimit)
	for id := 0; id < limit; id++ {
		mask := byte(1 << uint(id&7))
		if c.mode0TileDirty[id>>3]&mask == 0 {
			continue
		}
		c.mode0TileDirty[id>>3] &^= mask
		if !c.stageMode0Tile(id) {
			clear(c.mode0TileStage[:])
		}
		if id != 0 {
			hw.CopyToVRAM32DMA(c.mode0TileStage[:], hw.VRAM+uintptr(id*64), 16)
			bgUploaded += 64
		}
		objUploaded += c.uploadMode0ObjectTile(id, c.mode0TileStage[:])
	}
	return bgUploaded, objUploaded
}
