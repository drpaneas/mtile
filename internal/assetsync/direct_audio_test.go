package assetsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectAudioValidatesBankAndCompanionSchema(t *testing.T) {
	fixture, err := LoadDirectAudio("../../examples/imported-audio")
	if err != nil || fixture == nil {
		t.Fatal(err)
	}
	if fixture.Manifest.Format != 0x19 || len(fixture.Manifest.Music) != 1 || len(fixture.Manifest.Samples) != 1 {
		t.Fatal("wrong fixture")
	}
	if _, err = LoadDirectAudio(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*DirectAudioManifest, []byte){
		func(m *DirectAudioManifest, b []byte) { b[len(b)-1] ^= 1 },
		func(m *DirectAudioManifest, b []byte) { m.Version = 2 },
		func(m *DirectAudioManifest, b []byte) { m.Samples[0].BankID = 65535 },
		func(m *DirectAudioManifest, b []byte) { m.Music[0].Source = "../escape.mod" },
	} {
		m := fixture.Manifest
		m.Music = append([]DirectAudioItem(nil), m.Music...)
		m.Samples = append([]DirectAudioItem(nil), m.Samples...)
		bank := append([]byte(nil), fixture.Bank...)
		mutate(&m, bank)
		if err := ValidateDirectAudio(m, bank); err == nil {
			t.Fatal("invalid bank/manifest accepted")
		}
	}
	project := t.TempDir()
	if err := os.CopyFS(project, os.DirFS("../../examples/imported-audio")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "assets/audio/direct/manifest.json"), []byte(`{"version":1,"bank":"../../outside"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDirectAudio(project); err == nil || !strings.Contains(err.Error(), "SHA256") {
		t.Fatal(err)
	}
}
