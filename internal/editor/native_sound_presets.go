package editor

import (
	"errors"
	"reflect"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/runtime/audio"
)

// NativeSoundPreset is a factory sound for one physical Game Boy channel.
// Parameter values are adapted from GB Studio's MIT-licensed template.uge;
// see docs/third-party/gb-studio-presets.md.
type NativeSoundPreset struct {
	Name  string
	Sound assetsync.NativeSoundAsset
}

func nativePresetAbs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func nativePresetEnvelope(initial, change int) assetsync.NativeEnvelopeAsset {
	direction := "down"
	if change > 0 {
		direction = "up"
	}
	pace := 0
	if change != 0 {
		pace = 8 - nativePresetAbs(change)
	}
	return assetsync.NativeEnvelopeAsset{Initial: initial, Direction: direction, Pace: pace}
}

func nativePresetSweep(pace, shift int) *assetsync.NativeSweepAsset {
	direction := "up"
	if shift <= 0 {
		direction = "down"
	}
	return &assetsync.NativeSweepAsset{Pace: pace, Direction: direction, Shift: nativePresetAbs(shift)}
}

func nativePulsePreset(channel int, name string, duty float64, length int, useLength bool, initial, envelopeChange, sweepPace, sweepShift int) NativeSoundPreset {
	pulse := &assetsync.NativePulseAsset{
		Duty: duty, Length: length, UseLength: useLength,
		Envelope: nativePresetEnvelope(initial, envelopeChange),
	}
	if channel == 0 {
		pulse.Sweep = nativePresetSweep(sweepPace, sweepShift)
	}
	sound := assetsync.NativeSoundAsset{Name: name, Kind: nativeKindName(channel), Route: "both", Pulse: pulse}
	return NativeSoundPreset{Name: name, Sound: sound}
}

func nativePresetLoopStart(value int) *int { return &value }

func nativePeriodMacro(deltas []int, loopStart int) *assetsync.NativeMacroAsset {
	steps := make([]assetsync.NativeMacroStepAsset, len(deltas))
	for index, delta := range deltas {
		steps[index].PeriodDelta = delta
	}
	return &assetsync.NativeMacroAsset{Steps: steps, LoopStart: nativePresetLoopStart(loopStart)}
}

func nativeNoiseMacro(tableNotes []int, loopStart int) *assetsync.NativeMacroAsset {
	steps := make([]assetsync.NativeMacroStepAsset, len(tableNotes))
	for index, note := range tableNotes {
		steps[index] = assetsync.NativeMacroStepAsset{
			NoiseNoteOffset: note - 36, HasNoiseNote: true,
		}
	}
	return &assetsync.NativeMacroAsset{Steps: steps, LoopStart: nativePresetLoopStart(loopStart)}
}

func nativeWavePreset(name string, samples []int, length int, useLength bool) NativeSoundPreset {
	sound := assetsync.NativeSoundAsset{
		Name: name, Kind: "ch3", Route: "both",
		Wave: &assetsync.NativeWaveAsset{Samples: append([]int(nil), samples...), Level: 100, Length: length, UseLength: useLength},
	}
	return NativeSoundPreset{Name: name, Sound: sound}
}

func nativeNoisePreset(name string, tableNotes []int, loopStart, length int, useLength bool, initial, envelopeChange int, sevenBit bool) NativeSoundPreset {
	width := 15
	if sevenBit {
		width = 7
	}
	macro := nativeNoiseMacro(tableNotes, loopStart)
	// GB Studio auditions at MIDI 60. hUGEDriver triggers CH4 from that base
	// note first, then applies table row zero as a separate non-triggering NR43
	// update. Runtime playback derives this base state from the actual note.
	initialDivisor, initialShift, _ := audio.NativeGBStudioNoisePolynomial(60, 0)
	sound := assetsync.NativeSoundAsset{
		Name: name, Kind: "ch4", Route: "both",
		Noise: &assetsync.NativeNoiseAsset{
			Width: width, Divisor: int(initialDivisor), Shift: int(initialShift),
			Length: length, UseLength: useLength, Envelope: nativePresetEnvelope(initial, envelopeChange),
		},
		Macro: macro,
	}
	return NativeSoundPreset{Name: name, Sound: sound}
}

func nativeDutyPresets(channel int) []NativeSoundPreset {
	presets := []NativeSoundPreset{
		nativePulsePreset(channel, "Fade Out 25% Pulse", 25, 0, false, 11, -3, 0, 0),
		nativePulsePreset(channel, "Fade Out 50% Pulse", 50, 0, false, 11, -3, 0, 0),
		nativePulsePreset(channel, "Fade In 12.5% Pulse", 12.5, 0, true, 0, 7, 0, 0),
		nativePulsePreset(channel, "Short 12.5% Pulse", 12.5, 48, true, 11, 0, 0, 0),
		nativePulsePreset(channel, "Short 25% Pulse", 25, 48, true, 11, 0, 0, 0),
		nativePulsePreset(channel, "Short 50% Pulse", 50, 48, true, 11, 0, 0, 0),
		nativePulsePreset(channel, "12.5% Pulse", 12.5, 0, false, 11, 0, 0, 0),
		nativePulsePreset(channel, "25% Pulse", 25, 0, false, 11, 0, 0, 0),
		nativePulsePreset(channel, "50% Pulse", 50, 0, false, 11, 0, 0, 0),
		nativePulsePreset(channel, "75% Pulse Custom Vibrato", 75, 0, false, 11, 0, 0, 0),
	}
	// hUGEDriver advances the eight-row table every display frame. Its table
	// dispatcher skips the ordinary tick-zero guard in 1xx/2xx, so every row
	// changes the period and the original eight-frame cycle is preserved.
	presets[9].Sound.Macro = nativePeriodMacro([]int{
		1, 1, 1, 1, -1, -1, -1, -1,
	}, 0)
	if channel == 0 {
		presets = append(presets,
			nativePulsePreset(channel, "Bass Drum 50% Pulse", 50, 0, true, 11, -6, 1, -4),
			nativePulsePreset(channel, "Soft Sweep 50% Pulse", 50, 0, false, 11, -3, 1, -1),
			nativePulsePreset(channel, "Sweep 12.5% Pulse", 12.5, 0, false, 11, -1, 4, 7),
			nativePulsePreset(channel, "Sweep 25% Pulse", 25, 32, true, 11, -1, 1, 5),
		)
	}
	return presets
}

func nativeWavePresets() []NativeSoundPreset {
	return []NativeSoundPreset{
		nativeWavePreset("12.5% Pulse", []int{15, 15, 15, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 128, true),
		nativeWavePreset("25% Pulse", []int{15, 15, 15, 15, 15, 15, 15, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 128, true),
		nativeWavePreset("31.25% Pulse", []int{15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 128, true),
		nativeWavePreset("37.5% Pulse", []int{15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 128, true),
		nativeWavePreset("43.75% Pulse", []int{15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 128, true),
		nativeWavePreset("50% Pulse", []int{15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 128, true),
		nativeWavePreset("50% Pulse (Volume 5)", []int{12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3}, 0, false),
		nativeWavePreset("50% Pulse (Volume 3)", []int{10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5}, 0, false),
		nativeWavePreset("50% Pulse (Volume 1)", []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7}, 0, false),
		nativeWavePreset("Layered Square", []int{15, 15, 15, 15, 12, 12, 12, 12, 15, 15, 15, 15, 12, 12, 12, 12, 0, 0, 0, 0, 3, 3, 3, 3, 0, 0, 0, 0, 3, 3, 3, 3}, 0, false),
		nativeWavePreset("Triangle", []int{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, 0, false),
		nativeWavePreset("Layered Triangle", []int{15, 15, 15, 15, 11, 10, 9, 8, 10, 9, 8, 7, 3, 2, 1, 0, 3, 4, 5, 6, 4, 5, 6, 7, 11, 12, 13, 14, 12, 13, 14, 15}, 0, false),
		nativeWavePreset("Saw", []int{15, 15, 14, 14, 13, 13, 12, 12, 11, 11, 10, 10, 9, 9, 8, 8, 7, 7, 6, 6, 5, 5, 4, 4, 3, 3, 2, 2, 1, 1, 0, 0}, 0, false),
		nativeWavePreset("Distorted Saw", []int{15, 15, 15, 14, 15, 13, 15, 12, 15, 11, 15, 10, 15, 9, 15, 8, 14, 7, 13, 6, 12, 5, 11, 4, 10, 3, 9, 2, 8, 1, 7, 0}, 0, false),
	}
}

func nativeNoisePresets() []NativeSoundPreset {
	// These are the exact non-empty rows and jump targets from GB Studio's
	// template.uge. They update NR43 at the original hUGEDriver cadence.
	return []NativeSoundPreset{
		nativeNoisePreset("Closed Hi-Hat", []int{68, 68}, 1, 48, true, 9, -7, false),
		nativeNoisePreset("Closed Hi-Hat 2", []int{67, 67}, 1, 32, true, 9, -7, false),
		nativeNoisePreset("Closed Hi-Hat 3", []int{66, 65, 64, 64}, 3, 0, false, 9, -7, false),
		nativeNoisePreset("Open Hi-Hat", []int{68, 68}, 1, 0, false, 9, -6, false),
		nativeNoisePreset("Crash", []int{60, 60}, 1, 0, false, 9, -4, false),
		nativeNoisePreset("Crash 2", []int{52, 52}, 1, 0, false, 9, -4, false),
		nativeNoisePreset("Snare Drum", []int{60, 60}, 1, 0, false, 11, -7, false),
		nativeNoisePreset("Snare Drum 2", []int{60, 60}, 1, 0, false, 11, -7, true),
		nativeNoisePreset("Snare Drum 3", []int{52, 51}, 1, 0, false, 11, -7, true),
		nativeNoisePreset("Metallic Sound", []int{68, 68}, 1, 0, false, 9, -7, true),
		nativeNoisePreset("Bass Drum", []int{52, 51}, 1, 0, true, 11, -6, false),
		nativeNoisePreset("Bass Drum 2", []int{63, 50, 53, 34, 36, 36}, 5, 22, true, 11, -7, false),
		nativeNoisePreset("Explosion", []int{49, 47, 45, 43, 43, 43}, 5, 0, false, 11, -1, false),
		nativeNoisePreset("Explosion 2", []int{44, 43, 42, 41, 40, 40}, 5, 0, false, 11, -1, true),
	}
}

// NativeSoundPresets returns detached factory sounds valid for channel.
func NativeSoundPresets(channel int) []NativeSoundPreset {
	var source []NativeSoundPreset
	switch channel {
	case 0, 1:
		source = nativeDutyPresets(channel)
	case 2:
		source = nativeWavePresets()
	case 3:
		source = nativeNoisePresets()
	default:
		return nil
	}
	result := make([]NativeSoundPreset, len(source))
	for index, preset := range source {
		result[index] = NativeSoundPreset{Name: preset.Name, Sound: cloneNativeSoundAsset(preset.Sound)}
	}
	return result
}

func nativeSoundPresetEquivalent(left, right assetsync.NativeSoundAsset) bool {
	left, right = cloneNativeSoundAsset(left), cloneNativeSoundAsset(right)
	left.ID, right.ID = 0, 0
	left.Name, right.Name = "", ""
	return reflect.DeepEqual(left, right)
}

// NativeSoundPresetIndex identifies the factory preset behind a project sound.
// Names and stable IDs are intentionally ignored so renamed presets still show
// as selected in the library.
func NativeSoundPresetIndex(channel int, sound assetsync.NativeSoundAsset) int {
	for index, preset := range NativeSoundPresets(channel) {
		if nativeSoundPresetEquivalent(sound, preset.Sound) {
			return index
		}
	}
	return -1
}

// EnsureNativeSoundPreset returns a reusable project instrument for preset.
// Re-selecting an unchanged preset reuses its stable ID instead of consuming
// another ROM sound slot.
func (m *Model) EnsureNativeSoundPreset(channel, presetIndex int) (id int, created bool, err error) {
	presets := NativeSoundPresets(channel)
	if presetIndex < 0 || presetIndex >= len(presets) {
		return -1, false, errors.New("sound preset does not exist on this channel")
	}
	id = -1
	preset := presets[presetIndex]
	err = m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		for _, sound := range candidate.Sounds.Sounds {
			if nativeSoundPresetEquivalent(sound, preset.Sound) {
				id = sound.ID
				return nil
			}
		}
		used := make(map[int]bool, len(candidate.Sounds.Sounds))
		for _, sound := range candidate.Sounds.Sounds {
			used[sound.ID] = true
		}
		newID, ok := firstFreeNativeID(used, audio.NativeMaxSounds)
		if !ok {
			return errors.New("the project has no free instrument IDs")
		}
		sound := cloneNativeSoundAsset(preset.Sound)
		sound.ID = newID
		candidate.Sounds.Sounds = append(candidate.Sounds.Sounds, sound)
		id, created = newID, true
		return nil
	})
	return id, created, err
}
