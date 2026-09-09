package transpiler

import (
	"fmt"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
)

func TestArrayIterationHoistingPreservesLiveMutationSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, setup, functions, iterator, body string
		hoisted                                bool
		visits, sum, calls                     int
	}{
		{"field_updates", "data={{x=1},{x=2},{x=3}}", "function bump(p) local x=p.x p.x=x+1 return x end", "data", "sum=sum+bump(p)", true, 3, 6, 0},
		{"indexed_reads", "data={1,2,3} other={10,20,30}", "function read(i) return other[i] end", "data", "sum=sum+read(p)", true, 3, 60, 0},
		{"rebind_and_allocate", "data={1,2,3}", "", "data", "sum=sum+p data={} for i=1,1024 do local discarded={} end", true, 3, 6, 0},
		{"iterator_evaluated_once", "", "function items() calls=calls+1 return {1,2,3} end", "items()", "sum=sum+p", true, 3, 6, 1},
		{"append", "data={1,2}", "", "data", "sum=sum+p if p==1 then add(data,3) end", false, 3, 6, 0},
		{"delete", "data={1,2,3}", "", "data", "sum=sum+p if p==1 then del(data,1) end", false, 2, 4, 0},
		{"delete_index", "data={1,2,3}", "", "data", "sum=sum+p if p==1 then deli(data,1) end", false, 2, 4, 0},
		{"earlier_hole_through_alias", "data={1,2,3} other=data", "", "data", "sum=sum+p if p==1 then other[1]=nil end", false, 1, 1, 0},
		{"callee_index_write", "data={1,2,3}", "function clear(t) t[1]=nil end", "data", "sum=sum+p clear(data)", false, 1, 1, 0},
		{"transitive_append", "data={1,2}", "function append(t) add(t,3) end function relay(t) append(t) end", "data", "sum=sum+p if p==1 then relay(data) end", false, 3, 6, 0},
		{"mutating_expression", "data={1,2,3}", "function clear(t) t[1]=nil return true end", "data", "sum=sum+p if clear(data) then calls=calls+1 end", false, 1, 1, 1},
		{"unrelated_array_conservative", "data={1,2,3} other={}", "", "data", "sum=sum+p add(other,p)", false, 3, 6, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := fmt.Sprintf(`data={} other={} visits=0 sum=0 calls=0
%s
function _init() %s end
function _update()
 calls=calls+0
 for p in all(%s) do
  visits=visits+1
  %s
 end
end`, tc.functions, tc.setup, tc.iterator, tc.body)
			generated, err := Transpile(tc.name+".lua", source, Options{})
			if err != nil {
				t.Fatal(err)
			}
			update := generatedSection(generated, "func (g *Game) gluUpdate(")
			if strings.Contains(update, "iterLen") != tc.hoisted || strings.Contains(update, "<= c.TableLen(") == tc.hoisted {
				t.Fatalf("expected hoisting=%v:\n%s", tc.hoisted, update)
			}
			harness := fmt.Sprintf(`package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestIteration(t *testing.T) {
 var g Game
 var c rt.Context
 g.Init(&c)
 g.Update(&c)
 if g.visits!=rt.FromInt(%d)||g.sum!=rt.FromInt(%d)||g.calls!=rt.FromInt(%d) {
  t.Fatalf("visits=%%v sum=%%v calls=%%v",g.visits,g.sum,g.calls)
 }
}`, tc.visits, tc.sum, tc.calls)
			runGeneratedHarness(t, generated, harness, "TestIteration")
		})
	}
}

func TestArrayIterationHoistingRollbackAndPairs(t *testing.T) {
	source := `data={1,2,3}
function _update() for p in all(data) do print(p) end end
function _draw() for k,v in pairs(data) do print(k,v) end end`
	for _, disabled := range []bool{false, true} {
		generated, err := Transpile("iteration-rollback.lua", source, Options{DisableIteratorLengthHoisting: disabled})
		if err != nil {
			t.Fatal(err)
		}
		update := generatedSection(generated, "func (g *Game) gluUpdate(")
		if strings.Contains(update, "<= c.TableLen(") != disabled {
			t.Fatalf("rollback=%v:\n%s", disabled, update)
		}
		draw := generatedSection(generated, "func (g *Game) gluDraw(")
		if !strings.Contains(draw, "< c.TableSlotCount(") || strings.Contains(draw, "iterLen") {
			t.Fatalf("pairs iteration changed:\n%s", draw)
		}
	}
}

func TestArrayIterationProofRejectsRecursiveAndUnknownCalls(t *testing.T) {
	for _, callee := range []string{"recur()", "unknown()", "foreach(data,recur)"} {
		chunk, err := luaparser.ParseString("iteration-effects.lua", "function recur() "+callee+" end")
		if err != nil {
			t.Fatal(err)
		}
		decl := chunk.Body[0].(*luaparser.FunctionDeclStmt)
		fn := &functionInfo{decl: decl}
		g := generator{funcs: map[string]*functionInfo{"recur": fn}}
		if g.iteratorLengthStable(decl.Body) {
			t.Errorf("unproven %s call admitted", callee)
		}
	}
}
