package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRecordedNamedPoseEvidence(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "benchmarks", "render-modes", "pose-names-v1", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		ROM    string   `json:"romSha256"`
		Go     string   `json:"generatedGoSha256"`
		Cycles []uint32 `json:"cycles"`
		Frames []string `json:"frameSha256"`
		Ticks  []uint32 `json:"frameNumbers"`
		Missed int      `json:"missedFrames"`
		Heap   bool     `json:"heapLinked"`
	}
	var report struct {
		Samples           int            `json:"samples"`
		Differences       int            `json:"differentPixels"`
		OracleDifferences int            `json:"oracleDifferentPixels"`
		LCDDifferences    int            `json:"lcdDifferentPixels"`
		LCDScans          int            `json:"lcdScans"`
		Renderers         map[string]row `json:"renderers"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Samples < 120 || report.Differences != 0 || report.OracleDifferences != 0 || report.LCDDifferences != 0 || report.LCDScans < 240 {
		t.Fatalf("missing named-pose parity evidence: %+v", report)
	}
	for _, renderer := range []string{"mode0", "mode4"} {
		a, b := report.Renderers[renderer+"-named"], report.Renderers[renderer+"-numeric"]
		if a.ROM == "" || a.Go == "" || a.ROM != b.ROM || a.Go != b.Go || !reflect.DeepEqual(a.Cycles, b.Cycles) || !reflect.DeepEqual(a.Frames, b.Frames) || !reflect.DeepEqual(a.Ticks, b.Ticks) {
			t.Fatalf("%s named/numeric code, cycles or pixels differ", renderer)
		}
		if len(a.Cycles) != report.Samples || len(a.Frames) != report.Samples || a.Missed != 0 || b.Missed != 0 || a.Heap || b.Heap {
			t.Fatalf("incomplete %s cost evidence", renderer)
		}
	}
	if !reflect.DeepEqual(report.Renderers["mode0-named"].Frames, report.Renderers["mode4-named"].Frames) || !reflect.DeepEqual(report.Renderers["mode0-named"].Ticks, report.Renderers["mode4-named"].Ticks) {
		t.Fatal("named frames were not synchronized across renderers")
	}
}

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
