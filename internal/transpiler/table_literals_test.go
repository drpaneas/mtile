package transpiler

import (
	"strings"
	"testing"
)

func tableLiteralBorrowIndex(body string) int {
	if index := strings.Index(body, "c.BorrowTableLiteral("); index >= 0 {
		return index
	}
	return strings.Index(body, "c.BorrowFreshTableLiteral(")
}

func TestStaticTableLiteralPlansOnlyProvenPrefix(t *testing.T) {
	source := `
function primitives() return {a=1,b=false,c="text"} end
function dynamic(value) return {a=1,b=value,c=3} end
function duplicate() return {a=1,a=2,c=3} end
function unknown_key(key) return {a=1,[key]=2,c=3} end
function positional(value) return {value,a=1,value,b=2} end
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("literal-prefix.lua", source, Options{DisableStaticTableLiterals: disabled, DisableTableLiteralImages: true})
		if err != nil {
			t.Fatal(err)
		}
		for name, count := range map[string]int{"primitives": 3, "dynamic": 3, "duplicate": 1, "unknown_key": 1, "positional": 2} {
			if disabled {
				count = 0
			}
			body := generatedSection(got, "func (g *Game) "+name+"(")
			if actual := strings.Count(body, "c.TableInitStatic(") + strings.Count(body, ".InitStatic(") + strings.Count(body, ".InitKnown("); actual != count {
				t.Fatalf("%s planned %d fields, want %d:\n%s", name, actual, count, body)
			}
		}
		compileGenerated(t, got)
	}
}

func TestTableLiteralViewsOnlyBorrowCompleteNonAllocatingConstructors(t *testing.T) {
	source := `
function plain(value) return {x=value,y=rnd(3),active=false} end
function read_field(value) return {x=value.x,y=value.y} end
function no_alloc(value) return value+1 end
function call_known() return {x=no_alloc(1),y=no_alloc(2)} end
function allocate() return {} end
function call_unknown() return {x=allocate(),y=2} end
function nested() return {x={},y=2} end
function concat(value) return {x="s"..value,y=2} end
function duplicate() return {x=1,x=2,y=3} end
function positional() return {1,x=2,y=3} end
function single() return {x=1} end
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("literal-view-eligibility.lua", source, Options{DisableTableLiteralViews: disabled, DisableTableLiteralImages: true})
		if err != nil {
			t.Fatal(err)
		}
		for name, borrow := range map[string]bool{"plain": true, "read_field": true, "call_known": true,
			"call_unknown": false, "nested": false, "concat": false, "duplicate": false, "positional": false, "single": false} {
			body := generatedSection(got, "func (g *Game) "+name+"(")
			if (tableLiteralBorrowIndex(body) >= 0) != (borrow && !disabled) {
				t.Fatalf("%s unexpected constructor borrow:\n%s", name, body)
			}
			if borrow && !disabled {
				if strings.Index(body, "c.PushLocal(") > tableLiteralBorrowIndex(body) {
					t.Fatalf("%s borrowed before rooting:\n%s", name, body)
				}
			}
		}
		compileGenerated(t, got)
	}
}

func TestStaticTableLiteralLayoutMatchesGenericIncludingCollisions(t *testing.T) {
	source := `
result = nil
function make_record()
  result = {x=1,y=2,spr=3,id=4,w=5,h=6,vx=7,vy=8,remx=9,remy=10,
    dead=false,grounded=true,flip=false,jump=0,dash=0,ready=true,
    name="thing",label="",step=9,age=11,timer=12,count=13,kind=14,
    active=true,visible=true,touched=false,berry=0,last=0}
end
function _init() make_record() end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestLiteralLayout(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 names:=[]string{"x","y","spr","id","w","h","vx","vy","remx","remy","dead","grounded","flip","jump","dash","ready","name","label","step","age","timer","count","kind","active","visible","touched","berry","last"}
 values:=[]rt.Value{rt.IntVal(1),rt.IntVal(2),rt.IntVal(3),rt.IntVal(4),rt.IntVal(5),rt.IntVal(6),rt.IntVal(7),rt.IntVal(8),rt.IntVal(9),rt.IntVal(10),rt.BoolVal(false),rt.BoolVal(true),rt.BoolVal(false),rt.IntVal(0),rt.IntVal(0),rt.BoolVal(true),c.InternStatic("thing"),c.InternStatic(""),rt.IntVal(9),rt.IntVal(11),rt.IntVal(12),rt.IntVal(13),rt.IntVal(14),rt.BoolVal(true),rt.BoolVal(true),rt.BoolVal(false),rt.IntVal(0),rt.IntVal(0)}
 want,e:=c.NewTableWithCaps(1,len(names));if e!=rt.HeapOK {t.Fatal(e)}
 for i,name:=range names {if e:=c.TableSet(want,c.InternStatic(name),values[i]);e!=rt.HeapOK {t.Fatal(e)}}
 for repeat:=0;repeat<3;repeat++ {
  g.make_record(&c)
  if c.TableSlotCount(g.result)!=c.TableSlotCount(want) {t.Fatal("capacity changed")}
  for i:=0;i<c.TableSlotCount(want);i++ {
   k:=c.TableKeyAt(want,i);gotKey:=c.TableKeyAt(g.result,i)
   if !c.ValuesEqual(k,gotKey)||!c.ValuesEqual(c.TableGet(want,k),c.TableGet(g.result,gotKey)) {t.Fatalf("slot %d key/value differs, got key%#v want%#v",i,gotKey,k)}
  }
 }
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("literal-layout.lua", source, Options{DisableStaticTableLiterals: disabled})
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, got, harness, "TestLiteralLayout")
	}
}

func TestStaticTableLiteralPreservesNilDuplicatesOrderAndGCRoots(t *testing.T) {
	source := `
result = nil
trace = 0
function pressure(tag)
  trace = trace * 10 + tag
  for i=1,1100 do local trash={"temporary-"..i} end
  return 10 + tag
end
function constructor(value)
  trace = 0
  local kept = {text="keep-"..42}
  result = {a=pressure(1),b=false,c=pressure(2),nested=kept,d=value,e=pressure(3),a=99}
end
function _init() constructor(nil) constructor(true) end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestLiteralEffects(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 for _,v:=range []rt.Value{rt.NilVal(),rt.BoolVal(false),rt.BoolVal(true)} {
  g.constructor(&c,v)
  if g.trace!=rt.FromInt(123) {t.Fatalf("evaluation order%v",g.trace)}
  get:=func(k string)rt.Value{return c.TableGet(g.result,c.InternStatic(k))}
  if get("a")!=rt.IntVal(99)||get("b")!=rt.BoolVal(false)||get("c")!=rt.IntVal(12)||get("d")!=v||get("e")!=rt.IntVal(13) {t.Fatal("constructor field values changed")}
  nested:=get("nested")
  if !c.ValuesEqual(c.TableGet(nested,c.InternStatic("text")),c.InternStatic("keep-42")) {t.Fatal("earlier local lost across RHS allocation")}
 }
}

`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("literal-effects.lua", source, Options{DisableStaticTableLiterals: disabled})
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, got, harness, "TestLiteralEffects")
	}
}

func TestStaticTableLiteralUnknownValuesPreserveEveryNilLayout(t *testing.T) {
	source := `
result = nil
function construct(a,b,c,d)
  result = {x=a,y=b,age=c,dy=d}
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestNilLayouts(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 names:=[]string{"x","y","age","dy"}
 for mask:=0;mask<16;mask++ {
  values:=[4]rt.Value{}
  for i:=range values {if mask&(1<<i)!=0 {values[i]=rt.IntVal(i+1)}}
  g.construct(&c,values[0],values[1],values[2],values[3])
  want,e:=c.NewTableWithCaps(1,4);if e!=rt.HeapOK {t.Fatal(e)}
  for i,name:=range names {if e:=c.TableSet(want,c.InternStatic(name),values[i]);e!=rt.HeapOK {t.Fatal(e)}}
  for i:=0;i<c.TableSlotCount(want);i++ {
   k:=c.TableKeyAt(want,i);gotKey:=c.TableKeyAt(g.result,i)
   if !c.ValuesEqual(k,gotKey)||!c.ValuesEqual(c.TableGet(want,k),c.TableGet(g.result,gotKey)) {t.Fatalf("mask %d slot %d differs",mask,i)}
  }
 }
}

`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("literal-nil-layouts.lua", source, Options{DisableStaticTableLiterals: disabled})
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, got, harness, "TestNilLayouts")
	}
}

func TestTableLiteralViewsPreserveAllocationRootsAndRootFailure(t *testing.T) {
	source := `
result=nil
trace=0
function tag(n) trace=trace*10+n return n end
function simple() result={x=tag(1),y=tag(2)} end
function nested() result={inner={x=tag(1)},y=tag(2)} end
function pressure()
  for i=1,1100 do local trash={"temporary-"..i} end
end
function construct(n)
  local kept={"retained-"..n}
  pressure()
  trace=0
  result={x=kept[1],y=tag(1),z=tag(2)}
end
`
	harness := `package main
import("fmt";"testing";rt "github.com/drpaneas/mtile/runtime")
func TestLiteralBorrowRoots(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 for i:=0;i<4;i++ {
  g.construct(&c,rt.IntVal(i))
  get:=func(k string)rt.Value{return c.TableGet(g.result,c.InternStatic(k))}
  if !c.ValuesEqual(get("x"),c.InternStatic(fmt.Sprint("retained-",i))) {t.Fatal("earlier value lost across constructor allocation")}
  if g.trace!=rt.FromInt(12)||get("y")!=rt.IntVal(1)||get("z")!=rt.IntVal(2) {t.Fatal("RHS evaluation order changed")}
 }
 g.trace=0
 roots:=0
 for c.PushLocal(rt.NilVal()) {roots++}
 g.simple(&c)
 if g.trace!=rt.FromInt(12)||g.result.Kind!=rt.KindTable {t.Fatal("no-alloc constructor still required a local root slot")}
 g.trace=0
 g.result=rt.NilVal()
 g.nested(&c)
 if g.trace!=0||g.result.Kind!=rt.KindNil {t.Fatal("RHS executed after constructor root failure")}
 c.PopLocals(roots)
 g.nested(&c)
 if g.trace!=rt.FromInt(12)||g.result.Kind!=rt.KindTable {t.Fatal("root failure changed later construction")}
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("literal-view-roots.lua", source, Options{DisableTableLiteralViews: disabled})
		if err != nil {
			t.Fatal(err)
		}
		if !disabled && tableLiteralBorrowIndex(generatedSection(got, "func (g *Game) construct(")) < 0 {
			t.Fatal("GC/root harness did not exercise a borrowed constructor")
		}
		runGeneratedHarness(t, got, harness, "TestLiteralBorrowRoots")
	}
}
