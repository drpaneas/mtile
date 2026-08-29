package transpiler

import (
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
)

func TestMode0AcceptsStaticDefaultMapProof(t *testing.T) {
	_, report, err := TranspileWithReport("map.lua", `
function _draw()
  cls(225)
  map()
end
`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 60, MapH: 20})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Reason != reasonMode0ForcedProven || report.Mode0Layers != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestMode0CountsDirectMapLayersAndPlansTileVRAM(t *testing.T) {
	_, report, err := TranspileWithReport("layers.lua", `
function _draw()
  cls()
  map(0,0,0,0,30,20,1)
  map(30,0,0,0,30,20,2)
  map(60,0,0,0,30,20,4)
  map(90,0,0,0,30,20,8)
end
`, Options{Renderer: RendererMode0, SheetW: 192, SheetH: 256, MapW: 120, MapH: 20}) // 768 tiles
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0Layers != 4 {
		t.Fatalf("layers = %d, want 4", report.Mode0Layers)
	}

	_, _, err = TranspileWithReport("too-many-tiles.lua", `
function _draw()
  cls()
  map()
  map()
  map()
  map()
end
`, Options{Renderer: RendererMode0, SheetW: 200, SheetH: 256, MapW: 30, MapH: 20}) // 800 tiles
	if err == nil || !strings.Contains(err.Error(), "M0_RESOURCE_LIMIT") || !strings.Contains(err.Error(), "768") {
		t.Fatalf("resource error = %v", err)
	}
}

func TestMode0RejectsFramebufferBuiltinWithStableReason(t *testing.T) {
	_, _, err := TranspileWithReport("draw.lua", `
function _draw()
  cls()
  map()
  pset(1, 2, 3)
end
`, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8, MapW: 30, MapH: 20})
	if err == nil || !strings.Contains(err.Error(), "[M0_MODE4_BUILTIN]") || !strings.Contains(err.Error(), "draw.lua:5") {
		t.Fatalf("error = %v", err)
	}
}

func TestMode0RejectsConditionalOrOversizedMap(t *testing.T) {
	tests := []struct {
		name   string
		source string
		code   string
	}{
		{"conditional", `function _draw() cls() if btn(0) then map() end end`, "M0_DRAW_SHAPE"},
		{"dynamic-width", `function _draw() cls() map(1, 2, 3, 4, width, 20) end`, "M0_MAP_SHAPE"},
		{"wide-window", `function _draw() cls() map(1, 2, 3, 4, 65, 20) end`, "M0_MAP_SHAPE"},
		{"tall-window", `function _draw() cls() map(1, 2, 3, 4, 30, 33) end`, "M0_MAP_SHAPE"},
		{"wrong-order", `function _draw() map() cls() end`, "M0_CLS_SHAPE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := TranspileWithReport(test.name+".lua", test.source, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8, MapW: 30, MapH: 20})
			if err == nil || !strings.Contains(err.Error(), "["+test.code+"]") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestMode0AcceptsCameraLargeMapAndMutations(t *testing.T) {
	_, report, err := TranspileWithReport("dynamic.lua", `
function _update()
  camera(t(), 3)
  sset(8, 0, 12)
  mset(200, 80, 1)
  fset(1, 2, true)
end
function _draw()
  cls(2)
  map(180, 70, -3, 5, 31, 21, 4)
end
`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 256, MapH: 128})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestMode0BoundsVisibleSsetUploads(t *testing.T) {
	_, _, err := TranspileWithReport("nested-sset.lua", `
function paint()
  sset(8,0,12)
end
function _update()
  paint()
end
function _draw()
  cls()
  map()
end
`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20})
	if err == nil || !strings.Contains(err.Error(), "M0_MUTABLE_SHEET") {
		t.Fatalf("nested sset error = %v", err)
	}

	calls := strings.Repeat("  sset(8,0,12)\n", 17)
	_, _, err = TranspileWithReport("many-sset.lua", "function _update()\n"+calls+"end\nfunction _draw() cls() map() end\n",
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20})
	if err == nil || !strings.Contains(err.Error(), "M0_RESOURCE_LIMIT") || !strings.Contains(err.Error(), "at most 16") {
		t.Fatalf("sset budget error = %v", err)
	}
}

func TestMode0AcceptsBoundedHardwareSpritesAfterMaps(t *testing.T) {
	_, report, err := TranspileWithReport("sprites.lua", `
function _draw()
  cls()
  map()
  spr(9, px, py, 2, 2, flip, false)
  spr(11, 40, 24)
end
`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Mode0Objects || report.Mode0Layers != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestMode0RejectsUnsafeHardwareSpriteShapes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		code   string
	}{
		{"before-map", `function _draw() cls() spr(1,0,0) map() end`, "M0_SPR_SHAPE"},
		{"conditional", `function _draw() cls() map() if btn(0) then spr(1,0,0) end end`, "M0_DRAW_SHAPE"},
		{"dynamic-id", `function _draw() cls() map() spr(player,0,0) end`, "M0_SPR_SHAPE"},
		{"obj-vram", `function _draw() cls() map() spr(512,0,0) end`, "M0_SPR_SHAPE"},
		{"scanline-budget", `function _draw() cls() map()
spr(1,0,0,4,1) spr(1,0,0,4,1) spr(1,0,0,4,1) spr(1,0,0,4,1) spr(1,0,0,4,1)
spr(1,0,0,4,1) spr(1,0,0,4,1) spr(1,0,0,4,1) spr(1,0,0,4,1) end`, "M0_OBJ_BUDGET"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := TranspileWithReport(test.name+".lua", test.source,
				Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20})
			if err == nil || !strings.Contains(err.Error(), "["+test.code+"]") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestMode0AcceptsBoundedStaticObjectText(t *testing.T) {
	_, report, err := TranspileWithReport("text.lua", `
function _draw()
  cls()
  map()
  spr(9, 8, 8)
  print("MODE 0", px, 4, 12)
end
`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Mode0Objects || !report.Mode0Text {
		t.Fatalf("report = %+v", report)
	}
}

func TestMode0RejectsDynamicOrControlCodeText(t *testing.T) {
	tests := []string{
		`function _draw() cls() map() print(label,0,0,7) end`,
		"function _draw() cls() map() print(\"line\\n\",0,0,7) end",
		`function _draw() cls() map() print("text",0,0,color) end`,
		`function _draw() cls() map() print("implicit") end`,
	}
	for index, source := range tests {
		_, _, err := TranspileWithReport("bad-text.lua", source,
			Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20})
		if err == nil || !strings.Contains(err.Error(), "M0_PRINT_SHAPE") {
			t.Fatalf("case %d error = %v", index, err)
		}
	}
}

func TestMode0AcceptsExactSSprPaletteAndFullClipSubsets(t *testing.T) {
	_, report, err := TranspileWithReport("subset.lua", `
function _update()
  pal(7,12,0)
  pal(12,14,1)
  palt(0,true)
end
function _draw()
  cls()
  clip()
  map()
  sspr(72,0,16,16,px,16,16,16,flip,false)
end
`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Mode0Objects {
		t.Fatalf("report = %+v", report)
	}
}

func TestMode0RejectsInexactSSprClipPaletteAndTransparency(t *testing.T) {
	tests := []struct {
		name   string
		source string
		code   string
	}{
		{"scaled-sspr", `function _draw() cls() map() sspr(0,0,8,8,0,0,16,16) end`, "M0_SPR_SHAPE"},
		{"unaligned-sspr", `function _draw() cls() map() sspr(1,0,8,8,0,0) end`, "M0_SPR_SHAPE"},
		{"partial-clip", `function _draw() cls() clip(0,0,120,80) map() end`, "M0_UNSUPPORTED_BUILTIN"},
		{"opaque-zero", `function _update() palt(0,false) end function _draw() cls() map() end`, "M0_UNSUPPORTED_BUILTIN"},
		{"pal-after-map", `function _draw() cls() map() pal(1,2,0) end`, "M0_DRAW_SHAPE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := TranspileWithReport(test.name+".lua", test.source,
				Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20})
			if err == nil || !strings.Contains(err.Error(), "["+test.code+"]") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestRendererAutoSelectsMode0WhenProofPasses(t *testing.T) {
	_, report, err := TranspileWithReport("auto.lua", `function _draw() cls() map() end`, Options{Renderer: RendererAuto, SheetW: 8, SheetH: 8, MapW: 30, MapH: 20})
	if err != nil {
		t.Fatal(err)
	}
	if report.Requested != RendererAuto || report.Selected != RendererMode0 || report.Reason != reasonAutoMode0Eligible || report.Profile != autoProfileTileV1 || report.Diagnostic != "" {
		t.Fatalf("report = %+v", report)
	}
}

func TestRendererAutoFallsBackToMode4WithDiagnostic(t *testing.T) {
	_, report, err := TranspileWithReport("auto.lua", `function _draw() cls() pset(1,2,3) end`, Options{Renderer: RendererAuto, SheetW: 8, SheetH: 8, MapW: 30, MapH: 20})
	if err != nil {
		t.Fatal(err)
	}
	if report.Requested != RendererAuto || report.Selected != RendererMode4 || report.Reason != reasonAutoMode0Ineligible {
		t.Fatalf("report = %+v", report)
	}
	if report.Profile != autoProfileTileV1 {
		t.Fatalf("profile = %q", report.Profile)
	}
	if !strings.Contains(report.Diagnostic, "M0_MODE4_BUILTIN") || !strings.Contains(report.Diagnostic, "auto.lua:1") {
		t.Fatalf("diagnostic = %q", report.Diagnostic)
	}
}

func TestRendererAutoFallsBackForRuntimeAssetMutationCost(t *testing.T) {
	for _, source := range []string{
		"function mutate() mset(0,0,1) end\nfunction _update() mutate() end\nfunction _draw() cls() map() end",
		"function _update() fset(1,0,true) end\nfunction _draw() cls() map() end",
	} {
		_, report, err := TranspileWithReport("mutation.lua", source,
			Options{Renderer: RendererAuto, SheetW: 16, SheetH: 8, MapW: 30, MapH: 20})
		if err != nil {
			t.Fatal(err)
		}
		if report.Selected != RendererMode4 || report.Reason != reasonAutoMode0CostUnknown {
			t.Fatalf("report = %+v", report)
		}
		if !strings.Contains(report.Diagnostic, reasonMode0CostUnproven) || !strings.Contains(report.Diagnostic, "mutation.lua:1") {
			t.Fatalf("diagnostic = %q", report.Diagnostic)
		}
	}
}

func TestRendererAutoAcceptsEvidenceBoundedRuntimeMutations(t *testing.T) {
	updates := "  sset(8,0,12)\n" + strings.Repeat("  mset(0,0,1)\n", 16)
	source := "function _init() fset(1,0,true) end\nfunction _update()\n" + updates + "end\nfunction _draw() cls() map() end"
	_, report, err := TranspileWithReport("bounded.lua", source,
		Options{Renderer: RendererAuto, SheetW: 16, SheetH: 8, MapW: 30, MapH: 20})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Reason != reasonAutoMode0Eligible {
		t.Fatalf("report = %+v", report)
	}
}

func TestRendererAutoRejectsMoreThanSixteenMsetsPerUpdate(t *testing.T) {
	source := "function _update()\n" + strings.Repeat("  mset(0,0,1)\n", 17) + "end\nfunction _draw() cls() map() end"
	_, report, err := TranspileWithReport("many-mset.lua", source,
		Options{Renderer: RendererAuto, SheetW: 16, SheetH: 8, MapW: 30, MapH: 20})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || report.Reason != reasonAutoMode0CostUnknown || !strings.Contains(report.Diagnostic, "at most 16") {
		t.Fatalf("report = %+v", report)
	}
}

func TestForcedMode0StillAcceptsBoundedRuntimeAssetMutations(t *testing.T) {
	source := `
function _update()
  sset(8,0,12)
  mset(0,0,1)
  fset(1,0,true)
end
function _draw()
  cls()
  map()
end`
	_, report, err := TranspileWithReport("mutation.lua", source,
		Options{Renderer: RendererMode0, SheetW: 16, SheetH: 8, MapW: 30, MapH: 20})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Reason != reasonMode0ForcedProven {
		t.Fatalf("report = %+v", report)
	}
}

func TestGenerateCannotBypassForcedMode0Validation(t *testing.T) {
	chunk, err := luaparser.ParseString("direct.lua", `function _draw() cls() map() pset(1, 2, 3) end`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Generate(chunk, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8, MapW: 1, MapH: 1})
	if err == nil || !strings.Contains(err.Error(), "M0_MODE4_BUILTIN") {
		t.Fatalf("Generate error = %v, want forced Mode 0 validation", err)
	}
}

func TestParseRenderer(t *testing.T) {
	for input, want := range map[string]Renderer{"mode4": RendererMode4, "mode0": RendererMode0, "auto": RendererAuto} {
		got, err := ParseRenderer(input)
		if err != nil || got != want {
			t.Fatalf("ParseRenderer(%q) = %v, %v", input, got, err)
		}
	}
	if _, err := ParseRenderer("adaptive"); err == nil {
		t.Fatal("ParseRenderer accepted an unsupported alias")
	}
}
