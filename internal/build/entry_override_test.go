package build

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestEntryOverrideBuildsUnsavedSourcesWithoutChangingProject(t *testing.T) {
	root := checkProjectFixture(t)
	project, err := assetsync.LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	project.Entry = "regular.lua"
	if err := assetsync.SaveProject(root, project); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, project.Entry), []byte("function _draw() cls(0) end\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "project.json")
	before, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	const lesson = "lessons/intro.lua"
	overlay := map[string]string{
		lesson:                     "require('local_helper')\nrequire('shared')\nfunction _draw() cls(0) end\n",
		"lessons/local_helper.lua": "lesson_position = 1\n",
		"src/shared.lua":           "shared_position = 2\n",
	}
	wantSources := []string{"lessons/local_helper.lua", "src/shared.lua", lesson}
	var toolchainReached bool
	service := Service{
		EntryOverride: lesson, SourceOverlay: overlay,
		UseProjectRenderer: true, TinyGoBin: filepath.Join(t.TempDir(), "missing-tinygo"),
		Trace: func(event Event) { toolchainReached = toolchainReached || event.Phase == PhaseToolchain },
	}
	result, err := service.Build(root, "")
	if err == nil || !toolchainReached || result.Entry != lesson || !slices.Equal(result.SourceFiles, wantSources) {
		t.Fatalf("lesson build result = %+v, toolchain reached %t, error %v", result, toolchainReached, err)
	}
	analysis := service.CheckProject(root, overlay)
	if !analysis.RendererReady || analysis.Entry != lesson || !slices.Equal(analysis.SourceFiles, wantSources) {
		t.Fatalf("lesson check result = %+v", analysis)
	}
	if withoutOverlay := service.CheckProject(root, nil); withoutOverlay.RendererReady {
		t.Fatalf("CheckProject used the Build-only overlay: %+v", withoutOverlay)
	}
	if normal := (Service{UseProjectRenderer: true}).CheckProject(root, nil); !normal.RendererReady || normal.Entry != project.Entry {
		t.Fatalf("normal check did not keep project entry %q: %+v", project.Entry, normal)
	}
	service.EntryOverride = ""
	service.SourceOverlay = map[string]string{project.Entry: "unsaved_game = 3\nfunction _draw() cls(0) end\n"}
	toolchainReached = false
	result, err = service.Build(root, "")
	if err == nil || !toolchainReached || result.Entry != project.Entry || !slices.ContainsFunc(result.GlobalUsage.Symbols, func(symbol GlobalSymbol) bool {
		return symbol.Name == "unsaved_game"
	}) {
		t.Fatalf("normal build did not use the unsaved project entry: %+v, error %v", result, err)
	}
	after, err := os.ReadFile(projectPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("build changed project.json: %s, %v", after, err)
	}
	for name := range overlay {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Errorf("build wrote overlay source %s: %v", name, err)
		}
	}
}

func TestEntryOverrideRejectsUnsafePaths(t *testing.T) {
	root := checkProjectFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.lua")
	if err := os.WriteFile(outside, []byte("outside = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ entry, reason string }{
		{outside, "project-relative"},
		{"../outside.lua", "stay inside the project"},
		{"lessons/intro.txt", "must name a .lua file"},
		{"escape.lua", "symlink escapes the project"},
	} {
		t.Run(test.entry, func(t *testing.T) {
			if test.entry == "escape.lua" {
				if err := os.Symlink(outside, filepath.Join(root, test.entry)); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			service := Service{EntryOverride: test.entry}
			if _, err := service.Build(root, ""); err == nil || !strings.Contains(err.Error(), test.reason) {
				t.Fatalf("build error = %v, want %q", err, test.reason)
			}
			analysis := service.CheckProject(root, nil)
			if analysis.RendererReady || len(analysis.Diagnostics) != 1 || !strings.Contains(analysis.Diagnostics[0].Message, test.reason) {
				t.Fatalf("check result = %+v, want %q", analysis, test.reason)
			}
			if test.entry == "escape.lua" && analysis.Diagnostics[0].EndColumn != 1 {
				t.Fatalf("check read a diagnostic range from outside the project: %+v", analysis.Diagnostics[0])
			}
		})
	}
}
