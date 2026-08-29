package assetsync

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

func nativeInt(value int) *int { return &value }

func nativeAudioFixture() (NativeAudioSettings, NativeSoundLibrary, NativeMusicLibrary) {
	wave := make([]int, 32)
	for index := range wave {
		wave[index] = index & 15
	}
	envelope := NativeEnvelopeAsset{Initial: 12, Direction: "down", Pace: 2}
	sounds := NativeSoundLibrary{Version: NativeAudioSchemaVersion, Sounds: []NativeSoundAsset{
		{ID: 0, Name: "bright-lead", Kind: "ch1", Route: "both", Pulse: &NativePulseAsset{
			Duty: 50, Envelope: envelope, Sweep: &NativeSweepAsset{Pace: 2, Direction: "up", Shift: 1},
		}, Macro: &NativeMacroAsset{Steps: []NativeMacroStepAsset{{PeriodDelta: 1}, {PeriodDelta: -1}}, LoopStart: nativeInt(0)}},
		{ID: 1, Name: "soft-pulse", Kind: "ch2", Route: "both", Pulse: &NativePulseAsset{Duty: 25, Envelope: envelope}},
		{ID: 2, Name: "round-wave", Kind: "ch3", Route: "both", Wave: &NativeWaveAsset{Samples: wave, Level: 100}},
		{ID: 3, Name: "short-noise", Kind: "ch4", Route: "both", Noise: &NativeNoiseAsset{Width: 7, Divisor: 3, Shift: 5, Envelope: envelope},
			Macro: &NativeMacroAsset{Steps: []NativeMacroStepAsset{{NoiseNoteOffset: 32, HasNoiseNote: true}}, LoopStart: nativeInt(0)}},
	}}
	music := NativeMusicLibrary{
		Version: NativeAudioSchemaVersion,
		PPQN:    audio.NativePPQN,
		Clips: []NativeClipAsset{
			{ID: 0, Name: "lead-a", Kind: "ch1", LengthTicks: 96, Events: []NativeEventAsset{{Tick: 0, DurationTicks: 24, Note: 69, SoundID: 0, Priority: 100}}},
			{ID: 1, Name: "pulse-a", Kind: "ch2", LengthTicks: 96, Events: []NativeEventAsset{{Tick: 0, DurationTicks: 24, Note: 57, SoundID: 1, Priority: 90}}},
			{ID: 2, Name: "wave-a", Kind: "ch3", LengthTicks: 96, Events: []NativeEventAsset{{Tick: 0, DurationTicks: 24, Note: 57, SoundID: 2, Priority: 80}}},
			{ID: 3, Name: "noise-a", Kind: "ch4", LengthTicks: 96, Events: []NativeEventAsset{{Tick: 0, DurationTicks: 12, Note: 60, SoundID: 3, Priority: 70}}},
		},
		Parts: []NativePartAsset{{
			ID: 0, Name: "intro", LengthTicks: 96,
			Lanes: NativeLaneAssets{CH1: nativeInt(0), CH2: nativeInt(1), CH3: nativeInt(2), CH4: nativeInt(3)},
		}},
		Songs: []NativeSongAsset{{ID: 0, Name: "new-song", Tempo: 120, Sequence: []int{0}, LoopStart: nativeInt(0)}},
	}
	return DefaultNativeAudioSettings(), sounds, music
}

func TestCompileNativeAudioMapsAllFourHardwareChannels(t *testing.T) {
	settings, sounds, music := nativeAudioFixture()
	music.Parts[0].Tempo = 180
	library, err := CompileNativeAudio(settings, sounds, music)
	if err != nil {
		t.Fatal(err)
	}
	if len(library.Sounds) != 4 || len(library.Clips) != 4 || len(library.Parts) != 1 || len(library.Songs) != 1 {
		t.Fatalf("compiled library sizes = %d/%d/%d/%d", len(library.Sounds), len(library.Clips), len(library.Parts), len(library.Songs))
	}
	if library.Sounds[0].Pulse1.Pulse.Duty != hw.PulseDuty50 || library.Sounds[0].Pulse1.Sweep.Shift != 1 {
		t.Fatalf("CH1 mapping = %+v", library.Sounds[0].Pulse1)
	}
	if library.Sounds[2].Wave.Wave.Length != 32 || library.Sounds[2].Wave.Level != hw.Wave100 {
		t.Fatalf("CH3 mapping = %+v", library.Sounds[2].Wave)
	}
	if library.Sounds[3].Noise.Width != hw.Noise7Bit || library.Parts[0].PSGClips != [4]int16{0, 1, 2, 3} {
		t.Fatalf("CH4/part mapping = %+v / %+v", library.Sounds[3].Noise, library.Parts[0])
	}
	if got := library.Sounds[0].Macro; len(got.Steps) != 2 || got.LoopStart != 0 || got.Steps[1].PeriodDelta != -1 {
		t.Fatalf("CH1 macro mapping = %+v", got)
	}
	if got := library.Sounds[3].Macro; len(got.Steps) != 1 || !got.Steps[0].HasNoiseNote || got.Steps[0].NoiseNoteOffset != 32 {
		t.Fatalf("CH4 macro mapping = %+v", got)
	}
	if library.Parts[0].Tempo != 180 {
		t.Fatalf("part tempo = %d, want 180", library.Parts[0].Tempo)
	}
}

func TestNativeAudioRoundTripUsesOnlyThreeV2Files(t *testing.T) {
	dir := t.TempDir()
	settings, sounds, music := nativeAudioFixture()
	music.Parts[0].Tempo = 180
	if err := SaveNativeAudioSettings(dir, &settings); err != nil {
		t.Fatal(err)
	}
	if err := SaveNativeSounds(dir, &sounds); err != nil {
		t.Fatal(err)
	}
	if err := SaveNativeMusic(dir, sounds, &music); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadNativeAudio(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Settings.Profile != NativeAudioProfile || len(loaded.Library.Sounds) != 4 ||
		loaded.Music.Parts[0].Tempo != 180 || loaded.Library.Parts[0].Tempo != 180 {
		t.Fatalf("loaded native assets = %+v", loaded)
	}
	for _, relative := range []string{
		filepath.Join("assets", "audio", "settings.json"),
		filepath.Join("assets", "audio", "sounds.json"),
		filepath.Join("assets", "audio", "music.json"),
	} {
		if _, err := os.Stat(filepath.Join(dir, relative)); err != nil {
			t.Fatalf("missing %s: %v", relative, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "audio", "samples")); !os.IsNotExist(err) {
		t.Fatalf("native save created samples directory: %v", err)
	}
}

func TestNativeAudioStrictlyRejectsPCMAndLegacyAssets(t *testing.T) {
	dir := t.TempDir()
	settingsPath := audioSettingsPath(dir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := `{"version":2,"profile":"gameboy-psg","leftLevel":7,"rightLevel":7,"psgRatio":100,"sampleRate":16384}`
	if err := os.WriteFile(settingsPath, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadNativeAudio(dir); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("PCM field error = %v", err)
	}

	dir = t.TempDir()
	legacy := filepath.Join(dir, "assets", "audio", "sfx.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`{"version":1,"sfx":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadNativeAudio(dir); err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("legacy format error = %v", err)
	}
}

func TestNativeAudioRejectsWrongChannelReferencesAndOverlap(t *testing.T) {
	settings, sounds, music := nativeAudioFixture()
	music.Clips[0].Events[0].SoundID = 1
	if _, err := CompileNativeAudio(settings, sounds, music); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("wrong-channel sound error = %v", err)
	}
	_, sounds, music = nativeAudioFixture()
	music.Clips[0].Events = append(music.Clips[0].Events, NativeEventAsset{Tick: 12, DurationTicks: 24, Note: 72, SoundID: 0, Priority: 100})
	if _, err := CompileNativeAudio(settings, sounds, music); err == nil || !strings.Contains(err.Error(), "overlapping") {
		t.Fatalf("overlap error = %v", err)
	}
	_, sounds, music = nativeAudioFixture()
	music.Parts[0].Tempo = 19
	if _, err := CompileNativeAudio(settings, sounds, music); err == nil {
		t.Fatal("out-of-range part tempo was accepted")
	}
}

func TestNativeAudioRejectsInvalidRelativeNoiseMacros(t *testing.T) {
	settings, sounds, music := nativeAudioFixture()
	sounds.Sounds[3].Macro.Steps[0].HasNoise = true
	if _, err := CompileNativeAudio(settings, sounds, music); err == nil || !strings.Contains(err.Error(), "mixes absolute and note-relative") {
		t.Fatalf("mixed absolute/relative CH4 macro error = %v", err)
	}

	settings, sounds, music = nativeAudioFixture()
	sounds.Sounds[3].Macro.Steps[0].NoiseNoteOffset = 36
	if _, err := CompileNativeAudio(settings, sounds, music); err == nil || !strings.Contains(err.Error(), "invalid CH4 note offset") {
		t.Fatalf("out-of-range relative CH4 macro error = %v", err)
	}

	settings, sounds, music = nativeAudioFixture()
	music.Clips[3].Events[0].Note = 108
	if _, err := CompileNativeAudio(settings, sounds, music); err == nil || !strings.Contains(err.Error(), "hardware") {
		t.Fatalf("out-of-range relative CH4 event error = %v", err)
	}
}

func TestDetectAudioVersionDefaultsToV2AndKeepsExplicitLegacySeparate(t *testing.T) {
	dir := t.TempDir()
	if got, err := DetectAudioVersion(dir); err != nil || got != NativeAudioSchemaVersion {
		t.Fatalf("empty project version = %d, %v", got, err)
	}
	legacy := DefaultAudioSettings()
	if err := SaveAudioSettings(dir, &legacy); err != nil {
		t.Fatal(err)
	}
	if got, err := DetectAudioVersion(dir); err != nil || got != AudioSettingsSchemaVersion {
		t.Fatalf("legacy project version = %d, %v", got, err)
	}
	if err := SaveNativeAudioSettings(dir, ptrNativeSettings(DefaultNativeAudioSettings())); err != nil {
		t.Fatal(err)
	}
	if got, err := DetectAudioVersion(dir); err != nil || got != NativeAudioSchemaVersion {
		t.Fatalf("native project version = %d, %v", got, err)
	}
}

func ptrNativeSettings(settings NativeAudioSettings) *NativeAudioSettings { return &settings }

func TestGenerateNativeAudioGoCompilesAgainstRealRuntime(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)
	settings, sounds, music := nativeAudioFixture()
	music.Parts[0].Tempo = 180
	if err := SaveNativeAudioSettings(dir, &settings); err != nil {
		t.Fatal(err)
	}
	if err := SaveNativeSounds(dir, &sounds); err != nil {
		t.Fatal(err)
	}
	if err := SaveNativeMusic(dir, sounds, &music); err != nil {
		t.Fatal(err)
	}
	source, err := GenerateGo(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, wanted := range []string{"nativeSoundMacro0", "nativeSoundMacro3", "nativeSoundROM", "nativeClipROM", "nativePartROM", "nativeSongROM", "InitNativeAudio"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("generated source is missing %s", wanted)
		}
	}
	if !strings.Contains(text, "Tempo: 180") {
		t.Fatal("generated native part omitted its tempo override")
	}
	for _, forbidden := range []string{"audio.Mixer", "audio.SFXPlayer", "samplePCMROM", "InitSFX"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("generated PSG source contains legacy token %s", forbidden)
		}
	}

	genDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(genDir, "assets.go"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeDir, err := filepath.Abs(filepath.Join("..", "..", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	module := fmt.Sprintf("module generated-native\n\ngo 1.22\n\nrequire github.com/drpaneas/mtile/runtime v0.0.0\n\nreplace github.com/drpaneas/mtile/runtime => %s\n", runtimeDir)
	if err := os.WriteFile(filepath.Join(genDir, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	"testing"
	"github.com/drpaneas/mtile/runtime/audio"
)

func TestNativeGeneratedLibrary(t *testing.T) {
	var controller audio.Controller
	if !InitNativeAudio(&controller) || !controller.PlaySong(0) {
		t.Fatal("generated native audio initialization failed")
	}
	controller.Service()
	preview := controller.Preview()
	if !preview.Stats.RunningSong || !preview.Stats.Channels[0].Active || !preview.Stats.Channels[3].Active {
		t.Fatalf("generated native song did not reach CH1-CH4: %+v", preview.Stats)
	}
}
`
	if err := os.WriteFile(filepath.Join(genDir, "harness_test.go"), []byte(harness), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-run", "TestNativeGeneratedLibrary", "-v", "./...")
	cmd.Dir = genDir
	cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(os.TempDir(), "mtile-generated-native-gocache"))
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("generated native assets failed: %v\n%s", err, output.String())
	}
}
