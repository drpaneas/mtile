package transpiler

import (
	"strings"
	"testing"
)

const monomorphicFieldStringHarness = `package main

import (
  rt "github.com/drpaneas/mtile/runtime"
  "testing"
)

func TestFieldStringValue(t *testing.T) {
  var g Game
  var c rt.Context
  g.Init(&c)
  g.Update(&c)
  if !c.ValuesEqual(g.observed, c.InternStatic("wide")) {
    t.Fatalf("field initialized from string = %#v, want wide", g.observed)
  }
}
`

func TestMonomorphicFieldsRespectShadowedInitializerBindings(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{
			name: "local",
			body: `function _init()
  local width = "wide"
  box = {size = width}
end`,
		},
		{
			name: "parameter",
			body: `function make_box(width)
  return {size = width}
end
function _init()
  box = make_box("wide")
end`,
		},
		{
			name: "nested-block",
			body: `function _init()
  do
    local width = "wide"
    box = {size = width}
  end
end`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := `local width = 4
local box = {}
local observed = ""
` + test.body + `
function _update()
  observed = box.size
end
`
			for _, mode := range []struct {
				name string
				opts Options
			}{
				{name: "optimized"},
				{name: "generic", opts: Options{DisableMonomorphicNumbers: true}},
			} {
				t.Run(mode.name, func(t *testing.T) {
					generated, err := Transpile("shadowed-field.lua", source, mode.opts)
					if err != nil {
						t.Fatal(err)
					}
					runGeneratedHarness(t, generated, monomorphicFieldStringHarness, "TestFieldStringValue")
				})
			}
		})
	}
}

func TestMonomorphicFieldsUseNumericLocalAliases(t *testing.T) {
	generated, err := Transpile("local-numeric-fields.lua", `
local box = {}
local decoy = {}
local observed = ""
function _init()
  local width = 1.5
  local alias = (width)
  do
    local width = "wide"
    decoy = {label = width}
  end
  box = {size = alias, speed = width}
end
function _update()
  observed = box.size + box.speed
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	update := generatedSection(generated, "func (g *Game) gluUpdate(")
	if strings.Count(update, "c.TableGetStableNumber(") != 2 || strings.Contains(update, "c.TableGetStable(") {
		t.Fatalf("numeric aliases or restored outer scope lost native fields:\n%s", update)
	}
	runGeneratedHarness(t, generated, `package main

import (
  rt "github.com/drpaneas/mtile/runtime"
  "testing"
)

func TestNumericLocalFields(t *testing.T) {
  var g Game
  var c rt.Context
  g.Init(&c)
  g.Update(&c)
  if g.observed != rt.IntVal(3) {
    t.Fatalf("numeric field sum = %#v, want 3", g.observed)
  }
}
`, "TestNumericLocalFields")
}

func TestMonomorphicFieldsDoNotUseProvisionalObservations(t *testing.T) {
	generated, err := Transpile("mixed-field-alias.lua", `
local box = {}
local observed = ""
function _init()
  box = {size = 4}
  set_string()
  local alias = box.size
  box = {copied = alias}
end
function set_string()
  box = {size = "wide"}
end
function _update()
  observed = box.copied
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	update := generatedSection(generated, "func (g *Game) gluUpdate(")
	if strings.Contains(update, "c.TableGetStableNumber(") || !strings.Contains(update, "c.TableGetStable(") {
		t.Fatalf("mixed field's local alias was inferred from a provisional observation:\n%s", update)
	}
	runGeneratedHarness(t, generated, monomorphicFieldStringHarness, "TestFieldStringValue")
}
