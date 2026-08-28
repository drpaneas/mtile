package assetsync

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func FuzzDecodeWAVBoundaries(f *testing.F) {
	f.Add(pcmWAV(f, 16384, 1, 8, []byte{0, 128, 255, 64}))
	f.Add([]byte("RIFF\x04\x00\x00\x00WAVE"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, raw []byte) {
		decoded, err := decodeWAV(raw)
		if err != nil {
			return
		}
		if decoded.SampleRate < 1 || decoded.SampleRate > 192000 {
			t.Fatalf("accepted sample rate %d", decoded.SampleRate)
		}
		if decoded.Channels != 1 && decoded.Channels != 2 {
			t.Fatalf("accepted channel count %d", decoded.Channels)
		}
		if len(decoded.Samples) == 0 || len(decoded.ChannelPCM[0]) != len(decoded.Samples) {
			t.Fatalf("accepted inconsistent WAV lengths: mono=%d left=%d", len(decoded.Samples), len(decoded.ChannelPCM[0]))
		}
		if decoded.Channels == 2 && len(decoded.ChannelPCM[1]) != len(decoded.Samples) {
			t.Fatalf("accepted inconsistent right channel length %d", len(decoded.ChannelPCM[1]))
		}
	})
}

func FuzzSFXJSONValidationRoundTrip(f *testing.F) {
	f.Add([]byte(`{"version":1,"sfx":[{"id":0,"name":"seed","speed":1,"steps":[{"pitch":33,"waveform":"triangle","volume":7,"effect":"none"}]}]}`))
	f.Add([]byte(`{"version":2,"sfx":[]}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "assets", "audio", "sfx.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		library, err := LoadSFX(dir)
		if err != nil {
			return
		}
		if _, _, err := CompileSFX(*library); err != nil {
			t.Fatalf("LoadSFX accepted a library CompileSFX rejected: %v", err)
		}
		roundTrip := t.TempDir()
		if err := SaveSFX(roundTrip, library); err != nil {
			t.Fatalf("save accepted library: %v", err)
		}
		reloaded, err := LoadSFX(roundTrip)
		if err != nil {
			t.Fatalf("reload accepted library: %v", err)
		}
		if !reflect.DeepEqual(*reloaded, *library) {
			t.Fatalf("SFX round trip changed library: got %#v want %#v", *reloaded, *library)
		}
	})
}

func FuzzSFXLoopBounds(f *testing.F) {
	f.Add(4, 0, 4)
	f.Add(4, 3, 3)
	f.Add(1, -1, 1)
	f.Fuzz(func(t *testing.T, rawCount, start, end int) {
		count := 1 + int(uint(rawCount)%MaxSFXSteps)
		steps := make([]SFXStep, count)
		for i := range steps {
			steps[i] = SFXStep{Pitch: 33, Waveform: "triangle", Volume: 7, Effect: "none"}
		}
		library := SFXLibrary{Version: SFXSchemaVersion, SFX: []SFXAsset{{
			ID: 0, Name: "loop", Speed: 1, Loop: &SFXLoop{Start: start, End: end}, Steps: steps,
		}}}
		_, _, err := CompileSFX(library)
		valid := start >= 0 && start < end && end <= count
		if valid && err != nil {
			t.Fatalf("valid loop [%d,%d) over %d steps rejected: %v", start, end, count, err)
		}
		if !valid && err == nil {
			t.Fatalf("invalid loop [%d,%d) over %d steps accepted", start, end, count)
		}
	})
}

func FuzzSampleLoopBounds(f *testing.F) {
	f.Add(0, 4)
	f.Add(3, 3)
	f.Add(-1, 2)
	f.Fuzz(func(t *testing.T, start, end int) {
		dir := t.TempDir()
		sampleDir := samplesDir(dir)
		if err := os.MkdirAll(sampleDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sampleDir, "seed.wav"), pcmWAV(t, SampleTargetRate, 1, 8, []byte{0, 64, 128, 255}), 0o644); err != nil {
			t.Fatal(err)
		}
		library := SampleLibrary{Version: SampleSchemaVersion, Samples: []SampleAsset{{
			ID: 0, Name: "seed", Source: "seed.wav", TargetRate: SampleTargetRate,
			Gain: 256, Priority: 64, Loop: &SampleLoop{Start: start, End: end},
		}}}
		_, _, err := CompileSamples(dir, library)
		valid := start >= 0 && end-start >= 2 && end <= 4
		if valid && err != nil {
			t.Fatalf("valid sample loop [%d,%d) rejected: %v", start, end, err)
		}
		if !valid && err == nil {
			t.Fatalf("invalid sample loop [%d,%d) accepted", start, end)
		}
	})
}
