package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
)

func TestDevelopDiagnosticsWrappingAndCopyPreserveFullUnicode(t *testing.T) {
	preserveDisplayState(t)
	source := "ERROR game.lua:272:1: [M0_OBJ_BUDGET] " + strings.Repeat("Ελληνικά Grüße details. ", 40) + "\nLast complete line"
	g := &Game{developPanel: developProblems, runError: source}
	for _, size := range [][2]int{{1024, 768}, {1920, 1200}} {
		setEditorLayout(size[0], size[1])
		layout := newDevelopPanelLayout(developProblems)
		view := g.developTextView(layout)
		if len(view.rows) <= 2 {
			t.Fatal("long error was not wrapped")
		}
		for _, row := range view.rows {
			if len([]rune(row.text)) > view.columns || strings.Contains(row.text, "...") {
				t.Fatal("diagnostic was truncated")
			}
		}
		if view.document.Text() != source {
			t.Fatal("wrapping changed original diagnostic")
		}
		view.document.SetCursor(0, false)
		var copied string
		write := func(value string) error { copied = value; return nil }
		g.copyDevelopText(false, write)
		if copied != source {
			t.Fatal("Cmd/Ctrl+C with no selection lost the full error")
		}
		g.copyDevelopText(true, write)
		if copied != source {
			t.Fatal("COPY ALL lost the complete unwrapped error")
		}
		view.document.SetCursor(7, false)
		view.document.SetCursor(75, true)
		g.copyDevelopText(false, write)
		if copied != string([]rune(source)[7:75]) {
			t.Fatal("selected text copy crossed the wrong rune offsets")
		}
		before := view.document.SelectedText()
		g.scrollDevelopText(layout, 10000)
		if g.developScroll[developProblems] == 0 {
			t.Fatal("cannot scroll to tail")
		}
		tail := view.rows[len(view.rows)-1]
		if !strings.HasSuffix(tail.text, "Last complete line") {
			t.Fatal("error tail lost")
		}
		g.scrollDevelopText(layout, -10000)
		if g.developScroll[developProblems] != 0 {
			t.Fatal("scroll underflow")
		}
		setEditorLayout(size[0]+80, size[1])
		g.developTextView(newDevelopPanelLayout(developProblems))
		if view.document.SelectedText() != before {
			t.Fatal("resizing changed selection")
		}
		g.copyDevelopText(false, func(string) error { return errors.New("clipboard unavailable") })
		if !strings.Contains(g.statusMsg, "clipboard unavailable") {
			t.Fatal("clipboard failure was swallowed")
		}
	}
}
func TestDevelopDiagnosticMouseSelectionAndHeaderFit(t *testing.T) {
	preserveDisplayState(t)
	for _, size := range [][2]int{{1024, 768}, {1600, 1000}} {
		setEditorLayout(size[0], size[1])
		g := &Game{developPanel: developProblems, runError: strings.Repeat("abcdefgh", 50), codeAnalysis: build.ProjectAnalysis{}}
		layout := newDevelopPanelLayout(g.developPanel)
		body := developTextBody(layout, g.developPanel)
		view := g.developTextView(layout)
		g.beginDevelopTextSelection(layout, body.X, body.Y, false)
		view.document.SetCursor(g.developTextOffset(layout, body.X+5*codeCharWidth, body.Y+codeLineHeight), true)
		if view.document.SelectedText() != string([]rune(g.runError)[:view.columns+5]) {
			t.Fatal("dragging across a wrapped row changed copy offsets")
		}
		copy := g.developCopyRect(layout)
		if !rectInsideIDE(copy, layout.Code.Diagnostics) {
			t.Fatal("copy action is outside panel")
		}
		for _, tab := range g.developPanelTabRects(layout) {
			if !rectInsideIDE(tab, layout.Code.Diagnostics) || rectsOverlapIDE(tab, copy) {
				t.Fatal("tabs overlap the copy action")
			}
		}
	}
}
func TestDevelopDiagnosticViewsKeepIndependentSelectionAndRefresh(t *testing.T) {
	preserveDisplayState(t)
	g := &Game{developPanel: developProblems, runError: "first error"}
	layout := newDevelopPanelLayout(g.developPanel)
	first := g.developTextView(layout)
	first.document.SelectAll()
	g.developPanel = developBuild
	other := g.developTextView(layout)
	if other.document == first.document {
		t.Fatal("panels share a selection document")
	}
	g.developPanel = developProblems
	if g.developTextView(layout).document.SelectedText() != "first error" {
		t.Fatal("switching panels lost selection")
	}
	g.runError = "replacement error"
	if g.developTextView(layout).document.Text() != g.runError {
		t.Fatal("new build errors leave stale copy contents")
	}
}

func TestDevelopDiagnosticWrapKeepsWordsAndHardBreaksLongTokens(t *testing.T) {
	var view developTextView
	source := "error message " + strings.Repeat("z", 25)
	view.sync(source, 10)
	if view.rows[0].text != "error " || view.rows[1].text != "message " {
		t.Fatal("ordinary words split across visual rows")
	}
	var restored strings.Builder
	for _, row := range view.rows {
		restored.WriteString(row.text)
		if len([]rune(row.text)) > 10 {
			t.Fatal("long token escaped viewport")
		}
	}
	if restored.String() != source {
		t.Fatal("word wrapping discarded spaces or text")
	}
}
