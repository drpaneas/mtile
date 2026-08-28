//go:build tinygo && pcm && pcmtwo && !pcmthree && !stereo

package main

import "github.com/drpaneas/mtile/runtime/audio"

const coexistTitle = "PCM TWO + GRAPHICS"
const coexistProfileText = "2 SYNTH + 1 PCM / 6 VOICES"
const coexistActiveChannels = 2
const coexistMixerVoices = 6

var coexistMusicChannels = [audio.SFXChannels]int8{0, 0, -1, -1}
