//go:build tinygo && pcm && !pcmtwo && !pcmthree && !stereo

package main

import "github.com/drpaneas/mtile/runtime/audio"

const coexistTitle = "PCM + GRAPHICS MATRIX"
const coexistProfileText = "4 SYNTH + 1 PCM / 6 VOICES"
const coexistActiveChannels = audio.SFXChannels
const coexistMixerVoices = 6

var coexistMusicChannels = [audio.SFXChannels]int8{0, 0, 0, 0}
