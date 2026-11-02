//go:build tinygo

package hw

import (
	"device/gba"
	"runtime/interrupt"
)

func WaitForPresentWindow() {
	waitDisplayPhase(true)
}

func WaitVDraw() {
	waitDisplayPhase(false)
}

func waitDisplayPhase(vblank bool) {
	enableDisplayClock()
	if gba.INTERRUPT.PAUSE.Get() == 0 || interrupt.In() {
		for (GetReg16(VCOUNT) >= ScreenHeight) != vblank {
		}
		return
	}
	for {
		state := interrupt.Disable()
		ready := (GetReg16(VCOUNT) >= ScreenHeight) == vblank
		if !ready {
			haltDisplayWait()
		}
		interrupt.Restore(state)
		if ready {
			return
		}
	}
}
