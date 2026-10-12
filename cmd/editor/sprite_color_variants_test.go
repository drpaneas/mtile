package main

import (
	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"reflect"
	"testing"
)

func TestVariantRowsOnlyContainVisiblePoseColors(t *testing.T) {
	preview := editor.PosePreview{Pixels: []int{0, 11, 11, 158, 0, 175, 158}}
	if got := posePreviewColors(preview); !reflect.DeepEqual(got, []int{11, 158, 175}) {
		t.Fatalf("colors = %v", got)
	}
	v := &assetsync.PoseColorVariant{Colors: []assetsync.PoseColorSwap{{From: 11, To: 175}, {From: 175, To: 37}, {From: 99, To: 100}}}
	if variantReplacement(v, 11) != 175 || variantReplacement(v, 158) != 158 {
		t.Fatal("replacements cascaded or changed an unmapped color")
	}
	g := newTestGameWithPoses(t)
	g.selectedPose = -1
	if len(g.poseVariantColors()) != 0 {
		t.Fatal("no selected pose should show no source colors")
	}
}

func TestMapUsesSharedTilesAfterPoseEditing(t *testing.T) {
	g := newTestGameWithPoses(t)
	if err := g.model.SetSpritePixel(1, 0, 0, 7); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetObjSpritePixel(1, 0, 0, 11); err != nil {
		t.Fatal(err)
	}
	if err := g.refreshPreview(); err != nil {
		t.Fatal(err)
	}
	if err := g.setObjEditing(true); err != nil {
		t.Fatal(err)
	}
	if got := g.pixelAt(8, 0); got != 11 {
		t.Fatalf("pose pixel=%d", got)
	}
	g.mode = modeMap
	if g.activeSheet() != &g.model.Sheet || g.spriteCols() != 16 || g.pixelAt(8, 0) != 7 {
		t.Fatal("map read pose artwork or dimensions")
	}
	if !g.spriteIsReserved(0) {
		t.Fatal("map's zero tile must remain the reserved eraser")
	}
	g.currentSprite = 1
	if !g.placeGridSprites(2, 3) || g.mapCellAt(2, 3) != 1 {
		t.Fatal("map painting did not retain shared tile ID")
	}
	g.mode = modeSprites
	g.setMode(modeMap)
	if g.objEditing {
		t.Fatal("opening Map Lab must select the shared tile bank")
	}
	g.setMode(modeSprites)
	if g.activeSheet() != &g.model.Sheet {
		t.Fatal("returning to Sprite Lab should keep the map's shared tile source")
	}
}
