package transpiler

import (
	"strings"
	"testing"
)

func TestStableNumericReadsEvaluateBasesOnceInOrderAcrossGC(t *testing.T) {
	source := `
trace=0
total=0
function make_record(tag)
  trace=trace*10+tag
  for i=1,1100 do local trash={"temporary-"..i} end
  return {x=5}
end
function exercise()
  trace=0
  total=make_record(1).x+make_record(2).x
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestStableReadEvaluation(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 for repeat:=0;repeat<4;repeat++ {
  g.exercise(&c)
  if g.trace!=rt.FromInt(12)||g.total!=rt.FromInt(10) {t.Fatalf("read evaluation trace%v result%v",g.trace,g.total)}
 }
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("stable-read-evaluation.lua", source, Options{DisableStableReadSpecialization: disabled})
		if err != nil {
			t.Fatal(err)
		}
		body := generatedSection(got, "func (g *Game) exercise(")
		if strings.Contains(body, "c.TableGetCachedNumber(") == disabled {
			t.Fatal("stable read option did not select its warm ABI")
		}
		runGeneratedHarness(t, got, harness, "TestStableReadEvaluation")
	}
}

func TestStableNumericReadsPreserveColdKindAndMissingChecks(t *testing.T) {
	source := `
decoy={x=99}
value=nil
function read_value() return value.x end
function assign_value() value={x=7} end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestStableReadColdChecks(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 for _,value:=range []rt.Value{rt.NilVal(),rt.IntVal(0),rt.BoolVal(false)} {
  g.value=value
  if n:=g.read_value(&c);n!=0 {t.Fatalf("non-table with live ref0 read as%v",n)}
 }
 var err rt.HeapError
 g.value,err=c.NewTableWithCaps(1,1);if err!=rt.HeapOK {t.Fatal(err)}
 if n:=g.read_value(&c);n!=0 {t.Fatalf("missing field read as%v",n)}
 g.assign_value(&c)
 for i:=0;i<4;i++ {
  if n:=g.read_value(&c);n!=rt.FromInt(7) {t.Fatalf("valid/warm field read as%v",n)}
  if err:=c.Collect();err!=rt.HeapOK {t.Fatal(err)}
 }
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("stable-read-cold.lua", source, Options{DisableStableReadSpecialization: disabled})
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, got, harness, "TestStableReadColdChecks")
	}
}

func TestStableNumericReadsDoNotFollowHandleZeroAfterRebind(t *testing.T) {
	source := `
decoy={x=99}
value=nil
function read_value() return value.x end
function assign_value() value={x=7} end
function clear_value() value=nil end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestStableReadWarmRebind(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 g.assign_value(&c)
 if n:=g.read_value(&c);n!=rt.FromInt(7) {t.Fatalf("warm table read as%v",n)}
 g.clear_value(&c)
 if n:=g.read_value(&c);n!=0 {t.Fatalf("rebound nil read decoy as%v",n)}
 g.value=rt.IntVal(0)
 if n:=g.read_value(&c);n!=0 {t.Fatalf("rebound number read decoy as%v",n)}
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("stable-read-rebind.lua", source, Options{DisableStableReadSpecialization: disabled})
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, got, harness, "TestStableReadWarmRebind")
	}
}
