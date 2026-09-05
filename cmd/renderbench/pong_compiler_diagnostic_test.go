//go:build runtimeperfdiagnostic

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

type pongCompilerSnapshot struct {
	Build                                  build.Result
	Cycles, Draw, Commit, Frames           []uint32
	Pixels                                 []string
	Watches                                [][]emulator.Watch
	Missed, HeapAllocated, HeapHandles, GC uint32
}

// TestPongCompilerDiagnostic records a 600-tick target capture for comparing
// compiler revisions. Run once with MTILE_PONG_SNAPSHOT, then use another output
// path plus MTILE_PONG_COMPARE pointing at that baseline. SHA-256 hashes keep
// pixel comparison exact without storing hundreds of full framebuffers.
func TestPongCompilerDiagnostic(t *testing.T) {
	out := os.Getenv("MTILE_PONG_SNAPSHOT")
	if out == "" {
		t.Skip("set MTILE_PONG_SNAPSHOT")
	}
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	result, c, err := buildAndCapture(service, transpiler.RendererMode4, filepath.Join("..", "..", "examples", "pong"), filepath.Join("..", "..", "build", "player", "mtile-player"), t.TempDir(), 20, 600)
	if err != nil {
		t.Fatal(err)
	}
	snap := pongCompilerSnapshot{Build: result, Cycles: c.cycles, Draw: c.draw, Commit: c.commit, Frames: c.frameNumbers, Watches: c.watches, Missed: c.missed, HeapAllocated: c.heapAllocated, HeapHandles: c.heapHandles, GC: c.gcCount}
	for _, frame := range c.frames {
		hash := sha256.Sum256(frame)
		snap.Pixels = append(snap.Pixels, hex.EncodeToString(hash[:]))
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("snapshot=%s median=%d p95=%d p99=%d max=%d draw=%d commit=%d ROM=%d IWRAM=%d EWRAM=%d heap=%d handles=%d GC=%d missed=%d", out, percentile(c.cycles, 50), percentile(c.cycles, 95), percentile(c.cycles, 99), percentile(c.cycles, 100), percentile(c.draw, 50), percentile(c.commit, 50), result.ROMSize, result.IWRAMUsed, result.EWRAMUsed, c.heapAllocated, c.heapHandles, c.gcCount, c.missed)
	if c.warmupFrame != 20 || c.missed != 0 || percentile(c.cycles, 100) >= twoFrameCycleBudget {
		t.Fatal("Pong must capture the expected warm-up tick and stay within its tick budget without misses")
	}
	if beforePath := os.Getenv("MTILE_PONG_COMPARE"); beforePath != "" {
		var before pongCompilerSnapshot
		if filepath.Clean(beforePath) == filepath.Clean(out) {
			t.Fatal("snapshot output must differ from the baseline input")
		}
		data, err := os.ReadFile(beforePath)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(data, &before); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(snap.Frames, before.Frames) || !reflect.DeepEqual(snap.Pixels, before.Pixels) || !reflect.DeepEqual(snap.Watches, before.Watches) {
			t.Fatal("frames, pixels, or watches differ")
		}
		if snap.Missed != before.Missed || snap.HeapAllocated != before.HeapAllocated || snap.HeapHandles != before.HeapHandles || snap.GC != before.GC {
			t.Fatal("misses or heap activity differ")
		}
		if percentile(c.cycles, 50) >= percentile(before.Cycles, 50) || percentile(c.cycles, 95) > percentile(before.Cycles, 95) {
			t.Error("compiler change must improve median cycles without regressing p95")
		}
		t.Logf("saving=%.2f%% unchanged_pixel_frames=%d ROM_delta=%d IWRAM_delta=%d EWRAM_delta=%d", 100*(1-float64(percentile(c.cycles, 50))/float64(percentile(before.Cycles, 50))), len(snap.Frames), result.ROMSize-before.Build.ROMSize, int64(result.IWRAMUsed)-int64(before.Build.IWRAMUsed), int64(result.EWRAMUsed)-int64(before.Build.EWRAMUsed))
	}
	if err = os.WriteFile(out, data, 0644); err != nil {
		t.Fatal(err)
	}
}
