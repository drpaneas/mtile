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
