package transpiler

import (
	"strings"
	"testing"
)

func TestNumericLengthsPreserveAllOperandKindsAndMutableTables(t *testing.T) {
	source := `
size = 0
total = 0
function measure(value)
  size = #value
  total = 0
  for i = 1, #value do total = total + i end
end
`
	harness := `package main
import (
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestLengths(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 dynamic,err:=c.NewString([]byte("dynamic"));if err!=rt.HeapOK {t.Fatal(err)}
 table,err:=c.NewTableWithCaps(32,8);if err!=rt.HeapOK {t.Fatal(err)}
 values:=[]rt.Value{rt.NilVal(),rt.BoolVal(false),rt.BoolVal(true),rt.IntVal(7),c.InternStatic(""),c.InternStatic("fixed"),dynamic,table}
 for _,v:=range values {
  g.measure(&c,v)
  n:=0;if v.Kind==rt.KindString||v.Kind==rt.KindStaticString {n=c.StrLen(v)} else if v.Kind==rt.KindTable {n=c.TableLen(v)}
  if g.size!=rt.FromInt(n)||g.total!=rt.FromInt(n*(n+1)/2) {t.Fatalf("value=%#v size=%v total=%v",v,g.size,g.total)}
 }
 for n:=1;n<=20;n++ {
  if err:=c.TableSet(table,rt.IntVal(n),rt.IntVal(100+n));err!=rt.HeapOK {t.Fatal(err)}
  g.measure(&c,table)
  if g.size!=rt.FromInt(n)||g.total!=rt.FromInt(n*(n+1)/2) {t.Fatalf("table length %d size=%v total=%v",n,g.size,g.total)}
 }
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("numeric-lengths.lua", source, Options{DisableNumericLengths: disabled})
		if err != nil {
			t.Fatal(err)
		}
		body := generatedSection(got, "func (g *Game) measure(")
		if strings.Contains(body, "glu_numFor_") == disabled {
			t.Fatalf("ordinary length loop did not follow rollback=%v:\n%s", disabled, body)
		}
		runGeneratedHarness(t, got, harness, "TestLengths")
	}
}

func TestNumericLengthArgumentsPreserveAllocationOrderAndRoots(t *testing.T) {
	source := `
trace = 0
observed = 0
function pressure(tag)
  trace = trace * 10 + tag
  for i = 1, 1100 do local trash = {"temporary-" .. i} end
  return "abc"
end
function combine(kept, count, suffix)
  observed = #kept.text + count + #suffix
end
function _update()
  trace = 0
  combine({text = "keep-" .. trace}, #pressure(1), pressure(2))
end
`
	harness := `package main
import (
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestLengthRoots(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 for i:=0;i<3;i++ {g.Update(&c);if g.trace!=rt.FromInt(12)||g.observed!=rt.FromInt(12) {t.Fatalf("trace=%v observed=%v",g.trace,g.observed)}}
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("length-roots.lua", source, Options{DisableNumericLengths: disabled})
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, got, harness, "TestLengthRoots")
	}
}

func TestNumericLoopReassignmentRetainsLuaValue(t *testing.T) {
	source := `
items = {1,2,3}
total = 0
sink = nil
function length_loop()
  total = 0
  for i = 1, #items do
    total = total + i
    i = "changed"
    sink = i
  end
end
function constant_loop()
  total = 0
  for i = 1, 3 do
    total = total + i
    if i == 3 then i = false else i = "changed" end
    sink = i
  end
end
function shadowed_loop()
  total = 0
  for i = 1, #items do
    do local i = "shadow" i = false sink = i end
    for i = 1, 2 do i = "inner" sink = i end
    total = total + i
  end
end
`
	harness := `package main
import (
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestLoopBindings(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 g.length_loop(&c)
 if g.total!=rt.FromInt(6)||!c.ValuesEqual(g.sink,c.InternStatic("changed")) {t.Fatalf("length loop total=%v sink=%#v",g.total,g.sink)}
 g.constant_loop(&c)
 if g.total!=rt.FromInt(6)||g.sink!=rt.BoolVal(false) {t.Fatalf("constant loop total=%v sink=%#v",g.total,g.sink)}
 g.shadowed_loop(&c)
 if g.total!=rt.FromInt(6)||!c.ValuesEqual(g.sink,c.InternStatic("inner")) {t.Fatalf("shadowed loop total=%v sink=%#v",g.total,g.sink)}
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("loop-bindings.lua", source, Options{DisableNumericLengths: disabled})
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"length_loop", "constant_loop"} {
			if body := generatedSection(got, "func (g *Game) "+name+"("); strings.Contains(body, "glu_numFor_") {
				t.Fatalf("reassigned loop %s lost its Value binding:\n%s", name, body)
			}
		}
		if !disabled && !strings.Contains(generatedSection(got, "func (g *Game) shadowed_loop("), "glu_numFor_") {
			t.Fatal("nested shadowing unnecessarily disabled outer numeric loop")
		}
		runGeneratedHarness(t, got, harness, "TestLoopBindings")
	}
}
