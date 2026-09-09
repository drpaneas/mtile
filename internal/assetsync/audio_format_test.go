package assetsync

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndGenerateRejectSupersededAudio(t *testing.T) {
	for _, relative := range []string{"settings.json", "music.json", "sfx.json", "samples/manifest.json"} {
		t.Run(relative, func(t *testing.T) {
			dir := t.TempDir()
			sheet, mapData := nativeFixture()
			writeTestProject(t, dir, canonicalPalette(), sheet, mapData)
			path := filepath.Join(dir, "assets", "audio", relative)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			original := []byte(`{"version":1}`)
			if err := os.WriteFile(path, original, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "superseded") {
				t.Fatalf("Load accepted audio v1: %v", err)
			}
			if _, err := GenerateGo(dir, "main"); err == nil || !strings.Contains(err.Error(), "superseded") {
				t.Fatalf("GenerateGo accepted audio v1: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatalf("rejected audio changed: %q, %v", got, err)
			}
		})
	}
}
