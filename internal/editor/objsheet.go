package editor

import (
	"fmt"
	"sort"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// ResizeObjSheet keeps artwork at its pixel coordinates and remaps every pose
// tile ID when the row stride changes. Shrinking may discard transparent space,
// but never painted pixels, flags, or a tile referenced by an animation frame.
// Validation completes before the model changes; saving follows the normal
// editor save path, and the caller can undo the entire operation as one edit.
func (m *Model) ResizeObjSheet(width, height int) error {
	if m == nil || m.ObjSheet == nil {
		return fmt.Errorf("create an animation sheet before resizing it")
	}
	if err := assetsync.ValidateObjSheetDimensions(width, height); err != nil {
		return err
	}
	old := m.ObjSheet
	if old.Width == width && old.Height == height {
		return nil
	}
	oldCols, newCols := old.Width/8, width/8
	remap := func(id int) (int, bool) {
		if id < 0 || id >= oldCols*(old.Height/8) {
			return 0, false
		}
		col, row := id%oldCols, id/oldCols
		return row*newCols + col, col < newCols && row < height/8
	}
	poses := m.Poses
	poses.Poses = make([]assetsync.PoseRecord, len(m.Poses.Poses))
	for i, pose := range m.Poses.Poses {
		poses.Poses[i] = pose
		poses.Poses[i].Parts = make([]assetsync.PosePartRecord, len(pose.Parts))
		for j, part := range pose.Parts {
			id, ok := remap(part.Tile)
			if !ok {
				return fmt.Errorf("cannot shrink: frame %d (%s) uses tile %d outside the new sheet", i, pose.Name, part.Tile)
			}
			part.Tile = id
			poses.Poses[i].Parts[j] = part
		}
	}
	cells := make(map[int]*assetsync.SpriteRecord)
	cell := func(id int) *assetsync.SpriteRecord {
		if cells[id] == nil {
			sprite := newCellSprite(id, id%newCols*8, id/newCols*8)
			cells[id] = &sprite
		}
		return cells[id]
	}
	for _, sprite := range old.Sprites {
		if sprite.Flags != 0 || !sprite.UnknownFields.IsZero() {
			id, ok := remap(sprite.ID)
			if !ok {
				return fmt.Errorf("cannot shrink: tile %d has flags or unknown metadata outside the new sheet", sprite.ID)
			}
			cell(id).Flags = sprite.Flags
			cell(id).UnknownFields = sprite.UnknownFields
		}
		for y, row := range sprite.Pixels {
			for x, color := range row {
				if color == 0 {
					continue
				}
				px, py := sprite.X+x, sprite.Y+y
				if px >= width || py >= height {
					return fmt.Errorf("cannot shrink: painted pixel (%d,%d) would be removed; erase or move that artwork first", px, py)
				}
				id := (py/8)*newCols + px/8
				cell(id).Pixels[py%8][px%8] = color
			}
		}
	}
	resized := assetsync.Spritesheet{UnknownFields: old.UnknownFields, Width: width, Height: height, Sprites: make([]assetsync.SpriteRecord, 0, len(cells))}
	for _, sprite := range cells {
		resized.Sprites = append(resized.Sprites, *sprite)
	}
	sort.Slice(resized.Sprites, func(i, j int) bool { return resized.Sprites[i].ID < resized.Sprites[j].ID })
	m.ObjSheet = &resized
	m.Poses = poses
	return nil
}
