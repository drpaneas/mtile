package transpiler

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func differentialLua(corpus []*differentialExpr) string {
	var source strings.Builder
	source.WriteString(`trace = {}
minimum = -32768
function mark(tag, value) add(trace, tag) return value end
`)
	for i, e := range corpus {
		fmt.Fprintf(&source, "function probe%d() return %s end\n", i, e.source(0))
		fmt.Fprintf(&source, "function condition%d() if %s then return true else return false end end\n", i, e.source(0))
	}
	// An independently specified statement witness also checks compound lowering:
	// the base, key, and RHS must run exactly once, in that order.
	source.WriteString(`
target = {5}
function base() add(trace, 90) return target end
function key() add(trace, 91) return 1 end
function rhs() add(trace, 92) return 7 end
function compound() base()[key()] += rhs() return target[1] end
function minimumneg() return -minimum end
function parenthesizedminimum() return -(32768) end
function numbermark(tag) add(trace, tag) return tag end
function negatedcall() return -numbermark(1) + numbermark(2) end
function skippednegatedcall() return false and -numbermark(3) end
`)
	return source.String()
}

func differentialHarness(corpus []*differentialExpr) string {
	var h strings.Builder
	h.WriteString(`package main
import (
 "fmt"
 "slices"
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func differentialBox[T rt.Value | rt.Number](v T) rt.Value {
 switch x := any(v).(type) { case rt.Value: return x; case rt.Number: return rt.NumVal(x) }
 panic("unexpected generated return type")
}
func TestDifferentialGenerated(t *testing.T) {
 var c rt.Context
 var g Game
 g.Init(&c)
 cases := []struct {name, source, want string; trace []int; call func(*rt.Context) rt.Value} {
`)
	for i, e := range corpus {
		var trace []int
		want := e.eval(&trace)
		fmt.Fprintf(&h, "{%q, %q, %q, %#v, func(c *rt.Context) rt.Value { return differentialBox(g.__glu_call_probe%d(c)) }},\n", fmt.Sprintf("value%d", i), e.source(0), want.String(), trace, i)
		fmt.Fprintf(&h, "{%q, %q, %q, %#v, func(c *rt.Context) rt.Value { return differentialBox(g.__glu_call_condition%d(c)) }},\n", fmt.Sprintf("condition%d", i), e.source(0), differentialBool(want.truth()).String(), trace, i)
	}
	h.WriteString(`{"compound", "base()[key()] += rhs()", "number:786432", []int{90,91,92}, func(c *rt.Context) rt.Value { return differentialBox(g.__glu_call_compound(c)) }},
 {"minimumglobal", "minimum=-32768; return -minimum", "number:-2147483648", nil, func(c *rt.Context) rt.Value { return differentialBox(g.__glu_call_minimumneg(c)) }},
 {"minimumparen", "return -(32768)", "number:-2147483648", nil, func(c *rt.Context) rt.Value { return differentialBox(g.__glu_call_parenthesizedminimum(c)) }},
 {"negatedcall", "return -numbermark(1) + numbermark(2)", "number:65536", []int{1,2}, func(c *rt.Context) rt.Value { return differentialBox(g.__glu_call_negatedcall(c)) }},
 {"skippednegatedcall", "return false and -numbermark(3)", "bool:0", nil, func(c *rt.Context) rt.Value { return differentialBox(g.__glu_call_skippednegatedcall(c)) }},
 }
 for _, tc := range cases {
  t.Run(tc.name, func(t *testing.T) {
   var err rt.HeapError
   g.trace, err = c.NewTable()
   if err != rt.HeapOK { t.Fatalf("trace allocation: %v", err) }
   value := tc.call(&c)
   kind := "unexpected"
   switch value.Kind {case rt.KindNil: kind="nil"; case rt.KindBool: kind="bool"; case rt.KindNumber: kind="number"}
   got := fmt.Sprintf("%s:%d", kind, int32(value.Num))
   var trace []int
   for i:=1; i<=c.TableLen(g.trace); i++ { trace=append(trace, c.TableGetInt(g.trace,i).Int()) }
   if got != tc.want || !slices.Equal(trace, tc.trace) {
    t.Fatalf("semantic mismatch: %s: got %s trace %v, want %s trace %v", tc.source, got, trace, tc.want, tc.trace)
   }
  })
 }
}
`)
	return h.String()
}

func TestDifferentialExecution(t *testing.T) {
	corpus := differentialCorpus()
	source, harness := differentialLua(corpus), differentialHarness(corpus)
	for _, config := range []struct {
		name string
		opts Options
	}{
		{"optimized", Options{}},
		{"generic", Options{DisableMonomorphicNumbers: true, DisableMonomorphicFunctions: true, DisableNumericConstants: true, DisableConditionSpecialization: true}},
	} {
		t.Run(config.name, func(t *testing.T) {
			start := time.Now()
			got, err := Transpile("differential.lua", source, config.opts)
			if err != nil {
				t.Fatal(err)
			}
			transpileTime := time.Since(start)
			start = time.Now()
			runGeneratedHarness(t, got, harness, "TestDifferentialGenerated")
			t.Logf("%d value/condition expressions + 5 statement/literal witnesses: transpile=%s; generated format/build/execute=%s", len(corpus), transpileTime, time.Since(start))
		})
	}
}

var differentialTranspileSink string

// The fixed corpus is constructed before timing. This measures host compiler
// work only, separately from the oracle and generated-package build/execution.
func BenchmarkDifferentialCorpusTranspile(b *testing.B) {
	source := differentialLua(differentialCorpus())
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for range b.N {
		got, err := Transpile("differential.lua", source, Options{})
		if err != nil {
			b.Fatal(err)
		}
		differentialTranspileSink = got
	}
}
