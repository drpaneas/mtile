package editor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func objectResizeFixture() *Model {
	return &Model{
		ObjSheet: &assetsync.Spritesheet{Width: 16, Height: 16, Sprites: []assetsync.SpriteRecord{filledCell(3, 8, 8, 7)}},
		Poses:    assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Name: "kick", Parts: []assetsync.PosePartRecord{{Tile: 3, X: -8, Y: 16, FlipX: true}}}}},
	}
}

func TestResizeObjectSheetPreservesPixelsAndRemapsFrames(t *testing.T) {
	m := objectResizeFixture()
	before := ComposePose(*m.ObjSheet, m.Poses.Poses[0])
	if err := m.ResizeObjSheet(1032, 16); err != nil {
		t.Fatal(err)
	}
	if part := m.Poses.Poses[0].Parts[0]; part.Tile != 130 || part.X != -8 || part.Y != 16 || !part.FlipX {
		t.Fatalf("remapped part = %+v", part)
	}
	if got := ComposePose(*m.ObjSheet, m.Poses.Poses[0]); !reflect.DeepEqual(got, before) {
		t.Fatal("resizing changed the composed frame")
	}
	if got := m.ObjSheet.Sprites[0]; got.ID != 130 || got.X != 8 || got.Y != 8 {
		t.Fatalf("remapped cell = %+v", got)
	}
	if err := m.ResizeObjSheet(16, 16); err != nil {
		t.Fatal(err)
	}
	if m.Poses.Poses[0].Parts[0].Tile != 3 {
		t.Fatal("shrinking unused space failed to restore tile ID")
	}
}

func TestResizeObjectSheetRejectsDataLossWithoutMutation(t *testing.T) {
	for _, reason := range []string{"frame", "painted pixel", "flags"} {
		t.Run(reason, func(t *testing.T) {
			m := objectResizeFixture()
			if reason != "frame" {
				m.Poses.Poses = nil
			}
			if reason == "flags" {
				m.ObjSheet.Sprites[0] = filledCell(3, 8, 8, 0)
				m.ObjSheet.Sprites[0].Flags = 1
			}
			oldSheet, oldPoses := m.ObjSheet, m.Poses
			if err := m.ResizeObjSheet(8, 8); err == nil || !strings.Contains(err.Error(), reason) {
				t.Fatalf("error = %v", err)
			}
			if m.ObjSheet != oldSheet || !reflect.DeepEqual(m.Poses, oldPoses) {
				t.Fatal("rejected resize changed model")
			}
		})
	}
}

func TestResizeObjectSheetCanTrimTransparentRectangles(t *testing.T) {
	m := &Model{ObjSheet: &assetsync.Spritesheet{Width: 16, Height: 16, Sprites: []assetsync.SpriteRecord{{ID: 0, Width: 16, Height: 16, Pixels: make([][]int, 16)}}}}
	for y := range m.ObjSheet.Sprites[0].Pixels {
		m.ObjSheet.Sprites[0].Pixels[y] = make([]int, 16)
	}
	m.ObjSheet.Sprites[0].Pixels[0][0] = 4
	if err := m.ResizeObjSheet(8, 8); err != nil {
		t.Fatal(err)
	}
	if got := sheetPixel(*m.ObjSheet, 0, 0); got != 4 {
		t.Fatalf("retained pixel = %d", got)
	}
}

func TestResizeObjectSheetPersistsFramesWithNewIDs(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	fixture := objectResizeFixture()
	m.ObjSheet, m.Poses = fixture.ObjSheet, fixture.Poses
	if err := m.ResizeObjSheet(24, 16); err != nil {
		t.Fatal(err)
	}
	wantFrame := ComposePose(*m.ObjSheet, m.Poses.Poses[0])
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(m.Dir)
	if err != nil {
		t.Fatal(err)
	}
	assertUniformObjectPages(t, reloaded)
	if got := ComposePose(*reloaded.ObjSheet, reloaded.Poses.Poses[0]); !reflect.DeepEqual(got, wantFrame) {
		t.Fatal("save/load normalization changed the resized frame's pixels")
	}
}

func TestCreateObjectSheetPreservesLegacySharedFrames(t *testing.T) {
	m, err := Open(editorFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	m.Poses = assetsync.PoseFile{Version: 1, Poses: []assetsync.PoseRecord{{Parts: []assetsync.PosePartRecord{{Tile: 1}}}}}
	before := ComposePose(m.Sheet, m.Poses.Poses[0])
	if err := m.CreateObjSheet(480, 320); err != nil {
		t.Fatal(err)
	}
	if got := ComposePose(*m.ObjSheet, m.Poses.Poses[0]); !reflect.DeepEqual(got, before) {
		t.Fatal("creating animation bank changed legacy frame")
	}
	if err := m.SetObjSpritePixel(1, 0, 0, 7); err != nil {
		t.Fatal(err)
	}
	if got := ComposePose(m.Sheet, m.Poses.Poses[0]); !reflect.DeepEqual(got, before) {
		t.Fatal("animation bank pixels alias shared bank")
	}
	if err := m.CreateObjSheet(8, 8); err == nil {
		t.Fatal("creation overwrote existing bank")
	}
}
