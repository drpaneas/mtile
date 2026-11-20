package editor

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func orderedTestColor(c assetsync.RGB15) color.NRGBA {
	return color.NRGBA{R: uint8((c.R*255 + 15) / 31), G: uint8((c.G*255 + 15) / 31), B: uint8((c.B*255 + 15) / 31), A: 255}
}

func TestOrderedImportPreservesExactColorsAndTransparency(t *testing.T) {
	for _, size := range []int{16, 256} {
		palette := imageImportBenchmarkPalette(size)
		palette[2] = palette[1] // exact duplicate must retain first paint index
		img := image.NewNRGBA(image.Rect(5, 7, 5+size, 11))
		for i := range palette {
			for row, alpha := range []uint8{0, 1, 127, 255} {
				c := orderedTestColor(palette[max(1, i)])
				c.A = alpha
				img.SetNRGBA(5+i, 7+row, c)
			}
		}
		got, want := QuantizeImageToPaletteOrdered(img, palette), QuantizeImageToPalette(img, palette)
		if !slices.Equal(got, want) {
			t.Fatalf("%d-color exact palette/alpha mapping changed", size)
		}
		for i, index := range got {
			if (i < size) != (index == 0) {
				t.Fatalf("pixel %d index %d violates transparent-only index zero", i, index)
			}
		}
	}
	if got := QuantizeImageToPaletteOrdered(nil, nil); got != nil {
		t.Fatal("nil image did not return nil")
	}
}

func TestOrderedImportBayerCoverageAndTilePhase(t *testing.T) {
	palette := []assetsync.RGB15{{}, {}, {R: 31, G: 31, B: 31}}
	img := image.NewNRGBA(image.Rect(5, 7, 29, 23))
	zero := image.NewNRGBA(image.Rect(0, 0, 24, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			c := color.NRGBA{R: 128, G: 128, B: 128, A: 255}
			img.SetNRGBA(5+x, 7+y, c)
			zero.SetNRGBA(x, y, c)
		}
	}
	got := QuantizeImageToPaletteOrdered(img, palette)
	if !slices.Equal(got, QuantizeImageToPaletteOrdered(zero, palette)) {
		t.Fatal("image bounds changed Bayer phase")
	}
	black := 0
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if got[y*24+x] == 1 {
				black++
			}
		}
	}
	if black != 8 {
		t.Fatalf("midpoint coverage = %d/16 black, want 8", black)
	}
	if uniqueImportTiles(got, 24, 16) != 1 {
		t.Fatal("identical 8x8 tiles acquired different dither patterns")
	}
	if !slices.Equal(got, QuantizeImageToPaletteOrdered(img, palette)) {
		t.Fatal("ordered import is nondeterministic")
	}
}

func TestOrderedImportPreservesAuthoredPixelArt(t *testing.T) {
	for _, fixture := range []struct {
		project string
		colors  int
	}{
		{"examples/lantern-rooms", 16},
		{"testdata/projects/testio", 256},
	} {
		dir := filepath.Join("..", "..", fixture.project)
		p, err := assetsync.LoadProject(dir)
		if err != nil {
			t.Fatal(err)
		}
		palette, err := assetsync.LoadPalette(dir, p.ActivePalette)
		if err != nil {
			t.Fatal(err)
		}
		if len(palette.Colors) != fixture.colors {
			t.Fatalf("%s palette has %d colors, want %d", fixture.project, len(palette.Colors), fixture.colors)
		}
		sheet, err := assetsync.LoadSpritesheet(dir)
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewNRGBA(image.Rect(0, 0, min(sheet.Width, 128), min(sheet.Height, 128)))
		visible := 0
		for _, sprite := range sheet.Sprites {
			for y, row := range sprite.Pixels {
				for x, index := range row {
					if index != 0 && image.Pt(sprite.X+x, sprite.Y+y).In(img.Bounds()) {
						img.SetNRGBA(sprite.X+x, sprite.Y+y, orderedTestColor(palette.Colors[index]))
						visible++
					}
				}
			}
		}
		if visible == 0 {
			t.Fatalf("%s authored-art fixture has no visible pixels", fixture.project)
		}
		if !slices.Equal(QuantizeImageToPaletteOrdered(img, palette.Colors), QuantizeImageToPalette(img, palette.Colors)) {
			t.Fatalf("%s authored pixel art changed", fixture.project)
		}
	}
}

type orderedQualityRow struct {
	Palette                string  `json:"palette"`
	Fixture                string  `json:"fixture"`
	NearestRawMSE          float64 `json:"nearestRawMSE"`
	OrderedRawMSE          float64 `json:"orderedRawMSE"`
	NearestBox2MSE         float64 `json:"nearestBox2MSE"`
	OrderedBox2MSE         float64 `json:"orderedBox2MSE"`
	NearestBox4MSE         float64 `json:"nearestBox4MSE"`
	OrderedBox4MSE         float64 `json:"orderedBox4MSE"`
	Box4ImprovementPercent float64 `json:"box4ImprovementPercent"`
	NearestUniqueTiles     int     `json:"nearestUnique8x8Tiles"`
	OrderedUniqueTiles     int     `json:"orderedUnique8x8Tiles"`
}

// Fixed before measurement: horizontal/vertical gray and color ramps, a
// diagonal color ramp, and an orange shaded sphere with transparent surround.
func orderedQualityFixtures() map[string]*image.NRGBA {
	out := make(map[string]*image.NRGBA)
	for _, name := range []string{"gray-horizontal", "gray-vertical", "color-horizontal", "color-vertical", "color-diagonal", "shaded-sphere"} {
		img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
		for y := 0; y < 128; y++ {
			for x := 0; x < 128; x++ {
				v, w := x*255/127, y*255/127
				c := color.NRGBA{A: 255}
				switch name {
				case "gray-horizontal":
					c.R, c.G, c.B = uint8(v), uint8(v), uint8(v)
				case "gray-vertical":
					c.R, c.G, c.B = uint8(w), uint8(w), uint8(w)
				case "color-horizontal":
					c.R, c.G, c.B = uint8(v), 90, uint8(220-v/2)
				case "color-vertical":
					c.R, c.G, c.B = uint8(40+w/2), uint8(w), uint8(220-w/3)
				case "color-diagonal":
					v = (v + w) / 2
					c.R, c.G, c.B = uint8(v), uint8(255-v), uint8(40+v/2)
				case "shaded-sphere":
					xf, yf := float64(x-64)/56, float64(y-64)/56
					if xf*xf+yf*yf > 1 {
						c.A = 0
						break
					}
					light := 0.25 + 0.75*max(0.0, -0.35*xf-0.4*yf+0.84*math.Sqrt(1-xf*xf-yf*yf))
					c.R, c.G, c.B = uint8(240*light), uint8(150*light), uint8(70*light)
				}
				img.SetNRGBA(x, y, c)
			}
		}
		out[name] = img
	}
	return out
}

// Error is measured in encoded RGB15, not claimed as a perceptual metric.
// Each box averages only visible source pixels; both outputs share its mask.
func importBoxMSE(img *image.NRGBA, pixels []int, palette []assetsync.RGB15, box int) float64 {
	var sum float64
	count := 0
	for by := 0; by < img.Rect.Dy(); by += box {
		for bx := 0; bx < img.Rect.Dx(); bx += box {
			var delta [3]int
			visible := 0
			for y := by; y < min(by+box, img.Rect.Dy()); y++ {
				for x := bx; x < min(bx+box, img.Rect.Dx()); x++ {
					s := img.NRGBAAt(x, y)
					if s.A == 0 {
						continue
					}
					p := palette[pixels[y*img.Rect.Dx()+x]]
					delta[0] += (int(s.R)*31+127)/255 - p.R
					delta[1] += (int(s.G)*31+127)/255 - p.G
					delta[2] += (int(s.B)*31+127)/255 - p.B
					visible++
				}
			}
			if visible == 0 {
				continue
			}
			for _, d := range delta {
				mean := float64(d) / float64(visible)
				sum += mean * mean
			}
			count += 3
		}
	}
	return sum / float64(count)
}

func uniqueImportTiles(pixels []int, width, height int) int {
	unique := make(map[[64]byte]struct{})
	for by := 0; by < height; by += 8 {
		for bx := 0; bx < width; bx += 8 {
			var tile [64]byte
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					tile[y*8+x] = byte(pixels[(by+y)*width+bx+x])
				}
			}
			unique[tile] = struct{}{}
		}
	}
	return len(unique)
}

func writeOrderedComparison(path string, source *image.NRGBA, nearest, ordered []int, palette []assetsync.RGB15) error {
	// Left-to-right: source, nearest, ordered; 3x nearest-neighbor enlargement.
	out := image.NewNRGBA(image.Rect(0, 0, 3*128*3, 128*3))
	for panel := 0; panel < 3; panel++ {
		for y := 0; y < 128; y++ {
			for x := 0; x < 128; x++ {
				c := source.NRGBAAt(x, y)
				if panel > 0 && c.A != 0 {
					index := nearest[y*128+x]
					if panel == 2 {
						index = ordered[y*128+x]
					}
					c = orderedTestColor(palette[index])
				}
				if c.A == 0 {
					gray := uint8(45 + ((x/4+y/4)&1)*20)
					c = color.NRGBA{R: gray, G: gray, B: gray, A: 255}
				}
				for dy := 0; dy < 3; dy++ {
					for dx := 0; dx < 3; dx++ {
						out.SetNRGBA(panel*128*3+x*3+dx, y*3+dy, c)
					}
				}
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(f, out)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func TestOrderedDitherReport(t *testing.T) {
	out := os.Getenv("MTILE_DITHER_REPORT")
	if out == "" {
		t.Skip("set MTILE_DITHER_REPORT to export the fixed experiment corpus")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	var rows []orderedQualityRow
	var improvements []float64
	fixtures := orderedQualityFixtures()
	names := make([]string, 0, len(fixtures))
	for name := range fixtures {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, p := range []struct{ name, project string }{{"pico-8", "pong"}, {"aurora", "testio"}} {
		base := "examples"
		if p.project == "testio" {
			base = filepath.Join("testdata", "projects")
		}
		palette, err := assetsync.LoadPalette(filepath.Join("..", "..", base, p.project), p.name)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			img := fixtures[name]
			nearest := QuantizeImageToPalette(img, palette.Colors)
			ordered := QuantizeImageToPaletteOrdered(img, palette.Colors)
			row := orderedQualityRow{Palette: p.name, Fixture: name,
				NearestRawMSE: importBoxMSE(img, nearest, palette.Colors, 1), OrderedRawMSE: importBoxMSE(img, ordered, palette.Colors, 1),
				NearestBox2MSE: importBoxMSE(img, nearest, palette.Colors, 2), OrderedBox2MSE: importBoxMSE(img, ordered, palette.Colors, 2),
				NearestBox4MSE: importBoxMSE(img, nearest, palette.Colors, 4), OrderedBox4MSE: importBoxMSE(img, ordered, palette.Colors, 4),
				NearestUniqueTiles: uniqueImportTiles(nearest, 128, 128), OrderedUniqueTiles: uniqueImportTiles(ordered, 128, 128)}
			row.Box4ImprovementPercent = 100 * (1 - row.OrderedBox4MSE/row.NearestBox4MSE)
			improvements = append(improvements, row.Box4ImprovementPercent)
			rows = append(rows, row)
			if err := writeOrderedComparison(filepath.Join(out, p.name+"-"+name+".png"), img, nearest, ordered, palette.Colors); err != nil {
				t.Fatal(err)
			}
		}
	}
	slices.Sort(improvements)
	median := (improvements[5] + improvements[6]) / 2
	data, err := json.MarshalIndent(struct {
		Rows   []orderedQualityRow `json:"rows"`
		Median float64             `json:"medianBox4ImprovementPercent"`
	}{rows, median}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "quality.json"), append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("fixed 12-image corpus: median 4x4 RGB15 mean-error reduction %.2f%%", median)
	if median < 15 {
		t.Fatalf("quality gate: %.2f%% < 15%%", median)
	}
	// End-to-end function latency at maximum dimensions, with two warmups.
	latencies := make(map[string][]float64)
	for _, size := range []int{16, 256} {
		img, palette := imageImportBenchmarkImage(), imageImportBenchmarkPalette(size)
		name := "16"
		if size == 256 {
			name = "256"
		}
		for i := -2; i < 40; i++ {
			start := time.Now()
			imageImportBenchmarkPixels = QuantizeImageToPaletteOrdered(img, palette)
			elapsed := float64(time.Since(start)) / float64(time.Millisecond)
			if i >= 0 {
				latencies[name] = append(latencies[name], elapsed)
			}
		}
		sorted := slices.Clone(latencies[name])
		slices.Sort(sorted)
		p95 := sorted[37]
		t.Logf("128x128/%d colors first-compute p95 %.3fms", size, p95)
		if p95 >= 50 {
			t.Errorf("first-compute latency gate: %.3fms >= 50ms", p95)
		}
	}
	data, err = json.MarshalIndent(latencies, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "latency-ms.json"), append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
