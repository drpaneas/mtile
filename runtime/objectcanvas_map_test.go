//go:build mtile_tilecanvas && !mtile_mode0

package runtime

import (
	"fmt"
	"github.com/drpaneas/mtile/runtime/hw"
	"testing"
)

type objectCanvasMapCall struct{ bg, cx, cy, x, y, w, h, flags, camX, camY int }

func objectCanvasMapContext(mask uint8) *Context {
	c := objectCanvasDrawContext(true)
	c.cart.MapW, c.cart.MapH, c.cart.MapLayers, c.cart.MapPlaneMask = 80, 45, 4, mask
	planes := 0
	for bg := 0; bg < 4; bg++ {
		if mask&(1<<uint(bg)) != 0 {
			planes++
		}
	}
	c.cart.Map = make([]uint16, planes*80*45)
	c.cart.Flags = make([]byte, c.cart.SpriteN)
	for id := range c.cart.Flags {
		c.cart.Flags[id] = byte(1 | 1<<uint(id%3))
	}
	storage := 0
	for bg := 0; bg < 4; bg++ {
		if mask&(1<<uint(bg)) != 0 {
			for y := 0; y < 45; y++ {
				for x := 0; x < 80; x++ {
					if (x+y+bg)%9 != 0 {
						c.cart.Map[(storage*45+y)*80+x] = uint16(1 + (x*3+y*5+bg)%7)
					}
				}
			}
			storage++
		}
	}
	c.InitHardware()
	return c
}

// Independent finite-map painter: each authored cell has world position
// (cell*8 + map-screen-origin), matching Mtile's documented map semantics.
// No screenblock address, wrapping, cache or mutation helper is shared here.
func objectCanvasMapOracle(c *Context, calls []objectCanvasMapCall, backdrop byte) []byte {
	pixels := make([]byte, FramebufferSize)
	for i := range pixels {
		pixels[i] = backdrop
	}
	for _, call := range calls {
		if c.cart.MapPlaneMask&(1<<uint(call.bg)) == 0 {
			continue
		}
		storage := 0
		for bg := 0; bg < call.bg; bg++ {
			if c.cart.MapPlaneMask&(1<<uint(bg)) != 0 {
				storage++
			}
		}
		w, h := call.w, call.h
		if w <= 0 {
			w = 30
			hidden := call.camX - call.x - call.cx*8
			if hidden > 0 {
				w += (hidden + 7) / 8
			}
		}
		if h <= 0 {
			h = 20
			hidden := call.camY - call.y - call.cy*8
			if hidden > 0 {
				h += (hidden + 7) / 8
			}
		}
		for y := 0; y < ScreenHeight; y++ {
			for x := 0; x < ScreenWidth; x++ {
				worldX, worldY := x+call.camX-call.x, y+call.camY-call.y
				mx, my := worldX>>3, worldY>>3
				if mx < call.cx || my < call.cy || mx >= call.cx+w || my >= call.cy+h || mx < 0 || my < 0 || mx >= c.cart.MapW || my >= c.cart.MapH {
					continue
				}
				id := c.cart.Map[(storage*c.cart.MapH+my)*c.cart.MapW+mx]
				if id == 0 || int(id) >= c.cart.SpriteN {
					continue
				}
				if call.flags > 0 && call.flags <= 255 && c.cart.Flags[id]&byte(call.flags) != byte(call.flags) {
					continue
				}
				slot := c.cart.SheetIndex[id]
				if slot == 0 {
					continue
				}
				color := c.cart.Sheet[(int(slot)-1)*64+(worldY&7)*8+(worldX&7)]
				if color != 0 {
					pixels[y*ScreenWidth+x] = color
				}
			}
		}
	}
	return pixels
}

// Decode actual 64x32 8bpp GBA maps, including the second screenblock, active
// BG enable bits, priority ties and source transparency. Compare indices to
// avoid accidental palette aliases concealing a coordinate/selection error.
func objectCanvasMapHardwarePixel(x, y int, backdrop byte) byte {
	enabled := hw.GetReg16(hw.DISPCNT)
	for priority := 0; priority < 4; priority++ {
		for bg := 0; bg < 4; bg++ {
			if enabled&(1<<uint(bg+8)) == 0 {
				continue
			}
			control := hw.GetReg16(hw.BG0CNT + uintptr(bg*2))
			if int(control&3) != priority {
				continue
			}
			sx := (x + int(hw.GetReg16(hw.BG0HOFS+uintptr(bg*4)))) & 511
			sy := (y + int(hw.GetReg16(hw.BG0VOFS+uintptr(bg*4)))) & 255
			block := int(control >> 8 & 31)
			if sx >= 256 {
				block++
			}
			address := hw.VRAM + uintptr(block*2048+((sy/8)*32+(sx/8)%32)*2)
			entry := hw.GetReg16(address)
			px, py := sx&7, sy&7
			if entry&(1<<10) != 0 {
				px = 7 - px
			}
			if entry&(1<<11) != 0 {
				py = 7 - py
			}
			offset := int(entry&1023)*64 + py*8 + px
			base := hw.VRAM + uintptr((control>>2&3)*16384)
			word := hw.GetReg16(base + uintptr(offset&^1))
			color := byte(word >> uint((offset&1)*8))
			if color != 0 {
				return color
			}
		}
	}
	return backdrop
}
func objectCanvasDrawMaps(c *Context, calls []objectCanvasMapCall, backdrop byte) {
	c.BeginRenderFrame()
	c.Cls(int(backdrop))
	for _, call := range calls {
		c.Camera(call.camX, call.camY)
		c.MapBG(call.bg, call.cx, call.cy, call.x, call.y, call.w, call.h, call.flags)
	}
}
func assertObjectCanvasMapsPresented(t *testing.T, c *Context, want []byte, backdrop byte) {
	t.Helper()
	c.PrepareRender()
	hw.SetReg16(hw.VCOUNT, 160)
	c.PresentRender()
	for y := 0; y < ScreenHeight; y++ {
		for x := 0; x < ScreenWidth; x++ {
			if got := objectCanvasMapHardwarePixel(x, y, backdrop); got != want[y*ScreenWidth+x] {
				t.Fatalf("pixel(%d,%d)=%d want%d", x, y, got, want[y*ScreenWidth+x])
			}
		}
	}
}

func TestObjectCanvasFiniteMapCameraAndViewportFallbackPixelParity(t *testing.T) {
	c := objectCanvasMapContext(1)
	shapes := []objectCanvasMapCall{
		{bg: 0, w: 48, h: 20},
		{bg: 0, cx: 3, cy: 2, x: 17, y: -9, w: 37, h: 23, flags: 3},
		{bg: 0, cx: -4, cy: -3, x: 5, y: 11, w: 42, h: 27},
		{bg: 0, cx: 60, cy: 35, x: -413, y: -229, w: 30, h: 20},
	}
	cameras := [][2]int{{0, 0}, {7, 7}, {8, 8}, {-1, -1}, {-128, -96}, {-129, -97}, {272, 96}, {273, 97}, {509, 257}, {0, 0}}
	for shape, call := range shapes {
		for frame, cam := range cameras {
			t.Run(fmt.Sprintf("shape%d/frame%d", shape, frame), func(t *testing.T) {
				call.camX, call.camY = cam[0], cam[1]
				calls := []objectCanvasMapCall{call}
				objectCanvasDrawMaps(c, calls, 42)
				assertObjectCanvasMapsPresented(t, c, objectCanvasMapOracle(c, calls, 42), 42)
			})
		}
	}
}

func TestObjectCanvasFourBackgroundsSparsePlanesAndScrollOnlyReuse(t *testing.T) {
	for _, mask := range []uint8{15, 10} {
		c := objectCanvasMapContext(mask)
		for frame := 0; frame < 4; frame++ {
			var calls []objectCanvasMapCall
			for bg := 0; bg < 4; bg++ {
				if mask&(1<<uint(bg)) != 0 {
					calls = append(calls, objectCanvasMapCall{bg: bg, cx: bg, cy: 1, x: bg * 3, y: -5, w: 48, h: 24, camX: frame * (bg + 1) * 3, camY: frame - bg, flags: bg % 2})
				}
			}
			objectCanvasDrawMaps(c, calls, 11)
			assertObjectCanvasMapsPresented(t, c, objectCanvasMapOracle(c, calls, 11), 11)
			if frame > 0 && c.RendererUploadStats().LastMapBytes != 0 {
				t.Fatalf("mask%04b frame%d: finite cache uploaded%d map bytes for scroll only", mask, frame, c.RendererUploadStats().LastMapBytes)
			}
		}
	}
}

func TestObjectCanvasFiniteMapUpdateAndDrawTimeMutationSnapshots(t *testing.T) {
	c := objectCanvasMapContext(15)
	call := objectCanvasMapCall{bg: 2, w: 48, h: 20, flags: 2}
	calls := []objectCanvasMapCall{call}
	objectCanvasDrawMaps(c, calls, 0)
	assertObjectCanvasMapsPresented(t, c, objectCanvasMapOracle(c, calls, 0), 0)
	if !c.canvasFullMap(2) {
		t.Fatal("fixture did not use full finite-map cache")
	}
	// This cell is outside the first viewport but inside the cached rectangle.
	// An update-phase mutation must reach the full map before scrolling it in.
	c.MsetLayer(2, 45, 5, 4)
	c.FsetByte(4, 2)
	calls[0].camX = 128
	objectCanvasDrawMaps(c, calls, 0)
	assertObjectCanvasMapsPresented(t, c, objectCanvasMapOracle(c, calls, 0), 0)
	// Mutation after map() retains the pending frame's already-drawn snapshot.
	objectCanvasDrawMaps(c, calls, 0)
	before := objectCanvasMapOracle(c, calls, 0)
	c.MsetLayer(2, 20, 5, 7)
	c.FsetByte(4, 0)
	assertObjectCanvasMapsPresented(t, c, before, 0)
	if c.mode0MapSigSet[2] {
		t.Fatal("draw-time mutation kept a stale reusable map signature")
	}
	objectCanvasDrawMaps(c, calls, 0)
	assertObjectCanvasMapsPresented(t, c, objectCanvasMapOracle(c, calls, 0), 0)
	// Flags updated between frames affect every occurrence, including currently
	// offscreen cells in the finite cache, without changing another source BG.
	c.FsetByte(7, 2)
	calls[0].camX = 0
	objectCanvasDrawMaps(c, calls, 0)
	assertObjectCanvasMapsPresented(t, c, objectCanvasMapOracle(c, calls, 0), 0)
}

func TestObjectCanvasDefaultMapExtentsAndMissingPlane(t *testing.T) {
	c := objectCanvasMapContext(10)
	for _, call := range []objectCanvasMapCall{
		{bg: 1, camX: 17, camY: 9},
		{bg: 1, cx: 4, cy: 3, x: -41, y: -29, camX: 7, camY: 3},
		{bg: 2, cx: -2, cy: -2, x: 13, y: 7}, // Logical plane absent from compact storage.
	} {
		calls := []objectCanvasMapCall{call}
		objectCanvasDrawMaps(c, calls, 96)
		assertObjectCanvasMapsPresented(t, c, objectCanvasMapOracle(c, calls, 96), 96)
	}
}
