package transpiler

import (
	"strings"
	"testing"
)

func TestKnownTableAppendsPreserveRNGRootingAndSkippedAttempts(t *testing.T) {
	source := `
result=nil
function fill()
  local list={}
  for i=9,-23,-2 do add(list,{n=i,r=rnd(13)}) end
  result=list
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
var attempts int
func auditAppend(c *rt.Context, table,value rt.Value,at int) rt.HeapError {
 attempts++
 if err:=c.Collect();err!=rt.HeapOK {return err}
 if attempts==3 {return rt.HeapOOM}
 APPEND
}
func TestKnownAppendGCAndFailure(t *testing.T) {
 var g Game;var c,random rt.Context;g.Init(&c)
 for pass:=0;pass<3;pass++ {
  attempts=0;c.Srand(rt.FromInt(pass+1));random.Srand(rt.FromInt(pass+1));g.fill(&c)
  if attempts!=17||c.TableLen(g.result)!=16||c.TableSlotCount(g.result)!=33 {t.Fatal("attempt count or dense capacity changed")}
  index:=1
  for attempt:=1;attempt<=17;attempt++ {
   wantRandom:=rt.NumVal(random.Rnd(rt.FromInt(13)))
   if attempt==3 {continue}
   value:=c.TableGetInt(g.result,index)
   if c.TableGet(value,c.InternStatic("n"))!=rt.IntVal(11-2*attempt) || c.TableGet(value,c.InternStatic("r"))!=wantRandom {t.Fatal("RNG/evaluation or append order changed")}
   index++
  }
  for slot:=0;slot<rt.MaxLocalRoots;slot++ {if !c.PushLocal(g.result) {t.Fatalf("leaked roots before slot %d",slot)}}
  c.PopLocals(rt.MaxLocalRoots)
 }
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("known-append.lua", source, Options{DisableKnownTableAppends: disabled})
		if err != nil {
			t.Fatal(err)
		}
		body := generatedSection(got, "func (g *Game) fill(")
		if strings.Contains(body, "gluTableAppendKnown(c,") == disabled {
			t.Fatalf("known append selection does not match rollback:\n%s", body)
		}
		old, replacement := "err := c.TableAppendKnown(t,v,at)", "err := auditAppend(c,t,v,at)"
		appendCall := "return c.TableAppendKnown(table,value,at)"
		if disabled {
			old, replacement = "err := c.TableAdd(t,v,i)", "err := auditAppend(c,t,v,0)"
			appendCall = "return c.TableAdd(table,value,rt.NilVal())"
		}
		if !strings.Contains(got, old) {
			t.Fatal("missing rooted append helper")
		}
		got = strings.Replace(got, old, replacement, 1)
		runGeneratedHarness(t, got, strings.Replace(harness, "APPEND", appendCall, 1), "TestKnownAppendGCAndFailure")
	}
}

func TestKnownTableAppendsOnlyUseReservedExactLoops(t *testing.T) {
	for _, options := range []Options{{}, {DisableKnownTableAppends: true}, {DisableLoopTableReservations: true}} {
		got, err := Transpile("known-append-shapes.lua", `
function fill()
  local positive={} for i=1,17 do add(positive,true) end
  local negative={} for i=9,-23,-2 do add(negative,"word") end
  local alias={} local other=alias for i=1,17 do add(alias,1) end
  local index={} for i=1,17 do add(index,1,1) end
  local nullable={} for i=1,17 do add(nullable,nil) end
  local observed={} for i=1,17 do add(observed,{old=#observed}) end
end`, options)
		if err != nil {
			t.Fatal(err)
		}
		want := 2
		if options.DisableKnownTableAppends || options.DisableLoopTableReservations {
			want = 0
		}
		if count := strings.Count(generatedSection(got, "func (g *Game) fill("), "gluTableAppendKnown(c,"); count != want {
			t.Fatalf("got %d known appends, want %d", count, want)
		}
	}
}

func TestKnownAppendOfGlobalListSkipsConstructorListRoots(t *testing.T) {
	source := `
values=nil
function fill()
  values={}
  for i=1,4 do add(values,{x=rnd(7),y=i}) end
end
`
	got, err := Transpile("known-append-global.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	body := generatedSection(got, "func (g *Game) fill(")
	if !strings.Contains(body, "gluTableAppendKnownHeld(c,") {
		t.Fatal("expected reserved append of the already-rooted global list")
	}
	if strings.Contains(body, "gluTableAppendKnown(c,") {
		t.Fatal("global list still used the local-list append helper")
	}
	if strings.Contains(body, "PushLocal(") {
		t.Fatalf("global list constructor still pushed GC roots:\n%s", body)
	}
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestGlobalAppend(t *testing.T) {
 var g Game;var c,random rt.Context;g.Init(&c)
 c.Srand(rt.FromInt(3));random.Srand(rt.FromInt(3));g.fill(&c)
 if c.TableLen(g.values)!=4 {t.Fatal(c.TableLen(g.values))}
 for i:=1;i<=4;i++ {
  record:=c.TableGetInt(g.values,i)
  if c.TableGet(record,c.InternStatic("x"))!=rt.NumVal(random.Rnd(rt.FromInt(7))) {t.Fatal("RNG changed")}
  if c.TableGet(record,c.InternStatic("y"))!=rt.IntVal(i) {t.Fatal("index changed")}
 }
 if err:=c.Collect();err!=rt.HeapOK {t.Fatal(err)}
}
`
	runGeneratedHarness(t, got, harness, "TestGlobalAppend")
}

func TestKnownAppendOfGlobalListSurvivesInjectedCollection(t *testing.T) {
	source := `
values=nil
function fill()
  values={}
  for i=1,4 do add(values,{x=rnd(7),y=i}) end
end
`
	got, err := Transpile("known-append-global-gc.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	old := "n := gluPushRoots(c,v); if n < 0 { return rt.HeapOOM }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,n); return rt.HeapOOM }; err := c.TableAppendKnown(t,v,at)"
	replacement := "n := gluPushRoots(c,v); if n < 0 { return rt.HeapOOM }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,n); return rt.HeapOOM }; if err:=c.Collect(); err!=rt.HeapOK { return err }; err := c.TableAppendKnown(t,v,at)"
	if !strings.Contains(got, old) {
		t.Fatal("missing held-list append helper")
	}
	got = strings.Replace(got, old, replacement, 1)
	runGeneratedHarness(t, got, `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestGlobalAppendGC(t *testing.T) {
 var g Game;var c,random rt.Context;g.Init(&c)
 c.Srand(rt.FromInt(3));random.Srand(rt.FromInt(3));g.fill(&c)
 if c.TableLen(g.values)!=4 {t.Fatal(c.TableLen(g.values))}
 for i:=1;i<=4;i++ {
  record:=c.TableGetInt(g.values,i)
  if c.TableGet(record,c.InternStatic("x"))!=rt.NumVal(random.Rnd(rt.FromInt(7))) {t.Fatal("collected new record")}
  if c.TableGet(record,c.InternStatic("y"))!=rt.IntVal(i) {t.Fatal("index changed")}
 }
}
`, "TestGlobalAppendGC")
}
