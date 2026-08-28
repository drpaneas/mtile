package assetsync

import "github.com/drpaneas/mtile/runtime/audio"

const (
	// StepOffset (uint16) plus six uint8 fields. The current TinyGo map confirms
	// an eight-byte ROM record; Filters occupies former tail padding and adds no
	// bytes to the definition.
	CompiledSFXDefinitionBytes    = 8
	CompiledSFXStepBytes          = 8
	CompiledMusicPatternBytes     = 12
	CompiledSampleDefinitionBytes = 24
	// PSG v2 target layouts are fixed by the generated TinyGo records. Slice
	// backing arrays are counted separately below.
	CompiledNativeSoundBytes = 100
	CompiledNativeEventBytes = 12
	CompiledNativeClipBytes  = 16
	// Part tempo uses the previous tail padding; field order keeps this record
	// at 16 bytes on the TinyGo target.
	CompiledNativePartBytes = 16
	CompiledNativeSongBytes = 16
)

// CompiledAudioROMBytes reports the exact packed target-record and PCM bytes
// used by the build pipeline. The record sizes are target layout contracts,
// not host unsafe.Sizeof results.
func CompiledAudioROMBytes(sfx []TargetSFX, steps []TargetSFXStep, music []TargetMusicPattern, samples []TargetSample, pcm []int8) int {
	return len(sfx)*CompiledSFXDefinitionBytes +
		len(steps)*CompiledSFXStepBytes +
		len(music)*CompiledMusicPatternBytes +
		len(samples)*CompiledSampleDefinitionBytes +
		len(pcm)
}

func CompiledNativeAudioROMBytes(library audio.Library) int {
	events, sequence := 0, 0
	for _, clip := range library.Clips {
		events += len(clip.NoteEvents)
	}
	for _, song := range library.Songs {
		sequence += len(song.Sequence)
	}
	return len(library.Sounds)*CompiledNativeSoundBytes +
		events*CompiledNativeEventBytes + len(library.Clips)*CompiledNativeClipBytes +
		len(library.Parts)*CompiledNativePartBytes + len(library.Songs)*CompiledNativeSongBytes + sequence
}
