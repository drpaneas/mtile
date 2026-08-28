//go:build !tinygo

package audio

import "github.com/drpaneas/mtile/runtime/hw"

type streamInterruptState uint8

var (
	activeMonoStream   *MonoStream
	activeStereoStream *StereoStream
)

func disableStreamInterrupts() streamInterruptState { return 0 }
func restoreStreamInterrupts(streamInterruptState)  {}

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
	const timer1IRQ = uint16(1 << 4)
	hw.SetReg16(hw.INTERRUPT_IE, hw.GetReg16(hw.INTERRUPT_IE)|timer1IRQ)
}

func enableStereoStreamIRQ() { enableMonoStreamIRQ() }

func settleStreamRedirectProgress(progress uint16) uint16 { return progress }
