package assetsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/drpaneas/mtile/runtime/audio"
)

const (
	MusicSchemaVersion    = 1
	MaxMusicPatterns      = 64
	MusicChannels         = 4
	MusicStopPattern      = -1
	MaxMusicDurationUnits = 255 * MaxSFXSteps
)

// MusicLibrary is the versioned authored assets/audio/music.json document.
// Pattern IDs are stable music() entry points. The explicit Next link forms a
// song arrangement: backward links loop and -1 stops after the pattern.
type MusicLibrary struct {
	Version  int            `json:"version"`
	Name     string         `json:"name,omitempty"`
	Patterns []MusicPattern `json:"patterns"`
}

type MusicPattern struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Channels      []int  `json:"channels"`
	Next          *int   `json:"next"`
	DurationUnits *int   `json:"durationUnits,omitempty"`
}

// TargetMusicPattern is the compact build-time form. DurationSamples is the
// longest full assigned SFX duration at the fixed 16,384 Hz target rate.
type TargetMusicPattern struct {
	DurationSamples uint32
	Channels        [MusicChannels]int8
	ID              uint8
	Next            int8
}

func musicPath(projectDir string) string {
	return filepath.Join(projectDir, "assets", "audio", "music.json")
}

// LoadMusic treats a missing file as an empty version-1 library. Once present,
// it is decoded strictly and every pattern/SFX reference is validated.
func LoadMusic(projectDir string, sfx SFXLibrary) (*MusicLibrary, error) {
	path := musicPath(projectDir)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return &MusicLibrary{Version: MusicSchemaVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read music.json: %w", err)
	}
	defer func() { _ = file.Close() }()

	var library MusicLibrary
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&library); err != nil {
		return nil, fmt.Errorf("decode music.json: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return nil, fmt.Errorf("decode music.json: trailing data: %w", err)
	}
	if _, err := CompileMusic(library, sfx); err != nil {
		return nil, err
	}
	return &library, nil
}

func SaveMusic(projectDir string, library *MusicLibrary, sfx SFXLibrary) error {
	if library == nil {
		return errors.New("music library is required")
	}
	if _, err := CompileMusic(*library, sfx); err != nil {
		return err
	}
	return writeJSON(musicPath(projectDir), library)
}

// CompileMusic validates and deterministically packs the pattern graph. SFX
// loops do not make a music pattern infinite: its transition clock is the full
// authored duration (speed * step count) of the longest assigned effect.
func CompileMusic(library MusicLibrary, sfx SFXLibrary) ([]TargetMusicPattern, error) {
	if library.Version != MusicSchemaVersion {
		return nil, fmt.Errorf("music.json version %d is unsupported (want %d)", library.Version, MusicSchemaVersion)
	}
	if len(library.Patterns) > MaxMusicPatterns {
		return nil, fmt.Errorf("music.json has %d patterns (maximum %d)", len(library.Patterns), MaxMusicPatterns)
	}
	if library.Name != "" && !validSFXName(library.Name) {
		return nil, fmt.Errorf("music name must be 1..%d non-control characters without edge whitespace", MaxSFXNameLength)
	}
	if _, _, err := CompileSFX(sfx); err != nil {
		return nil, fmt.Errorf("music SFX library: %w", err)
	}

	sfxByID := make(map[int]SFXAsset, len(sfx.SFX))
	for _, effect := range sfx.SFX {
		if _, exists := sfxByID[effect.ID]; exists {
			return nil, fmt.Errorf("SFX id %d is duplicated", effect.ID)
		}
		sfxByID[effect.ID] = effect
	}

	authored := slices.Clone(library.Patterns)
	sort.Slice(authored, func(i, j int) bool { return authored[i].ID < authored[j].ID })
	patternIDs := make(map[int]struct{}, len(authored))
	seenNames := make(map[string]struct{}, len(authored))
	for _, pattern := range authored {
		if pattern.ID < 0 || pattern.ID >= MaxMusicPatterns {
			return nil, fmt.Errorf("music pattern id %d is outside 0..%d", pattern.ID, MaxMusicPatterns-1)
		}
		if _, exists := patternIDs[pattern.ID]; exists {
			return nil, fmt.Errorf("music pattern id %d is duplicated", pattern.ID)
		}
		patternIDs[pattern.ID] = struct{}{}
		if !validSFXName(pattern.Name) {
			return nil, fmt.Errorf("music pattern %d name must be 1..%d non-control characters without edge whitespace", pattern.ID, MaxSFXNameLength)
		}
		if _, exists := seenNames[pattern.Name]; exists {
			return nil, fmt.Errorf("music pattern name %q is duplicated", pattern.Name)
		}
		seenNames[pattern.Name] = struct{}{}
	}

	target := make([]TargetMusicPattern, 0, len(authored))
	for _, pattern := range authored {
		if len(pattern.Channels) != MusicChannels {
			return nil, fmt.Errorf("music pattern %d has %d channels (want exactly %d)", pattern.ID, len(pattern.Channels), MusicChannels)
		}
		if pattern.Next == nil {
			return nil, fmt.Errorf("music pattern %d requires an explicit next pattern or -1 to stop", pattern.ID)
		}
		if *pattern.Next != MusicStopPattern {
			if _, exists := patternIDs[*pattern.Next]; !exists {
				return nil, fmt.Errorf("music pattern %d next pattern %d does not exist", pattern.ID, *pattern.Next)
			}
		}

		compiled := TargetMusicPattern{ID: uint8(pattern.ID), Next: int8(*pattern.Next)}
		var duration uint32
		assigned := false
		for channel, sfxID := range pattern.Channels {
			compiled.Channels[channel] = int8(sfxID)
			if sfxID == -1 {
				continue
			}
			assigned = true
			effect, exists := sfxByID[sfxID]
			if !exists {
				return nil, fmt.Errorf("music pattern %d channel %d references missing SFX id %d", pattern.ID, channel, sfxID)
			}
			samples := audio.SFXSamplesForUnits(uint32(effect.Speed) * uint32(len(effect.Steps)))
			if samples > duration {
				duration = samples
			}
		}
		if pattern.DurationUnits != nil {
			if *pattern.DurationUnits < 0 || *pattern.DurationUnits > MaxMusicDurationUnits {
				return nil, fmt.Errorf("music pattern %d duration units %d are outside 0..%d", pattern.ID, *pattern.DurationUnits, MaxMusicDurationUnits)
			}
			duration = audio.SFXSamplesForUnits(uint32(*pattern.DurationUnits))
		}
		if !assigned && duration == 0 {
			return nil, fmt.Errorf("music pattern %d must assign at least one SFX channel or have a positive explicit duration", pattern.ID)
		}
		compiled.DurationSamples = duration
		target = append(target, compiled)
	}
	return target, nil
}
