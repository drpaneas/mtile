package audioimport

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type Quality string

const (
	QualityBalanced Quality = "balanced"
	QualitySmall    Quality = "small"
	QualitySource   Quality = "source"
)

type WaveInfo struct {
	Rate, Channels, Bits, Frames int
	Float                        bool
}

func (w WaveInfo) Seconds() float64 {
	if w.Rate == 0 {
		return 0
	}
	return float64(w.Frames) / float64(w.Rate)
}

type Prepared struct {
	Name, Extension string
	Music           bool
	Data            []byte
	OriginalBytes   int
	Before, After   WaveInfo
	Quality         Quality
	Analysis        WaveAnalysis
	Trimmed         bool
}

type PrepareOptions struct {
	Quality     Quality
	TrimSilence bool
}

// Prepare does offline work only. Nothing in this path runs on the GBA.
// Tracker patterns and their instruments are passed intact to Maxmod.
func Prepare(path string, quality Quality) (Prepared, error) {
	return PrepareWithOptions(path, PrepareOptions{Quality: quality})
}

func PrepareWithOptions(path string, opts PrepareOptions) (Prepared, error) {
	return prepareWithOptions(path, opts, false)
}

func prepareWithOptions(path string, opts PrepareOptions, trimEligible bool) (Prepared, error) {
	quality := opts.Quality
	if trimEligible && quality == QualitySource {
		opts.TrimSilence = false
	}
	p := Prepared{Quality: quality, Extension: strings.ToLower(filepath.Ext(path))}
	if quality != QualityBalanced && quality != QualitySmall && quality != QualitySource {
		return p, fmt.Errorf("unknown audio quality %q", quality)
	}
	raw, err := readSource(path)
	if err != nil {
		return p, err
	}
	p.OriginalBytes = len(raw)
	p.Name = SafeName(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	switch p.Extension {
	case ".mod", ".xm", ".s3m", ".it":
		p.Music, p.Data = true, raw
		return p, nil
	case ".wav":
	default:
		return p, fmt.Errorf("choose a WAV recording or MOD/XM/S3M/IT tracker song; export other formats as WAV first")
	}
	info, samples, err := decodeWave(raw)
	if err != nil {
		return p, err
	}
	p.Before = info
	p.Analysis = analyzeWave(info, samples, raw)
	if quality == QualitySource {
		if opts.TrimSilence {
			return p, fmt.Errorf("Keep source preserves timing and metadata; choose Balanced or Smallest to trim quiet edges")
		}
		if info.Float || (info.Bits != 8 && info.Bits != 16) {
			return p, fmt.Errorf("Keep source requires 8/16-bit integer PCM WAV; choose GBA balanced to convert this file")
		}
		p.Data, p.After = raw, info
		return p, nil
	}
	rate := min(info.Rate, 16000)
	if quality == QualitySmall {
		rate = min(info.Rate, 8000)
	}
	mono := make([]float64, info.Frames)
	for i := range mono {
		for ch := 0; ch < info.Channels; ch++ {
			mono[i] += samples[i*info.Channels+ch] / float64(info.Channels)
		}
	}
	if opts.TrimSilence {
		if !p.Analysis.CanTrim() {
			if !trimEligible {
				return p, fmt.Errorf("no removable quiet edges; entirely quiet or loop-marked WAVs are never trimmed")
			}
		} else {
			mono = mono[p.Analysis.TrimStart:p.Analysis.TrimEnd]
			p.Trimmed = true
		}
	}
	mono = resampleMono(mono, info.Rate, rate)
	// Maxmod's GBA mixer uses 8-bit instrument samples. Quantize once, with
	// deterministic triangular dither, rather than storing unused precision.
	p.Data = encodeMono8(mono, rate)
	p.After = WaveInfo{Rate: rate, Channels: 1, Bits: 8, Frames: len(mono)}
	return p, nil
}

func SafeName(name string) string {
	var b strings.Builder
	for _, ch := range strings.ToLower(name) {
		if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_' {
			b.WriteRune(ch)
		} else if b.Len() > 0 {
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		return "audio"
	}
	if s[0] < 'a' || s[0] > 'z' {
		s = "audio_" + s
	}
	return s
}

func decodeWave(raw []byte) (WaveInfo, []float64, error) {
	var info WaveInfo
	bad := func() (WaveInfo, []float64, error) {
		return info, nil, fmt.Errorf("invalid or unsupported WAV; use uncompressed integer PCM (8/16/24/32-bit) or IEEE float (32/64-bit), mono or stereo")
	}
	if len(raw) < 12 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return bad()
	}
	end := uint64(binary.LittleEndian.Uint32(raw[4:])) + 8
	if end > uint64(len(raw)) || end < 12 {
		return bad()
	}
	var format, alignment int
	var data []byte
	haveFormat := false
	for off := uint64(12); off+8 <= end; {
		size := uint64(binary.LittleEndian.Uint32(raw[off+4:]))
		start := off + 8
		if start+size > end {
			return bad()
		}
		chunk := raw[start : start+size]
		switch string(raw[off : off+4]) {
		case "fmt ":
			if haveFormat || len(chunk) < 16 {
				return bad()
			}
			haveFormat = true
			format = int(binary.LittleEndian.Uint16(chunk))
			info.Channels = int(binary.LittleEndian.Uint16(chunk[2:]))
			info.Rate = int(binary.LittleEndian.Uint32(chunk[4:]))
			alignment = int(binary.LittleEndian.Uint16(chunk[12:]))
			info.Bits = int(binary.LittleEndian.Uint16(chunk[14:]))
		case "data":
			if data != nil {
				return bad()
			}
			data = chunk
		}
		off = start + size + (size & 1)
	}
	info.Float = format == 3
	validBits := format == 1 && (info.Bits == 8 || info.Bits == 16 || info.Bits == 24 || info.Bits == 32) || format == 3 && (info.Bits == 32 || info.Bits == 64)
	if !haveFormat || !validBits || info.Channels < 1 || info.Channels > 2 || info.Rate < 1000 || info.Rate > 192000 || alignment != info.Channels*(info.Bits/8) || len(data) == 0 || len(data)%alignment != 0 {
		return bad()
	}
	info.Frames = len(data) / alignment
	samples := make([]float64, info.Frames*info.Channels)
	stride := info.Bits / 8
	for i := range samples {
		b := data[i*stride:]
		var value float64
		switch {
		case info.Float && info.Bits == 32:
			value = float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
		case info.Float:
			value = math.Float64frombits(binary.LittleEndian.Uint64(b))
		case info.Bits == 8:
			value = float64(int(b[0])-128) / 128
		case info.Bits == 16:
			value = float64(int16(binary.LittleEndian.Uint16(b))) / 32768
		case info.Bits == 24:
			n := int32(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16)
			value = float64(n<<8>>8) / 8388608
		case info.Bits == 32:
			value = float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return bad()
		}
		samples[i] = max(-1, min(1, value))
	}
	return info, samples, nil
}

// A windowed sinc low-pass prevents high frequencies folding into audible
// aliases when downsampling. We never upsample assets just to meet a preset.
func resampleMono(src []float64, from, to int) []float64 {
	if from == to {
		return src
	}
	n := max(1, int(math.Round(float64(len(src))*float64(to)/float64(from))))
	dst := make([]float64, n)
	cutoff := .94 * min(1, float64(to)/float64(from))
	radius := int(math.Ceil(20 / cutoff))
	for i := range dst {
		pos := float64(i) * float64(from) / float64(to)
		center := int(pos)
		sum, weight := 0.0, 0.0
		for j := max(0, center-radius); j <= min(len(src)-1, center+radius); j++ {
			x := float64(j) - pos
			w := .5 + .5*math.Cos(math.Pi*x/float64(radius))
			t := math.Pi * x * cutoff
			if math.Abs(t) > 1e-10 {
				w *= math.Sin(t) / t
			}
			sum += src[j] * w
			weight += w
		}
		if math.Abs(weight) > 1e-10 {
			dst[i] = max(-1, min(1, sum/weight))
		}
	}
	return dst
}

func encodeMono8(samples []float64, rate int) []byte {
	size := len(samples)
	out := make([]byte, 44+size+(size&1))
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1)
	binary.LittleEndian.PutUint16(out[22:], 1)
	binary.LittleEndian.PutUint32(out[24:], uint32(rate))
	binary.LittleEndian.PutUint32(out[28:], uint32(rate))
	binary.LittleEndian.PutUint16(out[32:], 1)
	binary.LittleEndian.PutUint16(out[34:], 8)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(size))
	seed := uint32(1)
	noise := func() float64 { seed = seed*1664525 + 1013904223; return float64(seed>>8) / 16777216 }
	for i, value := range samples {
		dither := noise() - noise()
		q := math.Round(value*128 + dither)
		out[44+i] = byte(max(-128, min(127, q)) + 128)
	}
	return out
}

// PreviewPCM renders the prepared WAV to the editor's signed-16 stereo monitor.
// It is an asset audition, not a simulation of concurrent GBA mixer workload.
func (p Prepared) PreviewPCM() ([]byte, int, error) {
	info, samples, err := decodeWave(p.Data)
	if err != nil {
		return nil, 0, err
	}
	frames := min(info.Frames, info.Rate*15)
	out := make([]byte, frames*4)
	for i := 0; i < frames; i++ {
		for ch := 0; ch < 2; ch++ {
			value := samples[i*info.Channels+min(ch, info.Channels-1)]
			binary.LittleEndian.PutUint16(out[4*i+2*ch:], uint16(int16(max(-32768, min(32767, math.Round(value*32768))))))
		}
	}
	return out, info.Rate, nil
}

// PreviewFile auditions the external source without changing it, including
// high-precision WAV inputs that cannot be passed directly to mmutil.
func PreviewFile(path string) ([]byte, int, error) {
	raw, err := readSource(path)
	if err != nil {
		return nil, 0, err
	}
	return (Prepared{Data: raw}).PreviewPCM()
}

// ImportPrepared uses exactly the CLI's validated, atomic bank publisher.
func ImportPrepared(project string, p Prepared, name, converter string) (Result, error) {
	if !identifier.MatchString(name) {
		return Result{}, fmt.Errorf("name must start with a letter and contain only lowercase letters, digits or underscores")
	}
	work, err := os.MkdirTemp("", "mtile-prepared-audio-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(work)
	path := filepath.Join(work, name+p.Extension)
	if err := os.WriteFile(path, p.Data, 0o644); err != nil {
		return Result{}, err
	}
	return Import(project, []string{path}, converter)
}
