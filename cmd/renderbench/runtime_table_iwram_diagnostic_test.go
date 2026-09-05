//go:build runtimeperfdiagnostic

package main

import (
	"debug/elf"
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
	rt "github.com/drpaneas/mtile/runtime"
)

// TestRuntimeTableIWRAMDiagnostic isolates table helper placement using the
// unchanged numeric-hash workload and an unmodified baseline runtime snapshot.
//
//	MTILE_PERF_BASELINE_RUNTIME=/path/to/pre-iwram/runtime go test ./cmd/renderbench -tags=runtimeperfdiagnostic -run TestRuntimeTableIWRAMDiagnostic -count=1 -v
func TestRuntimeTableIWRAMDiagnostic(t *testing.T) {
	baselineRuntime := os.Getenv("MTILE_PERF_BASELINE_RUNTIME")
	if baselineRuntime == "" {
		t.Skip("set MTILE_PERF_BASELINE_RUNTIME to the runtime before IWRAM placement")
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
	baselineService.KeepWork, service.KeepWork = true, true
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
			baselineResult, baseline, err := buildAndCapture(baselineService, renderer, project, helper, t.TempDir(), 20, 600)
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(baselineResult.WorkDir)
			optimizedResult, optimized, err := buildAndCaptureAtFrames(service, renderer, project, helper, t.TempDir(), 20, 600, baseline.frameNumbers)
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(optimizedResult.WorkDir)
			if baselineResult.GeneratedGo != optimizedResult.GeneratedGo {
				t.Fatal("table placement comparison changed the generated Go workload")
			}
			if err := validateSynchronizedFrames(baseline.frameNumbers, optimized.frameNumbers); err != nil {
				t.Fatal(err)
			}
			for _, variant := range []struct {
				name     string
				result   build.Result
				captured capture
				iwram    bool
			}{{"rom", baselineResult, baseline, false}, {"iwram", optimizedResult, optimized, true}} {
				c := variant.captured
				t.Logf("%s median=%d p95=%d max=%d draw=%d commit=%d rom=%d iwram=%d ewram_free=%d heap_allocated=%d heap_handles=%d gc=%d warmup_frame=%d warmup_missed=%d final_missed=%d",
					variant.name, percentile(c.cycles, 50), percentile(c.cycles, 95), percentile(c.cycles, 100), percentile(c.draw, 50), percentile(c.commit, 50),
					variant.result.ROMSize, variant.result.IWRAMUsed, variant.result.EWRAMFree, c.heapAllocated, c.heapHandles, c.gcCount, c.warmupFrame, c.warmupMissed, c.missed)
				checkTableHelperPlacement(t, variant.name, variant.result.WorkDir, variant.iwram)
				checkIWRAMHashCapture(t, variant.name, c)
			}
			baselineMedian := percentile(baseline.cycles, 50)
			optimizedMedian := percentile(optimized.cycles, 50)
			differing := compareFrames(baseline.frames, optimized.frames)
			t.Logf("saving=%.2f%% rom_delta=%d iwram_delta=%d ewram_delta=%d differing_pixels=%d checksum_frames=%d",
				100*(1-float64(optimizedMedian)/float64(baselineMedian)), optimizedResult.ROMSize-baselineResult.ROMSize,
				int64(optimizedResult.IWRAMUsed)-int64(baselineResult.IWRAMUsed), int64(optimizedResult.EWRAMUsed)-int64(baselineResult.EWRAMUsed), differing, len(optimized.frameNumbers))
			if differing != 0 {
				t.Fatalf("ROM and IWRAM helper placement differ by %d pixels", differing)
			}
			if optimizedMedian >= baselineMedian || percentile(optimized.cycles, 95) > percentile(baseline.cycles, 95) {
				t.Error("IWRAM placement must improve median without regressing p95")
			}
			if optimizedResult.IWRAMUsed <= baselineResult.IWRAMUsed {
				t.Error("linked IWRAM usage did not account for moved table helpers")
			}
			if optimizedResult.EWRAMUsed != baselineResult.EWRAMUsed || optimized.heapAllocated != baseline.heapAllocated || optimized.heapHandles != baseline.heapHandles {
				t.Error("helper placement changed EWRAM or Lua heap storage")
			}
		})
	}
}

func checkTableHelperPlacement(t *testing.T, label, workDir string, iwram bool) {
	t.Helper()
	file, err := elf.Open(filepath.Join(workDir, "game.elf"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	symbols, err := file.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"hashFind", "tableData"} {
		found := false
		for _, symbol := range symbols {
			// Linker branch veneers also end in the method name, but remain
			// in ROM. Verify the actual Go function body, not its veneer.
			if symbol.Name != "(*github.com/drpaneas/mtile/runtime.Context)."+method {
				continue
			}
			found = true
			t.Logf("%s method=%s address=%#08x size=%d", label, method, symbol.Value, symbol.Size)
			start, end := uint64(0x08000000), uint64(0x0a000000)
			if iwram {
				start, end = 0x03000000, 0x03008000
			}
			if symbol.Size == 0 || symbol.Value < start || symbol.Value >= end || symbol.Size > end-symbol.Value {
				t.Errorf("%s %s address=%#x size=%d is outside [%#x,%#x)", label, method, symbol.Value, symbol.Size, start, end)
			}
		}
		if !found {
			t.Errorf("%s ELF has no %s function symbol", label, method)
		}
	}
}

func checkIWRAMHashCapture(t *testing.T, label string, c capture) {
	t.Helper()
	if c.warmupFrame != 20 || c.missed != c.warmupMissed {
		t.Errorf("%s warm-up frame=%d missed=%d, final missed=%d; want frame20 and no steady-state misses", label, c.warmupFrame, c.warmupMissed, c.missed)
	}
	if maxCycles := percentile(c.cycles, 100); maxCycles >= twoFrameCycleBudget {
		t.Errorf("%s worst tick %d must be below %d", label, maxCycles, twoFrameCycleBudget)
	}
	for i, frameNumber := range c.frameNumbers {
		// Preserve the workload's signed 16:16 wrap at tick 480.
		want := rt.FromInt(2080 + 64*int(frameNumber))
		found := false
		for _, watch := range c.watches[i] {
			if watch.Name == "checksum" {
				found = true
				if watch.Kind != byte(rt.KindNumber) || watch.Raw != int32(want) {
					t.Fatalf("%s frame %d checksum kind=%d raw=%d, want number raw=%d", label, frameNumber, watch.Kind, watch.Raw, want)
				}
			}
		}
		if !found {
			t.Fatalf("%s frame %d has no checksum watch", label, frameNumber)
		}
	}
	for _, frame := range c.frames[1:] {
		if countDifferingPixels(c.frames[0], frame) != 0 {
			return
		}
	}
	t.Errorf("%s checksum did not change the captured pixels", label)
}
