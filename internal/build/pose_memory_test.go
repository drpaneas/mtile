package build

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func paintMemoryCell(pixels []byte, width, id int, color byte) {
	cols := width / 8
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			pixels[(id/cols*8+y)*width+id%cols*8+x] = color
		}
	}
}

func TestPoseMemoryCountsArtworkWithoutBlankCapacityOrRepeatedReferences(t *testing.T) {
	pixels := make([]byte, 24*8)
	paintMemoryCell(pixels, 24, 0, 2)
	paintMemoryCell(pixels, 24, 1, 2)
	assets := &assetsync.Assets{ObjSheetW: 24, ObjSheetH: 8, ObjSpriteN: 3, ObjSheetROM: pixels, PoseIndex: []assetsync.FlatPose{{Count: 1}}, PoseNames: []string{"idle_0"}, AnimationClips: []assetsync.FlatAnimationClip{{Name: "idle"}}}
	got := analyzePoseMemory(assets)
	if got.Source != "obj" || got.SourcePixelBytes != 128 || got.UniquePixelBytes != 64 || got.PoseCount != 1 || got.ClipCount != 1 || !reflect.DeepEqual(got.PoseNames, []string{"idle_0"}) {
		t.Fatalf("deduplicated artwork: %+v", got)
	}
	assets.PoseNames[0] = "changed"
	if got.PoseNames[0] != "idle_0" {
		t.Fatal("last build pose names alias live assets")
	}
	// An authored bank is still library storage even if no pose references it.
	assets.PoseIndex = nil
	assets.PoseNames = nil
	assets.AnimationClips = nil
	got = analyzePoseMemory(assets)
	if got.PoseCount != 0 || got.SourcePixelBytes != 128 || got.UniquePixelBytes != 64 {
		t.Fatalf("unused bank confused with empty artwork: %+v", got)
	}
	got = analyzePoseMemory(&assetsync.Assets{ObjSheetW: 128, ObjSheetH: 256, ObjSpriteN: 512, ObjSheetROM: make([]byte, 128*256)})
	if got.Source != "obj" || got.SourcePixelBytes != 0 || got.UniquePixelBytes != 0 {
		t.Fatalf("blank page counted as artwork: %+v", got)
	}

	shared := make([]byte, 32*8)
	paintMemoryCell(shared, 32, 0, 4)
	paintMemoryCell(shared, 32, 1, 4)
	paintMemoryCell(shared, 32, 3, 5)
	assets = &assetsync.Assets{SheetW: 32, SheetH: 8, SpriteN: 4, SheetROM: shared, PoseIndex: []assetsync.FlatPose{{Count: 4}}, PoseParts: []assetsync.FlatPosePart{{Tile: 0}, {Tile: 1}, {Tile: 0}, {Tile: 2}}}
	got = analyzePoseMemory(assets)
	if got.Source != "shared" || got.SourcePixelBytes != 128 || got.UniquePixelBytes != 64 || got.PixelROMBytes != 0 {
		t.Fatalf("shared pose artwork includes blank/repeated/background-only cells: %+v", got)
	}
	assets.PoseIndex = nil
	assets.PoseParts = nil
	got = analyzePoseMemory(assets)
	if got.Source != "none" || got.SourcePixelBytes != 0 || got.UniquePixelBytes != 0 {
		t.Fatalf("background-only cartridge reported pose artwork: %+v", got)
	}
}

func TestPoseMemoryUsesTargetELFSymbolsAndDistinguishesUnavailableFromElided(t *testing.T) {
	report := PoseMemoryReport{Source: "obj", SourcePixelBytes: 128, UniquePixelBytes: 64}
	symbols := []elf.Symbol{{Name: "main.objSheetROM", Value: 0x08000000, Size: 64}, {Name: "main.sheetROM", Value: 0x08000100, Size: 4096}, {Name: "main.poseOBJStageRAM", Value: 0x02000000, Size: 128}}
	metadata := map[string]uint64{"main.objSheetIndexROM": 16, "main.poseIndexROM": 8, "main.posePartROM": 16, "main.animationClipROM": 20, "main.animationFrameROM": 6, "main.poseOBJIndexROM": 16, "main.poseOBJPartsROM": 10, "main.poseOBJTilesROM": 6}
	var wantMeta uint64
	for name, size := range metadata {
		symbols = append(symbols, elf.Symbol{Name: name, Value: 0x08002000, Size: size})
		wantMeta += size
	}
	symbols = append(symbols, symbols[0]) // Do not double count duplicate symbol records.
	applyPoseMemorySymbols(&report, symbols)
	if !report.LinkedMemoryAvailable || !report.PixelsLinked || !report.MetadataLinked || !report.StageLinked || report.PixelROMBytes != 64 || report.MetadataROMBytes != wantMeta || report.StageRAMBytes != 128 {
		t.Fatalf("linked storage: %+v", report)
	}
	var elided PoseMemoryReport
	applyPoseMemorySymbols(&elided, nil)
	if !elided.LinkedMemoryAvailable || elided.PixelsLinked || elided.MetadataLinked || elided.StageLinked || elided.PixelROMBytes != 0 {
		t.Fatalf("DCE must be known zero storage: %+v", elided)
	}
	missing := summarizePoseMemory(nil, filepath.Join(t.TempDir(), "missing.elf"))
	if missing.LinkedMemoryAvailable {
		t.Fatal("missing ELF reported known zero memory")
	}
	var resident PoseMemoryReport
	applyPoseMemorySymbols(&resident, []elf.Symbol{{Name: "main.poseOBJResidentROM", Value: 0x08004000, Size: 640}})
	if resident.PixelROMBytes != 640 || resident.ResidentVRAMBytes != 640 || resident.StageLinked || resident.StageRAMBytes != 0 {
		t.Fatalf("resident library is ROM and OBJ VRAM, not EWRAM staging: %+v", resident)
	}
}

func TestBuildPoseMemoryReportMatchesLinkedMode0AndMode4(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found")
	}
	if testing.Short() {
		t.Skip("real TinyGo link")
	}
	dir := filepath.Join(t.TempDir(), "pose-memory")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	obj := assetsync.Spritesheet{Width: 24, Height: 8}
	for id := 0; id < 2; id++ {
		record := assetsync.SpriteRecord{ID: id, X: id * 8, Width: 8, Height: 8}
		for y := 0; y < 8; y++ {
			record.Pixels = append(record.Pixels, []int{2, 2, 2, 2, 2, 2, 2, 2})
		}
		obj.Sprites = append(obj.Sprites, record)
	}
	if err := assetsync.SaveObjSheet(dir, &obj); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SavePoses(dir, &assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Name: "idle_0", Parts: []assetsync.PosePartRecord{{Tile: 0}}}, {Name: "idle_1", Parts: []assetsync.PosePartRecord{{Tile: 1}}}}}); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveAnimationWorkspace(dir, &assetsync.AnimationWorkspace{Version: 1, Clips: []assetsync.AnimationClip{{Name: "idle", Frames: []int{0, 1}, FPS: 8}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(`function _draw() cls() anim("idle",20,30) end`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			service, err := NewService("", "")
			if err != nil {
				t.Fatal(err)
			}
			service.Renderer = renderer
			result, err := service.Build(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			got := result.PoseMemory
			if got.PoseCount != 2 || got.ClipCount != 1 || got.Source != "obj" || got.SourcePixelBytes != 128 || got.UniquePixelBytes != 64 || !reflect.DeepEqual(got.PoseNames, []string{"idle_0", "idle_1"}) {
				t.Fatalf("source inventory: %+v", got)
			}
			if !got.LinkedMemoryAvailable || !got.PixelsLinked || !got.MetadataLinked || got.PixelROMBytes != 64 || got.MetadataROMBytes == 0 {
				t.Fatalf("actual linked inventory: %+v", got)
			}
			if renderer == transpiler.RendererMode0 {
				if !got.StageLinked || got.StageRAMBytes != 64 {
					t.Fatalf("Mode 0 staging: %+v", got)
				}
			} else if got.StageLinked || got.StageRAMBytes != 0 {
				t.Fatalf("Mode 4 must not claim hardware pose staging: %+v", got)
			}
		})
	}
}
