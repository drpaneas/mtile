package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

func preserveDisplayState(t *testing.T) {
	t.Helper()
	inputScale, drawScale := editorInputScale, editorDrawScale
	width, height := screenWidth, screenHeight
	t.Cleanup(func() {
		editorInputScale, editorDrawScale = inputScale, drawScale
		setEditorLayout(width, height)
	})
}

func TestEditorDisplayResolutionAndMonitorChanges(t *testing.T) {
	preserveDisplayState(t)
	w := &editorWindow{Game: &Game{}}
	for _, scale := range []float64{1, 1.25, 1.5, 2, 1.25, 1} {
		w.renderRequested = false
		width, height := w.layoutAtScale(1200, 800, scale)
		if width != 1200*scale || height != 800*scale {
			t.Fatalf("scale %g: framebuffer %gx%g", scale, width, height)
		}
		if screenWidth != 1200 || screenHeight != 800 || editorInputScale != scale {
			t.Fatalf("scale %g: logical layout %dx%d, input scale %g", scale, screenWidth, screenHeight, editorInputScale)
		}
		if !w.renderRequested {
			t.Fatal("monitor change did not invalidate retained frame")
		}
		w.renderRequested = false
		w.layoutAtScale(1200, 800, scale)
		if w.renderRequested {
			t.Fatal("unchanged display invalidates idle frame")
		}
	}
	for _, size := range [][2]float64{{1031, 800}, {1512, 982}, {1200.5, 800.25}, {1200.75, 800.25}} {
		w.renderRequested = false
		width, height := w.layoutAtScale(size[0], size[1], 1.5)
		if width != size[0]*1.5 || height != size[1]*1.5 || !w.renderRequested {
			t.Fatalf("resize %v: framebuffer %gx%g, invalidated %v", size, width, height, w.renderRequested)
		}
	}
}

func TestEditorInvalidDisplayScale(t *testing.T) {
	for _, scale := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if got := validDisplayScale(scale); got != 1 {
			t.Fatalf("invalid scale %g normalized to %g", scale, got)
		}
	}
}

func TestEditorDisplayFontRasterAndLogicalMetrics(t *testing.T) {
	for _, style := range uiFontStyles {
		for _, logical := range []*text.GoXFace{style.face, style.smallFace, style.titleFace, style.codeFace, style.developFace} {
			for _, scale := range []float64{1, 1.25, 1.5, 2, 1} {
				physical := displayFont(logical, scale).(*text.GoXFace)
				for _, value := range []string{"M", "i", "local value = 1", "()[]{};., /\\ != ->"} {
					want := text.Advance(value, logical) * scale
					if got := text.Advance(value, physical); math.Abs(got-want) > 0.02 {
						t.Fatalf("%q at %gx: advance %g, want %g", value, scale, got, want)
					}
				}
				if got, want := physical.Metrics().HAscent, logical.Metrics().HAscent*scale; math.Abs(got-want) > 0.02 {
					t.Fatalf("baseline at %gx: %g, want %g", scale, got, want)
				}
				if scale != 1 {
					bounds, _, _ := physical.UnsafeInternal().GlyphBounds('M')
					baseBounds, _, _ := logical.UnsafeInternal().GlyphBounds('M')
					if bounds.Max.Y-bounds.Min.Y <= baseBounds.Max.Y-baseBounds.Min.Y {
						t.Fatal("glyph raster did not gain physical resolution")
					}
					if physical != displayFont(logical, scale) {
						t.Fatal("unchanged density discarded glyph cache")
					}
				}
			}
		}
	}
}

func TestEditorDisplayPointerCodeSelectionAndScroll(t *testing.T) {
	preserveDisplayState(t)
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		g, layout := interactionFixture(strings.Repeat("local player_speed = 1\n", 100))
		point := func(column, row int) (int, int) {
			x := layout.Text.X + 8 + column*codeCharWidth
			y := layout.Text.Y + 1 + row*codeLineHeight
			return logicalCursorPosition((float64(x)+0.25)*scale, (float64(y)+0.25)*scale, scale)
		}
		for row := 0; row < 4; row++ {
			for col := 0; col < 20; col++ {
				x, y := point(col, row)
				line, column := g.code.LineColumn(g.codeOffsetAt(layout, x, y))
				if line != row || column != col {
					t.Fatalf("%gx click %d,%d selected %d,%d", scale, row, col, line, column)
				}
			}
		}
		x, y := point(6, 0)
		g.beginCodeSelection(layout, x, y, false, time.Unix(1000, 0))
		x, y = point(18, 0)
		g.dragCodeSelection(layout, x, y)
		if got := g.code.SelectedText(); got != "player_speed" {
			t.Fatalf("%gx drag selected %q", scale, got)
		}
		g.scrollCodeWheel(layout, 0, -6)
		g.finishCodeInput(layout, false)
		x, y = point(0, 0)
		if line, _ := g.code.LineColumn(g.codeOffsetAt(layout, x, y)); line != 18 {
			t.Fatalf("%gx wheel/click selected line %d, want 18", scale, line)
		}
		// Check both sides of a boundary before integer rounding, including
		// negative coordinates just outside the window.
		for _, logical := range []float64{-0.1, 0, 10.1, 10.9, 100.99} {
			got, _ := logicalCursorPosition(logical*scale, 0, scale)
			if got != int(math.Floor(logical)) {
				t.Fatalf("%gx boundary %g maps to %d", scale, logical, got)
			}
		}
	}
}
