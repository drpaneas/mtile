package audioimport

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func testWave(t *testing.T, format, bits, rate, channels int, sample func(int, int) float64) string {
	t.Helper()
	frames := rate / 4
	raw := make([]byte, 44+frames*channels*(bits/8))
	copy(raw, "RIFF")
	binary.LittleEndian.PutUint32(raw[4:], uint32(len(raw)-8))
	copy(raw[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(raw[16:], 16)
	binary.LittleEndian.PutUint16(raw[20:], uint16(format))
	binary.LittleEndian.PutUint16(raw[22:], uint16(channels))
	binary.LittleEndian.PutUint32(raw[24:], uint32(rate))
	binary.LittleEndian.PutUint32(raw[28:], uint32(rate*channels*bits/8))
	binary.LittleEndian.PutUint16(raw[32:], uint16(channels*bits/8))
	binary.LittleEndian.PutUint16(raw[34:], uint16(bits))
	copy(raw[36:], "data")
	binary.LittleEndian.PutUint32(raw[40:], uint32(len(raw)-44))
	for i := 0; i < frames; i++ {
		for ch := 0; ch < channels; ch++ {
			v := sample(i, ch)
			b := raw[44+(i*channels+ch)*bits/8:]
			if format == 3 {
				if bits == 32 {
					binary.LittleEndian.PutUint32(b, math.Float32bits(float32(v)))
				} else {
					binary.LittleEndian.PutUint64(b, math.Float64bits(v))
				}
				continue
			}
			switch bits {
			case 8:
				b[0] = byte(int(v*127) + 128)
			case 16:
				binary.LittleEndian.PutUint16(b, uint16(int16(v*32767)))
			case 24:
				n := int32(v * 8388607)
				b[0], b[1], b[2] = byte(n), byte(n>>8), byte(n>>16)
			case 32:
				binary.LittleEndian.PutUint32(b, uint32(int32(v*2147483647)))
			}
		}
	}
	path := filepath.Join(t.TempDir(), "Jump Sound.wav")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPrepareWAVPreservesDurationAndPitchAcrossFormats(t *testing.T) {
	for _, f := range []struct{ format, bits int }{{1, 8}, {1, 16}, {1, 24}, {1, 32}, {3, 32}, {3, 64}} {
		path := testWave(t, f.format, f.bits, 48000, 2, func(i, ch int) float64 { return .6 * math.Sin(2*math.Pi*1000*float64(i)/48000) })
		p, err := Prepare(path, QualityBalanced)
		if err != nil {
			t.Fatal(f, err)
		}
		if p.Name != "jump_sound" || p.After.Rate != 16000 || p.After.Bits != 8 || p.After.Channels != 1 || math.Abs(p.Before.Seconds()-p.After.Seconds()) > .0001 {
			t.Fatalf("bad conversion: %+v", p.After)
		}
		_, samples, err := decodeWave(p.Data)
		if err != nil {
			t.Fatal(err)
		}
		crossings := 0
		for i := 1; i < len(samples); i++ {
			if samples[i-1] <= 0 && samples[i] > 0 {
				crossings++
			}
		}
		if crossings < 249 || crossings > 251 {
			t.Fatalf("pitch changed: %d crossings", crossings)
		}
		pcm, rate, err := p.PreviewPCM()
		if err != nil || rate != 16000 || len(pcm) != p.After.Frames*4 {
			t.Fatalf("preview: %d %d %v", len(pcm), rate, err)
		}
	}
}
func TestPrepareDownsamplingRejectsAliasesAndKeepsSpeechBand(t *testing.T) {
	level := func(hz float64) float64 {
		path := testWave(t, 1, 16, 48000, 1, func(i, ch int) float64 { return .6 * math.Sin(2*math.Pi*hz*float64(i)/48000) })
		p, err := Prepare(path, QualitySmall)
		if err != nil {
			t.Fatal(err)
		}
		_, s, err := decodeWave(p.Data)
		if err != nil {
			t.Fatal(err)
		}
		sum := 0.0
		for _, v := range s[100 : len(s)-100] {
			sum += v * v
		}
		return math.Sqrt(sum / float64(len(s)-200))
	}
	pass, stop := level(1000), level(12000)
	if pass < .35 || stop > .015 {
		t.Fatalf("low-pass failure pass=%g stop=%g", pass, stop)
	}
}
func TestPrepareSourceAndMalformedFiles(t *testing.T) {
	path := testWave(t, 1, 16, 8000, 1, func(int, int) float64 { return 0 })
	original, _ := os.ReadFile(path)
	p, err := Prepare(path, QualitySource)
	if err != nil || string(p.Data) != string(original) {
		t.Fatalf("source changed: %v", err)
	}
	p, err = Prepare(path, QualityBalanced)
	if err != nil || p.After.Rate != 8000 {
		t.Fatal("upsampled low-rate source", err)
	}
	for _, raw := range [][]byte{nil, []byte("RIFF"), append([]byte(nil), original[:30]...), append([]byte(nil), original...)} {
		if len(raw) == len(original) {
			binary.LittleEndian.PutUint16(raw[32:], 0)
		}
		if _, _, err := decodeWave(raw); err == nil {
			t.Fatal("malformed WAV accepted")
		}
	}
	floatPath := testWave(t, 3, 32, 8000, 1, func(int, int) float64 { return math.NaN() })
	if _, err := Prepare(floatPath, QualityBalanced); err == nil {
		t.Fatal("nonfinite sample accepted")
	}
	floatPath = testWave(t, 3, 32, 8000, 1, func(int, int) float64 { return .5 })
	if _, err := Prepare(floatPath, QualitySource); err == nil {
		t.Fatal("unsupported source passed to mmutil")
	}
	if SafeName("01 jump!") != "audio_01_jump" {
		t.Fatal("unsafe suggested name")
	}
}
