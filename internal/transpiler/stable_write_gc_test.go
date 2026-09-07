package transpiler

import (
	"strings"
	"testing"
)

func TestStableWritesAfterGrowingMultiAssignmentAndOwnerReplacement(t *testing.T) {
	source := `
owner = {payload = false}
buckets = {}
tick = 0
function _update()
  tick = tick + 1
  owner, buckets[200], owner.payload = {payload = false}, owner, {label = "kept-" .. tick}
end
`
	harness := `package main
import (
 "fmt"
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestGrowingMultiAssignment(t *testing.T) {
 var g Game; var c rt.Context; g.Init(&c)
 for tick:=1;tick<=4;tick++ {
  var err rt.HeapError
  g.buckets,err=c.NewTableWithCaps(0,64);if err!=rt.HeapOK{t.Fatal(err)}
  for key:=1;key<=64;key++ {
   if err:=c.TableSet(g.buckets,rt.IntVal(key),rt.BoolVal(true));err!=rt.HeapOK{t.Fatal(err)}
  }
  // RHS allocations fit, but the first indexed store's hash growth cannot.
  // It must collect with the later RHS object and old LHS owner still live.
  for c.PerformanceStats().HeapAllocated < 56*1024-1024 {
   if _,err:=c.NewString(make([]byte,256));err!=rt.HeapOK{t.Fatal(err)}
  }
  g.Update(&c)
  if c.PerformanceStats().HeapAllocated>=12000 {t.Fatal("test failed to force a collection during the assignment")}
  if c.LastHeapError()!=rt.HeapOK {t.Fatal(c.LastHeapError())}
  old:=c.TableGetInt(g.buckets,200)
  if old==g.owner {t.Fatal("LHS owner was not captured before replacement")}
  payload:=c.TableGet(old,c.InternStatic("payload"))
  label:=c.TableGet(payload,c.InternStatic("label"))
  if !c.ValuesEqual(label,c.InternStatic(fmt.Sprintf("kept-%d",tick))) {t.Fatal("lost later RHS object/string or old LHS",label)}
  if got:=c.TableGet(g.owner,c.InternStatic("payload"));got!=rt.BoolVal(false) {t.Fatal("cached store used replacement owner",got)}
 }
}
`
	for _, disabled := range []bool{false, true} {
		generated, err := Transpile("stable-multiassignment-gc.lua", source, Options{DisableStableWriteSpecialization: disabled})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(generated, "c.TableSetCached(") == disabled {
			t.Fatal("cached-store callsite did not follow the option")
		}
		runGeneratedHarness(t, generated, harness, "TestGrowingMultiAssignment")
	}
}

func TestGeneratedStableWritesSurviveAllocatingRHSAndMovingGC(t *testing.T) {
	generated, err := Transpile("stable-write-gc.lua", `
local owner = {payload = false, count = 0}
local tick = 0
function allocating_rhs(n)
  local label = "payload-" .. n
  for i = 1, 1100 do
    local garbage = {i, label .. i}
  end
  if n % 2 == 0 then return {label = label, nested = {n}} end
  return label
end
function _update()
  tick = tick + 1
  owner.payload = allocating_rhs(tick)
  owner.count = tick
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, `package main
import (
 "fmt"
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestWarmWritesAfterGC(t *testing.T){
 var c rt.Context
 var g Game
 g.Init(&c)
 for tick:=1;tick<=12;tick++{
  g.Update(&c)
  if err:=c.Collect();err!=rt.HeapOK{t.Fatal(err)}
  payload:=c.TableGet(g.owner,c.InternStatic("payload"))
  label:=payload
  if tick%2==0{
   label=c.TableGet(payload,c.InternStatic("label"))
   nested:=c.TableGet(payload,c.InternStatic("nested"))
   if got:=c.TableGetInt(nested,1);got!=rt.IntVal(tick){t.Fatalf("nested %d: %#v",tick,got)}
  }
  if !c.ValuesEqual(label,c.InternStatic(fmt.Sprintf("payload-%d",tick))){t.Fatalf("lost payload at %d",tick)}
  if got:=c.TableGet(g.owner,c.InternStatic("count"));got!=rt.IntVal(tick){t.Fatalf("numeric warm write %d: %#v",tick,got)}
 }
}
`, "TestWarmWritesAfterGC")
}

func TestGeneratedStableWriteColdPathPreservesRootScopes(t *testing.T) {
	generated, err := Transpile("cold-stable-write.lua", "function _update() end", Options{})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, `package main
import (
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestColdStableWrite(t *testing.T){
 var c rt.Context
 owner,err:=c.NewTableWithCaps(0,1);if err!=rt.HeapOK{t.Fatal(err)}
 c.RegisterGlobal(&owner)
 value,err:=c.NewString([]byte("kept by owner"));if err!=rt.HeapOK{t.Fatal(err)}
 key:=c.InternStatic("payload")
 var cache rt.TableFieldCache
 for i:=0;i<rt.MaxLocalRoots-1;i++{if !c.PushLocal(owner){t.Fatal("setup roots")}}
 if err:=gluTableSetStable(&c,owner,key,value,&cache);err!=rt.HeapOOM{t.Fatalf("cold root exhaustion: %v",err)}
 if !c.PushLocal(owner){t.Fatal("cold helper leaked partial roots")}
 c.PopLocals(rt.MaxLocalRoots)
 if err:=gluTableSetStable(&c,owner,key,value,&cache);err!=rt.HeapOK{t.Fatal(err)}
 if cache==0{t.Fatal("cold lookup did not prime cache")}
 if err:=c.Collect();err!=rt.HeapOK{t.Fatal(err)}
 if !c.ValuesEqual(c.TableGet(owner,key),c.InternStatic("kept by owner")){t.Fatal("cold value lost")}
 for i:=0;i<rt.MaxLocalRoots;i++{if !c.PushLocal(owner){t.Fatal("cold helper changed root balance")}}
 if err:=gluTableSetStableNumber(&c,owner,key,rt.FromInt(42),&cache);err!=rt.HeapOK{t.Fatalf("warm nonallocating write: %v",err)}
 c.PopLocals(rt.MaxLocalRoots)
 if got:=c.TableGet(owner,key);got!=rt.IntVal(42){t.Fatalf("warm numeric value: %#v",got)}
}
`, "TestColdStableWrite")
}

func TestStableWritesDoNotStoreThroughHandleZeroAfterRebind(t *testing.T) {
	source := `
decoy={payload=99, tag="keep"}
owner={payload=1, tag="o"}
function write_payload(n)
  owner.payload = n
end
function clear_owner()
  owner = nil
end
function decoy_payload()
  return decoy.payload
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestStableWriteWarmRebind(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 g.write_payload(&c, rt.IntVal(7))
 if got:=g.decoy_payload(&c);got!=rt.IntVal(99) {t.Fatal("setup mutated decoy")}
 g.clear_owner(&c)
 g.write_payload(&c, rt.IntVal(3))
 if got:=g.decoy_payload(&c);got!=rt.IntVal(99) {t.Fatalf("nil owner smashed decoy %#v",got)}
}
`
	for _, disabled := range []bool{false, true} {
		generated, err := Transpile("stable-write-rebind.lua", source, Options{DisableStableWriteSpecialization: disabled})
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, generated, harness, "TestStableWriteWarmRebind")
	}
}
