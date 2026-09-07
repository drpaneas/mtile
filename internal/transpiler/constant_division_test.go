package transpiler

import (
	"fmt"
	"strings"
	"testing"
)

func TestConstantDivisionExactSignedRangeSamples(t *testing.T) {
	source := `
result = 0
function positive(n) return n / (1.5) end
function negative(n) return n / -1.5 end
function _init() result = positive(0) + negative(0) end
`
	harness := `package main
import("testing"; rt "github.com/drpaneas/mtile/runtime")
func TestDivisionSamples(t *testing.T) {
 var g Game; var c rt.Context; g.Init(&c)
 check:=func(n int32){
  want:=rt.Number((int64(n)<<16)/98304)
  if got:=g.positive(&c,rt.Number(n));got!=want{t.Fatalf("positive raw%d got%d want%d",n,got,want)}
  if got:=g.negative(&c,rt.Number(n));got!=-want{t.Fatalf("negative raw%d got%d want%d",n,got,-want)}
 }
 // Exhaust every residue mod3 densely around zero and both signed endpoints.
 for n:=int32(-1048576);n<=1048576;n++ {check(n)}
 for i:=int32(0);i<100000;i++ {check(-2147483648+i);check(2147483647-i)}
 // Powers of two and their neighbours exercise magnitude/sign transitions.
 for bit:=0;bit<31;bit++{for d:=int32(-4);d<=4;d++{check((int32(1)<<bit)+d);check(-(int32(1)<<bit)+d)}}
 seed:=uint32(0x87654321)
 for i:=0;i<1000000;i++{seed=seed*1664525+1013904223;check(int32(seed))}
}
`
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("disabled=%v", disabled), func(t *testing.T) {
			generated, err := Transpile("constant-division.lua", source, Options{DisableConstantDivision: disabled})
			if err != nil {
				t.Fatal(err)
			}
			if hasCall := strings.Contains(generated, "return gluNumberDivThreeHalves(a0)"); hasCall == disabled {
				t.Fatalf("specialized call present=%v disabled=%v", hasCall, disabled)
			}
			runGeneratedHarness(t, generated, harness, "TestDivisionSamples")
		})
	}
}

func TestConstantDivisionPreservesEvaluationAndGenericSemantics(t *testing.T) {
	source := `
seen = 0
result = 0
local values = {3, -3, "3", false, nil, {}}
results = {}
function next_number() seen = seen + 1 return -7.25 end
function boxed(value) return value / 1.5 end
function dynamic(n, divisor) return n / divisor end
function floored(n) return n \ 1.5 end
function nearby(n) return n / 1.50002 end
function _init()
 result = next_number() / 1.5
 for i = 1,6 do add(results, {value=boxed(values[i])}) end
 result = result + dynamic(4.5,1.5) + floored(-1) + nearby(0)
end
`
	harness := `package main
import("testing"; rt "github.com/drpaneas/mtile/runtime")
func TestDivisionSemantics(t *testing.T){
 var g Game; var c rt.Context; g.Init(&c)
 if g.seen!=rt.FromInt(1){t.Fatalf("left operand evaluated %v times",g.seen)}
 want:=rt.FromFloat(-7.25).Div(rt.FromFloat(1.5))+rt.FromInt(2)
 if g.result!=want{t.Fatalf("result%d want%d",g.result,want)}
 for i:=1;i<=6;i++ {obj:=c.TableGetInt(g.results,i);v:=c.TableGet(obj,c.InternStatic("value"));
  if i<=2 {n:=rt.FromInt(2);if i==2{n=-n};if v.Kind!=rt.KindNumber||v.Num!=n{t.Fatalf("boxed numeric%d %#v",i,v)}} else if v.Kind!=rt.KindNil {t.Fatalf("boxed non-number%d %#v",i,v)}
 }
 if g.dynamic(&c,rt.FromInt(7),0)!=rt.Number(0x7fffffff){t.Fatal("dynamic divide by zero changed")}
 if g.floored(&c,rt.FromInt(-1))!=rt.FromInt(-1){t.Fatal("floor division changed")}
 if g.nearby(&c,rt.FromInt(7))!=rt.FromInt(7).Div(rt.FromFloat(1.50002)){t.Fatal("nearby divisor incorrectly specialized")}
}
`
	for _, disabled := range []bool{false, true} {
		generated, err := Transpile("constant-division-semantics.lua", source, Options{DisableConstantDivision: disabled})
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, generated, harness, "TestDivisionSemantics")
	}
}
