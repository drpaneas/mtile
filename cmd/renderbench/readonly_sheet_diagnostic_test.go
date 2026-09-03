//go:build readonlysheetdiagnostic

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestReadOnlySheetDiagnostic(t *testing.T) {
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	project, err := stageProjectWithSource(
		filepath.Join("..", "..", "examples", "celeste"),
		filepath.Join("testdata", "workloads", "celeste_constant_collision.lua"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(project)
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")

	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			baselineService := service
			baselineService.DisableReadOnlySheetSpecialization = true
			baselineResult, baseline, err := buildAndCapture(baselineService, renderer, project, helper, t.TempDir(), 20, 120)
			if err != nil {
				t.Fatal(err)
			}
			optimizedResult, optimized, err := buildAndCapture(service, renderer, project, helper, t.TempDir(), 20, 120)
			if err != nil {
				t.Fatal(err)
			}
			if baselineResult.GeneratedGo != optimizedResult.GeneratedGo {
				t.Fatal("runtime-only sheet specialization unexpectedly changed generated Go")
			}
			baselineMedian := percentile(baseline.cycles, 50)
			optimizedMedian := percentile(optimized.cycles, 50)
			differing := compareFrames(baseline.frames, optimized.frames)
			t.Logf("generic-sheet-runtime median=%d draw=%d commit=%d cold=%d rom=%d ewram_free=%d missed=%d",
				baselineMedian, percentile(baseline.draw, 50), percentile(baseline.commit, 50), baseline.cold,
				baselineResult.ROMSize, baselineResult.EWRAMFree, baseline.missed)
			t.Logf("immutable-sheet-proof median=%d draw=%d commit=%d cold=%d rom=%d ewram_free=%d missed=%d pixels=%d",
				optimizedMedian, percentile(optimized.draw, 50), percentile(optimized.commit, 50), optimized.cold,
				optimizedResult.ROMSize, optimizedResult.EWRAMFree, optimized.missed, differing)
			if differing != 0 {
				t.Fatalf("generic and read-only-specialized ROMs differ by %d pixels", differing)
			}
			if optimizedMedian >= baselineMedian {
				t.Fatalf("read-only specialization did not improve median from %d (got %d)", baselineMedian, optimizedMedian)
			}
			if optimizedResult.ROMSize >= baselineResult.ROMSize {
				t.Fatalf("read-only specialization did not reduce ROM from %d (got %d)", baselineResult.ROMSize, optimizedResult.ROMSize)
			}
		})
	}
}
