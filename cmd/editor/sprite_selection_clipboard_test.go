package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/hostclipboard"
)

type selectionTestClipboard struct {
	data []byte
	err  error
}

func stubSelectionClipboard(t *testing.T) *selectionTestClipboard {
	t.Helper()
	clipboard := &selectionTestClipboard{err: hostclipboard.ErrNoImage}
	original := readSpriteClipboardImagePNG
	readSpriteClipboardImagePNG = func() ([]byte, error) { return clipboard.data, clipboard.err }
	t.Cleanup(func() { readSpriteClipboardImagePNG = original })
	return clipboard
}

// Four identical RGB colors have distinct authored indices. Internal copies
// must preserve those indices and transparency rather than quantize a PNG.
func newSelectionClipboardGame(t *testing.T) (*Game, []int) {
	t.Helper()
	g := newTestGame(t, 32, 32, 2, 2)
	t.Cleanup(g.closeNativePerformance)
	setClipboardTestPalette(g)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if _, err := g.model.AddObjPage(); err != nil {
		t.Fatal(err)
	}
	pixels := make([]int, 16*16)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if (x+y)%7 != 0 {
				pixels[y*16+x] = 4 + y/8*2 + x/8
			}
		}
	}
	if _, err := g.model.SetObjSpritePixelBlock(2, 2, 16, 16, pixels); err != nil {
		t.Fatal(err)
	}
	cols := g.spriteCols()
	for i := range g.model.ObjSheet.Sprites {
		sprite := &g.model.ObjSheet.Sprites[i]
		sprite.Flags = 1 << uint((sprite.ID/cols-2)*2+sprite.ID%cols-2)
	}
	g.model.Poses = assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{
		{Name: "idle", Parts: []assetsync.PosePartRecord{{Tile: 2*cols + 2, X: -8, Y: 16, FlipX: true}, {Tile: 2*cols + 3, Y: 16}, {Tile: 3*cols + 2, X: -8, Y: 24, FlipY: true}, {Tile: 3*cols + 3, Y: 24}}},
		{Name: "shared", Parts: []assetsync.PosePartRecord{{Tile: 2*cols + 2}, {Tile: 2*cols + 2, X: 8}}},
	}}
	loop := false
	g.model.Animation.Clips = []assetsync.AnimationClip{{Name: "idle", Frames: []int{1, 0, 1}, FPS: 12, Loop: &loop}}
	g.selectObjPage(0)
	g.currentSprite, g.gridCols, g.gridRows, g.selectedPose = 2*cols+2, 2, 2, 0
	g.dirty = false
	g.undo.reset(g.snapshot())
	return g, pixels
}

func selectionObjectBlock(g *Game, col, row, width, height int) []int {
	pixels := make([]int, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixels[y*width+x] = spriteSheetPixel(*g.model.ObjSheet, col*8+x, row*8+y)
		}
	}
	return pixels
}

func selectionObjectFlags(g *Game, tile int) int {
	for _, sprite := range g.model.ObjSheet.Sprites {
		if sprite.ID == tile {
			return sprite.Flags
		}
	}
	return 0
}

func assertSelectionModelUnchanged(t *testing.T, g *Game, before undoSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(g.model.Sheet, before.Sheet) || !reflect.DeepEqual(g.model.ObjSheet, before.ObjSheet) || !reflect.DeepEqual(g.model.Poses, before.Poses) || !reflect.DeepEqual(g.model.Animation, before.Animation) {
		t.Fatal("clipboard action unexpectedly changed artwork, poses, or clips")
	}
}

func TestSelectionClipboardCopyStartsGhostWithoutMutationOrDithering(t *testing.T) {
	stubSelectionClipboard(t)
	g, pixels := newSelectionClipboardGame(t)
	before, history := g.snapshot().clone(), len(g.undo.past)
	g.copyCurrentSprite()
	g.pasteSpriteClipboard()
	if g.spritePaste == nil || !g.spritePaste.selection || g.spritePaste.cut != nil || g.spritePaste.pixelWidth != 16 || g.spritePaste.pixelHeight != 16 {
		t.Fatalf("copied selection did not start a 16x16 copy preview: %+v", g.spritePaste)
	}
	if !reflect.DeepEqual(g.spritePaste.pixels, pixels) {
		t.Fatal("internal copy changed exact palette indices or transparency")
	}
	g.toggleSpritePasteDither()
	if g.spritePaste.dither || !reflect.DeepEqual(g.spritePaste.pixels, pixels) || strings.Contains(strings.ToLower(g.spritePastePlacementStatus()), "dither") {
		t.Fatal("internal tiles were offered dithering or requantized")
	}
	g.moveSpritePastePlacement(10, 12)
	assertSelectionModelUnchanged(t, g, before)
	if g.dirty || len(g.undo.past) != history {
		t.Fatal("copying or moving the ghost changed dirty state or undo history")
	}
}

func TestSelectionClipboardClickCopiesIndependentlyWithOneUndo(t *testing.T) {
	stubSelectionClipboard(t)
	g, pixels := newSelectionClipboardGame(t)
	before, history := g.snapshot().clone(), len(g.undo.past)
	g.copyCurrentSprite()
	if err := g.beginCopiedSpritePaste(); err != nil {
		t.Fatal(err)
	}
	g.moveSpritePastePlacement(10, 12)
	if err := g.commitSpritePastePlacement(); err != nil {
		t.Fatal(err)
	}
	if g.spritePaste != nil || !g.dirty || len(g.undo.past) != history+1 || g.selectedPose != -1 {
		t.Fatal("copy click did not finish one independent, undoable edit")
	}
	if !reflect.DeepEqual(selectionObjectBlock(g, 10, 12, 16, 16), pixels) || !reflect.DeepEqual(selectionObjectBlock(g, 2, 2, 16, 16), pixels) || !reflect.DeepEqual(g.model.Poses, before.Poses) || !reflect.DeepEqual(g.model.Animation, before.Animation) {
		t.Fatal("copy changed source artwork or existing pose/clip recipes")
	}
	if g.currentSprite != 12*g.spriteCols()+10 || g.gridCols != 2 || g.gridRows != 2 {
		t.Fatal("copy did not select its destination rectangle")
	}
	state, ok := g.undo.undo()
	if !ok {
		t.Fatal("copy cannot be undone")
	}
	g.restore(state)
	assertSelectionModelUnchanged(t, g, before)
	state, ok = g.undo.redo()
	if !ok {
		t.Fatal("copy cannot be redone")
	}
	g.restore(state)
	if g.selectedPose != -1 {
		t.Fatal("redoing an independent copy selected an existing pose that Update Pose could overwrite")
	}
	if err := g.model.SetObjSpritePixel(12*g.spriteCols()+10, 1, 0, 13); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selectionObjectBlock(g, 2, 2, 16, 16), pixels) || g.copiedSprite.pixel(1, 0) != pixels[1] {
		t.Fatal("editing the pasted copy changed its source or clipboard buffer")
	}
}

func TestSelectionClipboardCutCancelNeverChangesSource(t *testing.T) {
	stubSelectionClipboard(t)
	g, _ := newSelectionClipboardGame(t)
	before, history := g.snapshot().clone(), len(g.undo.past)
	g.cutCurrentSprite()
	assertSelectionModelUnchanged(t, g, before)
	if g.copiedSprite.cut == nil || g.dirty || len(g.undo.past) != history {
		t.Fatal("cut must only capture a deferred move")
	}
	g.pasteSpriteClipboard()
	if g.spritePaste == nil || g.spritePaste.cut == nil {
		t.Fatal("cut paste did not retain the deferred move")
	}
	g.moveSpritePastePlacement(20, 10)
	g.cancelSpritePastePlacement()
	assertSelectionModelUnchanged(t, g, before)
	if g.spritePaste != nil || g.copiedSprite.cut != nil || g.dirty || len(g.undo.past) != history {
		t.Fatal("cancel did not clear the pending cut without an edit")
	}
}

func TestSelectionClipboardCutAcrossPagesRemapsSharedPosesAndUndo(t *testing.T) {
	stubSelectionClipboard(t)
	g, pixels := newSelectionClipboardGame(t)
	before, history := g.snapshot().clone(), len(g.undo.past)
	previews := []editor.PosePreview{editor.ComposePose(*g.model.ObjSheet, g.model.Poses.Poses[0]), editor.ComposePose(*g.model.ObjSheet, g.model.Poses.Poses[1])}
	g.cutCurrentSprite()
	g.selectObjPage(1)
	destCol, destRow := 3, g.currentObjPageRect().Y/8+4
	g.pasteSpriteClipboard()
	if g.spritePaste == nil || g.spritePaste.cut == nil {
		t.Fatal("changing pages discarded the pending cut")
	}
	g.moveSpritePastePlacement(destCol, destRow)
	if err := g.commitSpritePastePlacement(); err != nil {
		t.Fatal(err)
	}
	if g.spritePaste != nil || g.copiedSprite.cut != nil || len(g.undo.past) != history+1 {
		t.Fatal("move did not finish as one undoable edit")
	}
	if !reflect.DeepEqual(selectionObjectBlock(g, destCol, destRow, 16, 16), pixels) || !reflect.DeepEqual(selectionObjectBlock(g, 2, 2, 16, 16), make([]int, 16*16)) {
		t.Fatal("move lost the destination pixels or failed to clear its source")
	}
	cols := g.spriteCols()
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if selectionObjectFlags(g, (2+y)*cols+2+x) != 0 || selectionObjectFlags(g, (destRow+y)*cols+destCol+x) != 1<<uint(y*2+x) {
				t.Fatal("move did not transfer source flags")
			}
		}
	}
	for id, pose := range g.model.Poses.Poses {
		if pose.Name != before.Poses.Poses[id].Name || !reflect.DeepEqual(previews[id], editor.ComposePose(*g.model.ObjSheet, pose)) {
			t.Fatal("moving tiles changed a registered pose's name or drawn pixels")
		}
		for index, part := range pose.Parts {
			want := before.Poses.Poses[id].Parts[index]
			want.Tile = (want.Tile/cols-2+destRow)*cols + want.Tile%cols - 2 + destCol
			if part != want {
				t.Fatalf("pose %d part %d = %+v, want %+v", id, index, part, want)
			}
		}
	}
	if !reflect.DeepEqual(g.model.Animation.Clips, before.Animation.Clips) {
		t.Fatal("moving source tiles changed clip pose IDs, order, speed or loop")
	}
	moved := g.snapshot().clone()
	state, ok := g.undo.undo()
	if !ok {
		t.Fatal("move cannot be undone")
	}
	g.restore(state)
	assertSelectionModelUnchanged(t, g, before)
	state, ok = g.undo.redo()
	if !ok {
		t.Fatal("move cannot be redone")
	}
	g.restore(state)
	assertSelectionModelUnchanged(t, g, moved)
}

func TestSelectionClipboardOverlappingCutUsesOriginalPixels(t *testing.T) {
	stubSelectionClipboard(t)
	g, pixels := newSelectionClipboardGame(t)
	before := g.snapshot().clone()
	g.cutCurrentSprite()
	if err := g.beginCopiedSpritePaste(); err != nil {
		t.Fatal(err)
	}
	g.moveSpritePastePlacement(3, 2)
	if err := g.commitSpritePastePlacement(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selectionObjectBlock(g, 3, 2, 16, 16), pixels) || !reflect.DeepEqual(selectionObjectBlock(g, 2, 2, 8, 16), make([]int, 8*16)) {
		t.Fatal("overlapping move erased or reread already moved pixels")
	}
	for id, pose := range g.model.Poses.Poses {
		for index, part := range pose.Parts {
			want := before.Poses.Poses[id].Parts[index]
			want.Tile++
			if part != want {
				t.Fatal("overlapping move remapped a source reference more than once")
			}
		}
	}
}

func TestSelectionClipboardStaleCutRejectsWithoutOverwriting(t *testing.T) {
	for _, change := range []string{"pixels", "flags"} {
		t.Run(change, func(t *testing.T) {
			stubSelectionClipboard(t)
			g, _ := newSelectionClipboardGame(t)
			g.cutCurrentSprite()
			if err := g.beginCopiedSpritePaste(); err != nil {
				t.Fatal(err)
			}
			g.moveSpritePastePlacement(10, 12)
			if change == "pixels" {
				if err := g.model.SetObjSpritePixel(2*g.spriteCols()+2, 1, 0, 13); err != nil {
					t.Fatal(err)
				}
			} else {
				g.model.ObjSheet.Sprites[0].Flags ^= 16
			}
			before, history := g.snapshot().clone(), len(g.undo.past)
			if err := g.commitSpritePastePlacement(); err == nil {
				t.Fatal("cut committed after its captured source changed")
			}
			assertSelectionModelUnchanged(t, g, before)
			if len(g.undo.past) != history || g.dirty {
				t.Fatal("rejected stale cut created an edit")
			}
		})
	}
}

func selectionClipboardPNG(t *testing.T, clr color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	img.SetNRGBA(0, 0, clr)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestSelectionClipboardInternalCopyWinsOldPNGAndNewPNGWinsAgain(t *testing.T) {
	clipboard := stubSelectionClipboard(t)
	clipboard.data, clipboard.err = selectionClipboardPNG(t, color.NRGBA{R: 255, A: 255}), nil
	g, pixels := newSelectionClipboardGame(t)
	g.copyCurrentSprite()
	g.pasteSpriteClipboard()
	if g.spritePaste == nil || !g.spritePaste.selection || !reflect.DeepEqual(g.spritePaste.pixels, pixels) {
		t.Fatal("unchanged older desktop PNG replaced the newly copied selection")
	}
	g.cancelSpritePastePlacement()
	clipboard.data = selectionClipboardPNG(t, color.NRGBA{G: 255, A: 255})
	g.pasteSpriteClipboard()
	if g.spritePaste == nil || g.spritePaste.selection || g.spritePaste.pixelWidth != 8 || g.spritePaste.pixelHeight != 8 || g.spritePaste.pixels[0] != 2 {
		t.Fatal("new desktop PNG did not replace the older internal selection")
	}
}

func TestSelectionClipboardPendingCutWithoutPreviewIsCancelledByContextChanges(t *testing.T) {
	for _, action := range []string{"restore", "undo", "bank change"} {
		t.Run(action, func(t *testing.T) {
			stubSelectionClipboard(t)
			g, pixels := newSelectionClipboardGame(t)
			before := g.snapshot().clone()
			if action == "undo" {
				if err := g.model.SetObjSpritePixel(20*g.spriteCols()+30, 0, 0, 3); err != nil {
					t.Fatal(err)
				}
				g.undo.forcePush(g.snapshot())
			}
			g.cutCurrentSprite()
			if g.copiedSprite.cut == nil || g.spritePaste != nil {
				t.Fatal("fixture requires a pending cut without a placement preview")
			}
			switch action {
			case "restore":
				g.restore(before)
			case "undo":
				state, ok := g.undo.undo()
				if !ok {
					t.Fatal("fixture edit cannot be undone")
				}
				g.restore(state)
			case "bank change":
				if err := g.setObjEditing(false); err != nil {
					t.Fatal(err)
				}
				if g.copiedSprite.cut != nil {
					t.Fatal("switching to Background retained an OBJ cut")
				}
				if err := g.setObjEditing(true); err != nil {
					t.Fatal(err)
				}
			}
			if g.copiedSprite.cut != nil || g.spritePaste != nil {
				t.Fatal("context change retained a deferred cut without a preview")
			}
			assertSelectionModelUnchanged(t, g, before)
			if !reflect.DeepEqual(selectionObjectBlock(g, 2, 2, 16, 16), pixels) {
				t.Fatal("cancelling the pending cut changed its source")
			}
			g.pasteSpriteClipboard()
			if g.spritePaste == nil || g.spritePaste.cut != nil || !g.spritePaste.selection {
				t.Fatal("clipboard reuse after cancellation must make a copy, not move the old source")
			}
		})
	}
}

func TestSelectionClipboardPageAddPreservesPendingCutForNewPage(t *testing.T) {
	stubSelectionClipboard(t)
	g, pixels := newSelectionClipboardGame(t)
	before, history, pages := g.snapshot().clone(), len(g.undo.past), len(g.model.Animation.Pages)
	poseBefore := editor.ComposePose(*g.model.ObjSheet, g.model.Poses.Poses[0])
	g.cutCurrentSprite()
	if err := g.addObjPage(); err != nil {
		t.Fatal(err)
	}
	if g.copiedSprite.cut == nil || g.spritePaste != nil || g.objPage != pages || len(g.model.Animation.Pages) != pages+1 {
		t.Fatal("PAGE+ discarded the pending cut or failed to select the new page")
	}
	if !reflect.DeepEqual(selectionObjectBlock(g, 2, 2, 16, 16), pixels) || !reflect.DeepEqual(g.model.Poses, before.Poses) || len(g.undo.past) != history+1 {
		t.Fatal("PAGE+ moved source art before the destination click")
	}
	pageAdded := g.snapshot().clone()
	destCol, destRow := 4, g.currentObjPageRect().Y/8+3
	g.pasteSpriteClipboard()
	if g.spritePaste == nil || g.spritePaste.cut == nil {
		t.Fatal("new page did not accept the pending cut preview")
	}
	g.moveSpritePastePlacement(destCol, destRow)
	if err := g.commitSpritePastePlacement(); err != nil {
		t.Fatal(err)
	}
	if g.copiedSprite.cut != nil || g.spritePaste != nil || len(g.undo.past) != history+2 {
		t.Fatal("move onto the new page did not complete as one additional edit")
	}
	if !reflect.DeepEqual(selectionObjectBlock(g, destCol, destRow, 16, 16), pixels) || !reflect.DeepEqual(selectionObjectBlock(g, 2, 2, 16, 16), make([]int, 16*16)) || !reflect.DeepEqual(poseBefore, editor.ComposePose(*g.model.ObjSheet, g.model.Poses.Poses[0])) || !reflect.DeepEqual(g.model.Animation.Clips, before.Animation.Clips) {
		t.Fatal("move onto the new page changed pixels or pose/clip playback")
	}
	state, ok := g.undo.undo()
	if !ok {
		t.Fatal("new-page move cannot be undone")
	}
	g.restore(state)
	assertSelectionModelUnchanged(t, g, pageAdded)
}

func TestSelectionClipboardReadRecoveryDoesNotReplaceNewInternalCopy(t *testing.T) {
	clipboard := stubSelectionClipboard(t)
	clipboard.err = errors.New("temporary clipboard access failure")
	g, pixels := newSelectionClipboardGame(t)
	before, history := g.snapshot().clone(), len(g.undo.past)
	g.copyCurrentSprite()
	// The first successful read could still be an image copied before the
	// internal selection. Without a baseline hash it cannot prove a new PNG.
	clipboard.data = selectionClipboardPNG(t, color.NRGBA{R: 255, A: 255})
	clipboard.err = nil
	g.pasteSpriteClipboard()
	if g.spritePaste == nil || !g.spritePaste.selection || g.spritePaste.pixelWidth != 16 || !reflect.DeepEqual(g.spritePaste.pixels, pixels) {
		t.Fatal("clipboard recovery replaced the new internal copy with an unverified desktop image")
	}
	assertSelectionModelUnchanged(t, g, before)
	if g.dirty || len(g.undo.past) != history {
		t.Fatal("starting the recovered copy preview mutated the project")
	}
	g.cancelSpritePastePlacement()
	clipboard.data = selectionClipboardPNG(t, color.NRGBA{G: 255, A: 255})
	g.pasteSpriteClipboard()
	if g.spritePaste == nil || g.spritePaste.selection || g.spritePaste.pixelWidth != 8 || g.spritePaste.pixelHeight != 8 || g.spritePaste.pixels[0] != 2 {
		t.Fatal("a changed PNG did not take priority after clipboard recovery established its baseline")
	}
	assertSelectionModelUnchanged(t, g, before)
	if g.dirty || len(g.undo.past) != history {
		t.Fatal("starting the subsequent PNG preview mutated the project")
	}
}
