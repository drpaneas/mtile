//go:build !mtile_mode0

package runtime

import "testing"

func TestClsFillsEveryPixel(t *testing.T) {
	var c Context
	c.Pset(0, 0, 7)
	c.Pset(17, 23, 8)
	c.Pset(ScreenWidth-1, ScreenHeight-1, 9)
	c.Cls(3)
	for y := 0; y < ScreenHeight; y++ {
		for x := 0; x < ScreenWidth; x++ {
			if got := c.Pget(x, y); got != 3 {
				t.Fatalf("Cls left pixel (%d,%d) = %d, want 3", x, y, got)
			}
		}
	}
}

func TestDrawingUsesNativeFramebuffer(t *testing.T) {
	var c Context
	c.Cls(3)
	if c.Pget(0, 0) != 3 || c.Pget(ScreenWidth-1, ScreenHeight-1) != 3 {
		t.Fatal("Cls did not cover the full 240x160 framebuffer")
	}
	c.Pal(3, 9, 0)
	c.Pset(5, 6, 3)
	if got := c.Pget(5, 6); got != 9 {
		t.Fatalf("Pset remap = %d, want 9", got)
	}
	c.Rectfill(-2, -1, 1, 1, 7)
	for y := 0; y <= 1; y++ {
		for x := 0; x <= 1; x++ {
			if c.Pget(x, y) != 7 {
				t.Fatalf("Rectfill pixel (%d,%d) = %d, want 7", x, y, c.Pget(x, y))
			}
		}
	}
	if c.Pget(2, 0) != 3 {
		t.Fatalf("Rectfill escaped clip: pixel (2,0) = %d", c.Pget(2, 0))
	}
}

func TestRectfillUnalignedAndWordSpans(t *testing.T) {
	var c Context
	c.Cls(1)
	c.Rectfill(1, 10, 6, 10, 9)
	if c.Pget(0, 10) != 1 || c.Pget(7, 10) != 1 {
		t.Fatal("unaligned Rectfill wrote outside [1,6]")
	}
	for x := 1; x <= 6; x++ {
		if got := c.Pget(x, 10); got != 9 {
			t.Fatalf("unaligned Rectfill pixel (%d,10) = %d, want 9", x, got)
		}
	}
	c.Rectfill(2, 12, 17, 12, 4)
	if c.Pget(1, 12) != 1 || c.Pget(18, 12) != 1 {
		t.Fatal("word-span Rectfill wrote outside [2,17]")
	}
	for x := 2; x <= 17; x++ {
		if got := c.Pget(x, 12); got != 4 {
			t.Fatalf("word-span Rectfill pixel (%d,12) = %d, want 4", x, got)
		}
	}
	c.Rectfill(239, 0, 239, 0, 7)
	if c.Pget(238, 0) != 1 {
		t.Fatal("single-pixel Rectfill wrote left of the last column")
	}
	if c.Pget(239, 0) != 7 {
		t.Fatal("single-pixel Rectfill missed the last column")
	}
}

func TestDrawingPersistsUntilExplicitClear(t *testing.T) {
	var c Context
	c.Pset(10, 10, 255)
	if c.Pget(10, 10) != 255 {
		t.Fatal("Pset(255) did not preserve the full native index range")
	}
	c.Cls(0)
	if c.Pget(10, 10) != 0 {
		t.Fatal("Cls did not explicitly clear the previous pixel")
	}
}

func TestPsetZeroIsOpaqueBlack(t *testing.T) {
	t.Parallel()
	var c Context
	c.Pset(4, 5, 7)
	c.Pset(4, 5, 0)
	if c.Pget(4, 5) != 0 {
		t.Fatal("Pset(0) did not write opaque black over a nonzero pixel")
	}
}

// TestLineMatchesIndependentBresenhamVariant pins Line's diagonal stepping
// against a second, differently coded Bresenham formulation (a doubled,
// purely additive error term with an explicit d<0 branch) rather than Line's
// own dx/2-initialized accumulator. The two formulations do not agree on
// every possible line - they break ties at exact half-integer errors
// differently - so this only asserts agreement for cases already confirmed
// to coincide; its purpose is regression coverage for the stepping loop
// itself, which no other test in this package exercises for a genuinely
// diagonal segment (dx != dy, both nonzero). runtime/golden_test.go's mixed
// scene also draws a diagonal line, but only as a composition smoke test.
func TestLineMatchesIndependentBresenhamVariant(t *testing.T) {
	cases := []struct{ x0, y0, x1, y1 int }{
		{0, 0, 10, 4},        // shallow, right-down
		{0, 0, 4, 10},        // steep, right-down
		{2, 3, 9, 3},         // horizontal
		{5, 1, 5, 9},         // vertical
		{10, 4, 0, 0},        // shallow, left-up (reverse of the first case)
		{0, 10, 10, 0},       // diagonal, right-up
		{0, 0, 7, 3},         // shallow, odd dx
		{0, 0, 3, 7},         // steep, odd dy
		{100, 100, 110, 110}, // dx == dy exactly
	}
	for _, tc := range cases {
		var c Context
		c.Cls(0)
		c.Line(tc.x0, tc.y0, tc.x1, tc.y1, 7)
		got := map[[2]int]bool{}
		for y := 0; y < ScreenHeight; y++ {
			for x := 0; x < ScreenWidth; x++ {
				if c.Pget(x, y) == 7 {
					got[[2]int{x, y}] = true
				}
			}
		}
		want := doubledErrorBresenhamPixels(tc.x0, tc.y0, tc.x1, tc.y1)
		if len(got) != len(want) {
			t.Fatalf("Line(%d,%d,%d,%d): drew %d pixels, want %d", tc.x0, tc.y0, tc.x1, tc.y1, len(got), len(want))
		}
		for p := range want {
			if !got[p] {
				t.Fatalf("Line(%d,%d,%d,%d): missing pixel %v", tc.x0, tc.y0, tc.x1, tc.y1, p)
			}
		}
	}
}

// doubledErrorBresenhamPixels is a textbook Bresenham formulation using a
// doubled, purely additive error term and an explicit d<0 branch, instead of
// Line's dx/2-initialized subtract-and-correct accumulator. It is
// deliberately different code so it cannot silently share a bug with the
// function under test.
func doubledErrorBresenhamPixels(x0, y0, x1, y1 int) map[[2]int]bool {
	pts := map[[2]int]bool{}
	dx := x1 - x0
	adx := dx
	if adx < 0 {
		adx = -adx
	}
	dy := y1 - y0
	ady := dy
	if ady < 0 {
		ady = -ady
	}
	stepX := 1
	if dx < 0 {
		stepX = -1
	}
	stepY := 1
	if dy < 0 {
		stepY = -1
	}
	x, y := x0, y0
	pts[[2]int{x, y}] = true
	if adx > ady {
		d := 2*ady - adx
		incrE := 2 * ady
		incrNE := 2 * (ady - adx)
		for x != x1 {
			x += stepX
			if d < 0 {
				d += incrE
			} else {
				d += incrNE
				y += stepY
			}
			pts[[2]int{x, y}] = true
		}
		return pts
	}
	d := 2*adx - ady
	incrE := 2 * adx
	incrNE := 2 * (adx - ady)
	for y != y1 {
		y += stepY
		if d < 0 {
			d += incrE
		} else {
			d += incrNE
			x += stepX
		}
		pts[[2]int{x, y}] = true
	}
	return pts
}
