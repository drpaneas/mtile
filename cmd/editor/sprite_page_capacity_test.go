package main

import (
	"image"
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
)

func TestFullObjectPagePasteAllowsFirstAndLastTile(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	t.Cleanup(g.closeNativePerformance)
	setClipboardTestPalette(g)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	w, h := editor.ObjPageWidth, editor.ObjPageHeight
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(w-1, h-1, color.NRGBA{G: 255, A: 255})
	if err := g.beginSpritePasteImage(img); err != nil {
		t.Fatal(err)
	}
	if err := g.commitSpritePastePlacement(); err != nil {
		t.Fatal(err)
	}
	if spriteSheetPixel(*g.model.ObjSheet, 0, 0) != 1 || spriteSheetPixel(*g.model.ObjSheet, w-1, h-1) != 2 {
		t.Fatal("full-page paste lost the first or last tile")
	}
}

func TestPoseSelectionCanCoverTheWholeObjectPage(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	t.Cleanup(g.closeNativePerformance)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.sheetSelectionAnchorCol, g.sheetSelectionAnchorRow = 0, 0
	g.selectSpritesheetDrag(editor.ObjPageWidth/8-1, editor.ObjPageHeight/8-1, g.spriteCols(), g.spriteRows())
	if g.gridPixelsW() != editor.ObjPageWidth || g.gridPixelsH() != editor.ObjPageHeight {
		t.Fatalf("pose selection clipped to %dx%d", g.gridPixelsW(), g.gridPixelsH())
	}
}

// A sparse bank avoids expensive artwork fixtures while still reaching the
// exact source-address limit through the same PAGE+ action as the live editor.
func newPenultimateObjectPageGame(t *testing.T) *Game {
	t.Helper()
	g := newTestGame(t, 32, 32, 2, 2)
	t.Cleanup(g.closeNativePerformance)
	t.Cleanup(func() { setEditorLayout(defaultScreenWidth, defaultScreenHeight) })
	g.Layout(minimumScreenWidth, minimumScreenHeight)
	g.model.ObjSheet = &assetsync.Spritesheet{Width: 512, Height: 25 * 320}
	g.model.Animation = assetsync.AnimationWorkspace{Version: 1}
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.selectObjPage(24)
	g.undo.reset(g.snapshot())
	return g
}

func TestTerminalObjectPageAddCapacityAndUndoRefreshLiveView(t *testing.T) {
	g := newPenultimateObjectPageGame(t)
	g.renderRequested = false
	if err := g.addObjPage(); err != nil {
		t.Fatal(err)
	}
	want := editor.Rect{Y: 8000, W: 512, H: 192}
	if g.objPage != 25 || len(g.model.Animation.Pages) != 26 || g.currentObjPageRect() != want || g.spriteN() != assetsync.MaxObjSheetCells {
		t.Fatalf("terminal page = %+v, selected %d, count %d, tile slots %d", g.currentObjPageRect(), g.objPage, len(g.model.Animation.Pages), g.spriteN())
	}
	assertLiveObjectPageVisible(t, g)
	before := g.snapshot().clone()
	history := len(g.undo.past)
	if err := g.addObjPage(); err == nil {
		t.Fatal("PAGE+ accepted a page beyond source capacity")
	}
	if !reflect.DeepEqual(before, g.snapshot().clone()) || len(g.undo.past) != history {
		t.Fatal("capacity rejection changed the model, selection, or undo history")
	}
	snapshot, ok := g.undo.undo()
	if !ok {
		t.Fatal("terminal page creation cannot be undone")
	}
	g.renderRequested = false
	g.restore(snapshot)
	if g.objPage != 24 || len(g.model.Animation.Pages) != 25 || g.model.ObjSheet.Height != 8000 {
		t.Fatal("undo did not restore the last full page and original capacity")
	}
	assertLiveObjectPageVisible(t, g)
	snapshot, ok = g.undo.redo()
	if !ok {
		t.Fatal("terminal page creation cannot be redone")
	}
	g.renderRequested = false
	g.restore(snapshot)
	if g.objPage != 25 || g.currentObjPageRect() != want {
		t.Fatal("redo did not restore the selected terminal page")
	}
	assertLiveObjectPageVisible(t, g)
}

func TestTerminalObjectPageSelectionAndPasteUseItsActualBounds(t *testing.T) {
	g := newPenultimateObjectPageGame(t)
	setClipboardTestPalette(g)
	if err := g.addObjPage(); err != nil {
		t.Fatal(err)
	}
	p := g.currentObjPageRect()
	g.sheetSelectionAnchorCol, g.sheetSelectionAnchorRow = p.X/8, p.Y/8
	// Drag past both edges; the selection must stop at the shorter page.
	g.selectSpritesheetDrag(1000, 10000, g.spriteCols(), g.spriteRows())
	if g.gridPixelsW() != p.W || g.gridPixelsH() != p.H {
		t.Fatalf("terminal selection = %dx%d, want %dx%d", g.gridPixelsW(), g.gridPixelsH(), p.W, p.H)
	}
	oversized := image.NewNRGBA(image.Rect(0, 0, p.W, p.H+8))
	before := g.snapshot().clone()
	if err := g.beginSpritePasteImage(oversized); err == nil || !strings.Contains(err.Error(), "current 512x192 page") {
		t.Fatalf("oversized terminal paste error = %v", err)
	}
	if g.spritePaste != nil || !reflect.DeepEqual(before, g.snapshot().clone()) {
		t.Fatal("rejected terminal paste changed artwork or began placement")
	}
	img := image.NewNRGBA(image.Rect(0, 0, p.W, p.H))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(p.W-1, p.H-1, color.NRGBA{G: 255, A: 255})
	if err := g.beginSpritePasteImage(img); err != nil {
		t.Fatal(err)
	}
	if err := g.commitSpritePastePlacement(); err != nil {
		t.Fatal(err)
	}
	if spriteSheetPixel(*g.model.ObjSheet, p.X, p.Y) != 1 || spriteSheetPixel(*g.model.ObjSheet, p.X+p.W-1, p.Y+p.H-1) != 2 {
		t.Fatal("terminal paste lost its first or final source tile")
	}
	if got := (p.Y+p.H-1)/8*g.spriteCols() + (p.X+p.W-1)/8; got != 65535 {
		t.Fatalf("last painted tile = %d, want65535", got)
	}
}

func TestUndoFirstObjectPageReturnsToSharedTiles(t *testing.T) {
	g := newTestGame(t, 32, 32, 2, 2)
	g.undo.reset(g.snapshot())
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.undo.forcePush(g.snapshot())
	g.setAnimationWorkspaceActive(true)
	snapshot, ok := g.undo.undo()
	if !ok {
		t.Fatal("creating the first page has no undo")
	}
	g.restore(snapshot)
	if g.objEditing || g.animationWorkspaceActive() || g.model.ObjSheet != nil {
		t.Fatal("undo left an object editor open without any object page")
	}
}
