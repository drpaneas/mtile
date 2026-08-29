// Package gbapu contains Mtile's pure-Go, AGB-mode implementation of the four
// inherited Game Boy PSG channels. Hardware behavior and timing were adapted
// from TotalJustice/gb_apu, Copyright (c) 2024 TotalJustice, under the MIT
// License. See docs/third-party/gb-apu.md and gb-apu-LICENSE.txt.
package gbapu

import (
	runtimeaudio "github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

// The PSG inherited by the GBA is clocked from the 16.78 MHz system clock.
// Its frame sequencer advances at 512 Hz.
const (
	agbClockRate       = hw.GBAClockHz
	frameSequenceClock = agbClockRate / 512
	// Four PSG channels at envelope 15 and terminal level 8 must fit together.
	// This keeps one isolated audition close to the editor's established level
	// while retaining exact four-channel headroom.
	psgMixMaximum = 15 * 8 * 4
)

var dutyCycles = [4][8]bool{
	{false, false, false, false, false, false, false, true},
	{true, false, false, false, false, false, false, true},
	{true, false, false, false, false, true, true, true},
	{false, true, true, true, true, true, true, false},
}

var waveMultipliers = [5]int{0, 4, 2, 1, 3}
var noiseDivisors = [8]int{8, 16, 32, 48, 64, 80, 96, 112}

type channelState struct {
	active     bool
	note       uint8
	sound      runtimeaudio.Sound
	period     int
	timer      int
	phase      int
	lfsr       uint16
	amp        [2]int
	length     int
	envelope   int
	envTimer   int
	envStopped bool

	// monitorGain is a host-preview-only gain. The cartridge-facing PSG
	// behavior still uses the hardware envelope above; live MIDI releases use
	// this short ramp so a key-up cannot cut a non-zero square/wave sample into
	// silence and produce an avoidable speaker click.
	monitorGain   int
	releaseFrames int
	releaseTotal  int

	sweepShadow int
	sweepTimer  int
	sweepActive bool

	macroStep     int
	macroTimer    int
	macroActive   bool
	macroBaseNote uint8
	noiseDivisor  uint8
	noiseShift    uint8
}

// Renderer is an AGB-only implementation of the four inherited Game Boy PSG
// channels. It emits timestamped hardware edges through blip_buf's sinc kernel
// and intentionally contains no Direct Sound/FIFO implementation.
type Renderer struct {
	sampleRate int
	left       *blipBuffer
	right      *blipBuffer
	channels   [4]channelState
	frameTimer int
	frameStep  uint8
	leftLevel  int
	rightLevel int
	psgShift   int
	silence    int
}

func NewRenderer(sampleRate int) *Renderer {
	if sampleRate < 8_000 {
		sampleRate = 48_000
	}
	r := &Renderer{
		sampleRate: sampleRate,
		left:       newBlipBuffer(max(64, sampleRate/10), float64(agbClockRate), float64(sampleRate)),
		right:      newBlipBuffer(max(64, sampleRate/10), float64(agbClockRate), float64(sampleRate)),
		frameTimer: int(frameSequenceClock),
	}
	r.SetOutput(7, 7, 100)
	return r
}

func (r *Renderer) SetOutput(leftLevel, rightLevel, psgRatio int) {
	if r == nil {
		return
	}
	r.leftLevel = min(max(leftLevel, 0), 7)
	r.rightLevel = min(max(rightLevel, 0), 7)
	switch psgRatio {
	case 25:
		r.psgShift = 2
	case 50:
		r.psgShift = 1
	default:
		r.psgShift = 0
	}
	for channel := range r.channels {
		r.updateAmplitude(channel, 0)
	}
}

func (r *Renderer) Start(channel, note uint8, sound runtimeaudio.Sound) bool {
	if r == nil || channel < 1 || channel > 4 || int(sound.Kind) != int(channel-1) ||
		!runtimeaudio.NativeSoundNoteValid(sound, note) {
		return false
	}
	index := int(channel - 1)
	state := &r.channels[index]
	previousAmp := state.amp
	previousPhase := state.phase
	*state = channelState{
		active:        true,
		note:          note,
		sound:         sound,
		lfsr:          0,
		envelope:      r.initialEnvelope(sound),
		envTimer:      r.envelopePace(sound),
		monitorGain:   256,
		sweepShadow:   0,
		macroActive:   len(sound.Macro.Steps) != 0,
		macroTimer:    int(runtimeaudio.NativeMacroClockCycles),
		macroBaseNote: note,
		noiseDivisor:  sound.Noise.Divisor,
		noiseShift:    sound.Noise.Shift,
		amp:           previousAmp,
	}
	// Triggering CH1/CH2 does not reset the duty sequencer on hardware. This
	// also avoids inventing a phase discontinuity for every MIDI note.
	if sound.Kind == runtimeaudio.SoundCH1 || sound.Kind == runtimeaudio.SoundCH2 {
		state.phase = previousPhase & 7
	} else if sound.Kind == runtimeaudio.SoundCH3 && sound.Wave.Wave.Length == 64 {
		state.phase = int(sound.Wave.StartBank&1) * 32
	}
	state.length = hardwareLengthCounter(sound)
	if sound.Kind != runtimeaudio.SoundCH4 {
		period, ok := runtimeaudio.NativeSoundPeriod(sound, note)
		if !ok {
			state.active = false
			r.updateAmplitude(index, 0)
			return false
		}
		state.period = int(period)
	}
	if state.macroActive && !r.applyMacro(state) {
		state.active = false
		r.updateAmplitude(index, 0)
		return false
	}
	state.timer = oscillatorPeriod(state)
	if sound.Kind == runtimeaudio.SoundCH3 {
		// AGB wave playback fetches the first nibble after the trigger delay.
		state.timer += 24
	}
	if sound.Kind == runtimeaudio.SoundCH1 {
		sweep := sound.Pulse1.Sweep
		state.sweepShadow = state.period
		state.sweepTimer = int(sweep.Pace)
		if state.sweepTimer == 0 {
			state.sweepTimer = 8
		}
		state.sweepActive = sweep.Pace != 0 || sweep.Shift != 0
		if sweep.Shift != 0 && !r.sweepCalculate(state, false) {
			state.active = false
		}
	}
	if !dacEnabled(sound) {
		state.active = false
	}
	r.updateAmplitude(index, 0)
	return state.active
}

func (r *Renderer) Stop(channel uint8) {
	if r == nil || channel < 1 || channel > 4 {
		return
	}
	index := int(channel - 1)
	r.channels[index].active = false
	r.updateAmplitude(index, 0)
}

// Release fades a live host-monitor voice over frames output samples. This is
// deliberately separate from Stop: Stop retains exact hardware semantics for
// cartridge rendering and deterministic PSG comparisons, while a physical
// MIDI key release gets a tiny de-click ramp in the editor only.
func (r *Renderer) Release(channel uint8, frames int) {
	if r == nil || channel < 1 || channel > 4 {
		return
	}
	if frames <= 0 {
		r.Stop(channel)
		return
	}
	state := &r.channels[channel-1]
	if !state.active {
		return
	}
	state.releaseFrames = frames
	state.releaseTotal = frames
}

func (r *Renderer) StopAll() {
	if r == nil {
		return
	}
	for channel := range r.channels {
		r.channels[channel].active = false
		r.updateAmplitude(channel, 0)
	}
}

func (r *Renderer) Active(channel uint8) bool {
	return r != nil && channel >= 1 && channel <= 4 && r.channels[channel-1].active
}

// Render fills equal-length stereo host buffers without allocating.
func (r *Renderer) Render(left, right []int16) int {
	if r == nil {
		return 0
	}
	count := min(len(left), len(right))
	for index := 0; index < count; index++ {
		r.clockMonitorReleases()
		clocks := r.left.clocksNeeded(1)
		r.advance(clocks)
		r.left.endFrame(clocks)
		r.right.endFrame(clocks)
		left[index] = r.left.readSample()
		right[index] = r.right.readSample()
		if r.anyActive() {
			r.silence = 0
		} else {
			r.silence++
			// blip_buf's 16-tap reconstruction has finished after 64 host
			// frames. Clear the remaining DC integrator tail so silence stays
			// mathematically silent and cannot become a background MIDI tick.
			if r.silence == 64 {
				r.left.clear()
				r.right.clear()
				left[index], right[index] = 0, 0
			}
		}
	}
	return count
}

func (r *Renderer) clockMonitorReleases() {
	for channel := range r.channels {
		state := &r.channels[channel]
		if !state.active || state.releaseFrames <= 0 || state.releaseTotal <= 0 {
			continue
		}
		state.releaseFrames--
		state.monitorGain = state.releaseFrames * 256 / state.releaseTotal
		if state.releaseFrames == 0 {
			state.active = false
		}
		r.updateAmplitude(channel, 0)
	}
}

func (r *Renderer) anyActive() bool {
	if r == nil {
		return false
	}
	for channel := range r.channels {
		if r.channels[channel].active {
			return true
		}
	}
	return false
}

func (r *Renderer) advance(clocks int) {
	remaining := clocks
	at := 0
	for remaining > 0 {
		step := min(remaining, r.frameTimer)
		for channel := range r.channels {
			state := &r.channels[channel]
			if !state.active {
				continue
			}
			step = min(step, state.timer)
			if state.macroActive {
				step = min(step, state.macroTimer)
			}
		}
		if step < 0 {
			step = 0
		}
		remaining -= step
		at += step
		r.frameTimer -= step
		for channel := range r.channels {
			state := &r.channels[channel]
			if !state.active {
				continue
			}
			state.timer -= step
			if state.macroActive {
				state.macroTimer -= step
			}
		}

		handled := false
		for channel := range r.channels {
			state := &r.channels[channel]
			if !state.active {
				continue
			}
			if state.timer == 0 {
				r.clockOscillator(channel)
				state.timer = oscillatorPeriod(state)
				r.updateAmplitude(channel, at)
				handled = true
			}
			if state.macroActive && state.macroTimer == 0 {
				if !r.applyMacro(state) {
					state.active = false
				}
				state.macroTimer = int(runtimeaudio.NativeMacroClockCycles)
				r.updateAmplitude(channel, at)
				handled = true
			}
		}
		if r.frameTimer == 0 {
			r.clockFrameSequencer(at)
			r.frameTimer = int(frameSequenceClock)
			handled = true
		}
		if step == 0 && !handled {
			break
		}
	}
}

func (r *Renderer) clockOscillator(channel int) {
	state := &r.channels[channel]
	switch state.sound.Kind {
	case runtimeaudio.SoundCH1, runtimeaudio.SoundCH2:
		state.phase = (state.phase + 1) & 7
	case runtimeaudio.SoundCH3:
		length := max(1, int(state.sound.Wave.Wave.Length))
		state.phase = (state.phase + 1) % length
	case runtimeaudio.SoundCH4:
		result := (((state.lfsr >> 1) ^ state.lfsr) ^ 1) & 1
		state.lfsr >>= 1
		bits := uint16(0x4000)
		if state.sound.Noise.Width == hw.Noise7Bit {
			bits = 0x4040
		}
		state.lfsr &^= bits
		if result != 0 {
			state.lfsr |= bits
		}
	}
}

func (r *Renderer) clockFrameSequencer(at int) {
	switch r.frameStep {
	case 0, 4:
		r.clockLengths(at)
	case 2, 6:
		r.clockLengths(at)
		r.clockSweep(at)
	case 7:
		r.clockEnvelopes(at)
	}
	r.frameStep = (r.frameStep + 1) & 7
}

func (r *Renderer) clockLengths(at int) {
	for channel := range r.channels {
		state := &r.channels[channel]
		if !state.active || !usesHardwareLength(state.sound) || state.length <= 0 {
			continue
		}
		state.length--
		if state.length == 0 {
			state.active = false
			r.updateAmplitude(channel, at)
		}
	}
}

func (r *Renderer) clockEnvelopes(at int) {
	for channel := range r.channels {
		state := &r.channels[channel]
		if !state.active || state.sound.Kind == runtimeaudio.SoundCH3 || state.envStopped {
			continue
		}
		pace := r.envelopePace(state.sound)
		if pace == 0 {
			continue
		}
		state.envTimer--
		if state.envTimer > 0 {
			continue
		}
		state.envTimer = pace
		next := state.envelope - 1
		if envelopeDirection(state.sound) == hw.EnvelopeUp {
			next = state.envelope + 1
		}
		if next < 0 || next > 15 {
			state.envStopped = true
			continue
		}
		state.envelope = next
		r.updateAmplitude(channel, at)
	}
}

func (r *Renderer) clockSweep(at int) {
	state := &r.channels[0]
	if !state.active || !state.sweepActive {
		return
	}
	state.sweepTimer--
	if state.sweepTimer > 0 {
		return
	}
	pace := int(state.sound.Pulse1.Sweep.Pace)
	state.sweepTimer = pace
	if state.sweepTimer == 0 {
		state.sweepTimer = 8
		return
	}
	if !r.sweepCalculate(state, true) || !r.sweepCalculate(state, false) {
		state.active = false
		r.updateAmplitude(0, at)
	}
}

func (r *Renderer) sweepCalculate(state *channelState, update bool) bool {
	if state == nil {
		return false
	}
	sweep := state.sound.Pulse1.Sweep
	delta := state.sweepShadow >> sweep.Shift
	next := state.sweepShadow + delta
	if sweep.Direction == hw.SweepDown {
		next = state.sweepShadow - delta
	}
	if next < 0 || next > 2047 {
		return false
	}
	if sweep.Shift != 0 && update {
		state.sweepShadow = next
		state.period = next
	}
	return true
}

func (r *Renderer) applyMacro(state *channelState) bool {
	if state == nil || !state.macroActive {
		return true
	}
	if state.macroStep >= len(state.sound.Macro.Steps) {
		state.macroActive = false
		return true
	}
	step := state.sound.Macro.Steps[state.macroStep]
	if step.PeriodDelta != 0 {
		state.period += int(step.PeriodDelta)
		if state.period < 0 || state.period > 2047 {
			state.macroActive = false
			return false
		}
	}
	if step.HasNoise {
		state.noiseDivisor = step.NoiseDivisor
		state.noiseShift = step.NoiseShift
	}
	if step.HasNoiseNote {
		divisor, shift, ok := runtimeaudio.NativeGBStudioNoisePolynomial(state.macroBaseNote, step.NoiseNoteOffset)
		if !ok {
			state.macroActive = false
			return false
		}
		state.noiseDivisor = divisor
		state.noiseShift = shift
	}
	state.macroStep++
	if state.macroStep >= len(state.sound.Macro.Steps) {
		if state.sound.Macro.LoopStart < 0 {
			state.macroActive = false
		} else {
			state.macroStep = int(state.sound.Macro.LoopStart)
		}
	}
	return true
}

func (r *Renderer) updateAmplitude(channel, clockTime int) {
	if r == nil || channel < 0 || channel >= len(r.channels) {
		return
	}
	state := &r.channels[channel]
	raw := r.rawAmplitude(state)
	route := soundRoute(state.sound)
	left, right := 0, 0
	if route == hw.SoundRouteLeft || route == hw.SoundRouteBoth {
		left = raw * (r.leftLevel + 1) >> r.psgShift
	}
	if route == hw.SoundRouteRight || route == hw.SoundRouteBoth {
		right = raw * (r.rightLevel + 1) >> r.psgShift
	}
	left = left * 32767 / psgMixMaximum
	right = right * 32767 / psgMixMaximum
	leftDelta := left - state.amp[0]
	rightDelta := right - state.amp[1]
	if state.sound.Kind == runtimeaudio.SoundCH4 {
		r.left.addDeltaFast(clockTime, leftDelta)
		r.right.addDeltaFast(clockTime, rightDelta)
	} else {
		r.left.addDelta(clockTime, leftDelta)
		r.right.addDelta(clockTime, rightDelta)
	}
	state.amp[0], state.amp[1] = left, right
}

func (r *Renderer) rawAmplitude(state *channelState) int {
	if state == nil || !state.active {
		return 0
	}
	raw := 0
	switch state.sound.Kind {
	case runtimeaudio.SoundCH1:
		if dutyCycles[state.sound.Pulse1.Pulse.Duty][state.phase] {
			raw = -state.envelope
		} else {
			raw = state.envelope
		}
	case runtimeaudio.SoundCH2:
		if dutyCycles[state.sound.Pulse2.Duty][state.phase] {
			raw = -state.envelope
		} else {
			raw = state.envelope
		}
	case runtimeaudio.SoundCH3:
		wave := state.sound.Wave.Wave
		if wave.Length == 0 || state.sound.Wave.Level > hw.Wave75 {
			return 0
		}
		sample := int(wave.Samples[state.phase%int(wave.Length)] ^ 0x0f)
		raw = ((sample*2 - 15) * waveMultipliers[state.sound.Wave.Level]) >> 2
	case runtimeaudio.SoundCH4:
		if state.lfsr&1 != 0 {
			raw = -state.envelope
		} else {
			raw = state.envelope
		}
	default:
		return 0
	}
	return raw * state.monitorGain / 256
}

func oscillatorPeriod(state *channelState) int {
	if state == nil {
		return 1
	}
	switch state.sound.Kind {
	case runtimeaudio.SoundCH1, runtimeaudio.SoundCH2:
		return max(1, (2048-state.period)*16)
	case runtimeaudio.SoundCH3:
		return max(1, (2048-state.period)*8)
	case runtimeaudio.SoundCH4:
		if state.noiseShift >= 14 {
			// On AGB the polynomial timer does not clock for NR43 shifts 14
			// and 15; the channel holds its current DAC value.
			return int(^uint(0) >> 2)
		}
		divisor := noiseDivisors[state.noiseDivisor&7]
		return max(1, (divisor<<state.noiseShift)*4)
	default:
		return 1
	}
}

func hardwareLengthCounter(sound runtimeaudio.Sound) int {
	switch sound.Kind {
	case runtimeaudio.SoundCH1:
		return 64 - int(sound.Pulse1.Pulse.Length)
	case runtimeaudio.SoundCH2:
		return 64 - int(sound.Pulse2.Length)
	case runtimeaudio.SoundCH3:
		return 256 - int(sound.Wave.Length)
	case runtimeaudio.SoundCH4:
		return 64 - int(sound.Noise.Length)
	default:
		return 0
	}
}

func usesHardwareLength(sound runtimeaudio.Sound) bool {
	switch sound.Kind {
	case runtimeaudio.SoundCH1:
		return sound.Pulse1.Pulse.UseLength
	case runtimeaudio.SoundCH2:
		return sound.Pulse2.UseLength
	case runtimeaudio.SoundCH3:
		return sound.Wave.UseLength
	case runtimeaudio.SoundCH4:
		return sound.Noise.UseLength
	default:
		return false
	}
}

func (r *Renderer) initialEnvelope(sound runtimeaudio.Sound) int {
	switch sound.Kind {
	case runtimeaudio.SoundCH1:
		return int(sound.Pulse1.Pulse.Envelope.Initial)
	case runtimeaudio.SoundCH2:
		return int(sound.Pulse2.Envelope.Initial)
	case runtimeaudio.SoundCH4:
		return int(sound.Noise.Envelope.Initial)
	default:
		return 0
	}
}

func (r *Renderer) envelopePace(sound runtimeaudio.Sound) int {
	switch sound.Kind {
	case runtimeaudio.SoundCH1:
		return int(sound.Pulse1.Pulse.Envelope.Pace)
	case runtimeaudio.SoundCH2:
		return int(sound.Pulse2.Envelope.Pace)
	case runtimeaudio.SoundCH4:
		return int(sound.Noise.Envelope.Pace)
	default:
		return 0
	}
}

func envelopeDirection(sound runtimeaudio.Sound) hw.EnvelopeDirection {
	switch sound.Kind {
	case runtimeaudio.SoundCH1:
		return sound.Pulse1.Pulse.Envelope.Direction
	case runtimeaudio.SoundCH2:
		return sound.Pulse2.Envelope.Direction
	case runtimeaudio.SoundCH4:
		return sound.Noise.Envelope.Direction
	default:
		return hw.EnvelopeDown
	}
}

func dacEnabled(sound runtimeaudio.Sound) bool {
	if sound.Kind == runtimeaudio.SoundCH3 {
		return sound.Wave.Wave.Length != 0
	}
	return rEnvelopeDAC(sound)
}

func rEnvelopeDAC(sound runtimeaudio.Sound) bool {
	switch sound.Kind {
	case runtimeaudio.SoundCH1:
		envelope := sound.Pulse1.Pulse.Envelope
		return envelope.Initial != 0 || envelope.Direction == hw.EnvelopeUp
	case runtimeaudio.SoundCH2:
		envelope := sound.Pulse2.Envelope
		return envelope.Initial != 0 || envelope.Direction == hw.EnvelopeUp
	case runtimeaudio.SoundCH4:
		envelope := sound.Noise.Envelope
		return envelope.Initial != 0 || envelope.Direction == hw.EnvelopeUp
	default:
		return false
	}
}

func soundRoute(sound runtimeaudio.Sound) hw.SoundRoute {
	switch sound.Kind {
	case runtimeaudio.SoundCH1:
		return sound.Pulse1.Pulse.Route
	case runtimeaudio.SoundCH2:
		return sound.Pulse2.Route
	case runtimeaudio.SoundCH3:
		return sound.Wave.Route
	case runtimeaudio.SoundCH4:
		return sound.Noise.Route
	default:
		return hw.SoundRouteNone
	}
}
