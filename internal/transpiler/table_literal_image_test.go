package transpiler

import (
	"strings"
	"testing"
)

func TestTableLiteralImagesCopyConstantsAndOverlayDynamics(t *testing.T) {
	source := `
result = nil
function constants() result = {a=1,b=false,c="text"} end
function mixed(value) result = {x=rnd(8),y=1,z=value==nil,label="ok"} end
function unknown(value) result = {x=value,y=1} end
function once() result = {a=1,b=2} end
function twice() result = {a=1,b=2} end
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("literal-images.lua", source, Options{DisableTableLiteralImages: disabled})
		if err != nil {
			t.Fatal(err)
		}
		constants := generatedSection(got, "func (g *Game) constants(")
		mixed := generatedSection(got, "func (g *Game) mixed(")
		unknown := generatedSection(got, "func (g *Game) unknown(")
		if strings.Contains(constants, "NewTableFromImage(") != !disabled {
			t.Fatalf("constants image disabled=%v:\n%s", disabled, constants)
		}
		if !disabled && (strings.Contains(constants, ".InitKnown(") || strings.Contains(constants, ".StorePlanned(") || strings.Contains(constants, ".StorePlannedNumber(")) {
			t.Fatalf("constant image still wrote fields:\n%s", constants)
		}
		if strings.Contains(mixed, "NewTableFromImage(") != !disabled {
			t.Fatalf("mixed image disabled=%v:\n%s", disabled, mixed)
		}
		if !disabled && (strings.Count(mixed, ".StorePlannedNumber(") != 1 || strings.Count(mixed, ".StorePlanned(") != 1) {
			t.Fatalf("mixed overlay count:\n%s", mixed)
		}
		if strings.Contains(unknown, "NewTableFromImage(") {
			t.Fatalf("unknown constructor used an image:\n%s", unknown)
		}
		once := generatedSection(got, "func (g *Game) once(")
		twice := generatedSection(got, "func (g *Game) twice(")
		if disabled {
			if strings.Contains(got, "var gluTableImg") {
				t.Fatal("disabled images still emitted ROM bytes")
			}
		} else if !strings.Contains(once, "NewTableFromImage(") || !strings.Contains(twice, "NewTableFromImage(") {
			t.Fatal("identical constructors did not use a ROM image")
		} else {
			onceImg := once[strings.Index(once, "NewTableFromImage("):]
			twiceImg := twice[strings.Index(twice, "NewTableFromImage("):]
			if i := strings.Index(onceImg, ")"); i >= 0 {
				onceImg = onceImg[:i]
			}
			if i := strings.Index(twiceImg, ")"); i >= 0 {
				twiceImg = twiceImg[:i]
			}
			if onceImg != twiceImg {
				t.Fatalf("identical constructors used %q and %q", onceImg, twiceImg)
			}
			name := strings.TrimSuffix(strings.TrimPrefix(onceImg, "NewTableFromImage("), "[:]")
			if strings.Count(got, "var "+name+" =") != 1 {
				t.Fatalf("shared image %s was not interned once", name)
			}
		}
		compileGenerated(t, got)
	}
}

func TestTableLiteralImagesOverlayNestedTables(t *testing.T) {
	source := `
result = nil
function construct()
  result = {x=1, nested={}, inner={a=2,b=false}, label="ok"}
end
`
	got, err := Transpile("nested-literal-image.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	body := generatedSection(got, "func (g *Game) construct(")
	if !strings.Contains(body, "NewTableFromImage(") {
		t.Fatalf("nested table constructor did not use a ROM image:\n%s", body)
	}
	if strings.Count(body, ".StorePlanned(") != 2 {
		t.Fatalf("nested overlays:\n%s", body)
	}
	runGeneratedHarness(t, got, `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestNestedLiteralImage(t *testing.T) {
 var g Game
 var c rt.Context
 g.Init(&c)
 g.construct(&c)
 get:=func(t rt.Value,k string)rt.Value{return c.TableGet(t,c.InternStatic(k))}
 if get(g.result,"x")!=rt.IntVal(1)||!c.ValuesEqual(get(g.result,"label"),c.InternStatic("ok")) {
  t.Fatal("constant parent fields changed")
 }
 nested:=get(g.result,"nested")
 if nested.Kind!=rt.KindTable||c.TableLen(nested)!=0 {
  t.Fatalf("empty nested table = %#v", nested)
 }
 inner:=get(g.result,"inner")
 if get(inner,"a")!=rt.IntVal(2)||get(inner,"b")!=rt.BoolVal(false) {
  t.Fatal("nested record fields changed")
 }
}
`, "TestNestedLiteralImage")
}

func TestTableLiteralImagesOverlayNestedTablesAcrossCollection(t *testing.T) {
	source := `
result = nil
function construct()
  result = {x=1, nested={}, inner={a=2,b=false}, label="ok"}
end
`
	got, err := Transpile("nested-literal-image-gc.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	body := generatedSection(got, "func (g *Game) construct(")
	store := 0
	for {
		next := strings.Index(body[store:], ".StorePlanned(")
		if next < 0 {
			break
		}
		at := store + next
		borrow := strings.LastIndex(body[:at], "BorrowTableLiteral(")
		nested := strings.LastIndex(body[:at], "NewTableFromImage(")
		if caps := strings.LastIndex(body[:at], "NewTableWithCaps("); caps > nested {
			nested = caps
		}
		if borrow >= 0 && nested >= 0 && borrow < nested {
			t.Fatalf("nested overlay borrowed before the nested table allocated:\n%s", body)
		}
		store = at + 1
	}
	runGeneratedHarness(t, got, `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestNestedLiteralImageGC(t *testing.T) {
 var g Game
 var c rt.Context
 g.Init(&c)
 for c.PerformanceStats().HeapAllocated < 56*1024-2048 {
  if _,err:=c.NewString(make([]byte,256));err!=rt.HeapOK {t.Fatal(err)}
 }
 g.construct(&c)
 if c.LastHeapError()!=rt.HeapOK {t.Fatal(c.LastHeapError())}
 get:=func(t rt.Value,k string)rt.Value{return c.TableGet(t,c.InternStatic(k))}
 nested:=get(g.result,"nested")
 if nested.Kind!=rt.KindTable {t.Fatalf("nested lost across collection: %#v", nested)}
 inner:=get(g.result,"inner")
 if get(inner,"a")!=rt.IntVal(2)||get(inner,"b")!=rt.BoolVal(false) {
  t.Fatalf("inner overlay lost across collection: %#v", inner)
 }
}
`, "TestNestedLiteralImageGC")
}

func TestTableLiteralImagesPreserveLayoutRNGAndOverlayValues(t *testing.T) {
	source := `
result = nil
function construct(flag)
  result = {x=rnd(20),y=4,enabled=false,title="label",empty="",flag=flag==nil}
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestLiteralImageValues(t *testing.T) {
 var g Game;var c,random rt.Context;g.Init(&c)
 for _,flag:=range []rt.Value{rt.NilVal(),rt.BoolVal(true)} {
  c.Srand(rt.FromInt(11));random.Srand(rt.FromInt(11));g.construct(&c,flag)
  get:=func(k string)rt.Value{return c.TableGet(g.result,c.InternStatic(k))}
  if get("x")!=rt.NumVal(random.Rnd(rt.FromInt(20))) {t.Fatal("RNG order changed")}
  if get("y")!=rt.IntVal(4)||get("enabled")!=rt.BoolVal(false) {t.Fatal("constant fields changed")}
  if !c.ValuesEqual(get("title"),c.InternStatic("label"))||!c.ValuesEqual(get("empty"),c.InternStatic("")) {t.Fatal("static strings changed")}
  if get("flag")!=rt.BoolVal(flag.Kind==rt.KindNil) {t.Fatal("overlay boolean changed")}
  want,e:=c.NewTableWithCaps(1,6);if e!=rt.HeapOK {t.Fatal(e)}
  names:=[]string{"x","y","enabled","title","empty","flag"}
  values:=[]rt.Value{get("x"),get("y"),get("enabled"),get("title"),get("empty"),get("flag")}
  for i,name:=range names {if e:=c.TableSet(want,c.InternStatic(name),values[i]);e!=rt.HeapOK {t.Fatal(e)}}
  if c.TableSlotCount(g.result)!=c.TableSlotCount(want) {t.Fatal("capacity changed")}
  for i:=0;i<c.TableSlotCount(want);i++ {
   k:=c.TableKeyAt(want,i);gotKey:=c.TableKeyAt(g.result,i)
   if !c.ValuesEqual(k,gotKey)||!c.ValuesEqual(c.TableGet(want,k),c.TableGet(g.result,gotKey)) {t.Fatalf("slot %d differs",i)}
  }
 }
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("literal-image-layout.lua", source, Options{DisableTableLiteralImages: disabled})
		if err != nil {
			t.Fatal(err)
		}
		if !disabled && !strings.Contains(generatedSection(got, "func (g *Game) construct("), "NewTableFromImage(") {
			t.Fatal("layout harness did not use a constructor image")
		}
		runGeneratedHarness(t, got, harness, "TestLiteralImageValues")
	}
}

func TestTableLiteralImagesUseNumericConstructorParameters(t *testing.T) {
	source := `
markers = {}
entities = {}
function marker_id(n)
  return n + 1
end
function collect_markers()
  for i = 1, 3 do
    add(markers, {kind = marker_id(i), x = i * 8, y = 0})
  end
end
function spawn(kind, x, y)
  local e = {kind = kind, spr = kind, x = x, y = y, vx = 0, alive = true}
  add(entities, e)
end
function reset()
  entities = {}
  for marker in all(markers) do
    spawn(marker.kind, marker.x, marker.y)
  end
end
function burst()
  spawn(-1, 4, 5)
end
`
	got, err := Transpile("numeric-constructor-params.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	body := generatedSection(got, "func (g *Game) spawn(")
	if !strings.Contains(body, "NewTableFromImage(") {
		t.Fatalf("numeric constructor parameters did not use a ROM image:\n%s", body)
	}
	if strings.Contains(body, ".InitStatic(") {
		t.Fatalf("numeric constructor image still used guarded stores:\n%s", body)
	}
	runGeneratedHarness(t, got, `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestNumericConstructorParamValues(t *testing.T) {
 var g Game
 var c rt.Context
 g.Init(&c)
 g.collect_markers(&c)
 g.reset(&c)
 get:=func(index int,k string)rt.Value{
  e:=c.TableGetInt(g.entities,index)
  return c.TableGet(e,c.InternStatic(k))
 }
 if get(1,"kind")!=rt.IntVal(2)||get(1,"spr")!=rt.IntVal(2)||get(1,"x")!=rt.IntVal(8)||get(1,"y")!=rt.IntVal(0) {
  t.Fatal("marker-backed spawn fields changed")
 }
 if get(1,"vx")!=rt.IntVal(0)||get(1,"alive")!=rt.BoolVal(true) {
  t.Fatal("constant spawn fields changed")
 }
 g.burst(&c)
 if get(4,"kind")!=rt.IntVal(-1)||get(4,"x")!=rt.IntVal(4)||get(4,"y")!=rt.IntVal(5) {
  t.Fatal("literal spawn fields changed")
 }
}
`, "TestNumericConstructorParamValues")
}

func TestTableLiteralImagesFromNumericMapTileHelpers(t *testing.T) {
	source := `
markers = {}
entities = {}
function tile_kind(tile)
  local n = flr(tile / 30) * 16 + tile % 30
  if tile % 30 >= 16 then return 0 end
  if n == 8 or n == 11 then return n end
  return 0
end
function collect()
  for x = 0, 1 do
    local kind = tile_kind(mget(3, x, 0))
    add(markers, {kind = kind, x = x * 8, y = 0})
  end
end
function spawn(kind, x, y)
  local e = {kind = kind, spr = kind, x = x, y = y, vx = 0, alive = true}
  add(entities, e)
end
function reset()
  entities = {}
  for marker in all(markers) do
    spawn(marker.kind, marker.x, marker.y)
  end
end
function burst()
  spawn(-1, 4, 5)
end
`
	got, err := Transpile("map-tile-constructor.lua", source, Options{MapW: 4, MapH: 1, MapLayers: 4})
	if err != nil {
		t.Fatal(err)
	}
	body := generatedSection(got, "func (g *Game) spawn(")
	if !strings.Contains(body, "NewTableFromImage(") {
		t.Fatalf("map-tile constructor parameters did not use a ROM image:\n%s", body)
	}
	if !strings.Contains(got, "func (g *Game) tile_kind(c *rt.Context, a0 rt.Number) rt.Number") {
		t.Fatalf("mget result did not narrow the tile helper:\n%s", generatedSection(got, "func (g *Game) tile_kind("))
	}
	compileGenerated(t, got)
}

func TestTableLiteralImagesSkipRootMachineryWithoutLiveValues(t *testing.T) {
	source := `
result = nil
function empty() result = {} end
function mixed() result = {x=rnd(8),y=1,z=false} end
`
	got, err := Transpile("literal-image-no-roots.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"empty", "mixed"} {
		body := generatedSection(got, "func (g *Game) "+name+"(")
		if strings.Contains(body, "PushLocal(") || strings.Contains(body, "PopLocals(") {
			t.Fatalf("%s still rooted a no-alloc constructor:\n%s", name, body)
		}
		if strings.Contains(body, "tblFail") {
			t.Fatalf("%s still used constructor fail flags:\n%s", name, body)
		}
	}
	mixed := generatedSection(got, "func (g *Game) mixed(")
	if strings.Count(mixed, ".StorePlannedNumber(") != 1 || strings.Contains(mixed, "if !") {
		t.Fatalf("trusted image overlay was not compact:\n%s", mixed)
	}
	runGeneratedHarness(t, got, `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestNoRootConstructors(t *testing.T) {
 var g Game;var c,random rt.Context;g.Init(&c)
 g.empty(&c)
 if g.result.Kind!=rt.KindTable||c.TableLen(g.result)!=0 {t.Fatal("empty")}
 c.Srand(rt.FromInt(4));random.Srand(rt.FromInt(4));g.mixed(&c)
 get:=func(k string)rt.Value{return c.TableGet(g.result,c.InternStatic(k))}
 if get("x")!=rt.NumVal(random.Rnd(rt.FromInt(8)))||get("y")!=rt.IntVal(1)||get("z")!=rt.BoolVal(false) {t.Fatal("mixed")}
}
`, "TestNoRootConstructors")
}
