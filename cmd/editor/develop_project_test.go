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
