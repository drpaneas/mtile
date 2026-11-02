package audioimport

import (
	"encoding/binary"
	"math"
)

// These are authoring suggestions, not GBA validity limits. Quiet edges can be
// deliberate timing, so removal always requires an explicit user choice.
const (
	QuietPeak            = 0.001 // approximately -60 dBFS, checked before 8-bit dither
	MinQuietSeconds      = 0.1
	QuietPaddingSeconds  = 0.02
	LongRecordingSeconds = 30
	LargeRecordingBytes  = 256 << 10
)

type WaveAnalysis struct {
	Info                          WaveInfo
	SourceBytes                   int
	LeadingFrames, TrailingFrames int
	TrimStart, TrimEnd            int // proposed retained interval; contains 20 ms edge padding
	AllQuiet, LoopMarkers         bool
}

func analyzeWave(info WaveInfo, samples []float64, raw []byte) WaveAnalysis {
	a := WaveAnalysis{Info: info, SourceBytes: len(raw), TrimEnd: info.Frames, LoopMarkers: waveHasLoops(raw)}
	first, last := -1, -1
	for i := 0; i < info.Frames; i++ {
		active := false
		for ch := 0; ch < info.Channels; ch++ {
			if math.Abs(samples[i*info.Channels+ch]) > QuietPeak {
				active = true
				break
			}
		}
		if active {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		a.AllQuiet = true
		return a
	}
	a.LeadingFrames, a.TrailingFrames = first, info.Frames-last-1
	// Loop metadata refers to source positions. Keep these assets untouched
	// rather than propose a trim that would change an authored loop's meaning.
	if a.LoopMarkers {
		return a
	}
	pad := int(math.Round(float64(info.Rate) * QuietPaddingSeconds))
	minimum := int(math.Ceil(float64(info.Rate) * MinQuietSeconds))
	if a.LeadingFrames >= minimum {
		a.TrimStart = max(0, first-pad)
	}
	if a.TrailingFrames >= minimum {
		a.TrimEnd = min(info.Frames, last+1+pad)
	}
	return a
}

func (a WaveAnalysis) CanTrim() bool {
	return !a.AllQuiet && !a.LoopMarkers && a.TrimStart >= 0 && a.TrimEnd > a.TrimStart && a.TrimEnd <= a.Info.Frames && (a.TrimStart > 0 || a.TrimEnd < a.Info.Frames)
}
func (a WaveAnalysis) RemovedSeconds() float64 {
	if a.Info.Rate <= 0 {
		return 0
	}
	return float64(a.TrimStart+a.Info.Frames-a.TrimEnd) / float64(a.Info.Rate)
}

// PreparedBytes predicts the WAV produced by the same frame rounding and
// header/padding rules as Prepare. It is not a prediction of Maxmod bank size.
func (a WaveAnalysis) PreparedBytes(quality Quality, trim bool) int {
	if quality == QualitySource {
		return a.SourceBytes
	}
	if a.Info.Rate <= 0 {
		return 0
	}
	frames := a.Info.Frames
	if trim && a.CanTrim() {
		frames = a.TrimEnd - a.TrimStart
	}
	rate := min(a.Info.Rate, 16000)
	if quality == QualitySmall {
		rate = min(a.Info.Rate, 8000)
	}
	frames = max(1, int(math.Round(float64(frames)*float64(rate)/float64(a.Info.Rate))))
	return 44 + frames + (frames & 1)
}

// waveHasLoops is called only after decodeWave validated the RIFF chunks.
func waveHasLoops(raw []byte) bool {
	end := uint64(binary.LittleEndian.Uint32(raw[4:])) + 8
	for off := uint64(12); off+8 <= end; {
		size := uint64(binary.LittleEndian.Uint32(raw[off+4:]))
		start := off + 8
		if start+size > end {
			return false
		}
		if string(raw[off:off+4]) == "smpl" && size >= 36 && binary.LittleEndian.Uint32(raw[start+28:]) > 0 {
			return true
		}
		off = start + size + (size & 1)
	}
	return false
}
