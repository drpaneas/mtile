package main

import (
	"github.com/drpaneas/mtile/internal/assetsync"
	"testing"
)

func TestFontEditorCreatePreviewSaveUndo(t *testing.T) {
	g := newTestGame(t, 8, 8, 1, 1)
	defer g.closeNativePerformance()
	g.preferencesOpen = true
	g.openFontEditor()
	if g.preferencesOpen {
		t.Fatal("font editor did not own modal focus")
	}
	if !g.fontEditor.open || g.fontEditor.preview == "" {
		t.Fatal("preview not initialized")
	}
	g.fontEditor.name = "test-font"
	g.fontAction("blank")
	if g.model.Project.Font != "test-font" {
		t.Fatal(g.fontEditor.message)
	}
	if err := g.model.SetFontPixel('A', 0, 4, true); err != nil {
		t.Fatal(err)
	}
	g.fontChanged()
	if err := g.model.SelectFont("greek"); err != nil {
		t.Fatal(err)
	}
	g.fontChanged()
	state, ok := g.undo.undo()
	if !ok {
		t.Fatal("undo unavailable")
	}
	g.restore(state)
	if g.model.Project.Font != "test-font" {
		t.Fatal("font selection not undoable")
	}
	f, _ := assetsync.CompileFont(g.model.ActiveFont())
	if f.Columns('A')[0] != 16 {
		t.Fatal("undo lost artwork")
	}
	g.fontAction("save")
	disk, err := assetsync.LoadProject(g.model.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if disk.Font != "test-font" {
		t.Fatal("selection not saved")
	}
	saved, err := assetsync.LoadFont(g.model.Dir, disk.Font)
	if err != nil {
		t.Fatal(err)
	}
	f, _ = assetsync.CompileFont(saved)
	if f.Columns('A')[0] != 16 {
		t.Fatal("art not saved")
	}
}
func TestFontEditorLayout(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range [][2]int{{1024, 768}, {1600, 1000}, {1920, 1200}} {
		setEditorLayout(size[0], size[1])
		l := newFontEditorLayout()
		if l.Name.W < 120 || l.Glyphs.H < 180 || l.GlyphInput.Y+l.GlyphInput.H > l.Panel.Y+l.Panel.H-98 {
			t.Fatalf("font tools overlap: %+v", l)
		}
		if fontButtonRect().X+fontButtonRect().W > size[0] {
			t.Fatal("font button outside window")
		}
	}
}

func TestFontEditorSwitchResetsUnavailableSelection(t *testing.T) {
	g := newTestGame(t, 8, 8, 1, 1)
	defer g.closeNativePerformance()
	g.openFontEditor()
	g.model.SelectFont("greek")
	g.fontEditor.selected = 'Ω'
	g.fontChanged()
	g.model.SelectFont("german")
	g.fontChanged()
	if g.fontEditor.selected != 'A' {
		t.Fatal("selection points to missing glyph")
	}
}
