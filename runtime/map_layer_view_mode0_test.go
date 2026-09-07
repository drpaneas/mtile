//go:build mtile_mode0 || mtile_tilecanvas

package runtime

import (
	"math/rand"
	"reflect"
	"testing"
	"unsafe"
)

func TestMapLayerViewKeepsMode0PatchesAndDrawSnapshots(t *testing.T) {
	if mapStorageStreaming || mode0LayerCount == 0 {
		t.Skip("requires resident Mode0 maps")
	}
	for _, source := range []int{0, 3} {
		for _, drawing := range []bool{false, true} {
			got, want := mapLayerViewFixture(3, 2, 4, 0x09), mapLayerViewFixture(3, 2, 4, 0x09)
			for _, c := range []*Context{got, want} {
				c.mode0DrawActive = drawing
				c.mode0MapSeen = 1
				for slot := 0; slot < mode0LayerCount; slot++ {
					c.mode0MapSource[slot] = uint8(source)
					c.mode0MapSigSet[slot] = true
					c.mode0MapFullCache[slot] = true
					c.mode0MapCellsW[slot], c.mode0MapCellsH[slot] = 3, 2
					c.mode0MapLayer[slot] = 1
				}
			}
			view := got.BorrowMapLayer(source)
			if littleEndianNumberWords && (view.fallback || view.count != mode0LayerCount) {
				t.Fatal("aligned valid renderer did not borrow every matching cache")
			}
			for _, id := range []int{2, 2, 3, 0, -1, 8} {
				view.Set(1, 1, id)
				want.MsetLayer(source, 1, 1, id)
				got.Fset(2, 0, true)
				want.Fset(2, 0, true)
				if !reflect.DeepEqual(got.renderState, want.renderState) {
					t.Fatalf("BG%d drawing%v id%d changed cache patch/snapshot behavior", source, drawing, id)
				}
			}
		}
	}
}

func TestMapLayerViewMatchesMode0WindowClipWrapAndFlagMutations(t *testing.T) {
	if mapStorageStreaming || mode0LayerCount == 0 {
		t.Skip("requires resident Mode0 maps")
	}
	rng := rand.New(rand.NewSource(470))
	for trial := 0; trial < 160; trial++ {
		got := mapLayerViewFixture(40, 24, 4, 0x0f)
		got.cart.SpriteN = mode0TileLimit + 2
		got.cart.Flags = make([]byte, got.cart.SpriteN)
		for i := range got.cart.Flags {
			got.cart.Flags[i] = byte(rng.Intn(256))
		}
		source := trial % 4
		got.clipX0, got.clipX1 = rng.Intn(256)-8, rng.Intn(256)-8
		got.clipY0, got.clipY1 = rng.Intn(176)-8, rng.Intn(176)-8
		got.mode0DrawActive = trial%3 == 0
		got.mode0MapSeen = uint8(rng.Intn(16))
		for slot := 0; slot < mode0LayerCount; slot++ {
			got.mode0MapSource[slot] = uint8((source + slot%2) % 4)
			got.mode0MapSigSet[slot] = rng.Intn(4) != 0
			got.mode0MapFullCache[slot] = rng.Intn(2) != 0
			got.mode0MapCellX[slot], got.mode0MapCellY[slot] = rng.Intn(24)-8, rng.Intn(16)-8
			got.mode0MapCellsW[slot], got.mode0MapCellsH[slot] = rng.Intn(64)+1, rng.Intn(32)+1
			got.mode0MapTileX[slot], got.mode0MapTileY[slot] = rng.Intn(129)-64, rng.Intn(65)-32
			got.mode0ScrollX[slot], got.mode0ScrollY[slot] = uint16(rng.Intn(512)), uint16(rng.Intn(256))
			got.mode0MapLayer[slot] = []byte{0, 1, 3, 128}[trial%4]
			for i := range got.mode0Screen[slot] {
				got.mode0Screen[slot][i] = byte(rng.Intn(256))
			}
		}
		want := mapLayerViewFixture(40, 24, 4, 0x0f)
		want.cart.SpriteN, want.cart.Flags = got.cart.SpriteN, append([]byte(nil), got.cart.Flags...)
		want.renderState = got.renderState
		want.clipX0, want.clipX1, want.clipY0, want.clipY1 = got.clipX0, got.clipX1, got.clipY0, got.clipY1
		view := got.BorrowMapLayer(source)
		if littleEndianNumberWords && view.fallback {
			t.Fatal("normal cache unexpectedly used fallback")
		}
		for step := 0; step < 80; step++ {
			x, y := rng.Intn(42)-1, rng.Intn(26)-1
			id := []int{0, 1, 2, 3, mode0TileLimit - 1, mode0TileLimit, mode0TileLimit + 1}[step%7]
			view.Set(x, y, id)
			want.MsetLayer(source, x, y, id)
			if step%9 == 0 {
				got.Fset(id, 0, step%2 == 0)
				want.Fset(id, 0, step%2 == 0)
			}
			if !reflect.DeepEqual(got.renderState, want.renderState) || !reflect.DeepEqual(got.cart.Map, want.cart.Map) {
				t.Fatalf("trial%d step%d source%d position%d,%d id%d changed exact cache state", trial, step, source, x, y, id)
			}
		}
	}
}

func TestMapLayerViewMode0MalformedFlagsFallback(t *testing.T) {
	if mapStorageStreaming || mode0LayerCount == 0 {
		t.Skip("requires resident Mode0 maps")
	}
	c := mapLayerViewFixture(3, 2, 1, 1)
	c.cart.Flags = nil
	view := c.BorrowMapLayer(0)
	if !view.fallback {
		t.Fatal("malformed flag backing lost ordinary accessor fallback")
	}
	view.Set(0, 0, 1)
	if c.cart.Map[0] != 1 {
		t.Fatal("malformed flags changed unrelated map writes")
	}
}

func BenchmarkMapLayerViewCachedPatches(b *testing.B) {
	if mapStorageStreaming || mode0LayerCount == 0 {
		b.Skip("requires resident Mode0 maps")
	}
	for _, name := range []string{"ordinary-hook", "borrowed-patches"} {
		b.Run(name, func(b *testing.B) {
			c := mapLayerViewFixture(60, 20, 4, 0x08)
			for slot := 0; slot < mode0LayerCount; slot++ {
				c.mode0MapSource[slot] = uint8(slot)
				c.mode0MapSigSet[slot], c.mode0MapFullCache[slot] = true, true
				c.mode0MapCellsW[slot], c.mode0MapCellsH[slot] = 60, 20
			}
			last := mode0LayerCount - 1
			c.mode0MapSource[last] = 3
			view := c.BorrowMapLayer(3)
			if name == "ordinary-hook" {
				view.fallback = true
			}
			b.ReportMetric(float64(unsafe.Sizeof(view)), "view-bytes")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				x, y := i%17+7, i/17%12+4
				view.Set(x, y, int(view.Get(x, y)^1))
			}
		})
	}
}
