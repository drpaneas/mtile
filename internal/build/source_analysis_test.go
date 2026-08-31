package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestAnalyzeProjectSourcesReportsRequiredModulesAndRemappedGlobals(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "player.lua"), []byte("local player = {x = 1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "game.lua"), []byte("require(\"player\")\nscore = 0\nfunction _update() local frame_value = score end\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeProjectSources(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(analysis.SourceFiles, ","); got != "src/player.lua,game.lua" {
		t.Fatalf("source files = %q", got)
	}
	if analysis.Globals.Count != 2 || analysis.Globals.Limit != transpiler.GlobalLimit {
		t.Fatalf("global usage = %#v", analysis.Globals)
	}
	want := []GlobalSymbol{
		{Name: "player", Source: "src/player.lua", Line: 1, Column: 1},
		{Name: "score", Source: "game.lua", Line: 2, Column: 1},
	}
	for index, symbol := range analysis.Globals.Symbols {
		if symbol != want[index] {
			t.Fatalf("symbol %d = %#v, want %#v", index, symbol, want[index])
		}
	}
}

func TestAnalyzeProjectSourcesUsesEditorOverlay(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "game.lua"), []byte("score = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	analysis, err := AnalyzeProjectSources(root, map[string]string{"game.lua": "score = 0\nlives = 3\n"})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Globals.Count != 2 || analysis.Globals.Symbols[1].Name != "lives" {
		t.Fatalf("overlay global usage = %#v", analysis.Globals)
	}
}

func TestBuildReturnsGlobalUsageWhenLimitBlocksBeforeAssets(t *testing.T) {
	root := t.TempDir()
	var source strings.Builder
	for index := 0; index < transpiler.GlobalLimit+1; index++ {
		fmt.Fprintf(&source, "g%d = %d\n", index, index)
	}
	if err := os.WriteFile(filepath.Join(root, "game.lua"), []byte(source.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := (Service{}).Build(root, "")
	if err == nil || !strings.Contains(err.Error(), "maxGlobals") {
		t.Fatalf("Build error = %v, want global limit failure", err)
	}
	if result.GlobalUsage.Count != transpiler.GlobalLimit+1 || result.GlobalUsage.Limit != transpiler.GlobalLimit {
		t.Fatalf("partial build global usage = %#v", result.GlobalUsage)
	}
}

func TestGlobalLimitDiagnosticRemapsFirstExcessModulePosition(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	var module strings.Builder
	for index := 0; index < transpiler.GlobalLimit+1; index++ {
		fmt.Fprintf(&module, "g%d = %d\n", index, index)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "state.lua"), []byte(module.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "game.lua"), []byte("require(\"state\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	analysis, err := AnalyzeProjectSources(root, nil)
	if err == nil || !strings.Contains(err.Error(), `src/state.lua:257:1`) || !strings.Contains(err.Error(), `first excess global "g256"`) {
		t.Fatalf("global limit error = %v", err)
	}
	if analysis.Globals.Count != transpiler.GlobalLimit+1 || analysis.Globals.Symbols[transpiler.GlobalLimit].Source != "src/state.lua" {
		t.Fatalf("remapped overflow analysis = %#v", analysis.Globals)
	}
}

func TestExplicitMultiFileExampleSourceContract(t *testing.T) {
	projectDir := filepath.Join("..", "..", "examples", "multi-file")
	analysis, err := AnalyzeProjectSources(projectDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := "src/player.lua,src/hud.lua,game.lua"
	if got := strings.Join(analysis.SourceFiles, ","); got != wantFiles {
		t.Fatalf("example source order = %q, want %q", got, wantFiles)
	}
	if analysis.Globals.Count != 2 || len(analysis.Globals.Symbols) != 2 ||
		analysis.Globals.Symbols[0].Name != "player" || analysis.Globals.Symbols[1].Name != "hud" {
		t.Fatalf("example global usage = %#v", analysis.Globals)
	}
}
