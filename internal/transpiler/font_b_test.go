package transpiler

import (
	"github.com/drpaneas/mtile/internal/assetsync"
	rt "github.com/drpaneas/mtile/runtime"
	"strings"
	"testing"
)

func TestFontBMode0AtlasStoresBothTileRows(t *testing.T) {
	f, err := assetsync.LoadFont("", "international")
	if err != nil {
		t.Fatal(err)
	}
	font, err := assetsync.CompileFont(f)
	if err != nil {
		t.Fatal(err)
	}
	key := rt.Mode0TextGlyphKey('g', 7)
	atlas := atlasFromMode0TextKeys([]uint16{key}, font)
	if len(atlas.tiles) != 32 {
		t.Fatalf("B glyph needs two 64-byte tiles; got %d words", len(atlas.tiles))
	}
	var c rt.Context
	c.SetFont(font)
	c.Print("g", 0, 0, 7)
	for y := 0; y < 16; y++ {
		for x := 0; x < 8; x++ {
			off := y*8 + x
			got := byte(atlas.tiles[off/4] >> uint(off%4*8))
			if got != c.Pget(x, y) {
				t.Fatalf("tile pair mismatch at %d,%d", x, y)
			}
		}
	}
}
func TestFontBMode0PairCapacityAndScanlineOverlap(t *testing.T) {
	for _, count := range []int{16, 17} {
		source := "function _draw()\n cls()\n print(\"" + "ABCDEFGHIJKLMNOPQ"[:count] + "\",0,0,7)\nend"
		_, report, err := TranspileWithReport("font-b.lua", source, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
		if count == 16 {
			if err != nil || !report.Mode0Text {
				t.Fatalf("16 pairs rejected: %v", err)
			}
		} else if err == nil || !strings.Contains(err.Error(), "16") {
			t.Fatalf("17 glyphs must reject bounded 32-tile bank: %v", err)
		}
	}
}
func TestFontBMode0ScanlineOverlap(t *testing.T) {
	source := "function _draw() cls() print(\"" + strings.Repeat("A", 40) + "\",0,0,7) print(\"" + strings.Repeat("A", 40) + "\",0,10,7) end"
	_, report, err := TranspileWithReport("font-b.lua", source, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OBJLineCycles != 640 {
		t.Fatalf("overlapping 8x16 objects budgeted %d cycles, want 640", report.Mode0OBJLineCycles)
	}
}
