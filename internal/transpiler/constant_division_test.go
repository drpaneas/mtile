package transpiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/sourcebundle"
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
			if disabled {
				body := generatedSection(generated, "func (g *Game) positive(")
				if !strings.Contains(body, "(int64(") || !strings.Contains(body, "<< 16) / 98304)") {
					t.Fatal("disabled /1.5 did not retain the widening division baseline")
				}
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

func TestConstantPow2DivisionAndModulo(t *testing.T) {
	source := `
result = 0
function div8(n) return n / 8 end
function div32(n) return n / 32 end
function divneg8(n) return n / -8 end
function mod8(n) return n % 8 end
function mod4(n) return n % 4 end
function mod2(n) return n % 2 end
function div7(n) return n / 7 end
function mod3(n) return n % 3 end
function nearby(n) return n / 8.00002 end
function divx(n, d) return n / d end
function modx(n, d) return n % d end
function _init()
 result = div8(0)+div32(0)+divneg8(0)+mod8(0)+mod4(0)+mod2(0)+div7(0)+mod3(0)+nearby(0)+divx(0,1)+modx(0,1)
end
`
	generated, err := Transpile("constant-pow2.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, helper := range []string{
		"return gluNumberDivPow2(a0, 3)",
		"return gluNumberDivPow2(a0, 5)",
		"return -(gluNumberDivPow2(a0, 3))",
		"return gluNumberModPow2(a0, 19)",
		"return gluNumberModPow2(a0, 18)",
		"return gluNumberModPow2(a0, 17)",
	} {
		if !strings.Contains(generated, helper) {
			t.Fatalf("missing specialized call %q in\n%s", helper, generated)
		}
	}
	if !strings.Contains(generated, "gluNumberModLit(a0, 196608)") {
		t.Fatal("literal %3 was not lowered to a constant-modulus remainder")
	}
	if nearby := generatedSection(generated, "func (g *Game) nearby("); !strings.Contains(nearby, "gluNumberDivReciprocal(") || !strings.Contains(nearby, ", 524289)") {
		t.Fatal("non-integer literal /8.00002 was not lowered to an exact reciprocal")
	}
	if !strings.Contains(generated, ").Div(") {
		t.Fatal("runtime divisor lost the generic Number.Div path")
	}
	if !strings.Contains(generated, "gluNumberMod(glu_num_") {
		t.Fatal("runtime modulus lost the generic gluNumberMod path")
	}
	if strings.Contains(generated, "gluNumberDivPow2(a0, 3)") && strings.Count(generated, "gluNumberDivPow2(a0, 3)") < 2 {
		t.Fatal("expected both /8 and /-8 to share the shift helper")
	}

	harness := `package main
import("testing"; rt "github.com/drpaneas/mtile/runtime")
func luaMod(a, z rt.Number) rt.Number {
 if z == 0 { return 0 }
 r := a % z
 if r != 0 && ((r < 0) != (z < 0)) { r += z }
 return r
}
func TestPow2DivisionAndModulo(t *testing.T) {
 var g Game
 var c rt.Context
 g.Init(&c)
 eight, thirtyTwo, four, two := rt.FromInt(8), rt.FromInt(32), rt.FromInt(4), rt.FromInt(2)
 check := func(n int32) {
  v := rt.Number(n)
  if got, want := g.div8(&c, v), v.Div(eight); got != want {
   t.Fatalf("/8 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.div32(&c, v), v.Div(thirtyTwo); got != want {
   t.Fatalf("/32 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.divneg8(&c, v), v.Div(-eight); got != want {
   t.Fatalf("/-8 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.mod8(&c, v), luaMod(v, eight); got != want {
   t.Fatalf("mod8 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.mod4(&c, v), luaMod(v, four); got != want {
   t.Fatalf("mod4 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.mod2(&c, v), luaMod(v, two); got != want {
   t.Fatalf("mod2 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.div7(&c, v), v.Div(rt.FromInt(7)); got != want {
   t.Fatalf("/7 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.mod3(&c, v), luaMod(v, rt.FromInt(3)); got != want {
   t.Fatalf("mod3 raw%d got%d want%d", n, got, want)
  }
 }
 for n := int32(-1048576); n <= 1048576; n++ { check(n) }
 for i := int32(0); i < 100000; i++ { check(-2147483648 + i); check(2147483647 - i) }
 for bit := 0; bit < 31; bit++ {
  for d := int32(-4); d <= 4; d++ {
   check((int32(1) << bit) + d)
   check(-(int32(1) << bit) + d)
  }
 }
 seed := uint32(0x87654321)
 for i := 0; i < 1000000; i++ {
  seed = seed*1664525 + 1013904223
  check(int32(seed))
 }
 if g.nearby(&c, rt.FromInt(7)) != rt.FromInt(7).Div(rt.FromFloat(8.00002)) {
  t.Fatal("nearby divisor incorrectly specialized")
 }
 if g.divx(&c, rt.FromInt(21), rt.FromInt(7)) != rt.FromInt(3) {
  t.Fatal("runtime divisor path broke")
 }
 if g.modx(&c, rt.FromInt(8), rt.FromInt(3)) != luaMod(rt.FromInt(8), rt.FromInt(3)) {
  t.Fatal("runtime modulus path broke")
 }
}
`
	runGeneratedHarness(t, generated, harness, "TestPow2DivisionAndModulo")
}

func TestCelesteMode0SpecializesTileDivision(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "celeste-mode0")
	if _, err := os.Stat(filepath.Join(dir, "game.lua")); err != nil {
		t.Skip("celeste-mode0 example is not present")
	}
	bundle, err := sourcebundle.Resolve(dir, "game.lua")
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Transpile(bundle.Entry, bundle.Source, Options{
		Renderer: RendererMode0, MapW: 60, MapH: 20, MapLayers: 4, Map: make([]uint16, 60*20*4),
		SheetW: 240, SheetH: 160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generated, "gluNumberDivPow2(") {
		t.Fatal("celeste-mode0 still uses generic division for integer powers of two")
	}
	if !strings.Contains(generated, "gluNumberModPow2(") {
		t.Fatal("celeste-mode0 still uses generic modulo for integer powers of two")
	}
	if strings.Contains(generated, ").Div(rt.Number(524288))") || strings.Contains(generated, ").Div(rt.Number(2097152))") {
		t.Fatal("celeste-mode0 left a literal /8 or /32 on the generic 64-bit path")
	}
	if !strings.Contains(generated, "gluValueDivPow2(") {
		t.Fatal("celeste-mode0 tile /8 still uses boxed Value.Div")
	}
	if !strings.Contains(generated, "gluValueModPow2(") {
		t.Fatal("celeste-mode0 tile %8 still uses boxed gluMod")
	}
	if !strings.Contains(generated, "gluValueDivThreeHalves(") {
		t.Fatal("celeste-mode0 hair /1.5 still uses boxed Value.Div")
	}
	if strings.Contains(generated, ").Div(rt.Number(1966080))") || strings.Contains(generated, ").Div(rt.Number(3932160))") {
		t.Fatal("celeste-mode0 left a literal /30 or /60 on the generic 64-bit path")
	}
	if !strings.Contains(generated, "int32(") || (!strings.Contains(generated, "/ 30)") && !strings.Contains(generated, "gluValueDivInt(")) {
		t.Fatal("celeste-mode0 HUD /30 still uses generic division")
	}
	if !strings.Contains(generated, "gluNumberModLit(") || !strings.Contains(generated, "15728640") {
		t.Fatal("celeste-mode0 camera %240 still uses generic modulo")
	}
}

func TestConstantValuePow2DivisionPreservesNil(t *testing.T) {
	source := `
results = {}
function boxed_div(value) return value / 8 end
function boxed_mod(value) return value % 8 end
function boxed_half(value) return value / 1.5 end
function _init()
 add(results, {value=boxed_div(16)})
 add(results, {value=boxed_div("16")})
 add(results, {value=boxed_mod(-1)})
 add(results, {value=boxed_mod(false)})
 add(results, {value=boxed_half(3)})
 add(results, {value=boxed_half({})})
end
`
	generated, err := Transpile("constant-value-pow2.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, helper := range []string{"gluValueDivPow2(", "gluValueModPow2(", "gluValueDivThreeHalves("} {
		if !strings.Contains(generated, helper) {
			t.Fatalf("missing boxed helper %s", helper)
		}
	}
	harness := `package main
import("testing"; rt "github.com/drpaneas/mtile/runtime")
func TestBoxedPow2(t *testing.T){
 var g Game; var c rt.Context; g.Init(&c)
 want := []struct{ kind rt.Kind; num rt.Number }{
  {rt.KindNumber, rt.FromInt(16).Div(rt.FromInt(8))},
  {rt.KindNil, 0},
  {rt.KindNumber, func() rt.Number { a,z:=rt.FromInt(-1),rt.FromInt(8); r:=a%z; if r!=0 && ((r<0)!=(z<0)){r+=z}; return r }()},
  {rt.KindNil, 0},
  {rt.KindNumber, rt.FromInt(3).Div(rt.FromFloat(1.5))},
  {rt.KindNil, 0},
 }
 for i, row := range want {
  obj := c.TableGetInt(g.results, i+1)
  v := c.TableGet(obj, c.InternStatic("value"))
  if v.Kind != row.kind || (row.kind == rt.KindNumber && v.Num != row.num) {
   t.Fatalf("result %d kind=%d num=%d want kind=%d num=%d", i+1, v.Kind, v.Num, row.kind, row.num)
  }
 }
}
`
	runGeneratedHarness(t, generated, harness, "TestBoxedPow2")
}

func TestConstantIntegerDivisionAndModulo(t *testing.T) {
	source := `
result = 0
function div30(n) return n / 30 end
function div60(n) return n / 60 end
function div7(n) return n / 7 end
function divneg30(n) return n / -30 end
function idiv7(n) return n \ 7 end
function half(n) return n / 0.5 end
function mod30(n) return n % 30 end
function mod240(n) return n % 240 end
function divx(n, d) return n / d end
function nearby(n) return n / 30.00002 end
function _init()
 result = div30(0)+div60(0)+div7(0)+divneg30(0)+idiv7(0)+half(0)+mod30(0)+mod240(0)+divx(0,1)+nearby(0)
end
`
	generated, err := Transpile("constant-integer.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"int32(a0) / 30",
		"int32(a0) / 60",
		"int32(a0) / 7",
		"int32(a0) / -30",
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("missing 32-bit integer divide %q in\n%s", fragment, generated)
		}
	}
	if !strings.Contains(generated, ").Flr()") {
		t.Fatal("integer divide \\ 7 lost the floor")
	}
	if !strings.Contains(generated, "<< 1") && !strings.Contains(generated, "<<1") {
		t.Fatal("n / 0.5 was not lowered to a left shift")
	}
	if strings.Contains(generated, ").Div(rt.Number(1966080))") ||
		strings.Contains(generated, ").Div(rt.Number(3932160))") ||
		strings.Contains(generated, ").Div(rt.Number(458752))") {
		t.Fatal("integer literal still uses 64-bit Number.Div")
	}
	if !strings.Contains(generated, ").Div(") {
		t.Fatal("runtime divisor lost the generic Number.Div path")
	}
	if !strings.Contains(generated, "gluNumberModLit(a0, 1966080)") {
		t.Fatal("literal %30 was not lowered to a constant-modulus remainder")
	}
	if !strings.Contains(generated, "gluNumberModLit(a0, 15728640)") {
		t.Fatal("literal %240 was not lowered to a constant-modulus remainder")
	}
	if nearby := generatedSection(generated, "func (g *Game) nearby("); !strings.Contains(nearby, "gluNumberDivReciprocal(") || !strings.Contains(nearby, ", 1966081)") {
		t.Fatal("non-integer literal division was not lowered to an exact reciprocal")
	}

	harness := `package main
import("testing"; rt "github.com/drpaneas/mtile/runtime")
func luaMod(a, z rt.Number) rt.Number {
 if z == 0 { return 0 }
 r := a % z
 if r != 0 && ((r < 0) != (z < 0)) { r += z }
 return r
}
func TestIntegerDivisionAndModulo(t *testing.T) {
 var g Game
 var c rt.Context
 g.Init(&c)
 thirty, sixty, seven, half := rt.FromInt(30), rt.FromInt(60), rt.FromInt(7), rt.FromFloat(0.5)
 twoForty := rt.FromInt(240)
 check := func(n int32) {
  v := rt.Number(n)
  if got, want := g.div30(&c, v), v.Div(thirty); got != want {
   t.Fatalf("/30 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.div60(&c, v), v.Div(sixty); got != want {
   t.Fatalf("/60 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.div7(&c, v), v.Div(seven); got != want {
   t.Fatalf("/7 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.divneg30(&c, v), v.Div(-thirty); got != want {
   t.Fatalf("/-30 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.idiv7(&c, v), v.IDiv(seven); got != want {
   t.Fatalf("\\7 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.half(&c, v), v.Div(half); got != want {
   t.Fatalf("/0.5 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.mod30(&c, v), luaMod(v, thirty); got != want {
   t.Fatalf("mod30 raw%d got%d want%d", n, got, want)
  }
  if got, want := g.mod240(&c, v), luaMod(v, twoForty); got != want {
   t.Fatalf("mod240 raw%d got%d want%d", n, got, want)
  }
 }
 for n := int32(-1048576); n <= 1048576; n++ { check(n) }
 for i := int32(0); i < 100000; i++ { check(-2147483648 + i); check(2147483647 - i) }
 for bit := 0; bit < 31; bit++ {
  for d := int32(-4); d <= 4; d++ {
   check((int32(1) << bit) + d)
   check(-(int32(1) << bit) + d)
  }
 }
 seed := uint32(0x87654321)
 for i := 0; i < 1000000; i++ {
  seed = seed*1664525 + 1013904223
  check(int32(seed))
 }
 if g.nearby(&c, rt.FromInt(7)) != rt.FromInt(7).Div(rt.FromFloat(30.00002)) {
  t.Fatal("nearby divisor incorrectly specialized")
 }
 if g.divx(&c, rt.FromInt(21), rt.FromInt(7)) != rt.FromInt(3) {
  t.Fatal("runtime divisor path broke")
 }
}
`
	runGeneratedHarness(t, generated, harness, "TestIntegerDivisionAndModulo")
}

func TestConstantValueIntegerDivisionPreservesNil(t *testing.T) {
	source := `
results = {}
function boxed_div(value) return value / 30 end
function boxed_idiv(value) return value \ 7 end
function boxed_half(value) return value / 0.5 end
function boxed_mod(value) return value % 30 end
function boxed_frac(value) return value / 1.7 end
function _init()
 add(results, {value=boxed_div(60)})
 add(results, {value=boxed_div("60")})
 add(results, {value=boxed_idiv(-8)})
 add(results, {value=boxed_idiv(false)})
 add(results, {value=boxed_half(3)})
 add(results, {value=boxed_half({})})
 add(results, {value=boxed_mod(-1)})
 add(results, {value=boxed_mod(false)})
 add(results, {value=boxed_frac(17)})
 add(results, {value=boxed_frac("17")})
end
`
	generated, err := Transpile("constant-value-integer.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generated, "gluValueDivInt(") {
		t.Fatal("boxed /30 still uses Value.Div")
	}
	if !strings.Contains(generated, "gluValueIDivInt(") && !strings.Contains(generated, ").Flr()") {
		t.Fatal("boxed \\7 still uses Value.IDiv")
	}
	if !strings.Contains(generated, "gluValueModLit(") {
		t.Fatal("boxed %30 still uses gluMod")
	}
	if !strings.Contains(generatedSection(generated, "func (g *Game) boxed_frac("), "gluValueDivReciprocal(") {
		t.Fatal("boxed /1.7 does not use an exact reciprocal")
	}
	if strings.Contains(generated, ").Div(rt.Number(1966080))") {
		t.Fatal("boxed /30 left the generic 64-bit path")
	}
	harness := `package main
import("testing"; rt "github.com/drpaneas/mtile/runtime")
func TestBoxedInteger(t *testing.T){
 var g Game; var c rt.Context; g.Init(&c)
 want := []struct{ kind rt.Kind; num rt.Number }{
  {rt.KindNumber, rt.FromInt(60).Div(rt.FromInt(30))},
  {rt.KindNil, 0},
  {rt.KindNumber, rt.FromInt(-8).IDiv(rt.FromInt(7))},
  {rt.KindNil, 0},
  {rt.KindNumber, rt.FromInt(3).Div(rt.FromFloat(0.5))},
  {rt.KindNil, 0},
  {rt.KindNumber, func() rt.Number { a,z:=rt.FromInt(-1),rt.FromInt(30); r:=a%z; if r!=0 && ((r<0)!=(z<0)){r+=z}; return r }()},
  {rt.KindNil, 0},
  {rt.KindNumber, rt.FromInt(17).Div(rt.FromFloat(1.7))},
  {rt.KindNil, 0},
 }
 for i, row := range want {
  obj := c.TableGetInt(g.results, i+1)
  v := c.TableGet(obj, c.InternStatic("value"))
  if v.Kind != row.kind || (row.kind == rt.KindNumber && v.Num != row.num) {
   t.Fatalf("result %d kind=%d num=%d want kind=%d num=%d", i+1, v.Kind, v.Num, row.kind, row.num)
  }
 }
}
`
	runGeneratedHarness(t, generated, harness, "TestBoxedInteger")
}

func TestConstantModuloFromNumericBinding(t *testing.T) {
	source := `
result = 0
SCREEN = 240
function wrap(n) return n % SCREEN end
function _init() result = wrap(0) end
`
	generated, err := Transpile("constant-mod-binding.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generated, "gluNumberModLit(a0, 15728640)") {
		t.Fatalf("frozen numeric constant %%SCREEN was not lowered:\n%s", generated)
	}
	harness := `package main
import("testing"; rt "github.com/drpaneas/mtile/runtime")
func TestModBinding(t *testing.T){
 var g Game; var c rt.Context; g.Init(&c)
 a, z := rt.FromInt(-1), rt.FromInt(240)
 r := a % z
 if r != 0 && ((r < 0) != (z < 0)) { r += z }
 if got := g.wrap(&c, a); got != r {
  t.Fatalf("wrap got %d want %d", got, r)
 }
}
`
	runGeneratedHarness(t, generated, harness, "TestModBinding")
}

func TestConstantDivisionWrapsLiteralAndPropagatedOperands(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		want   string
	}{
		{"literal_pow2", "result = 20000 / 0.5", "divide(rt.FromInt(20000), rt.Number(32768))"},
		{"literal_fraction", "result = 20000 / 0.6", "divide(rt.FromInt(20000), rt.Number(39322))"},
		{"literal_negative_unit", "result = -32768 / -1", "divide(rt.Number(-2147483648), -rt.One)"},
		{"propagated_pow2", "LEFT = 20000\nRIGHT = 0.5\nresult = LEFT / RIGHT", "divide(rt.FromInt(20000), rt.Number(32768))"},
		{"propagated_fraction", "LEFT = 20000\nRIGHT = 0.6\nresult = LEFT / RIGHT", "divide(rt.FromInt(20000), rt.Number(39322))"},
		{"propagated_negative_unit", "LEFT = -32768\nRIGHT = -1\nresult = LEFT / RIGHT", "divide(rt.Number(-2147483648), -rt.One)"},
		{"floor_pow2", `result = 20000 \ 0.5`, "divide(rt.FromInt(20000), rt.Number(32768)).Flr()"},
		{"floor_fraction", `result = 20000 \ 0.6`, "divide(rt.FromInt(20000), rt.Number(39322)).Flr()"},
		{"nested_wrapping_quotient", "result = -(10000 / 0.5 / 0.5)", "-divide(divide(rt.FromInt(10000), rt.Number(32768)), rt.Number(32768))"},
		{"negated_identity_quotient", "result = -(-32768 / 1)", "-divide(rt.Number(-2147483648), rt.One)"},
		{"local_numeric_record", "local cfg = {x = 2 / 0.5, y = 20000 / 0.6}\nresult = cfg.x + cfg.y", "divide(rt.FromInt(2), rt.Number(32768)) + divide(rt.FromInt(20000), rt.Number(39322))"},
		{"global_numeric_record", "cfg = {x = 2 / 0.5, y = 20000 / 0.6}\nresult = cfg.x + cfg.y", "divide(rt.FromInt(2), rt.Number(32768)) + divide(rt.FromInt(20000), rt.Number(39322))"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			generated, err := Transpile("constant-division-wrap.lua", tc.source, Options{})
			if err != nil {
				t.Fatal(err)
			}
			harness := `package main
import("testing"; rt "github.com/drpaneas/mtile/runtime")
// Use the original widening formula independently of runtime fast paths.
func divide(a, b rt.Number) rt.Number { return rt.Number((int64(a) << 16) / int64(b)) }
func TestDivisionWrap(t *testing.T) {
 var g Game; var c rt.Context; g.Init(&c)
 want := ` + tc.want + `
 if g.result != want { t.Fatalf("got raw %d, want raw %d", g.result, want) }
}

`
			runGeneratedHarness(t, generated, harness, "TestDivisionWrap")
		})
	}
}

func TestConstantFractionalDivisionMatchesWidening(t *testing.T) {
	source := `
result = 0
results = {}
function positive(n) return n / 1.7 end
function negative(n) return n / -1.7 end
function floored(n) return n \ 1.7 end
function boxed(n) return n / 1.7 end
function subunit(n) return n / 0.6 end
function subunit_negative(n) return n / -0.6 end
function subunit_floor(n) return n \ 0.6 end
function subunit_boxed(n) return n / 0.6 end
function _init()
 result = positive(0) + negative(0) + floored(0) + subunit(0) + subunit_negative(0) + subunit_floor(0)
 add(results, {value=boxed(17)})
 add(results, {value=boxed("17")})
 add(results, {value=subunit_boxed(17)})
 add(results, {value=subunit_boxed("17")})
end
`
	generated, err := Transpile("constant-division-reciprocal.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generatedSection(generated, "func (g *Game) positive("), "gluNumberDivReciprocal(a0, 2526455886, 111411)") {
		t.Fatal("literal /1.7 did not retain its constant reciprocal parameters")
	}
	if !strings.Contains(generatedSection(generated, "func (g *Game) subunit("), "gluNumberDivFraction(a0, 39322)") {
		t.Fatal("literal /0.6 did not retain its constant fraction divisor")
	}
	harness := `package main
import("testing"; rt "github.com/drpaneas/mtile/runtime")
func widening(a rt.Number, b int64) rt.Number { return rt.Number((int64(a) << 16) / b) }
func TestReciprocal(t *testing.T) {
 var g Game; var c rt.Context; g.Init(&c)
 checkCalls := func(n int32) {
  v := rt.Number(n)
  want := widening(v, 111411)
  if g.positive(&c,v) != want || g.negative(&c,v) != -want || g.floored(&c,v) != want.Flr() {
   t.Fatalf("emitted reciprocal mismatch raw %d", n)
  }
  got := g.boxed(&c,rt.NumVal(v))
  if got.Kind != rt.KindNumber || got.Num != want { t.Fatalf("boxed reciprocal mismatch raw %d: %+v",n,got) }
  want = widening(v,39322)
  if g.subunit(&c,v) != want || g.subunit_negative(&c,v) != -want || g.subunit_floor(&c,v) != want.Flr() {
   t.Fatalf("emitted subunit mismatch raw %d",n)
  }
  got = g.subunit_boxed(&c,rt.NumVal(v))
  if got.Kind != rt.KindNumber || got.Num != want { t.Fatalf("boxed subunit mismatch raw %d: %+v",n,got) }
 }
 checkHelper := func(n int32, d uint32) {
  var got rt.Number
  if d <= 65536 { got = gluNumberDivFraction(rt.Number(n),d) } else {
   multiplier := uint32(((uint64(1)<<48) + uint64(d) - 1) / uint64(d))
   got = gluNumberDivReciprocal(rt.Number(n),multiplier,d)
  }
  if want := widening(rt.Number(n),int64(d)); got != want {
   t.Fatalf("raw %d / %d got %d want %d",n,d,got,want)
  }
 }
 for n:=int32(-1048576); n<=1048576; n++ { checkCalls(n) }
 for i:=int32(0); i<100000; i++ { checkCalls(-2147483648+i); checkCalls(2147483647-i) }
 // Exhaust divisors below 2 and every numerator power-of-two boundary.
 for d:=uint32(1); d<131072; d++ {
  checkHelper(-2147483648,d)
  for bit:=uint(0); bit<31; bit++ {
   for delta:=int32(-1); delta<=1; delta++ {
    checkHelper((int32(1)<<bit)+delta,d)
    checkHelper(-(int32(1)<<bit)+delta,d)
   }
  }
 }
 seed:=uint32(0x87654321)
 for i:=0; i<1000000; i++ {
  seed=seed*1664525+1013904223
  n:=int32(seed)
  checkCalls(n)
  seed=seed*1664525+1013904223
  d:=1+seed%2147483647
  checkHelper(n,d)
 }
 for _,bad:=range []rt.Value{rt.NilVal(),rt.BoolVal(false),c.InternStatic("17"),g.results} {
  if got:=g.boxed(&c,bad); got.Kind != rt.KindNil { t.Fatalf("boxed nonnumber accepted: %+v",got) }
  if got:=g.subunit_boxed(&c,bad); got.Kind != rt.KindNil { t.Fatalf("boxed subunit nonnumber accepted: %+v",got) }
 }
}
`
	runGeneratedHarness(t, generated, harness, "TestReciprocal")
}
