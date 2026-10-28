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
		for i, watches := range c.watches {
			found := false
			for _, watch := range watches {
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
		"capturedFrames": samples, "differingPixels": differences, "zeroSaveErrors": true,
		"mode0": map[string]any{"romSha256": romHash(b0.ROMPath), "medianCycles": percentile(c0.cycles, 50), "p95Cycles": percentile(c0.cycles, 95)},
		"mode4": map[string]any{"romSha256": romHash(b4.ROMPath), "medianCycles": percentile(c4.cycles, 50), "p95Cycles": percentile(c4.cycles, 95)},
	})
}

func TestSaveExampleTargetDiagnostic(t *testing.T) {
	helper, err := filepath.Abs(filepath.Join("..", "..", "build", "player", "mtile-player"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(helper); err != nil {
		t.Skip("build mtile-player first")
	}
	project, err := filepath.Abs(filepath.Join("..", "..", "examples", "save-game"))
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
	service, err := build.NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.Debug = true
	var reference [][]byte
	var steadyReference [][]byte
	measurements := map[string]any{}
	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode4, transpiler.RendererMode0, transpiler.RendererAuto} {
		t.Run(renderer.String(), func(t *testing.T) {
			service.Renderer = renderer
			result, err := service.Build(project, filepath.Join(out, "example-"+renderer.String()+".gba"))
			if err != nil {
				t.Fatal(err)
			}
			if renderer != transpiler.RendererMode4 && result.Renderer != transpiler.RendererMode0 {
				t.Fatal("example AUTO fallback")
			}
			session, err := emulator.Start(helper, result.ROMPath)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			step := func(keys uint16, count int, reset bool) emulator.Frame {
				var frame emulator.Frame
				for i := 0; i < count; i++ {
					frame, err = session.Step(keys, reset && i == 0)
					if err != nil {
						t.Fatal(err)
					}
				}
				return frame
			}
			watch := func(frame emulator.Frame, name string) int32 {
				for _, w := range frame.Telemetry.Watches {
					if w.Name == name && w.Kind == 2 {
						return w.Raw
					}
				}
				t.Fatalf("missing numeric watch %s", name)
				return 0
			}
			fresh := step(0, 12, false)
			if watch(fresh, "status") != 0 {
				t.Fatal("fresh save status")
			}
			step(emulator.KeyLeft, 80, false)
			// Frame capture can precede the next game input poll. Release and
			// settle before recording the coordinate that A should persist.
			moved := step(0, 4, false)
			x := watch(moved, "player_x")
			if x != 8<<16 {
				t.Fatal("movement did not reach the reproducible left boundary")
			}

			press := func(keys uint16) emulator.Frame { step(0, 8, false); return step(keys, 8, false) }
			paused := press(emulator.KeyStart)
			assertPauseBackground(t, moved.RGBA, paused.RGBA)
			frozen := step(emulator.KeyRight, 20, false)
			if watch(frozen, "player_x") != x {
				t.Fatal("game moved while paused")
			}
			var steady [][]byte
			var costs []uint32
			for i := 0; i < 120; i++ {
				frame := step(0, 2, false)
				steady = append(steady, frame.RGBA)
				costs = append(costs, frame.Telemetry.Stats.LastFrame)
			}
			if renderer == transpiler.RendererMode4 {
				steadyReference = steady
			} else if n := compareFrames(steadyReference, steady); n != 0 {
				t.Fatalf("steady pause differs by %d pixels", n)
			}
			hash, err := fileFingerprint(result.ROMPath)
			if err != nil {
				t.Fatal(err)
			}
			measurements[renderer.String()] = map[string]any{"romSha256": hash, "romBytes": result.ROMSize, "ewramFree": result.EWRAMFree, "samples": len(costs), "medianMenuCycles": percentile(costs, 50), "p95MenuCycles": percentile(costs, 95)}
			press(emulator.KeyDown)
			saved := press(emulator.KeyA)
			held := step(emulator.KeyA, 50, false)
			if watch(held, "saves") != 1<<16 || watch(saved, "status") != 4<<16 {
				t.Fatalf("menu save or held-A failure: saves=%d saved-status=%d held-status=%d", watch(held, "saves"), watch(saved, "status"), watch(held, "status"))
			}
			waiting := press(emulator.KeyB)
			if watch(waiting, "player_x") != x {
				t.Fatal("wait resumed game")
			}
			completed := step(0, 310, false)
			if bytes.Equal(saved.RGBA, completed.RGBA) {
				t.Fatal("Omega wait message never cleared")
			}
			resumed := press(emulator.KeyStart)
			step(0, 12, false) // Release menu keys after the resume presentation.
			step(emulator.KeyRight, 12, false)
			away := step(0, 8, false)
			if watch(away, "player_x") <= x {
				_ = writePNG(filepath.Join(out, renderer.String()+"-resume-error.png"), away.RGBA)
				t.Fatalf("Resume did not restore game input: x=%d want>%d status=%d", watch(away, "player_x"), x, watch(away, "status"))
			}
			press(emulator.KeyStart)
			loadMenu := press(emulator.KeyA)
			if watch(loadMenu, "player_x") != x || watch(loadMenu, "status") != 1<<16 {
				t.Fatal("menu Load did not restore progress")
			}
			loaded := press(emulator.KeyStart)
			reset := step(0, 12, true)
			if watch(reset, "player_x") != x || watch(reset, "saves") != 1<<16 || watch(reset, "status") != 1<<16 {
				t.Fatal("reset did not restore cartridge save")
			}
			press(emulator.KeyStart)
			press(emulator.KeyUp)
			ended := press(emulator.KeyA)
			later := step(emulator.KeyRight|emulator.KeyB|emulator.KeyStart, 20, false)
			if watch(later, "player_x") != x || !bytes.Equal(ended.RGBA, later.RGBA) {
				t.Fatal("Exit did not end the session")
			}
			for name, frame := range map[string]emulator.Frame{"fresh": fresh, "paused": paused, "waiting": saved, "completed": completed, "ended": ended} {
				if err := writePNG(filepath.Join(out, renderer.String()+"-"+name+".png"), frame.RGBA); err != nil {
					t.Fatal(err)
				}
			}
			frames := [][]byte{fresh.RGBA, moved.RGBA, paused.RGBA, saved.RGBA, waiting.RGBA, completed.RGBA, resumed.RGBA, loadMenu.RGBA, loaded.RGBA, reset.RGBA, ended.RGBA}

			if renderer == transpiler.RendererMode4 {
				reference = frames
			} else if n := compareFrames(reference, frames); n != 0 {
				for i, label := range []string{"fresh", "moved", "paused", "saved", "waiting", "completed", "resumed", "loadMenu", "loaded", "reset", "ended"} {
					if d := compareFrames([][]byte{reference[i]}, [][]byte{frames[i]}); d != 0 {
						_ = writePNG(filepath.Join(out, renderer.String()+"-mismatch-"+label+".png"), frames[i])
						t.Logf("checkpoint %s differs by %d pixels", label, d)
					}
				}
				t.Fatalf("example differs by %d pixels", n)
			}
		})
	}
	if t.Failed() {
		return
	}
	writePSprJSON(t, filepath.Join(out, "example-playthrough.json"), map[string]any{"schemaVersion": 1, "renderers": []string{"mode4", "mode0", "auto"}, "heldASavesOnce": true, "pauseFreezesGame": true, "resumeRestoresInput": true, "exitEndsSession": true, "waitMessageExpires": true, "loadRestores": true, "resetRestores": true, "differingPixels": 0, "physicalPowerCycleVerified": false})
	writePSprJSON(t, filepath.Join(out, "pause-menu.json"), map[string]any{"schemaVersion": 1, "capturedFrames": 120, "differingPixels": 0, "renderers": measurements})
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
	if err := os.WriteFile(filepath.Join(project, "game.lua"), []byte(`function _draw() cls(1) end`), 0600); err != nil {
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
	var reference [][]byte
	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode4, transpiler.RendererMode0} {
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
		paused := press(emulator.KeyStart)
		if bytes.Equal(fresh.RGBA, paused.RGBA) {
			t.Fatal("game without input API cannot pause")
		}
		press(emulator.KeyDown)
		unavailable := press(emulator.KeyA)
		if bytes.Equal(paused.RGBA, unavailable.RGBA) {
			t.Fatal("unavailable saving has no feedback")
		}
		press(emulator.KeyStart)
		resumed := step(0, 16)
		if !bytes.Equal(fresh.RGBA, resumed.RGBA) {
			t.Fatal("plain game graphics not restored")
		}
		press(emulator.KeyStart)
		press(emulator.KeyUp)
		ended := press(emulator.KeyA)
		later := step(emulator.KeyB|emulator.KeyStart, 16)
		if !bytes.Equal(ended.RGBA, later.RGBA) {
			t.Fatal("plain game Exit resumed")
		}
		frames := [][]byte{paused.RGBA, unavailable.RGBA, ended.RGBA}
		if renderer == transpiler.RendererMode4 {
			reference = frames
		} else if n := compareFrames(reference, frames); n != 0 {
			t.Fatalf("plain menu differs by %d pixels", n)
		}
		if err := writePNG(filepath.Join(out, renderer.String()+"-unavailable.png"), unavailable.RGBA); err != nil {
			t.Fatal(err)
		}
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
	}
	writePSprJSON(t, filepath.Join(out, "pause-no-input.json"), map[string]any{"schemaVersion": 1, "worksWithoutGameInput": true, "unavailableSavingHasFeedback": true, "resumeRestoresPixels": true, "exitEndsSession": true, "differingPixels": 0})
}

// Excluding the whole 64x64 system OBJ also covers the taller feedback panel.
func assertPauseBackground(t *testing.T, before, paused []byte) {
	t.Helper()
	for y := 0; y < 160; y++ {
		for x := 0; x < 240; x++ {
			if x >= 88 && x < 152 && y >= 48 && y < 112 {
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
	source.WriteString("function _draw() cls(1)\n")
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
