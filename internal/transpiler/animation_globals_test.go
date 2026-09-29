package transpiler

import (
	"strings"
	"testing"

	rt "github.com/drpaneas/mtile/runtime"
)

const staticGlobalAnimationSource = `name="idle"
missing="absent"
alternate=false
function actor(x) anim(name,x,30) end
function _update()
 alternate=not alternate
 if alternate then name="hold" else name="idle" end
end
function _draw()
 cls(4)
 actor(20) actor(40)
 anim(missing,0,0)
end`

func TestStaticGlobalAnimationChoicesDoNotNeedHeap(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode0, RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			opts := animationOptions()
			opts.Renderer = renderer
			opts.PoseIndex = []rt.PoseRange{{Count: 1}, {Start: 1, Count: 1}}
			opts.PoseParts = []rt.PosePart{{Tile: 0}, {Tile: 1}}
			opts.AnimationClips = []rt.AnimationClip{{Name: "idle", Count: 1, FPS: 8, Loop: true}, {Name: "hold", Start: 1, Count: 1, FPS: 8, Loop: true}}
			opts.AnimationFrames = []uint16{0, 1}
			generated, report, err := TranspileWithReport("global-animation.lua", staticGlobalAnimationSource, opts)
			if err != nil {
				t.Fatal(err)
			}
			if report.HeapLinked || strings.Contains(generated, "c.RegisterGlobal(") || strings.Contains(generated, "ctx.CollectIfNeeded()") {
				t.Fatal("static global names unnecessarily retain heap initialization or collection")
			}
			if !strings.Contains(generated, "c.AnimStatic") {
				t.Fatal("dynamic choice of static global names must use the heapless decoder")
			}
			if renderer == RendererMode4 {
				// The generic host harness uses the software runtime; the native
				// build regression covers Mode 0's hardware-only entry point.
				runGeneratedHarness(t, generated, staticGlobalAnimationHarness, "TestStaticGlobalAnimationPixels")
			}
		})
	}
}

func TestBoxedGlobalsRetainRootsWhenAnyCodeAllocates(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		options      Options
	}{
		{name: "concatenation", source: `name="idle" function _update() name="id".."le" end function _draw() cls() anim(name,20,30) end`},
		{name: "table", source: `names={"idle"} name="idle" function _update() name=names[1] end function _draw() cls() anim(name,20,30) end`},
		{name: "allocating-coordinate", source: `name="idle" function coordinate() local p={20} return p[1] end function _draw() cls() anim(name,coordinate(),30) end`},
		{name: "heap-rollback", source: staticGlobalAnimationSource, options: Options{DisableHeapElision: true}},
		{name: "proof-rollback", source: staticGlobalAnimationSource, options: Options{DisableNoAllocFunctionCalls: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := animationOptions()
			opts.Renderer = RendererMode4
			opts.DisableHeapElision = tc.options.DisableHeapElision
			opts.DisableNoAllocFunctionCalls = tc.options.DisableNoAllocFunctionCalls
			generated, report, err := TranspileWithReport("global-allocation.lua", tc.source, opts)
			if err != nil {
				t.Fatal(err)
			}
			if !report.HeapLinked || !strings.Contains(generated, "c.RegisterGlobal(&g.name)") || !strings.Contains(generated, "ctx.CollectIfNeeded()") {
				t.Fatal("potential allocation lost heap, global roots, or periodic collection")
			}
			compileGenerated(t, generated)
		})
	}
}

func TestNonAllocatingBoxedGlobalsDoNotInitializeHeap(t *testing.T) {
	source := `message="static" active=true absent=nil function _update() active=not active end function _draw() cls() end`
	generated, report, err := TranspileWithReport("boxed-globals.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.HeapLinked || strings.Contains(generated, "c.RegisterGlobal(") {
		t.Fatal("boxed static strings, booleans and nil do not require a Lua heap")
	}
	compileGenerated(t, generated)
}

const staticGlobalAnimationHarness = `package main
import (
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestStaticGlobalAnimationPixels(t *testing.T) {
 sheet:=make([]byte,128)
 for i:=range sheet {sheet[i]=byte((i%16)/8+1)}
 image:=rt.AssetImage{
  SheetW:16,SheetH:8,SpriteCols:2,SpriteRows:1,SpriteN:2,
  MapW:1,MapH:1,Sheet:sheet,Map:[]uint16{0},Flags:[]byte{0,0},
  PoseIndex:[]rt.PoseRange{{Count:1},{Start:1,Count:1}},
  PoseParts:[]rt.PosePart{{Tile:0},{Tile:1}},
  AnimationClips:[]rt.AnimationClip{{Name:"idle",Count:1,FPS:8,Loop:true},{Name:"hold",Start:1,Count:1,FPS:8,Loop:true}},
  AnimationFrames:[]uint16{0,1},
 }
 memory:=rt.AssetMemory{SheetW:16,SheetH:8,SpriteCols:2,SpriteRows:1,SpriteN:2,MapW:1,MapH:1,Sheet:make([]byte,128),Map:make([]uint16,1),Flags:make([]byte,2)}
 var c rt.Context
 if !c.Init(&image,&memory) {t.Fatal("init")}
 var g Game
 g.Init(&c)
 for frame:=0;frame<4;frame++ {
  g.Draw(&c)
  for y:=0;y<rt.ScreenHeight;y++ {for x:=0;x<rt.ScreenWidth;x++ {
   want:=byte(4)
   if y>=30&&y<38&&(x>=20&&x<28||x>=40&&x<48) {want=byte(frame%2+1)}
   if got:=c.Pget(x,y);got!=want {t.Fatalf("frame%d pixel(%d,%d)=%d want%d",frame,x,y,got,want)}
  }}
  g.Update(&c)
 }
}
`
