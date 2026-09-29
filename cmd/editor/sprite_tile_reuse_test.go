package main

import (
	"reflect"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
)

func objectPageFirstTileForTest(g *Game, page int) int {
	p := g.model.Animation.Pages[page]
	return p.Y/8*(g.model.ObjSheet.Width/8) + p.X/8
}

func newTileReuseTestGame(t *testing.T) *Game {
	t.Helper()
	g := newTestGame(t, 32, 32, 2, 2)
	t.Cleanup(g.closeNativePerformance)
	if err := g.model.CreateObjSheet(editor.ObjPageWidth, editor.ObjPageHeight); err != nil {
		t.Fatal(err)
	}
	if _, err := g.model.AddObjPage(); err != nil {
		t.Fatal(err)
	}
	secondPage := objectPageFirstTileForTest(g, 1)
	for _, id := range []int{0, 1, secondPage} {
		if err := g.model.SetObjSpritePixel(id, 0, 0, 3); err != nil {
			t.Fatal(err)
		}
		if err := g.model.SetObjSpritePixel(id, 7, 7, 4); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.model.SetObjSpritePixel(16, 0, 0, 5); err != nil {
		t.Fatal(err)
	}
	g.model.Poses = assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{
		{Parts: []assetsync.PosePartRecord{{Tile: 0}, {Tile: 0, X: 8}}},
		{Parts: []assetsync.PosePartRecord{{Tile: 1}}},
		{Parts: []assetsync.PosePartRecord{{Tile: secondPage, FlipX: true}}},
		{Parts: []assetsync.PosePartRecord{{Tile: 16}}},
		{Parts: []assetsync.PosePartRecord{{Tile: 0}, {Tile: 16, X: 8}}},
		{Parts: []assetsync.PosePartRecord{{Tile: 2}}},
	}}
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.objShowReuse = true
	g.currentSprite = 0
	g.undo.reset(g.snapshot())
	return g
}

func TestObjectTileReuseMatchesArtAcrossPagesAndPoses(t *testing.T) {
	g := newTileReuseTestGame(t)
	secondPage := objectPageFirstTileForTest(g, 1)
	u := g.objectTileUsage()
	if u != g.objectTileUsage() {
		t.Fatal("unchanged artwork did not reuse its analysis cache")
	}
	if u.analysis.PaintedTiles != 4 || u.analysis.UniqueTiles != 2 || u.analysis.SavedBytes != 128 {
		t.Fatalf("painted/unique/saved = %d/%d/%d, want 4/2/128", u.analysis.PaintedTiles, u.analysis.UniqueTiles, u.analysis.SavedBytes)
	}
	if !reflect.DeepEqual(u.groups[0], []int{0, 1, secondPage}) {
		t.Fatalf("matching logical tiles = %v", u.groups[0])
	}
	if !reflect.DeepEqual(u.poses[0], map[int]bool{0: true, 1: true, 2: true, 4: true}) {
		t.Fatalf("matching global pose IDs = %v", u.poses[0])
	}
	// Repeated parts count once per pose; equal pixels in another logical
	// tile must not inflate the warning about edits to this exact tile.
	if want := (map[int]int{0: 2, 1: 1, 16: 2, secondPage: 1}); !reflect.DeepEqual(u.directReferences, want) {
		t.Fatalf("exact logical tile references = %v, want %v", u.directReferences, want)
	}
	for _, tile := range []int{0, 1, secondPage} {
		g.currentSprite = tile
		if g.selectedTileCanonical() != 0 || !g.objPageSharesSelectedTile(0) || !g.objPageSharesSelectedTile(1) {
			t.Fatalf("tile %d did not find equal artwork on both pages", tile)
		}
		for id := range g.authoredPoses() {
			if got, want := g.poseSharesSelectedTile(id), u.poses[0][id]; got != want {
				t.Fatalf("tile %d pose %d match = %v, want %v", tile, id, got, want)
			}
		}
	}
	if _, exists := u.groups[-1]; exists {
		t.Fatal("transparent cells were collected as duplicate artwork")
	}
	for _, tile := range []int{2, -1, g.spriteN()} {
		g.currentSprite = tile
		if g.selectedTileCanonical() != -1 || g.objPageSharesSelectedTile(0) || g.objPageSharesSelectedTile(1) {
			t.Fatalf("transparent or invalid tile %d produced matches", tile)
		}
		for id := range g.authoredPoses() {
			if g.poseSharesSelectedTile(id) {
				t.Fatalf("transparent or invalid tile %d highlighted pose %d", tile, id)
			}
		}
	}
	g.currentSprite, g.objShowReuse = 0, false
	if g.poseSharesSelectedTile(0) {
		t.Fatal("disabled reuse highlighting still highlighted a frame")
	}
}

func TestObjectTileReuseImagePasteAndUndoRefreshIndependentCopies(t *testing.T) {
	g := newTileReuseTestGame(t)
	secondPage := objectPageFirstTileForTest(g, 1)
	before := g.objectTileUsage()
	beforePoses := g.snapshot().clone().Poses
	g.animation.previews = map[int]editor.PosePreview{0: {Width: 1, Height: 1, Pixels: []int{3}}}
	g.dirty = false
	pixels := make([]int, 64)
	pixels[0], pixels[63] = 9, 4
	if err := g.applySpritePaste(&spritePastePlacement{pixels: pixels, pixelWidth: 8, pixelHeight: 8}, 0, 0); err != nil {
		t.Fatal(err)
	}
	after := g.objectTileUsage()
	if after == before || after.analysis.UniqueTiles != 3 || after.analysis.SavedBytes != 64 {
		t.Fatalf("image paste retained stale reuse analysis: same cache %v, unique %d, saved %d", after == before, after.analysis.UniqueTiles, after.analysis.SavedBytes)
	}
	if !g.dirty || g.animation.previews != nil {
		t.Fatal("image paste did not invalidate the live animation preview")
	}
	if !reflect.DeepEqual(g.model.Poses, beforePoses) {
		t.Fatal("editing artwork changed authored logical pose references")
	}
	if !reflect.DeepEqual(after.groups[1], []int{1, secondPage}) || !reflect.DeepEqual(after.groups[0], []int{0}) {
		t.Fatal("editing one logical copy changed the other copies")
	}
	for _, tile := range []int{1, secondPage} {
		x, y := tile%g.spriteCols()*8, tile/g.spriteCols()*8
		if spriteSheetPixel(*g.model.ObjSheet, x, y) != 3 || spriteSheetPixel(*g.model.ObjSheet, x+7, y+7) != 4 {
			t.Fatalf("independent logical copy %d was changed", tile)
		}
	}
	snapshot, ok := g.undo.undo()
	if !ok {
		t.Fatal("image paste did not record undo history")
	}
	g.restore(snapshot)
	restored := g.objectTileUsage()
	if restored == after || restored.analysis.UniqueTiles != 2 || restored.analysis.SavedBytes != 128 || !reflect.DeepEqual(restored.groups[0], []int{0, 1, secondPage}) {
		t.Fatal("undo retained reuse data from the edited artwork")
	}
	snapshot, ok = g.undo.redo()
	if !ok {
		t.Fatal("image paste undo did not retain redo history")
	}
	g.restore(snapshot)
	if got := g.objectTileUsage(); got.analysis.UniqueTiles != 3 || got.analysis.SavedBytes != 64 {
		t.Fatal("redo retained reuse data from the original artwork")
	}
}

func TestObjectTileReusePoseEditAndUndoRefreshReferences(t *testing.T) {
	g := newTileReuseTestGame(t)
	g.selectPose(3)
	g.currentSprite = 0
	before := g.objectTileUsage()
	if before.poses[0][3] {
		t.Fatal("fixture pose unexpectedly references matching artwork")
	}
	g.togglePoseTile(0)
	after := g.objectTileUsage()
	if after == before || !g.poseSharesSelectedTile(3) || len(after.poses[0]) != 5 || after.directReferences[0] != 3 {
		t.Fatal("adding a pose part did not refresh tile-to-pose matches")
	}
	snapshot, ok := g.undo.undo()
	if !ok {
		t.Fatal("pose edit did not record undo history")
	}
	g.restore(snapshot)
	g.currentSprite = 0
	if g.poseSharesSelectedTile(3) || len(g.objectTileUsage().poses[0]) != 4 || g.objectTileUsage().directReferences[0] != 2 {
		t.Fatal("pose edit undo retained stale tile-to-pose matches")
	}
}
