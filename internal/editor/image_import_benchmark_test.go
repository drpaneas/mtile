package editor

import (
	"image"
	"image/color"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

var imageImportBenchmarkPixels []int

func imageImportBenchmarkImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	// A fixed, high-variation source exercises palette searches across RGB15.
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 2), G: uint8(y * 2), B: uint8((x*17 + y*29) & 255), A: 255})
		}
	}
	return img
}

func imageImportBenchmarkPalette(size int) []assetsync.RGB15 {
	palette := make([]assetsync.RGB15, size)
	for i := range palette {
		palette[i] = assetsync.RGB15{R: (i * 13) & 31, G: (i*7 + i/32*3) & 31, B: (i*19 + i/16*5) & 31}
	}
	return palette
}

func BenchmarkImageImportNearest(b *testing.B) {
	benchmarkImageImport(b, QuantizeImageToPalette)
}

func BenchmarkImageImportOrdered(b *testing.B) {
	benchmarkImageImport(b, QuantizeImageToPaletteOrdered)
}

func benchmarkImageImport(b *testing.B, quantize func(image.Image, []assetsync.RGB15) []int) {
	for _, size := range []int{16, 256} {
		name := "16"
		if size == 256 {
			name = "256"
		}
		b.Run(name, func(b *testing.B) {
			img, palette := imageImportBenchmarkImage(), imageImportBenchmarkPalette(size)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				imageImportBenchmarkPixels = quantize(img, palette)
			}
		})
	}
}
