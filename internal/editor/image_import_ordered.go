package editor

import (
	"image"
	"image/color"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// QuantizeImageToPaletteOrdered offers a tile-stable alternative to nearest
// color import. Each visible pixel mixes its nearest RGB15 entry with one
// other paint color on a 4x4 Bayer grid. An exact RGB15 match is never mixed;
// transparent pixels remain zero, and zero is never used as a paint color.
// The pattern is local to the image, so every 8-pixel tile has the same phase.
func QuantizeImageToPaletteOrdered(img image.Image, palette []assetsync.RGB15) []int {
	if img == nil {
		return nil
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	pixels := make([]int, width*height)
	if len(palette) < 2 {
		return pixels
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			source := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			if source.A == 0 {
				continue
			}
			nearest, other, count := orderedPaletteMixture(source, palette)
			index := nearest
			if bayer4[y&3][x&3] < count {
				index = other
			}
			pixels[y*width+x] = index
		}
	}
	return pixels
}

var bayer4 = [4][4]int{
	{0, 8, 2, 10},
	{12, 4, 14, 6},
	{3, 11, 1, 9},
	{15, 7, 13, 5},
}

// Only pairs containing the nearest color are considered: two linear palette
// scans per pixel, never all color pairs. Integer projection rounds the mixture
// to one of the 16 available Bayer coverages before comparing its mean error.
func orderedPaletteMixture(source color.NRGBA, palette []assetsync.RGB15) (nearest, other, count int) {
	nearest = nearestPaletteIndex(source, palette)
	a := palette[nearest]
	r := (int(source.R)*31 + 127) / 255
	g := (int(source.G)*31 + 127) / 255
	b := (int(source.B)*31 + 127) / 255
	er, eg, eb := r-a.R, g-a.G, b-a.B
	bestError := 256 * (er*er + eg*eg + eb*eb)
	other = nearest
	if bestError == 0 {
		return
	}
	for i := 1; i < len(palette); i++ {
		candidate := palette[i]
		dr, dg, db := candidate.R-a.R, candidate.G-a.G, candidate.B-a.B
		length := dr*dr + dg*dg + db*db
		projection := er*dr + eg*dg + eb*db
		if length == 0 || projection <= 0 {
			continue
		}
		k := (16*projection + length/2) / length
		// An endpoint cannot beat the nearest entry already selected.
		if k <= 0 || k >= 16 {
			continue
		}
		rr, rg, rb := 16*er-k*dr, 16*eg-k*dg, 16*eb-k*db
		error := rr*rr + rg*rg + rb*rb
		if error < bestError {
			other, count, bestError = i, k, error
		}
	}
	return
}
