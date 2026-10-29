//go:build mtile_editor_bench

package main

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/hajimehoshi/ebiten/v2"
)

func TestDevelopLongDiagnosticWrapsAtNativeDensity(t *testing.T) {
	preserveDisplayState(t)
	g := newTestGame(t, 8, 8, 1, 1)
	defer g.closeNativePerformance()
	g.mode = modeCode
	g.developPanel = developProblems
	g.codeAnalysis = build.ProjectAnalysis{}
	g.runError = "ERROR game.lua:272:1: [M0_OBJ_BUDGET] " + strings.Repeat("Long diagnostic with Ελληνικά and extra detail. ", 12) + " END OF ERROR"
	for _, size := range [][2]int{{1024, 768}, {1920, 1200}} {
		g.Layout(size[0], size[1])
		l := newDevelopPanelLayout(g.developPanel)
		r := l.Code.Diagnostics
		for _, scale := range []float64{1, 1.5, 2} {
			t.Run(fmt.Sprintf("%dx%d/%gx", size[0], size[1], scale), func(t *testing.T) {
				w, h := int(math.Ceil(float64(size[0])*scale)), int(math.Ceil(float64(size[1])*scale))
				target := ebiten.NewImage(w, h)
				defer target.Deallocate()
				img := image.NewRGBA(image.Rect(0, 0, w, h))
				onRenderBenchmarkFrame(func() { g.requestRender(); g.drawAtScale(target, scale); target.ReadPixels(img.Pix) })
				writeHiDPICapture(t, fmt.Sprintf("diagnostics-%dx%d-%gx", size[0], size[1], scale), img)
				// The second visual line must contain the same error's continuation.
				// Previously the whole diagnostic was ellipsized onto only the first row.
				ink := 0
				for y := int(float64(r.Y+40+codeLineHeight) * scale); y < int(float64(r.Y+40+2*codeLineHeight)*scale); y++ {
					for x := int(float64(r.X+14) * scale); x < int(float64(r.X+r.W-30)*scale); x++ {
						c := img.RGBAAt(x, y)
						if c.R > 150 && int(c.R) > int(c.G)+50 {
							ink++
						}
					}
				}
				if ink < 8 {
					t.Fatalf("wrapped continuation is invisible: %d error pixels", ink)
				}
			})
		}
	}
}
