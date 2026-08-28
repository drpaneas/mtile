package hw

import "unsafe"

const (
	// FIFOAStreamGuardSamples is the minimum readable tail after a logical
	// block. FIFO DMA fetches 16 bytes at a time and reads ahead of playback.
	FIFOAStreamGuardSamples = 16

	timerIRQEnable uint16 = 1 << 6
	irqTimer1      uint16 = 1 << 4

	fifoDMABurstSamples        uint16 = 16
	fifoInitialPrefetchSamples uint16 = 20
	// The FIFO reaches its 16-byte DMA threshold four samples after a
	// 20-sample boundary fill. mGBA 0.10.3 exposes the completed source burst
	// two sample clocks later; redirecting at progress 4 or 5 must therefore
	// retain the previous source offset instead of skipping 16 samples.
	fifoDMAVisibleProgress uint16 = 6
)

func validFIFOAStreamBlock(samples []uint32, blockSamples uint16) bool {
	if blockSamples == 0 || blockSamples&15 != 0 {
		return false
	}
	requiredSamples := int(blockSamples) + FIFOAStreamGuardSamples
	return len(samples)*4 >= requiredSamples && validFIFOSourceRange(samples)
}

// StartFIFOAStreamBlock starts the production-candidate mono stream. Timer 1
// cascades from the Timer 0 sample clock and requests an IRQ every logical
// block. The supplied internal-RAM page includes a readable guard tail because
// FIFO DMA fetches ahead in 16-byte bursts.
//
// The caller must register and enable the Timer 1 interrupt after this returns.
func StartFIFOAStreamBlock(samples []uint32, blockSamples uint16, clock DirectSoundClock) bool {
	if !clock.valid() || !validFIFOAStreamBlock(samples, blockSamples) {
		return false
	}

	StopFIFOAStream()
	ConfigureDirectSound(DirectSoundA, SoundRouteBoth, DirectSound100, DirectSoundTimer0)
	programFIFOAStreamBlock(samples)

	SetReg16(TM0CNT_L, clock.TimerReload)
	SetReg16(TM1CNT_L, uint16(0)-blockSamples)
	SetReg16(TM1CNT_H, timerStartCascade|timerIRQEnable)
	SetReg16(TM0CNT_H, timerStartF1)
	return true
}

// RedirectFIFOAStreamBlock is the bounded Timer 1 IRQ operation. DMA has
// already prefetched part of the next logical block when the IRQ runs. The
// redirect preserves those queued samples and resumes at the next 16-sample
// burst, avoiding a FIFO reset, duplicated prefix, or silence gap.
func RedirectFIFOAStreamBlock(samples []uint32, blockSamples, progress uint16) bool {
	if progress >= blockSamples || !validFIFOAStreamBlock(samples, blockSamples) {
		return false
	}
	nextSample := fifoInitialPrefetchSamples +
		((progress+fifoDMABurstSamples-fifoDMAVisibleProgress)/fifoDMABurstSamples)*fifoDMABurstSamples
	if int(nextSample)+int(fifoDMABurstSamples) > len(samples)*4 {
		return false
	}
	SetReg16(DMA1CNT+2, 0)
	SetReg32(DMA1SAD, uint32(uintptr(unsafe.Pointer(&samples[nextSample/4]))))
	SetReg32(DMA1DAD, uint32(FIFO_A))
	SetReg16(DMA1CNT, 4)
	SetReg16(DMA1CNT+2, fifoAStreamDMA)
	return true
}

func programFIFOAStreamBlock(samples []uint32) {
	SetReg16(DMA1CNT+2, 0)
	ConfigureDirectSound(DirectSoundA, SoundRouteBoth, DirectSound100, DirectSoundTimer0)
	SetReg32(FIFO_A, samples[0])
	SetReg32(DMA1SAD, uint32(uintptr(unsafe.Pointer(&samples[1]))))
	SetReg32(DMA1DAD, uint32(FIFO_A))
	SetReg16(DMA1CNT, 4)
	SetReg16(DMA1CNT+2, fifoAStreamDMA)
}

// FIFOAStreamBlockProgress returns Timer 1's modulo-block sample progress.
// A non-zero value observed by the IRQ indicates handler latency after the
// exact logical boundary; the caller's guard supplies the following logical
// samples, or cleared silence for an unpublished page, during that window.
func FIFOAStreamBlockProgress(blockSamples uint16) uint16 {
	if blockSamples == 0 {
		return 0
	}
	reload := uint16(0) - blockSamples
	return GetReg16(TM1CNT_L) - reload
}

// StopFIFOAStream disables the sample/block timers before DMA, masks and
// acknowledges Timer 1, resets FIFO A, and disables Direct Sound.
func StopFIFOAStream() {
	SetReg16(TM0CNT_H, 0)
	SetReg16(TM1CNT_H, 0)
	SetReg16(INTERRUPT_IE, GetReg16(INTERRUPT_IE)&^irqTimer1)
	SetReg16(INTERRUPT_IF, irqTimer1)
	SetReg16(DMA1CNT+2, 0)
	DisableDirectSound(DirectSoundA)
	SetReg32(FIFO_A, 0)
	SetReg32(DMA1SAD, 0)
	SetReg32(DMA1DAD, 0)
	SetReg16(DMA1CNT, 0)
	SetReg16(TM0CNT_L, 0)
	SetReg16(TM1CNT_L, 0)
}
