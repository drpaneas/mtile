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
		PoseIndex: []PoseRange{{Start: 0, Count: 2}},
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
	spr.Spr(0, 10+(-0-8), 20, 1, 1, true, false)  // part0 flipped in X
	spr.Spr(1, 10+(-8-8), 20, 1, 1, false, false) // part1 FlipX xor true
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

func TestPSprNilContextIsNoOp(t *testing.T) {
	var c *Context
	c.PSpr(0, 0, 0, false, false) // must not panic
}

func TestPSprBeforeInitIsNoOp(t *testing.T) {
	var c Context
	c.PSpr(0, 0, 0, false, false) // must not panic; c.cart is nil
}
