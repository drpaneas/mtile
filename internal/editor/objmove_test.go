package editor

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func objectMoveFixture() *Model {
	m := spriteBlockTestModel()
	m.ActivePalette.Colors = make([]assetsync.RGB15, 256)
	m.ObjSheet = &assetsync.Spritesheet{Width: 48, Height: 32}
	for id := 0; id < 24; id++ {
		sprite := filledCell(id, id%6*8, id/6*8, id+1)
		sprite.Flags = id + 10
		sprite.Pixels[2][3] = id + 50
		m.ObjSheet.Sprites = append(m.ObjSheet.Sprites, sprite)
	}
	return m
}

func TestMoveObjectTilesOverlappingDirectionsPreserveSnapshotAndPosePixels(t *testing.T) {
	for _, test := range []struct {
		name                           string
		srcCol, srcRow, dstCol, dstRow int
	}{
		{"right", 1, 1, 2, 1},
		{"left", 2, 1, 1, 1},
		{"down", 1, 0, 1, 1},
		{"up", 1, 1, 1, 0},
		{"diagonal", 1, 0, 2, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := objectMoveFixture()
			pose := assetsync.PoseRecord{Name: "moving"}
			for y := 0; y < 2; y++ {
				for x := 0; x < 2; x++ {
					pose.Parts = append(pose.Parts, assetsync.PosePartRecord{Tile: (test.srcRow+y)*6 + test.srcCol + x, X: x*8 - 8, Y: y * 8, FlipX: x == 1, FlipY: y == 1})
				}
			}
			m.Poses = assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{pose}}
			before := cloneSpritesheet(*m.ObjSheet)
			beforePose := ComposePose(before, pose)
			if err := m.MoveObjSpritePixelBlock(test.srcCol, test.srcRow, 2, 2, test.dstCol, test.dstRow); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(beforePose, ComposePose(*m.ObjSheet, m.Poses.Poses[0])) {
				t.Fatal("overlapping move changed the registered pose image")
			}
			for row := 0; row < 4; row++ {
				for col := 0; col < 6; col++ {
					id := row*6 + col
					wantID := id
					inSource := col >= test.srcCol && col < test.srcCol+2 && row >= test.srcRow && row < test.srcRow+2
					inDestination := col >= test.dstCol && col < test.dstCol+2 && row >= test.dstRow && row < test.dstRow+2
					if inDestination {
						wantID = (test.srcRow+row-test.dstRow)*6 + test.srcCol + col - test.dstCol
					}
					want, _ := spriteRecordByID(before, wantID)
					got, exists := spriteRecordByID(*m.ObjSheet, id)
					if inSource && !inDestination {
						if exists {
							t.Fatalf("vacated source %d retains a record/flags", id)
						}
						continue
					}
					if !exists || !reflect.DeepEqual(got.Pixels, want.Pixels) || got.Flags != want.Flags || got.X != col*8 || got.Y != row*8 {
						t.Fatalf("tile %d lost snapshot pixels, flags, or coordinates", id)
					}
				}
			}
		})
	}
}

func TestMoveObjectTilesAcrossPagesRemapsSharedAndPartialPosesOnly(t *testing.T) {
	m := spriteBlockTestModel()
	m.ObjSheet = &assetsync.Spritesheet{Width: ObjPageWidth, Height: ObjPageHeight * 2, Sprites: []assetsync.SpriteRecord{
		filledCell(0, 0, 0, 1), filledCell(1, 8, 0, 2), filledCell(7, 56, 0, 7),
	}}
	m.ObjSheet.Sprites[0].Flags = 42
	m.EnsureObjPages()
	m.Poses = assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{
		{Name: "whole", Parts: []assetsync.PosePartRecord{{Tile: 0, X: -8, FlipY: true}, {Tile: 1, FlipX: true}}},
		{Name: "partial shared", Parts: []assetsync.PosePartRecord{{Tile: 0, Y: 8}, {Tile: 7, X: 16, Y: 8}}},
	}}
	loop := false
	m.Animation.Clips = []assetsync.AnimationClip{{Name: "idle", Frames: []int{1, 0, 1}, FPS: 12, Loop: &loop}}
	beforePoses := posePagePreviews(m)
	beforeSheet, beforeAnimation := cloneSpritesheet(m.Sheet), objectPageSnapshot(t, m.Animation)
	oldPoses := m.Poses
	if err := m.MoveObjSpritePixelBlock(0, 0, 2, 1, 4, 42); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforePoses, posePagePreviews(m)) || !bytes.Equal(beforeAnimation, objectPageSnapshot(t, m.Animation)) || !reflect.DeepEqual(beforeSheet, m.Sheet) {
		t.Fatal("cross-page move changed pose appearance, pages/clips, or Background artwork")
	}
	for id, pose := range m.Poses.Poses {
		want := oldPoses.Poses[id]
		if pose.Name != want.Name || len(pose.Parts) != len(want.Parts) {
			t.Fatal("move changed pose registration/order")
		}
		for i, part := range pose.Parts {
			expected := want.Parts[i]
			if expected.Tile < 2 {
				expected.Tile += 42*64 + 4
			}
			if part != expected {
				t.Fatalf("pose %d part %d = %+v, want %+v", id, i, part, expected)
			}
		}
	}
	if got, _ := spriteRecordByID(*m.ObjSheet, 42*64+4); got.Flags != 42 {
		t.Fatal("cross-page move dropped source flags")
	}
	if got, _ := spriteRecordByID(*m.ObjSheet, 7); got.Pixels[0][0] != 7 {
		t.Fatal("partial pose's unrelated source tile changed")
	}
}

func TestMoveObjectTilesMovesBlankReferencesAndFlagsWithoutLeavingOldFlags(t *testing.T) {
	m := spriteBlockTestModel()
	m.ObjSheet = &assetsync.Spritesheet{Width: 32, Height: 8, Sprites: []assetsync.SpriteRecord{filledCell(0, 0, 0, 0), filledCell(2, 16, 0, 3), filledCell(3, 24, 0, 4)}}
	m.ObjSheet.Sprites[0].Flags = 16
	m.ObjSheet.Sprites[1].Flags = 32
	m.ObjSheet.Sprites[2].Flags = 64
	m.Poses = assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Parts: []assetsync.PosePartRecord{{Tile: 0}, {Tile: 1, X: 8}}}}}
	if err := m.MoveObjSpritePixelBlock(0, 0, 2, 1, 2, 0); err != nil {
		t.Fatal(err)
	}
	if len(m.ObjSheet.Sprites) != 1 || m.ObjSheet.Sprites[0].ID != 2 || m.ObjSheet.Sprites[0].Flags != 16 || sheetPixel(*m.ObjSheet, 16, 0) != 0 || sheetPixel(*m.ObjSheet, 24, 0) != 0 {
		t.Fatal("blank move failed to replace destination art/flags or left old source flags")
	}
	if parts := m.Poses.Poses[0].Parts; parts[0].Tile != 2 || parts[1].Tile != 3 {
		t.Fatal("blank source references did not move")
	}
}

func TestMoveObjectTilesSaveOpenPreservesPixelsReferencesAndClips(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	m.ObjSheet = &assetsync.Spritesheet{Width: ObjPageWidth, Height: ObjPageHeight * 2, Sprites: []assetsync.SpriteRecord{
		filledCell(0, 0, 0, 1), filledCell(1, 8, 0, 2), filledCell(7, 56, 0, 7),
	}}
	m.ObjSheet.Sprites[0].Flags = 42
	m.EnsureObjPages()
	m.Poses = assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{
		{Name: "idle", Parts: []assetsync.PosePartRecord{{Tile: 0, X: -8, FlipY: true}, {Tile: 1, FlipX: true}}},
		{Name: "shared", Parts: []assetsync.PosePartRecord{{Tile: 0, Y: 8}, {Tile: 7, X: 16, Y: 8}}},
	}}
	loop := false
	m.Animation.Clips = []assetsync.AnimationClip{{Name: "sequence", Frames: []int{1, 0, 1}, FPS: 12, Loop: &loop}}
	beforePixels := posePagePreviews(m)
	if err := m.MoveObjSpritePixelBlock(0, 0, 2, 1, 4, 42); err != nil {
		t.Fatal(err)
	}
	expectedSheet, expectedPoses, expectedAnimation := objectPageSnapshot(t, m.ObjSheet), objectPageSnapshot(t, m.Poses), objectPageSnapshot(t, m.Animation)
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(m.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expectedSheet, objectPageSnapshot(t, reloaded.ObjSheet)) || !bytes.Equal(expectedPoses, objectPageSnapshot(t, reloaded.Poses)) || !bytes.Equal(expectedAnimation, objectPageSnapshot(t, reloaded.Animation)) {
		t.Fatal("save/open changed relocated pixels, flags, pose references, or clip metadata")
	}
	if !reflect.DeepEqual(beforePixels, posePagePreviews(reloaded)) {
		t.Fatal("saved/reopened relocation changed a registered pose's appearance")
	}
}

func TestMoveObjectTilesRejectsInvalidInputTransactionally(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, args := range [][6]int{
		{-1, 0, 1, 1, 0, 0}, {0, 0, 0, 1, 1, 0}, {0, 0, 1, -1, 1, 0},
		{5, 0, 2, 1, 0, 0}, {0, 3, 1, 2, 0, 0}, {0, 0, 1, 1, 6, 0},
		{0, 0, 1, 1, 0, 4}, {0, 0, maxInt, 1, 0, 0}, {maxInt, 0, 2, 1, 0, 0}, {0, 0, 2, 1, maxInt, 0},
	} {
		m := objectMoveFixture()
		before := objectPageSnapshot(t, m)
		if err := m.MoveObjSpritePixelBlock(args[0], args[1], args[2], args[3], args[4], args[5]); err == nil || !bytes.Equal(before, objectPageSnapshot(t, m)) {
			t.Fatalf("invalid move %v succeeded or changed model", args)
		}
	}
	for _, invalid := range []string{"palette", "pixel rows", "flags", "pose reference", "overlapping legacy record"} {
		t.Run(invalid, func(t *testing.T) {
			m := objectMoveFixture()
			switch invalid {
			case "palette":
				m.ObjSheet.Sprites[1].Pixels[7][7] = 256
			case "pixel rows":
				m.ObjSheet.Sprites[1].Pixels[7] = nil
			case "flags":
				m.ObjSheet.Sprites[1].Flags = 256
			case "pose reference":
				m.Poses.Poses = []assetsync.PoseRecord{{Parts: []assetsync.PosePartRecord{{Tile: 999}}}}
			case "overlapping legacy record":
				m.ObjSheet.Sprites[0].Width = 16
			}
			before := objectPageSnapshot(t, m)
			if err := m.MoveObjSpritePixelBlock(1, 0, 2, 1, 2, 1); err == nil || !bytes.Equal(before, objectPageSnapshot(t, m)) {
				t.Fatalf("invalid %s succeeded or changed model", invalid)
			}
		})
	}
}

func TestMoveObjectTilesSameLocationIsNoOp(t *testing.T) {
	m := objectMoveFixture()
	before := objectPageSnapshot(t, m)
	if err := m.MoveObjSpritePixelBlock(1, 1, 2, 2, 1, 1); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, objectPageSnapshot(t, m)) {
		t.Fatal("moving in place changed the model")
	}
	var missing *Model
	if err := missing.MoveObjSpritePixelBlock(0, 0, 1, 1, 0, 0); err == nil {
		t.Fatal("missing model accepted a move")
	}
	if err := (&Model{}).MoveObjSpritePixelBlock(0, 0, 1, 1, 0, 0); err == nil {
		t.Fatal("missing sheet accepted a move")
	}
}
