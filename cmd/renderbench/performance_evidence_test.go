package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type recordedOptimizationBaseline struct {
	MedianCycles     uint32 `json:"medianCycles"`
	MedianDrawCycles uint32 `json:"medianDrawCycles"`
	MissedFrames     uint32 `json:"missedFrames"`
	DifferingPixels  int    `json:"differingPixels"`
}

type recordedBenchmarkEvidence struct {
	SchemaVersion        int                          `json:"schemaVersion"`
	GatePolicy           string                       `json:"gatePolicy"`
	CapturedFrames       int                          `json:"capturedFrames"`
	DifferingPixels      int                          `json:"differingPixels"`
	EWRAMGain            int64                        `json:"ewramGain"`
	Mode0MedianSavingPC  float64                      `json:"mode0MedianSavingPercent"`
	Mode0P95Headroom     int64                        `json:"mode0P95Headroom"`
	Mode4                rendererResult               `json:"mode4"`
	Mode0                rendererResult               `json:"mode0"`
	OptimizationBaseline recordedOptimizationBaseline `json:"optimizationBaseline"`
	ParityPass           bool                         `json:"parityPass"`
	Pass                 bool                         `json:"pass"`
}

func loadRecordedBenchmarkEvidence(t *testing.T, name string) recordedBenchmarkEvidence {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "benchmarks", "render-modes", name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var evidence recordedBenchmarkEvidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return evidence
}

func recordedReductionPercent(before, after uint32) float64 {
	if before == 0 || after >= before {
		return 0
	}
	return float64(before-after) * 100 / float64(before)
}

func requireRecordedBenchmarkParity(t *testing.T, evidence recordedBenchmarkEvidence) {
	t.Helper()
	if evidence.SchemaVersion != reportSchemaVersion || evidence.GatePolicy != gateParity || evidence.CapturedFrames < 120 {
		t.Fatalf("evidence metadata = %+v, want schema %d parity gate with at least 120 frames", evidence, reportSchemaVersion)
	}
	if evidence.DifferingPixels != 0 || !evidence.ParityPass || !evidence.Pass {
		t.Fatalf("parity evidence = %+v, want zero differing pixels and passing parity gate", evidence)
	}
	if evidence.Mode0.ROMSHA == "" || evidence.Mode4.ROMSHA == "" || evidence.Mode0.CycleSamples < 120 || evidence.Mode4.CycleSamples < 120 {
		t.Fatalf("evidence lacks reproducible ROM hashes or cycle samples: %+v", evidence)
	}
}

func TestRecordedOAM64EvidenceMeetsPerformanceFloor(t *testing.T) {
	evidence := loadRecordedBenchmarkEvidence(t, "oam64-v1")
	requireRecordedBenchmarkParity(t, evidence)
	if evidence.Mode0.OAMEntries != 64 || evidence.Mode0.OBJLineCycles != 64 {
		t.Fatalf("Mode 0 hardware budget = %+v, want OAM=64 line=64", evidence.Mode0)
	}
	if evidence.Mode0MedianSavingPC < 30 || evidence.Mode0.MedianCycles >= evidence.Mode4.MedianCycles {
		t.Fatalf("median performance = Mode0 %d, Mode4 %d, saving %.2f%%; want at least 30%%",
			evidence.Mode0.MedianCycles, evidence.Mode4.MedianCycles, evidence.Mode0MedianSavingPC)
	}
	if evidence.Mode0P95Headroom < 400000 || evidence.EWRAMGain < 37000 || evidence.Mode0.MissedFrames != 0 {
		t.Fatalf("headroom/memory/frame evidence regressed: %+v", evidence)
	}
}

func TestRecordedScanline38EvidenceMeetsPerformanceFloor(t *testing.T) {
	evidence := loadRecordedBenchmarkEvidence(t, "scanline38-v1")
	requireRecordedBenchmarkParity(t, evidence)
	if evidence.Mode0.OAMEntries != 38 || evidence.Mode0.OBJLineCycles != 1216 {
		t.Fatalf("Mode 0 hardware budget = %+v, want OAM=38 line=1216", evidence.Mode0)
	}
	if evidence.Mode0MedianSavingPC < 90 || evidence.Mode0P95Headroom < 400000 || evidence.Mode0.MissedFrames != 0 {
		t.Fatalf("scanline stress evidence regressed: %+v", evidence)
	}
}

func TestRecordedPacked16EvidenceBeatsPrePackingBaseline(t *testing.T) {
	evidence := loadRecordedBenchmarkEvidence(t, "packed-sprite16-v1")
	requireRecordedBenchmarkParity(t, evidence)
	totalReduction := recordedReductionPercent(evidence.OptimizationBaseline.MedianCycles, evidence.Mode0.MedianCycles)
	drawReduction := recordedReductionPercent(evidence.OptimizationBaseline.MedianDrawCycles, evidence.Mode0.MedianDraw)
	if totalReduction < 5 || drawReduction < 30 {
		t.Fatalf("packed16 reduction total=%.2f%% draw=%.2f%%, want at least 5%%/30%%", totalReduction, drawReduction)
	}
	if evidence.Mode0.OAMEntries != 8 || evidence.Mode0.OBJLineCycles != 64 || evidence.Mode0.MissedFrames != 0 {
		t.Fatalf("packed16 evidence = %+v, want OAM=8 line=64 with no missed frames", evidence.Mode0)
	}
}

func TestRecordedPacked32EvidenceBeatsPrePackingBaseline(t *testing.T) {
	evidence := loadRecordedBenchmarkEvidence(t, "packed-sprite32-v1")
	requireRecordedBenchmarkParity(t, evidence)
	totalReduction := recordedReductionPercent(evidence.OptimizationBaseline.MedianCycles, evidence.Mode0.MedianCycles)
	drawReduction := recordedReductionPercent(evidence.OptimizationBaseline.MedianDrawCycles, evidence.Mode0.MedianDraw)
	if totalReduction < 20 || drawReduction < 70 {
		t.Fatalf("packed32 reduction total=%.2f%% draw=%.2f%%, want at least 20%%/70%%", totalReduction, drawReduction)
	}
	if evidence.Mode0.OAMEntries != 2 || evidence.Mode0.OBJLineCycles != 64 || evidence.Mode0.MissedFrames != 0 {
		t.Fatalf("packed32 evidence = %+v, want OAM=2 line=64 with no missed frames", evidence.Mode0)
	}
}

func TestRecordedGlyphInternEvidenceBeatsPerOccurrenceUploadBaseline(t *testing.T) {
	evidence := loadRecordedBenchmarkEvidence(t, "glyph-intern-v1")
	requireRecordedBenchmarkParity(t, evidence)
	totalReduction := recordedReductionPercent(evidence.OptimizationBaseline.MedianCycles, evidence.Mode0.MedianCycles)
	drawReduction := recordedReductionPercent(evidence.OptimizationBaseline.MedianDrawCycles, evidence.Mode0.MedianDraw)
	if totalReduction < 50 || drawReduction < 80 {
		t.Fatalf("glyph interning reduction total=%.2f%% draw=%.2f%%, want at least 50%%/80%%", totalReduction, drawReduction)
	}
	if evidence.Mode0.OAMEntries != 32 || evidence.Mode0.OBJLineCycles != 256 ||
		evidence.Mode0MedianSavingPC < 50 || evidence.EWRAMGain < 37000 || evidence.Mode0.MissedFrames != 0 {
		t.Fatalf("glyph interning evidence regressed: %+v", evidence)
	}
}

func TestRecordedObjectBudgetEvidenceAlwaysFavorsMode0Median(t *testing.T) {
	for _, name := range []string{"oam64-v1", "scanline38-v1", "packed-sprite16-v1", "packed-sprite32-v1", "glyph-intern-v1", "dynamic-sprite-frames-v1", "inactive-sprite-slots-v1", "object-depth-lanes-v1"} {
		t.Run(name, func(t *testing.T) {
			evidence := loadRecordedBenchmarkEvidence(t, name)
			requireRecordedBenchmarkParity(t, evidence)
			if evidence.Mode0.MedianCycles >= evidence.Mode4.MedianCycles || evidence.EWRAMGain <= 0 || evidence.Mode0P95Headroom <= 0 {
				t.Fatalf("Mode 0 does not improve median cycles, EWRAM, and p95 headroom: %+v", evidence)
			}
		})
	}
}

func TestRecordedDynamicSpriteFramesEvidenceMeetsPerformanceFloor(t *testing.T) {
	evidence := loadRecordedBenchmarkEvidence(t, "dynamic-sprite-frames-v1")
	requireRecordedBenchmarkParity(t, evidence)
	if evidence.Mode0.OAMEntries != 16 || evidence.Mode0.OBJLineCycles != 32 {
		t.Fatalf("Mode 0 hardware budget = %+v, want OAM=16 line=32", evidence.Mode0)
	}
	if evidence.Mode0MedianSavingPC < 50 || evidence.Mode0P95Headroom < 400000 ||
		evidence.EWRAMGain < 37000 || evidence.Mode0.MissedFrames != 0 || evidence.Mode4.MissedFrames != 0 {
		t.Fatalf("dynamic sprite evidence regressed: %+v", evidence)
	}
}

func TestRecordedInactiveSpriteSlotsEvidenceMeetsPerformanceFloor(t *testing.T) {
	evidence := loadRecordedBenchmarkEvidence(t, "inactive-sprite-slots-v1")
	requireRecordedBenchmarkParity(t, evidence)
	if evidence.Mode0.OAMEntries != 64 || evidence.Mode0.OBJLineCycles != 64 {
		t.Fatalf("Mode 0 static hardware budget = %+v, want conservative OAM=64 line=64", evidence.Mode0)
	}
	if evidence.Mode0MedianSavingPC < 50 || evidence.Mode0P95Headroom < 400000 ||
		evidence.EWRAMGain < 37000 || evidence.Mode0.MissedFrames != 0 || evidence.Mode4.MissedFrames != 0 {
		t.Fatalf("inactive sprite-slot evidence regressed: %+v", evidence)
	}
}

func TestRecordedObjectDepthLanesEvidenceMeetsParityAndPerformanceFloor(t *testing.T) {
	evidence := loadRecordedBenchmarkEvidence(t, "object-depth-lanes-v1")
	requireRecordedBenchmarkParity(t, evidence)
	if evidence.Mode0.OAMEntries != 4 || evidence.Mode0.OBJLineCycles != 16 {
		t.Fatalf("Mode 0 object-depth budget = %+v, want OAM=4 line=16", evidence.Mode0)
	}
	if evidence.Mode0MedianSavingPC < 90 || evidence.Mode0P95Headroom < 500000 ||
		evidence.EWRAMGain < 20000 || evidence.Mode0.MissedFrames > 1 {
		t.Fatalf("object-depth evidence regressed: %+v", evidence)
	}
}
