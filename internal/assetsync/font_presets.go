package assetsync

import (
	rt "github.com/drpaneas/mtile/runtime"
	"sort"
	"strings"
)

// These tiny arcade/Greek/German patterns are original mTile artwork, inspired
// by the compact, high-contrast menus of GBA games. They are not game extracts.
func builtinFont(name string) *Font {
	f := &Font{Version: 1, Name: name, CellWidth: 6, CellHeight: 8, Width: 4, Height: 5}
	glyphs := map[rune][4]byte{}
	for cp := rune(32); cp <= 126; cp++ {
		glyphs[cp] = rt.DefaultFont().Columns(cp)
	}
	if name != "pico8" {
		for cp, pattern := range arcadePatterns {
			glyphs[cp] = patternColumns(pattern)
		}
	}
	if name == "german" || name == "international" {
		for cp, pattern := range germanPatterns {
			glyphs[cp] = patternColumns(pattern)
		}
	}
	if name == "greek" || name == "international" {
		for cp, pattern := range greekPatterns {
			glyphs[cp] = patternColumns(pattern)
		}
		for greek, latin := range map[rune]rune{'Α': 'A', 'Β': 'B', 'Ε': 'E', 'Ζ': 'Z', 'Η': 'H', 'Ι': 'I', 'Κ': 'K', 'Μ': 'M', 'Ν': 'N', 'Ο': 'O', 'Ρ': 'P', 'Τ': 'T', 'Υ': 'Y', 'Χ': 'X'} {
			glyphs[greek] = glyphs[latin]
		}
		// Tonos and dialytika reserve the first row; bodies stay inside 4x5.
		for accented, base := range map[rune]rune{'Ά': 'Α', 'Έ': 'Ε', 'Ή': 'Η', 'Ί': 'Ι', 'Ό': 'Ο', 'Ύ': 'Υ', 'Ώ': 'Ω', 'ά': 'α', 'έ': 'ε', 'ή': 'η', 'ί': 'ι', 'ό': 'ο', 'ύ': 'υ', 'ώ': 'ω', 'Ϊ': 'Ι', 'Ϋ': 'Υ', 'ϊ': 'ι', 'ϋ': 'υ', 'ΐ': 'ι', 'ΰ': 'υ'} {
			cols := glyphs[base]
			for i := range cols {
				cols[i] = (cols[i] << 1) & 31
			}
			cols[1] |= 1
			if strings.ContainsRune("ΪΫϊϋΐΰ", accented) {
				cols[0] |= 1
				cols[2] |= 1
				cols[1] &= 30
			}
			if accented == 'ΐ' || accented == 'ΰ' {
				cols[3] |= 1
			}
			glyphs[accented] = cols
		}
		glyphs[';'] = glyphs[';']
		glyphs['·'] = glyphs['.']
	}
	keys := make([]rune, 0, len(glyphs))
	for cp := range glyphs {
		keys = append(keys, cp)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, cp := range keys {
		c := glyphs[cp]
		f.Glyphs = append(f.Glyphs, FontGlyph{Codepoint: int(cp), Columns: []int{int(c[0]), int(c[1]), int(c[2]), int(c[3])}})
	}
	return f
}
func patternColumns(pattern string) [4]byte {
	var cols [4]byte
	for y, row := range strings.Split(pattern, "/") {
		for x, p := range row {
			if p == '#' {
				cols[x] |= 1 << uint(y)
			}
		}
	}
	return cols
}

var arcadePatterns = map[rune]string{
	'A': ".##./#..#/####/#..#/#..#", 'B': "###./#..#/###./#..#/###.",
	'C': ".###/#.../#.../#.../.###", 'D': "###./#..#/#..#/#..#/###.",
	'E': "####/#.../###./#.../####", 'F': "####/#.../###./#.../#...",
	'G': ".###/#.../#.##/#..#/.###", 'H': "#..#/#..#/####/#..#/#..#",
	'I': "###./.#../.#../.#../###.", 'J': "..##/...#/...#/#..#/.##.",
	'K': "#..#/#.#./##../#.#./#..#", 'L': "#.../#.../#.../#.../####",
	'M': "#..#/####/####/#..#/#..#", 'N': "#..#/##.#/#.##/#..#/#..#",
	'O': ".##./#..#/#..#/#..#/.##.", 'P': "###./#..#/###./#.../#...",
	'Q': ".##./#..#/#..#/#.##/.###", 'R': "###./#..#/###./#.#./#..#",
	'S': ".###/#.../.##./...#/###.", 'T': "###./.#../.#../.#../.#..",
	'U': "#..#/#..#/#..#/#..#/.##.", 'V': "#..#/#..#/#..#/.##./.##.",
	'W': "#..#/#..#/####/####/#..#", 'X': "#..#/#..#/.##./#..#/#..#",
	'Y': "#.#./#.#./.#../.#../.#..", 'Z': "####/...#/.##./#.../####",
}
var germanPatterns = map[rune]string{
	'Ä': "#.#./.##./#..#/####/#..#", 'Ö': "#.#./.##./#..#/#..#/.##.",
	'Ü': "#.#./#..#/#..#/#..#/.##.", 'ä': "#.#./..../.###/#..#/.###",
	'ö': "#.#./..../.##./#..#/.##.", 'ü': "#.#./..../#..#/#..#/.###",
	'ß': ".##./#..#/###./#..#/###.", 'ẞ': "###./#..#/###./#..#/#.##",
}
var greekPatterns = map[rune]string{
	'Γ': "####/#.../#.../#.../#...", 'Δ': ".##./#..#/#..#/#..#/####",
	'Θ': ".##./#..#/####/#..#/.##.", 'Λ': ".##./#..#/#..#/#..#/#..#",
	'Ξ': "####/..../.##./..../####", 'Π': "####/#..#/#..#/#..#/#..#",
	'Σ': "####/#.../.#../#.../####", 'Φ': ".#../####/#.#./####/.#..",
	'Ψ': "#.#./#.#./###./.#../.#..", 'Ω': ".##./#..#/#..#/#..#/##.#",
	'α': "..../.###/#.#./#.#./.###", 'β': ".##./#..#/###./#..#/###.",
	'γ': "..../#..#/.##./.#../.#..", 'δ': ".##./.#../.##./#..#/.##.",
	'ε': "..../.##./#.../##../.##.", 'ζ': "####/...#/.#../#.../.##.",
	'η': "..../###./#..#/#..#/...#", 'θ': ".##./#..#/####/#..#/.##.",
	'ι': "..../.#../.#../.#../..#.", 'κ': "..../#.#./##../#.#./#..#",
	'λ': "#.../.#../.##./#..#/#..#", 'μ': "..../#..#/#..#/####/#...",
	'ν': "..../#..#/#..#/.##./.#..", 'ξ': ".###/#.../.##./#.../.###",
	'ο': "..../.##./#..#/#..#/.##.", 'π': "..../####/.##./.##./#..#",
	'ρ': "..../.##./#..#/###./#...", 'ς': "..../.###/#.../.##./...#",
	'σ': "..../####/#.#./#.#./.##.", 'τ': "..../####/.#../.#../..#.",
	'υ': "..../#..#/#..#/#..#/.##.", 'φ': "..../.##./####/.##./.#..",
	'χ': "..../#..#/.##./.##./#..#", 'ψ': "..../#.#./#.#./###./.#..",
	'ω': "..../#..#/#..#/####/.##.",
}
