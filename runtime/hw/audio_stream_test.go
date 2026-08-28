//go:build !tinygo

package hw

import (
	"testing"
	"unsafe"
)

const testStreamBlockSamples = 256

func testStreamPage(seed uint32) [testStreamBlockSamples/4 + FIFOAStreamGuardSamples/4]uint32 {
	var page [testStreamBlockSamples/4 + FIFOAStreamGuardSamples/4]uint32
	for i := range page {
		page[i] = seed + uint32(i)
	}
	return page
}

func TestStartFIFOAStreamBlockProgramsPeriodicTimerIRQ(t *testing.T) {
	ResetForTest()
	page := testStreamPage(0x1000)
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	if !StartFIFOAStreamBlock(page[:], testStreamBlockSamples, clock) {
		t.Fatal("StartFIFOAStreamBlock rejected a valid page")
	}

	if got, want := GetReg32(FIFO_A), page[0]; got != want {
		t.Errorf("FIFO_A = %#08x, want %#08x", got, want)
	}
	if got, want := GetReg32(DMA1SAD), uint32(uintptr(unsafe.Pointer(&page[1]))); got != want {
		t.Errorf("DMA1SAD = %#08x, want %#08x", got, want)
	}
	if got := GetReg16(DMA1CNT + 2); got != fifoAStreamDMA {
		t.Errorf("DMA1CNT_H = %#04x, want %#04x", got, fifoAStreamDMA)
	}
	if got, want := GetReg16(TM1CNT_L), uint16(0xFF00); got != want {
		t.Errorf("TM1CNT_L = %#04x, want reload %#04x", got, want)
	}
	if got, want := GetReg16(TM1CNT_H), timerStartCascade|timerIRQEnable; got != want {
		t.Errorf("TM1CNT_H = %#04x, want %#04x", got, want)
	}
	if got := GetReg16(TM0CNT_H); got != timerStartF1 {
		t.Errorf("TM0CNT_H = %#04x, want running", got)
	}
}

func TestFIFOAStreamRedirectPreservesPrefetchedFIFOAndProgress(t *testing.T) {
	ResetForTest()
	first := testStreamPage(0x1000)
	second := testStreamPage(0x2000)
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	if !StartFIFOAStreamBlock(first[:], testStreamBlockSamples, clock) {
		t.Fatal("initial stream start failed")
	}

	SetReg16(TM1CNT_L, 0xFF07)
	if got := FIFOAStreamBlockProgress(testStreamBlockSamples); got != 7 {
		t.Fatalf("block progress = %d, want 7", got)
	}
	SetReg32(FIFO_A, 0xAABBCCDD)
	if !RedirectFIFOAStreamBlock(second[:], testStreamBlockSamples, 7) {
		t.Fatal("stream redirect failed")
	}
	if got := GetReg32(FIFO_A); got != 0xAABBCCDD {
		t.Errorf("redirect changed prefetched FIFO_A to %#08x", got)
	}
	// Progress 7 means the request observed at sample 6 already advanced the source:
	// samples 0..35 are queued, so DMA resumes at sample 36 (word 9).
	if got, want := GetReg32(DMA1SAD), uint32(uintptr(unsafe.Pointer(&second[9]))); got != want {
		t.Errorf("redirected DMA1SAD = %#08x, want %#08x", got, want)
	}
	if got := GetReg16(TM0CNT_H); got != timerStartF1 {
		t.Errorf("switch stopped Timer 0: %#04x", got)
	}
}

func TestFIFOAStreamRedirectAccountsForBurstPrefetch(t *testing.T) {
	ResetForTest()
	var page [140]uint32 // 256 samples plus the production-sized guard
	tests := []struct {
		progress uint16
		word     int
	}{
		{0, 5}, // 20 samples are already queued at an exact boundary
		{3, 5},
		{4, 5}, // mGBA exposes the FIFO request two samples after the threshold
		{5, 5},
		{6, 9}, // the next 16-sample DMA request has occurred
		{19, 9},
		{20, 9},
		{22, 13},
		{255, 69},
	}
	for _, tt := range tests {
		if !RedirectFIFOAStreamBlock(page[:], testStreamBlockSamples, tt.progress) {
			t.Fatalf("redirect at progress %d failed", tt.progress)
		}
		if got, want := GetReg32(DMA1SAD), uint32(uintptr(unsafe.Pointer(&page[tt.word]))); got != want {
			t.Errorf("progress %d DMA1SAD = %#08x, want word %d (%#08x)", tt.progress, got, tt.word, want)
		}
	}
	before := GetReg32(DMA1SAD)
	if RedirectFIFOAStreamBlock(page[:], testStreamBlockSamples, testStreamBlockSamples) {
		t.Fatal("redirect accepted progress outside the logical block")
	}
	if got := GetReg32(DMA1SAD); got != before {
		t.Fatalf("rejected progress changed DMA1SAD to %#08x", got)
	}
}

func TestFIFOAStreamRejectsUnsafeGeometryWithoutMutation(t *testing.T) {
	ResetForTest()
	SetReg32(DMA1SAD, 0x12345678)
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	short := make([]uint32, testStreamBlockSamples/4)
	if StartFIFOAStreamBlock(short, testStreamBlockSamples, clock) {
		t.Fatal("start accepted a page without a DMA guard")
	}
	if RedirectFIFOAStreamBlock(short, testStreamBlockSamples, 0) {
		t.Fatal("redirect accepted a page without a DMA guard")
	}
	if got := GetReg32(DMA1SAD); got != 0x12345678 {
		t.Fatalf("rejected geometry changed DMA1SAD to %#08x", got)
	}
	page := testStreamPage(0)
	if StartFIFOAStreamBlock(page[:], 250, clock) {
		t.Fatal("start accepted a block not aligned to FIFO bursts")
	}
}

func TestStopFIFOAStreamMasksIRQAndClearsHardware(t *testing.T) {
	ResetForTest()
	page := testStreamPage(0)
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	StartFIFOAStreamBlock(page[:], testStreamBlockSamples, clock)
	SetReg16(INTERRUPT_IE, 0xFFFF)
	StopFIFOAStream()

	if got := GetReg16(INTERRUPT_IE); got != 0xFFEF {
		t.Errorf("INTERRUPT_IE = %#04x, want Timer 1 masked", got)
	}
	if got := GetReg16(INTERRUPT_IF); got != irqTimer1 {
		t.Errorf("INTERRUPT_IF = %#04x, want Timer 1 acknowledgement", got)
	}
	for _, addr := range []uintptr{TM0CNT_H, TM1CNT_H, DMA1CNT + 2, SOUNDCNT_X} {
		if got := GetReg16(addr); got != 0 {
			t.Errorf("register %#x = %#04x after stop, want 0", addr, got)
		}
	}
}
