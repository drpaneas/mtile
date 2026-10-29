//go:build !mtile_mode0

package runtime

import "testing"

func testUnicodeFont() *Font {
	f := *DefaultFont()
	f.Extra = []FontGlyph{{'ä', [6]uint16{1, 2, 4, 8}}, {'Ω', [6]uint16{31, 1, 1, 31}}}
	return &f
}
func TestFontUTF8RenderingAndMetrics(t *testing.T) {
	f := testUnicodeFont()
	var c Context
	if !c.SetFont(f) {
		t.Fatal("valid font rejected")
	}
	for _, test := range []struct {
		text string
		end  int
	}{{"Ωä", 12}, {"Ω\tΩ", 30}, {"\x012Ω", 12}, {"Ω\nΩ", 6}, {"\xff", 6}, {"🙂", 6}, {"Ω\x00Ω", 6}} {
		c.Cls(0)
		if got := c.Print(test.text, 0, 0, 7); got != test.end {
			t.Errorf("%q end=%d want=%d", test.text, got, test.end)
		}
	}
	c.Cls(0)
	c.Print("Ωä", 0, 0, 7)
	for i, ch := range []rune("Ωä") {
		cols := f.Columns(ch)
		for y := 0; y < 10; y++ {
			for x := 0; x < 6; x++ {
				want := byte(0)
				if x < 6 && y < 10 && cols[x]&(1<<uint(y)) != 0 {
					want = 7
				}
				if got := c.Pget(i*6+x, y); got != want {
					t.Fatalf("pixel %d,%d=%d want=%d", i*6+x, y, got, want)
				}
			}
		}
	}
	c.Cls(0)
	c.Clip(0, 0, 6, 16)
	c.Print("Ωä", 0, 0, 7)
	if c.Pget(3, 13) != 7 {
		t.Fatal("UTF8 wrap lost glyph")
	}
	c.Clip(0, 0, 240, 160)
	c.Cls(0)
	c.PrintStaticV(c.InternStatic("Ωä"), IntVal(0), IntVal(0), IntVal(7))
	if c.Pget(6, 0) != 7 {
		t.Fatal("static UTF8")
	}
	v, err := c.NewString([]byte("Ωä"))
	if err != HeapOK {
		t.Fatal(err)
	}
	c.Cls(0)
	c.PrintV(v, IntVal(0), IntVal(0), IntVal(7))
	if c.Pget(6, 0) != 7 {
		t.Fatal("heap UTF8")
	}
	if n := testing.AllocsPerRun(50, func() { c.Print("Ωä", 0, 0, 7) }); n != 0 {
		t.Fatalf("print allocated %g", n)
	}
}
func TestSetFontRejectsInvalidAndPreservesPrevious(t *testing.T) {
	f := testUnicodeFont()
	var c Context
	c.SetFont(f)
	bad := *f
	bad.ASCII[1][0] = 1024
	if c.SetFont(&bad) || c.font != f {
		t.Fatal("invalid bitmap accepted")
	}
	bad = *f
	bad.Extra = []FontGlyph{{'Ω', [6]uint16{}}, {'ä', [6]uint16{}}}
	if c.SetFont(&bad) {
		t.Fatal("unsorted extras accepted")
	}
	bad = *f
	bad.ASCII[0][0] = 1
	if c.SetFont(&bad) {
		t.Fatal("painted space")
	}
	if c.SetFont(nil) {
		t.Fatal("nil accepted")
	}
}
func BenchmarkSelectedFontPrint(b *testing.B) {
	var c Context
	c.SetFont(testUnicodeFont())
	for _, s := range []string{"SCORE 12345", "ΩäΩäΩäΩäΩä"} {
		b.Run(s, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				c.Print(s, 0, 0, 7)
			}
		})
	}
}
