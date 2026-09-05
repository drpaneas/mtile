//go:build runtimeperfdiagnostic

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// TestRuntimePongDiagnostic compares runtime changes with an unchanged Pong
// cartridge, including its music and sound effects. Captures use identical
// runtime ticks, so faster drawing cannot hide a changed game trajectory.
//
// MTILE_PERF_BASELINE_RUNTIME=/path/to/baseline/runtime go test ./cmd/renderbench -tags=runtimeperfdiagnostic -run '^TestRuntimePongDiagnostic$' -count=1 -v
func TestRuntimePongDiagnostic(t *testing.T) {
	baselineDir := os.Getenv("MTILE_PERF_BASELINE_RUNTIME")
	if baselineDir == "" {
		t.Skip("set MTILE_PERF_BASELINE_RUNTIME to the runtime before the Pong optimization")
	}
	samples := 600
	if value := os.Getenv("MTILE_PERF_SAMPLES"); value != "" {
		var err error
		samples, err = strconv.Atoi(value)
		if err != nil || samples < 1 {
			t.Fatal("MTILE_PERF_SAMPLES must be a positive integer")
		}
	}
	baselineService, err := build.NewService("", baselineDir)
	if err != nil {
		t.Fatal(err)
	}
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	baselineService.Debug, service.Debug = true, true
	project := filepath.Join("..", "..", "examples", "pong")
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")
	beforeBuild, before, err := buildAndCapture(baselineService, transpiler.RendererMode4, project, helper, t.TempDir(), 20, samples)
	if err != nil {
		t.Fatal(err)
	}
	afterBuild, after, err := buildAndCaptureAtFrames(service, transpiler.RendererMode4, project, helper, t.TempDir(), 20, samples, before.frameNumbers)
	if err != nil {
		t.Fatal(err)
	}
	if beforeBuild.GeneratedGo != afterBuild.GeneratedGo {
		t.Fatal("runtime comparison changed generated Pong code")
	}
	if err := validateSynchronizedFrames(before.frameNumbers, after.frameNumbers); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []struct {
		name    string
		result  build.Result
		capture capture
	}{{"before", beforeBuild, before}, {"after", afterBuild, after}} {
		c := variant.capture
		t.Logf("%s samples=%d median=%d p95=%d p99=%d max=%d draw=%d commit=%d rom=%d iwram=%d ewram=%d heap_allocated=%d heap_live=%d handles=%d gc=%d warmup_missed=%d final_missed=%d",
			variant.name, len(c.cycles), percentile(c.cycles, 50), percentile(c.cycles, 95), percentile(c.cycles, 99), percentile(c.cycles, 100), percentile(c.draw, 50), percentile(c.commit, 50),
			variant.result.ROMSize, variant.result.IWRAMUsed, variant.result.EWRAMUsed, c.heapAllocated, c.heapLive, c.heapHandles, c.gcCount, c.warmupMissed, c.missed)
		if c.warmupFrame != 20 || c.missed != c.warmupMissed {
			t.Errorf("%s warm-up frame=%d missed=%d, final missed=%d", variant.name, c.warmupFrame, c.warmupMissed, c.missed)
		}
		if percentile(c.cycles, 100) >= twoFrameCycleBudget {
			t.Errorf("%s exceeds the %d-cycle tick budget", variant.name, twoFrameCycleBudget)
		}
	}
	different := compareFrames(before.frames, after.frames)
	t.Logf("saving=%.2f%% draw_saving=%.2f%% differing_pixels=%d rom_delta=%d iwram_delta=%d ewram_delta=%d",
		100*(1-float64(percentile(after.cycles, 50))/float64(percentile(before.cycles, 50))),
		100*(1-float64(percentile(after.draw, 50))/float64(percentile(before.draw, 50))), different,
		afterBuild.ROMSize-beforeBuild.ROMSize, int64(afterBuild.IWRAMUsed)-int64(beforeBuild.IWRAMUsed), int64(afterBuild.EWRAMUsed)-int64(beforeBuild.EWRAMUsed))
	if different != 0 || !reflect.DeepEqual(before.watches, after.watches) {
		t.Error("optimized Pong differs in pixels or watched game values")
	}
	if percentile(after.cycles, 50) >= percentile(before.cycles, 50) || percentile(after.cycles, 95) > percentile(before.cycles, 95) {
		t.Error("Pong must improve median cycles without regressing p95")
	}
	if afterBuild.EWRAMUsed != beforeBuild.EWRAMUsed || after.heapAllocated != before.heapAllocated || after.gcCount != before.gcCount {
		t.Error("drawing optimization changed EWRAM or Lua heap activity")
	}
}
