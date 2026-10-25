package assetsync

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func intPtr(v int) *int { return &v }

func TestValidateTileLinksRejectsInvalidLinks(t *testing.T) {
	clips := []string{"sway"}
	for _, test := range []struct {
		name  string
		links []TileLink
		want  string
	}{
		{"map eraser", []TileLink{{Tile: 0, Pose: intPtr(0)}}, "tile must be 1.."},
		{"outside sheet", []TileLink{{Tile: 16, Pose: intPtr(0)}}, "tile must be 1.."},
		{"duplicate", []TileLink{{Tile: 3, Pose: intPtr(0)}, {Tile: 3, Animation: "sway"}}, "more than once"},
		{"both targets", []TileLink{{Tile: 3, Pose: intPtr(0), Animation: "sway"}}, "not both"},
		{"no target", []TileLink{{Tile: 3}}, "needs a pose or an animation"},
		{"missing pose", []TileLink{{Tile: 3, Pose: intPtr(2)}}, "pose 2 does not exist"},
		{"missing clip", []TileLink{{Tile: 3, Animation: "walk"}}, `animation "walk" does not exist`},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateTileLinks(TileLinkFile{Version: 1, Links: test.links}, 16, 2, clips)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	many := TileLinkFile{Version: 1}
	for i := 1; i <= MaxTileLinks+1; i++ {
		many.Links = append(many.Links, TileLink{Tile: i, Pose: intPtr(0)})
	}
	if err := ValidateTileLinks(many, 1024, 1, nil); err == nil || !strings.Contains(err.Error(), "exceed the limit") {
		t.Fatalf("over-limit error = %v", err)
	}
}

func TestTileLinksSaveLoadFlatten(t *testing.T) {
	dir := t.TempDir()
	file := &TileLinkFile{Version: 1, Links: []TileLink{{Tile: 9, Animation: "sway"}, {Tile: 4, Pose: intPtr(1)}}}
	if err := SaveTileLinks(dir, file); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTileLinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Links) != 2 || loaded.Links[0].Tile != 4 || loaded.Links[1].Tile != 9 {
		t.Fatalf("saved links are not sorted by tile: %+v", loaded.Links)
	}
	if err := ValidateTileLinks(*loaded, 16, 2, []string{"idle", "sway"}); err != nil {
		t.Fatal(err)
	}
	flat := FlattenTileLinks(*loaded, []string{"idle", "sway"})
	want := []FlatTileLink{{Tile: 4, ID: 1}, {Tile: 9, Clip: true, ID: 1}}
	if len(flat) != len(want) || flat[0] != want[0] || flat[1] != want[1] {
		t.Fatalf("flattened = %+v, want %+v", flat, want)
	}
	// Removing every link removes the file, so unlinked projects stay unchanged.
	if err := SaveTileLinks(dir, &TileLinkFile{Version: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "tilelinks.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty links left tilelinks.json behind: %v", err)
	}
	empty, err := LoadTileLinks(dir)
	if err != nil || len(empty.Links) != 0 {
		t.Fatalf("missing file = %+v, %v; want no links", empty, err)
	}
}

func TestLoadTileLinksRejectsUnknownVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "tilelinks.json"), []byte(`{"version":2,"links":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTileLinks(dir); err == nil || !strings.Contains(err.Error(), "version 2") {
		t.Fatalf("error = %v, want unsupported version", err)
	}
}
