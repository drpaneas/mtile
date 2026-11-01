//go:build mtile_editor_bench

package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/hajimehoshi/ebiten/v2"
)

func TestDevelopLintNativeCaptures(t *testing.T) {
	preserveDisplayState(t)
	g := newTestGame(t, 8, 8, 1, 1)
	defer g.closeNativePerformance()
	module := `function assemble_ribbon()
 actor.ribbon={}
 for n=1,3 do add(actor.ribbon,{x=actor.x,y=actor.y}) end
end
`
	entry := `require("parts")
function reset_actor()
 actor={x=10,y=20,ribbon={}}
 assemble_ribbon()
end
function enter_scene()
 reset_actor()
 actor.y=100
 assemble_ribbon()
end
function _update() if btn(4) then enter_scene() end end
function _draw() cls(0) end
`
	if err := os.MkdirAll(filepath.Join(g.model.Dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g.model.Dir, "src", "parts.lua"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	g.code.SelectAll()
	g.code.Insert(entry)
	g.refreshCodeDiagnostics()
	g.scanDevelopProject()
	if _, err := g.refreshProjectSourceAnalysis(); err != nil {
		t.Fatal(err)
	}
	g.codeAnalysis = (build.Service{UseProjectRenderer: true}).CheckProject(g.model.Dir, g.projectSourceOverlay())
	g.codeAnalysisDue = time.Time{}
	if !g.codeAnalysis.LintReady || g.lintAdviceCount() != 2 {
		t.Fatalf("capture requires two real analyzer findings: %+v", g.codeAnalysis)
	}
	g.mode, g.developPanel = modeCode, developLint
	for _, d := range g.codeAnalysis.Lint {
		if d.Rule == "MTL008" && d.Path == "game.lua" {
			g.openProjectDiagnostic(d.Path, d.Line, d.Column)
			break
		}
	}
	for _, size := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1920, 1200}} {
		g.Layout(size[0], size[1])
		for _, scale := range []float64{1, 2} {
			t.Run(fmt.Sprintf("%dx%d/%gx", size[0], size[1], scale), func(t *testing.T) {
				target := ebiten.NewImage(int(float64(size[0])*scale), int(float64(size[1])*scale))
				defer target.Deallocate()
				img := image.NewRGBA(image.Rect(0, 0, target.Bounds().Dx(), target.Bounds().Dy()))
				onRenderBenchmarkFrame(func() { g.requestRender(); g.drawAtScale(target, scale); target.ReadPixels(img.Pix) })
				writeHiDPICapture(t, fmt.Sprintf("lint-%dx%d-%gx", size[0], size[1], scale), img)
				if !g.code.Dirty() {
					t.Fatal("rendering saved or replaced the source snapshot")
				}
			})
		}
	}
}
