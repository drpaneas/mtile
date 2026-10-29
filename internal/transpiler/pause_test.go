package transpiler

import (
	rt "github.com/drpaneas/mtile/runtime"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPauseOnlyGeneratedLoop(t *testing.T) {
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		generated, report, err := TranspileWithReport("pause.lua", `function _draw() cls(0) end`, Options{Renderer: renderer, SheetW: 8, SheetH: 8})
		if err != nil {
			t.Fatal(err)
		}
		if renderer != RendererMode4 && report.Selected != RendererMode0 {
			t.Fatal(report)
		}
		for _, want := range []string{"ctx.PollInput()", "ctx.PollPauseMenu()", "if !paused {", "ctx.PreparePauseMenu()", "ctx.PresentPauseMenu()"} {
			if !strings.Contains(generated, want) {
				t.Fatal(want)
			}
		}
		for _, removed := range []string{"ConfigurePauseMenu", "PauseSave", "PauseLoad", "PauseExit", "__glu_pause"} {
			if strings.Contains(generated, removed) {
				t.Fatalf("obsolete menu action %s", removed)
			}
		}
	}
	if _, ok := Builtins["pause_menu"]; ok {
		t.Fatal("removed registration is still public")
	}
	if _, err := Transpile("old.lua", `function _init() pause_menu("store", "load") end`, Options{}); err == nil {
		t.Fatal("accepted obsolete registration")
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

func TestPauseEnableBuiltinAndGeneratedTransitions(t *testing.T) {
	if Builtins["pause"] != (BuiltinSpec{"PauseEnableV", 1, 1, 0}) || BuiltinRendererCompatibility("pause") != RendererIndependent {
		t.Fatal("pause API contract")
	}
	if _, exists := Builtins["pause_enable"]; exists || BuiltinRendererCompatibility("pause_enable") != RendererUnknown {
		t.Fatal("retired pause_enable name is still public")
	}
	if _, err := Transpile("old-name.lua", `function _init() pause_enable(true) end`, Options{}); err == nil {
		t.Fatal("retired pause_enable call was accepted")
	}
	const source = `local calls=0
function flag() calls+=1 return true end
function _init() pause(true) pause(flag()) end
function disable() pause(false) end
function _draw() cls(0) end`
	const harness = `package main
import("testing";rt "github.com/drpaneas/mtile/runtime";"github.com/drpaneas/mtile/runtime/hw")
func box[T rt.Number|rt.Value](v T)rt.Value {switch n:=any(v).(type){case rt.Number:return rt.NumVal(n);case rt.Value:return n};panic("kind")}
func TestTransitions(t *testing.T){
 hw.ResetForTest();var c rt.Context;var g Game
 poll:=func(keys uint16){hw.SetKeyInputForTest(0x3ff &^ keys);c.PollInput();c.PollPauseMenu()}
 poll(8);if c.PauseActive()||!c.Btn(rt.ButtonStart){t.Fatal("not disabled by default")}
 g.Init(&c);if box(g.calls)!=rt.IntVal(1){t.Fatal("dynamic enable evaluated more than once")}
 poll(8);if c.PauseActive(){t.Fatal("held Start paused after enable")}
 poll(0);poll(8);if !c.PauseActive(){t.Fatal("explicit enable did not pause")};c.PreparePauseMenu();c.PresentPauseMenu()
 g.disable(&c);poll(0);c.PresentPauseMenu();poll(8)
 if c.PauseActive()||!c.Btn(rt.ButtonStart){t.Fatal("explicit disable did not restore menu Start")}
}`
	for _, generic := range []bool{false, true} {
		for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
			generated, report, err := TranspileWithReport("enable.lua", source, Options{Renderer: renderer, SheetW: 8, SheetH: 8, DisableMonomorphicNumbers: generic})
			if err != nil {
				t.Fatal(err)
			}
			if renderer != RendererMode4 && report.Selected != RendererMode0 {
				t.Fatal(report)
			}
			dir := writeGeneratedPackage(t, generated)
			if err := os.WriteFile(filepath.Join(dir, "enable_test.go"), []byte(harness), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"test", "-run=TestTransitions"}
			if report.Selected == RendererMode0 {
				args = append(args, "-tags=mtile_mode0")
			}
			cmd := exec.Command("go", append(args, "./...")...)
			cmd.Dir, cmd.Env = dir, generatedGoCacheEnv()
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		}
	}
	for _, call := range []string{"pause()", "pause(true,false)"} {
		if _, err := Transpile("bad.lua", "function _init() "+call+" end", Options{}); err == nil {
			t.Fatal("accepted bad arity", call)
		}
	}
}
