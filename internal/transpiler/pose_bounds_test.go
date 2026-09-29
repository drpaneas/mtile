package transpiler

import (
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
	rt "github.com/drpaneas/mtile/runtime"
)

func TestPoseFlipBoundsDeriveForDirectOptionsAndAllRenderers(t *testing.T) {
	source := `function _draw()
 cls()
 local flip=btn(0)
 pspr(0,80,72,flip,false)
 anim("idle",80,100,false,flip)
end`
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		opts := psprMode0Opts(rt.PosePart{Tile: 0, X: -24, Y: -8}, rt.PosePart{Tile: 1, X: 24, Y: 16})
		opts.Renderer = renderer
		opts.PoseIndex = []rt.PoseRange{{Count: 1, FlipOffsetX: 123}, {Start: 1, Count: 1, FlipOffsetY: 456}}
		opts.AnimationClips = []rt.AnimationClip{{Name: "idle", Count: 2, FPS: 8, Loop: true, FlipOffsetX: 789}}
		opts.AnimationFrames = []uint16{0, 1}
		beforePoses := append([]rt.PoseRange(nil), opts.PoseIndex...)
		beforeClips := append([]rt.AnimationClip(nil), opts.AnimationClips...)
		got, report, err := TranspileWithReport("flip.lua", source, opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"FlipOffsetX: -40, FlipOffsetY: -8", "FlipOffsetX: 56, FlipOffsetY: 40",
			"FlipOffsetX:8,FlipOffsetY:16",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("renderer %s missing derived offsets %q", renderer, want)
			}
		}
		if !reflect.DeepEqual(opts.PoseIndex, beforePoses) || !reflect.DeepEqual(opts.AnimationClips, beforeClips) {
			t.Fatal("compilation mutated caller-owned asset metadata")
		}
		if renderer != RendererMode4 && report.Selected != RendererMode0 {
			t.Fatalf("automatic anchoring changed renderer eligibility: %+v", report)
		}
		// Parsed-AST callers must receive the same derivation, too.
		chunk, err := luaparser.ParseString("flip.lua", source)
		if err != nil {
			t.Fatal(err)
		}
		direct, err := Generate(chunk, opts)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(direct, "FlipOffsetX:8,FlipOffsetY:16") {
			t.Fatal("direct Generate retained stale clip bounds")
		}
	}
}

func TestPoseFlipBoundsHandleEmptyInvalidAndExtremeDirectTables(t *testing.T) {
	opts := Options{
		PoseIndex:       []rt.PoseRange{{Count: 1}, {Start: 1, Count: 1}, {}, {Start: ^uint32(0), Count: 2}},
		PoseParts:       []rt.PosePart{{X: -32768, Y: 32767}, {X: 32767, Y: -32768}},
		AnimationClips:  []rt.AnimationClip{{Count: 3}, {Start: 3, Count: 1}, {Start: ^uint32(0), Count: 2}},
		AnimationFrames: []uint16{0, 1, 2, 2},
	}
	got := withPoseFlipOffsets(opts)
	for id, want := range [][2]int32{{-65528, 65542}, {65542, -65528}, {0, 0}, {0, 0}} {
		pose := got.PoseIndex[id]
		if pose.FlipOffsetX != want[0] || pose.FlipOffsetY != want[1] {
			t.Fatalf("pose %d = %+v, want %v", id, pose, want)
		}
	}
	for id, want := range [][2]int32{{7, 7}, {0, 0}, {0, 0}} {
		clip := got.AnimationClips[id]
		if clip.FlipOffsetX != want[0] || clip.FlipOffsetY != want[1] {
			t.Fatalf("clip %d = %+v, want %v", id, clip, want)
		}
	}
	// Changing raw parts must invalidate any previous derived values.
	opts.PoseParts[0].X = -8
	if updated := withPoseFlipOffsets(got); updated.PoseIndex[0].FlipOffsetX != -8 || updated.AnimationClips[0].FlipOffsetX != 32767 {
		t.Fatal("direct raw-part edits did not regenerate offsets")
	}
}

func TestAutomaticFlipMetadataPreservesDynamicFlagEvaluationOrder(t *testing.T) {
	opts := animationOptions()
	opts.Renderer = RendererMode4
	source := `order=0
function flag(n) order=order*10+n return n==1 end
function probe()
 pspr(0,20,30,flag(1),flag(2))
 anim("idle",20,30,flag(3),flag(4))
 return order
end`
	generated, err := Transpile("flip-order.lua", source, opts)
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 1234, false), "TestGeneratedValue")
}
