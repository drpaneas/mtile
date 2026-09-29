package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
)

// Page changes must update the live camera themselves. Calling fitSheetCamera
// from these tests would mask the resize-until-restart regression.
func assertLiveObjectPageVisible(t *testing.T, g *Game) {
	t.Helper()
	p, canvas := g.currentObjPageRect(), g.sheetGridCanvas()
	x1, y1 := g.sheetCam.WorldToScreen(p.X/8, p.Y/8, canvas)
	x2, y2 := g.sheetCam.WorldToScreen((p.X+p.W)/8, (p.Y+p.H)/8, canvas)
	const epsilon = 0.000001
	if p.W <= 0 || p.H <= 0 || x2 <= x1 || y2 <= y1 ||
		x1 < float64(canvas.X)-epsilon || y1 < float64(canvas.Y)-epsilon ||
		x2 > float64(canvas.X+canvas.W)+epsilon || y2 > float64(canvas.Y+canvas.H)+epsilon {
		t.Fatalf("page %+v is not fully visible: screen (%g,%g)-(%g,%g), canvas %+v, camera %+v", p, x1, y1, x2, y2, canvas, g.sheetCam)
	}
	if math.Abs(x1-float64(canvas.X)) > epsilon || math.Abs(y1-float64(canvas.Y)) > epsilon ||
		math.Abs(x2-float64(canvas.X+canvas.W)) > epsilon || math.Abs(y2-float64(canvas.Y+canvas.H)) > epsilon {
		t.Fatalf("page does not fill its canvas: screen (%g,%g)-(%g,%g), canvas %+v", x1, y1, x2, y2, canvas)
	}
	if !g.renderRequested {
		t.Fatal("page changed without requesting a render")
	}
}

func TestObjectPageAddSwitchUndoRefreshesLiveView(t *testing.T) {
	t.Cleanup(func() {
		setInterfaceTextSize(textComfort)
		setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	})
	for _, size := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1280, 1024}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			setInterfaceTextSize(textComfort)
			g := newTestGame(t, 32, 32, 2, 2)
			t.Cleanup(g.closeNativePerformance)
			g.Layout(size[0], size[1])
			if err := g.setObjEditing(true); err != nil {
				t.Fatal(err)
			}
			g.model.EnsureObjPages()
			g.undo.reset(g.snapshot())
			oldCount, oldPage, oldHeight := len(g.model.Animation.Pages), g.objPage, g.model.ObjSheet.Height
			g.renderRequested = false
			if err := g.addObjPage(); err != nil {
				t.Fatal(err)
			}
			if len(g.model.Animation.Pages) != oldCount+1 || g.objPage != oldCount {
				t.Fatalf("new page not selected: count %d, selection %d", len(g.model.Animation.Pages), g.objPage)
			}
			want := editor.Rect{Y: oldHeight, W: editor.ObjPageWidth, H: editor.ObjPageHeight}
			if got := g.currentObjPageRect(); got != want {
				t.Fatalf("new page = %+v, want %+v", got, want)
			}
			assertLiveObjectPageVisible(t, g)
			for _, index := range []int{oldPage, oldCount} {
				g.renderRequested = false
				g.selectObjPage(index)
				if g.objPage != index {
					t.Fatalf("selected page = %d, want %d", g.objPage, index)
				}
				assertLiveObjectPageVisible(t, g)
			}
			snapshot, ok := g.undo.undo()
			if !ok {
				t.Fatal("adding a page did not create undo history")
			}
			g.renderRequested = false
			g.restore(snapshot)
			if len(g.model.Animation.Pages) != oldCount || g.objPage != oldPage || g.model.ObjSheet.Height != oldHeight {
				t.Fatalf("undo did not restore page, count, and bank geometry: page %d, count %d, height %d", g.objPage, len(g.model.Animation.Pages), g.model.ObjSheet.Height)
			}
			assertLiveObjectPageVisible(t, g)
			snapshot, ok = g.undo.redo()
			if !ok {
				t.Fatal("added page cannot be redone")
			}
			g.renderRequested = false
			g.restore(snapshot)
			if len(g.model.Animation.Pages) != oldCount+1 || g.objPage != oldCount || g.currentObjPageRect() != want {
				t.Fatal("redo did not restore the newly selected page")
			}
			assertLiveObjectPageVisible(t, g)
		})
	}
}

func TestLegacyObjectSheetOpensLargerPagesWithoutRestart(t *testing.T) {
	t.Cleanup(func() { setEditorLayout(defaultScreenWidth, defaultScreenHeight) })
	g := newTestGame(t, 32, 32, 2, 2)
	t.Cleanup(g.closeNativePerformance)
	g.model.ObjSheet = &assetsync.Spritesheet{Width: 800, Height: 640}
	g.model.Animation = assetsync.AnimationWorkspace{}
	g.Layout(minimumScreenWidth, minimumScreenHeight)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if len(g.model.Animation.Pages) == 0 {
		t.Fatal("migration produced no page")
	}
	for index, page := range g.model.Animation.Pages {
		if page.Width != editor.ObjPageWidth || page.Height != editor.ObjPageHeight {
			t.Fatalf("legacy page %d retained uneven dimensions: %+v", index, page)
		}
		g.renderRequested = false
		g.selectObjPage(index)
		assertLiveObjectPageVisible(t, g)
	}
}

func TestAnimationPageDockFitsMinimumWindow(t *testing.T) {
	t.Cleanup(func() {
		setInterfaceTextSize(textComfort)
		setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	})
	for size := textCompact; size < interfaceTextSizeCount; size++ {
		for _, window := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1920, 1200}} {
			t.Run(fmt.Sprintf("%s/%dx%d", size, window[0], window[1]), func(t *testing.T) {
				setInterfaceTextSize(size)
				setEditorLayout(window[0], window[1])
				g := &Game{objEditing: true, model: &editor.Model{Animation: assetsync.AnimationWorkspace{Pages: make([]assetsync.AnimationPage, 8)}}}
				layout := newAnimationDockLayout(g.animationDockRect())
				if rectsOverlapIDE(g.sheetGridCanvas(), layout.Panel) {
					t.Fatal("animation dock overlaps the page canvas")
				}
				controls := []editor.Rect{layout.Track, layout.Preview, layout.RegisterPose, layout.UpdatePose, layout.DeletePose, layout.ScrollPrev, layout.ScrollNext}
				for index, r := range controls {
					if r.W <= 0 || r.H <= 0 || r.X < layout.Panel.X || r.Y < layout.Panel.Y || r.X+r.W > layout.Panel.X+layout.Panel.W || r.Y+r.H > layout.Panel.Y+layout.Panel.H {
						t.Errorf("control %d %+v escapes dock %+v", index, r, layout.Panel)
					}
					for other := index + 1; other < len(controls); other++ {
						if rectsOverlapIDE(r, controls[other]) {
							t.Errorf("controls %d %+v and %d %+v overlap", index, r, other, controls[other])
						}
					}
				}
				for _, selected := range []int{0, 7} {
					g.objPage = selected
					bar := g.objPageBar()
					if len(bar.Tabs) < 3 || selected < bar.First || selected >= bar.First+len(bar.Tabs) {
						t.Fatalf("page bar does not show at least three tabs including selection %d: %+v", selected, bar)
					}
					controls := append([]editor.Rect{bar.Prev, bar.Next, bar.Paste, bar.Add}, bar.Tabs...)
					for index, r := range controls {
						if r.W <= 0 || r.H <= 0 || r.X < sheetPanelRect.X || r.X+r.W > sheetPanelRect.X+sheetPanelRect.W || r.Y < spritePanelContentY(sheetPanelRect) || r.Y+r.H > g.sheetGridCanvas().Y {
							t.Errorf("page control %d %+v escapes its header", index, r)
						}
						for other := index + 1; other < len(controls); other++ {
							if rectsOverlapIDE(r, controls[other]) {
								t.Errorf("page controls %d %+v and %d %+v overlap", index, r, other, controls[other])
							}
						}
					}
					if smallTextWidth("PASTE PNG")+12 > bar.Paste.W {
						t.Error("PASTE PNG label does not fit its visible button")
					}
					if smallTextWidth("PAGE+")+12 > bar.Add.W {
						t.Error("PAGE+ label does not fit its visible button")
					}
				}
				button := tileReuseButtonRect()
				for row := range 2 {
					r := editor.Rect{X: button.X, Y: button.Y + button.H + 8 + row*(uiSmallTextHeight+6), W: button.W, H: uiSmallTextHeight}
					if r.X < spriteFlagsPanelRect.X || r.Y < button.Y+button.H || r.X+r.W > spriteFlagsPanelRect.X+spriteFlagsPanelRect.W || r.Y+r.H+4 > spriteFlagsPanelRect.Y+spriteFlagsPanelRect.H {
						t.Errorf("reuse summary row %d %+v escapes panel %+v", row, r, spriteFlagsPanelRect)
					}
				}
			})
		}
	}
}

func TestSpriteWorkspaceTabsFitAtEveryTextSize(t *testing.T) {
	t.Cleanup(func() {
		setInterfaceTextSize(textComfort)
		setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	})
	for size := textCompact; size < interfaceTextSizeCount; size++ {
		setInterfaceTextSize(size)
		setEditorLayout(minimumScreenWidth, minimumScreenHeight)
		shared, poses, animations := spriteWorkspaceTabRects()
		for _, control := range []struct {
			rect editor.Rect
			text string
		}{{shared, "SHARED TILES"}, {poses, "POSES"}, {animations, "ANIMATIONS"}} {
			if !rectInsideIDE(control.rect, spriteHeaderRect) || smallTextWidth(control.text)+12 > control.rect.W {
				t.Fatalf("%s tab does not fit at %s", control.text, size)
			}
		}
		if rectsOverlapIDE(shared, poses) || rectsOverlapIDE(poses, animations) {
			t.Fatalf("workspace tabs overlap at %s", size)
		}
	}
}
