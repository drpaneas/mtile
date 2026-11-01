package main

import (
	"fmt"
	"image/color"
	"strings"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/hostdocs"
	"github.com/drpaneas/mtile/internal/lint"
)

type developLintAction uint8

const (
	lintNoAction developLintAction = iota
	lintOpenSource
	lintOpenDocs
	lintToggleNotes
	lintOpenProblems
)

type developLintRow struct {
	text   string
	action developLintAction
	at     lint.Location
	guide  string
	role   string
}

func (g *Game) lintAdviceCount() int {
	count := 0
	for _, d := range g.codeAnalysis.Lint {
		if d.Severity != "note" {
			count++
		}
	}
	return count
}

func (g *Game) developLintRows() []developLintRow {
	if !g.codeAnalysis.LintReady {
		if g.codeAnalysisBusy || !g.codeAnalysisDue.IsZero() {
			return []developLintRow{{text: "Checking the current source buffers...", role: "summary"}}
		}
		return []developLintRow{
			{text: "Lint needs valid Lua and a resolved require graph.", role: "summary"},
			{text: "Open Problems to inspect source/project diagnostics.", action: lintOpenProblems, role: "link"},
		}
	}
	advice, notes := 0, 0
	for _, d := range g.codeAnalysis.Lint {
		if d.Severity == "note" {
			notes++
		} else {
			advice++
		}
	}
	rows := []developLintRow{{text: fmt.Sprintf("MTile Lint: %d advice | %d context notes | current source buffers", advice, notes), role: "summary"}}
	if notes > 0 {
		label := "Show"
		if g.lintShowNotes {
			label = "Hide"
		}
		rows = append(rows, developLintRow{text: fmt.Sprintf("%s context notes (%d) - notes are not optimization errors", label, notes), action: lintToggleNotes, role: "link"})
	}
	frame := func(at lint.Location, label string, snippet []lint.SourceLine) {
		rows = append(rows, developLintRow{text: fmt.Sprintf("%s:%d:%d  %s", at.Path, at.Line, at.Column, label), action: lintOpenSource, at: at, role: "link"})
		for _, line := range snippet {
			location := lint.Location{Path: at.Path, Line: line.Number, Column: 1}
			if line.Number == at.Line {
				location.Column = at.Column
			}
			text := strings.ReplaceAll(line.Text, "\t", "    ")
			rows = append(rows, developLintRow{text: fmt.Sprintf("%4d | %s", line.Number, text), action: lintOpenSource, at: location, role: "source"})
			if line.Number == at.Line {
				prefix := line.Text[:min(max(0, at.Column-1), len(line.Text))]
				width := utf8.RuneCountInString(strings.ReplaceAll(prefix, "\t", "    "))
				rows = append(rows, developLintRow{text: "     | " + strings.Repeat(" ", width) + "^ " + label, action: lintOpenSource, at: at, role: "link"})
			}
		}
	}
	visible := 0
	for _, d := range g.codeAnalysis.Lint {
		if d.Severity == "note" && !g.lintShowNotes {
			continue
		}
		visible++
		at := lint.Location{Path: d.Path, Line: d.Line, Column: d.Column}
		rows = append(rows, developLintRow{text: strings.ToUpper(d.Severity) + " [" + d.Rule + "]", action: lintOpenSource, at: at, role: d.Severity})
		frame(at, d.Label, d.Snippet)
		rows = append(rows, developLintRow{text: "Why: " + d.Message})
		for _, related := range d.Related {
			frame(related.Location, related.Label, related.Snippet)
		}
		if len(d.CallPath) > 0 {
			rows = append(rows, developLintRow{text: "Call path: " + strings.Join(d.CallPath, " -> "), role: "dim"})
		}
		for _, note := range d.Notes {
			rows = append(rows, developLintRow{text: "Note: " + note, role: "dim"})
		}
		rows = append(rows, developLintRow{text: "Help: " + d.Suggestion})
		rows = append(rows, developLintRow{text: "Docs: " + d.Guide + " (open explanation)", action: lintOpenDocs, guide: d.Guide, role: "link"})
		rows = append(rows, developLintRow{})
	}
	if visible == 0 {
		rows = append(rows, developLintRow{text: "No advice in this source snapshot. This does not certify the frame budget.", role: "dim"})
	}
	rows = append(rows, developLintRow{text: "Click a source row or press Enter to open it. Docs opens the explanation. Advice needs behaviour checks and profiling.", role: "dim"})
	return rows
}

func (g *Game) developLintLines() []string {
	rows := g.developLintRows()
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = row.text
	}
	return lines
}

func developLintColor(row developLintRow) color.RGBA {
	switch row.role {
	case "summary":
		return colorLCD
	case "link":
		return colorCode
	case "advice":
		return colorHighlight
	case "note", "dim":
		return colorDim
	default:
		return colorText
	}
}

func (g *Game) activateDevelopLintRow(index int, open func(string) error) bool {
	rows := g.developLintRows()
	if index < 0 || index >= len(rows) {
		return false
	}
	row := rows[index]
	switch row.action {
	case lintToggleNotes:
		g.lintShowNotes = !g.lintShowNotes
		g.developScroll[developLint] = 0
		g.developTextDragging = false
		v := g.developTextView(newDevelopPanelLayout(developLint))
		v.document.SetCursor(v.document.Offset(index, 0), false)
	case lintOpenProblems:
		g.developPanel = developProblems
	case lintOpenSource:
		g.openProjectDiagnostic(row.at.Path, row.at.Line, row.at.Column)
		g.developTextFocused = false
		g.developTextDragging = false
	case lintOpenDocs:
		if g.model == nil {
			return false
		}
		if err := open(hostdocs.ForProject(g.model.Dir, row.guide)); err != nil {
			g.statusMsg = "open lint explanation: " + err.Error()
		} else {
			g.statusMsg = "Opening lint explanation in the browser"
		}
	default:
		return false
	}
	g.requestRender()
	return true
}

func (g *Game) openDevelopLintRow() {
	v := g.developTextView(newDevelopPanelLayout(developLint))
	if v.document == nil {
		return
	}
	line, _ := v.document.LineColumn(v.document.Cursor())
	g.activateDevelopLintRow(line, hostdocs.Open)
}
