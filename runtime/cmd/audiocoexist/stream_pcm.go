//go:build tinygo && pcm && !stereo

package main

import "github.com/drpaneas/mtile/runtime/audio"

// A short, seamless signed 8-bit triangle keeps the native PCM source active
// without spending ROM on an unrelated test fixture. At the production sample
// rate its 32-sample loop is a 512 Hz tone.
//
//go:section .rodata
var coexistPCM = [...]int8{
	-64, -56, -48, -40, -32, -24, -16, -8,
	0, 8, 16, 24, 32, 40, 48, 56,
	64, 56, 48, 40, 32, 24, 16, 8,
	0, -8, -16, -24, -32, -40, -48, -56,
}

//go:section .rodata
var coexistSampleDefinitions = [...]audio.SampleDefinition{{
	DataLength: uint32(len(coexistPCM)),
	LoopStart:  0,
	LoopEnd:    uint32(len(coexistPCM)),
	Volume:     audio.MaxVolume / 4,
	ID:         0,
	Priority:   128,
	Loop:       true,
}}

//go:section .ewram
var stream audio.MonoStream

func configureCoexistChannels() bool { return true }

func configureCoexistProfile() bool {
	return sfxPlayer.InitSamples(coexistSampleDefinitions[:], coexistPCM[:]) &&
		sfxPlayer.Sample(0, -1, -1)
}

func coexistProfileStateValid(stats audio.SFXStats) bool {
	return stats.ActiveSampleVoices == 1 && stats.SampleStarts == 1 &&
		stats.FailedSampleStarts == 0 && stats.InvalidSampleCalls == 0
}

// Generated sample cartridges service after draw as well as at frame start
// and after presentation DMA. Keep the matrix on that production cadence.
func serviceAfterDraw() (uint32, uint32) { return timedService() }
