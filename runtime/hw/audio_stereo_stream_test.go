//go:build !tinygo

package hw

import (
	"testing"
	"unsafe"
)

func TestStartStereoStreamBlockProgramsDualFIFOAndSharedClock(t *testing.T) {
	ResetForTest()
	left := testStreamPage(0x1000)
	right := testStreamPage(0x2000)
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	if !StartStereoStreamBlock(left[:], right[:], testStreamBlockSamples, clock) {
		t.Fatal("StartStereoStreamBlock rejected valid pages")
	}

	wantSound := soundAFIFOVolume | soundBFIFOVolume | soundALeft | soundBRight
	if got := GetReg16(SOUNDCNT_H); got != wantSound {
		t.Errorf("SOUNDCNT_H = %#04x, want left A/right B routing %#04x", got, wantSound)
	}
	if got, want := GetReg32(FIFO_A), left[0]; got != want {
		t.Errorf("FIFO_A = %#08x, want %#08x", got, want)
	}
	if got, want := GetReg32(FIFO_B), right[0]; got != want {
		t.Errorf("FIFO_B = %#08x, want %#08x", got, want)
	}
	if got, want := GetReg32(DMA1SAD), uint32(uintptr(unsafe.Pointer(&left[1]))); got != want {
		t.Errorf("DMA1SAD = %#08x, want %#08x", got, want)
	}
	if got, want := GetReg32(DMA2SAD), uint32(uintptr(unsafe.Pointer(&right[1]))); got != want {
		t.Errorf("DMA2SAD = %#08x, want %#08x", got, want)
	}
	if got := GetReg32(DMA1DAD); got != uint32(FIFO_A) {
		t.Errorf("DMA1DAD = %#08x, want FIFO_A", got)
	}
	if got := GetReg32(DMA2DAD); got != uint32(FIFO_B) {
		t.Errorf("DMA2DAD = %#08x, want FIFO_B", got)
	}
	if got := GetReg16(DMA1CNT + 2); got != fifoAStreamDMA {
		t.Errorf("DMA1CNT_H = %#04x, want %#04x", got, fifoAStreamDMA)
	}
	if got := GetReg16(DMA2CNT + 2); got != fifoBStreamDMA {
		t.Errorf("DMA2CNT_H = %#04x, want %#04x", got, fifoBStreamDMA)
	}
	if got, want := GetReg16(TM1CNT_L), uint16(0xFF00); got != want {
		t.Errorf("TM1CNT_L = %#04x, want %#04x", got, want)
	}
	if got := GetReg16(TM1CNT_H); got != timerStartCascade|timerIRQEnable {
		t.Errorf("TM1CNT_H = %#04x, want running cascade IRQ", got)
	}
	if got := GetReg16(TM0CNT_H); got != timerStartF1 {
		t.Errorf("TM0CNT_H = %#04x, want running sample clock", got)
	}
}

func TestRedirectStereoStreamBlockKeepsChannelsAtSameBurstOffset(t *testing.T) {
	ResetForTest()
	left := testStreamPage(0x1000)
	right := testStreamPage(0x2000)
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	if !StartStereoStreamBlock(left[:], right[:], testStreamBlockSamples, clock) {
		t.Fatal("initial stereo start failed")
	}

	SetReg16(TM1CNT_L, 0xFF07)
	if got := StereoStreamBlockProgress(testStreamBlockSamples); got != 7 {
		t.Fatalf("stereo progress = %d, want 7", got)
	}
	SetReg32(FIFO_A, 0xAABBCCDD)
	SetReg32(FIFO_B, 0x11223344)
	if !RedirectStereoStreamBlock(left[:], right[:], testStreamBlockSamples, 7) {
		t.Fatal("stereo redirect failed")
	}
	if got := GetReg32(FIFO_A); got != 0xAABBCCDD {
		t.Errorf("redirect changed prefetched FIFO_A to %#08x", got)
	}
	if got := GetReg32(FIFO_B); got != 0x11223344 {
		t.Errorf("redirect changed prefetched FIFO_B to %#08x", got)
	}
	if got, want := GetReg32(DMA1SAD), uint32(uintptr(unsafe.Pointer(&left[9]))); got != want {
		t.Errorf("DMA1SAD = %#08x, want left word 9 %#08x", got, want)
	}
	if got, want := GetReg32(DMA2SAD), uint32(uintptr(unsafe.Pointer(&right[9]))); got != want {
		t.Errorf("DMA2SAD = %#08x, want right word 9 %#08x", got, want)
	}
}

func TestStereoStreamRejectsMismatchedGeometryWithoutMutation(t *testing.T) {
	ResetForTest()
	left := testStreamPage(0)
	right := make([]uint32, testStreamBlockSamples/4)
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	SetReg32(DMA1SAD, 0x12345678)
	SetReg32(DMA2SAD, 0x87654321)
	if StartStereoStreamBlock(left[:], right, testStreamBlockSamples, clock) {
		t.Fatal("start accepted a right page without a guard")
	}
	if RedirectStereoStreamBlock(left[:], right, testStreamBlockSamples, 0) {
		t.Fatal("redirect accepted mismatched pages")
	}
	if got := GetReg32(DMA1SAD); got != 0x12345678 {
		t.Fatalf("rejected operation changed DMA1SAD to %#08x", got)
	}
	if got := GetReg32(DMA2SAD); got != 0x87654321 {
		t.Fatalf("rejected operation changed DMA2SAD to %#08x", got)
	}
}

func TestStopStereoStreamClearsBothChannels(t *testing.T) {
	ResetForTest()
	left := testStreamPage(0x1000)
	right := testStreamPage(0x2000)
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	StartStereoStreamBlock(left[:], right[:], testStreamBlockSamples, clock)
	SetReg16(INTERRUPT_IE, 0xFFFF)
	StopStereoStream()

	if got := GetReg16(INTERRUPT_IE); got != 0xFFEF {
		t.Errorf("INTERRUPT_IE = %#04x, want Timer 1 masked", got)
	}
	if got := GetReg16(INTERRUPT_IF); got != irqTimer1 {
		t.Errorf("INTERRUPT_IF = %#04x, want Timer 1 acknowledgement", got)
	}
	if got := GetReg16(SOUNDCNT_H); got != 0 {
		t.Errorf("SOUNDCNT_H = %#04x, want cleared stable state", got)
	}
	for _, addr := range []uintptr{
		TM0CNT_H, TM1CNT_H, DMA1CNT + 2, DMA2CNT + 2, SOUNDCNT_X,
		FIFO_A, FIFO_B, DMA1SAD, DMA1DAD, DMA2SAD, DMA2DAD,
	} {
		if got := GetReg32(addr); got != 0 {
			t.Errorf("register %#x = %#08x after stop, want 0", addr, got)
		}
	}
}
