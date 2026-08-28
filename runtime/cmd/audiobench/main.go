//go:build tinygo

// Command audiobench measures fixed-size mixer and SFX sequencer workloads in
// real ARM7TDMI cycles. Direct Sound stays off because TM0/TM1 are the
// benchmark counter; the audiospike command separately proves FIFO playback.
package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/volatile"
	"unsafe" // unsafe.String for zero-allocation result formatting
)

const (
	blockSamples  = 256
	blockWords    = blockSamples / 4
	cyclesPerTick = 2 * 1232 * 228

	statusMeasuring = 0xA0B00001
	statusDone      = 0xA0B00002
	statusFailed    = 0xDEADA0B0
)

type benchCase struct {
	name        string
	synthVoices uint8
	pcmVoices   uint8
	sfxChannels uint8
	effect      audio.SFXEffect
	filter      audio.SFXFilter
	custom      bool
	customMixed bool
	customStart bool
	mixedRamp   bool
	rewind      bool
	music       bool
	musicFade   bool
}

const (
	filterNoiz    audio.SFXFilter = 2
	filterBuzz    audio.SFXFilter = 4
	filterDetune2 audio.SFXFilter = 16
	filterReverb1 audio.SFXFilter = 24
	filterReverb2 audio.SFXFilter = 48
	filterDampen1 audio.SFXFilter = 72
	filterDampen2 audio.SFXFilter = 144
	filterFull    audio.SFXFilter = 214
)

var benchCases = [...]benchCase{
	{name: "SILENCE"},
	{name: "SYNTH X1", synthVoices: 1},
	{name: "SYNTH X4", synthVoices: 4},
	{name: "SYNTH X8", synthVoices: 8},
	{name: "SYNTH X12", synthVoices: 12},
	{name: "PCM X1", pcmVoices: 1},
	{name: "PCM X4", pcmVoices: 4},
	{name: "MIX 4+1", synthVoices: 4, pcmVoices: 1},
	{name: "MIX 4+4", synthVoices: 4, pcmVoices: 4},
	{name: "SFX NONE X1", sfxChannels: 1},
	{name: "SFX SLIDE X1", sfxChannels: 1, effect: audio.SFXEffectSlide},
	{name: "SFX VIB X1", sfxChannels: 1, effect: audio.SFXEffectVibrato},
	{name: "SFX DROP X1", sfxChannels: 1, effect: audio.SFXEffectDrop},
	{name: "SFX FADEIN X1", sfxChannels: 1, effect: audio.SFXEffectFadeIn},
	{name: "SFX FADEOUT X1", sfxChannels: 1, effect: audio.SFXEffectFadeOut},
	{name: "SFX ARPFAST X1", sfxChannels: 1, effect: audio.SFXEffectArpeggioFast},
	{name: "SFX ARPSLOW X1", sfxChannels: 1, effect: audio.SFXEffectArpeggioSlow},
	{name: "SFX NONE X4", sfxChannels: 4},
	{name: "SFX SLIDE X4", sfxChannels: 4, effect: audio.SFXEffectSlide},
	{name: "SFX VIB X4", sfxChannels: 4, effect: audio.SFXEffectVibrato},
	{name: "SFX DROP X4", sfxChannels: 4, effect: audio.SFXEffectDrop},
	{name: "SFX FADEIN X4", sfxChannels: 4, effect: audio.SFXEffectFadeIn},
	{name: "SFX FADEOUT X4", sfxChannels: 4, effect: audio.SFXEffectFadeOut},
	{name: "SFX ARPFAST X4", sfxChannels: 4, effect: audio.SFXEffectArpeggioFast},
	{name: "SFX ARPSLOW X4", sfxChannels: 4, effect: audio.SFXEffectArpeggioSlow},
	{name: "CUSTOM ORGAN X1", sfxChannels: 1, custom: true},
	{name: "CUSTOM ORGAN X2", sfxChannels: 2, custom: true},
	{name: "CUSTOM ORGAN X3", sfxChannels: 3, custom: true},
	{name: "CUSTOM ORGAN X4", sfxChannels: 4, custom: true},
	{name: "CUSTOM MIX X3", sfxChannels: 3, customMixed: true},
	{name: "CUSTOM MIX X4", sfxChannels: 4, customMixed: true},
	{name: "CUSTOM START X4", customStart: true},
	{name: "MIXED RAMP X4", mixedRamp: true},
	{name: "Q8 SLIDE X4", effect: audio.SFXEffectSlide, rewind: true},
	{name: "Q8 VIB X4", effect: audio.SFXEffectVibrato, rewind: true},
	{name: "Q8 DROP X4", effect: audio.SFXEffectDrop, rewind: true},
	{name: "Q8 FADEIN X4", effect: audio.SFXEffectFadeIn, rewind: true},
	{name: "Q8 FADEOUT X4", effect: audio.SFXEffectFadeOut, rewind: true},
	{name: "Q8 ARPFAST X4", effect: audio.SFXEffectArpeggioFast, rewind: true},
	{name: "Q8 ARPSLOW X4", effect: audio.SFXEffectArpeggioSlow, rewind: true},
	{name: "MUSIC X4", music: true},
	{name: "MUSIC FADE X4", musicFade: true},
	{name: "NOIZ X4", sfxChannels: 4, filter: filterNoiz},
	{name: "BUZZ X4", sfxChannels: 4, filter: filterBuzz},
	{name: "DETUNE2 X4", sfxChannels: 4, filter: filterDetune2},
	{name: "REVERB1 X1", sfxChannels: 1, filter: filterReverb1},
	{name: "REVERB1 X4", sfxChannels: 4, filter: filterReverb1},
	{name: "REVERB2 X4", sfxChannels: 4, filter: filterReverb2},
	{name: "DAMPEN1 X1", sfxChannels: 1, filter: filterDampen1},
	{name: "DAMPEN1 X4", sfxChannels: 4, filter: filterDampen1},
	{name: "DAMPEN2 X4", sfxChannels: 4, filter: filterDampen2},
	{name: "FILTER FULL X4", sfxChannels: 4, filter: filterFull},
}

//go:section .ewram
var ctx rt.Context

// The candidate production layout keeps frequently accessed voice state on
// the 32-bit IWRAM bus. PCM source data and generated blocks remain in EWRAM.
//
//go:section .iwram
var mixer audio.Mixer

//go:section .ewram
var sfxPlayer audio.SFXPlayer

//go:section .ewram
var stream audio.MonoStream

//go:section .ewram
var output [blockWords]uint32

//go:section .ewram
var pcm [blockSamples]int8

var sfxDefinitions = [...]audio.SFXDefinition{{
	ID: 0, Speed: 4, StepCount: 2, LoopStart: audio.NoSFXLoop, LoopEnd: audio.NoSFXLoop,
}}

var sfxArpeggioDefinitions = [...]audio.SFXDefinition{{
	ID: 0, Speed: 8, StepCount: 4, LoopStart: audio.NoSFXLoop, LoopEnd: audio.NoSFXLoop,
}}

// customOrganDefinitions reproduces the dense custom-instrument boundary
// cadence in the PICO-8 corpus: an outer speed-32 phrase drives a looping
// speed-2 organ envelope. Exact captured PICO pitch steps keep this benchmark
// on the table-transposition path used by generated carts.
var customOrganDefinitions = [...]audio.SFXDefinition{
	{ID: 0, Speed: 2, StepOffset: 0, StepCount: 2, LoopStart: 0, LoopEnd: 2},
	{ID: 8, Speed: 32, StepOffset: 2, StepCount: 4, LoopStart: audio.NoSFXLoop, LoopEnd: audio.NoSFXLoop},
}

var customOrganSteps = [...]audio.SFXStep{
	{PhaseStep: 68583572, Volume: 256, Kind: audio.SourceOrgan}, // pitch 24
	{PhaseStep: 68583572, Volume: 128, Kind: audio.SourceOrgan},
	{PhaseStep: 54434817, Volume: 7, Kind: audio.SourceCustom0}, // pitch 20
	{PhaseStep: 57671680, Volume: 7, Kind: audio.SourceCustom0},
	{PhaseStep: 61101017, Volume: 6, Kind: audio.SourceCustom0, Effect: audio.SFXEffectSlide},
	{PhaseStep: 64734272, Volume: 6, Kind: audio.SourceCustom0},
}

var customMixedDefinitions = [...]audio.SFXDefinition{
	{ID: 0, Speed: 2, StepOffset: 0, StepCount: 2, LoopStart: 0, LoopEnd: 2},
	{ID: 1, Speed: 2, StepOffset: 2, StepCount: 2, LoopStart: 0, LoopEnd: 2},
	{ID: 2, Speed: 2, StepOffset: 4, StepCount: 2, LoopStart: 0, LoopEnd: 2},
	{ID: 4, Speed: 16, StepOffset: 14, StepCount: 2, LoopStart: 0, LoopEnd: 2},
	{ID: 8, Speed: 32, StepOffset: 6, StepCount: 2, LoopStart: audio.NoSFXLoop, LoopEnd: audio.NoSFXLoop},
	{ID: 9, Speed: 32, StepOffset: 8, StepCount: 2, LoopStart: audio.NoSFXLoop, LoopEnd: audio.NoSFXLoop},
	{ID: 10, Speed: 32, StepOffset: 10, StepCount: 2, LoopStart: audio.NoSFXLoop, LoopEnd: audio.NoSFXLoop},
	{ID: 11, Speed: 32, StepOffset: 12, StepCount: 2, LoopStart: audio.NoSFXLoop, LoopEnd: audio.NoSFXLoop},
}

var customMixedSteps = [...]audio.SFXStep{
	{PhaseStep: 68583572, Volume: 192, Kind: audio.SourceSquare},
	{PhaseStep: 68583572, Volume: 224, Kind: audio.SourceOrgan},
	{PhaseStep: 68583572, Volume: 224, Kind: audio.SourceOrgan},
	{PhaseStep: 68583572, Volume: 192, Kind: audio.SourceTiltedSaw},
	{PhaseStep: 68583572, Volume: 224, Kind: audio.SourceTriangle},
	{PhaseStep: 68583572, Volume: 224, Kind: audio.SourceOrgan},
	{PhaseStep: 54434817, Volume: 7, Kind: audio.SourceCustom0},
	{PhaseStep: 61101017, Volume: 6, Kind: audio.SourceCustom0, Effect: audio.SFXEffectSlide},
	{PhaseStep: 57671680, Volume: 7, Kind: audio.SourceCustom1},
	{PhaseStep: 64734272, Volume: 6, Kind: audio.SourceCustom1, Effect: audio.SFXEffectSlide},
	{PhaseStep: 61101017, Volume: 7, Kind: audio.SourceCustom2},
	{PhaseStep: 68583572, Volume: 6, Kind: audio.SourceCustom2, Effect: audio.SFXEffectSlide},
	{PhaseStep: 64734272, Volume: 7, Kind: audio.SourceCustom4},
	{PhaseStep: 72661764, Volume: 6, Kind: audio.SourceCustom4, Effect: audio.SFXEffectSlide},
	{PhaseStep: 68583572, Volume: 224, Kind: audio.SourceNoise},
	{PhaseStep: 96991818, Volume: 160, Kind: audio.SourceNoise},
}

var filteredDefinition [1]audio.SFXDefinition

var musicFadePatterns = [...]audio.MusicPattern{{
	ID: 0, Channels: [audio.SFXChannels]int8{0, 0, 0, 0}, Next: 0, DurationSamples: 1024,
}}

var customStartPatterns = [...]audio.MusicPattern{
	{ID: 0, Channels: [audio.SFXChannels]int8{-1, -1, -1, -1}, Next: 1, DurationSamples: blockSamples},
	{ID: 1, Channels: [audio.SFXChannels]int8{8, 9, 10, 11}, Next: audio.NoMusicPattern, DurationSamples: 1024},
}

var sfxNoneSteps = [...]audio.SFXStep{
	{PhaseStep: 1 << 24, Volume: 32, Kind: audio.SourceTriangle},
	{PhaseStep: 1 << 25, Volume: 16, Kind: audio.SourceTriangle},
}

var sfxSlideSteps = [...]audio.SFXStep{
	{PhaseStep: 1 << 24, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectSlide},
	{PhaseStep: 1 << 25, Volume: 16, Kind: audio.SourceTriangle},
}

var sfxVibratoSteps = [...]audio.SFXStep{
	{PhaseStep: 1 << 24, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectVibrato},
	{PhaseStep: 1 << 25, Volume: 16, Kind: audio.SourceTriangle},
}

var sfxDropSteps = [...]audio.SFXStep{
	{PhaseStep: 1 << 24, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectDrop},
	{PhaseStep: 1 << 25, Volume: 16, Kind: audio.SourceTriangle},
}

var sfxFadeInSteps = [...]audio.SFXStep{
	{PhaseStep: 1 << 24, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectFadeIn},
	{PhaseStep: 1 << 25, Volume: 16, Kind: audio.SourceTriangle},
}

var sfxFadeOutSteps = [...]audio.SFXStep{
	{PhaseStep: 1 << 24, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectFadeOut},
	{PhaseStep: 1 << 25, Volume: 16, Kind: audio.SourceTriangle},
}

var sfxArpeggioFastSteps = [...]audio.SFXStep{
	{PhaseStep: 1 << 24, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectArpeggioFast},
	{PhaseStep: 1 << 25, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectArpeggioFast},
	{PhaseStep: 1 << 26, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectArpeggioFast},
	{PhaseStep: 1 << 27, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectArpeggioFast},
}

var sfxArpeggioSlowSteps = [...]audio.SFXStep{
	{PhaseStep: 1 << 24, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectArpeggioSlow},
	{PhaseStep: 1 << 25, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectArpeggioSlow},
	{PhaseStep: 1 << 26, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectArpeggioSlow},
	{PhaseStep: 1 << 27, Volume: 32, Kind: audio.SourceTriangle, Effect: audio.SFXEffectArpeggioSlow},
}

//go:section .ewram
var benchStatus volatile.Register32

//go:section .ewram
var benchCycles [len(benchCases)]volatile.Register32

//go:section .ewram
var benchCount volatile.Register32

//go:section .ewram
var benchFailure volatile.Register32

var currentBenchCase uint32

var results [len(benchCases)]uint32
var rowBuf [40]byte

func halt() {
	for {
	}
}

func utoa(buf []byte, value uint32) int {
	if value == 0 {
		buf[0] = '0'
		return 1
	}
	n := 0
	for value > 0 {
		buf[n] = byte('0' + value%10)
		value /= 10
		n++
	}
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return n
}

func printRow(y int, name string, cycles uint32) {
	n := 0
	for i := 0; i < len(name) && n < 12; i++ {
		rowBuf[n] = name[i]
		n++
	}
	for n < 13 {
		rowBuf[n] = ' '
		n++
	}
	n += utoa(rowBuf[n:], cycles)
	for n < 23 {
		rowBuf[n] = ' '
		n++
	}
	n += utoa(rowBuf[n:], cycles/blockSamples)
	rowBuf[n] = 'c'
	n++
	rowBuf[n] = '/'
	n++
	rowBuf[n] = 's'
	n++
	ctx.Print(unsafe.String(&rowBuf[0], n), 4, y, 7)
}

func initPCM() {
	for i := range pcm {
		phase := i & 63
		if phase < 32 {
			pcm[i] = int8(-64 + phase*4)
		} else {
			pcm[i] = int8(64 - (phase-32)*4)
		}
	}
}

func profileFor(voices int) int {
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

func configure(c benchCase) bool {
	if c.mixedRamp {
		if !mixer.Init(4) {
			return false
		}
		kinds := [...]audio.SourceKind{
			audio.SourceOrgan, audio.SourcePhaser, audio.SourceNoise, audio.SourceTriangle,
		}
		for index, kind := range kinds {
			id, ok := mixer.Start(audio.Request{
				Kind: kind, Step: uint32(68583572 + index*9699181), Volume: uint16(96 + index*24), Music: true,
			})
			if !ok {
				return false
			}
			voice, _ := mixer.Voice(id)
			if !mixer.SetVoiceVolumeRamp(id, voice.Started, 0, voice.Volume, 1024) {
				return false
			}
		}
		return true
	}
	if c.customStart {
		return mixer.Init(4) &&
			sfxPlayer.Init(&mixer, customMixedDefinitions[:], customMixedSteps[:]) &&
			sfxPlayer.InitMusic(customStartPatterns[:]) && sfxPlayer.Music(0, 0)
	}
	if c.music || c.musicFade {
		fadeMS := 0
		if c.musicFade {
			fadeMS = 1000
		}
		if !mixer.Init(4) || !sfxPlayer.Init(&mixer, sfxDefinitions[:], sfxNoneSteps[:]) ||
			!sfxPlayer.InitMusic(musicFadePatterns[:]) || !sfxPlayer.Music(0, fadeMS) {
			return false
		}
		return true
	}
	if c.sfxChannels != 0 {
		if !mixer.Init(4) {
			return false
		}
		if c.customMixed {
			if !sfxPlayer.Init(&mixer, customMixedDefinitions[:], customMixedSteps[:]) {
				return false
			}
			for channel := 0; channel < int(c.sfxChannels); channel++ {
				if _, ok := sfxPlayer.Play(8+channel, channel, 0, 0); !ok {
					return false
				}
			}
			// Align the recorded page with the mixed-waveform half of each
			// speed-2 custom envelope after runBenchmarks' normal warm page.
			sfxPlayer.MixMonoWords(output[:])
			return true
		}
		if c.custom {
			if !sfxPlayer.Init(&mixer, customOrganDefinitions[:], customOrganSteps[:]) {
				return false
			}
			for channel := 0; channel < int(c.sfxChannels); channel++ {
				if _, ok := sfxPlayer.Play(8, channel, 0, 0); !ok {
					return false
				}
			}
			return true
		}
		steps := stepsForEffect(c.effect)
		if !sfxPlayer.Init(&mixer, definitionsForCase(c), steps) {
			return false
		}
		for channel := 0; channel < int(c.sfxChannels); channel++ {
			if _, ok := sfxPlayer.Play(0, channel, 0, 0); !ok {
				return false
			}
		}
		// The generic warm page leaves fast arpeggio immediately before its
		// measured pitch update. Slow needs one additional untimed page so its
		// four-tick low-speed cadence is measured on the same boundary.
		if c.effect == audio.SFXEffectArpeggioSlow {
			sfxPlayer.MixMonoWords(output[:])
		}
		return true
	}
	if !mixer.Init(profileFor(int(c.synthVoices) + int(c.pcmVoices))) {
		return false
	}
	for i := 0; i < int(c.synthVoices); i++ {
		if _, ok := mixer.Start(audio.Request{
			Kind: audio.SourceTriangle, Step: uint32(1<<23 + i*104729), Volume: 16,
		}); !ok {
			return false
		}
	}
	for i := 0; i < int(c.pcmVoices); i++ {
		if _, ok := mixer.Start(audio.Request{
			Kind: audio.SourcePCM, Step: uint32(1<<16 + i*257), Volume: 16,
			PCM: pcm[:], Loop: true, LoopStart: 0, LoopEnd: uint32(len(pcm)),
		}); !ok {
			return false
		}
	}
	return true
}

func stepsForEffect(effect audio.SFXEffect) []audio.SFXStep {
	switch effect {
	case audio.SFXEffectSlide:
		return sfxSlideSteps[:]
	case audio.SFXEffectVibrato:
		return sfxVibratoSteps[:]
	case audio.SFXEffectDrop:
		return sfxDropSteps[:]
	case audio.SFXEffectFadeIn:
		return sfxFadeInSteps[:]
	case audio.SFXEffectFadeOut:
		return sfxFadeOutSteps[:]
	case audio.SFXEffectArpeggioFast:
		return sfxArpeggioFastSteps[:]
	case audio.SFXEffectArpeggioSlow:
		return sfxArpeggioSlowSteps[:]
	default:
		return sfxNoneSteps[:]
	}
}

func definitionsForEffect(effect audio.SFXEffect) []audio.SFXDefinition {
	if effect == audio.SFXEffectArpeggioFast || effect == audio.SFXEffectArpeggioSlow {
		return sfxArpeggioDefinitions[:]
	}
	return sfxDefinitions[:]
}

func definitionsForCase(c benchCase) []audio.SFXDefinition {
	definitions := definitionsForEffect(c.effect)
	if c.filter == 0 {
		return definitions
	}
	filteredDefinition[0] = definitions[0]
	filteredDefinition[0].Filters = c.filter
	return filteredDefinition[:]
}

func mixCase(c benchCase) {
	if c.sfxChannels != 0 || c.music || c.musicFade || c.customStart {
		sfxPlayer.MixMonoWords(output[:])
		return
	}
	mixer.MixMonoWords(output[:])
}

// configureRewind starts an inactive four-page SFX stream, then fills the
// complete deferred-command queue with two explicit starts per channel. The
// rewind must restore the page-after-next checkpoint, apply all eight
// commands, and generate one page ending with four effect-bearing channels.
func configureRewind(c benchCase) bool {
	stream.Stop()
	if !mixer.Init(4) || !sfxPlayer.Init(&mixer, definitionsForCase(c), stepsForEffect(c.effect)) ||
		!stream.StartSFX(&sfxPlayer, hw.DirectSoundRate16384) {
		return false
	}
	for command := 0; command < audio.SFXCommandCapacity; command++ {
		if _, ok := sfxPlayer.Play(0, command%audio.SFXChannels, 0, 0); !ok {
			stream.Stop()
			return false
		}
	}
	return true
}

func rewindValidationCode() uint32 {
	streamStats := stream.Stats()
	sfxStats := sfxPlayer.Stats()
	var code uint32
	if !streamStats.Running {
		code |= 1 << 0
	}
	if streamStats.QueuedBlocks != 2 {
		code |= 1 << 1
	}
	if sfxStats.PendingCommands != 0 {
		code |= 1 << 2
	}
	if sfxStats.Starts != audio.SFXCommandCapacity {
		code |= 1 << 3
	}
	if sfxStats.ActiveChannels != audio.SFXChannels {
		code |= 1 << 4
	}
	if sfxStats.Rewinds != 1 {
		code |= 1 << 5
	}
	if sfxStats.FallbackCommands != 0 {
		code |= 1 << 6
	}
	if sfxStats.DroppedCommands != 0 {
		code |= 1 << 7
	}
	return code
}

func recordBenchFailure(stage, detail uint32) {
	benchFailure.Set((currentBenchCase+1)<<16 | stage<<8 | detail)
}

func runRewindCase(c benchCase) (uint32, bool) {
	// Exercise the exact setup and rewind path once before the recorded run.
	if !configureRewind(c) {
		recordBenchFailure(1, 0)
		stream.Stop()
		return 0, false
	}
	stream.Service(audio.StreamPageCount)
	if code := rewindValidationCode(); code != 0 {
		recordBenchFailure(3, code)
		stream.Stop()
		return 0, false
	}
	stream.Stop()
	hw.WaitVDraw()

	if !configureRewind(c) {
		recordBenchFailure(4, 0)
		return 0, false
	}
	hw.StartFrameCycleCounter()
	stream.Service(audio.StreamPageCount)
	cycles := hw.StopFrameCycleCounter()
	code := rewindValidationCode()
	ok := code == 0
	if code != 0 {
		recordBenchFailure(6, code)
	}
	stream.Stop()
	return cycles, ok
}

func runBenchmarks() bool {
	for i, c := range benchCases {
		currentBenchCase = uint32(i)
		if c.rewind {
			cycles, ok := runRewindCase(c)
			if !ok {
				return false
			}
			results[i] = cycles
			benchCycles[i].Set(cycles)
			continue
		}
		if !configure(c) {
			return false
		}
		// Warm the source-state path before taking the one exact target
		// measurement. ARM7 has no data cache; this mainly normalizes branch
		// and ROM-prefetch entry state across cases.
		mixCase(c)
		hw.WaitVDraw()
		hw.StartCycleCounter()
		mixCase(c)
		cycles := hw.StopCycleCounter()
		results[i] = cycles
		benchCycles[i].Set(cycles)
	}
	benchCount.Set(uint32(len(benchCases)))
	return true
}

func main() {
	initPCM()
	ctx.InitHardware()
	benchStatus.Set(statusMeasuring)
	if !runBenchmarks() {
		benchStatus.Set(statusFailed)
		halt()
	}
	benchStatus.Set(statusDone)

	ctx.Cls(0)
	ctx.Print("AUDIO MIX: 256 SAMPLES", 4, 4, 7)
	ctx.Print("WORKLOAD     CYCLES    C/S", 4, 12, 7)
	// Sixteen rows fit the 160-line display; all cases remain available in
	// benchCycles for debugger and hardware extraction.
	for i, c := range benchCases {
		if i == 16 {
			break
		}
		printRow(24+i*8, c.name, results[i])
	}
	ctx.Print("30HZ TICK: 561792 CYCLES", 4, 4, 18)
	ctx.WaitForPresentWindow()
	ctx.PrepareHiddenPage()
	ctx.FlipPage()
	halt()
}

// Keep the tick budget tied into this command even though the compact screen
// table displays cycles/sample. Debuggers and future table formats use it.
var _ = cyclesPerTick
