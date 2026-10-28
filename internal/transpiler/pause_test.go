package transpiler

import (
	"github.com/drpaneas/mtile/internal/luaparser"
	rt "github.com/drpaneas/mtile/runtime"
	"slices"
	"strings"
	"testing"
)

func TestPauseMenuRegistrationAndGeneratedLoop(t *testing.T) {
	const source = `local seen=0
function store() seen+=1 save_set(0,seen) return save_write("pause-v1") end
function restore() return save_read("pause-v1") end
function _init() pause_menu("store","restore") end
function _draw() cls(0) end`
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		generated, report, err := TranspileWithReport("pause.lua", source, Options{Renderer: renderer, SheetW: 8, SheetH: 8})
		if err != nil {
			t.Fatal(err)
		}
		if renderer != RendererMode4 && report.Selected != RendererMode0 {
			t.Fatal(report)
		}
		for _, want := range []string{"ctx.PollPauseMenu()", "if !paused {", "ctx.PresentPauseMenu()", "func (g *Game) __glu_pause_save", "func (g *Game) __glu_pause_load", "c.ConfigurePauseMenu()"} {
			if !strings.Contains(generated, want) {
				t.Fatal(want)
			}
		}
		update := generatedSection(generated, "func (g *Game) gluUpdate(")
		if strings.Contains(update, "g.store(") || strings.Contains(update, "g.restore(") {
			t.Fatal("analysis-only callback edges executed during gameplay")
		}
	}
	got, err := Transpile("plain.lua", `function _draw() cls(0) end`, Options{})
	if err != nil || !strings.Contains(got, "ctx.PollInput()") || !strings.Contains(got, "ctx.PollPauseMenu()") {
		t.Fatalf("game without btn has no menu: %v", err)
	}
}

func TestPauseHelpersDoNotReserveGameNames(t *testing.T) {
	got, err := Transpile("names.lua", `__glu_pause_save=0
function PauseSave() return 1 end
function PauseLoad() return 1 end
function __glu_pause_load() return 1 end
function _init() pause_menu("PauseSave","PauseLoad") end`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	compileGenerated(t, got)
}

func TestPauseCallbackAnalysisIncludesEachAlternativeAndNormalUpdates(t *testing.T) {
	const source = `sprite=0
function store() sprite=1 return 1 end
function restore() sprite=2 return 1 end
function _init() pause_menu("store","restore") end
function _update() sprite+=1 if sprite==5 then sprite=4 end end
function _draw() cls(0) spr(sprite,0,0) end`
	chunk, err := luaparser.ParseString("states.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	restore, _, err := lowerPauseAnalysisRoots(chunk)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	ids, ok, detail := newMode0SpriteIDAnalysis(chunk).finiteIDs(&luaparser.VarExpr{Name: "sprite"})
	if !ok || !slices.Equal(ids, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("menu alternatives lost sprite values: %v %v %s", ids, ok, detail)
	}
}

func TestPauseCallbackCanRunBeforeFirstUpdate(t *testing.T) {
	const source = `sprite=0
function store() sprite+=1 if sprite==3 then sprite=0 end return 1 end
function restore() sprite=0 return 1 end
function _init() pause_menu("store","restore") end
function _update() sprite=2 end
function _draw() cls(0) spr(sprite,0,0) end`
	chunk, err := luaparser.ParseString("before-update.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	restore, _, err := lowerPauseAnalysisRoots(chunk)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	ids, ok, detail := newMode0SpriteIDAnalysis(chunk).finiteIDs(&luaparser.VarExpr{Name: "sprite"})
	if !ok || !slices.Equal(ids, []int{0, 1, 2}) {
		t.Fatalf("callback before first update was lost: %v %v %s", ids, ok, detail)
	}
}

func TestPauseMenuRejectsInvalidCallbacks(t *testing.T) {
	for _, src := range []string{
		`function _init() pause_menu("missing","missing") end`,
		`function f(x) return 1 end function _init() pause_menu("f","f") end`,
		`function f() return 1 end function _update() pause_menu("f","f") end`,
		`function f() return 1 end function _init() if btn(0) then pause_menu("f","f") end end`,
		`function f() return 1 end function _init() pause_menu("f","f") pause_menu("f","f") end`,
		`function f() end function _init() pause_menu("f","f") end`,
	} {
		_, err := Transpile("bad-pause.lua", src, Options{})
		if err == nil || !strings.Contains(err.Error(), "PAUSE_MENU") || !strings.Contains(err.Error(), "bad-pause.lua:") {
			t.Fatalf("%s: %v", src, err)
		}
	}
}

func TestPauseCallbacksParticipateInRendererProof(t *testing.T) {
	const source = `function store() return 1 end
function restore() pset(0,0,7) return 1 end
function _init() pause_menu("store","restore") end
function _draw() cls(0) end`
	_, report, err := TranspileWithReport("pause-raster.lua", source, Options{Renderer: RendererAuto, SheetW: 8, SheetH: 8})
	if err != nil || report.Selected != RendererMode4 || report.Diagnostic == "" {
		t.Fatalf("hidden callback graphics bypassed AUTO: %+v %v", report, err)
	}
	if _, _, err = TranspileWithReport("pause-raster.lua", source, Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8}); err == nil {
		t.Fatal("forced Mode 0 accepted callback raster")
	}
}

func TestPauseOverlayPaletteReservationFailsClosed(t *testing.T) {
	for _, test := range []struct {
		source string
		sheet  []byte
	}{
		{`function _draw() cls() spr(0,0,0) end`, append([]byte{241}, make([]byte, 63)...)},
		{`function _draw() cls() print("A",0,0,245) end`, nil},
		{`function _update() sset(0,0,rnd(256)) end function _draw() cls() spr(0,0,0) end`, nil},
		{`function _update() sset(0,0,243) end function _draw() cls() spr(0,0,0) end`, nil},
	} {
		opts := Options{Renderer: RendererMode0, SheetW: 8, SheetH: 8, Sheet: test.sheet}
		_, _, err := TranspileWithReport("palette.lua", test.source, opts)
		if err == nil || !strings.Contains(err.Error(), "palette.lua:") || !strings.Contains(err.Error(), "M0_PAUSE_PALETTE") {
			t.Fatalf("missing positioned palette proof: %v", err)
		}
		opts.Renderer = RendererAuto
		_, report, err := TranspileWithReport("palette.lua", test.source, opts)
		if err != nil || report.Selected != RendererMode4 || !strings.Contains(report.Diagnostic, "M0_PAUSE_PALETTE") {
			t.Fatalf("missing exact fallback: %+v %v", report, err)
		}
	}
	// The same physical colors in BG-only data do not conflict with OBJ palettes.
	_, report, err := TranspileWithReport("background.lua", `function _draw() cls(241) map() end`, Options{Renderer: RendererAuto, SheetW: 8, SheetH: 8, Sheet: append([]byte{241}, make([]byte, 63)...), MapW: 1, MapH: 1, Map: []uint16{0}})
	if err != nil || report.Selected != RendererMode0 {
		t.Fatalf("background color was unnecessarily restricted: %+v %v", report, err)
	}
}

func TestPausePosePaletteReservationCoversBothSourceBanks(t *testing.T) {
	for _, separate := range []bool{false, true} {
		opts := psprMode0Opts(rt.PosePart{Tile: 0})
		pixels := make([]byte, 64)
		pixels[0] = 241
		if separate {
			opts.ObjSheetW, opts.ObjSheetH, opts.ObjSheet = 8, 8, pixels
		} else {
			opts.Sheet = pixels
		}
		_, _, err := TranspileWithReport("pose-palette.lua", `function _draw() cls() pspr(0,0,0) end`, opts)
		if err == nil || !strings.Contains(err.Error(), "M0_PAUSE_PALETTE") {
			t.Fatalf("separate=%t missing palette conflict: %v", separate, err)
		}
	}
}
