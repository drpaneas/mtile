package editor

import (
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func tileLinkModel(t *testing.T) *Model {
	t.Helper()
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	m.Poses.Poses = []assetsync.PoseRecord{
		{Name: "a", Parts: []assetsync.PosePartRecord{{Tile: 1}}},
		{Name: "b", Parts: []assetsync.PosePartRecord{{Tile: 2}}},
		{Name: "c", Parts: []assetsync.PosePartRecord{{Tile: 3}}},
	}
	m.Animation.Clips = []assetsync.AnimationClip{{Name: "sway", Frames: []int{0, 1}, FPS: 6}}
	return m
}

func TestTileLinksLinkUnlinkAndSave(t *testing.T) {
	m := tileLinkModel(t)
	if err := m.LinkTileToPose(7, 2); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkTileToAnimation(5, "sway"); err != nil {
		t.Fatal(err)
	}
	// Relinking replaces the tile's link instead of adding a second one.
	if err := m.LinkTileToPose(5, 0); err != nil {
		t.Fatal(err)
	}
	if link, ok := m.TileLinkFor(5); !ok || link.Pose == nil || *link.Pose != 0 || link.Animation != "" {
		t.Fatalf("tile 5 link = %+v, %t; want pose 0", link, ok)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(m.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.TileLinks.Links) != 2 || reopened.TileLinks.Links[0].Tile != 5 || reopened.TileLinks.Links[1].Tile != 7 {
		t.Fatalf("reopened links = %+v", reopened.TileLinks.Links)
	}
	if !reopened.UnlinkTile(7) || reopened.UnlinkTile(7) {
		t.Fatal("UnlinkTile must report removal exactly once")
	}
}

func TestTileLinksRejectReservedAndMissingTargets(t *testing.T) {
	m := tileLinkModel(t)
	if err := m.LinkTileToPose(0, 0); err == nil || !strings.Contains(err.Error(), "map eraser") {
		t.Fatalf("linking tile 0: %v", err)
	}
	if err := m.LinkTileToPose(4, 9); err == nil {
		t.Fatal("linked a missing pose")
	}
	if err := m.LinkTileToAnimation(4, "walk"); err == nil {
		t.Fatal("linked a missing clip")
	}
}

func TestTileLinksFollowPoseDeletionAndClipEdits(t *testing.T) {
	m := tileLinkModel(t)
	for tile, pose := range map[int]int{4: 0, 5: 1, 6: 2} {
		if err := m.LinkTileToPose(tile, pose); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.LinkTileToAnimation(8, "sway"); err != nil {
		t.Fatal(err)
	}
	snapshot := m.TileLinks
	RemovePoseFromTileLinks(&m.TileLinks, 1)
	if _, ok := m.TileLinkFor(5); ok {
		t.Fatal("link to the deleted pose survived")
	}
	if link, _ := m.TileLinkFor(6); *link.Pose != 1 {
		t.Fatalf("link to a later pose = %d, want it shifted to 1", *link.Pose)
	}
	if len(snapshot.Links) != 4 || *snapshot.Links[2].Pose != 2 {
		t.Fatal("pose deletion mutated a shared undo snapshot")
	}
	RenameAnimationInTileLinks(&m.TileLinks, "sway", "breeze")
	if link, _ := m.TileLinkFor(8); link.Animation != "breeze" {
		t.Fatalf("renamed clip link = %q", link.Animation)
	}
	RemoveAnimationFromTileLinks(&m.TileLinks, "breeze")
	if _, ok := m.TileLinkFor(8); ok {
		t.Fatal("link to the deleted clip survived")
	}
}

// The Windy Meadow example must be reproducible in Sprite Lab: every pose is
// exactly what REGISTER POSE records for a rectangle on the Poses page, and
// the project opens without an object-sheet migration.
func TestTileLinksExamplePosesAreEditorCaptures(t *testing.T) {
	m, err := Open("../../examples/tile-links")
	if err != nil {
		t.Fatal(err)
	}
	if m.ObjSheet == nil {
		t.Fatal("example poses must live on the Poses page (objsheet.json)")
	}
	cols := m.ObjSheet.Width / 8
	for i, pose := range m.Poses.Poses {
		minCol, minRow, maxCol, maxRow := 1<<30, 1<<30, -1, -1
		for _, part := range pose.Parts {
			col, row := part.Tile%cols, part.Tile/cols
			minCol, minRow = min(minCol, col), min(minRow, row)
			maxCol, maxRow = max(maxCol, col), max(maxRow, row)
		}
		captured, err := CapturePose(*m.ObjSheet, minCol, minRow, maxCol-minCol+1, maxRow-minRow+1)
		if err != nil {
			t.Fatalf("pose %d: %v", i, err)
		}
		if len(captured.Parts) != len(pose.Parts) {
			t.Fatalf("pose %d %q has %d parts; REGISTER POSE on its rectangle gives %d", i, pose.Name, len(pose.Parts), len(captured.Parts))
		}
		for j := range pose.Parts {
			if pose.Parts[j] != captured.Parts[j] {
				t.Fatalf("pose %d %q part %d = %+v; REGISTER POSE gives %+v", i, pose.Name, j, pose.Parts[j], captured.Parts[j])
			}
		}
	}
	before := m.Poses
	if err := m.NormalizeObjPages(); err != nil {
		t.Fatal(err)
	}
	for i := range before.Poses {
		for j := range before.Poses[i].Parts {
			if before.Poses[i].Parts[j] != m.Poses.Poses[i].Parts[j] {
				t.Fatal("opening Poses would migrate the example's object sheet")
			}
		}
	}
}
