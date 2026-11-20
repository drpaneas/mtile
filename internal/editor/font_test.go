package editor

import (
	"github.com/drpaneas/mtile/internal/assetsync"
	"testing"
)

func TestFontEditingCannotBypassContract(t *testing.T) {
	m := &Model{Dir: t.TempDir()}
	if err := m.LoadFonts(); err != nil {
		t.Fatal(err)
	}
	if err := m.SetFontPixel('A', 0, 0, true); err == nil {
		t.Fatal("edited builtin")
	}
	if err := m.CreateFont("custom", true); err != nil {
		t.Fatal(err)
	}
	if len(m.ActiveFont().Glyphs) <= 95 {
		t.Fatal("blank lacks international repertoire")
	}
	for _, point := range [][2]int{{6, 0}, {0, 10}, {-1, 0}, {0, -1}, {100, 100}} {
		if err := m.SetFontPixel('A', point[0], point[1], true); err == nil {
			t.Fatal("painted outside bitmap")
		}
	}
	if err := m.SetFontPixel(' ', 0, 0, true); err == nil {
		t.Fatal("painted spacing")
	}
	if err := m.SetFontPixel('Ω', 3, 4, true); err != nil {
		t.Fatal(err)
	}
	if err := m.AddFontGlyph('Ж'); err != nil {
		t.Fatal(err)
	}
	if err := m.AddFontGlyph(0xd800); err == nil {
		t.Fatal("invalid scalar")
	}
	if err := m.AddFontGlyph(0x301); err == nil {
		t.Fatal("combining mark accepted")
	}
	if err := m.CreateFont("../escape", false); err == nil {
		t.Fatal("traversal")
	}
	if err := m.saveFonts(); err != nil {
		t.Fatal(err)
	}
	saved, err := assetsync.LoadFont(m.Dir, "custom")
	if err != nil {
		t.Fatal(err)
	}
	compiled, _ := assetsync.CompileFont(saved)
	if *compiled.Columns('Ω') != [6]uint16{0, 0, 0, 16} {
		t.Fatal("glyph persistence")
	}
	if err := m.SelectFont("german"); err != nil {
		t.Fatal(err)
	}
	if err := m.SelectFont("custom"); err != nil {
		t.Fatal(err)
	}
	copied := m.ActiveFont()
	if err := m.CreateFont("copy", false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetFontPixel('Ω', 0, 0, true); err != nil {
		t.Fatal(err)
	}
	if copied.Glyphs[0].Columns[0] != 0 {
		t.Fatal("copy aliased source")
	}
	if err := m.validateFonts(); err != nil {
		t.Fatal(err)
	}
}
