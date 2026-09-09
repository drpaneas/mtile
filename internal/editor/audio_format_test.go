package editor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRejectsSupersededAudioWithoutWritingProject(t *testing.T) {
	for _, relative := range []string{"settings.json", "music.json", "sfx.json", "samples/manifest.json"} {
		t.Run(relative, func(t *testing.T) {
			dir := editorFixture(t)
			path := filepath.Join(dir, "assets", "audio", relative)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			original := []byte(`{"version":1}`)
			if err := os.WriteFile(path, original, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "superseded") {
				t.Fatalf("Open must reject audio v1: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatalf("rejected audio changed: %q, %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "assets", "audio", "sounds.json")); !os.IsNotExist(err) {
				t.Fatalf("rejected project acquired v2 sounds: %v", err)
			}
		})
	}
}

func TestUnsupportedModelCannotSaveOrInitializeNativeAudio(t *testing.T) {
	dir := editorFixture(t)
	model, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	model.AudioVersion = 1
	for _, operation := range []func() error{model.Save, model.EnableNativeAudioV2} {
		if err := operation(); err == nil || !strings.Contains(err.Error(), "superseded") {
			t.Fatalf("unsupported model accepted: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "audio")); !os.IsNotExist(err) {
		t.Fatalf("unsupported model wrote audio: %v", err)
	}
}
