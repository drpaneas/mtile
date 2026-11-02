package audioimport

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
)

func TestQuietEdgesAreOptionalAndSizeEstimatesMatchProducedWAVs(t *testing.T) {
	const rate = 48000
	path := testWave(t, 1, 16, rate, 2, func(i, ch int) float64 {
		if i < 4800 || i >= 7200 {
			return 0
		}
		return .5 * math.Sin(2*math.Pi*1000*float64(i)/rate)
	})
	original, _ := os.ReadFile(path)
	full, err := Prepare(path, QualityBalanced)
	if err != nil {
		t.Fatal(err)
	}
	a := full.Analysis
	if full.Trimmed || !a.CanTrim() || math.Abs(a.RemovedSeconds()-.16) > .001 {
		t.Fatalf("quiet edges: %+v", a)
	}
	if full.After.Seconds() != .25 {
		t.Fatal("default preparation trimmed timing")
	}
	for _, quality := range []Quality{QualityBalanced, QualitySmall, QualitySource} {
		for _, trim := range []bool{false, true} {
			if quality == QualitySource && trim {
				continue
			}
			p, err := PrepareWithOptions(path, PrepareOptions{Quality: quality, TrimSilence: trim})
			if err != nil {
				t.Fatal(err)
			}
			if estimate := a.PreparedBytes(quality, trim); estimate != len(p.Data) {
				t.Fatalf("%s trim %v estimated %d actual %d", quality, trim, estimate, len(p.Data))
			}
			if trim && (!p.Trimmed || math.Abs(p.After.Seconds()-.09) > .001) {
				t.Fatalf("trimmed duration: %+v", p.After)
			}
		}
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("external source changed")
	}
	if _, err := PrepareWithOptions(path, PrepareOptions{Quality: QualitySource, TrimSilence: true}); err == nil {
		t.Fatal("Keep source trimmed timing")
	}
}

func TestQuietAnalysisProtectsStereoTransientsAndInteriorPauses(t *testing.T) {
	for _, kind := range []string{"opposite_phase", "interior_pause", "edge_transients", "short_edges"} {
		t.Run(kind, func(t *testing.T) {
			path := testWave(t, 1, 16, 16000, 2, func(i, ch int) float64 {
				switch kind {
				case "opposite_phase":
					if ch == 0 {
						return .5
					}
					return -.5
				case "interior_pause":
					if i > 100 && i < 3900 {
						return 0
					}
					return .5
				case "edge_transients":
					if (i == 0 || i == 3999) && ch == 1 {
						return .5
					}
					return 0
				default:
					if i < 800 || i >= 3200 {
						return 0
					}
					return .5
				}
			})
			p, err := Prepare(path, QualityBalanced)
			if err != nil {
				t.Fatal(err)
			}
			if p.Analysis.AllQuiet || p.Analysis.CanTrim() {
				t.Fatalf("meaningful timing would change: %+v", p.Analysis)
			}
		})
	}
}

func TestQuietOnlyAndLoopMarkedWAVsCannotBeTrimmed(t *testing.T) {
	path := testWave(t, 1, 16, 8000, 1, func(int, int) float64 { return 0 })
	p, err := Prepare(path, QualityBalanced)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Analysis.AllQuiet || p.Analysis.CanTrim() {
		t.Fatal("quiet file was not reported")
	}
	if _, err := PrepareWithOptions(path, PrepareOptions{Quality: QualitySmall, TrimSilence: true}); err == nil {
		t.Fatal("all-quiet WAV trimmed")
	}
	path = testWave(t, 1, 16, 8000, 1, func(i, ch int) float64 {
		if i < 1000 {
			return 0
		}
		return .5
	})
	raw, _ := os.ReadFile(path)
	loop := make([]byte, 44)
	copy(loop, "smpl")
	binary.LittleEndian.PutUint32(loop[4:], 36)
	binary.LittleEndian.PutUint32(loop[36:], 1)
	raw = append(raw, loop...)
	binary.LittleEndian.PutUint32(raw[4:], uint32(len(raw)-8))
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	p, err = Prepare(path, QualityBalanced)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Analysis.LoopMarkers || p.Analysis.CanTrim() {
		t.Fatal("authored loop accepted a trim")
	}
}

func TestOriginalAuditionReadsHighPrecisionSource(t *testing.T) {
	path := testWave(t, 3, 32, 48000, 2, func(i, ch int) float64 { return .5 * math.Sin(2*math.Pi*1000*float64(i)/48000) })
	pcm, rate, err := PreviewFile(path)
	if err != nil || rate != 48000 || len(pcm) != 12000*4 {
		t.Fatalf("original audition: %d %d %v", len(pcm), rate, err)
	}
}
