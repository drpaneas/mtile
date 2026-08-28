package assetsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const (
	SFXSchemaVersion = 1
	MaxSFXAssets     = 64
	MaxSFXSteps      = 32
	MaxSFXNameLength = 64
	// picoSFXMaxVolume is the target mixer gain that reproduces one PICO-8
	// triangle-family voice at authored volume 7 in internal PICO/mGBA
	// captures. Saw and square have lower built-in gains in PICO-8. The general
	// mixer retains its 0..256 range for native PCM and headroom.
	picoSFXMaxVolume    = 167
	picoTiltedMaxVolume = 168
	picoSawMaxVolume    = 111
	picoSquareMaxVolume = 84

	SFXNoLoop uint8 = 0xff
)

// SFXLibrary is the versioned authored assets/audio/sfx.json document.
// Numeric IDs are stable API identifiers; slice order is editor-only.
type SFXLibrary struct {
	Version int        `json:"version"`
	SFX     []SFXAsset `json:"sfx"`
}

type SFXAsset struct {
	ID      int         `json:"id"`
	Name    string      `json:"name"`
	Speed   int         `json:"speed"`
	Loop    *SFXLoop    `json:"loop,omitempty"`
	Filters *SFXFilters `json:"filters,omitempty"`
	Steps   []SFXStep   `json:"steps"`
}

// SFXFilters mirrors PICO-8's per-SFX filter controls. The three level fields
// use 0=off, 1=first setting, and 2=second setting. A pointer keeps the field
// absent in existing projects and avoids rewriting every legacy sfx.json.
type SFXFilters struct {
	Noiz   bool `json:"noiz,omitempty"`
	Buzz   bool `json:"buzz,omitempty"`
	Detune int  `json:"detune,omitempty"`
	Reverb int  `json:"reverb,omitempty"`
	Dampen int  `json:"dampen,omitempty"`
}

// SFXLoop uses a half-open authored step range [start,end).
type SFXLoop struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type SFXStep struct {
	Pitch    int    `json:"pitch"`
	Waveform string `json:"waveform"`
	Volume   int    `json:"volume"`
	Effect   string `json:"effect"`
}

// TargetSFX and TargetSFXStep are compact build-time records. Generated Go
// converts Waveform to audio.SourceKind while keeping this host package
// independent from the nested runtime module.
type TargetSFX struct {
	ID         uint8
	Speed      uint8
	StepOffset uint16
	StepCount  uint8
	LoopStart  uint8
	LoopEnd    uint8
	Filters    uint8
}

type TargetSFXStep struct {
	PhaseStep uint32
	Volume    uint16
	Waveform  string
	Effect    string
}

func sfxPath(projectDir string) string {
	return filepath.Join(projectDir, "assets", "audio", "sfx.json")
}

// LoadSFX treats a missing file as an empty version-1 library so existing
// projects remain buildable. Once present, the document is decoded strictly:
// unknown fields and trailing JSON are rejected instead of silently lost.
func LoadSFX(projectDir string) (*SFXLibrary, error) {
	path := sfxPath(projectDir)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return &SFXLibrary{Version: SFXSchemaVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sfx.json: %w", err)
	}
	defer func() { _ = file.Close() }()

	var library SFXLibrary
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&library); err != nil {
		return nil, fmt.Errorf("decode sfx.json: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return nil, fmt.Errorf("decode sfx.json: trailing data: %w", err)
	}
	if _, _, err := CompileSFX(library); err != nil {
		return nil, err
	}
	return &library, nil
}

func SaveSFX(projectDir string, library *SFXLibrary) error {
	if library == nil {
		return errors.New("SFX library is required")
	}
	if _, _, err := CompileSFX(*library); err != nil {
		return err
	}
	return writeJSON(sfxPath(projectDir), library)
}

func validSFXName(name string) bool {
	if name == "" || len(name) > MaxSFXNameLength || strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validSFXWaveform(waveform string) bool {
	switch waveform {
	case "triangle", "tilted-saw", "saw", "square", "pulse", "organ", "noise", "phaser":
		return true
	default:
		_, ok := customInstrumentID(waveform)
		return ok
	}
}

func customInstrumentID(waveform string) (int, bool) {
	if len(waveform) != len("custom-0") || !strings.HasPrefix(waveform, "custom-") || waveform[7] < '0' || waveform[7] > '7' {
		return 0, false
	}
	return int(waveform[7] - '0'), true
}

func targetWaveformGoName(waveform string) string {
	switch waveform {
	case "triangle":
		return "audio.SourceTriangle"
	case "tilted-saw":
		return "audio.SourceTiltedSaw"
	case "saw":
		return "audio.SourceSaw"
	case "square":
		return "audio.SourceSquare"
	case "pulse":
		return "audio.SourcePulse"
	case "organ":
		return "audio.SourceOrgan"
	case "noise":
		return "audio.SourceNoise"
	case "phaser":
		return "audio.SourcePhaser"
	default:
		if instrument, ok := customInstrumentID(waveform); ok {
			return fmt.Sprintf("audio.SourceCustom%d", instrument)
		}
		return ""
	}
}

func validSFXEffect(effect string) bool {
	switch effect {
	case "none", "slide", "vibrato", "drop", "fade-in", "fade-out", "arpeggio-fast", "arpeggio-slow":
		return true
	default:
		return false
	}
}

func targetEffectGoName(effect string) string {
	switch effect {
	case "none":
		return "audio.SFXEffectNone"
	case "slide":
		return "audio.SFXEffectSlide"
	case "vibrato":
		return "audio.SFXEffectVibrato"
	case "drop":
		return "audio.SFXEffectDrop"
	case "fade-in":
		return "audio.SFXEffectFadeIn"
	case "fade-out":
		return "audio.SFXEffectFadeOut"
	case "arpeggio-fast":
		return "audio.SFXEffectArpeggioFast"
	case "arpeggio-slow":
		return "audio.SFXEffectArpeggioSlow"
	default:
		return ""
	}
}

func targetPhaseStep(pitch int) uint32 {
	// PICO's numeric pitch 33 is concert A at 440 Hz (its displayed octave
	// labels are two below scientific pitch notation). The target stream is
	// exactly 16,384 Hz, so compile the full-cycle increment once on the host.
	frequency := 440.0 * math.Pow(2, float64(pitch-33)/12)
	return uint32(math.Round(frequency * (1 << 32) / 16384.0))
}

func targetVolume(volume int, waveform string) uint16 {
	if _, custom := customInstrumentID(waveform); custom {
		// Custom-note volume is a 0..7 multiplier applied to the calibrated
		// built-in voice produced by SFX 0..7, not an absolute mixer gain.
		return uint16(volume)
	}
	// Round authored 0..7 onto each built-in instrument's empirically
	// calibrated PICO voice range. Core mixer oscillators remain normalized for
	// non-PICO callers; this is an authored-asset compatibility property.
	maximum := picoSFXMaxVolume
	switch waveform {
	case "tilted-saw":
		maximum = picoTiltedMaxVolume
	case "saw":
		maximum = picoSawMaxVolume
	case "square", "pulse":
		maximum = picoSquareMaxVolume
	}
	return uint16((volume*maximum + 3) / 7)
}

func targetSFXFilters(filters *SFXFilters) (uint8, error) {
	if filters == nil {
		return 0, nil
	}
	if filters.Detune < 0 || filters.Detune > 2 {
		return 0, fmt.Errorf("detune %d is outside 0..2", filters.Detune)
	}
	if filters.Reverb < 0 || filters.Reverb > 2 {
		return 0, fmt.Errorf("reverb %d is outside 0..2", filters.Reverb)
	}
	if filters.Dampen < 0 || filters.Dampen > 2 {
		return 0, fmt.Errorf("dampen %d is outside 0..2", filters.Dampen)
	}
	// This is PICO-8's native mixed-radix SFX header encoding with the editor
	// mode bit (bit 0) deliberately clear.
	encoded := filters.Detune*8 + filters.Reverb*24 + filters.Dampen*72
	if filters.Noiz {
		encoded += 2
	}
	if filters.Buzz {
		encoded += 4
	}
	return uint8(encoded), nil
}

// CompileSFX validates and deterministically packs one authored library.
// Definitions are sorted by stable ID; editor list order never changes ROM
// lookup identity.
func CompileSFX(library SFXLibrary) ([]TargetSFX, []TargetSFXStep, error) {
	if library.Version != SFXSchemaVersion {
		return nil, nil, fmt.Errorf("sfx.json version %d is unsupported (want %d)", library.Version, SFXSchemaVersion)
	}
	if len(library.SFX) > MaxSFXAssets {
		return nil, nil, fmt.Errorf("sfx.json has %d effects (maximum %d)", len(library.SFX), MaxSFXAssets)
	}

	authored := slices.Clone(library.SFX)
	sort.Slice(authored, func(i, j int) bool { return authored[i].ID < authored[j].ID })
	available := make(map[int]struct{}, len(authored))
	for _, effect := range authored {
		available[effect.ID] = struct{}{}
	}
	var customGraph [8]uint8
	for _, effect := range authored {
		for _, step := range effect.Steps {
			instrument, custom := customInstrumentID(step.Waveform)
			if !custom || step.Volume == 0 {
				continue
			}
			if _, ok := available[instrument]; !ok {
				return nil, nil, fmt.Errorf("SFX %d references missing custom instrument SFX %d", effect.ID, instrument)
			}
			if effect.ID >= 0 && effect.ID < 8 {
				customGraph[effect.ID] |= 1 << instrument
			}
		}
	}
	var customDepth func(int, uint8) (int, error)
	customDepth = func(instrument int, active uint8) (int, error) {
		bit := uint8(1 << instrument)
		if active&bit != 0 {
			return 0, fmt.Errorf("custom instrument SFX cycle includes %d", instrument)
		}
		depth := 1
		for target := 0; target < 8; target++ {
			if customGraph[instrument]&(1<<target) == 0 {
				continue
			}
			child, err := customDepth(target, active|bit)
			if err != nil {
				return 0, err
			}
			if child+1 > depth {
				depth = child + 1
			}
		}
		return depth, nil
	}
	for instrument := 0; instrument < 8; instrument++ {
		if _, ok := available[instrument]; !ok {
			continue
		}
		depth, err := customDepth(instrument, 0)
		if err != nil {
			return nil, nil, err
		}
		if depth > 2 {
			return nil, nil, fmt.Errorf("custom instrument SFX %d nesting depth %d exceeds 2", instrument, depth)
		}
	}
	definitions := make([]TargetSFX, 0, len(authored))
	steps := make([]TargetSFXStep, 0, len(authored)*MaxSFXSteps)
	var seenIDs uint64
	seenNames := make(map[string]struct{}, len(authored))
	for _, effect := range authored {
		if effect.ID < 0 || effect.ID >= MaxSFXAssets {
			return nil, nil, fmt.Errorf("SFX id %d is outside 0..%d", effect.ID, MaxSFXAssets-1)
		}
		bit := uint64(1) << effect.ID
		if seenIDs&bit != 0 {
			return nil, nil, fmt.Errorf("SFX id %d is duplicated", effect.ID)
		}
		seenIDs |= bit
		if !validSFXName(effect.Name) {
			return nil, nil, fmt.Errorf("SFX %d name must be 1..%d non-control characters without edge whitespace", effect.ID, MaxSFXNameLength)
		}
		if _, exists := seenNames[effect.Name]; exists {
			return nil, nil, fmt.Errorf("SFX name %q is duplicated", effect.Name)
		}
		seenNames[effect.Name] = struct{}{}
		if effect.Speed < 1 || effect.Speed > 255 {
			return nil, nil, fmt.Errorf("SFX %d speed %d is outside 1..255", effect.ID, effect.Speed)
		}
		if len(effect.Steps) < 1 || len(effect.Steps) > MaxSFXSteps {
			return nil, nil, fmt.Errorf("SFX %d has %d steps (want 1..%d)", effect.ID, len(effect.Steps), MaxSFXSteps)
		}
		filters, err := targetSFXFilters(effect.Filters)
		if err != nil {
			return nil, nil, fmt.Errorf("SFX %d filters: %w", effect.ID, err)
		}
		loopStart, loopEnd := SFXNoLoop, SFXNoLoop
		if effect.Loop != nil {
			if effect.Loop.Start < 0 || effect.Loop.Start >= effect.Loop.End || effect.Loop.End > len(effect.Steps) {
				return nil, nil, fmt.Errorf("SFX %d loop [%d,%d) is outside %d steps", effect.ID, effect.Loop.Start, effect.Loop.End, len(effect.Steps))
			}
			loopStart, loopEnd = uint8(effect.Loop.Start), uint8(effect.Loop.End)
		}
		if len(steps) > math.MaxUint16 {
			return nil, nil, errors.New("compiled SFX step offset exceeds uint16")
		}
		definitions = append(definitions, TargetSFX{
			ID: uint8(effect.ID), Speed: uint8(effect.Speed), StepOffset: uint16(len(steps)),
			StepCount: uint8(len(effect.Steps)), LoopStart: loopStart, LoopEnd: loopEnd, Filters: filters,
		})
		for index, step := range effect.Steps {
			if step.Pitch < 0 || step.Pitch > 63 {
				return nil, nil, fmt.Errorf("SFX %d step %d pitch %d is outside 0..63", effect.ID, index, step.Pitch)
			}
			if !validSFXWaveform(step.Waveform) {
				return nil, nil, fmt.Errorf("SFX %d step %d waveform %q is unsupported", effect.ID, index, step.Waveform)
			}
			if step.Volume < 0 || step.Volume > 7 {
				return nil, nil, fmt.Errorf("SFX %d step %d volume %d is outside 0..7", effect.ID, index, step.Volume)
			}
			if !validSFXEffect(step.Effect) {
				return nil, nil, fmt.Errorf("SFX %d step %d effect %q is unsupported", effect.ID, index, step.Effect)
			}
			steps = append(steps, TargetSFXStep{
				PhaseStep: targetPhaseStep(step.Pitch), Volume: targetVolume(step.Volume, step.Waveform), Waveform: step.Waveform, Effect: step.Effect,
			})
		}
	}
	return definitions, steps, nil
}
