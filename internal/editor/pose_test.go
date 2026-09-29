package editor

import (
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestPoseFrameLabelUsesPsprIndexAndName(t *testing.T) {
	if got := PoseFrameLabel(0, assetsync.PoseRecord{Name: "idle_0"}); got != "0  idle_0" {
		t.Fatalf("named pose = %q, want %q", got, "0  idle_0")
	}
	if got := PoseFrameLabel(3, assetsync.PoseRecord{}); got != "3" {
		t.Fatalf("unnamed pose = %q, want %q", got, "3")
	}
}

func TestPoseContainsTileAndFirstMatch(t *testing.T) {
	poses := []assetsync.PoseRecord{
		{Name: "a", Parts: []assetsync.PosePartRecord{{Tile: 2}, {Tile: 5}}},
		{Name: "b", Parts: []assetsync.PosePartRecord{{Tile: 5}, {Tile: 9}}},
	}
	if !PoseContainsTile(poses[0], 2) || PoseContainsTile(poses[0], 9) {
		t.Fatal("pose 0 tile membership")
	}
	if got := FirstPoseContainingTile(poses, 5); got != 0 {
		t.Fatalf("shared tile prefers lowest pose id = %d", got)
	}
	if got := FirstPoseContainingTile(poses, 9); got != 1 {
		t.Fatalf("tile 9 = pose %d, want 1", got)
	}
	if got := FirstPoseContainingTile(poses, 1); got != -1 {
		t.Fatalf("unknown tile = %d, want -1", got)
	}
}

func TestPoseSheetBoundsUsesTileGridNotOffsets(t *testing.T) {
	pose := assetsync.PoseRecord{Parts: []assetsync.PosePartRecord{
		{Tile: 0, X: 0, Y: 0},
		{Tile: 2, X: 16, Y: 0},
		{Tile: 10, X: 0, Y: 8},
	}}
	col, row, width, height, ok := PoseSheetBounds(pose, 8, 4)
	if !ok || col != 0 || row != 0 || width != 3 || height != 2 {
		t.Fatalf("bounds = %d,%d %dx%d ok=%v, want 0,0 3x2", col, row, width, height, ok)
	}
	if _, _, _, _, ok := PoseSheetBounds(assetsync.PoseRecord{}, 8, 4); ok {
		t.Fatal("empty pose should have no sheet bounds")
	}
}

func TestComposePoseAppliesOffsetAndFlip(t *testing.T) {
	sheet := assetsync.Spritesheet{Width: 16, Height: 8, Sprites: []assetsync.SpriteRecord{
		filledCell(0, 0, 0, 7),
		filledCell(1, 8, 0, 9),
	}}
	sheet.Sprites[1].Pixels[0][0] = 9
	sheet.Sprites[1].Pixels[0][7] = 4
	pose := assetsync.PoseRecord{Parts: []assetsync.PosePartRecord{
		{Tile: 0, X: 0, Y: 0},
		{Tile: 1, X: 8, Y: 0, FlipX: true},
	}}
	preview := ComposePose(sheet, pose)
	if preview.Width != 16 || preview.Height != 8 {
		t.Fatalf("preview size = %dx%d, want 16x8", preview.Width, preview.Height)
	}
	if preview.Pixels[0] != 7 {
		t.Fatalf("origin pixel = %d, want 7", preview.Pixels[0])
	}
	if preview.Pixels[8] != 4 {
		t.Fatalf("flipped tile left edge = %d, want source right edge 4", preview.Pixels[8])
	}
	if preview.Pixels[15] != 9 {
		t.Fatalf("flipped tile right edge = %d, want source left edge 9", preview.Pixels[15])
	}
}

func TestCapturePoseKeepsPaintedTilesAndSkipsBlanks(t *testing.T) {
	sheet := assetsync.Spritesheet{Width: 24, Height: 16, Sprites: []assetsync.SpriteRecord{
		filledCell(1, 8, 0, 7),
		filledCell(5, 16, 8, 9),
	}}
	pose, err := CapturePose(sheet, 1, 0, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []assetsync.PosePartRecord{
		{Tile: 1, X: 0, Y: 0},
		{Tile: 5, X: 8, Y: 8},
	}
	if len(pose.Parts) != len(want) {
		t.Fatalf("parts = %+v", pose.Parts)
	}
	for i, part := range want {
		if pose.Parts[i] != part {
			t.Fatalf("part %d = %+v, want %+v", i, pose.Parts[i], part)
		}
	}
}

func TestCapturePoseRejectsEmptyPaint(t *testing.T) {
	sheet := assetsync.Spritesheet{Width: 24, Height: 16}
	if _, err := CapturePose(sheet, 1, 0, 2, 2); err == nil || err.Error() != "pose selection has no painted tiles" {
		t.Fatalf("empty capture error = %v", err)
	}
}

func TestCapturePoseAcceptsMoreThan128Tiles(t *testing.T) {
	sheet := assetsync.Spritesheet{Width: 128, Height: 80}
	for id := 0; id < 144; id++ {
		sheet.Sprites = append(sheet.Sprites, filledCell(id, (id%16)*8, (id/16)*8, 1))
	}
	pose, err := CapturePose(sheet, 0, 0, 16, 9)
	if err != nil || len(pose.Parts) != 144 {
		t.Fatalf("144 tile pose: parts=%d error=%v", len(pose.Parts), err)
	}
}

func TestNextPoseNameFillsIdleGaps(t *testing.T) {
	poses := []assetsync.PoseRecord{{Name: "idle_0"}, {Name: "idle_2"}}
	if got := NextPoseName(poses, "idle"); got != "idle_1" {
		t.Fatalf("NextPoseName = %q, want idle_1", got)
	}
}

func TestRenamePoseRejectsDuplicatesAndInvalidChars(t *testing.T) {
	file := &assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{
		{Name: "idle_0"},
		{Name: "idle_1"},
	}}
	if err := RenamePose(file, 1, "walk_1"); err != nil {
		t.Fatal(err)
	}
	if file.Poses[1].Name != "walk_1" {
		t.Fatalf("renamed = %q", file.Poses[1].Name)
	}
	if err := RenamePose(file, 1, "idle_0"); err == nil {
		t.Fatal("duplicate name should fail")
	}
	if err := RenamePose(file, 1, "idle 1"); err == nil {
		t.Fatal("space in name should fail")
	}
	if err := RenamePose(file, 1, ""); err != nil {
		t.Fatal(err)
	}
	if file.Poses[1].Name != "" {
		t.Fatalf("cleared name = %q", file.Poses[1].Name)
	}
}

func TestTogglePosePartAddsRelativeToOriginThenRemoves(t *testing.T) {
	pose := assetsync.PoseRecord{Parts: []assetsync.PosePartRecord{{Tile: 0, X: 0, Y: 0}}}
	added, err := TogglePosePart(pose, 2, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(added.Parts) != 2 || added.Parts[1].Tile != 2 || added.Parts[1].X != 16 || added.Parts[1].Y != 0 {
		t.Fatalf("added part = %+v", added.Parts)
	}
	removed, err := TogglePosePart(added, 2, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Parts) != 1 || removed.Parts[0].Tile != 0 {
		t.Fatalf("removed = %+v", removed.Parts)
	}
}

func filledCell(id, x, y, color int) assetsync.SpriteRecord {
	sprite := newCellSprite(id, x, y)
	for row := range sprite.Pixels {
		for col := range sprite.Pixels[row] {
			sprite.Pixels[row][col] = color
		}
	}
	return sprite
}

func TestPosePreviewBoundsLargeSparsePoseWithoutHugeAllocation(t *testing.T) {
	sheet := assetsync.Spritesheet{Width: 8, Height: 8, Sprites: []assetsync.SpriteRecord{filledCell(0, 0, 0, 3)}}
	pose := assetsync.PoseRecord{Parts: []assetsync.PosePartRecord{{Tile: 0, X: -32768, Y: -32768}, {Tile: 0, X: 32767, Y: 32767}}}
	_, _, width, height := PosePixelBounds(sheet, pose)
	if width != 65543 || height != 65543 {
		t.Fatalf("bounds = %dx%d", width, height)
	}
	preview := ComposePose(sheet, pose)
	if preview.Width > 512 || preview.Height > 512 || len(preview.Pixels) > 512*512 {
		t.Fatalf("unbounded preview = %dx%d", preview.Width, preview.Height)
	}
	if preview.Pixels[0] != 3 || preview.Pixels[len(preview.Pixels)-1] != 3 {
		t.Fatal("scaled thumbnail lost endpoint tiles")
	}
}
