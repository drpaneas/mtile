package editor

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// PosePreview is a composed pspr frame in sheet pixels. Index 0 is
// transparent, matching an unauthored cell.
type PosePreview struct {
	Width  int
	Height int
	Pixels []int
}

func PoseFrameLabel(index int, pose assetsync.PoseRecord) string {
	if pose.Name == "" {
		return fmt.Sprintf("%d", index)
	}
	return fmt.Sprintf("%d  %s", index, pose.Name)
}

func NextPoseName(poses []assetsync.PoseRecord, prefix string) string {
	if prefix == "" {
		prefix = "idle"
	}
	used := make(map[string]struct{}, len(poses))
	for _, pose := range poses {
		if pose.Name != "" {
			used[pose.Name] = struct{}{}
		}
	}
	for i := 0; i <= len(poses)+1; i++ {
		name := fmt.Sprintf("%s_%d", prefix, i)
		if _, exists := used[name]; !exists {
			return name
		}
	}
	return fmt.Sprintf("%s_%d", prefix, len(poses))
}

func NormalizePoseName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	if len(name) > assetsync.MaxPoseNameBytes {
		return "", fmt.Errorf("pose name is %d bytes (maximum %d)", len(name), assetsync.MaxPoseNameBytes)
	}
	for _, r := range name {
		if r > unicode.MaxASCII || unicode.IsControl(r) || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return "", fmt.Errorf("pose name must be letters, numbers, _ or -")
		}
	}
	return name, nil
}

func RenamePose(file *assetsync.PoseFile, index int, name string) error {
	if file == nil {
		return fmt.Errorf("poses file is required")
	}
	if index < 0 || index >= len(file.Poses) {
		return fmt.Errorf("pspr %d does not exist", index)
	}
	normalized, err := NormalizePoseName(name)
	if err != nil {
		return err
	}
	if normalized != "" {
		for i, pose := range file.Poses {
			if i != index && pose.Name == normalized {
				return fmt.Errorf("pose name %q is already used by pspr %d", normalized, i)
			}
		}
	}
	file.Poses[index].Name = normalized
	return nil
}

func PoseContainsTile(pose assetsync.PoseRecord, tile int) bool {
	for _, part := range pose.Parts {
		if part.Tile == tile {
			return true
		}
	}
	return false
}

func FirstPoseContainingTile(poses []assetsync.PoseRecord, tile int) int {
	for index, pose := range poses {
		if PoseContainsTile(pose, tile) {
			return index
		}
	}
	return -1
}

func PoseSheetBounds(pose assetsync.PoseRecord, cols, rows int) (col, row, width, height int, ok bool) {
	if cols <= 0 || rows <= 0 {
		return 0, 0, 0, 0, false
	}
	minCol, minRow := math.MaxInt, math.MaxInt
	maxCol, maxRow := -1, -1
	for _, part := range pose.Parts {
		if part.Tile < 0 {
			continue
		}
		partCol := part.Tile % cols
		partRow := part.Tile / cols
		if partRow < 0 || partRow >= rows || partCol < 0 || partCol >= cols {
			continue
		}
		if partCol < minCol {
			minCol = partCol
		}
		if partRow < minRow {
			minRow = partRow
		}
		if partCol > maxCol {
			maxCol = partCol
		}
		if partRow > maxRow {
			maxRow = partRow
		}
	}
	if maxCol < 0 {
		return 0, 0, 0, 0, false
	}
	return minCol, minRow, maxCol - minCol + 1, maxRow - minRow + 1, true
}

// PosePixelBounds calculates layout without allocating or composing pixels.
func PosePixelBounds(sheet assetsync.Spritesheet, pose assetsync.PoseRecord) (minX, minY, width, height int) {
	minX, minY = math.MaxInt, math.MaxInt
	maxX, maxY := math.MinInt, math.MinInt
	for _, part := range pose.Parts {
		if !posePartOnSheet(part.Tile, sheet.Width/8, sheet.Height/8) {
			continue
		}
		minX, minY = min(minX, part.X), min(minY, part.Y)
		maxX, maxY = max(maxX, part.X+8), max(maxY, part.Y+8)
	}
	if maxX <= minX || maxY <= minY {
		return 0, 0, 0, 0
	}
	return minX, minY, maxX - minX, maxY - minY
}

func ComposePose(sheet assetsync.Spritesheet, pose assetsync.PoseRecord) PosePreview {
	minX, minY, width, height := PosePixelBounds(sheet, pose)
	if width == 0 || height == 0 {
		return PosePreview{}
	}
	// Sparse poses may span the full signed coordinate range. A thumbnail
	// must never allocate that bounding box at its original pixel resolution.
	divisor := max(1, (max(width, height)+511)/512)
	width, height = (width+divisor-1)/divisor, (height+divisor-1)/divisor
	pixels := make([]int, width*height)
	wanted := make(map[int]bool, len(pose.Parts))
	for _, part := range pose.Parts {
		wanted[part.Tile] = true
	}
	tiles := readSheetTiles(sheet, wanted)
	for _, part := range pose.Parts {
		tile, ok := tiles[part.Tile]
		if !ok {
			continue
		}
		for py := 0; py < 8; py++ {
			for px := 0; px < 8; px++ {
				sx, sy := px, py
				if part.FlipX {
					sx = 7 - px
				}
				if part.FlipY {
					sy = 7 - py
				}
				color := tile[sy*8+sx]
				if color == 0 {
					continue
				}
				dx, dy := (part.X+px-minX)/divisor, (part.Y+py-minY)/divisor
				pixels[dy*width+dx] = color
			}
		}
	}
	return PosePreview{Width: width, Height: height, Pixels: pixels}
}

// readSheetTiles visits authored rectangles once, avoiding a full sheet-record
// scan for every pixel of every animation thumbnail. Rectangular imports and
// ordinary 8x8 editor cells share the same path.
func readSheetTiles(sheet assetsync.Spritesheet, wanted map[int]bool) map[int][64]int {
	out := make(map[int][64]int, len(wanted))
	cols := sheet.Width / 8
	if cols <= 0 {
		return out
	}
	for _, sprite := range sheet.Sprites {
		for row := max(0, sprite.Y/8); row < min(sheet.Height/8, (sprite.Y+sprite.Height+7)/8); row++ {
			for col := max(0, sprite.X/8); col < min(cols, (sprite.X+sprite.Width+7)/8); col++ {
				id := row*cols + col
				if !wanted[id] {
					continue
				}
				tile := out[id]
				for py := max(row*8, sprite.Y); py < min(row*8+8, sprite.Y+sprite.Height); py++ {
					for px := max(col*8, sprite.X); px < min(col*8+8, sprite.X+sprite.Width); px++ {
						sy, sx := py-sprite.Y, px-sprite.X
						if sy < len(sprite.Pixels) && sx < len(sprite.Pixels[sy]) {
							tile[(py%8)*8+px%8] = sprite.Pixels[sy][sx]
						}
					}
				}
				out[id] = tile
			}
		}
	}
	return out
}

func CapturePose(sheet assetsync.Spritesheet, col, row, width, height int) (assetsync.PoseRecord, error) {
	cols := sheet.Width / 8
	rows := sheet.Height / 8
	if width <= 0 || height <= 0 {
		return assetsync.PoseRecord{}, fmt.Errorf("pose selection is empty")
	}
	if cols <= 0 || rows <= 0 || col < 0 || row < 0 || col+width > cols || row+height > rows {
		return assetsync.PoseRecord{}, fmt.Errorf("pose selection is outside the sheet")
	}
	wanted := make(map[int]bool, width*height)
	for dy := 0; dy < height; dy++ {
		for dx := 0; dx < width; dx++ {
			wanted[(row+dy)*cols+col+dx] = true
		}
	}
	tiles := readSheetTiles(sheet, wanted)
	parts := make([]assetsync.PosePartRecord, 0, width*height)
	for dy := 0; dy < height; dy++ {
		for dx := 0; dx < width; dx++ {
			tile := tiles[(row+dy)*cols+col+dx]
			ink := false
			for _, color := range tile {
				ink = ink || color != 0
			}
			if !ink {
				continue
			}
			parts = append(parts, assetsync.PosePartRecord{
				Tile: (row+dy)*cols + (col + dx),
				X:    dx * 8,
				Y:    dy * 8,
			})
		}
	}
	if len(parts) == 0 {
		return assetsync.PoseRecord{}, fmt.Errorf("pose selection has no painted tiles")
	}
	if len(parts) > assetsync.MaxPoseParts {
		return assetsync.PoseRecord{}, fmt.Errorf("pose selection has %d painted tiles (maximum %d parts)", len(parts), assetsync.MaxPoseParts)
	}
	return assetsync.PoseRecord{Parts: parts}, nil
}

func sheetTileHasInk(sheet assetsync.Spritesheet, col, row int) bool {
	baseX, baseY := col*8, row*8
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if sheetPixel(sheet, baseX+x, baseY+y) != 0 {
				return true
			}
		}
	}
	return false
}

func TogglePosePart(pose assetsync.PoseRecord, tile, sheetCols int) (assetsync.PoseRecord, error) {
	parts := append([]assetsync.PosePartRecord(nil), pose.Parts...)
	for i, part := range parts {
		if part.Tile != tile {
			continue
		}
		pose.Parts = append(parts[:i], parts[i+1:]...)
		return pose, nil
	}
	if len(parts) >= assetsync.MaxPoseParts {
		return pose, fmt.Errorf("pose already has %d parts", assetsync.MaxPoseParts)
	}
	if sheetCols <= 0 || tile < 0 {
		return pose, fmt.Errorf("pose tile %d is outside the sheet", tile)
	}
	col, row := tile%sheetCols, tile/sheetCols
	x, y := col*8, row*8
	if originX, originY, ok := posePixelOrigin(pose, sheetCols); ok {
		x -= originX
		y -= originY
	}
	pose.Parts = append(parts, assetsync.PosePartRecord{Tile: tile, X: x, Y: y})
	return pose, nil
}

func posePixelOrigin(pose assetsync.PoseRecord, cols int) (int, int, bool) {
	if cols <= 0 {
		return 0, 0, false
	}
	for _, part := range pose.Parts {
		if part.Tile < 0 {
			continue
		}
		return (part.Tile%cols)*8 - part.X, (part.Tile/cols)*8 - part.Y, true
	}
	return 0, 0, false
}

func posePartOnSheet(tile, cols, rows int) bool {
	if tile < 0 || cols <= 0 || rows <= 0 {
		return false
	}
	row := tile / cols
	col := tile % cols
	return row >= 0 && row < rows && col >= 0 && col < cols
}

func sheetPixel(sheet assetsync.Spritesheet, x, y int) int {
	if x < 0 || y < 0 || x >= sheet.Width || y >= sheet.Height {
		return 0
	}
	for _, sprite := range sheet.Sprites {
		if x < sprite.X || y < sprite.Y || x >= sprite.X+sprite.Width || y >= sprite.Y+sprite.Height {
			continue
		}
		ly := y - sprite.Y
		lx := x - sprite.X
		if ly < 0 || ly >= len(sprite.Pixels) || lx < 0 || lx >= len(sprite.Pixels[ly]) {
			return 0
		}
		return sprite.Pixels[ly][lx]
	}
	return 0
}
