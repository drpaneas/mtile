package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopPreviewROMUsesPrivateRunDirectory(t *testing.T) {
	project, cache := t.TempDir(), t.TempDir()
	release := filepath.Join(project, "game.gba")
	if err := os.WriteFile(release, []byte("release"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := prepareDevelopPreviewROM(cache, project)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareDevelopPreviewROM(cache, filepath.Join(project, "."))
	if err != nil {
		t.Fatal(err)
	}
	projectCache := filepath.Dir(filepath.Dir(first))
	if first == second || filepath.Dir(filepath.Dir(second)) != projectCache || filepath.Dir(projectCache) != cache {
		t.Fatalf("preview run paths %q and %q do not have separate runs in the same project cache", first, second)
	}
	other, err := prepareDevelopPreviewROM(cache, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(filepath.Dir(other)) == projectCache {
		t.Fatal("unrelated projects share a preview cache directory")
	}
	if err := os.WriteFile(first, []byte("preview"), 0o644); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(release); err != nil || string(content) != "release" {
		t.Fatalf("preview changed release ROM: %q, %v", content, err)
	}
}

func TestDevelopExportROMPathKeepsPreviewCachePrivate(t *testing.T) {
	cache, project := t.TempDir(), t.TempDir()
	preview, err := prepareDevelopPreviewROM(cache, project)
	if err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(project, "Α game.GBA")
	if path, err := validateDevelopExportROMPath(cache, valid); path != valid || err != nil {
		t.Fatalf("export path %q rejected or changed: %q, %v", valid, path, err)
	}
	for _, path := range []string{preview, filepath.Join(project, "game"), "game.gba"} {
		if path, err := validateDevelopExportROMPath(cache, path); path != "" || err == nil {
			t.Errorf("invalid destination accepted: %q, %v", path, err)
		}
	}
	alias := filepath.Join(project, "private-alias")
	if err := os.Symlink(filepath.Dir(preview), alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if path, err := validateDevelopExportROMPath(cache, filepath.Join(alias, "through-alias.gba")); path != "" || err == nil {
		t.Errorf("destination through a private cache alias accepted: %q, %v", path, err)
	}
}
