//go:build mtile_mode0 && mtile_mode0_obj && mtile_mode0_objpack

package runtime

import (
	"testing"
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

func newMode0PackedStageContext(t *testing.T) *Context {
	sheet := make([]byte, 64*64)
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			sheet[y*64+x] = byte(x*3 + y*7 + 1)
		}
	}
	return newMode0TestContext(t, 64, 64, 0, 0, sheet, nil, make([]byte, 64))
}

func assertMode0PackedTilePixels(t *testing.T, c *Context, sources []int, cells int) {
	t.Helper()
	offset := 0
	for _, source := range sources {
		for row := 0; row < cells; row++ {
			for col := 0; col < cells; col++ {
				id := source + row*c.cart.SpriteCols + col
				for y := 0; y < 8; y++ {
					for x := 0; x < 8; x++ {
						want := c.cart.Sheet[((id/c.cart.SpriteCols)*8+y)*c.cart.SheetW+(id%c.cart.SpriteCols)*8+x]
						address := hw.OBJVRAM + uintptr((mode0PackedObjectTileBase+offset)*64+y*8+x)
						got := byte(hw.ReadVRAM32(address&^3) >> (uint(address&3) * 8))
						if got != want {
							t.Fatalf("packed%d source%d pixel%d,%d=%d want%d", offset, id, x, y, got, want)
						}
					}
				}
				offset++
			}
		}
	}
}

func TestMode0PackedPreparationStagesEntireBoundWithoutVisibleWrites(t *testing.T) {
	for _, cells := range []int{2, 4} {
		hw.ResetForTest()
		c := newMode0PackedStageContext(t)
		sources := []int{0, 4}
		if cells == 2 {
			sources = []int{0, 2, 4, 6, 16, 18, 20, 22}
		}
		for _, id := range sources {
			if _, ok := c.mode0PackedObjectTile(id, cells); !ok {
				t.Fatal("valid full packed cache rejected")
			}
		}
		mask := uint8((1 << len(sources)) - 1)
		for repeat := 0; repeat < 2; repeat++ {
			c.PrepareRender()
			if c.mode0PackedObjectPrepared != mask || c.mode0PackedObjectDirty != mask {
				t.Fatal("prepare consumed or omitted packed entries")
			}
			for index := 0; index < mode0PackedObjectTileCapacity; index++ {
				if uintptr(unsafe.Pointer(&c.mode0PackedObjectStage[index*16]))&3 != 0 {
					t.Fatal("unaligned packed DMA source")
				}
				for word := 0; word < 16; word++ {
					if hw.ReadVRAM32(hw.OBJVRAM+uintptr((mode0PackedObjectTileBase+index)*64+word*4)) != 0 {
						t.Fatal("prepare changed visible packed tile VRAM")
					}
				}
			}
		}
		if uploaded := c.presentMode0PackedObjects(); uploaded != mode0PackedObjectTileCapacity*64 {
			t.Fatalf("prepared packed upload%d want2048", uploaded)
		}
		assertMode0PackedTilePixels(t, c, sources, cells)
		c.PrepareRender()
		if c.mode0PackedObjectPrepared != 0 || c.mode0PackedObjectDirty != 0 || c.presentMode0PackedObjects() != 0 {
			t.Fatal("clean packed cache was committed twice")
		}
	}
}

func TestMode0PackedPreparationPreservesLateChangesAndNewEntries(t *testing.T) {
	hw.ResetForTest()
	c := newMode0PackedStageContext(t)
	for _, id := range []int{0, 2} {
		c.mode0PackedObjectTile(id, 2)
	}
	c.PrepareRender()
	if c.mode0PackedObjectPrepared != 3 {
		t.Fatal("initial entries were not prepared")
	}
	// Source changes invalidate matching entries only; a newly allocated cache
	// entry after preparation follows the original direct-call fallback.
	c.Sset(0, 0, 99)
	c.mode0PackedObjectTile(4, 2)
	if c.mode0PackedObjectPrepared != 2 || c.mode0PackedObjectDirty != 7 {
		t.Fatal("late mutation/allocation lost the independent prepared entry")
	}
	if uploaded := c.presentMode0PackedObjects(); uploaded != 12*64 {
		t.Fatalf("mixed prepared/direct upload%d want768", uploaded)
	}
	assertMode0PackedTilePixels(t, c, []int{0, 2, 4}, 2)
}

func TestMode0PackedPreparationDoesNotAllocate(t *testing.T) {
	hw.ResetForTest()
	c := newMode0PackedStageContext(t)
	c.mode0PackedObjectTile(0, 4)
	if allocations := testing.AllocsPerRun(100, func() {
		c.dirtyMode0PackedObject(0)
		c.PrepareRender()
		c.presentMode0PackedObjects()
	}); allocations != 0 {
		t.Fatalf("packed preparation allocated%g objects", allocations)
	}
}
