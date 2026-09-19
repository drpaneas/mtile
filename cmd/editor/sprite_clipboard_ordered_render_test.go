//go:build mtile_editor_bench

package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/hajimehoshi/ebiten/v2"
)

// Render the actual editor with an in-memory import; never touch the user's
// clipboard or projects. The shared opt-in harness provides GPU readback.
func TestSpritePasteDitherRender(t *testing.T) {
	out := os.Getenv("MTILE_DITHER_REPORT")
	if out == "" {
		t.Skip("set MTILE_DITHER_REPORT to capture paste previews")
	}
	g := newTestGame(t, 128, 128, 8, 8)
	defer g.closeNativePerformance()
	g.preferences = defaultEditorPreferences()
	g.applyPreferences()
	g.Layout(1200, 800)
	palette, err := assetsync.LoadPalette(filepath.Join("..", "..", "examples", "pong"), "pico-8")
	if err != nil {
		t.Fatal(err)
	}
	g.model.ActivePalette = *palette
	g.model.Project.ActivePalette = palette.Name
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	g.currentSprite = 17
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			v := uint8((x + y) * 255 / 126)
			img.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	if err := g.beginSpritePasteImage(img); err != nil {
		t.Fatal(err)
	}
	target := ebiten.NewImage(1200, 800)
	defer target.Deallocate()
	pixels := make([]byte, 1200*800*4)
	for _, name := range []string{"editor-nearest", "editor-ordered"} {
		if name == "editor-ordered" {
			g.toggleSpritePasteDither()
		}
		onRenderBenchmarkFrame(func() { g.requestRender(); g.Draw(target); target.ReadPixels(pixels) })
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, &image.RGBA{Pix: pixels, Stride: 1200 * 4, Rect: image.Rect(0, 0, 1200, 800)})
		closeErr := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
}
