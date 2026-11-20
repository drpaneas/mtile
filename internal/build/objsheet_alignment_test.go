package build

import (
	"bytes"
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// Odd-length pose tile metadata used to place the byte bank at address 2 mod 4,
// silently forcing each tile upload through the GBA memcpy byte loop.
func TestBuildObjectPixelsHaveWordAlignedROMStorage(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found")
	}
	if testing.Short() {
		t.Skip("real TinyGo link")
	}
	dir := filepath.Join(t.TempDir(), "aligned-objects")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	obj := assetsync.Spritesheet{Width: 24, Height: 8}
	var parts []assetsync.PosePartRecord
	var want []byte
	for id := 0; id < 3; id++ {
		sprite := assetsync.SpriteRecord{ID: id, X: id * 8, Width: 8, Height: 8}
		for y := 0; y < 8; y++ {
			row := make([]int, 8)
			for x := range row {
				row[x] = id + 1
				want = append(want, byte(id+1))
			}
			sprite.Pixels = append(sprite.Pixels, row)
		}
		obj.Sprites = append(obj.Sprites, sprite)
		parts = append(parts, assetsync.PosePartRecord{Tile: id, X: id * 8})
	}
	if err := assetsync.SaveObjSheet(dir, &obj); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SavePoses(dir, &assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Name: "pose", Parts: parts}}}); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveAnimationWorkspace(dir, &assetsync.AnimationWorkspace{Version: 1, Clips: []assetsync.AnimationClip{{Name: "idle", Frames: []int{0, 0}, FPS: 8}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(`function _draw() cls() anim("idle",100,60) end`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			service, err := NewService("", "")
			if err != nil {
				t.Fatal(err)
			}
			service.KeepWork = true
			service.Renderer = renderer
			result, err := service.Build(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.RemoveAll(result.WorkDir) }()
			binary, err := elf.Open(filepath.Join(result.WorkDir, "game.elf"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = binary.Close() }()
			symbols, err := binary.Symbols()
			if err != nil {
				t.Fatal(err)
			}
			for _, symbol := range symbols {
				if symbol.Name != "main.objSheetROM" {
					continue
				}
				if symbol.Value%4 != 0 {
					t.Fatalf("OBJ pixel bank address %#x is not word aligned", symbol.Value)
				}
				section := binary.Sections[symbol.Section]
				data, err := section.Data()
				if err != nil {
					t.Fatal(err)
				}
				offset := symbol.Value - section.Addr
				if symbol.Size != uint64(len(want)) || !bytes.Equal(data[offset:offset+symbol.Size], want) {
					t.Fatal("alignment changed tile bytes or added pixel padding")
				}
				if symbol.Value < 0x08000000 || symbol.Value >= 0x0a000000 {
					t.Fatalf("object pixels escaped ROM: %#x", symbol.Value)
				}
				return
			}
			t.Fatalf("%s ELF omitted OBJ pixels", renderer)
		})
	}
}
