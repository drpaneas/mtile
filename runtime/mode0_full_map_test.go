//go:build mtile_mode0

package runtime

import (
	"fmt"
	"github.com/drpaneas/mtile/runtime/hw"
	"testing"
)

type mode0FiniteMapCall struct{ bg, cx, cy, x, y, w, h, flags, camX, camY int }

func mode0FiniteMapContext(t *testing.T, mask uint8) *Context {
	requireMode0LayerCount(t, 4)
	hw.ResetForTest()
	return mode0MaterialFixture(t, mask, mapStorageStreaming)
}

// Independent finite-map painter: each authored cell has world position
// (cell*8 + map-screen-origin), matching Mtile's documented map semantics.
// No screenblock address, wrapping, cache or mutation helper is shared here.
func mode0FiniteMapOracle(c *Context, calls []mode0FiniteMapCall, backdrop byte) []byte {
	pixels := make([]byte, FramebufferSize)
	for i := range pixels {
		pixels[i] = backdrop
	}
	for _, call := range calls {
		if c.cart.MapPlaneMask&(1<<uint(call.bg)) == 0 {
			continue
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
				id := c.mapGetLayer(call.bg, mx, my)
				if id == 0 || int(id) >= c.cart.SpriteN || id >= mode0TileLimit {
					continue
				}
				if call.flags > 0 && call.flags <= 255 && c.cart.Flags[id]&byte(call.flags) != byte(call.flags) {
					continue
				}
				color := c.Sget(int(id)%c.cart.SpriteCols*8+(worldX&7), int(id)/c.cart.SpriteCols*8+(worldY&7))
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
func mode0FiniteMapHardwarePixel(x, y int, backdrop byte) byte {
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
func mode0FiniteDrawMaps(c *Context, calls []mode0FiniteMapCall, backdrop byte) {
	c.BeginRenderFrame()
	c.Cls(int(backdrop))
	for _, call := range calls {
		c.Camera(call.camX, call.camY)
		c.MapBG(call.bg, call.cx, call.cy, call.x, call.y, call.w, call.h, call.flags)
	}
}
func assertMode0FiniteMapsPresented(t *testing.T, c *Context, want []byte, backdrop byte) {
	t.Helper()
	c.PrepareRender()
	hw.SetReg16(hw.VCOUNT, 160)
	c.PresentRender()
	for y := 0; y < ScreenHeight; y++ {
		for x := 0; x < ScreenWidth; x++ {
			if got := mode0FiniteMapHardwarePixel(x, y, backdrop); got != want[y*ScreenWidth+x] {
				t.Fatalf("pixel(%d,%d)=%d want%d", x, y, got, want[y*ScreenWidth+x])
			}
		}
	}
}

func TestMode0FiniteMapCameraAndViewportFallbackPixelParity(t *testing.T) {
	c := mode0FiniteMapContext(t, 1)
	shapes := []mode0FiniteMapCall{
		{bg: 0, w: 60, h: 20},
		{bg: 0, w: 64, h: 32},
		{bg: 0, w: 48, h: 20},
		{bg: 0, cx: 3, cy: 2, x: 17, y: -9, w: 37, h: 23, flags: 3},
		{bg: 0, cx: -4, cy: -3, x: 5, y: 11, w: 42, h: 27},
		{bg: 0, cx: 60, cy: 35, x: -413, y: -229, w: 30, h: 20},
	}
	cameras := [][2]int{{0, 0}, {7, 7}, {8, 8}, {-1, -1}, {-32, -96}, {-33, -97}, {-128, -96}, {-129, -97}, {272, 96}, {273, 97}, {509, 257}, {0, 0}}
	for shape, call := range shapes {
		for frame, cam := range cameras {
			t.Run(fmt.Sprintf("shape%d/frame%d", shape, frame), func(t *testing.T) {
				call.camX, call.camY = cam[0], cam[1]
				calls := []mode0FiniteMapCall{call}
				mode0FiniteDrawMaps(c, calls, 42)
				assertMode0FiniteMapsPresented(t, c, mode0FiniteMapOracle(c, calls, 42), 42)
			})
		}
	}
}

func TestMode0FiniteFourBackgroundsSparsePlanesAndScrollOnlyReuse(t *testing.T) {
	for _, mask := range []uint8{15, 10} {
		c := mode0FiniteMapContext(t, mask)
		for frame := 0; frame < 32; frame++ {
			var calls []mode0FiniteMapCall
			for bg := 0; bg < 4; bg++ {
				if mask&(1<<uint(bg)) != 0 {
					calls = append(calls, mode0FiniteMapCall{bg: bg, cx: bg, cy: 1, x: bg * 3, y: -5, w: 48, h: 24, camX: frame * (bg + 1), camY: frame - bg, flags: bg % 2})
				}
			}
			mode0FiniteDrawMaps(c, calls, 11)
			assertMode0FiniteMapsPresented(t, c, mode0FiniteMapOracle(c, calls, 11), 11)
			if frame > 0 && c.RendererUploadStats().LastMapBytes != 0 {
				t.Fatalf("mask%04b frame%d: finite cache uploaded%d map bytes for scroll only", mask, frame, c.RendererUploadStats().LastMapBytes)
			}
		}
	}
}

func TestMode0FiniteMapUpdateAndDrawTimeMutationSnapshots(t *testing.T) {
	c := mode0FiniteMapContext(t, 15)
	call := mode0FiniteMapCall{bg: 2, w: 48, h: 20, flags: 2}
	calls := []mode0FiniteMapCall{call}
	mode0FiniteDrawMaps(c, calls, 0)
	assertMode0FiniteMapsPresented(t, c, mode0FiniteMapOracle(c, calls, 0), 0)
	if !c.mode0MapFullCache[call.bg] {
		t.Fatal("fixture did not use full finite-map cache")
	}
	// This cell is outside the first viewport but inside the cached rectangle.
	// An update-phase mutation must reach the full map before scrolling it in.
	c.MsetLayer(2, 45, 5, 4)
	c.FsetByte(4, 2)
	calls[0].camX = 128
	mode0FiniteDrawMaps(c, calls, 0)
	assertMode0FiniteMapsPresented(t, c, mode0FiniteMapOracle(c, calls, 0), 0)
	// Mutation after map() retains the pending frame's already-drawn snapshot.
	mode0FiniteDrawMaps(c, calls, 0)
	before := mode0FiniteMapOracle(c, calls, 0)
	c.MsetLayer(2, 20, 5, 7)
	c.FsetByte(4, 0)
	assertMode0FiniteMapsPresented(t, c, before, 0)
	if c.mode0MapSigSet[call.bg] {
		t.Fatal("draw-time mutation kept a stale reusable map signature")
	}
	mode0FiniteDrawMaps(c, calls, 0)
	assertMode0FiniteMapsPresented(t, c, mode0FiniteMapOracle(c, calls, 0), 0)
	// Flags updated between frames affect every occurrence, including currently
	// offscreen cells in the finite cache, without changing another source BG.
	c.FsetByte(7, 2)
	calls[0].camX = 0
	mode0FiniteDrawMaps(c, calls, 0)
	assertMode0FiniteMapsPresented(t, c, mode0FiniteMapOracle(c, calls, 0), 0)
}

func TestMode0FiniteDefaultMapExtentsAndMissingPlane(t *testing.T) {
	c := mode0FiniteMapContext(t, 10)
	for _, call := range []mode0FiniteMapCall{
		{bg: 1, camX: 17, camY: 9},
		{bg: 1, cx: 4, cy: 3, x: -41, y: -29, camX: 7, camY: 3},
		{bg: 2, cx: -2, cy: -2, x: 13, y: 7}, // Logical plane absent from compact storage.
	} {
		calls := []mode0FiniteMapCall{call}
		mode0FiniteDrawMaps(c, calls, 96)
		assertMode0FiniteMapsPresented(t, c, mode0FiniteMapOracle(c, calls, 96), 96)
	}
}

func TestMode0FiniteStreamedMapCacheRetainsOffscreenEdits(t *testing.T) {
	requireMode0LayerCount(t, 4)
	if mapStorageFixed && !mapStorageStreaming {
		t.Skip("build links resident maps")
	}
	hw.ResetForTest()
	c := mode0MaterialFixture(t, 15, true)
	calls := []mode0FiniteMapCall{{bg: 0, w: 48, h: 24, flags: 1}, {bg: 3, w: 60, h: 20, flags: 2}}
	mode0FiniteDrawMaps(c, calls, 9)
	assertMode0FiniteMapsPresented(t, c, mode0FiniteMapOracle(c, calls, 9), 9)
	c.MsetLayer(3, 45, 5, 4)
	c.FsetByte(4, 2)
	for _, camera := range [][2]int{{128, 0}, {272, 96}, {273, 97}, {-33, -97}, {0, 0}} {
		for i := range calls {
			calls[i].camX, calls[i].camY = camera[0], camera[1]
		}
		mode0FiniteDrawMaps(c, calls, 9)
		assertMode0FiniteMapsPresented(t, c, mode0FiniteMapOracle(c, calls, 9), 9)
	}
}

func TestMode0StreamedFullMapMisalignedChunkAndScreenBoundaries(t *testing.T) {
	requireMode0LayerCount(t, 4)
	if mapStorageFixed && !mapStorageStreaming {
		t.Skip("build links resident maps")
	}
	hw.ResetForTest()
	c := mode0MaterialFixture(t, 10, true)
	// These source origins put the 32-cell chunk boundary at a different
	// position from the destination screen-block boundary. The last chunk
	// is partial, and the requested rectangle extends beyond finite edges.
	for _, cx := range []int{-3, 1, 17, 31, 32} {
		for _, cy := range []int{-2, 1, 29, 31, 32} {
			for _, mask := range []int{0, 1, 3} {
				call := mode0FiniteMapCall{bg: 1, cx: cx, cy: cy, w: 48, h: 24,
					camX: cx * 8, camY: cy * 8, flags: mask}
				calls := []mode0FiniteMapCall{call}
				c.MsetLayer(1, 32, 32, 4)
				mode0FiniteDrawMaps(c, calls, 9)
				if !c.mode0MapFullCache[call.bg] {
					t.Fatal("fixture missed streamed full-map path")
				}
				assertMode0FiniteMapsPresented(t, c, mode0FiniteMapOracle(c, calls, 9), 9)
			}
		}
	}
}

func BenchmarkMode0FiniteMapScroll(b *testing.B) {
	if mode0LayerCount < 1 {
		b.Skip("requires a hardware BG")
	}
	for _, full := range []bool{false, true} {
		b.Run(fmt.Sprintf("full%t", full), func(b *testing.B) {
			const w, h = 60, 20
			image := AssetImage{SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
				MapW: w, MapH: h, Sheet: make([]byte, 128), Map: make([]uint16, w*h), Flags: make([]byte, 2)}
			for i := range image.Map {
				image.Map[i] = 1
			}
			ram := AssetMemory{SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
				MapW: w, MapH: h, Sheet: make([]byte, 128), Map: make([]uint16, w*h), Flags: make([]byte, 2)}
			c := new(Context)
			if !c.Init(&image, &ram) {
				b.Fatal("Init rejected benchmark map")
			}
			c.BeginRenderFrame()
			c.Cls(0)
			c.MapBGAt(0, 0, 0, 0, 0, 0, w, h, 0)
			var uploads int64
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				camera := i % 240
				cx, width := camera/8, 31
				if full {
					cx, width = 0, 60
				}
				c.mode0MapDirty[0] = false
				c.BeginRenderFrame()
				c.Cls(0)
				c.Camera(camera, 0)
				c.MapBGAt(0, 0, cx, 0, 0, 0, width, h, 0)
				if c.mode0MapDirty[0] {
					uploads += 4096
				}
			}
			b.ReportMetric(float64(uploads)/float64(b.N), "map-bytes/frame")
		})
	}
}
