package assetsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

func LoadPalette(projectDir, name string) (*Palette, error) {
	if err := validatePaletteName(name); err != nil {
		return nil, err
	}
	path := palettePath(projectDir, name)
	var palette Palette
	if err := readJSON(path, &palette); err != nil {
		if errors.Is(err, os.ErrNotExist) && name == CanonicalPaletteName {
			palette = canonicalPalette()
		} else if errors.Is(err, os.ErrNotExist) {
			available, listErr := ListPalettes(projectDir)
			if listErr == nil {
				return nil, fmt.Errorf("palette %q is missing; available palettes: %s", name, strings.Join(available, ", "))
			}
			return nil, fmt.Errorf("palette %q is missing (also could not list palettes: %w)", name, listErr)
		} else {
			return nil, fmt.Errorf("read palette %q: %w", name, err)
		}
	}
	if palette.Name == "" {
		palette.Name = name
	}
	if palette.Name != name {
		return nil, fmt.Errorf("palette file %q declares name %q", name, palette.Name)
	}
	if _, err := expandPalette(palette); err != nil {
		return nil, err
	}
	palette.Colors = slices.Clone(palette.Colors)
	return &palette, nil
}

func SavePalette(projectDir string, palette *Palette) error {
	if palette == nil {
		return fmt.Errorf("palette is required")
	}
	if _, err := expandPalette(*palette); err != nil {
		return err
	}
	return writeJSON(palettePath(projectDir, palette.Name), palette)
}

func ListPalettes(projectDir string) ([]string, error) {
	dir := filepath.Join(projectDir, "assets", "palettes")
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read palettes: %w", err)
	}
	names := []string{CanonicalPaletteName}
	seen := map[string]bool{CanonicalPaletteName: true}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := entry.Name()[:len(entry.Name())-len(filepath.Ext(entry.Name()))]
		if err := validatePaletteName(name); err != nil || seen[name] {
			continue
		}
		names = append(names, name)
		seen[name] = true
	}
	sort.Strings(names)
	return names, nil
}

func CreatePalette(projectDir string, palette Palette) error {
	if err := validatePaletteName(palette.Name); err != nil {
		return err
	}
	if palette.Name == CanonicalPaletteName {
		// The built-in palette is logically present even when its optional
		// on-disk override has not been materialized yet. Creation and rename
		// must not be able to claim that reserved library name; SavePalette is
		// still allowed for editing the active canonical palette.
		return fmt.Errorf("palette %q already exists", CanonicalPaletteName)
	}
	if _, err := os.Stat(palettePath(projectDir, palette.Name)); err == nil {
		return fmt.Errorf("palette %q already exists", palette.Name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return SavePalette(projectDir, &palette)
}

func DuplicatePalette(projectDir, sourceName, destinationName string) error {
	source, err := LoadPalette(projectDir, sourceName)
	if err != nil {
		return err
	}
	source.Name = destinationName
	return CreatePalette(projectDir, *source)
}

func RenamePalette(projectDir, oldName, newName string) error {
	if err := RenamePaletteUnchecked(projectDir, oldName, newName); err != nil {
		return err
	}
	project, err := LoadProject(projectDir)
	if err != nil {
		return err
	}
	if project.ActivePalette == oldName {
		project.ActivePalette = newName
		return SaveProject(projectDir, project)
	}
	return nil
}

// RenamePaletteUnchecked renames a palette file without consulting or
// updating project.json. Editor.Model uses this variant because its active
// palette selection is authoritative in memory until Model.Save; consulting
// disk here could rewrite a stale project selection as a side effect of
// renaming an otherwise inactive palette.
func RenamePaletteUnchecked(projectDir, oldName, newName string) error {
	if err := validatePaletteName(oldName); err != nil {
		return err
	}
	if err := validatePaletteName(newName); err != nil {
		return err
	}
	if oldName == CanonicalPaletteName {
		return fmt.Errorf("the built-in %q palette cannot be renamed", CanonicalPaletteName)
	}
	if newName == CanonicalPaletteName {
		return fmt.Errorf("palette %q already exists", CanonicalPaletteName)
	}
	if _, err := os.Stat(palettePath(projectDir, newName)); err == nil {
		return fmt.Errorf("palette %q already exists", newName)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	p, err := LoadPalette(projectDir, oldName)
	if err != nil {
		return err
	}
	p.Name = newName
	if err := SavePalette(projectDir, p); err != nil {
		return err
	}
	if err := os.Remove(palettePath(projectDir, oldName)); err != nil {
		// Do not leave a duplicate if removing the source fails. The cleanup is
		// best effort because the source error is the useful diagnostic.
		_ = os.Remove(palettePath(projectDir, newName))
		return fmt.Errorf("remove old palette %q: %w", oldName, err)
	}
	return nil
}

func DeletePalette(projectDir, name string) error {
	if err := rejectUndeletablePaletteName(name); err != nil {
		return err
	}
	project, err := LoadProject(projectDir)
	if err != nil {
		return err
	}
	if project.ActivePalette == name {
		return fmt.Errorf("cannot delete active palette %q; select another palette first", name)
	}
	return removePaletteFile(projectDir, name)
}

// DeletePaletteUnchecked removes a palette file without consulting the
// on-disk project.json's active-palette selection. It still rejects the
// canonical palette name. This is for callers like editor.Model that track
// "active" in memory and may have an unsaved SelectPalette pending: since
// only Model.Save writes project.json, the on-disk active palette can
// legitimately lag the in-memory one, and DeletePalette's disk-based check
// would otherwise reject a delete that the model's own (authoritative,
// current) in-memory check has already approved.
func DeletePaletteUnchecked(projectDir, name string) error {
	if err := rejectUndeletablePaletteName(name); err != nil {
		return err
	}
	return removePaletteFile(projectDir, name)
}

func rejectUndeletablePaletteName(name string) error {
	if err := validatePaletteName(name); err != nil {
		return err
	}
	if name == CanonicalPaletteName {
		return fmt.Errorf("the built-in %q palette cannot be deleted", CanonicalPaletteName)
	}
	return nil
}

func removePaletteFile(projectDir, name string) error {
	if err := os.Remove(palettePath(projectDir, name)); err != nil {
		return fmt.Errorf("remove palette %q: %w", name, err)
	}
	return nil
}

func SetActivePalette(projectDir, name string) error {
	if _, err := LoadPalette(projectDir, name); err != nil {
		return err
	}
	project, err := LoadProject(projectDir)
	if err != nil {
		return err
	}
	project.ActivePalette = name
	return SaveProject(projectDir, project)
}

func palettePath(projectDir, name string) string {
	return filepath.Join(projectDir, "assets", "palettes", name+".json")
}
