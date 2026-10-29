package transpiler

import (
	"fmt"
	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/luaparser"
	rt "github.com/drpaneas/mtile/runtime"
	"strings"
	"testing"
)

func TestSelectedFontMode0PixelParity(t *testing.T) {
	for _, name := range assetsync.BuiltinFontNames {
		f, _ := assetsync.LoadFont("", name)
		font, _ := assetsync.CompileFont(f)
		for cp := byte(32); cp <= 126; cp++ {
			for _, color := range []byte{1, 7, 255} {
				key := rt.Mode0TextGlyphKey(cp, color)
				atlas := atlasFromMode0TextKeys([]uint16{key}, font)
				var c rt.Context
				c.SetFont(font)
				c.Print(string(cp), 0, 0, int(color))
				for y := 0; y < 8; y++ {
					for x := 0; x < 8; x++ {
						off := y*8 + x
						got := byte(atlas.tiles[off/4] >> uint(off%4*8))
						want := byte(c.Pget(x, y))
						if got != want {
							t.Fatalf("%s U+%04X color %d pixel %d,%d=%d want=%d", name, cp, color, x, y, got, want)
						}
					}
				}
			}
		}
	}
}
func TestFontUnicodeRendererContract(t *testing.T) {
	f, _ := assetsync.LoadFont("", "international")
	font, _ := assetsync.CompileFont(f)
	for _, text := range []string{"Ελλάδα", "Grüße"} {
		source := fmt.Sprintf("function _draw()\n cls()\n print(%q,0,0,7)\nend", text)
		_, report, err := TranspileWithReport("font.lua", source, Options{SheetW: 8, SheetH: 8, Font: font, Renderer: RendererAuto})
		if err != nil {
			t.Fatal(err)
		}
		if report.Selected != RendererMode4 {
			t.Fatal("Unicode admitted to Mode0")
		}
		_, _, err = TranspileWithReport("font.lua", source, Options{SheetW: 8, SheetH: 8, Font: font, Renderer: RendererMode0})
		if err == nil || !strings.Contains(err.Error(), "M0_PRINT_SHAPE") || !strings.Contains(err.Error(), "font.lua:3:") {
			t.Fatalf("diagnostic=%v", err)
		}
	}
	source := `function _draw() cls() print("AB",0,0,7) end`
	chunk, _ := luaparser.ParseString("font.lua", source)
	draw := mode0TextTestDraw(t, source)
	atlas := buildMode0TextAtlasForSource(chunk, draw, Options{Font: font})
	want := rt.BuildMode0GlyphTileWithFont(font, 'A', 7)
	if byte(atlas.tiles[0]) != want[0] {
		t.Fatal("selected font lost in source atlas")
	}
	generated, report, err := TranspileWithReport("font.lua", source, Options{SheetW: 8, SheetH: 8, Font: font, Renderer: RendererAuto})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 {
		t.Fatal("selected ASCII font rejected")
	}
	if strings.Count(generated, "var projectFontROM =") != 1 || strings.Contains(generated, "builtinFont") {
		t.Fatal("font ROM selection")
	}
}
func TestGeneratedUnicodeFontExecutes(t *testing.T) {
	f, _ := assetsync.LoadFont("", "international")
	font, _ := assetsync.CompileFont(f)
	generated, err := Transpile("font.lua", `function _draw() result=print("Ωä",0,0,7) end`, Options{Font: font})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestFontRun(t *testing.T){var c rt.Context;var g Game;g.Init(&c);g.Draw(&c);if g.result!=rt.IntVal(12){t.Fatal("UTF8 advance")};if c.Pget(1,0)!=7{t.Fatal("selected Ω missing")}}`, "TestFontRun")
}
func BenchmarkSelectedFontAtlas(b *testing.B) {
	f, _ := assetsync.LoadFont("", "arcade")
	font, _ := assetsync.CompileFont(f)
	keys := []uint16{rt.Mode0TextGlyphKey('A', 7), rt.Mode0TextGlyphKey('1', 7)}
	for i := 0; i < b.N; i++ {
		atlasFromMode0TextKeys(keys, font)
	}
}
