package main

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
)

func spriteDitherFixture(width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x*2 + 80), G: uint8(y*2 + 80), B: 128, A: 255})
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{})
	return img
}

func TestSpritePasteDitherPreviewIsTransientAndCached(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	g.currentSprite = 5
	img := spriteDitherFixture(16, 16)
	before, undoBefore := g.snapshot().clone(), len(g.undo.past)
	if err := g.beginSpritePasteImage(img); err != nil {
		t.Fatal(err)
	}
	if g.spritePaste.dither || g.spritePaste.ordered != nil {
		t.Fatal("paste eagerly enabled or computed dithering")
	}
	nearest := slices.Clone(g.spritePaste.pixels)
	if !slices.Equal(nearest, editor.QuantizeImageToPalette(img, g.model.ActivePalette.Colors)) {
		t.Fatal("default nearest output changed")
	}
	g.toggleSpritePasteDither()
	ordered := slices.Clone(g.spritePaste.pixels)
	if !g.spritePaste.dither || !strings.Contains(g.statusMsg, "Dither on (D)") || slices.Equal(nearest, ordered) {
		t.Fatal("toggle did not select a distinct dither preview")
	}
	g.moveSpritePastePlacement(1, 2)
	if !slices.Equal(ordered, g.spritePaste.pixels) {
		t.Fatal("tile movement changed dither phase")
	}
	g.toggleSpritePasteDither()
	if !slices.Equal(nearest, g.spritePaste.pixels) {
		t.Fatal("second toggle did not restore exact nearest pixels")
	}
	if !reflect.DeepEqual(g.model.Sheet, before.Sheet) || len(g.undo.past) != undoBefore {
		t.Fatal("preview changed model or undo history")
	}
	if allocs := testing.AllocsPerRun(100, g.toggleSpritePasteDither); allocs != 0 {
		t.Fatalf("full cached UI toggle allocated %g times", allocs)
	}
	g.cancelSpritePastePlacement()
	if g.spritePaste != nil || !reflect.DeepEqual(g.model.Sheet, before.Sheet) || len(g.undo.past) != undoBefore {
		t.Fatal("cancel changed project data")
	}
}

func TestSpritePasteDitherCommitIsOneUndoStep(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	g.currentSprite = 5
	before, undoBefore := g.snapshot().clone(), len(g.undo.past)
	if err := g.beginSpritePasteImage(spriteDitherFixture(16, 16)); err != nil {
		t.Fatal(err)
	}
	g.toggleSpritePasteDither()
	want := slices.Clone(g.spritePaste.pixels)
	if err := g.commitSpritePastePlacement(); err != nil {
		t.Fatal(err)
	}
	if g.spritePaste != nil || len(g.undo.past) != undoBefore+1 {
		t.Fatal("commit did not consume preview in exactly one undo step")
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if got := g.pixelAt(8+x, 8+y); got != want[y*16+x] {
				t.Fatalf("committed pixel (%d,%d)=%d want %d", x, y, got, want[y*16+x])
			}
		}
	}
	state, ok := g.undo.undo()
	if !ok {
		t.Fatal("missing commit undo")
	}
	g.restore(state)
	if !reflect.DeepEqual(g.model.Sheet, before.Sheet) {
		t.Fatal("undo did not restore sheet")
	}
}

func TestSpritePasteDitherUsesImportPaletteSnapshot(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	g.currentSprite = 5
	img := spriteDitherFixture(16, 16)
	if err := g.beginSpritePasteImage(img); err != nil {
		t.Fatal(err)
	}
	want := editor.QuantizeImageToPaletteOrdered(img, slices.Clone(g.model.ActivePalette.Colors))
	for i := range g.model.ActivePalette.Colors {
		g.model.ActivePalette.Colors[i] = assetsync.RGB15{R: 31}
	}
	g.toggleSpritePasteDither()
	if !slices.Equal(want, g.spritePaste.pixels) {
		t.Fatal("lazy quantization used mutable active palette instead of import snapshot")
	}
}

func TestUndoPaletteChangeCancelsSpritePastePreview(t *testing.T) {
	for _, dither := range []bool{false, true} {
		g := newTestGame(t, 32, 32, 2, 2)
		g.currentSprite = 5
		before := g.snapshot().clone()
		g.model.ActivePalette.Colors[1] = assetsync.RGB15{R: 31}
		g.undo.forcePush(g.snapshot())
		if err := g.beginSpritePasteImage(spriteDitherFixture(16, 16)); err != nil {
			t.Fatal(err)
		}
		if dither {
			g.toggleSpritePasteDither()
		}
		state, ok := g.undo.undo()
		if !ok {
			t.Fatal("missing palette undo")
		}
		g.restore(state)
		if g.spritePaste != nil {
			t.Fatal("undo left paste pixels associated with old palette")
		}
		if !reflect.DeepEqual(g.model.ActivePalette, before.Palette) || !reflect.DeepEqual(g.model.Sheet, before.Sheet) {
			t.Fatal("undo did not restore palette/sheet")
		}
	}
}

func TestSpritePasteDitherToggleLatency(t *testing.T) {
	out := os.Getenv("MTILE_DITHER_REPORT")
	if out == "" {
		t.Skip("set MTILE_DITHER_REPORT for full UI toggle latency")
	}
	g := newTestGame(t, 256, 512, 1, 1)
	g.currentSprite = 641
	img := spriteDitherFixture(128, 128)
	results := make(map[string]any)
	for _, p := range []struct{ name, project string }{{"pico-8", "pong"}, {"aurora", "testio"}} {
		palette, err := assetsync.LoadPalette(filepath.Join("..", "..", "examples", p.project), p.name)
		if err != nil {
			t.Fatal(err)
		}
		g.model.ActivePalette = *palette
		var ms []float64
		for i := -2; i < 40; i++ {
			if err := g.beginSpritePasteImage(img); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			g.toggleSpritePasteDither()
			elapsed := float64(time.Since(start)) / float64(time.Millisecond)
			if i >= 0 {
				ms = append(ms, elapsed)
			}
		}
		sorted := slices.Clone(ms)
		slices.Sort(sorted)
		p95 := sorted[37]
		allocs := testing.AllocsPerRun(100, g.toggleSpritePasteDither)
		results[p.name] = map[string]any{"firstToggleMs": ms, "p95Ms": p95, "cachedFullUIToggleAllocations": allocs}
		t.Logf("%s max128x128 first UI toggle p95=%.3fms; cached full toggle allocations=%g", p.name, p95, allocs)
		if p95 >= 50 || allocs != 0 {
			t.Errorf("toggle acceptance failed for %s", p.name)
		}
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "ui-toggle-latency.json"), append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
