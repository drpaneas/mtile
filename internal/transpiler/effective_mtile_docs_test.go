package transpiler

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Run the programs printed in Effective mTile, including their stated asset
// preconditions. Output/parity checks are separate from fast-path eligibility.
func TestEffectiveMtilePrograms(t *testing.T) {
	data, err := os.ReadFile("../../docs/performance.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```lua\\n-- effective-example: ([a-z-]+)\\n(.*?)\\n```").FindAllStringSubmatch(string(data), -1)
	if len(blocks) != 22 {
		t.Fatalf("got %d examples, want eleven complete pairs", len(blocks))
	}
	seen := make(map[string]bool)
	for _, block := range blocks {
		name, source := block[1], block[2]
		if seen[name] {
			t.Fatal("duplicate example", name)
		}
		seen[name] = true
		t.Run(name, func(t *testing.T) {
			cells := make([]uint16, 12)
			for i := range cells {
				cells[i] = uint16(i % 4)
			}
			if name == "asset-after" {
				cells[0] = 2
			}
			opts := Options{Renderer: RendererMode4, SheetW: 32, SheetH: 8, MapW: 4, MapH: 3, MapLayers: 1, Map: cells}
			if name == "placement-after" {
				opts.IWRAMFunctions = []string{"probe"}
			}
			got, report, err := TranspileWithReport("effective/"+name+".lua", source, opts)
			if err != nil {
				t.Fatal(err)
			}
			// Older compilers lack these report fields. The same test also runs on
			// the pinned optimized revision, where these assertions are mandatory.
			reported := reflect.ValueOf(report)
			for prefix, field := range map[string]string{"wrap-": "BoundedMapReads", "palette-": "PartialPalette"} {
				if strings.HasPrefix(name, prefix) {
					if value := reported.FieldByName(field); value.IsValid() {
						if value.Bool() != strings.HasSuffix(name, "after") {
							t.Fatalf("%s eligibility differs", field)
						}
					}
				}
			}
			if name == "fixed-after" {
				if value := reported.FieldByName("AutomaticLoopIWRAMFunctions"); value.IsValid() {
					selected := false
					for i := 0; i < value.Len(); i++ {
						selected = selected || value.Index(i).String() == "probe"
					}
					if !selected {
						t.Fatal("fixed probe no longer selected")
					}
				}
			}
			first, second := -1, -1
			switch strings.Split(name, "-")[0] {
			case "reuse":
				first, second = 10, 14
			case "meaning":
				first, second = 0, 13
			case "list":
				first, second = 14, 18
			case "fixed":
				first, second = 96, 96
			case "wrap":
				first, second = 384, 384
			case "placement":
				first, second = 6, 7
			}
			totalCheck := ""
			if first >= 0 {
				totalCheck = fmt.Sprintf("want:=[]int{%d,%d}[tick-1];if g.total!=rt.FromInt(want){t.Fatalf(\"total=%%v want%%d\",g.total,want)}", first, second)
			}
			drawingCheck := ""
			if strings.HasPrefix(name, "shape-") {
				drawingCheck = `for y:=0;y<160;y++{for x:=0;x<240;x++{want:=byte(1);if x>=20&&x<=29&&y>=10&&y<=19{want=7};if c.Pget(x,y)!=want{t.Fatalf("shape pixel%d,%d",x,y)}}}`
			}
			if strings.HasPrefix(name, "map-draw-") || strings.HasPrefix(name, "asset-") {
				edited := "false"
				if strings.HasPrefix(name, "asset-") {
					edited = "true"
				}
				drawingCheck = `for y:=0;y<160;y++{for x:=0;x<240;x++{want:=byte(1);if x<32&&y<24{id:=x/8;if ` + edited + `&&x<8&&y<8{id=2};if id>0{want=byte(id+1)}};if c.Pget(x,y)!=want{t.Fatalf("map pixel%d,%d got%d want%d",x,y,c.Pget(x,y),want)}}}`
			}
			if strings.HasPrefix(name, "text-") {
				drawingCheck = `var reference rt.Context;reference.Cls(1);reference.Print("SCORE",8,8,7);ink:=0;for y:=0;y<160;y++{for x:=0;x<240;x++{if c.Pget(x,y)!=reference.Pget(x,y){t.Fatalf("text pixel%d,%d",x,y)};if c.Pget(x,y)==7{ink++}}};if ink==0{t.Fatal("empty label")}`
			}
			if strings.HasPrefix(name, "palette-") {
				drawingCheck = `if g.phase!=rt.FromInt(tick%2){t.Fatal("phase")};for i:=0;i<256;i++{source:=i;if i==2{source=4+tick%2};if hw.ReadPaletteRAM16(i)!=c.PaletteEntry(source){t.Fatalf("palette entry%d",i)}};for y:=0;y<160;y++{for x:=0;x<240;x++{if c.Pget(x,y)!=2{t.Fatal("palette pixel")}}}`
			}
			initEdit := ""
			if name == "asset-after" {
				initEdit = "image.Map[0]=2"
			}
			harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime";"github.com/drpaneas/mtile/runtime/hw")
func TestEffectiveProgram(t *testing.T){
 hw.ResetForTest()
 image:=rt.AssetImage{SheetW:32,SheetH:8,SpriteCols:4,SpriteRows:1,SpriteN:4,MapW:4,MapH:3,MapLayers:1,MapPlaneMask:1,Sheet:make([]byte,256),Flags:make([]byte,4),Map:make([]uint16,12)}
 for y:=0;y<8;y++{for x:=8;x<32;x++{image.Sheet[y*32+x]=byte(x/8+1)}}
 for i:=range image.Map{image.Map[i]=uint16(i%4)}
 ` + initEdit + `
 memory:=rt.AssetMemory{SheetW:32,SheetH:8,SpriteCols:4,SpriteRows:1,SpriteN:4,MapW:4,MapH:3,MapLayers:1,MapPlaneMask:1,Sheet:make([]byte,256),Flags:make([]byte,4),Map:make([]uint16,12)}
 var g Game;var c rt.Context;if !c.Init(&image,&memory){t.Fatal("fixture rejected")};g.Init(&c);c.ApplyDisplayPalette()
 for tick:=1;tick<=2;tick++{g.Update(&c);c.BeginRenderFrame();g.Draw(&c);c.ApplyDisplayPalette();` + totalCheck + drawingCheck + `}
}
`
			runGeneratedHarness(t, got, harness, "TestEffectiveProgram")
		})
	}
}
