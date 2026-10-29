package assetsync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rt "github.com/drpaneas/mtile/runtime"
)

var BuiltinFontNames = []string{"pico8", "arcade", "greek", "german", "international"}

func IsBuiltinFont(name string) bool {
	for _, n := range BuiltinFontNames {
		if name == n {
			return true
		}
	}
	return false
}

func LoadFont(dir, name string) (*Font, error) {
	if name == "" {
		name = DefaultFontName
	}
	if err := validatePaletteName(name); err != nil {
		return nil, fmt.Errorf("font name: %w", err)
	}
	if IsBuiltinFont(name) {
		return builtinFont(name), nil
	}
	var f Font
	if err := readJSON(filepath.Join(dir, "assets", "fonts", name+".json"), &f); err != nil {
		return nil, fmt.Errorf("font %q: %w", name, err)
	}
	if f.Name != name {
		return nil, fmt.Errorf("font filename and name must match")
	}
	if err := ValidateFont(&f); err != nil {
		return nil, err
	}
	return &f, nil
}
func SaveFont(dir string, f *Font) error {
	if err := ValidateFont(f); err != nil {
		return err
	}
	if IsBuiltinFont(f.Name) {
		return fmt.Errorf("built-in fonts are read-only; create a copy first")
	}
	return writeJSON(filepath.Join(dir, "assets", "fonts", f.Name+".json"), f)
}
func ListFonts(dir string) ([]string, error) {
	names := append([]string(nil), BuiltinFontNames...)
	entries, err := os.ReadDir(filepath.Join(dir, "assets", "fonts"))
	if os.IsNotExist(err) {
		return names, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			name := strings.TrimSuffix(e.Name(), ".json")
			if !IsBuiltinFont(name) {
				if _, err := LoadFont(dir, name); err != nil {
					return nil, err
				}
				names = append(names, name)
			}
		}
	}
	return names, nil
}
func CompileFont(f *Font) (*rt.Font, error) {
	if err := ValidateFont(f); err != nil {
		return nil, err
	}
	out := &rt.Font{}
	for _, g := range f.Glyphs {
		var cols [6]uint16
		for i, v := range g.Columns {
			cols[i] = uint16(v)
		}
		if g.Codepoint <= 126 {
			out.ASCII[g.Codepoint-32] = cols
		} else {
			out.Extra = append(out.Extra, rt.FontGlyph{Codepoint: rune(g.Codepoint), Columns: cols})
		}
	}
	sort.Slice(out.Extra, func(i, j int) bool { return out.Extra[i].Codepoint < out.Extra[j].Codepoint })
	return out, nil
}

// FontGoDeclaration emits only the selected font. Every bitmap is six little-endian
// uint16 columns; no alternative font, name, JSON, rasterizer or width table ships.
func FontGoDeclaration(f *rt.Font) string {
	var b strings.Builder
	b.WriteString("//go:section .rodata\nvar projectFontExtraROM = [...]rt.FontGlyph{\n")
	for _, g := range f.Extra {
		fmt.Fprintf(&b, "{Codepoint:%d,Columns:[6]uint16{%d,%d,%d,%d,%d,%d}},\n", g.Codepoint, g.Columns[0], g.Columns[1], g.Columns[2], g.Columns[3], g.Columns[4], g.Columns[5])
	}
	b.WriteString("}\n//go:section .rodata\nvar projectFontROM = rt.Font{ASCII:[95][6]uint16{\n")
	for _, c := range f.ASCII {
		fmt.Fprintf(&b, "{%d,%d,%d,%d,%d,%d},\n", c[0], c[1], c[2], c[3], c[4], c[5])
	}
	b.WriteString("},Extra:projectFontExtraROM[:]}\n")
	return b.String()
}

// CompiledFont is called after Load has validated the document. Flatten-only
// callers have no font document and retain the default selection.
func (a *Assets) CompiledFont() *rt.Font {
	if a.Font == nil {
		f, _ := LoadFont("", a.Project.Font)
		if f == nil {
			return rt.DefaultFont()
		}
		out, _ := CompileFont(f)
		return out
	}
	out, _ := CompileFont(a.Font)
	return out
}
