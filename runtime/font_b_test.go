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
func pauseBPixel(v *pauseVideo, x, y int) uint32 {
	return v.pixels[(y/8*8+x/8)*8+y%8] >> uint(x%8*4) & 15
}
func TestFontBPauseRendersWholeGlyphAndFitsAllStates(t *testing.T) {
	f := fontBSolidFixture(t)
	var v pauseVideo
	v.font = f
	v.text("AA", 0, 0, 4)
	for y := 0; y < 10; y++ {
		for x := 0; x < 12; x++ {
			if pauseBPixel(&v, x, y) != 4 {
				t.Fatalf("pause truncated B at %d,%d", x, y)
			}
		}
	}
	// Every status uses a single, bounded heading; items and cooldown stay visible.
	for _, state := range []struct {
		result  int8
		heading string
	}{{0, "PAUSED"}, {1, "Saved"}, {2, "Loaded"}, {3, "No save"}, {4, "Wrong save"}, {5, "Bad save"}, {-1, "Save error"}, {-2, "Load error"}, {10, "Disabled"}} {
		var c Context
		c.SetFont(DefaultFont())
		c.ConfigurePauseMenu()
		c.pause.result = state.result
		c.pause.wait = 1
		c.PreparePauseMenu()
		assertPauseBText(t, &c.pause.video, state.heading, (64-len(state.heading)*6)/2, 1)
		for i, label := range []string{"Load", "Save", "Exit"} {
			assertPauseBText(t, &c.pause.video, label, 10, 16+i*13)
		}
		assertPauseBText(t, &c.pause.video, "WAIT", 20, 54)
	}
}
func assertPauseBText(t *testing.T, v *pauseVideo, s string, x, y int) {
	t.Helper()
	for i, cp := range s {
		cols := DefaultFont().Columns(cp)
		for col, bits := range cols {
			for row := 0; row < 10; row++ {
				if bits&(1<<uint(row)) == 0 {
					continue
				}
				px, py := x+i*6+col, y+row
				if px < 0 || px >= 64 || py < 0 || py >= 64 {
					t.Fatal("pause ink outside system OBJ")
				}
				got := pauseBPixel(v, px, py)
				if got != 4 && got != 5 {
					t.Fatalf("pause %q lost ink at %d,%d", s, px, py)
				}
			}
		}
	}
}
