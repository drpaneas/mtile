//go:build !mtile_mode0 && !mtile_tilecanvas

package runtime

import (
	"math/rand"
	"testing"
	"unsafe"
)

func solidMapFixture(t testing.TB, metadata, streaming bool) *Context {
	t.Helper()
	sheet := make([]byte, 6*64)
	for i := 0; i < 64; i++ {
		sheet[i] = 7
		sheet[2*64+i] = byte(i % 11)
		sheet[3*64+i] = 7
		sheet[4*64+i] = 255
		sheet[5*64+i] = 214
	}
	cells := make([]uint16, 64*24*4)
	for bg := 0; bg < 4; bg++ {
		for y := 0; y < 24; y++ {
			for x := 0; x < 64; x++ {
				cells[(bg*24+y)*64+x] = uint16((x/9 + y + bg) % 8)
			}
		}
	}
	image := &AssetImage{
		SheetW: 64, SheetH: 8, SpriteCols: 8, SpriteRows: 1, SpriteN: 8,
		MapW: 64, MapH: 24, MapLayers: 4, MapPlaneMask: 15, MapStreaming: streaming,
		SheetReadOnly: true, Sheet: sheet, SheetIndex: []uint16{0, 1, 2, 3, 0, 4, 5, 6},
		Map: cells, Flags: []byte{0, 1, 2, 3, 1, 2, 3, 1},
	}
	memory := &AssetMemory{
		SheetW: 64, SheetH: 8, SpriteCols: 8, SpriteRows: 1, SpriteN: 8,
		MapW: 64, MapH: 24, MapLayers: 4, MapPlaneMask: 15, MapStreaming: streaming,
		SheetReadOnly: true, Sheet: make([]byte, len(sheet)), SheetIndex: make([]uint16, 8),
		Map: make([]uint16, len(cells)), Flags: make([]byte, 8),
	}
	if metadata {
		// Sprite7 is solid but deliberately unannotated, and sprite4 is
		// implicit zero. Both still exercise the ordinary fallback.
		memory.SheetSolid = []uint16{8, 1, 0, 8, 256, 0}
	}
	if streaming {
		memory.Map = make([]uint16, MapCacheCells)
		memory.MapChunks = make([]MapChunk, MapCacheSlots)
		memory.MapEdits = make([]MapEdit, MapEditCapacity)
	}
	c := new(Context)
	if !c.Init(image, memory) {
		t.Fatal("Init")
	}
	return c
}

func TestCompactMapFastPathsMatchSprites(t *testing.T) {
	for sourceAlignment := 0; sourceAlignment < 4; sourceAlignment++ {
		for cameraX := 0; cameraX < 8; cameraX++ {
			fast := solidMapFixture(t, true, false)
			plain := solidMapFixture(t, false, false)
			for _, c := range []*Context{fast, plain} {
				// Compact byte slices are not required to be word aligned.
				storage := make([]byte, len(c.cart.Sheet)+7)
				offset := int((-uintptr(unsafe.Pointer(&storage[0])))&3) + sourceAlignment
				sheet := storage[offset : offset+len(c.cart.Sheet)]
				copy(sheet, c.cart.Sheet)
				c.cart.Sheet = sheet
			}
			for state := 0; state < 4; state++ {
				for _, c := range []*Context{fast, plain} {
					c.CameraReset()
					c.ClipReset()
					c.PalReset()
					c.PaltReset()
					c.Cls(201)
					c.Line(0, 159, 239, 0, 43)
					c.Camera(cameraX, cameraX%3)
					if state == 1 {
						c.Clip(5, 3, 229, 151)
					}
					if state == 2 {
						c.Pal(7, 0, 0)
					}
					if state == 3 {
						c.PaltColor(0, false)
						c.Pal(0, 99, 0)
						c.PaltColor(255, true)
					}
				}
				for bg := 0; bg < 4; bg++ {
					fast.MapBGAt(bg, bg, 0, 0, 0, 0, 30, 20, 0)
					for y := 0; y < 20; y++ {
						for x := 0; x < 30; x++ {
							id := plain.cart.Map[(bg*24+y)*64+x]
							if id != 0 {
								plain.Spr(int(id), x*8, y*8, 1, 1, false, false)
							}
						}
					}
				}
				assertFramesMatch(t, fast, plain, "compact map fast paths versus independent sprite renderer")
			}
		}
	}
}

func TestSolidMapRunsMatchCellRenderer(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		fast := solidMapFixture(t, true, streaming)
		plain := solidMapFixture(t, false, streaming)
		rng := rand.New(rand.NewSource(9163))
		for trial := 0; trial < 500; trial++ {
			cellX, cellY := rng.Intn(14)-6, rng.Intn(10)-4
			cellsW, cellsH := rng.Intn(75)-2, rng.Intn(30)-2
			sx, sy := rng.Intn(41)-20, rng.Intn(31)-15
			camX, camY := rng.Intn(380)-20, rng.Intn(90)-20
			cx, cy, cw, ch := rng.Intn(25), rng.Intn(20), rng.Intn(240), rng.Intn(160)
			mask := []int{0, 1, 2, 3, -1, 256}[trial%6]
			if trial < 16 {
				cellX, cellY, cellsW, cellsH, sx, sy = 0, 0, 64, 24, 0, 0
				camX, camY = trial&7, trial/8
				cx, cy, cw, ch = 0, 0, 240, 160
			}
			for _, c := range []*Context{fast, plain} {
				c.CameraReset()
				c.ClipReset()
				c.PalReset()
				c.PaltReset()
				for i := range c.framebuffer {
					c.framebuffer[i] = byte(i*17 + i/240 + trial)
				}
				switch trial % 8 {
				case 1:
					c.Pal(7, 0, 0) // Opaque source stays opaque after mapping to0.
				case 2:
					c.PaltColor(0, false)
					c.Pal(0, 99, 0)
				case 3:
					c.PaltColor(7, true)
				case 4:
					c.PaltColor(255, true)
					c.Pal(7, 255, 0) // Transparency tests source, not destination.
				case 5:
					c.PaltColor(0, false)
				case 6:
					c.Pal(255, 4, 0)
				}
				c.Camera(camX, camY)
				c.Clip(cx, cy, cw, ch)
				c.cursor = drawCursor{x: 91, y: 47, color: 123}
				cursor := c.cursor
				for bg := 0; bg < 4; bg++ {
					c.MapBGAt(bg, bg, cellX, cellY, sx, sy, cellsW, cellsH, mask)
				}
				if c.cursor != cursor {
					t.Fatalf("trial%d streaming%v: map changed draw cursor", trial, streaming)
				}
			}
			assertFramesMatch(t, fast, plain, "solid map runs with palette/clip/layers")
			// Edits split/extend uniform runs, including at a streaming chunk
			// boundary. Subsequent frames must see each edited live cell.
			if trial%17 == 0 {
				x, y, bg, id := 30+trial%5, trial%24, trial%4, trial%8
				for _, c := range []*Context{fast, plain} {
					c.MsetLayer(bg, x, y, id)
					c.FsetByte(1, trial&3)
				}
			}
		}
	}
}
