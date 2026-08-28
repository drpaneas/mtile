package assetsync

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/runtime/audio"
)

func musicInt(value int) *int { return &value }

func validMusicLibrary() MusicLibrary {
	return MusicLibrary{Version: MusicSchemaVersion, Name: "Adventure Starts", Patterns: []MusicPattern{
		{ID: 3, Name: "loop", Channels: []int{7, -1, -1, -1}, Next: musicInt(3)},
		{ID: 0, Name: "intro", Channels: []int{-1, 7, -1, -1}, Next: musicInt(3)},
	}}
}

func TestCompileMusicPacksStablePatternGraph(t *testing.T) {
	patterns, err := CompileMusic(validMusicLibrary(), validSFXLibrary())
	if err != nil {
		t.Fatal(err)
	}
	if len(patterns) != 2 || patterns[0].ID != 0 || patterns[1].ID != 3 {
		t.Fatalf("patterns are not sorted by stable ID: %#v", patterns)
	}
	if patterns[0].Channels != [MusicChannels]int8{-1, 7, -1, -1} || patterns[0].Next != 3 {
		t.Fatalf("intro pattern = %#v", patterns[0])
	}
	if got, want := patterns[0].DurationSamples, audio.SFXSamplesForUnits(8*7); got != want {
		t.Fatalf("compiled pattern duration = %d, want %d", got, want)
	}
}

func TestCompileMusicSupportsExplicitPICOClockAndInfiniteLoop(t *testing.T) {
	library := validMusicLibrary()
	infinite := 0
	library.Patterns[0].DurationUnits = &infinite
	units := 9
	library.Patterns[1].DurationUnits = &units
	patterns, err := CompileMusic(library, validSFXLibrary())
	if err != nil {
		t.Fatal(err)
	}
	if patterns[1].DurationSamples != 0 {
		t.Fatalf("infinite PICO pattern duration = %d, want 0", patterns[1].DurationSamples)
	}
	if got, want := patterns[0].DurationSamples, audio.SFXSamplesForUnits(9); got != want {
		t.Fatalf("explicit PICO duration = %d, want %d", got, want)
	}
}

func TestCompileMusicSupportsTimedSilentSection(t *testing.T) {
	library := validMusicLibrary()
	units := 32
	library.Patterns[0].Channels = []int{-1, -1, -1, -1}
	library.Patterns[0].DurationUnits = &units
	patterns, err := CompileMusic(library, validSFXLibrary())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := patterns[1].DurationSamples, audio.SFXSamplesForUnits(uint32(units)); got != want {
		t.Fatalf("silent section duration = %d, want %d", got, want)
	}
}

func TestLoadMusicMissingIsEmptyVersionOne(t *testing.T) {
	library, err := LoadMusic(t.TempDir(), validSFXLibrary())
	if err != nil {
		t.Fatal(err)
	}
	if library.Version != MusicSchemaVersion || len(library.Patterns) != 0 {
		t.Fatalf("missing library = %#v", library)
	}
}

func TestSaveAndLoadMusicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := validMusicLibrary()
	if err := SaveMusic(dir, &want, validSFXLibrary()); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMusic(dir, validSFXLibrary())
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || !slices.EqualFunc(got.Patterns, want.Patterns, func(a, b MusicPattern) bool {
		return a.ID == b.ID && a.Name == b.Name && slices.Equal(a.Channels, b.Channels) && *a.Next == *b.Next
	}) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestLoadMusicRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	for name, content := range map[string]string{
		"unknown":  `{"version":1,"patterns":[],"typo":true}`,
		"trailing": `{"version":1,"patterns":[]} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "assets", "audio", "music.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadMusic(dir, validSFXLibrary()); err == nil {
				t.Fatal("invalid JSON was accepted")
			}
		})
	}
}

func TestCompileMusicRejectsInvalidPatterns(t *testing.T) {
	tests := []struct {
		name string
		edit func(*MusicLibrary)
		want string
	}{
		{"version", func(l *MusicLibrary) { l.Version++ }, "version"},
		{"song name", func(l *MusicLibrary) { l.Name = " bad" }, "music name"},
		{"id", func(l *MusicLibrary) { l.Patterns[0].ID = 64 }, "outside"},
		{"name", func(l *MusicLibrary) { l.Patterns[0].Name = " bad" }, "name"},
		{"channels", func(l *MusicLibrary) { l.Patterns[0].Channels = []int{7} }, "exactly 4"},
		{"missing SFX", func(l *MusicLibrary) { l.Patterns[0].Channels[0] = 6 }, "missing SFX"},
		{"silent", func(l *MusicLibrary) { l.Patterns[0].Channels = []int{-1, -1, -1, -1} }, "at least one"},
		{"next omitted", func(l *MusicLibrary) { l.Patterns[0].Next = nil }, "explicit next"},
		{"next missing", func(l *MusicLibrary) { l.Patterns[0].Next = musicInt(2) }, "does not exist"},
		{"duration", func(l *MusicLibrary) { value := MaxMusicDurationUnits + 1; l.Patterns[0].DurationUnits = &value }, "duration units"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			library := validMusicLibrary()
			test.edit(&library)
			if _, err := CompileMusic(library, validSFXLibrary()); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadAndGenerateGoIncludesCompiledMusic(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)
	sfx := validSFXLibrary()
	if err := SaveSFX(dir, &sfx); err != nil {
		t.Fatal(err)
	}
	music := validMusicLibrary()
	if err := SaveMusic(dir, &music, sfx); err != nil {
		t.Fatal(err)
	}
	assets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets.MusicROM) != 2 || assets.MusicROM[0].ID != 0 {
		t.Fatalf("compiled loaded music = %#v", assets.MusicROM)
	}
	source, err := GenerateGo(dir, "gameassets")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"var musicROM", "audio.MusicPattern", "player.SetMasterVolume(256)", "player.InitMusic(musicROM[:])"} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated music assets missing %q", want)
		}
	}
}
