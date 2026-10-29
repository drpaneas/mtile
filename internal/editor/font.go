package editor

import (
	"fmt"
	"github.com/drpaneas/mtile/internal/assetsync"
	"sort"
)

func (m *Model) LoadFonts() error {
	names, err := assetsync.ListFonts(m.Dir)
	if err != nil {
		return err
	}
	fonts := make(map[string]*assetsync.Font, len(names))
	for _, name := range names {
		f, err := assetsync.LoadFont(m.Dir, name)
		if err != nil {
			return err
		}
		fonts[name] = f
	}
	selected := m.Project.Font
	if selected == "" {
		selected = assetsync.DefaultFontName
	}
	if fonts[selected] == nil {
		return fmt.Errorf("selected font %q is missing", selected)
	}
	m.Fonts = fonts
	return nil
}
func (m *Model) FontNames() []string {
	names := append([]string(nil), assetsync.BuiltinFontNames...)
	custom := []string{}
	for name := range m.Fonts {
		if !assetsync.IsBuiltinFont(name) {
			custom = append(custom, name)
		}
	}
	sort.Strings(custom)
	return append(names, custom...)
}
func (m *Model) ActiveFont() *assetsync.Font {
	name := m.Project.Font
	if name == "" {
		name = assetsync.DefaultFontName
	}
	return m.Fonts[name]
}
func (m *Model) SelectFont(name string) error {
	if m.Fonts[name] == nil {
		return fmt.Errorf("font %q is missing", name)
	}
	m.Project.Font = name
	return nil
}

// CreateFont starts with a copy, or a blank international repertoire. A visible
// question mark is retained so missing glyphs are always discoverable.
func (m *Model) CreateFont(name string, blank bool) error {
	if m.Fonts[name] != nil || assetsync.IsBuiltinFont(name) {
		return fmt.Errorf("font %q already exists", name)
	}
	source := m.ActiveFont()
	if blank {
		source, _ = assetsync.LoadFont("", "international")
	}
	if source == nil {
		return fmt.Errorf("select a source font first")
	}
	f, err := assetsync.UpgradeFont(source)
	if err != nil {
		return err
	}
	f.Name = name
	if blank {
		for i, g := range f.Glyphs {
			if g.Codepoint != '?' {
				clear(f.Glyphs[i].Columns)
			}
		}
	}
	if err := assetsync.ValidateFont(f); err != nil {
		return err
	}
	m.Fonts[name] = f
	m.Project.Font = name
	return nil
}
func (m *Model) SetFontPixel(cp, x, y int, on bool) error {
	f := m.ActiveFont()
	if f == nil {
		return fmt.Errorf("select a font first")
	}
	if assetsync.IsBuiltinFont(f.Name) {
		return fmt.Errorf("built-in font is read-only; choose COPY first")
	}
	if x < 0 || x >= assetsync.FontBitmapWidth || y < 0 || y >= assetsync.FontBitmapHeight {
		return fmt.Errorf("font pixels must stay inside the fixed 6x10 cell")
	}
	upgraded, err := assetsync.UpgradeFont(f)
	if err != nil {
		return err
	}
	f = upgraded
	for i, g := range f.Glyphs {
		if g.Codepoint == cp {
			changed := g
			changed.Columns = append([]int(nil), g.Columns...)
			changed.Columns[x] &= ^(1 << uint(y))
			if on {
				changed.Columns[x] |= 1 << uint(y)
			}
			candidate := *f
			candidate.Glyphs = append([]assetsync.FontGlyph(nil), f.Glyphs...)
			candidate.Glyphs[i] = changed
			if err := assetsync.ValidateFont(&candidate); err != nil {
				return err
			}
			m.Fonts[f.Name] = &candidate
			return nil
		}
	}
	return fmt.Errorf("glyph U+%04X is not in this font", cp)
}
func (m *Model) AddFontGlyph(cp int) error {
	f := m.ActiveFont()
	if f == nil {
		return fmt.Errorf("select a font first")
	}
	if assetsync.IsBuiltinFont(f.Name) {
		return fmt.Errorf("choose COPY before adding a glyph")
	}
	for _, g := range f.Glyphs {
		if g.Codepoint == cp {
			return nil
		}
	}
	upgraded, err := assetsync.UpgradeFont(f)
	if err != nil {
		return err
	}
	f = upgraded
	candidate := *f
	candidate.Glyphs = append(append([]assetsync.FontGlyph(nil), f.Glyphs...), assetsync.FontGlyph{Codepoint: cp, Columns: make([]int, assetsync.FontBitmapWidth)})
	if err := assetsync.ValidateFont(&candidate); err != nil {
		return err
	}
	sort.Slice(candidate.Glyphs, func(i, j int) bool { return candidate.Glyphs[i].Codepoint < candidate.Glyphs[j].Codepoint })
	m.Fonts[f.Name] = &candidate
	return nil
}
func (m *Model) validateFonts() error {
	if m.Fonts == nil {
		return m.LoadFonts()
	}
	f := m.ActiveFont()
	if f == nil {
		return fmt.Errorf("selected font is missing")
	}
	for name, f := range m.Fonts {
		if f.Name != name {
			return fmt.Errorf("font name mismatch")
		}
		if err := assetsync.ValidateFont(f); err != nil {
			return err
		}
	}
	return nil
}
func (m *Model) saveFonts() error {
	for name, f := range m.Fonts {
		if !assetsync.IsBuiltinFont(name) {
			if err := assetsync.SaveFont(m.Dir, f); err != nil {
				return err
			}
		}
	}
	return nil
}
