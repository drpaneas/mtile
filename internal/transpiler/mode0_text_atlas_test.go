package transpiler

import (
	"fmt"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

func mode0TextTestDraw(t *testing.T, source string) *luaparser.FunctionDeclStmt {
	t.Helper()
	chunk, err := luaparser.ParseString("atlas.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range chunk.Body {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok && functionKey(fn) == "_draw" {
			return fn
		}
	}
	t.Fatal("test source has no _draw")
	return nil
}

func mode0TextColorCalls(count int) string {
	var source strings.Builder
	source.WriteString("function _draw()\n  cls()\n")
	for color := 1; color <= count; color++ {
		fmt.Fprintf(&source, "  print(\"A\",0,0,%d)\n", color)
	}
	source.WriteString("end\n")
	return source.String()
}

func TestMode0TextAtlasDeduplicatesRepeatedGlyphAndColor(t *testing.T) {
	draw := mode0TextTestDraw(t, `function _draw() cls() print("AAAA",0,0,7) end`)
	keys := mode0TextAtlasKeysForDraw(draw)
	if len(keys) != 1 || keys[0] != runtimeapi.Mode0TextGlyphKey('A', 7) {
		t.Fatalf("keys = %#v, want one A/color-7 key", keys)
	}
}

func TestMode0TextAtlasSeparatesOneGlyphAcrossColors(t *testing.T) {
	draw := mode0TextTestDraw(t, `function _draw() cls() print("A",0,0,12) print("A",8,0,7) end`)
	keys := mode0TextAtlasKeysForDraw(draw)
	want := []uint16{runtimeapi.Mode0TextGlyphKey('A', 7), runtimeapi.Mode0TextGlyphKey('A', 12)}
	if len(keys) != len(want) || keys[0] != want[0] || keys[1] != want[1] {
		t.Fatalf("keys = %#v, want %#v", keys, want)
	}
}

func TestMode0TextAtlasSortsKeysDeterministically(t *testing.T) {
	draw := mode0TextTestDraw(t, `function _draw() cls() print("BA",0,0,7) print("A",0,8,3) end`)
	keys := mode0TextAtlasKeysForDraw(draw)
	want := []uint16{
		runtimeapi.Mode0TextGlyphKey('A', 3),
		runtimeapi.Mode0TextGlyphKey('A', 7),
		runtimeapi.Mode0TextGlyphKey('B', 7),
	}
	if len(keys) != len(want) {
		t.Fatalf("keys = %#v, want %#v", keys, want)
	}
	for index := range want {
		if keys[index] != want[index] {
			t.Fatalf("keys = %#v, want %#v", keys, want)
		}
	}
}

func TestMode0TextAtlasBuildsExactRuntimeTileBytes(t *testing.T) {
	draw := mode0TextTestDraw(t, `function _draw() cls() print("A",0,0,7) end`)
	atlas := buildMode0TextAtlas(draw)
	want := runtimeapi.BuildMode0GlyphTile('A', 7)
	wantFirst := uint32(want[0]) | uint32(want[1])<<8 | uint32(want[2])<<16 | uint32(want[3])<<24
	if len(atlas.keys) != 1 || len(atlas.tiles) != 16 || atlas.tiles[0] != wantFirst {
		t.Fatalf("atlas sizes keys=%d tiles=%d or tile data mismatch", len(atlas.keys), len(atlas.tiles))
	}
}

func TestGeneratedMode0EmbedsAndLoadsStaticTextAtlas(t *testing.T) {
	generated, report, err := TranspileWithReport("atlas.lua", `function _draw() cls() print("AAAA",0,0,7) end`, Options{
		Renderer: RendererMode0, SheetW: 8, SheetH: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Mode0Text || !strings.Contains(generated, "var mode0TextKeysROM = [1]uint16{0x4107}") ||
		!strings.Contains(generated, "var mode0TextTilesROM = [16]uint32{") ||
		!strings.Contains(generated, "ctx.LoadMode0TextAtlas(mode0TextKeysROM[:], mode0TextTilesROM[:])") {
		t.Fatalf("generated Mode 0 source omitted the one-tile static atlas:\n%s", generated)
	}
}

func TestGeneratedMode4OmitsMode0TextAtlas(t *testing.T) {
	generated, report, err := TranspileWithReport("mode4.lua", `function _draw() cls() print("AAAA",0,0,7) end`, Options{
		Renderer: RendererMode4, SheetW: 8, SheetH: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || strings.Contains(generated, "mode0TextKeysROM") || strings.Contains(generated, "LoadMode0TextAtlas") {
		t.Fatalf("Mode 4 unexpectedly contains Mode 0 atlas state")
	}
}

func TestMode0TextAtlasAccepts128RepeatedOccurrences(t *testing.T) {
	source := fmt.Sprintf("function _draw() cls() print(%q,0,0,7) end", strings.Repeat("A", 128))
	_, report, err := TranspileWithReport("text-128.lua", source, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 128 || report.Mode0OBJLineCycles != 320 {
		t.Fatalf("report = %+v, want 128 OAM entries and wrapped 320-cycle peak", report)
	}
}

func TestMode0TextAtlasRejects33UniqueGlyphColorTiles(t *testing.T) {
	_, _, err := TranspileWithReport("text-colors-33.lua", mode0TextColorCalls(33), Options{
		Renderer: RendererMode0, SheetW: 8, SheetH: 8,
	})
	if err == nil || !strings.Contains(err.Error(), "[M0_OBJ_BUDGET]") || !strings.Contains(err.Error(), "33 unique glyph/color tiles") {
		t.Fatalf("error = %v, want stable unique-tile capacity rejection", err)
	}
}

func TestAutoFallsBackFor33UniqueGlyphColorTiles(t *testing.T) {
	_, report, err := TranspileWithReport("text-colors-33-auto.lua", mode0TextColorCalls(33), Options{
		Renderer: RendererAuto, SheetW: 8, SheetH: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || report.Reason != reasonAutoMode0Ineligible ||
		!strings.Contains(report.Diagnostic, "[M0_OBJ_BUDGET]") || !strings.Contains(report.Diagnostic, "33 unique glyph/color tiles") {
		t.Fatalf("AUTO report = %+v, want stable unique-tile fallback", report)
	}
}

func TestMode0TextAtlasStillRejects129RepeatedOAMEntries(t *testing.T) {
	source := fmt.Sprintf("function _draw() cls() print(%q,0,0,7) end", strings.Repeat("A", 129))
	_, _, err := TranspileWithReport("text-129.lua", source, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
	if err == nil || !strings.Contains(err.Error(), "[M0_OBJ_BUDGET]") || !strings.Contains(err.Error(), "129 OAM entries") {
		t.Fatalf("error = %v, want OAM occurrence rejection independent of one interned tile", err)
	}
}
