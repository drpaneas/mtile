package runtime

import (
	"reflect"
	"testing"
	"unsafe"
)

func mapLayerViewFixture(width, height, layers int, mask uint8) *Context {
	var c Context
	c.cart = &AssetMemory{MapW: width, MapH: height, MapLayers: layers, MapPlaneMask: mask,
		SpriteN: 8, Map: make([]uint16, width*height*mapMaskPopulation(mask)), Flags: make([]byte, 8)}
	for i := range c.cart.Map {
		c.cart.Map[i] = uint16(i % 8)
	}
	return &c
}

func TestMapLayerViewMatchesResidentAccessorsForEveryMaskAndBoundary(t *testing.T) {
	if mapStorageStreaming {
		t.Skip("resident fixture; streaming has a separate fallback test")
	}
	positions := [][2]int{{-1, 0}, {0, -1}, {0, 0}, {2, 1}, {3, 0}, {0, 2}, {2147483647, 0}, {0, -2147483647}}
	for layers := 1; layers <= 4; layers++ {
		for mask := uint8(0); mask < 16; mask++ {
			for layer := -1; layer <= 4; layer++ {
				got, want := mapLayerViewFixture(3, 2, layers, mask), mapLayerViewFixture(3, 2, layers, mask)
				view := got.BorrowMapLayer(layer)
				readView := got.BorrowMapLayerRead(layer)
				got.mapError, want.mapError = MapEditFull, MapEditFull
				for _, p := range positions {
					for _, id := range []int{-1, 0, 1, 7, 8, 65536, 2147483647} {
						view.Set(p[0], p[1], id)
						want.MsetLayer(layer, p[0], p[1], id)
						if !reflect.DeepEqual(got.cart.Map, want.cart.Map) || got.mapError != want.mapError {
							t.Fatalf("layer%d/%d mask%x position%v id%d mutation mismatch", layer, layers, mask, p, id)
						}
						if actual, expected := view.Get(p[0], p[1]), want.MgetLayer(layer, p[0], p[1]); actual != expected {
							t.Fatalf("layer%d mask%x position%v get%d want%d", layer, mask, p, actual, expected)
						}
						if actual, expected := readView.Get(p[0], p[1]), want.MgetLayer(layer, p[0], p[1]); actual != expected {
							t.Fatalf("read-only layer%d mask%x position%v get%d want%d", layer, mask, p, actual, expected)
						}
					}
				}
			}
		}
	}
}

func TestMapLayerViewInvalidBackingRetainsOrdinaryFallback(t *testing.T) {
	var empty MapLayerView
	if empty.Get(0, 0) != 0 {
		t.Fatal("zero view did not preserve nil-cart read")
	}
	empty.Set(0, 0, 1)
	for _, kind := range []string{"nil-cart", "zero-layers", "negative-width", "short-storage", "high-mask"} {
		c := mapLayerViewFixture(2, 2, 1, 1)
		switch kind {
		case "nil-cart":
			c.cart = nil
		case "zero-layers":
			c.cart.MapLayers = 0
		case "negative-width":
			c.cart.MapW = -1
		case "short-storage":
			c.cart.Map = c.cart.Map[:1]
		case "high-mask":
			c.cart.MapPlaneMask = 0x11
		}
		view := c.BorrowMapLayer(0)
		if view.data != nil {
			t.Fatalf("%s accepted malformed backing", kind)
		}
		read := func(fn func() uint16) (value uint16, panicked bool) {
			defer func() { panicked = recover() != nil }()
			return fn(), false
		}
		got, gotPanic := read(func() uint16 { return view.Get(1, 1) })
		want, wantPanic := read(func() uint16 { return c.MgetLayer(0, 1, 1) })
		if got != want || gotPanic != wantPanic {
			t.Fatalf("%s changed malformed-storage behavior", kind)
		}
		write := func(fn func()) (panicked bool) {
			defer func() { panicked = recover() != nil }()
			fn()
			return false
		}
		var before, actual []uint16
		if c.cart != nil {
			before = append([]uint16(nil), c.cart.Map...)
		}
		gotPanic = write(func() { view.Set(1, 1, 1) })
		if c.cart != nil {
			actual = append([]uint16(nil), c.cart.Map...)
			copy(c.cart.Map, before)
		}
		wantPanic = write(func() { c.MsetLayer(0, 1, 1, 1) })
		if gotPanic != wantPanic || c.cart != nil && !reflect.DeepEqual(actual, c.cart.Map) {
			t.Fatalf("%s changed malformed-storage mutation behavior", kind)
		}
	}
}

func TestMapLayerViewStreamingFallbackPreservesEditsEvictionsAndErrors(t *testing.T) {
	if mapStorageFixed && !mapStorageStreaming {
		t.Skip("fixed resident build omits streaming semantics")
	}
	imageA, ramA := streamingMapFixture(96, 80, func(x, y int) uint16 { return uint16((x + y) % 2) })
	imageB, ramB := streamingMapFixture(96, 80, func(x, y int) uint16 { return uint16((x + y) % 2) })
	var got, want Context
	if !got.Init(imageA, ramA) || !want.Init(imageB, ramB) {
		t.Fatal("streaming fixture initialization failed")
	}
	view := got.BorrowMapLayer(0)
	readView := got.BorrowMapLayerRead(0)
	if view.data != nil {
		t.Fatal("streaming backing was borrowed as resident storage")
	}
	for _, p := range [][2]int{{1, 1}, {40, 1}, {72, 1}, {1, 40}, {40, 40}, {72, 72}, {1, 1}, {-1, 1}} {
		for _, id := range []int{0, 1, -1, imageA.SpriteN} {
			view.Set(p[0], p[1], id)
			want.MsetLayer(0, p[0], p[1], id)
			if view.Get(p[0], p[1]) != want.MgetLayer(0, p[0], p[1]) ||
				readView.Get(p[0], p[1]) != want.MgetLayer(0, p[0], p[1]) ||
				got.LastMapError() != want.LastMapError() || got.mapEditCount != want.mapEditCount ||
				!reflect.DeepEqual(ramA.MapEdits, ramB.MapEdits) || !reflect.DeepEqual(ramA.MapChunks, ramB.MapChunks) {
				t.Fatal("streaming view changed cache/overlay/error behavior")
			}
		}
	}
	got.mapError, want.mapError = MapEditFull, MapEditFull
	view.Set(0, 0, 1)
	want.MsetLayer(0, 0, 0, 1)
	if got.LastMapError() != want.LastMapError() {
		t.Fatal("view changed sticky streaming error")
	}
}

func BenchmarkMapLayerView(b *testing.B) {
	if mapStorageStreaming {
		b.Skip("resident benchmark")
	}
	for _, name := range []string{"ordinary", "compact", "borrowed"} {
		b.Run(name, func(b *testing.B) {
			c := mapLayerViewFixture(60, 20, 4, 0x08)
			view := c.BorrowMapLayer(3)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				x, y := i%17+7, i/17%12+4
				if name == "borrowed" {
					view.Set(x, y, int(view.Get(x, y)^1))
				} else if name == "compact" {
					c.MsetCompact(3, 0, x, y, int(c.MgetCompact(3, 0, x, y)^1))
				} else {
					c.MsetLayer(3, x, y, int(c.MgetLayer(3, x, y)^1))
				}
			}
		})
	}
}

func TestMapLayerViewDoesNotAllocate(t *testing.T) {
	if mapStorageStreaming {
		t.Skip("resident allocation check")
	}
	c := mapLayerViewFixture(3, 2, 1, 1)
	if allocations := testing.AllocsPerRun(100, func() {
		view := c.BorrowMapLayer(0)
		readView := c.BorrowMapLayerRead(0)
		view.Set(1, 1, int(readView.Get(1, 1)^1))
	}); allocations != 0 {
		t.Fatalf("borrow/access allocated %g objects", allocations)
	}
}

func TestMapLayerReadViewHasNoRendererCacheStorage(t *testing.T) {
	if unsafe.Sizeof(MapLayerReadView{}) != 6*unsafe.Sizeof(uintptr(0)) {
		t.Fatal("read-only view contains data beyond the validated plane")
	}
	var empty MapLayerReadView
	if empty.Get(0, 0) != 0 {
		t.Fatal("zero read-only view changed nil-cart reads")
	}
}
