//go:build runtimeperfdiagnostic

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// TestPSprStreamTargetDiagnostic exercises the actual ARM/PPU path against
// Mode 4. Run with -tags=runtimeperfdiagnostic -run=TestPSprStreamTargetDiagnostic.
// MTILE_PSPR_OUT preserves ROMs, hashes, paired pixel evidence, and cycle data.
func TestPSprStreamTargetDiagnostic(t *testing.T) {
	helper := filepath.Join("..", "..", "build", "player", "mtile-player")
	if _, err := os.Stat(helper); err != nil {
		t.Skip("build the headless mtile-player first")
	}
	out := os.Getenv("MTILE_PSPR_OUT")
	if out == "" {
		out = t.TempDir()
	}
	for _, name := range []string{"anim-clock", "anim-clips", "fighters", "large-bank", "deduplicated-bank", "mutable-source", "full-window", "full-window-mid-oam", "full-window-high-oam", "combined-budget", "packed-budget", "full-window-four-bg", "full-window-audio", "full-window-resident-four-bg-audio", "full-window-overflow-four-bg", "full-window-overflow-audio"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(out, name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			project := filepath.Join(dir, "project")
			if err := build.CreateProject(project, build.NewProjectOptions{SheetWidth: 32, SheetHeight: 16, MapWidth: 30, MapHeight: 20}); err != nil {
				t.Fatal(err)
			}
			writePSprStreamFixture(t, project, name)
			service, err := build.NewService("", "")
			if err != nil {
				t.Fatal(err)
			}
			service.Debug = true
			environment, err := benchmarkEnvironment(service, project, helper)
			if err != nil {
				t.Fatal(err)
			}
			const warmup, samples = 2, 120
			mode4Build, mode4, err := buildAndCapture(service, transpiler.RendererMode4, project, helper, dir, warmup, samples)
			if err != nil {
				t.Fatal(err)
			}
			mode0Build, mode0, err := buildAndCaptureAtFrames(service, transpiler.RendererMode0, project, helper, dir, warmup, samples, mode4.frameNumbers)
			requiresFallback := strings.Contains(name, "overflow")
			if requiresFallback && err != nil {
				if !strings.Contains(err.Error(), "[M0_RESOURCE_LIMIT]") {
					t.Fatal(err)
				}
				rejection := err.Error()
				autoBuild, auto, autoErr := buildAndCaptureAtFrames(service, transpiler.RendererAuto, project, helper, dir, warmup, samples, mode4.frameNumbers)
				if autoErr != nil {
					t.Fatal(autoErr)
				}
				if autoBuild.Renderer != transpiler.RendererMode4 || autoBuild.RendererDiagnostic == "" {
					t.Fatalf("oversized commit must fall back to Mode4: %+v", autoBuild)
				}
				differences := compareFrames(mode4.frames, auto.frames)
				writePSprJSON(t, filepath.Join(dir, "fallback.json"), map[string]any{
					"forcedMode0Diagnostic": rejection,
					"autoDiagnostic":        autoBuild.RendererDiagnostic,
					"autoRenderer":          autoBuild.Renderer.String(),
					"capturedFrames":        samples, "differingPixels": differences,
				})
				if differences != 0 {
					t.Fatalf("AUTO fallback differs by %d pixels", differences)
				}
				t.Logf("%s: oversized commit rejected; AUTO matches Mode4 on %d ARM frames", name, samples)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if name == "full-window-four-bg" || name == "full-window-audio" || name == "full-window-resident-four-bg-audio" {
				wantTiles := uint64(128)
				if name == "full-window-resident-four-bg-audio" {
					wantTiles = 8
				}
				if mode0Build.PoseMemory.ResidentVRAMBytes != wantTiles*64 || mode0Build.PoseMemory.StageRAMBytes != 0 ||
					!strings.Contains(mode0Build.GeneratedGo, "PoseOBJResident: poseOBJResidentROM[:]") {
					t.Fatalf("missing resident allocation: %+v", mode0Build.PoseMemory)
				}
				writePSprJSON(t, filepath.Join(dir, "resident-allocation.json"), mode0Build.PoseMemory)
			}
			if name == "deduplicated-bank" {
				for _, result := range []build.Result{mode4Build, mode0Build} {
					if !strings.Contains(result.GeneratedGo, "var objSheetROM = struct { _ [0]uint32; Pixels [128]byte }") {
						t.Fatalf("%s ROM must contain two unique OBJ cells, not five duplicated cells", result.Renderer)
					}
				}
			}
			if err := validateSynchronizedFrames(mode4.frameNumbers, mode0.frameNumbers); err != nil {
				t.Fatal(err)
			}
			// Check every LCD scan, including the first scan after presentation.
			// A later screenshot can hide a transfer that overran VBlank.
			oracleScans, err := capturePresentWindow(helper, mode4Build.ROMPath, warmup+samples)
			if err != nil {
				t.Fatal(err)
			}
			candidateScans, err := capturePresentWindow(helper, mode0Build.ROMPath, warmup+samples)
			if err != nil {
				t.Fatal(err)
			}
			lcdScans, lcdDifferences := 0, 0
			for tick := warmup; tick < len(candidateScans); tick++ {
				for _, scan := range candidateScans[tick].Scans {
					lcdScans++
					lcdDifferences += countDifferingPixels(oracleScans[tick].lastRGBA, scan.rgba)
				}
			}
			writePSprJSON(t, filepath.Join(dir, "lcd-scan-parity.json"), map[string]any{
				"runtimeFrames": samples, "lcdScans": lcdScans, "differingPixels": lcdDifferences,
			})
			if lcdDifferences != 0 {
				t.Errorf("first/all LCD scans differ by %d pixels", lcdDifferences)
			}
			for _, result := range []struct {
				name string
				data capture
			}{{"mode0", mode0}, {"mode4", mode4}} {
				if err := writePNG(filepath.Join(dir, result.name+"-final.png"), result.data.lastRGB); err != nil {
					t.Fatal(err)
				}
			}
			differences := compareFrames(mode4.frames, mode0.frames)
			if index, _ := firstDifferingFrame(mode4.frames, mode0.frames); index >= 0 {
				_ = writePNG(filepath.Join(dir, "mode4-first-diff.png"), mode4.frames[index])
				_ = writePNG(filepath.Join(dir, "mode0-first-diff.png"), mode0.frames[index])
				t.Errorf("first differing runtime frame: %d", mode0.frameNumbers[index])
			}
			m4, err := summarize(mode4Build, mode4, filepath.Join(dir, "mode4-final.png"))
			if err != nil {
				t.Fatal(err)
			}
			m0, err := summarize(mode0Build, mode0, filepath.Join(dir, "mode0-final.png"))
			if err != nil {
				t.Fatal(err)
			}
			report := gateReport{
				SchemaVersion: reportSchemaVersion, GatePolicy: gateParity,
				Project: project, Helper: helper, Environment: environment,
				WarmupFrames: warmup, CapturedFrames: samples, DifferingPixels: differences,
				EWRAMGain:           int64(m0.EWRAMFree) - int64(m4.EWRAMFree),
				Mode0MedianSavingPC: 100 * (1 - float64(m0.MedianCycles)/float64(m4.MedianCycles)),
				Mode0P95Headroom:    int64(twoFrameCycleBudget) - int64(m0.P95Cycles),
				Mode0:               m0, Mode4: m4, ParityPass: differences == 0,
				MemoryPass:      int64(m0.EWRAMFree)-int64(m4.EWRAMFree) >= minimumEWRAMGain,
				MedianCyclePass: uint64(m0.MedianCycles)*100 <= uint64(m4.MedianCycles)*mode0MedianPercent,
				P95HeadroomPass: int64(twoFrameCycleBudget)-int64(m0.P95Cycles) >= minimumP95Headroom,
				MissedFramePass: mode4.missed == 0 && mode0.missed == 0,
			}
			if err := applyGatePolicy(&report, gateParity); err != nil {
				t.Fatal(err)
			}
			writePSprJSON(t, filepath.Join(dir, "report.json"), report)
			writePSprJSON(t, filepath.Join(dir, "commit-cycles.json"), map[string]any{
				"mode0MaximumCommitCycles": percentile(mode0.commit, 100),
				"mode0P95CommitCycles":     percentile(mode0.commit, 95),
				"mode0MaximumFrameCycles":  percentile(mode0.cycles, 100),
				"frameNumbers":             mode0.frameNumbers,
				"mode0CommitCycles":        mode0.commit,
			})
			t.Logf("%s: %d exact ARM frames; pixels=%d, OAM=%d, OBJ line=%d, median Mode0/Mode4=%d/%d, max commit=%d, EWRAM gain=%d; %s",
				name, samples, differences, m0.OAMEntries, m0.OBJLineCycles,
				m0.MedianCycles, m4.MedianCycles, percentile(mode0.commit, 100), report.EWRAMGain, dir)
			if differences != 0 {
				t.Fatalf("ARM Mode0/Mode4 differ by %d pixels", differences)
			}
			if compareFrames(mode0.frames[:1], mode0.frames[1:2]) == 0 {
				t.Fatal("animation fixture produced static pixels")
			}
			if maximum := percentile(mode0.commit, 100); maximum >= 67*1232 {
				t.Fatalf("Mode0 commit %d exceeds the 82,544-cycle usable VBlank window", maximum)
			}
			if requiresFallback {
				t.Fatal("Mode0 must reject this transfer budget and AUTO must fall back")
			}
		})
	}
}

func writePSprJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

func psprFixtureCell(id, cols, seed int) assetsync.SpriteRecord {
	cell := assetsync.SpriteRecord{ID: id, X: (id % cols) * 8, Y: (id / cols) * 8, Width: 8, Height: 8}
	for y := 0; y < 8; y++ {
		row := make([]int, 8)
		for x := range row {
			if (x+2*y+seed)%7 != 0 {
				row[x] = 1 + (seed*13+x*3+y*7)%254
			}
		}
		cell.Pixels = append(cell.Pixels, row)
	}
	return cell
}

func writePSprStreamFixture(t *testing.T, project, name string) {
	t.Helper()
	sheet := assetsync.Spritesheet{Width: 32, Height: 16}
	for id := 0; id < 8; id++ {
		sheet.Sprites = append(sheet.Sprites, psprFixtureCell(id, 4, id+20))
	}
	if name == "packed-budget" {
		sheet = assetsync.Spritesheet{Width: 128, Height: 32}
		for id := 0; id < 64; id++ {
			sheet.Sprites = append(sheet.Sprites, psprFixtureCell(id, 16, id+20))
		}
	}
	writePSprJSON(t, filepath.Join(project, "assets", "spritesheet.json"), sheet)
	poses := assetsync.PoseFile{Version: 1}
	var obj assetsync.Spritesheet
	var source string
	if name == "anim-clock" || name == "anim-clips" {
		obj = assetsync.Spritesheet{Width: 32, Height: 24}
		for id := 0; id < 12; id++ {
			obj.Sprites = append(obj.Sprites, psprFixtureCell(id, 4, id+3))
		}
		for frame := 0; frame < 3; frame++ {
			poses.Poses = append(poses.Poses, assetsync.PoseRecord{Name: fmt.Sprintf("frame%d", frame), Parts: []assetsync.PosePartRecord{
				{Tile: frame * 4}, {Tile: frame*4 + 1, X: 8}, {Tile: frame*4 + 2, Y: 8, FlipX: frame == 1}, {Tile: frame*4 + 3, X: 8, Y: 8},
			}})
		}
		hold := false
		animation := assetsync.AnimationWorkspace{Version: 1, Clips: []assetsync.AnimationClip{
			{Name: "loop", Frames: []int{0, 1, 2}, FPS: 60}, {Name: "once", Frames: []int{2, 1, 0}, FPS: 12, Loop: &hold}, {Name: "empty", FPS: 8},
		}}
		writePSprJSON(t, filepath.Join(project, "assets", "animation.json"), animation)
		source = `function _draw() cls(4) anim("loop",40,50) end`
		if name == "anim-clips" {
			source = `tick=0
started=0
name="loop"
missing="absent"
function actor(x) anim(name,x,110,false,false,time()-started+x/100) end
function marker() anim("loop",210,20) return 30 end
function _update()
 tick=tick+1
 if tick%40<20 then name="loop" else name="once" end
 if tick%20==0 then started=time() end
end
function _draw()
 cls(4)
 anim("loop",marker(),40)
 anim("once",80,40,true,false,tick/12)
 anim("loop",125,80,false,true,(tick%24-6)/12)
 actor(40) actor(90)
 anim("empty",0,0)
 anim(missing,0,0)
 pspr(tick%3,170,110)
 spr(0,190,135)
end`
		}
	} else if name == "fighters" {
		obj = assetsync.Spritesheet{Width: 128, Height: 128}
		for id := 0; id < 256; id++ {
			obj.Sprites = append(obj.Sprites, psprFixtureCell(id, 16, id+1))
		}
		for frame := 0; frame < 2; frame++ {
			pose := assetsync.PoseRecord{Name: fmt.Sprintf("fighter%d", frame)}
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					pose.Parts = append(pose.Parts, assetsync.PosePartRecord{
						Tile: frame*128 + y*16 + x, X: x * 8, Y: y * 8,
					})
				}
			}
			poses.Poses = append(poses.Poses, pose)
		}
		poses.Poses = append(poses.Poses,
			assetsync.PoseRecord{Name: "fireball", Parts: []assetsync.PosePartRecord{
				{Tile: 8}, {Tile: 9, X: 8}, {Tile: 24, Y: 8}, {Tile: 25, X: 8, Y: 8},
			}},
			assetsync.PoseRecord{Name: "overlap", Parts: []assetsync.PosePartRecord{
				{Tile: 40, X: -3, Y: 2}, {Tile: 41, X: 1, Y: 4, FlipX: true}, {Tile: 42, X: 1, Y: 4, FlipY: true},
			}},
		)
		m := assetsync.Map{Version: 2, Width: 30, Height: 20}
		for layer := 0; layer < 4; layer++ {
			cells := make([]int, 600)
			if layer == 0 {
				for x := 0; x < 30; x++ {
					cells[18*30+x] = 1
				}
			}
			m.Layers = append(m.Layers, assetsync.MapLayer{ID: layer, Cells: cells})
		}
		writePSprJSON(t, filepath.Join(project, "assets", "map.json"), m)
		source = `tick=0
function _update()
 tick=tick+1
 sset(8,0,1+tick%254)
end
function draw_effect(id,x,y)
 pspr(id,x,y,tick%2==0,tick%3==0)
end
function mark()
 pspr(2,110,88)
 return 20
end
function _draw()
 cls(4)
 clip(0,0,240,160)
 camera(tick%7-3,tick%5-2)
 map(0,0,0,0,0,30,20)
 local frame=flr(tick/6)%2
 pspr(frame,mark(),25,false,false)
 spr(1,50,42)
 pspr(1-frame,211,25,true,false)
 sspr(0,0,16,16,105,60)
 draw_effect(2,112,80)
 draw_effect(3,115,84)
 if tick%2==0 then pspr(65536,30,116) else pspr(3,38,115) end
 print("FIGHT",4,4,7)
end
`
	} else if name == "large-bank" {
		obj = assetsync.Spritesheet{Width: 2048, Height: 2048}
		for id := 0; id < 1000; id++ {
			tile := id
			if id == 999 {
				tile = 65535
			}
			obj.Sprites = append(obj.Sprites, psprFixtureCell(tile, 256, id+1))
			poses.Poses = append(poses.Poses, assetsync.PoseRecord{
				Name:  fmt.Sprintf("frame%d", id),
				Parts: []assetsync.PosePartRecord{{Tile: tile, FlipX: id%2 == 0, FlipY: id%3 == 0}},
			})
		}
		source = `tick=0
function _update() tick=tick+1 end
function _draw()
 cls(4)
 local frame=(tick*37)%1000
 pspr(frame,20+tick%180,40,tick%2==0,tick%3==0)
 pspr(999,110,80)
 pspr(-1,60,60)
 pspr(1000,70,70)
end
`
	} else if name == "deduplicated-bank" {
		obj = assetsync.Spritesheet{Width: 2048, Height: 2048}
		for _, cell := range []struct{ id, seed int }{{0, 11}, {1, 11}, {2, 29}, {256, 29}, {65535, 11}} {
			obj.Sprites = append(obj.Sprites, psprFixtureCell(cell.id, 256, cell.seed))
		}
		poses.Poses = []assetsync.PoseRecord{
			{Name: "aliases", Parts: []assetsync.PosePartRecord{
				{Tile: 0}, {Tile: 1, X: 8}, {Tile: 65535, X: 16}, {Tile: 2, X: 24}, {Tile: 256, X: 32},
			}},
			{Name: "flipped-aliases", Parts: []assetsync.PosePartRecord{
				{Tile: 65535, FlipX: true}, {Tile: 0, X: 8, FlipY: true}, {Tile: 1, X: 16},
				{Tile: 256, X: 24, FlipX: true, FlipY: true}, {Tile: 2, X: 32},
			}},
			{Name: "highest-id", Parts: []assetsync.PosePartRecord{{Tile: 65535}}},
		}
		source = `tick=0
function _update() tick=tick+1 end
function _draw()
 cls(4)
 pspr(flr(tick/3)%2,20+tick%80,30,tick%2==0,tick%3==0)
 pspr(0,40,80)
 pspr(1,95,80)
 pspr(2,150,80)
end
`
	} else if name == "mutable-source" {
		poses.Poses = []assetsync.PoseRecord{{Name: "mutable", Parts: []assetsync.PosePartRecord{
			{Tile: 0}, {Tile: 1, X: 8}, {Tile: 4, Y: 8}, {Tile: 5, X: 8, Y: 8},
		}}}
		source = `tick=0
function _update()
 tick=tick+1
 sset(tick%32,flr(tick/2)%16,1+tick%254)
end
function _draw()
 cls(4)
 pspr(0,20,20)
 pspr(0,60,20,true,true)
 spr(1,90,20)
end
`
	} else if strings.HasPrefix(name, "full-window") || strings.HasSuffix(name, "budget") {
		cols, rows, tileCount, copies := 8, 8, 128, 8
		budgetEdge := strings.HasSuffix(name, "budget")
		if name == "full-window-high-oam" {
			cols, rows, tileCount, copies = 2, 2, 8, 128
		} else if name == "full-window-resident-four-bg-audio" {
			cols, rows, tileCount, copies = 2, 2, 8, 128
		} else if name == "full-window-mid-oam" {
			cols, rows, tileCount, copies = 4, 2, 16, 64
		} else if budgetEdge {
			copies = 5
		}
		obj = assetsync.Spritesheet{Width: 128, Height: 64}
		frames := 2
		if strings.Contains(name, "overflow") {
			tileCount, frames, obj.Height = 640, 10, 320
		}
		for id := 0; id < tileCount; id++ {
			obj.Sprites = append(obj.Sprites, psprFixtureCell(id, 16, id+1))
		}
		for frame := 0; frame < frames; frame++ {
			pose := assetsync.PoseRecord{Name: fmt.Sprintf("full%d", frame)}
			for y := 0; y < rows; y++ {
				for x := 0; x < cols; x++ {
					pose.Parts = append(pose.Parts, assetsync.PosePartRecord{Tile: ((frame*cols*rows + y*cols + x) * 37) % tileCount, X: x * 8, Y: y * 8})
				}
			}
			poses.Poses = append(poses.Poses, pose)
		}
		if name == "packed-budget" {
			for frame := 0; frame < 2; frame++ {
				pose := assetsync.PoseRecord{Name: fmt.Sprintf("half%d", frame)}
				for y := 0; y < 4; y++ {
					for x := 0; x < 8; x++ {
						pose.Parts = append(pose.Parts, assetsync.PosePartRecord{Tile: ((frame*64 + y*8 + x) * 37) % 128, X: x * 8, Y: y * 8})
					}
				}
				poses.Poses = append(poses.Poses, pose)
			}
		}
		var code strings.Builder
		code.WriteString("tick=0\n")
		if strings.HasSuffix(name, "audio") {
			mode0PresentAudioAssets(t, filepath.Join("..", ".."), project, "audio-dense")
			code.WriteString("function _init() music(0) end\n")
		}
		code.WriteString("function _update()\n tick=tick+1\n")
		mapLayers := 0
		if strings.Contains(name, "four-bg") {
			mapLayers = 4
		}
		if budgetEdge {
			mapLayers = 1
			code.WriteString(" pal(1,1+tick%254)\n")
			if name == "combined-budget" {
				for x := 0; x < 32; x += 8 {
					fmt.Fprintf(&code, " sset(%d,0,1+tick%%254)\n", x)
				}
			}
		}
		if mapLayers > 0 {
			m := assetsync.Map{Version: 2, Width: 30, Height: 20}
			for layer := 0; layer < 4; layer++ {
				cells := make([]int, 600)
				for cell := range cells {
					if layer < mapLayers && cell%7 == layer {
						cells[cell] = 1
					}
				}
				m.Layers = append(m.Layers, assetsync.MapLayer{ID: layer, Cells: cells})
				if layer < mapLayers {
					fmt.Fprintf(&code, " mset(%d,tick%%30,0,1+tick%%7)\n", layer)
				}
			}
			writePSprJSON(t, filepath.Join(project, "assets", "map.json"), m)
		}
		code.WriteString("end\nfunction _draw()\n cls(4)\n")
		if mapLayers > 0 {
			for layer := 0; layer < mapLayers; layer++ {
				fmt.Fprintf(&code, " map(%d,0,0,0,0,30,20)\n", layer)
			}
		}
		if name == "packed-budget" {
			code.WriteString(" if tick>=4 then spr(0,8,4,4,4) spr(4,48,4,4,4) end\n")
		}
		for copy := 0; copy < copies; copy++ {
			x, y := copy*20, copy*9
			if name == "full-window-high-oam" || name == "full-window-resident-four-bg-audio" {
				x, y = (copy%16)*14, (copy/16)*16
			} else if name == "full-window-mid-oam" {
				x, y = (copy%8)*28, (copy/8)*18
			}
			fmt.Fprintf(&code, " pspr(tick%%%d,%d,%d)\n", frames, x, y)
		}
		if name == "packed-budget" {
			code.WriteString(" pspr(2+tick%2,140,100)\n")
		}
		code.WriteString("end\n")
		source = code.String()
	} else {
		t.Fatalf("unknown fixture %q", name)
	}
	if obj.Width > 0 {
		writePSprJSON(t, filepath.Join(project, "assets", "objsheet.json"), obj)
	}
	writePSprJSON(t, filepath.Join(project, "assets", "poses.json"), poses)
	if err := os.WriteFile(filepath.Join(project, "game.lua"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
}
