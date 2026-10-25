package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestProjectFingerprintIsStableAndIgnoresRunOutputs(t *testing.T) {
	directory := t.TempDir()
	writeBenchmarkFile(t, filepath.Join(directory, "game.lua"), "function _draw() cls(1) end\n")
	writeBenchmarkFile(t, filepath.Join(directory, "assets", "map.json"), "{}\n")

	first, err := projectFingerprint(directory)
	if err != nil {
		t.Fatal(err)
	}
	writeBenchmarkFile(t, filepath.Join(directory, "game.gba"), "generated ROM")
	writeBenchmarkFile(t, filepath.Join(directory, "game.sav"), "generated save")
	second, err := projectFingerprint(directory)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("run outputs changed project fingerprint: %s != %s", first, second)
	}

	writeBenchmarkFile(t, filepath.Join(directory, "game.lua"), "function _draw() cls(2) end\n")
	third, err := projectFingerprint(directory)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("source change did not change project fingerprint")
	}
}

func TestProjectFingerprintIncludesRelativePaths(t *testing.T) {
	left := t.TempDir()
	right := t.TempDir()
	writeBenchmarkFile(t, filepath.Join(left, "a.lua"), "same")
	writeBenchmarkFile(t, filepath.Join(right, "b.lua"), "same")

	leftSHA, err := projectFingerprint(left)
	if err != nil {
		t.Fatal(err)
	}
	rightSHA, err := projectFingerprint(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftSHA == rightSHA {
		t.Fatal("relative path was not part of project fingerprint")
	}
}

func TestStageProjectWithSourceOverridesGameAndSkipsRunOutputs(t *testing.T) {
	project := t.TempDir()
	workload := filepath.Join(t.TempDir(), "workload.lua")
	writeBenchmarkFile(t, filepath.Join(project, "game.lua"), "old")
	writeBenchmarkFile(t, filepath.Join(project, "project.json"), "{}")
	writeBenchmarkFile(t, filepath.Join(project, "assets", "map.json"), "map")
	writeBenchmarkFile(t, filepath.Join(project, "game.gba"), "rom")
	writeBenchmarkFile(t, filepath.Join(project, "game.sav"), "save")
	writeBenchmarkFile(t, workload, "new")

	staged, err := stageProjectWithSource(project, workload)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(staged) }()
	game, err := os.ReadFile(filepath.Join(staged, "game.lua"))
	if err != nil || string(game) != "new" {
		t.Fatalf("staged game = %q, %v", game, err)
	}
	if _, err := os.Stat(filepath.Join(staged, "assets", "map.json")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"game.gba", "game.sav"} {
		if _, err := os.Stat(filepath.Join(staged, name)); !os.IsNotExist(err) {
			t.Fatalf("staged run output %s: %v", name, err)
		}
	}
}

func TestGraphicsCharacterizationWorkloadsRemainMode0Eligible(t *testing.T) {
	tests := []struct {
		name    string
		layers  int
		objects bool
	}{
		{"layers4", 4, false},
		{"parallax_layers", 4, false},
		{"map_arities_short", 4, false},
		{"map_arities_long", 4, false},
		{"map_mutation_isolation", 2, false},
		{"layered_mget", 4, false},
		{"layered_mset16", 1, false},
		{"layered_map_dynamic", 2, false},
		{"object_order_current", 2, true},
		{"object_depth_lanes", 4, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("testdata", "workloads", test.name+".lua"))
			if err != nil {
				t.Fatal(err)
			}
			_, report, err := transpiler.TranspileWithReport(test.name+".lua", string(source), transpiler.Options{
				Renderer:  transpiler.RendererMode0,
				SheetW:    240,
				SheetH:    160,
				MapW:      60,
				MapH:      20,
				MapLayers: 4,
				Map:       benchmarkLayerMap(60, 20, test.layers),
			})
			if err != nil {
				t.Fatal(err)
			}
			if report.Selected != transpiler.RendererMode0 || report.Mode0Layers != test.layers || report.Mode0Objects != test.objects {
				t.Fatalf("renderer report = %+v, want Mode 0 layers=%d objects=%t", report, test.layers, test.objects)
			}
		})
	}
}

func benchmarkLayerMap(width, height, layers int) []uint16 {
	data := make([]uint16, width*height*4)
	for layer := 0; layer < layers; layer++ {
		data[layer*width*height] = 1
	}
	return data
}

func TestZeroBGTextWorkloadRemainsAutoEligible(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "workloads", "zero_bg_text.lua"))
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := transpiler.TranspileWithReport("zero_bg_text.lua", string(source), transpiler.Options{
		Renderer: transpiler.RendererAuto,
		SheetW:   240,
		SheetH:   160,
		MapW:     30,
		MapH:     20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != transpiler.RendererMode0 || report.Mode0Layers != 0 || !report.Mode0Objects || !report.Mode0Text {
		t.Fatalf("renderer report = %+v, want zero-BG Mode 0 text", report)
	}
}

func TestTextInternWorkloadUses32OAMEntriesAndOneStaticTile(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "workloads", "text_intern.lua"))
	if err != nil {
		t.Fatal(err)
	}
	generated, report, err := transpiler.TranspileWithReport("text_intern.lua", string(source), transpiler.Options{
		Renderer: transpiler.RendererAuto,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != transpiler.RendererMode0 || !report.Mode0Text ||
		report.Mode0OAMEntries != 32 || report.Mode0OBJLineCycles != 256 {
		t.Fatalf("renderer report = %+v, want Mode 0 text with OAM=32 line=256", report)
	}
	if !strings.Contains(generated, "var mode0TextKeysROM = [1]uint16") ||
		!strings.Contains(generated, "var mode0TextTilesROM = [16]uint32") {
		t.Fatalf("generated workload did not intern 32 occurrences into one static glyph tile")
	}
}

func TestPack16WorkloadEnablesMode0PackedObjectCache(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "workloads", "pack16.lua"))
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := transpiler.TranspileWithReport("pack16.lua", string(source), transpiler.Options{
		Renderer: transpiler.RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != transpiler.RendererMode0 || !report.Mode0Objects || !report.Mode0ObjectPacking ||
		report.Mode0OAMEntries != 8 || report.Mode0OBJLineCycles != 64 {
		t.Fatalf("renderer report = %+v, want packed Mode 0 objects", report)
	}
}

func TestPack32WorkloadEnablesMode0PackedObjectCache(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "workloads", "pack32.lua"))
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := transpiler.TranspileWithReport("pack32.lua", string(source), transpiler.Options{
		Renderer: transpiler.RendererMode0,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != transpiler.RendererMode0 || !report.Mode0Objects || !report.Mode0ObjectPacking ||
		report.Mode0OAMEntries != 2 || report.Mode0OBJLineCycles != 64 {
		t.Fatalf("renderer report = %+v, want packed Mode 0 objects", report)
	}
}

func TestFullOAMBudgetWorkloadsRemainAutoEligible(t *testing.T) {
	tests := []struct {
		name       string
		oamEntries int
		lineCycles int
		packing    bool
	}{
		{name: "oam64", oamEntries: 64, lineCycles: 64},
		{name: "scanline38", oamEntries: 38, lineCycles: 1216, packing: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("testdata", "workloads", test.name+".lua"))
			if err != nil {
				t.Fatal(err)
			}
			_, report, err := transpiler.TranspileWithReport(test.name+".lua", string(source), transpiler.Options{
				Renderer: transpiler.RendererAuto,
				SheetW:   240,
				SheetH:   160,
			})
			if err != nil {
				t.Fatal(err)
			}
			if report.Selected != transpiler.RendererMode0 || report.Mode0OAMEntries != test.oamEntries ||
				report.Mode0OBJLineCycles != test.lineCycles || report.Mode0ObjectPacking != test.packing {
				t.Fatalf("renderer report = %+v, want Mode 0 OAM=%d line=%d packing=%t",
					report, test.oamEntries, test.lineCycles, test.packing)
			}
		})
	}
}

func TestDynamicSpriteFramesWorkloadRemainsAutoEligible(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "workloads", "dynamic_sprite_frames.lua"))
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := transpiler.TranspileWithReport("dynamic_sprite_frames.lua", string(source), transpiler.Options{
		Renderer: transpiler.RendererAuto,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != transpiler.RendererMode0 || !report.Mode0Objects || report.Mode0ObjectPacking ||
		report.Mode0OAMEntries != 16 || report.Mode0OBJLineCycles != 32 {
		t.Fatalf("renderer report = %+v, want bounded dynamic Mode 0 sprites with OAM=16 line=32", report)
	}
}

func TestWrappingSpriteFramesWorkloadRemainsAutoEligible(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "workloads", "wrapping_sprite_frames.lua"))
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := transpiler.TranspileWithReport("wrapping_sprite_frames.lua", string(source), transpiler.Options{
		Renderer: transpiler.RendererAuto,
		SheetW:   72,
		SheetH:   8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != transpiler.RendererMode0 || !report.Mode0Objects || report.Mode0ObjectPacking ||
		report.Mode0OAMEntries != 16 || report.Mode0OBJLineCycles != 32 {
		t.Fatalf("renderer report = %+v, want wrapping-counter Mode 0 sprites with OAM=16 line=32", report)
	}
}

func TestInactiveSpriteSlotsWorkloadRemainsAutoEligible(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "workloads", "inactive_sprite_slots.lua"))
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := transpiler.TranspileWithReport("inactive_sprite_slots.lua", string(source), transpiler.Options{
		Renderer: transpiler.RendererAuto,
		SheetW:   240,
		SheetH:   160,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Selected != transpiler.RendererMode0 || !report.Mode0Objects || report.Mode0ObjectPacking ||
		report.Mode0OAMEntries != 64 || report.Mode0OBJLineCycles != 64 {
		t.Fatalf("renderer report = %+v, want conservative bounded dynamic OAM=64 line=64", report)
	}
}

func TestApplyGatePolicySeparatesParityFromPerformanceEvidence(t *testing.T) {
	base := gateReport{
		ParityPass:      true,
		MemoryPass:      false,
		MedianCyclePass: true,
		P95HeadroomPass: true,
		MissedFramePass: true,
	}
	full := base
	if err := applyGatePolicy(&full, gateFull); err != nil {
		t.Fatal(err)
	}
	if full.Pass {
		t.Fatal("full gate accepted a workload below the memory threshold")
	}
	parity := base
	if err := applyGatePolicy(&parity, gateParity); err != nil {
		t.Fatal(err)
	}
	if !parity.Pass || parity.GatePolicy != gateParity {
		t.Fatalf("parity gate report = %+v, want semantic pass", parity)
	}
	parity.MissedFramePass = false
	if err := applyGatePolicy(&parity, gateParity); err != nil {
		t.Fatal(err)
	}
	if !parity.Pass {
		t.Fatal("parity gate treated recorded frame-delivery evidence as a parity failure")
	}
	parity.ParityPass = false
	if err := applyGatePolicy(&parity, gateParity); err != nil {
		t.Fatal(err)
	}
	if parity.Pass {
		t.Fatal("parity gate accepted differing pixels")
	}
	if err := applyGatePolicy(&parity, "unknown"); err == nil {
		t.Fatal("unknown gate policy was accepted")
	}
}

func TestValidateSynchronizedFramesRejectsOrdinallyAlignedDifferentTicks(t *testing.T) {
	if err := validateSynchronizedFrames([]uint32{21, 22, 24}, []uint32{21, 22, 24}); err != nil {
		t.Fatal(err)
	}
	if err := validateSynchronizedFrames([]uint32{21, 22, 24}, []uint32{21, 23, 24}); err == nil {
		t.Fatal("different runtime ticks were accepted as synchronized captures")
	}
	if err := validateSynchronizedFrames([]uint32{21, 22}, []uint32{21}); err == nil {
		t.Fatal("different capture lengths were accepted as synchronized")
	}
}

func TestFirstDifferingFrameReportsExactCapture(t *testing.T) {
	left := [][]byte{{0, 0, 0, 255}, {1, 2, 3, 255, 4, 5, 6, 255}}
	right := [][]byte{{0, 0, 0, 255}, {1, 2, 3, 255, 4, 5, 7, 255}}
	index, pixels := firstDifferingFrame(left, right)
	if index != 1 || pixels != 1 || compareFrames(left, right) != 1 {
		t.Fatalf("first difference = frame %d pixels %d total %d, want 1/1/1", index, pixels, compareFrames(left, right))
	}
}

func writeBenchmarkFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryGateAllowsBoundedMode0Cost(t *testing.T) {
	for _, test := range []struct {
		gain int64
		want bool
	}{
		{34040, true},
		{0, true},
		{-maximumEWRAMCost, true},
		{-maximumEWRAMCost - 1, false},
	} {
		if got := memoryGatePass(test.gain); got != test.want {
			t.Errorf("memoryGatePass(%d) = %t, want %t", test.gain, got, test.want)
		}
	}
}
