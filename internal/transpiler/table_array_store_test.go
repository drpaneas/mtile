package transpiler

import (
	"strings"
	"testing"
)

func TestGeneratedIndexedStoreDefersRootArguments(t *testing.T) {
	generated, err := Transpile("indexed-store-arguments.lua", `
array = {0, 0, 0, 0}
function write_item(i, value) array[i] = value end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// A helper-only guard still constructs the variadic roots slice at every
	// call. Keep the allocating fallback behind the caller's nonallocating test.
	body := generatedSection(generated, "func (g *Game) write_item(")
	if !strings.Contains(body, "|| !c.TryTableSetArray(") || !strings.Contains(body, "rt.MaxTableArrayCapacity") || !strings.Contains(body, "\n\t\t_ = gluTableSetRooted(") {
		t.Fatalf("indexed store eagerly prepares fallback roots:\n%s", body)
	}
}

func TestGeneratedIndexedStoresPreserveEvaluationAndMixedGrowthGC(t *testing.T) {
	generated, err := Transpile("indexed-store-mixed-gc.lua", `
array = {0, 0, 0, 0}
buckets = {}
events = 0
tick = 0
function choose_array() events = events * 10 + 1 return array end
function choose_key() events = events * 10 + 2 return 3 end
function choose_value() events = events * 10 + 3 return "ordered" end
function ordered_store() choose_array()[choose_key()] = choose_value() end
function write_item(i, value) array[i] = value end
function _update()
 tick = tick + 1
 array[1], buckets[200], array[2] = {label = "first-" .. tick}, {label = "slow-" .. tick}, {label = "last-" .. tick}
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, `package main
import("fmt";"testing";rt "github.com/drpaneas/mtile/runtime")
func TestIndexedStores(t *testing.T) {
 var c rt.Context; var g Game; g.Init(&c)
 g.ordered_store(&c)
 if g.events!=rt.FromInt(123){t.Fatalf("evaluation order or duplicated side effects: %v",g.events)}
 if !c.ValuesEqual(c.TableGetInt(g.array,3),c.InternStatic("ordered")){t.Fatal("ordered store lost RHS")}
 for _,value:=range []rt.Value{rt.BoolVal(true),rt.IntVal(-9),c.InternStatic("tagged"),rt.NilVal()} {
  g.write_item(&c,rt.IntVal(4),value)
  if got:=c.TableGetInt(g.array,4);got!=value{t.Fatalf("tagged value: %#v != %#v",got,value)}
 }
 g.write_item(&c,rt.NumVal(rt.One+rt.One/2),rt.BoolVal(true))
 if c.TableGet(g.array,rt.NumVal(rt.One+rt.One/2))!=rt.BoolVal(true){t.Fatal("fractional-key fallback")}
 for _,key:=range []rt.Value{rt.IntVal(0),rt.IntVal(-1),rt.IntVal(rt.MaxTableArrayCapacity+1),c.InternStatic("hash")} {
  g.write_item(&c,key,rt.BoolVal(true))
  if c.TableGet(g.array,key)!=rt.BoolVal(true){t.Fatalf("hash-key fallback: %#v",key)}
 }
 for tick:=1;tick<=4;tick++ {
  var err rt.HeapError
  g.buckets,err=c.NewTableWithCaps(0,64);if err!=rt.HeapOK{t.Fatal(err)}
  for key:=1;key<=64;key++{if err:=c.TableSet(g.buckets,rt.IntVal(key),rt.BoolVal(true));err!=rt.HeapOK{t.Fatal(err)}}
  for c.PerformanceStats().HeapAllocated<56*1024-1024{if _,err:=c.NewString(make([]byte,256));err!=rt.HeapOK{t.Fatal(err)}}
  // The first array store takes the fast path. Hash growth in the second
  // store must collect while keeping the third RHS and every owner alive.
  g.Update(&c)
  if c.PerformanceStats().HeapAllocated>=12000{t.Fatal("fixture failed to force collection during mixed stores")}
  if c.LastHeapError()!=rt.HeapOK{t.Fatal(c.LastHeapError())}
  if err:=c.Collect();err!=rt.HeapOK{t.Fatal(err)}
  for i,item:=range []rt.Value{c.TableGetInt(g.array,1),c.TableGetInt(g.buckets,200),c.TableGetInt(g.array,2)} {
   label:=c.TableGet(item,c.InternStatic("label"))
   want:=fmt.Sprintf("%s-%d",[]string{"first","slow","last"}[i],tick)
   if !c.ValuesEqual(label,c.InternStatic(want)){t.Fatalf("mixed store lost %s: %#v",want,label)}
  }
 }
}
`, "TestIndexedStores")
}

func TestGeneratedArrayStorePreservesRootsAndFallback(t *testing.T) {
	generated, err := Transpile("array-store-roots.lua", `
owner = {0, 0, 0, 0}
function store_item(i, value) owner[i] = value end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, `package main
import ("testing"; rt "github.com/drpaneas/mtile/runtime")
func TestArrayStoreRoots(t *testing.T) {
 var c rt.Context
 var g Game; g.Init(&c)
 owner:=g.owner
 value,err:=c.NewString([]byte("retained"));if err!=rt.HeapOK{t.Fatal(err)}
 // Existing slots cannot collect, even with no room for temporary roots.
 for i:=0;i<rt.MaxLocalRoots;i++{if !c.PushLocal(owner){t.Fatal("setup roots")}}
 g.store_item(&c,rt.IntVal(1),value)
 if got:=c.TableGetInt(owner,1);got!=value{t.Fatal("nonallocating generated store failed with a full root stack")}
 // A hash/growth store must still use the rooted path, and leave scopes balanced.
 if err:=gluTableSetRooted(&c,owner,rt.IntVal(200),value);err!=rt.HeapOOM{t.Fatal("unrooted slow path",err)}
 c.PopLocals(rt.MaxLocalRoots)
 if err:=c.Collect();err!=rt.HeapOK{t.Fatal(err)}
 if !c.ValuesEqual(c.TableGetInt(owner,1),c.InternStatic("retained")){t.Fatal("fast store lost reference")}
 value=c.TableGetInt(owner,1)
 if err:=gluTableSetRooted(&c,owner,rt.IntVal(200),value);err!=rt.HeapOK{t.Fatal(err)}
 if err:=c.Collect();err!=rt.HeapOK{t.Fatal(err)}
 if !c.ValuesEqual(c.TableGetInt(owner,200),c.InternStatic("retained")){t.Fatal("slow store lost reference")}
 for i:=0;i<rt.MaxLocalRoots;i++{if !c.PushLocal(owner){t.Fatal("helper leaked roots")}}
 c.PopLocals(rt.MaxLocalRoots)
}
`, "TestArrayStoreRoots")
}
