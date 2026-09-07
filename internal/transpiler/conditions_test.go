package transpiler

import (
	"strings"
	"testing"
)

func TestConditionsPreserveTruthinessOrderAndValueResults(t *testing.T) {
	source := `
trace = 0
score = 0
sink = nil
function mark(tag, value)
  trace = trace * 10 + tag
  return value
end
function exercise(a, b)
  trace = 0
  score = 0
  if mark(1, a) and mark(2, b) then score = score + 1 end
  if not mark(3, a) or mark(4, b) then score = score + 2 end
  while mark(5, a) and mark(6, b) do score = score + 4 break end
  repeat
    local result = mark(7, a)
    score = score + 8
  until (not result or mark(8, b)) or true
  sink = a and b or a
end
function _init() exercise(false, false) end
`
	harness := `package main
import (
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestConditions(t *testing.T) {
 var g Game
 var c rt.Context
 g.Init(&c)
 table,err:=c.NewTable(); if err!=rt.HeapOK { t.Fatal(err) }
 values:=[]rt.Value{rt.NilVal(),rt.BoolVal(false),rt.BoolVal(true),rt.IntVal(0),rt.IntVal(-1),c.InternStatic(""),c.InternStatic("text"),table}
 for _,a:=range values { for _,b:=range values {
  g.exercise(&c,a,b)
  expected:=0
  log:=func(n int) { expected=expected*10+n }
  log(1); if a.Truthy() {log(2)}
  log(3); if a.Truthy() {log(4)}
  log(5); if a.Truthy() {log(6)}
  log(7); if a.Truthy() {log(8)}
  score:=8
  if a.Truthy()&&b.Truthy(){score+=5}
  if !a.Truthy()||b.Truthy(){score+=2}
  value:=a; if a.Truthy() {value=b}; if !value.Truthy() {value=a}
  if g.trace!=rt.FromInt(expected)||g.score!=rt.FromInt(score)||g.sink!=value {t.Fatalf("a=%#v b=%#v trace=%v score=%v sink=%#v",a,b,g.trace,g.score,g.sink)}
 } }
}
`
	for _, disabled := range []bool{false, true} {
		generated, err := Transpile("conditions.lua", source, Options{DisableConditionSpecialization: disabled})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(generated, "glu_condition_") == disabled {
			t.Fatal("condition specialization did not follow option")
		}
		runGeneratedHarness(t, generated, harness, "TestConditions")
	}
}

func TestConditionLocalsSurviveAllocatingRightOperands(t *testing.T) {
	source := `
calls = 0
observed = ""
function pressure()
  calls = calls + 1
  for i = 1, 1100 do local trash = {"temporary-" .. i} end
  return true
end
function first_false() return false, true end
function _update()
  local kept = {text = "keep-" .. calls}
  if not first_false() and pressure() and kept.text ~= "" then
    observed = kept.text
  end
  if false and pressure() then observed = "wrong" end
  if true or pressure() then
    local i = 0
    while i < 2 and kept.text ~= "" do i = i + 1 end
    repeat
      local done = i == 0
      i = i - 1
    until done or i == 0
  end
end
`
	harness := `package main
import (
 "fmt"
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestConditionGC(t *testing.T) {
 var g Game; var c rt.Context; g.Init(&c)
 for tick:=0;tick<4;tick++ {
  g.Update(&c)
  if g.calls!=rt.FromInt(tick+1) {t.Fatal("short circuit call count",g.calls)}
  if !c.ValuesEqual(g.observed,c.InternStatic(fmt.Sprintf("keep-%d",tick))) {t.Fatal("lost condition local",g.observed)}
  if e:=c.Collect();e!=rt.HeapOK {t.Fatal(e)}
 }
}
`
	for _, disabled := range []bool{false, true} {
		generated, err := Transpile("condition-gc.lua", source, Options{DisableConditionSpecialization: disabled})
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, generated, harness, "TestConditionGC")
	}
}

func TestNumericComparisonConditionsStayUnboxed(t *testing.T) {
	source := `
calls = 0
score = 0
flag = false
function bump(n)
  calls = calls + 1
  return n
end
function exercise(kind, limit)
  calls = 0
  score = 0
  if kind == 22 then score = score + 1 end
  if kind ~= 8 and kind ~= 20 then score = score + 2 end
  if kind < limit or kind >= 26 then score = score + 4 end
  if bump(1) == bump(2) then score = score + 8 end
  if 0 then score = score + 16 end
  flag = kind == 22
  if kind == nil then score = score + 32 end
end
function _init() exercise(22, 10) end
`
	harness := `package main
import (
 "testing"
 rt "github.com/drpaneas/mtile/runtime"
)
func TestNumericConditions(t *testing.T) {
 var g Game
 var c rt.Context
 g.Init(&c)
 check:=func(kind, limit, wantScore, wantCalls int, wantFlag bool) {
  g.exercise(&c, rt.FromInt(kind), rt.FromInt(limit))
  if g.score!=rt.FromInt(wantScore) || g.calls!=rt.FromInt(wantCalls) || g.flag!=rt.BoolVal(wantFlag) {
   t.Fatalf("kind=%d limit=%d score=%v calls=%v flag=%v", kind, limit, g.score, g.calls, g.flag)
  }
 }
 check(22, 10, 19, 2, true)
 check(8, 10, 20, 2, false)
 check(20, 30, 20, 2, false)
 check(26, 10, 22, 2, false)
 check(0, 1, 22, 2, false)
}
`
	for _, disabled := range []bool{false, true} {
		generated, err := Transpile("numeric-conditions.lua", source, Options{DisableConditionSpecialization: disabled})
		if err != nil {
			t.Fatal(err)
		}
		if disabled {
			if !strings.Contains(generated, "if ((rt.BoolVal(") {
				t.Fatal("disabled comparisons did not keep boxed truthiness")
			}
		} else {
			if strings.Contains(generated, "if ((rt.BoolVal(") {
				t.Fatal("numeric if still boxed a BoolVal comparison")
			}
			if !strings.Contains(generated, " == ") || !strings.Contains(generated, " != ") {
				t.Fatal("numeric if did not emit Go comparisons")
			}
			if !strings.Contains(generated, "c.ValuesEqual(") {
				t.Fatal("mixed-type equality left the ValuesEqual path")
			}
			if !strings.Contains(generated, "rt.BoolVal(") {
				t.Fatal("value-producing equality must still box a Lua boolean")
			}
		}
		runGeneratedHarness(t, generated, harness, "TestNumericConditions")
	}
}
