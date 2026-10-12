//go:build mtile_editor_bench

package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestWorkspacePolishRender(t *testing.T) {
	out := os.Getenv("MTILE_POLISH_REPORT")
	if out == "" {
		t.Skip("set MTILE_POLISH_REPORT for screenshots")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "examples", "celeste"))); err != nil {
		t.Fatal(err)
	}
	g, err := newGame(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.closeNativePerformance()
	g.theme.level = effectsClean
	for _, size := range []struct {
		name string
		w, h int
	}{{"desktop", 1920, 1200}, {"compact", 1024, 900}} {
		g.Layout(size.w, size.h)
		for _, view := range []string{"sprite", "map", "audio", "develop"} {
			modes := map[string]editorMode{"sprite": modeSprites, "map": modeMap, "audio": modeMusic, "develop": modeCode}
			g.mode = modes[view]
			g.prepareMapView()
			w, h := g.mapBounds()
			g.mapCam.Fit(w, h, newMapLayout(size.w, size.h).Canvas)
			target := ebiten.NewImage(size.w, size.h)
			pixels := make([]byte, size.w*size.h*4)
			onRenderBenchmarkFrame(func() { g.requestRender(); g.Draw(target); target.ReadPixels(pixels) })
			f, err := os.Create(filepath.Join(out, size.name+"-"+view+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, &image.NRGBA{Pix: pixels, Stride: size.w * 4, Rect: image.Rect(0, 0, size.w, size.h)})
			f.Close()
			target.Deallocate()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
