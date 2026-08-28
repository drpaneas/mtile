//go:build tinygo && !standard

package main

import "github.com/drpaneas/mtile/runtime/audio"

const (
	profileMixerVoices  = 6
	profileSampleVoices = 1
	profileLabel        = "2 SYNTH + 1 LOOPING PCM"
)

var patterns = [...]audio.MusicPattern{{
	ID: 0, Channels: [audio.SFXChannels]int8{0, 0, -1, -1}, Next: 0, DurationSamples: 4096,
}}

//go:section .rodata
var pcm = [...]int8{
	-64, -56, -48, -40, -32, -24, -16, -8,
	0, 8, 16, 24, 32, 40, 48, 56,
	64, 56, 48, 40, 32, 24, 16, 8,
	0, -8, -16, -24, -32, -40, -48, -56,
}

//go:section .rodata
var samples = [...]audio.SampleDefinition{{
	DataLength: uint32(len(pcm)), LoopEnd: uint32(len(pcm)), Volume: audio.MaxVolume / 4,
	ID: 0, Priority: 128, Loop: true,
}}

func initAudioProfile() bool {
	return player.InitMusic(patterns[:]) && player.InitSamples(samples[:], pcm[:])
}

func startAudioProfile() bool {
	return player.Music(0, fadeMS) && player.Sample(0, -1, -1)
}
