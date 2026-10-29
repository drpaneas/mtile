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

func TestFontBMode0StillReservesAll32PhysicalTiles(t *testing.T) {
	for id := Mode0TextTileBase; id < Mode0TextTileBase+Mode0TextTileCount; id++ {
		if !mode0TextObjectTileReserved(id) || !mode0TextObjectWindowReserved(id, 1) {
			t.Fatalf("physical tile %d became writable", id)
		}
	}
	if mode0TextObjectTileReserved(Mode0TextTileBase-1) || mode0TextObjectTileReserved(Mode0TextTileBase+Mode0TextTileCount) {
		t.Fatal("reservation grew outside bank")
	}
}

func TestFontBMode0MixedGlyphPackingUsesCorrectShapesAndOffsets(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 8, 8, 1, 1, make([]byte, 64), []uint16{0}, make([]byte, 1))
	loadMode0TextTestAtlas(t, c, mode0TextTestGlyph{'A', 7}, mode0TextTestGlyph{'g', 7})
	if c.mode0TextLayout[0] != 0 || c.mode0TextLayout[1] != 0x81 {
		t.Fatalf("mixed layout=%v", c.mode0TextLayout[:2])
	}
	c.BeginRenderFrame()
	c.Cls(0)
	c.PrintV(c.InternStatic("Ag"), IntVal(0), IntVal(0), IntVal(7))
	c.PresentRender()
	if hw.GetReg16(hw.OAM)&0xc000 != 0x8000 || hw.GetReg16(hw.OAM+4) != (Mode0TextTileBase+1)*2 {
		t.Fatal("tall glyph shape/offset")
	}
	if hw.GetReg16(hw.OAM+8)&0xc000 != 0 || hw.GetReg16(hw.OAM+12) != Mode0TextTileBase*2 {
		t.Fatal("compact glyph shape/offset")
	}
	// A custom pixel in the tenth row must force a pair, even for an uppercase A.
	f := *DefaultFont()
	f.ASCII['A'-32][5] |= 512
	c.SetFont(&f)
	loadMode0TextTestAtlas(t, c, mode0TextTestGlyph{'A', 7})
	if c.mode0TextLayout[0] != 0x80 {
		t.Fatal("custom tenth-row pixel was silently cropped")
	}
}
