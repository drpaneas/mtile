package lint

import (
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/sourcebundle"
)

func TestAllocationContextIsNotInferredFromFunctionNames(t *testing.T) {
	for _, tt := range []struct {
		name, source, severity string
		path                   []string
	}{
		{"conditional-reset", `function reset() state={} end
function _update() if dead then reset() end end`, "note", []string{"_update", "reset"}},
		{"unconditional-initializer", `function initialize_cache() cache={} end
function _update() initialize_cache() end`, "advice", []string{"_update", "initialize_cache"}},
		{"conditional-local", `function _update() if ready then local scratch={} end end`, "note", []string{"_update"}},
		{"known-nonempty-loop", `function _update() for i=1,4 do local scratch={} end end`, "advice", []string{"_update"}},
		{"variable-loop", `function _update() for i=1,count do local scratch={} end end`, "note", []string{"_update"}},
		{"short-circuit", `function scratch() return {} end
function _update() local result=ready and scratch() end`, "note", []string{"_update", "scratch"}},
		{"unguarded-alternate-caller", `function scratch() return {} end
function gated() if ready then scratch() end end
function direct() scratch() end
function _update() gated() end
function _draw() direct() end`, "advice", []string{"_draw", "direct", "scratch"}},
		{"unguarded-through-cycle", `function a() b() end
function b() if ready then a() end local scratch={} end
function _update() if dead then a() end end
function _draw() a() end`, "advice", []string{"_draw", "a", "b"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds := analyzeSource(t, tt.source)
			if len(ds) != 1 || ds[0].Severity != tt.severity || !reflect.DeepEqual(ds[0].CallPath, tt.path) {
				t.Fatalf("wrong context: %+v", ds)
			}
			if (ds[0].ConditionalAt != nil) != (tt.severity == "note") {
				t.Fatalf("wrong conditional witness: %+v", ds[0])
			}
		})
	}
}

func TestRecognizesReuseBeforeNilFallback(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		recognized bool
	}{
		{"real-pool", `local p=deli(inactive)
if p==nil then p={x=x} else p.x=x end
add(active,p)`, true},
		{"nested-pool", `if ready then
local p=deli(inactive)
if nil==p then p={x=x} else p.x=x end
end`, true},
		{"not-a-reuse-operation", `local p=lookup(inactive)
if p==nil then p={x=x} end`, false},
		{"intervening-write", `local p=deli(inactive)
p=nil
if p==nil then p={x=x} end`, false},
		{"different-binding", `local p=deli(inactive)
if other==nil then p={x=x} end`, false},
		{"not-a-nil-test", `local p=deli(inactive)
if p==false then p={x=x} end`, false},
		{"shadow-before-constructor", `local p=deli(inactive)
if p==nil then local p=0 p={x=x} end`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds := analyzeSource(t, "function spawn(x)\n"+tt.body+"\nend\nfunction _update() spawn(1) end")
			if len(ds) != 1 {
				t.Fatalf("expected one allocation: %+v", ds)
			}
			recognized := strings.Contains(ds[0].Message, "reuse attempt")
			if recognized != tt.recognized {
				t.Fatalf("wrong fallback evidence: %+v", ds[0])
			}
			if recognized && ds[0].Severity != "note" {
				t.Fatalf("pool fallback should not imply missing reuse: %+v", ds[0])
			}
		})
	}
	shadow := analyzeSource(t, `function deli(x) return x end
function _update()
local p=deli(inactive)
if p==nil then p={} end
end`)
	if len(shadow) != 1 || strings.Contains(shadow[0].Message, "reuse attempt") {
		t.Fatalf("assumed builtin despite override: %+v", shadow)
	}
}

func TestSourceSnippetUsesResolvedSnapshotAndOriginalLines(t *testing.T) {
	bundle, err := sourcebundle.ResolveWithOverlay(t.TempDir(), "game.lua", map[string]string{
		"game.lua":     "require(\"step\")\nfunction _update() step() end\n",
		"src/step.lua": "function step()\n\tlocal tag = \"π界\"; local scratch = {}\nend\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	ds, err := Analyze(bundle)
	if err != nil || len(ds) != 1 {
		t.Fatalf("analysis: %v %+v", err, ds)
	}
	d := ds[0]
	if len(d.Snippet) != 3 || d.Snippet[0].Number != 1 || d.Snippet[1].Number != 2 || !strings.Contains(d.Snippet[1].Text, "π界") || d.Snippet[2].Text != "end" {
		t.Fatalf("wrong snapshot: %+v", d)
	}
	if d.Column != strings.Index(d.Snippet[1].Text, "{")+1 {
		t.Fatalf("column must remain a byte offset: %+v", d)
	}
	// A first-line constructor in the entry must not quote the generated source
	// marker as if it were a line in the user's file.
	bundle, err = sourcebundle.ResolveWithOverlay(t.TempDir(), "game.lua", map[string]string{
		"game.lua":     "function _update() local scratch={} end\nrequire(\"step\")\n",
		"src/step.lua": "function step() end\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	ds, err = Analyze(bundle)
	if err != nil || len(ds) != 1 {
		t.Fatalf("entry analysis: %v %+v", err, ds)
	}
	for _, line := range ds[0].Snippet {
		if strings.Contains(line.Text, "mtile source:") || line.Number > 2 {
			t.Fatalf("leaked generated marker: %+v", ds[0])
		}
	}
}
