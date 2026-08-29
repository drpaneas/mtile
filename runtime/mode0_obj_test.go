//go:build mtile_mode0 && mtile_mode0_obj

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0HardwareSpriteUsesOBJVRAMCameraFlipAndOAM(t *testing.T) {
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
	first := hw.OAM + uintptr(127*8)
	if got := hw.GetReg16(first); got != 0x2000|17 {
		t.Fatalf("first attr0 = %#04x, want y=17 8bpp", got)
	}
	if got := hw.GetReg16(first + 2); got != 0x1000|8 {
		t.Fatalf("first attr1 = %#04x, want x=8+hflip", got)
	}
	if got := hw.GetReg16(first + 4); got != 4 { // flipped first cell sources sprite 2; 8bpp index is id*2
		t.Fatalf("first attr2 = %#04x, want tile number 4", got)
	}
	second := hw.OAM + uintptr(126*8)
	if got := hw.GetReg16(second + 2); got != 0x1000|16 {
		t.Fatalf("second attr1 = %#04x, want x=16+hflip", got)
	}
	if got := hw.GetReg16(second + 4); got != 2 {
		t.Fatalf("second attr2 = %#04x, want tile number 2", got)
	}
	if stats := c.RendererUploadStats(); stats.LastOAMBytes != hw.OAMBytes {
		t.Fatalf("OAM upload = %+v, want %d bytes", stats, hw.OAMBytes)
	}
}

func TestMode0AlignedSSprLowersToMetasprite(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 32, 8, 1, 1, make([]byte, 32*8), []uint16{0}, make([]byte, 4))
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 1, 1, 0)
	c.SSpr(8, 0, 16, 8, 20, 30, 16, 8, false, false)
	c.PresentRender()
	first := hw.OAM + uintptr(127*8)
	if got := hw.GetReg16(first + 2); got != 20 {
		t.Fatalf("sspr first x = %d, want 20", got)
	}
	if got := hw.GetReg16(first + 4); got != 2 {
		t.Fatalf("sspr first tile = %d, want source sprite 1", got)
	}
	second := hw.OAM + uintptr(126*8)
	if got := hw.GetReg16(second + 4); got != 4 {
		t.Fatalf("sspr second tile = %d, want source sprite 2", got)
	}
}
