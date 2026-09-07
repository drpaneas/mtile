//go:build mtile_mode0

package runtime

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

type mode0MaterialCall struct {
	bg, cellX, cellY, x, y, width, height, mask int
}

// Retain the old independent per-cell bounds and lookup algorithm as the
// semantic oracle for the hoisted materializer, including its guard cells.
func referenceMode0Materialize(c *Context, call mode0MaterialCall) [4096]byte {
	var result [4096]byte
	ox, oy := call.x+call.cellX*8-c.camX, call.y+call.cellY*8-c.camY
	w, h := call.width, call.height
	if w <= 0 {
		w = (c.clipX1 - c.clipX0 + 7) / 8
		if hidden := c.clipX0 - ox; hidden > 0 {
			w += (hidden + 7) / 8
		}
	}
	if h <= 0 {
		h = (c.clipY1 - c.clipY0 + 7) / 8
		if hidden := c.clipY0 - oy; hidden > 0 {
			h += (hidden + 7) / 8
		}
	}
	tx, ty := (ox+positiveMod(-ox, 8))/8, (oy+positiveMod(-oy, 8))/8
	var mask byte
	if call.mask > 0 && call.mask <= 255 {
		mask = byte(call.mask)
	}
	for my := 0; my < h; my++ {
		if sy := oy + my*8; sy+8 <= c.clipY0 || sy > c.clipY1 {
			continue
		}
		mapY := call.cellY + my
		if mapY < 0 || mapY >= c.cart.MapH {
			continue
		}
		for mx := 0; mx < w; mx++ {
			if sx := ox + mx*8; sx+8 <= c.clipX0 || sx > c.clipX1 {
				continue
			}
			mapX := call.cellX + mx
			if mapX < 0 || mapX >= c.cart.MapW {
				continue
			}
			id := c.mapGetLayer(call.bg, mapX, mapY)
			if id == 0 || int(id) >= c.cart.SpriteN || id >= mode0TileLimit || mask != 0 && c.cart.Flags[id]&mask != mask {
				continue
			}
			x, y := positiveMod(tx+mx, 64), positiveMod(ty+my, 32)
			offset := ((x/32)*1024 + y*32 + x%32) * 2
			result[offset], result[offset+1] = byte(id), byte(id>>8)
		}
	}
	return result
}

func mode0MaterialFixture(t *testing.T, mask uint8, streaming bool) *Context {
	t.Helper()
	const w, h, spriteN = 48, 40, 9
	sheet := make([]byte, spriteN*8*8)
	for y := 0; y < 8; y++ {
		for x := 8; x < spriteN*8; x++ {
			if (x+y)%5 != 0 {
				sheet[y*spriteN*8+x] = byte(x/8*16 + x%8 + y)
			}
		}
	}
	var cells []uint16
	for bg := 0; bg < 4; bg++ {
		if mask&(1<<uint(bg)) == 0 {
			continue
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				cells = append(cells, uint16((x+y*3+bg*2)%spriteN))
			}
		}
	}
	flags := make([]byte, spriteN)
	for id := range flags {
		flags[id] = byte(id % 4)
	}
	image := &AssetImage{SheetW: spriteN * 8, SheetH: 8, SpriteCols: spriteN, SpriteRows: 1, SpriteN: spriteN,
		MapW: w, MapH: h, MapLayers: 4, MapPlaneMask: mask, MapStreaming: streaming, Sheet: sheet, Map: cells, Flags: flags}
	ram := &AssetMemory{SheetW: spriteN * 8, SheetH: 8, SpriteCols: spriteN, SpriteRows: 1, SpriteN: spriteN,
		MapW: w, MapH: h, MapLayers: 4, MapPlaneMask: mask, MapStreaming: streaming,
		Sheet: make([]byte, len(sheet)), Map: make([]uint16, len(cells)), Flags: make([]byte, len(flags))}
	if streaming {
		ram.Map = make([]uint16, MapCacheCells)
		ram.MapChunks = make([]MapChunk, MapCacheSlots)
		ram.MapEdits = make([]MapEdit, MapEditCapacity)
	}
	c := new(Context)
	if !c.Init(image, ram) {
		t.Fatal("materialization fixture rejected")
	}
	c.InitHardware()
	return c
}

func assertMode0MaterialPixels(t *testing.T, c *Context, expected *[4][4096]byte) {
	t.Helper()
	for y := 0; y < 160; y++ {
		for x := 0; x < 240; x++ {
			var got, want byte
			for layer := 0; layer < 4; layer++ {
				sx, sy := x+int(c.mode0ScrollX[layer]), y+int(c.mode0ScrollY[layer])
				tx, ty := positiveMod(sx/8, 64), positiveMod(sy/8, 32)
				entry := (tx/32)*1024 + ty*32 + tx%32
				actualID := hw.GetReg16(mode0LayerScreenBase(layer) + uintptr(entry*2))
				// The independent old materializer anchors its shadow to the
				// current viewport and only uses the fine scroll phase.
				ex, ey := x+int(c.mode0ScrollX[layer]&7), y+int(c.mode0ScrollY[layer]&7)
				expectEntry := ((ex/8)/32)*1024 + (ey/8)*32 + (ex/8)%32
				expectID := uint16(expected[layer][expectEntry*2]) | uint16(expected[layer][expectEntry*2+1])<<8
				pixelOffset := (sy%8)*8 + sx%8
				word := hw.GetReg16(hw.VRAM + uintptr(int(actualID)*64+pixelOffset&^1))
				if pixel := byte(word >> uint((pixelOffset&1)*8)); actualID != 0 && pixel != 0 {
					got = pixel
				}
				if expectID != 0 {
					pixel := c.Sget(int(expectID)%c.cart.SpriteCols*8+sx%8, int(expectID)/c.cart.SpriteCols*8+sy%8)
					if pixel != 0 {
						want = pixel
					}
				}
			}
			if got != want {
				t.Fatalf("composed pixel(%d,%d)=%d, want %d", x, y, got, want)
			}
		}
	}
}

func TestMode0MaterializationMatchesOldLoopAcrossFourBackgrounds(t *testing.T) {
	requireMode0LayerCount(t, 4)
	for _, streaming := range []bool{false, true} {
		if mapStorageFixed && streaming != mapStorageStreaming {
			continue
		}
		for _, planes := range []uint8{0x0f, 0x0b} {
			t.Run(fmt.Sprintf("streaming%t/planes%x", streaming, planes), func(t *testing.T) {
				hw.ResetForTest()
				c := mode0MaterialFixture(t, planes, streaming)
				for trial, camera := range [][2]int{{0, 0}, {7, 7}, {8, 8}, {9, 9}, {-1, -1}, {-8, -8}, {-9, -9}, {15, 17}, {16, 16}, {239, 159}} {
					c.BeginRenderFrame()
					c.Cls(0)
					var expected [4][4096]byte
					for slot := 0; slot < 4; slot++ {
						c.Camera(camera[0]+slot, camera[1]-slot)
						c.ClipReset()
						if trial == 7 {
							c.Clip(3, 5, 229, 147)
							c.mode0MapSigSet[slot] = false
						}
						call := mode0MaterialCall{bg: (slot + trial/3) % 4, cellX: trial%3 - 1, cellY: trial%4 - 1,
							width: 64, height: 32, mask: trial % 4}
						if trial < 4 {
							call.width, call.height = 0, 0
						}
						c.MapBGAt(slot, call.bg, call.cellX, call.cellY, call.x, call.y, call.width, call.height, call.mask)
						expected[slot] = referenceMode0Materialize(c, call)
						if !c.mode0MapFullCache[slot] && !bytes.Equal(c.mode0Screen[slot][:], expected[slot][:]) {
							t.Fatalf("trial%d slot%d screen differs from per-cell oracle", trial, slot)
						}
					}
					c.PresentRender()
					assertMode0MaterialPixels(t, c, &expected)
				}
			})
		}
	}
}

func TestMode0MaterializationRetainsMutationSnapshots(t *testing.T) {
	requireMode0LayerCount(t, 4)
	for _, streaming := range []bool{false, true} {
		if mapStorageFixed && streaming != mapStorageStreaming {
			continue
		}
		c := mode0MaterialFixture(t, 15, streaming)
		draw := func() [4][4096]byte {
			c.BeginRenderFrame()
			c.Cls(0)
			var expected [4][4096]byte
			for bg := 0; bg < 4; bg++ {
				call := mode0MaterialCall{bg: bg, width: 31, height: 21, mask: 1}
				c.MapBGAt(bg, bg, 0, 0, 0, 0, 31, 21, 1)
				expected[bg] = referenceMode0Materialize(c, call)
				if !c.mode0MapFullCache[bg] && !bytes.Equal(c.mode0Screen[bg][:], expected[bg][:]) {
					t.Fatalf("streaming%t BG%d mutation/reference mismatch", streaming, bg)
				}
			}
			return expected
		}
		before := draw()
		shadowBefore := c.mode0Screen
		c.MsetLayer(2, 1, 1, 7)
		c.FsetByte(3, 0)
		for bg := 0; bg < 4; bg++ {
			if c.mode0Screen[bg] != shadowBefore[bg] {
				t.Fatal("mutation changed an already-drawn map snapshot")
			}
		}
		c.PresentRender()
		after := draw()
		if before == after {
			t.Fatal("map/flag mutation was not visible on next frame")
		}
		c.PresentRender()
		c.MsetLayer(1, 2, 2, 5)
		c.FsetByte(3, 3)
		after = draw()
		c.PresentRender()
		assertMode0MaterialPixels(t, c, &after)
	}
}

func TestMode0ScreenEntryPowerOfTwoWrap(t *testing.T) {
	for y := -1025; y <= 1025; y++ {
		for x := -129; x <= 129; x++ {
			px, py := positiveMod(x, 64), positiveMod(y, 32)
			if got, want := mode0ScreenEntry(x, y), (px/32)*1024+py*32+px%32; got != want {
				t.Fatalf("entry(%d,%d)=%d want%d", x, y, got, want)
			}
		}
	}
}
