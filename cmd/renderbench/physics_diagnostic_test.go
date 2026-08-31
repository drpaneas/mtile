//go:build physicsdiagnostic

package main

import (
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestPhysicsMode4Diagnostic(t *testing.T) {
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	// Keep later call-path optimizations out of this numeric-storage A/B.
	service.DisableNoAllocFunctionCalls = true
	project := filepath.Join("..", "..", "examples", "physics-stress")
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")
	legacyService := service
	legacyService.DisableMonomorphicNumbers = true
	legacyResult, legacy, err := buildAndCapture(legacyService, transpiler.RendererMode4, project, helper, t.TempDir(), 20, 600)
	if err != nil {
		t.Fatal(err)
	}
	optimizedResult, optimized, err := buildAndCapture(service, transpiler.RendererMode4, project, helper, t.TempDir(), 20, 600)
	if err != nil {
		t.Fatal(err)
	}
	logResult := func(label string, result build.Result, captured capture) {
		median := percentile(captured.cycles, 50)
		draw := percentile(captured.draw, 50)
		commit := percentile(captured.commit, 50)
		t.Logf("%s median=%d p95=%d p99=%d max=%d draw=%d commit=%d residual=%d missed=%d ewram_used=%d ewram_free=%d rom=%d",
			label, median, percentile(captured.cycles, 95), percentile(captured.cycles, 99), percentile(captured.cycles, 100),
			draw, commit, median-draw-commit, captured.missed,
			result.EWRAMUsed, result.EWRAMFree, result.ROMSize)
	}
	logResult("tagged", legacyResult, legacy)
	logResult("monomorphic", optimizedResult, optimized)
	differing := compareFrames(legacy.frames, optimized.frames)
	t.Logf("same-index differing pixels=%d", differing)
	if differing != 0 {
		t.Fatalf("tagged and monomorphic ROMs differ by %d pixels", differing)
	}
	if percentile(optimized.cycles, 50) >= percentile(legacy.cycles, 50) {
		t.Fatal("monomorphic median did not improve over tagged baseline")
	}
}
