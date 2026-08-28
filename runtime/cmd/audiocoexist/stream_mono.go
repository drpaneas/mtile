//go:build tinygo && !stereo && !pcm

package main

import "github.com/drpaneas/mtile/runtime/audio"

const coexistTitle = "MUSIC + GRAPHICS MATRIX"
const coexistProfileText = "7 FOUR-CHANNEL FADE CASES"
const coexistActiveChannels = audio.SFXChannels
const coexistMixerVoices = audio.SFXChannels

var coexistMusicChannels = [audio.SFXChannels]int8{0, 0, 0, 0}

//go:section .ewram
var stream audio.MonoStream

func configureCoexistChannels() bool { return true }

func configureCoexistProfile() bool { return true }

func coexistProfileStateValid(stats audio.SFXStats) bool {
	return stats.ActiveSampleVoices == 0 && stats.SampleStarts == 0 &&
		stats.FailedSampleStarts == 0 && stats.InvalidSampleCalls == 0
}

// Generated cartridges service after draw as well as at frame start and after
// presentation DMA. Keep the matrix on that production refill cadence.
func serviceAfterDraw() (uint32, uint32) { return timedService() }
