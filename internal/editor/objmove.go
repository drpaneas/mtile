package editor

import (
	"fmt"
	"slices"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// MoveObjSpritePixelBlock relocates a rectangle of 8x8 source tiles, including
// their flags and every registered pose reference. Coordinates and dimensions
// are in tiles. Destination cells are replaced, as with a normal paste. Source
// cells outside the destination become blank; overlapping moves use a snapshot.
// All validation finishes before either artwork or pose references change.
func (m *Model) MoveObjSpritePixelBlock(srcCol, srcRow, cols, rows, dstCol, dstRow int) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if m.ObjSheet == nil {
		return fmt.Errorf("object sheet is missing")
	}
	sheet := m.ObjSheet
	if err := assetsync.ValidateObjSheetDimensions(sheet.Width, sheet.Height); err != nil {
		return err
	}
	sheetCols, sheetRows := sheet.Width/8, sheet.Height/8
	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("tile move must have positive dimensions (got %dx%d)", cols, rows)
	}
	for _, rect := range []struct {
		name     string
		col, row int
	}{{"source", srcCol, srcRow}, {"destination", dstCol, dstRow}} {
		// Subtraction avoids overflowing when a caller supplies oversized input.
		if rect.col < 0 || rect.row < 0 || cols > sheetCols || rows > sheetRows || rect.col > sheetCols-cols || rect.row > sheetRows-rows {
			return fmt.Errorf("%dx%d tile move %s at (%d,%d) exceeds the %dx%d object sheet", cols, rows, rect.name, rect.col, rect.row, sheetCols, sheetRows)
		}
	}
	if len(m.ActivePalette.Colors) == 0 {
		return fmt.Errorf("pixel color 0 is outside active palette length 0")
	}
	contains := func(id, col, row int) bool {
		x, y := id%sheetCols, id/sheetCols
		return x >= col && x < col+cols && y >= row && y < row+rows
	}
	intersects := func(sprite assetsync.SpriteRecord, col, row int) bool {
		return sprite.X < (col+cols)*8 && sprite.X+sprite.Width > col*8 && sprite.Y < (row+rows)*8 && sprite.Y+sprite.Height > row*8
	}
	seen := make(map[int]bool, len(sheet.Sprites))
	for _, sprite := range sheet.Sprites {
		if sprite.ID < 0 || sprite.ID >= sheetCols*sheetRows || seen[sprite.ID] {
			return fmt.Errorf("object sprite id %d is invalid or duplicated", sprite.ID)
		}
		seen[sprite.ID] = true
		if sprite.Width <= 0 || sprite.Height <= 0 || sprite.X < 0 || sprite.Y < 0 || sprite.X > sheet.Width-sprite.Width || sprite.Y > sheet.Height-sprite.Height {
			return fmt.Errorf("sprite %d has invalid object sheet bounds", sprite.ID)
		}
		inSource, inDestination := contains(sprite.ID, srcCol, srcRow), contains(sprite.ID, dstCol, dstRow)
		if !inSource && !inDestination && !intersects(sprite, srcCol, srcRow) && !intersects(sprite, dstCol, dstRow) {
			continue
		}
		if sprite.X != sprite.ID%sheetCols*8 || sprite.Y != sprite.ID/sheetCols*8 || sprite.Width != 8 || sprite.Height != 8 || len(sprite.Pixels) != 8 {
			return fmt.Errorf("sprite %d is not an editable 8x8 cell", sprite.ID)
		}
		if sprite.Flags < 0 || sprite.Flags > 255 {
			return fmt.Errorf("sprite %d flags are outside 0..255", sprite.ID)
		}
		for _, pixelRow := range sprite.Pixels {
			if len(pixelRow) != 8 {
				return fmt.Errorf("sprite %d is not an editable 8x8 cell", sprite.ID)
			}
			if inSource {
				for _, color := range pixelRow {
					if color < 0 || color >= len(m.ActivePalette.Colors) {
						return fmt.Errorf("pixel color %d is outside active palette length %d", color, len(m.ActivePalette.Colors))
					}
				}
			}
		}
	}

	// Remap from each original ID once. Applying the mapping repeatedly would
	// incorrectly chase references along an overlapping source/destination row.
	delta := (dstRow-srcRow)*sheetCols + dstCol - srcCol
	poses := m.Poses
	poses.Poses = slices.Clone(poses.Poses)
	for i, pose := range poses.Poses {
		poses.Poses[i].Parts = slices.Clone(pose.Parts)
		for j, part := range pose.Parts {
			if part.Tile < 0 || part.Tile >= sheetCols*sheetRows {
				return fmt.Errorf("pose %d uses tile %d outside the object sheet", i, part.Tile)
			}
			if contains(part.Tile, srcCol, srcRow) {
				poses.Poses[i].Parts[j].Tile = part.Tile + delta
			}
		}
	}
	if srcCol == dstCol && srcRow == dstRow {
		return nil
	}

	// Deep-copy first, then retain only unrelated records and relocated source
	// records. Implicit transparent source cells also replace destination art;
	// they remain sparse rather than creating an empty record for every tile.
	updated := cloneSpritesheet(*sheet)
	records := make([]assetsync.SpriteRecord, 0, len(updated.Sprites))
	moved := make([]assetsync.SpriteRecord, 0)
	for _, sprite := range updated.Sprites {
		if contains(sprite.ID, srcCol, srcRow) {
			sprite.ID += delta
			sprite.X += (dstCol - srcCol) * 8
			sprite.Y += (dstRow - srcRow) * 8
			moved = append(moved, sprite)
		} else if !contains(sprite.ID, dstCol, dstRow) {
			records = append(records, sprite)
		}
	}
	updated.Sprites = append(records, moved...)
	m.ObjSheet, m.Poses = &updated, poses
	return nil
}
