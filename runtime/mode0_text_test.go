//go:build mtile_mode0 && mtile_mode0_text && mtile_mode0_obj

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0StaticTextStagesGlyphTileAndObject(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	c.BeginRenderFrame()
	c.Cls(0)
	c.Camera(2, 1)
	c.Map(0, 0, 0, 0, 1, 1, 0)
	if got := c.PrintV(c.InternStatic("A"), IntVal(8), IntVal(6), IntVal(12)); got != 14 {
		t.Fatalf("print return = %d, want 14", got)
	}
	c.PresentRender()

	tileBase := hw.OBJVRAM + uintptr(mode0TextTileBase*64)
	if got := byte(hw.ReadVRAM32(tileBase)); got != 12 {
		t.Fatalf("glyph first pixel = %d, want color 12", got)
	}
	oam := hw.OAM + uintptr(127*8)
	if got := hw.GetReg16(oam); got != 0x2000|5 {
		t.Fatalf("glyph attr0 = %#04x, want y=5", got)
	}
	if got := hw.GetReg16(oam + 2); got != 6 {
		t.Fatalf("glyph attr1 = %#04x, want x=6", got)
	}
	if got := hw.GetReg16(oam + 4); got != mode0TextTileBase*2 {
		t.Fatalf("glyph attr2 = %#04x, want text tile", got)
	}
	stats := c.RendererUploadStats()
	if stats.LastOBJTileBytes != 64 || stats.LastOAMBytes != hw.OAMBytes {
		t.Fatalf("text uploads = %+v, want OBJ tile=64 OAM=%d", stats, hw.OAMBytes)
	}
}
