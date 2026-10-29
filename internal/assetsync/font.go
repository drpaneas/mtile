package assetsync

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

const (
	FontCellWidth      = 6
	FontCellHeight     = 8
	FontBitmapWidth    = 4
	FontBitmapHeight   = 5
	MaxFontExtraGlyphs = 128
	DefaultFontName    = "pico8"
)

// Font is a versioned 1bpp font document. Dimensions are explicit so a malformed
// import cannot silently truncate artwork or change print/cursor/wrap metrics.
type Font struct {
	Version    int         `json:"version"`
	Name       string      `json:"name"`
	CellWidth  int         `json:"cellWidth"`
	CellHeight int         `json:"cellHeight"`
	Width      int         `json:"width"`
	Height     int         `json:"height"`
	Glyphs     []FontGlyph `json:"glyphs"`
}
type FontGlyph struct {
	Codepoint int   `json:"codepoint"`
	Columns   []int `json:"columns"`
}

func ValidateFont(f *Font) error {
	if f == nil {
		return fmt.Errorf("font is required")
	}
	if f.Version != 1 {
		return fmt.Errorf("font version must be 1")
	}
	if err := validatePaletteName(f.Name); err != nil {
		return fmt.Errorf("font name: %w", err)
	}
	if f.CellWidth != FontCellWidth || f.CellHeight != FontCellHeight || f.Width != FontBitmapWidth || f.Height != FontBitmapHeight {
		return fmt.Errorf("font must use a 4x5 bitmap in a fixed 6x8 cell")
	}
	if len(f.Glyphs) < 95 || len(f.Glyphs) > 95+MaxFontExtraGlyphs {
		return fmt.Errorf("font requires ASCII 32..126 and at most %d extra glyphs", MaxFontExtraGlyphs)
	}
	seen := make(map[int]bool, len(f.Glyphs))
	for _, g := range f.Glyphs {
		r := rune(g.Codepoint)
		if g.Codepoint < 32 || g.Codepoint > utf8.MaxRune || !utf8.ValidRune(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.M, r) {
			return fmt.Errorf("font codepoint U+%04X must be a printable, precomposed Unicode scalar", g.Codepoint)
		}
		if seen[g.Codepoint] {
			return fmt.Errorf("font has duplicate codepoint U+%04X", g.Codepoint)
		}
		seen[g.Codepoint] = true
		if len(g.Columns) != FontBitmapWidth {
			return fmt.Errorf("font U+%04X must have exactly 4 columns", g.Codepoint)
		}
		bits := 0
		for _, col := range g.Columns {
			if col < 0 || col >= 1<<FontBitmapHeight {
				return fmt.Errorf("font U+%04X column must be 0..31 (five 1-bit rows)", g.Codepoint)
			}
			bits |= col
		}
		if g.Codepoint == 32 && bits != 0 {
			return fmt.Errorf("font space must be blank")
		}
		if g.Codepoint == '?' && bits == 0 {
			return fmt.Errorf("font question mark fallback must be visible")
		}
	}
	for cp := 32; cp <= 126; cp++ {
		if !seen[cp] {
			return fmt.Errorf("font is missing required ASCII U+%04X", cp)
		}
	}
	return nil
}
