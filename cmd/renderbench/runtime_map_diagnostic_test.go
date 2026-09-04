//go:build runtimeperfdiagnostic

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// TestRuntimeMapClippingDiagnostic isolates map walking against an exported
// baseline runtime, with the same transpiler and synchronized runtime frames.
//
//	MTILE_PERF_BASELINE_RUNTIME=/path/to/baseline/runtime go test ./cmd/renderbench -tags=runtimeperfdiagnostic -run TestRuntimeMapClippingDiagnostic -count=1 -v
func TestRuntimeMapClippingDiagnostic(t *testing.T) {
	baselineRuntime := os.Getenv("MTILE_PERF_BASELINE_RUNTIME")
	if baselineRuntime == "" {
		t.Skip("set MTILE_PERF_BASELINE_RUNTIME to the baseline runtime directory")
	}
	baselineService, err := build.NewService("", baselineRuntime)
	if err != nil {
		t.Fatal(err)
	}
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	baselineService.Debug, service.Debug = true, true
	project, err := stageProjectWithSource(
		filepath.Join("..", "..", "examples", "mapdemo"),
		filepath.Join("testdata", "workloads", "map_clipped_extent.lua"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(project)
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")
	baselineResult, baseline, err := buildAndCapture(baselineService, transpiler.RendererMode4, project, helper, t.TempDir(), 20, 120)
	if err != nil {
		t.Fatal(err)
	}
	optimizedResult, optimized, err := buildAndCaptureAtFrames(service, transpiler.RendererMode4, project, helper, t.TempDir(), 20, 120, baseline.frameNumbers)
	if err != nil {
		t.Fatal(err)
	}
	if baselineResult.GeneratedGo != optimizedResult.GeneratedGo {
		t.Fatal("runtime comparison changed the generated Go workload")
	}
	if err := validateSynchronizedFrames(baseline.frameNumbers, optimized.frameNumbers); err != nil {
		t.Fatal(err)
	}
	for _, result := range []struct {
		name    string
		build   build.Result
		capture capture
	}{{"baseline", baselineResult, baseline}, {"optimized", optimizedResult, optimized}} {
		c := result.capture
		t.Logf("%s median=%d p95=%d draw=%d commit=%d rom=%d ewram_free=%d missed=%d",
			result.name, percentile(c.cycles, 50), percentile(c.cycles, 95), percentile(c.draw, 50), percentile(c.commit, 50),
			result.build.ROMSize, result.build.EWRAMFree, c.missed)
	}
	differing := compareFrames(baseline.frames, optimized.frames)
	baselineMedian, optimizedMedian := percentile(baseline.cycles, 50), percentile(optimized.cycles, 50)
	t.Logf("saving=%.2f%% differing_pixels=%d", 100*(1-float64(optimizedMedian)/float64(baselineMedian)), differing)
	if differing != 0 {
		t.Fatalf("map clipping runtime builds differ by %d pixels", differing)
	}
	var changingFrames bool
	for _, frame := range optimized.frames[1:] {
		if countDifferingPixels(optimized.frames[0], frame) != 0 {
			changingFrames = true
			break
		}
	}
	if !changingFrames {
		t.Fatal("scrolling map did not change the captured pixels")
	}
	if optimizedMedian >= baselineMedian {
		t.Fatalf("map clipping median %d did not improve over %d", optimizedMedian, baselineMedian)
	}
}
