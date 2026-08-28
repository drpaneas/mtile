package assetsync

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func validSFXLibrary() SFXLibrary {
	return SFXLibrary{Version: SFXSchemaVersion, SFX: []SFXAsset{
		{
			ID: 7, Name: "jump", Speed: 8, Loop: &SFXLoop{Start: 1, End: 3},
			Filters: &SFXFilters{Noiz: true, Buzz: true, Detune: 1, Reverb: 2, Dampen: 1},
			Steps: []SFXStep{
				{Pitch: 57, Waveform: "triangle", Volume: 7, Effect: "slide"},
				{Pitch: 60, Waveform: "square", Volume: 4, Effect: "vibrato"},
				{Pitch: 62, Waveform: "pulse", Volume: 0, Effect: "drop"},
				{Pitch: 63, Waveform: "saw", Volume: 3, Effect: "fade-in"},
				{Pitch: 62, Waveform: "tilted-saw", Volume: 2, Effect: "fade-out"},
				{Pitch: 60, Waveform: "organ", Volume: 4, Effect: "arpeggio-fast"},
				{Pitch: 57, Waveform: "phaser", Volume: 5, Effect: "arpeggio-slow"},
			},
		},
	}}
}

func TestCompileSFXPacksStableTargetRecords(t *testing.T) {
	library := validSFXLibrary()
	library.SFX = append(library.SFX, SFXAsset{
		ID: 2, Name: "coin", Speed: 4,
		Steps: []SFXStep{{Pitch: 57, Waveform: "noise", Volume: 1, Effect: "none"}},
	})
	definitions, steps, err := CompileSFX(library)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 2 || definitions[0].ID != 2 || definitions[1].ID != 7 {
		t.Fatalf("definitions are not sorted by stable ID: %#v", definitions)
	}
	if definitions[1].StepOffset != 1 || definitions[1].LoopStart != 1 || definitions[1].LoopEnd != 3 || definitions[1].Filters != 134 {
		t.Fatalf("jump definition = %#v", definitions[1])
	}
	if len(steps) != 8 || steps[1].PhaseStep != 461373440 || steps[1].Volume != 167 || steps[1].Effect != "slide" || steps[2].Effect != "vibrato" || steps[3].Effect != "drop" || steps[3].Volume != 0 || steps[4].Effect != "fade-in" || steps[5].Effect != "fade-out" || steps[5].Waveform != "tilted-saw" || steps[6].Effect != "arpeggio-fast" || steps[7].Effect != "arpeggio-slow" {
		t.Fatalf("compiled steps = %#v", steps)
	}
}

func TestTargetVolumeUsesPICOCaptureCalibration(t *testing.T) {
	want := [...]uint16{0, 24, 48, 72, 95, 119, 143, 167}
	for volume, expected := range want {
		if got := targetVolume(volume, "triangle"); got != expected {
			t.Fatalf("targetVolume(%d, triangle) = %d, want %d", volume, got, expected)
		}
	}
	if got := targetVolume(5, "tilted-saw"); got != 120 {
		t.Fatalf("targetVolume(5, tilted-saw) = %d, want 120", got)
	}
	if got := targetVolume(5, "saw"); got != 79 {
		t.Fatalf("targetVolume(5, saw) = %d, want 79", got)
	}
	if got := targetVolume(5, "square"); got != 60 {
		t.Fatalf("targetVolume(5, square) = %d, want 60", got)
	}
	if got := targetVolume(5, "pulse"); got != 60 {
		t.Fatalf("targetVolume(5, pulse) = %d, want 60", got)
	}
	if got := targetVolume(5, "custom-0"); got != 5 {
		t.Fatalf("targetVolume(5, custom-0) = %d, want multiplier 5", got)
	}
}

func TestCompileSFXPacksCustomInstrumentReferences(t *testing.T) {
	library := SFXLibrary{Version: SFXSchemaVersion, SFX: []SFXAsset{
		{ID: 0, Name: "instrument", Speed: 3, Steps: []SFXStep{{Pitch: 24, Waveform: "triangle", Volume: 7, Effect: "none"}}},
		{ID: 8, Name: "melody", Speed: 8, Steps: []SFXStep{{Pitch: 30, Waveform: "custom-0", Volume: 5, Effect: "vibrato"}}},
	}}
	_, steps, err := CompileSFX(library)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[1].Waveform != "custom-0" || steps[1].Volume != 5 || targetWaveformGoName(steps[1].Waveform) != "audio.SourceCustom0" {
		t.Fatalf("compiled custom step = %#v", steps)
	}
}

func TestCompileSFXRejectsInvalidCustomInstrumentGraphs(t *testing.T) {
	step := func(waveform string) []SFXStep {
		return []SFXStep{{Pitch: 24, Waveform: waveform, Volume: 7, Effect: "none"}}
	}
	for _, test := range []struct {
		name    string
		library SFXLibrary
		want    string
	}{
		{
			"missing",
			SFXLibrary{Version: 1, SFX: []SFXAsset{{ID: 8, Name: "outer", Speed: 1, Steps: step("custom-0")}}},
			"missing",
		},
		{
			"cycle",
			SFXLibrary{Version: 1, SFX: []SFXAsset{{ID: 0, Name: "cycle", Speed: 1, Steps: step("custom-0")}}},
			"cycle",
		},
		{
			"depth",
			SFXLibrary{Version: 1, SFX: []SFXAsset{
				{ID: 0, Name: "zero", Speed: 1, Steps: step("custom-1")},
				{ID: 1, Name: "one", Speed: 1, Steps: step("custom-2")},
				{ID: 2, Name: "two", Speed: 1, Steps: step("triangle")},
			}},
			"depth",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := CompileSFX(test.library); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CompileSFX error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCompileSFXIgnoresSilentCustomInstrumentReferences(t *testing.T) {
	library := SFXLibrary{Version: SFXSchemaVersion, SFX: []SFXAsset{
		{ID: 0, Name: "silent self reference", Speed: 1, Steps: []SFXStep{
			{Pitch: 24, Waveform: "triangle", Volume: 7, Effect: "none"},
			{Pitch: 24, Waveform: "custom-0", Volume: 0, Effect: "none"},
		}},
	}}
	if _, _, err := CompileSFX(library); err != nil {
		t.Fatalf("silent custom reference rejected: %v", err)
	}
}

func TestTargetPhaseStepUsesPICONumericPitch(t *testing.T) {
	if got := targetPhaseStep(33); got != 115343360 {
		t.Fatalf("PICO pitch 33 phase step = %d, want A=440 Hz step 115343360", got)
	}
	if got := targetPhaseStep(36); got != 137167144 {
		t.Fatalf("PICO pitch 36 phase step = %d, want approximately 523.25 Hz step 137167144", got)
	}
}

func TestLoadSFXMissingIsEmptyVersionOne(t *testing.T) {
	library, err := LoadSFX(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if library.Version != SFXSchemaVersion || len(library.SFX) != 0 {
		t.Fatalf("missing library = %#v", library)
	}
}

func TestSaveAndLoadSFXRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := validSFXLibrary()
	if err := SaveSFX(dir, &want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSFX(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(got.SFX, want.SFX, func(a, b SFXAsset) bool {
		return a.ID == b.ID && a.Name == b.Name && a.Speed == b.Speed && slices.Equal(a.Steps, b.Steps) && *a.Loop == *b.Loop && *a.Filters == *b.Filters
	}) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestLoadSFXRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	for name, content := range map[string]string{
		"unknown":  `{"version":1,"sfx":[],"typo":true}`,
		"trailing": `{"version":1,"sfx":[]} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "assets", "audio", "sfx.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadSFX(dir); err == nil {
				t.Fatal("invalid JSON was accepted")
			}
		})
	}
}

func TestCompileSFXRejectsInvalidAssets(t *testing.T) {
	tests := []struct {
		name string
		edit func(*SFXLibrary)
		want string
	}{
		{"version", func(l *SFXLibrary) { l.Version++ }, "version"},
		{"id", func(l *SFXLibrary) { l.SFX[0].ID = 64 }, "outside"},
		{"name", func(l *SFXLibrary) { l.SFX[0].Name = " bad" }, "name"},
		{"speed", func(l *SFXLibrary) { l.SFX[0].Speed = 0 }, "speed"},
		{"steps", func(l *SFXLibrary) { l.SFX[0].Steps = nil }, "steps"},
		{"loop", func(l *SFXLibrary) { l.SFX[0].Loop.End = 8 }, "loop"},
		{"pitch", func(l *SFXLibrary) { l.SFX[0].Steps[0].Pitch = 64 }, "pitch"},
		{"waveform", func(l *SFXLibrary) { l.SFX[0].Steps[0].Waveform = "custom" }, "waveform"},
		{"volume", func(l *SFXLibrary) { l.SFX[0].Steps[0].Volume = 8 }, "volume"},
		{"effect", func(l *SFXLibrary) { l.SFX[0].Steps[0].Effect = "bogus" }, "effect"},
		{"detune", func(l *SFXLibrary) { l.SFX[0].Filters.Detune = 3 }, "detune"},
		{"reverb", func(l *SFXLibrary) { l.SFX[0].Filters.Reverb = -1 }, "reverb"},
		{"dampen", func(l *SFXLibrary) { l.SFX[0].Filters.Dampen = 3 }, "dampen"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			library := validSFXLibrary()
			test.edit(&library)
			if _, _, err := CompileSFX(library); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadAndGenerateGoIncludesCompiledSFX(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)
	library := validSFXLibrary()
	if err := SaveSFX(dir, &library); err != nil {
		t.Fatal(err)
	}
	assets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets.SFXROM) != 1 || len(assets.SFXStepROM) != 7 || assets.SFXROM[0].ID != 7 {
		t.Fatalf("compiled loaded SFX = definitions %#v steps %#v", assets.SFXROM, assets.SFXStepROM)
	}
	source, err := GenerateGo(dir, "gameassets")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"github.com/drpaneas/mtile/runtime/audio"`,
		"var sfxROM",
		"var sfxStepROM",
		"func InitSFX(player *audio.SFXPlayer, mixer *audio.Mixer) bool",
		"audio.SourceTriangle",
		"audio.SourceTiltedSaw",
		"audio.SourceOrgan",
		"audio.SourcePhaser",
		"audio.SourcePulse",
		"audio.SFXEffectSlide",
		"audio.SFXEffectVibrato",
		"audio.SFXEffectDrop",
		"audio.SFXEffectFadeIn",
		"audio.SFXEffectFadeOut",
		"audio.SFXEffectArpeggioFast",
		"audio.SFXEffectArpeggioSlow",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated audio assets missing %q", want)
		}
	}
}
