//go:build mtile_mode0 && mtile_mode0_text && mtile_mode0_obj

package runtime

import "github.com/drpaneas/mtile/runtime/hw"

const (
	mode0TextTileBase = Mode0TextTileBase
	mode0TextLimit    = Mode0TextTileCount
)

type mode0TextState struct {
	mode0TextKeys      [mode0TextLimit]uint16
	mode0TextTileCount int
}

func mode0TextObjectTileReserved(id int) bool {
	return id >= mode0TextTileBase && id < mode0TextTileBase+mode0TextLimit
}

func mode0TextObjectWindowReserved(base, count int) bool {
	return count > 0 && base < mode0TextTileBase+mode0TextLimit && base+count > mode0TextTileBase
}

func (c *Context) beginMode0Text() {}

func (c *Context) presentMode0Text() uint32 { return 0 }

// LoadMode0TextAtlas installs the transpiler's sorted, deduplicated static
// glyph atlas while Mode 0 is still force-blanked. OAM allocation remains per
// visible character; only immutable tile pixels are shared across occurrences
// and frames.
func (c *Context) LoadMode0TextAtlas(keys []uint16, tiles []uint32) bool {
	if c == nil || len(keys) > mode0TextLimit || len(tiles) != len(keys)*16 {
		return false
	}
	for index, key := range keys {
		ch, color := byte(key>>8), byte(key)
		if ch < 32 || ch > 126 || color == 0 || index > 0 && key <= keys[index-1] {
			return false
		}
	}
	start := hw.ReadFrameCycleCounter()
	for index := range c.mode0TextKeys {
		c.mode0TextKeys[index] = 0
	}
	copy(c.mode0TextKeys[:], keys)
	c.mode0TextTileCount = len(keys)
	hw.CopyWordsToVRAM32DMA(tiles, hw.OBJVRAM+uintptr(mode0TextTileBase*64), len(tiles))
	c.mode0ColdCycles += hw.ReadFrameCycleCounter() - start
	c.mode0UploadStats.ColdTileBytes += uint32(len(tiles) * 4)
	return true
}

func (c *Context) renderMode0Glyph(ch byte, sx, sy int, value byte) bool {
	if !c.mode0ObjectIntersectsClip(sx, sy, 8, 8) {
		return true
	}
	if c.mode0ObjectCount >= mode0ObjectLimit {
		return false
	}
	key := Mode0TextGlyphKey(ch, value)
	low, high := 0, c.mode0TextTileCount
	for low < high {
		mid := low + (high-low)/2
		if c.mode0TextKeys[mid] < key {
			low = mid + 1
		} else {
			high = mid
		}
	}
	if low >= c.mode0TextTileCount || c.mode0TextKeys[low] != key {
		return false
	}
	tileIndex := low

	slot := 127 - c.mode0ObjectCount
	mode0PutOAMEntry(&c.mode0OAM, slot, uint16(sy&0xff)|0x2000, uint16(sx&0x1ff), uint16((mode0TextTileBase+tileIndex)*2))
	c.mode0ObjectCount++
	c.mode0OAMDirty = true
	return true
}

// hardwarePrint implements the statically admitted print() subset: a literal
// ASCII string or a decimal number, with a literal color. Numbers use the same
// formatNumber layout as Tostr, on a stack buffer, so a heapless cart can print
// scores. Non-numeric positions use the shared cursor exactly like the software
// renderer; the compiler excludes controls, concat, tostr/chr/sub, palette
// remaps, and object-budget overflow before adding mtile_mode0_text.
func (c *Context) hardwarePrint(s, x, y, color Value) (int, bool) {
	if c == nil || color.Kind != KindNumber {
		return 0, true
	}
	if s.Kind == KindNumber {
		var buf [16]byte
		n := formatNumber(buf[:], s.Num, false)
		return c.hardwarePrintStaticBytes(buf[:n], x, y, color)
	}
	if !isStringKind(s.Kind) {
		return 0, true
	}
	return c.hardwarePrintStaticBytes(c.stringBytes(s), x, y, color)
}

// hardwarePrintStatic is kept separate from hardwarePrint so the generated
// literal-only call graph cannot reach stringBytes' heap-backed KindString
// branch. This distinction is observable in the final GBA link: a genuinely
// heapless text cart must not retain either 56 KiB Lua semispace.
func (c *Context) hardwarePrintStatic(s, x, y, color Value) (int, bool) {
	if c == nil || s.Kind != KindStaticString || int(s.SRef) >= int(c.staticN) || color.Kind != KindNumber {
		return 0, true
	}
	return c.hardwarePrintStaticBytes(c.staticStringBytes(s), x, y, color)
}

// Keep the literal-only front end small enough for TinyGo to fold the
// always-handled branch in PrintStaticV. The measured ARM target is faster
// with this leaf split than with one large hardwarePrintStatic body.
func (c *Context) hardwarePrintStaticBytes(bytes []byte, x, y, color Value) (int, bool) {
	c.ensureDrawState()
	implicitPosition := x.Kind != KindNumber || y.Kind != KindNumber
	worldX, worldY := x.Int(), y.Int()
	if implicitPosition {
		worldX, worldY = c.cursor.x, c.cursor.y
	}
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
	if implicitPosition {
		c.cursor.x = worldX
		c.cursor.y = sy + c.camY + glyphH
	} else {
		c.cursor.x, c.cursor.y = worldX, worldY
	}
	return rightMost + c.camX, true
}
