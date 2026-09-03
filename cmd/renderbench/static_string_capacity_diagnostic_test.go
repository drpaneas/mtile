//go:build staticstringdiagnostic

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestStaticStringCapacityDiagnostic(t *testing.T) {
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
			baselineService.DisableStaticStringSpecialization = true
			baselineResult, baseline, err := buildAndCapture(baselineService, renderer, project, helper, t.TempDir(), 20, 120)
			if err != nil {
				t.Fatal(err)
			}
			optimizedResult, optimized, err := buildAndCapture(service, renderer, project, helper, t.TempDir(), 20, 120)
			if err != nil {
				t.Fatal(err)
			}
			if baselineResult.GeneratedGo != optimizedResult.GeneratedGo {
				t.Fatal("capacity-only specialization unexpectedly changed generated Go")
			}
			baselineMedian := percentile(baseline.cycles, 50)
			optimizedMedian := percentile(optimized.cycles, 50)
			differing := compareFrames(baseline.frames, optimized.frames)
			t.Logf("capacity-256 median=%d draw=%d commit=%d rom=%d ewram_free=%d missed=%d",
				baselineMedian, percentile(baseline.draw, 50), percentile(baseline.commit, 50),
				baselineResult.ROMSize, baselineResult.EWRAMFree, baseline.missed)
			t.Logf("capacity-tier median=%d draw=%d commit=%d rom=%d ewram_free=%d missed=%d pixels=%d",
				optimizedMedian, percentile(optimized.draw, 50), percentile(optimized.commit, 50),
				optimizedResult.ROMSize, optimizedResult.EWRAMFree, optimized.missed, differing)
			if differing != 0 {
				t.Fatalf("full and tiered capacities differ by %d pixels", differing)
			}
			if optimizedMedian > baselineMedian {
				t.Fatalf("static string tier regressed median from %d to %d", baselineMedian, optimizedMedian)
			}
			if optimizedResult.EWRAMFree <= baselineResult.EWRAMFree {
				t.Fatalf("static string tier did not return EWRAM: %d -> %d", baselineResult.EWRAMFree, optimizedResult.EWRAMFree)
			}
		})
	}
}
