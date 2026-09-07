//go:build runtimeperfdiagnostic

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// The old three-BG experiment has been replaced by the reusable four-BG/OBJ
// backend. Keep the earlier opt-in command as an alias to the current native
// diagnostic; its pre-migration snapshot remains a required independent input.
func TestCelesteTileCanvasDiagnostic(t *testing.T) {
	if os.Getenv("MTILE_TILECANVAS_DIAGNOSTIC") != "1" {
		t.Skip("set MTILE_TILECANVAS_DIAGNOSTIC=1 and MTILE_CELESTE_PARALLAX_BASELINE for the current four-BG/OBJ diagnostic")
	}
	t.Setenv("MTILE_CELESTE_PARALLAX_NATIVE", "1")
	TestCelesteParallaxNativeDiagnostic(t)
}

type tileCanvasRow struct {
	Tick        int
	Watches     []emulator.Watch
	Stats       emulator.DebugStats
	RGBASHA     string
	FirstSHA    string
	LCDSamples  int
	BlitStart   uint16
	BlitEnd     uint16
	UploadBytes uint32
	DMARuns     uint32
	OBJCount    uint32
	rgba        []byte
}

func (r tileCanvasRow) Value(name string) int32 {
	for _, watch := range r.Watches {
		if watch.Name == name {
			return watch.Raw
		}
	}
	return 0
}

func tileCanvasGameplayWatches(watches []emulator.Watch) []emulator.Watch {
	var values []emulator.Watch
	for _, watch := range watches {
		if watch.Name != "p_display" && watch.Name != "p_window" {
			values = append(values, watch)
		}
	}
	return values
}

func tileCanvasCapture(helper, rom string) ([]tileCanvasRow, error) {
	session, err := emulator.Start(helper, rom)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	var rows []tileCanvasRow
	var pending *tileCanvasRow
	var audible bool
	for step := 0; step < 12000; step++ {
		frame, err := session.Step(0, false)
		if err != nil {
			return rows, err
		}
		if frame.Overflow {
			return rows, fmt.Errorf("native audio overflow at LCD frame %d", frame.Number)
		}
		for _, value := range frame.PCM {
			audible = audible || value != 0
		}
		if !frame.Telemetry.DebugValid {
			continue
		}
		stats := frame.Telemetry.Stats
		if stats.HeapError != emulator.HeapOK {
			return rows, fmt.Errorf("native heap failure: %+v", stats)
		}
		if stats.Frames == 0 {
			continue
		}
		if pending == nil || uint32(pending.Tick) != stats.Frames {
			if pending != nil {
				rows = append(rows, *pending)
				if pending.Value("p_stage") == 4*65536 {
					if !audible {
						return rows, fmt.Errorf("scenario emitted no audible samples")
					}
					return rows, nil
				}
			}
			if stats.Frames != uint32(len(rows)+1) {
				return rows, fmt.Errorf("skipped publication %d after %d", stats.Frames, len(rows))
			}
			pending = &tileCanvasRow{Tick: int(stats.Frames), Stats: stats, Watches: append([]emulator.Watch(nil), frame.Telemetry.Watches...)}
			if len(pending.Watches) != 16 || pending.Watches[0].Name != "p_window" {
				return rows, fmt.Errorf("probe watches missing: %+v", pending.Watches)
			}
			window := uint32(pending.Value("p_window")) >> 16
			pending.BlitStart, pending.BlitEnd = uint16(window>>8), uint16(window&255)
			pending.UploadBytes = uint32(pending.Value("p_window")) & 65535
			pending.DMARuns = uint32(pending.Value("p_display")) & 255
			pending.OBJCount = uint32(pending.Value("p_display")) >> 8 & 255
		}
		// The first LCD sample carrying a post-VBlank mailbox can still show the
		// previous scan. Retain the LAST completed scan before the next logical
		// publication, and record both hashes so presentation transitions remain
		// visible in the evidence rather than being mistaken for raster parity.
		pending.rgba = append(pending.rgba[:0], frame.RGBA...)
		pending.RGBASHA = fmt.Sprintf("%x", sha256.Sum256(frame.RGBA))
		if pending.LCDSamples == 0 {
			pending.FirstSHA = pending.RGBASHA
		}
		pending.LCDSamples++
	}
	if pending != nil {
		rows = append(rows, *pending)
		return rows, fmt.Errorf("scenario stalled within 12000 LCD frames: last tick %d watches=%+v stats=%+v", pending.Tick, pending.Watches, pending.Stats)
	}
	return rows, fmt.Errorf("scenario published no debug frame within 12000 LCD frames (startup failure)")
}

func tileCanvasPhase(row tileCanvasRow, routeLength int) string {
	switch row.Value("p_stage") / 65536 {
	case 0:
		return "spawn"
	case 1:
		if row.Value("p_step") <= int32(routeLength)*65536 {
			return "route"
		}
		return "complete"
	case 2:
		return "restart"
	case 3:
		if row.Value("p_dead") != 0 {
			return "death"
		}
		return "respawn"
	default:
		return "settled"
	}
}

func tileCanvasCoverage(rows []tileCanvasRow, routeLength int) error {
	seen := map[string]bool{}
	routeTicks := 0
	for _, row := range rows {
		seen[tileCanvasPhase(row, routeLength)] = true
		if tileCanvasPhase(row, routeLength) == "route" {
			routeTicks++
		}
		if row.Value("p_hair") != 5*65536 {
			return fmt.Errorf("hair lifecycle corruption at tick %d", row.Tick)
		}
		if row.Value("p_freeze") > 0 {
			seen["freeze"] = true
		}
		if row.Value("p_shake_x") != 0 || row.Value("p_shake_y") != 0 {
			seen["shake"] = true
		}
		if row.Value("p_done") != 0 {
			seen["level_complete"] = true
		}
		if row.Value("p_dead_count") > 0 {
			seen["death_particles"] = true
		}
		if row.Value("p_stage") == 2*65536 && row.Value("p_spawn") != 0 && row.Value("p_done") == 0 {
			seen["Start"] = true
		}
	}
	if routeTicks != routeLength {
		return fmt.Errorf("route contains %d/%d samples", routeTicks, routeLength)
	}
	for _, required := range []string{"spawn", "route", "complete", "restart", "death", "respawn", "settled", "freeze", "shake", "level_complete", "death_particles", "Start"} {
		if !seen[required] {
			return fmt.Errorf("missing %s coverage", required)
		}
	}
	return nil
}

func tileCanvasReport(t *testing.T, name string, rows []tileCanvasRow, routeLength int) {
	for _, phase := range []string{"spawn", "route", "complete", "restart", "death", "respawn"} {
		var frame, update, draw, commit []uint32
		var overBudget, commitOutsideVBlank int
		var cadenceTicks, lcdScans int
		var maxOverhead int64
		for _, row := range rows {
			if tileCanvasPhase(row, routeLength) != phase {
				continue
			}
			s := row.Stats
			frame, update, draw, commit = append(frame, s.LastFrame), append(update, s.LastUpdate), append(draw, s.LastDraw), append(commit, s.LastBlit)
			if s.LastFrame > twoFrameCycleBudget {
				overBudget++
			}
			if row.BlitEnd < 160 || row.BlitEnd < row.BlitStart {
				commitOutsideVBlank++
			}
			if strings.HasPrefix(name, "tilecanvas") && s.LastBlit != 0 && row.BlitEnd >= row.BlitStart {
				// Whole scanlines deliberately overestimate elapsed DMA work.
				overhead := int64(row.BlitEnd-row.BlitStart+1)*1232 - 2*int64(row.UploadBytes) - 192*int64(row.DMARuns)
				if overhead > maxOverhead {
					maxOverhead = overhead
				}
			}
			// The final row may be an indefinitely held image after a guard
			// failure. Only intervals closed by another publication define Hz.
			if row.Tick != rows[len(rows)-1].Tick {
				cadenceTicks++
				lcdScans += row.LCDSamples
			}
		}
		t.Logf("%s %s samples=%d frame p50/p95/max=%d/%d/%d update=%d/%d draw=%d/%d prepare+commit=%d/%d over_budget=%d commit_end_outside_VBlank=%d", name, phase, len(frame), percentile(frame, 50), percentile(frame, 95), percentile(frame, 100), percentile(update, 50), percentile(update, 100), percentile(draw, 50), percentile(draw, 100), percentile(commit, 50), percentile(commit, 100), overBudget, commitOutsideVBlank)
		if lcdScans != 0 {
			lcdHz := 16777216.0 / (float64(twoFrameCycleBudget) / 2)
			t.Logf("%s %s displayed cadence: %d closed ticks/%d LCD scans = %.2f updates/s", name, phase, cadenceTicks, lcdScans, lcdHz*float64(cadenceTicks)/float64(lcdScans))
		}
		if strings.HasPrefix(name, "tilecanvas") {
			t.Logf("%s %s observed DMA fixed-overhead upper bound=%d cycles", name, phase, maxOverhead)
		}
	}
}

func tileCanvasCopyTree(source, destination string, project bool) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if project && (entry.Name() == "tests" || entry.Name() == "__pycache__") {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(destination, relative), 0755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if project {
			switch strings.ToLower(filepath.Ext(path)) {
			case ".gba", ".elf", ".sav":
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destination, relative), data, 0644)
	})
}

func tileCanvasTinyGoWrapper(out, tinygo string, candidate, probes bool) (string, error) {
	return tileCanvasTinyGoFeatureWrapper(out, tinygo, candidate, false, probes)
}

func tileCanvasTinyGoFeatureWrapper(out, tinygo string, candidate, objects, probes bool) (string, error) {
	name, enabled := "tinygo-mode4", "False"
	objectEnabled := "False"
	if objects {
		candidate = true
		objectEnabled = "True"
	}
	if candidate {
		name, enabled = "tinygo-tilecanvas", "True"
	}
	if objects {
		name += "-objects"
	}
	instrument := "True"
	if !probes {
		name += "-playable"
		instrument = "False"
	}
	path := filepath.Join(out, name)
	// No shell interpolation: os.execv forwards every original argument, and
	// env/version discovery uses the real toolchain unchanged.
	source := "#!/usr/bin/env python3\nimport os,sys,re\nfrom pathlib import Path\nreal=" + strconv.Quote(tinygo) + "\ncandidate=" + enabled + "\nobjects=" + objectEnabled + "\ninstrument=" + instrument + `
args=sys.argv[1:]
if args and args[0]=='build':
    if candidate:
        tags='mtile_tilecanvas'
        for i,arg in enumerate(args):
            if arg.startswith('-tags='):
                args[i]=arg+','+tags
                break
        else:
            args.insert(1,'-tags='+tags)
if instrument and args and args[0]=='build':
    # Both ROMs replace the same reserved debug-only watch, after PresentRender.
    # This reads real hardware; neither Lua nor its renderer contract changes.
    path=Path('main.go')
    display='uint32(hw.GetReg16(hw.DISPCNT))<<16|(ctx.TileCanvasDMARuns()&255)'
    if objects:
        display+='|(uint32(ctx.TileCanvasOBJCount())&255)<<8'
    text,count=re.subn(r'debugMailbox.Add\("p_display",[^\n]*',
        'debugMailbox.Add("p_display", rt.NumVal(rt.Number('+display+')))',path.read_text())
    if count!=1:
        raise RuntimeError('expected one reserved p_display debug watch')
    text,count=re.subn(r'debugMailbox.Add\("(?:p_tick|p_window)",[^\n]*',
        'debugMailbox.Add("p_window", tileCanvasProbeWindow())',text)
    if count!=1:
        raise RuntimeError('expected one reserved p_tick/p_window debug watch')
    if 'func tileCanvasProbeWindow()' not in text:
        # The retired three-BG baseline counts only raster tile uploads. The
        # new backend also uploads hardware map blocks on scroll boundaries.
        # Preserve old evidence, while accounting for both in the new probe.
        upload='ctx.RendererUploadStats().LastTileBytes'
        if objects:
            upload='(ctx.RendererUploadStats().LastTileBytes+ctx.RendererUploadStats().LastMapBytes)'
        text+='\nfunc tileCanvasProbeWindow() rt.Value { start,end:=ctx.LastBlitWindow(); return rt.NumVal(rt.Number(uint32(start)<<24|uint32(end)<<16|'+upload+')) }\n'
    path.write_text(text)
os.execv(real,[real]+args)
`
	return path, os.WriteFile(path, []byte(source), 0755)
}

func tileCanvasBuildPlayable(t *testing.T, repository, out string, service build.Service, helper string) {
	tileCanvasBuildPlayableVariant(t, repository, out, service, helper, false)
}

func tileCanvasBuildPlayableVariant(t *testing.T, repository, out string, service build.Service, helper string, objects bool) {
	t.Helper()
	project := filepath.Join(out, "playable-project")
	if err := tileCanvasCopyTree(filepath.Join(repository, "examples", "celeste"), project, true); err != nil {
		t.Fatal(err)
	}
	// Exercise the same scoped build policy used by `mtile edit
	// -experimental-tilecanvas`, without a TinyGo wrapper or generated patches.
	t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_PROJECT", project)
	name, profile := "celeste-tilecanvas-playable", "experimental-tilecanvas"
	if objects {
		t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_OBJECTS", "1")
		name = "celeste-tilecanvas-objects-alias-playable"
	} else {
		t.Setenv("MTILE_EXPERIMENTAL_TILECANVAS_OBJECTS", "")
	}
	service.Debug, service.KeepWork, service.UseProjectRenderer = true, true, true
	result, err := service.Build(project, filepath.Join(out, name+".gba"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Renderer != transpiler.RendererMode0 || result.RendererProfile != profile || result.RendererLayers != 4 || !result.RendererObjects {
		t.Fatalf("editor experiment reported incorrect presentation: %+v", result)
	}
	t.Logf("playable build: ROM=%d IWRAM=%d/%d EWRAMfree=%d work=%s", result.ROMSize, result.IWRAMUsed, build.IWRAMCapacity, result.EWRAMFree, result.WorkDir)
	// This build uses the original Lua and physical joypad. The scoped build
	// policy does not inject debug watches or input.
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
		if last.Overflow {
			t.Fatal("playable ROM audio overflow")
		}
		if last.Telemetry.DebugValid && last.Telemetry.Stats.HeapError != emulator.HeapOK {
			t.Fatalf("playable ROM heap failure: %+v", last.Telemetry.Stats)
		}
		for _, value := range last.PCM {
			audible = audible || value != 0
		}
	}
	// Original-source startup is deliberately not shortened by the probe's
	// extra globals/control flow. Start the input smoke window after boot.
	warmup := 0
	for ; warmup < 1200; warmup++ {
		step(0)
		if last.Telemetry.DebugValid && last.Telemetry.Stats.Frames > 0 {
			break
		}
	}
	if warmup == 1200 {
		t.Fatal("playable ROM did not finish boot within1200LCD frames")
	}
	for lcd := 0; lcd < 120; lcd++ {
		var keys uint16
		if lcd >= 80 {
			keys = emulator.KeyRight
		}
		if lcd >= 84 && lcd < 90 {
			keys |= emulator.KeyA
		}
		if lcd >= 102 && lcd < 106 {
			keys |= emulator.KeyB
		}
		step(keys)
	}
	if !last.Telemetry.DebugValid || last.Telemetry.Stats.Frames < 10 || !audible {
		t.Fatal("playable ROM did not produce active game frames and sound")
	}
	if err := writePNG(filepath.Join(out, "playable-smoke-test.png"), last.RGBA); err != nil {
		t.Fatal(err)
	}
	t.Logf("playable original-source ROM: %s;%dLCD boot steps then120LCD frames with physical input, sound active, heapOK", result.ROMPath, warmup+1)
}

func tileCanvasPlayerWrapper(out, player string) (string, error) {
	path := filepath.Join(out, "mtile-player-capture")
	// The emulator session intentionally encapsulates stderr. Redirect only
	// this experiment's child process so guard panics remain durable evidence.
	source := "#!/usr/bin/env python3\nimport os,sys\nreal=" + strconv.Quote(player) + `
log=os.open(sys.argv[1]+'.stderr.log',os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o644)
os.dup2(log,2)
os.close(log)
os.execv(real,[real]+sys.argv[1:])
`
	return path, os.WriteFile(path, []byte(source), 0755)
}

func tileCanvasInstrument(project string, route []int) error {
	button := regexp.MustCompile(`\bbtn\(`)
	err := filepath.WalkDir(project, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".lua" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := button.ReplaceAllString(string(data), "probe_btn(")
		if filepath.Base(path) == "game.lua" {
			source = strings.Replace(source, "function _update()", "function probe_game_update()", 1)
		}
		return os.WriteFile(path, []byte(source), 0644)
	})
	if err != nil {
		return err
	}
	values := make([]string, len(route))
	for i, value := range route {
		values[i] = strconv.Itoa(value)
	}
	probe := `p_tick=0 p_stage=0 p_step=0
p_x=0 p_y=0 p_vx=0 p_vy=0 p_freeze=0
p_dead=false p_spawn=true p_done=false p_hair=0
p_shake_x=0 p_shake_y=0 p_dead_count=0 p_display=0
probe_keys=0 probe_settled=0
probe_route={` + strings.Join(values, ",") + `}
function probe_btn(button)
 return band(probe_keys,shl(1,button))~=0
end
`
	if err := os.WriteFile(filepath.Join(project, "src", "tilecanvas_probe.lua"), []byte(probe), 0644); err != nil {
		return err
	}
	path := filepath.Join(project, "game.lua")
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	wrapper := `
function _update()
 p_tick=p_tick+1
 probe_keys=0
 if p_stage==0 and not spawning then p_stage=1 end
 if p_stage==1 then
  p_step=p_step+1
  probe_keys=probe_route[p_step] or 0
  if p_step==#probe_route+8 then probe_keys=64 p_stage=2 end
 elseif p_stage==2 and not spawning then
  -- Deterministic fixture after the unmodified completion/Start route:
  -- place a complete death burst on screen using the real game helper,
  -- then observe the normal death countdown and respawn lifecycle.
  player.x=120 player.y=80 kill_player() p_stage=3
 elseif p_stage==3 and not spawning and not player.dead then
  probe_settled=probe_settled+1
  if probe_settled==12 then p_stage=4 end
 end
 probe_game_update()
 p_x=player.x p_y=player.y p_vx=player.vx p_vy=player.vy
 p_freeze=freeze p_dead=player.dead p_spawn=spawning p_done=level_complete
 p_hair=#player.hair p_shake_x=shake_x p_shake_y=shake_y
 p_dead_count=#dead_particles
end
`
	return os.WriteFile(path, []byte("require(\"tilecanvas_probe\")\n"+string(source)+wrapper), 0644)
}
