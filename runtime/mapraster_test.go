//go:build !mtile_mode0 && !mtile_no_map_raster

package runtime

import (
	"math/rand"
	"testing"
	"unsafe"
)

func rasterFixture(t testing.TB, cache bool, checker bool, bg int) (*Context, *AssetImage, *AssetMemory) {
	t.Helper()
	sheet := make([]byte, 128)
	for i := 0; i < 64; i++ {
		sheet[i] = byte(1 + i%15)
		if i%8 < 5 {
			sheet[64+i] = byte(3 + i%11)
		}
	}
	if checker {
		for i := 0; i < 64; i++ {
			if (i+i/8)%2 == 0 {
				sheet[i] = 0
			}
		}
	}
	cells := make([]uint16, 600)
	for i := range cells {
		cells[i] = 1
		if !checker && i%7 == 0 {
			cells[i] = 2
		}
		if !checker && i%11 == 0 {
			cells[i] = 0
		}
		if !checker && i%29 == 0 {
			cells[i] = 3
		}
	}
	image := &AssetImage{SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1, SpriteN: 4, MapW: 30, MapH: 20, MapLayers: bg + 1, MapPlaneMask: 1 << bg, SheetReadOnly: true, Sheet: sheet, SheetIndex: []uint16{0, 1, 2, 0}, Map: cells, Flags: []byte{0, 1, 3, 2}}
	memory := &AssetMemory{SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1, SpriteN: 4, MapW: 30, MapH: 20, MapLayers: bg + 1, MapPlaneMask: 1 << bg, SheetReadOnly: true, Sheet: make([]byte, 128), SheetIndex: make([]uint16, 4), Map: make([]uint16, 600), Flags: make([]byte, 4)}
	if cache {
		memory.MapRasterPixels = make([]byte, FramebufferSize)
		memory.MapRasterSpans = make([]MapRasterSpan, MapRasterMaxSpans)
	}
	c := new(Context)
	if !c.Init(image, memory) {
		t.Fatal("Init")
	}
	return c, image, memory
}

func TestMapRasterMatchesCellPath(t *testing.T) {
	for _, bg := range []int{0, 1, 3} {
		cached, _, _ := rasterFixture(t, true, false, bg)
		plain, _, _ := rasterFixture(t, false, false, bg)
		rng := rand.New(rand.NewSource(718))
		for trial := 0; trial < 300; trial++ {
			cellX, cellY := rng.Intn(38)-4, rng.Intn(28)-4
			cellsW, cellsH := rng.Intn(40), rng.Intn(30)
			screenX, screenY := rng.Intn(17)-8, rng.Intn(17)-8
			camX, camY := rng.Intn(41)-20, rng.Intn(41)-20
			cx, cy, cw, ch := rng.Intn(30), rng.Intn(20), rng.Intn(220)+20, rng.Intn(150)+10
			mask := 0
			if trial == 0 {
				cellX = 0
				cellY = 0
				cellsW = 0
				cellsH = 0
				screenX = 0
				screenY = 0
				camX = 0
				camY = 0
				cx = 0
				cy = 0
				cw = 240
				ch = 160
			}
			for _, c := range []*Context{cached, plain} {
				c.CameraReset()
				c.ClipReset()
				c.PalReset()
				c.PaltReset()
				c.Cls(201)
				// Nonuniform existing pixels detect any overwrite of transparent holes.
				c.Line(0, 0, 239, 159, 212)
				c.Rectfill(60, 30, 100, 60, 42)
				c.Camera(camX, camY)
				c.Clip(cx, cy, cw, ch)
				switch trial % 13 {
				case 1:
					c.Pal(7, 0, 0)
				case 2:
					c.Pal(0, 9, 0)
					c.PaltColor(0, false)
				case 3:
					c.PaltColor(3, true)
				case 4:
					mask = 1
					c.FsetByte(2, 0)
				case 5:
					mask = 256 // Existing semantics: out-of-byte masks draw all cells.
				}
				c.MapBG(bg, cellX, cellY, screenX, screenY, cellsW, cellsH, mask)
			}
			assertFramesMatch(t, cached, plain, "cached map with camera/clip/palette/mask")
			if trial == 0 && !cached.mapRasterReady {
				t.Fatal("fixture failed to exercise cache")
			}
		}
	}
}

func TestMapRasterMutationReinitAndOverflow(t *testing.T) {
	cached, image, ram := rasterFixture(t, true, false, 0)
	plain, _, _ := rasterFixture(t, false, false, 0)
	draw := func() {
		for _, c := range []*Context{cached, plain} {
			c.Cls(200)
			c.Map(0, 0, 0, 0, 0, 0, 0)
		}
		assertFramesMatch(t, cached, plain, "cache mutation")
	}
	draw()
	if !cached.mapRasterReady {
		t.Fatal("cold cache not built")
	}
	cached.PalReset()
	cached.PaltReset()
	if !cached.mapRasterReady {
		t.Fatal("palette reset unnecessarily invalidated source raster")
	}
	for _, c := range []*Context{cached, plain} {
		c.Mset(1, 1, 2)
	}
	if !cached.mapRasterReady {
		t.Fatal("Mset discarded the raster for a one-cell edit")
	}
	draw()
	cached.Mset(1, 1, 2)
	if !cached.mapRasterReady {
		t.Fatal("no-op Mset invalidated cache")
	}
	for _, c := range []*Context{cached, plain} {
		c.MsetLayer(0, 1, 1, 0)
	}
	draw()
	image.Map[0] = 1
	if !cached.Init(image, ram) {
		t.Fatal("reinit")
	}
	if cached.mapRasterReady {
		t.Fatal("reinit retained old raster")
	}

	crowded, _, _ := rasterFixture(t, true, true, 0)
	fallback, _, _ := rasterFixture(t, false, true, 0)
	for _, c := range []*Context{crowded, fallback} {
		c.Cls(33)
		c.Map(0, 0, 0, 0, 0, 0, 0)
	}
	if !crowded.mapRasterFailed || crowded.mapRasterReady {
		t.Fatal("fragmented map did not fail closed")
	}
	assertFramesMatch(t, crowded, fallback, "span overflow fallback")
	// After a real edit, failure state is cleared so a newly simpler map can
	// become cacheable; no repeated cold rebuild while an unchanged map fails.
	crowded.Mset(0, 0, 0)
	if crowded.mapRasterFailed {
		t.Fatal("edit did not reset failed-cache state")
	}
}

func TestMapRasterCellEditsPreserveOtherRowsAndFallBackOnOverflow(t *testing.T) {
	cached, _, ram := rasterFixture(t, true, false, 3)
	plain, _, _ := rasterFixture(t, false, false, 3)
	cached.PrepareMapRaster()
	// Cell (0,0) starts empty. An opaque five-pixel run in each row needs
	// eight additional spans, which this deliberately full buffer cannot fit.
	ram.MapRasterSpans = ram.MapRasterSpans[:cached.mapRasterSpanCount]
	edit := func(x, y, tile int) {
		t.Helper()
		for _, c := range []*Context{cached, plain} {
			c.MsetLayer(3, x, y, tile)
			c.Cls(200)
			c.MapBG(3, 0, 0, 0, 0, 0, 0, 0)
		}
		assertFramesMatch(t, cached, plain, "cell edit on sparse BG3")
	}
	edit(0, 0, 2)
	if !cached.mapRasterFailed || cached.mapRasterReady {
		t.Fatal("span overflow should use live cells")
	}
	edit(0, 0, 0)
	if !cached.mapRasterReady {
		t.Fatal("removing the extra runs should recover the raster")
	}
	ram.MapRasterSpans = ram.MapRasterSpans[:cap(ram.MapRasterSpans)]
	for _, cell := range [][2]int{{0, 0}, {15, 9}, {29, 19}} {
		for _, tile := range []int{1, 2, 3, 0} {
			edit(cell[0], cell[1], tile)
			if !cached.mapRasterReady {
				t.Fatal("in-bounds tile edit discarded the raster")
			}
		}
	}
}

func TestMapRasterRejectsMissingAndUndersizedStorage(t *testing.T) {
	c, _, ram := rasterFixture(t, true, false, 0)
	ram.MapRasterPixels = ram.MapRasterPixels[:100]
	c.Map(0, 0, 0, 0, 0, 0, 0)
	if c.mapRasterReady {
		t.Fatal("undersized cache accepted")
	}
	ram.MapRasterPixels = make([]byte, FramebufferSize)
	ram.MapRasterSpans = nil
	c.Map(0, 0, 0, 0, 0, 0, 0)
	if c.mapRasterReady {
		t.Fatal("absent span storage accepted")
	}
}

func TestMapRasterPrewarmUsesInitEditsWithoutDrawing(t *testing.T) {
	c, _, _ := rasterFixture(t, true, false, 1)
	c.Cls(199)
	c.Camera(3, 2)
	c.Clip(4, 5, 70, 60)
	c.Pal(3, 8, 0)
	c.MsetLayer(1, 1, 1, 0)
	c.PrepareMapRaster()
	if !c.mapRasterReady || c.mapRasterBG != 1 {
		t.Fatal("startup did not prepare the logical populated plane")
	}
	for _, pixel := range c.fbForTest() {
		if pixel != 199 {
			t.Fatal("startup preparation modified the display")
		}
	}
	if c.camX != 3 || c.camY != 2 || c.clipX0 != 4 || c.clipY0 != 5 || c.drawPal[3] != 8 {
		t.Fatal("startup preparation modified draw state")
	}
	for y := 8; y < 16; y++ {
		for x := 8; x < 16; x++ {
			if c.cart.MapRasterPixels[y*240+x] != 0 {
				t.Fatal("startup cache ignored the game's Init map edit")
			}
		}
	}
}

func TestMapRasterNarrowMapsAndUnalignedStorage(t *testing.T) {
	for _, width := range []int{1, 3, 17, 29, 30} {
		cached, image, ram := rasterFixture(t, true, false, 0)
		plain, plainImage, plainRAM := rasterFixture(t, false, false, 0)
		for i, c := range []*Context{cached, plain} {
			im, mem := image, ram
			if i == 1 {
				im, mem = plainImage, plainRAM
			}
			im.MapW, mem.MapW = width, width
			im.Map, mem.Map = im.Map[:width*20], mem.Map[:width*20]
			if i == 0 {
				mem.MapRasterPixels = make([]byte, width*8*160)
			}
			if !c.Init(im, mem) {
				t.Fatal("narrow Init")
			}
		}
		for _, camX := range []int{-3, -2, -1, 0, 1, 2, 3} {
			for _, c := range []*Context{cached, plain} {
				c.Cls(201)
				c.Camera(camX, 1)
				c.Map(0, 0, 0, 0, 0, 0, 0)
			}
			assertFramesMatch(t, cached, plain, "narrow shifted raster")
		}
		buffer := make([]byte, width*8*160+4)
		offset := 0
		for uintptr(unsafe.Pointer(&buffer[offset]))&3 == 0 {
			offset++
		}
		ram.MapRasterPixels = buffer[offset : offset+width*8*160]
		cached.invalidateMapRaster()
		for _, c := range []*Context{cached, plain} {
			c.Cls(201)
			c.Camera(0, 0)
			c.Map(0, 0, 0, 0, 0, 0, 0)
		}
		assertFramesMatch(t, cached, plain, "unaligned raster fallback")
	}
}

func BenchmarkMapRaster(b *testing.B) {
	for _, cache := range []bool{false, true} {
		name := "cells"
		if cache {
			name = "raster"
		}
		b.Run(name, func(b *testing.B) {
			c, _, _ := rasterFixture(b, cache, false, 0)
			c.Map(0, 0, 0, 0, 0, 0, 0)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.Map(0, 0, 0, 0, 0, 0, 0)
			}
		})
	}
}
