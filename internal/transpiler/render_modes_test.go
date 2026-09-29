package transpiler

import (
	"os"
	"strings"
	"testing"
)

func TestEveryBuiltinHasRendererCompatibility(t *testing.T) {
	want := map[RendererCompatibility]int{
		RendererIndependent:      45,
		RendererBoth:             9,
		RendererMode0Conditional: 10,
		RendererMode4Only:        7,
	}
	counts := make(map[RendererCompatibility]int, len(want))
	for name := range Builtins {
		got := BuiltinRendererCompatibility(name)
		if got == RendererUnknown {
			t.Errorf("builtin %q has no renderer compatibility; classify it in render_modes.go and document it in docs/render-modes.md", name)
			continue
		}
		if _, known := want[got]; !known {
			t.Errorf("builtin %q uses unreviewed renderer compatibility %d; add the category to the renderer contract and this exhaustive gate", name, got)
			continue
		}
		counts[got]++
	}
	if got := BuiltinRendererCompatibility("not_a_builtin"); got != RendererUnknown {
		t.Fatalf("unknown builtin compatibility = %d", got)
	}
	for compatibility, wantCount := range want {
		if got := counts[compatibility]; got != wantCount {
			t.Errorf("compatibility %d has %d builtins, want %d", compatibility, got, wantCount)
		}
	}
}

func TestRendererCompatibilityKeepsExactRasterCallsInMode4(t *testing.T) {
	for _, name := range []string{"pset", "pget", "line", "rect", "rectfill", "circ", "circfill"} {
		if got := BuiltinRendererCompatibility(name); got != RendererMode4Only {
			t.Errorf("%s compatibility = %d, want Mode 4-only", name, got)
		}
	}
	for _, name := range []string{"map", "spr", "print", "pal", "pspr"} {
		if got := BuiltinRendererCompatibility(name); got != RendererMode0Conditional {
			t.Errorf("%s compatibility = %d, want conditional Mode 0", name, got)
		}
	}
}

func TestMapBuiltinIncludesExplicitBackgroundForm(t *testing.T) {
	spec, ok := Builtins["map"]
	if !ok {
		t.Fatal("map builtin is missing")
	}
	if spec.MinArgs != 0 || spec.MaxArgs != 8 {
		t.Fatalf("map arity = %d..%d, want 0..8", spec.MinArgs, spec.MaxArgs)
	}
	if got := BuiltinRendererCompatibility("map"); got != RendererMode0Conditional {
		t.Fatalf("map compatibility = %d, want conditional Mode 0", got)
	}
}

func TestLayeredMapMemoryBuiltinsPreserveLegacyArities(t *testing.T) {
	for name, want := range map[string]BuiltinSpec{
		"mget": {GoName: "Mget", MinArgs: 2, MaxArgs: 3, Results: 1},
		"mset": {GoName: "Mset", MinArgs: 3, MaxArgs: 4, Results: 0},
	} {
		if got := Builtins[name]; got != want {
			t.Fatalf("%s spec = %+v, want %+v", name, got, want)
		}
		if got := BuiltinRendererCompatibility(name); got != RendererBoth {
			t.Fatalf("%s renderer class = %v, want RendererBoth", name, got)
		}
	}
}

func TestRenderModeDocumentationMentionsEveryBuiltin(t *testing.T) {
	document, err := os.ReadFile("../../docs/render-modes.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(document)
	for name := range Builtins {
		if !strings.Contains(text, "`"+name+"`") {
			t.Errorf("docs/render-modes.md does not mention builtin %q", name)
		}
	}
}
