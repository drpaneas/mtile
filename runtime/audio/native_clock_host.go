//go:build !tinygo

package audio

import "github.com/drpaneas/mtile/runtime/hw"

const (
	nativeTimer0IRQ    uint16 = 1 << 3
	nativeTimerEnabled uint16 = 1<<7 | 1<<6
)

var activeNativeController *Controller

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
	hw.SetReg16(hw.INTERRUPT_IE, hw.GetReg16(hw.INTERRUPT_IE)|nativeTimer0IRQ)
	hw.SetReg16(hw.TM0CNT_H, nativeTimerEnabled)
}

func stopNativeClock() {
	hw.SetReg16(hw.TM0CNT_H, 0)
	hw.SetReg16(hw.INTERRUPT_IE, hw.GetReg16(hw.INTERRUPT_IE)&^nativeTimer0IRQ)
}

func serviceNativeTimerForTest() {
	if activeNativeController != nil {
		activeNativeController.serviceCommit()
	}
}

type audioInterruptState uint8

// Mirror the GBA interrupt master-enable register so host tests can verify
// that every critical-section exit preserves the caller's interrupt state.
func disableAudioInterrupts() audioInterruptState {
	state := audioInterruptState(hw.GetReg16(hw.INTERRUPT_IME))
	hw.SetReg16(hw.INTERRUPT_IME, 0)
	return state
}

func restoreAudioInterrupts(state audioInterruptState) {
	hw.SetReg16(hw.INTERRUPT_IME, uint16(state))
}
