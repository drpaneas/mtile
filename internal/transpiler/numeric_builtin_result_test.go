package transpiler

import (
	"strings"
	"testing"
)

func TestNumericBuiltinResultsWithGenericArguments(t *testing.T) {
	source := `
local values = {4, "text", false, true, nil, {}, -3.5}
local results = {}
function narrowed(value)
  return min(value, 2) + max(value, -2) + mid(-1, value, 1)
end
function _init()
  for i = 1, 7 do
    add(results, narrowed(values[i]))
  end
end
`
	harness := `package main
import (
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestGenericNumericArguments(t *testing.T) {
 var g Game
 var c rt.Context
 g.Init(&c)
 want:=[]float64{7,0,0,0,0,0,-6.5}
 for i,w:=range want {
  v:=c.TableGetInt(g.results,i+1)
  if v.Kind!=rt.KindNumber||v.Num!=rt.FromFloat(w) {t.Fatalf("argument %d got %#v want %g",i,v,w)}
 }
}
`
	for _, opts := range []Options{{}, {DisableCoercedNumberParams: true}, {DisableMonomorphicNumbers: true}} {
		generated, err := Transpile("numeric-builtin-result.lua", source, opts)
		if err != nil {
			t.Fatal(err)
		}
		if !opts.DisableMonomorphicNumbers {
			parameter := "rt.Number"
			if opts.DisableCoercedNumberParams {
				parameter = "rt.Value"
			}
			if !strings.Contains(generated, "narrowed(c *rt.Context, a0 "+parameter+") rt.Number") {
				t.Fatalf("generic argument must preserve a native numeric result:\n%s", generated)
			}
		}
		runGeneratedHarness(t, generated, harness, "TestGenericNumericArguments")
	}
}

func TestNumericBuiltinNarrowingPreservesArgumentEvaluation(t *testing.T) {
	source := `
local seen = 0
local result = ""
function next_value()
  seen = seen + 1
  if seen == 1 then return "string" end
  return 9
end
function _init()
  result = min(next_value(), next_value()) + max(next_value(), false) + mid("x", next_value(), -1)
end
`
	harness := `package main
import (
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestEvaluationOrder(t *testing.T){
 var g Game
 var c rt.Context
 g.Init(&c)
 if g.result!=rt.IntVal(9){t.Fatalf("result=%#v",g.result)}
 if g.seen!=rt.FromInt(4){t.Fatalf("seen=%#v",g.seen)}
}
`
	for _, generic := range []bool{false, true} {
		generated, err := Transpile("numeric-builtin-evaluation.lua", source, Options{DisableMonomorphicNumbers: generic})
		if err != nil {
			t.Fatal(err)
		}
		adjusted := harness
		if generic {
			adjusted = strings.ReplaceAll(adjusted, "g.seen!=rt.FromInt(4)", "g.seen!=rt.IntVal(4)")
		}
		runGeneratedHarness(t, generated, adjusted, "TestEvaluationOrder")
	}
}

func TestNumericBuiltinNumberArgsAvoidValueTemps(t *testing.T) {
	source := `
result = 0
function roll()
  result = rnd(8) + min(1, 4)
end
`
	got, err := Transpile("numeric-builtin-temps.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	body := generatedSection(got, "func (g *Game) roll(")
	if strings.Contains(body, "glu_arg_") {
		t.Fatalf("numeric builtin arguments still used Value sequencing temps:\n%s", body)
	}
	if !strings.Contains(body, "c.Rnd(") || !strings.Contains(body, "rt.Min(") {
		t.Fatalf("numeric builtins were not lowered:\n%s", body)
	}
	runGeneratedHarness(t, got, `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestRoll(t *testing.T) {
 var g Game;var c,random rt.Context;g.Init(&c)
 c.Srand(rt.FromInt(2));random.Srand(rt.FromInt(2));g.roll(&c)
 if g.result!=random.Rnd(rt.FromInt(8))+rt.Min(rt.FromInt(1),rt.FromInt(4)) {t.Fatal(g.result)}
}
`, "TestRoll")
}
