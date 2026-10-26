package editor

import (
	"fmt"
	"slices"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// TileLinkFor returns the link of a shared tile, if any.
func (m *Model) TileLinkFor(tile int) (assetsync.TileLink, bool) {
	for _, link := range m.TileLinks.Links {
		if link.Tile == tile {
			return link, true
		}
	}
	return assetsync.TileLink{}, false
}

// LinkTileToPose makes every map occurrence of tile draw as pose id.
func (m *Model) LinkTileToPose(tile, pose int) error {
	if pose < 0 || pose >= len(m.Poses.Poses) {
		return fmt.Errorf("pose %d does not exist", pose)
	}
	return m.setTileLink(assetsync.TileLink{Tile: tile, Pose: &pose})
}

// LinkTileToAnimation makes every map occurrence of tile play clip name.
func (m *Model) LinkTileToAnimation(tile int, name string) error {
	if !slices.ContainsFunc(m.Animation.Clips, func(clip assetsync.AnimationClip) bool { return clip.Name == name }) {
		return fmt.Errorf("animation %q does not exist", name)
	}
	return m.setTileLink(assetsync.TileLink{Tile: tile, Animation: name})
}

func (m *Model) setTileLink(link assetsync.TileLink) error {
	if SpriteIsReserved(link.Tile) {
		return reservedSpriteEditError(link.Tile)
	}
	spriteN := (m.Sheet.Width / 8) * (m.Sheet.Height / 8)
	if link.Tile <= 0 || link.Tile >= spriteN {
		return fmt.Errorf("tile %d is outside the shared sheet", link.Tile)
	}
	if previous, ok := m.TileLinkFor(link.Tile); ok {
		link.UnknownFields = previous.UnknownFields
	}
	links := slices.DeleteFunc(slices.Clone(m.TileLinks.Links), func(l assetsync.TileLink) bool { return l.Tile == link.Tile })
	if len(links) >= assetsync.MaxTileLinks {
		return fmt.Errorf("a project can link at most %d tiles", assetsync.MaxTileLinks)
	}
	links = append(links, link)
	slices.SortFunc(links, func(a, b assetsync.TileLink) int { return a.Tile - b.Tile })
	m.TileLinks.Version, m.TileLinks.Links = 1, links
	return nil
}

// UnlinkTile removes a tile's link; the map draws the tile itself again.
func (m *Model) UnlinkTile(tile int) bool {
	before := len(m.TileLinks.Links)
	m.TileLinks.Links = slices.DeleteFunc(slices.Clone(m.TileLinks.Links), func(l assetsync.TileLink) bool { return l.Tile == tile })
	return len(m.TileLinks.Links) != before
}

// RemovePoseFromTileLinks mirrors pose deletion: links to the deleted pose
// are removed and links to later poses shift down, like animation frames.
func RemovePoseFromTileLinks(file *assetsync.TileLinkFile, id int) {
	if file == nil || id < 0 {
		return
	}
	// Build a new slice: undo snapshots may share the old backing array.
	links := make([]assetsync.TileLink, 0, len(file.Links))
	for _, link := range file.Links {
		if link.Pose != nil {
			pose := *link.Pose
			if pose == id {
				continue
			}
			if pose > id {
				pose--
			}
			link.Pose = &pose
		}
		links = append(links, link)
	}
	file.Links = links
}

// RenameAnimationInTileLinks keeps links attached to a renamed clip.
func RenameAnimationInTileLinks(file *assetsync.TileLinkFile, from, to string) {
	if file == nil || from == to {
		return
	}
	file.Links = slices.Clone(file.Links)
	for i := range file.Links {
		if file.Links[i].Animation == from {
			file.Links[i].Animation = to
		}
	}
}

// RemoveAnimationFromTileLinks drops links to a deleted clip.
func RemoveAnimationFromTileLinks(file *assetsync.TileLinkFile, name string) {
	if file == nil {
		return
	}
	file.Links = slices.DeleteFunc(slices.Clone(file.Links), func(l assetsync.TileLink) bool { return l.Animation == name })
}

func (m *Model) validateTileLinks() error {
	clipNames := make([]string, len(m.Animation.Clips))
	for i, clip := range m.Animation.Clips {
		clipNames[i] = clip.Name
	}
	spriteN := (m.Sheet.Width / 8) * (m.Sheet.Height / 8)
	return assetsync.ValidateTileLinks(m.TileLinks, spriteN, len(m.Poses.Poses), clipNames)
}
