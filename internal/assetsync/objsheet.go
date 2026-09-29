package assetsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func LoadObjSheet(projectDir string) (*Spritesheet, error) {
	path := filepath.Join(projectDir, "assets", "objsheet.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("stat objsheet.json: %w", err)
	}
	var sheet Spritesheet
	if err := readJSON(path, &sheet); err != nil {
		return nil, fmt.Errorf("read objsheet.json: %w", err)
	}
	if _, _, _, _, err := validateObjSheetDimensions(sheet); err != nil {
		return nil, err
	}
	project, err := LoadProject(projectDir)
	if err != nil {
		return nil, err
	}
	palette, err := LoadPalette(projectDir, project.ActivePalette)
	if err != nil {
		return nil, err
	}
	if _, _, _, _, err := validateObjSheet(sheet, len(palette.Colors)); err != nil {
		return nil, err
	}
	return &sheet, nil
}

func SaveObjSheet(projectDir string, sheet *Spritesheet) error {
	if sheet == nil {
		return fmt.Errorf("object sheet is required")
	}
	project, err := LoadProject(projectDir)
	if err != nil {
		return err
	}
	palette, err := LoadPalette(projectDir, project.ActivePalette)
	if err != nil {
		return err
	}
	if _, _, _, _, err := validateObjSheet(*sheet, len(palette.Colors)); err != nil {
		return err
	}
	return writeJSON(filepath.Join(projectDir, "assets", "objsheet.json"), sheet)
}

func poseBankN(assets *Assets) int {
	if assets != nil && assets.ObjSpriteN > 0 {
		return assets.ObjSpriteN
	}
	if assets == nil {
		return 0
	}
	return assets.SpriteN
}

func poseBankName(assets *Assets) string {
	if assets != nil && assets.ObjSpriteN > 0 {
		return "object sheet"
	}
	return "spritesheet"
}

func attachObjSheet(assets *Assets, sheet *Spritesheet, paletteLen int) error {
	if assets == nil || sheet == nil {
		return nil
	}
	width, height, sheetN, spriteN, err := validateObjSheet(*sheet, paletteLen)
	if err != nil {
		return err
	}
	flat := make([]byte, sheetN)
	occupied := make([]bool, sheetN)
	seenIDs := make(map[int]struct{}, len(sheet.Sprites))
	for _, sprite := range sheet.Sprites {
		if _, exists := seenIDs[sprite.ID]; exists {
			return fmt.Errorf("objsheet.json: sprite id %d is duplicated", sprite.ID)
		}
		seenIDs[sprite.ID] = struct{}{}
		if err := validateSpriteRecord(sprite, width, height, spriteN, paletteLen); err != nil {
			return fmt.Errorf("objsheet.json: %w", err)
		}
		for y := 0; y < sprite.Height; y++ {
			for x := 0; x < sprite.Width; x++ {
				index := (sprite.Y+y)*width + sprite.X + x
				if occupied[index] {
					return fmt.Errorf("objsheet.json: sprites overlap at pixel (%d,%d)", sprite.X+x, sprite.Y+y)
				}
				occupied[index] = true
				flat[index] = byte(sprite.Pixels[y][x])
			}
		}
	}
	assets.ObjSheetW = width
	assets.ObjSheetH = height
	assets.ObjSpriteCols = width / 8
	assets.ObjSpriteRows = height / 8
	assets.ObjSpriteN = spriteN
	assets.ObjSheetROM = flat
	return nil
}

func validateObjSheet(sheet Spritesheet, paletteLen int) (width, height, sheetN, spriteN int, err error) {
	if _, _, _, _, err := validateObjSheetDimensions(sheet); err != nil {
		return 0, 0, 0, 0, err
	}
	width, height, sheetN, spriteN, err = validateSheet(sheet, paletteLen)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("objsheet.json: %w", err)
	}
	return width, height, sheetN, spriteN, nil
}

// ValidateObjSheetDimensions checks the addressable tile grid without allocating
// pixel storage, so editors can reject impossible sizes before changing data.
func ValidateObjSheetDimensions(width, height int) error {
	_, _, _, _, err := validateObjSheetDimensions(Spritesheet{Width: width, Height: height})
	return err
}

func validateObjSheetDimensions(sheet Spritesheet) (width, height, sheetN, spriteN int, err error) {
	if sheet.Width < 1 || sheet.Height < 1 {
		return 0, 0, 0, 0, fmt.Errorf("objsheet.json: dimensions must be positive, got %dx%d", sheet.Width, sheet.Height)
	}
	if sheet.Width%8 != 0 || sheet.Height%8 != 0 {
		return 0, 0, 0, 0, fmt.Errorf("objsheet.json: dimensions %dx%d must be multiples of 8", sheet.Width, sheet.Height)
	}
	sheetN, err = checkedProduct(sheet.Width, sheet.Height)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("objsheet.json: %w", err)
	}
	spriteN, err = checkedProduct(sheet.Width/8, sheet.Height/8)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("objsheet.json: %w", err)
	}
	if spriteN > MaxObjSheetCells {
		return 0, 0, 0, 0, fmt.Errorf("objsheet.json: %d cells exceed %d", spriteN, MaxObjSheetCells)
	}
	return sheet.Width, sheet.Height, sheetN, spriteN, nil
}

func CompactSheetCells(sheet []byte, sheetW, spriteN int) (compact []byte, index []uint16) {
	index = make([]uint16, spriteN)
	if len(sheet) == 0 || sheetW < 8 || spriteN <= 0 {
		return compact, index
	}
	cols := sheetW / 8
	for id := 0; id < spriteN; id++ {
		cellX, cellY := id%cols, id/cols
		nonzero := false
		for row := 0; row < 8 && !nonzero; row++ {
			start := (cellY*8+row)*sheetW + cellX*8
			if start < 0 || start+8 > len(sheet) {
				break
			}
			for _, pixel := range sheet[start : start+8] {
				if pixel != 0 {
					nonzero = true
					break
				}
			}
		}
		if !nonzero {
			continue
		}
		slot := len(compact) / 64
		index[id] = uint16(slot + 1)
		for row := 0; row < 8; row++ {
			start := (cellY*8+row)*sheetW + cellX*8
			compact = append(compact, sheet[start:start+8]...)
		}
	}
	return compact, index
}

// ObjectSheetCellAnalysis describes immutable OBJ pixel storage without changing
// authored tile IDs. Index maps each logical tile ID to its compact slot + 1;
// zero denotes an all-transparent tile. CanonicalIDs maps painted tiles to the
// first logical tile ID with exactly equal pixels, and transparent tiles to -1.
type ObjectSheetCellAnalysis struct {
	Index        []uint32
	CanonicalIDs []int
	PaintedTiles int
	UniqueTiles  int
	// SavedBytes counts duplicate pixel payloads omitted from the build ROM.
	// Transparent tiles were already implicit and do not contribute savings.
	SavedBytes int
}

// AnalyzeObjectSheetCells uses the same equality and ordering as ROM compaction,
// without retaining a second copy of the unique pixel payloads.
func AnalyzeObjectSheetCells(sheet []byte, sheetW, spriteN int) ObjectSheetCellAnalysis {
	_, analysis := compactObjectSheetCells(sheet, sheetW, spriteN, false)
	return analysis
}

// CompactObjectSheetCells stores each distinct painted tile exactly once, in
// first-logical-ID order. Logical IDs remain independent through the index.
// A wider slot index permits all 65,536 cells to contain distinct painted art:
// index zero denotes transparency, so the final cell can require slot 65,536.
func CompactObjectSheetCells(sheet []byte, sheetW, spriteN int) (compact []byte, index []uint32) {
	compact, analysis := compactObjectSheetCells(sheet, sheetW, spriteN, true)
	return compact, analysis.Index
}

func compactObjectSheetCells(sheet []byte, sheetW, spriteN int, retainPixels bool) (compact []byte, analysis ObjectSheetCellAnalysis) {
	if spriteN <= 0 || spriteN > MaxObjSheetCells {
		return compact, analysis
	}
	analysis.Index = make([]uint32, spriteN)
	analysis.CanonicalIDs = make([]int, spriteN)
	for id := range analysis.CanonicalIDs {
		analysis.CanonicalIDs[id] = -1
	}
	if len(sheet) == 0 || sheetW < 8 {
		return compact, analysis
	}
	type firstCell struct {
		slot uint32
		id   int
	}
	seen := make(map[[64]byte]firstCell)
	cols := sheetW / 8
	for id := 0; id < spriteN; id++ {
		cellX, cellY := id%cols, id/cols
		var tile [64]byte
		for row := 0; row < 8; row++ {
			start := (cellY*8+row)*sheetW + cellX*8
			if start < 0 || start+8 > len(sheet) {
				return compact, analysis
			}
			copy(tile[row*8:], sheet[start:start+8])
		}
		if tile == ([64]byte{}) {
			continue
		}
		analysis.PaintedTiles++
		first, exists := seen[tile]
		if exists {
			analysis.SavedBytes += 64
		} else {
			analysis.UniqueTiles++
			first = firstCell{slot: uint32(analysis.UniqueTiles), id: id}
			seen[tile] = first
			if retainPixels {
				compact = append(compact, tile[:]...)
			}
		}
		analysis.Index[id] = first.slot
		analysis.CanonicalIDs[id] = first.id
	}
	return compact, analysis
}
