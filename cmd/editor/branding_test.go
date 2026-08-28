package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestMtileHeaderLogoIsCompactTransparentArtwork(t *testing.T) {
	logo, err := png.Decode(bytes.NewReader(mtileHeaderPNG))
	if err != nil {
		t.Fatalf("decode Mtile header logo: %v", err)
	}
	bounds := logo.Bounds()
	if bounds.Dx() != 220 || bounds.Dy() != 48 {
		t.Fatalf("Mtile header logo = %dx%d, want 220x48", bounds.Dx(), bounds.Dy())
	}
	for _, point := range [][2]int{{0, 0}, {bounds.Max.X - 1, 0}, {0, bounds.Max.Y - 1}, {bounds.Max.X - 1, bounds.Max.Y - 1}} {
		_, _, _, alpha := logo.At(point[0], point[1]).RGBA()
		if alpha != 0 {
			t.Fatalf("Mtile header logo corner %v is not transparent: alpha=%d", point, alpha)
		}
	}
}
