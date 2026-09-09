package assetsync

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeAuthoredJSON(t *testing.T, dir, relative, source string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAuthoredAssetLoadersRejectAmbiguousOrDamagedJSON(t *testing.T) {
	loadProject := func(dir string) error { _, err := LoadProject(dir); return err }
	loadSheet := func(dir string) error { _, err := LoadSpritesheet(dir); return err }
	loadMap := func(dir string) error { _, err := LoadMap(dir); return err }
	loadPalette := func(dir string) error { _, err := LoadPalette(dir, "aurora"); return err }
	for _, test := range []struct {
		name, file, source, want string
		load                     func(string) error
	}{
		{"project duplicate", "project.json", `{"renderer":"mode4","renderer":"mode0"}`, "duplicate", loadProject},
		{"escaped duplicate", "project.json", `{"renderer":"mode4","\u0072enderer":"mode0"}`, "duplicate", loadProject},
		{"case alias duplicate", "project.json", `{"renderer":"mode4","Renderer":"mode0"}`, "duplicate", loadProject},
		{"ignored field duplicate", "project.json", `{"notes":{"title":"first","title":"second"}}`, "duplicate", loadProject},
		{"sprite duplicate", "assets/spritesheet.json", `{"width":8,"height":8,"sprites":[{"id":0,"id":1}]}`, "duplicate", loadSheet},
		{"map duplicate", "assets/map.json", `{"version":2,"layers":[{"id":0,"cells":[],"cells":[1]}]}`, "duplicate", loadMap},
		{"palette duplicate", "assets/palettes/aurora.json", `{"colors":[{"r":0,"r":31,"g":0,"b":0}]}`, "duplicate", loadPalette},
		{"invalid UTF-8 value", "project.json", "{\"entry\":\"bad\xff.lua\"}", "UTF-8", loadProject},
		{"invalid UTF-8 name", "project.json", "{\"bad\xff\":1}", "UTF-8", loadProject},
		{"unpaired surrogate", "project.json", `{"entry":"bad\ud800.lua"}`, "surrogate", loadProject},
		{"trailing value", "assets/map.json", `{"cells":[]} {"cells":[1]}`, "after top-level value", loadMap},
		{"trailing garbage", "project.json", `{"renderer":"auto"} broken`, "after top-level value", loadProject},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeAuthoredJSON(t, dir, test.file, test.source)
			err := test.load(dir)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), filepath.Base(test.file)) {
				t.Fatalf("load %s = %v, want filename and %q", test.file, err, test.want)
			}
		})
	}
}

func TestAudioLoadAndVersionDetectionRejectMalformedJSON(t *testing.T) {
	for _, file := range []string{"settings.json", "sounds.json", "music.json"} {
		for _, test := range []struct{ name, source, want string }{
			{"duplicate version", `{"version":2,"version":1}`, "duplicate"},
			{"escaped duplicate version", `{"version":2,"\u0076ersion":2}`, "duplicate"},
			{"case alias duplicate version", `{"version":2,"Version":1}`, "duplicate"},
			{"invalid UTF-8", "{\"version\":2,\"bad\xff\":0}", "UTF-8"},
			{"trailing value", `{"version":2} {"version":1}`, "after top-level value"},
		} {
			t.Run(file+"/"+test.name, func(t *testing.T) {
				dir := t.TempDir()
				writeAuthoredJSON(t, dir, "assets/audio/"+file, test.source)
				_, detectErr := DetectAudioVersion(dir)
				_, loadErr := LoadNativeAudio(dir)
				for operation, err := range map[string]error{"detect": detectErr, "load": loadErr} {
					if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), file) {
						t.Errorf("%s %s = %v, want filename and %q", operation, file, err, test.want)
					}
				}
			})
		}
	}
}

func TestAudioVersionDetectionValidatesIgnoredNestedContent(t *testing.T) {
	for _, source := range []string{
		`{"version":2,"parts":[{"lanes":{"ch1":0,"\u0063h1":1}}]}`,
		"{\"version\":2,\"parts\":[{\"name\":\"bad\xff\"}]}",
	} {
		dir := t.TempDir()
		writeAuthoredJSON(t, dir, "assets/audio/music.json", source)
		if _, err := DetectAudioVersion(dir); err == nil {
			t.Fatalf("version detection ignored malformed nested content: %q", source)
		}
		if _, err := LoadNativeAudio(dir); err == nil {
			t.Fatalf("audio loader ignored malformed nested content: %q", source)
		}
	}
}

func TestAuthoredJSONPreservesFieldMatchingDefaultsAndNulls(t *testing.T) {
	dir := t.TempDir()
	writeAuthoredJSON(t, dir, "project.json", `{"ACTIVEPALETTE":null,"Renderer":"auto","notes":{"title":"café 🎮"}}`)
	project, err := LoadProject(dir)
	if err != nil || project.ActivePalette != CanonicalPaletteName || project.Renderer != "auto" {
		t.Fatalf("project defaults/case/unknown-field compatibility = %+v, %v", project, err)
	}
	writeAuthoredJSON(t, dir, "assets/audio/settings.json", `{"Version":2,"Profile":null,"LEFTLEVEL":null,"rightLevel":3}`)
	writeAuthoredJSON(t, dir, "assets/audio/sounds.json", `{"version":null,"sounds":null}`)
	writeAuthoredJSON(t, dir, "assets/audio/music.json", `{"PPQN":null,"clips":null,"parts":null,"songs":null}`)
	assets, err := LoadNativeAudio(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantSettings := DefaultNativeAudioSettings()
	wantSettings.RightLevel = 3
	if assets.Settings != wantSettings || !reflect.DeepEqual(assets.Sounds, DefaultNativeSoundLibrary()) || !reflect.DeepEqual(assets.Music, DefaultNativeMusicLibrary()) {
		t.Fatalf("null/default compatibility = %+v / %+v / %+v", assets.Settings, assets.Sounds, assets.Music)
	}
	if got, err := DetectAudioVersion(dir); got != NativeAudioSchemaVersion || err != nil {
		t.Fatalf("case-insensitive version = %d, %v", got, err)
	}
}

func TestNativeAudioKeepsUnknownFieldsStrict(t *testing.T) {
	dir := t.TempDir()
	writeAuthoredJSON(t, dir, "assets/audio/music.json", `{"version":2,"parts":[{"lanes":{"ch5":0}}]}`)
	if _, err := LoadNativeAudio(dir); err == nil || !strings.Contains(err.Error(), `unknown field "ch5"`) {
		t.Fatalf("nested unknown audio field = %v", err)
	}
}
