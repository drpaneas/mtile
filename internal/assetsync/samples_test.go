package assetsync

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func pcmWAV(t testing.TB, sampleRate, channels, bits int, data []byte) []byte {
	t.Helper()
	blockAlign := channels * bits / 8
	dataPad := len(data) & 1
	riffSize := 4 + 8 + 16 + 8 + len(data) + dataPad
	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(riffSize))
	out.WriteString("WAVEfmt ")
	_ = binary.Write(&out, binary.LittleEndian, uint32(16))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&out, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&out, binary.LittleEndian, uint32(sampleRate*blockAlign))
	_ = binary.Write(&out, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&out, binary.LittleEndian, uint16(bits))
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(data)))
	out.Write(data)
	if dataPad != 0 {
		out.WriteByte(0)
	}
	return out.Bytes()
}

func samplePCM16(values ...int16) []byte {
	out := make([]byte, len(values)*2)
	for index, value := range values {
		binary.LittleEndian.PutUint16(out[index*2:], uint16(value))
	}
	return out
}

func samplePCM24(values ...int32) []byte {
	out := make([]byte, len(values)*3)
	for index, value := range values {
		unsigned := uint32(value) & 0xffffff
		out[index*3] = byte(unsigned)
		out[index*3+1] = byte(unsigned >> 8)
		out[index*3+2] = byte(unsigned >> 16)
	}
	return out
}

func writeSampleWAV(t *testing.T, projectDir, name string, wav []byte) {
	t.Helper()
	file := filepath.Join(samplesDir(projectDir), filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, wav, 0o600); err != nil {
		t.Fatal(err)
	}
}

func validSampleLibrary() SampleLibrary {
	return SampleLibrary{Version: SampleSchemaVersion, Samples: []SampleAsset{{
		ID: 7, Name: "impact", Source: "impact.wav", TargetRate: SampleTargetRate,
		Gain: 256, Priority: 80,
	}}}
}

func TestDecodeWAVIntegerPCMDepthsAndStereo(t *testing.T) {
	tests := []struct {
		name     string
		channels int
		bits     int
		data     []byte
		want     []int8
	}{
		{"mono8", 1, 8, []byte{0, 64, 128, 255}, []int8{-128, -64, 0, 127}},
		{"stereo16", 2, 16, samplePCM16(-32768, -32768, 32767, 32767), []int8{-128, 127}},
		{"mono24", 1, 24, samplePCM24(-8388608, 0x400000, 0x7fffff), []int8{-128, 64, 127}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoded, err := decodeWAV(pcmWAV(t, SampleTargetRate, test.channels, test.bits, test.data))
			if err != nil {
				t.Fatal(err)
			}
			if got := quantizeSample(decoded.Samples, 256); !slices.Equal(got, test.want) {
				t.Fatalf("PCM = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCompileSamplesResamplesTrimsNormalizesLoopsAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	writeSampleWAV(t, dir, "tone.wav", pcmWAV(t, 8192, 1, 8, []byte{0, 128, 255, 255}))
	writeSampleWAV(t, dir, "trim.wav", pcmWAV(t, 8192, 1, 8, []byte{128, 192, 128}))
	library := SampleLibrary{Version: SampleSchemaVersion, Samples: []SampleAsset{
		{ID: 7, Name: "tone-copy", Source: "tone.wav", TargetRate: SampleTargetRate, Gain: 256, Priority: 9},
		{ID: 2, Name: "tone", Source: "tone.wav", TargetRate: SampleTargetRate, Gain: 256, Priority: 8, Loop: &SampleLoop{Start: 0, End: 3}},
		{ID: 3, Name: "trim", Source: "trim.wav", TargetRate: SampleTargetRate, Gain: 256, Priority: 7, Normalize: true, TrimSilence: true},
	}}
	definitions, pcm, err := CompileSamples(dir, library)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 3 || definitions[0].ID != 2 || definitions[1].ID != 3 || definitions[2].ID != 7 {
		t.Fatalf("definitions are not stable-ID sorted: %#v", definitions)
	}
	if got, want := pcm[:8], []int8{-128, -64, 0, 63, 127, 127, 127, 127}; !slices.Equal(got, want) {
		t.Fatalf("resampled PCM = %v, want %v", got, want)
	}
	if !definitions[0].Loop || definitions[0].LoopStart != 0 || definitions[0].LoopEnd != 6 {
		t.Fatalf("resampled loop = %#v", definitions[0])
	}
	if definitions[2].DataOffset != definitions[0].DataOffset || definitions[2].DataLength != definitions[0].DataLength {
		t.Fatalf("identical samples were not deduplicated: %#v / %#v", definitions[0], definitions[2])
	}
	trim := definitions[1]
	got := pcm[trim.DataOffset : trim.DataOffset+trim.DataLength]
	if trim.DataLength != 2 || !slices.Equal(got, []int8{127, 127}) {
		t.Fatalf("trimmed normalized PCM = %v definition=%#v", got, trim)
	}
}

func TestCompileSamplesKeepsPCMAtUnityAndStoresDefaultGain(t *testing.T) {
	dir := t.TempDir()
	writeSampleWAV(t, dir, "quiet.wav", pcmWAV(t, SampleTargetRate, 1, 8, []byte{0, 128, 255}))
	library := SampleLibrary{Version: SampleSchemaVersion, Samples: []SampleAsset{{
		ID: 1, Name: "quiet", Source: "quiet.wav", TargetRate: SampleTargetRate,
		Gain: 64, Priority: 3,
	}}}
	definitions, pcm, err := CompileSamples(dir, library)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pcm, []int8{-128, 0, 127}) || len(definitions) != 1 || definitions[0].Volume != 64 {
		t.Fatalf("compiled sample = definitions %#v PCM %v", definitions, pcm)
	}
}

func TestPreviewSampleConversionSeparatesSourceAndTargetProcessing(t *testing.T) {
	dir := t.TempDir()
	writeSampleWAV(t, dir, "preview.wav", pcmWAV(t, 8192, 1, 8, []byte{128, 192, 128}))
	asset := SampleAsset{
		ID: 4, Name: "preview", Source: "preview.wav", TargetRate: SampleTargetRate,
		Gain: 64, Priority: 2, Normalize: true, TrimSilence: true,
	}
	preview, err := PreviewSampleConversion(dir, asset)
	if err != nil {
		t.Fatal(err)
	}
	if preview.SourceRate != 8192 || preview.SourceChannels != 1 || !slices.Equal(preview.SourcePCM, []int8{0, 64, 0}) {
		t.Fatalf("source preview = rate %d channels %d PCM %v", preview.SourceRate, preview.SourceChannels, preview.SourcePCM)
	}
	if !slices.Equal(preview.SourcePlaybackPCM, []int8{0, 32, 64, 32, 0, 0}) {
		t.Fatalf("adapted source preview = %v", preview.SourcePlaybackPCM)
	}
	if !slices.Equal(preview.TargetPCM, []int8{127, 127}) || preview.Definition.Volume != 64 {
		t.Fatalf("target preview = definition %#v PCM %v", preview.Definition, preview.TargetPCM)
	}
}

func TestPreviewSampleConversionPreservesStereoSourceAndMonoTarget(t *testing.T) {
	dir := t.TempDir()
	writeSampleWAV(t, dir, "stereo.wav", pcmWAV(t, 8192, 2, 8, []byte{
		255, 0,
		128, 128,
		0, 255,
	}))
	asset := SampleAsset{
		ID: 5, Name: "stereo", Source: "stereo.wav", TargetRate: SampleTargetRate,
		Gain: 256, Priority: 3,
	}
	preview, err := PreviewSampleConversion(dir, asset)
	if err != nil {
		t.Fatal(err)
	}
	if preview.SourceChannels != 2 ||
		!slices.Equal(preview.SourceLeftPCM, []int8{127, 0, -128}) ||
		!slices.Equal(preview.SourceRightPCM, []int8{-128, 0, 127}) {
		t.Fatalf("stereo source preview = %#v", preview)
	}
	if !slices.Equal(preview.SourcePCM, []int8{-1, 0, -1}) || !slices.Equal(preview.TargetPCM, []int8{-1, -1, 0, -1, -1, -1}) {
		t.Fatalf("mono conversion preview = source %v target %v", preview.SourcePCM, preview.TargetPCM)
	}
	if !slices.Equal(preview.SourcePlaybackLeftPCM, []int8{127, 63, 0, -64, -128, -128}) ||
		!slices.Equal(preview.SourcePlaybackRightPCM, []int8{-128, -64, 0, 63, 127, 127}) {
		t.Fatalf("stereo source playback = left %v right %v", preview.SourcePlaybackLeftPCM, preview.SourcePlaybackRightPCM)
	}
}

func TestLoadAndGenerateGoIncludesCompiledSamples(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)
	writeSampleWAV(t, dir, "impact.wav", pcmWAV(t, SampleTargetRate, 1, 8, []byte{0, 128, 255}))
	library := validSampleLibrary()
	if err := SaveSamples(dir, &library); err != nil {
		t.Fatal(err)
	}
	assets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets.SampleROM) != 1 || assets.SampleROM[0].ID != 7 || !slices.Equal(assets.SamplePCMROM, []int8{-128, 0, 127}) {
		t.Fatalf("compiled loaded samples = definitions %#v PCM %v", assets.SampleROM, assets.SamplePCMROM)
	}
	source, err := GenerateGo(dir, "gameassets")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"github.com/drpaneas/mtile/runtime/audio"`,
		"var samplePCMROM", "var sampleROM", "audio.SampleDefinition",
		"mixer.Init(12)", "player.InitSamples(sampleROM[:], samplePCMROM[:])",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated sample assets missing %q", want)
		}
	}
}

func TestLoadSamplesMissingAndSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	empty, err := LoadSamples(dir)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Version != SampleSchemaVersion || len(empty.Samples) != 0 {
		t.Fatalf("missing sample library = %#v", empty)
	}
	writeSampleWAV(t, dir, "impact.wav", pcmWAV(t, SampleTargetRate, 1, 8, []byte{0, 128, 255}))
	want := validSampleLibrary()
	if err := SaveSamples(dir, &want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSamples(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Samples, want.Samples) {
		t.Fatalf("sample round trip = %#v, want %#v", got, want)
	}
}

func TestLoadSamplesRejectsUnknownFieldsTrailingDataAndEscapes(t *testing.T) {
	for name, content := range map[string]string{
		"unknown":  `{"version":1,"samples":[],"typo":true}`,
		"trailing": `{"version":1,"samples":[]} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			file := samplesPath(dir)
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadSamples(dir); err == nil {
				t.Fatal("invalid sample manifest was accepted")
			}
		})
	}

	dir := t.TempDir()
	library := validSampleLibrary()
	for _, source := range []string{"../impact.wav", "/impact.wav", `sub\impact.wav`, "sub/../impact.wav"} {
		library.Samples[0].Source = source
		if _, _, err := CompileSamples(dir, library); err == nil || !strings.Contains(err.Error(), "source") {
			t.Fatalf("source %q error = %v", source, err)
		}
	}
}

func TestCompileSamplesRejectsInvalidMetadataAndWAV(t *testing.T) {
	dir := t.TempDir()
	writeSampleWAV(t, dir, "impact.wav", pcmWAV(t, SampleTargetRate, 1, 8, []byte{0, 128, 255}))
	tests := []struct {
		name string
		edit func(*SampleLibrary)
		want string
	}{
		{"version", func(l *SampleLibrary) { l.Version++ }, "version"},
		{"id", func(l *SampleLibrary) { l.Samples[0].ID = 64 }, "outside"},
		{"name", func(l *SampleLibrary) { l.Samples[0].Name = " bad" }, "name"},
		{"rate", func(l *SampleLibrary) { l.Samples[0].TargetRate = 22050 }, "target rate"},
		{"gain", func(l *SampleLibrary) { l.Samples[0].Gain = 0 }, "gain"},
		{"priority", func(l *SampleLibrary) { l.Samples[0].Priority = 256 }, "priority"},
		{"loop", func(l *SampleLibrary) { l.Samples[0].Loop = &SampleLoop{Start: 1, End: 4} }, "loop"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			library := validSampleLibrary()
			test.edit(&library)
			if _, _, err := CompileSamples(dir, library); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	writeSampleWAV(t, dir, "bad.wav", []byte("not a wave"))
	library := validSampleLibrary()
	library.Samples[0].Source = "bad.wav"
	if _, _, err := CompileSamples(dir, library); err == nil || !strings.Contains(err.Error(), "RIFF/WAVE") {
		t.Fatalf("malformed WAV error = %v", err)
	}
}
