package editor

import "testing"

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
