//go:build runtimeperfdiagnostic

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/sourcebundle"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// This source-only inspection is useful while editing the authored queue:
// public Mode 0 admission does not imply that a Lua record was scalarized.
func TestCelesteMode0SourceDiagnostic(t *testing.T) {
	if os.Getenv("MTILE_CELESTE_MODE0_SOURCE") != "1" {
		t.Skip("set MTILE_CELESTE_MODE0_SOURCE=1 to inspect public Mode 0 admission and scalar pose storage")
	}
	project, err := filepath.Abs(filepath.Join("..", "..", "research", "celeste-mode0"))
	if err != nil {
		t.Fatal(err)
	}
	assets, err := assetsync.Load(project)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := sourcebundle.Resolve(project, "game.lua")
	if err != nil {
		t.Fatal(err)
	}
	generated, report, err := transpiler.TranspileWithReport(bundle.Entry, bundle.Source, transpiler.Options{
		Renderer: transpiler.RendererMode0, PackageName: "main", IWRAMFunctions: assets.Project.IWRAMFunctions,
		SheetW: assets.SheetW, SheetH: assets.SheetH, MapW: assets.MapW, MapH: assets.MapH, MapLayers: assets.MapLayers,
		Sheet: assets.SheetROM, Map: assets.MapROM, Flags: assets.FlagROM, Palette: assets.PaletteROM[:],
		NativeAudio: assets.NativeAudio,
	})
	if err != nil {
		t.Fatal(bundle.RemapError(err))
	}
	file, err := os.CreateTemp("", "celeste-mode0-source-*.go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(generated); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("generated source: %s; renderer=%s layers=%d OAM=%d scanline cycles=%d", file.Name(), report.Selected,
		report.Mode0Layers, report.Mode0OAMEntries, report.Mode0OBJLineCycles)
	poseHeapFields := regexp.MustCompile(`(?m)^\s+(\w*pose\w*) rt.Value$`).FindAllStringSubmatch(generated, -1)
	for _, field := range poseHeapFields {
		t.Errorf("pose record %s still uses a heap table; keep each fixed record at most 32 numeric fields", field[1])
	}
	t.Logf("scalar record fields=%d", len(regexp.MustCompile(`(?m)^\s+_gluRecord\d+Field\d+ rt.Number$`).FindAllString(generated, -1)))
}

// Both ROMs use precisely the same authored tiles and Lua. The candidate is
// selected through the normal public Mode 0 proof; no private renderer tags,
// Lua API exceptions, game-specific runtime branches or pixel substitutions
// are injected. The generated-Go probe only reads display/OAM state and the
// scanlines immediately surrounding PresentRender.
//
// MTILE_CELESTE_MODE0_NATIVE=1 GOCACHE=/private/tmp/mtile-celeste-go-cache \
// go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
// -run '^TestCelesteMode0NativeDiagnostic$' -count=1 -timeout=15m -v
func TestCelesteMode0NativeDiagnostic(t *testing.T) {
	if os.Getenv("MTILE_CELESTE_MODE0_NATIVE") != "1" {
		t.Skip("set MTILE_CELESTE_MODE0_NATIVE=1 for the ordinary Mode 0 Celeste comparison")
	}
	t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_PROJECT", "")
	t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_OBJECTS", "")
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(repository, "research", "celeste-mode0")
	out, project, runtimeDir, routeLength := prepareCelesteParallaxProbe(t, os.TempDir(), source,
		filepath.Join(repository, "runtime"), "celeste-mode0-")
	// Reuse the low byte of the existing display watch rather than displacing
	// any of the sixteen physics/effect watches with a seventeenth watch.
	gamePath := filepath.Join(project, "game.lua")
	gameSource, err := os.ReadFile(gamePath)
	if err != nil {
		t.Fatal(err)
	}
	const probeEnd = "p_dead_count=#dead_particles"
	if strings.Count(string(gameSource), probeEnd) != 1 {
		t.Fatal("expected one probe update endpoint for the render queue overflow watch")
	}
	oracle := runCelesteMode0Variant(t, repository, out, project, runtimeDir, "tiles-mode4-oracle", transpiler.RendererMode4, routeLength)
	candidate := runCelesteMode0Variant(t, repository, out, project, runtimeDir, "tiles-mode0", transpiler.RendererMode0, routeLength)
	compareCelesteParallaxPixels(t, out, "tiles-mode4-oracle", "tiles-mode0", oracle, candidate)
	compareCelesteMode0PreviousGameplay(t, repository, candidate, routeLength)
	if !t.Failed() {
		buildCelesteMode0Playable(t, repository, out, source, runtimeDir)
	}
}

func runCelesteMode0Variant(t *testing.T, repository, out, project, runtimeDir, name string,
	renderer transpiler.Renderer, routeLength int) []tileCanvasRow {
	t.Helper()
	service, err := build.NewService("", runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	service.Debug, service.KeepWork, service.Renderer = true, true, renderer
	wrapperDir := filepath.Join(out, name+"-tools")
	if err := os.MkdirAll(wrapperDir, 0755); err != nil {
		t.Fatal(err)
	}
	service.TinyGoBin, err = celesteMode0TinyGoProbe(wrapperDir, service.TinyGoBin)
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
	if result.Renderer != renderer || strings.Contains(result.RendererProfile, "experimental") {
		t.Fatalf("%s selected an unexpected renderer: %+v", name, result)
	}
	if renderer == transpiler.RendererMode0 && (result.RendererLayers != 4 || !result.RendererObjects) {
		t.Fatalf("%s did not prove four BGs plus native OBJs: %+v", name, result)
	}
	if err := os.WriteFile(filepath.Join(out, name+"-build.json"), mustCelesteParallaxJSON(t, result), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s ROM=%d IWRAM=%d/%d EWRAMfree=%d public OAM proof=%d scanline cycles=%d/1226 work=%s",
		name, result.ROMSize, result.IWRAMUsed, build.IWRAMCapacity, result.EWRAMFree,
		result.RendererOAMEntries, result.RendererLineCycles, result.WorkDir)
	rows, captureErr := tileCanvasCapture(helper, result.ROMPath)
	var maxObjects, mapUploadFrames uint32
	var queueErrorFrames, lateCommitFrames int
	displayBits := uint16(0x0404)
	if renderer == transpiler.RendererMode0 {
		displayBits = 0x1f00
	}
	for _, row := range rows {
		display := uint16(uint32(row.Value("p_display")) >> 16)
		// The shared trace reader calls this byte DMARuns for the old hybrid
		// probe. This diagnostic reserves it for the authored queue error flag.
		if row.DMARuns != 0 {
			queueErrorFrames++
			if queueErrorFrames == 1 {
				t.Errorf("%s render queue overflow flag=%d first observed at tick %d", name, row.DMARuns, row.Tick)
			}
		}
		if display&0x1f07 != displayBits || display&0x0080 != 0 {
			t.Errorf("%s DISPCNT tick %d = %04x, want %04x", name, row.Tick, display, displayBits)
		}
		if renderer == transpiler.RendererMode0 {
			if row.FirstSHA != row.RGBASHA {
				t.Errorf("%s first scan differs from the held image at tick %d (%d LCD samples)", name, row.Tick, row.LCDSamples)
			}
			if row.BlitStart < 160 || row.BlitEnd < row.BlitStart || row.BlitEnd >= 228 {
				lateCommitFrames++
				if lateCommitFrames <= 8 {
					t.Errorf("%s commit outside VBlank tick %d: %d..%d", name, row.Tick, row.BlitStart, row.BlitEnd)
				}
			}
			if row.OBJCount > uint32(result.RendererOAMEntries) {
				t.Errorf("%s OAM count %d exceeds the compiler proof %d at tick %d", name, row.OBJCount, result.RendererOAMEntries, row.Tick)
			}
			if row.OBJCount > maxObjects {
				maxObjects = row.OBJCount
			}
			if row.UploadBytes != 0 {
				mapUploadFrames++
			}
		}
	}
	if renderer == transpiler.RendererMode0 && maxObjects == 0 && captureErr == nil {
		t.Error("ordinary Mode 0 never enabled an OAM object")
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
	t.Logf("%s native OAM peak=%d frames with BG/tile uploads=%d/%d", name, maxObjects, mapUploadFrames, len(rows))
	t.Logf("%s queue error frames=%d late commit frames=%d", name, queueErrorFrames, lateCommitFrames)
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
		t.Fatalf("%s capture: %v; partial evidence retained in %s", name, captureErr, out)
	}
	return rows
}

func compareCelesteMode0PreviousGameplay(t *testing.T, repository string, rows []tileCanvasRow, routeLength int) {
	t.Helper()
	path := os.Getenv("MTILE_CELESTE_MODE0_BASELINE_TRACES")
	if path == "" {
		path = filepath.Join(repository, "build", "experiments", "celeste-parallax", "after", "tilecanvas-parallax-traces.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pre-tile-conversion gameplay evidence (override with MTILE_CELESTE_MODE0_BASELINE_TRACES): %v", err)
	}
	var before []tileCanvasRow
	if err := json.Unmarshal(data, &before); err != nil {
		t.Fatal(err)
	}
	if len(before) != len(rows) {
		t.Fatalf("tile conversion changed publication count: %d/%d", len(before), len(rows))
	}
	var normalFrames, normalDifferences, uiFrames, uiDifferences int
	for i, old := range before {
		if old.Tick != rows[i].Tick || !reflect.DeepEqual(tileCanvasGameplayWatches(old.Watches), tileCanvasGameplayWatches(rows[i].Watches)) {
			t.Errorf("tile conversion changed previous gameplay at tick %d: %+v / %+v", old.Tick, old.Watches, rows[i].Watches)
		}
		// Completion is triggered during drawing, after the historical watch
		// snapshot. The final route update already shows the new tile-authored
		// panel even though p_done has not changed yet. These eight authored UI
		// images are the only deliberate differences from the previous game.
		completionUI := rows[i].Value("p_stage") == 65536 && rows[i].Value("p_step") >= int32(routeLength)*65536
		if completionUI {
			uiFrames++
			if old.RGBASHA != rows[i].RGBASHA {
				uiDifferences++
			}
		} else {
			normalFrames++
			if old.RGBASHA != rows[i].RGBASHA {
				normalDifferences++
				if normalDifferences <= 8 {
					t.Errorf("tile conversion changed previous non-UI RGB at tick %d", old.Tick)
				}
			}
		}
	}
	t.Logf("pre/post tile conversion: %d gameplay snapshots compared against %s", len(rows), path)
	t.Logf("previous hybrid/new tiles RGB: normal frames=%d different=%d; intentionally authored completion UI frames=%d different=%d",
		normalFrames, normalDifferences, uiFrames, uiDifferences)
}

func celesteMode0TinyGoProbe(out, tinygo string) (string, error) {
	path := filepath.Join(out, "tinygo-public-renderer-probe")
	// Only generated diagnostic code is patched. Read all OAM entries for both
	// variants so neither timing/cadence baseline receives a one-sided read loop.
	source := "#!/usr/bin/env python3\nimport os,sys,re\nfrom pathlib import Path\nreal=" + strconv.Quote(tinygo) + `
args=sys.argv[1:]
if args and args[0]=='build':
    if any('mtile_tilecanvas' in arg for arg in args):
        raise RuntimeError('public Mode 0 diagnostic must not use private tags')
    path=Path('main.go')
    text=path.read_text()
    # The build service may relink after measuring heap/IWRAM requirements.
    # Instrument each generated source once, even across those tool invocations.
    if 'func celesteMode0ProbeWindow()' in text:
        os.execv(real,[real]+args)
    text,count=re.subn(r'debugMailbox.Add\("p_display", (.+)\)',
        lambda match: 'debugMailbox.Add("p_display", celesteMode0ProbeDisplay('+match.group(1)+'))',text)
    if count!=1:
        raise RuntimeError('expected one reserved p_display debug watch')
    text,count=re.subn(r'debugMailbox.Add\("(?:p_tick|p_window)",[^\n]*',
        'debugMailbox.Add("p_window", celesteMode0ProbeWindow())',text)
    if count!=1:
        raise RuntimeError('expected one reserved p_tick/p_window debug watch')
    text,count=re.subn(r'ctx.PresentRender\(\)',
        'celesteMode0PresentStart = hw.GetReg16(hw.VCOUNT)\nctx.PresentRender()\ncelesteMode0PresentEnd = hw.GetReg16(hw.VCOUNT)',text)
    if count!=1:
        raise RuntimeError('expected one PresentRender call')
    text+='''
var celesteMode0PresentStart, celesteMode0PresentEnd uint16
func celesteMode0ProbeWindow() rt.Value {
    stats:=ctx.RendererUploadStats()
    start,end:=celesteMode0PresentStart,celesteMode0PresentEnd
    if hw.GetReg16(hw.DISPCNT)&7==0 {
        // The normal renderer records writes after any internal window wait.
        // Including that wait would incorrectly label retained frames as DMA.
        start,end=ctx.LastBlitWindow()
    }
    return rt.NumVal(rt.Number(uint32(start)<<24|uint32(end)<<16|(stats.LastMapBytes+stats.LastTileBytes+stats.LastOBJTileBytes)&65535))
}
func celesteMode0ProbeDisplay(queueError rt.Value) rt.Value {
    var objects uint32
    for i:=0;i<128;i++ {
        attr0:=hw.GetReg16(hw.OAM+uintptr(i*8))
        if attr0&0x0300!=0x0200 { objects++ }
    }
    return rt.NumVal(rt.Number(uint32(hw.GetReg16(hw.DISPCNT))<<16|objects<<8|uint32(queueError.Int())&255))
}
'''
    path.write_text(text)
os.execv(real,[real]+args)
`
	return path, os.WriteFile(path, []byte(source), 0755)
}

func buildCelesteMode0Playable(t *testing.T, repository, out, source, runtimeDir string) {
	t.Helper()
	project := filepath.Join(out, "playable-project")
	if err := tileCanvasCopyTree(source, project, true); err != nil {
		t.Fatal(err)
	}
	service, err := build.NewService("", runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	service.Debug, service.KeepWork, service.UseProjectRenderer = true, true, true
	result, err := service.Build(project, filepath.Join(out, "celeste-mode0-playable.gba"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Renderer != transpiler.RendererMode0 || strings.Contains(result.RendererProfile, "experimental") {
		t.Fatalf("project.json did not select ordinary Mode 0: %+v", result)
	}
	if err := os.WriteFile(filepath.Join(out, "celeste-mode0-playable-build.json"), mustCelesteParallaxJSON(t, result), 0644); err != nil {
		t.Fatal(err)
	}
	helper, err := tileCanvasPlayerWrapper(out, filepath.Join(repository, "build", "player", "mtile-player"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := emulator.Start(helper, result.ROMPath)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var audible bool
	var last emulator.Frame
	step := func(keys uint16) {
		last, err = session.Step(keys, false)
		if err != nil {
			t.Fatal(err)
		}
		if last.Overflow || last.Telemetry.DebugValid && last.Telemetry.Stats.HeapError != emulator.HeapOK {
			t.Fatalf("playable ROM audio/heap failure: overflow=%v stats=%+v", last.Overflow, last.Telemetry.Stats)
		}
		for _, sample := range last.PCM {
			audible = audible || sample != 0
		}
	}
	warmup := 0
	for ; warmup < 1200; warmup++ {
		step(0)
		if last.Telemetry.DebugValid && last.Telemetry.Stats.Frames > 0 {
			break
		}
	}
	if warmup == 1200 {
		t.Fatal("playable ROM did not boot within 1200 LCD frames")
	}
	for lcd := 0; lcd < 160; lcd++ {
		var keys uint16
		if lcd >= 80 {
			keys = emulator.KeyRight
		}
		if lcd >= 84 && lcd < 90 {
			keys |= emulator.KeyA
		}
		if lcd >= 102 && lcd < 106 {
			keys |= emulator.KeyB | emulator.KeyUp
		}
		if lcd >= 150 && lcd < 154 {
			keys = emulator.KeyStart
		}
		step(keys)
	}
	if !last.Telemetry.DebugValid || last.Telemetry.Stats.Frames < 10 || !audible {
		t.Fatal("playable ROM did not produce game frames and sound")
	}
	if err := writePNG(filepath.Join(out, "celeste-mode0-playable-smoke.png"), last.RGBA); err != nil {
		t.Fatal(err)
	}
	t.Logf("playable ordinary Mode 0: %s; boot=%d LCD frames, 160 LCD frames of real input, sound active, heap OK; %s",
		result.ROMPath, warmup+1, fmt.Sprintf("EWRAMfree=%d IWRAM=%d/%d", result.EWRAMFree, result.IWRAMUsed, build.IWRAMCapacity))
}
