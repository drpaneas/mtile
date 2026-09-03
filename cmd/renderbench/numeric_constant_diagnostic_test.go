//go:build numericconstantdiagnostic

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestNumericConstantDiagnostic(t *testing.T) {
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
			baselineService.DisableNumericConstants = true
			baselineResult, baseline, err := buildAndCapture(baselineService, renderer, project, helper, t.TempDir(), 20, 120)
			if err != nil {
				t.Fatal(err)
			}
			optimizedResult, optimized, err := buildAndCapture(service, renderer, project, helper, t.TempDir(), 20, 120)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(baselineResult.GeneratedGo, "TILE_SIZE rt.Number") {
				t.Fatal("rollback build did not retain numeric constant storage")
			}
			if strings.Contains(optimizedResult.GeneratedGo, "TILE_SIZE rt.Number") {
				t.Fatal("optimized build retained numeric constant storage")
			}
			baselineMedian := percentile(baseline.cycles, 50)
			optimizedMedian := percentile(optimized.cycles, 50)
			differing := compareFrames(baseline.frames, optimized.frames)
			t.Logf("rollback median=%d draw=%d commit=%d rom=%d ewram_free=%d missed=%d",
				baselineMedian, percentile(baseline.draw, 50), percentile(baseline.commit, 50),
				baselineResult.ROMSize, baselineResult.EWRAMFree, baseline.missed)
			t.Logf("constants median=%d draw=%d commit=%d rom=%d ewram_free=%d missed=%d pixels=%d",
				optimizedMedian, percentile(optimized.draw, 50), percentile(optimized.commit, 50),
				optimizedResult.ROMSize, optimizedResult.EWRAMFree, optimized.missed, differing)
			if differing != 0 {
				t.Fatalf("rollback and constant-propagated ROMs differ by %d pixels", differing)
			}
			if optimizedMedian >= baselineMedian {
				t.Fatalf("constant propagation median %d did not improve over rollback %d", optimizedMedian, baselineMedian)
			}
			if optimizedResult.EWRAMFree < baselineResult.EWRAMFree {
				t.Fatalf("constant propagation reduced free EWRAM from %d to %d", baselineResult.EWRAMFree, optimizedResult.EWRAMFree)
			}
		})
	}
}
