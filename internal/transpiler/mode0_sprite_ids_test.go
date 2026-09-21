package transpiler

import (
	"slices"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
)

func TestMode0AcceptsSpriteIDFromExplicitFiniteAssignments(t *testing.T) {
	source := `
PLAYER_IDLE_1 = 1
PLAYER_RUN_3 = 35
player_sprite = PLAYER_IDLE_1

function _update()
  if btn(0) then
    player_sprite = PLAYER_RUN_3
  else
    player_sprite = PLAYER_IDLE_1
  end
end

function _draw()
  cls()
  spr(player_sprite, 24, 32)
end
`
	generated, report, err := TranspileWithReport("finite-sprite.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects || report.Mode0ObjectPacking ||
		report.Mode0OAMEntries != 1 || report.Mode0OBJLineCycles != 8 {
		t.Fatalf("report = %+v, want one ordinary Mode 0 OBJ", report)
	}
	if !strings.Contains(generated, "c.Spr(") || !strings.Contains(generated, "player_sprite") {
		t.Fatalf("generated code does not retain the runtime sprite selection:\n%s", generated)
	}
}

func TestMode0ResolvesChainedParenthesizedSpriteConstantsDeterministically(t *testing.T) {
	source := `
FRAME_BASE = 1
FRAME_ALIAS = (FRAME_BASE)
player_sprite = (FRAME_ALIAS)

function _draw()
  cls()
  spr(player_sprite, 0, 0)
end
`
	for run := 0; run < 20; run++ {
		_, report, err := TranspileWithReport("finite-constant-chain.lua", source, Options{
			Renderer: RendererMode0,
			SheetW:   16,
			SheetH:   8,
		})
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if report.Selected != RendererMode0 {
			t.Fatalf("run %d report = %+v, want Mode 0", run, report)
		}
	}
}

func TestMode0RejectsForwardReferencedTopLevelSpriteConstant(t *testing.T) {
	_, _, err := TranspileWithReport("forward-sprite-constant.lua", `
FRAME_ALIAS = FRAME_BASE
FRAME_BASE = 1
player_sprite = FRAME_ALIAS

function _draw()
  cls()
  spr(player_sprite, 0, 0)
end
`, Options{Renderer: RendererMode0, SheetW: 16, SheetH: 8})
	if err == nil || !strings.Contains(err.Error(), reasonMode0SpriteShape) ||
		!strings.Contains(err.Error(), "non-enumerated assignment") {
		t.Fatalf("error = %v, want forward-reference sprite proof rejection", err)
	}
}

func TestMode0AcceptsTopLevelLocalFiniteSpriteSet(t *testing.T) {
	source := `
local PLAYER_IDLE = 2
local PLAYER_RUN = 34
local player_sprite = PLAYER_IDLE

function _update()
  if btn(0) then
    player_sprite = PLAYER_RUN
  else
    player_sprite = PLAYER_IDLE
  end
end

function _draw()
  cls()
  spr(player_sprite, 0, 0)
end
`
	_, report, err := TranspileWithReport("finite-local-sprite.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("report = %+v, want Mode 0 objects", report)
	}
}

func TestMode0AcceptsDrawLocalLiteralSprite(t *testing.T) {
	source := `
function _draw()
  local player_sprite = 1
  cls()
  spr(player_sprite, 0, 0)
end
`
	_, report, err := TranspileWithReport("draw-local-literal.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || !report.Mode0Objects {
		t.Fatalf("report = %+v, want Mode 0 objects from a literal _draw local", report)
	}
}

func TestMode0AcceptsUnconditionalInitForFiniteSpriteSet(t *testing.T) {
	source := `
PLAYER_IDLE = 1
PLAYER_RUN = 35

function _init()
  player_sprite = PLAYER_IDLE
end

function _update()
  if btn(0) then player_sprite = PLAYER_RUN end
end

function _draw()
  cls()
  spr(player_sprite, 0, 0)
end
`
	_, report, err := TranspileWithReport("finite-init-sprite.lua", source, Options{
		Renderer: RendererAuto,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Reason != reasonAutoMode0Eligible {
		t.Fatalf("AUTO report = %+v, want proven Mode 0", report)
	}
}

func TestMode0RejectsUnprovenDynamicSpriteIDs(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "unassigned",
			source: `function _draw()
  cls()
  spr(player_sprite, 0, 0)
end`,
			want: "has no assignments",
		},
		{
			name: "arithmetic-next-frame",
			source: `player_sprite = 1
function _update()
  player_sprite = player_sprite + 1
end
function _draw()
  cls()
  spr(player_sprite, 0, 0)
end`,
			want: "non-enumerated assignment",
		},
		{
			name: "runtime-result",
			source: `player_sprite = 1
function _update()
  player_sprite = flr(rnd(10))
end
function _draw()
  cls()
  spr(player_sprite, 0, 0)
end`,
			want: "non-enumerated assignment",
		},
		{
			name: "conditional-init",
			source: `function _init()
  if btn(0) then player_sprite = 1 end
end
function _draw()
  cls()
  spr(player_sprite, 0, 0)
end`,
			want: "unconditional top-level or _init assignment",
		},
		{
			name: "draw-local",
			source: `function _draw()
  local player_sprite = foo()
  cls()
  spr(player_sprite, 0, 0)
end
function foo()
end`,
			want: "finite integer set",
		},
		{
			name: "mutable-frame-constant",
			source: `PLAYER_RUN = 35
player_sprite = 1
function _update()
  PLAYER_RUN = flr(rnd(10))
  player_sprite = PLAYER_RUN
end
function _draw()
  cls()
  spr(player_sprite, 0, 0)
end`,
			want: "non-enumerated assignment",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := TranspileWithReport(test.name+".lua", test.source, Options{
				Renderer: RendererMode0,
				SheetW:   240,
				SheetH:   160,
			})
			if err == nil || !strings.Contains(err.Error(), "[M0_SPR_SHAPE]") || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want M0_SPR_SHAPE containing %q", err, test.want)
			}
		})
	}
}

func TestMode0RejectsFiniteSpriteSetOutsideSourceBounds(t *testing.T) {
	source := `
PLAYER_SAFE = 1
PLAYER_EDGE = 29
player_sprite = PLAYER_SAFE
function _update()
  if btn(0) then player_sprite = PLAYER_EDGE end
end
function _draw()
  cls()
  spr(player_sprite, 0, 0, 2, 1)
end
`
	_, _, err := TranspileWithReport("finite-source-bounds.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "[M0_SPR_SHAPE]") || !strings.Contains(err.Error(), "finite id 29") {
		t.Fatalf("error = %v, want finite-set source-bound failure", err)
	}
}

func TestMode0ChecksEveryFiniteSpriteIDAgainstTextReservation(t *testing.T) {
	source := `
PLAYER_SAFE = 1
PLAYER_TEXT_TILE = 480
player_sprite = PLAYER_SAFE
function _update()
  if btn(0) then player_sprite = PLAYER_TEXT_TILE end
end
function _draw()
  cls()
  spr(player_sprite, 0, 0)
  print("A", 16, 0, 7)
end
`
	_, _, err := TranspileWithReport("finite-text-reservation.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err == nil || !strings.Contains(err.Error(), "[M0_RESOURCE_LIMIT]") || !strings.Contains(err.Error(), "480-511") {
		t.Fatalf("error = %v, want finite-set text-reservation failure", err)
	}
}

func TestAutoFallsBackForUnprovenDynamicSpriteID(t *testing.T) {
	source := `
player_sprite = 1
function _update()
  player_sprite = player_sprite + 1
end
function _draw()
  cls()
  spr(player_sprite, 0, 0)
end
`
	_, report, err := TranspileWithReport("dynamic-sprite-auto.lua", source, Options{
		Renderer: RendererAuto,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode4 || report.Reason != reasonAutoMode0Ineligible ||
		!strings.Contains(report.Diagnostic, "[M0_SPR_SHAPE]") ||
		!strings.Contains(report.Diagnostic, "dynamic-sprite-auto.lua:8") ||
		!strings.Contains(report.Diagnostic, "non-enumerated assignment") {
		t.Fatalf("AUTO report = %+v, want source-positioned Mode 4 fallback", report)
	}
}

func TestMode0DisablesPackedOBJCacheForDynamicSpriteSource(t *testing.T) {
	source := `
FRAME_A = 32
FRAME_B = 64
player_sprite = FRAME_A
function _update()
  if btn(0) then player_sprite = FRAME_B else player_sprite = FRAME_A end
end
function _draw()
  cls()
  spr(player_sprite, 0, 0, 2, 2)
end
`
	_, report, err := TranspileWithReport("dynamic-sprite-pack.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0ObjectPacking || report.Mode0OAMEntries != 4 || report.Mode0OBJLineCycles != 16 {
		t.Fatalf("report = %+v, want the ordinary four-entry metasprite path", report)
	}
}

func TestMode0InactiveZeroCandidateKeepsConservativeStaticObjectBudget(t *testing.T) {
	source := `
FRAME_HIDDEN = 0
FRAME_VISIBLE = 1
slot_sprite = FRAME_HIDDEN
function _update()
  if btn(0) then slot_sprite = FRAME_VISIBLE else slot_sprite = FRAME_HIDDEN end
end
function _draw()
  cls()
  spr(slot_sprite, 0, 0)
end
`
	_, report, err := TranspileWithReport("inactive-sprite-slot.lua", source, Options{
		Renderer: RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Mode0Objects || report.Mode0OAMEntries != 1 || report.Mode0OBJLineCycles != 8 {
		t.Fatalf("report = %+v, want conservative one-entry/eight-cycle proof despite the zero candidate", report)
	}
}

func TestMode0BoundedSpriteCounterClosesIncrementAndWrap(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []int
	}{
		{
			name: "nerdo-init-equality",
			source: `
function _init()
  first=1
  last=8
  sprite=0
end
function _update()
  sprite += 1
  if sprite == 9 then sprite = 1 end
end
function _draw()
  cls(0)
  spr(sprite,10,10)
end
`,
			want: []int{0, 1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name: "top-level-add-equality",
			source: `
sprite = 0
function _update()
  sprite = sprite + 1
  if sprite == 9 then sprite = 1 end
end
function _draw()
  cls()
  spr(sprite, 0, 0)
end
`,
			want: []int{0, 1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name: "greater-wrap",
			source: `
sprite = 0
function _update()
  sprite += 1
  if sprite > 8 then sprite = 1 end
end
function _draw()
  cls()
  spr(sprite, 0, 0)
end
`,
			want: []int{0, 1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name: "frozen-bound-and-reset",
			source: `
first = 1
last = 8
sprite = 0
function _update()
  sprite += 1
  if sprite == last + 1 then sprite = first end
end
function _draw()
  cls()
  spr(sprite, 0, 0)
end
`,
			want: []int{0, 1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name: "init-frozen-bound",
			source: `
function _init()
  first = 1
  last = 8
  sprite = 0
end
function _update()
  sprite += 1
  if sprite == last + 1 then sprite = first end
end
function _draw()
  cls()
  spr(sprite, 0, 0)
end
`,
			want: []int{0, 1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name: "inlined-helper",
			source: `
sprite = 0
function bump()
  sprite += 1
  if sprite == 9 then sprite = 1 end
end
function _update()
  bump()
end
function _draw()
  cls()
  spr(sprite, 0, 0)
end
`,
			want: []int{0, 1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name: "modulo-wrap",
			source: `
sprite = 0
function _update()
  sprite = (sprite + 1) % 8
end
function _draw()
  cls()
  spr(sprite, 0, 0)
end
`,
			want: []int{0, 1, 2, 3, 4, 5, 6, 7},
		},
		{
			name: "decrement-wrap",
			source: `
sprite = 8
function _update()
  sprite -= 1
  if sprite == 0 then sprite = 8 end
end
function _draw()
  cls()
  spr(sprite, 0, 0)
end
`,
			want: []int{1, 2, 3, 4, 5, 6, 7, 8},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := finiteSpriteIDs(t, test.source, "sprite")
			if !slices.Equal(got, test.want) {
				t.Fatalf("ids = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMode0AcceptsIncrementAndWrapOnSheetThatExcludesSentinel(t *testing.T) {
	source := `
sprite = 0
function _update()
  sprite += 1
  if sprite == 9 then sprite = 1 end
end
function _draw()
  cls()
  spr(sprite, 0, 0)
end
`
	_, report, err := TranspileWithReport("wrap-tight.lua", source, Options{
		Renderer: RendererAuto,
		SheetW:   72,
		SheetH:   8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Reason != reasonAutoMode0Eligible || !report.Mode0Objects {
		t.Fatalf("AUTO report = %+v, want proven Mode 0 without the wrap sentinel", report)
	}
}

func TestMode0AcceptsNerdoIncrementAndWrap(t *testing.T) {
	source := `
function _init()
  first=1
  last=8
  sprite=0
end

function _update()
  sprite += 1
  if sprite == 9 then sprite = 1 end
end

function _draw()
  cls(0)
  spr(sprite,10,10)
end
`
	_, report, err := TranspileWithReport("nerdo.lua", source, Options{
		Renderer: RendererAuto,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != RendererMode0 || report.Reason != reasonAutoMode0Eligible {
		t.Fatalf("AUTO report = %+v, want Nerdo wrap proven for Mode 0", report)
	}
}

func TestMode0RejectsBoundedCounterIfDrawRunsMidUpdate(t *testing.T) {
	source := `
sprite = 0
function _update()
  sprite += 1
  _draw()
  if sprite == 9 then sprite = 1 end
end
function _draw()
  cls()
  spr(sprite, 0, 0)
end
`
	chunk, err := luaparser.ParseString("draw-from-update.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, detail := newMode0SpriteIDAnalysis(chunk).finiteIDs(&luaparser.VarExpr{Name: "sprite"})
	if ok || !strings.Contains(detail, "calls _draw") {
		t.Fatalf("detail = %q, want bounded-counter proof to fail closed when _draw runs mid-update", detail)
	}
}

func finiteSpriteIDs(t *testing.T, source, name string) []int {
	t.Helper()
	chunk, err := luaparser.ParseString(t.Name()+".lua", source)
	if err != nil {
		t.Fatal(err)
	}
	ids, ok, detail := newMode0SpriteIDAnalysis(chunk).finiteIDs(&luaparser.VarExpr{Name: name})
	if !ok {
		t.Fatalf("finiteIDs(%s) failed: %s", name, detail)
	}
	return ids
}
