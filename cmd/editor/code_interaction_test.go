package main

import (
	"strings"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/codeeditor"
	"github.com/drpaneas/mtile/internal/editor"
)

func interactionFixture(source string) (*Game, codeLayout) {
	r := editor.Rect{X: 10, Y: 60, W: codeCharWidth*30 + 110, H: codeLineHeight*8 + 20}
	gutter, text, horizontal, vertical := codeViewportLayout(r)
	return &Game{code: codeeditor.New(source), codeLastParsed: source}, codeLayout{Editor: r, Gutter: gutter, Text: text, Horizontal: horizontal, Vertical: vertical}
}

func TestCodeManualScrollSurvivesIdleUntilKeyboardNavigation(t *testing.T) {
	g, layout := interactionFixture(strings.Repeat("local player_speed = 1\n", 100))
	g.scrollCodeWheel(layout, 0, -6)
	for tick := 0; tick < 60; tick++ {
		g.finishCodeInput(layout, false)
	}
	if g.codeScrollLine != 18 || g.code.Cursor() != 0 {
		t.Fatalf("manual viewport snapped to caret: scroll=%d cursor=%d", g.codeScrollLine, g.code.Cursor())
	}
	g.code.MoveHorizontal(1, false, false)
	g.finishCodeInput(layout, true)
	if g.codeScrollLine != 0 {
		t.Fatal("keyboard navigation did not reveal caret")
	}
}

func TestCodeVerticalThumbAndDragStayInsideDocument(t *testing.T) {
	g, layout := interactionFixture(strings.Repeat("line\n", 100))
	thumb := g.codeVerticalThumb(layout)
	if !rectInsideIDE(thumb, layout.Vertical) {
		t.Fatal("thumb outside track")
	}
	g.beginCodeVerticalDrag(layout, thumb.Y+2)
	g.updateCodeVerticalDrag(layout, layout.Vertical.Y+layout.Vertical.H+100)
	if g.codeScrollLine != g.codeVerticalScrollLimit(layout) {
		t.Fatal("drag did not reach bottom")
	}
	g.finishCodeInput(layout, false)
	if g.codeScrollLine == 0 {
		t.Fatal("scrollbar snapped back to cursor")
	}
	g.updateCodeVerticalDrag(layout, layout.Vertical.Y-100)
	if g.codeScrollLine != 0 {
		t.Fatal("drag did not reach top")
	}
	g.code.SetText("short")
	g.clampCodeScroll(layout)
	if g.codeVerticalScrollLimit(layout) != 0 {
		t.Fatal("short document remains scrollable")
	}
}

func TestCodeDoubleTripleClickAndWordDrag(t *testing.T) {
	g, layout := interactionFixture("local player_speed = 1\nprint(player_speed)\n")
	g.codeSearchOpen = true
	x, y := layout.Text.X+8+8*codeCharWidth+1, layout.Text.Y+5
	at := time.Unix(1000, 0)
	g.beginCodeSelection(layout, x, y, false, at)
	if g.codeSearchOpen {
		t.Fatal("source click left keyboard focus in Find")
	}
	g.beginCodeSelection(layout, x, y, false, at.Add(100*time.Millisecond))
	g.dragCodeSelection(layout, x, y)
	if g.code.SelectedText() != "player_speed" {
		t.Fatalf("double click: %q", g.code.SelectedText())
	}
	g.extendCodeSelection(0)
	if g.code.SelectedText() != "local player_speed" || g.code.Cursor() != 0 {
		t.Fatalf("reverse word drag: %q", g.code.SelectedText())
	}
	g.beginCodeSelection(layout, x, y, false, at.Add(200*time.Millisecond))
	g.dragCodeSelection(layout, x, y)
	if g.code.SelectedText() != "local player_speed = 1\n" {
		t.Fatalf("triple click: %q", g.code.SelectedText())
	}
	g.beginCodeSelection(layout, x, y, false, at.Add(time.Second))
	if _, _, selected := g.code.Selection(); selected {
		t.Fatal("slow click retained whole-line selection")
	}
}

func TestCodeClickRowMatchesRenderedRow(t *testing.T) {
	g, layout := interactionFixture("first\nsecond\nthird")
	for row := 0; row < 3; row++ {
		at := g.codeOffsetAt(layout, layout.Text.X+8, layout.Text.Y+1+row*codeLineHeight)
		if line, _ := g.code.LineColumn(at); line != row {
			t.Fatalf("top of rendered row %d selected %d", row, line)
		}
	}
}

func TestCodeDragAutoScrollAndGutterLineSelection(t *testing.T) {
	g, layout := interactionFixture(strings.Repeat("local value = 1\n", 80))
	g.beginCodeSelection(layout, layout.Text.X+9, layout.Text.Y+5, false, time.Now())
	for tick := 0; tick < 8; tick++ {
		g.dragCodeSelection(layout, layout.Text.X+20, layout.Text.Y+layout.Text.H+30)
		g.finishCodeInput(layout, false)
	}
	if g.codeScrollLine == 0 || g.code.SelectedText() == "" {
		t.Fatal("drag did not select beyond viewport")
	}
	g.beginCodeLineSelection(layout, layout.Gutter.X+3, layout.Text.Y+5)
	if g.code.SelectedText() != "local value = 1\n" {
		t.Fatalf("gutter selection: %q", g.code.SelectedText())
	}
}

func TestCodeScrollbarsAndIndentActionFitAllLayouts(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1920, 1200}, {2560, 1440}} {
		setEditorLayout(size[0], size[1])
		for _, layout := range []codeLayout{newCodeLayout(), newDevelopLayout().Code} {
			if !rectInsideIDE(layout.Vertical, layout.Editor) || !rectInsideIDE(layout.Format, layout.Tabs) || rectsOverlapIDE(layout.Text, layout.Vertical) || rectsOverlapIDE(layout.Vertical, layout.Horizontal) {
				t.Fatalf("scrollbars or format action overlap at %v: %+v", size, layout)
			}
			g := &Game{codeTabs: []string{"game.lua", "sound.lua"}, codeActiveTab: 1}
			for _, tab := range g.visibleCodeTabs(layout) {
				if rectsOverlapIDE(tab.Rect, layout.Format) {
					t.Fatal("format action covers file tab")
				}
			}
		}
	}
}
