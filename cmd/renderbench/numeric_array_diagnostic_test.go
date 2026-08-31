//go:build numericarraydiagnostic

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestNumericArrayDiagnostic(t *testing.T) {
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	project, err := stageProjectWithSource(
		filepath.Join("..", "..", "examples", "testio"),
		filepath.Join("testdata", "workloads", "numeric_array_particles.lua"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(project)
	help := filepath.Join("..", "..", "build", "player", "mtile-player")

	baselineService := service
	baselineService.DisableNumericArrays = true
	baselineResult, baseline, err := buildAndCapture(baselineService, transpiler.RendererMode4, project, help, t.TempDir(), 20, 600)
	if err != nil {
		t.Fatal(err)
	}
	optimizedResult, optimized, err := buildAndCapture(service, transpiler.RendererMode4, project, help, t.TempDir(), 20, 600)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(baselineResult.GeneratedGo, "c.NewTableWithCaps(64, 1)") || strings.Contains(baselineResult.GeneratedGo, "NewNumberArray") {
		t.Fatal("baseline did not retain generic 64-slot Lua tables")
	}
	if strings.Count(optimizedResult.GeneratedGo, "c.NewNumberArray(64)") != 4 || !strings.Contains(optimizedResult.GeneratedGo, "c.NumberArrayGet(") || !strings.Contains(optimizedResult.GeneratedGo, "c.NumberArraySet(") {
		t.Fatal("optimized build did not lower all four arrays to compact numeric storage")
	}

	logResult := func(label string, result build.Result, captured capture) {
		median := percentile(captured.cycles, 50)
		draw := percentile(captured.draw, 50)
		commit := percentile(captured.commit, 50)
		t.Logf("%s median=%d p95=%d p99=%d max=%d draw=%d commit=%d residual=%d missed=%d ewram_used=%d ewram_free=%d rom=%d heap_allocated=%d heap_live=%d heap_peak=%d heap_handles=%d gc=%d",
			label, median, percentile(captured.cycles, 95), percentile(captured.cycles, 99), percentile(captured.cycles, 100),
			draw, commit, median-draw-commit, captured.missed,
			result.EWRAMUsed, result.EWRAMFree, result.ROMSize,
			captured.heapAllocated, captured.heapLive, captured.heapPeakLive, captured.heapHandles, captured.gcCount)
	}
	change := func(before, after uint32) float64 {
		if before == 0 {
			return 0
		}
		return 100 * (float64(after)/float64(before) - 1)
	}
	logResult("generic-table", baselineResult, baseline)
	logResult("numeric-array", optimizedResult, optimized)
	differing := compareFrames(baseline.frames, optimized.frames)
	baselineResidual := percentile(baseline.cycles, 50) - percentile(baseline.draw, 50) - percentile(baseline.commit, 50)
	optimizedResidual := percentile(optimized.cycles, 50) - percentile(optimized.draw, 50) - percentile(optimized.commit, 50)
	t.Logf("change median=%+.2f%% residual=%+.2f%% rom=%+.2f%% linker_ewram=%+d heap=%+d (%+.2f%%) pixels=%d",
		change(percentile(baseline.cycles, 50), percentile(optimized.cycles, 50)),
		change(baselineResidual, optimizedResidual),
		change(uint32(baselineResult.ROMSize), uint32(optimizedResult.ROMSize)),
		int64(optimizedResult.EWRAMUsed)-int64(baselineResult.EWRAMUsed),
		int64(optimized.heapAllocated)-int64(baseline.heapAllocated),
		change(baseline.heapAllocated, optimized.heapAllocated), differing)
	if differing != 0 {
		t.Fatalf("generic and numeric arrays differ by %d pixels", differing)
	}
	if optimized.heapAllocated >= baseline.heapAllocated {
		t.Fatal("compact numeric arrays did not reduce Lua heap allocation")
	}
}
