package editor

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func (m *Model) poseColorVariantIndex(name string) int {
	if m != nil {
		for i, variant := range m.Poses.Variants {
			if variant.Name == name {
				return i
			}
		}
	}
	return -1
}

func (m *Model) validateNewPoseColorVariant(name string) error {
	if m == nil {
		return fmt.Errorf("model is required")
	}
	if name == "" || strings.TrimSpace(name) != name || len(name) > assetsync.MaxPoseNameBytes || !utf8.ValidString(name) {
		return fmt.Errorf("variant name must be nonempty, trimmed and at most %d bytes", assetsync.MaxPoseNameBytes)
	}
	if m.poseColorVariantIndex(name) >= 0 {
		return fmt.Errorf("variant %q already exists", name)
	}
	return nil
}

// CreatePoseColorVariant adds an identity variant without changing artwork.
func (m *Model) CreatePoseColorVariant(name string) error {
	if err := m.validateNewPoseColorVariant(name); err != nil {
		return err
	}
	m.Poses.Variants = append(m.Poses.Variants, assetsync.PoseColorVariant{Name: name})
	return nil
}

func (m *Model) DuplicatePoseColorVariant(sourceName, newName string) error {
	i := m.poseColorVariantIndex(sourceName)
	if i < 0 {
		return fmt.Errorf("variant %q does not exist", sourceName)
	}
	if err := m.validateNewPoseColorVariant(newName); err != nil {
		return err
	}
	copy := m.Poses.Variants[i]
	copy.Name, copy.Colors = newName, slices.Clone(copy.Colors)
	m.Poses.Variants = append(m.Poses.Variants, copy)
	return nil
}

func (m *Model) RenamePoseColorVariant(oldName, newName string) error {
	i := m.poseColorVariantIndex(oldName)
	if i < 0 {
		return fmt.Errorf("variant %q does not exist", oldName)
	}
	if oldName == newName {
		return nil
	}
	if err := m.validateNewPoseColorVariant(newName); err != nil {
		return err
	}
	m.Poses.Variants[i].Name = newName
	return nil
}

func (m *Model) DeletePoseColorVariant(name string) error {
	i := m.poseColorVariantIndex(name)
	if i < 0 {
		return fmt.Errorf("variant %q does not exist", name)
	}
	m.Poses.Variants = slices.Delete(m.Poses.Variants, i, i+1)
	return nil
}

// SetPoseColorReplacement changes one mapping; identity removes it.
func (m *Model) SetPoseColorReplacement(name string, from, to int) error {
	i := m.poseColorVariantIndex(name)
	if i < 0 {
		return fmt.Errorf("variant %q does not exist", name)
	}
	if from < 1 || from > 255 || to < 1 || to > 255 {
		return fmt.Errorf("variant colors must be in 1..255")
	}
	colors := &m.Poses.Variants[i].Colors
	for j, swap := range *colors {
		if swap.From != from {
			continue
		}
		if from == to {
			*colors = slices.Delete(*colors, j, j+1)
		} else {
			(*colors)[j].To = to
		}
		return nil
	}
	if from != to {
		*colors = append(*colors, assetsync.PoseColorSwap{From: from, To: to})
	}
	return nil
}
