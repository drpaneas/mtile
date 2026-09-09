//go:build runtimeperfdiagnostic

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/transpiler"
)

// TestSheetAlignmentTargetDiagnostic deliberately offsets the generated sheet
// slice without changing its length. ARM7 unaligned word loads rotate bytes,
// which host tests cannot detect. Both map and spr are checked against a Lua
// pset source-pattern oracle: comparing them to each other hides shared bugs.
func TestSheetAlignmentTargetDiagnostic(t *testing.T) {
	helper := os.Getenv("MTILE_DIVISION_PLAYER")
	if helper == "" {
		helper = filepath.Join("..", "..", "build", "player", "mtile-player")
	}
	if _, err := os.Stat(helper); err != nil {
		t.Skip("build the headless mtile-player or set MTILE_DIVISION_PLAYER")
	}
	out := os.Getenv("MTILE_SHEET_ALIGNMENT_OUT")
	if out == "" {
		out = t.TempDir()
	}
	project := filepath.Join(out, "project")
	if err := build.CreateProject(project, build.NewProjectOptions{SheetWidth: 32, SheetHeight: 8, MapWidth: 3, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	sheet := assetsync.Spritesheet{Width: 32, Height: 8}
	for id := 1; id <= 3; id++ {
		sprite := assetsync.SpriteRecord{ID: id, X: id * 8, Width: 8, Height: 8}
		for y := 0; y < 8; y++ {
			row := make([]int, 8)
			for x := range row {
				row[x] = (x+8*y+19*id)%255 + 1
				if (x+y)%11 == 0 {
					row[x] = 0
				}
			}
			sprite.Pixels = append(sprite.Pixels, row)
		}
		sheet.Sprites = append(sheet.Sprites, sprite)
	}
	m := assetsync.Map{Version: 2, Width: 3, Height: 1}
	for layer := 0; layer < 4; layer++ {
		cells := make([]int, 3)
		if layer == 0 {
			copy(cells, []int{1, 2, 3})
		}
		m.Layers = append(m.Layers, assetsync.MapLayer{ID: layer, Cells: cells})
	}
	for name, value := range map[string]any{"spritesheet.json": sheet, "map.json": m} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, "assets", name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	source := `tick=0
alignment=0
function _update() tick=tick+1 end
function _draw()
 cls(42)
 spr(1,0,0) spr(2,8,0) spr(3,16,0)
 map(0,0,0,0,16,3,1)
 for y=0,7 do for x=0,23 do
  local id=1+flr(x/8)
  local sx=x%8
  local v=(sx+8*y+19*id)%255+1
  if (sx+y)%11~=0 then pset(x,y+32,v) end
 end end
end
`
	if err := os.WriteFile(filepath.Join(project, build.GameFileName), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	service, err := build.NewService("", os.Getenv("MTILE_SHEET_ALIGNMENT_RUNTIME"))
	if err != nil {
		t.Fatal(err)
	}
	service.Debug, service.KeepWork = true, true
	service.Renderer = transpiler.RendererMode4
	service.DisableMapRasterCache = true
	result, err := service.Build(project, filepath.Join(out, "base.gba"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(result.WorkDir)
	mainPath := filepath.Join(result.WorkDir, "main.go")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	generated := string(data)
	// The fixture has exactly three authored compact cells. Four slice
	// positions exercise every actual pointer alignment, independent of the
	// linker-selected alignment of the byte-array global itself.
	for _, replacement := range [][2]string{
		{`_ "unsafe"`, `"unsafe"`},
		{"var sheetRAM [192]byte", "var sheetRAM [195]byte"},
		{"cartData.Sheet = sheetRAM[:]", "cartData.Sheet = sheetRAM[:192]"},
		{"func (g *Game) Draw(c *rt.Context) {", "func (g *Game) Draw(c *rt.Context) {\n offset := g.tick.Int() & 3\n cartData.Sheet = sheetRAM[offset:offset+192]\n copy(cartData.Sheet, sheetROM[:])"},
		{`debugMailbox.Add("alignment", rt.NumVal(rt.Number(0)))`, `debugMailbox.Add("alignment", rt.NumVal(rt.FromInt(int(uintptr(unsafe.Pointer(&cartData.Sheet[0]))&3))))`},
	} {
		if strings.Count(generated, replacement[0]) != 1 {
			t.Fatalf("alignment wrapper expected exactly one %q", replacement[0])
		}
		generated = strings.Replace(generated, replacement[0], replacement[1], 1)
	}
	if err := os.WriteFile(mainPath, []byte(generated), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "wrapped-main.go"), []byte(generated), 0600); err != nil {
		t.Fatal(err)
	}
	rom, err := filepath.Abs(filepath.Join(out, "alignment.gba"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(service.TinyGoBin, "build", "-gc=none", "-opt=2", "-target="+filepath.Join(result.WorkDir, "target-gba.json"),
		"-tags=mtile_map_compact,mtile_heapless,mtile_sheet_readonly,mtile_static_strings_1", "-o", rom, ".")
	cmd.Dir, cmd.Env = result.WorkDir, append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build offset wrapper: %v\n%s", err, output)
	}
	session, err := emulator.Start(helper, rom)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	seen := uint8(0)
	for step := 0; step < 120 && seen != 15; step++ {
		frame, err := session.Step(0, false)
		if err != nil {
			t.Fatal(err)
		}
		if !frame.Telemetry.DebugValid || frame.Telemetry.Stats.Frames < 3 {
			continue
		}
		alignment := -1
		for _, watch := range frame.Telemetry.Watches {
			if watch.Name == "alignment" && watch.Kind == 2 {
				alignment = int(watch.Raw >> 16)
			}
		}
		if alignment < 0 || alignment > 3 {
			t.Fatalf("missing valid source alignment watch: %d", alignment)
		}
		if seen&(1<<alignment) != 0 {
			continue
		}
		seen |= 1 << alignment
		for _, region := range []struct {
			name string
			y    int
		}{{"sprite", 0}, {"map", 16}} {
			different := 0
			for y := 0; y < 8; y++ {
				for x := 0; x < 24; x++ {
					a, b := ((y+region.y)*240+x)*4, ((y+32)*240+x)*4
					if !bytes.Equal(frame.RGBA[a:a+4], frame.RGBA[b:b+4]) {
						different++
					}
				}
			}
			if different != 0 {
				_ = writePNG(filepath.Join(out, fmt.Sprintf("alignment-%d.png", alignment)), frame.RGBA)
				t.Errorf("source alignment %d: %s differs from independent pset oracle at %d pixels", alignment, region.name, different)
			}
		}
	}
	if seen != 15 {
		t.Fatalf("captured alignment mask %04b, want all four alignments", seen)
	}
}
