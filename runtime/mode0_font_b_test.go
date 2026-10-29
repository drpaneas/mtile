//go:build mtile_mode0 && mtile_mode0_text && mtile_mode0_obj

package runtime

import (
	"github.com/drpaneas/mtile/runtime/hw"
	"testing"
)

func TestFontBMode0Uses8x16ObjectAndTwoTiles(t *testing.T) {
	tile := BuildMode0GlyphTile('g', 7)
	if len(tile) != 128 {
		t.Fatal("B glyph was clipped into one tile")
	}
	hw.ResetForTest()
	c := newMode0TestContext(t, 8, 8, 1, 1, make([]byte, 64), []uint16{0}, make([]byte, 1))
	loadMode0TextTestAtlas(t, c, mode0TextTestGlyph{'g', 7})
	c.BeginRenderFrame()
	c.Cls(0)
	c.PrintV(c.InternStatic("g"), IntVal(0), IntVal(0), IntVal(7))
	c.PresentRender()
	if hw.GetReg16(hw.OAM)&0xc000 != 0x8000 {
		t.Fatal("font object is not vertical 8x16")
	}
	for i := 0; i < 128; i++ {
		got := byte(hw.ReadVRAM32(hw.OBJVRAM+uintptr(Mode0TextTileBase*64+i/4*4)) >> uint(i%4*8))
		if got != tile[i] {
			t.Fatalf("uploaded glyph byte %d differs", i)
		}
	}
}
