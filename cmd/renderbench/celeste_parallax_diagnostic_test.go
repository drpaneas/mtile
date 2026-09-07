//go:build runtimeperfdiagnostic

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// This baseline is explicitly sourced from the pre-edit snapshot, including
// its runtime. Running after the authored clouds change must not silently
// relabel the new source as the old game.
//
// MTILE_CELESTE_PARALLAX_BASELINE=/private/tmp/... go test ./cmd/renderbench
// -tags=runtimeperfdiagnostic -run '^TestCelesteParallaxBaselineDiagnostic$'
// -count=1 -timeout=15m -v
func TestCelesteParallaxBaselineDiagnostic(t *testing.T) {
	baseline := os.Getenv("MTILE_CELESTE_PARALLAX_BASELINE")
	if baseline == "" {
		t.Skip("set MTILE_CELESTE_PARALLAX_BASELINE to the frozen pre-edit snapshot")
	}
	if _, err := os.Stat(filepath.Join(baseline, "manifest.json")); err != nil {
		t.Fatalf("baseline snapshot manifest: %v", err)
	}
	out, project, runtimeDir, routeLength := prepareCelesteParallaxProbe(t, baseline,
		filepath.Join(baseline, "project"), filepath.Join(baseline, "runtime"), "before-")
	before := runCelesteParallaxVariant(t, out, project, runtimeDir, "old-mode4", false, false, 0x0404, routeLength)
	after := runCelesteParallaxVariant(t, out, project, runtimeDir, "old-tilecanvas", true, false, 0x0700, routeLength)
	compareCelesteParallaxPixels(t, out, "old-mode4", "old-tilecanvas", before, after)
	if !t.Failed() {
		if err := os.WriteFile(filepath.Join(baseline, "verified-before-evidence.txt"), []byte(out+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// Keep this separately runnable while the hardware candidate is being built:
// the new authored cloud scene needs its own software oracle and cost baseline.
func TestCelesteParallaxMode4Diagnostic(t *testing.T) {
	if os.Getenv("MTILE_CELESTE_PARALLAX_MODE4") != "1" {
		t.Skip("set MTILE_CELESTE_PARALLAX_MODE4=1 for the new scene software baseline")
	}
	out, project, runtimeDir, routeLength := prepareCurrentCelesteParallaxProbe(t)
	rows := runCelesteParallaxVariant(t, out, project, runtimeDir, "new-mode4", false, false, 0x0404, routeLength)
	compareCelesteParallaxOriginalGameplay(t, rows)
}

func TestCelesteParallaxNativeDiagnostic(t *testing.T) {
	if os.Getenv("MTILE_CELESTE_PARALLAX_NATIVE") != "1" {
		t.Skip("set MTILE_CELESTE_PARALLAX_NATIVE=1 for the four-BG/OBJ native comparison")
	}
	out, project, runtimeDir, routeLength := prepareCurrentCelesteParallaxProbe(t)
	oracle := runCelesteParallaxVariant(t, out, project, runtimeDir, "new-mode4", false, false, 0x0404, routeLength)
	candidate := runCelesteParallaxVariant(t, out, project, runtimeDir, "tilecanvas-parallax", true, true, 0x1f00, routeLength)
	compareCelesteParallaxOriginalGameplay(t, oracle)
	compareCelesteParallaxPixels(t, out, "new-mode4", "tilecanvas-parallax", oracle, candidate)
	if !t.Failed() {
		repository, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			t.Fatal(err)
		}
		service, err := build.NewService("", runtimeDir)
		if err != nil {
			t.Fatal(err)
		}
		helper, err := tileCanvasPlayerWrapper(out, filepath.Join(repository, "build", "player", "mtile-player"))
		if err != nil {
			t.Fatal(err)
		}
		// Exercise the actual editor build policy with uninstrumented Lua and
		// physical GBA input, after the paired renderer proof succeeds.
		tileCanvasBuildPlayable(t, repository, out, service, helper)
	}
}

func prepareCurrentCelesteParallaxProbe(t *testing.T) (out, project, runtimeDir string, routeLength int) {
	t.Helper()
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return prepareCelesteParallaxProbe(t, os.TempDir(), filepath.Join(repository, "examples", "celeste"), filepath.Join(repository, "runtime"), "celeste-parallax-after-")
}

// Different cloud artwork is intentional. Physics, effects, RNG consequences,
// completion, restart and death must nevertheless retain every watched value.
func compareCelesteParallaxOriginalGameplay(t *testing.T, after []tileCanvasRow) {
	t.Helper()
	baseline := os.Getenv("MTILE_CELESTE_PARALLAX_BASELINE")
	if baseline == "" {
		t.Fatal("set MTILE_CELESTE_PARALLAX_BASELINE to require pre-migration gameplay proof")
	}
	pointer, err := os.ReadFile(filepath.Join(baseline, "verified-before-evidence.txt"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(pointer)), "old-mode4-traces.json"))
	if err != nil {
		t.Fatal(err)
	}
	var before []tileCanvasRow
	if err := json.Unmarshal(data, &before); err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("cloud migration changed publication count: %d/%d", len(before), len(after))
	}
	for i, left := range before {
		right := after[i]
		if left.Tick != right.Tick || !reflect.DeepEqual(tileCanvasGameplayWatches(left.Watches), tileCanvasGameplayWatches(right.Watches)) {
			t.Errorf("cloud migration changed original gameplay at tick%d: %+v / %+v", left.Tick, left.Watches, right.Watches)
		}
	}
	t.Logf("pre/post authored-cloud gameplay comparison: %d exact watch snapshots", len(after))
}

func prepareCelesteParallaxProbe(t *testing.T, parent, projectSource, runtimeSource, prefix string) (out, project, runtimeDir string, routeLength int) {
	t.Helper()
	var err error
	out, err = os.MkdirTemp(parent, prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("evidence:", out)
	project, runtimeDir = filepath.Join(out, "project"), filepath.Join(out, "runtime")
	if err := tileCanvasCopyTree(projectSource, project, true); err != nil {
		t.Fatal(err)
	}
	if err := tileCanvasCopyTree(runtimeSource, runtimeDir, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(projectSource, "tests", "level_completion_route.json"))
	if err != nil {
		t.Fatal(err)
	}
	var route struct{ Inputs []int }
	if err := json.Unmarshal(data, &route); err != nil || len(route.Inputs) == 0 {
		t.Fatalf("completion route: %v", err)
	}
	if err := tileCanvasInstrument(project, route.Inputs); err != nil {
		t.Fatal(err)
	}
	return out, project, runtimeDir, len(route.Inputs)
}

func runCelesteParallaxVariant(t *testing.T, out, project, runtimeDir, name string, canvas, objects bool, displayBits uint16, routeLength int) []tileCanvasRow {
	t.Helper()
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	service, err := build.NewService("", runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	service.Debug, service.KeepWork = true, true
	service.Renderer, service.DisableMapRasterCache = transpiler.RendererMode4, canvas
	wrapperDir := filepath.Join(out, name+"-tools")
	if err := os.MkdirAll(wrapperDir, 0755); err != nil {
		t.Fatal(err)
	}
	service.TinyGoBin, err = tileCanvasTinyGoFeatureWrapper(wrapperDir, service.TinyGoBin, canvas, objects, true)
	if err != nil {
		t.Fatal(err)
	}
	helper, err := tileCanvasPlayerWrapper(wrapperDir, filepath.Join(repository, "build", "player", "mtile-player"))
	if err != nil {
		t.Fatal(err)
	}
	t.Log("building", name)
	result, err := service.Build(project, filepath.Join(out, name+".gba"))
	if err != nil {
		t.Fatalf("%s build: %v", name, err)
	}
	t.Logf("%s ROM=%d IWRAM=%d/%d EWRAM=%d free=%d work=%s", name, result.ROMSize, result.IWRAMUsed, build.IWRAMCapacity, result.EWRAMUsed, result.EWRAMFree, result.WorkDir)
	if err := os.WriteFile(filepath.Join(out, name+"-build.json"), mustCelesteParallaxJSON(t, result), 0644); err != nil {
		t.Fatal(err)
	}
	rows, captureErr := tileCanvasCapture(helper, result.ROMPath)
	var activeFrames int
	var maxObjects uint32
	for _, row := range rows {
		display := uint16(uint32(row.Value("p_display")) >> 16)
		if display&0x1f07 != displayBits || display&0x0080 != 0 {
			t.Errorf("%s DISPCNT tick%d=%04x, want enabled layers/mode %04x", name, row.Tick, display, displayBits)
		}
		if canvas && row.Stats.LastBlit != 0 && (row.BlitStart < 160 || row.BlitEnd < row.BlitStart || row.BlitEnd >= 228) {
			t.Errorf("%s commit outside VBlank tick%d: %d..%d", name, row.Tick, row.BlitStart, row.BlitEnd)
		}
		if row.OBJCount > maxObjects {
			maxObjects = row.OBJCount
		}
		if row.OBJCount != 0 {
			activeFrames++
		}
	}
	if objects && captureErr == nil && (maxObjects == 0 || maxObjects > 128) {
		t.Errorf("%s lacked valid native OAM coverage: max%d", name, maxObjects)
	}
	if err := os.WriteFile(filepath.Join(out, name+"-traces.json"), mustCelesteParallaxJSON(t, rows), 0644); err != nil {
		t.Fatal(err)
	}
	if captureErr == nil {
		if err := tileCanvasCoverage(rows, routeLength); err != nil {
			t.Errorf("%s coverage: %v", name, err)
		}
	}
	tileCanvasReport(t, name, rows, routeLength)
	t.Logf("%s native objects: frames=%d max=%d", name, activeFrames, maxObjects)
	for _, phase := range []string{"spawn", "route", "complete", "restart", "death", "respawn"} {
		for _, row := range rows {
			if tileCanvasPhase(row, routeLength) == phase {
				if err := writePNG(filepath.Join(out, name+"-"+phase+".png"), row.rgba); err != nil {
					t.Fatal(err)
				}
				break
			}
		}
	}
	if captureErr != nil {
		t.Fatalf("%s capture: %v; partial evidence retained in%s", name, captureErr, out)
	}
	return rows
}

func mustCelesteParallaxJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func compareCelesteParallaxPixels(t *testing.T, out, beforeName, afterName string, before, after []tileCanvasRow) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("%s/%s publication count changed: %d/%d", beforeName, afterName, len(before), len(after))
	}
	var pixelFrames, pixels int
	for i, left := range before {
		right := after[i]
		if left.Tick != right.Tick || !reflect.DeepEqual(tileCanvasGameplayWatches(left.Watches), tileCanvasGameplayWatches(right.Watches)) {
			t.Errorf("%s/%s gameplay state mismatch tick%d: %+v / %+v", beforeName, afterName, left.Tick, left.Watches, right.Watches)
		}
		if count := countDifferingPixels(left.rgba, right.rgba); count != 0 {
			pixelFrames++
			pixels += count
			if pixelFrames <= 8 {
				prefix := filepath.Join(out, fmt.Sprintf("mismatch-%s-%03d", afterName, left.Tick))
				if err := writePNG(prefix+"-reference.png", left.rgba); err != nil {
					t.Fatal(err)
				}
				if err := writePNG(prefix+"-candidate.png", right.rgba); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	t.Logf("%s/%s exact native RGB: updates=%d different_frames=%d pixels=%d", beforeName, afterName, len(before), pixelFrames, pixels)
	if pixelFrames != 0 {
		t.Errorf("%s differs from%s; inspect%s", afterName, beforeName, out)
	}
}
