package transpiler

import (
	"fmt"
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
`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 60, MapH: 20, Map: nonEmptyMap(60, 20, 1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Reason != reasonMode0ForcedProven || report.Mode0Layers != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestExpandedMsetArgumentsRetainConservativeMapPlanes(t *testing.T) {
	generated, report, err := TranspileWithReport("expanded-mset.lua", `
function edit() return 2, 0, 0, 7 end
function _update() mset(edit()) end
`, Options{Renderer: RendererMode4, MapW: 1, MapH: 1, MapLayers: 4, Map: make([]uint16, 4)})
	if err != nil {
		t.Fatal(err)
	}
	if report.MapPlaneMask != 0x0f {
		t.Fatalf("map plane mask = %04b, want all planes for expanded dynamic layer", report.MapPlaneMask)
	}
	if !strings.Contains(generated, "c.MsetLayer(") {
		t.Fatalf("expanded four-result mset did not lower to layered mutation:\n%s", generated)
	}
}

func TestMode0RejectsExpandedConditionalBuiltinShape(t *testing.T) {
	source := `
function draw_args() return 1, 8, 8 end
function _draw()
  cls()
  spr(draw_args())
end
`
	_, _, err := TranspileWithReport("expanded-spr.lua", source, Options{Renderer: RendererMode0, SheetW: 16, SheetH: 8})
	if err == nil || !strings.Contains(err.Error(), "M0_DRAW_SHAPE") || !strings.Contains(err.Error(), "expanded multi-results") {
		t.Fatalf("forced Mode 0 error = %v, want explicit expanded-shape rejection", err)
	}
	_, report, err := TranspileWithReport("expanded-spr.lua", source, Options{Renderer: RendererAuto, SheetW: 16, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "expanded multi-results") {
		t.Fatalf("AUTO report = %+v, want conservative Mode 4 fallback", report)
	}
}

func TestMode0TreatsOrdinaryParenthesesAsStaticShapes(t *testing.T) {
	_, report, err := TranspileWithReport("parenthesized-mode0.lua", `
local frame = 1
function _init() palt((0), (true)) end
function _draw()
  cls((0))
  print(("ok"), (8), (8), (7))
  spr((frame), (16), (16))
end
`, Options{Renderer: RendererMode0, SheetW: 16, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Text || !report.Mode0Objects {
		t.Fatalf("report = %+v, want parenthesized static Mode 0 proof", report)
	}
}

func TestAutoTreatsExpandedLayeredMsetAsDynamicCost(t *testing.T) {
	source := `
function edit() return 1, 0, 0, 7 end
function _update() mset(edit()) end
function _draw() cls() map() end
`
	_, report, err := TranspileWithReport("expanded-auto-mset.lua", source, Options{
		Renderer: RendererAuto, SheetW: 8, SheetH: 8,
		MapW: 1, MapH: 1, MapLayers: 2, Map: []uint16{1, 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || report.Reason != reasonAutoMode0CostUnknown || !strings.Contains(report.Diagnostic, "layer argument") {
		t.Fatalf("AUTO report = %+v, want dynamic-layer cost fallback", report)
	}
}

func TestMode0AcceptsBackdropAndStaticTextWithoutMap(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode0, RendererAuto} {
		_, report, err := TranspileWithReport("text-only.lua", `
function _draw()
  cls(0)
  print("hello gba", 8, 8, 7)
end
`, Options{Renderer: renderer, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20})
		if err != nil {
			t.Fatalf("renderer %s: %v", renderer, err)
		}
		if report.Selected != RendererMode0 || report.Mode0Layers != 0 || !report.Mode0Objects || !report.Mode0Text {
			t.Fatalf("renderer %s report = %+v", renderer, report)
		}
	}
}

func TestMode0RejectsObjectBeforeClsWithoutMap(t *testing.T) {
	_, _, err := TranspileWithReport("wrong-order.lua", `function _draw() print("text",0,0,7) cls(0) end`,
		Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
	if err == nil || !strings.Contains(err.Error(), "M0_SPR_SHAPE") || !strings.Contains(err.Error(), "follow cls()") {
		t.Fatalf("error = %v", err)
	}
}

func TestMode0OnlyRequiresLogicalMapWhenMapIsDrawn(t *testing.T) {
	_, report, err := TranspileWithReport("no-map-assets.lua", `function _draw() cls(0) end`,
		Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
	if err != nil || report.Selected != RendererMode0 || report.Mode0Layers != 0 {
		t.Fatalf("mapless report = %+v, error = %v", report, err)
	}

	_, _, err = TranspileWithReport("missing-map-assets.lua", `function _draw() cls(0) map() end`,
		Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
	if err == nil || !strings.Contains(err.Error(), "M0_RESOURCE_LIMIT") || !strings.Contains(err.Error(), "map() requires positive map dimensions") {
		t.Fatalf("map-backed error = %v", err)
	}
}

func TestMode0CountsDirectMapLayersAndPlansTileVRAM(t *testing.T) {
	_, report, err := TranspileWithReport("layers.lua", `
function _draw()
  cls()
  map(0,0,0,0,0,30,20,1)
  map(1,0,0,0,0,30,20,2)
  map(2,0,0,0,0,30,20,4)
  map(3,0,0,0,0,30,20,8)
end
`, Options{Renderer: RendererMode0, SheetW: 192, SheetH: 256, MapW: 120, MapH: 20, MapLayers: 4, Map: nonEmptyMap(120, 20, 4, 0, 1, 2, 3)}) // 768 tiles
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
end
`, Options{Renderer: RendererMode0, SheetW: 200, SheetH: 256, MapW: 30, MapH: 20, MapLayers: 4, Map: nonEmptyMap(30, 20, 4, 0, 1, 2, 3)}) // 800 tiles
	if err == nil || !strings.Contains(err.Error(), "M0_RESOURCE_LIMIT") || !strings.Contains(err.Error(), "768") {
		t.Fatalf("resource error = %v", err)
	}
}

func TestExplicitMapBackgroundContractSelectsExactMode0Slots(t *testing.T) {
	source := `
function _draw()
  cls()
  map(0, 0, 0, 0, 0, 30, 20, 0)
  map(2, 0, 0, 0, 0, 30, 20, 0)
  map(3, 0, 0, 0, 0, 30, 20, 0)
end
`
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		generated, report, err := TranspileWithReport("explicit-bg.lua", source, Options{
			Renderer: renderer, SheetW: 64, SheetH: 64, MapW: 30, MapH: 20, MapLayers: 4,
			Map: nonEmptyMap(30, 20, 4, 0, 2, 3),
		})
		if err != nil {
			t.Fatalf("renderer %s: %v", renderer, err)
		}
		if renderer != RendererMode4 && (report.Selected != RendererMode0 || report.Mode0Layers != 3) {
			t.Fatalf("renderer %s report = %+v", renderer, report)
		}
		if strings.Count(generated, "c.MapBGAt(") != 3 || !strings.Contains(generated, "c.MapBGAt(0,") || !strings.Contains(generated, "c.MapBGAt(1,") || !strings.Contains(generated, "c.MapBGAt(2,") {
			t.Fatalf("renderer %s did not emit explicit 8-argument map calls:\n%s", renderer, generated)
		}
	}
}

func TestExplicitMapAritySurvivesNilMask(t *testing.T) {
	generated, _, err := TranspileWithReport("explicit-nil.lua", `
function _draw()
  cls()
  map(2, 0, 0, 0, 0, 30, 20, nil)
end
`, Options{Renderer: RendererMode4, SheetW: 64, SheetH: 64, MapW: 30, MapH: 20, MapLayers: 4, Map: nonEmptyMap(30, 20, 4, 2)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generated, "c.MapBGAtV(0,") || strings.Contains(generated, "c.MapV(") {
		t.Fatalf("explicit nil-mask map lost its syntactic arity:\n%s", generated)
	}
}

func TestMapSingleBackgroundUsesLogicalBGArgument(t *testing.T) {
	generated, report, err := TranspileWithReport("single-bg0.lua", `
function _draw()
  cls()
  map(0)
end
`, Options{Renderer: RendererMode0, SheetW: 64, SheetH: 64, MapW: 30, MapH: 20, MapLayers: 1, Map: nonEmptyMap(30, 20, 1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Mode0Layers != 1 {
		t.Fatalf("single map renderer report = %+v", report)
	}
	if !strings.Contains(generated, "c.MapBGAt(0,") || strings.Contains(generated, "c.MapV(") {
		t.Fatalf("single-BG map did not use the dense runtime call:\n%s", generated)
	}
}

func TestOldUnprefixedMapShapeFailsBecauseFirstArgumentIsNowBG(t *testing.T) {
	_, _, err := TranspileWithReport("old-map.lua", `function _draw() cls() map(4,2) end`,
		Options{Renderer: RendererMode4, SheetW: 8, SheetH: 8, MapW: 30, MapH: 20})
	if err == nil || !strings.Contains(err.Error(), reasonExplicitMapContract) || !strings.Contains(err.Error(), "integer literal from 0 to 3") {
		t.Fatalf("migration error = %v", err)
	}
}

func TestMapAllowsEveryOptionalArgumentAfterBG(t *testing.T) {
	args := []string{"0", "0,1", "0,1,2", "0,1,2,3", "0,1,2,3,4", "0,1,2,3,4,5", "0,1,2,3,4,5,6", "0,1,2,3,4,5,6,7"}
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		for _, callArgs := range args {
			source := "function _draw() cls() map(" + callArgs + ") end"
			if _, _, err := TranspileWithReport("optional-map.lua", source, Options{
				Renderer: renderer, SheetW: 8, SheetH: 8, MapW: 30, MapH: 20,
				Map: nonEmptyMap(30, 20, 1, 0),
			}); err != nil {
				t.Fatalf("renderer %s map(%s): %v", renderer, callArgs, err)
			}
		}
	}
}

func TestMapAllUsesOnlyNonEmptyPlanesAndCompactsTheirStorage(t *testing.T) {
	mapData := []uint16{
		0, 0, // BG0 empty
		1, 0, // BG1 authored
		0, 0, // BG2 empty
		2, 0, // BG3 authored
	}
	generated, report, err := TranspileWithReport("sparse-map.lua", `function _draw() cls() map() end`, Options{
		Renderer: RendererMode0, SheetW: 24, SheetH: 8,
		MapW: 2, MapH: 1, MapLayers: 4, Map: mapData,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0Layers != 2 || report.MapPlaneMask != 0x0a || report.MapDrawMask != 0x0a {
		t.Fatalf("sparse report = %+v", report)
	}
	for _, want := range []string{"MapPlaneMask = 10", "var mapROM = [ 4 ]uint16", "c.MapAll(10)"} {
		if !strings.Contains(generated, want) {
			t.Fatalf("generated sparse map missing %q:\n%s", want, generated)
		}
	}
}

func TestMapOnePlanePreservesLogicalIDButUsesDenseSlot(t *testing.T) {
	generated, report, err := TranspileWithReport("bg3.lua", `function _draw() cls() map(3) end`, Options{
		Renderer: RendererMode0, SheetW: 16, SheetH: 8,
		MapW: 1, MapH: 1, MapLayers: 4, Map: []uint16{0, 1, 0, 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0Layers != 1 || report.MapPlaneMask != 0x0a || report.MapDrawMask != 0x08 {
		t.Fatalf("BG3 report = %+v", report)
	}
	if !strings.Contains(generated, "c.MapBGAt(0,") {
		t.Fatalf("BG3 was not assigned dense slot 0:\n%s", generated)
	}
}

func TestMsetKeepsInitiallyEmptyBG0AllocatedForMapAll(t *testing.T) {
	generated, report, err := TranspileWithReport("mutable-empty.lua", `
function _update() mset(0,0,1) end
function _draw() cls() map() end
`, Options{Renderer: RendererMode0, SheetW: 16, SheetH: 8, MapW: 2, MapH: 1, MapLayers: 4, Map: make([]uint16, 8)})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0Layers != 1 || report.MapPlaneMask != 1 || report.MapDrawMask != 1 {
		t.Fatalf("mutable empty report = %+v", report)
	}
	if !strings.Contains(generated, "var mapROM = [ 2 ]uint16") || !strings.Contains(generated, "c.MapAll(1)") {
		t.Fatalf("mutable BG0 storage was omitted:\n%s", generated)
	}
}

func TestLayeredMsetKeepsTargetedEmptyPlaneAllocated(t *testing.T) {
	generated, report, err := TranspileWithReport("mutable-bg2.lua", `
function _update() mset(2,0,0,1) end
function _draw() cls() map() end
`, Options{Renderer: RendererMode0, SheetW: 16, SheetH: 8, MapW: 2, MapH: 1, MapLayers: 4, Map: make([]uint16, 8)})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0Layers != 1 || report.MapPlaneMask != 0x04 || report.MapDrawMask != 0x04 {
		t.Fatalf("mutable BG2 report = %+v", report)
	}
	for _, want := range []string{"var mapROM = [ 2 ]uint16", "c.MsetCompact(2,0,", "c.MapAll(4)"} {
		if !strings.Contains(generated, want) {
			t.Fatalf("mutable BG2 generation missing %q:\n%s", want, generated)
		}
	}
}

func TestLayeredMsetStorageUsesRuntimeFixedPointLayer(t *testing.T) {
	generated, report, err := TranspileWithReport("fractional-layer.lua", `
function _update() mset(1.5,0,0,1) end
function _draw() cls() map() end
`, Options{Renderer: RendererMode0, SheetW: 16, SheetH: 8, MapW: 2, MapH: 1, MapLayers: 4, Map: make([]uint16, 8)})
	if err != nil {
		t.Fatal(err)
	}
	if report.MapPlaneMask != 0x02 || report.MapDrawMask != 0x02 {
		t.Fatalf("fractional literal report = %+v, want runtime-floored BG1", report)
	}
	if !strings.Contains(generated, "c.MsetCompact(1,0,") {
		t.Fatalf("fractional BG1 generation missing fixed-slot write:\n%s", generated)
	}
}

func TestDynamicLayeredMsetRetainsEveryLogicalPlane(t *testing.T) {
	_, report, err := TranspileWithReport("mutable-dynamic.lua", `
bg = 2
function _update() mset(bg,0,0,1) end
function _draw() cls() map() end
`, Options{Renderer: RendererMode0, SheetW: 16, SheetH: 8, MapW: 2, MapH: 1, MapLayers: 4, Map: make([]uint16, 8)})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0Layers != 4 || report.MapPlaneMask != 0x0f || report.MapDrawMask != 0x0f {
		t.Fatalf("dynamic layered mset report = %+v, want all four retained planes", report)
	}
}

func TestExplicitMapBackgroundContractRejectsAmbiguousProgramsInEveryRenderer(t *testing.T) {
	tests := []struct {
		name, source, detail string
	}{
		{"mixed", `function _draw() cls() map() map(1,0,0,0,0,30,20,0) end`, "cannot be mixed"},
		{"duplicate", `function _draw() cls() map(1,0,0,0,0,30,20,0) map(1,0,0,0,0,30,20,0) end`, "more than once"},
		{"out-of-order", `function _draw() cls() map(2,0,0,0,0,30,20,0) map(1,0,0,0,0,30,20,0) end`, "back-to-front"},
		{"dynamic", `function _draw() cls() map(bg,0,0,0,0,30,20,0) end`, "integer literal"},
		{"missing-plane", `function _draw() cls() map(3,0,0,0,0,30,20,0) end`, "not present"},
		{"conditional", `function _draw() cls() if btn(0) then map(0,0,0,0,0,30,20,0) end end`, "unconditional direct"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
				layers := 4
				if test.name == "missing-plane" {
					layers = 1
				}
				_, _, err := TranspileWithReport(test.name+".lua", test.source, Options{
					Renderer: renderer, SheetW: 64, SheetH: 64, MapW: 30, MapH: 20, MapLayers: layers,
				})
				if err == nil || !strings.Contains(err.Error(), reasonExplicitMapContract) || !strings.Contains(err.Error(), test.detail) {
					t.Fatalf("renderer %s error = %v", renderer, err)
				}
			}
		})
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
		{"conditional", `function _draw() cls() if btn(0) then map() end end`, reasonExplicitMapContract},
		{"dynamic-width", `function _draw() cls() map(0,1,2,3,4,width,20,0) end`, "M0_MAP_SHAPE"},
		{"wide-window", `function _draw() cls() map(0,1,2,3,4,65,20,0) end`, "M0_MAP_SHAPE"},
		{"tall-window", `function _draw() cls() map(0,1,2,3,4,30,33,0) end`, "M0_MAP_SHAPE"},
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
  map(0, 180, 70, -3, 5, 31, 21, 4)
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
`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20, Map: nonEmptyMap(30, 20, 1, 0)})
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
`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20, Map: nonEmptyMap(30, 20, 1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Mode0Objects || !report.Mode0ObjectPacking || report.Mode0Layers != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestMode0ObjectPackingRequiresImmutableLowSpriteSources(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{"spr16", `function _draw() cls() spr(32,0,0,2,2) end`, true},
		{"sspr16", `function _draw() cls() sspr(16,8,16,16,0,0,16,16) end`, true},
		{"spr32", `function _draw() cls() spr(32,0,0,4,4) end`, true},
		{"sspr32", `function _draw() cls() sspr(16,8,32,32,0,0,32,32) end`, true},
		{"only-8x8", `function _draw() cls() spr(32,0,0) end`, false},
		{"mutable-sheet", `function _update() sset(0,0,1) end function _draw() cls() spr(32,0,0,2,2) end`, false},
		{"reserved-source-range", `function _draw() cls() spr(446,0,0,2,2) end`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, report, err := TranspileWithReport(test.name+".lua", test.source,
				Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
			if err != nil {
				t.Fatal(err)
			}
			if report.Mode0ObjectPacking != test.want {
				t.Fatalf("Mode0ObjectPacking = %t, want %t; report=%+v", report.Mode0ObjectPacking, test.want, report)
			}
		})
	}

	for _, test := range []struct {
		name, source string
	}{
		{"pack16", `function _draw() cls() spr(32,0,0,2,2) end`},
		{"pack32", `function _draw() cls() spr(32,0,0,4,4) end`},
	} {
		_, auto, err := TranspileWithReport("auto-"+test.name+".lua", test.source,
			Options{Renderer: RendererAuto, SheetW: 240, SheetH: 160})
		if err != nil {
			t.Fatal(err)
		}
		if auto.Selected != RendererMode0 || !auto.Mode0ObjectPacking {
			t.Fatalf("AUTO %s report = %+v, want packed Mode 0", test.name, auto)
		}
	}
}

func TestMode0RejectsUnsafeHardwareSpriteShapes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		code   string
	}{
		{"before-map", `function _draw() cls() spr(1,0,0) map() end`, "M0_SPR_SHAPE"},
		{"dynamic-id", `function _draw() cls() map() spr(player,0,0) end`, "M0_SPR_SHAPE"},
		{"obj-vram", `function _draw() cls() map() spr(512,0,0) end`, "M0_SPR_SHAPE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := TranspileWithReport(test.name+".lua", test.source,
				Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20, Map: nonEmptyMap(30, 20, 1, 0)})
			if err == nil || !strings.Contains(err.Error(), "["+test.code+"]") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestMode0AcceptsSpritesBetweenMaterializedBackgrounds(t *testing.T) {
	mapData := []uint16{1, 1, 1, 1}
	source := `
function _draw()
  cls()
  map(0)
  spr(1,8,8)
  map(1)
  sspr(8,0,8,8,16,16)
  map(2)
  spr(2,24,24)
  map(3)
  spr(3,32,32)
  print("D",40,40,7)
end
`
	_, report, err := TranspileWithReport("object-depth-lanes.lua", source, Options{
		Renderer:  RendererMode0,
		SheetW:    240,
		SheetH:    160,
		MapW:      1,
		MapH:      1,
		MapLayers: 4,
		Map:       mapData,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Mode0Layers != 4 || !report.Mode0Objects || !report.Mode0Text || report.Mode0OAMEntries != 5 {
		t.Fatalf("report = %+v, want four BGs and five interleaved/front objects", report)
	}
}

func TestMode0ObjectDepthUsesDenseNonEmptyBackgroundOrder(t *testing.T) {
	mapData := []uint16{0, 1, 0, 1}
	source := `
function _draw()
  cls()
  map(1)
  spr(1,8,8)
  map(3)
  spr(2,16,16)
end
`
	_, report, err := TranspileWithReport("sparse-object-depth.lua", source, Options{
		Renderer:  RendererAuto,
		SheetW:    240,
		SheetH:    160,
		MapW:      1,
		MapH:      1,
		MapLayers: 4,
		Map:       mapData,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Mode0Layers != 2 || report.MapDrawMask != 0x0a {
		t.Fatalf("report = %+v, want dense two-BG object-depth proof", report)
	}
}

func TestMode0RejectsObjectBehindEveryMaterializedBackground(t *testing.T) {
	mapData := []uint16{0, 1, 0, 0}
	source := `
function _draw()
  cls()
  map(0)
  spr(1,8,8)
  map(1)
end
`
	_, _, err := TranspileWithReport("object-behind-all.lua", source, Options{
		Renderer:  RendererMode0,
		SheetW:    240,
		SheetH:    160,
		MapW:      1,
		MapH:      1,
		MapLayers: 4,
		Map:       mapData,
	})
	if err == nil || !strings.Contains(err.Error(), "[M0_SPR_SHAPE]") || !strings.Contains(err.Error(), "behind every materialized background") {
		t.Fatalf("error = %v, want stable behind-all depth rejection", err)
	}
	_, auto, err := TranspileWithReport("object-behind-all.lua", source, Options{
		Renderer:  RendererAuto,
		SheetW:    240,
		SheetH:    160,
		MapW:      1,
		MapH:      1,
		MapLayers: 4,
		Map:       mapData,
	})
	if err != nil {
		t.Fatal(err)
	}
	if auto.Selected != RendererMode4 || auto.Reason != reasonAutoMode0Ineligible ||
		!strings.Contains(auto.Diagnostic, "object-behind-all.lua:") ||
		!strings.Contains(auto.Diagnostic, "[M0_SPR_SHAPE]") {
		t.Fatalf("AUTO report = %+v, want source-positioned Mode 4 fallback", auto)
	}
	if _, _, err := TranspileWithReport("object-behind-all.lua", source, Options{
		Renderer:  RendererMode4,
		SheetW:    240,
		SheetH:    160,
		MapW:      1,
		MapH:      1,
		MapLayers: 4,
		Map:       mapData,
	}); err != nil {
		t.Fatalf("Mode 4 must preserve the arbitrary painter sequence: %v", err)
	}
}

func TestMode0KeepsPrintAfterEveryBackground(t *testing.T) {
	_, _, err := TranspileWithReport("text-between-backgrounds.lua", `
function _draw()
  cls()
  map(0)
  print("D",8,8,7)
  map(1)
end
`, Options{
		Renderer:  RendererMode0,
		SheetW:    240,
		SheetH:    160,
		MapW:      1,
		MapH:      1,
		MapLayers: 2,
		Map:       []uint16{1, 1},
	})
	if err == nil || !strings.Contains(err.Error(), "[M0_PRINT_SHAPE]") {
		t.Fatalf("error = %v, want interleaved print rejection", err)
	}
}

func TestMode0BudgetsPhysicalOAMEntriesInsteadOfConflatingScanlines(t *testing.T) {
	build := func(count int, call string) string {
		return "function _draw()\n  cls()\n" + strings.Repeat("  "+call+"\n", count) + "end\n"
	}
	_, report, err := TranspileWithReport("oam-100.lua", build(100, "spr(1,0,0)"),
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 100 || report.Mode0OBJLineCycles != 800 {
		t.Fatalf("100-sprite budget = %+v, want 100 OAM entries and 800 line cycles", report)
	}

	_, report, err = TranspileWithReport("two-64.lua", build(2, "spr(32,0,0,8,8)"),
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 128 || report.Mode0OBJLineCycles != 128 {
		t.Fatalf("two 64x64 metasprites budget = %+v, want 128 OAM entries and 128 line cycles", report)
	}

	_, _, err = TranspileWithReport("oam-129.lua", build(129, "spr(1,0,0)"),
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err == nil || !strings.Contains(err.Error(), "[M0_OBJ_BUDGET]") || !strings.Contains(err.Error(), "129 OAM entries") {
		t.Fatalf("129-entry error = %v", err)
	}
	_, auto, err := TranspileWithReport("oam-129-auto.lua", build(129, "spr(1,0,0)"),
		Options{Renderer: RendererAuto, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if auto.Selected != RendererMode4 || auto.Reason != reasonAutoMode0Ineligible ||
		!strings.Contains(auto.Diagnostic, "[M0_OBJ_BUDGET]") || !strings.Contains(auto.Diagnostic, "129 OAM entries") {
		t.Fatalf("129-entry AUTO report = %+v", auto)
	}
}

func TestMode0BudgetsNormalOBJRenderingCyclesPerScanline(t *testing.T) {
	buildPacked := func(count int, y func(int) string) string {
		var source strings.Builder
		source.WriteString("function _draw()\n  cls()\n")
		for index := 0; index < count; index++ {
			fmt.Fprintf(&source, "  spr(32,%d,%s,4,4)\n", index%8*30, y(index))
		}
		source.WriteString("end\n")
		return source.String()
	}

	_, report, err := TranspileWithReport("packed-38.lua", buildPacked(38, func(int) string { return "64" }),
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Mode0ObjectPacking || report.Mode0OAMEntries != 38 || report.Mode0OBJLineCycles != 1216 {
		t.Fatalf("38 packed sprites budget = %+v, want 38 OAM entries and 1216 line cycles", report)
	}

	_, _, err = TranspileWithReport("packed-39.lua", buildPacked(39, func(int) string { return "64" }),
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err == nil || !strings.Contains(err.Error(), "[M0_OBJ_BUDGET]") || !strings.Contains(err.Error(), "1248 rendering cycles") {
		t.Fatalf("1248-cycle error = %v", err)
	}

	_, report, err = TranspileWithReport("packed-distributed.lua", buildPacked(39, func(index int) string {
		return fmt.Sprintf("%d", index%8*32)
	}), Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 39 || report.Mode0OBJLineCycles != 160 {
		t.Fatalf("distributed packed budget = %+v, want 39 OAM entries and 160 line cycles", report)
	}

	_, report, err = TranspileWithReport("packed-wrap.lua", buildPacked(2, func(index int) string {
		if index == 0 {
			return "240"
		}
		return "0"
	}), Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 2 || report.Mode0OBJLineCycles != 64 {
		t.Fatalf("wrapped-Y packed budget = %+v, want 2 OAM entries and 64 line cycles", report)
	}

	_, _, err = TranspileWithReport("packed-dynamic.lua", buildPacked(39, func(int) string { return "py" }),
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err == nil || !strings.Contains(err.Error(), "1248 rendering cycles") {
		t.Fatalf("dynamic-y worst-case error = %v", err)
	}
}

func TestMode0BudgetMatchesPackedCacheFallbackOrder(t *testing.T) {
	_, report, err := TranspileWithReport("pack-fallback.lua", `
function _draw()
  cls()
  spr(32,0,0,4,4)
  spr(36,40,0,4,4)
  spr(40,80,0,4,4)
end
`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Mode0ObjectPacking || report.Mode0OAMEntries != 18 || report.Mode0OBJLineCycles != 96 {
		t.Fatalf("packed fallback report = %+v, want 1+1+16 OAM entries and 96 line cycles", report)
	}
}

func TestMode0InternsRepeatedTextWithinReservedGlyphTiles(t *testing.T) {
	source := fmt.Sprintf("function _draw() cls() print(%q,0,0,7) end", strings.Repeat("A", 33))
	_, report, err := TranspileWithReport("text-33.lua", source,
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Mode0Text || report.Mode0OAMEntries != 33 || report.Mode0OBJLineCycles != 264 {
		t.Fatalf("33 repeated glyph report = %+v, want one interned tile with 33 OAM entries", report)
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

func TestMode0EmptyPrintDoesNotLinkObjectTextBackend(t *testing.T) {
	_, report, err := TranspileWithReport("empty-text.lua", `function _draw() cls() print("",0,0,7) end`,
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0Objects || report.Mode0Text || report.Mode0OAMEntries != 0 {
		t.Fatalf("empty-print report = %+v, want no OBJ/text backend", report)
	}
}

func TestMode0RejectsDrawPaletteMutationThroughHelper(t *testing.T) {
	source := `
function recolor()
  pal(1,2,0)
end
function _draw()
  cls()
  spr(1,0,0)
  recolor()
  spr(1,8,0)
end
`
	_, _, err := TranspileWithReport("helper-pal.lua", source,
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err == nil || !strings.Contains(err.Error(), "[M0_DRAW_SHAPE]") || !strings.Contains(err.Error(), "helper-pal.lua:3") {
		t.Fatalf("error = %v, want source-positioned helper palette diagnostic", err)
	}
}

func TestMode0RejectsInvalidOrDynamicClsColor(t *testing.T) {
	for _, source := range []string{
		`function _draw() cls(300) end`,
		`function _draw() cls(clear_color) end`,
	} {
		_, _, err := TranspileWithReport("bad-cls.lua", source,
			Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
		if err == nil || !strings.Contains(err.Error(), "[M0_CLS_SHAPE]") {
			t.Fatalf("forced Mode 0 error = %v", err)
		}
		_, report, err := TranspileWithReport("bad-cls-auto.lua", source,
			Options{Renderer: RendererAuto, SheetW: 240, SheetH: 160})
		if err != nil {
			t.Fatal(err)
		}
		if report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "[M0_CLS_SHAPE]") {
			t.Fatalf("AUTO report = %+v, want Mode 4 fallback", report)
		}
	}
}

func TestMode0RejectsReturnBeforeClsAndAutoFallsBack(t *testing.T) {
	source := `function _draw()
  if btn(0) then
    return
  end
  cls(3)
end`
	_, _, err := TranspileWithReport("return-before-cls.lua", source,
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err == nil || !strings.Contains(err.Error(), "[M0_CLS_SHAPE]") || !strings.Contains(err.Error(), "return-before-cls.lua:3") {
		t.Fatalf("forced Mode 0 error = %v, want source-positioned cls dominance diagnostic", err)
	}

	_, report, err := TranspileWithReport("return-before-cls-auto.lua", source,
		Options{Renderer: RendererAuto, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "[M0_CLS_SHAPE]") {
		t.Fatalf("AUTO report = %+v, want Mode 4 fallback", report)
	}
}

func TestMode0AcceptsReturnAfterCls(t *testing.T) {
	_, report, err := TranspileWithReport("return-after-cls.lua", `function _draw()
  cls(3)
  if btn(0) then return end
end`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 {
		t.Fatalf("report = %+v, want Mode 0", report)
	}
}

func TestMode0RejectsUserLifecycleCallbackInvocationOrReference(t *testing.T) {
	tests := []string{
		`function _update() _draw() end function _draw() cls() spr(1,0,0) end`,
		`function _update() foreach({1},_draw) end function _draw() cls() spr(1,0,0) end`,
	}
	for _, source := range tests {
		_, _, err := TranspileWithReport("lifecycle.lua", source,
			Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
		if err == nil || !strings.Contains(err.Error(), "[M0_DRAW_SHAPE]") || !strings.Contains(err.Error(), "lifecycle callbacks") {
			t.Fatalf("error = %v, want lifecycle rejection", err)
		}
		_, report, err := TranspileWithReport("lifecycle-auto.lua", source,
			Options{Renderer: RendererAuto, SheetW: 240, SheetH: 160})
		if err != nil {
			t.Fatal(err)
		}
		if report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "[M0_DRAW_SHAPE]") {
			t.Fatalf("AUTO report = %+v, want Mode 4 fallback", report)
		}
	}
}

func TestMode0RejectsDynamicOrControlCodeText(t *testing.T) {
	tests := []string{
		`function _draw() cls() map() print("x"..n,0,0,7) end`,
		`function _draw() cls() map() print(tostr(n),0,0,7) end`,
		"function _draw() cls() map() print(\"line\\n\",0,0,7) end",
		`function _draw() cls() map() print("text",0,0,ink) end`,
		`function _draw() cls() map() print("implicit") end`,
		`function _draw() cls() map() print(true,0,0,7) end`,
		`label="HI" function _draw() cls() map() print(label,0,0,7) end`,
		`flag=false function _draw() cls() map() print(flag,0,0,7) end`,
		`player={dead=false} function _draw() cls() map() print(player.dead,0,0,7) end`,
	}
	for index, source := range tests {
		_, _, err := TranspileWithReport("bad-text.lua", source,
			Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20})
		if err == nil || !strings.Contains(err.Error(), "M0_PRINT_SHAPE") {
			t.Fatalf("case %d error = %v", index, err)
		}
	}
}

func TestMode0AcceptsNumericPrint(t *testing.T) {
	_, report, err := TranspileWithReport("num-print.lua", `
n=42
function _draw()
  cls()
  print(n,0,0,7)
end
`, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Mode0Text || report.Mode0OAMEntries != mode0PrintNumberOAMEntries {
		t.Fatalf("report = %+v, want numeric print with %d OAM entries", report, mode0PrintNumberOAMEntries)
	}
	if len(report.Mode0TextKeys) != len(mode0PrintNumberCharset) {
		t.Fatalf("numeric atlas keys = %d, want %d digit-bank glyphs", len(report.Mode0TextKeys), len(mode0PrintNumberCharset))
	}
}

func TestMode0RejectsOpaqueColorZeroTextAndAutoFallsBack(t *testing.T) {
	source := `function _draw() cls(3) print("A",0,0,0) end`
	_, _, err := TranspileWithReport("opaque-zero-text.lua", source,
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err == nil || !strings.Contains(err.Error(), "[M0_PRINT_SHAPE]") || !strings.Contains(err.Error(), "opaque-zero-text.lua:1") {
		t.Fatalf("forced Mode 0 error = %v, want source-positioned opaque-zero diagnostic", err)
	}

	_, report, err := TranspileWithReport("opaque-zero-text-auto.lua", source,
		Options{Renderer: RendererAuto, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "[M0_PRINT_SHAPE]") {
		t.Fatalf("AUTO report = %+v, want Mode 4 fallback", report)
	}
}

func TestMode0AcceptsEmptyColorZeroText(t *testing.T) {
	_, report, err := TranspileWithReport("empty-zero-text.lua", `function _draw() cls(3) print("",0,0,0) end`,
		Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Mode0Objects || report.Mode0Text {
		t.Fatalf("report = %+v, want backend-free Mode 0 empty print", report)
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
	updates := "  sset(8,0,12)\n" + strings.Repeat("  mset(1,0,0,1)\n", 16)
	source := "function _init() fset(1,0,true) end\nfunction _update()\n" + updates + "end\nfunction _draw() cls() map() end"
	_, report, err := TranspileWithReport("bounded.lua", source,
		Options{Renderer: RendererAuto, SheetW: 16, SheetH: 8, MapW: 30, MapH: 20, MapLayers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Reason != reasonAutoMode0Eligible {
		t.Fatalf("report = %+v", report)
	}
}

func TestRendererAutoFallsBackForDynamicLayeredMsetCost(t *testing.T) {
	source := `
bg = 1
function _update()
  mset(bg,0,0,1)
end
function _draw()
  cls()
  map()
end`
	_, report, err := TranspileWithReport("dynamic-layer-mset.lua", source,
		Options{Renderer: RendererAuto, SheetW: 16, SheetH: 8, MapW: 30, MapH: 20, MapLayers: 4})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || report.Reason != reasonAutoMode0CostUnknown {
		t.Fatalf("report = %+v", report)
	}
	if !strings.Contains(report.Diagnostic, reasonMode0CostUnproven) ||
		!strings.Contains(report.Diagnostic, "dynamic-layer-mset.lua:4") ||
		!strings.Contains(report.Diagnostic, "numeric literal") {
		t.Fatalf("diagnostic = %q", report.Diagnostic)
	}

	_, forced, err := TranspileWithReport("dynamic-layer-mset.lua", source,
		Options{Renderer: RendererMode0, SheetW: 16, SheetH: 8, MapW: 30, MapH: 20, MapLayers: 4})
	if err != nil {
		t.Fatal(err)
	}
	if forced.Selected != RendererMode0 || forced.MapPlaneMask != 0x0f {
		t.Fatalf("forced report = %+v, want exact Mode 0 with all planes retained", forced)
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

func TestMode0AcceptsConditionalAndHelperSprites(t *testing.T) {
	mapData := nonEmptyMap(30, 20, 1, 0)
	opts := Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160, MapW: 30, MapH: 20, Map: mapData}
	tests := []struct {
		name, source string
	}{
		{"conditional", "function _draw() cls() map() if btn(0) then spr(1,0,0) end end"},
		{"helper-after-map", `
function draw_player()
  spr(3, 8, 8)
end
function _draw()
  cls()
  map()
  draw_player()
end
`},
		{"numeric-for", `
function _draw()
  cls()
  map()
  for i = 0, 3 do
    spr(1, i * 8, 0)
  end
end
`},
		{"finite-arithmetic-helper", `
function sheet_id(id)
  local tile = flr(id)
  return flr(tile / 16) * 30 + tile % 16
end
function _draw()
  cls()
  map()
  spr(sheet_id(29), 0, 0)
end
`},
		{"trig-wrapper", `
function unit_sin(turn)
  return sin(turn)
end
function _draw()
  cls()
  map()
  spr(flr(9.5 + unit_sin(0.25)), 0, 0)
end
`},
		{"numeric-for-table", `
sprites = {{id=1, x=0, y=0}}
function _draw()
  cls()
  map()
  for i = 1, 1 do
    local s = sprites[i]
    if s then
      spr(s.id, s.x, s.y)
    end
  end
end
`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, report, err := TranspileWithReport(test.name+".lua", test.source, opts)
			if err != nil {
				t.Fatal(err)
			}
			if report.Selected != RendererMode0 || !report.Mode0Objects {
				t.Fatalf("report = %+v, want Mode 0 objects", report)
			}
		})
	}
}

func TestMode0RejectsHelperSpriteBeforeMaterializedMap(t *testing.T) {
	source := `
function draw_player()
  spr(1, 0, 0)
end
function _draw()
  cls()
  draw_player()
  map()
end
`
	_, _, err := TranspileWithReport("helper-before-map.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
		MapW:     30,
		MapH:     20,
		Map:      nonEmptyMap(30, 20, 1, 0),
	})
	if err == nil || !strings.Contains(err.Error(), "[M0_SPR_SHAPE]") || !strings.Contains(err.Error(), "behind every materialized background") {
		t.Fatalf("error = %v, want helper spr before map rejected", err)
	}
}

func nonEmptyMap(width, height, layers int, nonEmpty ...int) []uint16 {
	data := make([]uint16, width*height*layers)
	for _, layer := range nonEmpty {
		data[layer*width*height] = 1
	}
	return data
}

func TestMode0RejectsPspr(t *testing.T) {
	_, _, err := TranspileWithReport("draw.lua", `
function _draw()
  cls()
  pspr(0, 0, 0)
end
`, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8, MapW: 30, MapH: 20})
	if err == nil || !strings.Contains(err.Error(), "[M0_MODE4_BUILTIN]") || !strings.Contains(err.Error(), "pspr") {
		t.Fatalf("error = %v", err)
	}
}

func TestAutoPsprSelectsMode4(t *testing.T) {
	_, report, err := TranspileWithReport("auto.lua", `function _draw() cls() pspr(0,0,0) end`, Options{Renderer: RendererAuto, SheetW: 8, SheetH: 8, MapW: 30, MapH: 20})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 {
		t.Fatalf("report = %+v", report)
	}
}
