//go:build !mtile_mode0 && !mtile_tilecanvas

package runtime

import (
	"testing"
	"unsafe"
)

func TestMapLayerViewMode4HasNoRendererCacheStorage(t *testing.T) {
	type originalView struct {
		Context                       *Context
		Data                          *uint16
		Layer, Width, Height, SpriteN int
	}
	if unsafe.Sizeof(MapLayerView{}) != unsafe.Sizeof(originalView{}) {
		t.Fatal("Mode0 cache metadata enlarged Mode4 views")
	}
}

func TestMapLayerViewInvalidatesMode4Raster(t *testing.T) {
	if mapStorageStreaming {
		t.Skip("resident raster fixture")
	}
	c := mapLayerViewFixture(1, 1, 4, 0x08)
	c.cart.SheetW, c.cart.SheetH = 64, 8
	c.cart.SpriteCols, c.cart.SpriteRows = 8, 1
	c.cart.Sheet = make([]byte, 64*8)
	for y := 0; y < 8; y++ {
		for x := 8; x < 24; x++ {
			c.cart.Sheet[y*64+x] = byte(x / 8)
		}
	}
	for _, id := range []int{1, 2, 0, 1} {
		view := c.BorrowMapLayer(3)
		view.Set(0, 0, id)
		c.Cls(0)
		c.MapBG(3, 0, 0, 0, 0, 1, 1, 0)
		if got := c.Pget(0, 0); got != byte(id) {
			t.Fatalf("map view write%d left stale pixel%d", id, got)
		}
	}
}
