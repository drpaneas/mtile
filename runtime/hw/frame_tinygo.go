//go:build tinygo

package hw

import (
	"device/gba"
	"runtime/interrupt"
	"runtime/volatile"
)

// LCD events own the wakeup clock. Audio attaches to these same interrupts;
// long mixing must not hide the fact that a visible interval has started.
var displayFrame volatile.Register32
var displayClockReady bool
var paceFrame uint32
var displayAudioVBlank, displayAudioMix func()
var displayVBlankIRQ = interrupt.New(gba.IRQ_VBLANK, displayVBlank)
var displayVCountIRQ = interrupt.New(gba.IRQ_VCOUNT, displayVCount)

func enableDisplayClock() {
	state := interrupt.Disable()
	if !displayClockReady {
		const dispstat uintptr = 0x04000004
		// Compare at line 0; retain unrelated DISPSTAT interrupt controls.
		SetReg16(dispstat, GetReg16(dispstat)&0x00ff|1<<3|1<<5)
		displayClockReady = true
		displayVBlankIRQ.Enable()
		displayVCountIRQ.Enable()
	}
	interrupt.Restore(state)
}

// SetDisplayAudio shares the LCD interrupts with the optional PCM controller.
// Callbacks are fixed, allocation-free functions installed once at startup.
func SetDisplayAudio(vblank, mix func()) {
	state := interrupt.Disable()
	displayAudioVBlank, displayAudioMix = vblank, mix
	enableDisplayClock()
	interrupt.Restore(state)
}

func displayVBlank(interrupt.Interrupt) {
	if displayAudioVBlank != nil {
		displayAudioVBlank()
	}
}

func displayVCount(interrupt.Interrupt) {
	displayFrame.Set(displayFrame.Get() + 1)
	if displayAudioMix != nil {
		displayAudioMix()
	}
}

// BeginFramePace anchors presentation to LCD events rather than to the time
// that the main loop finally regains control after a potentially long IRQ.
func BeginFramePace() {
	enableDisplayClock()
	state := interrupt.Disable()
	paceFrame = displayFrame.Get() + 1
	StartFrameCycleCounter()
	interrupt.Restore(state)
}

func WaitFramePace() {
	if gba.INTERRUPT.PAUSE.Get() == 0 || interrupt.In() {
		// Native callers may deliberately mask IRQs. Their timer still runs,
		// but the event counter cannot advance until handlers can execute.
		PaceWaitUntil(280896)
		return
	}
	for {
		state := interrupt.Disable()
		ready := int32(displayFrame.Get()-paceFrame) >= 0
		if !ready {
			haltDisplayWait()
		}
		interrupt.Restore(state)
		if ready {
			return
		}
	}
}

// WaitFrameStart consumes the current VBlank even if an IRQ occupies the
// entire following visible interval. Rendering code needing a physically
// visible scanline must instead use WaitVDraw, then recheck its VRAM window.
func WaitFrameStart() {
	enableDisplayClock()
	state := interrupt.Disable()
	frame := displayFrame.Get()
	visible := GetReg16(VCOUNT) < ScreenHeight
	interrupt.Restore(state)
	if visible {
		return
	}
	if state == 0 || interrupt.In() {
		WaitVDraw()
		return
	}
	for {
		state = interrupt.Disable()
		ready := displayFrame.Get() != frame
		if !ready {
			haltDisplayWait()
		}
		interrupt.Restore(state)
		if ready {
			return
		}
	}
}

// IME stays disabled from the predicate check until HALT. Pending IE&IF
// requests wake HALT even with IME=0, closing the check/IRQ/sleep race. The
// caller restores IME and services the request before checking again.
//
//export mtile_halt_display_wait
func haltDisplayWait()
