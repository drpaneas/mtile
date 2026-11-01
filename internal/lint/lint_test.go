package lint

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/sourcebundle"
)

func analyzeSource(t *testing.T, source string) []Diagnostic {
	t.Helper()
	got, err := Analyze(sourcebundle.Bundle{Entry: "game.lua", Source: source})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func ruleIDs(ds []Diagnostic) []string {
	ids := []string{}
	for _, d := range ds {
		ids = append(ids, d.Rule)
	}
	return ids
}

// The teaching programs are the acceptance corpus: detect each supported
// before shape and remain quiet on its behavior-preserving after program.
func TestEffectiveGuidePrograms(t *testing.T) {
	data, err := os.ReadFile("../../docs/performance.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```lua\\n-- effective-example: ([^\\n]+)\\n(.*?)\\n```").FindAllStringSubmatch(string(data), -1)
	if len(blocks) != 22 {
		t.Fatalf("expected 22 documented programs, got %d", len(blocks))
	}
	expected := map[string]string{"reuse-before": "MTL001", "list-before": "MTL002", "palette-before": "MTL003", "asset-before": "MTL004", "text-before": "MTL005", "shape-before": "MTL006", "meaning-before": "MTL007"}
	for _, block := range blocks {
		t.Run(block[1], func(t *testing.T) {
			want := []string{}
			if id := expected[block[1]]; id != "" {
				want = append(want, id)
			}
			if got := ruleIDs(analyzeSource(t, block[2])); !reflect.DeepEqual(got, want) {
				t.Fatalf("rules = %v, want %v", got, want)
			}
		})
	}
}

func TestReachabilityAndConservativeBinding(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         []string
	}{
		{"transitive", `function scratch() local a = {} end
function step() scratch() end
function _update() step() end`, []string{"MTL001"}},
		{"recursive", `function a() b() local x = {} end
function b() a() end
function _draw() a() end`, []string{"MTL001"}},
		{"startup-only", `function setup() local a = {} pal() mset(0,0,2) end
function _init() setup() end
function _draw() cls(0) end`, []string{}},
		{"dead-helper", `function unused() local a = {} pal() end
function _update() end`, []string{}},
		{"comment-string", `function _draw()
-- pal(); local scratch = {}
print("pal() and mset(0,0,2)",0,0)
end`, []string{}},
		{"shadowed-builtin", `function unrelated(pal) return pal end
function _draw() pal() end`, []string{}},
		{"declared-builtin-name", `function pal() end
function _draw() pal() end`, []string{}},
		{"duplicate-builtin-name", `function pal() end
function pal() end
function _draw() pal() end`, []string{}},
		{"rebound-helper", `function step() local x = {} end
step = 1
function _update() step() end`, []string{}},
		{"method-call", `function _draw() object:pal() end`, []string{}},
		{"generic-loop", `function _update()
for enemy in all(enemies) do local scratch = {} end
end`, []string{"MTL001"}},
		{"literal-alias-global-proof", `a={1,2} local b=a
function _update() end`, []string{"MTL002"}},
		{"shadowed-list", `a={1,2}
function scratch(a) local b=a end
function _update() scratch(2) end`, []string{}},
		{"dynamic-table", `a={1,2} a={3}
function _update() local b=a end`, []string{}},
		{"mixed-different-slots", `a={1,"none"}
function _update() a[1]=2 a[2]="yes" end`, []string{}},
		{"mixed-same-slot", `a={1}
function _update() a[1]=false end`, []string{"MTL007"}},
		{"mixed-local-shadow", `a={1}
function unrelated(a) a[1]="none" end
function _update() a[1]=2 end`, []string{}},
		{"dynamic-map-edit", `function _update() mset(0,0,tile) end`, []string{}},
		{"literal-layer-map-edit", `function _update() mset(0,0,2,1) end`, []string{"MTL004"}},
		{"dense-palette-effect", `function _draw() for i=0,255 do pal(i,i,1) end end`, []string{}},
		{"display-palette-reset", `function _draw() pal(1) end`, []string{"MTL003"}},
		{"draw-palette-reset", `function _draw() pal(0) end`, []string{"MTL003"}},
		{"unsupported-palette-layer", `function _draw() pal(2) end`, []string{}},
		{"unknown-palette-layer", `function _draw() pal(layer) end`, []string{}},
		{"dynamic-rectangle", `function _draw()
for y=0,height do for x=0,10 do pset(x,y,7) end end
end`, []string{}},
		{"pixel-effects", `function _draw()
for y=0,5 do for x=0,10 do pset(x,y,7) other() end end
end`, []string{}},
		{"pixel-colour-varies", `function _draw()
for y=0,5 do for x=0,10 do pset(x,y,x) end end
end`, []string{}},
		{"reverse-rectangle", `function _draw()
for y=5,0,-1 do for x=0,10 do pset(x,y,7) end end
end`, []string{}},
		{"partial-label", `function _draw()
for i=1,2 do print(sub("SCORE",i,i),8,8,7) end
end`, []string{}},
		{"local-helper", `local function scratch() local a={} end
function _update() scratch() end`, []string{"MTL001"}},
		{"lowered-foreach", `function scratch(enemy) local a={} end
function _update() foreach(enemies,scratch) end`, []string{"MTL001"}},
		{"nested-control-flow", `function _update()
do while running do repeat local a={} until ready end end
end`, []string{"MTL001"}},
		{"label-index-shadow", `function _draw()
for i=1,5 do local i=1 print(sub("SCORE",i,i),8,8,7) end
end`, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ruleIDs(analyzeSource(t, tt.source)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
	oneLine := `a={0}; function _draw() a[1]="none" end; function _update() a[1]=1 end`
	var first []Diagnostic
	for i := 0; i < 25; i++ {
		got := analyzeSource(t, oneLine)
		if i > 0 && !reflect.DeepEqual(first, got) {
			t.Fatalf("same-line declarations changed report: %+v vs %+v", first, got)
		}
		first = got
	}
}

func TestBundledPositionsAndDeterminism(t *testing.T) {
	bundle, err := sourcebundle.ResolveWithOverlay(t.TempDir(), "game.lua", map[string]string{
		"game.lua":     "require(\"step\")\nfunction _update() step() end\n",
		"src/step.lua": "function step()\n  local scratch = {}\n  pal()\nend\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	var first []byte
	for i := 0; i < 25; i++ {
		ds, err := Analyze(bundle)
		if err != nil {
			t.Fatal(err)
		}
		if len(ds) != 2 || ds[0].Path != "src/step.lua" || ds[0].Line != 2 || ds[0].Column != 19 || ds[1].Line != 3 || ds[1].Column != 3 {
			t.Fatalf("source positions: %+v", ds)
		}
		data, _ := json.Marshal(ds)
		if i > 0 && string(first) != string(data) {
			t.Fatal("nondeterministic diagnostics")
		}
		first = data
	}
}

func TestMixedSlotReportIsStableAcrossCallbacks(t *testing.T) {
	source := `a={0}
function _draw() a[1]="none" end
function _update() a[1]=1 end`
	for i := 0; i < 25; i++ {
		ds := analyzeSource(t, source)
		if len(ds) != 1 || ds[0].Line != 2 {
			t.Fatalf("report changed: %+v", ds)
		}
	}
}

func TestParserErrorsAndRuleDocumentation(t *testing.T) {
	if _, err := Analyze(sourcebundle.Bundle{Entry: "bad.lua", Source: "function _draw("}); err == nil {
		t.Fatal("missing parser error")
	}
	data, err := os.ReadFile("../../docs/performance.md")
	if err != nil {
		t.Fatal(err)
	}
	headings := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## ") {
			headings[strings.Join(strings.Fields(strings.ToLower(line[3:])), "-")] = true
		}
	}
	seen := map[string]bool{}
	for _, rule := range Rules() {
		if seen[rule.ID] || !headings[rule.Guide] {
			t.Fatalf("duplicate or undocumented rule: %+v", rule)
		}
		seen[rule.ID] = true
	}
	copy := Rules()
	copy[0].ID = "bad"
	if Rules()[0].ID != "MTL001" {
		t.Fatal("caller mutated rule registry")
	}
}
