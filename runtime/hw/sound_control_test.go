//go:build !tinygo

package hw

import "testing"

func TestManagedSoundControlPreservesPSGAndBothFIFOs(t *testing.T) {
	ResetForTest()
	if !ConfigurePSGOutput(7, 6, PSGMix100) || !RoutePSGChannel(1, SoundRouteBoth) ||
		!RoutePSGChannel(3, SoundRouteLeft) {
		t.Fatal("valid PSG setup was rejected")
	}
	SetPSGActive(true)
	if !ConfigureDirectSound(DirectSoundA, SoundRouteLeft, DirectSound100, DirectSoundTimer0) ||
		!ConfigureDirectSound(DirectSoundB, SoundRouteRight, DirectSound50, DirectSoundTimer1) {
		t.Fatal("valid FIFO setup was rejected")
	}

	wantL := uint16(7<<4 | 6 | 1<<8 | 1<<12 | 1<<14)
	wantH := uint16(PSGMix100) | soundAFIFOVolume | soundALeft | soundBRight | 1<<14
	if got := GetReg16(SOUNDCNT_L); got != wantL {
		t.Fatalf("SOUNDCNT_L = %#04x, want %#04x", got, wantL)
	}
	if got := GetReg16(SOUNDCNT_H); got != wantH {
		t.Fatalf("SOUNDCNT_H = %#04x, want %#04x", got, wantH)
	}
	if got := GetReg16(SOUNDCNT_X); got != soundMasterEnable {
		t.Fatalf("sound master = %#04x, want enabled", got)
	}

	DisableDirectSound(DirectSoundA)
	wantH &^= fifoAFieldMask
	if got := GetReg16(SOUNDCNT_H); got != wantH {
		t.Errorf("stopping FIFO A changed unrelated fields: got %#04x want %#04x", got, wantH)
	}
	SetPSGActive(false)
	if got := GetReg16(SOUNDCNT_X); got != soundMasterEnable {
		t.Error("stopping PSG disabled active FIFO B")
	}
	DisableDirectSound(DirectSoundB)
	if got := GetReg16(SOUNDCNT_X); got != 0 {
		t.Errorf("last user release left sound master at %#04x", got)
	}
}

func TestManagedSoundControlRejectsInvalidWithoutMutation(t *testing.T) {
	ResetForTest()
	ConfigurePSGOutput(7, 7, PSGMix100)
	RoutePSGChannel(2, SoundRouteBoth)
	SetPSGActive(true)
	want := ManagedSoundControlSnapshot()

	if ConfigurePSGOutput(8, 7, PSGMix100) || RoutePSGChannel(0, SoundRouteBoth) ||
		ConfigureDirectSound(DirectSoundBus(9), SoundRouteBoth, DirectSound100, DirectSoundTimer0) {
		t.Fatal("invalid managed sound configuration was accepted")
	}
	if got := ManagedSoundControlSnapshot(); got != want {
		t.Fatalf("rejected operation mutated state: got %+v want %+v", got, want)
	}
}

func TestDirectSoundStartStopKeepsActivePSG(t *testing.T) {
	ResetForTest()
	ConfigurePSGOutput(7, 7, PSGMix100)
	RoutePSGChannel(1, SoundRouteBoth)
	SetPSGActive(true)
	page := testStreamPage(0)
	clock, _ := DirectSoundClockForRate(DirectSoundRate16384)
	if !StartFIFOAStreamBlock(page[:], testStreamBlockSamples, clock) {
		t.Fatal("stream start failed")
	}
	StopFIFOAStream()

	if got := GetReg16(SOUNDCNT_L); got != 0x1177 {
		t.Errorf("FIFO lifecycle changed PSG control: %#04x", got)
	}
	if got := GetReg16(SOUNDCNT_H); got != uint16(PSGMix100) {
		t.Errorf("FIFO lifecycle changed PSG ratio: %#04x", got)
	}
	if got := GetReg16(SOUNDCNT_X); got != soundMasterEnable {
		t.Errorf("FIFO stop disabled PSG master: %#04x", got)
	}
}
