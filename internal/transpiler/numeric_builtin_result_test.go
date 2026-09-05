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
	for _, generic := range []bool{false, true} {
		generated, err := Transpile("numeric-builtin-result.lua", source, Options{DisableMonomorphicNumbers: generic})
		if err != nil {
			t.Fatal(err)
		}
		if !generic && !strings.Contains(generated, "narrowed(c *rt.Context, a0 rt.Value) rt.Number") {
			t.Fatalf("generic argument must preserve a native numeric result:\n%s", generated)
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
