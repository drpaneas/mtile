//go:build runtimeperfdiagnostic

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// TestRuntimeNumericHashDiagnostic compares an unchanged Lua workload against
// a separately checked-out runtime. For example, export the baseline revision's
// runtime directory with git archive and run:
//
//	MTILE_PERF_BASELINE_RUNTIME=/path/to/baseline/runtime go test ./cmd/renderbench -tags=runtimeperfdiagnostic -run TestRuntimeNumericHashDiagnostic -count=1 -v
//
// Both builds use the current transpiler, identical assets, and synchronized
// runtime frames, so this isolates the runtime implementation being measured.
func TestRuntimeNumericHashDiagnostic(t *testing.T) {
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
		filepath.Join("..", "..", "examples", "hello"),
		filepath.Join("testdata", "workloads", "numeric_hash_keys.lua"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(project)
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")

	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			baselineResult, baseline, err := buildAndCapture(baselineService, renderer, project, helper, t.TempDir(), 20, 120)
			if err != nil {
				t.Fatal(err)
			}
			optimizedResult, optimized, err := buildAndCaptureAtFrames(service, renderer, project, helper, t.TempDir(), 20, 120, baseline.frameNumbers)
			if err != nil {
				t.Fatal(err)
			}
			if baselineResult.GeneratedGo != optimizedResult.GeneratedGo {
				t.Fatal("runtime comparison changed the generated Go workload")
			}
			generated := optimizedResult.GeneratedGo
			if !strings.Contains(generated, "c.TableGet(") || !strings.Contains(generated, "c.TableSet(") ||
				strings.Contains(generated, "NumberArrayGet") || strings.Contains(generated, "TableGetStable") {
				t.Fatal("workload must retain dynamic hash-table reads and writes")
			}
			if err := validateSynchronizedFrames(baseline.frameNumbers, optimized.frameNumbers); err != nil {
				t.Fatal(err)
			}
			baselineMedian := percentile(baseline.cycles, 50)
			optimizedMedian := percentile(optimized.cycles, 50)
			differing := compareFrames(baseline.frames, optimized.frames)
			for _, result := range []struct {
				name    string
				build   build.Result
				capture capture
			}{{"baseline", baselineResult, baseline}, {"optimized", optimizedResult, optimized}} {
				c := result.capture
				t.Logf("%s median=%d p95=%d draw=%d commit=%d rom=%d ewram_free=%d heap_allocated=%d heap_live=%d heap_peak=%d heap_handles=%d gc=%d missed=%d",
					result.name, percentile(c.cycles, 50), percentile(c.cycles, 95), percentile(c.draw, 50), percentile(c.commit, 50),
					result.build.ROMSize, result.build.EWRAMFree, c.heapAllocated, c.heapLive, c.heapPeakLive, c.heapHandles, c.gcCount, c.missed)
			}
			t.Logf("saving=%.2f%% differing_pixels=%d", 100*(1-float64(optimizedMedian)/float64(baselineMedian)), differing)
			if differing != 0 {
				t.Fatalf("numeric hash runtime builds differ by %d pixels", differing)
			}
			var changingFrames bool
			for _, frame := range optimized.frames[1:] {
				if countDifferingPixels(optimized.frames[0], frame) != 0 {
					changingFrames = true
					break
				}
			}
			if !changingFrames {
				t.Fatal("checksum did not affect the captured pixels")
			}
			if optimizedMedian >= baselineMedian {
				t.Fatalf("numeric hash median %d did not improve over %d", optimizedMedian, baselineMedian)
			}
		})
	}
}
