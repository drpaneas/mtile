package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/transpiler"
)

func TestParallaxRectanglesUsesExistingFourBackgroundAndObjectRenderer(t *testing.T) {
	dir := examplesProjectDir(t, "parallax-rectangles")
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(dir, GameFileName))
	if err != nil {
		t.Fatal(err)
	}
	generated, report, err := transpiler.TranspileWithReport("game.lua", string(source), transpiler.Options{
		Renderer: transpiler.RendererMode0, SheetW: assets.SheetW, SheetH: assets.SheetH,
		MapW: assets.MapW, MapH: assets.MapH, MapLayers: assets.MapLayers,
		Sheet: assets.SheetROM, Map: assets.MapROM, Flags: assets.FlagROM,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0Layers != 4 || report.MapDrawMask != 15 || !report.Mode0Objects || !report.Mode0Text {
		t.Fatalf("example does not exercise the standard four-BG/OBJ renderer: %+v", report)
	}
	if strings.Contains(generated, "Rectfill") || strings.Contains(generated, "Pset") || strings.Contains(generated, "tilecanvas") {
		t.Fatal("parallax example unexpectedly depends on pixel drawing or the private experiment")
	}
	if assets.MapW != 60 || assets.MapH != 20 || len(assets.MapROM) != 60*20*4 {
		t.Fatalf("map geometry cannot supply a repeating 31-column camera window")
	}
	// Every source window that crosses the 240-pixel camera wrap must see a
	// duplicate motif, including transparent holes and foreground terrain.
	for bg := 0; bg < 4; bg++ {
		for y := 0; y < 20; y++ {
			for x := 0; x < 30; x++ {
				index := bg*60*20 + y*60 + x
				id := assets.MapROM[index]
				if id != assets.MapROM[index+30] {
					t.Fatalf("BG%d motif does not wrap seamlessly at (%d,%d)", bg, x, y)
				}
			}
		}
	}
}
