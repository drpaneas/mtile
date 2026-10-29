//go:build mtile_editor_bench

package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFontEditorRender(t *testing.T) {
	out := os.Getenv("MTILE_FONT_SCREENSHOTS")
	if out == "" {
		t.Skip("set MTILE_FONT_SCREENSHOTS for screenshots")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	g := newTestGame(t, 8, 8, 1, 1)
	defer g.closeNativePerformance()
	g.openFontEditor()
	g.model.SelectFont("international")
	g.fontEditor.name = "my-font"
	g.fontEditor.glyphInput = "Ω"
	g.fontEditor.selected = 'Ω'
	for _, view := range []struct {
		name string
		w, h int
		menu bool
	}{{"desktop", 1600, 1000, false}, {"compact", 1024, 768, false}, {"dropdown", 1600, 1000, true}, {"scratch", 1024, 768, false}, {"overflow", 1024, 768, false}} {
		g.Layout(view.w, view.h)
		g.fontEditor.menu = view.menu
		if view.name == "overflow" {
			g.model.SelectFont("international")
			g.fontEditor.preview = strings.Repeat("A", 150) + strings.Repeat("🚀", 10)
		}
		if view.name == "scratch" {
			g.model.CreateFont("my-font", true)
			g.fontEditor.selected = 'A'
		}
		target := ebiten.NewImage(view.w, view.h)
		img := image.NewRGBA(target.Bounds())
		onRenderBenchmarkFrame(func() { g.requestRender(); g.Draw(target); target.ReadPixels(img.Pix) })
		file, err := os.Create(filepath.Join(out, view.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, img)
		file.Close()
		target.Deallocate()
		if err != nil {
			t.Fatal(err)
		}
	}
}
