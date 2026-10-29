package assetsync

import (
	"testing"
)

func contractFont() *Font {
	f := &Font{Version: 1, Name: "test", CellWidth: 6, CellHeight: 8, Width: 4, Height: 5}
	for cp := 32; cp <= 126; cp++ {
		cols := []int{0, 0, 0, 0}
		if cp == '?' {
			cols[0] = 1
		}
		f.Glyphs = append(f.Glyphs, FontGlyph{Codepoint: cp, Columns: cols})
	}
	return f
}
func TestFontContract(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Font)
	}{
		{"version", func(f *Font) { f.Version = 2 }},
		{"name traversal", func(f *Font) { f.Name = "../font" }},
		{"cell width", func(f *Font) { f.CellWidth = 5 }},
		{"cell height", func(f *Font) { f.CellHeight = 9 }},
		{"bitmap width", func(f *Font) { f.Width = 5 }},
		{"bitmap height", func(f *Font) { f.Height = 6 }},
		{"short columns", func(f *Font) { f.Glyphs[1].Columns = []int{1} }},
		{"long columns", func(f *Font) { f.Glyphs[1].Columns = []int{1, 2, 3, 4, 5} }},
		{"sixth row", func(f *Font) { f.Glyphs[1].Columns[0] = 32 }},
		{"negative pixel", func(f *Font) { f.Glyphs[1].Columns[0] = -1 }},
		{"nonblank space", func(f *Font) { f.Glyphs[0].Columns[0] = 1 }},
		{"blank fallback", func(f *Font) { f.Glyphs['?'-32].Columns[0] = 0 }},
		{"missing ascii", func(f *Font) { f.Glyphs = f.Glyphs[1:] }},
		{"duplicate", func(f *Font) { f.Glyphs[1].Codepoint = 32 }},
		{"control", func(f *Font) { f.Glyphs[1].Codepoint = 127 }},
		{"surrogate", func(f *Font) { f.Glyphs[1].Codepoint = 0xd800 }},
		{"beyond unicode", func(f *Font) { f.Glyphs[1].Codepoint = 0x110000 }},
		{"combining mark", func(f *Font) { f.Glyphs[1].Codepoint = 0x301 }},
		{"bidi control", func(f *Font) { f.Glyphs[1].Codepoint = 0x202e }},
		{"capacity", func(f *Font) {
			for i := 0; i <= MaxFontExtraGlyphs; i++ {
				f.Glyphs = append(f.Glyphs, FontGlyph{Codepoint: 0x400 + i, Columns: []int{0, 0, 0, 0}})
			}
		}},
	}
	if err := ValidateFont(contractFont()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFont(nil); err == nil {
		t.Fatal("nil accepted")
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := contractFont()
			tt.change(f)
			if err := ValidateFont(f); err == nil {
				t.Fatal("invalid font accepted")
			}
		})
	}
	f := contractFont()
	f.Glyphs = append(f.Glyphs, FontGlyph{Codepoint: 'Ω', Columns: []int{31, 17, 31, 0}})
	if err := ValidateFont(f); err != nil {
		t.Fatal(err)
	}
}
