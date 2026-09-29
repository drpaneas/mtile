package editor

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// RemovePoseFromAnimationClips mirrors deletion from the global pose slice.
// Repeated entries are removed together; all later IDs shift down exactly once.
func RemovePoseFromAnimationClips(workspace *assetsync.AnimationWorkspace, id int) {
	if workspace == nil || id < 0 {
		return
	}
	for i := range workspace.Clips {
		frames := workspace.Clips[i].Frames[:0]
		for _, frame := range workspace.Clips[i].Frames {
			if frame == id {
				continue
			}
			if frame > id {
				frame--
			}
			frames = append(frames, frame)
		}
		workspace.Clips[i].Frames = frames
	}
}

func AddAnimationClip(workspace *assetsync.AnimationWorkspace, frames []int, fps int) int {
	used := make(map[string]bool)
	for _, clip := range workspace.Clips {
		used[clip.Name] = true
	}
	name := ""
	for index := 1; ; index++ {
		name = fmt.Sprintf("Clip %d", index)
		if !used[name] {
			break
		}
	}
	workspace.Version = 1
	workspace.Clips = append(workspace.Clips, assetsync.AnimationClip{Name: name, Frames: append([]int(nil), frames...), FPS: min(60, max(1, fps))})
	return len(workspace.Clips) - 1
}

func MoveAnimationClipFrame(clip *assetsync.AnimationClip, index, delta int) int {
	if clip == nil || index < 0 || index >= len(clip.Frames) {
		return index
	}
	target := min(len(clip.Frames)-1, max(0, index+delta))
	clip.Frames[index], clip.Frames[target] = clip.Frames[target], clip.Frames[index]
	return target
}

// AnimationPreviewBounds retains one coordinate system across all frames.
// Independently centering each frame would make an idle character jump around.
func AnimationPreviewBounds(sheet assetsync.Spritesheet, poses []assetsync.PoseRecord, frames []int) Rect {
	bounds := Rect{}
	for _, id := range frames {
		if id < 0 || id >= len(poses) {
			continue
		}
		x, y, w, h := PosePixelBounds(sheet, poses[id])
		if w <= 0 || h <= 0 {
			continue
		}
		if bounds.W == 0 {
			bounds = Rect{X: x, Y: y, W: w, H: h}
			continue
		}
		left, top := min(bounds.X, x), min(bounds.Y, y)
		bounds = Rect{X: left, Y: top, W: max(bounds.X+bounds.W, x+w) - left, H: max(bounds.Y+bounds.H, y+h) - top}
	}
	return bounds
}

func ComposeAnimationFrame(sheet assetsync.Spritesheet, pose assetsync.PoseRecord, bounds Rect) PosePreview {
	if bounds.W <= 0 || bounds.H <= 0 {
		return PosePreview{}
	}
	divisor := max(1, (max(bounds.W, bounds.H)+511)/512)
	width, height := (bounds.W+divisor-1)/divisor, (bounds.H+divisor-1)/divisor
	pixels := make([]int, width*height)
	wanted := make(map[int]bool, len(pose.Parts))
	for _, part := range pose.Parts {
		wanted[part.Tile] = true
	}
	tiles := readSheetTiles(sheet, wanted)
	for _, part := range pose.Parts {
		tile, ok := tiles[part.Tile]
		if !ok {
			continue
		}
		for py := 0; py < 8; py++ {
			for px := 0; px < 8; px++ {
				x, y := part.X+px-bounds.X, part.Y+py-bounds.Y
				if x < 0 || y < 0 || x >= bounds.W || y >= bounds.H {
					continue
				}
				sx, sy := px, py
				if part.FlipX {
					sx = 7 - sx
				}
				if part.FlipY {
					sy = 7 - sy
				}
				if color := tile[sy*8+sx]; color != 0 {
					pixels[(y/divisor)*width+x/divisor] = color
				}
			}
		}
	}
	return PosePreview{Width: width, Height: height, Pixels: pixels}
}
