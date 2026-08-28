//go:build tinygo

// Command audiostereobench measures the provisional left/right mixer in an
// isolated ROM. Keeping it separate preserves the established mono/SFX
// benchmark's code layout and command-rewind baseline.
package main

import (
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/volatile"
)

const (
	stereoBlockSamples = 256
	stereoBlockWords   = stereoBlockSamples / 4

	stereoStatusMeasuring = 0xA0B50001
	stereoStatusDone      = 0xA0B50002
	stereoStatusFailed    = 0xDEADB500
)

type stereoMode uint8

const (
	modeMono stereoMode = iota
	modeCentered
	modeDistributed
	modeDistributedFade
	modeSampleRight
)

type stereoBenchCase struct {
	synthVoices uint8
	pcmVoices   uint8
	mode        stereoMode
}

var stereoBenchCases = [...]stereoBenchCase{
	{synthVoices: 4, mode: modeMono},
	{synthVoices: 4, mode: modeCentered},
	{synthVoices: 4, mode: modeDistributed},
	{synthVoices: 2, mode: modeDistributedFade},
	{synthVoices: 4, mode: modeDistributedFade},
	{synthVoices: 8, mode: modeDistributed},
	{synthVoices: 12, mode: modeDistributed},
	{synthVoices: 4, pcmVoices: 1, mode: modeSampleRight},
}

//go:section .iwram
var stereoMixer audio.Mixer

//go:section .ewram
var stereoLeft [stereoBlockWords]uint32

//go:section .ewram
var stereoRight [stereoBlockWords]uint32

//go:section .ewram
var stereoPCM [stereoBlockSamples]int8

//go:section .ewram
var stereoBenchStatus volatile.Register32

//go:section .ewram
var stereoBenchCycles [len(stereoBenchCases)]volatile.Register32

//go:section .ewram
var stereoBenchCount volatile.Register32

func halt() {
	for {
	}
}

func initStereoPCM() {
	for index := range stereoPCM {
		phase := index & 63
		if phase < 32 {
			stereoPCM[index] = int8(-64 + phase*4)
		} else {
			stereoPCM[index] = int8(64 - (phase-32)*4)
		}
	}
}

func stereoProfile(voices int) int {
	switch {
	case voices <= 4:
		return 4
	case voices <= 6:
		return 6
	case voices <= 8:
		return 8
	default:
		return 12
	}
}

func distributedPan(index, voices int) int16 {
	if voices <= 1 {
		return 0
	}
	return int16(-256 + index*512/(voices-1))
}

func configureStereoCase(c stereoBenchCase) bool {
	voiceCount := int(c.synthVoices) + int(c.pcmVoices)
	if !stereoMixer.Init(stereoProfile(voiceCount)) {
		return false
	}
	for index := 0; index < int(c.synthVoices); index++ {
		pan := int16(0)
		if c.mode == modeDistributed || c.mode == modeDistributedFade {
			pan = distributedPan(index, voiceCount)
		}
		if _, ok := stereoMixer.Start(audio.Request{
			Kind: audio.SourceTriangle, Step: uint32(1<<23 + index*104729), Volume: 16, Pan: pan,
			Music: c.mode == modeDistributedFade,
		}); !ok {
			return false
		}
	}
	for index := 0; index < int(c.pcmVoices); index++ {
		pan := int16(0)
		if c.mode == modeDistributed {
			pan = distributedPan(int(c.synthVoices)+index, voiceCount)
		} else if c.mode == modeSampleRight {
			pan = audio.MaxPan
		}
		if _, ok := stereoMixer.Start(audio.Request{
			Kind: audio.SourcePCM, Step: uint32(1<<16 + index*257), Volume: 16, Pan: pan,
			PCM: stereoPCM[:], Loop: true, LoopStart: 0, LoopEnd: uint32(len(stereoPCM)),
		}); !ok {
			return false
		}
	}
	if c.mode == modeDistributedFade &&
		(!stereoMixer.SetMusicVolume(audio.MaxVolume) || !stereoMixer.RampMusicVolume(0, audio.MusicSampleRate)) {
		return false
	}
	return true
}

func mixStereoCase(c stereoBenchCase) {
	if c.mode == modeMono {
		stereoMixer.MixMonoWords(stereoLeft[:])
		return
	}
	stereoMixer.MixStereoWords(stereoLeft[:], stereoRight[:])
}

func runStereoBenchmarks() bool {
	for index, c := range stereoBenchCases {
		if !configureStereoCase(c) {
			return false
		}
		mixStereoCase(c)
		hw.WaitVDraw()
		hw.StartCycleCounter()
		mixStereoCase(c)
		stereoBenchCycles[index].Set(hw.StopCycleCounter())
	}
	stereoBenchCount.Set(uint32(len(stereoBenchCases)))
	return true
}

func main() {
	initStereoPCM()
	hw.SetReg16(hw.DISPCNT, 0x0404)
	stereoBenchStatus.Set(stereoStatusMeasuring)
	if !runStereoBenchmarks() {
		stereoBenchStatus.Set(stereoStatusFailed)
		halt()
	}
	stereoBenchStatus.Set(stereoStatusDone)
	halt()
}
