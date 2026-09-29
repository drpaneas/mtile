package transpiler

import (
	"slices"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// Direct compiler callers can construct or edit raw pose/clip tables without
// assetsync. Always derive their ROM offsets afresh, without mutating Options
// slices owned by the caller. Invalid spans retain the runtime's no-op behavior.
func withPoseFlipOffsets(opts Options) Options {
	opts.PoseIndex = slices.Clone(opts.PoseIndex)
	opts.AnimationClips = slices.Clone(opts.AnimationClips)
	poseBounds := make([]assetsync.PoseFlipBounds, len(opts.PoseIndex))
	for id, pose := range opts.PoseIndex {
		end := uint64(pose.Start) + uint64(pose.Count)
		if end <= uint64(len(opts.PoseParts)) {
			for _, part := range opts.PoseParts[uint64(pose.Start):end] {
				poseBounds[id].AddTile(part.X, part.Y)
			}
		}
		opts.PoseIndex[id].FlipOffsetX, opts.PoseIndex[id].FlipOffsetY = poseBounds[id].Offsets()
	}
	for id, clip := range opts.AnimationClips {
		var bounds assetsync.PoseFlipBounds
		end := uint64(clip.Start) + uint64(clip.Count)
		if end <= uint64(len(opts.AnimationFrames)) {
			for _, poseID := range opts.AnimationFrames[uint64(clip.Start):end] {
				if int(poseID) < len(poseBounds) {
					bounds.Include(poseBounds[poseID])
				}
			}
		}
		opts.AnimationClips[id].FlipOffsetX, opts.AnimationClips[id].FlipOffsetY = bounds.Offsets()
	}
	return opts
}
