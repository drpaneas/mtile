package transpiler

import (
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
	rt "github.com/drpaneas/mtile/runtime"
)

func namedPoseOptions() Options {
	o := psprMode0Opts(rt.PosePart{Tile: 0}, rt.PosePart{Tile: 0, X: 8})
	o.PoseNames = []string{"prop", "hero_idle"}
	o.PoseIndex = []rt.PoseRange{{Count: 1}, {Start: 1, Count: 1}}
	return o
}

func TestNamedPoseBuiltinRendererContract(t *testing.T) {
	if Builtins["pspr"] != (BuiltinSpec{GoName: "PSprV", MinArgs: 3, MaxArgs: 5}) || BuiltinRendererCompatibility("pspr") != RendererMode0Conditional {
		t.Fatal("named pspr changed its numeric ABI or conditional renderer contract")
	}
}

func TestNamedPosePixelsSurviveOtherPoseDeletionAndReordering(t *testing.T) {
	for _, names := range [][]string{{"prop", "hero_idle"}, {"hero_idle", "prop"}, {"hero_idle"}} {
		o := namedPoseOptions()
		o.Renderer = RendererMode4
		o.PoseNames = names
		o.PoseIndex = nil
		for _, name := range names {
			start := uint32(0)
			if name == "hero_idle" {
				start = 1
			}
			o.PoseIndex = append(o.PoseIndex, rt.PoseRange{Start: start, Count: 1})
		}
		o.PoseParts = []rt.PosePart{{Tile: 0}, {Tile: 1}}
		source, _, err := TranspileWithReport("pose.lua", `function _draw() cls(4) pspr("hero_idle",10,20) end`, o)
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, source, `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestNamedPosePixels(t *testing.T){
 sheet:=make([]byte,128);sheet[1]=3;sheet[9]=7
 image:=rt.AssetImage{SheetW:16,SheetH:8,SpriteCols:2,SpriteRows:1,SpriteN:2,Sheet:sheet,MapW:1,MapH:1,Map:[]uint16{0},Flags:[]byte{0,0},PoseIndex:poseIndexROM[:],PoseParts:posePartROM[:]}
 memory:=rt.AssetMemory{SheetW:16,SheetH:8,SpriteCols:2,SpriteRows:1,SpriteN:2,Sheet:make([]byte,128),MapW:1,MapH:1,Map:make([]uint16,1),Flags:make([]byte,2)}
 var c rt.Context;if !c.Init(&image,&memory){t.Fatal("init")}
 var g Game;g.Init(&c);g.Draw(&c)
 for y:=0;y<rt.ScreenHeight;y++{for x:=0;x<rt.ScreenWidth;x++{
  want:=byte(4);if x==11&&y==20{want=7}
  if got:=c.Pget(x,y);got!=want{t.Fatalf("pixel(%d,%d)=%d want%d",x,y,got,want)}
 }}
}`, "TestNamedPosePixels")
	}
}

func TestNamedPoseLowersExactlyToNumericCall(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		for _, draw := range []string{
			`pspr(("hero_idle"),20,30)`,
			`pspr("hero_idle",20,30,true)`,
			`pspr("hero_idle",20,30,false,true)`,
			`pspr("hero_idle",20,30,true,true)`,
			`for i=1,2 do pspr("hero_idle",i*16,30) end`,
		} {
			o := namedPoseOptions()
			o.Renderer = renderer
			source := `function _draw() cls() ` + draw + ` end`
			numeric := strings.ReplaceAll(source, `"hero_idle"`, `1`)
			got, report, err := TranspileWithReport("pose.lua", source, o)
			if err != nil {
				t.Fatal(err)
			}
			want, control, err := TranspileWithReport("pose.lua", numeric, o)
			if err != nil {
				t.Fatal(err)
			}
			if got != want || !reflect.DeepEqual(report, control) {
				t.Fatalf("%s %s: named call differs from numeric control: %+v / %+v", renderer, draw, report, control)
			}
			if report.StaticStrings != 0 || report.HeapLinked || strings.Contains(got, "hero_idle") {
				t.Fatal("pose name retained runtime string or heap storage")
			}
		}
	}
}

func TestNamedPoseHelpersAndColorVariantsMatchNumericCalls(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		o := poseVariantOptions()
		o.PoseNames = []string{"hero_idle"}
		o.Renderer = renderer
		source := `function actor(x) pspr("hero_idle",x,30,true) end
function _draw() cls() pvariant("fire") for i=1,2 do actor(i*16) end pvariant() end`
		got, report, err := TranspileWithReport("variants.lua", source, o)
		if err != nil {
			t.Fatal(err)
		}
		want, control, err := TranspileWithReport("variants.lua", strings.ReplaceAll(source, `"hero_idle"`, `0`), o)
		if err != nil {
			t.Fatal(err)
		}
		if got != want || !reflect.DeepEqual(report, control) {
			t.Fatalf("%s named helper/variant changed drawing or proof", renderer)
		}
	}
}

func TestNamedPoseUnknownNameHasPositionAndSuggestion(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		o := namedPoseOptions()
		o.Renderer = renderer
		_, _, err := TranspileWithReport("pose.lua", "function _draw()\n cls()\n pspr(\"hero_idel\",0,0)\nend", o)
		for _, want := range []string{"[PSPR_UNKNOWN_POSE]", "pose.lua:3:7", `did you mean "hero_idle"`, "assets/poses.json"} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("missing %q: %v", want, err)
			}
		}
	}
}

func TestNamedPoseSelectionAndGenerationPreserveCallerAST(t *testing.T) {
	chunk, err := luaparser.ParseString("pose.lua", `function _draw() cls() pspr("hero_idle",20,30) end`)
	if err != nil {
		t.Fatal(err)
	}
	o := namedPoseOptions()
	if _, err := SelectRenderer(chunk, o); err != nil {
		t.Fatal(err)
	}
	first, err := Generate(chunk, o)
	if err != nil {
		t.Fatal(err)
	}
	o.PoseNames[0], o.PoseNames[1] = o.PoseNames[1], o.PoseNames[0]
	second, err := Generate(chunk, o)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("reusing the AST retained the old resolved index")
	}
	fn := chunk.Body[0].(*luaparser.FunctionDeclStmt)
	call := fn.Body.Body[1].(*luaparser.CallStmt).Call
	if name, ok := call.Args[0].(*luaparser.StringExpr); !ok || name.Value != "hero_idle" {
		t.Fatal("renderer selection/generation mutated the caller's literal name")
	}
}

func TestNamedPoseUsesOnlySelectedPoseBudget(t *testing.T) {
	o := namedPoseOptions()
	o.PoseParts = make([]rt.PosePart, 129) // Overlap prevents packing.
	o.PoseIndex = []rt.PoseRange{{Count: 129}, {Count: 1}}
	for _, renderer := range []Renderer{RendererMode0, RendererAuto} {
		o.Renderer = renderer
		_, report, err := TranspileWithReport("subset.lua", `function _draw() cls() pspr("hero_idle",0,0) end`, o)
		if err != nil || report.Selected != RendererMode0 || report.Mode0OAMEntries != 1 {
			t.Fatalf("safe named subset: %+v %v", report, err)
		}
		_, report, err = TranspileWithReport("oversized.lua", `function _draw() cls() pspr("prop",0,0) end`, o)
		if renderer == RendererMode0 && (err == nil || !strings.Contains(err.Error(), "129 OAM")) {
			t.Fatalf("oversized named pose admitted: %v", err)
		}
		if renderer == RendererAuto && (err != nil || report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "oversized.lua:1:")) {
			t.Fatalf("missing positioned AUTO fallback: %+v %v", report, err)
		}
	}
}

func TestNamedPoseRejectsMissingEmptyAndAmbiguousNames(t *testing.T) {
	for _, tc := range []struct {
		names      []string
		name, code string
	}{
		{nil, "hero_idle", "PSPR_UNKNOWN_POSE"},
		{[]string{"", "hero_idle"}, "", "PSPR_UNKNOWN_POSE"},
		{[]string{"hero_idle", "hero_idle"}, "hero_idle", "PSPR_AMBIGUOUS_POSE"},
		{[]string{"prop", "other", "hero_idle"}, "hero_idle", "PSPR_INVALID_POSE"},
	} {
		o := namedPoseOptions()
		o.PoseNames = tc.names
		_, _, err := TranspileWithReport("pose.lua", `function _draw() cls() pspr("`+tc.name+`",0,0) end`, o)
		if err == nil || !strings.Contains(err.Error(), tc.code) {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
	if got := closestPoseName("something_unrelated", []string{"hero_idle", "prop"}); got != "" {
		t.Fatalf("unrelated suggestion %q", got)
	}
	if a, b := closestPoseName("cat", []string{"car", "bat"}), closestPoseName("cat", []string{"bat", "car"}); a != b {
		t.Fatal("suggestions depend on library ordering")
	}
}

func TestPoseNumericWarningsPreserveRuntimeConversion(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		for _, tc := range []struct {
			id      string
			warning bool
		}{
			{"0", false}, {"1", false}, {"2", true}, {"-1", true}, {"65536", false},
			{"1.9", false}, {"(-0.000001)", false}, {"32768", true}, {"current_pose", false},
		} {
			o := namedPoseOptions()
			o.Renderer = renderer
			_, report, err := TranspileWithReport("numeric.lua", "function _draw()\n cls()\n pspr("+tc.id+",0,0)\nend", o)
			if err != nil {
				t.Fatal(err)
			}
			if (len(report.Warnings) == 1) != tc.warning {
				t.Fatalf("%s %s warnings=%v", renderer, tc.id, report.Warnings)
			}
			if tc.warning && (!strings.Contains(report.Warnings[0], "numeric.lua:3:") || !strings.Contains(report.Warnings[0], "[PSPR_ID_OUT_OF_RANGE]")) {
				t.Fatalf("missing positioned numeric warning: %v", report.Warnings)
			}
		}
	}
}

func TestNamedPosePreservesExpandedArgumentEvaluation(t *testing.T) {
	o := namedPoseOptions()
	o.Renderer = RendererMode4
	o.Sheet = make([]byte, 64)
	o.Sheet[0] = 7
	source, _, err := TranspileWithReport("order.lua", `order=0
function argument(n) order=order*10+n return n end
function coords() argument(1) return 20,30 end
function probe()
 pspr("hero_idle",argument(1),argument(2),argument(3),argument(4))
 local before=order
 order=0
 pspr("hero_idle",coords())
 return before+order
end`, o)
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, source, generatedValueHarness("g.__glu_call_probe(&c)", 1235, false), "TestGeneratedValue")
}
