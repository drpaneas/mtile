//go:build !tinygo

package hw

import (
	"testing"
	"unsafe"
)

func TestDirectSoundCandidateClocks(t *testing.T) {
	tests := []struct {
		rate   DirectSoundRate
		target uint32
		reload uint16
		period uint16
		mHz    uint32
	}{
		{DirectSoundRate16384, 16_384, 0xFC00, 1024, 16_384_000},
		{DirectSoundRate18157, 18_157, 0xFC64, 924, 18_157_160},
		{DirectSoundRate22050, 22_050, 0xFD07, 761, 22_046_276},
		{DirectSoundRate32768, 32_768, 0xFE00, 512, 32_768_000},
	}
	for _, tt := range tests {
		clock, ok := DirectSoundClockForRate(tt.rate)
		if !ok {
			t.Fatalf("DirectSoundClockForRate(%d) was rejected", tt.rate)
		}
		if clock.TargetHz != tt.target || clock.TimerReload != tt.reload || clock.PeriodCycles != tt.period {
			t.Errorf("clock %d = %+v, want target=%d reload=%#04x period=%d", tt.rate, clock, tt.target, tt.reload, tt.period)
		}
		if got := clock.ActualMilliHz(); got != tt.mHz {
			t.Errorf("clock %d actual = %d mHz, want %d", tt.rate, got, tt.mHz)
		}
	}
	if _, ok := DirectSoundClockForRate(DirectSoundRate(255)); ok {
		t.Fatal("unknown DirectSoundRate was accepted")
	}
}

func TestStartFIFOAProbeProgramsSoundTimerAndDMA(t *testing.T) {
	ResetForTest()
	word := uint32(0xC0C04040)
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	if !StartFIFOAProbe(&word, clock) {
		t.Fatal("StartFIFOAProbe rejected a valid request")
	}

	want16 := map[uintptr]uint16{
		SOUNDCNT_L:  0,
		SOUNDCNT_H:  soundAFIFOVolume | soundARight | soundALeft,
		SOUNDCNT_X:  soundMasterEnable,
		SOUNDBIAS:   0x0200,
		DMA1CNT:     4,
		DMA1CNT + 2: fifoAFixedWordDMA,
		TM0CNT_L:    clock.TimerReload,
		TM0CNT_H:    timerStartF1,
	}
	for addr, want := range want16 {
		if got := GetReg16(addr); got != want {
			t.Errorf("register %#x = %#04x, want %#04x", addr, got, want)
		}
	}
	if got, want := GetReg32(FIFO_A), word; got != want {
		t.Errorf("FIFO_A = %#08x, want %#08x", got, want)
	}
	if got, want := GetReg32(DMA1SAD), uint32(uintptr(unsafe.Pointer(&word))); got != want {
		t.Errorf("DMA1SAD = %#08x, want %#08x", got, want)
	}
	if got, want := GetReg32(DMA1DAD), uint32(FIFO_A); got != want {
		t.Errorf("DMA1DAD = %#08x, want %#08x", got, want)
	}
}

func TestStartFIFOAStreamProbeProgramsIncrementingDMAAndSampleCounter(t *testing.T) {
	ResetForTest()
	words := [3]uint32{0x03020100, 0x07060504, 0x0B0A0908}
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	if !StartFIFOAStreamProbe(words[:], clock) {
		t.Fatal("StartFIFOAStreamProbe rejected a valid request")
	}

	if got, want := GetReg32(FIFO_A), words[0]; got != want {
		t.Errorf("FIFO_A = %#08x, want priming word %#08x", got, want)
	}
	if got, want := GetReg32(DMA1SAD), uint32(uintptr(unsafe.Pointer(&words[1]))); got != want {
		t.Errorf("DMA1SAD = %#08x, want second word %#08x", got, want)
	}
	if got := GetReg16(DMA1CNT + 2); got != fifoAStreamDMA {
		t.Errorf("DMA1CNT_H = %#04x, want incrementing FIFO control %#04x", got, fifoAStreamDMA)
	}
	if got := GetReg16(TM1CNT_H); got != timerStartCascade {
		t.Errorf("TM1CNT_H = %#04x, want cascaded sample counter %#04x", got, timerStartCascade)
	}
	SetReg16(TM1CNT_L, 1234)
	if got := FIFOAProbeSampleCount(); got != 1234 {
		t.Errorf("FIFOAProbeSampleCount() = %d, want 1234", got)
	}
}

func TestFIFOAProbeRejectsInvalidRequestWithoutChangingHardware(t *testing.T) {
	ResetForTest()
	SetReg16(SOUNDCNT_X, 0x0055)
	if StartFIFOAProbe(nil, DirectSoundClock{}) {
		t.Fatal("StartFIFOAProbe accepted nil source and invalid clock")
	}
	if got := GetReg16(SOUNDCNT_X); got != 0x0055 {
		t.Fatalf("rejected start changed SOUNDCNT_X to %#04x", got)
	}
	word := uint32(0)
	if StartFIFOAStreamProbe(nil, DirectSoundClock{}) || StartFIFOAStreamProbe([]uint32{word}, DirectSoundClock{}) {
		t.Fatal("StartFIFOAStreamProbe accepted a short buffer and invalid clock")
	}
	if got := GetReg16(SOUNDCNT_X); got != 0x0055 {
		t.Fatalf("rejected stream start changed SOUNDCNT_X to %#04x", got)
	}
}

func TestStopFIFOAProbeClearsActiveState(t *testing.T) {
	ResetForTest()
	word := uint32(0xC0C04040)
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	StartFIFOAProbe(&word, clock)
	StopFIFOAProbe()

	if got := GetReg16(TM0CNT_H); got != 0 {
		t.Errorf("TM0CNT_H = %#04x after stop, want 0", got)
	}
	if got := GetReg16(TM1CNT_H); got != 0 {
		t.Errorf("TM1CNT_H = %#04x after stop, want 0", got)
	}
	if got := GetReg16(TM1CNT_L); got != 0 {
		t.Errorf("TM1CNT_L = %#04x after stop, want 0", got)
	}
	if got := GetReg16(DMA1CNT + 2); got != 0 {
		t.Errorf("DMA1CNT_H = %#04x after stop, want 0", got)
	}
	if got := GetReg16(SOUNDCNT_H); got != 0 {
		t.Errorf("SOUNDCNT_H = %#04x after stop, want cleared stable state", got)
	}
	if got := GetReg16(SOUNDCNT_X); got != 0 {
		t.Errorf("SOUNDCNT_X = %#04x after stop, want 0", got)
	}
	if got := GetReg32(FIFO_A); got != 0 {
		t.Errorf("FIFO_A = %#08x after stop, want 0", got)
	}
	if got := GetReg32(DMA1SAD); got != 0 {
		t.Errorf("DMA1SAD = %#08x after stop, want 0", got)
	}
	if got := GetReg32(DMA1DAD); got != 0 {
		t.Errorf("DMA1DAD = %#08x after stop, want 0", got)
	}
}
