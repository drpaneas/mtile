package assetsync

import (
	rt "github.com/drpaneas/mtile/runtime"
	"strings"
)

// All presets use the same geometry; language presets subset the approved B
// artwork. The PICO-8 style preserves the original pixels with empty padding.
func builtinFont(name string) *Font {
	f := &Font{Version: 2, Name: name, CellWidth: FontCellWidth, CellHeight: FontCellHeight, Width: FontBitmapWidth, Height: FontBitmapHeight}
	source := rt.DefaultFont()
	if name == "pico8" {
		source = rt.Pico8Font()
	}
	appendGlyph := func(cp int, cols *[6]uint16) {
		values := make([]int, 6)
		for i, v := range cols {
			values[i] = int(v)
		}
		f.Glyphs = append(f.Glyphs, FontGlyph{Codepoint: cp, Columns: values})
	}
	for cp := 32; cp <= 126; cp++ {
		appendGlyph(cp, source.Columns(rune(cp)))
	}
	for _, g := range source.Extra {
		german := strings.ContainsRune("ÄÖÜäöüßẞ", g.Codepoint)
		if name == "international" || name == "german" && german || name == "greek" && !german {
			appendGlyph(int(g.Codepoint), &g.Columns)
		}
	}
	return f
}
