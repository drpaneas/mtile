//go:build mtile_editor_bench

package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/audioimport"
	"github.com/hajimehoshi/ebiten/v2"
)

func TestPCMEditorRender(t *testing.T) {
	out := os.Getenv("MTILE_PCM_SCREENSHOTS")
	if out == "" {
		t.Skip("set MTILE_PCM_SCREENSHOTS for visual QA")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	source := filepath.Join("..", "..", "examples/imported-audio/assets/audio/direct/source/sfx_bell.wav")
	p, err := audioimport.Prepare(source, audioimport.QualityBalanced)
	if err != nil {
		t.Fatal(err)
	}
	large, err := audioimport.Prepare(pcmTimedQuietFixture(t, 16000, 40), audioimport.QualityBalanced)
	if err != nil {
		t.Fatal(err)
	}
	quietSource := pcmQuietFixture(t)
	quiet, err := audioimport.Prepare(quietSource, audioimport.QualityBalanced)
	if err != nil {
		t.Fatal(err)
	}
	trimmed, err := audioimport.PrepareWithOptions(quietSource, audioimport.PrepareOptions{Quality: audioimport.QualityBalanced, TrimSilence: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct {
		name string
		w, h int
	}{{"desktop", 1920, 1200}, {"compact", 1024, 900}} {
		g.Layout(size.w, size.h)
		for _, phase := range []string{"choose", "review", "asset", "done", "error", "quiet", "trimmed", "large"} {
			if phase == "asset" {
				tool := os.Getenv("MTILE_PCM_MMUTIL")
				if tool != "" {
					if _, err := audioimport.ImportPrepared(g.model.Dir, p, "jump", tool); err != nil {
						t.Fatal(err)
					}
					g.reloadPCMLibrary()
				}
			}
			g.pcm.phase = phase
			g.pcm.path = "/Users/developer/Music/jump.wav"
			g.pcm.name = "jump"
			g.pcm.prepared = p
			g.pcm.trimSilence = false
			g.pcm.command = "sample(0)"
			g.pcm.message = ""
			if phase == "error" {
				g.pcm.phase = "review"
				g.pcm.message = "mmutil not found. Install the Maxmod converter, then choose MMUTIL above to select its executable and retry IMPORT + SAVE."
			}
			if phase == "done" {
				g.pcm.message = "Imported and saved. Copy the Lua call, then Build + Reload to hear it in your game."
			}
			if phase == "quiet" || phase == "trimmed" {
				g.pcm.phase = "review"
				g.pcm.prepared = quiet
				g.pcm.path = quietSource
			}
			if phase == "trimmed" {
				g.pcm.prepared = trimmed
				g.pcm.trimSilence = true
			}
			if phase == "large" {
				g.pcm.phase = "review"
				g.pcm.prepared = large
			}
			target := ebiten.NewImage(size.w, size.h)
			img := image.NewRGBA(target.Bounds())
			onRenderBenchmarkFrame(func() {
				g.requestRender()
				g.Draw(target)
				target.ReadPixels(img.Pix)
				if phase == "asset" {
					t.Logf("asset capture %s: music=%v bytes=%d phase=%s preview=%+v", size.name, g.pcm.prepared.Music, len(g.pcm.prepared.Data), g.pcm.phase, newPCMLayout().Preview)
				}
			})
			f, err := os.Create(filepath.Join(out, size.name+"-"+phase+".png"))
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
	}
}
