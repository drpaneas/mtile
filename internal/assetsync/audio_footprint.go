package assetsync

import "github.com/drpaneas/mtile/runtime/audio"

const (
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
