package runtime

const (
	// Mode0TextTileBase is the first OBJ tile reserved for Mode 0 print glyphs.
	// The bank occupies the 32 8bpp OBJ tiles before the system bank, independent of sheet size.
	Mode0TextTileBase = 448
	// Keep authored sheet metadata stable when physical OBJ reservations move.
	Mode0TextSheetBase   = 480
	Mode0TextTileCount   = 32
	Mode0TextGlyphTiles  = 2
	Mode0TextGlyphLimit  = Mode0TextTileCount
	Mode0ObjectTileLimit = 480
)

// Mode0TextGlyphKey is the stable build/runtime identity for one 8bpp OBJ
// glyph. Mode 0 admits static ASCII literals and decimal numbers at a literal
// color, so the transpiler can intern every required pair before link.
func Mode0TextGlyphKey(ch, color byte) uint16 {
	return uint16(ch)<<8 | uint16(color)
}

// BuildMode0GlyphTile rasterizes one admitted font glyph into the exact 8bpp
// 8x16 OBJ tile pair layout used by the hardware renderer. It is called by the host
// transpiler while generating ROM data; TinyGo removes it from cartridges.
func BuildMode0GlyphTile(ch, color byte) [128]byte {
	return BuildMode0GlyphTileWithFont(DefaultFont(), ch, color)
}

func BuildMode0GlyphTileWithFont(font *Font, ch, color byte) [128]byte {
	var tile [128]byte
	columns := font.Columns(rune(ch))
	for col := 0; col < glyphBitmapW; col++ {
		bits := columns[col] & 0x3ff
		for row := 0; row < glyphBitmapH; row++ {
			if bits&(1<<uint(row)) != 0 {
				tile[row*8+col] = color
			}
		}
	}
	return tile
}

// OverlayMode0TextSheet stamps interned Mode 0 print glyphs into authored sheet
// cells 480-511 and clears any unused reserved cell. Build does this so leftover
// pixels cannot survive into sget/map of those IDs; OBJ VRAM still loads from
// the ROM atlas.
func OverlayMode0TextSheet(sheet []byte, sheetW, sheetH int, keys []uint16) {
	OverlayMode0TextSheetWithFont(sheet, sheetW, sheetH, keys, DefaultFont())
}

func OverlayMode0TextSheetWithFont(sheet []byte, sheetW, sheetH int, keys []uint16, font *Font) {
	if sheetW < 8 || sheetH < 8 || len(sheet) < sheetW*sheetH {
		return
	}
	cols := sheetW / 8
	rows := sheetH / 8
	spriteN := cols * rows
	keyIndex, part := 0, 0
	for index := 0; index < Mode0TextTileCount; index++ {
		id := Mode0TextSheetBase + index
		if id >= spriteN {
			return
		}
		col, row := id%cols, id/cols
		origin := row*8*sheetW + col*8
		var tile [64]byte
		if keyIndex < len(keys) {
			key := keys[keyIndex]
			pair := BuildMode0GlyphTileWithFont(font, byte(key>>8), byte(key))
			copy(tile[:], pair[part*64:])
			part++
			if part == Mode0GlyphTileCount(font, byte(key>>8)) {
				keyIndex++
				part = 0
			}
		}
		for y := 0; y < 8; y++ {
			copy(sheet[origin+y*sheetW:origin+y*sheetW+8], tile[y*8:y*8+8])
		}
	}
}

// Mode0GlyphTileCount uses one 8x8 tile when both lower bitmap rows are blank.
// A descender (or custom pixel) in rows 8/9 requires a vertical 8x16 tile pair.
// Cell metrics remain 6x10 in either case; transparent padding has no ink.
func Mode0GlyphTileCount(font *Font, ch byte) int {
	columns := font.Columns(rune(ch))
	for _, bits := range columns {
		if bits&0x300 != 0 {
			return 2
		}
	}
	return 1
}
