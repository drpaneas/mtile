package transpiler

import (
	"strings"
	"testing"
)

func TestTypedDrawCallsKeepNumericOperandsUnboxed(t *testing.T) {
	source := `
function xy() return 4.9, -0.9 end
function _draw()
  cls(0)
  camera(xy())
  map(0, 0, 0, 0, 0, 2, 2)
  spr(1, xy())
  spr(1, 20, 20, 1, 1, 0, "yes")
end
`
	for _, disabled := range []bool{false, true} {
		opts := rasterOptions()
		opts.DisableTypedDrawCalls = disabled
		got, err := Transpile("typed-draw.lua", source, opts)
		if err != nil {
			t.Fatal(err)
		}
		draw := generatedSection(got, "func (g *Game) gluDraw(")
		for _, name := range []string{"Spr", "Camera", "MapBGAt"} {
			if strings.Contains(draw, "c."+name+"V(") != disabled {
				t.Fatalf("%s Value ABI did not follow rollback=%v:\n%s", name, disabled, draw)
			}
			if strings.Contains(draw, "c."+name+"(") == disabled {
				t.Fatalf("%s typed ABI did not follow rollback=%v:\n%s", name, disabled, draw)
			}
		}
		compileGenerated(t, got)
	}
}

func TestTypedDrawCallsRetainGenericDefaultsAndMixedArguments(t *testing.T) {
	source := `
function generic(x)
  spr(1, x, 8, nil, false)
  camera(x, nil)
end
function _init() generic("text") end
function _draw()
  cls(0)
  map(0, 0, 0, 0, 0, 2, 2, nil)
end
`
	got, err := Transpile("generic-draw.lua", source, rasterOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SprV", "CameraV", "MapBGAtV"} {
		if !strings.Contains(got, "c."+name+"(") {
			t.Fatalf("mixed argument call lost %s fallback", name)
		}
	}
	compileGenerated(t, got)
}

func TestTypedDrawCallsPreservePixelsEvaluationAndTruthiness(t *testing.T) {
	source := `
trace = 0
flip = nil
function next_number(tag, value)
  trace = trace * 10 + tag
  return value
end
function coords() return -0.9, 3.9 end
function _draw()
  trace = 0
  cls(0)
  camera(coords())
  map(0, 0, 0, 32, 24, 2, 2)
  spr(next_number(1, 1), next_number(2, 9.9), next_number(3, -0.9), 1, 1, flip, false)
  spr(1, 20, 20, 1, 1, 0, "yes")
  spr(1, 40, 20)
  spr(1, 60, 20, nil, false)
  camera()
end
`
	harness := `package main
import (
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func initTypedCart(t *testing.T,c *rt.Context) {
 image:=rt.AssetImage{SheetW:SheetW,SheetH:SheetH,SpriteCols:SpriteCols,SpriteRows:SpriteRows,SpriteN:SpriteN,MapW:MapW,MapH:MapH,MapLayers:MapLayers,MapPlaneMask:MapPlaneMask,SheetReadOnly:SheetReadOnly,Sheet:sheetROM[:],SheetIndex:sheetIndexROM[:],Map:mapROM[:],Flags:flagROM[:]}
 cartData.SheetW,cartData.SheetH=SheetW,SheetH
 cartData.SpriteCols,cartData.SpriteRows,cartData.SpriteN=SpriteCols,SpriteRows,SpriteN
 cartData.MapW,cartData.MapH,cartData.MapLayers,cartData.MapPlaneMask=MapW,MapH,MapLayers,MapPlaneMask
 cartData.SheetReadOnly=SheetReadOnly
 cartData.Sheet,cartData.SheetIndex,cartData.Map,cartData.Flags=sheetRAM[:],sheetIndexRAM[:],mapRAM[:],flagRAM[:]
 if !c.Init(&image,&cartData) {t.Fatal("init")}
}
func TestTypedPixels(t *testing.T) {
 var g Game
 var got,want rt.Context
 initTypedCart(t,&got);initTypedCart(t,&want);g.Init(&got)
 table,err:=got.NewTable();if err!=rt.HeapOK {t.Fatal(err)}
 for _,flip:=range []rt.Value{rt.NilVal(),rt.BoolVal(false),rt.BoolVal(true),rt.IntVal(0),rt.IntVal(-2),got.InternStatic(""),table} {
  g.flip=flip;g.Draw(&got)
  want.Cls(0)
  want.CameraV(rt.NumVal(rt.FromFloat(-0.9)),rt.NumVal(rt.FromFloat(3.9)))
  want.MapBGAtV(0,rt.IntVal(0),rt.IntVal(0),rt.IntVal(0),rt.IntVal(32),rt.IntVal(24),rt.IntVal(2),rt.IntVal(2),rt.NilVal())
  want.SprV(rt.IntVal(1),rt.NumVal(rt.FromFloat(9.9)),rt.NumVal(rt.FromFloat(-0.9)),rt.IntVal(1),rt.IntVal(1),flip,rt.BoolVal(false))
  want.SprV(rt.IntVal(1),rt.IntVal(20),rt.IntVal(20),rt.IntVal(1),rt.IntVal(1),rt.IntVal(0),got.InternStatic("yes"))
  want.SprV(rt.IntVal(1),rt.IntVal(40),rt.IntVal(20),rt.NilVal(),rt.NilVal(),rt.NilVal(),rt.NilVal())
  want.SprV(rt.IntVal(1),rt.IntVal(60),rt.IntVal(20),rt.NilVal(),rt.BoolVal(false),rt.NilVal(),rt.NilVal())
  want.CameraV(rt.NilVal(),rt.NilVal())
  if g.trace!=rt.FromInt(123) {t.Fatalf("argument evaluation order=%v",g.trace)}
  if x,y:=got.CameraPos();x!=0||y!=0 {t.Fatalf("camera reset=(%d,%d)",x,y)}
  for y:=0;y<rt.ScreenHeight;y++ {for x:=0;x<rt.ScreenWidth;x++ {
   if got.Pget(x,y)!=want.Pget(x,y) {t.Fatalf("flip=%#v pixel(%d,%d) got %d want %d",flip,x,y,got.Pget(x,y),want.Pget(x,y))}
  }}
 }
}
`
	for _, disabled := range []bool{false, true} {
		opts := rasterOptions()
		opts.DisableTypedDrawCalls = disabled
		// Give the sprite a non-symmetric pattern so both flips are observable.
		for i := range opts.Sheet {
			opts.Sheet[i] = byte(i%13 + 1)
		}
		got, err := Transpile("typed-pixels.lua", source, opts)
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, got, harness, "TestTypedPixels")
	}
}
