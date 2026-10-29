package transpiler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveBuiltinContractAndLiteralKeys(t *testing.T) {
	for name, spec := range map[string]BuiltinSpec{
		"save_get": {"SaveGetV", 1, 1, 1}, "save_set": {"SaveSetV", 2, 2, 1},
		"save_read": {"SaveReadV", 1, 2, 1}, "save_write": {"SaveWriteV", 1, 2, 1},
		"save_pending": {"SavePending", 0, 0, 1},
	} {
		if Builtins[name] != spec || BuiltinRendererCompatibility(name) != RendererIndependent {
			t.Fatalf("%s contract", name)
		}
	}
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		for _, source := range []string{
			`function _init() save_read(42) end`,
			`key="my-game-v1" function _init() save_write(key) end`,
			`function _init() save_read("") end`,
			`function _init() save_write("012345678901234567890123456789012") end`,
		} {
			_, _, err := TranspileWithReport("save.lua", source+"\nfunction _draw() cls(0) end", Options{Renderer: renderer, SheetW: 8, SheetH: 8})
			if err == nil || !strings.Contains(err.Error(), "SAVE_KEY") || !strings.Contains(err.Error(), "save.lua:1:") {
				t.Fatalf("%s: %v", renderer, err)
			}
		}
	}
}

func TestSaveGeneratedRuntimeAndRendererSelection(t *testing.T) {
	const source = `
local loaded = 0
local written = 0
local value = 0
local accepted = false
function _init()
  loaded = save_read(("test-v1"))
  accepted = save_set(63, -12.5)
  written = save_write("test-v1")
  save_set(63, 0)
  loaded = save_read("test-v1")
  value = save_get(63)
end
function _draw() cls(0) end
`
	const harness = `package main
import("testing";rt "github.com/drpaneas/mtile/runtime";"github.com/drpaneas/mtile/runtime/hw")
func TestSave(t *testing.T) {
 hw.ResetSRAMForTest(255)
 var g Game;var c rt.Context;var memory rt.SaveMemory;c.AttachSaveMemory(&memory);g.Init(&c)
 if g.loaded!=rt.IntVal(1)||g.written!=rt.IntVal(1)||g.value!=rt.NumVal(rt.FromFloat(-12.5))||g.accepted!=rt.BoolVal(true) {t.Fatalf("save state: %#v",g)}
 // An unavailable device returns failure without losing the staging buffer.
 hw.SetSRAMWriteLimitForTest(0)
 if c.SaveWrite("test-v1")!=rt.SaveIOError||c.SaveGetV(rt.IntVal(63))!=rt.NumVal(rt.FromFloat(-12.5)) {t.Fatal("write failure")}
 hw.SetSRAMWriteLimitForTest(-1)
}
`
	for _, renderer := range []Renderer{RendererMode4, RendererMode0, RendererAuto} {
		t.Run(renderer.String(), func(t *testing.T) {
			generated, report, err := TranspileWithReport("save.lua", source, Options{Renderer: renderer, SheetW: 8, SheetH: 8, DisableMonomorphicNumbers: true})
			if err != nil {
				t.Fatal(err)
			}
			if renderer != RendererMode4 && report.Selected != RendererMode0 {
				t.Fatalf("saving changed renderer: %+v", report)
			}
			for _, want := range []string{"var saveMemory rt.SaveMemory", "ctx.AttachSaveMemory(&saveMemory)", `c.SaveReadV("test-v1", rt.IntVal(0))`, `c.SaveWriteV("test-v1", rt.IntVal(0))`} {
				if !strings.Contains(generated, want) {
					t.Fatal(want)
				}
			}
			dir := writeGeneratedPackage(t, generated)
			if err := os.WriteFile(filepath.Join(dir, "save_test.go"), []byte(harness), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"test", "-run=TestSave"}
			if report.Selected == RendererMode0 {
				args = append(args, "-tags=mtile_mode0")
			}
			cmd := exec.Command("go", append(args, "./...")...)
			cmd.Dir, cmd.Env = dir, generatedGoCacheEnv()
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
	pendingOnly, err := Transpile("pending.lua", `function _update() if save_pending() then end end`, Options{})
	if err != nil || strings.Contains(pendingOnly, "var saveMemory") {
		t.Fatalf("reminder alone linked storage: %v", err)
	}
	generated, err := Transpile("plain.lua", "function _draw() cls(0) end", Options{})
	if err != nil || strings.Contains(generated, "saveMemory") {
		t.Fatalf("plain cartridge links saving: %v", err)
	}
}

func TestSaveAritiesAndAdjustedFieldArguments(t *testing.T) {
	for _, call := range []string{"save_get()", "save_get(0, 1)", "save_set(0)", "save_set(0, 1, 2)", "save_read()", "save_write(\"key\", 1, 2)", "save_pending(1)"} {
		if _, err := Transpile("arity.lua", "function _init() "+call+" end", Options{}); err == nil {
			t.Fatalf("accepted %s", call)
		}
	}
	generated, err := Transpile("adjust.lua", `function pair() return 0, 3.25 end function _init() save_set(pair()) end`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, fmt.Sprintf(`package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestAdjusted(t *testing.T) {var g Game;var c rt.Context;var m rt.SaveMemory;c.AttachSaveMemory(&m);g.Init(&c);if c.SaveGetV(rt.IntVal(0))!=rt.NumVal(rt.FromFloat(%g)){t.Fatal("adjusted arguments")}}
`, 3.25), "TestAdjusted")
}

func TestSaveNumericIndexEvaluatesOnce(t *testing.T) {
	const source = `
local seen = 0
local result = 0
function next_index() seen += 1 return 0 end
function _init()
  save_set(0, -3.25)
  result = save_get(next_index()) + save_get(next_index())
end
`
	const harness = `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func box[T rt.Number|rt.Value](v T)rt.Value {switch n:=any(v).(type){case rt.Number:return rt.NumVal(n);case rt.Value:return n};panic("kind")}
func TestOnce(t *testing.T){var c rt.Context;var m rt.SaveMemory;c.AttachSaveMemory(&m);var g Game;g.Init(&c);if box(g.seen)!=rt.IntVal(2)||box(g.result)!=rt.NumVal(rt.FromFloat(-6.5)){t.Fatalf("index evaluation: %#v",g)}}`
	for _, generic := range []bool{false, true} {
		generated, err := Transpile("once.lua", source, Options{DisableMonomorphicNumbers: generic})
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, generated, harness, "TestOnce")
	}
}

func TestSaveSlotsDynamicArgumentAndPending(t *testing.T) {
	const source = `local calls=0
local status=0
local value=0
local pending=false
function slot() calls+=1 return 2 end
function _init()
 save_set(0,-7.25)
 status=save_write("slots-v1",slot())
 pending=save_pending()
 save_set(0,9)
 status=save_read("slots-v1",slot())
 value=save_get(0)
 status=save_read("slots-v1",false)
end
function _draw() cls(0) end`
	const harness = `package main
import("testing";rt "github.com/drpaneas/mtile/runtime";"github.com/drpaneas/mtile/runtime/hw")
func box[T rt.Number|rt.Value](v T)rt.Value {switch n:=any(v).(type){case rt.Number:return rt.NumVal(n);case rt.Value:return n};panic("kind")}
func TestSlots(t *testing.T){hw.ResetSRAMForTest(255);var g Game;var c rt.Context;var m rt.SaveMemory;c.AttachSaveMemory(&m);g.Init(&c)
if box(g.calls)!=rt.IntVal(2)||box(g.status)!=rt.IntVal(-5)||box(g.value)!=rt.NumVal(rt.FromFloat(-7.25))||g.pending!=rt.BoolVal(true){t.Fatalf("slot args: %#v",g)}
if c.SaveRead("slots-v1")!=rt.SaveEmpty {t.Fatal("slot 2 aliased slot 0")}}
`
	for _, generic := range []bool{false, true} {
		for _, renderer := range []Renderer{RendererMode0, RendererMode4, RendererAuto} {
			generated, report, err := TranspileWithReport("slots.lua", source, Options{Renderer: renderer, SheetW: 8, SheetH: 8, DisableMonomorphicNumbers: generic})
			if err != nil {
				t.Fatal(err)
			}
			if renderer != RendererMode4 && report.Selected != RendererMode0 {
				t.Fatal(report)
			}
			dir := writeGeneratedPackage(t, generated)
			if err := os.WriteFile(filepath.Join(dir, "slots_test.go"), []byte(harness), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"test", "-run=TestSlots"}
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
	for _, source := range []string{
		`function two() return 1,2 end function _init() save_read("key",two()) end`,
		`function _init() save_write("key",0,1) end`,
	} {
		if _, err := Transpile("args.lua", source, Options{}); err == nil {
			t.Fatal("accepted expanded extra arguments")
		}
	}
}
