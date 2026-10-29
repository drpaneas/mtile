package editor

import (
	"github.com/drpaneas/mtile/internal/assetsync"
	"testing"
)

func TestFontBEditorPaintsLastPixelAndLocksBounds(t *testing.T) {
	m := &Model{Dir: t.TempDir()}
	if err := m.LoadFonts(); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateFont("six-ten", true); err != nil {
		t.Fatal(err)
	}
	if err := m.SetFontPixel('A', 5, 9, true); err != nil {
		t.Fatalf("B last pixel rejected: %v", err)
	}
	f := m.ActiveFont()
	if f.Version != 2 || f.Glyphs['A'-32].Columns[5] != 512 {
		t.Fatal("B pixel truncated")
	}
	for _, p := range [][2]int{{6, 9}, {5, 10}, {-1, 0}, {0, -1}} {
		if err := m.SetFontPixel('A', p[0], p[1], true); err == nil {
			t.Fatal("editor bypassed B bounds")
		}
	}
	if err := m.saveFonts(); err != nil {
		t.Fatal(err)
	}
	if err := m.LoadFonts(); err != nil {
		t.Fatal(err)
	}
	if m.ActiveFont().Glyphs['A'-32].Columns[5] != 512 {
		t.Fatal("save/reload lost tenth row")
	}
}

func TestFontBLegacyEditingUpgradesOnlyTheChangedCopy(t *testing.T) {
	m := &Model{Dir: t.TempDir()}
	if err := m.LoadFonts(); err != nil {
		t.Fatal(err)
	}
	legacy := &assetsync.Font{Version: 1, Name: "legacy", CellWidth: 6, CellHeight: 8, Width: 4, Height: 5}
	for cp := 32; cp <= 126; cp++ {
		columns := []int{0, 0, 0, 0}
		if cp == '?' {
			columns[0] = 1
		}
		legacy.Glyphs = append(legacy.Glyphs, assetsync.FontGlyph{Codepoint: cp, Columns: columns})
	}
	m.Fonts[legacy.Name] = legacy
	if err := m.SelectFont("legacy"); err != nil {
		t.Fatal(err)
	}
	if err := m.saveFonts(); err != nil {
		t.Fatal(err)
	}
	if err := m.LoadFonts(); err != nil {
		t.Fatal(err)
	}
	if m.ActiveFont().Version != 1 {
		t.Fatal("loading upgraded the authored document")
	}
	original := m.ActiveFont()
	if err := m.SetFontPixel(' ', 5, 9, true); err == nil {
		t.Fatal("space accepted ink")
	}
	if m.ActiveFont() != original || original.Version != 1 {
		t.Fatal("failed edit changed legacy document")
	}
	if err := m.CreateFont("copy", false); err != nil {
		t.Fatal(err)
	}
	if m.ActiveFont().Version != 2 || original.Version != 1 {
		t.Fatal("copy migration changed its source")
	}
	if err := m.SelectFont("legacy"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetFontPixel('A', 5, 9, true); err != nil {
		t.Fatal(err)
	}
	if m.ActiveFont().Version != 2 || m.ActiveFont().Glyphs['A'-32].Columns[5] != 512 {
		t.Fatal("explicit edit did not upgrade")
	}
	if original.Version != 1 || len(original.Glyphs[0].Columns) != 4 {
		t.Fatal("edit mutated existing snapshot")
	}
}
