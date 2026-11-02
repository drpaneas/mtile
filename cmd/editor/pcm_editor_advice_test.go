package main

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/audioimport"
)

func pcmQuietFixture(t *testing.T) string {
	return pcmTimedQuietFixture(t, 48000, 2)
}
func pcmTimedQuietFixture(t *testing.T, rate, seconds int) string {
	t.Helper()
	samples := make([]byte, rate*seconds*2)
	for i := 0; i < rate*seconds; i++ {
		if i < rate/4 || i >= rate*seconds-rate/4 {
			continue
		}
		value := int16(18000 * math.Sin(2*math.Pi*440*float64(i)/float64(rate)))
		binary.LittleEndian.PutUint16(samples[i*2:], uint16(value))
	}
	raw := make([]byte, 44)
	copy(raw, "RIFF")
	binary.LittleEndian.PutUint32(raw[4:], uint32(36+len(samples)))
	copy(raw[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(raw[16:], 16)
	binary.LittleEndian.PutUint16(raw[20:], 1)
	binary.LittleEndian.PutUint16(raw[22:], 1)
	binary.LittleEndian.PutUint32(raw[24:], uint32(rate))
	binary.LittleEndian.PutUint32(raw[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(raw[32:], 2)
	binary.LittleEndian.PutUint16(raw[34:], 16)
	copy(raw[36:], "data")
	binary.LittleEndian.PutUint32(raw[40:], uint32(len(samples)))
	raw = append(raw, samples...)
	path := filepath.Join(t.TempDir(), "door.wav")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPCMAdviceShowsActionableSavingsAndQuietTiming(t *testing.T) {
	p, err := audioimport.Prepare(pcmQuietFixture(t), audioimport.QualityBalanced)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(pcmAudioAdvice(p), " ")
	for _, want := range []string{"Try Smallest:", "50%", "Quiet edges:", "retaining 20 ms"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	label := pcmProfileLabel(p, audioimport.QualitySmall)
	if !strings.Contains(label, "smaller than original") {
		t.Fatal("profile estimate missing", label)
	}
}

func TestPCMAdviceWarnsOnLargeOrLongPreparedRecordings(t *testing.T) {
	for _, tc := range []struct {
		seconds float64
		bytes   int
	}{{31, 16044}, {2, 300000}} {
		p := audioimport.Prepared{Quality: audioimport.QualitySmall, Data: make([]byte, tc.bytes), After: audioimport.WaveInfo{Rate: 8000, Frames: int(tc.seconds * 8000)}, Analysis: audioimport.WaveAnalysis{Info: audioimport.WaveInfo{Rate: 8000, Frames: int(tc.seconds * 8000)}}}
		if text := strings.Join(pcmAudioAdvice(p), " "); !strings.Contains(text, "Long/large recording:") {
			t.Fatal("large/long warning missing", text)
		}
	}
	p := audioimport.Prepared{Quality: audioimport.QualitySmall, Data: make([]byte, 8044), After: audioimport.WaveInfo{Rate: 8000, Frames: 8000}, Analysis: audioimport.WaveAnalysis{Info: audioimport.WaveInfo{Rate: 8000, Frames: 8000}}}
	if len(pcmAudioAdvice(p)) != 0 {
		t.Fatal("compact recording received unnecessary advice")
	}
}

func TestPCMEditorTrimCanBeReversedAndKeepSourcePreservesTiming(t *testing.T) {
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	path := pcmQuietFixture(t)
	g.pcmPrepare(path)
	waitPCM(t, g)
	g.pcm.name = "my_door"
	untrimmed := append([]byte(nil), g.pcm.prepared.Data...)
	g.pcmToggleTrim()
	waitPCM(t, g)
	if !g.pcm.trimSilence || !g.pcm.prepared.Trimmed || g.pcm.prepared.After.Seconds() >= 2 || g.pcm.name != "my_door" {
		t.Fatal("trim did not apply")
	}
	g.pcmToggleTrim()
	waitPCM(t, g)
	if g.pcm.trimSilence || string(g.pcm.prepared.Data) != string(untrimmed) {
		t.Fatal("restoring quiet edges changed the original preparation")
	}
	g.pcmToggleTrim()
	waitPCM(t, g)
	g.pcmSetQuality(audioimport.QualitySource)
	waitPCM(t, g)
	if g.pcm.trimSilence || g.pcm.prepared.After.Seconds() != 2 {
		t.Fatal("Keep Source retained trimming")
	}
	g.pcmToggleTrim()
	if g.pcm.job != nil || !strings.Contains(g.pcm.message, "Choose Balanced") {
		t.Fatal("Keep Source silently trimmed")
	}
}

func TestPCMTrimmedDataIsWhatTheRealImporterSaves(t *testing.T) {
	tool := os.Getenv("MTILE_PCM_MMUTIL")
	if tool == "" {
		t.Skip("set MTILE_PCM_MMUTIL for real import proof")
	}
	g := newNativeMusicTestGame(t)
	defer g.closeNativePerformance()
	g.openPCMEditor()
	g.pcm.converter = tool
	g.pcmPrepare(pcmQuietFixture(t))
	waitPCM(t, g)
	g.pcmToggleTrim()
	waitPCM(t, g)
	want := append([]byte(nil), g.pcm.prepared.Data...)
	g.pcm.name = "door"
	g.pcmImport()
	waitPCM(t, g)
	if g.pcm.phase != "done" {
		t.Fatal(g.pcm.message)
	}
	got, err := os.ReadFile(filepath.Join(g.model.Dir, "assets/audio/direct/source/sfx_door.wav"))
	if err != nil || string(got) != string(want) {
		t.Fatal("imported waveform differs from auditioned preparation", err)
	}
}
