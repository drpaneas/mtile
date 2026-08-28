package runtime

import "testing"

func TestAssetAccessorsAndSpriteTransparency(t *testing.T) {
	img, ram := spriteFixture()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	if c.Sget(0, 0) != 0 || c.Sget(1, 0) != 7 {
		t.Fatal("Sget did not read native sheet")
	}
	c.Sset(2, 0, 8)
	if c.Sget(2, 0) != 8 {
		t.Fatal("Sset did not write native sheet")
	}
	if c.Mget(0, 0) != 0 || !c.Fget(0, 0) || c.Fget(0, 1) || !c.Fget(0, 2) {
		t.Fatal("map/flag accessor read failed")
	}
	c.Mset(0, 0, 1) // invalid: only sprite 0 exists
	if c.Mget(0, 0) != 0 {
		t.Fatal("Mset accepted an out-of-range sprite ID")
	}
	c.Fset(0, 1, true)
	if !c.Fget(0, 1) {
		t.Fatal("Fset did not write flag bit 1")
	}
	c.Fset(0, 0, false)
	if c.Fget(0, 0) {
		t.Fatal("Fset did not clear flag bit 0")
	}

	c.Pset(0, 0, 3)
	c.Spr(0, 0, 0, 1, 1, false, false)
	if c.Pget(0, 0) != 3 {
		t.Fatal("default transparent source zero overwrote framebuffer")
	}
	if c.Pget(1, 0) != 7 {
		t.Fatalf("opaque sprite pixel = %d, want 7", c.Pget(1, 0))
	}
	c.PaltColor(0, false)
	c.Spr(0, 0, 0, 1, 1, false, false)
	if c.Pget(0, 0) != 0 {
		t.Fatal("palt(0,false) did not make source zero opaque")
	}
}

func compactSpriteFixture() (*AssetImage, *AssetMemory) {
	denseImg, _ := spriteFixture()
	index := []uint16{1}
	img := &AssetImage{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		SheetReadOnly: true, Sheet: append([]byte(nil), denseImg.Sheet...), SheetIndex: index,
		MapW: 1, MapH: 1, Map: []uint16{0}, Flags: []byte{0},
	}
	ram := &AssetMemory{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		SheetReadOnly: true, Sheet: make([]byte, 64), SheetIndex: make([]uint16, 1),
		MapW: 1, MapH: 1, Map: make([]uint16, 1), Flags: make([]byte, 1),
	}
	return img, ram
}

func TestCompactSheetSprAndMapMatchDense(t *testing.T) {
	denseImg, denseRAM := spriteFixture()
	compactImg, compactRAM := compactSpriteFixture()
	var dense, compact Context
	if !dense.Init(denseImg, denseRAM) || !compact.Init(compactImg, compactRAM) {
		t.Fatal("fixture Init failed")
	}
	dense.Cls(3)
	compact.Cls(3)
	dense.Spr(0, 8, 8, 1, 1, false, false)
	compact.Spr(0, 8, 8, 1, 1, false, false)
	dense.PaltColor(0, false)
	compact.PaltColor(0, false)
	dense.Spr(0, 24, 8, 1, 1, true, true)
	compact.Spr(0, 24, 8, 1, 1, true, true)
	// map() reserves ID 0 as empty, so compare with direct framebuffer
	// equality after the normal and flipped sprite paths above.
	for i := range dense.framebuffer {
		if dense.framebuffer[i] != compact.framebuffer[i] {
			t.Fatalf("compact framebuffer mismatch at byte %d: %d != %d", i, compact.framebuffer[i], dense.framebuffer[i])
		}
	}
}

func TestCompactSheetEmptyLogicalCellIsTransparent(t *testing.T) {
	img, ram := compactSpriteFixture()
	img.SheetW, img.SpriteCols, img.SpriteN = 16, 2, 2
	img.SheetIndex = []uint16{1, 0}
	img.Flags = make([]byte, 2)
	ram.SheetW, ram.SpriteCols, ram.SpriteN = 16, 2, 2
	ram.SheetIndex = make([]uint16, 2)
	ram.Flags = make([]byte, 2)
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("compact Init failed")
	}
	c.Cls(9)
	c.Spr(1, 0, 0, 1, 1, false, false)
	if got := c.Pget(0, 0); got != 9 {
		t.Fatalf("transparent empty logical cell drew color %d", got)
	}
	c.PaltColor(0, false)
	c.Spr(1, 0, 0, 1, 1, false, false)
	if got := c.Pget(0, 0); got != 0 {
		t.Fatalf("opaque empty logical cell drew color %d, want 0", got)
	}
	if got := c.Sget(8, 0); got != 0 {
		t.Fatalf("Sget on empty logical cell = %d", got)
	}
}

func TestPalRemapDoesNotBypassSourceTransparency(t *testing.T) {
	img, ram := spriteFixture()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.Pset(0, 0, 3)
	c.Pal(0, 8, 0)
	c.Spr(0, 0, 0, 1, 1, false, false)
	if c.Pget(0, 0) != 3 {
		t.Fatal("pal(0,8) turned transparent source zero into color 8")
	}
	if c.Pget(1, 0) != 7 {
		t.Fatalf("opaque sprite pixel = %d, want 7", c.Pget(1, 0))
	}
}

func TestMapSkipsSpriteIDsAtOrAboveSpriteN(t *testing.T) {
	img, ram := spriteFixture()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.cart.Map[0] = 1
	c.Pset(0, 0, 3)
	c.Map(0, 0, 0, 0, 1, 1, 0)
	if c.Pget(0, 0) != 3 {
		t.Fatal("map() drew a cell with id >= SpriteN")
	}
	if c.Pget(1, 0) != 0 {
		t.Fatal("map() drew opaque pixels from an invalid sprite id")
	}
}

func TestSprNoOpWhenSpriteColsIsZero(t *testing.T) {
	sheet := make([]byte, 7*8)
	img := &AssetImage{
		SheetW: 7, SheetH: 8, SpriteCols: 0, SpriteRows: 1,
		MapW: 0, MapH: 0, SpriteN: 0,
		Sheet: sheet, Map: []uint16{}, Flags: []byte{},
	}
	ram := &AssetMemory{
		SheetW: 7, SheetH: 8, SpriteCols: 0, SpriteRows: 1,
		MapW: 0, MapH: 0, SpriteN: 0,
		Sheet: make([]byte, len(sheet)), Map: []uint16{}, Flags: []byte{},
	}
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false for a sub-8-wide sheet")
	}
	c.Pset(0, 0, 9)
	c.Spr(0, 0, 0, 1, 1, false, false)
	if c.Pget(0, 0) != 9 {
		t.Fatal("spr with SpriteCols==0 overwrote the framebuffer")
	}
}

func TestPackedHasZeroByte(t *testing.T) {
	if packedHasZeroByte(0x04030201) {
		t.Fatal("0x04030201 has no zero bytes")
	}
	if !packedHasZeroByte(0x04030001) {
		t.Fatal("0x04030001 has a zero byte")
	}
	if !packedHasZeroByte(0) {
		t.Fatal("0 has four zero bytes")
	}
}

func TestSprRemapsOpaqueSource(t *testing.T) {
	img, ram := spriteFixture()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.Pal(7, 9, 0)
	c.Spr(0, 0, 0, 1, 1, false, false)
	if c.Pget(1, 0) != 9 {
		t.Fatalf("pal(7,9) left opaque pixel = %d, want 9", c.Pget(1, 0))
	}
	if c.Pget(0, 0) != 0 {
		t.Fatal("pal remap drew a transparent source zero")
	}
}

func TestSprOpaqueAlignedAndUnalignedRows(t *testing.T) {
	sheet := make([]byte, 64)
	for i := range sheet {
		sheet[i] = 5
	}
	img := &AssetImage{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1,
		MapW: 0, MapH: 0, SpriteN: 1,
		Sheet: sheet, Map: []uint16{}, Flags: []byte{0},
	}
	ram := &AssetMemory{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1,
		MapW: 0, MapH: 0, SpriteN: 1,
		Sheet: make([]byte, 64), Map: []uint16{}, Flags: []byte{0},
	}
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.Cls(1)
	c.Spr(0, 8, 8, 1, 1, false, false)
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			if got := c.Pget(8+col, 8+row); got != 5 {
				t.Fatalf("aligned spr pixel (%d,%d) = %d, want 5", 8+col, 8+row, got)
			}
		}
	}
	c.Cls(1)
	c.Spr(0, 9, 8, 1, 1, false, false)
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			if got := c.Pget(9+col, 8+row); got != 5 {
				t.Fatalf("unaligned spr pixel (%d,%d) = %d, want 5", 9+col, 8+row, got)
			}
		}
	}
	if c.Pget(8, 8) != 1 {
		t.Fatal("unaligned spr wrote left of its destination")
	}
}

func TestPaltNonZeroSkipsOpaqueSource(t *testing.T) {
	img, ram := spriteFixture()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.Pset(1, 0, 3)
	c.PaltColor(7, true)
	c.Spr(0, 0, 0, 1, 1, false, false)
	if c.Pget(1, 0) != 3 {
		t.Fatal("palt(7,true) still drew source color 7")
	}
}

func twoByTwoSheet(cellColor [4]byte) (*AssetImage, *AssetMemory) {
	sheet := make([]byte, 16*16)
	for row := 0; row < 16; row++ {
		for col := 0; col < 16; col++ {
			sheet[row*16+col] = cellColor[(row/8)*2+col/8]
		}
	}
	img := &AssetImage{
		SheetW: 16, SheetH: 16, SpriteCols: 2, SpriteRows: 2, SpriteN: 4,
		Sheet: sheet, Map: []uint16{}, Flags: []byte{0, 0, 0, 0},
	}
	ram := &AssetMemory{
		SheetW: 16, SheetH: 16, SpriteCols: 2, SpriteRows: 2, SpriteN: 4,
		Sheet: append([]byte(nil), sheet...), Map: []uint16{}, Flags: []byte{0, 0, 0, 0},
	}
	return img, ram
}

func TestSpr2x2UnflippedIsSheetRectangle(t *testing.T) {
	img, ram := twoByTwoSheet([4]byte{10, 11, 12, 13})
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.Cls(9)
	c.Spr(0, 24, 32, 2, 2, false, false)
	for row := 0; row < 16; row++ {
		for col := 0; col < 16; col++ {
			want := img.Sheet[row*16+col]
			if got := c.Pget(24+col, 32+row); got != want {
				t.Fatalf("spr 2x2 pixel (%d,%d) = %d, want %d", 24+col, 32+row, got, want)
			}
		}
	}
	if c.Pget(23, 32) != 9 || c.Pget(40, 32) != 9 || c.Pget(24, 31) != 9 || c.Pget(24, 48) != 9 {
		t.Fatal("spr 2x2 wrote outside the 16x16 destination")
	}
}

func TestSpr2x1DoesNotWrapToNextSheetRow(t *testing.T) {
	img, ram := twoByTwoSheet([4]byte{10, 11, 12, 13})
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.Cls(9)
	c.Spr(1, 0, 0, 2, 1, false, false)
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			if got := c.Pget(col, row); got != 11 {
				t.Fatalf("edge spr pixel (%d,%d) = %d, want 11 from sprite 1", col, row, got)
			}
		}
		for col := 8; col < 16; col++ {
			if got := c.Pget(col, row); got != 9 {
				t.Fatalf("edge spr wrapped into sprite 2 at (%d,%d) = %d", col, row, got)
			}
		}
	}
}

func TestSprFlipX8x8ReversesRows(t *testing.T) {
	sheet := make([]byte, 64)
	for i := range sheet {
		sheet[i] = byte(i + 1)
	}
	img := &AssetImage{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		Sheet: sheet, Map: []uint16{}, Flags: []byte{0},
	}
	ram := &AssetMemory{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		Sheet: append([]byte(nil), sheet...), Map: []uint16{}, Flags: []byte{0},
	}
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.Cls(9)
	c.Spr(0, 4, 6, 1, 1, true, false)
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			want := sheet[row*8+(7-col)]
			if got := c.Pget(4+col, 6+row); got != want {
				t.Fatalf("flipX pixel (%d,%d) = %d, want %d", 4+col, 6+row, got, want)
			}
		}
	}
}

func TestSprFlipY8x8ReversesColumns(t *testing.T) {
	sheet := make([]byte, 64)
	for i := range sheet {
		sheet[i] = byte(i + 1)
	}
	img := &AssetImage{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		Sheet: sheet, Map: []uint16{}, Flags: []byte{0},
	}
	ram := &AssetMemory{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		Sheet: append([]byte(nil), sheet...), Map: []uint16{}, Flags: []byte{0},
	}
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.Cls(9)
	c.Spr(0, 0, 0, 1, 1, false, true)
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			want := sheet[(7-row)*8+col]
			if got := c.Pget(col, row); got != want {
				t.Fatalf("flipY pixel (%d,%d) = %d, want %d", col, row, got, want)
			}
		}
	}
}

func TestSprFlipX2x1FlipsWholeRectangle(t *testing.T) {
	img, ram := twoByTwoSheet([4]byte{10, 11, 12, 13})
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.Cls(9)
	c.Spr(0, 0, 0, 2, 1, true, false)
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			if got := c.Pget(col, row); got != 11 {
				t.Fatalf("flipped 2x1 left pixel (%d,%d) = %d, want 11 (right cell)", col, row, got)
			}
			if got := c.Pget(8+col, row); got != 10 {
				t.Fatalf("flipped 2x1 right pixel (%d,%d) = %d, want 10 (left cell)", 8+col, row, got)
			}
		}
	}
}

func spriteFixture() (*AssetImage, *AssetMemory) {
	sheet := make([]byte, 64)
	sheet[1] = 7
	image := &AssetImage{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1,
		MapW: 1, MapH: 1, SpriteN: 1,
		Sheet: sheet, Map: []uint16{0}, Flags: []byte{0x55},
	}
	ram := &AssetMemory{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1,
		MapW: 1, MapH: 1, SpriteN: 1,
		Sheet: make([]byte, 64), Map: make([]uint16, 1), Flags: make([]byte, 1),
	}
	return image, ram
}
