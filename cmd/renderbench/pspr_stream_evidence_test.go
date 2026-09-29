package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordedPSprStreamingEvidencePreservesPixelsAndVBlank(t *testing.T) {
	for _, name := range []string{"anim-clock", "anim-clips", "fighters", "large-bank", "deduplicated-bank", "mutable-source", "full-window", "full-window-mid-oam", "full-window-high-oam", "combined-budget", "packed-budget"} {
		t.Run(name, func(t *testing.T) {
			requireRecordedBenchmarkParity(t, loadRecordedBenchmarkEvidence(t, "pspr-stream-"+name+"-v1"))
			data, err := os.ReadFile(filepath.Join("..", "..", "docs", "benchmarks", "render-modes", "pspr-stream-"+name+"-v1.json"))
			if err != nil {
				t.Fatal(err)
			}
			var evidence struct {
				MaximumCommit  uint32 `json:"mode0MaximumCommitCycles"`
				LCDScans       int    `json:"lcdScans"`
				LCDDifferences int    `json:"lcdDifferingPixels"`
			}
			if err := json.Unmarshal(data, &evidence); err != nil {
				t.Fatal(err)
			}
			if evidence.MaximumCommit == 0 || evidence.MaximumCommit >= 67*1232 || evidence.LCDScans < 120 || evidence.LCDDifferences != 0 {
				t.Fatalf("unsafe or incomplete presentation evidence: %+v", evidence)
			}
		})
	}
}
