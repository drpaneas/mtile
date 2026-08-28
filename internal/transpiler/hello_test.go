package transpiler

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func examplesHelloDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate this test file")
	}
	dir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "examples", "hello"))
	if _, err := os.Stat(filepath.Join(dir, "game.lua")); err != nil {
		t.Fatalf("examples/hello is missing game.lua: %v", err)
	}
	return dir
}

// TestExamplesHelloFirstFrameAndLeftMove locks the hello example's actual
// Lua scene, not just that TinyGo can link a ROM. A transpiler regression
// that still produces a valid GBA header (dropped print, spr args, or btn
// mapping) would leave TestBuildExamplesHelloEndToEnd green.
func TestExamplesHelloFirstFrameAndLeftMove(t *testing.T) {
	dir := examplesHelloDir(t)
	source, err := os.ReadFile(filepath.Join(dir, "game.lua"))
	if err != nil {
		t.Fatal(err)
	}
	assets, err := assetsync.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Transpile("game.lua", string(source), Options{
		PackageName: "main",
		SheetW:      assets.SheetW, SheetH: assets.SheetH,
		MapW: assets.MapW, MapH: assets.MapH,
		Sheet: assets.SheetROM, Map: assets.MapROM, Flags: assets.FlagROM,
		Palette: assets.PaletteROM[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	harness := `package main

import (
	"testing"

	rt "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/hw"
)

func initCart(t *testing.T, c *rt.Context) {
	t.Helper()
	cartImage := rt.AssetImage{
		SheetW: SheetW, SheetH: SheetH, SpriteCols: SpriteCols, SpriteRows: SpriteRows, SpriteN: SpriteN,
		MapW: MapW, MapH: MapH, SheetReadOnly: SheetReadOnly, Sheet: sheetROM[:], SheetIndex: sheetIndexROM[:], Map: mapROM[:], Flags: flagROM[:],
	}
	cartData.SheetW, cartData.SheetH = SheetW, SheetH
	cartData.SpriteCols, cartData.SpriteRows, cartData.SpriteN = SpriteCols, SpriteRows, SpriteN
	cartData.MapW, cartData.MapH = MapW, MapH
	cartData.SheetReadOnly = SheetReadOnly
	cartData.Sheet, cartData.SheetIndex, cartData.Map, cartData.Flags = sheetRAM[:], sheetIndexRAM[:], mapRAM[:], flagRAM[:]
	if !c.Init(&cartImage, &cartData) {
		t.Fatal("runtime Init failed")
	}
}

func paintHello(c *rt.Context, x, y int, dpad bool) {
	c.Cls(15)
	// Sprite 1 pixels from examples/hello/assets/spritesheet.json, blitted
	// with Pset rather than Spr. Generated cart RAM is shared, so a want-side
	// Spr would stay green if flatten or Init left that RAM blank.
	helloSprite := [8][8]byte{
		{0, 0, 31, 31, 31, 31, 0, 0},
		{0, 31, 37, 37, 37, 37, 31, 0},
		{31, 37, 40, 40, 40, 40, 37, 31},
		{31, 37, 40, 18, 18, 40, 37, 31},
		{31, 37, 40, 18, 18, 40, 37, 31},
		{31, 37, 40, 40, 40, 40, 37, 31},
		{0, 31, 37, 37, 37, 37, 31, 0},
		{0, 0, 31, 31, 31, 31, 0, 0},
	}
	blitHelloSprite := func(dstX, dstY int) {
		for row := 0; row < 8; row++ {
			for col := 0; col < 8; col++ {
				if color := helloSprite[row][col]; color != 0 {
					c.Pset(dstX+col, dstY+row, int(color))
				}
			}
		}
	}
	// map() with no args covers the default 240x160 clip (30x20 cells) at
	// cellX=0, cellY=0, screenX=0, screenY=0. The cart's map only ever
	// references sprite 0 (empty, skipped) or sprite 1 (helloSprite), so
	// reusing that same pixel table here - independent of the runtime's own
	// Map/Spr code path - still exercises whatever cells are actually
	// authored in assets/map.json instead of pinning specific coordinates.
	const mapCellsW, mapCellsH = 30, 20
	for cy := 0; cy < mapCellsH && cy < MapH; cy++ {
		for cx := 0; cx < mapCellsW && cx < MapW; cx++ {
			if mapROM[cy*MapW+cx] != 0 {
				blitHelloSprite(cx*8, cy*8)
			}
		}
	}
	blitHelloSprite(x, y)
	c.Print("HELLO GBA", 8, 8, 7)
	hint := 6
	if dpad {
		hint = 40
	}
	c.Print("ARROWS MOVE", 8, 150, hint)
}

func assertFramesEqual(t *testing.T, got, want *rt.Context, label string) {
	t.Helper()
	for y := 0; y < rt.ScreenHeight; y++ {
		for x := 0; x < rt.ScreenWidth; x++ {
			if g, w := got.Pget(x, y), want.Pget(x, y); g != w {
				t.Fatalf("%s framebuffer mismatch at (%d,%d): got %d, want %d", label, x, y, g, w)
			}
		}
	}
}

func TestHelloExampleScene(t *testing.T) {
	hw.ResetForTest()
	var g Game
	var got, want rt.Context
	initCart(t, &got)
	g.Init(&got)
	g.Draw(&got)
	paintHello(&want, 112, 72, false)
	assertFramesEqual(t, &got, &want, "first frame")

	hw.SetKeyInputForTest(0x03FF &^ 0x0020)
	got.PollInput()
	g.Update(&got)
	g.Draw(&got)
	paintHello(&want, 111, 72, true)
	assertFramesEqual(t, &got, &want, "after btn(0)")
}
`
	runGeneratedHarness(t, got, harness, "TestHelloExampleScene")
}
