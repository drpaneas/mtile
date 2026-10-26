package assetsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// MaxTileLinks keeps the per-tile lookup table at one byte per shared tile.
const MaxTileLinks = 255

// TileLink makes every map occurrence of a shared tile draw as a registered
// pose or animation clip instead of the tile itself. Exactly one of Pose and
// Animation is set. The map keeps the tile ID, so mget and fget still see it.
type TileLink struct {
	UnknownFields UnknownFields `json:"-"`
	Tile          int           `json:"tile"`
	Pose          *int          `json:"pose,omitempty"`
	Animation     string        `json:"animation,omitempty"`
}

type TileLinkFile struct {
	UnknownFields UnknownFields `json:"-"`
	Version       int           `json:"version"`
	Links         []TileLink    `json:"links"`
}

// FlatTileLink is one validated link. ID is a pose ID, or an index into
// Assets.AnimationClips when Clip is true.
type FlatTileLink struct {
	Tile uint16
	Clip bool
	ID   uint16
}

func tileLinksPath(projectDir string) string {
	return filepath.Join(projectDir, "assets", "tilelinks.json")
}

// LoadTileLinks returns an empty file when the project has no links.
func LoadTileLinks(projectDir string) (*TileLinkFile, error) {
	path := tileLinksPath(projectDir)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return &TileLinkFile{Version: 1}, nil
	} else if err != nil {
		return nil, fmt.Errorf("stat tilelinks.json: %w", err)
	}
	var file TileLinkFile
	if err := readJSON(path, &file); err != nil {
		return nil, fmt.Errorf("read tilelinks.json: %w", err)
	}
	if file.Version != 1 {
		return nil, fmt.Errorf("tilelinks.json: version %d is unsupported (want 1)", file.Version)
	}
	return &file, nil
}

// SaveTileLinks writes links sorted by tile, and removes the file when there
// are none and no unknown metadata needs preserving.
func SaveTileLinks(projectDir string, file *TileLinkFile) error {
	path := tileLinksPath(projectDir)
	if file == nil || len(file.Links) == 0 && file.UnknownFields.IsZero() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove tilelinks.json: %w", err)
		}
		return nil
	}
	normalized := *file
	normalized.Version, normalized.Links = 1, slices.Clone(file.Links)
	slices.SortFunc(normalized.Links, func(a, b TileLink) int { return a.Tile - b.Tile })
	return writeJSON(path, normalized)
}

// ValidateTileLinks checks links against the shared sheet, registered poses,
// and animation clip names. Tile 0 is the map eraser and cannot be linked.
func ValidateTileLinks(file TileLinkFile, spriteN, poseCount int, clipNames []string) error {
	if len(file.Links) > MaxTileLinks {
		return fmt.Errorf("tilelinks.json: %d links exceed the limit of %d", len(file.Links), MaxTileLinks)
	}
	seen := make(map[int]bool, len(file.Links))
	for i, link := range file.Links {
		label := fmt.Sprintf("tilelinks.json: link %d (tile %d)", i, link.Tile)
		if link.Tile <= 0 || link.Tile >= spriteN {
			return fmt.Errorf("%s: tile must be 1..%d", label, spriteN-1)
		}
		if seen[link.Tile] {
			return fmt.Errorf("%s: tile is linked more than once", label)
		}
		seen[link.Tile] = true
		switch {
		case link.Pose != nil && link.Animation != "":
			return fmt.Errorf("%s: link either a pose or an animation, not both", label)
		case link.Pose != nil:
			if *link.Pose < 0 || *link.Pose >= poseCount {
				return fmt.Errorf("%s: pose %d does not exist (%d registered)", label, *link.Pose, poseCount)
			}
		case link.Animation != "":
			if !slices.Contains(clipNames, link.Animation) {
				return fmt.Errorf("%s: animation %q does not exist", label, link.Animation)
			}
		default:
			return fmt.Errorf("%s: link needs a pose or an animation", label)
		}
	}
	return nil
}

// attachTileLinks validates and flattens tilelinks.json. It must run after
// poses and animations are attached and before pose color variants expand
// the pose index.
func attachTileLinks(projectDir string, assets *Assets, poseCount int) error {
	file, err := LoadTileLinks(projectDir)
	if err != nil {
		return err
	}
	clipNames := make([]string, len(assets.AnimationClips))
	for i, clip := range assets.AnimationClips {
		clipNames[i] = clip.Name
	}
	if err := ValidateTileLinks(*file, assets.SpriteN, poseCount, clipNames); err != nil {
		return err
	}
	assets.TileLinks = FlattenTileLinks(*file, clipNames)
	return nil
}

// FlattenTileLinks converts validated links, sorted by tile.
func FlattenTileLinks(file TileLinkFile, clipNames []string) []FlatTileLink {
	links := make([]FlatTileLink, 0, len(file.Links))
	for _, link := range file.Links {
		flat := FlatTileLink{Tile: uint16(link.Tile)}
		if link.Pose != nil {
			flat.ID = uint16(*link.Pose)
		} else {
			flat.Clip = true
			flat.ID = uint16(slices.Index(clipNames, link.Animation))
		}
		links = append(links, flat)
	}
	slices.SortFunc(links, func(a, b FlatTileLink) int { return int(a.Tile) - int(b.Tile) })
	return links
}
