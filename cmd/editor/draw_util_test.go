package main

import (
	"image/color"
	"testing"

	"github.com/drpaneas/mtile/internal/editor"
)

func TestCheckerCellSizeStaysPositive(t *testing.T) {
	tests := []struct {
		name string
		w, h int
		want int
	}{
		{name: "empty", w: 0, h: 0, want: 1},
		{name: "tiny", w: 2, h: 2, want: 1},
		{name: "swatch", w: 10, h: 10, want: 4},
		{name: "map tile", w: 30, h: 30, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkerCellSize(editor.Rect{W: tt.w, H: tt.h})
			if got != tt.want {
				t.Fatalf("checkerCellSize(%dx%d) = %d, want %d", tt.w, tt.h, got, tt.want)
			}
		})
	}
}

func TestSpriteTransparencyCheckerMatchesDarkCanvas(t *testing.T) {
	if got, want := colorSpriteCheckerA, (color.RGBA{R: 18, G: 19, B: 33, A: 255}); got != want {
		t.Fatalf("checker A = %#v, want %#v", got, want)
	}
	if got, want := colorSpriteCheckerB, (color.RGBA{R: 24, G: 24, B: 40, A: 255}); got != want {
		t.Fatalf("checker B = %#v, want %#v", got, want)
	}
}

func TestSpritesheetGridKeepsOnlyTileBoundaries(t *testing.T) {
	if _, visible := spriteGridLineColor(1, false); visible {
		t.Fatal("spritesheet exposed a minor source-pixel grid line")
	}
	if got, visible := spriteGridLineColor(8, false); !visible || got != spriteCanvasGridColor(44) {
		t.Fatalf("spritesheet tile boundary = (%v, %t), want subdued shared grid color", got, visible)
	}
	if got, visible := spriteGridLineColor(8, true); !visible || got != spriteCanvasGridColor(44) {
		t.Fatalf("editing-canvas tile boundary = (%v, %t), want subdued major grid color", got, visible)
	}
	if got, visible := spriteGridLineColor(1, true); !visible || got != spriteCanvasGridColor(28) {
		t.Fatalf("editing-canvas pixel boundary = (%v, %t), want minor grid color", got, visible)
	}
}
