//go:build tableeffectdiagnostic

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestTableEffectDiagnostic(t *testing.T) {
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	project, err := stageProjectWithSource(
		filepath.Join("..", "..", "examples", "hello"),
		filepath.Join("testdata", "workloads", "table_noalloc_calls.lua"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(project)
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")

	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			baselineService := service
			baselineService.DisableNoAllocFunctionCalls = true
			baselineResult, baseline, err := buildAndCapture(baselineService, renderer, project, helper, t.TempDir(), 20, 120)
			if err != nil {
				t.Fatal(err)
			}
			optimizedResult, optimized, err := buildAndCapture(service, renderer, project, helper, t.TempDir(), 20, 120)
			if err != nil {
				t.Fatal(err)
			}
			baselineMedian := percentile(baseline.cycles, 50)
			optimizedMedian := percentile(optimized.cycles, 50)
			differing := compareFrames(baseline.frames, optimized.frames)
			t.Logf("root-wrapped median=%d draw=%d commit=%d rom=%d ewram_free=%d missed=%d",
				baselineMedian, percentile(baseline.draw, 50), percentile(baseline.commit, 50),
				baselineResult.ROMSize, baselineResult.EWRAMFree, baseline.missed)
			t.Logf("direct-table-read-delete median=%d draw=%d commit=%d rom=%d ewram_free=%d missed=%d pixels=%d",
				optimizedMedian, percentile(optimized.draw, 50), percentile(optimized.commit, 50),
				optimizedResult.ROMSize, optimizedResult.EWRAMFree, optimized.missed, differing)
			if differing != 0 {
				t.Fatalf("root-wrapped and direct table-effect ROMs differ by %d pixels", differing)
			}
			if optimizedMedian >= baselineMedian {
				t.Fatalf("table effect specialization median %d did not improve over %d", optimizedMedian, baselineMedian)
			}
		})
	}
}
