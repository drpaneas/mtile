package hw

// PulseDuty is one of the four duty cycles implemented by CH1 and CH2.
type PulseDuty uint8

const (
	PulseDuty12_5 PulseDuty = iota
	PulseDuty25
	PulseDuty50
	PulseDuty75
)

// EnvelopeDirection is the direction of a CH1, CH2, or CH4 hardware envelope.
type EnvelopeDirection uint8

const (
	EnvelopeDown EnvelopeDirection = iota
	EnvelopeUp
)

// HardwareEnvelope maps directly to the GBA initial-volume envelope fields.
// Pace zero disables automatic stepping.
type HardwareEnvelope struct {
	Initial   uint8
	Direction EnvelopeDirection
	Pace      uint8
}

func (e HardwareEnvelope) Valid() bool {
	return e.Initial <= 15 && e.Direction <= EnvelopeUp && e.Pace <= 7
}

func (e HardwareEnvelope) bits() uint16 {
	value := uint16(e.Initial)<<12 | uint16(e.Pace)<<8
	if e.Direction == EnvelopeUp {
		value |= 1 << 11
	}
	return value
}

// SweepDirection is CH1's real hardware frequency-sweep direction.
type SweepDirection uint8

const (
	SweepUp SweepDirection = iota
	SweepDown
)

// HardwareSweep maps directly to CH1 sweep pace, direction, and shift.
type HardwareSweep struct {
	Pace      uint8
	Direction SweepDirection
	Shift     uint8
}

func (s HardwareSweep) Valid() bool {
	return s.Pace <= 7 && s.Direction <= SweepDown && s.Shift <= 7
}

func (s HardwareSweep) bits() uint16 {
	value := uint16(s.Pace)<<4 | uint16(s.Shift)
	if s.Direction == SweepDown {
		value |= 1 << 3
	}
	return value
}

// PulseConfig contains controls shared by CH1 and CH2. Length is the raw
// 0..63 hardware length value; UseLength selects the hardware length counter.
type PulseConfig struct {
	Duty      PulseDuty
	Length    uint8
	UseLength bool
	Envelope  HardwareEnvelope
	Route     SoundRoute
}

func (p PulseConfig) Valid() bool {
	return p.Duty <= PulseDuty75 && p.Length <= 63 && p.Envelope.Valid() && p.Route <= SoundRouteBoth
}

func (p PulseConfig) control() uint16 {
	return uint16(p.Length) | uint16(p.Duty)<<6 | p.Envelope.bits()
}

// Pulse1Config adds CH1's hardware sweep.
type Pulse1Config struct {
	Pulse PulseConfig
	Sweep HardwareSweep
}

// WaveLevel is CH3's hardware output level.
type WaveLevel uint8

const (
	WaveMute WaveLevel = iota
	Wave100
	Wave50
	Wave25
	Wave75
)

// Waveform contains one 32- or 64-sample, 4-bit CH3 waveform.
type Waveform struct {
	Length  uint8
	Samples [64]uint8
}

func (w Waveform) Valid() bool {
	if w.Length != 32 && w.Length != 64 {
		return false
	}
	for i := uint8(0); i < w.Length; i++ {
		if w.Samples[i] > 15 {
			return false
		}
	}
	return true
}

// WaveConfig contains CH3's bank, level, length, route, and waveform state.
type WaveConfig struct {
	Wave      Waveform
	Level     WaveLevel
	StartBank uint8
	Length    uint8
	UseLength bool
	Route     SoundRoute
}

func (w WaveConfig) Valid() bool {
	return w.Wave.Valid() && w.Level <= Wave75 && w.StartBank <= 1 && w.Route <= SoundRouteBoth
}

// NoiseWidth selects CH4's 15-bit or 7-bit LFSR mode.
type NoiseWidth uint8

const (
	Noise15Bit NoiseWidth = iota
	Noise7Bit
)

// NoiseConfig maps to CH4's polynomial counter and envelope controls.
type NoiseConfig struct {
	Width     NoiseWidth
	Divisor   uint8
	Shift     uint8
	Length    uint8
	UseLength bool
	Envelope  HardwareEnvelope
	Route     SoundRoute
}

func (n NoiseConfig) Valid() bool {
	return n.Width <= Noise7Bit && n.Divisor <= 7 && n.Shift <= 15 && n.Length <= 63 &&
		n.Envelope.Valid() && n.Route <= SoundRouteBoth
}

var managedPSGChannels uint8

func validTonePeriod(period uint16) bool { return period <= 2047 }

func triggerControl(period uint16, useLength bool) uint16 {
	value := period | 1<<15
	if useLength {
		value |= 1 << 14
	}
	return value
}

func markPSGChannel(channel uint8, active bool) {
	bit := uint8(1) << (channel - 1)
	if active {
		managedPSGChannels |= bit
	} else {
		managedPSGChannels &^= bit
	}
	SetPSGActive(managedPSGChannels != 0)
}

// TriggerPulse1 configures and explicitly retriggers CH1.
func TriggerPulse1(period uint16, config Pulse1Config) bool {
	if !validTonePeriod(period) || !config.Pulse.Valid() || !config.Sweep.Valid() {
		return false
	}
	initializeSoundRegisters()
	// Real GBA hardware blocks and clears the PSG register window while the
	// SOUNDCNT_X master bit is off. Claim the channel first so every routing,
	// envelope, frequency, and trigger write below reaches the sound circuit.
	markPSGChannel(1, true)
	RoutePSGChannel(1, config.Pulse.Route)
	SetReg16(SOUND1CNT_L, config.Sweep.bits())
	SetReg16(SOUND1CNT_H, config.Pulse.control())
	SetReg16(SOUND1CNT_X, triggerControl(period, config.Pulse.UseLength))
	return true
}

// SetPulse1Period performs a legato frequency update without retriggering.
func SetPulse1Period(period uint16, useLength bool) bool {
	if !validTonePeriod(period) {
		return false
	}
	value := period
	if useLength {
		value |= 1 << 14
	}
	SetReg16(SOUND1CNT_X, value)
	return true
}

// TriggerPulse2 configures and explicitly retriggers CH2.
func TriggerPulse2(period uint16, config PulseConfig) bool {
	if !validTonePeriod(period) || !config.Valid() {
		return false
	}
	initializeSoundRegisters()
	markPSGChannel(2, true)
	RoutePSGChannel(2, config.Route)
	SetReg16(SOUND2CNT_L, config.control())
	SetReg16(SOUND2CNT_H, triggerControl(period, config.UseLength))
	return true
}

// SetPulse2Period performs a legato frequency update without retriggering.
func SetPulse2Period(period uint16, useLength bool) bool {
	if !validTonePeriod(period) {
		return false
	}
	value := period
	if useLength {
		value |= 1 << 14
	}
	SetReg16(SOUND2CNT_H, value)
	return true
}

func packWaveBank(samples *[64]uint8, offset int) [4]uint32 {
	var words [4]uint32
	for word := 0; word < 4; word++ {
		var value uint32
		for pair := 0; pair < 4; pair++ {
			i := offset + word*8 + pair*2
			packed := samples[i]<<4 | samples[i+1]
			value |= uint32(packed) << (pair * 8)
		}
		words[word] = value
	}
	return words
}

func writeManagedWaveBank(bank uint8, words [4]uint32) {
	// The selected playback bank is not CPU-visible. Select the opposite bank
	// while disabled, write all 32 samples, then configure the final bank.
	selectedPlaybackBank := uint16(1 - bank)
	SetReg16(SOUND3CNT_L, selectedPlaybackBank<<6)
	SetReg32(WAVE_RAM0, words[0])
	SetReg32(WAVE_RAM1, words[1])
	SetReg32(WAVE_RAM2, words[2])
	SetReg32(WAVE_RAM3, words[3])
}

func waveLevelBits(level WaveLevel) uint16 {
	if level == Wave75 {
		return 1 << 15
	}
	return uint16(level) << 13
}

// TriggerWave safely loads CH3 Wave RAM and explicitly retriggers the channel.
func TriggerWave(period uint16, config WaveConfig) bool {
	if !validTonePeriod(period) || !config.Valid() {
		return false
	}
	initializeSoundRegisters()
	markPSGChannel(3, true)
	if config.Wave.Length == 32 {
		writeManagedWaveBank(config.StartBank, packWaveBank(&config.Wave.Samples, 0))
	} else {
		writeManagedWaveBank(0, packWaveBank(&config.Wave.Samples, 0))
		writeManagedWaveBank(1, packWaveBank(&config.Wave.Samples, 32))
	}
	control := uint16(config.StartBank)<<6 | 1<<7
	if config.Wave.Length == 64 {
		control |= 1 << 5
	}
	RoutePSGChannel(3, config.Route)
	SetReg16(SOUND3CNT_L, control)
	SetReg16(SOUND3CNT_H, uint16(config.Length)|waveLevelBits(config.Level))
	SetReg16(SOUND3CNT_X, triggerControl(period, config.UseLength))
	return true
}

// SetWavePeriod performs a legato CH3 frequency update without retriggering.
func SetWavePeriod(period uint16, useLength bool) bool {
	if !validTonePeriod(period) {
		return false
	}
	value := period
	if useLength {
		value |= 1 << 14
	}
	SetReg16(SOUND3CNT_X, value)
	return true
}

// TriggerNoise configures and explicitly retriggers CH4.
func TriggerNoise(config NoiseConfig) bool {
	if !config.Valid() {
		return false
	}
	initializeSoundRegisters()
	markPSGChannel(4, true)
	RoutePSGChannel(4, config.Route)
	SetReg16(SOUND4CNT_L, uint16(config.Length)|config.Envelope.bits())
	polynomial := uint16(config.Divisor) | uint16(config.Shift)<<4
	if config.Width == Noise7Bit {
		polynomial |= 1 << 3
	}
	SetReg16(SOUND4CNT_H, polynomial|triggerControl(0, config.UseLength))
	return true
}

// SetNoisePolynomial updates CH4's clock/divisor without retriggering its
// envelope or length counter. hUGEDriver instrument subpatterns use this to
// shape percussion over successive audio ticks.
func SetNoisePolynomial(divisor, shift uint8, width NoiseWidth) bool {
	if divisor > 7 || shift > 15 || width > Noise7Bit {
		return false
	}
	polynomial := uint16(divisor) | uint16(shift)<<4
	if width == Noise7Bit {
		polynomial |= 1 << 3
	}
	// NR44's length-enable bit shares this 16-bit GBA register with NR43.
	// Preserve it, while deliberately leaving the write-only trigger bit clear.
	polynomial |= GetReg16(SOUND4CNT_H) & (1 << 14)
	SetReg16(SOUND4CNT_H, polynomial)
	return true
}

// StopPSGChannel disables exactly one channel's DAC and preserves all shared
// routing and Direct Sound controls. channel is 1 through 4.
func StopPSGChannel(channel uint8) bool {
	if channel < 1 || channel > 4 {
		return false
	}
	switch channel {
	case 1:
		SetReg16(SOUND1CNT_H, 0)
		SetReg16(SOUND1CNT_X, 0)
	case 2:
		SetReg16(SOUND2CNT_L, 0)
		SetReg16(SOUND2CNT_H, 0)
	case 3:
		SetReg16(SOUND3CNT_L, 0)
		SetReg16(SOUND3CNT_H, 0)
		SetReg16(SOUND3CNT_X, 0)
	case 4:
		SetReg16(SOUND4CNT_L, 0)
		SetReg16(SOUND4CNT_H, 0)
	}
	markPSGChannel(channel, false)
	return true
}

// StopAllPSG disables all four fixed hardware voices without stopping PCM.
func StopAllPSG() {
	for channel := uint8(1); channel <= 4; channel++ {
		StopPSGChannel(channel)
	}
}

// PSGSnapshot is register-equivalent state for preview and telemetry.
type PSGSnapshot struct {
	ManagedActive  uint8
	HardwareActive uint8
	Channel1       [3]uint16
	Channel2       [2]uint16
	Channel3       [3]uint16
	Channel4       [2]uint16
}

func ManagedPSGSnapshot() PSGSnapshot {
	return PSGSnapshot{
		ManagedActive:  managedPSGChannels,
		HardwareActive: uint8(GetReg16(SOUNDCNT_X) & 0x000F),
		Channel1:       [3]uint16{GetReg16(SOUND1CNT_L), GetReg16(SOUND1CNT_H), GetReg16(SOUND1CNT_X)},
		Channel2:       [2]uint16{GetReg16(SOUND2CNT_L), GetReg16(SOUND2CNT_H)},
		Channel3:       [3]uint16{GetReg16(SOUND3CNT_L), GetReg16(SOUND3CNT_H), GetReg16(SOUND3CNT_X)},
		Channel4:       [2]uint16{GetReg16(SOUND4CNT_L), GetReg16(SOUND4CNT_H)},
	}
}

func resetManagedPSGForTest() { managedPSGChannels = 0 }
