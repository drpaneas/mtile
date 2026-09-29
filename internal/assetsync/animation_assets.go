package assetsync

import "fmt"

func attachAnimations(projectDir string, assets *Assets) error {
	workspace, err := LoadAnimationWorkspace(projectDir)
	if err != nil {
		return err
	}
	w, h := assets.SheetW, assets.SheetH
	if assets.ObjSpriteN > 0 {
		w, h = assets.ObjSheetW, assets.ObjSheetH
	}
	if err := ValidateAnimationWorkspace(*workspace, w, h, len(assets.PoseIndex)); err != nil {
		return err
	}
	if len(workspace.Clips) == 0 {
		return nil
	}
	poseBounds := make([]PoseFlipBounds, len(assets.PoseIndex))
	for id, pose := range assets.PoseIndex {
		for _, part := range assets.PoseParts[int(pose.Start) : int(pose.Start)+int(pose.Count)] {
			poseBounds[id].AddTile(part.X, part.Y)
		}
	}
	for _, clip := range workspace.Clips {
		// Frame IDs occupy two bytes in the 32 MiB cartridge address space.
		if len(clip.Frames) > (32*1024*1024)/2-len(assets.AnimationFrames) {
			return fmt.Errorf("animation.json: frame tables exceed cartridge capacity")
		}
		loop := clip.Loop == nil || *clip.Loop
		var bounds PoseFlipBounds
		for _, id := range clip.Frames {
			bounds.Include(poseBounds[id])
			assets.AnimationFrames = append(assets.AnimationFrames, uint16(id))
		}
		offsetX, offsetY := bounds.Offsets()
		assets.AnimationClips = append(assets.AnimationClips, FlatAnimationClip{Name: clip.Name, Start: uint32(len(assets.AnimationFrames) - len(clip.Frames)), Count: uint32(len(clip.Frames)), FPS: uint8(clip.FPS), Loop: loop, FlipOffsetX: offsetX, FlipOffsetY: offsetY})
	}
	return nil
}
