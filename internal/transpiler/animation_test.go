package transpiler

import (
	"fmt"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
	rt "github.com/drpaneas/mtile/runtime"
)

func animationOptions() Options {
	opts := psprMode0Opts(rt.PosePart{Tile: 0}, rt.PosePart{Tile: 1, X: 8})
	opts.PoseIndex = []rt.PoseRange{{Count: 1}, {Count: 2}}
	opts.AnimationClips = []rt.AnimationClip{{Name: "idle", Count: 2, FPS: 8, Loop: true}, {Name: "hold", Start: 2, Count: 1, FPS: 12}}
	opts.AnimationFrames = []uint16{0, 1, 1}
	return opts
}

func TestAnimationBuiltinAndParserContract(t *testing.T) {
	if got := Builtins["anim"]; got != (BuiltinSpec{GoName: "AnimV", MinArgs: 3, MaxArgs: 6}) {
		t.Fatalf("anim builtin: %+v", got)
	}
	if BuiltinRendererCompatibility("anim") != RendererMode0Conditional {
		t.Fatal("anim lacks conditional renderer classification")
	}
	for _, call := range []string{`anim("idle",1,2)`, `anim("idle",1,2,true)`, `anim("idle",1,2,false,true)`, `anim("idle",1,2,false,true,time()-start)`} {
		if _, err := luaparser.ParseString("animation.lua", "function _draw() "+call+" end"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnimationAloneEnablesTheAutomaticClock(t *testing.T) {
	opts := animationOptions()
	src, _, err := TranspileWithReport("animation-clock.lua", `function _draw() cls() anim("idle",20,30) end`, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ctx.SetTickRate(30)", "ctx.AdvanceTick()"} {
		if !strings.Contains(src, want) {
			t.Errorf("anim-only cartridge does not advance its clock: missing %s", want)
		}
	}
}

func TestAnimationTranspileUsesPSprAllocationAndROM(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		opts := animationOptions()
		opts.Renderer = renderer
		src, report, err := TranspileWithReport("animation.lua", `function actor(name,x) anim(name,x,32,false,false,time()) end
function marker() anim("idle",8,80) return 20 end
function _draw() cls() for i=1,2 do actor("idle",i*32) end anim("hold",marker(),64,true) end`, opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"animationClipROM", `Name:"idle"`, "FPS:8,Loop:true", "AnimationFrames: animationFrameROM[:]"} {
			if !strings.Contains(src, want) {
				t.Errorf("missing ROM binding %q", want)
			}
		}
		if renderer != RendererMode4 {
			if report.Selected != RendererMode0 || !report.Mode0PoseStreaming || report.Mode0OAMEntries != 4 {
				t.Fatalf("unsafe/incomplete anim proof: %+v", report)
			}
			if !strings.Contains(src, "c.AnimStaticSiteFromBaseV") || !strings.Contains(src, "c.AnimIDSiteV") {
				t.Fatal("anim did not reuse bounded PSpr sites")
			}
		} else if !strings.Contains(src, "c.AnimIDV(") {
			t.Fatal("missing software anim call")
		}
	}
}

func TestAnimationMissingLiteralClipGetsSourceDiagnostic(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		opts := animationOptions()
		opts.Renderer = renderer
		_, _, err := TranspileWithReport("animation.lua", "function _draw()\n cls()\n anim(\"typo\",0,0)\nend", opts)
		if err == nil || !strings.Contains(err.Error(), "[ANIM_UNKNOWN_CLIP]") || !strings.Contains(err.Error(), "animation.lua:3") {
			t.Fatalf("missing clip diagnostic: %v", err)
		}
	}
}

func TestAnimationDynamicNamesBudgetAllClips(t *testing.T) {
	opts := animationOptions()
	opts.PoseParts = make([]rt.PosePart, 129)
	opts.PoseIndex[1].Count = 129
	_, _, err := TranspileWithReport("animation.lua", `function _draw() cls() anim(name,0,0) end`, opts)
	if err == nil || !strings.Contains(err.Error(), "129 OAM") {
		t.Fatalf("dynamic clip omitted unsafe pose: %v", err)
	}
	opts.Renderer = RendererAuto
	_, report, err := TranspileWithReport("animation.lua", `function _draw() cls() anim(name,0,0) end`, opts)
	if err != nil || report.Selected != RendererMode4 {
		t.Fatalf("unrepresentable clip fallback: %+v %v", report, err)
	}
	opts.AnimationClips[0].Count = 1
	opts.Renderer = RendererMode0
	if _, _, err := TranspileWithReport("animation.lua", `function _draw() cls() anim("idle",0,0) end`, opts); err != nil {
		t.Fatalf("literal safe subset rejected: %v", err)
	}
}

func TestAnimationMode0RejectsUnboundedOrWrongLifecycleDraws(t *testing.T) {
	for _, source := range []string{`function _update() anim("idle",0,0) end function _draw() cls() end`, `function _draw() cls() while condition do anim("idle",0,0) end end`, `function loop() anim("idle",0,0) loop() end function _draw() cls() loop() end`} {
		if _, _, err := TranspileWithReport("animation.lua", source, animationOptions()); err == nil {
			t.Fatalf("unsafe anim draw accepted: %s", source)
		}
	}
}

func TestAnimationLiteralIDsAndAllocationProof(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode0, RendererMode4, RendererAuto} {
		for _, tc := range []struct {
			name, source, method string
			heap                 bool
			statics              int
		}{
			{"literal", `function _draw() cls() anim(("idle"),20,30) end`, "AnimID", false, 0},
			{"static helper", `function actor(name) anim(name,20,30) end function _draw() cls() actor("idle") end`, "AnimStatic", false, 1},
			{"pose", `function _draw() cls() pspr(0,20,30) end`, "PSpr", false, 0},
			{"allocated name", `function _draw() cls() anim("id".."le",20,30) end`, "Anim", true, 2},
			{"allocated coordinate", `function x() local a={20} return a[1] end function _draw() cls() anim("idle",x(),30) end`, "AnimID", true, 0},
		} {
			t.Run(fmt.Sprint(renderer)+"/"+tc.name, func(t *testing.T) {
				opts := animationOptions()
				opts.Renderer = renderer
				src, report, err := TranspileWithReport("animation.lua", tc.source, opts)
				if err != nil {
					t.Fatal(err)
				}
				if report.HeapLinked != tc.heap || report.StaticStrings != tc.statics {
					t.Fatalf("heap=%t statics=%d, want heap=%t statics=%d", report.HeapLinked, report.StaticStrings, tc.heap, tc.statics)
				}
				suffix := "V("
				if report.Selected == RendererMode0 {
					suffix = "SiteV("
				}
				if !strings.Contains(src, "c."+tc.method+suffix) {
					t.Fatalf("missing %s%s", tc.method, suffix)
				}
				if report.Selected == RendererMode4 {
					compileGenerated(t, src)
				}
			})
		}
	}
}

func TestAnimationIDPreservesArgumentOrderAndMultiReturn(t *testing.T) {
	opts := animationOptions()
	opts.Renderer = RendererMode4
	source := `order=0
 function argument(n) order=order*10+n return n end
 function coords() argument(2) return 20,30 end
 function probe()
   anim("idle",argument(1),argument(2),argument(3),argument(4),argument(5))
   local before=order
   order=0
   anim("idle",coords())
   return before+order
 end`
	src, _, err := TranspileWithReport("animation-order.lua", source, opts)
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, src, generatedValueHarness("g.__glu_call_probe(&c)", 12347, false), "TestGeneratedValue")
}
