//go:build tinygo

package audio

import (
	"device/gba"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/interrupt"
	_ "unsafe" // required for //go:section
)

const (
	nativeTimer0IRQBit uint16 = 1 << 3
	nativeTimerEnabled uint16 = 1<<7 | 1<<6
)

var (
	activeNativeController *Controller
	nativeTimer0IRQ        = interrupt.New(gba.IRQ_TIMER0, handleNativeTimer0)
)

func claimNativeController(controller *Controller) bool {
	if activeNativeController != nil && activeNativeController != controller {
		return false
	}
	activeNativeController = controller
	return true
}

func releaseNativeController(controller *Controller) {
	if activeNativeController == controller {
		activeNativeController = nil
	}
}

func startNativeClock() {
	// 16,777,216 / 65,536 = exactly 256 Hz with the F/1 prescaler.
	hw.SetReg16(hw.TM0CNT_H, 0)
	hw.SetReg16(hw.TM0CNT_L, 0)
	nativeTimer0IRQ.Enable()
	hw.SetReg16(hw.TM0CNT_H, nativeTimerEnabled)
}

func stopNativeClock() {
	hw.SetReg16(hw.TM0CNT_H, 0)
	hw.SetReg16(hw.INTERRUPT_IE, hw.GetReg16(hw.INTERRUPT_IE)&^nativeTimer0IRQBit)
}

//go:noinline
//go:section .iwram
func handleNativeTimer0(interrupt.Interrupt) {
	if activeNativeController != nil {
		activeNativeController.serviceCommit()
	}
}

type audioInterruptState = interrupt.State

func disableAudioInterrupts() audioInterruptState      { return interrupt.Disable() }
func restoreAudioInterrupts(state audioInterruptState) { interrupt.Restore(state) }
