package runtime

import "testing"

func fixtureAssets() (*AssetImage, *AssetMemory) {
	sheet := []byte{
		0x12, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0x34,
	}
	mapCells := []uint16{0x0056}
	flags := []byte{0x78}
	img := &AssetImage{
		SheetW:     8,
		SheetH:     8,
		SpriteCols: 1,
		SpriteRows: 1,
		MapW:       1,
		MapH:       1,
		SpriteN:    1,
		Sheet:      sheet,
		Map:        mapCells,
		Flags:      flags,
	}
	ramSheet := make([]byte, len(sheet))
	ramMap := make([]uint16, len(mapCells))
	ramFlags := make([]byte, len(flags))
	ram := &AssetMemory{
		SheetW:     8,
		SheetH:     8,
		SpriteCols: 1,
		SpriteRows: 1,
		MapW:       1,
		MapH:       1,
		SpriteN:    1,
		Sheet:      ramSheet,
		Map:        ramMap,
		Flags:      ramFlags,
	}
	return img, ram
}

func TestContext_Init_successCopiesROM(t *testing.T) {
	t.Parallel()
	img, ram := fixtureAssets()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	if c.cart != ram {
		t.Fatal("cart pointer not set")
	}
	if ram.Sheet[0] != 0x12 || ram.Sheet[63] != 0x34 {
		t.Fatalf("sheet copy = %02x ... %02x", ram.Sheet[0], ram.Sheet[63])
	}
	if ram.Map[0] != 0x0056 {
		t.Fatalf("map copy = %04x", ram.Map[0])
	}
	if ram.Flags[0] != 0x78 {
		t.Fatalf("flag copy = %02x", ram.Flags[0])
	}
}

func TestContext_FgetByte(t *testing.T) {
	t.Parallel()
	img, ram := fixtureAssets()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	if got := c.FgetByte(0); got != 0x78 {
		t.Fatalf("FgetByte(0) = %d, want 0x78", got)
	}
	if c.FgetByte(-1) != 0 || c.FgetByte(99) != 0 {
		t.Fatal("out-of-range FgetByte must be 0")
	}
}

func TestContext_FsetByte(t *testing.T) {
	t.Parallel()
	img, ram := fixtureAssets()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.FsetByte(0, 7)
	if got := c.FgetByte(0); got != 7 {
		t.Fatalf("FsetByte(0, 7) then FgetByte = %d, want 7", got)
	}
	c.FsetByte(-1, 1)
	c.FsetByte(99, 1)
	if c.FgetByte(0) != 7 {
		t.Fatal("out-of-range FsetByte mutated a live sprite")
	}
}

func TestContext_Init_mismatchLeavesCartNilAndNoCopy(t *testing.T) {
	t.Parallel()
	img, ram := fixtureAssets()
	ram.Sheet[0] = 0xAA
	img.SheetW = 9
	var c Context
	if c.Init(img, ram) {
		t.Fatal("Init returned true")
	}
	if c.cart != nil {
		t.Fatal("cart still set")
	}
	if ram.Sheet[0] != 0xAA {
		t.Fatalf("sheet was copied; got %02x", ram.Sheet[0])
	}
}

func TestContext_Init_spriteColsMismatch(t *testing.T) {
	t.Parallel()
	img, ram := fixtureAssets()
	img.SpriteCols = 2
	var c Context
	if c.Init(img, ram) {
		t.Fatal("Init returned true")
	}
	if c.cart != nil {
		t.Fatal("cart still set")
	}
}

func TestContext_Init_failedReinitClearsCart(t *testing.T) {
	t.Parallel()
	img, ram := fixtureAssets()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("first Init returned false")
	}
	if c.cart == nil {
		t.Fatal("cart not set after success")
	}
	bad := *img
	bad.SheetW = 16
	if c.Init(&bad, ram) {
		t.Fatal("re-init returned true")
	}
	if c.cart != nil {
		t.Fatal("failed re-init left cart set")
	}
	if ram.Sheet[0] != 0x12 || ram.Sheet[63] != 0x34 {
		t.Fatal("failed re-init copied over the previous RAM")
	}
}

func TestContext_Init_nilImage(t *testing.T) {
	t.Parallel()
	_, ram := fixtureAssets()
	var c Context
	c.cart = ram
	if c.Init(nil, ram) {
		t.Fatal("Init returned true")
	}
	if c.cart != nil {
		t.Fatal("cart still set")
	}
}

func TestSsetDoesNotAliasMap(t *testing.T) {
	t.Parallel()
	img, ram := fixtureAssets()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	before := c.Mget(0, 0)
	c.Sset(0, 0, 9)
	if c.Mget(0, 0) != before {
		t.Fatal("Sset changed mget")
	}
	c.Mset(0, 0, 0)
	if c.Sget(0, 0) != 9 {
		t.Fatal("Mset changed sget")
	}
}

func TestReadOnlySheetUsesROMWithoutRAMCopy(t *testing.T) {
	t.Parallel()
	img, ram := fixtureAssets()
	img.SheetReadOnly = true
	img.SheetIndex = []uint16{1}
	ram.SheetReadOnly = true
	ram.SheetIndex = make([]uint16, 1)
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("read-only sheet Init returned false")
	}
	if len(ram.Sheet) != len(img.Sheet) || ram.Sheet[0] != img.Sheet[0] || ram.SheetIndex[0] != 1 {
		t.Fatal("read-only compact sheet was not copied into EWRAM")
	}
	if got := c.Sget(0, 0); got != 0x12 {
		t.Fatalf("Sget from read-only sheet = %02x, want 12", got)
	}
	c.Sset(0, 0, 9)
	if got := c.Sget(0, 0); got != 0x12 {
		t.Fatalf("Sset mutated read-only sheet: %02x", got)
	}
}

func TestReadOnlySheetRejectsWrongOptionalRAMLength(t *testing.T) {
	t.Parallel()
	img, ram := fixtureAssets()
	img.SheetReadOnly = true
	img.SheetIndex = []uint16{1}
	ram.SheetReadOnly = true
	ram.Sheet = make([]byte, len(img.Sheet)-1)
	ram.SheetIndex = make([]uint16, 1)
	var c Context
	if c.Init(img, ram) {
		t.Fatal("read-only sheet accepted a nonempty wrong-sized RAM slice")
	}
}

func TestSsetEightBitGoldenAnd255(t *testing.T) {
	t.Parallel()
	img, ram := fixtureAssets()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.Sset(0, 0, 5)
	c.Sset(1, 0, 3)
	if ram.Sheet[0] != 5 || ram.Sheet[1] != 3 {
		t.Fatalf("8bpp golden = %d,%d want 5,3", ram.Sheet[0], ram.Sheet[1])
	}
	c.Sset(0, 0, 255)
	if ram.Sheet[0] != 255 {
		t.Fatal("Sset of 255 was not stored")
	}
	c.Sset(-1, 0, 4)
	c.Sset(0, -1, 4)
	c.Sset(8, 0, 4)
	c.Sset(0, 8, 4)
	c.Sset(0, 0, 256)
	if ram.Sheet[0] != 255 {
		t.Fatal("out-of-range Sset mutated the sheet")
	}
}

func TestInitOverflowingSheetProduct(t *testing.T) {
	t.Parallel()
	img, ram := fixtureAssets()
	img.SheetW = 1 << 30
	img.SheetH = 1 << 33
	ram.SheetW = img.SheetW
	ram.SheetH = img.SheetH
	img.SpriteCols = img.SheetW / 8
	img.SpriteRows = img.SheetH / 8
	ram.SpriteCols = img.SpriteCols
	ram.SpriteRows = img.SpriteRows
	ram.Sheet[0] = 0xAA
	var c Context
	if c.Init(img, ram) {
		t.Fatal("Init accepted an overflowing sheet product")
	}
	if c.cart != nil {
		t.Fatal("cart still set")
	}
	if ram.Sheet[0] != 0xAA {
		t.Fatal("overflow Init copied into RAM")
	}
}

func TestSpriteN65536Boundary(t *testing.T) {
	img, ram := cellGridAssets(256, 256, 1, 1)
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("SpriteN 65536 was rejected")
	}
	c.Mset(0, 0, 65535)
	if c.Mget(0, 0) != 65535 {
		t.Fatal("map cell 65535 was not valid when SpriteN is 65536")
	}
	c.Mset(0, 0, 65536)
	if c.Mget(0, 0) != 65535 {
		t.Fatal("Mset accepted id == SpriteN")
	}
}

func TestSpriteN65537Rejected(t *testing.T) {
	img, ram := cellGridAssets(65537, 1, 1, 1)
	ram.Sheet[0] = 0xAA
	var c Context
	if c.Init(img, ram) {
		t.Fatal("SpriteN 65537 was accepted")
	}
	if c.cart != nil {
		t.Fatal("cart still set")
	}
	if ram.Sheet[0] != 0xAA {
		t.Fatal("rejected SpriteN copied into RAM")
	}

	small, smallRAM := fixtureAssets()
	small.SpriteN = 65537
	smallRAM.SpriteN = 65537
	if c.Init(small, smallRAM) {
		t.Fatal("claimed SpriteN 65537 with derived 1 was accepted")
	}
}

func Test128x64MapUsesCompleteMutableRAM(t *testing.T) {
	img, ram := cellGridAssets(2, 1, 128, 64)
	img.Map[128*64-1] = 1
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("128x64 map Init returned false")
	}
	if ram.MapStreaming {
		t.Fatal("128x64 map unexpectedly selected streaming mode")
	}
	if len(ram.Map) != 128*64 {
		t.Fatalf("map RAM length = %d, want %d", len(ram.Map), 128*64)
	}
	c.Mset(127, 63, 0)
	if got := c.Mget(127, 63); got != 0 {
		t.Fatalf("Mset on the last 128x64 cell = %d, want 0", got)
	}
}

func cellGridAssets(cols, rows, mapW, mapH int) (*AssetImage, *AssetMemory) {
	sheetW := cols * 8
	sheetH := rows * 8
	spriteN := cols * rows
	mapN := mapW * mapH
	img := &AssetImage{
		SheetW:     sheetW,
		SheetH:     sheetH,
		SpriteCols: cols,
		SpriteRows: rows,
		MapW:       mapW,
		MapH:       mapH,
		SpriteN:    spriteN,
		Sheet:      make([]byte, sheetW*sheetH),
		Map:        make([]uint16, mapN),
		Flags:      make([]byte, spriteN),
	}
	ram := &AssetMemory{
		SheetW:     sheetW,
		SheetH:     sheetH,
		SpriteCols: cols,
		SpriteRows: rows,
		MapW:       mapW,
		MapH:       mapH,
		SpriteN:    spriteN,
		Sheet:      make([]byte, sheetW*sheetH),
		Map:        make([]uint16, mapN),
		Flags:      make([]byte, spriteN),
	}
	return img, ram
}
