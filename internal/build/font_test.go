package build

import (
	"bytes"
	"debug/elf"
	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/transpiler"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildLinksExactlyOneSelectedFontInROM(t *testing.T) {
	if testing.Short() {
		t.Skip("real TinyGo link")
	}
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("TinyGo unavailable")
	}
	dir := filepath.Join(t.TempDir(), "font")
	if err := CreateProject(dir, NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, text string
		renderer   transpiler.Renderer
	}{{"pico8", "A", transpiler.RendererMode4}, {"arcade", "A", transpiler.RendererMode0}, {"international", "Ωä", transpiler.RendererMode4}} {
		t.Run(test.name, func(t *testing.T) {
			project, err := assetsync.LoadProject(dir)
			if err != nil {
				t.Fatal(err)
			}
			project.Font = test.name
			if err := assetsync.SaveProject(dir, project); err != nil {
				t.Fatal(err)
			}
			source := `function _draw() cls() print("` + test.text + `",0,0,7) end`
			if err := os.WriteFile(filepath.Join(dir, GameFileName), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			// An unselected custom font must never appear in generated/linked data.
			inactive, _ := assetsync.LoadFont("", "german")
			inactive.Name = "unused"
			if err := assetsync.SaveFont(dir, inactive); err != nil {
				t.Fatal(err)
			}
			service, err := NewService("", "")
			if err != nil {
				t.Fatal(err)
			}
			service.KeepWork = true
			service.Renderer = test.renderer
			result, err := service.Build(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(result.WorkDir)
			binary, err := elf.Open(filepath.Join(result.WorkDir, "game.elf"))
			if err != nil {
				t.Fatal(err)
			}
			defer binary.Close()
			symbols, err := binary.Symbols()
			if err != nil {
				t.Fatal(err)
			}
			found := 0
			selected, _ := assetsync.LoadFont(dir, test.name)
			font, _ := assetsync.CompileFont(selected)
			var want []byte
			for _, col := range font.ASCII {
				for _, bits := range col {
					want = append(want, byte(bits), byte(bits>>8))
				}
			}
			for _, symbol := range symbols {
				if strings.Contains(symbol.Name, "builtinFont") || strings.Contains(symbol.Name, "pico8Font") {
					t.Fatalf("fallback font linked: %s", symbol.Name)
				}
				if symbol.Name != "main.projectFontROM" {
					continue
				}
				found++
				if symbol.Value < 0x08000000 || symbol.Value >= 0x0a000000 {
					t.Fatal("font copied into RAM")
				}
				section := binary.Sections[symbol.Section]
				data, err := section.Data()
				if err != nil {
					t.Fatal(err)
				}
				offset := symbol.Value - section.Addr
				if !bytes.Equal(data[offset:offset+uint64(len(want))], want) {
					t.Fatal("linked font pixels differ from selected font")
				}
			}
			if found != 1 {
				t.Fatalf("font banks=%d", found)
			}
		})
	}
}

func TestFontDemoUsesSupportedGlyphsAndMode4(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "font-demo")
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	font := assets.CompiledFont()
	source, err := os.ReadFile(filepath.Join(dir, GameFileName))
	if err != nil {
		t.Fatal(err)
	}
	// Every non-ASCII character in this teaching sample must have actual artwork.
	for _, cp := range string(source) {
		if cp < 128 {
			continue
		}
		found := false
		for _, g := range font.Extra {
			if g.Codepoint == cp {
				found = true
				if g.Columns == [6]uint16{} {
					t.Fatalf("blank demo glyph %c", cp)
				}
			}
		}
		if !found {
			t.Fatalf("unsupported demo glyph %c", cp)
		}
	}
	analysis := (Service{Renderer: transpiler.RendererAuto}).CheckProject(dir, nil)
	if !analysis.RendererReady || analysis.Renderer.Selected != transpiler.RendererMode4 || !strings.Contains(analysis.Renderer.Diagnostic, "M0_PRINT_SHAPE") {
		t.Fatalf("demo selection=%+v", analysis)
	}
}
