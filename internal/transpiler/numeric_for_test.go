package transpiler

import (
	"strings"
	"testing"
)

func TestIntegerNumericForEmitsNativeGoLoop(t *testing.T) {
	source := `
total = 0
unused = 0
function sum()
  total = 0
  for i = 1, 4 do total = total + i end
end
function clouds()
  unused = 0
  for i = 0, 16 do unused = unused + 1 end
end
function stride()
  total = 0
  for i = 9, -23, -2 do total = total + i end
end
function dynamic(n)
  total = 0
  for i = 1, n do total = total + i end
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestIntegerFor(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 g.sum(&c)
 if g.total!=rt.FromInt(10) {t.Fatalf("sum=%v",g.total)}
 g.clouds(&c)
 if g.unused!=rt.FromInt(17) {t.Fatalf("unused=%v",g.unused)}
 g.stride(&c)
 if g.total!=rt.FromInt(-119) {t.Fatalf("stride=%v",g.total)}
 g.dynamic(&c,rt.NumVal(rt.FromInt(4)))
 if g.total!=rt.FromInt(10) {t.Fatalf("dynamic=%v",g.total)}
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("integer-for.lua", source, Options{DisableIntegerNumericFor: disabled})
		if err != nil {
			t.Fatal(err)
		}
		sum := generatedSection(got, "func (g *Game) sum(")
		clouds := generatedSection(got, "func (g *Game) clouds(")
		stride := generatedSection(got, "func (g *Game) stride(")
		dynamic := generatedSection(got, "func (g *Game) dynamic(")
		if strings.Contains(sum, "glu_intFor_") == disabled {
			t.Fatalf("sum integer for disabled=%v:\n%s", disabled, sum)
		}
		if !disabled && !strings.Contains(sum, "rt.FromInt(") {
			t.Fatalf("used loop variable was not converted:\n%s", sum)
		}
		if strings.Contains(clouds, "glu_intFor_") == disabled {
			t.Fatalf("clouds integer for disabled=%v:\n%s", disabled, clouds)
		}
		if !disabled && strings.Contains(clouds, "rt.FromInt(glu_intFor_") {
			t.Fatalf("unused loop variable still boxed:\n%s", clouds)
		}
		if strings.Contains(stride, "glu_intFor_") == disabled {
			t.Fatalf("stride integer for disabled=%v:\n%s", disabled, stride)
		}
		if strings.Contains(dynamic, "glu_intFor_") {
			t.Fatalf("dynamic limit used a compile-time integer for:\n%s", dynamic)
		}
		if disabled && !strings.Contains(sum, "glu_numFor_") {
			t.Fatalf("rollback sum loop lost numeric induction:\n%s", sum)
		}
		runGeneratedHarness(t, got, harness, "TestIntegerFor")
	}
}

func TestIntegerNumericForHeader(t *testing.T) {
	cases := []struct {
		init, limit, step int64
		want              string
	}{
		{0, 16, 1, "for n := 0; n <= 16; n++ {"},
		{1, 4, 1, "for n := 1; n <= 4; n++ {"},
		{9, -23, -2, "for n := 9; n >= -23; n -= 2 {"},
		{0, 10, 2, "for n := 0; n <= 10; n += 2 {"},
		{5, 1, -1, "for n := 5; n >= 1; n-- {"},
	}
	for _, tc := range cases {
		if got := integerNumericForHeader("n", tc.init, tc.limit, tc.step); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
}

func TestIntegerNumberLiteralsUseFixedPointConstants(t *testing.T) {
	got, err := Transpile("integer-literal.lua", `
n = 0
function set()
  n = 8
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	body := generatedSection(got, "func (g *Game) set(")
	if !strings.Contains(body, "rt.Number(524288)") {
		t.Fatalf("integer 8 was not a 16.16 constant:\n%s", body)
	}
	if strings.Contains(body, "rt.FromInt(8)") {
		t.Fatalf("integer 8 still called FromInt:\n%s", body)
	}
}
