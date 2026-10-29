package runtime

import "unicode/utf8"

const (
	FontBitmapWidth  = 6
	FontBitmapHeight = 10
	FontCellWidth    = 6
	FontCellHeight   = 10
)

//go:section .rodata
var emptyFontGlyph [6]uint16

// Font keeps 1bpp columns in ROM. ASCII lookup is constant-time; the bounded,
// sorted Unicode extension is searched without allocations or RAM copies.
type Font struct {
	ASCII [95][6]uint16
	Extra []FontGlyph
}
type FontGlyph struct {
	Codepoint rune
	Columns   [6]uint16
}

func (f *Font) Columns(ch rune) *[6]uint16 {
	if f == nil {
		f = DefaultFont()
		if f == nil {
			return &emptyFontGlyph
		}
	}
	if ch >= 32 && ch <= 126 {
		return &f.ASCII[ch-32]
	}
	lo, hi := 0, len(f.Extra)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if f.Extra[mid].Codepoint < ch {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(f.Extra) && f.Extra[lo].Codepoint == ch {
		return &f.Extra[lo].Columns
	}
	return &f.ASCII['?'-32]
}

func glyphColumns(ch byte) *[6]uint16 { return DefaultFont().Columns(rune(ch)) }

// SetFont borrows trusted, immutable build data. Invalid input leaves the
// previous font untouched. Dimensions are structural; bits above row 9 fail.
func (c *Context) SetFont(f *Font) bool {
	if c == nil || f == nil || len(f.Extra) > 128 {
		return false
	}
	for _, cols := range f.ASCII {
		for _, col := range cols {
			if col >= 1<<FontBitmapHeight {
				return false
			}
		}
	}
	if f.ASCII[0] != [6]uint16{} || f.ASCII['?'-32] == [6]uint16{} {
		return false
	}
	previous := rune(126)
	for _, g := range f.Extra {
		if g.Codepoint <= previous || !utf8.ValidRune(g.Codepoint) || g.Codepoint < 160 {
			return false
		}
		previous = g.Codepoint
		for _, col := range g.Columns {
			if col >= 1<<FontBitmapHeight {
				return false
			}
		}
	}
	c.pause.video.prepared = false
	c.font = f
	return true
}

// ASCII controls retain their byte parameters. Only printable text is decoded;
// malformed UTF-8 consumes one byte and draws one fallback glyph, never a panic.
func printSeqRune(s string, b []byte, i int) (rune, int) {
	ch := printSeqByte(s, b, i)
	if ch < utf8.RuneSelf {
		return rune(ch), 1
	}
	if b != nil {
		return utf8.DecodeRune(b[i:])
	}
	return utf8.DecodeRuneInString(s[i:])
}
