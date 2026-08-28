package editor

import (
	"fmt"
	"slices"

	"github.com/drpaneas/mtile/internal/assetsync"
)

func cloneMusicPattern(pattern assetsync.MusicPattern) assetsync.MusicPattern {
	pattern.Channels = slices.Clone(pattern.Channels)
	if pattern.Next != nil {
		next := *pattern.Next
		pattern.Next = &next
	}
	return pattern
}

func cloneMusicLibrary(library assetsync.MusicLibrary) assetsync.MusicLibrary {
	library.Patterns = slices.Clone(library.Patterns)
	for index := range library.Patterns {
		library.Patterns[index] = cloneMusicPattern(library.Patterns[index])
	}
	return library
}

func (m *Model) MusicPattern(index int) (assetsync.MusicPattern, bool) {
	if m == nil || index < 0 || index >= len(m.Music.Patterns) {
		return assetsync.MusicPattern{}, false
	}
	return cloneMusicPattern(m.Music.Patterns[index]), true
}

func (m *Model) UpdateMusicPattern(index int, pattern assetsync.MusicPattern) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if index < 0 || index >= len(m.Music.Patterns) {
		return fmt.Errorf("music pattern index %d is outside 0..%d", index, len(m.Music.Patterns)-1)
	}
	candidate := cloneMusicLibrary(m.Music)
	oldID := candidate.Patterns[index].ID
	candidate.Patterns[index] = cloneMusicPattern(pattern)
	if oldID != pattern.ID {
		for candidateIndex := range candidate.Patterns {
			next := candidate.Patterns[candidateIndex].Next
			if next != nil && *next == oldID {
				updated := pattern.ID
				candidate.Patterns[candidateIndex].Next = &updated
			}
		}
	}
	if _, err := assetsync.CompileMusic(candidate, m.SFX); err != nil {
		return err
	}
	m.Music = candidate
	return nil
}

// PasteSFXIntoMusicPattern makes an independent copy of one authored clip and
// assigns it to a music cell transactionally. Stable IDs keep every existing
// music reference intact even though the copy is inserted beside its source.
func (m *Model) PasteSFXIntoMusicPattern(sourceIndex, patternIndex, channel int) (int, error) {
	if m == nil {
		return -1, fmt.Errorf("editor model is required")
	}
	if sourceIndex < 0 || sourceIndex >= len(m.SFX.SFX) {
		return -1, fmt.Errorf("SFX index %d is outside 0..%d", sourceIndex, len(m.SFX.SFX)-1)
	}
	if patternIndex < 0 || patternIndex >= len(m.Music.Patterns) {
		return -1, fmt.Errorf("music pattern index %d is outside 0..%d", patternIndex, len(m.Music.Patterns)-1)
	}
	if channel < 0 || channel >= assetsync.MusicChannels {
		return -1, fmt.Errorf("music channel %d is outside 0..%d", channel, assetsync.MusicChannels-1)
	}
	id := nextSFXID(m.SFX)
	if id < 0 {
		return -1, fmt.Errorf("SFX library already contains the maximum %d effects", assetsync.MaxSFXAssets)
	}

	candidateSFX := cloneSFXLibrary(m.SFX)
	copy := cloneSFXAsset(candidateSFX.SFX[sourceIndex])
	copy.ID = id
	copy.Name = nextSFXCopyName(candidateSFX, copy.Name)
	newIndex := sourceIndex + 1
	candidateSFX.SFX = slices.Insert(candidateSFX.SFX, newIndex, copy)

	candidateMusic := cloneMusicLibrary(m.Music)
	pattern := &candidateMusic.Patterns[patternIndex]
	empty := true
	for _, assigned := range pattern.Channels {
		if assigned >= 0 {
			empty = false
			break
		}
	}
	pattern.Channels[channel] = id
	if empty {
		pattern.DurationUnits = nil
	}
	if _, _, err := assetsync.CompileSFX(candidateSFX); err != nil {
		return -1, err
	}
	if _, err := assetsync.CompileMusic(candidateMusic, candidateSFX); err != nil {
		return -1, err
	}
	m.SFX, m.Music = candidateSFX, candidateMusic
	return newIndex, nil
}

// UpdateMusicName stores the human-facing song title without changing the
// stable music IDs used by game code. An empty title restores the editor's
// project-name fallback.
func (m *Model) UpdateMusicName(name string) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	candidate := cloneMusicLibrary(m.Music)
	candidate.Name = name
	if _, err := assetsync.CompileMusic(candidate, m.SFX); err != nil {
		return err
	}
	m.Music = candidate
	return nil
}

// SetMusicSpeed changes every SFX referenced by the song as one validated
// operation. This gives the Music Lab one beginner-friendly tempo control
// while keeping the cartridge format and runtime timing unchanged.
func (m *Model) SetMusicSpeed(speed int) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if speed < 1 || speed > 255 {
		return fmt.Errorf("music speed %d is outside 1..255", speed)
	}
	used := make(map[int]bool)
	for _, pattern := range m.Music.Patterns {
		for _, id := range pattern.Channels {
			if id >= 0 {
				used[id] = true
			}
		}
	}
	candidate := cloneSFXLibrary(m.SFX)
	for index := range candidate.SFX {
		if used[candidate.SFX[index].ID] {
			candidate.SFX[index].Speed = speed
		}
	}
	if _, _, err := assetsync.CompileSFX(candidate); err != nil {
		return err
	}
	if _, err := assetsync.CompileMusic(m.Music, candidate); err != nil {
		return err
	}
	m.SFX = candidate
	return nil
}

func nextMusicID(library assetsync.MusicLibrary) int {
	var used uint64
	for _, pattern := range library.Patterns {
		if pattern.ID >= 0 && pattern.ID < assetsync.MaxMusicPatterns {
			used |= uint64(1) << pattern.ID
		}
	}
	for id := 0; id < assetsync.MaxMusicPatterns; id++ {
		if used&(uint64(1)<<id) == 0 {
			return id
		}
	}
	return -1
}

func nextMusicName(library assetsync.MusicLibrary, id int) string {
	used := make(map[string]struct{}, len(library.Patterns))
	for _, pattern := range library.Patterns {
		used[pattern.Name] = struct{}{}
	}
	base := fmt.Sprintf("section-%02d", id+1)
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

func nextMusicCopyName(library assetsync.MusicLibrary, source string) string {
	used := make(map[string]struct{}, len(library.Patterns))
	for _, pattern := range library.Patterns {
		used[pattern.Name] = struct{}{}
	}
	for suffix := 2; ; suffix++ {
		candidate := copyNameCandidate(source, suffix)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

// AddMusicPattern appends one self-looping pattern assigned to the first SFX.
// A pattern cannot be meaningfully created until at least one SFX exists.
func (m *Model) AddMusicPattern() (int, error) {
	if m == nil {
		return -1, fmt.Errorf("editor model is required")
	}
	if len(m.SFX.SFX) == 0 {
		return -1, fmt.Errorf("add an SFX before adding a music pattern")
	}
	id := nextMusicID(m.Music)
	if id < 0 {
		return -1, fmt.Errorf("music library already contains the maximum %d patterns", assetsync.MaxMusicPatterns)
	}
	next := id
	pattern := assetsync.MusicPattern{
		ID: id, Name: nextMusicName(m.Music, id),
		Channels: []int{m.SFX.SFX[0].ID, -1, -1, -1}, Next: &next,
	}
	candidate := cloneMusicLibrary(m.Music)
	candidate.Patterns = append(candidate.Patterns, pattern)
	if _, err := assetsync.CompileMusic(candidate, m.SFX); err != nil {
		return -1, err
	}
	m.Music = candidate
	return len(candidate.Patterns) - 1, nil
}

// InsertMusicPatternAfter adds a playable row to the visible arrangement.
// The selected row flows into the new row, and the new row inherits the old
// transition. With no existing rows it creates a one-shot song that stops.
func (m *Model) InsertMusicPatternAfter(index int) (int, error) {
	return m.insertMusicPatternAfter(index, false)
}

// DuplicateMusicPattern copies all four voices and inserts the copy after the
// source while preserving the rest of the song flow.
func (m *Model) DuplicateMusicPattern(index int) (int, error) {
	return m.insertMusicPatternAfter(index, true)
}

func (m *Model) insertMusicPatternAfter(index int, duplicate bool) (int, error) {
	if m == nil {
		return -1, fmt.Errorf("editor model is required")
	}
	if len(m.SFX.SFX) == 0 {
		return -1, fmt.Errorf("create a sound before adding music")
	}
	if len(m.Music.Patterns) > 0 && (index < 0 || index >= len(m.Music.Patterns)) {
		return -1, fmt.Errorf("music pattern index %d is outside 0..%d", index, len(m.Music.Patterns)-1)
	}
	id := nextMusicID(m.Music)
	if id < 0 {
		return -1, fmt.Errorf("music library already contains the maximum %d patterns", assetsync.MaxMusicPatterns)
	}

	stop := assetsync.MusicStopPattern
	pattern := assetsync.MusicPattern{
		ID: id, Name: nextMusicName(m.Music, id),
		Channels: []int{m.SFX.SFX[0].ID, -1, -1, -1}, Next: &stop,
	}
	insertAt := 0
	candidate := cloneMusicLibrary(m.Music)
	if len(candidate.Patterns) > 0 {
		source := candidate.Patterns[index]
		insertAt = index + 1
		if duplicate {
			pattern = cloneMusicPattern(source)
			pattern.ID = id
			pattern.Name = nextMusicCopyName(candidate, source.Name)
		}
		if source.Next != nil {
			next := *source.Next
			if next == source.ID {
				next = id
			}
			pattern.Next = &next
		}
		candidate.Patterns[index].Next = &id
	}
	candidate.Patterns = slices.Insert(candidate.Patterns, insertAt, pattern)
	if _, err := assetsync.CompileMusic(candidate, m.SFX); err != nil {
		return -1, err
	}
	m.Music = candidate
	return insertAt, nil
}

// DeleteMusicPattern removes one node and turns incoming transitions into
// stops. This keeps the remaining arrangement valid and makes the severed
// signal cable explicit in the editor.
func (m *Model) DeleteMusicPattern(index int) error {
	if m == nil {
		return fmt.Errorf("editor model is required")
	}
	if index < 0 || index >= len(m.Music.Patterns) {
		return fmt.Errorf("music pattern index %d is outside 0..%d", index, len(m.Music.Patterns)-1)
	}
	candidate := cloneMusicLibrary(m.Music)
	removedID := candidate.Patterns[index].ID
	candidate.Patterns = slices.Delete(candidate.Patterns, index, index+1)
	for candidateIndex := range candidate.Patterns {
		next := candidate.Patterns[candidateIndex].Next
		if next != nil && *next == removedID {
			stop := assetsync.MusicStopPattern
			candidate.Patterns[candidateIndex].Next = &stop
		}
	}
	if _, err := assetsync.CompileMusic(candidate, m.SFX); err != nil {
		return err
	}
	m.Music = candidate
	return nil
}

func retargetMusicSFX(library assetsync.MusicLibrary, oldID, newID int) assetsync.MusicLibrary {
	candidate := cloneMusicLibrary(library)
	for patternIndex := range candidate.Patterns {
		for channel := range candidate.Patterns[patternIndex].Channels {
			if candidate.Patterns[patternIndex].Channels[channel] == oldID {
				candidate.Patterns[patternIndex].Channels[channel] = newID
			}
		}
	}
	return candidate
}
