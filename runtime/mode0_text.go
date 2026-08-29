//go:build mtile_mode0 && mtile_mode0_text && mtile_mode0_obj

package runtime

import "github.com/drpaneas/mtile/runtime/hw"

const (
	mode0TextTileBase = 480
	mode0TextLimit    = mode0ObjectTileLimit - mode0TextTileBase
)

type mode0TextState struct {
	mode0TextTiles [mode0TextLimit][64]byte
	mode0TextCount int
}

func (c *Context) beginMode0Text() {
	c.mode0TextCount = 0
}

func (c *Context) presentMode0Text() uint32 {
	for index := 0; index < c.mode0TextCount; index++ {
		hw.CopyToVRAM32DMA(c.mode0TextTiles[index][:],
			hw.OBJVRAM+uintptr((mode0TextTileBase+index)*64), 16)
	}
	return uint32(c.mode0TextCount * 64)
}

func (c *Context) renderMode0Glyph(ch byte, sx, sy int, value byte) bool {
	if c.mode0TextCount >= mode0TextLimit || c.mode0ObjectCount >= mode0ObjectLimit {
		return false
	}
	tileIndex := c.mode0TextCount
	tile := &c.mode0TextTiles[tileIndex]
	for index := range tile {
		tile[index] = 0
	}
	columns := glyphColumns(ch)
	for col := 0; col < glyphBitmapW; col++ {
		bits := columns[col] & 0x1f
		for row := 0; row < glyphBitmapH; row++ {
			if bits&(1<<uint(row)) != 0 {
				tile[row*8+col] = value
			}
		}
	}

	slot := 127 - c.mode0ObjectCount
	offset := slot * 8
	mode0PutOAM16(&c.mode0OAM, offset, uint16(sy&0xff)|0x2000)
	mode0PutOAM16(&c.mode0OAM, offset+2, uint16(sx&0x1ff))
	mode0PutOAM16(&c.mode0OAM, offset+4, uint16((mode0TextTileBase+tileIndex)*2))
	mode0PutOAM16(&c.mode0OAM, offset+6, 0)
	c.mode0TextCount++
	c.mode0ObjectCount++
	c.mode0OAMDirty = true
	return true
}

// hardwarePrint implements the statically admitted print("ASCII", x, y,
// color) subset. The compiler excludes controls, implicit cursor forms,
// palette remaps, and object-budget overflow before adding mtile_mode0_text.
func (c *Context) hardwarePrint(s, x, y, color Value) (int, bool) {
	if c == nil || !isStringKind(s.Kind) || x.Kind != KindNumber || y.Kind != KindNumber || color.Kind != KindNumber {
		return 0, true
	}
	c.ensureDrawState()
	worldX, worldY := x.Int(), y.Int()
	colorIndex := color.Int()
	if colorIndex < 0 || colorIndex > 255 {
		return worldX, true
	}
	c.cursor.color = byte(colorIndex)
	sx, sy := worldX-c.camX, worldY-c.camY
	left := sx
	if left < c.clipX0 {
		left = c.clipX0
	}
	rightMost := sx
	bytes := c.stringBytes(s)
	for index := 0; index < len(bytes); index++ {
		if sx+glyphW > c.clipX1 && sx > left {
			sx = left
			sy += glyphH
		}
		if !c.renderMode0Glyph(bytes[index], sx, sy, c.cursor.color) {
			break
		}
		sx += glyphW
		if sx > rightMost {
			rightMost = sx
		}
	}
	c.cursor.x, c.cursor.y = worldX, worldY
	return rightMost + c.camX, true
}
