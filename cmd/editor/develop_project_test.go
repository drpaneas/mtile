package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/codeeditor"
	projecteditor "github.com/drpaneas/mtile/internal/editor"
)

func TestSaveCodeDocumentRefusesExternalOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.lua")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	document := &codeDocument{Path: path, Relative: "game.lua", Editor: codeeditor.New("original\n"), DiskSource: "original\n"}
	document.Editor.Insert("local edit\n")
	if err := os.WriteFile(path, []byte("external\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &Game{}
	if g.saveCodeDocument(document) {
		t.Fatal("save overwrote an external edit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "external\n" || !strings.Contains(g.statusMsg, "changed outside") {
		t.Fatalf("disk=%q status=%q", data, g.statusMsg)
	}
}

func TestSaveCodeDocumentUpdatesDiskBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.lua")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	document := &codeDocument{Path: path, Relative: "game.lua", Editor: codeeditor.New("original\n"), DiskSource: "original\n"}
	document.Editor.Insert("saved\n")
	g := &Game{}
	if !g.saveCodeDocument(document) {
		t.Fatal(g.statusMsg)
	}
	if document.Editor.Dirty() || document.DiskSource != document.Editor.Text() {
		t.Fatalf("saved document state = dirty:%t baseline:%q", document.Editor.Dirty(), document.DiskSource)
	}
}

func TestReloadActiveCodeDocumentPreservesExternalEdit(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	t.Cleanup(g.closeNativePerformance)
	g.code.Insert("local_edit = 1\n")
	g.codeScrollLine, g.codeScrollColumn = 4, 8
	const external = "external_edit = 2\n"
	if err := os.WriteFile(g.codePath, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}
	if g.saveCode() {
		t.Fatal("save should report a conflict before reload")
	}

	g.reloadActiveCodeDocument()
	if g.code.Text() != external || g.code.Dirty() {
		t.Errorf("reloaded text = %q, dirty = %t", g.code.Text(), g.code.Dirty())
	}
	if g.code.Cursor() != 0 || g.codeScrollLine != 0 || g.codeScrollColumn != 0 {
		t.Errorf("reload left the cursor or viewport at the previous position")
	}
	if !g.saveCode() {
		t.Fatal(g.statusMsg)
	}
	data, err := os.ReadFile(g.codePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != external {
		t.Fatalf("save after reload overwrote external text: %q", data)
	}
}

func TestReloadActiveCodeDocumentClearsDiagnosticsForEmptyFile(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	t.Cleanup(g.closeNativePerformance)
	g.code.SelectAll()
	g.code.Insert("function _draw(\n")
	g.refreshCodeDiagnostics()
	if g.codeDiagnosticLine == 0 {
		t.Fatal("expected a parser error before reload")
	}
	if err := os.WriteFile(g.codePath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	g.reloadActiveCodeDocument()
	if !g.code.Dirty() {
		t.Fatal("reload discarded local changes before confirmation")
	}
	g.reloadActiveCodeDocument()
	if g.code.Text() != "" || g.code.Dirty() {
		t.Errorf("reloaded text = %q, dirty = %t", g.code.Text(), g.code.Dirty())
	}
	if g.codeDiagnosticLine != 0 || g.codeDiagnosticColumn != 0 {
		t.Fatalf("reload retained the old diagnostic: %s", g.codeDiagnostic)
	}
}

func TestProjectSourceAnalysisIncludesDirtyOpenModules(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	gamePath := filepath.Join(root, "game.lua")
	playerPath := filepath.Join(root, "src", "player.lua")
	if err := os.WriteFile(gamePath, []byte("require(\"player\")\nscore = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(playerPath, []byte("player = {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gameEditor := codeeditor.New("require(\"player\")\nscore = 0\n")
	playerEditor := codeeditor.New("player = {}\nlives = 3\n")
	g := &Game{
		model:    &projecteditor.Model{Dir: root},
		codePath: gamePath,
		code:     gameEditor,
		codeDocuments: map[string]*codeDocument{
			gamePath:   {Path: gamePath, Relative: "game.lua", Editor: gameEditor},
			playerPath: {Path: playerPath, Relative: "src/player.lua", Editor: playerEditor},
		},
	}
	usage, err := g.refreshProjectSourceAnalysis()
	if err != nil {
		t.Fatal(err)
	}
	if usage.Count != 3 || usage.Symbols[1].Name != "lives" || usage.Symbols[1].Source != "src/player.lua" {
		t.Fatalf("dirty-buffer global usage = %#v", usage)
	}
}
