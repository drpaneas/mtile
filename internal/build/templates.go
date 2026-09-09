package build

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/drpaneas/mtile/internal/assetsync"
)

// Keep the walkthrough and checkpoints available in installed binaries too.
//
//go:embed templates/coin-quest
var projectTemplates embed.FS

func writeCoinQuest(dir string, sheet *assetsync.Spritesheet) error {
	const root = "templates/coin-quest"
	files, err := fs.Sub(projectTemplates, root)
	if err != nil {
		return err
	}
	if err := fs.WalkDir(files, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		target := filepath.Join(dir, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(files, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		return err
	}
	// Original 8x8 artwork using Aurora: blue hat, teal coat, gold coin.
	// Zero is transparent; letters below refer to palette indices, not RGB.
	colors := map[rune]int{'0': 0, '1': 24, '3': 48, '7': 63, '8': 17, 'a': 40, 'f': 15}
	for index, rows := range [][]string{
		{"00333300", "03333330", "00777700", "00717000", "00888800", "08888880", "00800800", "00700700"},
		{"000aa000", "00aaaa00", "0aafaaa0", "0aafaaa0", "0aafaaa0", "0aaaaaa0", "00aaaa00", "000aa000"},
	} {
		pixels := make([][]int, 8)
		for y, row := range rows {
			pixels[y] = make([]int, 8)
			for x, ch := range row {
				pixels[y][x] = colors[ch]
			}
		}
		sheet.Sprites = append(sheet.Sprites, assetsync.SpriteRecord{ID: index + 1, X: (index + 1) * 8, Width: 8, Height: 8, Pixels: pixels})
	}
	return assetsync.SaveSpritesheet(dir, sheet)
}
