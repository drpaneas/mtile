//go:build mtile_mode0

package runtime

import (
	"fmt"
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func requireMode0LayerCount(t *testing.T, want int) {
	t.Helper()
	if mode0LayerCount != want {
		t.Skipf("test requires %d linked Mode 0 layer(s), got %d", want, mode0LayerCount)
	}
}

func TestMode0UploadsTilesMapAndTransparentBackdrop(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	var cell [64]byte
	for i := range cell {
		cell[i] = byte(i + 1)
	}
	image := AssetImage{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		MapW: 2, MapH: 1, SheetReadOnly: true,
		Sheet: cell[:], SheetIndex: []uint16{0, 1}, Map: []uint16{1, 0}, Flags: []byte{0, 0},
	}
	ramSheet := make([]byte, 64)
	ram := AssetMemory{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		MapW: 2, MapH: 1, SheetReadOnly: true,
		Sheet: ramSheet, SheetIndex: make([]uint16, 2), Map: make([]uint16, 2), Flags: make([]byte, 2),
	}
	var palette [256]uint16
	palette[5] = 0x1234
	var c Context
	if !c.Init(&image, &ram) {
		t.Fatal("Init rejected valid compact Mode 0 assets")
	}
	c.LoadPalette(&palette)
	c.InitHardware()
	if got := hw.GetReg16(hw.DISPCNT); got != mode0ForceBlank {
		t.Fatalf("DISPCNT after init = %#04x, want forced blank", got)
	}
	if got := hw.GetReg16(hw.BG0CNT); got != mode0BG0Control {
		t.Fatalf("BG0CNT = %#04x, want %#04x", got, mode0BG0Control)
	}
	if got := byte(hw.ReadVRAM32(hw.VRAM + 64)); got != 1 {
		t.Fatalf("tile 1 first pixel = %d, want 1", got)
	}

	c.BeginRenderFrame()
	c.Cls(5)
	c.Map(0, 0, 0, 0, 0, 0, 0)
	c.PresentRender()
	if got, want := hw.GetReg16(hw.DISPCNT), mode0LayerDisplay(1); got != want {
		t.Fatalf("DISPCNT after present = %#04x, want %#04x", got, want)
	}
	if got := hw.GetReg16(mode0ScreenBase); got != 1 {
		t.Fatalf("screen entry 0 = %d, want tile 1", got)
	}
	if got := hw.GetReg16(mode0ScreenBase + 2); got != 0 {
		t.Fatalf("screen entry 1 = %d, want transparent tile 0", got)
	}
	if got := hw.ReadPaletteRAM16(0); got != palette[5] {
		t.Fatalf("backdrop palette = %#04x, want %#04x", got, palette[5])
	}
}

func TestMode0ContextDoesNotContainMode4Framebuffer(t *testing.T) {
	var c Context
	if c.framebuffer != nil {
		t.Fatal("Mode 0 compatibility framebuffer unexpectedly has backing storage")
	}
}

func newMode0TestContext(t *testing.T, sheetW, sheetH, mapW, mapH int, sheet []byte, mapData []uint16, flags []byte) *Context {
	t.Helper()
	spriteN := (sheetW / 8) * (sheetH / 8)
	mapLayers := 1
	if planeCells := mapW * mapH; planeCells > 0 && len(mapData)%planeCells == 0 {
		mapLayers = len(mapData) / planeCells
	}
	image := AssetImage{
		SheetW: sheetW, SheetH: sheetH, SpriteCols: sheetW / 8, SpriteRows: sheetH / 8, SpriteN: spriteN,
		MapW: mapW, MapH: mapH, MapLayers: mapLayers, Sheet: sheet, Map: mapData, Flags: flags,
	}
	ram := &AssetMemory{
		SheetW: sheetW, SheetH: sheetH, SpriteCols: sheetW / 8, SpriteRows: sheetH / 8, SpriteN: spriteN,
		MapW: mapW, MapH: mapH, MapLayers: mapLayers, Sheet: make([]byte, len(sheet)), Map: make([]uint16, len(mapData)), Flags: make([]byte, len(flags)),
	}
	var c Context
	if !c.Init(&image, ram) {
		t.Fatal("Init rejected valid mutable Mode 0 fixture")
	}
	c.InitHardware()
	return &c
}

func mode0VRAMEntry(x, y int) uint16 {
	return hw.GetReg16(mode0ScreenBase + uintptr(mode0ScreenEntry(x, y)*2))
}

func TestMode0CameraOffsetsWindowAndAvoidsSubtileMapUpload(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	mapData := make([]uint16, 80*40)
	mapData[5*80+10] = 1
	mapData[5*80+11] = 2
	mapData[6*80+10] = 2
	c := newMode0TestContext(t, 24, 8, 80, 40, make([]byte, 24*8), mapData, make([]byte, 3))

	c.BeginRenderFrame()
	c.Cls(0)
	c.Camera(7, 9)
	c.Map(10, 5, 3, -2, 2, 2, 0)
	c.PresentRender()
	if got := hw.GetReg16(hw.BG0HOFS); got != 4 {
		t.Fatalf("BG0HOFS = %d, want 4", got)
	}
	if got := hw.GetReg16(hw.BG0VOFS); got != 3 {
		t.Fatalf("BG0VOFS = %d, want 3", got)
	}
	if got := mode0VRAMEntry(10, 4); got != 1 {
		t.Fatalf("window entry (10,4) = %d, want 1", got)
	}
	if got := mode0VRAMEntry(11, 4); got != 2 {
		t.Fatalf("window entry (11,4) = %d, want 2", got)
	}

	// One-pixel camera movement changes only hardware scroll while the source
	// window remains in the same tile phase.
	c.BeginRenderFrame()
	c.Cls(0)
	c.Camera(8, 9)
	c.Map(10, 5, 3, -2, 2, 2, 0)
	c.PresentRender()
	if got := hw.GetReg16(hw.BG0HOFS); got != 5 {
		t.Fatalf("BG0HOFS after subtile pan = %d, want 5", got)
	}
	if stats := c.RendererUploadStats(); stats.LastMapBytes != 0 {
		t.Fatalf("subtile pan uploaded %d map bytes, want 0", stats.LastMapBytes)
	}

	// Crossing a tile boundary rematerializes the wrapped screen placement.
	c.BeginRenderFrame()
	c.Cls(0)
	c.Camera(15, 9)
	c.Map(10, 5, 3, -2, 2, 2, 0)
	c.PresentRender()
	if stats := c.RendererUploadStats(); stats.LastMapBytes != 4096 {
		t.Fatalf("tile-boundary pan uploaded %d map bytes, want 4096", stats.LastMapBytes)
	}
	if got := mode0VRAMEntry(9, 4); got != 1 {
		t.Fatalf("shifted window entry (9,4) = %d, want 1", got)
	}
}

func TestMode0SsetMsetAndFsetDirtyOnlyChangedHardware(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	sheet := make([]byte, 16*8)
	sheet[8] = 3
	mapData := []uint16{1}
	c := newMode0TestContext(t, 16, 8, 1, 1, sheet, mapData, []byte{0, 0})

	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 1, 1, 1)
	c.PresentRender()
	if got := mode0VRAMEntry(0, 0); got != 0 {
		t.Fatalf("flag-filtered entry = %d, want empty", got)
	}

	c.Fset(1, 0, true)
	c.Sset(8, 0, 9)
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 1, 1, 1)
	c.PresentRender()
	stats := c.RendererUploadStats()
	if stats.LastTileBytes != 64 || stats.LastMapBytes != 4096 {
		t.Fatalf("mutation uploads = %+v, want tile=64 map=4096", stats)
	}
	if got := byte(hw.ReadVRAM32(hw.VRAM + 64)); got != 9 {
		t.Fatalf("mutated tile pixel = %d, want 9", got)
	}
	if got := mode0VRAMEntry(0, 0); got != 1 {
		t.Fatalf("entry after fset = %d, want 1", got)
	}

	// Repeating the same values must not consume upload bandwidth.
	c.Fset(1, 0, true)
	c.Sset(8, 0, 9)
	c.Mset(0, 0, 1)
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 1, 1, 1)
	c.PresentRender()
	if stats := c.RendererUploadStats(); stats.LastTileBytes != 0 || stats.LastMapBytes != 0 {
		t.Fatalf("idempotent mutations uploaded data: %+v", stats)
	}

	c.Mset(0, 0, 0)
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 1, 1, 1)
	c.PresentRender()
	if got := mode0VRAMEntry(0, 0); got != 0 {
		t.Fatalf("entry after mset to sprite 0 = %d, want empty", got)
	}
	if stats := c.RendererUploadStats(); stats.LastMapBytes != 4096 {
		t.Fatalf("mset map upload = %+v, want 4096 bytes", stats)
	}
}

func TestMode0OffscreenMsetDoesNotInvalidateVisibleWindow(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	sheet := make([]byte, 16*8)
	c := newMode0TestContext(t, 16, 8, 2, 1, sheet, []uint16{1, 1}, []byte{0, 0})
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 1, 1, 0)
	c.PresentRender()

	c.Mset(1, 0, 0)
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 1, 1, 0)
	c.PresentRender()
	if stats := c.RendererUploadStats(); stats.LastMapBytes != 0 {
		t.Fatalf("offscreen mset uploaded visible map: %+v", stats)
	}
	if got := mode0VRAMEntry(0, 0); got != 1 {
		t.Fatalf("visible entry after offscreen mset = %d, want 1", got)
	}
}

func TestMode0FinalizeInitCommitsSheetMutationsBeforeFirstFrame(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{1}, []byte{0, 0})
	coldBefore := c.RendererUploadStats().ColdTileBytes
	c.Sset(8, 0, 9)
	c.FinalizeInitHardware()
	if got := byte(hw.ReadVRAM32(hw.VRAM + 64)); got != 9 {
		t.Fatalf("finalized tile pixel = %d, want 9", got)
	}
	if got := c.RendererUploadStats().ColdTileBytes; got != coldBefore+64 {
		t.Fatalf("cold tile bytes = %d, want %d", got, coldBefore+64)
	}
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 1, 1, 0)
	c.PresentRender()
	if got := c.RendererUploadStats().LastTileBytes; got != 0 {
		t.Fatalf("first visible frame re-uploaded %d tile bytes", got)
	}
}

func TestMode0StreamsLargeLogicalMapWindow(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	const mapW, mapH = 256, 128
	mapROM := make([]uint16, mapW*mapH)
	mapROM[70*mapW+200] = 1
	image := AssetImage{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		MapW: mapW, MapH: mapH, MapStreaming: true,
		Sheet: make([]byte, 16*8), Map: mapROM, Flags: make([]byte, 2),
	}
	ram := AssetMemory{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		MapW: mapW, MapH: mapH, MapStreaming: true,
		Sheet: make([]byte, 16*8), Map: make([]uint16, MapCacheCells), Flags: make([]byte, 2),
		MapChunks: make([]MapChunk, MapCacheSlots), MapEdits: make([]MapEdit, MapEditCapacity),
	}
	var c Context
	if !c.Init(&image, &ram) {
		t.Fatal("Init rejected valid streamed Mode 0 map")
	}
	c.InitHardware()
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(200, 70, 0, 0, 31, 21, 0)
	c.PresentRender()
	if got := mode0VRAMEntry(200, 70); got != 1 {
		t.Fatalf("streamed entry = %d, want 1", got)
	}

	c.Mset(200, 70, 0)
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(200, 70, 0, 0, 31, 21, 0)
	c.PresentRender()
	if got := mode0VRAMEntry(200, 70); got != 0 {
		t.Fatalf("streamed edited entry = %d, want 0", got)
	}
}

func TestMode0CombinesDrawAndDisplayPaletteRemaps(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{1}, make([]byte, 2))
	var colors [256]uint16
	colors[3] = 0x4567
	c.LoadPalette(&colors)
	c.Pal(1, 2, 0)
	c.Pal(2, 3, 1)
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 1, 1, 0)
	c.PresentRender()
	if got := hw.ReadPaletteRAM16(1); got != colors[3] {
		t.Fatalf("combined palette[1] = %#04x, want %#04x", got, colors[3])
	}
}

func TestMode0MapBGAtVDefaultsEveryOptionalArgumentAfterBG(t *testing.T) {
	requireMode0LayerCount(t, 1)
	values := [8]Value{
		IntVal(2), // BG2
		IntVal(1), // cell x
		IntVal(1), // cell y
		IntVal(3), // screen x
		IntVal(5), // screen y
		IntVal(2), // cells wide
		IntVal(2), // cells high
		IntVal(2), // flag mask; tile 1 has only flag 0 and is filtered
	}
	for provided := 1; provided <= len(values); provided++ {
		t.Run(fmt.Sprintf("arity-%d", provided), func(t *testing.T) {
			makeContext := func() *Context {
				image := AssetImage{
					SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
					MapW: 3, MapH: 3, MapLayers: 4, MapPlaneMask: 0x04,
					Sheet: make([]byte, 16*8), Map: []uint16{1, 1, 1, 1, 1, 1, 1, 1, 1}, Flags: []byte{0, 1},
				}
				ram := &AssetMemory{
					SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
					MapW: 3, MapH: 3, MapLayers: 4, MapPlaneMask: 0x04,
					Sheet: make([]byte, 16*8), Map: make([]uint16, 9), Flags: make([]byte, 2),
				}
				var c Context
				if !c.Init(&image, ram) {
					t.Fatal("Init rejected compact BG2 arity fixture")
				}
				c.InitHardware()
				c.BeginRenderFrame()
				c.Cls(0)
				return &c
			}

			args := [8]Value{}
			copy(args[:provided], values[:provided])
			got := makeContext()
			got.MapBGAtV(0, args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7])

			expected := [8]int{}
			for index := 0; index < provided; index++ {
				expected[index] = values[index].Int()
			}
			want := makeContext()
			want.MapBGAt(0, expected[0], expected[1], expected[2], expected[3], expected[4], expected[5], expected[6], expected[7])

			if got.mode0Screen[0] != want.mode0Screen[0] ||
				got.mode0MapCellX[0] != want.mode0MapCellX[0] || got.mode0MapCellY[0] != want.mode0MapCellY[0] ||
				got.mode0MapCellsW[0] != want.mode0MapCellsW[0] || got.mode0MapCellsH[0] != want.mode0MapCellsH[0] ||
				got.mode0MapTileX[0] != want.mode0MapTileX[0] || got.mode0MapTileY[0] != want.mode0MapTileY[0] ||
				got.mode0MapLayer[0] != want.mode0MapLayer[0] || got.mode0MapSource[0] != want.mode0MapSource[0] ||
				got.mode0ScrollX[0] != want.mode0ScrollX[0] || got.mode0ScrollY[0] != want.mode0ScrollY[0] {
				t.Fatalf("MapBGAtV arity %d did not match explicit defaults", provided)
			}
		})
	}
}
