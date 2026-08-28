//go:build tinygo

package audio

import (
	"device/gba"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/interrupt"
	_ "unsafe" // required for //go:section
)

type streamInterruptState = interrupt.State

var (
	activeMonoStream    *MonoStream
	activeStereoStream  *StereoStream
	monoStreamTimer1IRQ = interrupt.New(gba.IRQ_TIMER1, handleMonoStreamTimer1)
)

func disableStreamInterrupts() streamInterruptState      { return interrupt.Disable() }
func restoreStreamInterrupts(state streamInterruptState) { interrupt.Restore(state) }

func claimMonoStream(stream *MonoStream) bool {
	if activeNativeController != nil || activeStereoStream != nil || activeMonoStream != nil && activeMonoStream != stream {
		return false
	}
	activeMonoStream = stream
	return true
}

func claimStereoStream(stream *StereoStream) bool {
	if activeNativeController != nil || activeMonoStream != nil || activeStereoStream != nil && activeStereoStream != stream {
		return false
	}
	activeStereoStream = stream
	return true
}

func releaseStereoStream(stream *StereoStream) {
	if activeStereoStream == stream {
		activeStereoStream = nil
	}
}

func releaseMonoStream(stream *MonoStream) {
	if activeMonoStream == stream {
		activeMonoStream = nil
	}
}

func enableMonoStreamIRQ() {
	monoStreamTimer1IRQ.Enable()
}

func enableStereoStreamIRQ() { monoStreamTimer1IRQ.Enable() }

// settleStreamRedirectProgress closes the two-sample mGBA FIFO-DMA race.
// At progress 4 or 5 within a 16-sample burst, the FIFO request may or may
// not have advanced DMA's internal source latch yet. Waiting until progress 6
// makes the completed-burst count deterministic before Redirect disables DMA.
// The production guard already covers substantially larger IRQ lateness.
//
//go:noinline
//go:section .iwram
func settleStreamRedirectProgress(progress uint16) uint16 {
	for streamRedirectProgressUnsettled(progress) {
		progress = hw.FIFOAStreamBlockProgress(StreamBlockSamples)
	}
	return progress
}

//go:noinline
//go:section .iwram
func handleMonoStreamTimer1(interrupt.Interrupt) {
	if activeMonoStream != nil {
		activeMonoStream.onBlockBoundary()
	} else if activeStereoStream != nil {
		activeStereoStream.onBlockBoundary()
	}
}
