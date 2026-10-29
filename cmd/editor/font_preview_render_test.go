//go:build mtile_editor_bench

package main

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/hajimehoshi/ebiten/v2"
)

func TestFontPreviewVisibleAndClippedAtDisplayScale(t *testing.T) {
	preserveDisplayState(t)
	g := newTestGame(t, 8, 8, 1, 1)
	defer g.closeNativePerformance()
	g.openFontEditor()
	if err := g.model.SelectFont("arcade"); err != nil {
		t.Fatal(err)
	}
	font, err := assetsync.CompileFont(g.model.ActiveFont())
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{1024, 768}, {1600, 1000}} {
		g.Layout(size[0], size[1])
		l := newFontEditorLayout()
		for _, scale := range []float64{1, 1.25, 1.5, 2} {
			t.Run(fmt.Sprintf("%dx%d/%gx", size[0], size[1], scale), func(t *testing.T) {
				for _, text := range []string{"Hello", "Hello\nGBA", strings.Repeat("A", 160)} {
					g.fontEditor.preview = text
					width, height := int(math.Ceil(float64(size[0])*scale)), int(math.Ceil(float64(size[1])*scale))
					target := ebiten.NewImage(width, height)
					img := image.NewRGBA(image.Rect(0, 0, width, height))
					onRenderBenchmarkFrame(func() { g.requestRender(); g.drawAtScale(target, scale); target.ReadPixels(img.Pix) })
					target.Deallocate()
					if text == "Hello" {
						writeHiDPICapture(t, fmt.Sprintf("font-preview-%dx%d-%gx", size[0], size[1], scale), img)
					}
					// Sample every glyph pixel at its physical center. This proves the actual
					// font preview, including the second line, survives native/fractional DPI.
					x, y := l.Preview.X+12, l.Preview.Y+12
					for _, cp := range text {
						if cp == '\n' || x+12 > l.Preview.X+l.Preview.W-12 {
							x = l.Preview.X + 12
							y += 20
							if cp == '\n' {
								continue
							}
						}
						for col, bits := range font.Columns(cp) {
							for row := 0; row < 10; row++ {
								lx, ly := x+col*2+1, y+row*2+1
								if ly >= l.Preview.Y+52 {
									continue
								}
								px, py := int(float64(lx)*scale), int(float64(ly)*scale)
								want := colorBackground
								if bits&(1<<uint(row)) != 0 {
									want = colorLCD
								}
								if got := img.RGBAAt(px, py); got != want {
									t.Fatalf("%q preview pixel (%d,%d)=%v, want %v", text[:min(5, len(text))], px, py, got, want)
								}
							}
						}
						x += 12
					}
					// Long samples must not paint the third line into the warning/footer area.
					if len(text) == 160 {
						for ly := l.Preview.Y + 54; ly < l.Preview.Y+l.Preview.H-2; ly++ {
							for lx := l.Preview.X + 2; lx < l.Preview.X+l.Preview.W-2; lx++ {
								if img.RGBAAt(int(float64(lx)*scale), int(float64(ly)*scale)) == colorLCD {
									t.Fatal("third preview line escaped the two-line clip")
								}
							}
						}
					}
				}
			})
		}
	}
}
