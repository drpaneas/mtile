package transpiler

import (
	"os/exec"
	"strings"
	"testing"
)

func TestIWRAMFunctionsEmitOnlyRequestedPlacement(t *testing.T) {
	const source = `counter=0
function hot(v) return v+2 end
function cold(v) return v-1 end
function _update() counter=cold(hot(counter)) end
function _draw() cls(0) end`
	for _, renderer := range []Renderer{RendererMode4, RendererMode0} {
		opts := Options{Renderer: renderer, SheetW: 8, SheetH: 8}
		plain, err := Transpile("placement.lua", source, opts)
		if err != nil {
			t.Fatal(err)
		}
		opts.IWRAMFunctions = []string{"hot"}
		placed, err := Transpile("placement.lua", source, opts)
		if err != nil {
			t.Fatal(err)
		}
		const declaration = "func (g *Game) hot("
		const placedDeclaration = "//go:noinline\n//go:section .iwram\n" + declaration
		if !strings.Contains(placed, placedDeclaration) {
			t.Fatal("selected function lacks directives")
		}
		if strings.Replace(placed, placedDeclaration, declaration, 1) != plain {
			t.Fatal("placement changed generated logic or another declaration")
		}
		if renderer == RendererMode4 {
			compileGenerated(t, placed)
		} else {
			cmd := exec.Command("go", "test", "-tags=mtile_mode0", "./...")
			cmd.Dir = writeGeneratedPackage(t, placed)
			cmd.Env = generatedGoCacheEnv()
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("Mode 0 generated package: %v\n%s", err, out)
			}
		}
	}
}

func TestPrintHelpersAreNoInline(t *testing.T) {
	got, err := Transpile("print-noinline.lua", `
function hud()
  print("HI", 0, 0, 7)
end
function _draw()
  cls(0)
  hud()
end
`, Options{SheetW: 8, SheetH: 8})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "//go:noinline\nfunc (g *Game) hud(") {
		t.Fatal("print helper must not be inlined into gluDraw")
	}
	if strings.Contains(got, "//go:noinline\nfunc (g *Game) gluDraw(") {
		t.Fatal("_draw without print must keep its ordinary frame")
	}
}

func TestIWRAMFunctionsRejectInvalidAndUnknownNames(t *testing.T) {
	for _, tc := range []struct {
		names []string
		want  string
	}{
		{[]string{"missing"}, `unknown top-level function "missing"`},
		{[]string{"hot", "hot"}, `duplicate function "hot"`},
		{[]string{"_init"}, "reserved lifecycle"},
		{[]string{"_update"}, "reserved lifecycle"},
		{[]string{"_draw"}, "reserved lifecycle"},
		{[]string{"actor.hot"}, "plain top-level"},
		{[]string{"cls"}, `unknown top-level function "cls"`},
	} {
		_, err := Transpile("placement.lua", "function hot() end", Options{IWRAMFunctions: tc.names})
		if err == nil || !strings.Contains(err.Error(), "iwramFunctions") || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: %v, want %q", tc.names, err, tc.want)
		}
	}
}
