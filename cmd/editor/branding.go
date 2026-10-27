package main

import (
	"bytes"
	_ "embed"
	"image/png"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed assets/mtile-header.png
var mtileHeaderPNG []byte

var (
	mtileHeaderOnce  sync.Once
	mtileHeaderImage *ebiten.Image
)

func loadMtileHeaderImage() *ebiten.Image {
	mtileHeaderOnce.Do(func() {
		decoded, err := png.Decode(bytes.NewReader(mtileHeaderPNG))
		if err != nil {
			return
		}
		mtileHeaderImage = ebiten.NewImageFromImage(decoded)
	})
	return mtileHeaderImage
}

// drawMtileHeaderLogo keeps the logo pixel-sharp and aspect-correct. The
// source asset is already sized for the 64-72 px workspace headers, but the
// bounds make this safe for the compact navigation strip too.
func drawMtileHeaderLogo(screen *ebiten.Image, x, y, maxWidth, maxHeight int) (width, height int) {
	logo := loadMtileHeaderImage()
	if logo == nil || maxWidth <= 0 || maxHeight <= 0 {
		return 0, 0
	}
	bounds := logo.Bounds()
	width, height = bounds.Dx(), bounds.Dy()
	scale := min(float64(maxWidth)/float64(width), float64(maxHeight)/float64(height))
	if scale > 1 {
		scale = 1
	}
	width, height = int(float64(width)*scale), int(float64(height)*scale)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	op.GeoM.Scale(editorDrawScale, editorDrawScale)
	screen.DrawImage(logo, op)
	return width, height
}
