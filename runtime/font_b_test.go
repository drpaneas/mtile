//go:build !mtile_mode0

package runtime

import "testing"

func fontBSolidFixture(t *testing.T) *Font {
	t.Helper()
	f := *DefaultFont()
	if len(f.ASCII[0]) != 6 {
		t.Fatal("B requires six ROM columns")
	}
	index := int('A' - 32)
	for x := range f.ASCII[index] {
		f.ASCII[index][x] = 0
		for y := 0; y < 10; y++ {
			f.ASCII[index][x] = f.ASCII[index][x]<<1 | 1
		}
	}
	return &f
}
func TestFontBMetricsAndScreenBounds(t *testing.T) {
	if glyphW != 6 || glyphH != 10 || glyphBitmapW != 6 || glyphBitmapH != 10 {
		t.Fatalf("B metrics are %dx%d / %dx%d", glyphBitmapW, glyphBitmapH, glyphW, glyphH)
	}
	f := fontBSolidFixture(t)
	var c Context
	if !c.SetFont(f) {
		t.Fatal("B font rejected")
	}
	for _, origin := range [][2]int{{234, 150}, {239, 159}, {-3, -3}, {236, 155}, {0, 0}} {
		for _, cam := range [][2]int{{0, 0}, {3, 4}, {-2, -1}} {
			for _, clip := range [][4]int{{0, 0, 240, 160}, {2, 2, 236, 156}, {230, 150, 4, 5}} {
				c.CameraReset()
				c.Cls(0)
				c.Camera(cam[0], cam[1])
				c.Clip(clip[0], clip[1], clip[2], clip[3])
				c.Print("A", origin[0], origin[1], 7)
				cx, cy, cw, ch := c.ClipRect()
				for y := 0; y < 160; y++ {
					for x := 0; x < 240; x++ {
						gx, gy := x-origin[0]+cam[0], y-origin[1]+cam[1]
						want := byte(0)
						if gx >= 0 && gx < 6 && gy >= 0 && gy < 10 && x >= cx && x < cx+cw && y >= cy && y < cy+ch {
							want = 7
						}
						if c.pixelRaw(x, y) != want {
							t.Fatalf("origin%v camera%v clip%v pixel%d,%d", origin, cam, clip, x, y)
						}
					}
				}
			}
		}
	}
	c.CameraReset()
	c.Cls(0)
	c.Cursor(0, 0, 7)
	if c.PrintCursor("AA") != 12 || c.cursor.y != 10 {
		t.Fatal("implicit cursor advance")
	}
	c.Cls(0)
	if c.Print("A\tA", 0, 0, 7) != 30 {
		t.Fatal("tab advance changed")
	}
	c.Cls(0)
	c.Clip(0, 0, 12, 30)
	c.Print("AAA", 0, 0, 7)
	if c.pixelRaw(0, 10) != 7 {
		t.Fatal("wrap did not advance ten rows")
	}
	c.Cls(0)
	c.Print("\x022A", 0, 0, 7)
	if c.pixelRaw(5, 9) != 7 || c.pixelRaw(5, 10) != 0 {
		t.Fatal("tenth row/background bounds")
	}
	if a := testing.AllocsPerRun(100, func() { c.Print("Grüße Ωάΐ", 0, 0, 7) }); a != 0 {
		t.Fatal("print allocates", a)
	}
}
func TestProjectFontDoesNotRestyleSystemPause(t *testing.T) {
	var a, b Context
	a.SetFont(DefaultFont())
	b.SetFont(fontBSolidFixture(t))
	a.PreparePauseMenu()
	b.PreparePauseMenu()
	if a.pause.video.pixels != b.pause.video.pixels {
		t.Fatal("custom font changed system pause message")
	}
}
