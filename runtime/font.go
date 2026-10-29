package runtime

import "unicode/utf8"

// Font keeps 1bpp columns in ROM. ASCII lookup is constant-time; the bounded,
// sorted Unicode extension is searched without allocations or RAM copies.
type Font struct {
	ASCII [95][4]byte
	Extra []FontGlyph
}
type FontGlyph struct {
	Codepoint rune
	Columns   [4]byte
}

func (f *Font) Columns(ch rune) [4]byte {
	if f == nil {
		f = DefaultFont()
		if f == nil {
			return [4]byte{}
		}
	}
	if ch >= 32 && ch <= 126 {
		return f.ASCII[ch-32]
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
		return f.Extra[lo].Columns
	}
	return f.ASCII['?'-32]
}

func glyphColumns(ch byte) [4]byte { return DefaultFont().Columns(rune(ch)) }

// SetFont borrows trusted, immutable build data. Invalid input leaves the
// previous font untouched. Dimensions are structural; bits above row 4 fail.
func (c *Context) SetFont(f *Font) bool {
	if c == nil || f == nil || len(f.Extra) > 128 {
		return false
	}
	for _, cols := range f.ASCII {
		for _, col := range cols {
			if col > 31 {
				return false
			}
		}
	}
	if f.ASCII[0] != [4]byte{} || f.ASCII['?'-32] == [4]byte{} {
		return false
	}
	previous := rune(126)
	for _, g := range f.Extra {
		if g.Codepoint <= previous || !utf8.ValidRune(g.Codepoint) || g.Codepoint < 160 {
			return false
		}
		previous = g.Codepoint
		for _, col := range g.Columns {
			if col > 31 {
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
