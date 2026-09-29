package assetsync

import (
	"strings"
	"testing"
)

func TestAnimationAssetsLoadFlattenAndGenerateROM(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)
	if err := SavePoses(dir, &PoseFile{Version: 1, Poses: []PoseRecord{{Name: "one", Parts: []PosePartRecord{{Tile: 0}}}, {Name: "two", Parts: []PosePartRecord{{Tile: 0}}}}}); err != nil {
		t.Fatal(err)
	}
	noLoop := false
	workspace := AnimationWorkspace{Version: 1, Clips: []AnimationClip{{Name: "idle", Frames: []int{1, 0, 1}, FPS: 8}, {Name: "hold", Frames: []int{0, 1}, FPS: 12, Loop: &noLoop}, {Name: "empty", FPS: 1}}}
	if err := SaveAnimationWorkspace(dir, &workspace); err != nil {
		t.Fatal(err)
	}
	a, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.AnimationClips) != 3 || len(a.AnimationFrames) != 5 || !a.AnimationClips[0].Loop || a.AnimationClips[1].Loop || a.AnimationClips[1].Start != 3 || a.AnimationFrames[0] != 1 {
		t.Fatalf("clip flattening: %+v %+v", a.AnimationClips, a.AnimationFrames)
	}
	source, err := GenerateGo(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`Name: "idle"`, `FPS: 8, Loop: true`, `Name: "hold"`, `FPS: 12, Loop: false`, "cartData.AnimationClips", "AnimationFrames: animationFrameROM[:]"} {
		if !strings.Contains(string(source), want) {
			t.Errorf("missing %q", want)
		}
	}
	workspace.Clips[0].Frames = []int{99}
	if err := SaveAnimationWorkspace(dir, &workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "missing pose") {
		t.Fatalf("invalid runtime clip accepted: %v", err)
	}
}
