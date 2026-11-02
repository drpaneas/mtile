package audioimport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestDeleteRebuildsBankKeepsIDsAndRestoresSameName(t *testing.T) {
	t.Setenv("MTILE_IMPORT_HELPER", "1")
	tool, _ := os.Executable()
	project, input := t.TempDir(), t.TempDir()
	write := func(name string) string {
		path := filepath.Join(input, name)
		if err := os.WriteFile(path, []byte("ok"), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	paths := []string{write("first.wav"), write("second.wav"), write("song.mod"), write("next.mod")}
	first, err := Import(project, paths, tool)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := Delete(project, false, 0, tool)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted.Manifest.Samples[0].Deleted || deleted.Manifest.Samples[1].Name != "second" || deleted.Manifest.Samples[1].BankID != 0 {
		t.Fatal("public IDs moved or bank not compacted")
	}
	if _, err := os.Stat(filepath.Join(project, "assets/audio/direct/source/sfx_first.wav")); !os.IsNotExist(err) {
		t.Fatal("deleted source retained", err)
	}
	if _, err := os.Stat(filepath.Join(project, "assets/audio/direct", first.Manifest.Bank)); !os.IsNotExist(err) {
		t.Fatal("old bank retained", err)
	}
	if _, err = Delete(project, true, 0, tool); err != nil {
		t.Fatal(err)
	}
	restored, err := Import(project, []string{paths[0], paths[2]}, tool)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Manifest.Samples[0].Deleted || restored.Manifest.Music[0].Deleted || restored.Manifest.Music[1].Name != "next" {
		t.Fatal("same-name import did not restore original ID")
	}
	for _, music := range []bool{true, false} {
		for id := 0; id < 2; id++ {
			if _, err = Delete(project, music, id, tool); err != nil {
				t.Fatal(err)
			}
		}
	}
	empty, err := assetsync.LoadDirectAudio(project)
	if err != nil || len(empty.Bank) != 12 {
		t.Fatal("final delete failed", err)
	}
	if _, err = Delete(project, false, 0, tool); err == nil {
		t.Fatal("deleted slot accepted")
	}
}

func TestFailedDeleteKeepsOldManifestBankAndSources(t *testing.T) {
	t.Setenv("MTILE_IMPORT_HELPER", "1")
	tool, _ := os.Executable()
	project, input := t.TempDir(), t.TempDir()
	var files []string
	for _, name := range []string{"first.wav", "second.wav"} {
		path := filepath.Join(input, name)
		os.WriteFile(path, []byte("ok"), 0600)
		files = append(files, path)
	}
	if _, err := Import(project, files, tool); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(project, "assets/audio/direct/manifest.json")
	before, _ := os.ReadFile(manifest)
	source := filepath.Join(project, "assets/audio/direct/source/sfx_second.wav")
	if err := os.WriteFile(source, []byte("FAIL"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Delete(project, false, 0, tool); err == nil {
		t.Fatal("converter error ignored")
	}
	after, _ := os.ReadFile(manifest)
	if string(after) != string(before) {
		t.Fatal("failed delete published manifest")
	}
	if _, err := os.Stat(filepath.Join(project, "assets/audio/direct/source/sfx_first.wav")); err != nil {
		t.Fatal("failed delete removed source")
	}
	if _, err := assetsync.LoadDirectAudio(project); err != nil {
		t.Fatal("old bank no longer playable", err)
	}
}
