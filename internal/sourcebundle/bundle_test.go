package sourcebundle

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
)

func writeSource(t *testing.T, root, relative, source string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBundleRemapsParserAndSemanticDiagnostics(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "src/player.lua", "score = (\n")
	writeSource(t, root, "game.lua", `require("player")`+"\n")
	// Resolve parses each unit first, so use a valid graph to inspect mapping.
	writeSource(t, root, "src/player.lua", "score = 1\n")
	bundle, err := Resolve(root, "game.lua")
	if err != nil {
		t.Fatal(err)
	}
	span := bundle.Spans[0]
	parse := &luaparser.Error{Pos: luaparser.Position{Source: bundle.Entry, Line: span.ContentStart, Column: 4}, Message: "broken"}
	remapped := bundle.RemapError(parse)
	var got *luaparser.Error
	if !errors.As(remapped, &got) || got.Pos.Source != "src/player.lua" || got.Pos.Line != 1 || got.Pos.Column != 4 {
		t.Fatalf("parser remap = %#v / %v", got, remapped)
	}
	semantic := bundle.RemapError(errors.New("transpiler: " + bundle.Entry + ":" + strconv.Itoa(span.ContentStart) + ":7: bad value"))
	if !strings.Contains(semantic.Error(), "src/player.lua:1:7") {
		t.Fatalf("semantic remap = %v", semantic)
	}
}

func TestResolveLegacySingleFile(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "game.lua", "score = 0\n")
	bundle, err := Resolve(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Entry != "game.lua" || len(bundle.Files) != 1 || bundle.Files[0] != "game.lua" {
		t.Fatalf("bundle metadata = %#v", bundle)
	}
	if !strings.Contains(bundle.Source, "score = 0") {
		t.Fatalf("bundle source = %q", bundle.Source)
	}
}

func TestResolveDependenciesDepthFirstOnce(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "src/shared.lua", "shared = 1\n")
	writeSource(t, root, "src/player.lua", "require(\"shared\")\nplayer = shared + 1\n")
	writeSource(t, root, "game.lua", "require \"player\"\nrequire(\"shared\") -- once\nstart = player\n")
	bundle, err := Resolve(root, "game.lua")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"src/shared.lua", "src/player.lua", "game.lua"}
	if strings.Join(bundle.Files, ",") != strings.Join(want, ",") {
		t.Fatalf("files = %#v, want %#v", bundle.Files, want)
	}
	if strings.Contains(bundle.Source, "require") {
		t.Fatalf("static require leaked into transpiler source:\n%s", bundle.Source)
	}
	if strings.Count(bundle.Source, "shared = 1") != 1 {
		t.Fatalf("dependency was not deduplicated:\n%s", bundle.Source)
	}
}

func TestResolveRejectsMissingAndTraversal(t *testing.T) {
	for _, source := range []string{`require("missing")`, `require("../outside")`} {
		root := t.TempDir()
		writeSource(t, root, "game.lua", source+"\n")
		if _, err := Resolve(root, "game.lua"); err == nil {
			t.Fatalf("Resolve(%q) succeeded", source)
		}
	}
}

func TestResolveRejectsCycle(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "game.lua", `require("a")`+"\n")
	writeSource(t, root, "src/a.lua", `require("b")`+"\n")
	writeSource(t, root, "src/b.lua", `require("a")`+"\n")
	_, err := Resolve(root, "game.lua")
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestResolveRejectsNestedRequire(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "game.lua", "function load()\n  require(\"player\")\nend\n")
	writeSource(t, root, "src/player.lua", "player = 1\n")
	_, err := Resolve(root, "game.lua")
	if err == nil || !strings.Contains(err.Error(), "top-level") {
		t.Fatalf("nested require error = %v", err)
	}
}

func TestResolveRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeSource(t, outside, "secret.lua", "secret = 1\n")
	if err := os.Symlink(filepath.Join(outside, "secret.lua"), filepath.Join(root, "escape.lua")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeSource(t, root, "game.lua", `require("escape")`+"\n")
	if _, err := Resolve(root, "game.lua"); err == nil || !strings.Contains(err.Error(), "symlink escapes") {
		t.Fatalf("symlink error = %v", err)
	}
}
