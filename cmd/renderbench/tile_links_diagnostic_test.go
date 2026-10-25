//go:build runtimeperfdiagnostic

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// Retain the native pixel comparisons and cycle samples for the repaired map
// shapes with MTILE_TILE_LINKS_OUT=<artifact directory>.
func TestTileLinkTargetDiagnostic(t *testing.T) {
	out := os.Getenv("MTILE_TILE_LINKS_OUT")
	if out == "" {
		t.Skip("set MTILE_TILE_LINKS_OUT for native tile-link evidence")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(repository, "build", "player", "mtile-player")
	service, err := build.NewService("", filepath.Join(repository, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	for _, name := range []string{"default-extent", "explicit-extent"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(out, name)
			project := filepath.Join(dir, "project")
			writeTileLinkRegressionFixture(t, project, name == "explicit-extent")
			environment, err := benchmarkEnvironment(service, project, helper)
			if err != nil {
				t.Fatal(err)
			}
			const warmup, samples = 20, 120
			b4, c4, err := buildAndCapture(service, transpiler.RendererMode4, project, helper, dir, warmup, samples)
			if err != nil {
				t.Fatal(err)
			}
			b0, c0, err := buildAndCaptureAtFrames(service, transpiler.RendererMode0, project, helper, dir, warmup, samples, c4.frameNumbers)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateSynchronizedFrames(c4.frameNumbers, c0.frameNumbers); err != nil {
				t.Fatal(err)
			}
			different := compareFrames(c4.frames, c0.frames)
			p4, p0 := filepath.Join(dir, "mode4-final.png"), filepath.Join(dir, "mode0-final.png")
			if err := writePNG(p4, c4.lastRGB); err != nil {
				t.Fatal(err)
			}
			if err := writePNG(p0, c0.lastRGB); err != nil {
				t.Fatal(err)
			}
			r4, err := summarize(b4, c4, p4)
			if err != nil {
				t.Fatal(err)
			}
			r0, err := summarize(b0, c0, p0)
			if err != nil {
				t.Fatal(err)
			}
			writePSprJSON(t, filepath.Join(dir, "report.json"), map[string]any{"environment": environment, "warmupFrames": warmup, "capturedFrames": samples, "differingPixels": different, "mode0": r0, "mode4": r4})
			if different != 0 || r0.MissedFrames != 0 || r4.MissedFrames != 0 {
				t.Fatalf("%d differing pixels, missed frames Mode 0 %d / Mode 4 %d", different, r0.MissedFrames, r4.MissedFrames)
			}
			t.Logf("%d paired frames, %d differing pixels; median/p95 cycles Mode 0 %d/%d, Mode 4 %d/%d", samples, different, r0.MedianCycles, r0.P95Cycles, r4.MedianCycles, r4.P95Cycles)
		})
	}
}

func writeTileLinkRegressionFixture(t *testing.T, project string, explicit bool) {
	t.Helper()
	if err := build.CreateProject(project, build.NewProjectOptions{SheetWidth: 40, SheetHeight: 8, MapWidth: 64, MapHeight: 40}); err != nil {
		t.Fatal(err)
	}
	sheet := assetsync.Spritesheet{Width: 40, Height: 8}
	for id := 1; id <= 4; id++ {
		cell := assetsync.SpriteRecord{ID: id, X: id * 8, Width: 8, Height: 8, Flags: id}
		for y := 0; y < 8; y++ {
			row := make([]int, 8)
			for x := range row {
				row[x] = 8 + id
			}
			cell.Pixels = append(cell.Pixels, row)
		}
		sheet.Sprites = append(sheet.Sprites, cell)
	}
	if err := assetsync.SaveSpritesheet(project, &sheet); err != nil {
		t.Fatal(err)
	}
	poses := assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{
		{Name: "edge", Parts: []assetsync.PosePartRecord{{Tile: 3, X: -8, Y: -8}}},
		{Name: "blink", Parts: []assetsync.PosePartRecord{{Tile: 4, X: -8, Y: -8}}},
	}}
	if err := assetsync.SavePoses(project, &poses); err != nil {
		t.Fatal(err)
	}
	loop := true
	if err := assetsync.SaveAnimationWorkspace(project, &assetsync.AnimationWorkspace{Version: 1, Clips: []assetsync.AnimationClip{{Name: "blink", Frames: []int{0, 1}, FPS: 8, Loop: &loop}}}); err != nil {
		t.Fatal(err)
	}
	pose := 0
	if err := assetsync.SaveTileLinks(project, &assetsync.TileLinkFile{Version: 1, Links: []assetsync.TileLink{{Tile: 1, Pose: &pose}, {Tile: 2, Animation: "blink"}}}); err != nil {
		t.Fatal(err)
	}
	m := assetsync.Map{Width: 64, Height: 40, Cells: make([]int, 64*40)}
	for _, cell := range [][3]int{{30, 2, 1}, {2, 20, 2}, {30, 20, 1}, {10, 5, 2}, {5, 5, 1}} {
		m.Cells[cell[1]*m.Width+cell[0]] = cell[2]
	}
	if err := assetsync.SaveMap(project, &m); err != nil {
		t.Fatal(err)
	}
	size := "0,0"
	if explicit {
		size = "30,20"
	}
	source := "tick=0\nmask=0\nfunction _update()\n tick+=1\n mask=flr(tick/5)%4\n mset(5,5,0)\nend\nfunction _draw()\n cls(0)\n camera(tick%2,tick%2)\n map(0,0,0,0,0," + size + ",mask)\nend\n"
	if err := os.WriteFile(filepath.Join(project, "game.lua"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
}
