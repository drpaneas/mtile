package build

import (
	"bytes"
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestBuildCartridgeSaveSignatureAndWRAMReader(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil || testing.Short() {
		t.Skip("requires a real TinyGo link")
	}
	dir := filepath.Join(t.TempDir(), "save")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	service, err := NewService("", "")
	if err != nil {
		t.Fatal(err)
	}
	service.KeepWork = true
	for _, renderer := range []transpiler.Renderer{transpiler.RendererMode0, transpiler.RendererMode4} {
		t.Run(renderer.String(), func(t *testing.T) {
			service.Renderer = renderer
			source := `function _init() save_set(63, -12.5) save_write("build-v1") save_read("build-v1") end function _draw() cls(0) end`
			if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := service.Build(dir, filepath.Join(t.TempDir(), "save.gba"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.RemoveAll(result.WorkDir) }()
			rom, err := os.ReadFile(result.ROMPath)
			if err != nil {
				t.Fatal(err)
			}
			offset := bytes.Index(rom, []byte("SRAM_Vnnn\x00\x00\x00"))
			if offset < 0 || offset%4 != 0 {
				t.Fatalf("missing/alignment of SRAM ID: %d", offset)
			}
			binary, err := elf.Open(filepath.Join(result.WorkDir, "game.elf"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = binary.Close() }()
			syms, err := binary.Symbols()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, symbol := range syms {
				if symbol.Name == "github.com/drpaneas/mtile/runtime/hw.ReadSRAMByte" {
					found = true
					if symbol.Value < gbaIWRAMOrigin || symbol.Value >= gbaIWRAMEnd {
						t.Fatalf("SRAM reader in ROM: %#x", symbol.Value)
					}
				}
			}
			if !found {
				t.Fatal("missing non-inlined SRAM reader")
			}
		})
	}
	if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(`function _draw() cls(0) end`), 0600); err != nil {
		t.Fatal(err)
	}
	plain, err := service.Build(dir, filepath.Join(t.TempDir(), "plain.gba"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(plain.WorkDir) }()
	rom, err := os.ReadFile(plain.ROMPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rom, []byte("SRAM_Vnnn")) || strings.Contains(plain.GeneratedGo, "saveMemory") {
		t.Fatal("non-saving game links SRAM support")
	}
}
