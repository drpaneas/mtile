//go:build mtile_mode0 && mtile_mode0_obj

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0HardwareSpriteUsesOBJVRAMCameraFlipAndOAM(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	sheet := make([]byte, 24*8)
	sheet[8] = 7
	sheet[16] = 9
	c := newMode0TestContext(t, 24, 8, 1, 1, sheet, []uint16{0}, make([]byte, 3))
	c.BeginRenderFrame()
	c.Cls(0)
	c.Camera(2, 3)
	c.Map(0, 0, 0, 0, 1, 1, 0)
	c.Spr(1, 10, 20, 2, 1, true, false)
	c.PresentRender()

	if got := hw.GetReg16(hw.DISPCNT); got != 0x1140 {
		t.Fatalf("DISPCNT = %#04x, want BG0+OBJ+1D", got)
	}
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM + 64)); got != 7 {
		t.Fatalf("OBJ tile 1 first pixel = %d, want 7", got)
	}
	// The later metasprite cell has higher painter priority and is compacted to
	// the lower OAM index.
	first := hw.OAM
	if got := hw.GetReg16(first); got != 0x2000|17 {
		t.Fatalf("first attr0 = %#04x, want y=17 8bpp", got)
	}
	if got := hw.GetReg16(first + 2); got != 0x1000|16 {
		t.Fatalf("first attr1 = %#04x, want x=16+hflip", got)
	}
	if got := hw.GetReg16(first + 4); got != 2 {
		t.Fatalf("first attr2 = %#04x, want tile number 2", got)
	}
	second := hw.OAM + 8
	if got := hw.GetReg16(second + 2); got != 0x1000|8 {
		t.Fatalf("second attr1 = %#04x, want x=8+hflip", got)
	}
	if got := hw.GetReg16(second + 4); got != 4 { // flipped first cell sources sprite 2; 8bpp index is id*2
		t.Fatalf("second attr2 = %#04x, want tile number 4", got)
	}
	if stats := c.RendererUploadStats(); stats.LastOAMBytes != hw.OAMBytes {
		t.Fatalf("OAM upload = %+v, want %d bytes", stats, hw.OAMBytes)
	}
}

func TestMode0AlignedSSprLowersToMetasprite(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	c := newMode0TestContext(t, 32, 8, 1, 1, make([]byte, 32*8), []uint16{0}, make([]byte, 4))
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 1, 1, 0)
	c.SSpr(8, 0, 16, 8, 20, 30, 16, 8, false, false)
	c.PresentRender()
	first := hw.OAM
	if got := hw.GetReg16(first + 2); got != 28 {
		t.Fatalf("sspr front x = %d, want 28", got)
	}
	if got := hw.GetReg16(first + 4); got != 4 {
		t.Fatalf("sspr front tile = %d, want source sprite 2", got)
	}
	second := hw.OAM + 8
	if got := hw.GetReg16(second + 2); got != 20 {
		t.Fatalf("sspr back x = %d, want 20", got)
	}
	if got := hw.GetReg16(second + 4); got != 2 {
		t.Fatalf("sspr back tile = %d, want source sprite 1", got)
	}
}

func TestMode0ObjectAllocatorUsesAll128OAMEntriesAndStopsSafely(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	c.BeginRenderFrame()
	c.Cls(0)
	for index := 0; index < 128; index++ {
		c.Spr(1, index, 0, 1, 1, false, false)
	}
	if c.mode0ObjectCount != 128 {
		t.Fatalf("object count = %d, want all 128 OAM entries", c.mode0ObjectCount)
	}
	c.Spr(1, 200, 0, 1, 1, false, false)
	if c.mode0ObjectCount != 128 {
		t.Fatalf("overflow object count = %d, want safe cap at 128", c.mode0ObjectCount)
	}
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM + 4); got != 2 {
		t.Fatalf("OAM entry 0 tile = %d, want source tile number 2", got)
	}
	if got := hw.GetReg16(hw.OAM + 2); got != 127 {
		t.Fatalf("OAM entry 0 x = %d, want final draw x=127", got)
	}

	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(1, 9, 0, 1, 1, false, false)
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM + 2); got != 9 {
		t.Fatalf("next-frame OAM entry 0 x = %d, want 9", got)
	}
	if got := hw.GetReg16(hw.OAM + 8); got != mode0ObjectDisable {
		t.Fatalf("next-frame OAM entry 1 attr0 = %#04x, want disabled", got)
	}
}
