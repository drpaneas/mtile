//go:build mtile_editor_bench

package main

import (
	"bytes"
	"fmt"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDevelopStatusPreservesDiagnosticsBottomBorder(t *testing.T) {
	t.Cleanup(func() {
		setInterfaceTextSize(textComfort)
		setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	})
	g := newTestGame(t, 16, 16, 8, 8)
	defer g.closeNativePerformance()
	g.mode = modeCode
	g.developPanel = developGenerated
	g.runGeneratedGo = "package main\n\nfunc main() {\n}\n"
	const width, height = 1920, 1200
	g.Layout(width, height)
	target := ebiten.NewImage(width, height)
	defer target.Deallocate()
	before, after := make([]byte, width*height*4), make([]byte, width*height*4)
	for textSize := textCompact; textSize < interfaceTextSizeCount; textSize++ {
		for _, message := range []string{"", "game input active; press Esc to return to code"} {
			t.Run(fmt.Sprintf("%s/message=%t", textSize, message != ""), func(t *testing.T) {
				setInterfaceTextSize(textSize)
				g.statusMsg = message
				onRenderBenchmarkFrame(func() {
					g.drawDevelopMode(target)
					target.ReadPixels(before)
					g.drawStatusBar(target)
					target.ReadPixels(after)
				})
				layout := newDevelopLayout()
				r := layout.Code.Diagnostics
				// StrokeRect straddles the panel boundary. Both rows must
				// survive drawing the notification/status area.
				for y := r.Y + r.H - 1; y <= r.Y+r.H; y++ {
					start := (y*width + r.X + 1) * 4
					end := (y*width + r.X + r.W - 1) * 4
					if !bytes.Equal(before[start:end], after[start:end]) {
						t.Errorf("status rendering erased the diagnostics bottom border at y=%d", y)
					}
				}
				// Check the surrounding workspace too: an intact pane border
				// alone does not prove the dark outer frame is complete.
				for _, sample := range []struct {
					y    int
					want color.RGBA
				}{
					{layout.Message.Y - 6, colorInk},
					{layout.Message.Y - 1, colorBorder},
				} {
					at := (sample.y*width + r.X + r.W/2) * 4
					got := color.RGBA{R: after[at], G: after[at+1], B: after[at+2], A: after[at+3]}
					if got != sample.want {
						t.Errorf("outer workspace at y=%d = %v, want %v", sample.y, got, sample.want)
					}
				}
			})
		}
	}
}
