//go:build multireturnphysicsdiagnostic

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestMultiReturnPhysicsDiagnostic(t *testing.T) {
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	project := filepath.Join("..", "..", "examples", "multi-return-physics-stress")
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
	numericSignature := "func (g *Game) accelerate(c *rt.Context, a0 rt.Number, a1 rt.Number, a2 rt.Number) (rt.Number, rt.Number)"
	runPair := func(label string, pairService build.Service, requireImprovement bool) {
		baselineService := pairService
		baselineService.DisableMonomorphicMultiReturns = true
		baselineResult, baseline, err := buildAndCapture(baselineService, transpiler.RendererMode4, project, helper, t.TempDir(), 20, 600)
		if err != nil {
			t.Fatal(err)
		}
		optimizedResult, optimized, err := buildAndCapture(pairService, transpiler.RendererMode4, project, helper, t.TempDir(), 20, 600)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(baselineResult.GeneratedGo, numericSignature) {
			t.Fatalf("%s tagged baseline unexpectedly emitted a numeric multi-return signature", label)
		}
		if !strings.Contains(optimizedResult.GeneratedGo, numericSignature) {
			t.Fatalf("%s optimized build did not emit the expected numeric multi-return signature", label)
		}
		logResult(label+"/tagged", baselineResult, baseline)
		logResult(label+"/numeric", optimizedResult, optimized)
		differing := compareFrames(baseline.frames, optimized.frames)
		t.Logf("%s same-index differing pixels=%d", label, differing)
		if differing != 0 {
			t.Fatalf("%s tagged and numeric multi-return ROMs differ by %d pixels", label, differing)
		}
		if requireImprovement && percentile(optimized.cycles, 50) >= percentile(baseline.cycles, 50) {
			t.Fatalf("%s numeric multi-return median did not improve over tagged baseline", label)
		}
	}

	// First measure shipping defaults. TinyGo may inline these proven
	// no-allocation helpers and erase the ABI distinction completely.
	runPair("direct", service, false)
	// Then keep the common call path out-of-line to expose the tuple ABI cost.
	protected := service
	protected.DisableNoAllocFunctionCalls = true
	runPair("root-protected", protected, true)
}
