//go:build !mtile_mode0

package runtime

import "testing"

// TestGoldenFramebufferScene renders one deterministic native scene through
// the public runtime operations and compares every one of the 38,400 logical
// pixels with an independent reference renderer. Keeping the reference here
// deliberately avoids calling Context drawing helpers, so a regression in
// Rectfill's word-aligned fill loop, sprite transparency/blitting, the map
// blit's cell walk, or Print's 6x8 PICO-8 glyph walk cannot make both sides of the
// comparison change together.
// goldenLine renders the scene's diagonal line with a differently coded
// Bresenham variant rather than Line's own accumulator (see
// TestLineMatchesIndependentBresenhamVariant in draw_test.go for why that
// distinction matters and for dedicated regression coverage of the stepping
// loop itself across more directions and slopes than this one scene draws).
func TestGoldenFramebufferScene(t *testing.T) {
	sheet := goldenSheet()
	image := AssetImage{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		MapW: 2, MapH: 1,
		Sheet: sheet, Map: []uint16{1, 1}, Flags: []byte{0, 0},
	}
	ram := AssetMemory{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
		MapW: 2, MapH: 1,
		Sheet: make([]byte, len(sheet)), Map: make([]uint16, 2), Flags: make([]byte, 2),
	}
	var c Context
	if !c.Init(&image, &ram) {
		t.Fatal("golden fixture failed to initialize")
	}
	c.Cls(1)
	c.Rectfill(4, 6, 15, 12, 4)
	c.Line(0, 0, ScreenWidth-1, ScreenHeight-1, 2)
	c.Spr(0, 32, 24, 1, 1, false, false)
	c.Map(0, 0, 80, 40, 2, 1, 0)
	c.Print("HI", 200, 8, 7)

	want := goldenReference(sheet, image.Map)
	for i, got := range c.framebuffer {
		if got != want[i] {
			mismatches := 1
			for j := i + 1; j < len(c.framebuffer); j++ {
				if c.framebuffer[j] != want[j] {
					mismatches++
				}
			}
			x, y := i%ScreenWidth, i/ScreenWidth
			t.Fatalf("golden framebuffer mismatch at (%d,%d): got %d, want %d; %d total mismatches", x, y, got, want[i], mismatches)
		}
	}
}

func goldenSheet() []byte {
	first := []byte{
		0, 0, 8, 8, 8, 8, 0, 0,
		0, 8, 9, 9, 9, 9, 8, 0,
		8, 9, 10, 10, 10, 10, 9, 8,
		8, 9, 10, 12, 12, 10, 9, 8,
		8, 9, 10, 12, 12, 10, 9, 8,
		8, 9, 10, 10, 10, 10, 9, 8,
		0, 8, 9, 9, 9, 9, 8, 0,
		0, 0, 8, 8, 8, 8, 0, 0,
	}
	second := []byte{
		0, 0, 13, 13, 13, 13, 0, 0,
		0, 13, 13, 13, 13, 13, 13, 0,
		13, 13, 13, 13, 13, 13, 13, 13,
		13, 13, 13, 13, 13, 13, 13, 13,
		13, 13, 13, 13, 13, 13, 13, 13,
		13, 13, 13, 13, 13, 13, 13, 13,
		0, 13, 13, 13, 13, 13, 13, 0,
		0, 0, 13, 13, 13, 13, 0, 0,
	}
	return append(first, second...)
}

func goldenReference(sheet []byte, mapData []uint16) [FramebufferSize]byte {
	var frame [FramebufferSize]byte
	for i := range frame {
		frame[i] = 1
	}
	for y := 6; y <= 12; y++ {
		for x := 4; x <= 15; x++ {
			frame[y*ScreenWidth+x] = 4
		}
	}
	goldenLine(&frame, 0, 0, ScreenWidth-1, ScreenHeight-1, 2)
	goldenSprite(&frame, sheet, 16, 0, 32, 24)
	for i, id := range mapData {
		goldenSprite(&frame, sheet, 16, int(id), 80+(i%2)*8, 40+(i/2)*8)
	}
	goldenPrint(&frame, "HI", 200, 8, 7)
	return frame
}

// goldenLine uses a doubled, purely additive error term and an explicit
// d<0 branch, rather than Line's dx/2-initialized subtract-and-correct
// accumulator. It is not guaranteed to agree with Line for every possible
// pair of endpoints (the two formulations break exact-half-integer ties
// differently), but it does for this scene's (0,0)-(239,159) diagonal, and
// being different code means it cannot silently reproduce a bug in Line's
// own loop the way copying that loop verbatim would.
func goldenLine(frame *[FramebufferSize]byte, x0, y0, x1, y1 int, color byte) {
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
	setGoldenPixel(frame, x, y, color)
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
			setGoldenPixel(frame, x, y, color)
		}
		return
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
		setGoldenPixel(frame, x, y, color)
	}
}

func goldenSprite(frame *[FramebufferSize]byte, sheet []byte, sheetW, id, dx, dy int) {
	cellX, cellY := id%(sheetW/8), id/(sheetW/8)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			color := sheet[(cellY*8+y)*sheetW+cellX*8+x]
			if color != 0 {
				setGoldenPixel(frame, dx+x, dy+y, color)
			}
		}
	}
}

// goldenPrint walks the 4x5 PICO-8 wide column font in a different order
// from Print's nested loops. Both read font4x5, but a swapped row/column in
// drawGlyph cannot reproduce itself here.
func goldenPrint(frame *[FramebufferSize]byte, s string, x, y int, color byte) {
	for i := 0; i < len(s); i++ {
		columns := glyphColumns(s[i])
		originX := x + i*glyphW
		for col := 0; col < glyphBitmapW; col++ {
			for row := 0; row < glyphBitmapH; row++ {
				if columns[col]&(1<<row) != 0 {
					setGoldenPixel(frame, originX+col, y+row, color)
				}
			}
		}
	}
}

func setGoldenPixel(frame *[FramebufferSize]byte, x, y int, color byte) {
	if x >= 0 && x < ScreenWidth && y >= 0 && y < ScreenHeight {
		frame[y*ScreenWidth+x] = color
	}
}
