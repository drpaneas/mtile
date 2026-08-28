//go:build tinygo && pcm && pcmthree && !pcmtwo && !stereo

package main

import "github.com/drpaneas/mtile/runtime/audio"

const coexistTitle = "PCM THREE + GRAPHICS"
const coexistProfileText = "3 SYNTH + 1 PCM / 6 VOICES"
const coexistActiveChannels = 3
const coexistMixerVoices = 6

var coexistMusicChannels = [audio.SFXChannels]int8{0, 0, 0, -1}
