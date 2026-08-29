package main

import (
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/editor"
)

func collectShape(tool spriteTool, x0, y0, x1, y1 int, filled bool) map[[2]int]bool {
	points := map[[2]int]bool{}
	visitSpriteShape(tool, x0, y0, x1, y1, filled, func(x, y int) {
		points[[2]int{x, y}] = true
	})
	return points
}

func TestMoveToolLeadsToolbarWithoutChangingDefaultPencil(t *testing.T) {
	if got := spriteTool(0); got != spriteToolPencil {
		t.Fatalf("zero-value sprite tool = %s, want PENCIL", got)
	}
	tool, ok := spriteToolForButton(0)
	if !ok || tool != spriteToolMove {
		t.Fatalf("first toolbar tool = %s (ok=%t), want MOVE", tool, ok)
	}
	tool, ok = spriteToolForButton(1)
	if !ok || tool != spriteToolPencil {
		t.Fatalf("second toolbar tool = %s (ok=%t), want PENCIL", tool, ok)
	}
}

func TestSpriteShapeRasterizersCoverExpectedGeometry(t *testing.T) {
	line := collectShape(spriteToolLine, 1, 1, 4, 4, false)
	for i := 1; i <= 4; i++ {
		if !line[[2]int{i, i}] {
			t.Fatalf("diagonal line omitted (%d,%d): %#v", i, i, line)
		}
	}

	outline := collectShape(spriteToolRect, 0, 0, 3, 3, false)
	if len(outline) != 12 || outline[[2]int{1, 1}] {
		t.Fatalf("rectangle outline = %#v, want 12 border cells and an empty center", outline)
	}
	filled := collectShape(spriteToolRect, 0, 0, 3, 3, true)
	if len(filled) != 16 || !filled[[2]int{1, 1}] {
		t.Fatalf("filled rectangle = %#v, want all 16 cells", filled)
	}

	circle := collectShape(spriteToolCircle, 0, 0, 6, 6, false)
	for _, point := range [][2]int{{0, 3}, {3, 0}, {6, 3}, {3, 6}} {
		if !circle[point] {
			t.Fatalf("circle omitted cardinal point %v: %#v", point, circle)
		}
	}
}

func TestFloodFillStopsAtColorBoundary(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	g.currentSprite = 1
	for y := 0; y < 8; y++ {
		if err := g.model.SetSpritePixel(1, 3, y, 2); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	if !g.fillSpriteSelection(0, 0, 5) {
		t.Fatal("flood fill reported no change")
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	if got := g.pixelAt(8, 0); got != 5 {
		t.Fatalf("left region = %d, want fill color 5", got)
	}
	if got := g.pixelAt(11, 0); got != 2 {
		t.Fatalf("boundary = %d, want original color 2", got)
	}
	if got := g.pixelAt(12, 0); got != 0 {
		t.Fatalf("right region crossed boundary: got %d, want 0", got)
	}
}

func TestFloodFillAndFlipKeepReservedSpriteZeroImmutable(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	g.currentSprite = editor.ReservedSpriteID
	if g.fillSpriteSelection(0, 0, 7) {
		t.Fatal("flood fill changed reserved sprite 0")
	}
	g.spriteTool = spriteToolLine
	g.spriteShapeStartX, g.spriteShapeStartY = 0, 0
	g.spriteShapeEndX, g.spriteShapeEndY = 7, 7
	if g.commitSpriteShape() {
		t.Fatal("primitive changed reserved sprite 0")
	}
	if g.flipSpriteSelection(true) {
		t.Fatal("flip changed a selection containing reserved sprite 0")
	}
	if !strings.Contains(g.statusMsg, "locked") {
		t.Fatalf("reserved operation status = %q, want locked explanation", g.statusMsg)
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	if got := g.pixelAt(0, 0); got != 0 {
		t.Fatalf("reserved pixel = %d, want transparent zero", got)
	}
}

func TestFlipTransformsWholeMultiSpriteSelection(t *testing.T) {
	g := newTestGame(t, 32, 16, 2, 2)
	g.currentSprite = 1
	g.gridCols = 2
	if err := g.model.SetSpritePixel(1, 0, 0, 3); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetSpritePixel(2, 7, 0, 4); err != nil {
		t.Fatal(err)
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	if !g.flipSpriteSelection(true) {
		t.Fatal("horizontal flip reported no change")
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	if got := g.pixelAt(8, 0); got != 4 {
		t.Fatalf("flipped left edge = %d, want original far-right color 4", got)
	}
	if got := g.pixelAt(23, 0); got != 3 {
		t.Fatalf("flipped right edge = %d, want original far-left color 3", got)
	}
}
