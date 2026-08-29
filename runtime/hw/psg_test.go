//go:build !tinygo

package hw

import "testing"

func testEnvelope() HardwareEnvelope {
	return HardwareEnvelope{Initial: 12, Direction: EnvelopeDown, Pace: 3}
}

func TestTypedPSGControlsProgramExactRegisters(t *testing.T) {
	ResetForTest()
	ConfigurePSGOutput(7, 7, PSGMix100)
	pulse := PulseConfig{Duty: PulseDuty50, Length: 9, UseLength: true, Envelope: testEnvelope(), Route: SoundRouteBoth}
	if !TriggerPulse1(1750, Pulse1Config{Pulse: pulse, Sweep: HardwareSweep{Pace: 2, Direction: SweepUp, Shift: 1}}) {
		t.Fatal("CH1 trigger rejected")
	}
	if got, want := GetReg16(SOUND1CNT_L), uint16(0x21); got != want {
		t.Errorf("SOUND1CNT_L = %#04x, want %#04x", got, want)
	}
	if got, want := GetReg16(SOUND1CNT_H), uint16(0xC389); got != want {
		t.Errorf("SOUND1CNT_H = %#04x, want %#04x", got, want)
	}
	if got, want := GetReg16(SOUND1CNT_X), uint16(1750|1<<14|1<<15); got != want {
		t.Errorf("SOUND1CNT_X = %#04x, want %#04x", got, want)
	}

	pulse.Duty = PulseDuty25
	pulse.Route = SoundRouteLeft
	if !TriggerPulse2(1610, pulse) {
		t.Fatal("CH2 trigger rejected")
	}
	if got, want := GetReg16(SOUND2CNT_L), uint16(0xC349); got != want {
		t.Errorf("SOUND2CNT_L = %#04x, want %#04x", got, want)
	}

	noise := NoiseConfig{Width: Noise7Bit, Divisor: 3, Shift: 5, Length: 7, Envelope: testEnvelope(), Route: SoundRouteRight}
	if !TriggerNoise(noise) {
		t.Fatal("CH4 trigger rejected")
	}
	if got, want := GetReg16(SOUND4CNT_H), uint16(0x805B); got != want {
		t.Errorf("SOUND4CNT_H = %#04x, want %#04x", got, want)
	}
	if got := ManagedPSGSnapshot().ManagedActive; got != 0x0B {
		t.Errorf("managed channels = %#02x, want CH1+CH2+CH4", got)
	}
}

func TestNoiseAcceptsEntireFourBitClockShift(t *testing.T) {
	ResetForTest()
	noise := NoiseConfig{Width: Noise15Bit, Divisor: 7, Shift: 15, Envelope: testEnvelope(), Route: SoundRouteBoth}
	if !TriggerNoise(noise) {
		t.Fatal("CH4 rejected a valid four-bit clock shift")
	}
	if got, want := GetReg16(SOUND4CNT_H), uint16(0x80f7); got != want {
		t.Fatalf("SOUND4CNT_H = %#04x, want %#04x", got, want)
	}
}

func TestSetNoisePolynomialPreservesLengthWithoutRetrigger(t *testing.T) {
	ResetForTest()
	config := NoiseConfig{
		Width: Noise7Bit, Divisor: 3, Shift: 5, Length: 12, UseLength: true,
		Envelope: testEnvelope(), Route: SoundRouteBoth,
	}
	if !TriggerNoise(config) || !SetNoisePolynomial(4, 1, Noise7Bit) {
		t.Fatal("noise trigger/polynomial update failed")
	}
	if got := GetReg16(SOUND4CNT_H); got != 0x401c {
		t.Fatalf("updated SOUND4CNT_H = %#04x, want length enabled and no trigger", got)
	}
	if SetNoisePolynomial(8, 1, Noise7Bit) || SetNoisePolynomial(1, 16, Noise15Bit) {
		t.Fatal("invalid polynomial update accepted")
	}
}

func TestFirstPSGTriggerEnablesMasterBeforeHardwareRegisterWrites(t *testing.T) {
	tests := []struct {
		name    string
		trigger func() bool
	}{
		{
			name: "CH1",
			trigger: func() bool {
				return TriggerPulse1(1750, Pulse1Config{
					Pulse: PulseConfig{Duty: PulseDuty50, Envelope: testEnvelope(), Route: SoundRouteBoth},
				})
			},
		},
		{
			name: "CH2",
			trigger: func() bool {
				return TriggerPulse2(1750, PulseConfig{Duty: PulseDuty50, Envelope: testEnvelope(), Route: SoundRouteBoth})
			},
		},
		{
			name: "CH3",
			trigger: func() bool {
				var wave Waveform
				wave.Length = 32
				return TriggerWave(1750, WaveConfig{Wave: wave, Level: Wave100, Route: SoundRouteBoth})
			},
		},
		{
			name: "CH4",
			trigger: func() bool {
				return TriggerNoise(NoiseConfig{Width: Noise15Bit, Divisor: 3, Shift: 4, Envelope: testEnvelope(), Route: SoundRouteBoth})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetForTest()
			if !ConfigurePSGOutput(7, 7, PSGMix100) {
				t.Fatal("PSG output configuration rejected")
			}
			if !tt.trigger() {
				t.Fatal("PSG trigger rejected")
			}

			masterEnabledAt := -1
			firstConfiguredPSGWrite := -1
			limit := min(hostWriteN, len(hostWrites))
			for i := 0; i < limit; i++ {
				write := hostWrites[i]
				if masterEnabledAt < 0 && write.Address == SOUNDCNT_X && write.Value&soundMasterEnable != 0 {
					masterEnabledAt = i
				}
				if firstConfiguredPSGWrite < 0 && write.Address >= SOUND1CNT_L && write.Address <= SOUNDCNT_L && write.Value != 0 {
					firstConfiguredPSGWrite = i
				}
			}
			if masterEnabledAt < 0 || firstConfiguredPSGWrite < 0 {
				t.Fatalf("register trace lacks master/configured PSG writes: master=%d PSG=%d", masterEnabledAt, firstConfiguredPSGWrite)
			}
			if masterEnabledAt >= firstConfiguredPSGWrite {
				t.Fatalf("SOUNDCNT_X master enabled at write %d after PSG configuration began at %d", masterEnabledAt, firstConfiguredPSGWrite)
			}
		})
	}
}

func TestWaveControlPacksBanksAndLevels(t *testing.T) {
	ResetForTest()
	var waveform Waveform
	waveform.Length = 64
	for i := range waveform.Samples {
		waveform.Samples[i] = uint8(i & 15)
	}
	config := WaveConfig{Wave: waveform, Level: Wave75, StartBank: 1, Length: 5, UseLength: true, Route: SoundRouteBoth}
	if !TriggerWave(1899, config) {
		t.Fatal("CH3 trigger rejected")
	}
	if got, want := GetReg16(SOUND3CNT_L), uint16(1<<5|1<<6|1<<7); got != want {
		t.Errorf("SOUND3CNT_L = %#04x, want %#04x", got, want)
	}
	if got, want := GetReg16(SOUND3CNT_H), uint16(1<<15|5); got != want {
		t.Errorf("SOUND3CNT_H = %#04x, want %#04x", got, want)
	}
	// The final CPU-visible bank is bank 0 because playback starts on bank 1.
	if got, want := GetReg32(WAVE_RAM0), uint32(0x67452301); got != want {
		t.Errorf("WAVE_RAM0 = %#08x, want packed sample order %#08x", got, want)
	}
}

func TestWaveControlLoadsSelectedBankIn32SampleMode(t *testing.T) {
	ResetForTest()
	var waveform Waveform
	waveform.Length = 32
	for i := 0; i < 32; i++ {
		waveform.Samples[i] = uint8(15 - i&15)
	}
	config := WaveConfig{Wave: waveform, Level: Wave100, StartBank: 1, Route: SoundRouteBoth}
	if !TriggerWave(1800, config) {
		t.Fatal("32-sample CH3 trigger rejected")
	}
	if got, want := GetReg32(WAVE_RAM0), uint32(0x98BADCFE); got != want {
		t.Errorf("selected bank data = %#08x, want %#08x", got, want)
	}
}

func TestPSGStopPreservesDirectSoundAndOtherChannels(t *testing.T) {
	ResetForTest()
	ConfigurePSGOutput(7, 7, PSGMix100)
	ConfigureDirectSound(DirectSoundA, SoundRouteBoth, DirectSound100, DirectSoundTimer0)
	pulse := PulseConfig{Duty: PulseDuty50, Envelope: testEnvelope(), Route: SoundRouteBoth}
	TriggerPulse1(1750, Pulse1Config{Pulse: pulse})
	TriggerPulse2(1750, pulse)
	StopPSGChannel(1)

	if got := GetReg16(SOUND2CNT_H); got == 0 {
		t.Error("stopping CH1 stopped CH2")
	}
	if got := GetReg16(SOUNDCNT_H); got != uint16(PSGMix100)|soundAFIFOVolume|soundARight|soundALeft {
		t.Errorf("stopping CH1 changed Direct Sound: %#04x", got)
	}
	StopPSGChannel(2)
	if got := GetReg16(SOUNDCNT_X); got != soundMasterEnable {
		t.Errorf("last PSG stop disabled FIFO A: %#04x", got)
	}
}

func TestPSGControlsRejectInvalidWithoutWrites(t *testing.T) {
	ResetForTest()
	SetReg16(SOUND1CNT_X, 0x1234)
	bad := PulseConfig{Duty: PulseDuty(9), Envelope: testEnvelope(), Route: SoundRouteBoth}
	if TriggerPulse1(1750, Pulse1Config{Pulse: bad}) || TriggerPulse2(2048, PulseConfig{}) ||
		TriggerNoise(NoiseConfig{Shift: 16}) {
		t.Fatal("invalid PSG controls were accepted")
	}
	if got := GetReg16(SOUND1CNT_X); got != 0x1234 {
		t.Fatalf("rejected trigger wrote SOUND1CNT_X: %#04x", got)
	}
}

func TestPSGAcceptedHardwareRangesEncodeEveryDiscreteControl(t *testing.T) {
	ResetForTest()
	envelope := testEnvelope()

	for duty := uint8(0); duty <= 3; duty++ {
		for route := uint8(0); route <= 3; route++ {
			config := PulseConfig{Duty: PulseDuty(duty), Length: 63, UseLength: true, Envelope: envelope, Route: SoundRoute(route)}
			if !TriggerPulse2(1750, config) {
				t.Fatalf("CH2 rejected duty=%d route=%d", duty, route)
			}
			if got, want := GetReg16(SOUND2CNT_L)&0x00FF, uint16(duty)<<6|63; got != want {
				t.Fatalf("CH2 duty/length duty=%d route=%d = %#04x, want %#04x", duty, route, got, want)
			}
			bit := uint16(1) << 1
			wantRoute := uint16(0)
			if route == uint8(SoundRouteRight) || route == uint8(SoundRouteBoth) {
				wantRoute |= bit << 8
			}
			if route == uint8(SoundRouteLeft) || route == uint8(SoundRouteBoth) {
				wantRoute |= bit << 12
			}
			if got := GetReg16(SOUNDCNT_L) & (bit<<8 | bit<<12); got != wantRoute {
				t.Fatalf("CH2 route %d = %#04x, want %#04x", route, got, wantRoute)
			}
		}
	}

	for pace := uint8(0); pace <= 7; pace++ {
		for direction := uint8(0); direction <= 1; direction++ {
			for shift := uint8(0); shift <= 7; shift++ {
				config := Pulse1Config{
					Pulse: PulseConfig{Duty: PulseDuty50, Envelope: envelope, Route: SoundRouteBoth},
					Sweep: HardwareSweep{Pace: pace, Direction: SweepDirection(direction), Shift: shift},
				}
				if !TriggerPulse1(1750, config) {
					t.Fatalf("CH1 rejected sweep pace=%d direction=%d shift=%d", pace, direction, shift)
				}
				want := uint16(pace)<<4 | uint16(shift)
				if direction == uint8(SweepDown) {
					want |= 1 << 3
				}
				if got := GetReg16(SOUND1CNT_L); got != want {
					t.Fatalf("CH1 sweep pace=%d direction=%d shift=%d = %#04x, want %#04x", pace, direction, shift, got, want)
				}
			}
		}
	}

	for initial := uint8(0); initial <= 15; initial++ {
		for pace := uint8(0); pace <= 7; pace++ {
			for direction := uint8(0); direction <= 1; direction++ {
				config := PulseConfig{
					Duty:     PulseDuty50,
					Envelope: HardwareEnvelope{Initial: initial, Pace: pace, Direction: EnvelopeDirection(direction)},
					Route:    SoundRouteBoth,
				}
				if !TriggerPulse2(1750, config) {
					t.Fatalf("CH2 rejected envelope initial=%d pace=%d direction=%d", initial, pace, direction)
				}
				want := uint16(initial)<<12 | uint16(pace)<<8
				if direction == uint8(EnvelopeUp) {
					want |= 1 << 11
				}
				if got := GetReg16(SOUND2CNT_L) & 0xFF00; got != want {
					t.Fatalf("envelope initial=%d pace=%d direction=%d = %#04x, want %#04x", initial, pace, direction, got, want)
				}
			}
		}
	}

	var wave Waveform
	wave.Length = 32
	for bank := uint8(0); bank <= 1; bank++ {
		for level := uint8(0); level <= uint8(Wave75); level++ {
			config := WaveConfig{Wave: wave, Level: WaveLevel(level), StartBank: bank, Route: SoundRouteBoth}
			if !TriggerWave(1750, config) {
				t.Fatalf("CH3 rejected bank=%d level=%d", bank, level)
			}
			want := uint16(level) << 13
			if WaveLevel(level) == Wave75 {
				want = 1 << 15
			}
			if got := GetReg16(SOUND3CNT_H) & 0xE000; got != want {
				t.Fatalf("CH3 level=%d = %#04x, want %#04x", level, got, want)
			}
			if got := GetReg16(SOUND3CNT_L) & (1 << 6); got != uint16(bank)<<6 {
				t.Fatalf("CH3 bank=%d control = %#04x", bank, got)
			}
		}
	}

	for width := uint8(0); width <= 1; width++ {
		for divisor := uint8(0); divisor <= 7; divisor++ {
			for shift := uint8(0); shift <= 15; shift++ {
				config := NoiseConfig{
					Width: NoiseWidth(width), Divisor: divisor, Shift: shift,
					Envelope: envelope, Route: SoundRouteBoth,
				}
				if !TriggerNoise(config) {
					t.Fatalf("CH4 rejected width=%d divisor=%d shift=%d", width, divisor, shift)
				}
				want := uint16(divisor) | uint16(shift)<<4
				if width == uint8(Noise7Bit) {
					want |= 1 << 3
				}
				if got := GetReg16(SOUND4CNT_H) & 0x00FF; got != want {
					t.Fatalf("CH4 width=%d divisor=%d shift=%d = %#04x, want %#04x", width, divisor, shift, got, want)
				}
			}
		}
	}
}
