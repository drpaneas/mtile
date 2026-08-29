package editor

import (
	"image"
	"image/color"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// QuantizeImageToPalette preserves transparency as palette index zero and
// maps every visible pixel to the nearest non-transparent RGB15 entry.
// Comparing in the palette's native 5-bit space makes exact colors stable.
func QuantizeImageToPalette(img image.Image, palette []assetsync.RGB15) []int {
	if img == nil {
		return nil
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	pixels := make([]int, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			source := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			if source.A == 0 {
				continue
			}
			pixels[y*width+x] = nearestPaletteIndex(source, palette)
		}
	}
	return pixels
}

func nearestPaletteIndex(source color.NRGBA, palette []assetsync.RGB15) int {
	if len(palette) < 2 {
		return 0
	}
	r := (int(source.R)*31 + 127) / 255
	g := (int(source.G)*31 + 127) / 255
	b := (int(source.B)*31 + 127) / 255
	bestIndex := 1
	bestDistance := colorDistanceSquared(r, g, b, palette[bestIndex])
	for i := 2; i < len(palette); i++ {
		distance := colorDistanceSquared(r, g, b, palette[i])
		if distance < bestDistance {
			bestIndex, bestDistance = i, distance
		}
	}
	return bestIndex
}

func colorDistanceSquared(r, g, b int, candidate assetsync.RGB15) int {
	dr, dg, db := r-candidate.R, g-candidate.G, b-candidate.B
	return dr*dr + dg*dg + db*db
}
