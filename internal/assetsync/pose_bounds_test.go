package assetsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlattenPoseFlipBoundsIncludeTileExtentsAndWideCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parts []PosePartRecord
		x, y  int32
	}{
		{"empty", nil, 0, 0},
		{"origin", []PosePartRecord{{}}, 8, 8},
		{"negative", []PosePartRecord{{X: -24, Y: -8}, {X: 0, Y: 16}}, -16, 16},
		{"positive", []PosePartRecord{{X: 20, Y: 40}, {X: 28, Y: 56}}, 56, 104},
		{"wide single", []PosePartRecord{{X: -32768, Y: 32767}}, -65528, 65542},
		{"wide union", []PosePartRecord{{X: -32768, Y: -32768}, {X: 32767, Y: 32767}}, 7, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, _, err := flattenPoses(PoseFile{Version: 1, Poses: []PoseRecord{{Parts: tc.parts}}}, 1, "object sheet")
			if err != nil {
				t.Fatal(err)
			}
			if index[0].FlipOffsetX != tc.x || index[0].FlipOffsetY != tc.y {
				t.Fatalf("offsets = (%d,%d), want (%d,%d)", index[0].FlipOffsetX, index[0].FlipOffsetY, tc.x, tc.y)
			}
		})
	}
}

func TestAnimationFlipBoundsUseReferencedPoseUnionAndRegenerate(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)
	poses := PoseFile{Version: 1, Poses: []PoseRecord{
		{Name: "left", Parts: []PosePartRecord{{X: -24, Y: -8, FlipX: true}}},
		{Name: "right", Parts: []PosePartRecord{{X: 24, Y: 32}, {X: 32, Y: 32, FlipY: true}}},
		{Name: "empty"},
		{Name: "unreferenced", Parts: []PosePartRecord{{X: 32767, Y: -32768}}},
	}}
	if err := SavePoses(dir, &poses); err != nil {
		t.Fatal(err)
	}
	workspace := AnimationWorkspace{Version: 1, Clips: []AnimationClip{
		{Name: "union", Frames: []int{0, 1, 0, 2}, FPS: 8},
		{Name: "left only", Frames: []int{2, 0}, FPS: 8},
		{Name: "empty", Frames: []int{2}, FPS: 8},
	}}
	if err := SaveAnimationWorkspace(dir, &workspace); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "assets", "poses.json"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range [][2]int32{{16, 32}, {-40, -8}, {0, 0}} {
		clip := a.AnimationClips[i]
		if clip.FlipOffsetX != want[0] || clip.FlipOffsetY != want[1] {
			t.Fatalf("clip %q offsets=(%d,%d), want %v", clip.Name, clip.FlipOffsetX, clip.FlipOffsetY, want)
		}
	}
	generated, err := GenerateGo(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"FlipOffsetX: -40, FlipOffsetY: -8", "FlipOffsetX: 16, FlipOffsetY: 32", "FlipOffsetX: 65542, FlipOffsetY: -65528"} {
		if !strings.Contains(string(generated), want) {
			t.Errorf("missing ROM offsets %q", want)
		}
	}
	after, err := os.ReadFile(filepath.Join(dir, "assets", "poses.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || strings.Contains(string(after), "flipOffset") {
		t.Fatal("deriving ROM offsets rewrote pose JSON")
	}
	poses.Poses[0].Parts[0].X = -32
	if err := SavePoses(dir, &poses); err != nil {
		t.Fatal(err)
	}
	a, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.AnimationClips[0].FlipOffsetX != 8 || a.PoseIndex[0].FlipOffsetX != -56 {
		t.Fatal("asset edits retained stale derived offsets")
	}
}
