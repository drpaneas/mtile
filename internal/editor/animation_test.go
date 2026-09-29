package editor

import (
	"reflect"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestAnimationClipOrderCopiesAndRemapsGlobalPoseIDs(t *testing.T) {
	workspace := assetsync.AnimationWorkspace{Version: 1}
	frames := []int{3, 1, 3, 0, 2}
	index := AddAnimationClip(&workspace, frames, 12)
	frames[0] = 99
	if index != 0 || workspace.Clips[0].Frames[0] != 3 || workspace.Clips[0].FPS != 12 {
		t.Fatal("clip did not copy authored order and speed")
	}
	if at := MoveAnimationClipFrame(&workspace.Clips[0], 1, -1); at != 0 {
		t.Fatalf("move index = %d", at)
	}
	if !reflect.DeepEqual(workspace.Clips[0].Frames, []int{1, 3, 3, 0, 2}) {
		t.Fatal("move lost frame order")
	}
	RemovePoseFromAnimationClips(&workspace, 3)
	if !reflect.DeepEqual(workspace.Clips[0].Frames, []int{1, 0, 2}) {
		t.Fatal("delete did not remove repeated references")
	}
	RemovePoseFromAnimationClips(&workspace, 1)
	if !reflect.DeepEqual(workspace.Clips[0].Frames, []int{0, 1}) {
		t.Fatal("delete did not shift higher IDs")
	}
	AddAnimationClip(&workspace, nil, 8)
	if workspace.Clips[0].Name == workspace.Clips[1].Name {
		t.Fatal("generated duplicate clip name")
	}
}

func TestAnimationPreviewKeepsSharedOriginAndFlips(t *testing.T) {
	sheet := assetsync.Spritesheet{Width: 8, Height: 8, Sprites: []assetsync.SpriteRecord{filledCell(0, 0, 0, 2)}}
	sheet.Sprites[0].Pixels[0][7] = 4
	poses := []assetsync.PoseRecord{{Parts: []assetsync.PosePartRecord{{Tile: 0, X: -8, Y: 0}}}, {Parts: []assetsync.PosePartRecord{{Tile: 0, X: 0, Y: 8, FlipX: true}}}}
	bounds := AnimationPreviewBounds(sheet, poses, []int{1, 0})
	if bounds != (Rect{X: -8, Y: 0, W: 16, H: 16}) {
		t.Fatalf("shared bounds = %+v", bounds)
	}
	first, second := ComposeAnimationFrame(sheet, poses[0], bounds), ComposeAnimationFrame(sheet, poses[1], bounds)
	if first.Width != 16 || first.Height != 16 || second.Width != 16 || second.Height != 16 {
		t.Fatal("frames did not share raster dimensions")
	}
	if first.Pixels[0] != 2 || first.Pixels[8*16+8] != 0 || second.Pixels[0] != 0 || second.Pixels[8*16+8] != 4 {
		t.Fatal("preview moved origins or lost flipping")
	}
}

func TestAnimationPreviewSparseCoordinatesStayBounded(t *testing.T) {
	sheet := assetsync.Spritesheet{Width: 8, Height: 8, Sprites: []assetsync.SpriteRecord{filledCell(0, 0, 0, 2)}}
	poses := []assetsync.PoseRecord{{Parts: []assetsync.PosePartRecord{{Tile: 0, X: -32768, Y: -32768}}}, {Parts: []assetsync.PosePartRecord{{Tile: 0, X: 32767, Y: 32767}}}}
	bounds := AnimationPreviewBounds(sheet, poses, []int{0, 1})
	for _, pose := range poses {
		preview := ComposeAnimationFrame(sheet, pose, bounds)
		if preview.Width > 512 || preview.Height > 512 || len(preview.Pixels) != preview.Width*preview.Height {
			t.Fatalf("unbounded sparse preview %dx%d", preview.Width, preview.Height)
		}
	}
}
