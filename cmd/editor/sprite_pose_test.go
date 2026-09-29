package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
)

func TestPoseStripHiddenUntilPoseBankIsActive(t *testing.T) {
	g := newTestGameWithPoses(t)
	full := g.sheetGridCanvas()
	if g.showsPoseStrip() || g.poseStripRect().H != 0 {
		t.Fatal("BG sheet must not show object-bank pspr frames")
	}
	wantFull := editor.Rect{
		X: sheetPanelRect.X + 10,
		Y: spritePanelContentY(sheetPanelRect),
		W: sheetPanelRect.W - 20,
		H: sheetPanelRect.H - spritePanelContentOffset() - 10,
	}
	if full != wantFull {
		t.Fatalf("BG grid canvas = %#v, want %#v", full, wantFull)
	}
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if !g.showsPoseStrip() {
		t.Fatal("OBJ sheet should show authored pspr frames")
	}
	canvas := g.sheetGridCanvas()
	strip := g.poseStripRect()
	if canvas.H >= full.H {
		t.Fatalf("pose strip did not shorten the sheet canvas: %d vs %d", canvas.H, full.H)
	}
	if strip.Y < canvas.Y+canvas.H || strip.Y+strip.H > sheetPanelRect.Y+sheetPanelRect.H {
		t.Fatalf("pose strip %#v escapes sheet panel under canvas %#v", strip, canvas)
	}
	if rectsOverlapIDE(canvas, strip) {
		t.Fatal("pose strip overlaps the spritesheet grid")
	}
}

func TestPoseStripSelectsPsprFrameAndItsTiles(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if g.selectedPose != 0 {
		t.Fatalf("opening the pose bank selected %d, want pspr 0", g.selectedPose)
	}
	g.selectPose(1)
	if g.selectedPose != 1 {
		t.Fatalf("selected pose = %d, want 1", g.selectedPose)
	}
	if g.currentSprite != 6 {
		t.Fatalf("selecting pspr 1 should focus tile 6, got %d", g.currentSprite)
	}
	if !g.poseHighlightsTile(6) || g.poseHighlightsTile(0) {
		t.Fatal("selected frame should highlight only its own sheet tiles")
	}
	strip := g.poseStripRect()
	cell := g.poseStripCellRect(1)
	if cell.W <= 0 || !rectInsideIDE(cell, strip) {
		t.Fatalf("pspr 1 cell %#v is not inside strip %#v", cell, strip)
	}
	if g.poseIndexAt(cell.X+cell.W/2, cell.Y+cell.H/2) != 1 {
		t.Fatalf("clicking cell 1 did not resolve to pspr 1")
	}
	if editor.PoseFrameLabel(1, g.model.Poses.Poses[1]) != "1  idle_1" {
		t.Fatalf("visible label = %q", editor.PoseFrameLabel(1, g.model.Poses.Poses[1]))
	}
}

func TestSheetClickSelectsOwningPsprFrame(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.syncPoseSelectionFromSprite(6)
	if g.selectedPose != 1 {
		t.Fatalf("tile 6 should select pspr 1, got %d", g.selectedPose)
	}
}

func TestEmptyObjectSheetShowsPoseEditor(t *testing.T) {
	g := newTestGame(t, 128, 64, 16, 16)
	if g.showsPoseStrip() {
		t.Fatal("BG tileset without poses should not steal space for the filmstrip")
	}
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if !g.showsPoseStrip() {
		t.Fatal("OBJ sheet should show the pspr editor even before any frames exist")
	}
	if g.selectedPose != -1 {
		t.Fatalf("empty pose bank selected %d", g.selectedPose)
	}
}

func TestCaptureAndDeletePoseFromSelection(t *testing.T) {
	g := newTestGame(t, 128, 64, 16, 16)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.currentSprite = 2
	g.gridCols = 2
	g.gridRows = 2
	cols := g.spriteCols()
	for _, id := range []int{2, 3, 2 + cols, 3 + cols} {
		if err := g.model.SetObjSpritePixel(id, 0, 0, 4); err != nil {
			t.Fatal(err)
		}
	}
	g.capturePoseFromSelection(false)
	if len(g.authoredPoses()) != 1 {
		t.Fatalf("NEW created %d poses", len(g.authoredPoses()))
	}
	if g.authoredPoses()[0].Name != "idle_0" {
		t.Fatalf("NEW name = %q, want idle_0", g.authoredPoses()[0].Name)
	}
	parts := g.authoredPoses()[0].Parts
	if len(parts) != 4 || parts[0].Tile != 2 || parts[1].Tile != 3 || parts[2].X != 0 || parts[2].Y != 8 {
		t.Fatalf("captured parts = %+v", parts)
	}
	g.persist()
	reloaded, err := editor.Open(g.model.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Poses.Poses) != 1 || reloaded.Poses.Poses[0].Parts[0].Tile != 2 {
		t.Fatalf("saved poses = %+v", reloaded.Poses)
	}
	g.deleteSelectedPose()
	if len(g.authoredPoses()) != 0 {
		t.Fatalf("DEL left %d poses", len(g.authoredPoses()))
	}
	if s, ok := g.undo.undo(); !ok {
		t.Fatal("undo after delete")
	} else {
		g.restore(s)
	}
	if len(g.authoredPoses()) != 1 {
		t.Fatalf("undo did not restore the pose: %d", len(g.authoredPoses()))
	}
}

func TestReplacePoseKeepsNameAndToggleAddsTile(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.selectPose(0)
	if err := g.model.SetObjSpritePixel(10, 0, 0, 4); err != nil {
		t.Fatal(err)
	}
	g.currentSprite = 10
	g.gridCols = 1
	g.gridRows = 1
	g.capturePoseFromSelection(true)
	if g.authoredPoses()[0].Name != "idle_0" {
		t.Fatalf("SET dropped the name: %+v", g.authoredPoses()[0])
	}
	if len(g.authoredPoses()[0].Parts) != 1 || g.authoredPoses()[0].Parts[0].Tile != 10 {
		t.Fatalf("SET parts = %+v", g.authoredPoses()[0].Parts)
	}
	g.togglePoseTile(11)
	if !editor.PoseContainsTile(g.authoredPoses()[0], 11) {
		t.Fatal("Shift+click should add tile 11")
	}
	g.togglePoseTile(11)
	if editor.PoseContainsTile(g.authoredPoses()[0], 11) {
		t.Fatal("second toggle should remove tile 11")
	}
}

func TestCapturePoseSkipsBlankTilesAndNamesIdle(t *testing.T) {
	g := newTestGame(t, 128, 64, 16, 16)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetObjSpritePixel(2, 0, 0, 4); err != nil {
		t.Fatal(err)
	}
	g.currentSprite = 2
	g.gridCols = 2
	g.gridRows = 2
	g.capturePoseFromSelection(false)
	parts := g.authoredPoses()[0].Parts
	if len(g.authoredPoses()) != 1 || g.authoredPoses()[0].Name != "idle_0" || len(parts) != 1 || parts[0].Tile != 2 {
		t.Fatalf("painted-only capture = %+v", g.authoredPoses())
	}
}

func TestShiftClickCreatesPoseWhenNoneSelected(t *testing.T) {
	g := newTestGame(t, 128, 64, 16, 16)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if g.selectedPose != -1 {
		t.Fatalf("empty bank selected %d", g.selectedPose)
	}
	g.togglePoseTile(4)
	if len(g.authoredPoses()) != 1 || g.selectedPose != 0 || g.authoredPoses()[0].Name != "idle_0" {
		t.Fatalf("first Shift+click = %+v selected=%d", g.authoredPoses(), g.selectedPose)
	}
	if !editor.PoseContainsTile(g.authoredPoses()[0], 4) {
		t.Fatal("new pose should contain the clicked tile")
	}
	g.togglePoseTile(5)
	if !editor.PoseContainsTile(g.authoredPoses()[0], 5) {
		t.Fatal("second Shift+click should add tile 5")
	}
}

func TestPoseNameEditRenamesSelectedFrame(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	g.selectPose(1)
	g.beginPoseNameEdit()
	if !g.poseNameEditing || g.poseNameDraft != "idle_1" {
		t.Fatalf("rename draft = %q editing=%v", g.poseNameDraft, g.poseNameEditing)
	}
	g.poseNameDraft = "walk_1"
	g.commitPoseNameEdit()
	if g.poseNameEditing || g.authoredPoses()[1].Name != "walk_1" {
		t.Fatalf("renamed pose = %+v editing=%v", g.authoredPoses()[1], g.poseNameEditing)
	}
	g.beginPoseNameEdit()
	g.poseNameDraft = "idle_0"
	g.commitPoseNameEdit()
	if !g.poseNameEditing || g.authoredPoses()[1].Name != "walk_1" {
		t.Fatal("duplicate rename should keep the current name and stay in the field")
	}
}

func TestUndoPoseRegistrationDiscardsItsRenameDraft(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetObjSpritePixel(2, 0, 0, 4); err != nil {
		t.Fatal(err)
	}
	g.selectPose(0)
	g.undo.reset(g.snapshot())
	oldCount, oldName := len(g.authoredPoses()), g.authoredPoses()[0].Name
	g.currentSprite, g.gridCols, g.gridRows = 2, 1, 1
	g.capturePoseFromSelection(false)
	if len(g.authoredPoses()) != oldCount+1 || !g.poseNameEditing {
		t.Fatal("registering a pose did not begin its rename")
	}
	g.poseNameDraft = "new_pose_draft"
	snapshot, ok := g.undo.undo()
	if !ok {
		t.Fatal("pose registration has no undo")
	}
	g.restore(snapshot)
	g.commitPoseNameEdit()
	if g.poseNameEditing || g.poseNameDraft != "" || len(g.authoredPoses()) != oldCount || g.authoredPoses()[0].Name != oldName {
		t.Fatal("unfinished rename changed the pose restored by Undo")
	}
}

func newTestGameWithPoses(t *testing.T) *Game {
	t.Helper()
	g := newTestGame(t, 128, 64, 16, 16)
	if err := g.model.CreateObjSheet(assetsync.DefaultObjSheetW, assetsync.DefaultObjSheetH); err != nil {
		t.Fatal(err)
	}
	if err := g.model.Save(); err != nil {
		t.Fatal(err)
	}
	payload := `{
		"version":1,
		"poses":[
			{"name":"idle_0","parts":[{"tile":0,"x":0,"y":0},{"tile":1,"x":8,"y":0}]},
			{"name":"idle_1","parts":[{"tile":6,"x":0,"y":0},{"tile":7,"x":8,"y":0}]}
		]
	}`
	if err := os.MkdirAll(filepath.Join(g.model.Dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g.model.Dir, "assets", "poses.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	reloaded, err := editor.Open(g.model.Dir)
	if err != nil {
		t.Fatal(err)
	}
	g.model = reloaded
	g.fitSheetCamera()
	return g
}
