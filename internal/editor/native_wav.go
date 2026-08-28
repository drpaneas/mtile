package editor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
)

// NativePreviewAudioStats describes the fully rendered desktop PSG monitor
// signal. It is intentionally derived from the same samples used by the
// editor's Play button and by RenderNativeMusicPreviewWAV.
type NativePreviewAudioStats struct {
	Frames   int
	Peak     int
	RMS      float64
	Duration float64
}

// AudioStats makes silence an explicit state rather than successfully opening
// an audio player on an all-zero song.
func (p *NativeMusicPreview) AudioStats() NativePreviewAudioStats {
	if p == nil || len(p.Samples) == 0 {
		return NativePreviewAudioStats{}
	}
	peak := 0
	energy := float64(0)
	channels := [][]int8{p.Samples}
	if len(p.LeftSamples) == len(p.Samples) && len(p.RightSamples) == len(p.Samples) {
		channels = [][]int8{p.LeftSamples, p.RightSamples}
	}
	for _, samples := range channels {
		for _, sample := range samples {
			value := int(sample)
			magnitude := value
			if magnitude < 0 {
				magnitude = -magnitude
			}
			peak = max(peak, magnitude)
			energy += float64(value * value)
		}
	}
	return NativePreviewAudioStats{
		Frames:   len(p.Samples),
		Peak:     peak,
		RMS:      math.Sqrt(energy / float64(len(p.Samples)*len(channels))),
		Duration: float64(len(p.Samples)) / NativePreviewSampleRate,
	}
}

// RenderNativeMusicPreviewWAV encodes the exact mono PSG preview signal as
// signed 16-bit PCM. The WAV is a deterministic, independently playable proof
// of what the editor sends to its host audio device before final resampling.
func RenderNativeMusicPreviewWAV(preview *NativeMusicPreview) ([]byte, error) {
	if preview == nil || len(preview.Samples) == 0 {
		return nil, errors.New("native music preview has no samples")
	}
	const (
		channels      = 1
		bitsPerSample = 16
		bytesPerFrame = channels * bitsPerSample / 8
	)
	dataSize := len(preview.Samples) * bytesPerFrame
	var output bytes.Buffer
	output.Grow(44 + dataSize)
	output.WriteString("RIFF")
	_ = binary.Write(&output, binary.LittleEndian, uint32(36+dataSize))
	output.WriteString("WAVEfmt ")
	_ = binary.Write(&output, binary.LittleEndian, uint32(16))
	_ = binary.Write(&output, binary.LittleEndian, uint16(1))
	_ = binary.Write(&output, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&output, binary.LittleEndian, uint32(NativePreviewSampleRate))
	_ = binary.Write(&output, binary.LittleEndian, uint32(NativePreviewSampleRate*bytesPerFrame))
	_ = binary.Write(&output, binary.LittleEndian, uint16(bytesPerFrame))
	_ = binary.Write(&output, binary.LittleEndian, uint16(bitsPerSample))
	output.WriteString("data")
	_ = binary.Write(&output, binary.LittleEndian, uint32(dataSize))
	for _, sample := range preview.Samples {
		_ = binary.Write(&output, binary.LittleEndian, int16(sample)<<8)
	}
	return output.Bytes(), nil
}
