package transpiler

import (
	"strings"
	"testing"
)

func TestTrustedTableLiteralWritesRequireCompleteNonNilProof(t *testing.T) {
	source := `
function known(value) return {x=rnd(8),enabled=false,label="",equal=value==nil,length=#value} end
function unknown(value) return {x=value,y=1} end
function conditional(value) return {x=value and 1,y=2} end
function duplicate() return {x=1,x=2,y=3} end
function nested() return {x={},y=2} end
function positional() return {1,x=2,y=3} end
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("trusted-literal-proof.lua", source, Options{DisableTrustedTableLiteralWrites: disabled})
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"known", "unknown", "conditional", "duplicate", "nested", "positional"} {
			body := generatedSection(got, "func (g *Game) "+name+"(")
			hasTrusted := strings.Contains(body, ".InitKnown(") || strings.Contains(body, "NewTableFromImage(")
			want := !disabled && (name == "known" || name == "nested")
			if hasTrusted != want {
				t.Fatalf("%s did not follow complete non-nil proof:\n%s", name, body)
			}
		}
		compileGenerated(t, got)
	}
}

func TestTrustedTableLiteralWritesPreserveRNGAndPrimitiveValues(t *testing.T) {
	source := `
result=nil
trace=0
function value(n) trace=trace*10+n return n end
function construct(arg)
  trace=0
  result={x=rnd(20),y=rnd(20),one=value(1),two=value(2),enabled=false,
    title="label",empty="",equal=arg==nil,inverted=not arg,length=#arg}
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestTrustedLiteralValues(t *testing.T) {
 var g Game;var c,random rt.Context;g.Init(&c)
 for _,arg:=range []rt.Value{rt.NilVal(),rt.BoolVal(false),rt.BoolVal(true),c.InternStatic("abc")} {
  c.Srand(rt.FromInt(17));random.Srand(rt.FromInt(17));g.construct(&c,arg)
  get:=func(k string)rt.Value{return c.TableGet(g.result,c.InternStatic(k))}
  if get("x")!=rt.NumVal(random.Rnd(rt.FromInt(20)))||get("y")!=rt.NumVal(random.Rnd(rt.FromInt(20)))||g.trace!=rt.FromInt(12) {t.Fatal("RNG/evaluation order changed")}
  if get("one")!=rt.IntVal(1)||get("two")!=rt.IntVal(2)||get("enabled")!=rt.BoolVal(false) {t.Fatal("numeric/boolean values changed")}
  if !c.ValuesEqual(get("title"),c.InternStatic("label"))||!c.ValuesEqual(get("empty"),c.InternStatic("")) {t.Fatal("static string values changed")}
  if get("equal")!=rt.BoolVal(arg.Kind==rt.KindNil)||get("inverted")!=rt.BoolVal(!arg.Truthy()) {t.Fatal("computed boolean values changed")}
  length:=0;if arg.Kind==rt.KindStaticString {length=3};if get("length")!=rt.IntVal(length) {t.Fatal("length value changed")}
  if err:=c.Collect();err!=rt.HeapOK {t.Fatal(err)}
 }
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("trusted-literal-values.lua", source, Options{DisableTrustedTableLiteralWrites: disabled})
		if err != nil {
			t.Fatal(err)
		}
		if !disabled && !strings.Contains(generatedSection(got, "func (g *Game) construct("), "NewTableFromImage(") &&
			!strings.Contains(generatedSection(got, "func (g *Game) construct("), ".InitKnown(") {
			t.Fatal("value harness did not exercise trusted writes")
		}
		runGeneratedHarness(t, got, harness, "TestTrustedLiteralValues")
	}
}
