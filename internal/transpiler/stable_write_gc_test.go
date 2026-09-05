package transpiler

import "testing"

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
