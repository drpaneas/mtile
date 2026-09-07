package transpiler

import (
	"strings"
	"testing"
)

func TestNumberArrayViewsReuseNestedLoopsAndPreserveBounds(t *testing.T) {
	source := `
local values={1,2,3,4,5,6}
local output={0,0,0,0,0,0}
total=0
function exercise()
  local i=1
  for y=0,1 do
    for x=0,2 do
      output[i]=values[i]*2
      values[i]=values[i]+1
      i=i+1
    end
  end
  total=0
  for i=-1,8 do
    total=total+output[i]
    if i<1 or i>6 then output[i]=99 end
  end
  for i=1,1 do values[2],values[3]=values[3],values[2] end
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestViewLoops(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c);g.exercise(&c)
 if g.total!=rt.FromInt(42) {t.Fatalf("sum=%v",g.total)}
 want:=[]int{2,4,3,5,6,7}
 for i,w:=range want {if got:=c.NumberArrayGet(g.values,i+1);got!=rt.FromInt(w) {t.Fatalf("value%d=%v want%d",i,got,w)}}
 for i:=1;i<=6;i++ {if got:=c.NumberArrayGet(g.output,i);got!=rt.FromInt(i*2) {t.Fatalf("output%d=%v",i,got)}}
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("array-views.lua", source, Options{DisableNumberArrayViews: disabled})
		if err != nil {
			t.Fatal(err)
		}
		body := generatedSection(got, "func (g *Game) exercise(")
		want := 4 // Two arrays in the outer loop, one in each following loop.
		if disabled {
			want = 0
		}
		if count := strings.Count(body, "c.BorrowNumberArray("); count != want {
			t.Fatalf("borrow count=%d want%d:\n%s", count, want, body)
		}
		runGeneratedHarness(t, got, harness, "TestViewLoops")
	}
}

func TestNumberArrayViewsBorrowAfterAllocatingBounds(t *testing.T) {
	source := `
local values={1,2,3,4}
trace=0
function bound(tag,value)
  trace=trace*10+tag
  for i=1,1100 do local trash={"temporary-"..i} end
  return value
end
function exercise()
  trace=0
  for i=bound(1,1),bound(2,4),bound(3,1) do
    values[i]=values[i]+2
  end
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestViewBoundOrder(t *testing.T) {
 var g Game;var c rt.Context;g.Init(&c)
 for repeat:=1;repeat<=3;repeat++ {
  g.exercise(&c)
  if g.trace!=rt.FromInt(123) {t.Fatalf("bound evaluation order=%v",g.trace)}
  for i:=1;i<=4;i++ {if v:=c.NumberArrayGet(g.values,i);v!=rt.FromInt(i+repeat*2) {t.Fatalf("moved array slot%d=%v",i,v)}}
 }
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("array-view-bounds.lua", source, Options{DisableNumberArrayViews: disabled})
		if err != nil {
			t.Fatal(err)
		}
		body := generatedSection(got, "func (g *Game) exercise(")
		if !disabled {
			if borrow, lastCall := strings.Index(body, "c.BorrowNumberArray("), strings.LastIndex(body, "g.__glu_call_bound("); borrow < 0 || borrow < lastCall {
				t.Fatalf("array borrowed before last allocating bound:\n%s", body)
			}
		}
		runGeneratedHarness(t, got, harness, "TestViewBoundOrder")
	}
}

func TestNumberArrayViewsRejectUnsafeOrUnknownBodies(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"table-literal", `local trash={} values[i]=i`},
		{"concat", `sink="x"..i values[i]=i`},
		{"unknown-call", `values[i]=identity(i)`},
		{"generic-table-store", `other[i]=i values[i]=i`},
		{"table-field-store", `other.value=i values[i]=i`},
		{"allocating-builtin", `sink=tostr(i) values[i]=i`},
		{"nested-allocating-bound", `for j=1,identity(i) do values[j]=i end`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := `local values={1,2,3}
other={}
sink=nil
function identity(x) return x end
function exercise() for i=1,3 do ` + tc.body + ` end end`
			got, err := Transpile("unsafe-view.lua", source, Options{})
			if err != nil {
				t.Fatal(err)
			}
			body := generatedSection(got, "func (g *Game) exercise(")
			// A nested loop can borrow after its own bound call; the outer
			// region must never retain a view across that unknown call.
			if tc.name == "nested-allocating-bound" {
				if borrow, call := strings.Index(body, "c.BorrowNumberArray("), strings.Index(body, "g.identity("); borrow >= 0 && borrow < call {
					t.Fatalf("outer loop borrowed across nested unknown call:\n%s", body)
				}
			} else if strings.Contains(body, "c.BorrowNumberArray(") {
				t.Fatalf("unsafe body retained a borrowed view:\n%s", body)
			}
			compileGenerated(t, got)
		})
	}
}
