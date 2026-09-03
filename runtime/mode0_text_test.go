//go:build mtile_mode0 && mtile_mode0_text && mtile_mode0_obj

package runtime

import (
	"sort"
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

type mode0TextTestGlyph struct {
	ch, color byte
}

func loadMode0TextTestAtlas(t *testing.T, c *Context, glyphs ...mode0TextTestGlyph) {
	t.Helper()
	keys := make([]uint16, 0, len(glyphs))
	byKey := make(map[uint16][64]byte, len(glyphs))
	for _, glyph := range glyphs {
		key := Mode0TextGlyphKey(glyph.ch, glyph.color)
		if _, exists := byKey[key]; exists {
			continue
		}
		keys = append(keys, key)
		byKey[key] = BuildMode0GlyphTile(glyph.ch, glyph.color)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	tiles := make([]uint32, 0, len(keys)*16)
	for _, key := range keys {
		tile := byKey[key]
		for offset := 0; offset < len(tile); offset += 4 {
			tiles = append(tiles,
				uint32(tile[offset])|uint32(tile[offset+1])<<8|uint32(tile[offset+2])<<16|uint32(tile[offset+3])<<24)
		}
	}
	if !c.LoadMode0TextAtlas(keys, tiles) {
		t.Fatalf("LoadMode0TextAtlas rejected valid keys %#v", keys)
	}
}

func TestMode0StaticTextStagesGlyphTileAndObject(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	coldBeforeAtlas := c.RendererUploadStats().ColdTileBytes
	loadMode0TextTestAtlas(t, c, mode0TextTestGlyph{'A', 12})
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
	oam := hw.OAM
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
	if stats.ColdTileBytes != coldBeforeAtlas+64 || stats.LastOBJTileBytes != 0 || stats.LastOAMBytes != hw.OAMBytes {
		t.Fatalf("text uploads = %+v, want one cold 64-byte tile, no frame tile upload, OAM=%d", stats, hw.OAMBytes)
	}
}

func TestMode0PrintStaticVMatchesLiteralPrintV(t *testing.T) {
	hw.ResetForTest()
	generic := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	specialized := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	loadMode0TextTestAtlas(t, generic, mode0TextTestGlyph{'A', 11})
	loadMode0TextTestAtlas(t, specialized, mode0TextTestGlyph{'A', 11})
	generic.BeginRenderFrame()
	specialized.BeginRenderFrame()
	generic.Cls(0)
	specialized.Cls(0)

	wantX := generic.PrintV(generic.InternStatic("A"), IntVal(30), IntVal(20), IntVal(11))
	gotX := specialized.PrintStaticV(specialized.InternStatic("A"), IntVal(30), IntVal(20), IntVal(11))
	if gotX != wantX || specialized.cursor != generic.cursor ||
		specialized.mode0ObjectCount != generic.mode0ObjectCount || specialized.mode0OAM != generic.mode0OAM {
		t.Fatalf("Mode 0 static print differs: x=%d/%d cursor=%+v/%+v objects=%d/%d",
			gotX, wantX, specialized.cursor, generic.cursor, specialized.mode0ObjectCount, generic.mode0ObjectCount)
	}

	generic.BeginRenderFrame()
	specialized.BeginRenderFrame()
	generic.Cursor(4, 5, 11)
	specialized.Cursor(4, 5, 11)
	generic.PrintV(generic.InternStatic("A"), NilVal(), NilVal(), IntVal(11))
	specialized.PrintStaticV(specialized.InternStatic("A"), NilVal(), NilVal(), IntVal(11))
	if specialized.cursor != generic.cursor || specialized.mode0ObjectCount != generic.mode0ObjectCount ||
		specialized.mode0OAM != generic.mode0OAM {
		t.Fatalf("Mode 0 implicit static print differs: cursor=%+v/%+v objects=%d/%d",
			specialized.cursor, generic.cursor, specialized.mode0ObjectCount, generic.mode0ObjectCount)
	}
}

func TestMode0TextCullsWrappedOffscreenGlyphs(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	loadMode0TextTestAtlas(t, c, mode0TextTestGlyph{'A', 7})
	c.BeginRenderFrame()
	c.Cls(0)

	if got := c.PrintV(c.InternStatic("A"), IntVal(512), IntVal(256), IntVal(7)); got != 518 {
		t.Fatalf("offscreen print return = %d, want 518", got)
	}
	if c.mode0ObjectCount != 0 || c.mode0TextTileCount != 1 {
		t.Fatalf("offscreen glyph allocated objects=%d with atlas tiles=%d, want zero objects and one persistent tile", c.mode0ObjectCount, c.mode0TextTileCount)
	}

	c.PrintV(c.InternStatic("A"), IntVal(0), IntVal(0), IntVal(7))
	if c.mode0ObjectCount != 1 || c.mode0TextTileCount != 1 {
		t.Fatalf("visible glyph allocated objects=%d atlas tiles=%d, want one each", c.mode0ObjectCount, c.mode0TextTileCount)
	}
}

func TestMode0TextUsesCursorForNonNumericPosition(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	loadMode0TextTestAtlas(t, c, mode0TextTestGlyph{'A', 7})
	c.BeginRenderFrame()
	c.Cls(0)
	c.Cursor(4, 5, 7)
	if got := c.PrintV(c.InternStatic("A"), NilVal(), IntVal(0), IntVal(7)); got != 10 {
		t.Fatalf("implicit print return = %d, want 10", got)
	}
	if c.cursor.x != 4 || c.cursor.y != 13 {
		t.Fatalf("implicit print cursor = (%d,%d), want (4,13)", c.cursor.x, c.cursor.y)
	}
	if c.mode0ObjectCount != 1 || c.mode0TextTileCount != 1 {
		t.Fatalf("implicit print allocated objects=%d atlas tiles=%d, want one each", c.mode0ObjectCount, c.mode0TextTileCount)
	}
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM); got != 0x2000|5 {
		t.Fatalf("implicit glyph attr0 = %#04x, want y=5", got)
	}
	if got := hw.GetReg16(hw.OAM + 2); got != 4 {
		t.Fatalf("implicit glyph attr1 = %#04x, want x=4", got)
	}
}

func TestMode0RepeatedGlyphOccurrencesShareOnePersistentTile(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	loadMode0TextTestAtlas(t, c, mode0TextTestGlyph{'A', 7})
	c.BeginRenderFrame()
	c.Cls(0)
	c.PrintV(c.InternStatic("AAA"), IntVal(0), IntVal(0), IntVal(7))
	if c.mode0ObjectCount != 3 || c.mode0TextTileCount != 1 {
		t.Fatalf("objects=%d atlas tiles=%d, want three occurrences sharing one tile", c.mode0ObjectCount, c.mode0TextTileCount)
	}
	c.PresentRender()
	for object := 0; object < 3; object++ {
		if got := hw.GetReg16(hw.OAM + uintptr(object*8+4)); got != mode0TextTileBase*2 {
			t.Fatalf("object %d tile = %d, want shared tile %d", object, got, mode0TextTileBase*2)
		}
	}
	if stats := c.RendererUploadStats(); stats.LastOBJTileBytes != 0 {
		t.Fatalf("steady frame re-uploaded %d text bytes", stats.LastOBJTileBytes)
	}
}

func TestMode0SameGlyphDifferentColorsUseDistinctInternedTiles(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	loadMode0TextTestAtlas(t, c, mode0TextTestGlyph{'A', 7}, mode0TextTestGlyph{'A', 12})
	c.BeginRenderFrame()
	c.Cls(0)
	c.PrintV(c.InternStatic("A"), IntVal(0), IntVal(0), IntVal(7))
	c.PrintV(c.InternStatic("A"), IntVal(8), IntVal(0), IntVal(12))
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM + 4); got != (mode0TextTileBase+1)*2 {
		t.Fatalf("front color-12 glyph tile = %d, want second interned tile", got)
	}
	if got := hw.GetReg16(hw.OAM + 8 + 4); got != mode0TextTileBase*2 {
		t.Fatalf("back color-7 glyph tile = %d, want first interned tile", got)
	}
}

func TestMode0MissingGlyphAtlasKeyFailsClosed(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	loadMode0TextTestAtlas(t, c, mode0TextTestGlyph{'A', 7})
	c.BeginRenderFrame()
	c.Cls(0)
	if got := c.PrintV(c.InternStatic("B"), IntVal(0), IntVal(0), IntVal(7)); got != 0 {
		t.Fatalf("missing glyph return = %d, want unchanged x=0", got)
	}
	if c.mode0ObjectCount != 0 {
		t.Fatalf("missing glyph emitted %d objects", c.mode0ObjectCount)
	}
}

func TestMode0TextAtlasRejectsMalformedStaticData(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	validTile := make([]uint32, 16)
	tests := []struct {
		name  string
		keys  []uint16
		tiles []uint32
	}{
		{name: "length", keys: []uint16{Mode0TextGlyphKey('A', 7)}, tiles: validTile[:15]},
		{name: "duplicate", keys: []uint16{Mode0TextGlyphKey('A', 7), Mode0TextGlyphKey('A', 7)}, tiles: make([]uint32, 32)},
		{name: "unsorted", keys: []uint16{Mode0TextGlyphKey('B', 7), Mode0TextGlyphKey('A', 7)}, tiles: make([]uint32, 32)},
		{name: "transparent", keys: []uint16{Mode0TextGlyphKey('A', 0)}, tiles: validTile},
		{name: "control", keys: []uint16{Mode0TextGlyphKey('\n', 7)}, tiles: validTile},
		{name: "capacity", keys: make([]uint16, mode0TextLimit+1), tiles: make([]uint32, (mode0TextLimit+1)*16)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if c.LoadMode0TextAtlas(test.keys, test.tiles) {
				t.Fatal("malformed atlas accepted")
			}
		})
	}
}

func TestMode0TextAtlasTilesStayReservedFromSpriteUploads(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
	loadMode0TextTestAtlas(t, c, mode0TextTestGlyph{'A', 7})
	address := hw.OBJVRAM + uintptr(mode0TextTileBase*64)
	want := hw.ReadVRAM32(address)

	var spriteTile [64]byte
	for index := range spriteTile {
		spriteTile[index] = 0xff
	}
	if uploaded := c.uploadMode0ObjectTile(mode0TextTileBase, spriteTile[:]); uploaded != 0 {
		t.Fatalf("reserved text tile upload = %d bytes, want 0", uploaded)
	}
	if got := hw.ReadVRAM32(address); got != want {
		t.Fatalf("reserved atlas tile was overwritten: got %#08x, want %#08x", got, want)
	}
}
