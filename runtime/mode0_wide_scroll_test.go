//go:build mtile_mode0

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

// A map() call without a size covers the clip. Scrolling far into a wide map
// must keep drawing the visible cells, not stop once the hidden cells to the
// left would exceed the 64-cell hardware window.
func TestMode0DefaultExtentMapScrollsAcrossWideMap(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	const mapW, mapH = 256, 20
	mapData := make([]uint16, mapW*mapH)
	for x := 0; x < mapW; x++ {
		mapData[(mapH-1)*mapW+x] = uint16(1 + x%3)
	}
	sheet := make([]byte, 32*8)
	for i := range sheet {
		sheet[i] = 5
	}
	c := newMode0TestContext(t, 32, 8, mapW, mapH, sheet, mapData, make([]byte, 4))
	for cam := 0; cam <= mapW*8-240; cam += 3 {
		c.BeginRenderFrame()
		c.Cls(0)
		c.Camera(cam, 0)
		c.MapAll(1)
		c.Camera(0, 0)
		c.PresentRender()
		scrollX := int(c.mode0ScrollX[0])
		for s := 0; s < 30; s++ {
			px := s*8 + 4
			bgCol := ((px + scrollX) / 8) & 63
			want := uint16(1 + ((px+cam)/8)%3)
			if got := mode0VRAMEntry(bgCol, mapH-1) & 0x3FF; got != want {
				t.Fatalf("camera %d, screen column %d: BG holds tile %d, want %d", cam, s, got, want)
			}
		}
	}
}

// Linked tiles draw as objects, so no Mode 0 path may put them in a BG shadow:
// the small-map full cache, the wide-map viewport, or an mset patch.
func TestMode0LinkedTilesStayOutOfBackgrounds(t *testing.T) {
	requireMode0LayerCount(t, 1)
	for _, mapW := range []int{32, 128} { // full-map cache, then viewport path
		hw.ResetForTest()
		mapData := make([]uint16, mapW*20)
		mapData[3*mapW+5] = 2 // linked
		mapData[3*mapW+6] = 1 // ordinary
		sheet := make([]byte, 32*8)
		for i := range sheet {
			sheet[i] = 5
		}
		c := newMode0TestContext(t, 32, 8, mapW, 20, sheet, mapData, make([]byte, 4))
		c.cart.TileLinkOf = []uint8{0, 0, 1, 0}
		c.cart.TileLinks = []TileLink{{Tile: 2, ID: 0}}
		c.SetTileLinkSites(-1, 0, 0)
		frame := func() {
			c.BeginRenderFrame()
			c.Cls(0)
			c.MapAll(1)
			c.PresentRender()
		}
		frame()
		if got := mode0VRAMEntry(5, 3) & 0x3FF; got != 0 {
			t.Fatalf("map width %d: linked tile reached the BG shadow as %d", mapW, got)
		}
		if got := mode0VRAMEntry(6, 3) & 0x3FF; got != 1 {
			t.Fatalf("map width %d: ordinary neighbour = %d, want 1", mapW, got)
		}
		c.Mset(7, 3, 2)
		frame()
		if got := mode0VRAMEntry(7, 3) & 0x3FF; got != 0 {
			t.Fatalf("map width %d: mset patched a linked tile into the BG shadow as %d", mapW, got)
		}
	}
}
