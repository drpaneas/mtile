package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckProjectLintUsesUnsavedModulesAndKeepsAdviceSeparate(t *testing.T) {
	root := checkProjectFixture(t)
	entry := "require(\"parts\")\nfunction _update() if ready then assemble() assemble() end end\nfunction _draw() cls(0) end\n"
	if err := os.WriteFile(filepath.Join(root, "game.lua"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	service := Service{UseProjectRenderer: true, TinyGoBin: filepath.Join(root, "missing-compiler")}
	overlay := map[string]string{"src/parts.lua": "function assemble() cache={} end\n"}
	analysis := service.CheckProject(root, overlay)
	if !analysis.LintReady || !analysis.RendererReady {
		t.Fatalf("source check failed: %+v", analysis)
	}
	count := 0
	for _, d := range analysis.Lint {
		if d.Rule == "MTL008" {
			count++
			if d.Path != "src/parts.lua" || len(d.Related) < 2 {
				t.Fatalf("lost module/related positions: %+v", d)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one discarded result: %+v", analysis.Lint)
	}
	for _, d := range analysis.Diagnostics {
		if d.Code == "MTL008" {
			t.Fatal("advice leaked into compile error diagnostics")
		}
	}
	for _, name := range []string{"src/parts.lua", "game.gba"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("source check wrote %s", name)
		}
	}
	overlay["game.lua"] = "require(\"parts\")\nfunction _update() if ready then assemble() end end\nfunction _draw() cls(0) end\n"
	fixed := service.CheckProject(root, overlay)
	for _, d := range fixed.Lint {
		if d.Rule == "MTL008" {
			t.Fatalf("old finding survived edit: %+v", d)
		}
	}
	overlay["src/parts.lua"] = "function assemble("
	broken := service.CheckProject(root, overlay)
	if broken.LintReady || len(broken.Lint) != 0 {
		t.Fatalf("invalid source retained findings: %+v", broken)
	}
}

func TestCheckProjectLintDoesNotRequireValidAssets(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "game.lua"), []byte("function _update() local scratch={} scratch={} end\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	analysis := (Service{UseProjectRenderer: true}).CheckProject(root, nil)
	if !analysis.LintReady || analysis.RendererReady || len(analysis.Diagnostics) == 0 {
		t.Fatalf("source-only advice should survive asset errors: %+v", analysis)
	}
	seen := false
	for _, d := range analysis.Lint {
		seen = seen || d.Rule == "MTL008"
	}
	if !seen {
		t.Fatal("asset error discarded independent source advice")
	}
}
