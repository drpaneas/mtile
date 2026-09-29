package build

import (
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func TestBuildPreservesDerivedWideFlipOffsets(t *testing.T) {
	a := &assetsync.Assets{
		PoseIndex:      []assetsync.FlatPose{{Count: 1, FlipOffsetX: 65542, FlipOffsetY: -65528}},
		PoseParts:      []assetsync.FlatPosePart{{X: 32767, Y: -32768}},
		AnimationClips: []assetsync.FlatAnimationClip{{Name: "wide", Count: 1, FPS: 8, Loop: true, FlipOffsetX: 65542, FlipOffsetY: -65528}},
	}
	poses, parts := flattenAssetPoses(a)
	clips := flattenAssetAnimations(a)
	if poses[0].FlipOffsetX != 65542 || poses[0].FlipOffsetY != -65528 || clips[0].FlipOffsetX != 65542 || clips[0].FlipOffsetY != -65528 || parts[0].X != 32767 || parts[0].Y != -32768 {
		t.Fatalf("build truncated flip offsets: poses=%+v parts=%+v clips=%+v", poses, parts, clips)
	}
}
