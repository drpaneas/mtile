//go:build runtimeperfdiagnostic

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// TestToolchain042RuntimeDiagnostic compares frozen runtime snapshots under
// the same current compiler. Set MTILE_LLVM22_BASELINE_RUNTIME,
// MTILE_LLVM22_CANDIDATE_RUNTIME and MTILE_LLVM22_OUT to reproduce a run.
func TestToolchain042RuntimeDiagnostic(t *testing.T) {
	before, after, out := os.Getenv("MTILE_LLVM22_BASELINE_RUNTIME"), os.Getenv("MTILE_LLVM22_CANDIDATE_RUNTIME"), os.Getenv("MTILE_LLVM22_OUT")
	if before == "" || after == "" || out == "" {
		t.Skip("set MTILE_LLVM22_BASELINE_RUNTIME, MTILE_LLVM22_CANDIDATE_RUNTIME and MTILE_LLVM22_OUT")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	beforeService, err := build.NewService("", before)
	if err != nil {
		t.Fatal(err)
	}
	afterService, err := build.NewService("", after)
	if err != nil {
		t.Fatal(err)
	}
	beforeService.Debug, afterService.Debug = true, true
	metadata := map[string]any{"dateUTC": time.Now().UTC().Format(time.RFC3339), "baselineRuntimeSHA256": celesteParallaxFingerprint(t, before), "candidateRuntimeSHA256": celesteParallaxFingerprint(t, after)}
	metadata["tinygo"], err = commandOutput(beforeService.TinyGoBin, "version")
	if err != nil {
		t.Fatal(err)
	}
	metadata["go"], err = commandOutput("go", "version")
	if err != nil {
		t.Fatal(err)
	}
	metadata["playerSHA256"], err = fileFingerprint(filepath.Join("..", "..", "build", "player", "mtile-player"))
	if err != nil {
		t.Fatal(err)
	}
	writeToolchain042JSON(t, filepath.Join(out, "metadata.json"), metadata)
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")
	for _, workload := range []struct{ name, fixture, project string }{{"hash", "numeric_hash_keys.lua", "hello"}, {"strings", "string_hash_keys.lua", "hello"}, {"camera", "camera.lua", "testio"}} {
		for _, mode := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
			t.Run(workload.name+"-"+mode.String(), func(t *testing.T) {
				project, err := stageProjectWithSource(filepath.Join("..", "..", diagnosticProjectPath(workload.project)), filepath.Join("testdata", "workloads", workload.fixture))
				if err != nil {
					t.Fatal(err)
				}
				defer os.RemoveAll(project)
				left, lc, err := buildAndCapture(beforeService, mode, project, helper, t.TempDir(), 20, 600)
				if err != nil {
					t.Fatal(err)
				}
				right, rc, err := buildAndCaptureAtFrames(afterService, mode, project, helper, t.TempDir(), 20, 600, lc.frameNumbers)
				if err != nil {
					t.Fatal(err)
				}
				if left.GeneratedGo != right.GeneratedGo {
					t.Fatal("generated workload changed")
				}
				if err := validateSynchronizedFrames(lc.frameNumbers, rc.frameNumbers); err != nil {
					t.Fatal(err)
				}
				report := map[string]any{"fixture": workload.fixture, "sourceSHA256": celesteParallaxFingerprint(t, project)}
				for _, v := range []struct {
					name string
					b    build.Result
					c    capture
				}{{"baseline", left, lc}, {"candidate", right, rc}} {
					pixels := make([]string, len(v.c.frames))
					for i, frame := range v.c.frames {
						pixels[i] = fmt.Sprintf("%x", sha256.Sum256(frame))
					}
					romSHA, err := fileFingerprint(v.b.ROMPath)
					if err != nil {
						t.Fatal(err)
					}
					report[v.name] = map[string]any{"cycles": v.c.cycles, "drawCycles": v.c.draw, "commitCycles": v.c.commit, "frames": v.c.frameNumbers, "rgbSHA256": pixels, "ROMBytes": v.b.ROMSize, "ROMSHA256": romSHA, "IWRAMBytes": v.b.IWRAMUsed, "EWRAMFreeBytes": v.b.EWRAMFree, "heapAllocatedBytes": v.c.heapAllocated, "missedFrames": v.c.missed, "warmupMissedFrames": v.c.warmupMissed}
					t.Logf("%s median=%d p95=%d max=%d rom=%d iwram=%d ewram_free=%d heap=%d missed=%d", v.name, percentile(v.c.cycles, 50), percentile(v.c.cycles, 95), percentile(v.c.cycles, 100), v.b.ROMSize, v.b.IWRAMUsed, v.b.EWRAMFree, v.c.heapAllocated, v.c.missed)
				}
				diff := compareFrames(lc.frames, rc.frames)
				t.Logf("saving=%.2f%% differing_pixels=%d", 100*(1-float64(percentile(rc.cycles, 50))/float64(percentile(lc.cycles, 50))), diff)
				if diff != 0 {
					t.Fatal("pixel parity failed")
				}
				report["differingPixels"] = diff
				writeToolchain042JSON(t, filepath.Join(out, workload.name+"-"+mode.String()+".json"), report)
				if workload.name == "hash" {
					checkIWRAMHashCapture(t, "baseline", lc)
					checkIWRAMHashCapture(t, "candidate", rc)
					if percentile(rc.cycles, 50) >= percentile(lc.cycles, 50) {
						t.Error("numeric hash median must improve")
					}
				}
				if float64(percentile(rc.cycles, 95)) > float64(percentile(lc.cycles, 95))*1.01 {
					t.Error("p95 regressed by more than 1 percent")
				}
				if rc.heapAllocated != lc.heapAllocated || right.EWRAMUsed != left.EWRAMUsed || rc.missed > lc.missed {
					t.Error("heap, EWRAM or missed frames regressed")
				}
			})
		}
	}
	t.Run("celeste", func(t *testing.T) {
		projectOut, project, _, routeLen := prepareCelesteParallaxProbe(t, out, filepath.Join("..", "..", "examples", "celeste"), before, "paired-")
		left := runCelesteParallaxVariant(t, projectOut, project, before, "baseline", false, false, 0x0404, routeLen)
		right := runCelesteParallaxVariant(t, projectOut, project, after, "candidate", false, false, 0x0404, routeLen)
		compareCelesteParallaxPixels(t, projectOut, "baseline", "candidate", left, right)
		writeToolchain042JSON(t, filepath.Join(out, "celeste-summary.json"), map[string]any{"baseline": celesteParallaxSummarize(t, projectOut, "baseline", left, routeLen), "candidate": celesteParallaxSummarize(t, projectOut, "candidate", right, routeLen), "exactPixelsAndWatches": celesteParallaxExactRows(left, right), "evidenceDirectory": projectOut})
	})
}

func writeToolchain042JSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.WriteFile(path, mustCelesteParallaxJSON(t, value), 0644); err != nil {
		t.Fatal(err)
	}
}
