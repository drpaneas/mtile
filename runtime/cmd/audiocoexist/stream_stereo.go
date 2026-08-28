//go:build tinygo && stereo

package main

import "github.com/drpaneas/mtile/runtime/audio"

const coexistTitle = "STEREO + GRAPHICS MATRIX"
const coexistProfileText = "7 TWO-CHANNEL FADE CASES"
const coexistActiveChannels = 2
const coexistMixerVoices = audio.SFXChannels

var coexistMusicChannels = [audio.SFXChannels]int8{0, 0, -1, -1}

//go:section .ewram
var stream audio.StereoStream

func configureCoexistChannels() bool {
	pans := [...]int16{audio.MinPan, audio.MaxPan}
	for channel, pan := range pans {
		if !sfxPlayer.SetChannelPan(channel, pan) {
			return false
		}
	}
	return true
}

func configureCoexistProfile() bool { return true }

func coexistProfileStateValid(stats audio.SFXStats) bool {
	return stats.ActiveSampleVoices == 0 && stats.SampleStarts == 0 &&
		stats.FailedSampleStarts == 0 && stats.InvalidSampleCalls == 0
}

// Generated audio loops service after draw and again after presentation DMA.
// Keep stereo refill bursts bounded and use both points; the retained mono
// matrix stays byte-for-byte on its historical two-point schedule.
func serviceAfterDraw() (uint32, uint32) { return timedService() }
