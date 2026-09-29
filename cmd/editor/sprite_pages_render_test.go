//go:build mtile_editor_bench

package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/hajimehoshi/ebiten/v2"
)

// Capture real artwork using a disposable copy. The author's current Nerdo
// sheet is never saved, resized, or rewritten by this diagnostic.
func TestObjectPagesLiveRender(t *testing.T) {
	out := os.Getenv("MTILE_OBJ_PAGES_REPORT")
	if out == "" {
		t.Skip("set MTILE_OBJ_PAGES_REPORT to capture object pages and playback")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct {
		name          string
		width, height int
	}{{"desktop", 1920, 1200}, {"compact", minimumScreenWidth, minimumScreenHeight}} {
		t.Run(size.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "examples", "nerdo"))); err != nil {
				t.Fatal(err)
			}
			g, err := newGame(dir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(g.closeNativePerformance)
			g.preferences = defaultEditorPreferences()
			g.applyPreferences()
			g.Layout(size.width, size.height)
			if err := g.setObjEditing(true); err != nil {
				t.Fatal(err)
			}
			g.selectObjPage(0)
			if len(g.authoredPoses()) > 0 {
				g.selectPose(0)
			}
			capture := func(name string) {
				t.Helper()
				target := ebiten.NewImage(size.width, size.height)
				defer target.Deallocate()
				pixels := make([]byte, size.width*size.height*4)
				onRenderBenchmarkFrame(func() { g.requestRender(); g.Draw(target); target.ReadPixels(pixels) })
				if name == "added-blank-page" || name == "terminal-page" {
					// Both full and terminal pages anchor their selected first cell
					// at the live canvas origin after switching pages.
					p := g.currentObjPageRect()
					cell := sheetCameraRect(g.sheetCam, g.sheetGridCanvas(), p.X/8, p.Y/8, 1, 1)
					cyanPixels := 0
					for y := cell.Y; y < cell.Y+cell.H; y++ {
						for x := cell.X; x < cell.X+cell.W; x++ {
							i := (y*size.width + x) * 4
							r, green, b := int(pixels[i]), int(pixels[i+1]), int(pixels[i+2])
							if green > 120 && green > r+70 && b > r+70 {
								cyanPixels++
							}
						}
					}
					if cyanPixels == 0 {
						t.Fatal("selection outline is missing from the displayed first cell of the new page")
					}
				}
				file, err := os.Create(filepath.Join(out, size.name+"-"+name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(file, &image.NRGBA{Pix: pixels, Stride: size.width * 4, Rect: image.Rect(0, 0, size.width, size.height)}); err != nil {
					_ = file.Close()
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			}
			assertLiveObjectPageVisible(t, g)
			capture("page-1")

			g.objShowReuse = true
			usage := g.objectTileUsage()
			for id, canonical := range usage.analysis.CanonicalIDs {
				if canonical >= 0 && len(usage.groups[canonical]) > 1 && g.objPageContains(id%g.spriteCols(), id/g.spriteCols()) {
					g.currentSprite, g.gridCols, g.gridRows = id, 1, 1
					break
				}
			}
			capture("pose-reuse")
			g.objShowReuse = false
			g.setAnimationWorkspaceActive(true)
			g.createAnimationClip()
			// Keep the generated unique name: the author's project can already
			// contain an idle clip, which must survive this disposable migration.
			if !g.commitAnimationNameEdit() {
				t.Fatal("could not name the captured clip")
			}
			for id := range g.authoredPoses() {
				g.animation.pickerPose = id
				g.editAnimationClipFrame("add")
			}
			if !g.registerAnimationClip() {
				t.Fatal("could not save the captured clip")
			}
			g.startAnimationPreview()
			now := time.Unix(100, 0)
			g.advanceAnimationPreview(now)
			g.advanceAnimationPreview(now.Add(time.Second / 8))
			capture("animations-playback")
			g.openAnimationPosePicker()
			capture("animations-pose-picker")
			g.setAnimationWorkspaceActive(false)

			g.objShowReuse = false
			g.renderRequested = false
			if err := g.addObjPage(); err != nil {
				t.Fatal(err)
			}
			assertLiveObjectPageVisible(t, g)
			if g.selectedPose != -1 || len(g.visiblePagePoseIDs()) != 0 || g.animationPreviewPose() != -1 {
				t.Fatal("new blank page retained an old frame or preview")
			}
			capture("added-blank-page")

			for g.model.ObjSheet.Width/8*(g.model.ObjSheet.Height/8) < assetsync.MaxObjSheetCells {
				if _, err := g.model.AddObjPage(); err != nil {
					t.Fatal(err)
				}
			}
			g.objUsage = nil
			g.selectObjPage(len(g.model.Animation.Pages) - 1)
			assertLiveObjectPageVisible(t, g)
			capture("terminal-page")
		})
	}
}
