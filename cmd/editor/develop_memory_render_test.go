//go:build mtile_editor_bench

package main

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/hajimehoshi/ebiten/v2"
)

// Use real native telemetry and a disposable cart copy; never touch the open
// editor or save changes to its assets during visual verification.
func TestDevelopMemoryRender(t *testing.T) {
	out, evidence := os.Getenv("MTILE_MEMORY_UI_REPORT"), os.Getenv("MTILE_MEMORY_UI_DATA")
	if out == "" || evidence == "" {
		t.Skip("set MTILE_MEMORY_UI_REPORT and MTILE_MEMORY_UI_DATA for visual verification")
	}
	var report struct {
		Debug struct {
			ROMBytes                        int
			EWRAMUsed, EWRAMFree, IWRAMUsed uint64
			PoseMemory                      build.PoseMemoryReport
		}
		RepresentativeFrames []struct {
			Stats emulator.DebugStats
			Image string
		}
	}
	data, err := os.ReadFile(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.RepresentativeFrames) == 0 {
		t.Fatal("native telemetry frames missing")
	}
	frame := report.RepresentativeFrames[0]
	imageFile, err := os.Open(filepath.Join(filepath.Dir(evidence), frame.Image))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := png.Decode(imageFile)
	_ = imageFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { setInterfaceTextSize(textComfort); setEditorLayout(defaultScreenWidth, defaultScreenHeight) })
	for _, size := range []struct {
		name string
		w, h int
		font interfaceTextSize
	}{
		{"desktop", 1920, 1200, textComfort}, {"compact-large-text", 1024, 900, textLarge},
	} {
		t.Run(size.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "examples", "nerdo"))); err != nil {
				t.Fatal(err)
			}
			g, err := newGame(dir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(g.closeNativePerformance)
			g.preferences = defaultEditorPreferences()
			g.applyPreferences()
			setInterfaceTextSize(size.font)
			g.Layout(size.w, size.h)
			g.mode, g.developPanel = modeCode, developMemory
			g.runID, g.playRunID, g.runReportID = 1, 1, 1
			g.runMemoryReady = true
			g.runRendererSelected, g.runRendererRequested = "mode0", "auto"
			g.runROMSize = report.Debug.ROMBytes
			g.runEWRAMUsed, g.runEWRAMFree, g.runIWRAMUsed = report.Debug.EWRAMUsed, report.Debug.EWRAMFree, report.Debug.IWRAMUsed
			g.runPoseMemory = report.Debug.PoseMemory
			g.runStatus = "RUNNING ON mGBA CORE"
			g.playTelemetry = emulator.Telemetry{Valid: true, DebugValid: true, Stats: frame.Stats}
			g.playImage = ebiten.NewImageFromImage(preview)
			defer g.playImage.Deallocate()
			capture := func(state string) {
				t.Helper()
				target := ebiten.NewImage(size.w, size.h)
				defer target.Deallocate()
				pixels := make([]byte, size.w*size.h*4)
				onRenderBenchmarkFrame(func() { g.requestRender(); g.Draw(target); target.ReadPixels(pixels) })
				file, err := os.Create(filepath.Join(out, size.name+"-"+state+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, &image.NRGBA{Pix: pixels, Stride: size.w * 4, Rect: image.Rect(0, 0, size.w, size.h)})
				_ = file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			capture("live")
			if size.w == minimumScreenWidth {
				g.scrollDevelopMemory(newDevelopPanelLayout(developMemory), 8)
				capture("live-middle")
			}
			g.scrollDevelopMemory(newDevelopPanelLayout(developMemory), 10000)
			capture("details")
			g.developScroll[developMemory] = 0
			g.playTelemetry = emulator.Telemetry{}
			capture("waiting")
			if size.w == minimumScreenWidth {
				g.scrollDevelopMemory(newDevelopPanelLayout(developMemory), 8)
				capture("waiting-middle")
				g.developScroll[developMemory] = 0
			}
			g.runRendererSelected = "mode4"
			g.runPoseMemory.StageLinked, g.runPoseMemory.StageRAMBytes = false, 0
			capture("bitmap")
		})
	}
}
