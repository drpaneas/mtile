package transpiler

import (
	"fmt"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
	rt "github.com/drpaneas/mtile/runtime"
)

func branchPoseOptions() Options {
	opts := psprMode0Opts(rt.PosePart{Tile: 0})
	opts.Sheet = make([]byte, 64)
	opts.Sheet[0] = 1
	return opts
}

func branchPoseSites(t *testing.T, source string, opts Options) int {
	t.Helper()
	chunk, err := luaparser.ParseString("branches.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range chunk.Body {
		draw, ok := statement.(*luaparser.FunctionDeclStmt)
		if !ok || functionKey(draw) != "_draw" {
			continue
		}
		ops, unbounded, err := mode0WalkDraw(chunk, draw, newMode0SpriteIDAnalysis(chunk), opts)
		if err != nil || unbounded {
			t.Fatalf("draw walk: unbounded=%t error=%v", unbounded, err)
		}
		return len(mode0PsprSitesFromOps(ops))
	}
	t.Fatal("missing draw")
	return 0
}

func TestMode0BranchPoseBudgetPreservesAllAllocationSites(t *testing.T) {
	source := `function _draw()
 cls()
 if flag then
  for i=1,100 do pspr(0,0,0) end
 else
  for i=1,100 do pspr(0,0,0) end
 end
end`
	opts := branchPoseOptions()
	_, report, err := TranspileWithReport("branches.lua", source, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 100 || report.Mode0OBJLineCycles != 800 {
		t.Fatalf("mutually exclusive poses: %+v, want100 OAM/800 cycles", report)
	}
	if sites := branchPoseSites(t, source, opts); sites != 200 {
		t.Fatalf("allocation sites=%d, want200 retained sites", sites)
	}
}

func TestMode0BranchBudgetSeparatesOAMAndScanlineMaxima(t *testing.T) {
	opts := branchPoseOptions()
	opts.SheetW, opts.SheetH, opts.Sheet = 64, 8, make([]byte, 512)
	opts.Sheet[0] = 1
	opts.PoseIndex = append(opts.PoseIndex, rt.PoseRange{Start: 1, Count: 8})
	for x := 0; x < 8; x++ {
		opts.PoseParts = append(opts.PoseParts, rt.PosePart{Tile: uint16(x), X: int16(x * 8)})
	}
	for _, source := range []string{
		`function _draw() cls()
   if flag then for i=1,100 do pspr(0,0,80) end
   else for i=1,20 do pspr(1,0,0) end end
  end`,
		`function _draw() cls()
   if flag then for i=1,100 do pspr(0,0,80) end
   else for i=1,10 do pspr(1,0,0) end end
   for i=1,10 do pspr(1,0,0) end
  end`,
	} {
		_, _, err := TranspileWithReport("branch-line.lua", source, opts)
		if err == nil || !strings.Contains(err.Error(), "1280 rendering cycles") {
			t.Fatalf("lower-OAM branch must retain worst scanline: %v", err)
		}
	}
}

func TestMode0NestedBranchesAndHelpersAddCommonDraws(t *testing.T) {
	source := `function actor()
 if flag then for i=1,2 do pspr(0,0,0) end
 elseif other then for i=1,3 do pspr(0,0,0) end
 else if final then pspr(0,0,0) end end
end
function _draw()
 cls()
 pspr(0,0,0)
 actor()
 actor()
 pspr(0,0,0)
end`
	opts := branchPoseOptions()
	_, report, err := TranspileWithReport("nested-branches.lua", source, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 8 || report.Mode0OBJLineCycles != 64 {
		t.Fatalf("nested envelope=%+v", report)
	}
	if sites := branchPoseSites(t, source, opts); sites != 14 {
		t.Fatalf("retained sites=%d,want14", sites)
	}
}

func TestMode0BranchConditionsRemainInCommonBudget(t *testing.T) {
	source := `function condition() pspr(0,0,0) return flag end
 function _draw() cls()
 if condition() then pspr(0,0,0)
 elseif condition() then pspr(0,0,0) end
 end`
	_, report, err := TranspileWithReport("branch-condition.lua", source, branchPoseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 3 || report.Mode0OBJLineCycles != 24 {
		t.Fatalf("condition budget=%+v", report)
	}
}

func TestMode0BranchOrdinaryPackingRequiresAllPossibleSourcesFit(t *testing.T) {
	opts := Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160}
	source := `function _draw() cls() if flag then spr(1,0,0,2,2) else spr(3,0,0,2,2) end spr(1,0,0,2,2) end`
	_, report, err := TranspileWithReport("branch-packed.lua", source, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 2 || report.Mode0OBJLineCycles != 32 {
		t.Fatalf("fitting branch cache=%+v", report)
	}
	var body strings.Builder
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&body, "spr(%d,0,0,2,2)\n", 3+i*2)
	}
	source = "function _draw() cls() if flag then spr(1,0,0,2,2) else\n" + body.String() + "end spr(1,0,0,2,2) end"
	_, report, err = TranspileWithReport("branch-cache-full.lua", source, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 36 || report.Mode0OBJLineCycles != 144 {
		t.Fatalf("uncertain cache must charge fallback=%+v", report)
	}
}

func TestMode0BranchTextAtlasIncludesAllAlternatives(t *testing.T) {
	source := `function _draw() cls() if flag then print("AAAA",0,0,7) else print("BBBBBB",0,0,7) end end`
	_, report, err := TranspileWithReport("branch-text.lua", source, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 6 || report.Mode0OBJLineCycles != 48 {
		t.Fatalf("text branch budget=%+v", report)
	}
	chunk, err := luaparser.ParseString("branch-text.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	draw := chunk.Body[0].(*luaparser.FunctionDeclStmt)
	ops, _, err := mode0WalkDraw(chunk, draw, newMode0SpriteIDAnalysis(chunk), Options{SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if keys := mode0TextAtlasKeysForOps(ops); len(keys) != 2 {
		t.Fatalf("atlas=%v,want both glyphs", keys)
	}
}
