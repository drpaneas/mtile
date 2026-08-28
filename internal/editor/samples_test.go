package editor

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func editorPCM8WAV(t *testing.T, sampleRate, channels int, samples []byte) []byte {
	t.Helper()
	padding := len(samples) & 1
	blockAlign := channels
	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(4+8+16+8+len(samples)+padding))
	out.WriteString("WAVEfmt ")
	_ = binary.Write(&out, binary.LittleEndian, uint32(16))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&out, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&out, binary.LittleEndian, uint32(sampleRate*blockAlign))
	_ = binary.Write(&out, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&out, binary.LittleEndian, uint16(8))
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(samples)))
	out.Write(samples)
	if padding != 0 {
		out.WriteByte(0)
	}
	return out.Bytes()
}

func TestModelImportsEditsPreviewsPersistsAndDeletesSamples(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Samples.Version != assetsync.SampleSchemaVersion || len(m.Samples.Samples) != 0 {
		t.Fatalf("initial samples = %#v", m.Samples)
	}
	index, err := m.ImportSampleWAV("Impact Hit.wav", editorPCM8WAV(t, 8192, 1, []byte{128, 192, 128}))
	if err != nil {
		t.Fatal(err)
	}
	sample, ok := m.SampleAsset(index)
	if !ok || sample.ID != 0 || sample.Name != "Impact-Hit" || sample.Source != "Impact-Hit.wav" || sample.Gain != 256 {
		t.Fatalf("imported sample = %#v ok=%t", sample, ok)
	}
	sourcePath := filepath.Join(dir, "assets", "audio", "samples", sample.Source)
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("copied source: %v", err)
	}
	sample.Gain = 128
	sample.Priority = 90
	sample.Normalize = true
	sample.TrimSilence = true
	if err := m.UpdateSample(index, sample); err != nil {
		t.Fatal(err)
	}
	preview, err := RenderSamplePreview(dir, sample)
	if err != nil {
		t.Fatal(err)
	}
	if preview.SourceRate != 8192 || !slices.Equal(preview.SourceSamples, []int8{0, 64, 0}) ||
		preview.SourceChannels != 1 || len(preview.SourcePlayback) != 6 ||
		!slices.Equal(preview.SourceLeftSamples, preview.SourceSamples) || len(preview.SourceRightSamples) != 0 ||
		!slices.Equal(preview.TargetROM, []int8{127, 127}) || len(preview.TargetSamples) != 2 {
		t.Fatalf("sample preview = %#v", preview)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Samples.Samples) != 1 || reloaded.Samples.Samples[0].Gain != 128 || !reloaded.Samples.Samples[0].Normalize {
		t.Fatalf("reloaded samples = %#v", reloaded.Samples)
	}
	if err := reloaded.DeleteSample(0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("delete should preserve source WAV: %v", err)
	}
}

func TestRenderSamplePreviewPreservesStereoSourceOnly(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	index, err := m.ImportSampleWAV("stereo.wav", editorPCM8WAV(t, 8192, 2, []byte{
		255, 0,
		128, 128,
		0, 255,
	}))
	if err != nil {
		t.Fatal(err)
	}
	sample, _ := m.SampleAsset(index)
	preview, err := RenderSamplePreview(dir, sample)
	if err != nil {
		t.Fatal(err)
	}
	if preview.SourceChannels != 2 ||
		!slices.Equal(preview.SourceLeftSamples, []int8{127, 0, -128}) ||
		!slices.Equal(preview.SourceRightSamples, []int8{-128, 0, 127}) ||
		len(preview.SourcePlaybackLeft) != 6 || len(preview.SourcePlaybackRight) != 6 {
		t.Fatalf("stereo source preview = %#v", preview)
	}
	if !slices.Equal(preview.TargetROM, []int8{-1, -1, 0, -1, -1, -1}) ||
		!slices.Equal(preview.TargetSamples, preview.TargetROM) {
		t.Fatalf("mono target preview = ROM %v mixer %v", preview.TargetROM, preview.TargetSamples)
	}
}

func TestModelRejectsInvalidSampleImportAndMetadataTransactionally(t *testing.T) {
	dir := editorFixture(t)
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ImportSampleWAV("bad.mp3", []byte("not audio")); err == nil {
		t.Fatal("non-WAV import succeeded")
	}
	if _, err := m.ImportSampleWAV("bad.wav", []byte("not audio")); err == nil {
		t.Fatal("malformed WAV import succeeded")
	}
	if len(m.Samples.Samples) != 0 {
		t.Fatalf("failed imports changed manifest: %#v", m.Samples)
	}
	index, err := m.ImportSampleWAV("good.wav", editorPCM8WAV(t, assetsync.SampleTargetRate, 1, []byte{0, 255}))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := m.SampleAsset(index)
	invalid := before
	invalid.Gain = 999
	if err := m.UpdateSample(index, invalid); err == nil {
		t.Fatal("invalid sample metadata update succeeded")
	}
	after, _ := m.SampleAsset(index)
	if after != before {
		t.Fatalf("invalid update changed sample: before=%#v after=%#v", before, after)
	}
}
