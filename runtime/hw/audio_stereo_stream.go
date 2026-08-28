package hw

import "unsafe"

func validStereoStreamBlock(left, right []uint32, blockSamples uint16) bool {
	return validFIFOAStreamBlock(left, blockSamples) && validFIFOAStreamBlock(right, blockSamples)
}

// StartStereoStreamBlock starts synchronized Direct Sound streams: FIFO A is
// routed to the left output and fed by DMA 1, while FIFO B is routed to the
// right output and fed by DMA 2. Both FIFOs consume Timer 0 and Timer 1 emits
// one logical-page IRQ for the pair.
//
// The caller must register and enable the Timer 1 interrupt after this
// returns. Both pages must remain in internal RAM while playback is active.
func StartStereoStreamBlock(left, right []uint32, blockSamples uint16, clock DirectSoundClock) bool {
	if !clock.valid() || !validStereoStreamBlock(left, right, blockSamples) {
		return false
	}

	StopStereoStream()
	programStereoStreamBlock(left, right)

	SetReg16(TM0CNT_L, clock.TimerReload)
	SetReg16(TM1CNT_L, uint16(0)-blockSamples)
	SetReg16(TM1CNT_H, timerStartCascade|timerIRQEnable)
	SetReg16(TM0CNT_H, timerStartF1)
	return true
}

// RedirectStereoStreamBlock switches both FIFO DMA sources at one Timer 1
// boundary. The same progress-derived burst offset is applied to each side so
// a delayed handler cannot skew the channels relative to one another.
func RedirectStereoStreamBlock(left, right []uint32, blockSamples, progress uint16) bool {
	if progress >= blockSamples || !validStereoStreamBlock(left, right, blockSamples) {
		return false
	}
	nextSample := fifoInitialPrefetchSamples +
		((progress+fifoDMABurstSamples-fifoDMAVisibleProgress)/fifoDMABurstSamples)*fifoDMABurstSamples
	if int(nextSample)+int(fifoDMABurstSamples) > len(left)*4 ||
		int(nextSample)+int(fifoDMABurstSamples) > len(right)*4 {
		return false
	}

	SetReg16(DMA1CNT+2, 0)
	SetReg16(DMA2CNT+2, 0)
	SetReg32(DMA1SAD, uint32(uintptr(unsafe.Pointer(&left[nextSample/4]))))
	SetReg32(DMA1DAD, uint32(FIFO_A))
	SetReg16(DMA1CNT, 4)
	SetReg32(DMA2SAD, uint32(uintptr(unsafe.Pointer(&right[nextSample/4]))))
	SetReg32(DMA2DAD, uint32(FIFO_B))
	SetReg16(DMA2CNT, 4)
	SetReg16(DMA1CNT+2, fifoAStreamDMA)
	SetReg16(DMA2CNT+2, fifoBStreamDMA)
	return true
}

func programStereoStreamBlock(left, right []uint32) {
	SetReg16(DMA1CNT+2, 0)
	SetReg16(DMA2CNT+2, 0)
	ConfigureDirectSound(DirectSoundA, SoundRouteLeft, DirectSound100, DirectSoundTimer0)
	ConfigureDirectSound(DirectSoundB, SoundRouteRight, DirectSound100, DirectSoundTimer0)
	SetReg32(FIFO_A, left[0])
	SetReg32(FIFO_B, right[0])
	SetReg32(DMA1SAD, uint32(uintptr(unsafe.Pointer(&left[1]))))
	SetReg32(DMA1DAD, uint32(FIFO_A))
	SetReg16(DMA1CNT, 4)
	SetReg32(DMA2SAD, uint32(uintptr(unsafe.Pointer(&right[1]))))
	SetReg32(DMA2DAD, uint32(FIFO_B))
	SetReg16(DMA2CNT, 4)
	SetReg16(DMA1CNT+2, fifoAStreamDMA)
	SetReg16(DMA2CNT+2, fifoBStreamDMA)
}

// StereoStreamBlockProgress is the shared modulo-page sample progress. Both
// FIFOs consume the same Timer 0 clock, so one Timer 1 counter governs them.
func StereoStreamBlockProgress(blockSamples uint16) uint16 {
	return FIFOAStreamBlockProgress(blockSamples)
}

// StopStereoStream stops the shared clock before both DMA channels, masks and
// acknowledges Timer 1, clears both FIFOs and routing, and disables sound.
func StopStereoStream() {
	SetReg16(TM0CNT_H, 0)
	SetReg16(TM1CNT_H, 0)
	SetReg16(INTERRUPT_IE, GetReg16(INTERRUPT_IE)&^irqTimer1)
	SetReg16(INTERRUPT_IF, irqTimer1)
	SetReg16(DMA1CNT+2, 0)
	SetReg16(DMA2CNT+2, 0)
	DisableDirectSound(DirectSoundA)
	DisableDirectSound(DirectSoundB)
	SetReg32(FIFO_A, 0)
	SetReg32(FIFO_B, 0)
	SetReg32(DMA1SAD, 0)
	SetReg32(DMA1DAD, 0)
	SetReg16(DMA1CNT, 0)
	SetReg32(DMA2SAD, 0)
	SetReg32(DMA2DAD, 0)
	SetReg16(DMA2CNT, 0)
	SetReg16(TM0CNT_L, 0)
	SetReg16(TM1CNT_L, 0)
}
