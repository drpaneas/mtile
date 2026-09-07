package main

import (
	"image/color"
	"math"
	"time"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
)

type codePointerState struct {
	lastAt                   time.Time
	lastX, lastY, lastOffset int
	clicks                   int
	originStart, originEnd   int
}

func codeFormatRect(tabs editor.Rect) editor.Rect {
	return editor.Rect{X: tabs.X + tabs.W - 100, Y: tabs.Y + 4, W: 94, H: tabs.H - 8}
}

func (g *Game) formatCodeIndentation(layout codeLayout) {
	if g.code.FormatIndentation() {
		g.statusMsg = "Lua indentation formatted; Cmd/Ctrl+Z to undo"
	} else {
		g.statusMsg = "Lua indentation is already consistent"
	}
	g.keepCodeCursorVisible(layout)
}

// Only an editing/navigation command reveals the caret. Wheel/scrollbar and
// idle updates must leave a deliberately scrolled-away viewport where it is.
func (g *Game) finishCodeInput(layout codeLayout, revealCursor bool) {
	g.refreshCodeDiagnostics()
	if revealCursor {
		g.keepCodeCursorVisible(layout)
	} else {
		g.clampCodeScroll(layout)
	}
}

func (g *Game) codeVerticalScrollLimit(layout codeLayout) int {
	visible := max(1, (layout.Text.H-8)/codeLineHeight)
	return max(0, g.code.LineCount()-visible)
}

func (g *Game) clampCodeScroll(layout codeLayout) {
	g.codeScrollLine = min(max(0, g.codeScrollLine), g.codeVerticalScrollLimit(layout))
	g.codeScrollColumn = min(max(0, g.codeScrollColumn), g.codeHorizontalScrollLimit(layout))
}

// Retain fractional trackpad motion instead of turning every tiny event into
// a whole line. Three lines per wheel unit also preserves mouse-wheel speed.
func codeWheelAxis(position, limit int, delta float64, remainder *float64) int {
	if delta == 0 {
		return min(max(0, position), limit)
	}
	if delta**remainder < 0 {
		*remainder = 0
	}
	*remainder += delta * 3
	steps := int(*remainder)
	*remainder -= float64(steps)
	next := min(max(0, position-steps), limit)
	if next == 0 && delta > 0 || next == limit && delta < 0 {
		*remainder = 0
	}
	return next
}

func (g *Game) scrollCodeWheel(layout codeLayout, dx, dy float64) {
	g.codeScrollLine = codeWheelAxis(g.codeScrollLine, g.codeVerticalScrollLimit(layout), dy, &g.codeWheelY)
	g.codeScrollColumn = codeWheelAxis(g.codeScrollColumn, g.codeHorizontalScrollLimit(layout), dx, &g.codeWheelX)
}

func (g *Game) codeVerticalThumb(layout codeLayout) editor.Rect {
	track := editor.Rect{X: layout.Vertical.X + 3, Y: layout.Vertical.Y + 3, W: max(3, layout.Vertical.W-6), H: max(1, layout.Vertical.H-6)}
	visible := max(1, (layout.Text.H-8)/codeLineHeight)
	height := min(track.H, max(28, track.H*visible/max(visible, g.code.LineCount())))
	limit := g.codeVerticalScrollLimit(layout)
	y := track.Y
	if limit > 0 {
		y += min(limit, max(0, g.codeScrollLine)) * (track.H - height) / limit
	}
	return editor.Rect{X: track.X, Y: y, W: track.W, H: height}
}

func (g *Game) beginCodeVerticalDrag(layout codeLayout, y int) {
	g.codeDragging, g.codeHorizontalDragging = false, false
	g.codeVerticalDragging = g.codeVerticalScrollLimit(layout) > 0
	thumb := g.codeVerticalThumb(layout)
	g.codeVerticalDragOffset = thumb.H / 2
	if y >= thumb.Y && y < thumb.Y+thumb.H {
		g.codeVerticalDragOffset = y - thumb.Y
	}
	g.updateCodeVerticalDrag(layout, y)
}

func (g *Game) updateCodeVerticalDrag(layout codeLayout, y int) {
	thumb := g.codeVerticalThumb(layout)
	travel := max(0, layout.Vertical.H-6-thumb.H)
	if travel == 0 {
		g.codeScrollLine = 0
		return
	}
	position := min(travel, max(0, y-layout.Vertical.Y-3-g.codeVerticalDragOffset))
	g.codeScrollLine = int(math.Round(float64(position*g.codeVerticalScrollLimit(layout)) / float64(travel)))
}

func (g *Game) drawCodeVerticalScrollbar(screen *ebiten.Image, layout codeLayout) {
	fillRect(screen, layout.Vertical, colorInk)
	thumb := g.codeVerticalThumb(layout)
	var clr color.Color = color.RGBA{R: 74, G: 75, B: 105, A: 255}
	mx, my := ebiten.CursorPosition()
	if g.codeVerticalScrollLimit(layout) > 0 && (g.codeVerticalDragging || rectContains(thumb, mx, my)) {
		clr = rgba(colorCode, 190)
	}
	fillRoundedRect(screen, thumb, 2, clr)
}

func (g *Game) beginCodeSelection(layout codeLayout, x, y int, shift bool, now time.Time) {
	at := g.codeOffsetAt(layout, x, y)
	p := &g.codePointer
	if !shift && now.Sub(p.lastAt) >= 0 && now.Sub(p.lastAt) <= 450*time.Millisecond &&
		absInt(x-p.lastX) <= 4 && absInt(y-p.lastY) <= 4 && at == p.lastOffset {
		p.clicks = p.clicks%3 + 1
	} else {
		p.clicks = 1
	}
	p.lastAt, p.lastX, p.lastY, p.lastOffset = now, x, y, at
	switch p.clicks {
	case 2:
		p.originStart, p.originEnd = g.code.WordRange(at)
		g.code.SelectWord(at)
	case 3:
		p.originStart, p.originEnd = g.code.LineRange(at)
		g.code.SelectLine(at)
	default:
		g.code.SetCursor(at, shift)
	}
	g.codeDragging = true
	g.codeSearchOpen = false
	g.codeHorizontalDragging, g.codeVerticalDragging, g.codeCompletionOpen = false, false, false
}

func (g *Game) beginCodeLineSelection(layout codeLayout, x, y int) {
	at := g.codeOffsetAt(layout, layout.Text.X, y)
	g.codePointer = codePointerState{clicks: 3}
	g.codePointer.originStart, g.codePointer.originEnd = g.code.LineRange(at)
	g.code.SelectLine(at)
	g.codeDragging = true
	g.codeSearchOpen = false
	g.codeHorizontalDragging, g.codeVerticalDragging, g.codeCompletionOpen = false, false, false
}

func (g *Game) dragCodeSelection(layout codeLayout, x, y int) {
	// Distance controls speed; holding at either edge continues selecting
	// while the viewport moves, even after the mouse stops moving.
	if y < layout.Text.Y {
		g.codeScrollLine -= min(6, 1+(layout.Text.Y-y)/codeLineHeight)
	} else if y >= layout.Text.Y+layout.Text.H {
		g.codeScrollLine += min(6, 1+(y-layout.Text.Y-layout.Text.H)/codeLineHeight)
	}
	if g.codePointer.clicks != 3 {
		if x < layout.Text.X {
			g.codeScrollColumn -= min(6, 1+(layout.Text.X-x)/codeCharWidth)
		} else if x >= layout.Text.X+layout.Text.W {
			g.codeScrollColumn += min(6, 1+(x-layout.Text.X-layout.Text.W)/codeCharWidth)
		}
	}
	g.clampCodeScroll(layout)
	x = min(max(x, layout.Text.X+8), layout.Text.X+layout.Text.W-9)
	y = min(max(y, layout.Text.Y+4), layout.Text.Y+layout.Text.H-9)
	g.extendCodeSelection(g.codeOffsetAt(layout, x, y))
}

func (g *Game) extendCodeSelection(at int) {
	p := &g.codePointer
	if p.clicks < 2 {
		g.code.SetCursor(at, true)
		return
	}
	var start, end int
	if p.clicks == 2 {
		start, end = g.code.WordRange(at)
	} else {
		start, end = g.code.LineRange(at)
	}
	if start < p.originStart {
		g.code.SetCursor(p.originEnd, false)
		g.code.SetCursor(start, true)
	} else {
		g.code.SetCursor(p.originStart, false)
		g.code.SetCursor(max(p.originEnd, end), true)
	}
}
