package runtime

// Mode0TextGlyphKey is the stable build/runtime identity for one 8bpp OBJ
// glyph. Mode 0 admits only literal text and literal colors, so the transpiler
// can intern every required pair before the cartridge is linked.
func Mode0TextGlyphKey(ch, color byte) uint16 {
	return uint16(ch)<<8 | uint16(color)
}

// BuildMode0GlyphTile rasterizes one admitted font glyph into the exact 8bpp
// 8x8 OBJ tile layout used by the hardware renderer. It is called by the host
// transpiler while generating ROM data; TinyGo removes it from cartridges.
func BuildMode0GlyphTile(ch, color byte) [64]byte {
	var tile [64]byte
	columns := glyphColumns(ch)
	for col := 0; col < glyphBitmapW; col++ {
		bits := columns[col] & 0x1f
		for row := 0; row < glyphBitmapH; row++ {
			if bits&(1<<uint(row)) != 0 {
				tile[row*8+col] = color
			}
		}
	}
	return tile
}
