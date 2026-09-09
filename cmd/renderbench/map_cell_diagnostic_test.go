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

// TestMapCellTargetDiagnostic compares the actual ARM map fast paths with
// individual spr calls, which retain the general pixel blitter. The two ROMs
// use identical assets, cameras, clips, and source palette/transparency state.
func TestMapCellTargetDiagnostic(t *testing.T) {
	helper := os.Getenv("MTILE_DIVISION_PLAYER")
	if helper == "" {
		helper = filepath.Join("..", "..", "build", "player", "mtile-player")
	}
	if _, err := os.Stat(helper); err != nil {
		t.Skip("build the headless mtile-player or set MTILE_DIVISION_PLAYER")
	}
	out := os.Getenv("MTILE_MAP_CELL_OUT")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(out, "project")
	if err := build.CreateProject(project, build.NewProjectOptions{SheetWidth: 32, SheetHeight: 8, MapWidth: 60, MapHeight: 20}); err != nil {
		t.Fatal(err)
	}
	sheet := assetsync.Spritesheet{Width: 32, Height: 8}
	for id := 1; id <= 3; id++ {
		sprite := assetsync.SpriteRecord{ID: id, X: id * 8, Width: 8, Height: 8, Flags: id}
		for y := 0; y < 8; y++ {
			row := make([]int, 8)
			for x := range row {
				switch id {
				case 1:
					row[x] = 206
				case 2:
					// Mixed words, empty rows, fully opaque rows, and 255.
					row[x] = (x*37 + y*61) % 256
					if y == 0 || (x+y)%5 == 0 {
						row[x] = 0
					}
					if y == 7 {
						row[x] = 255
					}
				case 3:
					row[x] = 255
				}
			}
			sprite.Pixels = append(sprite.Pixels, row)
		}
		sheet.Sprites = append(sheet.Sprites, sprite)
	}
	m := assetsync.Map{Version: 2, Width: 60, Height: 20}
	for layer := 0; layer < 4; layer++ {
		cells := make([]int, 60*20)
		for y := 0; y < 20; y++ {
			for x := 0; x < 60; x++ {
				if (y+layer*3)%9 < 2 && x%17 < 12 {
					cells[y*60+x] = 1 + (x/6+layer)%3
				}
			}
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
	var reference capture
	for _, name := range []string{"sprite-oracle", "map", "map-without-cache"} {
		oracle := name == "sprite-oracle"
		var source strings.Builder
		source.WriteString(`tick=0
function _update()
 tick=tick+1
 if tick==8 then mset(3,1,1,2) end
 if tick==16 then mset(3,1,1,0) end
end
function _draw()
 pal() palt()
 cls(42)
 clip(2+tick%3,1,234,158)
 if tick%5==0 then pal(206,0) end
 if tick%7==0 then palt(255,true) end
`)
		for layer := 0; layer < 4; layer++ {
			fmt.Fprintf(&source, " camera((tick*%d)%%13-6,tick%%7-3)\n", layer+1)
			if oracle {
				fmt.Fprintf(&source, " for y=0,19 do for x=0,59 do local id=mget(%d,x,y) if id>0 then spr(id,x*8,y*8) end end end\n", layer)
			} else {
				fmt.Fprintf(&source, " map(%d,0,0,0,0,60,20)\n", layer)
			}
		}
		source.WriteString("end\n")
		if err := os.WriteFile(filepath.Join(project, build.GameFileName), []byte(source.String()), 0600); err != nil {
			t.Fatal(err)
		}
		service, err := build.NewService("", "")
		if err != nil {
			t.Fatal(err)
		}
		service.Debug = true
		service.DisableMapRasterCache = name == "map-without-cache"
		dir := filepath.Join(out, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		result, captured, err := buildAndCaptureAtFrames(service, transpiler.RendererMode4, project, helper, dir, 2, 28, reference.frameNumbers)
		if err != nil {
			t.Fatal(err)
		}
		if oracle {
			reference = captured
		} else {
			if err := validateSynchronizedFrames(reference.frameNumbers, captured.frameNumbers); err != nil {
				t.Fatal(err)
			}
			if differences := compareFrames(reference.frames, captured.frames); differences != 0 {
				for i, frame := range captured.frames {
					if countDifferingPixels(reference.frames[i], frame) != 0 {
						t.Logf("first mismatch frame %d: %d pixels", captured.frameNumbers[i], countDifferingPixels(reference.frames[i], frame))
						_ = writePNG(filepath.Join(out, "reference.png"), reference.frames[i])
						_ = writePNG(filepath.Join(out, "map.png"), frame)
						break
					}
				}
				t.Fatalf("ARM map/sprite oracle differs by %d pixels", differences)
			}
			if compareFrames(captured.frames[:1], captured.frames[1:2]) == 0 {
				t.Fatal("camera fixture produced static pixels")
			}
			t.Logf("%s: 28 exact ARM frames: median=%d p95=%d IWRAM=%d ROM=%d", name, percentile(captured.cycles, 50), percentile(captured.cycles, 95), result.IWRAMUsed, result.ROMSize)
		}
	}
}
