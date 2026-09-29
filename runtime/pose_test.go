//go:build !mtile_mode0

package runtime

import "testing"

// poseFixture builds a two-part pose over a two-cell sheet: part 0 is tile 0
// at the origin, part 1 is tile 1 offset to (8,0) with its own FlipX bit
// already set. It mirrors spriteFixture's sheet-fill convention (byte index
// 1 of each 64-byte cell) so pixel comparisons against plain Spr calls are
// unambiguous.
func poseFixture() (*AssetImage, *AssetMemory) {
	sheet := make([]byte, 128) // two 8x8 cells
	sheet[1] = 7               // tile 0
	sheet[64+1] = 9            // tile 1
	image := &AssetImage{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		Sheet: sheet, MapW: 1, MapH: 1, Map: []uint16{0}, Flags: []byte{0, 0},
		PoseIndex: []PoseRange{{Start: 0, Count: 2, FlipOffsetX: 16, FlipOffsetY: 8}},
		PoseParts: []PosePart{
			{Tile: 0, X: 0, Y: 0},
			{Tile: 1, X: 8, Y: 0, Flags: PoseFlipX},
		},
	}
	ram := &AssetMemory{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		MapW: 1, MapH: 1,
		Sheet: make([]byte, 128), Map: make([]uint16, 1), Flags: make([]byte, 2),
	}
	return image, ram
}

func TestPSprMatchesSprSequenceAndFlip(t *testing.T) {
	image, ram := poseFixture()
	var pose, spr Context
	if !pose.Init(image, ram) {
		t.Fatal("init")
	}
	sprImg := *image
	sprImg.PoseIndex, sprImg.PoseParts = nil, nil
	sprRAM := &AssetMemory{SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		MapW: 1, MapH: 1,
		Sheet: make([]byte, 128), Map: make([]uint16, 1), Flags: make([]byte, 2)}
	if !spr.Init(&sprImg, sprRAM) {
		t.Fatal("spr init")
	}
	pose.Cls(0)
	spr.Cls(0)
	pose.PSpr(0, 10, 20, true, false)
	spr.Spr(0, 10+16-0-8, 20, 1, 1, true, false)  // part0 flipped within 16px bounds
	spr.Spr(1, 10+16-8-8, 20, 1, 1, false, false) // part1 FlipX xor true
	for i := range pose.framebuffer {
		if pose.framebuffer[i] != spr.framebuffer[i] {
			t.Fatalf("pixel %d: pose=%d spr=%d", i, pose.framebuffer[i], spr.framebuffer[i])
		}
	}
}

func TestPSprUnflippedMatchesPlainSprSequence(t *testing.T) {
	image, ram := poseFixture()
	var pose, spr Context
	if !pose.Init(image, ram) {
		t.Fatal("init")
	}
	sprImg := *image
	sprImg.PoseIndex, sprImg.PoseParts = nil, nil
	sprRAM := &AssetMemory{SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		MapW: 1, MapH: 1,
		Sheet: make([]byte, 128), Map: make([]uint16, 1), Flags: make([]byte, 2)}
	if !spr.Init(&sprImg, sprRAM) {
		t.Fatal("spr init")
	}
	pose.Cls(0)
	spr.Cls(0)
	pose.PSpr(0, 10, 20, false, false)
	spr.Spr(0, 10, 20, 1, 1, false, false)
	spr.Spr(1, 18, 20, 1, 1, true, false)
	for i := range pose.framebuffer {
		if pose.framebuffer[i] != spr.framebuffer[i] {
			t.Fatalf("pixel %d: pose=%d spr=%d", i, pose.framebuffer[i], spr.framebuffer[i])
		}
	}
}

func TestPSprInvalidIDIsNoOp(t *testing.T) {
	image, ram := poseFixture()
	var c Context
	if !c.Init(image, ram) {
		t.Fatal("init")
	}
	c.Cls(3)
	c.PSpr(99, 0, 0, false, false)
	for y := 0; y < 8; y++ {
		for x := 0; x < 16; x++ {
			if got := c.Pget(x, y); got != 3 {
				t.Fatalf("out-of-range PSpr id changed pixel (%d,%d) to %d", x, y, got)
			}
		}
	}
}

func TestPSprEmptyPoseIsNoOp(t *testing.T) {
	image, ram := poseFixture()
	image.PoseIndex = append(image.PoseIndex, PoseRange{Start: 2, Count: 0})
	var c Context
	if !c.Init(image, ram) {
		t.Fatal("init")
	}
	c.Cls(3)
	c.PSpr(1, 0, 0, false, false)
	for y := 0; y < 8; y++ {
		for x := 0; x < 16; x++ {
			if got := c.Pget(x, y); got != 3 {
				t.Fatalf("empty pose changed pixel (%d,%d) to %d", x, y, got)
			}
		}
	}
}

func TestPSprSeesSsetUpdate(t *testing.T) {
	image, ram := poseFixture()
	var c Context
	if !c.Init(image, ram) {
		t.Fatal("init")
	}
	c.Cls(0)
	c.Sset(1, 0, 42) // tile 0's opaque pixel, now via RAM sheet
	c.PSpr(0, 0, 0, false, false)
	if got := c.Pget(1, 0); got != 42 {
		t.Fatalf("PSpr did not see Sset update: got %d, want 42", got)
	}
}

func TestInitRejectsPoseIndexOutOfRangeSpan(t *testing.T) {
	image, ram := poseFixture()
	// PoseParts has 2 entries; a span of Start:0 Count:3 reaches past the end.
	image.PoseIndex = []PoseRange{{Start: 0, Count: 3}}
	var c Context
	if c.Init(image, ram) {
		t.Fatal("Init accepted a PoseIndex span reaching past the end of PoseParts")
	}
}

func TestPSprNilContextIsNoOp(t *testing.T) {
	var c *Context
	c.PSpr(0, 0, 0, false, false) // must not panic
}

func TestPSprBeforeInitIsNoOp(t *testing.T) {
	var c Context
	c.PSpr(0, 0, 0, false, false) // must not panic; c.cart is nil
}

func TestPSprReadsObjectSheetNotSpritesheet(t *testing.T) {
	sheet := make([]byte, 128)
	sheet[1] = 3
	sheet[64+1] = 4
	objCell := make([]byte, 64)
	objCell[1] = 11
	image := &AssetImage{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		Sheet: sheet, MapW: 1, MapH: 1, Map: []uint16{0}, Flags: []byte{0, 0},
		PoseIndex: []PoseRange{{Start: 0, Count: 1}},
		PoseParts: []PosePart{{Tile: 0, X: 0, Y: 0}},
		ObjSheetW: 8, ObjSheetH: 8, ObjSpriteCols: 1, ObjSpriteRows: 1, ObjSpriteN: 1,
		ObjSheet: objCell, ObjSheetIndex: []uint32{1},
	}
	ram := &AssetMemory{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		MapW: 1, MapH: 1,
		Sheet: make([]byte, 128), Map: make([]uint16, 1), Flags: make([]byte, 2),
	}
	var pose Context
	if !pose.Init(image, ram) {
		t.Fatal("init")
	}
	pose.Cls(0)
	pose.PSpr(0, 0, 0, false, false)
	if got := pose.framebuffer[1]; got != 11 {
		t.Fatalf("object-sheet pixel = %d, want 11 (spritesheet has 3 at the same cell)", got)
	}
}

func TestPSprUsesFullUint16TileAndWidePoseRanges(t *testing.T) {
	image, ram := poseFixture()
	image.ObjSheetW, image.ObjSheetH = 2048, 2048
	image.ObjSpriteCols, image.ObjSpriteRows, image.ObjSpriteN = 256, 256, 65536
	image.ObjSheet = make([]byte, 65536*64)
	image.ObjSheetIndex = make([]uint32, 65536)
	image.ObjSheetIndex[65535] = 65536
	image.ObjSheet[65535*64] = 11
	image.PoseParts = make([]PosePart, 65537)
	image.PoseParts[65536] = PosePart{Tile: 65535}
	image.PoseIndex = []PoseRange{{Start: 65536, Count: 1}}
	var c Context
	if !c.Init(image, ram) {
		t.Fatal("wide pose and full object bank rejected")
	}
	c.Cls(0)
	c.PSpr(0, 0, 0, false, false)
	if got := c.framebuffer[0]; got != 11 {
		t.Fatalf("last bank tile pixel=%d, want11", got)
	}
}
