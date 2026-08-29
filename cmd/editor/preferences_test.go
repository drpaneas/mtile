package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	editormodel "github.com/drpaneas/mtile/internal/editor"
)

func TestEditorPreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "editor.json")
	want := editorPreferences{
		Version: editorPreferencesVersion, Effects: effectsClean,
		LCDPreview: false, PreviewVolume: 30, AutoSaveBeforeBuild: false, TextSize: textLarge,
	}
	if err := saveEditorPreferences(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadEditorPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("preferences = %+v, want %+v", got, want)
	}
}

func TestProjectRendererCyclesWithoutChangingGlobalPreferences(t *testing.T) {
	g := &Game{
		model:       &editormodel.Model{Project: assetsync.Project{Renderer: "mode4"}},
		preferences: defaultEditorPreferences(),
	}
	wantPreferences := g.preferences
	g.cycleProjectRenderer(1)
	if g.model.Project.Renderer != "auto" || g.preferences != wantPreferences {
		t.Fatalf("after +1: renderer=%q preferences=%+v", g.model.Project.Renderer, g.preferences)
	}
	g.cycleProjectRenderer(1)
	if g.model.Project.Renderer != "mode0" {
		t.Fatalf("after +2 renderer=%q", g.model.Project.Renderer)
	}
	g.cycleProjectRenderer(1)
	if g.model.Project.Renderer != "mode4" {
		t.Fatalf("wrapped renderer=%q", g.model.Project.Renderer)
	}
	g.cycleProjectRenderer(-1)
	if g.model.Project.Renderer != "mode0" {
		t.Fatalf("reverse renderer=%q", g.model.Project.Renderer)
	}
}

func TestEditorPreferencesRejectMalformedAndClampValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "editor.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := loadEditorPreferences(path); err == nil || got != defaultEditorPreferences() {
		t.Fatalf("malformed preferences = %+v, error %v", got, err)
	}

	preferences := defaultEditorPreferences()
	preferences.Effects = effectLevel(99)
	preferences.PreviewVolume = 999
	preferences = normalizeEditorPreferences(preferences)
	preferences.TextSize = interfaceTextSize(99)
	preferences = normalizeEditorPreferences(preferences)
	if preferences.Effects != effectsSoft || preferences.PreviewVolume != 100 || preferences.TextSize != textComfort {
		t.Fatalf("normalized preferences = %+v", preferences)
	}
}

func TestEditorPreferencesMigratesReadableTextDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "editor.json")
	legacy := []byte(`{"version":1,"effects":0,"lcdPreview":false,"previewVolume":60,"autoSaveBeforeBuild":false}`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadEditorPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != editorPreferencesVersion || got.TextSize != textComfort || got.PreviewVolume != 60 || got.AutoSaveBeforeBuild {
		t.Fatalf("migrated preferences = %+v", got)
	}
}
