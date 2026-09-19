//go:build mtile_editor_bench

package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// The opt-in test loop makes GPU readback available to benchmarks without
// changing the ordinary editor test suite. This also works with Ebitengine 2.9.
type renderBenchmarkLoop struct {
	exit <-chan int
	done chan struct{}
	code int
}

type renderBenchmarkJob struct {
	run  func()
	done chan struct{}
}

var renderBenchmarkJobs = make(chan renderBenchmarkJob)

func (g *renderBenchmarkLoop) Update() error {
	select {
	case g.code = <-g.exit:
		return ebiten.Termination
	case job := <-renderBenchmarkJobs:
		job.run()
		g.done = job.done
	default:
	}
	return nil
}

func (g *renderBenchmarkLoop) Draw(*ebiten.Image) {
	if g.done != nil {
		close(g.done)
		g.done = nil
	}
}
func (*renderBenchmarkLoop) Layout(int, int) (int, int) { return 64, 64 }

func onRenderBenchmarkFrame(run func()) {
	done := make(chan struct{})
	renderBenchmarkJobs <- renderBenchmarkJob{run: run, done: done}
	<-done
}

func TestMain(m *testing.M) {
	ebiten.SetWindowSize(64, 64)
	ebiten.SetWindowTitle("mTile render benchmark")
	ebiten.SetScreenClearedEveryFrame(false)
	ebiten.SetTPS(ebiten.SyncWithFPS)
	ebiten.SetVsyncEnabled(false)
	ebiten.SetRunnableOnUnfocused(true)
	exit := make(chan int, 1)
	go func() { exit <- m.Run() }()
	g := &renderBenchmarkLoop{exit: exit, code: 1}
	if err := ebiten.RunGameWithOptions(g, &ebiten.RunGameOptions{InitUnfocused: true}); err != nil {
		panic(err)
	}
	os.Exit(g.code)
}

// BenchmarkEditorRender measures actual editor drawing plus a GPU readback
// fence. It is a host-renderer comparison, not a cartridge cycle benchmark.
func BenchmarkEditorRender(b *testing.B) {
	for _, test := range []struct {
		name string
		mode editorMode
	}{{"Sprites", modeSprites}, {"Map", modeMap}, {"Code", modeCode}} {
		b.Run(test.name, func(b *testing.B) {
			dir := b.TempDir()
			if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "examples", "background-layers"))); err != nil {
				b.Fatal(err)
			}
			g, err := newGame(dir)
			if err != nil {
				b.Fatal(err)
			}
			defer g.closeNativePerformance()
			g.preferences = defaultEditorPreferences()
			g.applyPreferences()
			g.Layout(1200, 800)
			g.mode = test.mode
			if test.mode == modeMap {
				g.prepareMapView()
			}
			target := ebiten.NewImage(1200, 800)
			defer target.Deallocate()
			pixels := make([]byte, 1200*800*4)
			var renderTime time.Duration
			draw := func() {
				start := time.Now()
				g.requestRender()
				g.Draw(target)
				target.ReadPixels(pixels)
				renderTime += time.Since(start)
			}
			for range 20 {
				onRenderBenchmarkFrame(draw)
			}
			b.ReportAllocs()
			renderTime = 0
			b.ResetTimer()
			for range b.N {
				onRenderBenchmarkFrame(draw)
			}
			b.StopTimer()
			b.ReportMetric(float64(renderTime.Nanoseconds())/float64(b.N), "render-ns/op")
			if out := os.Getenv("MTILE_RENDER_CAPTURE"); out != "" {
				if err := os.MkdirAll(out, 0o755); err != nil {
					b.Fatal(err)
				}
				f, err := os.Create(filepath.Join(out, test.name+".png"))
				if err != nil {
					b.Fatal(err)
				}
				err = png.Encode(f, &image.RGBA{Pix: pixels, Stride: 1200 * 4, Rect: image.Rect(0, 0, 1200, 800)})
				closeErr := f.Close()
				if err != nil {
					b.Fatal(err)
				}
				if closeErr != nil {
					b.Fatal(closeErr)
				}
			}
		})
	}
}
