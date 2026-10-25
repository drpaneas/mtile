package transpiler

import (
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
	rt "github.com/drpaneas/mtile/runtime"
)

func poseVariantOptions() Options {
	o := psprMode0Opts()
	o.ObjSheetW, o.ObjSheetH = 8, 16
	o.ObjSheet = make([]byte, 128)
	o.ObjSheet[1], o.ObjSheet[2] = 6, 7
	o.ObjSheet[65], o.ObjSheet[66] = 10, 12
	o.PoseVariantBaseCount = 1
	o.PoseVariantNames = []string{"fire"}
	o.PoseParts = []rt.PosePart{{Tile: 0}, {Tile: 1}}
	o.PoseIndex = []rt.PoseRange{{Count: 1, FlipOffsetX: 8, FlipOffsetY: 8}, {Start: 1, Count: 1, FlipOffsetX: 8, FlipOffsetY: 8}}
	return o
}

func TestPoseColorVariantCompilerAndRendererProof(t *testing.T) {
	const source = "function _draw() cls() pvariant('fire') pspr(0,0,0) pvariant() pspr(0,8,0) end"
	for _, renderer := range []Renderer{RendererMode0, RendererAuto, RendererMode4} {
		o := poseVariantOptions()
		o.Renderer = renderer
		code, report, err := TranspileWithReport("variants.lua", source, o)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(code, "PVariantID(1)") {
			t.Fatal("literal variant not resolved")
		}
		if renderer != RendererMode4 && (report.Selected != RendererMode0 || !report.Mode0PoseResident) {
			t.Fatalf("report=%+v", report)
		}
	}
	for _, source := range []string{"function _draw() cls() pvariant('missing') pspr(0,0,0) end", "function _draw() cls() pvariant(1,2) pspr(0,0,0) end"} {
		_, _, err := TranspileWithReport("badvariant.lua", source, poseVariantOptions())
		if err == nil || !strings.Contains(err.Error(), "badvariant.lua:") {
			t.Fatalf("missing source-positioned error: %v", err)
		}
	}
	if _, err := luaparser.ParseString("variant.lua", "function _draw() pvariant() pvariant('fire') end"); err != nil {
		t.Fatal(err)
	}
}

const staticGlobalPoseVariantSource = `variant="fire"
alternate=false
function select_variant() pvariant(variant) end
function _update()
 alternate=not alternate
 if alternate then variant="" else variant="fire" end
end
function _draw()
 cls()
 select_variant()
 pspr(0,0,0)
end`

func TestPoseColorVariantSelectionsDoNotNeedHeap(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode0, RendererMode4} {
		for _, tc := range []struct {
			name, source, call string
		}{
			{"literal", `function _draw() cls() pvariant(("fire")) pspr(0,0,0) end`, "c.PVariantID(1)"},
			{"reset", `function _draw() cls() pvariant() pspr(0,0,0) end`, "c.PVariantID(0)"},
			{"numeric", `function _draw() cls() pvariant(1) pspr(0,0,0) end`, "c.PVariantStaticV("},
			{"numeric-global", `variant=1 function _update() variant=1-variant end function _draw() cls() pvariant(variant) pspr(0,0,0) end`, "c.PVariantStaticV("},
			{"static-global", staticGlobalPoseVariantSource, "c.PVariantStaticV("},
		} {
			t.Run(renderer.String()+"/"+tc.name, func(t *testing.T) {
				o := poseVariantOptions()
				o.Renderer = renderer
				code, report, err := TranspileWithReport("heapless-variants.lua", tc.source, o)
				if err != nil {
					t.Fatal(err)
				}
				if report.HeapLinked || strings.Contains(code, "c.RegisterGlobal(") || strings.Contains(code, "ctx.CollectIfNeeded()") {
					t.Fatal("allocation-free variant selection retained heap, roots, or collection")
				}
				if !strings.Contains(code, tc.call) {
					t.Fatalf("missing variant selection %s", tc.call)
				}
				if renderer == RendererMode0 && (report.Selected != RendererMode0 || !report.Mode0PoseResident) {
					t.Fatalf("variant residency changed: %+v", report)
				}
				if tc.name == "static-global" {
					if strings.Contains(generatedSection(code, "func (g *Game) gluDraw("), "g.__glu_call_select_variant(") {
						t.Fatal("nonallocating selection helper retained a caller root wrapper")
					}
					if renderer == RendererMode4 {
						runGeneratedHarness(t, code, staticGlobalPoseVariantHarness, "TestStaticGlobalPoseVariantPixels")
					}
				}
			})
		}
	}
}

func TestPoseColorVariantAllocatingArgumentsRetainHeapAndRoots(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode0, RendererMode4} {
		for _, argument := range []string{`tostr(1)`, `"fi".."re"`} {
			t.Run(renderer.String()+"/"+argument, func(t *testing.T) {
				o := poseVariantOptions()
				o.Renderer = renderer
				source := `variant="fire"
function select_variant() pvariant(` + argument + `) end
function _draw() cls() pvariant(variant) select_variant() pspr(0,0,0) end`
				code, report, err := TranspileWithReport("allocating-variants.lua", source, o)
				if err != nil {
					t.Fatal(err)
				}
				if !report.HeapLinked || !strings.Contains(code, "c.RegisterGlobal(&g.variant)") || !strings.Contains(code, "ctx.CollectIfNeeded()") {
					t.Fatal("allocating argument lost heap, global roots, or collection")
				}
				if !strings.Contains(generatedSection(code, "func (g *Game) gluDraw("), "g.__glu_call_select_variant(") || !strings.Contains(code, "c.PVariantV(") {
					t.Fatal("allocating argument lost its caller root wrapper or heap-backed decoder")
				}
				if renderer == RendererMode0 && !report.Mode0PoseResident {
					t.Fatalf("variant residency changed: %+v", report)
				}
				if renderer == RendererMode4 {
					compileGenerated(t, code)
				}
			})
		}
	}
}

const staticGlobalPoseVariantHarness = `package main
import (
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestStaticGlobalPoseVariantPixels(t *testing.T) {
 obj:=make([]byte,128)
 obj[1],obj[2],obj[65],obj[66]=6,7,10,12
 image:=rt.AssetImage{
  SheetW:8,SheetH:8,SpriteCols:1,SpriteRows:1,SpriteN:1,Sheet:make([]byte,64),Flags:[]byte{0},
  ObjSheetW:8,ObjSheetH:16,ObjSpriteCols:1,ObjSpriteRows:2,ObjSpriteN:2,ObjSheet:obj,ObjSheetIndex:[]uint32{1,2},
  PoseIndex:[]rt.PoseRange{{Count:1,FlipOffsetX:8,FlipOffsetY:8},{Start:1,Count:1,FlipOffsetX:8,FlipOffsetY:8}},
  PoseParts:[]rt.PosePart{{Tile:0},{Tile:1}},PoseVariantBaseCount:1,PoseVariantNames:[]string{"fire"},
 }
 memory:=rt.AssetMemory{SheetW:8,SheetH:8,SpriteCols:1,SpriteRows:1,SpriteN:1,Sheet:make([]byte,64),Flags:make([]byte,1)}
 var c rt.Context
 if !c.Init(&image,&memory) {t.Fatal("init")}
 var g Game
 g.Init(&c)
 for frame:=0;frame<4;frame++ {
  g.Draw(&c)
  want1,want2:=byte(10),byte(12)
  if frame%2==1 {want1,want2=6,7}
  if got:=c.Pget(1,0);got!=want1 {t.Fatalf("frame%d first pixel=%d want%d",frame,got,want1)}
  if got:=c.Pget(2,0);got!=want2 {t.Fatalf("frame%d second pixel=%d want%d",frame,got,want2)}
  g.Update(&c)
 }
}
`

func TestPoseColorVariantRejectsUnprovenMode0Shapes(t *testing.T) {
	for _, extra := range []string{"spr(0,0,0)", "pal(6,7)"} {
		source := "function _draw() cls() pvariant('fire') pspr(0,0,0) " + extra + " end"
		o := poseVariantOptions()
		if _, _, err := TranspileWithReport("mixed.lua", source, o); err == nil {
			t.Fatalf("accepted %s", extra)
		}
		o.Renderer = RendererAuto
		_, report, err := TranspileWithReport("mixed.lua", source, o)
		if err != nil || report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "mixed.lua:") {
			t.Fatalf("fallback=%+v err=%v", report, err)
		}
	}
	o := poseVariantOptions()
	for i := 0; i < 16; i++ {
		o.ObjSheet[i] = byte(i + 1)
		o.ObjSheet[64+i] = byte(i + 17)
	}
	const source = "function _draw() cls() pvariant('fire') pspr(0,0,0) end"
	if _, _, err := TranspileWithReport("colors.lua", source, o); err == nil {
		t.Fatal("accepted 16 visible colors")
	}
	o.Renderer = RendererAuto
	_, report, err := TranspileWithReport("colors.lua", source, o)
	if err != nil || report.Selected != RendererMode4 {
		t.Fatalf("color fallback=%+v err=%v", report, err)
	}
}

func TestPoseColorVariantResidentPackingIncludesEveryVariant(t *testing.T) {
	o := poseVariantOptions()
	plan := residentFixturePlan(t, o, "function _draw() cls() pvariant('fire') pspr(0,0,0) end")
	if plan == nil {
		t.Fatal("no resident plan")
	}
	if len(plan.words) != 16 || len(plan.palettes) != 32 {
		t.Fatalf("bank words=%d palette bytes=%d", len(plan.words), len(plan.palettes))
	}
	if plan.index[0].Count != 1 || plan.index[1].Count != 1 {
		t.Fatalf("variant reachability=%+v", plan.index)
	}
	a, b := plan.parts[plan.index[0].Start], plan.parts[plan.index[1].Start]
	if a.Palette == 0 || b.Palette == 0 || a.Palette == b.Palette || a.TileOffset != b.TileOffset {
		t.Fatalf("pattern not shared: %+v %+v", a, b)
	}
	if plan.palettes[(int(a.Palette)-1)*16+1] != 6 || plan.palettes[(int(b.Palette)-1)*16+1] != 10 {
		t.Fatalf("palette mappings=%v", plan.palettes)
	}
}

func BenchmarkPoseColorVariantPaletteLayouts(b *testing.B) {
	o := poseVariantOptions()
	for i := 0; i < b.N; i++ {
		layouts, palettes := mode0PaletteLayouts(o, []bool{true, true})
		if len(layouts) == 0 || len(palettes) == 0 {
			b.Fatal("no layouts")
		}
	}
}
