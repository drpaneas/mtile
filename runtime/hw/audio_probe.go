package hw

import "unsafe"

// GBAClockHz is the ARM7TDMI and timer input clock in normal GBA operation.
const GBAClockHz uint32 = 16_777_216

// DirectSoundRate identifies one hardware-valid sample clock selected for the
// Milestone 0 probe matrix. These are named choices, not arbitrary user rates.
type DirectSoundRate uint8

const (
	DirectSoundRate16384 DirectSoundRate = iota
	DirectSoundRate18157
	DirectSoundRate22050
	DirectSoundRate32768
)

// DirectSoundClock is the exact Timer 0 configuration for a candidate rate.
// The hardware rate is GBAClockHz / PeriodCycles; TargetHz is only the human
// label used by settings and reports.
type DirectSoundClock struct {
	TargetHz     uint32
	TimerReload  uint16
	PeriodCycles uint16
}

// DirectSoundClockForRate returns a tested candidate clock. The 22.05 kHz
// class uses the nearest whole-cycle timer period and therefore runs at
// approximately 22,046.276 Hz.
func DirectSoundClockForRate(rate DirectSoundRate) (DirectSoundClock, bool) {
	switch rate {
	case DirectSoundRate16384:
		return DirectSoundClock{TargetHz: 16_384, TimerReload: 0xFC00, PeriodCycles: 1024}, true
	case DirectSoundRate18157:
		return DirectSoundClock{TargetHz: 18_157, TimerReload: 0xFC64, PeriodCycles: 924}, true
	case DirectSoundRate22050:
		return DirectSoundClock{TargetHz: 22_050, TimerReload: 0xFD07, PeriodCycles: 761}, true
	case DirectSoundRate32768:
		return DirectSoundClock{TargetHz: 32_768, TimerReload: 0xFE00, PeriodCycles: 512}, true
	default:
		return DirectSoundClock{}, false
	}
}

func (c DirectSoundClock) valid() bool {
	return c.TargetHz > 0 && c.PeriodCycles > 0 &&
		uint32(c.TimerReload)+uint32(c.PeriodCycles) == 1<<16
}

// ActualMilliHz reports the rounded physical rate for host-side diagnostics.
func (c DirectSoundClock) ActualMilliHz() uint32 {
	if !c.valid() {
		return 0
	}
	period := uint64(c.PeriodCycles)
	return uint32((uint64(GBAClockHz)*1000 + period/2) / period)
}

const (
	soundMasterEnable uint16 = 1 << 7
	soundAFIFOVolume  uint16 = 1 << 2
	soundBFIFOVolume  uint16 = 1 << 3
	soundARight       uint16 = 1 << 8
	soundALeft        uint16 = 1 << 9
	soundAFIFOReset   uint16 = 1 << 11
	soundBRight       uint16 = 1 << 12
	soundBLeft        uint16 = 1 << 13
	soundBFIFOReset   uint16 = 1 << 15

	audioTimerCascade uint16 = 1 << 2
	timerStartF1      uint16 = 1 << 7
	timerStartCascade        = timerStartF1 | audioTimerCascade

	dmaDestinationFixed uint16 = 2 << 5
	dmaSourceFixed      uint16 = 2 << 7
	dmaRepeat           uint16 = 1 << 9
	dmaTransfer32       uint16 = 1 << 10
	dmaStartSpecial     uint16 = 3 << 12
	dmaEnable           uint16 = 1 << 15

	fifoAFixedWordDMA = dmaDestinationFixed | dmaSourceFixed | dmaRepeat |
		dmaTransfer32 | dmaStartSpecial | dmaEnable
	fifoAStreamDMA = dmaDestinationFixed | dmaRepeat | dmaTransfer32 |
		dmaStartSpecial | dmaEnable
	fifoBStreamDMA = dmaDestinationFixed | dmaRepeat | dmaTransfer32 |
		dmaStartSpecial | dmaEnable
)

// StartFIFOAProbe starts the Milestone 0 fixed-word probe on FIFO A using
// Timer 0 and DMA 1. sampleWord must remain in IWRAM or EWRAM while playback
// is active: DMA 1 cannot read cartridge ROM. Fixed-source DMA repeats the
// same four signed 8-bit samples indefinitely, which is useful for proving
// routing, rate, repeat, stop, and restart before a refill design is chosen.
//
// This is intentionally a probe API, not the production streaming backend.
func StartFIFOAProbe(sampleWord *uint32, clock DirectSoundClock) bool {
	if sampleWord == nil || !clock.valid() {
		return false
	}
	return startFIFOAProbe(*sampleWord, sampleWord, clock, fifoAFixedWordDMA, false)
}

// StartFIFOAStreamProbe starts incrementing-source FIFO A DMA and uses Timer 1
// in cascade mode as a 16-bit count of samples consumed by Timer 0. samples
// must remain in IWRAM or EWRAM and contain at least two words. The first word
// primes FIFO A; DMA begins at the second word so startup does not duplicate
// the first four samples.
//
// FIFO DMA has no source-length or wrap facility. The caller must stop or
// restart this probe before DMA reads beyond the supplied internal-RAM array.
func StartFIFOAStreamProbe(samples []uint32, clock DirectSoundClock) bool {
	if len(samples) < 2 || !clock.valid() {
		return false
	}
	return startFIFOAProbe(samples[0], &samples[1], clock, fifoAStreamDMA, true)
}

func startFIFOAProbe(prime uint32, source *uint32, clock DirectSoundClock, dmaControl uint16, countSamples bool) bool {
	if source == nil || !clock.valid() {
		return false
	}

	StopFIFOAProbe()

	ConfigureDirectSound(DirectSoundA, SoundRouteBoth, DirectSound100, DirectSoundTimer0)

	// Prime the FIFO once, then let special-timing DMA refill it in 16-byte
	// bursts whenever the sound controller reaches its half-empty threshold.
	SetReg32(FIFO_A, prime)
	SetReg32(DMA1SAD, uint32(uintptr(unsafe.Pointer(source))))
	SetReg32(DMA1DAD, uint32(FIFO_A))
	SetReg16(DMA1CNT, 4) // ignored in FIFO mode; explicit for readable probes
	SetReg16(TM0CNT_L, clock.TimerReload)
	if countSamples {
		SetReg16(TM1CNT_L, 0)
		SetReg16(TM1CNT_H, timerStartCascade)
	}
	SetReg16(DMA1CNT+2, dmaControl)
	SetReg16(TM0CNT_H, timerStartF1)
	return true
}

// FIFOAProbeSampleCount returns Timer 1's cascaded count. It is meaningful
// only while an incrementing-source probe is active and wraps after 65,535.
func FIFOAProbeSampleCount() uint16 {
	return GetReg16(TM1CNT_L)
}

// StopFIFOAProbe halts consumption before DMA, resets FIFO A, clears the
// probe registers, and disables the sound master. A later start cannot replay
// stale non-zero FIFO contents from the preceding run.
func StopFIFOAProbe() {
	SetReg16(TM0CNT_H, 0)
	SetReg16(TM1CNT_H, 0)
	SetReg16(DMA1CNT+2, 0)
	DisableDirectSound(DirectSoundA)
	SetReg32(FIFO_A, 0)
	SetReg32(DMA1SAD, 0)
	SetReg32(DMA1DAD, 0)
	SetReg16(DMA1CNT, 0)
	SetReg16(TM0CNT_L, 0)
	SetReg16(TM1CNT_L, 0)
}
