package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/drpaneas/mtile/internal/codeeditor"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/hostclipboard"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type developTextRow struct {
	text        string
	start, line int
}
type developTextView struct {
	document *codeeditor.Editor
	source   string
	columns  int
	rows     []developTextRow
}

// Wrapping changes only the view. Selection/clipboard offsets always refer to
// the original rune-indexed diagnostic, including its real line breaks.
func (v *developTextView) sync(source string, columns int) {
	columns = max(1, columns)
	if v.document != nil && v.source == source && v.columns == columns {
		return
	}
	if v.document == nil || v.source != source {
		v.document = codeeditor.New(source)
	}
	v.source, v.columns = source, columns
	v.rows = nil
	for line := 0; line < v.document.LineCount(); line++ {
		runes := []rune(v.document.Line(line))
		start := v.document.Offset(line, 0)
		if len(runes) == 0 {
			v.rows = append(v.rows, developTextRow{"", start, line})
			continue
		}
		for from := 0; from < len(runes); {
			end := min(len(runes), from+columns)
			if end < len(runes) {
				for split := end; split > from; split-- {
					if unicode.IsSpace(runes[split-1]) {
						end = split
						break
					}
				}
			}
			v.rows = append(v.rows, developTextRow{string(runes[from:end]), start + from, line})
			from = end
		}

	}
}

func developTextBody(layout developLayout, panel developPanel) editor.Rect {
	r := layout.Code.Diagnostics
	x := r.X + 14
	if panel == developGenerated {
		x = r.X + 56
	}
	return editor.Rect{X: x, Y: r.Y + 40, W: max(1, r.X+r.W-24-x), H: max(1, r.H-45)}
}
func (g *Game) developCopyRect(layout developLayout) editor.Rect {
	r := layout.Code.Diagnostics
	return editor.Rect{X: r.X + r.W - 101, Y: r.Y + 3, W: 94, H: 28}
}
func (g *Game) developTextView(layout developLayout) *developTextView {
	v := &g.developText[g.developPanel]
	body := developTextBody(layout, g.developPanel)
	v.sync(strings.Join(g.developPanelLines(g.developPanel), "\n"), body.W/codeCharWidth)
	visible := max(1, body.H/codeLineHeight)
	g.developScroll[g.developPanel] = min(max(0, g.developScroll[g.developPanel]), max(0, len(v.rows)-visible))
	return v
}
func (g *Game) scrollDevelopText(layout developLayout, delta int) {
	v := g.developTextView(layout)
	body := developTextBody(layout, g.developPanel)
	visible := max(1, body.H/codeLineHeight)
	g.developScroll[g.developPanel] = min(max(0, g.developScroll[g.developPanel]+delta), max(0, len(v.rows)-visible))
	g.requestRender()
}
func (g *Game) developTextOffset(layout developLayout, x, y int) int {
	v := g.developTextView(layout)
	body := developTextBody(layout, g.developPanel)
	visible := max(1, body.H/codeLineHeight)
	row := g.developScroll[g.developPanel] + min(max(0, (y-body.Y)/codeLineHeight), visible-1)
	row = min(row, len(v.rows)-1)
	col := min(max(0, (x-body.X+codeCharWidth/2)/codeCharWidth), len([]rune(v.rows[row].text)))
	return v.rows[row].start + col
}
func developRowAtOffset(rows []developTextRow, offset int) int {
	return max(0, sort.Search(len(rows), func(i int) bool { return rows[i].start > offset })-1)
}
func (g *Game) beginDevelopTextSelection(layout developLayout, x, y int, shift bool) {
	g.developTextFocused, g.developTextDragging = true, true
	g.developTextScrollDragging = false
	v := g.developTextView(layout)
	v.document.SetCursor(g.developTextOffset(layout, x, y), shift)
	g.requestRender()
}
func (g *Game) copyDevelopText(all bool, write func(string) error) {
	v := &g.developText[g.developPanel]
	if v.document == nil {
		return
	}
	value := v.document.SelectedText()
	if all || value == "" {
		value = v.document.Text()
	}
	if err := write(value); err != nil {
		g.statusMsg = "copy diagnostics: " + err.Error()
	} else {
		g.statusMsg = "Copied " + g.developPanel.label() + " text"
	}
	g.requestRender()
}
func (g *Game) updateDevelopTextInput(layout developLayout) bool {
	if !g.developTextFocused || g.developPanel == developMemory {
		return false
	}
	v := g.developTextView(layout)
	body := developTextBody(layout, g.developPanel)
	visible := max(1, body.H/codeLineHeight)
	mx, my := editorCursorPosition()
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.developTextDragging = false
		g.developTextScrollDragging = false
	}
	if g.developTextScrollDragging {
		limit := max(0, len(v.rows)-visible)
		g.developScroll[g.developPanel] = min(max(0, (my-body.Y)*len(v.rows)/max(1, body.H)), limit)
		g.requestRender()
	} else if g.developTextDragging {
		if my < body.Y {
			g.scrollDevelopText(layout, -1)
		} else if my >= body.Y+body.H {
			g.scrollDevelopText(layout, 1)
		}
		v.document.SetCursor(g.developTextOffset(layout, mx, my), true)
		g.requestRender()
	}
	mod := codeModifier()
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	switch {
	case mod && inpututil.IsKeyJustPressed(ebiten.KeyS):
		return false // Saving the active Lua document stays a workspace shortcut.
	case mod && inpututil.IsKeyJustPressed(ebiten.KeyA):
		v.document.SelectAll()
		g.requestRender()
	case mod && inpututil.IsKeyJustPressed(ebiten.KeyC):
		g.copyDevelopText(false, hostclipboard.Write)
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		g.developTextFocused = false
		g.requestRender()
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter):
		switch g.developPanel {
		case developProblems:
			g.openDevelopDiagnostic()
			g.developTextFocused = false
		case developLint:
			g.openDevelopLintRow()
		}
	default:
		delta := 0
		switch {
		case codeKeyRepeat(ebiten.KeyArrowUp):
			delta = -1
		case codeKeyRepeat(ebiten.KeyArrowDown):
			delta = 1
		case codeKeyRepeat(ebiten.KeyPageUp):
			delta = -visible
		case codeKeyRepeat(ebiten.KeyPageDown):
			delta = visible
		case codeKeyRepeat(ebiten.KeyArrowLeft):
			v.document.MoveHorizontal(-1, shift, mod)
		case codeKeyRepeat(ebiten.KeyArrowRight):
			v.document.MoveHorizontal(1, shift, mod)
		case inpututil.IsKeyJustPressed(ebiten.KeyHome):
			v.document.SetCursor(0, shift)
		case inpututil.IsKeyJustPressed(ebiten.KeyEnd):
			v.document.SetCursor(len([]rune(v.document.Text())), shift)
		default:
			return true // Read-only: never send typing/delete/paste to Lua source.
		}
		row := developRowAtOffset(v.rows, v.document.Cursor())
		if delta != 0 {
			col := v.document.Cursor() - v.rows[row].start
			row = min(max(0, row+delta), len(v.rows)-1)
			v.document.SetCursor(v.rows[row].start+min(col, len([]rune(v.rows[row].text))), shift)
		}
		if row < g.developScroll[g.developPanel] {
			g.developScroll[g.developPanel] = row
		}
		if row >= g.developScroll[g.developPanel]+visible {
			g.developScroll[g.developPanel] = row - visible + 1
		}
		g.requestRender()
	}
	return true
}

func (g *Game) drawDevelopPanelText(screen *ebiten.Image, layout developLayout) {
	v := g.developTextView(layout)
	body := developTextBody(layout, g.developPanel)
	drawDevelopButton(screen, g.developCopyRect(layout), "COPY ALL", false, colorCode)
	// The clipping rectangle is physical, matching the Retina-aware draw helpers.
	s := editorDrawScale
	clip := screen.SubImage(image.Rect(int(math.Floor(float64(body.X)*s)), int(math.Floor(float64(body.Y)*s)), int(math.Ceil(float64(body.X+body.W)*s)), int(math.Ceil(float64(body.Y+body.H)*s)))).(*ebiten.Image)
	visible := max(1, body.H/codeLineHeight)
	start := g.developScroll[g.developPanel]
	var lintRows []developLintRow
	if g.developPanel == developLint {
		lintRows = g.developLintRows()
	}
	from, to, selected := v.document.Selection()
	for row := 0; row < visible && start+row < len(v.rows); row++ {
		item := v.rows[start+row]
		y := body.Y + row*codeLineHeight
		clr := colorText
		original := v.document.Line(item.line)
		lower := strings.ToLower(original)
		if g.developPanel == developLint && item.line < len(lintRows) {
			clr = developLintColor(lintRows[item.line])
		} else if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
			clr = colorDanger
		} else if strings.HasPrefix(original, "OK") || strings.Contains(lower, "completed successfully") {
			clr = colorRun
		} else if strings.HasPrefix(original, "[") {
			clr = colorLCD
		}
		if selected {
			a, b := max(from, item.start), min(to, item.start+len([]rune(item.text)))
			if a < b {
				fillRect(clip, editor.Rect{X: body.X + (a-item.start)*codeCharWidth, Y: y, W: (b - a) * codeCharWidth, H: codeLineHeight}, color.RGBA{65, 82, 122, 255})
			}
		}
		if g.developPanel == developGenerated {
			drawCodeText(screen, fmt.Sprintf("%4d", item.line+1), layout.Code.Diagnostics.X+12, y, colorDim)
			x := body.X
			for _, segment := range highlightGoLine(item.text) {
				drawCodeText(clip, segment.text, x, y, segment.color)
				x += len([]rune(segment.text)) * codeCharWidth
			}
		} else {
			drawCodeText(clip, item.text, body.X, y, clr)
		}
	}
	if g.developTextFocused && !selected {
		row := developRowAtOffset(v.rows, v.document.Cursor())
		if row >= start && row < start+visible {
			fillRect(clip, editor.Rect{X: body.X + (v.document.Cursor()-v.rows[row].start)*codeCharWidth, Y: body.Y + (row-start)*codeLineHeight, W: 1, H: codeLineHeight - 2}, colorCode)
		}
	}
	track := editor.Rect{X: layout.Code.Diagnostics.X + layout.Code.Diagnostics.W - 16, Y: body.Y, W: 9, H: body.H}
	fillRect(screen, track, colorInk)
	if len(v.rows) > visible {
		thumbH := max(14, track.H*visible/len(v.rows))
		y := track.Y + (track.H-thumbH)*start/(len(v.rows)-visible)
		fillRect(screen, editor.Rect{X: track.X, Y: y, W: track.W, H: thumbH}, colorSelection)
	}
}
