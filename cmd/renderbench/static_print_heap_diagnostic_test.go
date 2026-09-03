//go:build staticprintheapdiagnostic

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestStaticPrintHeapDiagnostic(t *testing.T) {
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	project, err := stageProjectWithSource(
		filepath.Join("..", "..", "examples", "hello"),
		filepath.Join("testdata", "workloads", "static_print_heapless.lua"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(project)
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")

	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			baselineService := service
			baselineService.DisableLiteralPrintSpecialization = true
			baselineResult, baseline, err := buildAndCapture(baselineService, renderer, project, helper, t.TempDir(), 20, 120)
			if err != nil {
				t.Fatal(err)
			}
			optimizedResult, optimized, err := buildAndCapture(service, renderer, project, helper, t.TempDir(), 20, 120)
			if err != nil {
				t.Fatal(err)
			}
			if !baselineResult.HeapLinked {
				t.Fatal("rollback build did not retain the Lua heap")
			}
			if optimizedResult.HeapLinked {
				t.Fatal("literal-print build retained the Lua heap")
			}
			baselineMedian := percentile(baseline.cycles, 50)
			optimizedMedian := percentile(optimized.cycles, 50)
			differing := compareFrames(baseline.frames, optimized.frames)
			t.Logf("heap-linked median=%d draw=%d commit=%d rom=%d ewram_free=%d missed=%d",
				baselineMedian, percentile(baseline.draw, 50), percentile(baseline.commit, 50),
				baselineResult.ROMSize, baselineResult.EWRAMFree, baseline.missed)
			t.Logf("heapless-static-print median=%d draw=%d commit=%d rom=%d ewram_free=%d missed=%d pixels=%d",
				optimizedMedian, percentile(optimized.draw, 50), percentile(optimized.commit, 50),
				optimizedResult.ROMSize, optimizedResult.EWRAMFree, optimized.missed, differing)
			if differing != 0 {
				t.Fatalf("heap-linked and heapless literal-print ROMs differ by %d pixels", differing)
			}
			if optimizedMedian > baselineMedian {
				t.Fatalf("literal-print heap elision regressed median from %d to %d", baselineMedian, optimizedMedian)
			}
			if optimizedResult.EWRAMFree <= baselineResult.EWRAMFree {
				t.Fatalf("literal-print heap elision did not return EWRAM: %d -> %d", baselineResult.EWRAMFree, optimizedResult.EWRAMFree)
			}
		})
	}
}
