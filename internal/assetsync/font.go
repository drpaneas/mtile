package assetsync

import (
	"fmt"
	rt "github.com/drpaneas/mtile/runtime"
	"unicode"
	"unicode/utf8"
)

const (
	FontCellWidth      = rt.FontCellWidth
	FontCellHeight     = rt.FontCellHeight
	FontBitmapWidth    = rt.FontBitmapWidth
	FontBitmapHeight   = rt.FontBitmapHeight
	MaxFontExtraGlyphs = 128
	DefaultFontName    = "international"
)

// Font is a versioned 1bpp font document. Dimensions are explicit so a malformed
// import cannot silently truncate artwork or change print/cursor/wrap metrics.
type Font struct {
	UnknownFields UnknownFields `json:"-"`
	Version       int           `json:"version"`
	Name          string        `json:"name"`
	CellWidth     int           `json:"cellWidth"`
	CellHeight    int           `json:"cellHeight"`
	Width         int           `json:"width"`
	Height        int           `json:"height"`
	Glyphs        []FontGlyph   `json:"glyphs"`
}
type FontGlyph struct {
	UnknownFields UnknownFields `json:"-"`
	Codepoint     int           `json:"codepoint"`
	Columns       []int         `json:"columns"`
}

func ValidateFont(f *Font) error {
	if f == nil {
		return fmt.Errorf("font is required")
	}
	if f.Version != 1 && f.Version != 2 {
		return fmt.Errorf("font version must be 1 (legacy) or 2")
	}
	if err := validatePaletteName(f.Name); err != nil {
		return fmt.Errorf("font name: %w", err)
	}
	width, height, cellHeight := FontBitmapWidth, FontBitmapHeight, FontCellHeight
	if f.Version == 1 {
		width, height, cellHeight = 4, 5, 8
	}
	if f.CellWidth != FontCellWidth || f.CellHeight != cellHeight || f.Width != width || f.Height != height {
		return fmt.Errorf("font version %d must use a %dx%d bitmap in a fixed 6x%d cell", f.Version, width, height, cellHeight)
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
		if len(g.Columns) != width {
			return fmt.Errorf("font U+%04X must have exactly %d columns", g.Codepoint, width)
		}
		bits := 0
		for _, col := range g.Columns {
			if col < 0 || col >= 1<<height {
				return fmt.Errorf("font U+%04X column must be 0..%d (%d 1-bit rows)", g.Codepoint, (1<<height)-1, height)
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

// UpgradeFont returns an independent current-format document, preserving pixels
// and extension metadata. Reading/building/saving a legacy document never calls
// this: only an explicit editor copy or edit upgrades the authored file.
func UpgradeFont(f *Font) (*Font, error) {
	if err := ValidateFont(f); err != nil {
		return nil, err
	}
	out := *f
	out.Version, out.Width, out.Height = 2, FontBitmapWidth, FontBitmapHeight
	out.CellWidth, out.CellHeight = FontCellWidth, FontCellHeight
	out.Glyphs = make([]FontGlyph, len(f.Glyphs))
	for i, g := range f.Glyphs {
		out.Glyphs[i] = g
		out.Glyphs[i].Columns = make([]int, FontBitmapWidth)
		copy(out.Glyphs[i].Columns, g.Columns)
	}
	return &out, nil
}
