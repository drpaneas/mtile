//go:build callsitecloningdiagnostic

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// TestCallSiteCloningDiagnostic measures the upper bound of numeric call-site
// cloning without changing the compiler. The split source manually duplicates
// one function into the generic and numeric variants that a compiler pass
// would emit. The two sources execute the same operations and render the same
// state; only their function signatures and call routing differ.
func TestCallSiteCloningDiagnostic(t *testing.T) {
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true

	baseProject := filepath.Join("..", "..", "examples", "testio")
	workloads := filepath.Join("testdata", "workloads")
	baselineProject, err := stageProjectWithSource(baseProject, filepath.Join(workloads, "callsite_clone_baseline.lua"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baselineProject)
	splitProject, err := stageProjectWithSource(baseProject, filepath.Join(workloads, "callsite_clone_split.lua"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(splitProject)
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")

	logResult := func(label string, result build.Result, captured capture) {
		median := percentile(captured.cycles, 50)
		draw := percentile(captured.draw, 50)
		commit := percentile(captured.commit, 50)
		t.Logf("%s median=%d p95=%d p99=%d max=%d draw=%d commit=%d residual=%d missed=%d ewram_used=%d ewram_free=%d rom=%d",
			label, median, percentile(captured.cycles, 95), percentile(captured.cycles, 99), percentile(captured.cycles, 100),
			draw, commit, median-draw-commit, captured.missed,
			result.EWRAMUsed, result.EWRAMFree, result.ROMSize)
	}
	change := func(before, after uint32) float64 {
		if before == 0 {
			return 0
		}
		return 100 * (float64(after)/float64(before) - 1)
	}

	runPair := func(label string, pairService build.Service) {
		baselineResult, baseline, err := buildAndCapture(pairService, transpiler.RendererMode4, baselineProject, helper, t.TempDir(), 20, 600)
		if err != nil {
			t.Fatal(err)
		}
		splitResult, split, err := buildAndCapture(pairService, transpiler.RendererMode4, splitProject, helper, t.TempDir(), 20, 600)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(baselineResult.GeneratedGo, "func (g *Game) mixed_step(c *rt.Context, a0 rt.Value, a1 rt.Number, a2 rt.Number) rt.Value") {
			t.Fatalf("%s baseline did not retain the expected mixed tagged ABI", label)
		}
		if !strings.Contains(splitResult.GeneratedGo, "func (g *Game) numeric_step(c *rt.Context, a0 rt.Number, a1 rt.Number, a2 rt.Number) rt.Number") {
			t.Fatalf("%s split source did not emit the expected numeric clone ABI", label)
		}
		if !strings.Contains(splitResult.GeneratedGo, "func (g *Game) generic_step(c *rt.Context, a0 rt.Value, a1 rt.Number, a2 rt.Number) rt.Value") {
			t.Fatalf("%s split source did not retain the expected generic clone ABI", label)
		}

		logResult(label+"/mixed", baselineResult, baseline)
		logResult(label+"/split", splitResult, split)
		differing := compareFrames(baseline.frames, split.frames)
		baselineResidual := percentile(baseline.cycles, 50) - percentile(baseline.draw, 50) - percentile(baseline.commit, 50)
		splitResidual := percentile(split.cycles, 50) - percentile(split.draw, 50) - percentile(split.commit, 50)
		t.Logf("%s change median=%+.2f%% residual=%+.2f%% rom=%+.2f%% ewram=%+d pixels=%d",
			label,
			change(percentile(baseline.cycles, 50), percentile(split.cycles, 50)),
			change(baselineResidual, splitResidual),
			change(uint32(baselineResult.ROMSize), uint32(splitResult.ROMSize)),
			int64(splitResult.EWRAMUsed)-int64(baselineResult.EWRAMUsed), differing)
		if differing != 0 {
			t.Fatalf("%s mixed and manually split workloads differ by %d pixels", label, differing)
		}
	}

	// Shipping defaults show whether TinyGo can erase the generic ABI itself.
	runPair("direct", service)
	// This exposes the out-of-line/root-protected ceiling for functions that
	// allocate, cannot be proven no-alloc, or do not inline.
	protected := service
	protected.DisableNoAllocFunctionCalls = true
	runPair("root-protected", protected)
}
