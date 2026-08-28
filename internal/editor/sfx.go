package editor

import (
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func copyNameCandidate(source string, suffix int) string {
	tail := fmt.Sprintf("-%d", suffix)
	limit := assetsync.MaxSFXNameLength - len(tail)
	for len(source) > limit {
		_, size := utf8.DecodeLastRuneInString(source)
		source = source[:len(source)-size]
	}
	return source + tail
}

const (
	defaultSFXSpeed    = 8
	defaultSFXPitch    = 24
	defaultSFXVolume   = 7
	defaultSFXWaveform = "triangle"
)

func cloneSFXAsset(effect assetsync.SFXAsset) assetsync.SFXAsset {
	effect.Steps = slices.Clone(effect.Steps)
	if effect.Loop != nil {
		loop := *effect.Loop
		effect.Loop = &loop
	}
	if effect.Filters != nil {
		filters := *effect.Filters
		effect.Filters = &filters
	}
	return effect
}

func cloneSFXLibrary(library assetsync.SFXLibrary) assetsync.SFXLibrary {
	library.SFX = slices.Clone(library.SFX)
	for index := range library.SFX {
		library.SFX[index] = cloneSFXAsset(library.SFX[index])
	}
	return library
}

// SFXAsset returns a detached copy so callers cannot accidentally bypass the
// model's whole-library validation when editing nested steps or loop bounds.
func (m *Model) SFXAsset(index int) (assetsync.SFXAsset, bool) {
	if m == nil || index < 0 || index >= len(m.SFX.SFX) {
		return assetsync.SFXAsset{}, false
	}
	return cloneSFXAsset(m.SFX.SFX[index]), true
}

// UpdateSFX validates the complete candidate library before committing an
// edit. Stable-ID and name collisions therefore fail without partially
// mutating the editor model.
func (m *Model) UpdateSFX(index int, effect assetsync.SFXAsset) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if index < 0 || index >= len(m.SFX.SFX) {
		return fmt.Errorf("SFX index %d is outside 0..%d", index, len(m.SFX.SFX)-1)
	}
	candidate := cloneSFXLibrary(m.SFX)
	oldID := candidate.SFX[index].ID
	candidate.SFX[index] = cloneSFXAsset(effect)
	if _, _, err := assetsync.CompileSFX(candidate); err != nil {
		return err
	}
	music := m.Music
	if oldID != effect.ID {
		music = retargetMusicSFX(m.Music, oldID, effect.ID)
	}
	if _, err := assetsync.CompileMusic(music, candidate); err != nil {
		return err
	}
	m.SFX = candidate
	m.Music = music
	return nil
}

func nextSFXID(library assetsync.SFXLibrary) int {
	var used uint64
	for _, effect := range library.SFX {
		if effect.ID >= 0 && effect.ID < assetsync.MaxSFXAssets {
			used |= uint64(1) << effect.ID
		}
	}
	for id := 0; id < assetsync.MaxSFXAssets; id++ {
		if used&(uint64(1)<<id) == 0 {
			return id
		}
	}
	return -1
}

func nextSFXName(library assetsync.SFXLibrary, id int) string {
	used := make(map[string]struct{}, len(library.SFX))
	for _, effect := range library.SFX {
		used[effect.Name] = struct{}{}
	}
	base := fmt.Sprintf("sound-%02d", id+1)
	if _, exists := used[base]; !exists {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

func nextSFXCopyName(library assetsync.SFXLibrary, source string) string {
	used := make(map[string]struct{}, len(library.SFX))
	for _, effect := range library.SFX {
		used[effect.Name] = struct{}{}
	}
	for suffix := 2; ; suffix++ {
		candidate := copyNameCandidate(source, suffix)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

// AddSFX appends one valid, audible effect using the first free stable ID.
func (m *Model) AddSFX() (int, error) {
	if m == nil {
		return -1, fmt.Errorf("editor model is required")
	}
	id := nextSFXID(m.SFX)
	if id < 0 {
		return -1, fmt.Errorf("SFX library already contains the maximum %d effects", assetsync.MaxSFXAssets)
	}
	effect := assetsync.SFXAsset{
		ID: id, Name: nextSFXName(m.SFX, id), Speed: defaultSFXSpeed,
		Steps: []assetsync.SFXStep{{
			Pitch: defaultSFXPitch, Waveform: defaultSFXWaveform,
			Volume: defaultSFXVolume, Effect: "none",
		}},
	}
	candidate := cloneSFXLibrary(m.SFX)
	candidate.SFX = append(candidate.SFX, effect)
	if _, _, err := assetsync.CompileSFX(candidate); err != nil {
		return -1, err
	}
	m.SFX = candidate
	return len(candidate.SFX) - 1, nil
}

// DuplicateSFX inserts an independent copy immediately after the source.
// Stable IDs and names are regenerated so the operation is transactional and
// cannot disturb music patterns that already reference the original sound.
func (m *Model) DuplicateSFX(index int) (int, error) {
	if m == nil {
		return -1, fmt.Errorf("editor model is required")
	}
	if index < 0 || index >= len(m.SFX.SFX) {
		return -1, fmt.Errorf("SFX index %d is outside 0..%d", index, len(m.SFX.SFX)-1)
	}
	id := nextSFXID(m.SFX)
	if id < 0 {
		return -1, fmt.Errorf("SFX library already contains the maximum %d effects", assetsync.MaxSFXAssets)
	}
	candidate := cloneSFXLibrary(m.SFX)
	copy := cloneSFXAsset(candidate.SFX[index])
	copy.ID = id
	copy.Name = nextSFXCopyName(candidate, copy.Name)
	candidate.SFX = slices.Insert(candidate.SFX, index+1, copy)
	if _, _, err := assetsync.CompileSFX(candidate); err != nil {
		return -1, err
	}
	if _, err := assetsync.CompileMusic(m.Music, candidate); err != nil {
		return -1, err
	}
	m.SFX = candidate
	return index + 1, nil
}

func (m *Model) DeleteSFX(index int) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if index < 0 || index >= len(m.SFX.SFX) {
		return fmt.Errorf("SFX index %d is outside 0..%d", index, len(m.SFX.SFX)-1)
	}
	candidate := cloneSFXLibrary(m.SFX)
	candidate.SFX = slices.Delete(candidate.SFX, index, index+1)
	if _, _, err := assetsync.CompileSFX(candidate); err != nil {
		return err
	}
	if _, err := assetsync.CompileMusic(m.Music, candidate); err != nil {
		return fmt.Errorf("delete SFX: %w", err)
	}
	m.SFX = candidate
	return nil
}

func (m *Model) AddSFXStep(effectIndex int) (int, error) {
	effect, ok := m.SFXAsset(effectIndex)
	if !ok {
		return -1, fmt.Errorf("SFX index %d is outside the library", effectIndex)
	}
	if len(effect.Steps) >= assetsync.MaxSFXSteps {
		return -1, fmt.Errorf("SFX %d already contains the maximum %d steps", effect.ID, assetsync.MaxSFXSteps)
	}
	step := assetsync.SFXStep{
		Pitch: defaultSFXPitch, Waveform: defaultSFXWaveform,
		Volume: defaultSFXVolume, Effect: "none",
	}
	if len(effect.Steps) != 0 {
		step = effect.Steps[len(effect.Steps)-1]
	}
	effect.Steps = append(effect.Steps, step)
	if err := m.UpdateSFX(effectIndex, effect); err != nil {
		return -1, err
	}
	return len(effect.Steps) - 1, nil
}

func (m *Model) DeleteSFXStep(effectIndex, stepIndex int) error {
	effect, ok := m.SFXAsset(effectIndex)
	if !ok {
		return fmt.Errorf("SFX index %d is outside the library", effectIndex)
	}
	if stepIndex < 0 || stepIndex >= len(effect.Steps) {
		return fmt.Errorf("SFX %d step %d is outside 0..%d", effect.ID, stepIndex, len(effect.Steps)-1)
	}
	if len(effect.Steps) == 1 {
		return fmt.Errorf("SFX %d must retain at least one step", effect.ID)
	}
	effect.Steps = slices.Delete(effect.Steps, stepIndex, stepIndex+1)
	if effect.Loop != nil {
		loop := *effect.Loop
		switch {
		case stepIndex < loop.Start:
			loop.Start--
			loop.End--
		case stepIndex < loop.End:
			loop.End--
		}
		if loop.Start >= loop.End {
			effect.Loop = nil
		} else {
			effect.Loop = &loop
		}
	}
	return m.UpdateSFX(effectIndex, effect)
}
