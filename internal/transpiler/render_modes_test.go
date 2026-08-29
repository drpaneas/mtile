package transpiler

import (
	"os"
	"strings"
	"testing"
)

func TestEveryBuiltinHasRendererCompatibility(t *testing.T) {
	counts := map[RendererCompatibility]int{}
	for name := range Builtins {
		got := BuiltinRendererCompatibility(name)
		if got == RendererUnknown {
			t.Errorf("builtin %q has no renderer compatibility", name)
		}
		counts[got]++
	}
	if got := BuiltinRendererCompatibility("not_a_builtin"); got != RendererUnknown {
		t.Fatalf("unknown builtin compatibility = %d", got)
	}
	want := map[RendererCompatibility]int{
		RendererIndependent:      45,
		RendererBoth:             9,
		RendererMode0Conditional: 8,
		RendererMode4Only:        7,
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
	for _, name := range []string{"map", "spr", "print", "pal"} {
		if got := BuiltinRendererCompatibility(name); got != RendererMode0Conditional {
			t.Errorf("%s compatibility = %d, want conditional Mode 0", name, got)
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
