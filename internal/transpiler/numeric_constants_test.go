package transpiler

import (
	"strings"
	"testing"
)

func TestImmutableNumericBindingsBecomeCompileTimeConstants(t *testing.T) {
	generated, err := Transpile("numeric-constants.lua", `
local TILE_SIZE = 8
local HALF = 0.5
ROOM_SCALE = HALF
function probe()
  return TILE_SIZE + ROOM_SCALE * 2
end
`, Options{Debug: true})
	if err != nil {
		t.Fatal(err)
	}
	game := generatedSection(generated, "type Game struct {")
	for _, name := range []string{"TILE_SIZE", "HALF", "ROOM_SCALE"} {
		if strings.Contains(game, name+" rt.Number") || strings.Contains(game, name+" rt.Value") {
			t.Fatalf("immutable numeric binding %s retained Game storage:\n%s", name, game)
		}
		if !strings.Contains(generated, `debugMailbox.Add("`+name+`", rt.NumVal(rt.Number(`) {
			t.Fatalf("debug watch lost compile-time constant %s:\n%s", name, generated)
		}
	}
	init := generatedSection(generated, "func (g *Game) Init(")
	if strings.Contains(init, "TILE_SIZE") || strings.Contains(init, "HALF") || strings.Contains(init, "ROOM_SCALE") {
		t.Fatalf("constant declaration retained runtime initialization:\n%s", init)
	}
	runGeneratedHarness(t, generated, generatedValueHarness("g.__glu_call_probe(&c)", 9, false), "TestGeneratedValue")
}

func TestNumericConstantPropagationFallsBackWhenProofIsIncomplete(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		options Options
		binding string
	}{
		{name: "reassigned", source: "A=1\nfunction _update() A=2 end", binding: "A"},
		{name: "read-before-declaration", source: "seen=A\nA=1\nfunction probe() return seen end", binding: "A"},
		{name: "shadowed-parameter", source: "A=1\nfunction probe(A) return A end", binding: "A"},
		{name: "declared-after-function", source: "function probe() return A end\nA=1", binding: "A"},
		{name: "computed-initializer", source: "A=1+2\nfunction probe() return A end", binding: "A"},
		{name: "multiple-assignment", source: "A,B=1,2\nfunction probe() return A end", binding: "A"},
		{name: "rollback", source: "A=1\nfunction probe() return A end", options: Options{DisableNumericConstants: true}, binding: "A"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generated, err := Transpile(test.name+".lua", test.source, test.options)
			if err != nil {
				t.Fatal(err)
			}
			game := generatedSection(generated, "type Game struct {")
			if !strings.Contains(game, test.binding+" rt.Number") && !strings.Contains(game, test.binding+" rt.Value") {
				t.Fatalf("unproven binding %s was removed:\n%s", test.binding, generated)
			}
			compileGenerated(t, generated)
		})
	}
}
