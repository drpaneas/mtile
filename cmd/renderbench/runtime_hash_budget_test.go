//go:build runtimeperfdiagnostic

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
	rt "github.com/drpaneas/mtile/runtime"
)

// TestRuntimeNumericHashFrameBudget keeps the original 64-key hash workload
// unchanged and requires all 600 captured ticks to fit the 30 Hz budget. Unlike
// a median-only comparison, it also catches presentation deadline misses and
// independently checks the checksum produced by all 64 reads and writes.
//
//	go test ./cmd/renderbench -tags=runtimeperfdiagnostic -run TestRuntimeNumericHashFrameBudget -count=1 -v
func TestRuntimeNumericHashFrameBudget(t *testing.T) {
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	project, err := stageProjectWithSource(
		filepath.Join("..", "..", "examples", "hello"),
		filepath.Join("testdata", "workloads", "numeric_hash_keys.lua"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(project)
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")
	var reference capture

	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			result, captured, err := buildAndCaptureAtFrames(service, renderer, project, helper, t.TempDir(), 20, 600, reference.frameNumbers)
			if err != nil {
				t.Fatal(err)
			}
			maxCycles := percentile(captured.cycles, 100)
			t.Logf("median=%d p95=%d max=%d budget=%d draw=%d commit=%d rom=%d iwram=%d ewram_free=%d heap_allocated=%d heap_handles=%d gc=%d warmup_frame=%d warmup_missed=%d final_missed=%d steady_missed=%d",
				percentile(captured.cycles, 50), percentile(captured.cycles, 95), maxCycles, twoFrameCycleBudget,
				percentile(captured.draw, 50), percentile(captured.commit, 50), result.ROMSize, result.IWRAMUsed, result.EWRAMFree,
				captured.heapAllocated, captured.heapHandles, captured.gcCount, captured.warmupFrame, captured.warmupMissed, captured.missed, captured.missed-captured.warmupMissed)
			if captured.warmupFrame != 20 {
				t.Fatalf("miss counter baseline is frame %d, want final warm-up frame 20", captured.warmupFrame)
			}
			if maxCycles >= twoFrameCycleBudget {
				t.Errorf("worst tick uses %d cycles, must be below %d", maxCycles, twoFrameCycleBudget)
			}
			if captured.missed != captured.warmupMissed {
				t.Errorf("missed-frame counter grew during capture: %d -> %d", captured.warmupMissed, captured.missed)
			}
			for i, frameNumber := range captured.frameNumbers {
				// The initial values sum to 1+...+64 = 2080. Every _update
				// adds one to each of the 64 entries before summing them.
				// FromInt preserves the 16:16 wrap reached at tick 480.
				want := rt.FromInt(2080 + 64*int(frameNumber))
				found := false
				for _, watch := range captured.watches[i] {
					if watch.Name != "checksum" {
						continue
					}
					found = true
					if watch.Kind != byte(rt.KindNumber) || watch.Raw != int32(want) {
						t.Fatalf("frame %d checksum kind=%d raw=%d, want number raw=%d", frameNumber, watch.Kind, watch.Raw, want)
					}
				}
				if !found {
					t.Fatalf("frame %d does not publish the checksum watch", frameNumber)
				}
			}
			changingFrames := false
			for _, frame := range captured.frames[1:] {
				if countDifferingPixels(captured.frames[0], frame) != 0 {
					changingFrames = true
					break
				}
			}
			if !changingFrames {
				t.Error("checksum did not change the captured pixels")
			}
			if len(reference.frames) == 0 {
				reference = captured
			} else {
				if err := validateSynchronizedFrames(reference.frameNumbers, captured.frameNumbers); err != nil {
					t.Fatal(err)
				}
				differing := compareFrames(reference.frames, captured.frames)
				t.Logf("cross_renderer_differing_pixels=%d checksum_frames=%d", differing, len(captured.frameNumbers))
				if differing != 0 {
					t.Fatalf("renderer captures differ by %d pixels", differing)
				}
			}
		})
	}
}
