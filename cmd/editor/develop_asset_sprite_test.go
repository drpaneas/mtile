package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestDevelopAssetThumbnailUsesAuthoredBank(t *testing.T) {
	g := newTestGameWithPoses(t)
	t.Cleanup(g.closeNativePerformance)
	g.mode = modeCode
	if err := g.model.SetSpritePixel(6, 0, 0, 5); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetObjSpritePixel(6, 0, 0, 4); err != nil {
		t.Fatal(err)
	}
	g.model.Animation.Clips = []assetsync.AnimationClip{{Name: "idle", Frames: []int{1, 0}, FPS: 8}}
	for kind, asset := range map[string]developAsset{
		"pspr": {Name: "idle_1", ID: 1},
		"anim": {Name: "idle", ID: 0},
	} {
		if !slices.Contains(g.developAssets(kind), asset) {
			t.Errorf("%s catalog does not contain %v", kind, asset)
		}
		thumb := g.developAssetThumbnail(kind, asset.ID)
		if thumb.Width != 16 || thumb.Height != 8 || thumb.Pixels[0] != 4 {
			t.Errorf("%s thumbnail did not use the authored OBJ artwork", kind)
		}
	}
	if !g.openDevelopAsset("pspr", 1) || g.mode != modeSprites || !g.objEditing || g.animationWorkspaceActive() || g.selectedPose != 1 || g.currentSprite != 6 {
		t.Fatal("Go to pose did not select the authored OBJ pose in Sprite Lab")
	}
}

func TestDevelopAssetJumpPreservesLegacyBankAndUnsavedCode(t *testing.T) {
	g := newTestGame(t, 16, 16, 8, 8)
	t.Cleanup(g.closeNativePerformance)
	g.model.Poses.Poses = []assetsync.PoseRecord{
		{Parts: []assetsync.PosePartRecord{{Tile: 1}}},
		{Name: "walk", Parts: []assetsync.PosePartRecord{{Tile: 2}}},
	}
	g.model.Animation.Clips = []assetsync.AnimationClip{
		{Name: "idle", Frames: []int{0}, FPS: 8},
		{Name: "walk", Frames: []int{1}, FPS: 8},
	}
	g.mode = modeCode
	g.code.Insert("-- unsaved\n")
	if assets := g.developAssets("pspr"); !slices.Equal(assets, []developAsset{{Name: "walk", ID: 1}}) {
		t.Fatalf("named pose catalog = %v", assets)
	}
	if !g.openDevelopAsset("anim", 1) || g.mode != modeSprites || !g.animationWorkspaceActive() {
		t.Fatal("Go to clip did not select the shared-sheet animation")
	}
	if clip := g.activeAnimationClip(); clip == nil || clip.Name != "walk" {
		t.Fatalf("active shared-sheet clip = %v", clip)
	}
	g.mode = modeCode
	if !g.openDevelopAsset("pspr", 1) || g.mode != modeSprites || g.animationWorkspaceActive() || g.selectedPose != 1 || g.currentSprite != 2 {
		t.Fatal("Go to pose did not select the shared-sheet pose")
	}
	if g.model.ObjSheet != nil || g.objEditing {
		t.Fatal("asset navigation migrated the shared sheet to an OBJ sheet")
	}
	if _, err := os.Stat(filepath.Join(g.model.Dir, "assets", "poses.json")); !os.IsNotExist(err) {
		t.Fatalf("asset navigation wrote the pose file: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(g.model.Dir, "game.lua"))
	if err != nil || string(source) != "-- test project\n" || !g.code.Dirty() {
		t.Fatalf("asset navigation saved or discarded unsaved source: %q, %v", source, err)
	}
}
