package transpiler

import (
	"strings"
	"testing"

	runtimeapi "github.com/drpaneas/mtile/runtime"
)

func densePoseParts(cols, rows int) []runtimeapi.PosePart {
	var parts []runtimeapi.PosePart
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			parts = append(parts, runtimeapi.PosePart{Tile: uint16(y*cols + x), X: int16(x * 8), Y: int16(y * 8)})
		}
	}
	return parts
}

func TestMode0PosePackingLeavesRoomForEffects(t *testing.T) {
	opts := psprMode0Opts(densePoseParts(8, 8)...)
	src, report, err := TranspileWithReport("fighters.lua", `function _draw()
cls()
pspr(0, 20, 40)
pspr(0, 160, 40, true)
pspr(0, 100, 40)
spr(0, 0, 0)
end`, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 4 || report.Mode0OBJLineCycles != 192 {
		t.Fatalf("packed fighters/effect budget: %+v", report)
	}
	for _, want := range []string{"PoseOBJIndex: poseOBJIndexROM[:]", "Width:64,Height:64", "c.PSprSiteV(rt.IntVal(2), rt.IntVal(129)", "var poseOBJStageRAM [3072]uint32", "cartData.PoseOBJStage, cartData.PoseOBJStageBase = poseOBJStageRAM[:], 1"} {
		if !strings.Contains(src, want) {
			t.Errorf("missing generated plan %q", want)
		}
	}
}

func TestMode0PoseLargeDynamicLibrary(t *testing.T) {
	opts := psprMode0Opts(runtimeapi.PosePart{Tile: 0})
	opts.PoseIndex = make([]runtimeapi.PoseRange, 1000)
	for i := range opts.PoseIndex {
		opts.PoseIndex[i].Count = 1
	}
	_, report, err := TranspileWithReport("many_frames.lua", `function _draw() cls() pspr((counter%1000),0,0) end`, opts)
	if err != nil || report.Mode0OAMEntries != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestMode0PoseUnknownIDBudgetsLargestFrame(t *testing.T) {
	parts := make([]runtimeapi.PosePart, 129) // Overlap prevents rectangle packing.
	opts := psprMode0Opts(parts...)
	opts.PoseIndex = append(opts.PoseIndex, runtimeapi.PoseRange{Count: 1})
	_, _, err := TranspileWithReport("overflow.lua", `function _draw() cls() pspr(current_frame,0,0) end`, opts)
	if err == nil || !strings.Contains(err.Error(), "129 OAM entries") {
		t.Fatalf("expected actual OAM overflow, got %v", err)
	}
	opts.Renderer = RendererAuto
	_, report, err := TranspileWithReport("overflow.lua", `function _draw() cls() pspr(current_frame,0,0) end`, opts)
	if err != nil || report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "129 OAM") {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	opts.Renderer = RendererMode0
	_, report, err = TranspileWithReport("subset.lua", `function _draw() cls() pspr(1,0,0) end`, opts)
	if err != nil || report.Mode0OAMEntries != 1 {
		t.Fatalf("unused oversized pose must not prevent a proven subset: %+v, %v", report, err)
	}
}

func TestMode0PoseLoopsAndHelpersGetSeparateWindows(t *testing.T) {
	opts := psprMode0Opts(densePoseParts(2, 2)...)
	src, report, err := TranspileWithReport("effects.lua", `function actor(x) pspr(0,x,0) end
function _draw()
 cls()
 for i=1,3 do actor(i*16) end
 actor(64)
 pspr(0,80,0)
end`, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 5 {
		t.Fatalf("report=%+v", report)
	}
	for _, want := range []string{"c.PSprSiteFromBaseV(rt.IntVal(0), rt.IntVal(0), rt.IntVal(4)", "c.PSprSiteV(rt.IntVal(4), rt.IntVal(16)"} {
		if !strings.Contains(src, want) {
			t.Errorf("missing disjoint allocation %q", want)
		}
	}
}

func TestMode0PoseReservesSheetTilesFromEveryBranch(t *testing.T) {
	opts := psprMode0Opts(runtimeapi.PosePart{})
	opts.SheetW, opts.SheetH = 128, 8
	src, _, err := TranspileWithReport("branches.lua", `function _draw()
 cls()
 if condition then spr(0,0,0) else spr(15,0,0) end
 pspr(0,8,0)
end`, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "c.PSprSiteV(rt.IntVal(0), rt.IntVal(16)") {
		t.Fatal("pose window overlaps alternate branch sheet tile")
	}
}

func TestMode0PoseReclaimsUnusedTextAndPackingTiles(t *testing.T) {
	opts := psprMode0Opts(densePoseParts(8, 8)...)
	source := "function _draw() cls() for i=1,8 do pspr(0,0,0) end end"
	_, report, err := TranspileWithReport("full_vram.lua", source, opts)
	if err != nil || report.Mode0OAMEntries != 8 {
		t.Fatalf("512 streamed tiles should fit without reservations: %+v %v", report, err)
	}
	_, _, err = TranspileWithReport("with_text.lua", strings.Replace(source, "cls()", `cls() print("A",0,0,7)`, 1), opts)
	if err == nil || !strings.Contains(err.Error(), "480 available") {
		t.Fatalf("text reservation missing: %v", err)
	}
}

func TestMode0PoseScanlineAccountsForOffsetAndFlip(t *testing.T) {
	opts := psprMode0Opts(runtimeapi.PosePart{Y: 0}, runtimeapi.PosePart{Y: 80})
	_, report, err := TranspileWithReport("rows.lua", `function _draw() cls() pspr(0,0,0,false,flipped) end`, opts)
	if err != nil || report.Mode0OBJLineCycles != 8 {
		t.Fatalf("separate pose rows should not be summed: %+v %v", report, err)
	}
}

func TestMode0PoseIDsUseTargetFixedPointArithmetic(t *testing.T) {
	for _, declaration := range []string{
		`local id=65536`,
		`local a=-32768 local id=(a+a)/2`,
		`local a=0.1 local id=flr(a*10)`,
		`local a=0.999999 local id=flr(a)`,
	} {
		opts := psprMode0Opts(runtimeapi.PosePart{Tile: 0}, runtimeapi.PosePart{Tile: 1})
		opts.PoseIndex = []runtimeapi.PoseRange{{Count: 1}, {Start: 1, Count: 1}}
		src, report, err := TranspileWithReport("fixed.lua", `function _draw() cls() `+declaration+` pspr(id,0,0) pspr(1,8,0) end`, opts)
		if err != nil || report.Mode0OAMEntries != 2 {
			t.Fatalf("%s: report=%+v err=%v", declaration, report, err)
		}
		if !strings.Contains(src, "c.PSprSiteV(rt.IntVal(1), rt.IntVal(1)") {
			t.Fatalf("%s: runtime pose IDs require disjoint resident windows", declaration)
		}
	}
}
