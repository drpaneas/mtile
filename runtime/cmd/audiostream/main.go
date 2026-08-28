//go:build tinygo

// Command audiostream exercises the Timer 1 interrupt-assisted production
// candidate: normal code mixes fixed pages and the IRQ only swaps FIFO A DMA.
package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/volatile"
	_ "unsafe" // required for //go:section
)

const (
	statusRunning = 0xA0570001
	statusStalled = 0xA0570002
	statusStopped = 0xA0570003
	statusFailed  = 0xDEADA057
)

//go:section .ewram
var ctx rt.Context

// The queue and its DMA pages live in EWRAM; only hot voice state consumes
// scarce IWRAM.
//
//go:section .ewram
var stream audio.MonoStream

//go:section .iwram
var mixer audio.Mixer

// Debugger-visible telemetry is copied from the interrupt-shared snapshot in
// the normal loop, so external tools never need private struct offsets.
//
//go:section .ewram
var streamStatus volatile.Register32

//go:section .ewram
var streamBoundaries volatile.Register32

//go:section .ewram
var streamUnderruns volatile.Register32

//go:section .ewram
var streamLateBoundaries volatile.Register32

//go:section .ewram
var streamMaxLateSamples volatile.Register32

//go:section .ewram
var streamMixedBlocks volatile.Register32

//go:section .ewram
var streamQueuedBlocks volatile.Register32

// streamServiceEnabled is writable from a debugger for deterministic
// underrun/recovery tests without synthesizing keypad input.
//
//go:section .ewram
var streamServiceEnabled volatile.Register32

func start() bool {
	stream.Stop()
	if !mixer.Init(4) {
		streamStatus.Set(statusFailed)
		return false
	}
	steps := [...]uint32{1 << 23, 1<<23 + 193273, 1<<24 + 104729, 1<<24 + 297121}
	for i, step := range steps {
		if _, ok := mixer.Start(audio.Request{
			Kind: audio.SourceTriangle, Step: step, Volume: uint16(28 - i*2), Priority: 1,
		}); !ok {
			streamStatus.Set(statusFailed)
			return false
		}
	}
	if !stream.Start(&mixer, hw.DirectSoundRate16384) {
		streamStatus.Set(statusFailed)
		return false
	}
	streamServiceEnabled.Set(1)
	streamStatus.Set(statusRunning)
	return true
}

func stop() {
	stream.Stop()
	streamServiceEnabled.Set(0)
	streamStatus.Set(statusStopped)
}

func service() {
	if streamServiceEnabled.Get() != 0 {
		stream.Service(audio.StreamPageCount)
	}
}

func publishTelemetry() audio.StreamStats {
	stats := stream.Stats()
	streamBoundaries.Set(stats.Boundaries)
	streamUnderruns.Set(stats.Underruns)
	streamLateBoundaries.Set(stats.LateBoundaries)
	streamMaxLateSamples.Set(uint32(stats.MaxLateSamples))
	streamMixedBlocks.Set(stats.MixedBlocks)
	streamQueuedBlocks.Set(uint32(stats.QueuedBlocks))
	return stats
}

func draw(stats audio.StreamStats) {
	ctx.Cls(0)
	ctx.Print("MTILE IRQ AUDIO STREAM", 8, 16, 15)
	ctx.Print("16384 HZ  4X256 PAGES", 8, 32, 15)
	ctx.Print("TIMER1 IRQ: DMA SWAP ONLY", 8, 48, 18)
	ctx.Print("A: STALL/RESUME REFILLS", 8, 72, 18)
	ctx.Print("START: RESTART  SELECT: STOP", 8, 88, 18)
	switch {
	case !stats.Running:
		ctx.Print("STATE: STOPPED", 8, 120, 21)
	case streamServiceEnabled.Get() == 0:
		ctx.Print("STATE: STALLED -> SILENCE", 8, 120, 20)
	case stats.Underruns != 0:
		ctx.Print("STATE: RECOVERED UNDERRUN", 8, 120, 20)
	default:
		ctx.Print("STATE: STREAMING", 8, 120, 19)
	}
}

func main() {
	ctx.InitHardware()
	if !start() {
		for {
			draw(stream.Stats())
			ctx.PrepareHiddenPage()
			ctx.WaitForPresentWindow()
			ctx.FlipPage()
			ctx.WaitVDraw()
		}
	}

	for {
		ctx.BeginFramePace()
		ctx.PollInput()
		ctx.AdvanceTick()
		switch {
		case ctx.Btnp(rt.ButtonA):
			if streamServiceEnabled.Get() == 0 {
				streamServiceEnabled.Set(1)
				streamStatus.Set(statusRunning)
			} else {
				streamServiceEnabled.Set(0)
				streamStatus.Set(statusStalled)
			}
		case ctx.Btnp(rt.ButtonStart):
			start()
		case ctx.Btnp(rt.ButtonSelect):
			stop()
		}

		service()
		stats := publishTelemetry()
		draw(stats)
		ctx.PrepareHiddenPage()
		service()
		ctx.PaceFrame30()
		ctx.WaitForPresentWindow()
		ctx.ApplyDisplayPalette()
		ctx.FlipPage()
		ctx.EndFramePace30()
		ctx.WaitVDraw()
	}
}
