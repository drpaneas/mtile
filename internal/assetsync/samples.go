package assetsync

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const (
	SampleSchemaVersion  = 1
	MaxSampleAssets      = 64
	MaxSampleNameLength  = 64
	MaxSampleSourcePath  = 256
	SampleTargetRate     = 16384
	MaxSampleFrames      = 262144
	MaxSampleROMBytes    = 1 << 20
	maxSampleWAVDataSize = 16 << 20
)

// SampleLibrary is the versioned assets/audio/samples/manifest.json document.
// Source paths are portable slash-separated paths relative to that directory.
type SampleLibrary struct {
	Version int           `json:"version"`
	Samples []SampleAsset `json:"samples"`
}

type SampleAsset struct {
	ID          int         `json:"id"`
	Name        string      `json:"name"`
	Source      string      `json:"source"`
	TargetRate  int         `json:"targetRate"`
	Gain        int         `json:"gain"`
	Priority    int         `json:"priority"`
	Normalize   bool        `json:"normalize,omitempty"`
	TrimSilence bool        `json:"trimSilence,omitempty"`
	Loop        *SampleLoop `json:"loop,omitempty"`
}

// SampleLoop uses a half-open range of source WAV frames. CompileSamples
// adjusts it for silence trimming and target-rate resampling.
type SampleLoop struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// TargetSample is the compact build-time record emitted beside one shared
// signed 8-bit PCM array. Loop points are relative to this sample's data.
type TargetSample struct {
	DataOffset uint32
	DataLength uint32
	LoopStart  uint32
	LoopEnd    uint32
	Volume     uint16
	ID         uint8
	Priority   uint8
	Loop       bool
}

// SampleConversionPreview exposes host-editor data without making the editor
// duplicate WAV decoding or resampling. SourcePCM and SourcePlaybackPCM retain
// the deterministic mono downmix used by version-1 builds. The channel-specific
// fields preserve stereo only for source waveform display and host playback.
// TargetPCM is exactly the ROM byte range produced by CompileSamples before
// runtime volume is applied.
type SampleConversionPreview struct {
	SourceRate             int
	SourceChannels         int
	SourcePCM              []int8
	SourceLeftPCM          []int8
	SourceRightPCM         []int8
	SourcePlaybackPCM      []int8
	SourcePlaybackLeftPCM  []int8
	SourcePlaybackRightPCM []int8
	TargetPCM              []int8
	Definition             TargetSample
}

type decodedWAV struct {
	SampleRate int
	Channels   int
	Samples    []int32    // deterministic mono downmix in signed 24-bit domain
	ChannelPCM [2][]int32 // source channels; index 1 is nil for mono
}

func samplesDir(projectDir string) string {
	return filepath.Join(projectDir, "assets", "audio", "samples")
}

func samplesPath(projectDir string) string {
	return filepath.Join(samplesDir(projectDir), "manifest.json")
}

func LoadSamples(projectDir string) (*SampleLibrary, error) {
	file, err := os.Open(samplesPath(projectDir))
	if errors.Is(err, os.ErrNotExist) {
		return &SampleLibrary{Version: SampleSchemaVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sample manifest: %w", err)
	}
	defer func() { _ = file.Close() }()

	var library SampleLibrary
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&library); err != nil {
		return nil, fmt.Errorf("decode sample manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return nil, fmt.Errorf("decode sample manifest: trailing data: %w", err)
	}
	if _, _, err := CompileSamples(projectDir, library); err != nil {
		return nil, err
	}
	return &library, nil
}

func SaveSamples(projectDir string, library *SampleLibrary) error {
	if library == nil {
		return errors.New("sample library is required")
	}
	if _, _, err := CompileSamples(projectDir, *library); err != nil {
		return err
	}
	return writeJSON(samplesPath(projectDir), library)
}

func validSampleSource(source string) bool {
	if source == "" || len(source) > MaxSampleSourcePath || strings.Contains(source, `\`) || path.IsAbs(source) {
		return false
	}
	clean := path.Clean(source)
	return clean == source && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func readSampleSource(projectDir, source string) ([]byte, error) {
	if !validSampleSource(source) {
		return nil, fmt.Errorf("sample source %q must be a clean relative slash path within assets/audio/samples", source)
	}
	raw, err := os.ReadFile(filepath.Join(samplesDir(projectDir), filepath.FromSlash(source)))
	if err != nil {
		return nil, fmt.Errorf("read sample source %q: %w", source, err)
	}
	return raw, nil
}

func decodePCM16(raw []byte) int32 {
	return int32(int16(binary.LittleEndian.Uint16(raw))) << 8
}

func decodePCM24(raw []byte) int32 {
	value := int32(raw[0]) | int32(raw[1])<<8 | int32(raw[2])<<16
	if value&0x800000 != 0 {
		value |= ^int32(0xffffff)
	}
	return value
}

func decodeWAV(raw []byte) (decodedWAV, error) {
	if len(raw) < 12 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return decodedWAV{}, errors.New("WAV requires a RIFF/WAVE header")
	}
	riffEnd64 := uint64(binary.LittleEndian.Uint32(raw[4:8])) + 8
	if riffEnd64 != uint64(len(raw)) {
		return decodedWAV{}, fmt.Errorf("WAV RIFF length is %d bytes, file is %d", riffEnd64, len(raw))
	}
	riffEnd := int(riffEnd64)
	var (
		formatSeen bool
		dataSeen   bool
		channels   int
		sampleRate int
		bits       int
		blockAlign int
		data       []byte
	)
	for offset := 12; offset < riffEnd; {
		if offset+8 > riffEnd {
			return decodedWAV{}, errors.New("WAV has a truncated chunk header")
		}
		chunkID := string(raw[offset : offset+4])
		size64 := uint64(binary.LittleEndian.Uint32(raw[offset+4 : offset+8]))
		start := offset + 8
		end64 := uint64(start) + size64
		if end64 > uint64(riffEnd) {
			return decodedWAV{}, fmt.Errorf("WAV %q chunk exceeds RIFF data", chunkID)
		}
		end := int(end64)
		switch chunkID {
		case "fmt ":
			if formatSeen {
				return decodedWAV{}, errors.New("WAV has multiple fmt chunks")
			}
			if size64 < 16 {
				return decodedWAV{}, errors.New("WAV fmt chunk is shorter than 16 bytes")
			}
			formatSeen = true
			format := binary.LittleEndian.Uint16(raw[start : start+2])
			channels = int(binary.LittleEndian.Uint16(raw[start+2 : start+4]))
			sampleRate64 := uint64(binary.LittleEndian.Uint32(raw[start+4 : start+8]))
			byteRate64 := uint64(binary.LittleEndian.Uint32(raw[start+8 : start+12]))
			blockAlign = int(binary.LittleEndian.Uint16(raw[start+12 : start+14]))
			bits = int(binary.LittleEndian.Uint16(raw[start+14 : start+16]))
			if format != 1 {
				return decodedWAV{}, fmt.Errorf("WAV format %d is unsupported (want integer PCM format 1)", format)
			}
			if channels != 1 && channels != 2 {
				return decodedWAV{}, fmt.Errorf("WAV has %d channels (want mono or stereo)", channels)
			}
			if bits != 8 && bits != 16 && bits != 24 {
				return decodedWAV{}, fmt.Errorf("WAV has %d-bit samples (want 8, 16, or 24)", bits)
			}
			if sampleRate64 == 0 || sampleRate64 > 192000 {
				return decodedWAV{}, fmt.Errorf("WAV sample rate %d is outside 1..192000", sampleRate64)
			}
			sampleRate = int(sampleRate64)
			wantAlign := channels * (bits / 8)
			if blockAlign != wantAlign || byteRate64 != sampleRate64*uint64(wantAlign) {
				return decodedWAV{}, errors.New("WAV fmt block alignment or byte rate is inconsistent")
			}
		case "data":
			if dataSeen {
				return decodedWAV{}, errors.New("WAV has multiple data chunks")
			}
			if size64 > maxSampleWAVDataSize {
				return decodedWAV{}, fmt.Errorf("WAV data is %d bytes (maximum %d)", size64, maxSampleWAVDataSize)
			}
			dataSeen = true
			data = raw[start:end]
		}
		offset = end
		if size64&1 != 0 {
			offset++
			if offset > riffEnd {
				return decodedWAV{}, fmt.Errorf("WAV %q chunk is missing its pad byte", chunkID)
			}
		}
	}
	if !formatSeen || !dataSeen {
		return decodedWAV{}, errors.New("WAV requires one fmt chunk and one data chunk")
	}
	if len(data) == 0 || len(data)%blockAlign != 0 {
		return decodedWAV{}, errors.New("WAV data is empty or not frame-aligned")
	}
	frames := len(data) / blockAlign
	if frames > MaxSampleFrames*12 {
		return decodedWAV{}, fmt.Errorf("WAV has %d source frames (conversion limit %d)", frames, MaxSampleFrames*12)
	}
	bytesPerSample := bits / 8
	mono := make([]int32, frames)
	var channelPCM [2][]int32
	for channel := 0; channel < channels; channel++ {
		channelPCM[channel] = make([]int32, frames)
	}
	for frame := 0; frame < frames; frame++ {
		var total int64
		for channel := 0; channel < channels; channel++ {
			offset := frame*blockAlign + channel*bytesPerSample
			var sample int32
			switch bits {
			case 8:
				sample = (int32(data[offset]) - 128) << 16
			case 16:
				sample = decodePCM16(data[offset : offset+2])
			case 24:
				sample = decodePCM24(data[offset : offset+3])
			}
			channelPCM[channel][frame] = sample
			total += int64(sample)
		}
		mono[frame] = int32(total / int64(channels))
	}
	return decodedWAV{SampleRate: sampleRate, Channels: channels, Samples: mono, ChannelPCM: channelPCM}, nil
}

func trimSampleSilence(samples []int32) (start, end int) {
	start, end = 0, len(samples)
	for start < end && int64(samples[start]) > -65536 && int64(samples[start]) < 65536 {
		start++
	}
	for end > start && int64(samples[end-1]) > -65536 && int64(samples[end-1]) < 65536 {
		end--
	}
	return start, end
}

func normalizeSample(samples []int32) {
	var peak int64
	for _, sample := range samples {
		value := int64(sample)
		if value < 0 {
			value = -value
		}
		if value > peak {
			peak = value
		}
	}
	if peak == 0 {
		return
	}
	for index, sample := range samples {
		value := int64(sample) * 8388607 / peak
		if value < -8388608 {
			value = -8388608
		} else if value > 8388607 {
			value = 8388607
		}
		samples[index] = int32(value)
	}
}

func targetFrameCount(sourceFrames, sourceRate, targetRate int) (int, error) {
	count64 := (uint64(sourceFrames)*uint64(targetRate) + uint64(sourceRate)/2) / uint64(sourceRate)
	if count64 == 0 {
		count64 = 1
	}
	if count64 > MaxSampleFrames {
		return 0, fmt.Errorf("converted sample has %d frames (maximum %d)", count64, MaxSampleFrames)
	}
	return int(count64), nil
}

func resampleLinear(samples []int32, sourceRate, targetRate int) ([]int32, error) {
	count, err := targetFrameCount(len(samples), sourceRate, targetRate)
	if err != nil {
		return nil, err
	}
	out := make([]int32, count)
	for index := range out {
		position := uint64(index) * uint64(sourceRate)
		left := int(position / uint64(targetRate))
		fraction := int64(position % uint64(targetRate))
		if left >= len(samples)-1 {
			out[index] = samples[len(samples)-1]
			continue
		}
		value := int64(samples[left])*int64(targetRate) +
			(int64(samples[left+1])-int64(samples[left]))*fraction
		out[index] = int32(value / int64(targetRate))
	}
	return out, nil
}

func quantizeSample(samples []int32, gain int) []int8 {
	out := make([]int8, len(samples))
	for index, sample := range samples {
		value := int64(sample) * int64(gain) / 256
		if value < -8388608 {
			value = -8388608
		} else if value > 8388607 {
			value = 8388607
		}
		value >>= 16
		if value < -128 {
			value = -128
		} else if value > 127 {
			value = 127
		}
		out[index] = int8(value)
	}
	return out
}

func resampleBoundary(frame, sourceRate, targetRate int) int {
	return int((uint64(frame)*uint64(targetRate) + uint64(sourceRate)/2) / uint64(sourceRate))
}

func compileSample(projectDir string, asset SampleAsset) ([]int8, uint32, uint32, error) {
	raw, err := readSampleSource(projectDir, asset.Source)
	if err != nil {
		return nil, 0, 0, err
	}
	decoded, err := decodeWAV(raw)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("sample %d source %q: %w", asset.ID, asset.Source, err)
	}
	loopStart, loopEnd := 0, 0
	if asset.Loop != nil {
		if asset.Loop.Start < 0 || asset.Loop.Start >= asset.Loop.End || asset.Loop.End > len(decoded.Samples) {
			return nil, 0, 0, fmt.Errorf("sample %d loop [%d,%d) is outside %d source frames", asset.ID, asset.Loop.Start, asset.Loop.End, len(decoded.Samples))
		}
		loopStart, loopEnd = asset.Loop.Start, asset.Loop.End
	}
	start, end := 0, len(decoded.Samples)
	if asset.TrimSilence {
		start, end = trimSampleSilence(decoded.Samples)
		if start == end {
			return nil, 0, 0, fmt.Errorf("sample %d is silent after trimming", asset.ID)
		}
		if asset.Loop != nil && (loopStart < start || loopEnd > end) {
			return nil, 0, 0, fmt.Errorf("sample %d loop is outside the retained non-silent source range [%d,%d)", asset.ID, start, end)
		}
	}
	decoded.Samples = slices.Clone(decoded.Samples[start:end])
	if asset.Loop != nil {
		loopStart -= start
		loopEnd -= start
	}
	if asset.Normalize {
		normalizeSample(decoded.Samples)
	}
	resampled, err := resampleLinear(decoded.Samples, decoded.SampleRate, asset.TargetRate)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("sample %d: %w", asset.ID, err)
	}
	// Keep converted PCM at unity. Gain remains runtime metadata so a call can
	// use the authored default or override it without duplicating sample bytes.
	pcm := quantizeSample(resampled, 256)
	if asset.Loop == nil {
		return pcm, 0, 0, nil
	}
	targetStart := resampleBoundary(loopStart, decoded.SampleRate, asset.TargetRate)
	targetEnd := resampleBoundary(loopEnd, decoded.SampleRate, asset.TargetRate)
	if targetEnd > len(pcm) {
		targetEnd = len(pcm)
	}
	if targetStart < 0 || targetStart >= targetEnd || targetEnd-targetStart < 2 {
		return nil, 0, 0, fmt.Errorf("sample %d loop becomes [%d,%d) after conversion (want at least two target frames)", asset.ID, targetStart, targetEnd)
	}
	return pcm, uint32(targetStart), uint32(targetEnd), nil
}

// CompileSamples validates metadata, decodes source WAV files, and packs
// deterministic target PCM sorted by stable ID. Identical converted byte
// sequences share one ROM range.
func CompileSamples(projectDir string, library SampleLibrary) ([]TargetSample, []int8, error) {
	if library.Version != SampleSchemaVersion {
		return nil, nil, fmt.Errorf("sample manifest version %d is unsupported (want %d)", library.Version, SampleSchemaVersion)
	}
	if len(library.Samples) > MaxSampleAssets {
		return nil, nil, fmt.Errorf("sample manifest has %d samples (maximum %d)", len(library.Samples), MaxSampleAssets)
	}
	authored := slices.Clone(library.Samples)
	sort.Slice(authored, func(i, j int) bool { return authored[i].ID < authored[j].ID })
	definitions := make([]TargetSample, 0, len(authored))
	pcmROM := make([]int8, 0)
	seenIDs := uint64(0)
	seenNames := make(map[string]struct{}, len(authored))
	deduplicated := make(map[string]uint32, len(authored))
	for _, asset := range authored {
		if asset.ID < 0 || asset.ID >= MaxSampleAssets {
			return nil, nil, fmt.Errorf("sample id %d is outside 0..%d", asset.ID, MaxSampleAssets-1)
		}
		bit := uint64(1) << asset.ID
		if seenIDs&bit != 0 {
			return nil, nil, fmt.Errorf("sample id %d is duplicated", asset.ID)
		}
		seenIDs |= bit
		if !validSFXName(asset.Name) {
			return nil, nil, fmt.Errorf("sample %d name must be 1..%d non-control characters without edge whitespace", asset.ID, MaxSampleNameLength)
		}
		if _, exists := seenNames[asset.Name]; exists {
			return nil, nil, fmt.Errorf("sample name %q is duplicated", asset.Name)
		}
		seenNames[asset.Name] = struct{}{}
		if !validSampleSource(asset.Source) {
			return nil, nil, fmt.Errorf("sample %d source %q must stay within assets/audio/samples", asset.ID, asset.Source)
		}
		if asset.TargetRate != SampleTargetRate {
			return nil, nil, fmt.Errorf("sample %d target rate %d is unsupported (want %d)", asset.ID, asset.TargetRate, SampleTargetRate)
		}
		if asset.Gain < 1 || asset.Gain > 256 {
			return nil, nil, fmt.Errorf("sample %d gain %d is outside 1..256", asset.ID, asset.Gain)
		}
		if asset.Priority < 0 || asset.Priority > math.MaxUint8 {
			return nil, nil, fmt.Errorf("sample %d priority %d is outside 0..255", asset.ID, asset.Priority)
		}
		pcm, loopStart, loopEnd, err := compileSample(projectDir, asset)
		if err != nil {
			return nil, nil, err
		}
		keyBytes := make([]byte, len(pcm))
		for index, sample := range pcm {
			keyBytes[index] = byte(sample)
		}
		key := string(keyBytes)
		offset, exists := deduplicated[key]
		if !exists {
			if len(pcmROM)+len(pcm) > MaxSampleROMBytes {
				return nil, nil, fmt.Errorf("compiled sample PCM exceeds %d-byte version-1 ROM budget", MaxSampleROMBytes)
			}
			offset = uint32(len(pcmROM))
			pcmROM = append(pcmROM, pcm...)
			deduplicated[key] = offset
		}
		definitions = append(definitions, TargetSample{
			DataOffset: offset, DataLength: uint32(len(pcm)), LoopStart: loopStart, LoopEnd: loopEnd,
			Volume: uint16(asset.Gain), ID: uint8(asset.ID), Priority: uint8(asset.Priority), Loop: asset.Loop != nil,
		})
	}
	return definitions, pcmROM, nil
}

// PreviewSampleConversion validates and converts one authored sample using
// the same routines as the build. The source playback adaptation intentionally
// omits trim, normalization, gain, and loop processing so A/B preview preserves
// the distinction between source material and target conversion.
func PreviewSampleConversion(projectDir string, asset SampleAsset) (*SampleConversionPreview, error) {
	definitions, pcm, err := CompileSamples(projectDir, SampleLibrary{
		Version: SampleSchemaVersion,
		Samples: []SampleAsset{asset},
	})
	if err != nil {
		return nil, err
	}
	raw, err := readSampleSource(projectDir, asset.Source)
	if err != nil {
		return nil, err
	}
	decoded, err := decodeWAV(raw)
	if err != nil {
		return nil, fmt.Errorf("sample %d source %q: %w", asset.ID, asset.Source, err)
	}
	sourcePlayback, err := resampleLinear(decoded.Samples, decoded.SampleRate, SampleTargetRate)
	if err != nil {
		return nil, fmt.Errorf("sample %d source preview: %w", asset.ID, err)
	}
	leftPlayback, err := resampleLinear(decoded.ChannelPCM[0], decoded.SampleRate, SampleTargetRate)
	if err != nil {
		return nil, fmt.Errorf("sample %d left source preview: %w", asset.ID, err)
	}
	var sourceRight, rightPlayback []int8
	if decoded.Channels == 2 {
		adapted, resampleErr := resampleLinear(decoded.ChannelPCM[1], decoded.SampleRate, SampleTargetRate)
		if resampleErr != nil {
			return nil, fmt.Errorf("sample %d right source preview: %w", asset.ID, resampleErr)
		}
		sourceRight = quantizeSample(decoded.ChannelPCM[1], 256)
		rightPlayback = quantizeSample(adapted, 256)
	}
	return &SampleConversionPreview{
		SourceRate:             decoded.SampleRate,
		SourceChannels:         decoded.Channels,
		SourcePCM:              quantizeSample(decoded.Samples, 256),
		SourceLeftPCM:          quantizeSample(decoded.ChannelPCM[0], 256),
		SourceRightPCM:         sourceRight,
		SourcePlaybackPCM:      quantizeSample(sourcePlayback, 256),
		SourcePlaybackLeftPCM:  quantizeSample(leftPlayback, 256),
		SourcePlaybackRightPCM: rightPlayback,
		TargetPCM:              slices.Clone(pcm),
		Definition:             definitions[0],
	}, nil
}
