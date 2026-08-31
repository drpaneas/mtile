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

func TestCheckerboardShaderCompiles(t *testing.T) {
	if _, err := checkerboardFillShader(); err != nil {
		t.Fatalf("compile checkerboard shader: %v", err)
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

func TestJetBrainsMonoCodeFaceKeepsEveryPrintableASCIICharacterOnGrid(t *testing.T) {
	defer setInterfaceTextSize(textComfort)
	wantCells := [...]int{7, 8, 10}
	wantHeights := [...]int{16, 19, 22}
	for size := interfaceTextSize(0); size < interfaceTextSizeCount; size++ {
		setInterfaceTextSize(size)
		if codeCharWidth != wantCells[size] || codeTextHeight != wantHeights[size] {
			t.Fatalf("preset %d JetBrains Mono metrics = %dx%d, want %dx%d", size, codeCharWidth, codeTextHeight, wantCells[size], wantHeights[size])
		}
		for r := rune(32); r <= 126; r++ {
			if width := codeTextWidth(string(r)); width != codeCharWidth {
				t.Fatalf("preset %d rune %q width = %d, want cell width %d", size, r, width, codeCharWidth)
			}
		}
		const sample = "local cam_x=(scroll_period+1)%240"
		if got, want := codeTextWidth(sample), len([]rune(sample))*codeCharWidth; got != want {
			t.Fatalf("preset %d sample width = %d, want %d", size, got, want)
		}
	}
}

func TestDevelopChromeUsesProportionalSansWhileSourceStaysMonospaced(t *testing.T) {
	defer setInterfaceTextSize(textComfort)
	for size := interfaceTextSize(0); size < interfaceTextSizeCount; size++ {
		setInterfaceTextSize(size)
		if narrow, wide := developTextWidth("i"), developTextWidth("W"); narrow >= wide {
			t.Fatalf("preset %d Develop chrome is not proportional: i=%d W=%d", size, narrow, wide)
		}
		if codeTextWidth("i") != codeTextWidth("W") {
			t.Fatalf("preset %d source font lost its fixed grid", size)
		}
	}
}
