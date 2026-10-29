package transpiler

import (
	"fmt"
	"strings"
	"testing"
)

func TestFontBShortGlyphsKeep32PairCapacity(t *testing.T) {
	source := "function _draw() cls() "
	for color := 1; color <= 32; color++ {
		source += fmt.Sprintf("print(\"A\",0,0,%d) ", color)
	}
	source += "end"
	_, report, err := TranspileWithReport("short-b.lua", source, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
	if err != nil || !report.Mode0Text {
		t.Fatalf("short glyph capacity regressed: %v", err)
	}
}
func TestFontBTallGlyphsKeepBounded16PairCapacity(t *testing.T) {
	for _, n := range []int{16, 17} {
		source := "function _draw() cls() "
		for color := 1; color <= n; color++ {
			source += fmt.Sprintf("print(\"g\",0,0,%d) ", color)
		}
		source += "end"
		_, _, err := TranspileWithReport("tall-b.lua", source, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
		if n == 16 && err != nil {
			t.Fatal(err)
		}
		if n == 17 && (err == nil || !strings.Contains(err.Error(), "M0_OBJ_BUDGET")) {
			t.Fatalf("unbounded tall bank: %v", err)
		}
	}
}

func TestFontBGeneratedMainSelectsFontBeforeAtlasLayout(t *testing.T) {
	generated, err := Transpile("startup-b.lua", `function _draw() cls() print("Ag",0,0,7) end`, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	main := generated[strings.Index(generated, "func main()"):]
	selected, atlas := strings.Index(main, "SetFont(&projectFontROM)"), strings.Index(main, "LoadMode0TextAtlas(")
	if selected < 0 || selected > atlas {
		t.Fatal("atlas layout was validated before the selected font was installed")
	}
}
