//go:build tinygo && standard

package main

import "github.com/drpaneas/mtile/runtime/audio"

const (
	profileMixerVoices  = 4
	profileSampleVoices = 0
	profileLabel        = "4 SYNTH VOICES"
)

var patterns = [...]audio.MusicPattern{{
	ID: 0, Channels: [audio.SFXChannels]int8{0, 0, 0, 0}, Next: 0, DurationSamples: 4096,
}}

func initAudioProfile() bool {
	return player.InitMusic(patterns[:])
}

func startAudioProfile() bool {
	return player.Music(0, fadeMS)
}
