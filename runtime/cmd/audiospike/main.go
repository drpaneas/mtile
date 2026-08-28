//go:build tinygo

package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/volatile"
	_ "unsafe" // required for //go:section
)

const (
	probeTone uint32 = iota + 1
	probeSilence
	probeStream
	probeStopped
	probeFailed
)

const (
	statusTone    = 0xA0D00001
	statusSilence = 0xA0D00002
	statusStopped = 0xA0D00003
	statusStream  = 0xA0D00004
	statusFailed  = 0xDEAD0001
)

const (
	streamPeriodSamples = 4096
	streamBufferSamples = streamPeriodSamples * 2
	streamBufferWords   = streamBufferSamples / 4
	streamLateThreshold = streamBufferSamples - 32
)

// DMA1 cannot read cartridge ROM, so both fixed source words live in IWRAM.
// FIFO_A consumes the least-significant byte first: +64,+64,-64,-64 is a
// 4.096 kHz square wave at the probe's exact 16.384 kHz sample clock.
//
//go:section .iwram
var toneWord uint32 = 0xC0C04040

//go:section .iwram
var silenceWord uint32

// The second half duplicates the first half. At 16.384 kHz the 30 Hz loop
// normally notices the 4,096-sample restart point within about 547 samples;
// the duplicate guard keeps DMA in bounds until software resets its source.
//
//go:section .ewram
var streamWords [streamBufferWords]uint32

// Match the measured production candidate: hot voice state stays on IWRAM's
// 32-bit bus while the DMA stream buffer remains in EWRAM.
//
//go:section .iwram
var probeMixer audio.Mixer

//go:section .ewram
var ctx rt.Context

// probeStatus and probeMode are debugger-visible hardware-test telemetry.
//
//go:section .ewram
var probeStatus volatile.Register32

// streamSamples, streamRestarts, and streamLateRestarts are debugger-visible
// scheduling telemetry. A late restart means DMA came within 32 samples of
// exhausting both the primary stream and its duplicate guard.
//
//go:section .ewram
var streamSamples volatile.Register32

//go:section .ewram
var streamRestarts volatile.Register32

//go:section .ewram
var streamLateRestarts volatile.Register32

//go:section .ewram
var probeMode uint32

var restartCount uint32
var lateRestartCount uint32

func fillStream() bool {
	if !probeMixer.Init(4) {
		return false
	}
	if _, ok := probeMixer.Start(audio.Request{
		Kind: audio.SourceTriangle, Step: 1 << 25, Volume: 128,
	}); !ok {
		return false
	}
	probeMixer.MixMonoWords(streamWords[:])
	return true
}

func start(mode uint32) {
	clock, _ := hw.DirectSoundClockForRate(hw.DirectSoundRate16384)
	ok := false
	switch mode {
	case probeTone:
		ok = hw.StartFIFOAProbe(&toneWord, clock)
	case probeSilence:
		ok = hw.StartFIFOAProbe(&silenceWord, clock)
	case probeStream:
		ok = hw.StartFIFOAStreamProbe(streamWords[:], clock)
	default:
		mode = probeTone
		ok = hw.StartFIFOAProbe(&toneWord, clock)
	}
	if !ok {
		probeMode = probeFailed
		probeStatus.Set(statusFailed)
		return
	}
	probeMode = mode
	switch mode {
	case probeSilence:
		probeStatus.Set(statusSilence)
	case probeStream:
		probeStatus.Set(statusStream)
	default:
		probeStatus.Set(statusTone)
	}
}

func serviceStream() {
	if probeMode != probeStream {
		return
	}
	samples := hw.FIFOAProbeSampleCount()
	streamSamples.Set(uint32(samples))
	if samples < streamPeriodSamples {
		return
	}
	restartCount++
	streamRestarts.Set(restartCount)
	if samples >= streamLateThreshold {
		lateRestartCount++
		streamLateRestarts.Set(lateRestartCount)
	}
	start(probeStream)
}

func stop() {
	hw.StopFIFOAProbe()
	probeMode = probeStopped
	probeStatus.Set(statusStopped)
}

func draw() {
	ctx.Cls(0)
	ctx.Print("MTILE FIFO A PROBE", 12, 16, 15)
	ctx.Print("TIMER0 16384 HZ  DMA1", 12, 32, 15)
	ctx.Print("A: FIXED  B: SILENCE  L: STREAM", 12, 48, 18)
	ctx.Print("START: RESTART", 12, 72, 18)
	ctx.Print("SELECT: STOP", 12, 88, 18)
	switch probeMode {
	case probeTone:
		ctx.Print("STATE: TONE 4096 HZ", 12, 120, 19)
	case probeSilence:
		ctx.Print("STATE: DMA SILENCE", 12, 120, 19)
	case probeStream:
		ctx.Print("STATE: STREAM 128 HZ", 12, 120, 19)
	case probeStopped:
		ctx.Print("STATE: STOPPED", 12, 120, 21)
	default:
		ctx.Print("STATE: INIT FAILED", 12, 120, 20)
	}
}

func main() {
	ctx.InitHardware()
	if !fillStream() {
		probeMode = probeFailed
		probeStatus.Set(statusFailed)
		for {
			draw()
			ctx.PrepareHiddenPage()
			ctx.WaitForPresentWindow()
			ctx.FlipPage()
			ctx.WaitVDraw()
		}
	}
	start(probeStream)

	for {
		ctx.BeginFramePace()
		ctx.PollInput()
		ctx.AdvanceTick()
		switch {
		case ctx.Btnp(rt.ButtonA):
			start(probeTone)
		case ctx.Btnp(rt.ButtonB):
			start(probeSilence)
		case ctx.Btnp(rt.ButtonL):
			start(probeStream)
		case ctx.Btnp(rt.ButtonSelect):
			stop()
		case ctx.Btnp(rt.ButtonStart):
			start(probeMode)
		}
		serviceStream()

		draw()
		ctx.PrepareHiddenPage()
		ctx.PaceFrame30()
		ctx.WaitForPresentWindow()
		ctx.ApplyDisplayPalette()
		ctx.FlipPage()
		ctx.EndFramePace30()
		ctx.WaitVDraw()
	}
}
