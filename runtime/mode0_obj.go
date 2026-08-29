//go:build mtile_mode0 && mtile_mode0_obj

package runtime

import "github.com/drpaneas/mtile/runtime/hw"

const (
	mode0ObjectTileLimit = 512
	mode0ObjectLimit     = 32 // hard worst-case GBA objects-per-scanline budget
	mode0ObjectDisable   = 0x0200
)

type mode0ObjectState struct {
	mode0TextState
	mode0OAM          [hw.OAMBytes]byte
	mode0ObjectCount  int
	mode0PreviousObjs int
	mode0OAMDirty     bool
}

func mode0ObjectDisplayBits() uint16 {
	return 0x1040 // OBJ_ON | one-dimensional OBJ tile mapping
}

func mode0PutOAM16(data *[hw.OAMBytes]byte, offset int, value uint16) {
	data[offset] = byte(value)
	data[offset+1] = byte(value >> 8)
}

func (c *Context) applyMode0ObjectPalette() {
	for index := 0; index < 256; index++ {
		hw.SetPaletteRAM16(256+index, c.palette[c.rendererPaletteIndex(index)])
	}
}

func (c *Context) renderSSpr(sx, sy, sw, sh, dx, dy, dw, dh int, flipX, flipY bool) bool {
	if sx < 0 || sy < 0 || sx&7 != 0 || sy&7 != 0 || sw <= 0 || sh <= 0 ||
		sw&7 != 0 || sh&7 != 0 || dw != sw || dh != sh || c.cart == nil {
		return true
	}
	id := (sy/8)*c.cart.SpriteCols + sx/8
	c.renderSprite(id, dx, dy, sw/8, sh/8, flipX, flipY)
	return true
}

func (c *Context) initMode0Objects() {
	if c == nil || c.cart == nil {
		return
	}
	for offset := 0; offset < 32*1024; offset += 4 {
		hw.SetVRAM32(hw.OBJVRAM+uintptr(offset), 0)
	}
	for object := 0; object < 128; object++ {
		mode0PutOAM16(&c.mode0OAM, object*8, mode0ObjectDisable)
	}
	hw.CopyToVRAM32DMA(c.mode0OAM[:], hw.OAM, len(c.mode0OAM)/4)
	limit := c.cart.SpriteN
	if limit > mode0ObjectTileLimit {
		limit = mode0ObjectTileLimit
	}
	for id := 1; id < limit; id++ {
		if c.stageMode0Tile(id) {
			c.uploadMode0ObjectTile(id, c.mode0TileStage[:])
		}
	}
	c.applyMode0ObjectPalette()
}

func (c *Context) uploadMode0ObjectTile(id int, tile []byte) {
	if id <= 0 || id >= mode0ObjectTileLimit || len(tile) < 64 {
		return
	}
	hw.CopyToVRAM32DMA(tile[:64], hw.OBJVRAM+uintptr(id*64), 16)
}

func (c *Context) beginMode0Objects() {
	c.beginMode0Text()
	for index := 0; index < c.mode0PreviousObjs; index++ {
		slot := 127 - index
		mode0PutOAM16(&c.mode0OAM, slot*8, mode0ObjectDisable)
	}
	c.mode0OAMDirty = c.mode0PreviousObjs != 0
	c.mode0ObjectCount = 0
}

func (c *Context) renderSprite(id, x, y, w, h int, flipX, flipY bool) {
	if c == nil || c.cart == nil || id < 0 || w <= 0 || h <= 0 ||
		c.mode0ObjectCount+w*h > mode0ObjectLimit {
		return
	}
	cols := c.cart.SpriteCols
	if cols <= 0 {
		return
	}
	baseCellX, baseCellY := id%cols, id/cols
	if baseCellX+w > cols || baseCellY+h > c.cart.SpriteRows {
		return
	}
	baseX, baseY := x-c.camX, y-c.camY
	for drawY := 0; drawY < h; drawY++ {
		for drawX := 0; drawX < w; drawX++ {
			sourceX, sourceY := drawX, drawY
			if flipX {
				sourceX = w - 1 - drawX
			}
			if flipY {
				sourceY = h - 1 - drawY
			}
			sourceID := id + sourceY*cols + sourceX
			if sourceID < 0 || sourceID >= c.cart.SpriteN || sourceID >= mode0ObjectTileLimit {
				continue
			}
			slot := 127 - c.mode0ObjectCount
			offset := slot * 8
			attr0 := uint16((baseY+drawY*8)&0xff) | 0x2000 // 8bpp, square 8x8
			attr1 := uint16((baseX + drawX*8) & 0x1ff)
			if flipX {
				attr1 |= 1 << 12
			}
			if flipY {
				attr1 |= 1 << 13
			}
			attr2 := uint16(sourceID * 2) // 8bpp uses two 32-byte tile-number units
			mode0PutOAM16(&c.mode0OAM, offset, attr0)
			mode0PutOAM16(&c.mode0OAM, offset+2, attr1)
			mode0PutOAM16(&c.mode0OAM, offset+4, attr2)
			mode0PutOAM16(&c.mode0OAM, offset+6, 0)
			c.mode0ObjectCount++
		}
	}
	c.mode0OAMDirty = true
}

func (c *Context) presentMode0Objects() uint32 {
	c.mode0UploadStats.LastOBJTileBytes = c.presentMode0Text()
	if !c.mode0OAMDirty {
		return 0
	}
	hw.CopyToVRAM32DMA(c.mode0OAM[:], hw.OAM, len(c.mode0OAM)/4)
	c.mode0PreviousObjs = c.mode0ObjectCount
	c.mode0OAMDirty = false
	c.applyMode0ObjectPalette()
	return uint32(len(c.mode0OAM))
}
