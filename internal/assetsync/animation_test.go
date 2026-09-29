package assetsync

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAnimationClipLoopMetadataRoundTrip(t *testing.T) {
	for _, value := range []string{"", ",\"loop\":true", ",\"loop\":false"} {
		t.Run(value, func(t *testing.T) {
			var clip AnimationClip
			if err := json.Unmarshal([]byte(`{"name":"Idle","frames":[0,1,0],"fps":12`+value+`}`), &clip); err != nil {
				t.Fatal(err)
			}
			if (clip.Loop == nil) != (value == "") {
				t.Fatal("omitted loop metadata did not retain the default-loop sentinel")
			}
			want := AnimationWorkspace{Version: 1, Clips: []AnimationClip{clip}}
			if err := ValidateAnimationWorkspace(want, 128, 256, 2); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := SaveAnimationWorkspace(dir, &want); err != nil {
				t.Fatal(err)
			}
			got, err := LoadAnimationWorkspace(dir)
			if err != nil || !reflect.DeepEqual(*got, want) {
				t.Fatalf("loop metadata round trip = %+v, %v", got, err)
			}
		})
	}
	var clip AnimationClip
	if err := json.Unmarshal([]byte(`{"name":"Idle","fps":12,"loop":"false"}`), &clip); err == nil {
		t.Fatal("non-boolean loop metadata was accepted")
	}
}

func TestAnimationWorkspaceMissingAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	empty, err := LoadAnimationWorkspace(dir)
	if err != nil || empty == nil || empty.Version != 1 || len(empty.Pages)+len(empty.Clips) != 0 {
		t.Fatalf("legacy workspace = %+v, %v", empty, err)
	}
	if err := SaveAnimationWorkspace(dir, empty); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "assets", "animation.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("empty save created metadata: %v", err)
	}
	want := AnimationWorkspace{Version: 1, Pages: []AnimationPage{{Name: "Hero", Width: 480, Height: 320}, {Name: "Enemy", Y: 320, Width: 480, Height: 320}}, Clips: []AnimationClip{{Name: "Hero idle", Frames: []int{2, 0, 2, 1}, FPS: 12}}}
	if err := SaveAnimationWorkspace(dir, &want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadAnimationWorkspace(dir)
	if err != nil || !reflect.DeepEqual(*got, want) {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
	if err := ValidateAnimationWorkspace(*got, 480, 640, 3); err != nil {
		t.Fatal(err)
	}
	if err := SaveAnimationWorkspace(dir, empty); err != nil {
		t.Fatal(err)
	}
	got, err = LoadAnimationWorkspace(dir)
	if err != nil || len(got.Pages)+len(got.Clips) != 0 {
		t.Fatalf("clearing existing metadata = %+v, %v", got, err)
	}
}

func TestAnimationWorkspaceRejectsMalformedMetadata(t *testing.T) {
	cases := map[string]AnimationWorkspace{
		"version":        {Version: 2},
		"blank page":     {Version: 1, Pages: []AnimationPage{{Width: 8, Height: 8}}},
		"duplicate page": {Version: 1, Pages: []AnimationPage{{Name: "a", Width: 8, Height: 8}, {Name: "a", X: 8, Width: 8, Height: 8}}},
		"overlap":        {Version: 1, Pages: []AnimationPage{{Name: "a", Width: 16, Height: 8}, {Name: "b", X: 8, Width: 8, Height: 8}}},
		"alignment":      {Version: 1, Pages: []AnimationPage{{Name: "a", X: 1, Width: 8, Height: 8}}},
		"negative":       {Version: 1, Pages: []AnimationPage{{Name: "a", X: -8, Width: 8, Height: 8}}},
		"overflow":       {Version: 1, Pages: []AnimationPage{{Name: "a", X: math.MaxInt, Width: math.MaxInt, Height: 8}}},
		"fps zero":       {Version: 1, Clips: []AnimationClip{{Name: "idle", FPS: 0}}},
		"fps high":       {Version: 1, Clips: []AnimationClip{{Name: "idle", FPS: 61}}},
		"negative pose":  {Version: 1, Clips: []AnimationClip{{Name: "idle", FPS: 8, Frames: []int{-1}}}},
		"pose format":    {Version: 1, Clips: []AnimationClip{{Name: "idle", FPS: 8, Frames: []int{MaxPoseCount}}}},
		"duplicate clip": {Version: 1, Clips: []AnimationClip{{Name: "idle", FPS: 8}, {Name: "idle", FPS: 8}}},
		"control name":   {Version: 1, Clips: []AnimationClip{{Name: "bad\nname", FPS: 8}}},
	}
	for name, workspace := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := SaveAnimationWorkspace(dir, &workspace); err == nil {
				t.Fatal("save accepted malformed metadata")
			}
			if err := writeJSON(filepath.Join(dir, "assets", "animation.json"), workspace); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadAnimationWorkspace(dir); err == nil {
				t.Fatal("load accepted malformed metadata")
			}
		})
	}
}

func TestAnimationWorkspaceValidationUsesActualSheetAndPoses(t *testing.T) {
	workspace := AnimationWorkspace{Version: 1, Pages: []AnimationPage{{Name: "page", X: 8, Width: 8, Height: 8}}, Clips: []AnimationClip{{Name: "clip", Frames: []int{1}, FPS: 60}}}
	if err := ValidateAnimationWorkspace(workspace, 8, 8, 2); err == nil {
		t.Fatal("out-of-sheet page accepted")
	}
	if err := ValidateAnimationWorkspace(workspace, 16, 8, 1); err == nil {
		t.Fatal("missing pose accepted")
	}
	if err := ValidateAnimationWorkspace(workspace, 16, 8, 2); err != nil {
		t.Fatal(err)
	}
}
