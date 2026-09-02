//go:build mtile_mode0 && mtile_mode0_obj && mtile_mode0_objpack

package runtime

import (
	"testing"
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0PackedObjectsKeepOAMDMAAligned(t *testing.T) {
	var state mode0ObjectState
	base := uintptr(unsafe.Pointer(&state))
	oam := uintptr(unsafe.Pointer(&state.mode0OAM[0]))
	if offset := oam - base; offset%4 != 0 {
		t.Fatalf("mode0OAM offset = %d, want 4-byte DMA alignment", offset)
	}
}

func TestMode0Packed16UsesOneObjectAndStableVRAMTiles(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	const sheetW, sheetH = 240, 24
	const cols = sheetW / 8
	sheet := make([]byte, sheetW*sheetH)
	for index, id := range []int{32, 33, 62, 63} {
		x, y := (id%cols)*8, (id/cols)*8
		sheet[y*sheetW+x] = byte(11 + index)
	}
	c := newMode0TestContext(t, sheetW, sheetH, 0, 0, sheet, nil, make([]byte, cols*(sheetH/8)))

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(32, 20, 30, 2, 2, true, true)
	c.PresentRender()

	if c.mode0ObjectCount != 1 {
		t.Fatalf("object count = %d, want one packed 16x16 OBJ", c.mode0ObjectCount)
	}
	first := hw.OAM
	if got := hw.GetReg16(first); got != 0x2000|30 {
		t.Fatalf("attr0 = %#04x, want 8bpp square y=30", got)
	}
	if got := hw.GetReg16(first + 2); got != 0x4000|0x3000|20 {
		t.Fatalf("attr1 = %#04x, want 16x16 x=20 with both flips", got)
	}
	if got := hw.GetReg16(first + 4); got != mode0PackedObjectTileBase*2 {
		t.Fatalf("attr2 = %#04x, want packed tile %d", got, mode0PackedObjectTileBase*2)
	}
	if got := hw.GetReg16(hw.OAM + 8); got != mode0ObjectDisable {
		t.Fatalf("second OAM attr0 = %#04x, want disabled", got)
	}
	for index, want := range []byte{11, 12, 13, 14} {
		got := byte(hw.ReadVRAM32(hw.OBJVRAM + uintptr((mode0PackedObjectTileBase+index)*64)))
		if got != want {
			t.Fatalf("packed tile %d first pixel = %d, want %d", index, got, want)
		}
	}
	if stats := c.RendererUploadStats(); stats.LastOBJTileBytes != 4*64 || stats.LastOAMBytes != hw.OAMBytes {
		t.Fatalf("first packed upload stats = %+v, want 256 OBJ bytes and %d OAM bytes", stats, hw.OAMBytes)
	}

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(32, 20, 30, 2, 2, false, false)
	c.PresentRender()
	if stats := c.RendererUploadStats(); stats.LastOBJTileBytes != 0 {
		t.Fatalf("stable packed sprite re-uploaded %d OBJ bytes", stats.LastOBJTileBytes)
	}
}

func TestMode0Packed32UsesOneObjectAndSixteenContiguousTiles(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	const sheetW, sheetH = 240, 40
	const cols = sheetW / 8
	sheet := make([]byte, sheetW*sheetH)
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			id := 32 + row*cols + col
			x, y := (id%cols)*8, (id/cols)*8
			sheet[y*sheetW+x] = byte(1 + row*4 + col)
		}
	}
	c := newMode0TestContext(t, sheetW, sheetH, 0, 0, sheet, nil, make([]byte, cols*(sheetH/8)))

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(32, 20, 30, 4, 4, true, true)
	c.PresentRender()

	if c.mode0ObjectCount != 1 {
		t.Fatalf("object count = %d, want one packed 32x32 OBJ", c.mode0ObjectCount)
	}
	first := hw.OAM
	if got := hw.GetReg16(first + 2); got != 0x8000|0x3000|20 {
		t.Fatalf("attr1 = %#04x, want 32x32 x=20 with both flips", got)
	}
	if got := hw.GetReg16(first + 4); got != mode0PackedObjectTileBase*2 {
		t.Fatalf("attr2 = %#04x, want packed tile %d", got, mode0PackedObjectTileBase*2)
	}
	for index := 0; index < 16; index++ {
		got := byte(hw.ReadVRAM32(hw.OBJVRAM + uintptr((mode0PackedObjectTileBase+index)*64)))
		if want := byte(index + 1); got != want {
			t.Fatalf("packed tile %d first pixel = %d, want %d", index, got, want)
		}
	}
	if stats := c.RendererUploadStats(); stats.LastOBJTileBytes != 16*64 || stats.LastOAMBytes != hw.OAMBytes {
		t.Fatalf("first packed upload stats = %+v, want 1024 OBJ bytes and %d OAM bytes", stats, hw.OAMBytes)
	}

	c.BeginRenderFrame()
	c.Cls(0)
	c.SSpr(16, 8, 32, 32, 20, 30, 32, 32, false, false)
	c.PresentRender()
	if c.mode0ObjectCount != 1 {
		t.Fatalf("exact 32x32 sspr object count = %d, want one packed OBJ", c.mode0ObjectCount)
	}
	if stats := c.RendererUploadStats(); stats.LastOBJTileBytes != 0 {
		t.Fatalf("stable packed 32x32 sprite re-uploaded %d OBJ bytes", stats.LastOBJTileBytes)
	}
}

func TestMode0PackedObjectCacheMixes32And16Sources(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	const sheetW, sheetH = 240, 48
	const cols = sheetW / 8
	c := newMode0TestContext(t, sheetW, sheetH, 0, 0, make([]byte, sheetW*sheetH), nil, make([]byte, cols*(sheetH/8)))
	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(32, 0, 0, 4, 4, false, false)
	for index, id := range []int{36, 40, 44, 48} {
		c.Spr(id, 40+index*20, 0, 2, 2, false, false)
	}
	if c.mode0PackedObjectCount != 5 || c.mode0PackedObjectTiles != mode0PackedObjectTileCapacity {
		t.Fatalf("mixed cache = %d entries/%d tiles, want 5/%d", c.mode0PackedObjectCount, c.mode0PackedObjectTiles, mode0PackedObjectTileCapacity)
	}
	if c.mode0ObjectCount != 5 {
		t.Fatalf("mixed object count = %d, want five packed OBJs", c.mode0ObjectCount)
	}
}

func TestMode0PackedObjectCacheFitsTwo32AndFallsBackWhenFull(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	const sheetW, sheetH = 240, 40
	const cols = sheetW / 8
	c := newMode0TestContext(t, sheetW, sheetH, 0, 0, make([]byte, sheetW*sheetH), nil, make([]byte, cols*(sheetH/8)))
	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(32, 0, 0, 4, 4, false, false)
	c.Spr(36, 40, 0, 4, 4, false, false)
	c.Spr(40, 80, 0, 2, 2, false, false)
	if c.mode0PackedObjectCount != 2 || c.mode0PackedObjectTiles != mode0PackedObjectTileCapacity {
		t.Fatalf("cache = %d entries/%d tiles, want 2/%d", c.mode0PackedObjectCount, c.mode0PackedObjectTiles, mode0PackedObjectTileCapacity)
	}
	if c.mode0ObjectCount != 6 {
		t.Fatalf("object count = %d, want two packed OBJs plus four-object fallback", c.mode0ObjectCount)
	}
}

func TestMode0PackedObjectMutationRefreshesCachedRectangle(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	const sheetW, sheetH = 240, 24
	const cols = sheetW / 8
	c := newMode0TestContext(t, sheetW, sheetH, 0, 0, make([]byte, sheetW*sheetH), nil, make([]byte, cols*(sheetH/8)))
	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(32, 0, 0, 2, 2, false, false)
	c.PresentRender()

	// Sprite 62 is the lower-left source cell of spr(32,...,2,2).
	c.Sset(16, 16, 27)
	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(32, 0, 0, 2, 2, false, false)
	c.PresentRender()
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM + uintptr((mode0PackedObjectTileBase+2)*64))); got != 27 {
		t.Fatalf("mutated packed lower-left pixel = %d, want 27", got)
	}
}
