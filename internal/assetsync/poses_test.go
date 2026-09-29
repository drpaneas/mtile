package assetsync

import (
	"math"
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
	if assets.PoseIndex[0] != (FlatPose{Start: 0, Count: 1, FlipOffsetX: 10, FlipOffsetY: 4}) {
		t.Fatalf("unexpected PoseIndex[0]: %+v", assets.PoseIndex[0])
	}
	if assets.PoseIndex[1] != (FlatPose{Start: 1, Count: 2, FlipOffsetX: 12, FlipOffsetY: 12}) {
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

func TestSavePosesSkipsMissingEmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := SavePoses(dir, &PoseFile{Version: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "poses.json")); !os.IsNotExist(err) {
		t.Fatalf("empty save created poses.json: %v", err)
	}
}

func TestSavePosesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := &PoseFile{Version: 1, Poses: []PoseRecord{
		{Name: "idle_0", Parts: []PosePartRecord{{Tile: 3, X: 8, Y: 16, FlipX: true}}},
	}}
	if err := SavePoses(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPoses(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Poses) != 1 || got.Poses[0].Name != "idle_0" {
		t.Fatalf("loaded = %+v", got)
	}
	part := got.Poses[0].Parts[0]
	if part.Tile != 3 || part.X != 8 || part.Y != 16 || !part.FlipX {
		t.Fatalf("part = %+v", part)
	}
}

func TestPoseOffsetsCrossUint16WithoutWrapping(t *testing.T) {
	poses := PoseFile{Version: 1, Poses: make([]PoseRecord, 8193)}
	for i := range poses.Poses {
		poses.Poses[i].Parts = make([]PosePartRecord, 8)
	}
	index, parts, err := flattenPoses(poses, 1, "object sheet")
	if err != nil {
		t.Fatal(err)
	}
	if index[8192].Start != 65536 || len(parts) != 65544 {
		t.Fatalf("last span = %+v, total parts = %d", index[8192], len(parts))
	}
}

func TestPoseLimitsReflectRepresentableIDsAndPartCounts(t *testing.T) {
	poses := PoseFile{Version: 1, Poses: make([]PoseRecord, MaxPoseCount)}
	if err := validatePoseFile(poses); err != nil {
		t.Fatal(err)
	}
	poses.Poses = append(poses.Poses, PoseRecord{})
	if err := validatePoseFile(poses); err == nil {
		t.Fatal("accepted unrepresentable Lua pose ID")
	}
	poses.Poses = []PoseRecord{{Parts: make([]PosePartRecord, MaxPoseParts)}}
	if err := validatePoseFile(poses); err != nil {
		t.Fatal(err)
	}
	poses.Poses[0].Parts = append(poses.Poses[0].Parts, PosePartRecord{})
	if err := validatePoseFile(poses); err == nil {
		t.Fatal("accepted overflowing uint16 part count")
	}
}

func TestPoseLoadAndSaveRejectUnrepresentableRawParts(t *testing.T) {
	validPart := PosePartRecord{Tile: math.MaxUint16, X: math.MinInt16, Y: math.MaxInt16}
	valid := &PoseFile{Version: 1, Poses: []PoseRecord{{Parts: []PosePartRecord{validPart}}}}
	for _, test := range []struct {
		name, field string
		part        PosePartRecord
	}{
		{"negative tile", "tile", PosePartRecord{Tile: -1}},
		{"tile overflow", "tile", PosePartRecord{Tile: math.MaxUint16 + 1}},
		{"x underflow", "x", PosePartRecord{X: math.MinInt16 - 1}},
		{"x overflow", "x", PosePartRecord{X: math.MaxInt16 + 1}},
		{"y underflow", "y", PosePartRecord{Y: math.MinInt16 - 1}},
		{"y overflow", "y", PosePartRecord{Y: math.MaxInt16 + 1}},
		{"extreme preview coordinate", "x", PosePartRecord{X: math.MaxInt}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "assets", "poses.json")
			bad := &PoseFile{Version: 1, Poses: []PoseRecord{{Parts: []PosePartRecord{test.part}}}}
			if err := writeJSON(path, bad); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadPoses(dir); err == nil || !strings.Contains(err.Error(), "pose 0 part 0 "+test.field) {
				t.Fatalf("invalid part loaded: %v", err)
			}
			if err := SavePoses(dir, valid); err != nil {
				t.Fatalf("representable boundary rejected: %v", err)
			}
			loaded, err := LoadPoses(dir)
			if err != nil || loaded.Poses[0].Parts[0] != validPart {
				t.Fatalf("boundary part changed: %+v error=%v", loaded, err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := SavePoses(dir, bad); err == nil || !strings.Contains(err.Error(), "pose 0 part 0 "+test.field) {
				t.Fatalf("invalid part saved: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("rejected save changed existing poses.json")
			}
		})
	}
}
