package transpiler

import (
	"strings"
	"testing"
)

const mapViewAssetHarness = `
 image:=rt.AssetImage{SheetW:32,SheetH:8,SpriteCols:4,SpriteRows:1,SpriteN:4,MapW:4,MapH:3,MapLayers:4,MapPlaneMask:15,Sheet:make([]byte,256),Flags:make([]byte,4),Map:make([]uint16,48)}
 memory:=rt.AssetMemory{SheetW:32,SheetH:8,SpriteCols:4,SpriteRows:1,SpriteN:4,MapW:4,MapH:3,MapLayers:4,MapPlaneMask:15,Sheet:make([]byte,256),Flags:make([]byte,4),Map:make([]uint16,48)}
 if !c.Init(&image,&memory) {t.Fatal("asset initialization")}
`

func TestMapLayerViewsUseLightReadOnlyBorrowAndShareMixedLayers(t *testing.T) {
	source := `
total=0
function read_only()
  total=0
  for y=0,2 do for x=0,3 do total=total+mget(3,x,y) end end
end
function mixed()
  total=0
  for y=0,2 do
    total=total+mget(3,0,y)
    for x=0,3 do mset(3,x,y,2) end
  end
end
function dynamic_write(layer)
  for x=0,3 do
    mset(layer,x,0,3)
    total=total+mget(3,x,0)
  end
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestMapBorrowAccessKinds(t *testing.T) {
 var g Game;var c rt.Context
` + mapViewAssetHarness + `
 g.Init(&c)
 g.mixed(&c);g.read_only(&c)
 if g.total!=rt.FromInt(24) {t.Fatal("read-only borrow differs from write view")}
 g.total=0;g.dynamic_write(&c,rt.IntVal(3))
 if g.total!=rt.FromInt(12) {t.Fatal("read borrow missed edits through dynamic accessor")}
}
`
	for _, disabled := range []bool{false, true} {
		opts := Options{MapW: 4, MapH: 3, MapLayers: 4, Map: make([]uint16, 48), SheetW: 32, SheetH: 8, DisableMapLayerViews: disabled}
		got, err := Transpile("map-view-access-kinds.lua", source, opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, fn := range []string{"read_only", "mixed", "dynamic_write"} {
			body := generatedSection(got, "func (g *Game) "+fn+"(")
			wantRead, wantWrite := 0, 0
			if !disabled {
				if fn == "mixed" {
					wantWrite = 1
				} else {
					wantRead = 1
				}
			}
			if strings.Count(body, "c.BorrowMapLayerRead(") != wantRead || strings.Count(body, "c.BorrowMapLayer(") != wantWrite {
				t.Fatalf("%s incorrect borrow type:\n%s", fn, body)
			}
		}
		runGeneratedHarness(t, got, harness, "TestMapBorrowAccessKinds")
	}
}

func TestMapLayerViewsReuseNestedLoopsAndPreserveBounds(t *testing.T) {
	source := `
local saved={0,0,0,0,0,0,0,0,0,0,0,0}
total=0
function exercise()
  local i=1
  total=0
  for y=-1,3 do
    for x=-1,4 do
      mset(3,x,y,2)
      mset(x,y,1)
      total=total+mget(3,x,y)+mget(x,y)
      mset(3,x,y,-1)
      mset(3,x,y,4)
      if x>=0 and x<4 and y>=0 and y<3 then
        saved[i]=mget(3,x,y)
        i=i+1
      end
    end
  end
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestMapViewLoops(t *testing.T) {
 var g Game;var c rt.Context
` + mapViewAssetHarness + `
 g.Init(&c)
 for pass:=0;pass<3;pass++ {
  g.exercise(&c)
  if g.total!=rt.FromInt(36) {t.Fatalf("sum=%v",g.total)}
  for y:=0;y<3;y++ {for x:=0;x<4;x++ {
   if c.MgetLayer(0,x,y)!=1||c.MgetLayer(3,x,y)!=2||c.MgetLayer(1,x,y)!=0||c.MgetLayer(2,x,y)!=0 {t.Fatal("layer/bounds/invalid ID behavior changed")}
   if c.NumberArrayGet(g.saved,y*4+x+1)!=rt.FromInt(2) {t.Fatal("map and numeric-array views disagree")}
  }}
 }
}
`
	for _, disabled := range []bool{false, true} {
		opts := Options{MapW: 4, MapH: 3, MapLayers: 4, Map: make([]uint16, 48), SheetW: 32, SheetH: 8, DisableMapLayerViews: disabled}
		got, err := Transpile("map-view-loop.lua", source, opts)
		if err != nil {
			t.Fatal(err)
		}
		body := generatedSection(got, "func (g *Game) exercise(")
		want := 2
		if disabled {
			want = 0
		}
		if count := strings.Count(body, "c.BorrowMapLayer("); count != want {
			t.Fatalf("borrow count%d want%d:\n%s", count, want, body)
		}
		runGeneratedHarness(t, got, harness, "TestMapViewLoops")
	}
}

func TestMapLayerViewsBorrowAfterSideEffectingBounds(t *testing.T) {
	source := `
trace=0
function bound(tag,value)
  trace=trace*10+tag
  mset(3,0,0,tag)
  for i=1,1100 do local trash={"temporary-"..i} end
  return value
end
function exercise()
  trace=0
  for x=bound(1,0),bound(2,3),bound(3,1) do
    mset(3,x,1,mget(3,0,0))
  end
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestMapViewBoundOrder(t *testing.T) {
 var g Game;var c rt.Context
` + mapViewAssetHarness + `
 g.Init(&c);g.exercise(&c)
 if g.trace!=rt.FromInt(123) {t.Fatal("bounds reordered")}
 for x:=0;x<4;x++ {if c.MgetLayer(3,x,1)!=3 {t.Fatal("map edits in bounds were not observed")}}
}
`
	for _, disabled := range []bool{false, true} {
		opts := Options{MapW: 4, MapH: 3, MapLayers: 4, Map: make([]uint16, 48), SheetW: 32, SheetH: 8, DisableMapLayerViews: disabled}
		got, err := Transpile("map-view-bounds.lua", source, opts)
		if err != nil {
			t.Fatal(err)
		}
		body := generatedSection(got, "func (g *Game) exercise(")
		if !disabled && strings.Index(body, "c.BorrowMapLayer(") < strings.LastIndex(body, "g.__glu_call_bound(") {
			t.Fatalf("map borrowed before bounds evaluated:\n%s", body)
		}
		runGeneratedHarness(t, got, harness, "TestMapViewBoundOrder")
	}
}

func TestMapLayerViewsRejectUnsafeDynamicAndStreamingRegions(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"allocation", `local t={} mset(3,i,0,1)`},
		{"unknown-call", `unknown() mset(3,i,0,1)`},
		{"drawing", `cls(0) mset(3,i,0,1)`},
		{"dynamic-layer", `mset(layer,i,0,1)`},
		{"shadowed-layer", `local layer=2 mset(layer,i,0,1)`},
		{"expanded-results", `mset(3,coords())`},
		{"streaming", `mset(3,i,0,1)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{MapW: 4, MapH: 3, MapLayers: 4, Map: make([]uint16, 48), SheetW: 32, SheetH: 8}
			if tc.name == "streaming" {
				opts.MapW, opts.MapH, opts.Map = 129, 128, make([]uint16, 129*128*4)
			}
			source := "local layer=3\nfunction unknown() end\nfunction coords() return 0,0,1 end\nfunction exercise() for i=0,3 do " + tc.body + " end end"
			got, err := Transpile("map-view-rejection.lua", source, opts)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(generatedSection(got, "func (g *Game) exercise("), "c.BorrowMapLayer(") {
				t.Fatal("unproven region borrowed map storage")
			}
			compileGenerated(t, got)
		})
	}
}
