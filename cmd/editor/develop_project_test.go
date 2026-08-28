package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/codeeditor"
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
