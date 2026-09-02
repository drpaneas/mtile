//go:build mtile_mode0 && mtile_mode0_layers0 && mtile_mode0_obj && mtile_mode0_text

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0ZeroLayersPresentsBackdropAndText(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 0, 0, make([]byte, 16*8), nil, make([]byte, 2))
	loadMode0TextTestAtlas(t, c, mode0TextTestGlyph{'A', 12})
	var palette [256]uint16
	palette[5] = 0x1234
	c.LoadPalette(&palette)

	c.BeginRenderFrame()
	c.Cls(5)
	if got := c.PrintV(c.InternStatic("A"), IntVal(8), IntVal(6), IntVal(12)); got != 14 {
		t.Fatalf("print return = %d, want 14", got)
	}
	c.PresentRender()

	if got := hw.GetReg16(hw.DISPCNT); got != mode0ObjectDisplayBits() {
		t.Fatalf("DISPCNT = %#04x, want OBJ-only Mode 0 %#04x", got, mode0ObjectDisplayBits())
	}
	if got := hw.ReadPaletteRAM16(0); got != palette[5] {
		t.Fatalf("backdrop palette = %#04x, want %#04x", got, palette[5])
	}
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM + uintptr(mode0TextTileBase*64))); got != 12 {
		t.Fatalf("glyph first pixel = %d, want 12", got)
	}
}
