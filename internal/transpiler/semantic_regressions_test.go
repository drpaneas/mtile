package transpiler

import (
	"fmt"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

func TestParenthesizedCallCollapsesMultipleResults(t *testing.T) {
	for _, options := range []Options{{}, {DisableMonomorphicNumbers: true}} {
		generated, err := Transpile("parenthesized-results.lua", `
function pair() return 11, 22 end
function probe()
  local a, b = (pair())
  if a == 11 and b == nil then return 1 end
  return 0
end
`, options)
		if err != nil {
			t.Fatalf("options %+v: %v", options, err)
		}
		runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 1, false), "TestGeneratedValue")
	}
}

func TestTrailingMultipleResultsExpandIntoUserCallArguments(t *testing.T) {
	for _, options := range []Options{{}, {DisableMonomorphicNumbers: true}} {
		generated, err := Transpile("call-results.lua", `
function pair() return 11, 22 end
function combine(a, b) return a + b end
function probe() return combine(pair()) end
`, options)
		if err != nil {
			t.Fatalf("options %+v: %v", options, err)
		}
		runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 33, false), "TestGeneratedValue")
	}
}

func TestTrailingMultipleResultsParticipateInBuiltinArity(t *testing.T) {
	for _, options := range []Options{{}, {DisableMonomorphicNumbers: true}} {
		generated, err := Transpile("builtin-results.lua", `
function pair() return 11, 22 end
function probe() return max(pair()) end
`, options)
		if err != nil {
			t.Fatalf("options %+v: %v", options, err)
		}
		runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 22, false), "TestGeneratedValue")
	}
}

func TestExpandedBuiltinArgumentsSelectTheEffectiveArity(t *testing.T) {
	generated, err := Transpile("builtin-effective-arity.lua", `
function flag_args() return 3, 1 end
function map_args() return 2, 4, 5 end
function probe()
  local flag = fget(flag_args())
  local tile = mget(map_args())
  return flag, tile
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generated, "c.Fget(") || strings.Contains(generated, "c.FgetByte(") {
		t.Fatalf("expanded two-result fget did not select the two-argument operation:\n%s", generated)
	}
	if !strings.Contains(generated, "c.MgetLayer(") {
		t.Fatalf("expanded three-result mget did not select layered access:\n%s", generated)
	}
	compileGenerated(t, generated)
}

func TestLiteralBG0FastPathPreservesTrailingMultiReturnArguments(t *testing.T) {
	generated, err := Transpile("bg0-multi-results.lua", `
function coords() return 4, 5 end
function edit() return 4, 5, 6 end
function probe()
  local tile = mget(0, coords())
  mset(0, edit())
  return tile
end
`, Options{MapW: 8, MapH: 8, Map: make([]uint16, 64)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generated, "c.MgetCompact(0,0,") || !strings.Contains(generated, "c.MsetCompact(0,0,") ||
		strings.Contains(generated, "c.MgetLayer(") || strings.Contains(generated, "c.MsetLayer(") {
		t.Fatalf("literal BG0 trailing multi-results missed the compact fixed-slot path:\n%s", generated)
	}
	compileGenerated(t, generated)
}

func TestStreamingLiteralBG0FastPathDoesNotMaterializeUnusedLayer(t *testing.T) {
	generated, err := Transpile("streaming-bg0.lua", `
function coords() return 4, 5 end
function edit() return 4, 5, 6 end
function probe()
  local tile = mget(0, coords())
  mset(0, edit())
  return tile
end
`, Options{MapW: 129, MapH: 128, Map: make([]uint16, 129*128)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generated, "c.Mget(") || !strings.Contains(generated, "c.Mset(") ||
		strings.Contains(generated, "c.MgetLayer(") || strings.Contains(generated, "c.MsetLayer(") {
		t.Fatalf("streaming literal BG0 missed the existing BG0 path:\n%s", generated)
	}
	compileGenerated(t, generated)
}

func TestCompactMapLayerConstantsUseFixedStorageSlots(t *testing.T) {
	source := `
local COLLISION_BG=1
function probe(x,y)
  local tile=mget(COLLISION_BG,x,y)
  mset(COLLISION_BG,x,y,tile)
  return tile
end
`
	options := Options{MapW: 2, MapH: 2, MapLayers: 2, Map: make([]uint16, 8)}
	generated, err := Transpile("fixed-map-layer.lua", source, options)
	if err != nil {
		t.Fatal(err)
	}
	probe := generatedSection(generated, "func (g *Game) probe(")
	if !strings.Contains(probe, "c.MgetCompact(1,1,") || !strings.Contains(probe, "c.MsetCompact(1,1,") ||
		strings.Contains(probe, "c.MgetLayer(") || strings.Contains(probe, "c.MsetLayer(") {
		t.Fatalf("constant compact layer retained logical-to-storage lookup:\n%s", probe)
	}
	compileGenerated(t, generated)

	options.DisableFixedMapLayerSpecialization = true
	rollback, err := Transpile("fixed-map-layer-rollback.lua", source, options)
	if err != nil {
		t.Fatal(err)
	}
	rollbackProbe := generatedSection(rollback, "func (g *Game) probe(")
	if !strings.Contains(rollbackProbe, "c.MgetLayer(") || !strings.Contains(rollbackProbe, "c.MsetLayer(") ||
		strings.Contains(rollbackProbe, "c.MgetCompact(") || strings.Contains(rollbackProbe, "c.MsetCompact(") {
		t.Fatalf("fixed-layer rollback did not retain logical lookup:\n%s", rollbackProbe)
	}
	compileGenerated(t, rollback)
}

func TestGenericForChecksAdjustedIteratorArguments(t *testing.T) {
	_, err := Transpile("iterator-results.lua", `
function pair() return {}, {} end
function probe()
  for value in all(pair()) do end
end
`, Options{})
	if err == nil || !strings.Contains(err.Error(), "all expects 1..1 arguments, got 2") {
		t.Fatalf("error = %v, want adjusted iterator arity rejection", err)
	}
}

func TestGenericNumericForDeclaresLimitBeforeRootedStepEvaluation(t *testing.T) {
	generated, err := Transpile("generic-for-order.lua", `
function limit_value() return 2 end
function step_value(a, b) return 1 end
function probe()
  local total = 0
  for i = 1, limit_value(), step_value(1, tostr(3)) do
    total += i
  end
  return total
end
`, Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	compileGenerated(t, generated)
}

func TestPowerCompilesAndUsesIntegerExponentExactly(t *testing.T) {
	for _, options := range []Options{{}, {DisableMonomorphicNumbers: true}} {
		generated, err := Transpile("power.lua", `
function probe() return 3 ^ 4 end
`, options)
		if err != nil {
			t.Fatalf("options %+v: %v", options, err)
		}
		runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 81, false), "TestGeneratedValue")
	}
}

func TestPowerRejectsFractionalOrDynamicExponent(t *testing.T) {
	for _, source := range []string{
		"function probe() return 9 ^ 0.5 end",
		"function probe(n) return 9 ^ n end",
	} {
		_, err := Transpile("power-contract.lua", source, Options{})
		if err == nil || !strings.Contains(err.Error(), "integer literal exponent") || !strings.Contains(err.Error(), "power-contract.lua:1") {
			t.Fatalf("error = %v, want source-positioned exponent contract rejection", err)
		}
	}
}

func TestModuloUsesFloorDivisionSignRule(t *testing.T) {
	for _, options := range []Options{{}, {DisableMonomorphicNumbers: true}} {
		generated, err := Transpile("modulo.lua", `
function probe() return -3 % 2 end
`, options)
		if err != nil {
			t.Fatalf("options %+v: %v", options, err)
		}
		runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 1, false), "TestGeneratedValue")
	}
}

func TestStringOrderingIsLexicographic(t *testing.T) {
	generated, err := Transpile("string-order.lua", `
function probe()
  if "alpha" < "beta" and "beta" >= "alpha" and not ("beta" < "alpha") then
    return 1
  end
  return 0
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 1, false), "TestGeneratedValue")
}

func TestFunctionValuesAndShadowedCallsFailExplicitly(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"function-value", "function target() return 7 end\nlocal callback = target", "first-class function value"},
		{"parameter-call", "function target() return 7 end\nfunction probe(target) return target() end", "dynamic call through local"},
		{"builtin-shadow", "function probe() local abs = 3 return abs() end", "dynamic call through local"},
		{"indexed-function-value", "actor = {}\nfunction actor:draw() return 7 end\nfunction probe() return actor[\"draw\"] end", "first-class function value"},
		{"method-field-value", "actor = {}\nfunction actor:draw() return 7 end\nfunction probe() return actor.draw end", "first-class function value"},
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

func TestAmbiguousTopLevelLocalAndLocalFunctionFailExplicitly(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			"late-local",
			"function before() return state end\nlocal state = 9\nfunction after() return state end",
			"declared after",
		},
		{
			"local-function",
			"local function helper() return 1 end\nfunction probe() return helper() end",
			"local function declarations are unsupported",
		},
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

func TestTopLevelLocalRedeclarationAndCallableMutationFailExplicitly(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			"top-local-redeclare",
			"local state = 1\nfunction first() return state end\nlocal state = 2",
			"top-level local \"state\" redeclaration",
		},
		{
			"builtin-write",
			"abs = 3",
			"assignment to static builtin",
		},
		{
			"function-field-write",
			"function actor.draw() return 1 end\nactor.draw = 2",
			"assignment to static function field",
		},
		{
			"method-field-write",
			"function actor:draw() return 1 end\nactor.draw = 2",
			"assignment to static method field",
		},
		{
			"indexed-method-field-write",
			"function actor:draw() return 1 end\nactor[\"draw\"] = 2",
			"assignment to static method field",
		},
		{
			"namespace-write-after",
			"actor = {}\nfunction actor:draw() return 1 end\nactor = {}",
			"assignment to static function namespace",
		},
		{
			"namespace-write-in-function",
			"actor = {}\nfunction actor:draw() return 1 end\nfunction reset() actor = {} end",
			"assignment to static function namespace",
		},
		{
			"dynamic-function-field-write",
			"actor = {}\nkey = \"draw\"\nfunction actor:draw() return 1 end\nfunction reset() actor[key] = 2 end",
			"dynamic indexed assignment to static function namespace",
		},
		{
			"dynamic-function-field-read",
			"actor = {}\nkey = \"draw\"\nfunction actor:draw() return 1 end\nfunction probe() return actor[key] end",
			"dynamic indexed access to static function namespace",
		},
		{
			"nested-namespace-field-write",
			"actor = {body = {}}\nfunction actor.body:draw() return 1 end\nactor.body = {}",
			"assignment to static function namespace \"actor.body\"",
		},
		{
			"nested-namespace-index-write",
			"actor = {body = {}}\nfunction actor.body:draw() return 1 end\nactor[\"body\"] = {}",
			"assignment to static function namespace \"actor.body\"",
		},
		{
			"nested-namespace-dynamic-write",
			"actor = {body = {}}\nkey = \"body\"\nfunction actor.body:draw() return 1 end\nactor[key] = {}",
			"dynamic indexed assignment to static function namespace \"actor\"",
		},
		{
			"nested-namespace-dynamic-read",
			"actor = {body = {}}\nkey = \"body\"\nfunction actor.body:draw() return 1 end\nfunction probe() return actor[key] end",
			"dynamic indexed access to static function namespace \"actor\"",
		},
		{
			"deep-literal-method-write",
			"actor = {body = {}}\nfunction actor.body:draw() return 1 end\nactor[\"body\"][\"draw\"] = 2",
			"assignment to static method field \"actor.body.draw\"",
		},
		{
			"deep-literal-method-read",
			"actor = {body = {}}\nfunction actor.body:draw() return 1 end\nfunction probe() return actor[\"body\"][\"draw\"] end",
			"first-class function value \"actor.body.draw\"",
		},
		{
			"namespace-alias",
			"actor = {body = {}}\nfunction actor.body:draw() return 1 end\nalias = actor.body\nalias.draw = 2",
			"first-class function namespace \"actor.body\"",
		},
		{
			"root-namespace-initializer-alias",
			"shared = {}\nactor = shared\nfunction actor:draw() return 1 end\nshared.draw = 2",
			"static function namespace \"actor\" must be initialized with a fresh table constructor",
		},
		{
			"nested-namespace-initializer-alias",
			"shared = {}\nactor = {body = shared}\nfunction actor.body:draw() return 1 end\nshared.draw = 2",
			"static function namespace \"actor.body\" must be initialized with a fresh table constructor",
		},
		{
			"nested-namespace-assignment-alias",
			"actor = {}\nshared = {}\nactor.body = shared\nfunction actor.body:draw() return 1 end\nshared.draw = 2",
			"static function namespace \"actor.body\" must be initialized with a fresh table constructor",
		},
		{
			"dynamic-nested-namespace-initializer",
			"key = \"body\"\nactor = {[key] = {}}\nfunction actor.body:draw() return 1 end",
			"dynamic initializer field may alias a nested static function namespace",
		},
		{
			"root-namespace-alias",
			"actor = {}\nfunction actor:draw() return 1 end\nfunction capture() return actor end",
			"first-class function namespace \"actor\"",
		},
		{
			"compound-method-field-write",
			"actor = {}\nfunction actor:draw() return 1 end\nactor.draw += 1",
			"assignment to static method field \"actor.draw\"",
		},
		{
			"compound-namespace-write",
			"actor = {body = {}}\nfunction actor.body:draw() return 1 end\nactor.body ..= \"bad\"",
			"assignment to static function namespace \"actor.body\"",
		},
		{
			"parenthesized-method-write",
			"actor = {body = {}}\nfunction actor.body:draw() return 1 end\n(actor.body).draw = 2",
			"assignment to static method field \"actor.body.draw\"",
		},
		{
			"method-self-write",
			"actor = {}\nfunction actor:draw() return 1 end\nfunction actor:reset() self.draw = 2 end",
			"assignment to static method field \"actor.draw\"",
		},
		{
			"method-self-escape",
			"actor = {}\nfunction actor:draw() return 1 end\nfunction actor:capture() return self end",
			"first-class function namespace \"actor\"",
		},
		{
			"method-self-reassignment",
			"actor = {}\nenemy = {}\nfunction enemy:draw() return 1 end\nfunction actor:reset() self = enemy self.draw = 2 end",
			"reassignment of method receiver self",
		},
		{
			"method-self-shadow",
			"actor = {}\nfunction actor:draw() local self = {} return 1 end",
			"shadowing method receiver self",
		},
		{
			"method-self-parameter-shadow",
			"actor = {}\nfunction actor:draw(self) return self end",
			"shadowing method receiver self",
		},
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

func TestLocalTableMayShadowStaticFunctionNamespace(t *testing.T) {
	generated, err := Transpile("local-namespace-shadow.lua", `
actor = {}
function actor:draw() return 1 end
function probe()
  local actor = {draw = 7}
  actor["draw"] = 8
  return actor.draw
end
`, Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 8, false), "TestGeneratedValue")
}

func TestStaticFunctionNamespaceStillAllowsOrdinaryDataFields(t *testing.T) {
	generated, err := Transpile("namespace-data.lua", `
actor = {[1] = "root data", body = {[2] = "body data", value = 3}}
function actor.body:add(n) return self.value + n end
function probe()
  actor.body.value += 1
  return actor.body:add(2)
end
`, Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 6, false), "TestGeneratedValue")
}

func TestChunkNestedLocalsDoNotBecomeGlobals(t *testing.T) {
	report, err := AnalyzeGlobals("chunk-scopes.lua", `
do
  local temporary = 7
  result = temporary
end
if true then
  local branch = 9
  other = branch
end
`)
	if err != nil {
		t.Fatal(err)
	}
	if report.Count != 2 || report.Symbols[0].Name != "result" || report.Symbols[1].Name != "other" {
		t.Fatalf("global report = %#v, want only result and other", report)
	}
	generated, err := Transpile("chunk-scopes.lua", `
do local temporary = 7 result = temporary end
if true then local branch = 9 other = branch end
function probe() return result * 10 + other end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 79, false), "TestGeneratedValue")
}

func TestTopLevelCompoundTempsStayLexical(t *testing.T) {
	report, err := AnalyzeGlobals("compound-temp.lua", `
object = {x = 1}
object.x += 2
function probe() return object.x end
`)
	if err != nil {
		t.Fatal(err)
	}
	if report.Count != 1 || len(report.Symbols) != 1 || report.Symbols[0].Name != "object" {
		t.Fatalf("global report = %#v, want only the source global object", report)
	}
	generated, err := Transpile("compound-temp.lua", `
object = {x = 1}
object.x += 2
function probe() return object.x end
`, Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(generatedSection(generated, "type Game struct {"), "__glu_tmp") {
		t.Fatalf("synthetic compound temporary escaped into Game state:\n%s", generated)
	}
	runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 3, false), "TestGeneratedValue")
}

func TestStaticStringLimitsFailAtCompileTime(t *testing.T) {
	build := func(count int) string {
		var source strings.Builder
		source.WriteString("function probe()\n")
		for i := 0; i < count; i++ {
			fmt.Fprintf(&source, "  printh(%q)\n", fmt.Sprintf("literal-%03d", i))
		}
		source.WriteString("end\n")
		return source.String()
	}
	if _, err := Transpile("static-limit.lua", build(256), Options{}); err != nil {
		t.Fatalf("exact static-string limit rejected: %v", err)
	}
	if _, err := Transpile("static-overflow.lua", build(257), Options{}); err == nil || !strings.Contains(err.Error(), "256 static strings") || !strings.Contains(err.Error(), "static-overflow.lua") {
		t.Fatalf("error = %v, want source-positioned static-string count rejection", err)
	}
	tooLong := strings.Repeat("x", 4097)
	if _, err := Transpile("static-length.lua", "function probe() printh("+fmt.Sprintf("%q", tooLong)+") end", Options{}); err == nil || !strings.Contains(err.Error(), "4096 bytes") || !strings.Contains(err.Error(), "static-length.lua") {
		t.Fatalf("error = %v, want source-positioned static-string length rejection", err)
	}
}

func TestExpressionWalkerVisitsEveryASTNodeOnce(t *testing.T) {
	chunk, err := luaparser.ParseString("single-walk.lua", `result = ({label = "value"})["label"] .. ("!")`)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[luaparser.Expr]int)
	collectStmtExprs(chunk.Body[0], func(expr luaparser.Expr) { seen[expr]++ })
	if len(seen) == 0 {
		t.Fatal("expression walker visited no nodes")
	}
	for expr, visits := range seen {
		if visits != 1 {
			t.Fatalf("%T at %s visited %d times, want exactly once", expr, expr.Pos(), visits)
		}
	}
}

func TestTableLiteralCapacityLimitsFailAtCompileTime(t *testing.T) {
	exactArray := "function probe() local value = {" + strings.TrimSuffix(strings.Repeat("1,", 256), ",") + "} end"
	if _, err := Transpile("array-capacity-exact.lua", exactArray, Options{}); err != nil {
		t.Fatalf("exact array capacity rejected: %v", err)
	}
	array := "function probe() local value = {" + strings.TrimSuffix(strings.Repeat("1,", 257), ",") + "} end"
	if _, err := Transpile("array-capacity.lua", array, Options{}); err == nil || !strings.Contains(err.Error(), "257 array slots") || !strings.Contains(err.Error(), "array-capacity.lua") {
		t.Fatalf("array error = %v, want source-positioned capacity rejection", err)
	}
	var fields strings.Builder
	fields.WriteString("function probe() local value = {")
	for i := 0; i < 129; i++ {
		fmt.Fprintf(&fields, "field%d=1,", i)
	}
	fields.WriteString("} end")
	if _, err := Transpile("hash-capacity.lua", fields.String(), Options{}); err == nil || !strings.Contains(err.Error(), "129 hash slots") || !strings.Contains(err.Error(), "hash-capacity.lua") {
		t.Fatalf("hash error = %v, want source-positioned capacity rejection", err)
	}
	var exactFields strings.Builder
	exactFields.WriteString("function probe() local value = {")
	for i := 0; i < 128; i++ {
		fmt.Fprintf(&exactFields, "field%d=1,", i)
	}
	exactFields.WriteString("} end")
	if _, err := Transpile("hash-capacity-exact.lua", exactFields.String(), Options{}); err != nil {
		t.Fatalf("exact hash capacity rejected: %v", err)
	}
	var returns strings.Builder
	returns.WriteString("function many() return ")
	returns.WriteString(strings.TrimSuffix(strings.Repeat("1,", 257), ","))
	returns.WriteString(" end\nfunction probe() local value = {many()} end")
	if _, err := Transpile("expanded-capacity.lua", returns.String(), Options{}); err == nil || !strings.Contains(err.Error(), "257 array slots") {
		t.Fatalf("expanded array error = %v, want adjusted capacity rejection", err)
	}
}

func TestTrailingZeroResultCallAddsNoTableFieldButStillRuns(t *testing.T) {
	generated, err := Transpile("zero-result-table.lua", `
local calls = 0
function touch() calls += 1 end
function probe()
  local values = {7, touch()}
  return #values * 10 + calls
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 11, false), "TestGeneratedValue")
}

func TestTrailingZeroResultBuiltinAddsNoTableField(t *testing.T) {
	generated, err := Transpile("zero-result-builtin-table.lua", `
function probe()
  local values = {7, color(3)}
  return #values
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 1, false), "TestGeneratedValue")
}

func TestTrailingZeroResultBuiltinReducesEffectiveArgumentArity(t *testing.T) {
	_, err := Transpile("zero-result-builtin-args.lua", `
function probe() return max(7, color(3)) end
`, Options{})
	if err == nil || !strings.Contains(err.Error(), "max expects 2..2 arguments, got 1") {
		t.Fatalf("error = %v, want adjusted builtin arity rejection", err)
	}
}

func TestBuiltinResultAritiesAreExhaustivelyClassified(t *testing.T) {
	zeroResults := map[string]bool{
		"cls": true, "pset": true, "line": true, "rect": true, "rectfill": true,
		"circ": true, "circfill": true, "spr": true, "sspr": true, "pspr": true, "map": true,
		"camera": true, "clip": true, "pal": true, "palt": true, "color": true,
		"cursor": true, "sset": true, "fset": true, "mset": true, "srand": true,
		"foreach": true, "sfx": true, "music": true, "sample": true,
		"gba_note": true, "gba_stop": true, "printh": true,
	}
	for name, spec := range Builtins {
		want := 1
		if zeroResults[name] {
			want = 0
			delete(zeroResults, name)
		}
		if spec.Results != want {
			t.Errorf("builtin %q results = %d, want %d", name, spec.Results, want)
		}
	}
	for name := range zeroResults {
		t.Errorf("zero-result classification names missing builtin %q", name)
	}
}

func TestTableConstructorListFieldsUseDeclaredIndices(t *testing.T) {
	for _, options := range []Options{{}, {DisableMonomorphicNumbers: true}} {
		generated, err := Transpile("constructor-indices.lua", `
function pair() return 7, 8 end
function probe()
  local single = {[1]=9, 7}
  local multiple = {[1]=9, pair()}
  return single[1] * 100 + #single * 10 + multiple[1] + multiple[2]
end
`, options)
		if err != nil {
			t.Fatalf("options %+v: %v", options, err)
		}
		// single is {7}; multiple is {7,8}. A #table-driven append would
		// incorrectly produce {9,7} and {9,7,8}.
		runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 725, false), "TestGeneratedValue")
	}
}

func TestNumericReturnPreservesEarlierValueBeforeZeroResultTail(t *testing.T) {
	for _, options := range []Options{{}, {DisableMonomorphicNumbers: true}} {
		generated, err := Transpile("numeric-return-zero-tail.lua", `
local calls = 0
function touch() calls += 1 end
function value() return 7, touch() end
function probe() return value() * 10 + calls end
`, options)
		if err != nil {
			t.Fatalf("options %+v: %v", options, err)
		}
		runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 71, false), "TestGeneratedValue")
	}
}

func TestGenericNumericForRequiresNumericStep(t *testing.T) {
	for _, options := range []Options{{}, {DisableMonomorphicNumbers: true}} {
		generated, err := Transpile("numeric-for-step-kind.lua", `
function probe()
  local count = 0
  for i = 1, 2, false do
    count += 1
    break
  end
  return count
end
`, options)
		if err != nil {
			t.Fatalf("options %+v: %v", options, err)
		}
		runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 0, false), "TestGeneratedValue")
	}
}

func TestNestedLoopActiveRootOverflowIsCompileError(t *testing.T) {
	var source strings.Builder
	source.WriteString("function pressure()\n")
	loops := runtimeapi.MaxLocalRoots/3 + 1
	for i := 0; i < loops; i++ {
		fmt.Fprintf(&source, "for loop_%d = 1, 1 do\n", i)
	}
	for i := 0; i < loops; i++ {
		source.WriteString("end\n")
	}
	source.WriteString("end\n")
	_, err := Transpile("active-root-budget.lua", source.String(), Options{DisableMonomorphicNumbers: true})
	if err == nil || !strings.Contains(err.Error(), "active GC root budget") || !strings.Contains(err.Error(), "active-root-budget.lua") {
		t.Fatalf("error = %v, want source-positioned active-root budget rejection", err)
	}
}

func TestTransitiveUserCallRootBudgetFailsAtCompileTime(t *testing.T) {
	var source strings.Builder
	source.WriteString("function allocate() return tostr(7) end\nfunction pressure()\n")
	for i := 0; i < runtimeapi.MaxLocalRoots; i++ {
		fmt.Fprintf(&source, "local value_%d = \"live\"\n", i)
	}
	source.WriteString("return allocate()\nend\n")
	_, err := Transpile("transitive-root-budget.lua", source.String(), Options{DisableMonomorphicNumbers: true})
	if err == nil || !strings.Contains(err.Error(), "transitive GC root budget") ||
		!strings.Contains(err.Error(), "513 slots") || !strings.Contains(err.Error(), "transitive-root-budget.lua") {
		t.Fatalf("error = %v, want source-positioned 513-slot transitive root rejection", err)
	}
}

func TestRecursiveRootGrowthFailsAtCompileTime(t *testing.T) {
	_, err := Transpile("recursive-root-budget.lua", `
function recurse(n)
  local keep = "live"
  if n > 0 then return recurse(n - 1) end
  return tostr(n)
end
`, Options{DisableMonomorphicNumbers: true})
	if err == nil || !strings.Contains(err.Error(), "recursive call cycle can grow the GC root stack without bound") ||
		!strings.Contains(err.Error(), "recursive-root-budget.lua") {
		t.Fatalf("error = %v, want source-positioned recursive root-growth rejection", err)
	}
}

func TestRecursiveCallWithoutRetainedRootGrowthCompiles(t *testing.T) {
	generated, err := Transpile("bounded-recursive-roots.lua", `
n = 3
function recurse()
  if n <= 0 then
    return tostr(n)
  end
  n -= 1
  return recurse()
end
function probe()
  return recurse()
end
`, Options{})
	if err != nil {
		t.Fatalf("root-neutral recursion rejected: %v", err)
	}
	runGeneratedHarness(t, generated, `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestGeneratedValue(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	got := g.__glu_call_probe(&c)
	if !c.ValuesEqual(got, c.InternStatic("0")) {
		t.Fatalf("recursive result = %+v, want string 0", got)
	}
}
`, "TestGeneratedValue")
}

func TestTableGrowthNestedRootBudgetFailsAtCompileTime(t *testing.T) {
	var source strings.Builder
	source.WriteString("items = {}\nfunction pressure()\n")
	for i := 0; i < runtimeapi.MaxLocalRoots-4; i++ {
		fmt.Fprintf(&source, "local value_%d = \"live\"\n", i)
	}
	source.WriteString("add(items, 1)\nend\n")
	_, err := Transpile("table-growth-root-budget.lua", source.String(), Options{DisableMonomorphicNumbers: true})
	if err == nil || !strings.Contains(err.Error(), "local GC root budget") ||
		!strings.Contains(err.Error(), "513 slots") || !strings.Contains(err.Error(), "table-growth-root-budget.lua") {
		t.Fatalf("error = %v, want source-positioned 513-slot table-growth root rejection", err)
	}
}

func TestParenthesizedNegativePowerExponentRemainsStatic(t *testing.T) {
	generated, err := Transpile("negative-power.lua", `function probe() return 4 ^ -(1) end`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"testing"
)

func TestGeneratedValue(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	if got := g.__glu_call_probe(&c); got != rt.Number(16384) {
		t.Fatalf("got %d, want 0.25", got)
	}
}
`
	runGeneratedHarness(t, generated, harness, "TestGeneratedValue")
}

func TestTableLiteralDoesNotDuplicateCallerRootsDuringFieldAllocation(t *testing.T) {
	var source strings.Builder
	source.WriteString("function probe()\n")
	for i := 0; i < runtimeapi.MaxLocalRoots/2; i++ {
		fmt.Fprintf(&source, "local value_%d = \"live\"\n", i)
	}
	source.WriteString("local result = {tostr(7)}\nreturn #result\nend\n")
	generated, err := Transpile("table-root-pressure.lua", source.String(), Options{DisableMonomorphicNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 1, false), "TestGeneratedValue")
}

func TestRendererSelectionRejectsInvalidMapShapeBeforeMaskArithmetic(t *testing.T) {
	for _, test := range []struct {
		name    string
		options Options
		want    string
	}{
		{"layers", Options{Renderer: RendererMode4, MapLayers: 5}, "map layer count"},
		{"overflow", Options{Renderer: RendererMode4, MapW: int(^uint(0) >> 1), MapH: 2}, "map dimensions overflow"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := TranspileWithReport(test.name+".lua", "function probe() return 1 end", test.options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
