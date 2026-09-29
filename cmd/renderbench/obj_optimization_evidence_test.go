package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Keep the native evidence inspectable without installing TinyGo or mGBA.
// New measured phases must retain exactly the same source projects and pixels.
func TestRecordedOBJOptimizationEvidence(t *testing.T) {
	type renderer struct {
		ROMBytes     int        `json:"romBytes"`
		ROMSHA       string     `json:"romSha256"`
		FrameNumbers []uint32   `json:"frameNumbers"`
		FrameSHA     []string   `json:"frameSha256"`
		Cycles       []uint32   `json:"cycles"`
		Commit       []uint32   `json:"commitCycleSamples"`
		LCD          [][]string `json:"lcdFrameSha256"`
	}
	type report struct {
		Environment     environmentReport `json:"environment"`
		Samples         int               `json:"samples"`
		DifferingPixels int               `json:"differingPixels"`
		LCDScans        int               `json:"lcdScans"`
		LCDDifferences  int               `json:"lcdDifferingPixels"`
		BeforeAfter     bool              `json:"beforeAfterPixelsMatch"`
		Mode0           renderer          `json:"mode0"`
		Mode4           renderer          `json:"mode4"`
	}
	root := filepath.Join("..", "..", "docs", "benchmarks", "render-modes", "obj-optimization-v1")
	phases, err := filepath.Glob(filepath.Join(root, "phase*"))
	if err != nil || len(phases) == 0 {
		t.Fatalf("optimized phases missing: %v", err)
	}
	load := func(t *testing.T, path string) report {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var r report
		if err := json.Unmarshal(data, &r); err != nil {
			t.Fatal(err)
		}
		if r.Samples != 120 || r.DifferingPixels != 0 || r.LCDScans < r.Samples || r.LCDDifferences != 0 {
			t.Fatalf("incomplete parity evidence: %s", path)
		}
		if !reflect.DeepEqual(r.Mode0.FrameNumbers, r.Mode4.FrameNumbers) || !reflect.DeepEqual(r.Mode0.FrameSHA, r.Mode4.FrameSHA) {
			t.Fatalf("renderer frames differ: %s", path)
		}
		for _, mode := range []renderer{r.Mode0, r.Mode4} {
			if mode.ROMBytes <= 0 || len(mode.ROMSHA) != 64 || len(mode.FrameNumbers) != r.Samples || len(mode.FrameSHA) != r.Samples || len(mode.Cycles) != r.Samples || len(mode.Commit) != r.Samples || len(mode.LCD) != r.Samples {
				t.Fatalf("incomplete native samples: %s", path)
			}
			for i, n := range mode.FrameNumbers {
				if i > 0 && n != mode.FrameNumbers[i-1]+1 {
					t.Fatalf("skipped game frame: %s", path)
				}
				if mode.Cycles[i] == 0 || len(mode.FrameSHA[i]) != 64 || len(mode.LCD[i]) == 0 {
					t.Fatalf("empty native sample: %s", path)
				}
			}
		}
		for _, cycles := range r.Mode0.Commit {
			if cycles == 0 || cycles >= 67*1232 {
				t.Fatalf("Mode0 commit outside VBlank: %s (%d)", path, cycles)
			}
		}
		return r
	}
	for _, name := range []string{"anim-clock", "anim-clips", "fighters", "full-window", "library-180", "nerdo"} {
		t.Run(name, func(t *testing.T) {
			baseline := load(t, filepath.Join(root, "baseline", name+".json"))
			for _, phase := range phases {
				after := load(t, filepath.Join(phase, name+".json"))
				if !after.BeforeAfter || baseline.Environment.ProjectSHA != after.Environment.ProjectSHA || baseline.Environment.HelperSHA != after.Environment.HelperSHA {
					t.Fatalf("before/after input mismatch: %s", phase)
				}
				for _, pair := range [][2]renderer{{baseline.Mode0, after.Mode0}, {baseline.Mode4, after.Mode4}} {
					if !reflect.DeepEqual(pair[0].FrameNumbers, pair[1].FrameNumbers) || !reflect.DeepEqual(pair[0].FrameSHA, pair[1].FrameSHA) {
						t.Fatalf("before/after game pixels differ: %s", phase)
					}
					for i, hashes := range pair[1].LCD {
						previous := pair[0].LCD[i]
						for _, hash := range hashes {
							if hash != previous[len(previous)-1] {
								t.Fatalf("before/after LCD pixels differ: %s", phase)
							}
						}
					}
				}
			}
		})
	}
}
