//go:build runtimeperfdiagnostic

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// The ordered matrix stops at its first failed acceptance gate. The larger
// Celeste validation is deliberately separate and runs only if this passes.
func TestHashFingerprintDiagnostic(t *testing.T) {
	baseline, candidate, out := os.Getenv("MTILE_HASH_BASELINE"), os.Getenv("MTILE_HASH_CANDIDATE"), os.Getenv("MTILE_HASH_OUT")
	baselineOnly := os.Getenv("MTILE_HASH_BASELINE_ONLY") == "1"
	if baseline == "" || candidate == "" || out == "" {
		t.Skip("set MTILE_HASH_BASELINE, MTILE_HASH_CANDIDATE and MTILE_HASH_OUT")
	}
	leftService, err := build.NewService("", baseline)
	if err != nil {
		t.Fatal(err)
	}
	rightService, err := build.NewService("", candidate)
	if err != nil {
		t.Fatal(err)
	}
	leftService.Debug, rightService.Debug = true, true
	leftService.KeepWork, rightService.KeepWork = true, true
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	goVersion, err := commandOutput("go", "version")
	if err != nil {
		t.Fatal(err)
	}
	tinygoVersion, err := commandOutput(leftService.TinyGoBin, "version")
	if err != nil {
		t.Fatal(err)
	}
	playerSHA, err := fileFingerprint(helper)
	if err != nil {
		t.Fatal(err)
	}
	writeToolchain042JSON(t, filepath.Join(out, "metadata.json"), map[string]any{"go": goVersion, "tinygo": tinygoVersion, "playerSHA256": playerSHA, "warmupFrames": 20, "samples": 600})
	workloads := []struct{ name, fixture, project string }{
		{"collision", "hash_fingerprint_collision.lua", "hello"},
		{"numeric", "numeric_hash_keys.lua", "hello"},
		{"static", "string_hash_keys.lua", "hello"},
		{"distributed", "hash_fingerprint_distributed.lua", "hello"},
		{"same-tag", "hash_fingerprint_same_tag.lua", "hello"},
		{"camera", "camera.lua", "testio"},
	}
	for _, mode := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
		for _, workload := range workloads {
			name := workload.name + "-" + mode.String()
			dir := filepath.Join(out, name)
			for _, variant := range []string{"baseline", "candidate"} {
				if err := os.MkdirAll(filepath.Join(dir, variant), 0755); err != nil {
					t.Fatal(err)
				}
			}
			project, err := stageProjectWithSource(filepath.Join("..", "..", "examples", workload.project), filepath.Join("testdata", "workloads", workload.fixture))
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(project)
			left, lc, err := buildAndCapture(leftService, mode, project, helper, filepath.Join(dir, "baseline"), 20, 600)
			if err != nil {
				t.Fatal(err)
			}
			writeHashFingerprintCapture(t, dir, "baseline", left, lc)
			checkHashFingerprintCapture(t, name+" baseline", workload.name, lc)
			if baselineOnly {
				t.Logf("%s baseline valid: median=%d p95=%d max=%d heap=%d", name, percentile(lc.cycles, 50), percentile(lc.cycles, 95), percentile(lc.cycles, 100), lc.heapAllocated)
				continue
			}
			right, rc, err := buildAndCaptureAtFrames(rightService, mode, project, helper, filepath.Join(dir, "candidate"), 20, 600, lc.frameNumbers)
			if err != nil {
				t.Fatal(err)
			}
			writeHashFingerprintCapture(t, dir, "candidate", right, rc)
			checkHashFingerprintCapture(t, name+" candidate", workload.name, rc)
			if left.GeneratedGo != right.GeneratedGo || !reflect.DeepEqual(lc.frameNumbers, rc.frameNumbers) || !reflect.DeepEqual(lc.watches, rc.watches) || compareFrames(lc.frames, rc.frames) != 0 {
				t.Fatal(name + ": generated source, frame synchronization, watches or pixels differ")
			}
			saving := 100 * (1 - float64(percentile(rc.cycles, 50))/float64(percentile(lc.cycles, 50)))
			p95Change := 100 * (float64(percentile(rc.cycles, 95))/float64(percentile(lc.cycles, 95)) - 1)
			memoryEqual := left.EWRAMUsed == right.EWRAMUsed && lc.heapAllocated == rc.heapAllocated && lc.heapHandles == rc.heapHandles && lc.heapLive == rc.heapLive && lc.heapPeakLive == rc.heapPeakLive
			report := map[string]any{"name": name, "medianImprovementPercent": saving, "p95ChangePercent": p95Change, "memoryEqual": memoryEqual, "pixelsEqual": true, "watchesEqual": true, "generatedSourceEqual": true,
				"warmupMissedFramesDelta": int64(rc.warmupMissed) - int64(lc.warmupMissed),
				"baselineRuntimeSHA256":   celesteParallaxFingerprint(t, baseline), "candidateRuntimeSHA256": celesteParallaxFingerprint(t, candidate), "projectSHA256": celesteParallaxFingerprint(t, project),
				"generatedGoSHA256": fmt.Sprintf("%x", sha256.Sum256([]byte(left.GeneratedGo)))}
			writeToolchain042JSON(t, filepath.Join(dir, "comparison.json"), report)
			t.Logf("%s median %d -> %d (%+.2f%% improvement), p95 %d -> %d (%+.2f%%), memory_equal=%v", name, percentile(lc.cycles, 50), percentile(rc.cycles, 50), saving, percentile(lc.cycles, 95), percentile(rc.cycles, 95), p95Change, memoryEqual)
			if !memoryEqual {
				t.Fatal(name + ": heap/EWRAM changed; stop")
			}
			if workload.name == "collision" {
				if saving < 5 {
					t.Fatal(name + ": median improvement below 5%; stop")
				}
			} else if p95Change > 1 {
				t.Fatal(name + ": control p95 regressed by more than 1%; stop")
			}
		}
	}
}

func checkHashFingerprintCapture(t *testing.T, label, workload string, c capture) {
	t.Helper()
	if len(c.frames) != 600 || c.warmupFrame != 20 || c.missed != c.warmupMissed || percentile(c.cycles, 100) >= twoFrameCycleBudget {
		t.Fatalf("%s: invalid baseline/steady frame budget (samples=%d warmup=%d missed=%d->%d max=%d)", label, len(c.frames), c.warmupFrame, c.warmupMissed, c.missed, percentile(c.cycles, 100))
	}
	for i, frame := range c.frameNumbers {
		want := map[string]int64{}
		switch workload {
		case "collision", "distributed", "same-tag":
			group := int64((frame - 1) % 3)
			generation := int64((frame + 2) / 3)
			sum := 22 + 4*group
			squares := 166 + 44*group + 4*group*group
			want = map[string]int64{"tick": int64(frame), "generation": generation, "checksum": sum + 4*generation, "weighted": squares + sum*generation, "misses": 1, "errors": 0}
			if c.gcCount != 0 {
				t.Fatal(label + ": unexpectedly collected despite initialization-only allocation")
			}
		case "numeric":
			want["checksum"] = 2080 + 64*int64(frame)
		case "static":
			want["checksum"] = 136 + 16*int64(frame)
		case "camera":
			want["frame"] = int64(frame % 120)
		}
		for name, value := range want {
			found := false
			for _, watch := range c.watches[i] {
				if watch.Name == name {
					found = true
					if watch.Kind != 2 || watch.Raw != int32(value*65536) {
						t.Fatalf("%s frame%d %s=%d/%d want number/%d", label, frame, name, watch.Kind, watch.Raw, int32(value*65536))
					}
				}
			}
			if !found {
				t.Fatalf("%s: missing independent watch %s", label, name)
			}
		}
	}
}

func writeHashFingerprintCapture(t *testing.T, dir, variant string, result build.Result, c capture) {
	t.Helper()
	checksums := make([]string, len(c.frames))
	for i, pixels := range c.frames {
		checksums[i] = fmt.Sprintf("%x", sha256.Sum256(pixels))
	}
	romSHA, err := fileFingerprint(result.ROMPath)
	if err != nil {
		t.Fatal(err)
	}
	writeToolchain042JSON(t, filepath.Join(dir, variant+".json"), map[string]any{
		"cycles": c.cycles, "drawCycles": c.draw, "commitCycles": c.commit, "frames": c.frameNumbers, "watches": c.watches, "rgbSHA256": checksums,
		"ROMBytes": result.ROMSize, "ROMSHA256": romSHA, "IWRAMBytes": result.IWRAMUsed, "EWRAMUsedBytes": result.EWRAMUsed,
		"heapAllocatedBytes": c.heapAllocated, "heapLiveBytes": c.heapLive, "heapPeakLiveBytes": c.heapPeakLive, "heapHandles": c.heapHandles, "gcCount": c.gcCount,
		"missedFrames": c.missed, "warmupMissedFrames": c.warmupMissed,
		"workDir": result.WorkDir, "mapPath": result.MapPath,
	})
	for _, name := range []string{"game.elf", "game.map"} {
		data, err := os.ReadFile(filepath.Join(result.WorkDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, variant, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := writePNG(filepath.Join(dir, variant+".png"), c.lastRGB); err != nil {
		t.Fatal(err)
	}
}

// Run only after TestHashFingerprintDiagnostic passes. Reuse the existing
// independent route/physics assertions and exact pixel checks for a real game.
func TestHashFingerprintCelesteDiagnostic(t *testing.T) {
	if os.Getenv("MTILE_HASH_CELESTE") != "1" {
		t.Skip("set MTILE_HASH_CELESTE=1 explicitly after the early gate passes")
	}
	baseline, candidate, parent := os.Getenv("MTILE_HASH_BASELINE"), os.Getenv("MTILE_HASH_CANDIDATE"), os.Getenv("MTILE_HASH_OUT")
	if baseline == "" || candidate == "" || parent == "" {
		t.Skip("set MTILE_HASH_BASELINE, MTILE_HASH_CANDIDATE and MTILE_HASH_OUT after the early gate passes")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	out, project, frozenBaseline, routeLength := prepareCelesteParallaxProbe(t, parent,
		filepath.Join(repository, "examples", "celeste-mode0"), baseline, "celeste-")
	// Mode 0's baseline native-audio/pal/four-BG guard rejects this game. This
	// attempted Mode 4 fallback also failed baseline linking: .data overflowed
	// IWRAM by 880 bytes and .bss by 1252 bytes. Neither yields candidate timing.
	for _, mode := range []transpiler.Renderer{transpiler.RendererMode4} {
		leftName, rightName := "baseline-"+mode.String(), "candidate-"+mode.String()
		left := runCelesteMode0Variant(t, repository, out, project, frozenBaseline, leftName, mode, routeLength)
		right := runCelesteMode0Variant(t, repository, out, project, candidate, rightName, mode, routeLength)
		compareCelesteParallaxPixels(t, out, leftName, rightName, left, right)
		compareCelesteMode0PreviousGameplay(t, repository, right, routeLength)
		ls := celesteParallaxSummarize(t, out, leftName, left, routeLength)
		rs := celesteParallaxSummarize(t, out, rightName, right, routeLength)
		memoryEqual, missesEqual := ls.EWRAMUsed == rs.EWRAMUsed, true
		for i, l := range left {
			r := right[i]
			memoryEqual = memoryEqual && l.Stats.HeapAllocated == r.Stats.HeapAllocated && l.Stats.HeapLive == r.Stats.HeapLive && l.Stats.HeapPeakLive == r.Stats.HeapPeakLive && l.Stats.HeapHandles == r.Stats.HeapHandles
			missesEqual = missesEqual && l.Stats.MissedFrames == r.Stats.MissedFrames
		}
		changes := make(map[string]float64)
		for phase, metrics := range ls.Phases {
			if metrics.Samples != 0 {
				changes[phase] = 100 * (float64(rs.Phases[phase].Frame.P95)/float64(metrics.Frame.P95) - 1)
			}
		}
		writeToolchain042JSON(t, filepath.Join(out, mode.String()+"-comparison.json"), map[string]any{
			"baseline": ls, "candidate": rs, "phaseP95ChangePercent": changes, "memoryEqual": memoryEqual, "missesEqual": missesEqual,
			"projectSHA256": celesteParallaxFingerprint(t, project), "baselineRuntimeSHA256": celesteParallaxFingerprint(t, frozenBaseline), "candidateRuntimeSHA256": celesteParallaxFingerprint(t, candidate),
		})
		if !memoryEqual || !missesEqual {
			t.Fatalf("%s Celeste heap/EWRAM or missed-frame counts differ", mode.String())
		}
		for phase, change := range changes {
			t.Logf("%s Celeste %s p95 change %+.3f%%", mode.String(), phase, change)
			if change > 1 {
				t.Fatalf("%s Celeste %s control p95 regressed >1%%; stop", mode.String(), phase)
			}
		}
		if t.Failed() {
			t.FailNow()
		}
	}
}
