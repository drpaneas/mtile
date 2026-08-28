//go:build tinygo

// Command audiostereocapture runs the provisional two-voice stereo profile
// continuously so mGBA's internal audio callback can verify FIFO A/B routing.
package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
	"github.com/drpaneas/mtile/runtime/internal/stereocapture"
	"runtime/volatile"
	_ "unsafe" // required for //go:section
)

const (
	captureRunning = 0xA0CA0002
	captureFailed  = 0xDEADCA02
)

//go:section .ewram
var ctx rt.Context

//go:section .ewram
var stream audio.StereoStream

//go:section .iwram
var mixer audio.Mixer

//go:section .ewram
var captureStatus volatile.Register32

//go:section .ewram
var captureBoundaries volatile.Register32

//go:section .ewram
var captureUnderruns volatile.Register32

func fail() {
	captureStatus.Set(captureFailed)
	for {
	}
}

func main() {
	ctx.InitHardware()
	if !stereocapture.Start(&mixer) {
		fail()
	}
	if !stream.Start(&mixer, hw.DirectSoundRate16384) {
		fail()
	}
	captureStatus.Set(captureRunning)

	for {
		stream.Service(audio.StereoStreamMaxServiceBlocks)
		stats := stream.Stats()
		captureBoundaries.Set(stats.Boundaries)
		captureUnderruns.Set(stats.Underruns)
		if !stats.Running || stats.Underruns != 0 || stats.SwitchFailures != 0 {
			captureStatus.Set(captureFailed)
		}
	}
}
