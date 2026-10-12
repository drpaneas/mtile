package transpiler

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/sourcebundle"
	rt "github.com/drpaneas/mtile/runtime"
)

const numericFieldSumHarness = `package main

import (
	"testing"

	rt "github.com/drpaneas/mtile/runtime"
)

func TestNumericFieldSum(t *testing.T) {
	var g Game
	var c rt.Context
	g.Init(&c)
	g.Update(&c)
	if g.observed != rt.FromInt(24) {
		t.Fatalf("numeric field sum = %#v, want 24", g.observed)
	}
}
`

func assertNativeFieldMath(t *testing.T, generated, section string) {
	t.Helper()
	body := generatedSection(generated, section)
	if !strings.Contains(body, "TableGetStableNumber(") || strings.Contains(body, "TableGetStable(") {
		t.Fatalf("expected native numeric field reads:\n%s", body)
	}
}

func TestNumericFieldWritesPromoteNativeReads(t *testing.T) {
	generated, err := Transpile("field-writes.lua", `
local player = {}
local observed = 0
function _init()
  player.x = 16
  player.y = 8
end
function _update()
  observed = player.x + player.y
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertNativeFieldMath(t, generated, "func (g *Game) gluUpdate(")
	update := generatedSection(generated, "func (g *Game) gluUpdate(")
	if !strings.Contains(generatedSection(generated, "func (g *Game) gluInit("), "TableSetStableNumber(") {
		t.Fatalf("numeric field writes kept generic stores:\n%s", generatedSection(generated, "func (g *Game) gluInit("))
	}
	if strings.Contains(update, "max(") || strings.Contains(update, "Max(") {
		t.Fatalf("native field reads still required a max coercion:\n%s", update)
	}
	runGeneratedHarness(t, generated, numericFieldSumHarness, "TestNumericFieldSum")
}

func TestNumericFieldLocalsDoNotNeedMaxCoercion(t *testing.T) {
	generated, err := Transpile("field-locals.lua", `
local player = {x = 16, y = 8, flip = false, hair = {}}
local observed = 0
function _update()
  local px = player.x
  local py = player.y
  observed = px + py
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	update := generatedSection(generated, "func (g *Game) gluUpdate(")
	assertNativeFieldMath(t, generated, "func (g *Game) gluUpdate(")
	if !strings.Contains(update, "rt.Number") {
		t.Fatalf("field copies were not kept as numeric locals:\n%s", update)
	}
	if strings.Contains(update, "Max(") || strings.Contains(update, "max(") {
		t.Fatalf("numeric locals still went through max:\n%s", update)
	}
	runGeneratedHarness(t, generated, numericFieldSumHarness, "TestNumericFieldSum")
}

func TestNumericConstructorParamsPromoteFields(t *testing.T) {
	generated, err := Transpile("spawn-fields.lua", `
local actor = {}
local observed = 0
function spawn(x, y)
  return {x = x, y = y, flip = false}
end
function _init()
  actor = spawn(16, 8)
end
function _update()
  observed = actor.x + actor.y
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertNativeFieldMath(t, generated, "func (g *Game) gluUpdate(")
	if !strings.Contains(generated, "func (g *Game) spawn(c *rt.Context, a0 rt.Number, a1 rt.Number)") {
		t.Fatalf("numeric spawn parameters were not specialized:\n%s", generatedSection(generated, "func (g *Game) spawn("))
	}
	runGeneratedHarness(t, generated, numericFieldSumHarness, "TestNumericFieldSum")
}

func TestNumericFieldsCopiedFromMarkersStayNative(t *testing.T) {
	generated, err := Transpile("marker-fields.lua", `
local actor = {}
local observed = 0
function spawn(x, y)
  return {x = x, y = y}
end
function _init()
  local marker = {x = 16, y = 8}
  actor = spawn(marker.x, marker.y)
end
function _update()
  observed = actor.x + actor.y
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertNativeFieldMath(t, generated, "func (g *Game) gluUpdate(")
	runGeneratedHarness(t, generated, numericFieldSumHarness, "TestNumericFieldSum")
}

func TestNumericParticleLoopUsesNativeFieldMath(t *testing.T) {
	generated, err := Transpile("particles.lua", `
local particles = {}
local observed = 0
function _init()
  add(particles, {x = 16, y = 8, vx = 1, vy = -1})
end
function _update()
  for p in all(particles) do
    p.x = p.x + p.vx
    p.y = p.y + p.vy
    observed = p.x + p.y
  end
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	update := generatedSection(generated, "func (g *Game) gluUpdate(")
	if strings.Count(update, "TableGetStableNumber(") < 4 || strings.Contains(update, "TableGetStable(") {
		t.Fatalf("particle field math lost native reads:\n%s", update)
	}
	if !strings.Contains(update, "TableSetStableNumber(") {
		t.Fatalf("particle field math lost native writes:\n%s", update)
	}
	runGeneratedHarness(t, generated, numericFieldSumHarness, "TestNumericFieldSum")
}

func TestNumericFieldConfirmationKeepsReadWriteAliases(t *testing.T) {
	generated, err := Transpile("particle-field-aliases.lua", `
local particles = {}
local observed = 0
function _init()
  add(particles, {x = 16, y = 8})
end
function _update()
  for p in all(particles) do
    local x = p.x
    local y = p.y
    p.x = x
    p.y = y
    local copy = {copied_x = x, copied_y = y}
    observed = copy.copied_x + copy.copied_y
  end
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertNativeFieldMath(t, generated, "func (g *Game) gluUpdate(")
	runGeneratedHarness(t, generated, numericFieldSumHarness, "TestNumericFieldSum")
}

func TestNumericFieldConfirmationPropagatesMixedAliases(t *testing.T) {
	for _, tc := range []struct {
		name, replacement, copy string
		wantNil                 bool
	}{
		{"string_local", `"wide"`, "local alias = box.size", false},
		{"nil_local", "nil", "local alias = box.size", true},
		{"string_return", `"wide"`, "local alias = read_size()", false},
		{"nil_return", "nil", "local alias = read_size()", true},
		{"string_parameter", `"wide"`, "local alias = identity(box.size)", false},
		{"nil_parameter", "nil", "local alias = identity(box.size)", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := fmt.Sprintf(`
local box = {}
local observed = ""
local replacement = %s
function read_size()
  return box.size
end
function identity(value)
  return value
end
function _init()
  box = {size = 4}
  box.size = replacement
  %s
  box = {copied = alias}
end
function _update()
  observed = box.copied
end
`, tc.replacement, tc.copy)
			generated, err := Transpile(tc.name+".lua", source, Options{})
			if err != nil {
				t.Fatal(err)
			}
			update := generatedSection(generated, "func (g *Game) gluUpdate(")
			if strings.Contains(update, "TableGetStableNumber(") || !strings.Contains(update, "TableGetStable(") {
				t.Fatalf("mixed field alias retained a provisional numeric type:\n%s", update)
			}
			harness := monomorphicFieldStringHarness
			if tc.wantNil {
				harness = `package main
import("testing"; rt "github.com/drpaneas/mtile/runtime")
func TestFieldStringValue(t *testing.T) {
 var g Game
 var c rt.Context
 g.Init(&c)
 g.Update(&c)
 if g.observed.Kind != rt.KindNil {
  t.Fatalf("field alias = %#v, want nil", g.observed)
 }
}
`
			}
			runGeneratedHarness(t, generated, harness, "TestFieldStringValue")
		})
	}
}

func TestNumericFieldConfirmationPropagatesBeyondProvisionalPassLimit(t *testing.T) {
	// Every field starts with numeric evidence. The unknown string store must
	// invalidate one more alias per confirmation pass, beyond the 32 passes
	// used to discover provisional field/signature facts.
	var source strings.Builder
	source.WriteString("local box = {}\nlocal replacement = \"wide\"\nlocal observed = \"\"\nfunction _init()\nbox = {")
	const last = 40
	for i := 0; i <= last; i++ {
		fmt.Fprintf(&source, "f%d = 4,", i)
	}
	source.WriteString("}\nbox.f0 = replacement\n")
	for i := 1; i <= last; i++ {
		fmt.Fprintf(&source, "local alias%d = box.f%d\nbox.f%d = alias%d\n", i, i-1, i, i)
	}
	fmt.Fprintf(&source, "end\nfunction _update()\nobserved = box.f%d\nend\n", last)
	generated, err := Transpile("long-field-alias-chain.lua", source.String(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	update := generatedSection(generated, "func (g *Game) gluUpdate(")
	if strings.Contains(update, "TableGetStableNumber(") {
		t.Fatalf("field alias chain retained a numeric type:\n%s", update)
	}
	runGeneratedHarness(t, generated, monomorphicFieldStringHarness, "TestFieldStringValue")
}

func TestNumericFieldWritesPoisonMixedStores(t *testing.T) {
	generated, err := Transpile("mixed-field-write.lua", `
local box = {}
local observed = ""
function _init()
  box.size = 4
  box.size = "wide"
end
function _update()
  observed = box.size
end
`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	update := generatedSection(generated, "func (g *Game) gluUpdate(")
	if strings.Contains(update, "TableGetStableNumber(") || !strings.Contains(update, "TableGetStable(") {
		t.Fatalf("string field write did not poison native reads:\n%s", update)
	}
	runGeneratedHarness(t, generated, monomorphicFieldStringHarness, "TestFieldStringValue")
}

func TestNumericFieldInferenceRespectsRollback(t *testing.T) {
	source := `
local player = {}
local observed = 0
function _init()
  player.x = 16
  player.y = 8
end
function _update()
  observed = player.x + player.y
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func number(value interface{}) rt.Number {
 switch n := value.(type) {
 case rt.Number:
  return n
 case rt.Value:
  return n.NumberOr(0)
 }
 panic("unexpected numeric storage")
}
func TestNumericFieldSum(t *testing.T) {
 var g Game
 var c rt.Context
 g.Init(&c)
 g.Update(&c)
 if number(g.observed) != rt.FromInt(24) {
  t.Fatalf("numeric field sum = %#v, want 24", g.observed)
 }
}
`
	for _, opts := range []Options{
		{DisableMonomorphicNumbers: true},
		{DisableStableTableFields: true},
	} {
		generated, err := Transpile("field-write-rollback.lua", source, opts)
		if err != nil {
			t.Fatal(err)
		}
		update := generatedSection(generated, "func (g *Game) gluUpdate(")
		if strings.Contains(update, "TableGetStableNumber(") {
			t.Fatalf("rollback still specialized field writes:\n%s", update)
		}
		runGeneratedHarness(t, generated, harness, "TestNumericFieldSum")
	}
}

func TestCelesteExamplesDropNumericCoercionWrappers(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "examples", "celeste"),
		filepath.Join("..", "..", "examples", "celeste-mode0"),
	}
	found := false
	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		found = true
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".lua") {
				return err
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if bytes.Contains(data, []byte("effect_number")) || bytes.Contains(data, []byte("max(-32768")) {
				t.Errorf("%s still uses a numeric coercion wrapper", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !found {
		t.Skip("celeste examples are not present")
	}
}

func TestCelesteControllerKeepsNumericFieldLocals(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "celeste")
	if _, err := os.Stat(filepath.Join(dir, "game.lua")); err != nil {
		t.Skip("celeste example is not present")
	}
	bundle, err := sourcebundle.Resolve(dir, "game.lua")
	if err != nil {
		t.Fatal(err)
	}
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{
		MapW: assets.MapW, MapH: assets.MapH, MapLayers: assets.MapLayers,
		SheetW: assets.SheetW, SheetH: assets.SheetH,
		Sheet: assets.SheetROM, Map: assets.MapROM, Flags: assets.FlagROM,
		Palette:   assets.PaletteROM[:],
		ObjSheetW: assets.ObjSheetW, ObjSheetH: assets.ObjSheetH, ObjSheet: assets.ObjSheetROM,
		AnimationFrames:      assets.AnimationFrames,
		PoseVariantBaseCount: assets.PoseVariantBaseCount,
		PoseVariantNames:     assets.PoseVariantNames,
	}
	for _, pose := range assets.PoseIndex {
		opts.PoseIndex = append(opts.PoseIndex, rt.PoseRange{
			Start: pose.Start, Count: pose.Count, FlipOffsetX: pose.FlipOffsetX, FlipOffsetY: pose.FlipOffsetY,
		})
	}
	for _, part := range assets.PoseParts {
		opts.PoseParts = append(opts.PoseParts, rt.PosePart{Tile: part.Tile, X: part.X, Y: part.Y, Flags: part.Flags})
	}
	for _, clip := range assets.AnimationClips {
		opts.AnimationClips = append(opts.AnimationClips, rt.AnimationClip{
			Name: clip.Name, Start: clip.Start, Count: clip.Count, FPS: clip.FPS, Loop: clip.Loop,
			FlipOffsetX: clip.FlipOffsetX, FlipOffsetY: clip.FlipOffsetY,
		})
	}
	generated, err := Transpile(bundle.Entry, bundle.Source, opts)
	if err != nil {
		t.Fatal(err)
	}
	update := generatedSection(generated, "func (g *Game) update_player(")
	if !strings.Contains(update, "TableGetStableNumber(") {
		t.Fatalf("celeste player update lost native field reads:\n%s", update)
	}
	if strings.Contains(update, "FromInt(-32768)") {
		t.Fatalf("celeste player update still emits a max(-32768) coercion:\n%s", update)
	}
}
