//go:build runtimeperfdiagnostic

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// This checks an independent runtime oracle as well as paired pixels: both
// renderers must report zero failed SRAM/field/status checks on every sample.
func TestSaveTargetDiagnostic(t *testing.T) {
	helper, err := filepath.Abs(filepath.Join("..", "..", "build", "player", "mtile-player"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(helper); err != nil {
		t.Skip("build mtile-player first")
	}
	project, err := stageProjectWithSource(filepath.Join("..", "..", "examples", "save-game"), filepath.Join("testdata", "workloads", "saving.lua"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(project)
	out := os.Getenv("MTILE_SAVE_OUT")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	const warmup, samples = 8, 120
	b4, c4, err := buildAndCapture(service, transpiler.RendererMode4, project, helper, out, warmup, samples)
	if err != nil {
		t.Fatal(err)
	}
	b0, c0, err := buildAndCaptureAtFrames(service, transpiler.RendererMode0, project, helper, out, warmup, samples, c4.frameNumbers)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSynchronizedFrames(c4.frameNumbers, c0.frameNumbers); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]capture{"mode4": c4, "mode0": c0} {
		mask := int32(0)
		for i, watches := range c.watches {
			found := false
			for _, watch := range watches {
				if watch.Name == "checked_slots" && watch.Kind == 2 {
					mask = watch.Raw >> 16
				}
				if watch.Name == "errors" {
					found = true
					if watch.Kind != 2 || watch.Raw != 0 {
						t.Fatalf("%s sample %d SRAM failures: %+v", name, i, watch)
					}
				}
			}
			if !found {
				t.Fatalf("%s missing errors oracle at sample %d", name, i)
			}
		}
		if mask != 15 {
			t.Fatalf("%s did not exercise slots 0,1,2,31: %d", name, mask)
		}
	}
	differences := compareFrames(c4.frames, c0.frames)
	if differences != 0 {
		t.Fatalf("%d differing pixels", differences)
	}
	environment, err := benchmarkEnvironment(service, project, helper)
	if err != nil {
		t.Fatal(err)
	}
	romHash := func(path string) string {
		hash, err := fileFingerprint(path)
		if err != nil {
			t.Fatal(err)
		}
		return hash
	}
	writePSprJSON(t, filepath.Join(out, "save-oracle.json"), map[string]any{
		"schemaVersion": 1, "environment": environment,
		"capturedFrames": samples, "differingPixels": differences, "zeroSaveErrors": true, "checkedSlots": []int{0, 1, 2, 31}, "defaultLiteralAndDynamicSlots": true, "invalidSlotsRejected": true,
		"mode0": map[string]any{"romSha256": romHash(b0.ROMPath), "medianCycles": percentile(c0.cycles, 50), "p95Cycles": percentile(c0.cycles, 95)},
		"mode4": map[string]any{"romSha256": romHash(b4.ROMPath), "medianCycles": percentile(c4.cycles, 50), "p95Cycles": percentile(c4.cycles, 95)},
	})
}

func TestSaveExampleTargetDiagnostic(t *testing.T) {
	helper, _ := filepath.Abs(filepath.Join("..", "..", "build", "player", "mtile-player"))
	if _, err := os.Stat(helper); err != nil {
		t.Skip("build mtile-player first")
	}
	project, _ := filepath.Abs(filepath.Join("..", "..", "examples", "save-game"))
	out := os.Getenv("MTILE_SAVE_OUT")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	var reference [][]byte
	hashes := map[string]string{}
	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode4, transpiler.RendererAuto} {
		service.Renderer = renderer
		result, err := service.Build(project, filepath.Join(out, "example-"+renderer.String()+".gba"))
		if err != nil {
			t.Fatal(err)
		}
		if result.Renderer != transpiler.RendererMode4 {
			t.Fatal("dynamic names must use exact Mode 4 text")
		}
		hashes[renderer.String()], err = fileFingerprint(result.ROMPath)
		if err != nil {
			t.Fatal(err)
		}
		session, err := emulator.Start(helper, result.ROMPath)
		if err != nil {
			t.Fatal(err)
		}
		step := func(keys uint16, n int, reset bool) emulator.Frame {
			var frame emulator.Frame
			for i := 0; i < n; i++ {
				frame, err = session.Step(keys, reset && i == 0)
				if err != nil {
					t.Fatal(err)
				}
			}
			return frame
		}
		press := func(keys uint16) emulator.Frame { step(0, 8, false); return step(keys, 8, false) }
		watch := func(f emulator.Frame, name string) int32 {
			for _, w := range f.Telemetry.Watches {
				if w.Name == name && w.Kind == 2 {
					return w.Raw >> 16
				}
			}
			t.Fatalf("missing watch %s", name)
			return 0
		}
		require := func(f emulator.Frame, name string, want int32) {
			if got := watch(f, name); got != want {
				t.Fatalf("%s %s=%d want %d", renderer, name, got, want)
			}
		}
		fresh := step(0, 120, false)
		require(fresh, "mode", 0)
		frames := [][]byte{fresh.RGBA}
		writePNG(filepath.Join(out, renderer.String()+"-files.png"), fresh.RGBA)
		for slot := 0; slot < 3; slot++ {
			if slot > 0 {
				press(emulator.KeyDown)
			}
			naming := press(emulator.KeyStart)
			require(naming, "mode", 1)
			for i := 0; i < slot; i++ {
				press(emulator.KeyRight)
			}
			press(emulator.KeyA) // one A/B/C letter in the name
			if slot == 0 {
				writePNG(filepath.Join(out, renderer.String()+"-name.png"), step(0, 4, false).RGBA)
			}
			registered := press(emulator.KeyStart) // Menu Start registers; it cannot pause.

			require(registered, "mode", 4)
			require(registered, "saves", 1)
			completed := step(0, 320, false)
			require(completed, "mode", 0)
			if bytes.Equal(registered.RGBA, completed.RGBA) {
				t.Fatal("save wait never cleared")
			}
			play := press(emulator.KeyStart)
			require(play, "mode", 2)
			require(play, "active_slot", int32(slot))
			heldStart := step(emulator.KeyStart, 20, false)
			if !bytes.Equal(play.RGBA, heldStart.RGBA) {
				t.Fatal("Start held through entering gameplay triggered pause")
			}
			step(emulator.KeyLeft, 80, false)
			if slot > 0 {
				step(emulator.KeyRight, 240, false)
				step(emulator.KeyLeft, 280, false)
			}
			if slot == 2 {
				step(emulator.KeyRight, 400, false)
				step(emulator.KeyLeft, 400, false)
			}
			moved := step(0, 8, false)
			require(moved, "player_x", 8)
			require(moved, "coins", []int32{0, 1, 3}[slot])
			paused := press(emulator.KeyStart)
			assertPauseBackground(t, moved.RGBA, paused.RGBA)
			frozen := press(emulator.KeyRight | emulator.KeyA | emulator.KeyB)
			require(frozen, "player_x", 8)
			if !bytes.Equal(paused.RGBA, frozen.RGBA) {
				t.Fatal("pause has game/menu actions")
			}
			press(emulator.KeyStart)
			step(0, 8, false)
			menu := press(emulator.KeySelect)
			require(menu, "mode", 3)
			back := press(emulator.KeyStart)
			require(back, "mode", 2)
			heldStart = step(emulator.KeyStart, 20, false)
			if !bytes.Equal(back.RGBA, heldStart.RGBA) {
				t.Fatal("Start returning from game menu triggered pause")
			}
			menu = press(emulator.KeySelect)
			saved := press(emulator.KeyA)
			require(saved, "mode", 4)
			require(saved, "saves", 2)
			held := step(emulator.KeyA, 40, false)
			require(held, "saves", 2)
			waitingStart := press(emulator.KeyStart)
			if !bytes.Equal(saved.RGBA, waitingStart.RGBA) {
				t.Fatal("saving screen allowed system pause")
			}
			step(0, 320, false)
			step(emulator.KeyRight, 12, false)
			press(emulator.KeySelect)
			press(emulator.KeyDown)
			press(emulator.KeyDown)
			loaded := press(emulator.KeyA)
			require(loaded, "mode", 2)
			require(loaded, "player_x", 8)
			require(loaded, "saves", 2)
			// Save and quit returns to the file picker only after the reminder expires.
			press(emulator.KeySelect)
			press(emulator.KeyDown)
			quit := press(emulator.KeyA)
			require(quit, "saves", 3)
			require(step(0, 320, false), "mode", 0)
			frames = append(frames, naming.RGBA, registered.RGBA, play.RGBA, paused.RGBA, saved.RGBA, loaded.RGBA)
			if slot == 0 {
				for name, frame := range map[string]emulator.Frame{"paused": paused, "game": loaded, "menu": menu, "waiting": saved} {
					if err := writePNG(filepath.Join(out, renderer.String()+"-"+name+".png"), frame.RGBA); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		cold := step(0, 120, true)
		require(cold, "mode", 0)
		if err := writePNG(filepath.Join(out, renderer.String()+"-named-files.png"), cold.RGBA); err != nil {
			t.Fatal(err)
		}
		for slot := 0; slot < 3; slot++ {
			if slot > 0 {
				press(emulator.KeyDown)
			}
			restored := press(emulator.KeyA)
			require(restored, "mode", 2)
			require(restored, "active_slot", int32(slot))
			require(restored, "player_x", 8)
			require(restored, "saves", 3)
			require(restored, "coins", []int32{0, 1, 3}[slot])
			press(emulator.KeySelect)
			press(emulator.KeyUp)
			press(emulator.KeyA)
		}
		frames = append(frames, cold.RGBA)
		if renderer == transpiler.RendererMode4 {
			reference = frames
		} else if n := compareFrames(reference, frames); n != 0 {
			t.Fatalf("example checkpoints differ: %d", n)
		}
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
	}
	writePSprJSON(t, filepath.Join(out, "example-playthrough.json"), map[string]any{"schemaVersion": 2, "renderers": []string{"mode4", "auto"}, "romSha256": hashes, "namedFiles": 3, "independentSlots": true, "heldASavesOnce": true, "pauseFreezesGame": true, "pauseHasNoActions": true, "pauseOnlyInGameplay": true, "menuStartWorks": true, "heldStartDoesNotPause": true, "waitMessageExpires": true, "loadRestores": true, "resetRestoresAllSlots": true, "saveAndQuit": true, "differingPixels": 0, "physicalPowerCycleVerified": false})
}

func TestPauseWithoutGameInputTargetDiagnostic(t *testing.T) {
	helper, err := filepath.Abs(filepath.Join("..", "..", "build", "player", "mtile-player"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(helper); err != nil {
		t.Skip("build mtile-player first")
	}
	project := filepath.Join(t.TempDir(), "no-input")
	if err := build.CreateProject(project, build.NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "game.lua"), []byte(`local ticks=0
function _update()
 ticks=min(100,ticks+1)
 if ticks>=100 then pause(false)
 elseif ticks>=30 then pause(ticks>=30) end
end
function _draw() cls(1) end`), 0600); err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("MTILE_SAVE_OUT")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	var reference [][]byte
	measurements := map[string]any{}
	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode4, transpiler.RendererMode0, transpiler.RendererAuto} {
		service.Renderer = renderer
		result, err := service.Build(project, filepath.Join(out, "plain-"+renderer.String()+".gba"))
		if err != nil {
			t.Fatal(err)
		}
		session, err := emulator.Start(helper, result.ROMPath)
		if err != nil {
			t.Fatal(err)
		}
		step := func(keys uint16, n int) emulator.Frame {
			var frame emulator.Frame
			for i := 0; i < n; i++ {
				frame, err = session.Step(keys, false)
				if err != nil {
					t.Fatal(err)
				}
			}
			return frame
		}
		press := func(keys uint16) emulator.Frame { step(0, 8); return step(keys, 8) }
		fresh := step(0, 16)
		disabled := press(emulator.KeyStart)
		if !bytes.Equal(fresh.RGBA, disabled.RGBA) {
			t.Fatal("default/false pause intercepted Start")
		}
		step(0, 40) // The dynamic expression now enables pause, without btn/time.
		paused := press(emulator.KeyStart)
		if bytes.Equal(fresh.RGBA, paused.RGBA) {
			t.Fatal("game without input API cannot pause")
		}
		ignored := press(emulator.KeyA | emulator.KeyB | emulator.KeyDown)
		if !bytes.Equal(paused.RGBA, ignored.RGBA) {
			t.Fatal("pause responds to non-Start buttons")
		}
		var costs []uint32
		var steady [][]byte
		for i := 0; i < 120; i++ {
			f := step(0, 2)
			costs = append(costs, f.Telemetry.Stats.LastFrame)
			steady = append(steady, f.RGBA)
		}
		hash, err := fileFingerprint(result.ROMPath)
		if err != nil {
			t.Fatal(err)
		}
		measurements[renderer.String()] = map[string]any{"romSha256": hash, "romBytes": result.ROMSize, "ewramFree": result.EWRAMFree, "samples": 120, "medianPauseCycles": percentile(costs, 50), "p95PauseCycles": percentile(costs, 95)}
		press(emulator.KeyStart)
		resumed := step(0, 16)
		if !bytes.Equal(fresh.RGBA, resumed.RGBA) {
			t.Fatal("resume did not restore graphics")
		}
		step(0, 200) // Gameplay now calls literal false after the dynamic true.
		disabledAgain := press(emulator.KeyStart)
		if !bytes.Equal(fresh.RGBA, disabledAgain.RGBA) {
			t.Fatal("literal false did not disable Start pause")
		}
		frames := append([][]byte{disabled.RGBA, paused.RGBA, ignored.RGBA, resumed.RGBA, disabledAgain.RGBA}, steady...)
		if renderer == transpiler.RendererMode4 {
			reference = frames
		} else if n := compareFrames(reference, frames); n != 0 {
			t.Fatalf("pause differs by %d pixels", n)
		}
		if err := writePNG(filepath.Join(out, renderer.String()+"-pause-only.png"), paused.RGBA); err != nil {
			t.Fatal(err)
		}
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
	}
	writePSprJSON(t, filepath.Join(out, "pause-only.json"), map[string]any{"schemaVersion": 2, "capturedFrames": 120, "worksWithoutGameInput": true, "defaultDisabled": true, "literalAndDynamicEnable": true, "disabledAgain": true, "otherButtonsIgnored": true, "resumeRestoresPixels": true, "differingPixels": 0, "renderers": measurements, "physicalHardwareVerified": false})
}

// Only the centered 51x8 message (including its shadow) may cover game pixels.
func assertPauseBackground(t *testing.T, before, paused []byte) {
	t.Helper()
	for y := 0; y < 160; y++ {
		for x := 0; x < 240; x++ {
			if x >= 95 && x < 146 && y >= 76 && y < 84 {
				continue
			}
			at := (y*240 + x) * 4
			if !bytes.Equal(before[at:at+4], paused[at:at+4]) {
				t.Fatalf("pause changed frozen background at %d,%d", x, y)
			}
		}
	}
}

func TestPauseOverlayScenesTargetDiagnostic(t *testing.T) {
	helper, err := filepath.Abs(filepath.Join("..", "..", "build", "player", "mtile-player"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(helper); err != nil {
		t.Skip("build mtile-player first")
	}
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("MTILE_SAVE_OUT")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	var source strings.Builder
	source.WriteString("function _init() pause(true) end\nfunction _draw() cls(1)\n")
	for i := 0; i < 127; i++ {
		fmt.Fprintf(&source, "spr(1,%d,72)\n", i%30*8)
	}
	source.WriteString("end\n")
	sourceFile := filepath.Join(t.TempDir(), "game.lua")
	if err := os.WriteFile(sourceFile, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	saturated, err := stageProjectWithSource(filepath.Join("..", "..", "examples", "save-game"), sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(saturated)
	cases := map[string]string{
		"tidewalk":      filepath.Join("..", "..", "examples", "tidewalk"),
		"lantern-rooms": filepath.Join("..", "..", "examples", "lantern-rooms"),
		"127-objects":   saturated,
	}
	records := map[string]any{}
	for name, project := range cases {
		t.Run(name, func(t *testing.T) {
			var reference [][]byte
			hashes := map[string]string{}
			for _, renderer := range []transpiler.Renderer{transpiler.RendererMode4, transpiler.RendererMode0, transpiler.RendererAuto} {
				service.Renderer = renderer
				result, err := service.Build(project, filepath.Join(out, name+"-"+renderer.String()+".gba"))
				if err != nil {
					t.Fatal(err)
				}
				if renderer != transpiler.RendererMode4 && result.Renderer != transpiler.RendererMode0 {
					t.Fatalf("unexpected fallback: %+v", result)
				}
				session, err := emulator.Start(helper, result.ROMPath)
				if err != nil {
					t.Fatal(err)
				}
				defer session.Close()
				step := func(keys uint16, n int) emulator.Frame {
					var f emulator.Frame
					for i := 0; i < n; i++ {
						f, err = session.Step(keys, false)
						if err != nil {
							t.Fatal(err)
						}
					}
					return f
				}
				before := step(0, 240)
				paused := step(emulator.KeyStart, 16)
				if bytes.Equal(before.RGBA, paused.RGBA) {
					t.Fatalf("%s menu did not appear after cold initialization", renderer.String())
				}
				assertPauseBackground(t, before.RGBA, paused.RGBA)
				frozen := step(0, 24)
				if !bytes.Equal(paused.RGBA, frozen.RGBA) {
					t.Fatal("paused scene changed")
				}
				resumed := step(emulator.KeyStart, 8)
				if !bytes.Equal(before.RGBA, resumed.RGBA) {
					_ = writePNG(filepath.Join(out, name+"-"+renderer.String()+"-before-error.png"), before.RGBA)
					_ = writePNG(filepath.Join(out, name+"-"+renderer.String()+"-resume-error.png"), resumed.RGBA)
					t.Fatalf("%s Start did not restore the original scene exactly (%d pixels)", renderer.String(), compareFrames([][]byte{before.RGBA}, [][]byte{resumed.RGBA}))
				}
				frames := [][]byte{before.RGBA, paused.RGBA, resumed.RGBA}
				if renderer == transpiler.RendererMode4 {
					reference = frames
				} else if n := compareFrames(reference, frames); n != 0 {
					t.Fatalf("%s differs by %d pixels", renderer.String(), n)
				}
				if err := writePNG(filepath.Join(out, name+"-"+renderer.String()+"-paused.png"), paused.RGBA); err != nil {
					t.Fatal(err)
				}
				hash, err := fileFingerprint(result.ROMPath)
				if err != nil {
					t.Fatal(err)
				}
				hashes[renderer.String()] = hash
			}
			records[name] = map[string]any{"romSha256": hashes, "backgroundUnchanged": true, "pausedSceneStable": true, "startRestoresPixels": true, "differingPixels": 0}
		})
	}
	if t.Failed() {
		return
	}
	writePSprJSON(t, filepath.Join(out, "pause-overlay-scenes.json"), map[string]any{"schemaVersion": 1, "differingPixels": 0, "scenes": records, "physicalHardwareVerified": false})
}
