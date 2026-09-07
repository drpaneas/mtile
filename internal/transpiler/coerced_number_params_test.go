package transpiler

import (
	"strings"
	"testing"
)

func TestCoercedNumberParamsPreserveMixedValuesAndExpressionLists(t *testing.T) {
	source := `
trace=0
raw=nil
function leaf(x,y) return max(-20,x),min(30,y) end
function relay(x,y) return leaf(x,y) end
function produce() trace=trace*10+1 return {},7 end
function scalar() trace=trace*10+2 return false end
function empty() trace=trace*10+3 end
function exercise(value)
  raw=value
  a,b=relay(value,35)
  c,d=relay(produce())
  e,f=relay(scalar(),empty())
  h,j=relay((produce()))
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func number(value interface{}) rt.Number {switch n:=value.(type) {case rt.Number:return n;case rt.Value:return n.NumberOr(0)};panic("unexpected numeric storage")}
func TestDemandedNumberValues(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 table,err:=c.NewTableWithCaps(1,1);if err!=rt.HeapOK {t.Fatal(err)}
 text:=c.InternStatic("12.5")
 for _,input:=range []rt.Value{rt.NilVal(),rt.BoolVal(false),rt.BoolVal(true),table,text,rt.IntVal(-25),rt.NumVal(rt.Number(811008))} {
  g.trace=0;g.exercise(&c,input)
  if g.raw!=input||number(g.a)!=rt.Max(rt.FromInt(-20),input.NumberOr(0))||number(g.b)!=rt.FromInt(30) {t.Fatal("mixed coercion changed")}
  if number(g.c)!=0||number(g.d)!=rt.FromInt(7)||number(g.e)!=0||number(g.f)!=0||number(g.h)!=0||number(g.j)!=0 {t.Fatal("expression-list adjustment changed")}
  if g.trace!=rt.FromInt(1231) {t.Fatalf("evaluation order changed: %v",g.trace)}
 }
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("coerced-params.lua", source, Options{DisableCoercedNumberParams: disabled})
		if err != nil {
			t.Fatal(err)
		}
		kind := "rt.Number"
		if disabled {
			kind = "rt.Value"
		}
		for _, name := range []string{"leaf", "relay"} {
			want := "func (g *Game) " + name + "(c *rt.Context, a0 " + kind + ", a1 " + kind + ")"
			if !strings.Contains(got, want) {
				t.Fatalf("missing demanded signature %q", want)
			}
		}
		runGeneratedHarness(t, got, harness, "TestDemandedNumberValues")
	}
}

func TestCoercedNumberParamsRejectObservableAndAllocatingUses(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"raw-return", "return x"},
		{"truthiness", "if x then return max(0,x) end return 0"},
		{"raw-arithmetic", "return max(0,x+1)"},
		{"comparison", "if x==0 then return 1 end return max(0,x)"},
		{"alias", "local y=x return max(0,y)"},
		{"escape", "saved=x return max(0,x)"},
		{"write", "x='changed' return max(0,x)"},
		{"nested-write", "if true then x=1 end return max(0,x)"},
		{"shadow", "do local x=1 saved=x end return max(0,x)"},
		{"loop-shadow", "for x=1,2 do saved=x end return max(0,x)"},
		{"allocating", "local t={} return max(0,x)"},
		{"unknown-coercion", "return abs(x)"},
		{"unused", "return 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "saved=nil\nfunction candidate(x) " + tc.body + " end\nfunction exercise() candidate(nil) candidate('word') end"
			got, err := Transpile("unproven-coercion.lua", source, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, "func (g *Game) candidate(c *rt.Context, a0 rt.Value)") {
				t.Fatalf("narrowed observable parameter:\n%s", generatedSection(got, "func (g *Game) candidate("))
			}
		})
	}
}

func TestCoercedNumberParamsRequireNoAllocationAndInferenceOptions(t *testing.T) {
	for _, opts := range []Options{{DisableNoAllocFunctionCalls: true}, {DisableMonomorphicFunctions: true}, {DisableMonomorphicNumbers: true}} {
		got, err := Transpile("coercion-options.lua", "function number(x) return mid(-1,x,1) end function exercise() return number(nil) end", opts)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "func (g *Game) number(c *rt.Context, a0 rt.Value)") {
			t.Fatal("coercion bypassed inference/no-allocation rollback")
		}
	}
}
