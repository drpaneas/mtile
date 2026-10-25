//go:build !mtile_mode0

package runtime

import (
	"bytes"
	"testing"
)

func TestPrintDrawsLiteralGlyphsAndOpaqueZero(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cls(3)
	c.Print("A", 0, 0, 0)
	if c.Pget(2, 0) == 3 {
		t.Fatal("print color 0 did not overwrite the background")
	}
	c.Print("A", 12, 0, 7)
	if c.Pget(14, 0) != 7 {
		t.Fatal("print did not draw a glyph pixel")
	}
}

func TestPrintOneUsesPico8WideGlyph(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cls(0)
	c.Print("1", 0, 0, 7)
	want := [5]string{
		"##..",
		".#..",
		".#..",
		".#..",
		"###.",
	}
	for y, row := range want {
		for x, pixel := range row {
			got := c.Pget(x, y) == 7
			if got != (pixel == '#') {
				t.Fatalf("glyph 1 pixel (%d,%d) = %t, want %t", x, y, got, pixel == '#')
			}
		}
	}
}

func TestPrintLowercaseGUsesPico8WideGlyph(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cls(0)
	c.Print("g", 0, 0, 7)
	want := [5]string{
		"....",
		"###.",
		"#...",
		"#.#.",
		"###.",
	}
	for y, row := range want {
		for x, pixel := range row {
			got := c.Pget(x, y) == 7
			if got != (pixel == '#') {
				t.Fatalf("glyph g pixel (%d,%d) = %t, want %t", x, y, got, pixel == '#')
			}
		}
	}
}

func TestPrintWrapsAtClipWidth(t *testing.T) {
	t.Parallel()
	var c Context
	c.Clip(0, 0, 12, 16)
	c.Print("AAA", 0, 0, 7)
	if c.Pget(2, 0) != 7 {
		t.Fatal("first glyph missing")
	}
	if c.Pget(8, 0) != 7 {
		t.Fatal("second glyph missing on the first row")
	}
	if c.Pget(2, 8) != 7 {
		t.Fatal("third glyph did not wrap")
	}
}

func TestPrintAppliesCamera(t *testing.T) {
	t.Parallel()
	var c Context
	c.Camera(4, 0)
	c.Print("A", 4, 0, 7)
	if c.Pget(6, 0) != 7 {
		t.Fatal("print did not apply camera")
	}
}

func TestPrintHandlesTextControls(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cls(3)
	if got := c.Print("A\x00A", 0, 0, 7); got != 6 {
		t.Fatalf("NUL did not terminate print at x=6: got %d", got)
	}
	if got := c.Print("A\tA", 0, 12, 7); got != 30 {
		t.Fatalf("tab did not advance to the next four-cell stop: got %d", got)
	}
	if got := c.Print("A\x0c9A", 0, 24, 7); got != 12 {
		t.Fatalf("foreground control changed cursor unexpectedly: got %d", got)
	}
	if c.Pget(8, 24) != 9 {
		t.Fatal("foreground control did not change the following glyph color")
	}
	if got := c.Print("A\x07A", 0, 36, 7); got != 12 {
		t.Fatalf("unsupported print control changed cursor position: got %d", got)
	}
}

func TestPrintBackgroundControlFillsCell(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cls(3)
	c.Print("\x02aA", 0, 0, 7)
	if c.Pget(4, 0) != 10 || c.Pget(5, 7) != 10 {
		t.Fatal("background control did not fill the glyph cell")
	}
	if c.Pget(2, 0) != 7 {
		t.Fatal("foreground glyph was not drawn over its background")
	}
}

func TestPrintNativeGridFitsExactly(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cls(0)
	for row := 0; row < ScreenHeight/glyphH; row++ {
		c.Print("A", 0, row*glyphH, 7)
	}

	// The twentieth row occupies y=152..158. Its one-pixel cell spacing row
	// is y=159, so a full-height screen never clips the final glyph.
	if c.Pget(2, ScreenHeight-glyphH) != 7 {
		t.Fatal("last text row was not rendered")
	}
	if c.Pget(2, ScreenHeight-1) != 0 {
		t.Fatal("last text row has no bottom spacing")
	}
}

func TestPrintVUsesPersistentCursorAndImplicitNewline(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cls(0)
	text := c.InternStatic("A")

	for i := 0; i < 4; i++ {
		c.PrintV(text, NilVal(), NilVal(), NilVal())
	}

	for i := 0; i < 4; i++ {
		if got := c.Pget(2, i*glyphH); got != defaultDrawColor {
			t.Fatalf("implicit print line %d = %d, want default colour %d", i, got, defaultDrawColor)
		}
	}
	if c.cursor.x != 0 || c.cursor.y != 4*glyphH {
		t.Fatalf("cursor after implicit prints = (%d,%d), want (0,%d)", c.cursor.x, c.cursor.y, 4*glyphH)
	}
}

func TestPrintVCursorColorAndExplicitPosition(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cls(0)
	text := c.InternStatic("A")
	c.Cursor(12, 10, 9)
	c.PrintV(text, NilVal(), NilVal(), NilVal())
	if got := c.Pget(14, 10); got != 9 {
		t.Fatalf("cursor print colour = %d, want 9", got)
	}
	if c.cursor.x != 12 || c.cursor.y != 10+glyphH {
		t.Fatalf("cursor after positioned implicit print = (%d,%d), want (12,%d)", c.cursor.x, c.cursor.y, 10+glyphH)
	}

	c.PrintV(text, IntVal(30), IntVal(20), IntVal(11))
	if got := c.Pget(32, 20); got != 11 {
		t.Fatalf("explicit print colour = %d, want 11", got)
	}
	if c.cursor.x != 30 || c.cursor.y != 20 {
		t.Fatalf("cursor after explicit print = (%d,%d), want (30,20)", c.cursor.x, c.cursor.y)
	}
}

func TestPrintStaticVMatchesLiteralPrintV(t *testing.T) {
	t.Parallel()
	var generic, specialized Context
	generic.Cls(0)
	specialized.Cls(0)
	genericText := generic.InternStatic("A")
	specializedText := specialized.InternStatic("A")

	wantX := generic.PrintV(genericText, IntVal(30), IntVal(20), IntVal(11))
	gotX := specialized.PrintStaticV(specializedText, IntVal(30), IntVal(20), IntVal(11))
	if gotX != wantX || specialized.cursor != generic.cursor || !bytes.Equal(specialized.fbForTest(), generic.fbForTest()) {
		t.Fatalf("static print differs: x=%d/%d cursor=%+v/%+v", gotX, wantX, specialized.cursor, generic.cursor)
	}

	var genericCursor, specializedCursor Context
	genericCursor.Cls(0)
	specializedCursor.Cls(0)
	genericCursor.PrintV(genericCursor.InternStatic("A"), NilVal(), NilVal(), NilVal())
	specializedCursor.PrintStaticV(specializedCursor.InternStatic("A"), NilVal(), NilVal(), NilVal())
	if specializedCursor.cursor != genericCursor.cursor || !bytes.Equal(specializedCursor.fbForTest(), genericCursor.fbForTest()) {
		t.Fatalf("implicit static print differs: cursor=%+v/%+v", specializedCursor.cursor, genericCursor.cursor)
	}
	if got := specialized.PrintStaticV(NumVal(One), IntVal(30), IntVal(20), IntVal(11)); got != 30 {
		t.Fatalf("non-static input returned %d, want x=30", got)
	}
}

func TestPrintVTwoArgumentFormUsesColor(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cls(0)
	text := c.InternStatic("A")
	c.PrintV(text, IntVal(13), NilVal(), NilVal())
	if got := c.Pget(2, 0); got != 13 {
		t.Fatalf("print(str,col) colour = %d, want 13", got)
	}
	if c.cursor.x != 0 || c.cursor.y != glyphH {
		t.Fatalf("cursor after print(str,col) = (%d,%d), want (0,%d)", c.cursor.x, c.cursor.y, glyphH)
	}
}

func TestDrawColorIsUsedByOptionalPrintAndPrimitives(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cls(0)
	c.Color(12)
	c.PsetV(IntVal(20), IntVal(20), NilVal())
	if got := c.Pget(20, 20); got != 12 {
		t.Fatalf("PsetV omitted colour = %d, want 12", got)
	}
	c.PrintV(c.InternStatic("A"), NilVal(), NilVal(), NilVal())
	if got := c.Pget(2, 0); got != 12 {
		t.Fatalf("print omitted colour after PsetV = %d, want 12", got)
	}
	c.Rectfill(30, 30, 31, 31, 14)
	c.PrintV(c.InternStatic("A"), IntVal(40), IntVal(30), NilVal())
	if got := c.Pget(42, 30); got != 14 {
		t.Fatalf("print omitted colour after Rectfill = %d, want 14", got)
	}
}

func TestColorVWithoutArgumentResetsToDefault(t *testing.T) {
	t.Parallel()
	var c Context
	c.Color(9)
	c.ColorV(NilVal())
	c.PrintV(c.InternStatic("A"), NilVal(), NilVal(), NilVal())
	if got := c.Pget(2, 0); got != defaultDrawColor {
		t.Fatalf("COLOR() reset colour = %d, want %d", got, defaultDrawColor)
	}
}

func TestClsResetsCursorButPreservesDrawColor(t *testing.T) {
	t.Parallel()
	var c Context
	c.Cursor(20, 30, 12)
	c.Cls(0)
	c.PrintV(c.InternStatic("A"), NilVal(), NilVal(), NilVal())
	if got := c.Pget(2, 0); got != 12 {
		t.Fatalf("CLS changed draw colour = %d, want 12", got)
	}
	if c.cursor.x != 0 || c.cursor.y != glyphH {
		t.Fatalf("cursor after CLS and print = (%d,%d), want (0,%d)", c.cursor.x, c.cursor.y, glyphH)
	}
}
