package editor

import (
	"image"
	"image/color"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestQuantizeImageToPaletteUsesNearestColorAndReservesZero(t *testing.T) {
	palette := []assetsync.RGB15{
		{R: 0, G: 0, B: 0},
		{R: 31, G: 0, B: 0},
		{R: 0, G: 31, B: 0},
	}
	img := image.NewNRGBA(image.Rect(5, 7, 8, 8))
	img.SetNRGBA(5, 7, color.NRGBA{A: 0})
	img.SetNRGBA(6, 7, color.NRGBA{R: 250, G: 8, B: 4, A: 255})
	img.SetNRGBA(7, 7, color.NRGBA{R: 0, G: 0, B: 0, A: 255})

	pixels := QuantizeImageToPalette(img, palette)
	if len(pixels) != 3 {
		t.Fatalf("quantized pixels = %d, want 3", len(pixels))
	}
	if pixels[0] != 0 {
		t.Fatalf("transparent pixel mapped to %d, want color 0", pixels[0])
	}
	if pixels[1] != 1 {
		t.Fatalf("near-red pixel mapped to %d, want red index 1", pixels[1])
	}
	if pixels[2] == 0 {
		t.Fatal("opaque pixel mapped to transparent color 0")
	}
}
