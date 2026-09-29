//go:build runtimeperfdiagnostic

package main

import (
	"crypto/sha256"
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

type poseFlipBounds struct{ Left, Top, Right, Bottom int }
type poseFlipFixture struct {
	Sheet   assetsync.Spritesheet
	Poses   assetsync.PoseFile
	Palette assetsync.Palette
}
type poseFlipFrameEvidence struct {
	Frame             uint32           `json:"frame"`
	FlipX             bool             `json:"flipX"`
	FlipY             bool             `json:"flipY"`
	Poses             []int            `json:"poses"`
	ExpectedBounds    []poseFlipBounds `json:"expectedBounds"`
	ActualBounds      []poseFlipBounds `json:"actualBounds"`
	OracleDifferences int              `json:"oracleDifferences"`
}
type poseFlipTelemetry struct {
	Frame         uint32 `json:"frame"`
	UploadedBytes uint32 `json:"uploadedBytes"`
	ResidentBytes uint32 `json:"residentBytes"`
	PoseID        uint16 `json:"poseId"`
	LCDStep       int    `json:"lcdStep"`
	FrameSHA      string `json:"frameSha256"`
}

// TestPoseFlipTargetDiagnostic uses an independent pixel oracle, not generated
// FlipOffset fields or renderer output, to prove the public anchoring semantics.
// MTILE_POSE_FLIP_OUT retains fixture input, native ROMs, ELF/maps, and evidence.
func TestPoseFlipTargetDiagnostic(t *testing.T) {
	out := os.Getenv("MTILE_POSE_FLIP_OUT")
	if out == "" {
		t.Skip("set MTILE_POSE_FLIP_OUT for native flip evidence")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(repository, "build", "player", "mtile-player")
	service, err := build.NewService("", filepath.Join(repository, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	service.Debug, service.KeepWork = true, true
	const warmup, samples = 2, 120
	t.Run("parity", func(t *testing.T) {
		dir := filepath.Join(out, "parity")
		project := filepath.Join(dir, "project")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		fixture := writePoseFlipFixture(t, project, false)
		environment, err := benchmarkEnvironment(service, project, helper)
		if err != nil {
			t.Fatal(err)
		}
		b4, c4, err := buildAndCapture(service, transpiler.RendererMode4, project, helper, dir, warmup, samples)
		if err != nil {
			t.Fatal(err)
		}
		b0, c0, err := buildAndCaptureAtFrames(service, transpiler.RendererMode0, project, helper, dir, warmup, samples, c4.frameNumbers)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateSynchronizedFrames(c4.frameNumbers, c0.frameNumbers); err != nil {
			t.Fatal(err)
		}
		s4, err := capturePresentWindow(helper, b4.ROMPath, warmup+samples)
		if err != nil {
			t.Fatal(err)
		}
		s0, err := capturePresentWindow(helper, b0.ROMPath, warmup+samples)
		if err != nil {
			t.Fatal(err)
		}
		different, oracleDifferent, lcdDifferent, lcdScans := compareFrames(c4.frames, c0.frames), 0, 0, 0
		var evidence []poseFlipFrameEvidence
		for i, frame := range c0.frameNumbers {
			expected, row := poseFlipOracle(fixture, frame)
			row.OracleDifferences = poseFlipRGB5Differences(expected, c0.frames[i]) + poseFlipRGB5Differences(expected, c4.frames[i])
			oracleDifferent += row.OracleDifferences
			for actor, region := range []poseFlipBounds{{0, 0, 75, 100}, {75, 0, 150, 100}, {150, 0, 240, 120}} {
				box := poseFlipPixelBounds(c0.frames[i], region)
				row.ActualBounds = append(row.ActualBounds, box)
				if box != row.ExpectedBounds[actor] {
					t.Errorf("frame %d actor %d bounds %+v, want %+v", frame, actor, box, row.ExpectedBounds[actor])
				}
			}
			evidence = append(evidence, row)
			if i < 8 || row.OracleDifferences != 0 && i == 0 {
				for _, image := range []struct {
					name   string
					pixels []byte
				}{{"mode0", c0.frames[i]}, {"mode4", c4.frames[i]}, {"oracle", poseFlipRGB5Image(expected)}} {
					if err := writePNG(filepath.Join(dir, fmt.Sprintf("frame%03d-%s.png", frame, image.name)), image.pixels); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		for tick := warmup; tick < len(s0); tick++ {
			expected, _ := poseFlipOracle(fixture, s0[tick].Tick)
			for _, scan := range s0[tick].Scans {
				lcdScans++
				lcdDifferent += countDifferingPixels(s4[tick].lastRGBA, scan.rgba)
				oracleDifferent += poseFlipRGB5Differences(expected, scan.rgba)
			}
		}
		r0 := preserveOBJOptimizationRenderer(t, dir, b0, c0, s0[warmup:])
		r4 := preserveOBJOptimizationRenderer(t, dir, b4, c4, s4[warmup:])
		writePSprJSON(t, filepath.Join(dir, "frames.json"), evidence)
		writePSprJSON(t, filepath.Join(dir, "report.json"), map[string]any{"schemaVersion": 1, "environment": environment, "warmupFrames": warmup, "samples": samples, "differentPixels": different, "oracleDifferentPixels": oracleDifferent, "lcdDifferentPixels": lcdDifferent, "lcdScans": lcdScans, "mode0": r0, "mode4": r4, "linkedMetadata": map[string]any{"mode0": poseFlipLinkedMetadata(t, b0), "mode4": poseFlipLinkedMetadata(t, b4)}, "frameBudgetCycles": twoFrameCycleBudget, "oracle": "Raw source pixels, authored part flips, and independently specified min/max rectangle; RGB5 compared for every LCD pixel. Clip bounds are the union across both unequal frames."})
		if different != 0 || oracleDifferent != 0 || lcdDifferent != 0 {
			t.Fatalf("renderer %d, oracle %d, LCD %d differing pixels", different, oracleDifferent, lcdDifferent)
		}
		if r0.MissedFrames != 0 || r4.MissedFrames != 0 {
			t.Fatal("fixture missed a game frame")
		}
		t.Logf("%d synchronized frames and %d LCD scans match both renderers and the independent oracle; Mode 0 median/p95/max %d/%d/%d cycles, Mode 4 %d/%d/%d", samples, lcdScans, r0.Frame.Median, r0.Frame.P95, r0.Frame.Max, r4.Frame.Median, r4.Frame.P95, r4.Frame.Max)
	})
	t.Run("flip-cache", func(t *testing.T) {
		dir := filepath.Join(out, "flip-cache")
		project := filepath.Join(dir, "project")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		writePoseFlipFixture(t, project, true)
		environment, err := benchmarkEnvironment(service, project, helper)
		if err != nil {
			t.Fatal(err)
		}
		service.Renderer = transpiler.RendererMode0
		result, err := service.Build(project, filepath.Join(dir, "mode0.gba"))
		if err != nil {
			t.Fatal(err)
		}
		still, stillRows := capturePoseFlipTelemetry(t, helper, result.ROMPath, 0, warmup, samples)
		flipped, flipRows := capturePoseFlipTelemetry(t, helper, result.ROMPath, emulator.KeyA, warmup, samples)
		if err := validateSynchronizedFrames(still.frameNumbers, flipped.frameNumbers); err != nil {
			t.Fatal(err)
		}
		if compareFrames(still.frames, flipped.frames) == 0 {
			t.Fatal("held A did not change the displayed facing")
		}
		stillSummary := preserveOBJOptimizationRenderer(t, dir, result, still, nil)
		flippedSummary := preserveOBJOptimizationRenderer(t, dir, result, flipped, nil)
		if err := writePNG(filepath.Join(dir, "no-flip-final.png"), still.lastRGB); err != nil {
			t.Fatal(err)
		}
		writePSprJSON(t, filepath.Join(dir, "frames.json"), map[string]any{"noFlip": stillRows, "alternatingFlip": flipRows})
		writePSprJSON(t, filepath.Join(dir, "report.json"), map[string]any{"schemaVersion": 1, "environment": environment, "samples": samples, "warmupFrames": warmup, "sameROMControl": true, "linkedMetadata": poseFlipLinkedMetadata(t, result), "mode0NoFlip": stillSummary, "mode0AlternatingFlip": flippedSummary, "medianCycleDelta": int64(flippedSummary.Frame.Median) - int64(stillSummary.Frame.Median), "p95CycleDelta": int64(flippedSummary.Frame.P95) - int64(stillSummary.Frame.P95), "profilingNote": "Same debug ROM and Lua update path. A up draws no flip; A held cycles none/X/Y/XY on one constant pose. RAM/ROM are identical by construction; cycle delta measures active flip branches, not compiler-version overhead. Every sampled present reports zero OBJ tile uploads."})
		t.Logf("Same ROM: no-flip median/p95 %d/%d cycles, alternating flips %d/%d; all %d warmed presents per run upload 0 B; EWRAM %d B, IWRAM %d B, ROM %d B", stillSummary.Frame.Median, stillSummary.Frame.P95, flippedSummary.Frame.Median, flippedSummary.Frame.P95, samples, result.EWRAMUsed, result.IWRAMUsed, result.ROMSize)
	})
}

func writePoseFlipFixture(t *testing.T, project string, steady bool) poseFlipFixture {
	t.Helper()
	if err := build.CreateProject(project, build.NewProjectOptions{SheetWidth: 32, SheetHeight: 16, MapWidth: 30, MapHeight: 20}); err != nil {
		t.Fatal(err)
	}
	f := poseFlipFixture{Sheet: assetsync.Spritesheet{Width: 32, Height: 24}, Palette: assetsync.Palette{Name: "aurora", Colors: make([]assetsync.RGB15, 256)}, Poses: assetsync.PoseFile{Version: 1}}
	copy(f.Palette.Colors, []assetsync.RGB15{{}, {R: 31}, {G: 31}, {B: 31}, {R: 31, G: 31}, {R: 31, B: 31}, {G: 31, B: 31}, {R: 31, G: 31, B: 31}})
	for id := 0; id < 9; id++ {
		cell := assetsync.SpriteRecord{ID: id, X: id % 4 * 8, Y: id / 4 * 8, Width: 8, Height: 8}
		for y := 0; y < 8; y++ {
			row := make([]int, 8)
			for x := range row {
				row[x] = 1 + (id+x/3+y/2)%7
				if x > 0 && x < 7 && y > 0 && y < 7 && (x+2*y+id)%7 == 0 {
					row[x] = 0
				}
			}
			cell.Pixels = append(cell.Pixels, row)
		}
		f.Sheet.Sprites = append(f.Sheet.Sprites, cell)
	}
	f.Poses.Poses = []assetsync.PoseRecord{
		{Name: "shared", Parts: []assetsync.PosePartRecord{{Tile: 0, X: -8, Y: -8, FlipX: true}, {Tile: 1, Y: -8}, {Tile: 2, X: 8, Y: -8, FlipY: true}, {Tile: 3, X: -8, FlipY: true}, {Tile: 4, X: 8, FlipX: true, FlipY: true}}},
		{Name: "wide", Parts: []assetsync.PosePartRecord{{Tile: 5, X: -16, Y: -8}, {Tile: 6, X: 24, Y: 8}}},
		{Name: "tall", Parts: []assetsync.PosePartRecord{{Tile: 7, Y: -24}, {Tile: 8, X: 8, Y: 16}}},
	}
	animation := assetsync.AnimationWorkspace{Version: 1, Clips: []assetsync.AnimationClip{{Name: "wide", Frames: []int{0, 1}, FPS: 8}, {Name: "tall", Frames: []int{0, 2}, FPS: 8}}}
	writePSprJSON(t, filepath.Join(project, "assets", "objsheet.json"), f.Sheet)
	writePSprJSON(t, filepath.Join(project, "assets", "poses.json"), f.Poses)
	writePSprJSON(t, filepath.Join(project, "assets", "animation.json"), animation)
	if err := assetsync.SavePalette(project, &f.Palette); err != nil {
		t.Fatal(err)
	}
	source := `tick=0
function _update() tick=tick+1 end
function _draw()
 cls(0)
 local fx=tick%2==1
 local fy=flr(tick/2)%2==1
 pspr(0,38,38,fx,fy)
 anim("wide",110,38,fx,fy,flr(tick/4)/8)
 anim("tall",180,60,fx,fy,flr(tick/4)/8)
end
`
	if steady {
		source = `tick=0
function _update() tick=tick+1 end
function _draw()
 cls(0)
 local fx=tick%2==1 and btn(5)
 local fy=flr(tick/2)%2==1 and btn(5)
 pspr(0,100,60,fx,fy)
end
`
	}
	if err := os.WriteFile(filepath.Join(project, "game.lua"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	return f
}

func poseFlipOracle(f poseFlipFixture, frame uint32) ([]assetsync.RGB15, poseFlipFrameEvidence) {
	fx, fy := frame%2 == 1, (frame/2)%2 == 1
	ids := []int{0, int(frame/4) % 2, int(frame/4) % 2 * 2}
	// Bounds are independent constants from the authored fixture coordinates.
	own := []poseFlipBounds{{-8, -8, 16, 8}, {-16, -8, 32, 16}, {0, -24, 16, 24}}
	reference := []poseFlipBounds{own[0], {-16, -8, 32, 16}, {-8, -24, 16, 24}}
	anchors := [][2]int{{38, 38}, {110, 38}, {180, 60}}
	pixels := make([]assetsync.RGB15, emulator.Width*emulator.Height)
	evidence := poseFlipFrameEvidence{Frame: frame, FlipX: fx, FlipY: fy, Poses: ids}
	for actor, id := range ids {
		b, r, a := own[id], reference[actor], anchors[actor]
		if fx {
			b.Left, b.Right = r.Left+r.Right-b.Right, r.Left+r.Right-b.Left
		}
		if fy {
			b.Top, b.Bottom = r.Top+r.Bottom-b.Bottom, r.Top+r.Bottom-b.Top
		}
		b.Left += a[0]
		b.Right += a[0]
		b.Top += a[1]
		b.Bottom += a[1]
		evidence.ExpectedBounds = append(evidence.ExpectedBounds, b)
		for _, part := range f.Poses.Poses[id].Parts {
			tile := f.Sheet.Sprites[part.Tile]
			for py := 0; py < 8; py++ {
				for px := 0; px < 8; px++ {
					sx, sy := px, py
					if part.FlipX {
						sx = 7 - sx
					}
					if part.FlipY {
						sy = 7 - sy
					}
					color := tile.Pixels[sy][sx]
					if color == 0 {
						continue
					}
					x, y := part.X+px, part.Y+py
					if fx {
						x = r.Left + r.Right - 1 - x
					}
					if fy {
						y = r.Top + r.Bottom - 1 - y
					}
					x += a[0]
					y += a[1]
					if x >= 0 && x < emulator.Width && y >= 0 && y < emulator.Height {
						pixels[y*emulator.Width+x] = f.Palette.Colors[color]
					}
				}
			}
		}
	}
	return pixels, evidence
}
func poseFlipRGB5Differences(expected []assetsync.RGB15, actual []byte) int {
	different := 0
	for i, want := range expected {
		if int(actual[i*4])>>3 != want.R || int(actual[i*4+1])>>3 != want.G || int(actual[i*4+2])>>3 != want.B {
			different++
		}
	}
	return different
}
func poseFlipRGB5Image(pixels []assetsync.RGB15) []byte {
	rgba := make([]byte, len(pixels)*4)
	for i, p := range pixels {
		rgba[i*4], rgba[i*4+1], rgba[i*4+2], rgba[i*4+3] = byte(p.R<<3), byte(p.G<<3), byte(p.B<<3), 255
	}
	return rgba
}
func poseFlipPixelBounds(pixels []byte, region poseFlipBounds) poseFlipBounds {
	b := poseFlipBounds{region.Right, region.Bottom, region.Left, region.Top}
	for y := region.Top; y < region.Bottom; y++ {
		for x := region.Left; x < region.Right; x++ {
			i := (y*emulator.Width + x) * 4
			if pixels[i]|pixels[i+1]|pixels[i+2] != 0 {
				b.Left = min(b.Left, x)
				b.Right = max(b.Right, x+1)
				b.Top = min(b.Top, y)
				b.Bottom = max(b.Bottom, y+1)
			}
		}
	}
	return b
}
func capturePoseFlipTelemetry(t *testing.T, helper, rom string, keys uint16, warmup, samples int) (capture, []poseFlipTelemetry) {
	t.Helper()
	session, err := emulator.Start(helper, rom)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var c capture
	var rows []poseFlipTelemetry
	var last uint32
	for step := 0; step < (warmup+samples)*16+240 && len(rows) < samples; step++ {
		f, err := session.Step(keys, false)
		if err != nil {
			t.Fatal(err)
		}
		if f.Overflow {
			t.Fatal("native audio overflow")
		}
		if !f.Telemetry.DebugValid {
			continue
		}
		s := f.Telemetry.Stats
		if s.HeapError != emulator.HeapOK {
			t.Fatal("native heap error")
		}
		if s.Frames <= uint32(warmup) {
			c.warmupMissed = s.MissedFrames
			continue
		}
		if s.Frames == last {
			continue
		}
		last = s.Frames
		if s.Frames != uint32(warmup+len(rows)+1) {
			t.Fatal("native telemetry skipped frame")
		}
		if !s.OBJTelemetryValid || !s.OBJStreamAvailable || !s.OBJStreamResidencyValid || s.OBJStreamResidentSlots != 1 || s.OBJStreamResidentPoseIDs[0] != 0 || s.OBJUploadedBytes != 0 {
			t.Fatalf("flip-only frame %d uploaded or invalidated cached pose: %+v", s.Frames, s)
		}
		if len(rows) > 0 && step-rows[len(rows)-1].LCDStep != 2 {
			t.Fatal("flip-only telemetry missed 30Hz cadence")
		}
		c.frames = append(c.frames, append([]byte(nil), f.RGBA...))
		c.frameNumbers = append(c.frameNumbers, s.Frames)
		c.cycles = append(c.cycles, s.LastFrame)
		c.draw = append(c.draw, s.LastDraw)
		c.commit = append(c.commit, s.LastBlit)
		c.missed = s.MissedFrames
		c.heapAllocated, c.heapLive, c.heapPeakLive = s.HeapAllocated, s.HeapLive, s.HeapPeakLive
		c.heapHandles, c.gcCount = s.HeapHandles, s.GCCount
		c.lastRGB = c.frames[len(c.frames)-1]
		rows = append(rows, poseFlipTelemetry{s.Frames, s.OBJUploadedBytes, s.OBJStreamResidentBytes, s.OBJStreamResidentPoseIDs[0], step, fmt.Sprintf("%x", sha256.Sum256(f.RGBA))})
	}
	if len(rows) != samples || c.missed != c.warmupMissed {
		t.Fatal("incomplete or missed native flip frames")
	}
	return c, rows
}

// TinyGo ARM layouts: PoseRange 16 B (formerly 8), AnimationClip 28 B (formerly 20).
// Only retained ELF symbols contribute to this measured metadata accounting.
func poseFlipLinkedMetadata(t *testing.T, result build.Result) map[string]uint64 {
	t.Helper()
	file, err := elf.Open(filepath.Join(result.WorkDir, "game.elf"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	symbols, err := file.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	resultBytes := map[string]uint64{}
	for _, symbol := range symbols {
		switch symbol.Name {
		case "main.poseIndexROM":
			resultBytes["poseIndexBytes"] = symbol.Size
			resultBytes["retainedPoseCount"] = symbol.Size / 16
		case "main.animationClipROM":
			resultBytes["clipIndexBytes"] = symbol.Size
			resultBytes["retainedClipCount"] = symbol.Size / 28
		}
	}
	resultBytes["addedFlipOffsetROMBytes"] = 8 * (resultBytes["retainedPoseCount"] + resultBytes["retainedClipCount"])
	return resultBytes
}
