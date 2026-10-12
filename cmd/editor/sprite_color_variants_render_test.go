//go:build mtile_editor_bench

package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestPoseVariantEditorRender(t *testing.T) {
	out := os.Getenv("MTILE_VARIANT_REPORT")
	if out == "" {
		t.Skip("set MTILE_VARIANT_REPORT for screenshots")
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
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.selectPose(0)
	g.poseVariants.open = true
	for i, v := range g.model.Poses.Variants {
		if v.Name == "spent" {
			g.poseVariants.selected = i
		}
	}
	for _, size := range []struct {
		name string
		w, h int
	}{{"desktop", 1920, 1200}, {"compact", 1024, 900}} {
		g.Layout(size.w, size.h)
		for _, view := range []string{"variants", "picker", "map"} {
			g.mode = modeSprites
			g.poseVariants.picking = view == "picker"
			g.poseVariants.source = 158
			if view == "map" {
				g.mode = modeMap
				g.prepareMapView()
			}
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
