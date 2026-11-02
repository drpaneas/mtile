//go:build tinygo && mtile_pcm

package audio

import (
	"device/gba"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/interrupt"
	"unsafe"
)

var activePCM *PCMController
var pcmVBlankIRQ = interrupt.New(gba.IRQ_VBLANK, pcmVBlank)
var pcmMixIRQ = interrupt.New(gba.IRQ_VCOUNT, pcmMix)

func (p *PCMController) initPCM(bank []uint32) bool {
	if activePCM != nil {
		return false
	}
	state := interrupt.Disable()
	ok := maxmodInit(unsafe.Pointer(&bank[0])) != 0
	if ok {
		hw.ConfigureDirectSound(hw.DirectSoundA, hw.SoundRouteLeft, hw.DirectSound100, hw.DirectSoundTimer1)
		hw.ConfigureDirectSound(hw.DirectSoundB, hw.SoundRouteRight, hw.DirectSound100, hw.DirectSoundTimer1)
		activePCM = p
	}
	interrupt.Restore(state)
	return ok
}

func (p *PCMController) startPCM() {
	// DMA reset stays at VBlank. Mixing happens at line 0, outside the display
	// commit window, at the LCD's 59.73 Hz even while the Lua game runs at 30 FPS.
	state := interrupt.Disable()
	const dispstat uintptr = 0x04000004
	hw.SetReg16(dispstat, hw.GetReg16(dispstat)&0x00ff|1<<3|1<<5)
	pcmVBlankIRQ.Enable()
	pcmMixIRQ.Enable()
	interrupt.Restore(state)
}

func (p *PCMController) musicPCM(id int, loop bool) {
	state := interrupt.Disable()
	mode := 0
	if loop {
		mode = 1
	}
	maxmodMusic(id, mode)
	interrupt.Restore(state)
}
func (p *PCMController) samplePCM(id, volume, priority int) {
	state := interrupt.Disable()
	maxmodSample(id, volume, priority)
	interrupt.Restore(state)
}

//go:section .iwram
func pcmVBlank(interrupt.Interrupt) { maxmodVBlank() }

func pcmMix(interrupt.Interrupt) {
	if activePCM == nil {
		return
	}
	start := hw.ReadFrameCycleCounter()
	maxmodFrame()
	cycles := hw.ReadFrameCycleCounter() - start
	activePCM.frames++
	if cycles > activePCM.maxCycles {
		activePCM.maxCycles = cycles
	}
}

//export mtile_mm_init
func maxmodInit(unsafe.Pointer) int

//export mtile_mm_music
func maxmodMusic(id, loop int)

//export mtile_mm_sample
func maxmodSample(id, volume, priority int)

//export mmVBlank
func maxmodVBlank()

//export mmFrame
func maxmodFrame()
