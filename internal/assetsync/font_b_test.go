package assetsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fontBFixture() *Font {
	f := &Font{Version: 2, Name: "test-b", CellWidth: 6, CellHeight: 10, Width: 6, Height: 10}
	for cp := 32; cp <= 126; cp++ {
		cols := make([]int, 6)
		if cp == '?' {
			cols[0] = 1
		}
		f.Glyphs = append(f.Glyphs, FontGlyph{Codepoint: cp, Columns: cols})
	}
	f.Glyphs['A'-32].Columns[5] = 1 << 9
	return f
}
func TestFontBStrictShapeAndTenBitColumns(t *testing.T) {
	f := fontBFixture()
	if err := ValidateFont(f); err != nil {
		t.Fatalf("chosen B shape rejected: %v", err)
	}
	compiled, err := CompileFont(f)
	if err != nil {
		t.Fatal(err)
	}
	cols := compiled.Columns('A')
	last := 5
	if len(cols) != 6 || int(cols[last]) != 512 {
		t.Fatal("sixth column / tenth row truncated")
	}
	declaration := FontGoDeclaration(compiled)
	if !strings.Contains(declaration, "[95][6]uint16") || !strings.Contains(declaration, "512") {
		t.Fatal("generated ROM truncates ten-bit columns")
	}
	for _, change := range []func(*Font){func(f *Font) { f.Width = 5 }, func(f *Font) { f.Height = 11 }, func(f *Font) { f.CellHeight = 8 }, func(f *Font) { f.CellWidth = 7 }, func(f *Font) { f.Glyphs[1].Columns = make([]int, 7) }, func(f *Font) { f.Glyphs[1].Columns[0] = 1024 }, func(f *Font) { f.Glyphs[1].Columns[0] = -1 }, func(f *Font) { f.Version = 1 }} {
		bad := fontBFixture()
		change(bad)
		if err := ValidateFont(bad); err == nil {
			t.Fatal("invalid B shape accepted")
		}
	}
}
func TestFontBPresetsMatchApprovedArtwork(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "research", "font-typography", "6x10.json"))
	if err != nil {
		t.Fatal(err)
	}
	var approved Font
	if err = json.Unmarshal(raw, &approved); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFont("", "international")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 || got.Width != 6 || got.Height != 10 || got.CellWidth != 6 || got.CellHeight != 10 {
		t.Fatalf("preset has version %d, bitmap %dx%d, cell %dx%d", got.Version, got.Width, got.Height, got.CellWidth, got.CellHeight)
	}
	if !reflect.DeepEqual(got.Glyphs, approved.Glyphs) {
		t.Fatal("international preset differs from approved B bitmap artwork")
	}
	defaultFont, err := LoadFont("", "")
	if err != nil {
		t.Fatal(err)
	}
	if defaultFont.Name != "arcade" {
		t.Fatal("default is not the ASCII-only Arcade B font")
	}
	for _, name := range BuiltinFontNames {
		f, err := LoadFont("", name)
		if err != nil {
			t.Fatal(err)
		}
		if f.Version != 2 || f.Width != 6 || f.Height != 10 || f.CellHeight != 10 {
			t.Fatalf("%s bypasses fixed B metrics", name)
		}
	}
}
func TestFontBLegacyReadPreservesDataWithoutRewriting(t *testing.T) {
	dir := t.TempDir()
	old := contractFont()
	old.Name = "legacy"
	if err := SaveFont(dir, old); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "assets", "fonts", "legacy.json")
	before, _ := os.ReadFile(path)
	loaded, err := LoadFont(dir, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) || !reflect.DeepEqual(old, loaded) {
		t.Fatal("opening a legacy font rewrote/lost authored data")
	}
	compiled, err := CompileFont(loaded)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range old.Glyphs {
		cols := compiled.Columns(rune(g.Codepoint))
		if len(cols) != 6 {
			t.Fatal("legacy font was not padded into current ROM format")
		}
		for x, v := range g.Columns {
			if int(cols[x]) != v {
				t.Fatal("legacy pixels changed")
			}
		}
		for x := 4; x < 6; x++ {
			if cols[x] != 0 {
				t.Fatal("legacy padding added ink")
			}
		}
	}
}

func TestFontBLegacyMetadataSurvivesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f := contractFont()
	f.Name = "legacy"
	raw, _ := json.Marshal(f)
	var document map[string]any
	json.Unmarshal(raw, &document)
	document["designerNote"] = "keep this"
	document["glyphs"].([]any)[1].(map[string]any)["provenance"] = "original"
	raw, _ = json.Marshal(document)
	path := filepath.Join(dir, "assets", "fonts", "legacy.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFont(dir, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err = SaveFont(dir, loaded); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(path)
	var result map[string]any
	json.Unmarshal(saved, &result)
	if result["designerNote"] != "keep this" || result["glyphs"].([]any)[1].(map[string]any)["provenance"] != "original" {
		t.Fatal("font metadata lost during migration/read/save")
	}
}
