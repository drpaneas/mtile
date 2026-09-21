package assetsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPosesMissingFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	poses, err := LoadPoses(dir)
	if err != nil {
		t.Fatal(err)
	}
	if poses == nil || poses.Version != 1 || len(poses.Poses) != 0 {
		t.Fatalf("missing poses.json = %+v", poses)
	}
}

func TestLoadPosesRejectsBadVersionAndDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "assets", "poses.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"poses":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPoses(dir); err == nil || !strings.Contains(err.Error(), "poses.json") || !strings.Contains(err.Error(), "version") {
		t.Fatalf("version error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"poses":[{"name":"a","parts":[]},{"name":"a","parts":[]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPoses(dir); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestFlattenPosesRejectsOutOfRangeTile(t *testing.T) {
	dir := t.TempDir()
	sheet := Spritesheet{Width: 8, Height: 8, Sprites: []SpriteRecord{testSprite(0, 0, 0, 8, 8, 0, 0)}}
	mapData := Map{Width: 1, Height: 1, Cells: []int{0}}
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)

	posesJSON := `{"version":1,"poses":[{"name":"walk","parts":[{"tile":1,"x":0,"y":0}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "assets", "poses.json"), []byte(posesJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "tile") {
		t.Fatalf("out-of-range pose tile error = %v", err)
	}
}

func TestFlattenPosesRejectsOutOfRangeXY(t *testing.T) {
	dir := t.TempDir()
	sheet := Spritesheet{Width: 8, Height: 8, Sprites: []SpriteRecord{testSprite(0, 0, 0, 8, 8, 0, 0)}}
	mapData := Map{Width: 1, Height: 1, Cells: []int{0}}
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)

	posesJSON := `{"version":1,"poses":[{"name":"walk","parts":[{"tile":0,"x":32768,"y":0}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "assets", "poses.json"), []byte(posesJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("out-of-range pose x did not fail Load")
	}
	for _, want := range []string{"poses.json", "pose 0", "part 0", "x"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("out-of-range pose x error = %v, want it to mention %q", err, want)
		}
	}

	posesJSON = `{"version":1,"poses":[{"name":"walk","parts":[{"tile":0,"x":0,"y":-32769}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "assets", "poses.json"), []byte(posesJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(dir)
	if err == nil {
		t.Fatal("out-of-range pose y did not fail Load")
	}
	for _, want := range []string{"poses.json", "pose 0", "part 0", "y"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("out-of-range pose y error = %v, want it to mention %q", err, want)
		}
	}
}

func TestLoadFlattensValidPoses(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)

	posesJSON := `{"version":1,"poses":[` +
		`{"name":"idle","parts":[{"tile":0,"x":1,"y":-2,"flipX":true}]},` +
		`{"name":"walk","parts":[{"tile":0,"x":0,"y":0,"flipY":true},{"tile":0,"x":4,"y":4}]}` +
		`]}`
	if err := os.WriteFile(filepath.Join(dir, "assets", "poses.json"), []byte(posesJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	assets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets.PoseIndex) != 2 || len(assets.PoseParts) != 3 {
		t.Fatalf("unexpected pose flatten shape: index=%d parts=%d", len(assets.PoseIndex), len(assets.PoseParts))
	}
	if assets.PoseIndex[0] != (FlatPose{Start: 0, Count: 1}) {
		t.Fatalf("unexpected PoseIndex[0]: %+v", assets.PoseIndex[0])
	}
	if assets.PoseIndex[1] != (FlatPose{Start: 1, Count: 2}) {
		t.Fatalf("unexpected PoseIndex[1]: %+v", assets.PoseIndex[1])
	}
	if assets.PoseParts[0] != (FlatPosePart{Tile: 0, X: 1, Y: -2, Flags: 1}) {
		t.Fatalf("unexpected flipX pose part: %+v", assets.PoseParts[0])
	}
	if assets.PoseParts[1] != (FlatPosePart{Tile: 0, X: 0, Y: 0, Flags: 2}) {
		t.Fatalf("unexpected flipY pose part: %+v", assets.PoseParts[1])
	}
	if assets.PoseParts[2] != (FlatPosePart{Tile: 0, X: 4, Y: 4, Flags: 0}) {
		t.Fatalf("unexpected plain pose part: %+v", assets.PoseParts[2])
	}
}

func TestLoadWithoutPosesFileHasEmptyFlattenedPoses(t *testing.T) {
	dir := t.TempDir()
	sheet, mapData := nativeFixture()
	writeTestProject(t, dir, canonicalPalette(), sheet, mapData)

	assets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets.PoseIndex) != 0 || len(assets.PoseParts) != 0 {
		t.Fatalf("missing poses.json produced non-empty flatten: index=%d parts=%d", len(assets.PoseIndex), len(assets.PoseParts))
	}
}
