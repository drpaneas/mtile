package main

import "testing"

func TestFontBEditorCanvasAndControlsFit(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range [][2]int{{1024, 768}, {1600, 1000}} {
		setEditorLayout(size[0], size[1])
		l := newFontEditorLayout()
		if l.Canvas.W/l.Pixel != 6 || l.Canvas.H/l.Pixel != 10 {
			t.Fatal("canvas does not expose all B pixels")
		}
		if l.GlyphInput.Y+l.GlyphInput.H > l.Panel.Y+l.Panel.H-98 {
			t.Fatal("taller font canvas overlaps footer")
		}
		if l.Preview.H < 80 {
			t.Fatal("preview cannot fit two full B rows and fallback warning")
		}
	}
}
