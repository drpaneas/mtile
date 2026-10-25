package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordedTileLinkRegressionEvidence(t *testing.T) {
	for _, shape := range []string{"default-extent", "explicit-extent"} {
		t.Run(shape, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "docs", "benchmarks", "tile-links", "regression-"+shape+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var record struct {
				CapturedFrames  int            `json:"capturedFrames"`
				DifferingPixels int            `json:"differingPixels"`
				Mode0           rendererResult `json:"mode0"`
				Mode4           rendererResult `json:"mode4"`
			}
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			if record.CapturedFrames < 120 || record.DifferingPixels != 0 {
				t.Fatalf("incomplete pixel-parity evidence: %+v", record)
			}
			for _, renderer := range []rendererResult{record.Mode0, record.Mode4} {
				if renderer.CycleSamples < 120 || renderer.MedianCycles == 0 || renderer.P95Cycles == 0 || renderer.MissedFrames != 0 {
					t.Fatalf("incomplete cycle evidence: %+v", renderer)
				}
			}
		})
	}
}
