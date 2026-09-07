//go:build runtimeperfdiagnostic

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// This invasive diagnostic profiles a frozen generated program and runtime.
// Its per-call timer reads add overhead; it locates costs, not release cadence.
// No production Lua globals, renderer proof, or product runtime are modified.
func TestCelesteMode0ProfileDiagnostic(t *testing.T) {
	if os.Getenv("MTILE_CELESTE_MODE0_PROFILE") != "1" {
		t.Skip("set MTILE_CELESTE_MODE0_PROFILE=1 for frozen generated-Go cycle probes")
	}
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	frozen := filepath.Join(repo, "build", "experiments", "celeste-mode0", "final")
	if path := os.Getenv("MTILE_CELESTE_MODE0_PROFILE_BASELINE"); path != "" {
		frozen = path
	}
	generated := filepath.Join(frozen, "generated", "tiles-mode0", "main.go")
	if path := os.Getenv("MTILE_CELESTE_MODE0_PROFILE_GENERATED"); path != "" {
		generated = path
	}
	out, err := os.MkdirTemp("", "celeste-mode0-profile-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("profile evidence:", out)
	project, runtimeDir := filepath.Join(out, "project"), filepath.Join(out, "runtime")
	if err := tileCanvasCopyTree(filepath.Join(frozen, "project"), project, true); err != nil {
		t.Fatal(err)
	}
	if err := tileCanvasCopyTree(filepath.Join(frozen, "runtime"), runtimeDir, false); err != nil {
		t.Fatal(err)
	}
	// Explicit runtime-only overlays keep generated Lua/Go and the remaining
	// runtime frozen during independent optimization experiments. The retained
	// output directory contains the exact overlaid runtime for reproduction.
	if overlay := os.Getenv("MTILE_CELESTE_MODE0_PROFILE_RUNTIME_OVERLAY"); overlay != "" {
		files := strings.Split(os.Getenv("MTILE_CELESTE_MODE0_PROFILE_OVERLAY_FILES"), ",")
		for _, file := range files {
			file = strings.TrimSpace(file)
			if file == "" || filepath.IsAbs(file) || filepath.Clean(file) != file || strings.HasPrefix(file, "..") {
				t.Fatalf("invalid runtime overlay file %q", file)
			}
			data, err := os.ReadFile(filepath.Join(overlay, file))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(runtimeDir, file), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if os.Getenv("MTILE_CELESTE_MODE0_PROFILE_STATIC_EQUAL") == "1" {
		// Only the equality implementation may differ from the frozen runtime.
		data, err := os.ReadFile(filepath.Join(repo, "runtime", "value.go"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runtimeDir, "value.go"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	timers := os.Getenv("MTILE_CELESTE_MODE0_PROFILE_TIMERS") != "0"
	service, err := build.NewService("", runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	service.Debug, service.KeepWork, service.Renderer = true, true, transpiler.RendererMode0
	wrapper := filepath.Join(out, "tinygo-profile")
	script := "#!/usr/bin/env python3\nimport os,sys,re\nfrom pathlib import Path\nreal=" + strconv.Quote(service.TinyGoBin) + "\nfrozen=" + strconv.Quote(generated) + `
args=sys.argv[1:]
if args and args[0]=='build':
    text=Path(frozen).read_text()
    if os.environ.get('MTILE_CELESTE_MODE0_PROFILE_TIMERS')=='0':
        Path('main.go').write_text(text)
        os.execv(real,[real]+args)
    names=['new_game','initialize_effects','entities_reset','start_spawn','reset_player','create_hair','sound_init','clear_smoke','prepare_render_frame','draw_background','draw_ambient_particles','map_calls','spr_calls','draw_smoke']
    wrappers=[]
    nprof=len(names)
    for index,name in enumerate(names):
        if name.endswith('_calls'): continue
        pattern=r'func \(g \*Game\) '+name+r'\(([^\n]*)\)([^{]*)\{'
        match=re.search(pattern,text)
        if not match: raise RuntimeError('missing generated method '+name)
        signature=match.group(1)
        result=match.group(2).strip()
        args2=', '.join(item.strip().split()[0] for item in signature.split(',') if item.strip())
        text=text[:match.start()]+text[match.start():].replace('func (g *Game) '+name+'(', 'func (g *Game) profOriginal_'+name+'(',1)
        ret='g.profOriginal_'+name+'('+args2+')'
        if result:
            wrappers.append('func (g *Game) '+name+'('+signature+') '+result+' { start:=hw.ReadFrameCycleCounter(); out:='+ret+'; profCycles['+str(index)+']+=hw.ReadFrameCycleCounter()-start; return out }')
        else:
            wrappers.append('func (g *Game) '+name+'('+signature+') { start:=hw.ReadFrameCycleCounter(); '+ret+'; profCycles['+str(index)+']+=hw.ReadFrameCycleCounter()-start }')
    for name,call in [('map_calls','MapBGAtV?'),('spr_calls','SprV?')]:
        index=names.index(name)
        pattern=r'(?m)^(\s*)c\.'+call+r'\([^\n]*$'
        text,count=re.subn(pattern,lambda m:'{ profCallStart:=hw.ReadFrameCycleCounter(); '+m.group(0).strip()+'; profCycles['+str(index)+']+=hw.ReadFrameCycleCounter()-profCallStart }',text)
        if count<1: raise RuntimeError('missing runtime call '+call)
    text=text.replace('ctx.BeginFramePace()', 'ctx.BeginFramePace()\nprofCycles=['+str(nprof)+']uint32{}')
    text=re.sub(r'(?m)^\s*debugMailbox.Add\([^\n]*\n','',text)
    watches='\n'.join('debugMailbox.Add("'+name[:15]+'",rt.NumVal(rt.Number(profCycles['+str(i)+'])))' for i,name in enumerate(names))
    watches+='\ndebugMailbox.Add("p_stage",rt.NumVal(game.p_stage))\ndebugMailbox.Add("p_step",rt.NumVal(game.p_step))\n'
    text=text.replace('debugMailbox.End()',watches+'debugMailbox.End()')
    text+='\nvar profCycles ['+str(nprof)+']uint32\n'+'\n'.join(wrappers)+'\n'
    Path('main.go').write_text(text)
os.execv(real,[real]+args)
`
	if err := os.WriteFile(wrapper, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	service.TinyGoBin = wrapper
	result, err := service.Build(project, filepath.Join(out, "profile.gba"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "build.json"), mustCelesteParallaxJSON(t, result), 0644); err != nil {
		t.Fatal(err)
	}
	helper, err := tileCanvasPlayerWrapper(out, filepath.Join(repo, "build", "player", "mtile-player"))
	if err != nil {
		t.Fatal(err)
	}
	if !timers {
		rows, err := tileCanvasCapture(helper, result.ROMPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "profile-traces.json"), mustCelesteParallaxJSON(t, rows), 0644); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(frozen, "tiles-mode0-traces.json"))
		if err != nil {
			t.Fatal(err)
		}
		var before []tileCanvasRow
		if err := json.Unmarshal(data, &before); err != nil {
			t.Fatal(err)
		}
		if len(rows) != len(before) {
			t.Fatalf("publications %d/%d", len(rows), len(before))
		}
		for i, row := range rows {
			if row.Tick != before[i].Tick || row.RGBASHA != before[i].RGBASHA || !reflect.DeepEqual(tileCanvasGameplayWatches(row.Watches), tileCanvasGameplayWatches(before[i].Watches)) {
				t.Errorf("frozen gameplay/pixel mismatch tick %d", row.Tick)
			}
			if row.FirstSHA != row.RGBASHA || row.BlitStart < 160 || row.BlitEnd < row.BlitStart || row.BlitEnd >= 228 {
				t.Errorf("presentation mismatch tick %d: %d..%d", row.Tick, row.BlitStart, row.BlitEnd)
			}
		}
		tileCanvasReport(t, "frozen-replay", rows, 162)
		t.Logf("280 frozen gameplay/pixel comparisons; only value.go patch=%v; evidence=%s", os.Getenv("MTILE_CELESTE_MODE0_PROFILE_STATIC_EQUAL") == "1", out)
		t.Logf("baseline=%s generated=%s overlay=%s files=%s", frozen, generated, os.Getenv("MTILE_CELESTE_MODE0_PROFILE_RUNTIME_OVERLAY"), os.Getenv("MTILE_CELESTE_MODE0_PROFILE_OVERLAY_FILES"))
		return
	}
	session, err := emulator.Start(helper, result.ROMPath)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var rows []tileCanvasRow
	for lcd := 0; lcd < 12000; lcd++ {
		frame, err := session.Step(0, false)
		if err != nil {
			t.Fatal(err)
		}
		if frame.Overflow || frame.Telemetry.DebugValid && frame.Telemetry.Stats.HeapError != emulator.HeapOK {
			t.Fatalf("native failure: %+v", frame.Telemetry.Stats)
		}
		if !frame.Telemetry.DebugValid || frame.Telemetry.Stats.Frames == 0 {
			continue
		}
		stats := frame.Telemetry.Stats
		if len(rows) > 0 && rows[len(rows)-1].Tick == int(stats.Frames) {
			rows[len(rows)-1].LCDSamples++
			continue
		}
		row := tileCanvasRow{Tick: int(stats.Frames), Stats: stats, Watches: append([]emulator.Watch(nil), frame.Telemetry.Watches...), LCDSamples: 1}
		rows = append(rows, row)
		if row.Value("p_stage") == 4*65536 {
			break
		}
	}
	if err := os.WriteFile(filepath.Join(out, "profile-traces.json"), mustCelesteParallaxJSON(t, rows), 0644); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 280 {
		t.Fatalf("captured %d/280 publications", len(rows))
	}
	for _, phase := range []string{"route", "complete", "restart", "death", "respawn"} {
		var selected []tileCanvasRow
		for _, row := range rows {
			if tileCanvasPhase(row, 162) == phase {
				selected = append(selected, row)
			}
		}
		if len(selected) == 0 {
			continue
		}
		t.Logf("%s frames=%d", phase, len(selected))
		for _, watch := range selected[0].Watches {
			if strings.HasPrefix(watch.Name, "p_") {
				continue
			}
			values := make([]int, len(selected))
			var sum int64
			for i, row := range selected {
				values[i] = int(uint32(row.Value(watch.Name)))
				sum += int64(values[i])
			}
			sort.Ints(values)
			t.Logf("  %-15s mean=%d median=%d p95=%d max=%d", watch.Name, sum/int64(len(values)), values[len(values)/2], values[(len(values)-1)*95/100], values[len(values)-1])
		}
	}
	t.Log(fmt.Sprintf("frozen generated source; timer instrumentation is inclusive and adds per-call overhead; ROM=%d IWRAM=%d work=%s", result.ROMSize, result.IWRAMUsed, result.WorkDir))
}
