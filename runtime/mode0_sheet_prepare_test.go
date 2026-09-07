//go:build mtile_mode0 && !mtile_mode0_sheet_immutable

package runtime

import (
	"testing"
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0PrepareStagesMutableTilesWithoutChangingVisibleVRAM(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 160, 8, 1, 1, make([]byte, 160*8), []uint16{0}, make([]byte, 20))
	for id := 0; id < mode0PreparedTileCapacity; id++ {
		c.Sset(id*8, 0, id+1)
	}
	c.BeginRenderFrame()
	c.Cls(0)
	for repeat := 0; repeat < 2; repeat++ {
		c.PrepareRender()
		if !c.mode0TilesPrepared || c.mode0TilesOverflow || c.mode0PreparedTileCount != mode0PreparedTileCapacity {
			t.Fatal("accepted mutable budget was not completely staged")
		}
		for id := 0; id < mode0PreparedTileCapacity; id++ {
			if hw.ReadVRAM32(hw.VRAM+uintptr(id*64)) != 0 || hw.ReadVRAM32(hw.OBJVRAM+uintptr(id*64)) != 0 {
				t.Fatal("prepare changed visible tile VRAM before presentation")
			}
			if c.mode0TileDirty[id>>3]&(1<<uint(id&7)) == 0 {
				t.Fatal("prepare consumed a dirty tile before commit")
			}
			tile := c.preparedMode0Tile(id)
			if tile[0] != byte(id+1) || uintptr(unsafe.Pointer(&tile[0]))&3 != 0 {
				t.Fatal("prepared tile layout or DMA alignment changed")
			}
		}
	}
	c.PresentRender()
	for id := 0; id < mode0PreparedTileCapacity; id++ {
		bg := byte(0)
		if id != 0 {
			bg = byte(id + 1)
		}
		if byte(hw.ReadVRAM32(hw.VRAM+uintptr(id*64))) != bg {
			t.Fatalf("BG tile%d lost prepared pixels or sprite-zero transparency", id)
		}
		if mode0ObjectDisplayBits() != 0 && byte(hw.ReadVRAM32(hw.OBJVRAM+uintptr(id*64))) != byte(id+1) {
			t.Fatalf("OBJ tile%d lost prepared pixels", id)
		}
	}
	stats := c.RendererUploadStats()
	objBytes := uint32(0)
	if mode0ObjectDisplayBits() != 0 {
		objBytes = mode0PreparedTileCapacity * 64
	}
	if stats.LastTileBytes != (mode0PreparedTileCapacity-1)*64 || stats.LastOBJTileBytes != objBytes {
		t.Fatalf("prepared upload accounting changed: %+v", stats)
	}
	c.BeginRenderFrame()
	c.Cls(0)
	c.PrepareRender()
	c.PresentRender()
	if stats := c.RendererUploadStats(); stats.LastTileBytes != 0 || stats.LastOBJTileBytes != 0 {
		t.Fatal("committed tiles were uploaded again")
	}
}

func TestMode0PreparedTilesPreserveLateMutationAndOverflowFallback(t *testing.T) {
	for _, count := range []int{1, mode0PreparedTileCapacity + 1, 20} {
		for _, late := range []bool{false, true} {
			hw.ResetForTest()
			c := newMode0TestContext(t, 168, 8, 1, 1, make([]byte, 168*8), []uint16{0}, make([]byte, 21))
			for id := 1; id <= count; id++ {
				c.Sset(id*8, 0, id)
			}
			c.BeginRenderFrame()
			c.Cls(0)
			c.PrepareRender()
			if c.mode0TilesOverflow != (count > mode0PreparedTileCapacity) {
				t.Fatal("unproven direct-runtime upload was not detected")
			}
			if late {
				c.Sset(8, 0, 99)
				if c.mode0TilesPrepared {
					t.Fatal("late mutation retained a stale snapshot")
				}
			}
			c.PresentRender()
			for id := 1; id <= count; id++ {
				want := byte(id)
				if late && id == 1 {
					want = 99
				}
				if byte(hw.ReadVRAM32(hw.VRAM+uintptr(id*64))) != want {
					t.Fatalf("count%d late%v tile%d changed fallback pixels", count, late, id)
				}
			}
			if c.RendererUploadStats().LastTileBytes != uint32(count*64) {
				t.Fatal("overflow/late fallback lost or duplicated an upload")
			}
		}
	}
}

func TestMode0PreparedTilesDoNotAllocate(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 128), []uint16{0}, make([]byte, 2))
	if allocations := testing.AllocsPerRun(100, func() {
		c.Sset(8, 0, int(c.Sget(8, 0)^1))
		c.PrepareRender()
		c.uploadMode0DirtyTiles()
	}); allocations != 0 {
		t.Fatalf("tile preparation allocated %g objects", allocations)
	}
}
