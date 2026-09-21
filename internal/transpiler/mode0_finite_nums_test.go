package transpiler

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
	"github.com/drpaneas/mtile/internal/sourcebundle"
)

func TestMode0FiniteNumsProvePublicBuiltinAndUserHelpers(t *testing.T) {
	source := `
function unit_cos(turn)
  return cos(turn)
end
function sheet_id(id)
  local tile = flr(id)
  return flr(tile / 16) * 30 + tile % 16
end
function pick_n(n)
  if n == 8 or n == 11 or n == 22 then
    return n
  end
  return 0
end
function _draw()
  cls()
  spr(sheet_id(pick_n(8)), 0, 0)
  spr(flr(8.5 + unit_cos(0)), 8, 0)
end
`
	_, report, err := TranspileWithReport("finite-nums.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("report = %+v, want Mode 0 objects from public builtins and unnamed helpers", report)
	}
}

func TestMode0FiniteNumsProveFieldIncrementUntilBound(t *testing.T) {
	source := `
particles = {}
function spawn()
  add(particles, {sprite = 29, x = 0, y = 0})
end
function _update()
  for p in all(particles) do
    local sprite = p.sprite + 0.2
    p.sprite = sprite
    if sprite >= 32 then
      del(particles, p)
    end
  end
end
function _draw()
  cls()
  for i = 1, 8 do
    local p = particles[i]
    if p then
      spr(flr(p.sprite), p.x, p.y)
    end
  end
end
`
	_, report, err := TranspileWithReport("field-increment.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("report = %+v, want Mode 0 objects from a bounded field increment", report)
	}
}

func TestMode0FiniteNumsRejectUnknownDynamicId(t *testing.T) {
	_, _, err := TranspileWithReport("unknown-id.lua", `
function _draw()
  cls()
  spr(player.unknown, 0, 0)
end
`, Options{Renderer: RendererMode0, SheetW: 240, SheetH: 160})
	if err == nil || !strings.Contains(err.Error(), "[M0_SPR_SHAPE]") {
		t.Fatalf("error = %v, want unknown field id rejected", err)
	}
}

func TestMode0FiniteNumsProveReturnGuardRange(t *testing.T) {
	source := `
particles = {}
function emit(width, height)
  if width < 1 or width > 5 or height < 1 or height > 5 then
    return
  end
  local id = 273 + (width - 1) * 5 + height - 1
  spr(id, 0, 0)
end
function _draw()
  cls()
  for i = 1, 8 do
    local p = particles[i]
    if p then
      emit(p.w, p.h)
    end
  end
end
`
	_, report, err := TranspileWithReport("guard-range.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("report = %+v, want Mode 0 objects from a returning comparison guard", report)
	}
}

func TestMode0FiniteNumsProveArrayIndexAndMinMax(t *testing.T) {
	source := `
local pal = {8, 11, 22}
function pick(n)
  return pal[n % 3 + 1]
end
function _draw()
  cls()
  spr(pick(phase), 0, 0)
  spr(max(1, min(4, 3)), 8, 0)
end
`
	_, report, err := TranspileWithReport("array-minmax.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("report = %+v, want Mode 0 objects from array indexes and min/max", report)
	}
}

func TestMode0FiniteNumsProveNotEqualGuardSet(t *testing.T) {
	source := `
function emit(radius)
  if radius ~= 1 and radius ~= 2 then
    return
  end
  spr(260 + radius, 0, 0)
end
function _draw()
  cls()
  emit(n)
end
`
	_, report, err := TranspileWithReport("not-equal-guard.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("report = %+v, want Mode 0 objects from a returning ~= guard", report)
	}
}

func TestMode0FiniteNumsProveCelesteSpriteFields(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "celeste-mode0")
	bundle, err := sourcebundle.Resolve(dir, "game.lua")
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := luaparser.ParseString(bundle.Entry, bundle.Source)
	if err != nil {
		t.Fatal(err)
	}
	eval := newMode0NumEval(chunk, newMode0SpriteIDAnalysis(chunk))
	sprite, ok := eval.field("sprite")
	if !ok {
		t.Fatalf("field sprite was not a finite set")
	}
	_, report, err := TranspileWithReport(bundle.Entry, bundle.Source, Options{
		Renderer:  RendererMode0,
		SheetW:    240,
		SheetH:    160,
		MapW:      60,
		MapH:      20,
		MapLayers: 4,
		Map:       nonEmptyMap(60, 20, 4, 0, 1, 2, 3),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("celeste-mode0 report = %+v, want Mode 0 objects", report)
	}
	t.Logf("sprite=%v celeste-mode0 OAM=%d line=%d", sprite, report.Mode0OAMEntries, report.Mode0OBJLineCycles)
}

func TestMode0HelperSprShapes(t *testing.T) {
	tests := []struct {
		name, source string
	}{
		{"flr-assign", `
function emit(id, x, y)
  id = flr(id)
  spr(id, x, y, 1, 1, false, false)
end
function _draw()
  cls()
  emit(11, 0, 0)
end
`},
		{"panel-hide", `
tile_ui_visible = false
tile_camera_x = 0
tile_camera_y = 0
function emit(id, x, y, fx, fy)
  id = flr(id)
  x = flr(x)
  y = flr(y)
  if fx ~= true then fx = false end
  if fy ~= true then fy = false end
  if tile_ui_visible then
    local sx = x - tile_camera_x
    local sy = y - tile_camera_y
    if sx <= 191 and sx + 7 >= 56 and sy <= 127 and sy + 7 >= 32 then
      return
    end
  end
  spr(id, x, y, 1, 1, fx, fy)
end
function _draw()
  cls()
  emit(11, 0, 0, false, false)
end
`},
		{"rect-unguarded", `
function emit(x1, y1, x2, y2, color)
  local width = x2 - x1 + 1
  local height = y2 - y1 + 1
  local variant = 0
  if color == 62 then variant = 1 end
  spr(273 + variant * 25 + (width - 1) * 5 + height - 1, x1, y1)
end
function _draw()
  cls()
  emit(0, 0, 2, 2, 249)
end
`},
		{"rect-guarded", `
function emit(x1, y1, x2, y2, color)
  local width = x2 - x1 + 1
  local height = y2 - y1 + 1
  if width < 1 or width > 5 or height < 1 or height > 5 then
    return
  end
  local variant = 0
  if color == 62 then variant = 1 end
  spr(273 + variant * 25 + (width - 1) * 5 + height - 1, x1, y1)
end
function _draw()
  cls()
  emit(0, 0, 2, 2, 249)
end
`},
		{"rect-from-fields", `
particles = {}
function spawn()
  add(particles, {x = 10, y = 10, t = 2})
end
function emit(x1, y1, x2, y2, color)
  local width = x2 - x1 + 1
  local height = y2 - y1 + 1
  if width < 1 or width > 5 or height < 1 or height > 5 then
    return
  end
  spr(273 + (width - 1) * 5 + height - 1, x1, y1)
end
function _draw()
  cls()
  for i = 1, 8 do
    local p = particles[i]
    if p then
      emit(p.x - p.t, p.y - p.t, p.x + p.t, p.y + p.t, 249)
    end
  end
end
`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, report, err := TranspileWithReport(test.name+".lua", test.source, Options{
				Renderer: RendererMode0,
				SheetW:   240,
				SheetH:   160,
			})
			if err != nil {
				t.Fatalf("%v", err)
			}
			if report.Selected != RendererMode0 || !report.Mode0Objects {
				t.Fatalf("report = %+v", report)
			}
		})
	}
}

func TestMode0SSprProvesSingletonTableIndexes(t *testing.T) {
	source := `
function _init()
  idle={0,8,40,80}
end
function _draw()
  cls(0)
  sspr(idle[1],idle[2],idle[3],idle[4],10,10)
end
`
	opts := Options{Renderer: RendererAuto, SheetW: 240, SheetH: 160}
	_, report, err := TranspileWithReport("sspr-table.lua", source, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("report = %+v, want Mode 0 objects from proven sspr table indexes", report)
	}
	if report.Mode0OAMEntries != 50 {
		t.Fatalf("OAM entries = %d, want 50 for a 40x80 metasprite", report.Mode0OAMEntries)
	}
}

func TestMode0SSprRejectsUnprovenTableIndex(t *testing.T) {
	_, report, err := TranspileWithReport("sspr-dynamic.lua", `
function _init()
  idle={0,8,40,80}
end
function _draw()
  cls(0)
  sspr(idle[n],idle[2],idle[3],idle[4],10,10)
end
`, Options{Renderer: RendererAuto, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "[M0_SPR_SHAPE]") {
		t.Fatalf("report = %+v, want AUTO Mode 4 with M0_SPR_SHAPE", report)
	}
}

func TestMode0SSprProvesNestedRecordArrayIndexes(t *testing.T) {
	source := `
function _init()
  counter=0
  idle={
    fr1={0,8,40,80},
    fr2={40,8,40,80},
    fr3={80,8,40,80},
    fr4={120,8,40,80}
  }
end
function _update()
  counter += 1
  if counter == 21 then
    counter=1
  end
end
function _draw()
  cls(0)
  if counter <=5 then sspr(idle.fr1[1],idle.fr1[2],idle.fr1[3],idle.fr1[4],10,10)
  elseif counter <= 10 then sspr(idle.fr2[1],idle.fr2[2],idle.fr2[3],idle.fr2[4],10,10)
  elseif counter <= 15 then sspr(idle.fr3[1],idle.fr3[2],idle.fr3[3],idle.fr3[4],10,10)
  else sspr(idle.fr4[1],idle.fr4[2],idle.fr4[3],idle.fr4[4],10,10) end
end
`
	_, report, err := TranspileWithReport("sspr-nested.lua", source, Options{
		Renderer: RendererAuto, SheetW: 240, SheetH: 160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("report = %+v, want Mode 0 from nested idle.fr1[1] frame tables", report)
	}
	if report.Mode0OAMEntries != 50 {
		t.Fatalf("OAM entries = %d, want 50 for the worst exclusive 40x80 frame", report.Mode0OAMEntries)
	}
}

func TestMode0SSprRejectsMutatedNestedRecordArray(t *testing.T) {
	_, report, err := TranspileWithReport("sspr-nested-mut.lua", `
function _init()
  idle={fr1={0,8,40,80}}
end
function _update()
  idle.fr1[1]=8
end
function _draw()
  cls(0)
  sspr(idle.fr1[1],idle.fr1[2],idle.fr1[3],idle.fr1[4],10,10)
end
`, Options{Renderer: RendererAuto, SheetW: 240, SheetH: 160})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "[M0_SPR_SHAPE]") {
		t.Fatalf("report = %+v, want AUTO Mode 4 when idle.fr1 is mutated", report)
	}
}
