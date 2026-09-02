//go:build mtile_mode0 && mtile_mode0_obj

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0SpriteSelectsNonContiguousRuntimeFrameIDs(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	c := newMode0TestContext(t, 32, 8, 1, 1, make([]byte, 32*8), []uint16{0}, make([]byte, 4))

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(1, 10, 20, 1, 1, false, false)
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM + 4); got != 2 {
		t.Fatalf("first frame OBJ tile = %d, want logical sprite 1", got)
	}

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(3, 10, 20, 1, 1, false, false)
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM + 4); got != 6 {
		t.Fatalf("second frame OBJ tile = %d, want non-contiguous logical sprite 3", got)
	}
}

func TestMode0TransparentSpriteZeroIsAnInactiveDynamicSlot(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	c := newMode0TestContext(t, 32, 8, 1, 1, make([]byte, 32*8), []uint16{0}, make([]byte, 4))

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(1, 10, 20, 1, 1, false, false)
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM + 4); got != 2 {
		t.Fatalf("active frame OBJ tile = %d, want logical sprite 1", got)
	}

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(0, 10, 20, 1, 1, false, false)
	if c.mode0ObjectCount != 0 {
		t.Fatalf("transparent sprite 0 allocated %d OAM entries, want none", c.mode0ObjectCount)
	}
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM); got != mode0ObjectDisable {
		t.Fatalf("inactive frame OAM0 attr0 = %#04x, want disabled", got)
	}
	if stats := c.RendererUploadStats(); stats.LastOAMBytes != hw.OAMBytes {
		t.Fatalf("inactive transition OAM upload = %d, want one clearing upload of %d bytes", stats.LastOAMBytes, hw.OAMBytes)
	}

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(3, 10, 20, 1, 1, false, false)
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM + 4); got != 6 {
		t.Fatalf("reactivated frame OBJ tile = %d, want logical sprite 3", got)
	}
}

func TestMode0DirtySheetUploadCountsBGAndOBJBytesSeparately(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	c.Sset(8, 0, 9)
	c.BeginRenderFrame()
	c.Cls(0)
	c.PresentRender()
	stats := c.RendererUploadStats()
	if stats.LastTileBytes != 64 || stats.LastOBJTileBytes != 64 {
		t.Fatalf("dirty sheet uploads = %+v, want BG=64 and OBJ=64", stats)
	}
}

func TestMode0VisibleSpriteZeroStillRendersAndTracksSset(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	sheet := make([]byte, 16*8)
	sheet[0] = 7
	c := newMode0TestContext(t, 16, 8, 1, 1, sheet, []uint16{0}, make([]byte, 2))
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM)); got != 7 {
		t.Fatalf("initial OBJ tile 0 pixel = %d, want authored color 7", got)
	}
	if got := byte(hw.ReadVRAM32(hw.VRAM)); got != 0 {
		t.Fatalf("initial BG tile 0 pixel = %d, want map sentinel transparency", got)
	}

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(0, 10, 20, 1, 1, false, false)
	if c.mode0ObjectCount != 1 {
		t.Fatalf("visible sprite 0 allocated %d OAM entries, want one", c.mode0ObjectCount)
	}
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM + 4); got != 0 {
		t.Fatalf("visible sprite 0 OBJ tile = %d, want tile 0", got)
	}

	c.Sset(0, 0, 0)
	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(0, 10, 20, 1, 1, false, false)
	if c.mode0ObjectCount != 0 {
		t.Fatalf("sprite 0 cleared by sset allocated %d OAM entries, want none", c.mode0ObjectCount)
	}
	c.PresentRender()
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM)); got != 0 {
		t.Fatalf("cleared OBJ tile 0 pixel = %d, want 0", got)
	}

	c.Sset(0, 0, 9)
	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(0, 10, 20, 1, 1, false, false)
	if c.mode0ObjectCount != 1 {
		t.Fatalf("sprite 0 restored by sset allocated %d OAM entries, want one", c.mode0ObjectCount)
	}
	c.PresentRender()
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM)); got != 9 {
		t.Fatalf("restored OBJ tile 0 pixel = %d, want 9", got)
	}
	if got := byte(hw.ReadVRAM32(hw.VRAM)); got != 0 {
		t.Fatalf("sset leaked sprite 0 into BG tile 0: got %d, want 0", got)
	}
}

func TestMode0TransparentSpriteZeroDoesNotHideRestOfMetasprite(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	sheet := make([]byte, 16*8)
	sheet[8] = 5
	c := newMode0TestContext(t, 16, 8, 1, 1, sheet, []uint16{0}, make([]byte, 2))

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(0, 10, 20, 2, 1, false, false)
	if c.mode0ObjectCount != 1 {
		t.Fatalf("two-cell sprite beginning at transparent ID 0 allocated %d OAM entries, want the one visible cell", c.mode0ObjectCount)
	}
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM + 2); got != 18 {
		t.Fatalf("remaining cell x = %d, want 18", got)
	}
	if got := hw.GetReg16(hw.OAM + 4); got != 2 {
		t.Fatalf("remaining cell tile = %d, want logical sprite 1", got)
	}
}

func TestMode0CompactVisibleSpriteZeroUsesOBJTileZero(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	cell := make([]byte, 64)
	cell[0] = 11
	image := AssetImage{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		MapW: 1, MapH: 1, MapLayers: 1, SheetReadOnly: true,
		Sheet: cell, SheetIndex: []uint16{1}, Map: []uint16{0}, Flags: []byte{0},
	}
	ram := &AssetMemory{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		MapW: 1, MapH: 1, MapLayers: 1, SheetReadOnly: true,
		Sheet: make([]byte, 64), SheetIndex: make([]uint16, 1), Map: make([]uint16, 1), Flags: make([]byte, 1),
	}
	var c Context
	if !c.Init(&image, ram) {
		t.Fatal("Init rejected compact sprite-zero fixture")
	}
	c.InitHardware()

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(0, 10, 20, 1, 1, false, false)
	if c.mode0ObjectCount != 1 {
		t.Fatalf("compact visible sprite 0 allocated %d OAM entries, want one", c.mode0ObjectCount)
	}
	c.PresentRender()
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM)); got != 11 {
		t.Fatalf("compact OBJ tile 0 pixel = %d, want 11", got)
	}
	if got := byte(hw.ReadVRAM32(hw.VRAM)); got != 0 {
		t.Fatalf("compact sprite 0 leaked into BG tile 0: got %d, want 0", got)
	}
}
