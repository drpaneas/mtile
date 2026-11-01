package lint

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/sourcebundle"
)

func discarded(ds []Diagnostic) []Diagnostic {
	var result []Diagnostic
	for _, d := range ds {
		if d.Rule == "MTL008" {
			result = append(result, d)
		}
	}
	return result
}

func TestDiscardedConstructionDoesNotIgnoreObservationsOrEffects(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"field-read", `local model={part={}}
local length=#model.part
model.part={}`},
		{"child-alias", `local model={part={}}
saved=model.part
model.part={}`},
		{"parent-alias-before-field", `local model={}
saved=model
model.part={}
model.part={}`},
		{"parent-alias-after-field", `local model={part={}}
saved=model
model.part={}`},
		{"returned-value", `local model={}
return model
model={}`},
		{"unknown-observer", `state={part={}}
observe_external()
state.part={}`},
		{"unknown-argument", `state={part={}}
observe_external(state.part)
state.part={}`},
		{"random-initializer", `state={part={value=rnd()}}
state.part={}`},
		{"audio-between", `state={part={}}
sfx(1)
state.part={}`},
		{"dynamic-index-read", `state={part={}}
local old=state[index]
state.part={}`},
		{"dynamic-index-write", `state={part={}}
state[index]=1
state.part={}`},
		{"constructor-retains-old", `state={part={}}
state.part={previous=state.part}`},
		{"new-local-is-not-a-reassignment", `local scratch={}
local scratch={}
value=#scratch`},
		{"loop-local-identity", `for i=1,4 do local scratch={} end`},
		{"return-in-do", `local scratch={}
do return end
scratch={}`},
		{"return-in-loop", `state={part={}}
for i=1,2 do return end
state.part={}`},
		{"all-branches-return", `state={part={}}
if ready then return else return end
state.part={}`},
		{"condition-boundary", `state={part={}}
if ready then score=1 end
state.part={}`},
		{"loop-induction-mutated", `state={part={}}
for i=1,3 do i=i+1 end
state.part={}`},
		{"false-branch", `if false then state={} state={} end`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds := discarded(analyzeSource(t, "function _update()\n"+tt.body+"\nend"))
			if len(ds) != 0 {
				t.Fatalf("unsupported ownership/effect proof: %+v", ds)
			}
		})
	}
}

func TestDiscardedConstructionsRespectHelpersAndLexicalScopes(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         int
	}{
		{"sibling-field-read", `function _update()
state={x=1,part={}}
local x=state.x
state.part={}
end`, 1},
		{"different-helpers-and-values", `function build_alpha() state.part={value=1} end
function build_beta() state.part={value=2} end
function _update()
state={}
build_alpha()
state.y=42
build_beta()
total=state.part.value
end`, 1},
		{"nested-object-path", `function _update()
state={skin={trail={}}}
state.skin.trail={}
end`, 1},
		{"replaced-by-nil", `function _update()
local scratch={}
scratch=nil
end`, 1},
		{"static-index-is-same-field", `function _update()
state={part={}}
state["part"]={}
end`, 1},
		{"arbitrary-string-key", `function _update()
state={["part.one"]={}}
state["part.one"]={}
end`, 1},
		{"unknown-parent-identity", `function _update()
state.part={}
state.part={}
end`, 0},
		{"chunk-local-declared-after-helper", `function replace() state.part={} end
local state={}
function _update()
state={part={}}
replace()
state.part={}
end`, 1},
		{"chunk-local-captured-by-helper", `local state={}
function inspect() local used=#state.part end
function _update()
state={part={}}
inspect()
state.part={}
end`, 0},
		{"local-shadow-is-a-different-object", `local model={}
function _update()
local model={}
do local model={} end
local used=#model
end`, 0},
		{"callee-does-not-capture-caller-local", `function replace() state={} end
function _update()
local state={}
replace()
local used=#state
end`, 0},
		{"parameter-call-is-a-barrier", `function inspect(x) return x end
function _update()
state={part={}}
inspect(state)
state.part={}
end`, 0},
		{"helper-observes-result", `function inspect() local length=#state.part end
function _update()
state={part={}}
inspect()
state.part={}
end`, 0},
		{"helper-copies-child", `function retain() saved=state.part end
function _update()
state={part={}}
retain()
state.part={}
end`, 0},
		{"helper-with-unknown-effects", `function fill() state.part={} sfx(1) end
function _update()
state={}
fill()
fill()
end`, 0},
		{"constructor-argument-escapes-parent", `function fill() add(state.part,state) end
function _update()
state={part={}}
fill()
state.part={}
end`, 0},
		{"opaque-helper-control-flow", `function maybe_read() if ready then value=#state.part end end
function _update()
state={part={}}
maybe_read()
state.part={}
end`, 0},
		{"recursive-helper", `function recur() recur() end
function _update()
state={part={}}
recur()
state.part={}
end`, 0},
		{"inside-one-entered-branch", `function _update()
if ready then
state={part={}}
state.part={}
end
end`, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds := discarded(analyzeSource(t, tt.source))
			if len(ds) != tt.want {
				t.Fatalf("got %d findings, want %d: %+v", len(ds), tt.want, ds)
			}
		})
	}
}

func TestDiscardedConstructionGuidePair(t *testing.T) {
	data, err := os.ReadFile("../../docs/lint.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```lua\\n-- lint-example: discarded-(before|after)\\n(.*?)\\n```").FindAllStringSubmatch(string(data), -1)
	if len(blocks) != 2 {
		t.Fatalf("need before/after programs, got %d", len(blocks))
	}
	for _, block := range blocks {
		ds := discarded(analyzeSource(t, block[2]))
		want := 0
		if block[1] == "before" {
			want = 1
		}
		if len(ds) != want {
			t.Fatalf("%s guide example: %+v", block[1], ds)
		}
	}
}

func TestDiscardedConstructionsAcrossModulesHaveBothSourceLocations(t *testing.T) {
	bundle, err := sourcebundle.ResolveWithOverlay(t.TempDir(), "game.lua", map[string]string{
		"game.lua":       "require(\"ribbon\")\nfunction reset_actor()\n actor={x=1,y=2}\n assemble_ribbon()\n actor.y=9\n assemble_ribbon()\nend\nfunction _update() if ready then reset_actor() end end\n",
		"src/ribbon.lua": "function assemble_ribbon()\n actor.ribbon={}\n for n=1,3 do add(actor.ribbon,{x=actor.x,y=actor.y}) end\nend\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	var first []byte
	for i := 0; i < 20; i++ {
		all, err := Analyze(bundle)
		if err != nil {
			t.Fatal(err)
		}
		ds := discarded(all)
		if len(ds) != 1 || ds[0].Path != "src/ribbon.lua" || ds[0].Line != 2 {
			t.Fatalf("wrong primary constructor: %+v", ds)
		}
		seen := map[int]bool{}
		for _, related := range ds[0].Related {
			if related.Path == "game.lua" {
				seen[related.Line] = true
			}
			if len(related.Snippet) == 0 {
				t.Fatalf("missing related source: %+v", related)
			}
		}
		if !seen[4] || !seen[6] {
			t.Fatalf("both call sites are needed: %+v", ds[0])
		}
		data, _ := json.Marshal(ds)
		if i > 0 && string(first) != string(data) {
			t.Fatal("unstable proof witness")
		}
		first = data
	}
}

func TestDiscardedConstructionsThroughRenamedHelpers(t *testing.T) {
	source := `function assemble_ribbon()
  actor.ribbon={}
  for n=1,3 do add(actor.ribbon,{x=actor.x,y=actor.y}) end
end
function reset_actor()
  actor={x=10,y=20,ribbon={}}
  assemble_ribbon()
end
function enter_scene()
  reset_actor()
  actor.y=100
  assemble_ribbon()
end
function _update() if ready then enter_scene() end end`
	ds := discarded(analyzeSource(t, source))
	if len(ds) != 2 {
		t.Fatalf("expected placeholder and repeated initializer findings: %+v", ds)
	}
	for _, d := range ds {
		if d.Severity != "advice" || !strings.Contains(d.Message, "actor.ribbon") || len(d.Related) < 2 {
			t.Fatalf("missing proof: %+v", d)
		}
		if len(d.CallPath) == 0 || d.CallPath[0] != "_update" {
			t.Fatalf("missing frame provenance: %+v", d)
		}
	}
}

func TestDiscardedConstructionStraightLine(t *testing.T) {
	ds := discarded(analyzeSource(t, `function _update()
 local scratch={1,2}
 scratch={3,4}
 total=scratch[1]
end`))
	if len(ds) != 1 || ds[0].Line != 2 || len(ds[0].Related) != 1 || ds[0].Related[0].Line != 3 {
		t.Fatalf("wrong write relationship: %+v", ds)
	}
}
