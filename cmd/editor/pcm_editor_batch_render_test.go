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

func TestPCMBatchEditorRender(t *testing.T) {
	out, tool := os.Getenv("MTILE_PCM_BATCH_SCREENSHOTS"), os.Getenv("MTILE_PCM_MMUTIL")
	if out == "" || tool == "" {
		t.Skip("set MTILE_PCM_BATCH_SCREENSHOTS and MTILE_PCM_MMUTIL for batch visual QA")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range []struct {
		name string
		w, h int
	}{{"desktop", 1920, 1200}, {"compact", 1024, 900}} {
		g := newNativeMusicTestGame(t)
		defer g.closeNativePerformance()
		defer g.closePCMEditor()
		g.openPCMEditor()
		g.pcm.converter = tool
		g.Layout(size.w, size.h)
		capture := func(name string) {
			target := ebiten.NewImage(size.w, size.h)
			img := image.NewRGBA(target.Bounds())
			onRenderBenchmarkFrame(func() { g.requestRender(); g.Draw(target); target.ReadPixels(img.Pix) })
			f, err := os.Create(filepath.Join(out, size.name+"-"+name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, img)
			f.Close()
			target.Deallocate()
			if err != nil {
				t.Fatal(err)
			}
		}
		paths := []string{pcmQuietFixture(t), filepath.Join("..", "..", "examples/imported-audio/assets/audio/direct/source/sfx_bell.wav"), filepath.Join("..", "..", "examples/imported-audio/assets/audio/direct/source/music_theme.mod")}
		g.pcmChooseFiles(paths, false)
		waitPCM(t, g)
		for i, name := range []string{"door", "bell", "theme"} {
			g.pcmSelectBatch(i)
			waitPCM(t, g)
			g.pcm.name = name
			g.pcmSaveBatchName()
		}
		g.pcmSelectBatch(0)
		waitPCM(t, g)
		capture("review")
		g.pcmToggleTrim()
		waitPCM(t, g)
		capture("trimmed")
		g.pcmSelectBatch(2)
		waitPCM(t, g)
		capture("tracker")
		bad := filepath.Join(t.TempDir(), "broken.wav")
		os.WriteFile(bad, []byte("bad"), 0600)
		g.pcmChooseFiles([]string{bad}, true)
		waitPCM(t, g)
		g.pcmSelectBatch(3)
		waitPCM(t, g)
		capture("error")
		g.pcmRemoveQueued()
		waitPCM(t, g)
		g.pcmImport()
		waitPCM(t, g)
		if g.pcm.phase != "done" {
			t.Fatal(g.pcm.message)
		}
		g.pcmSelectBatch(0)
		waitPCM(t, g)
		capture("done")
	}
}
