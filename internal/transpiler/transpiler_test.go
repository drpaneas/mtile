package transpiler

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

func generatedGoCacheEnv() []string {
	return append(os.Environ(), "GOCACHE="+filepath.Join(os.TempDir(), "mtile-generated-gocache"))
}

func TestGeneratePhase5SnapshotShape(t *testing.T) {
	source := `
local score = 1
function pair(x) return x, x + 1 end
function _init()
  score += 1
end
function _update()
  local a, b = pair(score)
  for v in all({1, 2}) do score += v end
end
function _draw()
  cls()
  spr(1, score, 2)
  print("hello", 0, 0, 7)
end
`
	got, err := Transpile("snapshot.lua", source, Options{SheetW: 16, SheetH: 8, MapW: 4, MapH: 4})
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := format.Source([]byte(got))
	if err != nil {
		t.Fatalf("generated Go is not syntactically valid: %v\n%s", err, got)
	}
	got = string(formatted)
	for _, want := range []string{
		"func (g *Game) Init(c *rt.Context)",
		"c.Spr(",
		"&ctx",
		"ctx.BeginFramePace()",
		"ctx.BeginProfilePhase(rt.ProfileUpdate)",
		"ctx.EndProfilePhase(rt.ProfileUpdate)",
		"ctx.BeginProfilePhase(rt.ProfileDraw)",
		"ctx.EndProfilePhase(rt.ProfileDraw)",
		"ctx.PaceFrame30()",
		"ctx.EndFramePace30()",
		"ctx.WaitVDraw()",
		"ctx.BeginRenderFrame()",
		"ctx.PrepareRender()",
		"ctx.PresentRender()",
		"sheetROM",
		"sheetRAM",
		"cartData",
		"c.TableLen(",
		"c.TableGetInt(",
		"return v0, v1",
		"func (g *Game) gluInit(",
		"g.gluInit(c)",
		"func (g *Game) __glu_call_gluInit(",
		"func (g *Game) gluUpdate(",
		"func (g *Game) gluDraw(",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated source does not contain %q", want)
		}
	}
	numericScore := false
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "score") && strings.Contains(line, "rt.Number") {
			numericScore = true
			break
		}
	}
	if !numericScore {
		t.Error("generated Game does not store score as rt.Number")
	}
	if strings.Contains(got, "c.RegisterGlobal(&g.score)") {
		t.Error("native numeric global was incorrectly registered as a heap root")
	}
	if strings.Contains(got, "func()") {
		t.Error("generated source contains an immediately-invoked function literal; TinyGo -gc=none cannot link runtime.alloc")
	}
	compileGenerated(t, got)
}

func TestGeneratedLoopElidesUnobservableInputAndTimeState(t *testing.T) {
	got, err := Transpile("idle.lua", `
function _update() end
function _draw() cls(0) end
`, Options{SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"ctx.PollInput()", "ctx.AdvanceTick()", "ctx.SetTickRate(30)"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("idle generated loop unexpectedly contains %q", unwanted)
		}
	}
}

func TestGeneratedLoopRetainsObservableInputAndTimeState(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantPoll   bool
		wantTick   bool
		wantRate30 bool
	}{
		{name: "input", source: `function _update() pressed = btnp(0) end`, wantPoll: true, wantTick: true},
		{name: "time", source: `function _update() elapsed = time() end`, wantTick: true, wantRate30: true},
		{name: "both", source: `function _update() pressed = btn(0) elapsed = t() end`, wantPoll: true, wantTick: true, wantRate30: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Transpile(test.name+".lua", test.source, Options{SheetW: 8, SheetH: 8})
			if err != nil {
				t.Fatal(err)
			}
			checks := []struct {
				text string
				want bool
			}{
				{text: "ctx.PollInput()", want: test.wantPoll},
				{text: "ctx.AdvanceTick()", want: test.wantTick},
				{text: "ctx.SetTickRate(30)", want: test.wantRate30},
			}
			for _, check := range checks {
				if got := strings.Contains(got, check.text); got != check.want {
					t.Errorf("generated source contains %q = %t, want %t", check.text, got, check.want)
				}
			}
		})
	}
}

func TestGeneratedCartridgeSharesExactRuntimeDefaultPalette(t *testing.T) {
	canonical := assetsync.CanonicalPaletteROM()
	got, err := Transpile("default_palette.lua", `function _draw() cls(0) end`, Options{
		SheetW: 8, SheetH: 8, Palette: canonical[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "paletteROM") || strings.Contains(got, "ctx.LoadPalette(") {
		t.Fatal("exact runtime default palette was duplicated in generated cartridge")
	}
	compileGenerated(t, got)

	custom := canonical
	custom[1] ^= 1
	got, err = Transpile("custom_palette.lua", `function _draw() cls(1) end`, Options{
		SheetW: 8, SheetH: 8, Palette: custom[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "var paletteROM = [256]uint16") || !strings.Contains(got, "ctx.LoadPalette(&paletteROM)") {
		t.Fatal("custom palette was not embedded and loaded")
	}
	compileGenerated(t, got)
}

func TestGenerateRejectsGlobalOverflow(t *testing.T) {
	var source strings.Builder
	for i := 0; i < 257; i++ {
		fmt.Fprintf(&source, "g%d = %d\n", i, i)
	}
	if _, err := Transpile("overflow.lua", source.String(), Options{}); err == nil || !strings.Contains(err.Error(), "maxGlobals") {
		t.Fatalf("error = %v, want maxGlobals rejection", err)
	}
}

func TestAnalyzeGlobalsReportsPersistentNamesAndFirstOrigins(t *testing.T) {
	report, err := AnalyzeGlobals("game.lua", `local player = {x = 1}
score = 0
function update(step)
  local scratch = step
  score = score + scratch + misspelled_score
  player.x = player.x + step
end
`)
	if err != nil {
		t.Fatal(err)
	}
	if report.Count != 3 || report.Limit != GlobalLimit {
		t.Fatalf("global report = %#v, want 3/%d", report, GlobalLimit)
	}
	want := []struct {
		name   string
		line   int
		column int
	}{
		{name: "player", line: 1, column: 1},
		{name: "score", line: 2, column: 1},
		{name: "misspelled_score", line: 5, column: 29},
	}
	for index, symbol := range report.Symbols {
		if symbol.Name != want[index].name || symbol.Position.Source != "game.lua" ||
			symbol.Position.Line != want[index].line || symbol.Position.Column != want[index].column {
			t.Fatalf("symbol %d = %#v, want %#v", index, symbol, want[index])
		}
	}
}

func TestAnalyzeGlobalsReturnsReportWithLimitError(t *testing.T) {
	var source strings.Builder
	for i := 0; i < GlobalLimit; i++ {
		fmt.Fprintf(&source, "g%d = %d\n", i, i)
	}
	atLimit, err := AnalyzeGlobals("full.lua", source.String())
	if err != nil || atLimit.Count != GlobalLimit {
		t.Fatalf("exact-limit report = %#v / %v", atLimit, err)
	}
	fmt.Fprintf(&source, "g%d = %d\n", GlobalLimit, GlobalLimit)
	report, err := AnalyzeGlobals("overflow.lua", source.String())
	var limitErr *GlobalLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("error = %v, want GlobalLimitError", err)
	}
	if report.Count != GlobalLimit+1 || len(report.Symbols) != GlobalLimit+1 || limitErr.Report.Count != report.Count {
		t.Fatalf("overflow report = %#v / %#v", report, limitErr.Report)
	}
}

func TestGenerateRejectsPalLayerTwo(t *testing.T) {
	_, err := Transpile("pal2.lua", `
function _draw() pal(2) end
`, Options{SheetW: 8, SheetH: 8})
	if err == nil || !strings.Contains(err.Error(), "pal(2)") {
		t.Fatalf("error = %v, want pal(2) rejection", err)
	}
}

func TestGeneratedGameCompilesWithHostGo(t *testing.T) {
	got, err := Transpile("compile.lua", `
function _draw()
  cls()
  pset(1, 2, 255)
  spr(0, 0, 0)
end
`, Options{SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := format.Source([]byte(got)); err != nil {
		t.Fatal(err)
	}

	compileGenerated(t, got)
}

func compileGenerated(t *testing.T, got string) {
	t.Helper()
	dir := writeGeneratedPackage(t, got)
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = generatedGoCacheEnv()
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("generated package does not compile: %v\n%s", err, out.String())
	}
}

func writeGeneratedPackage(t *testing.T, got string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(got), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeDir, err := filepath.Abs(filepath.Join("..", "..", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	mod := fmt.Sprintf("module generated\n\ngo 1.27.1\n\nrequire github.com/drpaneas/mtile/runtime v0.0.0\n\nreplace github.com/drpaneas/mtile/runtime => %s\n", runtimeDir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runGeneratedHarness compiles a generated package together with an extra
// test file (harness) written into the same directory/package, then runs
// the named test. This lets regression tests assert on actual runtime
// values produced by generated code, not just that it compiles.
func runGeneratedHarness(t *testing.T, got, harness, testName string) string {
	t.Helper()
	formatted, err := format.Source([]byte(got))
	if err != nil {
		t.Fatalf("generated Go is not syntactically valid: %v\n%s", err, got)
	}
	dir := writeGeneratedPackage(t, string(formatted))
	if err := os.WriteFile(filepath.Join(dir, "harness_test.go"), []byte(harness), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-run", testName, "-v", "./...")
	cmd.Dir = dir
	cmd.Env = generatedGoCacheEnv()
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("generated harness failed: %v\n%s", err, out.String())
	}
	return out.String()
}

func generatedSection(source, startMarker string) string {
	start := strings.Index(source, startMarker)
	if start < 0 {
		return ""
	}
	rest := source[start:]
	end := strings.Index(rest, "\n}")
	if end < 0 {
		return rest
	}
	return rest[:end+2]
}

// TestForLoopVariableDoesNotClobberOuterBinding guards against a regression
// where a for-loop control variable that shares a name with a function
// parameter (or any other enclosing local) reused the same Go variable slot.
// Lua scopes a for-loop's control variable to the loop body only; the outer
// binding of the same name must be unaffected once the loop ends.
func TestForLoopVariableDoesNotClobberOuterBinding(t *testing.T) {
	got, err := Transpile("shadow.lua", `
function foo(x)
  for x = 1, 3 do
  end
  return x
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestFooPreservesParamAcrossShadowingForLoop(t *testing.T) {
	var g Game
	var c rt.Context
	got := g.__glu_call_foo(&c, rt.IntVal(99))
	if got.Int() != 99 {
		t.Fatalf("foo(99) = %d, want 99 (param must survive a for-loop variable of the same name)", got.Int())
	}
}
`
	runGeneratedHarness(t, got, harness, "TestFooPreservesParamAcrossShadowingForLoop")
}

// TestNestedLocalDoesNotClobberOuterBinding guards the same class of bug for
// a plain nested-block `local` re-declaration (if/do/while), not just
// for-loop variables.
func TestNestedLocalDoesNotClobberOuterBinding(t *testing.T) {
	got, err := Transpile("shadow2.lua", `
function foo()
  local x = 5
  if true then
    local x = 10
  end
  return x
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestFooPreservesOuterLocalAcrossNestedShadow(t *testing.T) {
	var g Game
	var c rt.Context
	got := g.__glu_call_foo(&c)
	if got.Int() != 5 {
		t.Fatalf("foo() = %d, want 5 (nested local x must not clobber outer local x)", got.Int())
	}
}
`
	runGeneratedHarness(t, got, harness, "TestFooPreservesOuterLocalAcrossNestedShadow")
}

// TestLocalMultiAssignCapturesAllReturnValues guards against a regression
// where `local a, b = f()` (f returning more than one value) only captured
// the first return value and silently nil-filled the rest, instead of
// spreading all of f's return values across a, b, ....
func TestLocalMultiAssignCapturesAllReturnValues(t *testing.T) {
	got, err := Transpile("multiret.lua", `
function pair(x) return x, x + 1 end
function combine()
  local a, b = pair(5)
  return a + b
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestCombineSeesBothReturnValues(t *testing.T) {
	var g Game
	var c rt.Context
	got := g.__glu_call_combine(&c)
	if got.Int() != 11 {
		t.Fatalf("combine() = %d, want 11 (5 + 6; local a, b = pair(5) must capture both return values)", got.Int())
	}
}
`
	runGeneratedHarness(t, got, harness, "TestCombineSeesBothReturnValues")
}

// TestSameBlockLocalRedeclareUsesLatestBinding guards Lua's rule that a
// second `local x = ...` in the very same block shadows the first, with the
// RHS reading the binding that was live just before the new declaration.
func TestSameBlockLocalRedeclareUsesLatestBinding(t *testing.T) {
	got, err := Transpile("redeclare.lua", `
function foo()
  local x = 1
  local x = x + 1
  return x
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestFooRedeclareYieldsTwo(t *testing.T) {
	var g Game
	var c rt.Context
	got := g.__glu_call_foo(&c)
	if got.Int() != 2 {
		t.Fatalf("foo() = %d, want 2 (local x = x + 1 must read the prior x)", got.Int())
	}
}
`
	runGeneratedHarness(t, got, harness, "TestFooRedeclareYieldsTwo")
}

// TestRepeatUntilConditionSeesBodyLocal guards Lua's special repeat/until
// scoping rule: unlike while/for, the until-condition is evaluated inside
// the loop body's own scope and may reference locals declared there.
func TestRepeatUntilConditionSeesBodyLocal(t *testing.T) {
	got, err := Transpile("repeat.lua", `
function foo()
  local n = 0
  repeat
    local done = n >= 3
    n = n + 1
  until done
  return n
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestFooRepeatUntilCountsToFour(t *testing.T) {
	var g Game
	var c rt.Context
	got := g.__glu_call_foo(&c)
	if got.Int() != 4 {
		t.Fatalf("foo() = %d, want 4 (until condition must see body-local done)", got.Int())
	}
}
`
	runGeneratedHarness(t, got, harness, "TestFooRepeatUntilCountsToFour")
}

func TestGeneratedLuaRuntimeValues(t *testing.T) {
	tests := []struct {
		name   string
		source string
		call   string
		want   int
		opts   Options
		cart   bool
	}{
		{
			name: "assignment swap evaluates both rhs before writes",
			source: `
function foo()
  local a, b = 1, 2
  a, b = b, a
  return a * 10 + b
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 21,
		},
		{
			name: "indexed assignment uses the old key",
			source: `
function foo()
  local i = 1
  local t = {0, 0, 0}
  i, t[i] = 2, 99
  return t[1] + t[2] * 100 + i * 1000
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 2099,
		},
		{
			name: "trailing call expands in local assign",
			source: `
function pair(x) return x, x + 1 end
function foo()
  local a, b, c = 1, pair(5)
  return a * 100 + b * 10 + c
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 156,
		},
		{
			name: "return f() forwards every result",
			source: `
function pair(x) return x, x + 1 end
function wrap()
  return pair(5)
end
function foo()
  local a, b = wrap()
  return a + b
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 11,
		},
		{
			name: "return 1, f() expands the trailing call",
			source: `
function pair(x) return x, x + 1 end
function wrap()
  return 1, pair(5)
end
function foo()
  local a, b, c = wrap()
  return a * 100 + b * 10 + c
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 156,
		},
		{
			name: "extra rhs expressions still run",
			source: `
n = 0
function bump()
  n = n + 1
  return 9
end
function foo()
  local a = 1, bump()
  return a * 10 + n
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 11,
		},
		{
			name: "no-return call still runs in value context",
			source: `
n = 0
function bump()
  n = n + 1
end
function foo()
  local ignored = bump()
  return n
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 1,
		},
		{
			name: "discarded trailing multi-return call still runs",
			source: `
n = 0
function pair()
  n = n + 1
  return 2, 3
end
function foo()
  local a = 1, pair()
  return a * 10 + n
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 11,
		},
		{
			name: "nested local does not hide surrounding global",
			source: `
function foo()
  x = 7
  if true then
    local x = 1
  end
  return x
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 7,
		},
		{
			name: "method call resolves and passes self",
			source: `
obj = {k = 2}
function obj:inc(n)
  return n + self.k
end
function foo()
  return obj:inc(10)
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 12,
		},
		{
			name: "dotted function call resolves",
			source: `
function foo.bar()
  return 7
end
function baz()
  return foo.bar()
end
`,
			call: "g.__glu_call_baz(&c)",
			want: 7,
		},
		{
			name: "add expression returns the inserted value",
			source: `
function foo()
  local t = {}
  local v = add(t, 3)
  return v + t[1]
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 6,
		},
		{
			name: "two add on empty table grows",
			source: `
function foo()
  local t = {}
  add(t, 1)
  add(t, 2)
  return count(t)
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 2,
		},
		{
			name: "hash set beyond literal fields grows",
			source: `
function foo()
  local t = {k = 2}
  t.n = 3
  return t.k + t.n
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 5,
		},
		{
			name: "calling _init from user code does not re-register",
			source: `
n = 0
function _init()
  n = n + 1
end
function foo()
  _init()
  return n
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 2,
		},
		{
			name: "calling _update from user code",
			source: `
n = 0
function _update()
  n = n + 10
end
function foo()
  _update()
  return n
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 10,
		},
		{
			name: "one-arg fget returns the flag byte",
			source: `
function foo()
  return fget(0)
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 5,
			opts: Options{SheetW: 8, SheetH: 8, Flags: []byte{0x05}},
			cart: true,
		},
		{
			name: "pal() resets a draw remap",
			source: `
function foo()
  pal(3, 9)
  pal()
  pset(0, 0, 3)
  return pget(0, 0)
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 3,
		},
		{
			name: "pal(0) resets only the draw layer",
			source: `
function foo()
  pal(3, 9)
  pal(0)
  pset(0, 0, 3)
  return pget(0, 0)
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 3,
		},
		{
			name: "palt() restores default sprite transparency",
			source: `
function foo()
  cls(7)
  palt(0, false)
  spr(0, 0, 0)
  palt()
  cls(7)
  spr(0, 0, 0)
  return pget(0, 0)
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 7,
			opts: Options{SheetW: 8, SheetH: 8},
			cart: true,
		},
		{
			name: "two-arg fset writes the flag byte",
			source: `
function foo()
  fset(0, 7)
  return fget(0)
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 7,
			opts: Options{SheetW: 8, SheetH: 8, Flags: []byte{0x00}},
			cart: true,
		},
		{
			name: "missing user-call args are nil",
			source: `
function take(a, b)
  if b == nil then return a + 1 else return a + b end
end
function foo()
  return take(4)
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 5,
		},
		{
			name: "extra user-call args still run",
			source: `
n = 0
function bump()
  n = n + 1
  return 9
end
function take(a)
  return a
end
function foo()
  return take(3, bump()) + n
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 4,
		},
		{
			name: "generic for does not clobber an outer binding",
			source: `
function foo(x)
  for x in all({1, 2}) do
  end
  return x
end
`,
			call: "g.__glu_call_foo(&c, rt.IntVal(99))",
			want: 99,
		},
		{
			name: "generic for exposes yielded values",
			source: `
function foo()
  local n = 0
  for v in all({1, 2}) do
    n = n + v
  end
  for k, v in pairs({a = 3, b = 4}) do
    n = n + v
  end
  return n
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 10,
		},
		{
			name: "for body local shadows the loop variable",
			source: `
function foo()
  local n = 0
  for i = 1, 3 do
    local i = 9
    n = n + i
  end
  return n
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 27,
		},
		{
			name: "compound assignment with table index uses temp",
			source: `
function idx()
  n += 1
  return n
end
n = 0
function foo()
  local t = {0, 0, 0}
  n = 1
  t[idx()] += 10
  return t[1] * 100 + t[2] * 10 + n
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 102,
		},
		{
			name: "and short-circuits returning falsy value",
			source: `
function foo()
  local a = false and 99
  if a == false then return 1 end
  return 0
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 1,
		},
		{
			name: "or short-circuits returning truthy value",
			source: `
function foo()
  local a = 0 or 99
  return a
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 0,
		},
		{
			name: "and returns rhs when lhs truthy",
			source: `
function foo()
  local a = 5 and 42
  return a
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 42,
		},
		{
			name: "or returns rhs when lhs falsy",
			source: `
function foo()
  local a = nil or 77
  return a
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 77,
		},
		{
			name: "zero is truthy in and",
			source: `
function foo()
  local a = 0 and 33
  return a
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 33,
		},
		{
			name: "default parameter fills missing args",
			source: `
function foo()
  cls()
  return pget(0, 0)
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 0,
			opts: Options{SheetW: 8, SheetH: 8},
			cart: true,
		},
		{
			name: "fractional number literal preserves fraction",
			source: `
function foo()
  local x = 1.5
  return x + x
end
`,
			call: "g.__glu_call_foo(&c)",
			want: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Transpile(tt.name+".lua", tt.source, tt.opts)
			if err != nil {
				t.Fatalf("transpile: %v", err)
			}
			harness := generatedValueHarness(tt.call, tt.want, tt.cart)
			runGeneratedHarness(t, got, harness, "TestGeneratedValue")
		})
	}
}

func generatedValueHarness(call string, want int, cart bool) string {
	var setup strings.Builder
	setup.WriteString("\tvar g Game\n\tvar c rt.Context\n")
	if cart {
		setup.WriteString(`	cartImage := rt.AssetImage{
		SheetW: SheetW, SheetH: SheetH, SpriteCols: SpriteCols, SpriteRows: SpriteRows, SpriteN: SpriteN,
		MapW: MapW, MapH: MapH, SheetReadOnly: SheetReadOnly, Sheet: sheetROM[:], SheetIndex: sheetIndexROM[:], Map: mapROM[:], Flags: flagROM[:],
	}
	cartData.SheetW, cartData.SheetH = SheetW, SheetH
	cartData.SpriteCols, cartData.SpriteRows, cartData.SpriteN = SpriteCols, SpriteRows, SpriteN
	cartData.MapW, cartData.MapH = MapW, MapH
	cartData.SheetReadOnly = SheetReadOnly
	cartData.Sheet, cartData.SheetIndex, cartData.Map, cartData.Flags = sheetRAM[:], sheetIndexRAM[:], mapRAM[:], flagRAM[:]
	if !c.Init(&cartImage, &cartData) {
		t.Fatal("runtime Init failed")
	}
`)
	}
	setup.WriteString("\tg.Init(&c)\n")
	return fmt.Sprintf(`package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestGeneratedValue(t *testing.T) {
%s	got := %s
	if got.Int() != %d {
		t.Fatalf("got %%d, want %d", got.Int())
	}
}
`, setup.String(), call, want, want)
}

// TestStaticStringLiteralUsesStaticKind verifies that string literals in
// generated code produce KindStaticString values (ROM-resident, no heap
// allocation) rather than KindString (heap-allocated).
func TestStaticStringLiteralUsesStaticKind(t *testing.T) {
	got, err := Transpile("static.lua", `
function foo()
  local s = "hello"
  return s
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestStaticKind(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := g.__glu_call_foo(&c)
	if got.Kind != rt.KindStaticString {
		t.Fatalf("string literal Kind = %d, want KindStaticString (%d)", got.Kind, rt.KindStaticString)
	}
	if c.StrLen(got) != 5 {
		t.Fatalf("string literal len = %d, want 5", c.StrLen(got))
	}
}
`
	runGeneratedHarness(t, got, harness, "TestStaticKind")
}

// TestQuestionExprDesugarsIntoPrint verifies that ?expr at line start
// desugars into a print() call that the transpiler emits correctly.
func TestQuestionExprDesugarsIntoPrint(t *testing.T) {
	got, err := Transpile("qprint.lua", `
function _draw()
  ?"hello"
end
`, Options{SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := format.Source([]byte(got)); err != nil {
		t.Fatalf("generated Go is not syntactically valid: %v\n%s", err, got)
	}
	if !strings.Contains(got, "PrintV") {
		t.Error("?expr did not produce a PrintV call in generated code")
	}
	compileGenerated(t, got)
}

// TestBitwiseOnFractionalPreservesFractionBits verifies that bitwise
// operations work on the full 32-bit fixed-point representation, not just
// the integer part.
func TestBitwiseOnFractionalPreservesFractionBits(t *testing.T) {
	got, err := Transpile("bitfrac.lua", `
function foo()
  local x = 1.5
  local y = band(x, x)
  return y + y
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestBitFrac(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := g.__glu_call_foo(&c)
	// band(1.5, 1.5) = 1.5 (identity on full 32-bit). 1.5 + 1.5 = 3.
	// If fraction bits were stripped, band(1,1)=1, 1+1=2.
	if got.Int() != 3 {
		t.Fatalf("band(1.5,1.5)+band(1.5,1.5) = %d, want 3 (fraction bits must be preserved)", got.Int())
	}
}
`
	runGeneratedHarness(t, got, harness, "TestBitFrac")
}

// TestTableLiteralRootsWithPushLocal verifies that table literal
// construction uses PushLocal/PopLocals for GC safety, and the resulting
// table is usable.
func TestTableLiteralRootsWithPushLocal(t *testing.T) {
	got, err := Transpile("tableroots.lua", `
function foo()
  local t = {10, 20, 30}
  return t[1] + t[2] + t[3]
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "PushLocal") || !strings.Contains(got, "PopLocals") {
		t.Error("table literal codegen must use PushLocal/PopLocals for GC rooting")
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestTableRoots(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := g.__glu_call_foo(&c)
	if got.Int() != 60 {
		t.Fatalf("t[1]+t[2]+t[3] = %d, want 60", got.Int())
	}
}
`
	runGeneratedHarness(t, got, harness, "TestTableRoots")
}

// TestFractionalLiteralCodegenPrecision verifies that fractional number
// literals use the correct 16:16 fixed-point encoding, whether they remain a
// Value or lower directly to monomorphic Number storage.
func TestFractionalLiteralCodegenPrecision(t *testing.T) {
	got, err := Transpile("frac.lua", `
function foo()
  local x = 0.25
  local y = x + x + x + x
  return y
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "rt.Number(16384)") {
		t.Error("fractional literal must preserve 0.25 as 16:16 rt.Number(16384)")
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestFracPrecision(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := g.__glu_call_foo(&c)
	// 0.25 * 4 = 1.0. If truncated to int, 0*4=0.
	if got.Int() != 1 {
		t.Fatalf("0.25+0.25+0.25+0.25 = %d, want 1 (fraction must not be truncated)", got.Int())
	}
}
`
	runGeneratedHarness(t, got, harness, "TestFracPrecision")
}

func TestFractionalLiteralCodegenWrapsToRuntimeNumber(t *testing.T) {
	const source = `x = 32768.5`
	const want = -2147450880
	tests := []struct {
		name  string
		opts  Options
		value string
	}{
		{name: "monomorphic", opts: Options{DisableNumericConstants: true}, value: "g.x"},
		{name: "generic-value", opts: Options{DisableMonomorphicNumbers: true, DisableNumericConstants: true}, value: "g.x.NumberOr(0)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Transpile(test.name+".lua", source, test.opts)
			if err != nil {
				t.Fatal(err)
			}
			harness := fmt.Sprintf(`package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestWrappedFractionalLiteral(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := %s
	if got != rt.Number(%d) {
		t.Fatalf("wrapped literal = %%d, want %d", got)
	}
}
`, test.value, want, want)
			runGeneratedHarness(t, got, harness, "TestWrappedFractionalLiteral")
		})
	}
}

func TestUnaryMinimumFractionalLiteralCompilesAndWraps(t *testing.T) {
	const source = `x = -32768.0`
	const want = -2147483648
	tests := []struct {
		name  string
		opts  Options
		value string
	}{
		{name: "monomorphic", opts: Options{DisableNumericConstants: true}, value: "g.x"},
		{name: "generic-value", opts: Options{DisableMonomorphicNumbers: true, DisableNumericConstants: true}, value: "g.x.NumberOr(0)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Transpile(test.name+".lua", source, test.opts)
			if err != nil {
				t.Fatal(err)
			}
			harness := fmt.Sprintf(`package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestUnaryMinimumLiteral(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := %s
	if got != rt.Number(%d) {
		t.Fatalf("unary minimum literal = %%d, want %d", got)
	}
}
`, test.value, want, want)
			runGeneratedHarness(t, got, harness, "TestUnaryMinimumLiteral")
		})
	}
}

func TestLargeIntegerLiteralLowersWithoutTargetIntOverflow(t *testing.T) {
	const source = `x = 2147483648`
	tests := []struct {
		name  string
		opts  Options
		value string
	}{
		{name: "monomorphic", opts: Options{DisableNumericConstants: true}, value: "g.x"},
		{name: "generic-value", opts: Options{DisableMonomorphicNumbers: true, DisableNumericConstants: true}, value: "g.x.NumberOr(0)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Transpile(test.name+".lua", source, test.opts)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got, "FromInt(2147483648)") || strings.Contains(got, "IntVal(2147483648)") {
				t.Fatalf("large integer literal still depends on a target-sized int:\n%s", got)
			}
			harness := fmt.Sprintf(`package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestLargeIntegerLiteral(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := %s
	if got != 0 {
		t.Fatalf("wrapped large integer literal = %%d, want 0", got)
	}
}
`, test.value)
			runGeneratedHarness(t, got, harness, "TestLargeIntegerLiteral")
		})
	}
}

// TestSiblingCallArgumentsSurviveCollectionDuringLaterSibling is a stress
// regression test for a GC-rooting bug: when two sibling sub-expressions of
// one call/operator both allocate on the heap (e.g. f(noisy(), make())), the
// first sibling's result must stay reachable while the second sibling
// evaluates, even if the second sibling's allocations trigger a collection.
// Without incremental rooting (see sequenceExprs in transpiler.go), the
// first sibling's heap object has no registered root at the moment of
// collection and is silently reclaimed, corrupting the value the enclosing
// call receives.
//
// runonce() assigns the exact vulnerable shape (result = combine(noisy(),
// make())) to a global, once per outer iteration. noisy() returns a freshly
// allocated string (via tostr) so it is heap-tracked. make() repeatedly
// calls add() to grow a table's backing array, which reallocates many times
// and produces enough garbage that, across hundreds of iterations, a
// collection is essentially guaranteed to land while make() is still
// running - exactly the window where noisy()'s result would previously go
// unrooted. Each iteration's result is checked immediately, so if the fix
// regresses, this test fails deterministically rather than flakily.
func TestSiblingCallArgumentsSurviveCollectionDuringLaterSibling(t *testing.T) {
	got, err := Transpile("rootgap.lua", `
counter = 0
result = 0

function noisy()
  counter = counter + 1
  return tostr(counter)
end

function make()
  local t = {}
  for i = 1, 300 do
    add(t, i)
  end
  return t
end

function combine(a, b)
  return a
end

function runonce()
  result = combine(noisy(), make())
end
`, Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestSiblingArgsSurviveCollection(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	for i := 1; i <= 300; i++ {
		g.__glu_call_runonce(&c)
		want, herr := c.Tostr(rt.IntVal(i), false)
		if herr != rt.HeapOK {
			t.Fatalf("iteration %d: building expected value: %v", i, herr)
		}
		if !c.ValuesEqual(g.result, want) {
			t.Fatalf("iteration %d: combine(noisy(), make()) was corrupted by a collection during make() (noisy()'s result did not survive unrooted)", i)
		}
	}
}
`
	runGeneratedHarness(t, got, harness, "TestSiblingArgsSurviveCollection")
}

// TestReturnInsideRootedForLoopsDoesNotLeakLocalRoots guards the control-flow
// side of the generated root protocol. Generic numeric loops root their init,
// limit, and step values, while all()/pairs() loops root the iterated table.
// A Lua return from either loop must unwind those roots before it leaves the
// generated Go function. Otherwise repeated collision-style probes eventually
// fill Context's fixed local-root stack and make a loop silently stop running.
func TestReturnInsideRootedForLoopsDoesNotLeakLocalRoots(t *testing.T) {
	got, err := Transpile("loop-return-roots.lua", `
items = {1}

function numeric_hit(first, last)
  for y = first, last do
    for x = first, last do
      return true
    end
  end
  return false
end

function generic_hit()
  for value in all(items) do
    return true
  end
  return false
end
`, Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestLoopReturnsReleaseRoots(t *testing.T) {
	var numericGame Game
	var numericContext rt.Context
	numericGame.Init(&numericContext)
	for call := 1; call <= 100; call++ {
		if !numericGame.__glu_call_numeric_hit(&numericContext, rt.IntVal(0), rt.IntVal(0)).Truthy() {
			t.Fatalf("numeric loop stopped returning its first hit on call %d", call)
		}
	}

	var genericGame Game
	var genericContext rt.Context
	genericGame.Init(&genericContext)
	for call := 1; call <= 600; call++ {
		if !genericGame.__glu_call_generic_hit(&genericContext).Truthy() {
			t.Fatalf("generic loop stopped returning its first hit on call %d", call)
		}
	}
}
`
	runGeneratedHarness(t, got, harness, "TestLoopReturnsReleaseRoots")
}

func TestTableLiteralRootsLiveSiblingAcrossCollection(t *testing.T) {
	// Make the later table literal large enough that repeated calls force a
	// collection during NewTableWithCaps, while the earlier sibling is still
	// only held in a generated temporary.
	fields := strings.TrimSuffix(strings.Repeat("1,", 64), ",")
	source := fmt.Sprintf(`
counter = 0
result = 0

function noisy()
  counter = counter + 1
  return tostr(counter)
end

function combine(a, b)
  return a
end

function runonce()
  result = combine(noisy(), {%s})
end
`, fields)
	got, err := Transpile("tableliteral-rootgap.lua", source, Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestTableLiteralSiblingSurvivesCollection(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	for i := 1; i <= 200; i++ {
		g.__glu_call_runonce(&c)
		want, err := c.Tostr(rt.IntVal(i), false)
		if err != rt.HeapOK {
			t.Fatal(err)
		}
		if !c.ValuesEqual(g.result, want) {
			t.Fatalf("iteration %d: table literal collection corrupted noisy() result", i)
		}
	}
}
`
	runGeneratedHarness(t, got, harness, "TestTableLiteralSiblingSurvivesCollection")
}

func TestBinaryOperandsSurviveCollection(t *testing.T) {
	got, err := Transpile("binary-rootgap.lua", `
function left()
  return tostr(123456)
end

function right()
  for i = 1, 20000 do chr(65) end
  return tostr(123456)
end

function equal()
  return left() == right()
end

function logical()
  return left() and right()
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestBinaryOperandsRemainLive(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	for i := 0; i < 8; i++ {
		if !g.__glu_call_equal(&c).Truthy() {
			t.Fatalf("equal() was false on iteration %d", i)
		}
		if !g.__glu_call_logical(&c).Truthy() {
			t.Fatalf("logical() was false on iteration %d", i)
		}
	}
}
`
	runGeneratedHarness(t, got, harness, "TestBinaryOperandsRemainLive")
}

func TestZeroResultTailCallArityAndSideEffect(t *testing.T) {
	got, err := Transpile("zero-tail.lua", `
n = 0
function bump()
  n = n + 1
end
function wrap()
  return bump()
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "func (g *Game) wrap(c *rt.Context) {") {
		t.Fatal("wrap() must remain a zero-result function when its trailing call returns no values")
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestZeroResultTailCallRunsSideEffect(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	g.__glu_call_wrap(&c)
	if g.n.Int() != 1 {
		t.Fatalf("wrap() side effect count = %d, want 1", g.n.Int())
	}
}
`
	runGeneratedHarness(t, got, harness, "TestZeroResultTailCallRunsSideEffect")
}

// TestDiscardedBuiltinResultsStillEvaluateArguments guards real audio calls
// and the remaining printh no-op from discarding argument
// expressions when their return value is ignored. sequenceExprs only produces Go statements
// when there are 2+ sibling arguments to protect from each other; with 0 or
// 1 arguments it returns the raw expression string unevaluated, so a naive
// "return rt.NilVal()" for these builtins silently dropped a single
// argument's side effect (e.g. printh(bump()) never called bump()). This
// checks both the single-argument case and the multi-argument case, and
// both statement context (bare call) and value context (used as an
// expression).
func TestDiscardedBuiltinResultsStillEvaluateArguments(t *testing.T) {
	got, err := Transpile("noop-args.lua", `
n = 0
m = 0
x = 0

function bump()
  n = n + 1
  return n
end

function bump2()
  m = m + 1
  return m
end

function _update()
  printh(bump())
  x = sfx(bump(), bump2())
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestDiscardedBuiltinArgsRunSideEffects(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	g.__glu_call_gluUpdate(&c)
	if g.n.Int() != 2 {
		t.Fatalf("bump() call count (via n) = %d, want 2 (once for printh(bump()), once for sfx(bump(),bump2()))", g.n.Int())
	}
	if g.m.Int() != 1 {
		t.Fatalf("bump2() call count (via m) = %d, want 1", g.m.Int())
	}
}
`
	runGeneratedHarness(t, got, harness, "TestDiscardedBuiltinArgsRunSideEffects")
}

// TestTrailingMultiReturnExpandsInTableLiteral verifies that a multi-return
// call as the last field of a table constructor expands into all of its
// results (Lua semantics: `{1, pair()}` with `pair() = 10, 20` builds a
// 3-element array {1, 10, 20}), rather than silently keeping only the
// first result the way any other single-value expression context would.
func TestTrailingMultiReturnExpandsInTableLiteral(t *testing.T) {
	got, err := Transpile("mr-table.lua", `
function pair()
  return 10, 20
end

function foo()
  local t = {1, pair()}
  return t
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestTrailingMultiReturnTableLiteral(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := g.__glu_call_foo(&c)
	if n := c.TableLen(got); n != 3 {
		t.Fatalf("#t = %d, want 3 ({1, pair()} must expand pair()'s two results)", n)
	}
	if v := c.TableGetInt(got, 1).Int(); v != 1 {
		t.Fatalf("t[1] = %d, want 1", v)
	}
	if v := c.TableGetInt(got, 2).Int(); v != 10 {
		t.Fatalf("t[2] = %d, want 10 (pair()'s first result)", v)
	}
	if v := c.TableGetInt(got, 3).Int(); v != 20 {
		t.Fatalf("t[3] = %d, want 20 (pair()'s second result)", v)
	}
}
`
	runGeneratedHarness(t, got, harness, "TestTrailingMultiReturnTableLiteral")
}

func TestGeneratedBuiltinSurfaceCompiles(t *testing.T) {
	source := `
function _draw()
  cls(); pset(1, 2); pget(1, 2)
  line(1, 2, 3, 4); rect(1, 2, 3, 4); rectfill(1, 2, 3, 4)
  circ(4, 4, 2); circfill(4, 4, 2)
  spr(0, 0, 0); sspr(0, 0, 8, 8, 0, 0)
  map(); camera(); clip(); pal(); pal(0); palt(); color(); cursor()
  print("hello", 0, 0, 7); sget(0, 0); sset(0, 0, 1); fget(0); fset(0, 7); fset(0, 0, true)
  mget(0, 0); mget(1, 0, 0); mset(0, 0, 0); mset(1, 0, 0, 0)
  btn(0); btn(0, 0); btnp(0); btnp(0, 0)
  flr(1); ceil(1); abs(1); sgn(1); min(1, 2); max(1, 2); mid(1, 2, 3)
  sin(1); cos(1); atan2(1, 2); sqrt(1); rnd(); srand(1)
  band(1, 2); bor(1, 2); bxor(1, 2); bnot(1); shl(1, 2); shr(1, 2); lshr(1, 2); rotl(1, 2); rotr(1, 2)
  local items = {1, 2, name = "x"}; add(items, 3); del(items, 3); deli(items); count(items)
  sub("abc", 1); tostr(1); tonum("1"); chr(65); ord("a"); t(); time(); sfx(0); music(0); printh("x")
  for i = 1, 3 do pset(i, i) end
end
`
	got, err := Transpile("builtins.lua", source, Options{SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := format.Source([]byte(got)); err != nil {
		t.Fatalf("generated Go is not syntactically valid: %v", err)
	}
	if strings.Contains(got, "func()") {
		t.Error("generated source contains an immediately-invoked function literal; TinyGo -gc=none cannot link runtime.alloc")
	}
	compileGenerated(t, got)
}

func TestLayeredMapAccessUsesCompactFixedSlots(t *testing.T) {
	got, err := Transpile("layered-map-access.lua", `
function _update()
  local legacy = mget(4, 5)
  local explicit_bg0 = mget(0, 4, 5)
  local front = mget(2, 4, 5)
  mset(4, 5, 6)
  mset(0, 4, 5, 6)
  mset(2, 4, 5, 6)
end
`, Options{SheetW: 16, SheetH: 8, MapW: 8, MapH: 8, MapLayers: 4, Map: make([]uint16, 8*8*4)})
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(got, "c.MgetCompact(0,0,"); count != 2 {
		t.Fatalf("implicit/explicit BG0 compact mget count = %d, want 2:\n%s", count, got)
	}
	if count := strings.Count(got, "c.MgetCompact(2,1,"); count != 1 {
		t.Fatalf("BG2 fixed-slot mget count = %d, want 1:\n%s", count, got)
	}
	if count := strings.Count(got, "c.MsetCompact(0,0,"); count != 2 {
		t.Fatalf("implicit/explicit BG0 compact mset count = %d, want 2:\n%s", count, got)
	}
	if count := strings.Count(got, "c.MsetCompact(2,1,"); count != 1 {
		t.Fatalf("BG2 fixed-slot mset count = %d, want 1:\n%s", count, got)
	}
	if strings.Contains(got, "c.MgetLayer(") || strings.Contains(got, "c.MsetLayer(") {
		t.Fatalf("compile-time map layers retained runtime slot lookup:\n%s", got)
	}
	if update := generatedSection(got, "func (g *Game) gluUpdate("); strings.Contains(update, "rt.FromInt(0)") {
		t.Fatalf("literal BG0 operands were materialized as dead temporaries:\n%s", update)
	}
	compileGenerated(t, got)
}

func TestLayeredMapLiteralBG0FastPathUsesRuntimeFixedPointConversion(t *testing.T) {
	got, err := Transpile("layered-map-bg0-fixed.lua", `
function _update()
  local tile = mget(0.5, 4, 5)
  mset(0.5, 4, 5, tile)
end
`, Options{SheetW: 16, SheetH: 8, MapW: 8, MapH: 8, MapLayers: 4, Map: make([]uint16, 8*8*4)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "c.MgetLayer(") || strings.Contains(got, "c.MsetLayer(") {
		t.Fatalf("runtime-floored BG0 literal missed compatibility fast path:\n%s", got)
	}
	if !strings.Contains(got, "c.MgetCompact(0,0,") || !strings.Contains(got, "c.MsetCompact(0,0,") {
		t.Fatalf("runtime-floored BG0 literal did not use fixed-slot methods:\n%s", got)
	}
	compileGenerated(t, got)
}

func TestLayeredMapAccessArityValidation(t *testing.T) {
	for _, test := range []struct {
		name, call, want string
	}{
		{"mget-too-few", "mget(0)", "mget expects 2..3 arguments, got 1"},
		{"mget-too-many", "mget(0,0,0,0)", "mget expects 2..3 arguments, got 4"},
		{"mset-too-few", "mset(0,0)", "mset expects 3..4 arguments, got 2"},
		{"mset-too-many", "mset(0,0,0,0,0)", "mset expects 3..4 arguments, got 5"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Transpile(test.name+".lua", "function _update() "+test.call+" end", Options{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestGeneratedGameCompilesWithTinyGo(t *testing.T) {
	got, err := Transpile("tinygo.lua", `
function _draw()
  local t = {1, 2, name = "x"}
  if t[1] and t[2] then
    spr(0, 0, 0)
  end
  pal()
  palt()
  map()
  print("hi", 0, 0, 7)
  cls(0)
end
`, Options{SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	buildGeneratedWithTinyGo(t, got)
}

func TestGeneratedAudioGameCompilesWithTinyGoWithoutAllocations(t *testing.T) {
	options := nativeTranspilerOptions(t)
	got, err := Transpile("tinygo-audio.lua", `
function _init()
  music(0)
  sfx(0, 120)
  gba_note(1, 69, 0, 24)
end
function _update()
	if btnp(4) then sfx(0, 120) end
	if btnp(5) then gba_stop(1) end
end
function _draw()
  cls(0)
  print(stat(80), 0, 0, 7)
  print(stat(81), 0, 8, 7)
  print(stat(82), 0, 16, 7)
  print(stat(88), 0, 24, 7)
  print(stat(89), 0, 32, 7)
end
`, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"audio.MonoStream", "audio.Mixer", "audio.SFXPlayer", "DirectSoundRate", "samplePCMROM"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("generated PSG-only game contains legacy Direct Sound token %q", forbidden)
		}
	}
	buildGeneratedWithTinyGo(t, got)
}

func buildGeneratedWithTinyGo(t *testing.T, got string) {
	t.Helper()
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo is not installed")
	}
	dir := writeGeneratedPackage(t, got)
	target, err := filepath.Abs(filepath.Join("..", "..", "runtime", "target-gba.json"))
	if err != nil {
		t.Fatal(err)
	}
	outROM := filepath.Join(dir, "generated.gba")
	cmd := exec.Command("tinygo", "build", "-target", target, "-gc=none", "-opt=2", "-print-allocs-cover=.", "-o", outROM, ".")
	cmd.Dir = dir
	cmd.Env = generatedGoCacheEnv()
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		if strings.Contains(out.String(), "operation not permitted") && strings.Contains(out.String(), "tinygo") {
			t.Skipf("TinyGo compiler cache is outside the writable test sandbox: %s", out.String())
		}
		t.Fatalf("generated package does not compile with TinyGo: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "mode: set\n") {
		t.Fatalf("TinyGo did not emit an allocation coverage report:\n%s", out.String())
	}
	if allocs := nonZeroTinyGoAllocations(out.String()); len(allocs) != 0 {
		t.Fatalf("generated TinyGo package has target heap allocations:\n%s", strings.Join(allocs, "\n"))
	}
	if info, err := os.Stat(outROM); err != nil || info.Size() == 0 {
		t.Fatalf("TinyGo did not produce a ROM: %v", err)
	}
}

func nonZeroTinyGoAllocations(output string) []string {
	var allocs []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || !strings.Contains(fields[0], ".go:") {
			continue
		}
		count, countErr := strconv.Atoi(fields[len(fields)-1])
		if _, statementsErr := strconv.Atoi(fields[len(fields)-2]); countErr != nil || statementsErr != nil {
			continue
		}
		if count != 0 {
			allocs = append(allocs, strings.TrimSpace(line))
		}
	}
	return allocs
}

func TestNonZeroTinyGoAllocations(t *testing.T) {
	output := "mode: set\n/a/runtime/audio/mixer.go:1.1,2.2 1 0\n/a/generated/main.go:3.1,4.2 1 2\ncompiler note\n"
	got := nonZeroTinyGoAllocations(output)
	if len(got) != 1 || !strings.Contains(got[0], "generated/main.go") {
		t.Fatalf("nonZeroTinyGoAllocations() = %#v", got)
	}
}

const generatedAssetInitHarness = `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestGeneratedAssetInitSucceeds(t *testing.T) {
	cartImage := rt.AssetImage{
		SheetW: SheetW, SheetH: SheetH, SpriteCols: SpriteCols, SpriteRows: SpriteRows, SpriteN: SpriteN,
		MapW: MapW, MapH: MapH, SheetReadOnly: SheetReadOnly, Sheet: sheetROM[:], SheetIndex: sheetIndexROM[:], Map: mapROM[:], Flags: flagROM[:],
	}
	cartData.SheetW, cartData.SheetH = SheetW, SheetH
	cartData.SpriteCols, cartData.SpriteRows, cartData.SpriteN = SpriteCols, SpriteRows, SpriteN
	cartData.MapW, cartData.MapH = MapW, MapH
	cartData.SheetReadOnly = SheetReadOnly
	cartData.Sheet, cartData.SheetIndex, cartData.Map, cartData.Flags = sheetRAM[:], sheetIndexRAM[:], mapRAM[:], flagRAM[:]
	if !ctx.Init(&cartImage, &cartData) {
		t.Fatalf("Init failed: SpriteN=%d sheetROM=%d flagROM=%d flagRAM=%d mapROM=%d", SpriteN, len(sheetROM), len(flagROM), len(flagRAM), len(mapROM))
	}
	if len(flagROM) != SpriteN || len(flagRAM) != SpriteN {
		t.Fatalf("flag arrays are %d/%d, want SpriteN=%d", len(flagROM), len(flagRAM), SpriteN)
	}
}
`

func TestTranspileLeftoverHeightFlagsMatchSpriteN(t *testing.T) {
	got, err := Transpile("leftover-h.lua", `
function _draw()
  cls(0)
end
`, Options{SheetW: 64, SheetH: 9, MapW: 1, MapH: 1, Flags: make([]byte, 8)})
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := format.Source([]byte(got))
	if err != nil {
		t.Fatalf("generated Go is not syntactically valid: %v\n%s", err, got)
	}
	src := string(formatted)
	if !strings.Contains(src, "var flagROM = [8]byte") {
		t.Fatalf("flagROM length is not SpriteN=8:\n%s", src)
	}
	if !strings.Contains(src, "var flagRAM [8]byte") {
		t.Fatalf("flagRAM length is not SpriteN=8:\n%s", src)
	}
	if strings.Contains(src, "var flagROM = [9]byte") || strings.Contains(src, "var flagRAM [9]byte") {
		t.Fatal("emitted leftover-height flags with W/8*H/8 instead of SpriteN")
	}
	runGeneratedHarness(t, src, generatedAssetInitHarness, "TestGeneratedAssetInitSucceeds")
}

func TestTranspileAcceptsLeftoverEdgeSpriteNZero(t *testing.T) {
	got, err := Transpile("leftover-edge.lua", `
function _draw()
  cls(0)
end
`, Options{SheetW: 7, SheetH: 8, MapW: 0, MapH: 0})
	if err != nil {
		t.Fatalf("leftover-edge SpriteN==0 sheet was rejected: %v", err)
	}
	formatted, err := format.Source([]byte(got))
	if err != nil {
		t.Fatalf("generated Go is not syntactically valid: %v\n%s", err, got)
	}
	src := string(formatted)
	if !strings.Contains(src, "var flagROM = [0]byte") || !strings.Contains(src, "var flagRAM [0]byte") {
		t.Fatalf("SpriteN==0 must emit empty flag arrays:\n%s", src)
	}
	runGeneratedHarness(t, src, generatedAssetInitHarness, "TestGeneratedAssetInitSucceeds")
}

func TestForeachCallsNamedFunctionAndBuiltinInOrder(t *testing.T) {
	got, err := Transpile("foreach.lua", `
seen = 0
calls = 0

function scores()
  calls += 1
  return {9, 4, 1}
end

function remember(value)
  seen = seen * 10 + value
end

function _init()
  foreach(scores(), remember)
  foreach({1, 2, 3}, print)
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	"testing"
	rt "github.com/drpaneas/mtile/runtime"
)

func TestGeneratedForeach(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	if got := g.calls.Int(); got != 1 {
		t.Fatalf("foreach table expression evaluated %d times, want 1", got)
	}
	if got := g.seen.Int(); got != 941 {
		t.Fatalf("foreach callback order/result = %d, want 941", got)
	}
}
`
	runGeneratedHarness(t, got, harness, "TestGeneratedForeach")
}

func TestForeachRejectsUnknownCallback(t *testing.T) {
	_, err := Transpile("foreach.lua", `
function _init()
  foreach({1}, missing_callback)
end
`, Options{})
	if err == nil || !strings.Contains(err.Error(), "unsupported call") {
		t.Fatalf("Transpile error = %v, want unsupported callback diagnostic", err)
	}
}

func TestTranspileSelectsROMOrRAMSheetFromSsetUsage(t *testing.T) {
	sheet := make([]byte, 240*160)
	sheet[8] = 7 // one authored non-empty 8x8 cell (ID 1)
	immutable, immutableReport, err := TranspileWithReport("immutable.lua", `
function _draw()
  spr(1, 0, 0)
end
`, Options{SheetW: 240, SheetH: 160, Sheet: sheet})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(immutable, "SheetReadOnly = true") ||
		!strings.Contains(immutable, "var sheetRAM [64]byte") ||
		!strings.Contains(immutable, "var sheetIndexRAM [600]uint16") ||
		strings.Contains(immutable, "var sheetRAM [38400]byte") {
		t.Fatalf("immutable source reserved a writable sheet:\n%s", immutable[:min(len(immutable), 4000)])
	}
	if !immutableReport.SheetReadOnly {
		t.Fatal("immutable source report did not select read-only sheet specialization")
	}

	mutable, mutableReport, err := TranspileWithReport("mutable.lua", `
function paint()
  sset(239, 159, 7)
end
function _init()
  paint()
end
`, Options{SheetW: 240, SheetH: 160, Sheet: sheet})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mutable, "SheetReadOnly = false") ||
		!strings.Contains(mutable, "var sheetRAM [38400]byte") {
		t.Fatalf("sset source did not reserve a writable sheet:\n%s", mutable[:min(len(mutable), 4000)])
	}
	if mutableReport.SheetReadOnly {
		t.Fatal("sset source incorrectly selected read-only sheet specialization")
	}
}

func TestTranspileLargeMapUsesStreamingCache(t *testing.T) {
	mapData := make([]uint16, 192*96)
	for y := 0; y < 96; y++ {
		for x := 0; x < 192; x++ {
			mapData[y*192+x] = uint16((x + y) & 1)
		}
	}
	got, report, err := TranspileWithReport("large-map.lua", `
function _draw()
  map(0, 31, 0, 0, 0, 2, 1, 0)
end
`, Options{
		SheetW: 16, SheetH: 8, MapW: 192, MapH: 96,
		Sheet: make([]byte, 16*8), Map: mapData, Flags: []byte{0, 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.MapStreaming {
		t.Fatalf("large retained map was not reported as streaming: %+v", report)
	}
	src, err := format.Source([]byte(got))
	if err != nil {
		t.Fatalf("large-map generated Go is not syntactically valid: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "MapStreaming") || !strings.Contains(text, "= true") ||
		!strings.Contains(text, "var mapRAM [rt.MapCacheCells]uint16") ||
		!strings.Contains(text, "var mapChunksRAM [rt.MapCacheSlots]rt.MapChunk") ||
		!strings.Contains(text, "var mapEditsRAM [rt.MapEditCapacity]rt.MapEdit") {
		t.Fatalf("large map did not emit streaming storage:\n%s", text[:min(len(text), 4000)])
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestGeneratedLargeMapInitAndEdits(t *testing.T) {
	cartData.SheetW, cartData.SheetH = SheetW, SheetH
	cartData.SpriteCols, cartData.SpriteRows, cartData.SpriteN = SpriteCols, SpriteRows, SpriteN
	cartData.MapW, cartData.MapH, cartData.MapStreaming = MapW, MapH, MapStreaming
	cartData.SheetReadOnly = SheetReadOnly
	cartData.Sheet, cartData.SheetIndex, cartData.Map, cartData.Flags = sheetRAM[:], sheetIndexRAM[:], mapRAM[:], flagRAM[:]
	cartData.MapChunks, cartData.MapEdits = mapChunksRAM[:], mapEditsRAM[:]
	if !ctx.Init(&rt.AssetImage{
		SheetW: SheetW, SheetH: SheetH, SpriteCols: SpriteCols, SpriteRows: SpriteRows,
		MapW: MapW, MapH: MapH, SpriteN: SpriteN, MapStreaming: MapStreaming,
		SheetReadOnly: SheetReadOnly, Sheet: sheetROM[:], SheetIndex: sheetIndexROM[:], Map: mapROM[:], Flags: flagROM[:],
	}, &cartData) {
		t.Fatal("streaming generated asset Init failed")
	}
	if got := ctx.Mget(191, 95); got != mapROM[95*MapW+191] {
		t.Fatalf("far-edge Mget = %d", got)
	}
	ctx.Mset(31, 31, 1)
	for _, p := range [][2]int{{0, 0}, {32, 0}, {64, 0}, {0, 32}, {32, 32}, {64, 32}} {
		_ = ctx.Mget(p[0], p[1])
	}
	if got := ctx.Mget(31, 31); got != 1 {
		t.Fatalf("streaming Mset was lost after eviction: %d", got)
	}
}
`
	runGeneratedHarness(t, string(src), harness, "TestGeneratedLargeMapInitAndEdits")
}

func TestTranspileSmallMapReportsCompactStorage(t *testing.T) {
	_, report, err := TranspileWithReport("compact-map.lua", `
function _draw()
  map(0,0,0,0,0,30,20,0)
end
`, Options{MapW: 30, MapH: 20, Map: make([]uint16, 30*20)})
	if err != nil {
		t.Fatal(err)
	}
	if report.MapStreaming {
		t.Fatalf("small retained map was reported as streaming: %+v", report)
	}
}

func TestStaticTableFieldsUseStableCachesByDefault(t *testing.T) {
	source := `
local player = {x = 1}
function _update()
  player.x = player.x + 1
end
`
	optimized, err := Transpile("stable-fields.lua", source, Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	optimizedGame := generatedSection(optimized, "type Game struct {")
	optimizedUpdate := generatedSection(optimized, "func (g *Game) gluUpdate(")
	if !strings.Contains(optimizedGame, "rt.TableFieldCache") ||
		!strings.Contains(optimizedUpdate, "c.TableGetStable(") ||
		!strings.Contains(optimizedUpdate, "gluTableSetStable(") {
		t.Fatalf("default lowering did not emit stable field caches:\n%s", optimized)
	}

	legacy, err := Transpile("dynamic-fields.lua", source, Options{DisableStableTableFields: true})
	if err != nil {
		t.Fatal(err)
	}
	legacyGame := generatedSection(legacy, "type Game struct {")
	legacyUpdate := generatedSection(legacy, "func (g *Game) gluUpdate(")
	if strings.Contains(legacyGame, "rt.TableFieldCache") ||
		strings.Contains(legacyUpdate, "c.TableGetStable(") ||
		strings.Contains(legacyUpdate, "gluTableSetStable(") {
		t.Fatalf("legacy lowering retained stable field caches:\n%s", legacy)
	}
	if !strings.Contains(legacyUpdate, "c.TableGet(") || !strings.Contains(legacyUpdate, "gluTableSet(") {
		t.Fatalf("legacy lowering did not retain dynamic field operations:\n%s", legacy)
	}
}

func TestMonomorphicNumbersLowerToNativeStorageByDefault(t *testing.T) {
	source := `
local gravity = 0.25
local steps = 0
local body = {x = 1, vx = 0.5}
function _update()
  local delta = body.vx + gravity
  body.x = body.x + delta
  steps = steps + 1
  for i = 1, 3 do
    body.x = body.x + i * 0.125
  end
end
`
	optimized, err := Transpile("mono-numbers.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	optimizedUpdate := generatedSection(optimized, "func (g *Game) gluUpdate(")
	if strings.Contains(optimized, "gravity rt.Number") {
		t.Fatalf("immutable gravity constant retained runtime storage:\n%s", optimized)
	}
	for _, want := range []string{
		"steps rt.Number",
	} {
		if !strings.Contains(optimized, want) {
			t.Fatalf("monomorphic lowering is missing %q:\n%s", want, optimized)
		}
	}
	for _, want := range []string{"_gluRecord0Field0 rt.Number", "_gluRecord0Field1 rt.Number", "var l_delta_1 rt.Number", "var l_i_2 rt.Number"} {
		if !strings.Contains(optimizedUpdate, want) {
			if strings.Contains(want, " rt.Number") && strings.Contains(optimized, want) {
				continue
			}
			t.Fatalf("monomorphic update lowering is missing %q:\n%s", want, optimizedUpdate)
		}
	}
	if strings.Contains(optimizedUpdate, "TableGet") || strings.Contains(optimizedUpdate, "TableSet") {
		t.Fatalf("non-escaping numeric record retained table access:\n%s", optimizedUpdate)
	}

	legacy, err := Transpile("tagged-numbers.lua", source, Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	legacyUpdate := generatedSection(legacy, "func (g *Game) gluUpdate(")
	if strings.Contains(legacyUpdate, "TableGetStableNumber") || strings.Contains(legacyUpdate, "TableSetStableNumber") ||
		strings.Contains(legacy, "gravity rt.Number") || strings.Contains(legacy, "steps rt.Number") {
		t.Fatalf("numeric rollback path emitted monomorphic storage:\n%s", legacy)
	}
}

func TestMonomorphicTableFieldsPreserveNumericGlobalInitializers(t *testing.T) {
	generated, err := Transpile("numeric-record-constants.lua", `
local spawn_x = 16
local spawn_y = 24
player = {x = spawn_x, y = (spawn_y)}
function _update()
  player.x = player.x + 1
  player.y = player.y - 1
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	update := generatedSection(generated, "func (g *Game) gluUpdate(")
	for _, want := range []string{"_gluRecord0Field0 rt.Number", "_gluRecord0Field1 rt.Number"} {
		if !strings.Contains(generated, want) {
			t.Fatalf("numeric record storage is missing %q:\n%s", want, generated)
		}
	}
	if got := strings.Count(update, "g._gluRecord0Field"); got != 4 {
		t.Fatalf("direct numeric record accesses = %d, want 4:\n%s", got, update)
	}
	if strings.Contains(update, "TableGet") || strings.Contains(update, "TableSet") {
		t.Fatalf("numeric record constants retained table access:\n%s", update)
	}
	compileGenerated(t, generated)
}

func TestMonomorphicGlobalsPreservePriorNumericAliases(t *testing.T) {
	generated, err := Transpile("numeric-global-alias.lua", `
local PLAYER_IDLE = 1
local PLAYER_RUN = 2
player_sprite = PLAYER_IDLE
function _update()
  if btn(0) then player_sprite = PLAYER_RUN end
end
function _draw()
  cls(0)
  spr(player_sprite,16,24)
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	game := generatedSection(generated, "type Game struct {")
	if !strings.Contains(game, "player_sprite rt.Number") || strings.Contains(game, "player_sprite rt.Value") {
		t.Fatalf("prior numeric alias did not preserve Number storage:\n%s", game)
	}
	compileGenerated(t, generated)
}

func TestHeapElisionRequiresAClosedAllocationFreeProof(t *testing.T) {
	heapless := `
local frame = 1
player = {x=16,y=24}
function move(dx)
  player.x = player.x + dx
end
function _update()
  if btn(0) then move(-1) end
end
function _draw()
  cls(0)
  spr(frame,player.x,player.y)
end
`
	generated, report, err := TranspileWithReport("heapless.lua", heapless, Options{Debug: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.HeapLinked || strings.Contains(generated, "ctx.CollectIfNeeded()") {
		t.Fatalf("allocation-free cartridge retained periodic collection:\n%s", generated)
	}
	if !strings.Contains(generated, "heaplessStats.HeapCapacity, heaplessStats.HeapHandleLimit = 0, 0") {
		t.Fatalf("heapless debug telemetry reports nonexistent heap capacity:\n%s", generated)
	}
	compileGenerated(t, generated)

	for _, test := range []struct {
		name    string
		source  string
		options Options
	}{
		{name: "ordinary-table", source: `items={1}\nfunction _update() add(items,2) end`},
		{name: "dynamic-string", source: `message=0\nfunction _update() message=tostr(7) end`},
		{name: "boxed-global", source: `message="static"\nfunction _draw() cls(0) end`},
		{name: "rollback", source: heapless, options: Options{DisableHeapElision: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			generated, report, err := TranspileWithReport(test.name+".lua", strings.ReplaceAll(test.source, `\n`, "\n"), test.options)
			if err != nil {
				t.Fatal(err)
			}
			if !report.HeapLinked || !strings.Contains(generated, "ctx.CollectIfNeeded()") {
				t.Fatalf("unproven heap program elided collection:\n%s", generated)
			}
			compileGenerated(t, generated)
		})
	}
}

func TestAudioControlBuiltinsDoNotForceLuaHeap(t *testing.T) {
	for _, name := range []string{"sfx", "music", "sample", "gba_note", "gba_stop"} {
		if !builtinNoAlloc(name) {
			t.Fatalf("%s should be classified as Lua-heap-free", name)
		}
	}
	for _, name := range []string{"print", "sub", "tostr", "chr", "add"} {
		if builtinNoAlloc(name) {
			t.Fatalf("%s may allocate from the Lua heap and must remain conservative", name)
		}
	}

	generated, report, err := TranspileWithReport("heapless-audio-control.lua", `
frame=0
function _init()
  sfx(-1)
  music(-1)
  sample(-1)
end
function _update()
  frame=frame+1
end
function _draw()
  cls(0)
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.HeapLinked || strings.Contains(generated, "ctx.CollectIfNeeded()") {
		t.Fatalf("allocation-free audio controls retained Lua heap maintenance:\n%s", generated)
	}
	compileGenerated(t, generated)
}

func TestLiteralPrintDoesNotForceLuaHeap(t *testing.T) {
	heapless, report, err := TranspileWithReport("heapless-print.lua", `
function _draw()
  cls(0)
  print(("A"),8,8,7)
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.HeapLinked || strings.Contains(heapless, "ctx.CollectIfNeeded()") {
		t.Fatalf("literal print retained Lua heap maintenance:\n%s", heapless)
	}
	if !strings.Contains(heapless, "c.PrintStaticV(") || strings.Contains(generatedSection(heapless, "func (g *Game) gluDraw("), "gluPrintV(") {
		t.Fatalf("literal print retained a root-protected call:\n%s", heapless)
	}
	runGeneratedHarness(t, heapless, `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestLiteralPrintDrawsWithoutHeap(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	g.Draw(&c)
	if got := c.Pget(10, 8); got != 7 {
		t.Fatalf("literal print pixel = %d, want 7", got)
	}
}
`, "TestLiteralPrintDrawsWithoutHeap")

	rollback, rollbackReport, err := TranspileWithReport("generic-print.lua", `
function _draw()
  cls(0)
  print("A",8,8,7)
end
`, Options{DisableLiteralPrintSpecialization: true})
	if err != nil {
		t.Fatal(err)
	}
	if !rollbackReport.HeapLinked || !strings.Contains(generatedSection(rollback, "func (g *Game) gluDraw("), "gluPrintV(") {
		t.Fatalf("literal print rollback did not retain generic rooted path:\n%s", rollback)
	}
	compileGenerated(t, rollback)

	for _, source := range []string{
		`function _draw() print(42,8,8,7) end`,
		`value="STATIC" function _draw() print(value,8,8,7) end`,
	} {
		generated, report, err := TranspileWithReport("dynamic-print.lua", source, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if !report.HeapLinked || !strings.Contains(generated, "ctx.CollectIfNeeded()") {
			t.Fatalf("unproven print incorrectly elided Lua heap:\n%s", generated)
		}
		compileGenerated(t, generated)
	}
}

func TestNonGrowingTableBuiltinsAreNoAlloc(t *testing.T) {
	for _, name := range []string{"del", "deli", "count", "all", "pairs"} {
		if !builtinNoAlloc(name) {
			t.Fatalf("%s should be classified as Lua-heap-free", name)
		}
	}
	for _, name := range []string{"add", "foreach"} {
		if builtinNoAlloc(name) {
			t.Fatalf("%s may allocate or invoke an allocating callback", name)
		}
	}

	generated, err := Transpile("table-effects.lua", `
items={1,2,3}
function take_last()
  return deli(items)
end
function measure()
  return count(items)
end
function _update()
  local value=take_last()
  local remaining=measure()
  add(items,value)
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	update := generatedSection(generated, "func (g *Game) gluUpdate(")
	if !strings.Contains(update, "g.take_last(") || strings.Contains(update, "g.__glu_call_take_last(") ||
		!strings.Contains(update, "g.measure(") || strings.Contains(update, "g.__glu_call_measure(") {
		t.Fatalf("non-growing table helpers retained root-protected wrappers:\n%s", update)
	}
	compileGenerated(t, generated)
}

func TestNumericRecordScalarReplacementFallsBackForObservableTables(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{"alias", `local player={x=1}\nlocal alias=player\nfunction _update() alias.x=2 end`},
		{"dynamic-key", `local player={x=1}\nlocal key="x"\nfunction _update() player[key]=2 end`},
		{"rebind", `local player={x=1}\nfunction _update() player={x=2} end`},
		{"new-field", `local player={x=1}\nfunction _update() player.y=2 end`},
		{"nonnumeric-write", `local player={x=1}\nfunction _update() player.x="bad" end`},
		{"shadow", `local player={x=1}\nfunction move(player) return player.x end`},
		{"mixed-record", `local player={x=1,name="madeline"}\nfunction _update() player.x=2 end`},
		{"read-before-init", `local seen=player.x\nplayer={x=1}\nfunction _update() player.x=2 end`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generated, err := Transpile(test.name+".lua", strings.ReplaceAll(test.source, `\n`, "\n"), Options{})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(generated, "_gluRecord0Field0 rt.Number") {
				t.Fatalf("observable table was scalar-replaced:\n%s", generated)
			}
			if !strings.Contains(generated, "player rt.Value") {
				t.Fatalf("observable table did not retain ordinary storage:\n%s", generated)
			}
			compileGenerated(t, generated)
		})
	}
}

func TestGeneratedMonomorphicNumbersPreserveFixedPointResults(t *testing.T) {
	generated, err := Transpile("mono-results.lua", `
local body = {x = 1.25, vx = 0.5}
local total = 0
function _update()
  local acceleration = 0.125
  for i = 1, 4 do
    body.vx = body.vx + acceleration
    body.x = body.x + body.vx * i
  end
  total = total + body.x
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
  rt "github.com/drpaneas/mtile/runtime"
  "testing"
)

func TestMonomorphicResults(t *testing.T) {
  var g Game
  var c rt.Context
  g.Init(&c)
  x, vx, total := rt.Number(81920), rt.Number(32768), rt.Number(0)
  acceleration := rt.Number(8192)
  for tick := 0; tick < 12; tick++ {
    g.Update(&c)
    for i := 1; i <= 4; i++ {
      vx += acceleration
      x += vx.Mul(rt.FromInt(i))
    }
    total += x
    if got := g._gluRecord0Field0; got != x {
      t.Fatalf("tick %d x=%d want=%d", tick, got, x)
    }
    if got := g._gluRecord0Field1; got != vx {
      t.Fatalf("tick %d vx=%d want=%d", tick, got, vx)
    }
    if g.total != total {
      t.Fatalf("tick %d total=%d want=%d", tick, g.total, total)
    }
  }
}
`
	runGeneratedHarness(t, generated, harness, "TestMonomorphicResults")
}

func TestMonomorphicFunctionSignaturesRequireConsistentCallSitesAndReturns(t *testing.T) {
	source := `
local total = 0
function square(value)
  return value * value
end
function length_squared(x, y)
  return square(x) + square(y)
end
function _update()
  total = total + length_squared(total, 0.5)
end
`
	optimized, err := Transpile("mono-functions.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func (g *Game) square(c *rt.Context, a0 rt.Number) rt.Number",
		"func (g *Game) length_squared(c *rt.Context, a0 rt.Number, a1 rt.Number) rt.Number",
		"func (g *Game) __glu_call_square(c *rt.Context, a0 rt.Number, roots ...rt.Value) rt.Number",
	} {
		if !strings.Contains(optimized, want) {
			t.Fatalf("numeric function lowering is missing %q:\n%s", want, optimized)
		}
	}

	legacy, err := Transpile("tagged-functions.lua", source, Options{DisableMonomorphicFunctions: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(legacy, "func (g *Game) square(c *rt.Context, a0 rt.Number)") ||
		strings.Contains(legacy, "func (g *Game) length_squared(c *rt.Context, a0 rt.Number") {
		t.Fatalf("function rollback path retained a numeric signature:\n%s", legacy)
	}
	if !strings.Contains(legacy, "func (g *Game) square(c *rt.Context, a0 rt.Value) rt.Value") {
		t.Fatalf("function rollback path did not retain the tagged ABI:\n%s", legacy)
	}
}

func TestMonomorphicFunctionSignaturesStayTaggedForMixedOrFallingThroughFunctions(t *testing.T) {
	generated, err := Transpile("mixed-functions.lua", `
function identity(value)
  return value
end
function maybe_number(value)
  if value > 0 then
    return value
  end
end
local numeric = identity(1)
local text = identity("hello")
local maybe = maybe_number(1)
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func (g *Game) identity(c *rt.Context, a0 rt.Value) rt.Value",
		"func (g *Game) maybe_number(c *rt.Context, a0 rt.Number) rt.Value",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("conservative function lowering is missing %q:\n%s", want, generated)
		}
	}
}

func TestGeneratedMonomorphicFunctionPreservesFixedPointResults(t *testing.T) {
	generated, err := Transpile("mono-function-results.lua", `
local total = 0
function scale_add(x, y, scale)
  return x + y * scale
end
function _update()
  total = scale_add(total, 0.5, 2)
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
  rt "github.com/drpaneas/mtile/runtime"
  "testing"
)

func TestMonomorphicFunctionResults(t *testing.T) {
  var g Game
  var c rt.Context
  g.Init(&c)
  for tick := 1; tick <= 32; tick++ {
    g.Update(&c)
    if want := rt.FromInt(tick); g.total != want {
      t.Fatalf("tick %d total=%d want=%d", tick, g.total, want)
    }
  }
}
`
	runGeneratedHarness(t, generated, harness, "TestMonomorphicFunctionResults")
}

func TestMonomorphicMultiReturnSignaturesSpecializeEachProvenPosition(t *testing.T) {
	source := `
local total = 0
function pair(x, y)
  return x + 1, y + 2
end
function forward_pair(x, y)
  return x + 3, y + 4
end
function forward(x, y)
  return forward_pair(x, y)
end
function mixed(x)
  if x > 0 then
    return x, "positive", x + 2
  else
    return x + 1, "other", x + 3
  end
end
function padded(x)
  if x > 0 then
    return x, x + 1
  else
    return x
  end
end
function single(x)
  return x + 1
end
function _update()
  local a, b = pair(total, 1)
  local c, d = forward(total, 2)
  local n, label, m = mixed(total)
  local p, maybe = padded(total)
  local first = pair(total, 3)
  total = first + b + c + d + n + m + p + single(1)
end
`
	generated, err := Transpile("mono-multi-return.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func (g *Game) pair(c *rt.Context, a0 rt.Number, a1 rt.Number) (rt.Number, rt.Number)",
		"func (g *Game) forward_pair(c *rt.Context, a0 rt.Number, a1 rt.Number) (rt.Number, rt.Number)",
		"func (g *Game) forward(c *rt.Context, a0 rt.Number, a1 rt.Number) (rt.Number, rt.Number)",
		"func (g *Game) mixed(c *rt.Context, a0 rt.Number) (rt.Number, rt.Value, rt.Number)",
		"func (g *Game) padded(c *rt.Context, a0 rt.Number) (rt.Number, rt.Value)",
		"func (g *Game) __glu_multi_pair(c *rt.Context, a0 rt.Number, a1 rt.Number, roots ...rt.Value) (rt.Number, rt.Number)",
		"func (g *Game) __glu_first_pair(c *rt.Context, a0 rt.Number, a1 rt.Number, roots ...rt.Value) rt.Number",
		"var l_a_1 rt.Number",
		"var l_b_2 rt.Number",
		"var l_label_6 rt.Value",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("multi-return specialization is missing %q:\n%s", want, generated)
		}
	}

	rollback, err := Transpile("tagged-multi-return.lua", source, Options{DisableMonomorphicMultiReturns: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rollback, "func (g *Game) pair(c *rt.Context, a0 rt.Number, a1 rt.Number) (rt.Value, rt.Value)") {
		t.Fatalf("multi-return rollback did not retain tagged results:\n%s", rollback)
	}
	if !strings.Contains(rollback, "func (g *Game) single(c *rt.Context, a0 rt.Number) rt.Number") {
		t.Fatalf("multi-return rollback disabled single-return specialization:\n%s", rollback)
	}
}

func TestNumericArraysUseCompactNativeAccessors(t *testing.T) {
	source := `
local samples = {1, 2, 3, 4}
local total = 0

function _update()
  for i = 1, #samples do
    samples[i] = samples[i] + i
  end
  total = samples[1] + samples[4]
end
`
	generated, err := Transpile("numeric-array.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"c.NewNumberArray(4)",
		"c.NumberArrayGet(g.samples",
		"c.NumberArraySet(g.samples",
		"rt.FromInt(4)",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("numeric-array lowering is missing %q:\n%s", want, generated)
		}
	}
	if strings.Contains(generated, "c.NewTableWithCaps(4") {
		t.Fatalf("numeric-array literal retained generic table allocation:\n%s", generated)
	}

	harness := `package main

import (
  rt "github.com/drpaneas/mtile/runtime"
  "testing"
)

func TestNumericArrayProgram(t *testing.T) {
  var g Game
  var c rt.Context
  g.Init(&c)
  g.Update(&c)
  want := []int{2, 4, 6, 8}
  for i, value := range want {
    if got := c.NumberArrayGet(g.samples, i+1).Int(); got != value {
      t.Fatalf("samples[%d] = %d, want %d", i+1, got, value)
    }
  }
  if got := g.total.Int(); got != 10 {
    t.Fatalf("total = %d, want 10", got)
  }
}
`
	runGeneratedHarness(t, generated, harness, "TestNumericArrayProgram")
}

func TestNumericArraysFallbackOnEscapeShadowTypeChangeOrDisabledOption(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		options Options
	}{
		{
			name: "escape",
			source: `
local samples = {1, 2, 3}
function first(values) return values[1] end
function _update() first(samples) end
`,
			options: Options{},
		},
		{
			name: "shadow",
			source: `
local samples = {1, 2, 3}
function first()
  local samples = {4, 5, 6}
  return samples[1]
end
`,
			options: Options{},
		},
		{
			name: "type-change",
			source: `
local samples = {1, 2, 3}
function _update() samples[1] = "changed" end
`,
			options: Options{},
		},
		{
			name: "disabled",
			source: `
local samples = {1, 2, 3}
function _update() samples[1] = samples[1] + 1 end
`,
			options: Options{DisableNumericArrays: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generated, err := Transpile(test.name+".lua", test.source, test.options)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(generated, "NewNumberArray") || strings.Contains(generated, "NumberArrayGet") || strings.Contains(generated, "NumberArraySet") {
				t.Fatalf("unsafe/disabled array was specialized:\n%s", generated)
			}
			if !strings.Contains(generated, "NewTableWithCaps(3") {
				t.Fatalf("unsafe/disabled array did not retain generic table storage:\n%s", generated)
			}
		})
	}
}

func TestGeneratedMonomorphicMultiReturnsPreserveExpansionAndMixedValues(t *testing.T) {
	generated, err := Transpile("mono-multi-results.lua", `
local total = 0
local text = ""
function pair(x, y)
  return x + 0.5, y + 1.25
end
function forward_pair(x, y)
  return x + 0.5, y + 1.25
end
function forward(x, y)
  return forward_pair(x, y)
end
function mixed(x)
  return x + 2, "ok", x + 3
end
function padded(x)
  if x > 0 then
    return x + 4, x + 5
  end
  return x + 6
end
function _update()
  local a, b = forward(total, 1)
  local c, label, d = mixed(total)
  local e, missing = padded(-1)
  local first = pair(total, 3)
  local values = {10, pair(2, 3)}
  local mixed_values = {mixed(1)}
  total = first + b + c + d + e + values[2] + values[3] + mixed_values[1] + mixed_values[3]
  if missing == nil then
    text = mixed_values[2]
  end
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
  rt "github.com/drpaneas/mtile/runtime"
  "testing"
)

func TestMonomorphicMultiReturnResults(t *testing.T) {
  var g Game
  var c rt.Context
  g.Init(&c)
  g.Update(&c)
	if want := rt.Number(1736704); g.total != want {
    t.Fatalf("total=%d want=%d", g.total, want)
  }
	if !c.ValuesEqual(g.text, c.InternStatic("ok")) {
		t.Fatalf("text did not preserve mixed static-string result: %+v", g.text)
  }
}
`
	runGeneratedHarness(t, generated, harness, "TestMonomorphicMultiReturnResults")
}

func TestNoAllocFunctionsBypassRootWrappersTransitively(t *testing.T) {
	source := `
local state = {value = 1}
function leaf(value)
  return abs(value) + 1
end
function chain(value)
  return leaf(value)
end
function allocating(value)
  local temporary = {value}
  return value
end
function through_allocating(value)
  return allocating(value)
end
function _update()
  state.value = chain(state.value)
  state.value = through_allocating(state.value)
end
`
	generated, err := Transpile("noalloc-functions.lua", source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	functionBody := func(signature string) string {
		start := strings.Index(generated, signature)
		if start < 0 {
			t.Fatalf("generated source is missing %q:\n%s", signature, generated)
		}
		rest := generated[start:]
		end := strings.Index(rest, "\n}\n")
		if end < 0 {
			t.Fatalf("generated function %q has no end", signature)
		}
		return rest[:end]
	}
	if body := functionBody("func (g *Game) chain("); !strings.Contains(body, "return g.leaf(") || strings.Contains(body, "__glu_call_leaf") {
		t.Fatalf("transitively safe call retained root wrapper:\n%s", body)
	}
	if body := functionBody("func (g *Game) gluUpdate("); !strings.Contains(body, "g.chain(") || strings.Contains(body, "__glu_call_chain") {
		t.Fatalf("safe top-level call retained root wrapper:\n%s", body)
	}
	if body := functionBody("func (g *Game) through_allocating("); !strings.Contains(body, "g.__glu_call_allocating(") {
		t.Fatalf("transitive allocator bypassed root protection:\n%s", body)
	}
	rollback, err := Transpile("wrapped-functions.lua", source, Options{DisableNoAllocFunctionCalls: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rollback, "return g.__glu_call_leaf(") || !strings.Contains(rollback, "g.__glu_call_chain(") {
		t.Fatalf("no-alloc rollback path did not retain root wrappers:\n%s", rollback)
	}
}

func TestNoAllocAnalysisRejectsStringAndTableAllocation(t *testing.T) {
	generated, err := Transpile("allocating-functions.lua", `
function stringify(value)
  return tostr(value)
end
function concatenate(value)
  return "value:" .. value
end
function store(table_value, value)
  table_value.dynamic = value
end
function _update()
  stringify(1)
  concatenate(2)
  store({}, 3)
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stringify", "concatenate", "store"} {
		if !strings.Contains(generated, "g.__glu_call_"+name+"(") {
			t.Fatalf("allocating function %s bypassed its root wrapper:\n%s", name, generated)
		}
	}
}

func TestMonomorphicNumbersAreRendererIndependent(t *testing.T) {
	source := `
local counter = 1
function increment(value)
  return value + 0.5
end
function _update()
  counter = increment(counter)
end
function _draw()
  cls()
  map()
end
`
	for _, renderer := range []Renderer{RendererMode4, RendererMode0} {
		generated, report, err := TranspileWithReport("mono-renderer.lua", source, Options{
			Renderer: renderer,
			SheetW:   8,
			SheetH:   8,
			MapW:     30,
			MapH:     20,
		})
		if err != nil {
			t.Fatalf("%s: %v", renderer, err)
		}
		if report.Selected != renderer {
			t.Fatalf("requested %s selected %s", renderer, report.Selected)
		}
		if !strings.Contains(generated, "counter rt.Number") ||
			!strings.Contains(generated, "func (g *Game) increment(c *rt.Context, a0 rt.Number) rt.Number") {
			t.Fatalf("%s did not emit renderer-independent numeric storage and signature:\n%s", renderer, generated)
		}
		updateStart := strings.Index(generated, "func (g *Game) gluUpdate(")
		if updateStart < 0 {
			t.Fatalf("%s is missing generated update body:\n%s", renderer, generated)
		}
		updateEnd := strings.Index(generated[updateStart:], "\n}\n")
		if updateEnd < 0 {
			t.Fatalf("%s generated update body has no end:\n%s", renderer, generated)
		}
		updateBody := generated[updateStart : updateStart+updateEnd]
		if !strings.Contains(updateBody, "g.increment(") || strings.Contains(updateBody, "__glu_call_increment") {
			t.Fatalf("%s did not emit renderer-independent direct no-alloc call:\n%s", renderer, updateBody)
		}
	}
}

func TestAddRootsEarlierSiblingAcrossCollection(t *testing.T) {
	generated, err := Transpile("add-root-gap.lua", `
function choose(first, second)
  return first
end
function probe()
  local values = {1}
  return choose(tostr(12345), add(values, 2))
end
`, Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	patched := strings.Replace(generated, "err := c.TableAdd(t,v,i)", "err := auditTableAdd(c,t,v,i)", 1)
	if patched == generated {
		t.Fatal("generated TableAdd root helper was not found")
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func auditTableAdd(c *rt.Context, table, value, index rt.Value) rt.HeapError {
	if err := c.Collect(); err != rt.HeapOK { return err }
	return c.TableAdd(table, value, index)
}

func TestEarlierSiblingSurvivesTableAddCollection(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := g.__glu_call_probe(&c)
	if got.Kind != rt.KindString || c.StrLen(got) != 5 {
		t.Fatalf("earlier string sibling was not preserved: kind=%d len=%d", got.Kind, c.StrLen(got))
	}
}
`
	runGeneratedHarness(t, patched, harness, "TestEarlierSiblingSurvivesTableAddCollection")
}

func TestTableStoresRootPendingAssignmentAcrossCollection(t *testing.T) {
	generated, err := Transpile("store-root-gap.lua", `
function probe()
  local first = {occupied = 1}
  local second = {}
  first["x"], second["x"] = tostr(111), tostr(222)
  return second["x"]
end
`, Options{DisableMonomorphicNumbers: true, DisableStableTableFields: true})
	if err != nil {
		t.Fatal(err)
	}
	patched := strings.Replace(generated, "err := c.TableSet(t,k,v)", "err := auditTableSet(c,t,k,v)", 1)
	if patched == generated {
		t.Fatal("generated TableSet root helper was not found")
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func auditTableSet(c *rt.Context, table, key, value rt.Value) rt.HeapError {
	if err := c.Collect(); err != rt.HeapOK { return err }
	return c.TableSet(table, key, value)
}

func TestPendingAssignmentSurvivesTableSetCollection(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := g.__glu_call_probe(&c)
	if got.Kind != rt.KindString || c.StrLen(got) != 3 {
		t.Fatalf("pending assignment value was not preserved: kind=%d len=%d", got.Kind, c.StrLen(got))
	}
}
`
	runGeneratedHarness(t, patched, harness, "TestPendingAssignmentSurvivesTableSetCollection")
}

func TestPrintRootsLiveLocalsAcrossCollection(t *testing.T) {
	generated, err := Transpile("print-root-gap.lua", `
function probe()
  local live = tostr(12345)
  print(7)
  return live
end
`, Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	patched := strings.Replace(generated, "out := c.PrintV(s,x,y,color)", "out := auditPrint(c,s,x,y,color)", 1)
	if patched == generated {
		t.Fatal("generated PrintV root helper was not found")
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func auditPrint(c *rt.Context, text, x, y, color rt.Value) int {
	if err := c.Collect(); err != rt.HeapOK { return 0 }
	return c.PrintV(text, x, y, color)
}

func TestLiveLocalSurvivesPrintCollection(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := g.__glu_call_probe(&c)
	if got.Kind != rt.KindString || c.StrLen(got) != 5 {
		t.Fatalf("live local was not preserved: kind=%d len=%d", got.Kind, c.StrLen(got))
	}
}
`
	runGeneratedHarness(t, patched, harness, "TestLiveLocalSurvivesPrintCollection")
}

func TestLocalRootBudgetOverflowIsCompileError(t *testing.T) {
	var source strings.Builder
	source.WriteString("function pressure()\n")
	for i := 0; i < runtimeapi.MaxLocalRoots; i++ {
		fmt.Fprintf(&source, "  local value_%d = \"live\"\n", i)
	}
	source.WriteString("  return tostr(7)\nend\n")
	_, err := Transpile("root-budget.lua", source.String(), Options{DisableMonomorphicNumbers: true})
	if err == nil || !strings.Contains(err.Error(), "local GC root budget") || !strings.Contains(err.Error(), "root-budget.lua") {
		t.Fatalf("error = %v, want source-positioned local GC root budget rejection", err)
	}
}

func TestRecursiveExpandingReturnArityIsRejected(t *testing.T) {
	_, err := Transpile("recursive-width.lua", `
function expand(n)
  if n <= 0 then return 0 end
  return n, expand(n - 1)
end
`, Options{})
	if err == nil || !strings.Contains(err.Error(), "recursive return arity expands without bound") || !strings.Contains(err.Error(), "recursive-width.lua") {
		t.Fatalf("error = %v, want source-positioned recursive return-width rejection", err)
	}
}

func TestTopLevelReturnEvaluatesExpressionSideEffects(t *testing.T) {
	generated, err := Transpile("top-return.lua", `
n = 0
function bump()
  n += 1
  return n
end
return bump()
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestTopReturnExpressionExecuted(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	if g.n.Int() != 1 { t.Fatalf("n=%d, want 1", g.n.Int()) }
}
`
	runGeneratedHarness(t, generated, harness, "TestTopReturnExpressionExecuted")
}

func TestGeneratedGameMemberCollisionsAreRejected(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{"flattened-functions", "function foo_bar() return 1 end\nfunction foo.bar() return 2 end", "generated Game member"},
		{"lifecycle", "function Init() return 1 end", "generated Game member"},
		{"call-wrapper", "function foo() return 1 end\nfunction __glu_call_foo() return 2 end", "generated Game member"},
		{"global-method", "foo = 1\nfunction foo() return 2 end", "assignment to static function"},
		{"static-field", "_gluStatic0 = 1\nfunction probe() return \"text\" end", "generated Game member"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Transpile(test.name+".lua", test.source, Options{})
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), test.name+".lua") {
				t.Fatalf("error = %v, want source-positioned %q rejection", err, test.want)
			}
		})
	}
}

func TestBuiltinResolutionDoesNotDependOnNumericOptimization(t *testing.T) {
	source := `
function abs(value)
  return 99
end
function probe()
  return abs(-1)
end
`
	for _, options := range []Options{{}, {DisableMonomorphicNumbers: true}} {
		generated, err := Transpile("builtin-precedence.lua", source, options)
		if err != nil {
			t.Fatal(err)
		}
		start := strings.Index(generated, "func (g *Game) probe(")
		if start < 0 {
			t.Fatalf("probe body missing:\n%s", generated)
		}
		end := strings.Index(generated[start:], "\n}")
		if end < 0 {
			t.Fatalf("probe body is unterminated:\n%s", generated[start:])
		}
		body := generated[start : start+end]
		if strings.Contains(body, "g.abs(") || !strings.Contains(body, ".Abs()") {
			t.Fatalf("builtin abs lost precedence with options %+v:\n%s", options, body)
		}
	}
}
