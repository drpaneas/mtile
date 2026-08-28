// Package audio contains the target-independent, allocation-free audio core
// shared by GBA playback and editor preview.
package audio

import _ "unsafe" // required for //go:section

const (
	// MaxVoices bounds all mixer state at compile time. Projects select one of
	// the measured limits below; arbitrary counts are rejected by Init.
	MaxVoices = 12

	MaxVolume uint16 = 256
	MinPan    int16  = -256
	MaxPan    int16  = 256
	// SFXMixerVoices reserves one short release tail for each of PICO-8's
	// four logical channels. Sample cartridges retain two PCM voices on top
	// of that transient eight-synth peak.
	SFXMixerVoices       = 8
	SFXSampleMixerVoices = 12
	SFXEdgeSamples       = 48

	musicGainUnity uint32 = 1 << 16
	// PICO-8 linearly opens and closes an SFX voice over 64 samples at
	// 22,050 Hz. Forty-eight 16,384 Hz samples are the nearest target-clock
	// equivalent. Compatible note cells update one oscillator in place;
	// release/attack overlap is reserved for starts, stops, fade-in, and
	// incompatible waveform or filter replacements.
	sfxEdgeSamples uint8  = SFXEdgeSamples
	sfxEdgeStep    uint32 = musicGainUnity / uint32(sfxEdgeSamples)
)

// SourceKind is the tagged-union discriminator for Voice source state.
type SourceKind uint8

const (
	SourceNone SourceKind = iota
	SourceSquare
	SourceTriangle
	SourceSaw
	SourceNoise
	SourcePulse
	SourceWavetable
	SourcePCM
	// SourceTiltedSaw is PICO-8 instrument 1: an asymmetric triangle with a
	// seven-eighths rise and one-eighth fall.
	SourceTiltedSaw
	// SourceOrgan is PICO-8 instrument 5: two unequal triangle lobes per
	// period, equivalent to mixed fundamental and octave triangles.
	SourceOrgan
	// SourcePhaser is PICO-8 instrument 7: two weighted triangle oscillators
	// with a small frequency offset.
	SourcePhaser
	// SourceCustom0..SourceCustom7 are sequencer-only tags for PICO-8 SFX
	// instruments. Mixer.Start rejects them; SFXPlayer resolves each tag to
	// SFX 0..7 and starts the resulting built-in oscillator.
	SourceCustom0
	SourceCustom1
	SourceCustom2
	SourceCustom3
	SourceCustom4
	SourceCustom5
	SourceCustom6
	SourceCustom7
)

// WavetableSamples is deliberately small and fixed for predictable target
// reads. Richer authored tables can be converted to this target form.
const WavetableSamples = 32

// Voice contains every source variant in one fixed record. Oscillators use
// Phase as a full-cycle 0..2^32 accumulator. PCM uses Position plus Fraction
// and interprets Step as unsigned 16.16 source samples per output sample.
type Voice struct {
	Active    bool
	Protected bool
	Music     bool
	Kind      SourceKind
	Priority  uint8
	Filter    SFXFilter
	Detune    uint8
	// filterChannel is zero for ordinary mixer voices and 1..SFXChannels for
	// voices owned by SFXPlayer's fixed stateful-filter channels.
	filterChannel uint8

	Phase  uint32
	Step   uint32
	Phase2 uint32
	Step2  uint32
	Phase3 uint32
	Step3  uint32
	Phase4 uint32
	Step4  uint32

	Volume uint16
	Pan    int16

	Started uint32
	Noise   uint16
	// PICO noise is a pitch-dependent one-pole random process. NoiseValue is
	// Q8 sample state; NoiseAlpha and NoiseGain are Q8 coefficients computed
	// once when the voice starts so the sample loop remains division-free.
	NoiseValue int32
	NoiseAlpha uint16
	NoiseGain  uint16
	Fraction   uint16
	Position   uint32

	// A sample-continuous volume ramp uses Q16 volume so long, quiet fades do
	// not stall between integer 0..256 mixer levels. rampSamples fits every
	// SFX step: 255 exact PICO speed units round to fewer than 35,000 target
	// samples, so every accepted one-step ramp fits uint16.
	rampVolume  uint32
	rampDelta   int32
	rampSamples uint16
	attackGain  uint32
	attackLeft  uint8
	releaseLeft uint8

	Loop      bool
	LoopStart uint32
	LoopEnd   uint32

	Wavetable *[WavetableSamples]int8
	PCM       []int8
}

// Request describes one voice start without interfaces, closures, or maps.
// Volume is 0..256. Pan is -256 (hard left) through 0 (center) to 256
// (hard right). MixMono deliberately ignores pan; MixStereo applies it.
type Request struct {
	Kind      SourceKind
	Priority  uint8
	Protected bool
	Music     bool
	Filter    SFXFilter

	Step   uint32
	Volume uint16
	Pan    int16

	NoiseSeed uint16
	Wavetable *[WavetableSamples]int8
	PCM       []int8

	Loop      bool
	LoopStart uint32
	LoopEnd   uint32

	filterChannel uint8
}

// Stats is a bounded telemetry snapshot. FailedStarts includes malformed
// requests and requests that cannot steal an eligible voice.
type Stats struct {
	ActiveVoices     uint8
	PeakActiveVoices uint8
	VoiceSteals      uint32
	FailedStarts     uint32
}

// Mixer owns a compile-time fixed voice pool. Init selects how many leading
// entries are active for this project profile.
type Mixer struct {
	// Active voices are packed at the front so the per-sample loop can walk
	// contiguous state. IDs stay stable through the two small lookup tables.
	voices    [MaxVoices]Voice
	voiceIDs  [MaxVoices]uint8
	positions [MaxVoices]uint8
	activeIDs uint16

	voiceLimit   uint8
	active       uint8
	peak         uint8
	ramps        uint8
	attacks      uint8
	musicVoices  uint8
	filterVoices uint8
	started      uint32
	steals       uint32
	failed       uint32

	// Music gain is separate from per-voice note volume so a music() fade can
	// span note and pattern boundaries without attenuating manually triggered
	// SFX. The quotient/remainder clock reaches either endpoint exactly without
	// division in the sample loop, even for the longest accepted fade.
	musicGain          uint32
	musicGainTarget    uint32
	musicGainStep      int32
	musicGainRemainder uint32
	musicGainError     uint32
	musicGainSamples   uint32
	musicGainTotal     uint32
	musicGainDirection int8

	// Stateful filter buffers live with SFXPlayer in EWRAM. The mixer keeps a
	// non-owning pointer so its IWRAM voice pool remains small. Checkpoint
	// copies deliberately retain the destination pointer.
	filterStates *[SFXChannels]sfxFilterState
}

// copyVoiceState avoids the target compiler's byte-at-a-time memmove for a
// Voice assignment. Explicit typed fields preserve pointer/write-barrier
// semantics on the host while allowing ARM to copy aligned state as words.
//
//go:noinline
func copyVoiceState(dst, src *Voice) {
	dst.Active = src.Active
	dst.Protected = src.Protected
	dst.Music = src.Music
	dst.Kind = src.Kind
	dst.Priority = src.Priority
	dst.Filter = src.Filter
	dst.Detune = src.Detune
	dst.filterChannel = src.filterChannel
	dst.Phase = src.Phase
	dst.Step = src.Step
	dst.Phase2 = src.Phase2
	dst.Step2 = src.Step2
	dst.Phase3 = src.Phase3
	dst.Step3 = src.Step3
	dst.Phase4 = src.Phase4
	dst.Step4 = src.Step4
	dst.Volume = src.Volume
	dst.Pan = src.Pan
	dst.Started = src.Started
	dst.Noise = src.Noise
	dst.NoiseValue = src.NoiseValue
	dst.NoiseAlpha = src.NoiseAlpha
	dst.NoiseGain = src.NoiseGain
	dst.Fraction = src.Fraction
	dst.Position = src.Position
	dst.rampVolume = src.rampVolume
	dst.rampDelta = src.rampDelta
	dst.rampSamples = src.rampSamples
	dst.attackGain = src.attackGain
	dst.attackLeft = src.attackLeft
	dst.releaseLeft = src.releaseLeft
	dst.Loop = src.Loop
	dst.LoopStart = src.LoopStart
	dst.LoopEnd = src.LoopEnd
	dst.Wavetable = src.Wavetable
	dst.PCM = src.PCM
}

// SupportedVoiceCount reports the candidate profiles required by the audio
// benchmark plan.
func SupportedVoiceCount(count int) bool {
	return count == 4 || count == 6 || count == 8 || count == 12
}

// Init clears all state and selects a named voice profile.
func (m *Mixer) Init(voiceCount int) bool {
	if m == nil {
		return false
	}
	*m = Mixer{}
	if !SupportedVoiceCount(voiceCount) {
		return false
	}
	m.voiceLimit = uint8(voiceCount)
	m.musicGain = musicGainUnity
	m.musicGainTarget = musicGainUnity
	return true
}

// copyStateTo snapshots only active voices within the selected profile. Mixer
// reserves twelve slots and sample cartridges select all twelve, but inactive slots
// have no producer state and every lookup is bounded by activeIDs. Clearing a
// destination's formerly active tail prevents stale slice pointers from being
// retained after a restore with fewer voices.
func (m *Mixer) copyStateTo(dst *Mixer) bool {
	if m == nil || dst == nil || !SupportedVoiceCount(int(m.voiceLimit)) {
		return false
	}
	destinationActive := int(dst.active)
	for index := 0; index < int(m.active); index++ {
		copyVoiceState(&dst.voices[index], &m.voices[index])
		dst.voiceIDs[index] = m.voiceIDs[index]
		dst.positions[m.voiceIDs[index]] = uint8(index)
	}
	for index := int(m.active); index < destinationActive; index++ {
		dst.voices[index] = Voice{}
		dst.voiceIDs[index] = 0
	}
	dst.activeIDs = m.activeIDs
	dst.voiceLimit = m.voiceLimit
	dst.active = m.active
	dst.peak = m.peak
	dst.ramps = m.ramps
	dst.attacks = m.attacks
	dst.musicVoices = m.musicVoices
	dst.filterVoices = m.filterVoices
	dst.started = m.started
	dst.steals = m.steals
	dst.failed = m.failed
	dst.musicGain = m.musicGain
	dst.musicGainTarget = m.musicGainTarget
	dst.musicGainStep = m.musicGainStep
	dst.musicGainRemainder = m.musicGainRemainder
	dst.musicGainError = m.musicGainError
	dst.musicGainSamples = m.musicGainSamples
	dst.musicGainTotal = m.musicGainTotal
	dst.musicGainDirection = m.musicGainDirection
	return true
}

func (m *Mixer) attachSFXFilters(states *[SFXChannels]sfxFilterState) bool {
	if m == nil || m.active != 0 {
		return false
	}
	m.filterStates = states
	m.filterVoices = 0
	return true
}

func validKind(kind SourceKind) bool {
	return kind >= SourceSquare && kind <= SourcePhaser
}

func modulatableSynthKind(kind SourceKind) bool {
	return kind >= SourceSquare && kind <= SourcePulse || kind >= SourceTiltedSaw && kind <= SourcePhaser
}

func triangleSample(phase uint32) int16 {
	position := int32(phase >> 24)
	if position < 128 {
		return int16(-127 + position*2)
	}
	return int16(127 - (position-128)*2)
}

// PICO BUZZ on triangle is a fixed weighted triangle/tilted-saw waveform.
// Quantizing by the accumulator's high byte is already the oscillator's
// native resolution, so this ROM table replaces two piecewise calculations
// and divisions per oscillator without changing a sample.
var triangleBuzzSamples = [...]int8{
	-127, -125, -123, -121, -120, -118, -116, -114, -112, -111, -109, -107, -105, -104, -102, -100,
	-98, -96, -95, -93, -91, -89, -88, -86, -84, -82, -80, -79, -77, -75, -73, -71,
	-70, -68, -66, -64, -63, -61, -59, -57, -55, -54, -52, -50, -48, -46, -45, -43,
	-41, -39, -38, -36, -34, -32, -30, -29, -27, -25, -23, -22, -20, -18, -16, -14,
	-13, -11, -9, -7, -5, -4, -2, 0, 1, 3, 4, 6, 8, 10, 12, 13,
	15, 17, 19, 21, 22, 24, 26, 28, 29, 31, 33, 35, 37, 38, 40, 42,
	44, 45, 47, 49, 51, 53, 54, 56, 58, 60, 62, 63, 65, 67, 69, 70,
	72, 74, 76, 78, 79, 81, 83, 85, 87, 88, 90, 92, 94, 95, 97, 99,
	99, 98, 97, 96, 94, 93, 92, 91, 90, 88, 87, 86, 85, 83, 82, 81,
	80, 79, 77, 76, 75, 74, 73, 71, 70, 69, 68, 66, 65, 64, 63, 62,
	60, 59, 58, 57, 55, 54, 53, 52, 51, 49, 48, 47, 46, 45, 43, 42,
	41, 40, 38, 37, 36, 35, 34, 32, 31, 30, 29, 28, 26, 25, 24, 23,
	21, 20, 19, 18, 17, 15, 14, 13, 12, 10, 9, 8, 7, 6, 4, 3,
	2, 1, 0, -1, -2, -3, -5, -6, -7, -8, -9, -11, -12, -13, -14, -16,
	-17, -20, -23, -27, -30, -34, -37, -41, -44, -48, -51, -55, -58, -62, -65, -69,
	-72, -76, -79, -83, -86, -90, -93, -97, -100, -104, -107, -111, -114, -118, -121, -125,
}

func triangleFilteredSample(phase uint32, buzz bool) int16 {
	if !buzz {
		return triangleSample(phase)
	}
	return int16(triangleBuzzSamples[phase>>24])
}

//go:noinline
//go:section .iwram
func triangleVoiceSample(v *Voice) int16 {
	buzz := v.Filter&SFXFilterBuzz != 0
	primary := triangleFilteredSample(v.Phase, buzz)
	if v.Detune == 0 {
		return primary
	}
	secondary := triangleFilteredSample(v.Phase3, buzz)
	v.Phase3 += v.Step3
	// Triangle never owns the phaser/saw fourth accumulator checked by the
	// generic detune helper. Keeping this source-specific combination here
	// removes those type/filter branches from every detuned sample.
	return int16((int32(primary)*10 + int32(secondary)*5) / 12)
}

func tiltedSawSample(phase uint32) int16 {
	position := int32(phase >> 24)
	if position < 224 {
		return int16(-127 + position*127/112)
	}
	return int16(127 - (position-224)*127/16)
}

func tiltedSawFilteredSample(phase uint32, buzz bool) int16 {
	if !buzz {
		return tiltedSawSample(phase)
	}
	position := int32(phase >> 24)
	if position < 250 {
		return int16(-127 + position*254/250)
	}
	return int16(127 - (position-250)*254/6)
}

func tiltedSawVoiceSample(v *Voice) int16 {
	buzz := v.Filter&SFXFilterBuzz != 0
	primary := tiltedSawFilteredSample(v.Phase, buzz)
	if v.Detune == 0 {
		return primary
	}
	return finishDetune(v, primary, tiltedSawFilteredSample(v.Phase3, buzz))
}

func sawFilteredSample(phase, halfPhase uint32, buzz bool) int16 {
	plain := int32(phase>>24) - 128
	if !buzz {
		return int16(plain)
	}
	// BUZZ attenuates the saw to 83% and adds a two-period square offset.
	// Phase2 advances at half frequency, so its middle half is exactly the
	// source interval selected by PICO's measured waveform.
	sample := plain * 83 / 100
	if halfPhase >= 0x40000000 && halfPhase < 0xc0000000 {
		sample -= 22
	}
	return int16(sample * 96 / 100)
}

func sawVoiceSample(v *Voice) int16 {
	buzz := v.Filter&SFXFilterBuzz != 0
	primary := sawFilteredSample(v.Phase, v.Phase2, buzz)
	if v.Detune == 0 {
		return primary
	}
	return finishDetune(v, primary, sawFilteredSample(v.Phase3, v.Phase4, buzz))
}

func squareFilteredSample(phase uint32, buzz bool) int16 {
	threshold := uint32(0x80000000)
	if buzz {
		threshold = 0x66666666 // 40% duty
	}
	if phase < threshold {
		return 127
	}
	return -127
}

func squareVoiceSample(v *Voice) int16 {
	buzz := v.Filter&SFXFilterBuzz != 0
	primary := squareFilteredSample(v.Phase, buzz)
	if v.Detune == 0 {
		return primary
	}
	return finishDetune(v, primary, squareFilteredSample(v.Phase3, buzz))
}

func pulseFilteredSample(phase uint32, buzz bool) int16 {
	threshold := uint32(0x50000000)
	if buzz {
		threshold = 0x38000000 // captured 7/32 duty
	}
	if phase < threshold {
		return 127
	}
	return -127
}

func pulseVoiceSample(v *Voice) int16 {
	buzz := v.Filter&SFXFilterBuzz != 0
	primary := pulseFilteredSample(v.Phase, buzz)
	if v.Detune == 0 {
		return primary
	}
	return finishDetune(v, primary, pulseFilteredSample(v.Phase3, buzz))
}

func organSample(phase uint32) int16 {
	position := int32(phase >> 24)
	switch {
	case position < 64:
		return int16(-85 + position*170/64)
	case position < 128:
		return int16(85 - (position-64)*170/64)
	case position < 192:
		return int16(-85 + (position-128)*113/64)
	default:
		return int16(28 - (position-192)*113/64)
	}
}

func organFilteredSample(phase uint32, buzz bool) int16 {
	plain := int32(organSample(phase))
	if !buzz {
		return int16(plain)
	}
	if phase < 0x80000000 {
		if plain <= -69 {
			return int16((plain + 14) * 93 / 100)
		}
		return int16((plain*2/5 - 11) * 93 / 100)
	}
	return int16((plain + 14) * 93 / 100)
}

func organVoiceSample(v *Voice) int16 {
	buzz := v.Filter&SFXFilterBuzz != 0
	primary := organFilteredSample(v.Phase, buzz)
	if v.Detune == 0 {
		return primary
	}
	secondary := organFilteredSample(v.Phase3, buzz)
	if v.Detune == 2 {
		secondary = triangleFilteredSample(v.Phase3, buzz)
		v.Phase3 += v.Step3
		// Organ DETUNE-2 keeps 15/16 of the organ and adds a 3/16
		// four-octave triangle. Its mix differs from the generic 5/6 + 5/12.
		return int16((int32(primary)*15 + int32(secondary)*3) / 16)
	}
	return finishDetune(v, primary, secondary)
}

func phaserPhaseStep(step uint32) uint32 {
	ratio := picoPhaserRatios[picoPitchKey(step)]
	return uint32((uint64(step)*uint64(ratio) + 1<<15) >> 16)
}

func phaserSample(phase, phase2 uint32) int16 {
	return int16((2*int32(triangleSample(phase)) + int32(triangleSample(phase2))) / 3)
}

func phaserFilteredSample(phase, phase2 uint32, buzz bool) int16 {
	plain := int32(phaserSample(phase, phase2))
	if !buzz {
		return int16(plain)
	}
	// BUZZ adds the even harmonics absent from the two-triangle phaser: an
	// inverted second harmonic at 1/12 source scale and a fourth at 1/24.
	sample := plain - int32(triangleSample(phase*2))/12 + int32(triangleSample(phase*4))/24
	return int16(sample * 88 / 100)
}

func phaserVoiceSample(v *Voice) int16 {
	buzz := v.Filter&SFXFilterBuzz != 0
	// Phaser already owns a half-amplitude second triangle. DETUNE retunes that
	// oscillator rather than adding an entire second phaser voice.
	if v.Detune != 0 {
		plain := int32((16*int32(triangleSample(v.Phase)) + 8*int32(triangleSample(v.Phase2))) / 24)
		if !buzz {
			return int16(plain)
		}
		sample := plain - int32(triangleSample(v.Phase*2))/12 + int32(triangleSample(v.Phase*4))/24
		return int16(sample * 88 / 100)
	}
	return phaserFilteredSample(v.Phase, v.Phase2, buzz)
}

func scaledPhaseStep(step uint32, numerator, denominator uint32) uint32 {
	return uint32((uint64(step)*uint64(numerator) + uint64(denominator)/2) / uint64(denominator))
}

func extendedPICOPhaseStep(key int) uint32 {
	if key >= 0 && key < len(picoPitchPhaseSteps) {
		return picoPitchPhaseSteps[key]
	}
	if key < 0 {
		octaves := (-key + 11) / 12
		base := key + 12*octaves
		step := picoPitchPhaseSteps[base]
		return (step + 1<<uint(octaves-1)) >> uint(octaves)
	}
	octaves := (key - (len(picoPitchPhaseSteps) - 1) + 11) / 12
	base := key - 12*octaves
	return uint32(uint64(picoPitchPhaseSteps[base]) << uint(octaves))
}

// transposedPICOPhaseStep applies a custom-instrument pitch relative to C2.
// PICO permits the sum to extend below and above the editor's 0..63 authored
// range; low custom notes in the corpus depend on those sub-range frequencies.
// Exact authored notes use octave-scaled table entries without runtime divide.
// Interpolated effect states retain the general ratio fallback.
func transposedPICOPhaseStep(step, transposition uint32) uint32 {
	stepKey := picoPitchKey(step)
	transpositionKey := picoPitchKey(transposition)
	if step == picoPitchPhaseSteps[stepKey] && transposition == picoPitchPhaseSteps[transpositionKey] {
		key := stepKey + transpositionKey - 24
		return extendedPICOPhaseStep(key)
	}
	return scaledPhaseStep(step, transposition, picoPitchPhaseSteps[24])
}

func detunePhaseStep(kind SourceKind, level uint8, step uint32) uint32 {
	if level == 0 || kind == SourceNoise {
		return 0
	}
	if kind == SourceTriangle {
		if level == 1 {
			// Measured PICO ratio at a one-second resolution: 393.9903 / 522.8531.
			return uint32((uint64(step)*49385 + 1<<15) >> 16)
		}
		return step + step/2
	}
	if kind == SourceOrgan {
		if level == 1 {
			return scaledPhaseStep(step, 199, 200)
		}
		return scaledPhaseStep(step, 199, 50)
	}
	if kind == SourcePhaser {
		if level == 1 {
			return uint32((uint64(step)*63976 + 1<<15) >> 16)
		}
		return uint32((uint64(step)*130017 + 1<<15) >> 16)
	}
	if level == 1 {
		return scaledPhaseStep(step, 199, 200)
	}
	return scaledPhaseStep(step, 199, 100)
}

func finishDetune(v *Voice, primary, secondary int16) int16 {
	v.Phase3 += v.Step3
	if v.Kind == SourcePhaser || v.Kind == SourceSaw && v.Filter&SFXFilterBuzz != 0 {
		v.Phase4 += v.Step4
	}
	// PICO scales the primary to 5/6 and the second oscillator to 5/12.
	return int16((int32(primary)*10 + int32(secondary)*5) / 12)
}

var picoPitchPhaseSteps = [...]uint32{
	17145893, 18165441, 19245614, 20390018, 21602472, 22887021, 24247954, 25689813,
	27217409, 28835840, 30550508, 32367136, 34291786, 36330882, 38491228, 40780036,
	43204943, 45774043, 48495909, 51379626, 54434817, 57671680, 61101017, 64734272,
	68583572, 72661764, 76982457, 81560072, 86409886, 91548086, 96991818, 102759252,
	108869635, 115343360, 122202033, 129468544, 137167144, 145323527, 153964914, 163120144,
	172819773, 183096171, 193983636, 205518503, 217739269, 230686720, 244404066, 258937088,
	274334289, 290647054, 307929828, 326240288, 345639545, 366192342, 387967272, 411037006,
	435478539, 461373440, 488808132, 517874176, 548668578, 581294109, 615859655, 652480576,
}

// The phaser's second triangle is not separated from the fundamental by one
// fixed ratio. These Q16 ratios are measured from one-second PICO-8 0.2.6b
// captures. The low notes retain the 109/110 fit because their sub-hertz
// separation cannot be resolved in one note and already passes isolated-note
// conformance; from pitch 24 onward both spectral peaks are independently
// visible. Scaling the live step preserves pitch effects and arbitrary input
// frequencies instead of replacing them with table-quantized absolute steps.
var picoPhaserRatios = [...]uint16{
	64940, 64940, 64940, 64940, 64940, 64940, 64940, 64940,
	64940, 64940, 64940, 64940, 64940, 64940, 64940, 64940,
	64940, 64940, 64940, 64940, 64940, 64940, 64940, 64940,
	64946, 64979, 65010, 64977, 64980, 64959, 64990, 64973,
	65005, 64988, 65014, 64994, 64988, 65018, 65011, 65005,
	65000, 64999, 65000, 65002, 65005, 65010, 65015, 65020,
	65009, 65018, 65010, 65022, 65017, 65015, 65014, 65016,
	65018, 65022, 65015, 65020, 65019, 65018, 65019, 65022,
}

// These Q8 coefficients are a monotonic fit of one-second PICO-8 0.2.6b
// instrument-6 captures at every numeric pitch. The filter table reproduces
// spectral color at the target's 16,384 Hz stream rate; the gain table keeps
// stationary RMS aligned after the pitch-dependent filter change.
var picoNoiseAlphas = [...]uint8{
	5, 5, 5, 5, 7, 8, 8, 8, 8, 8, 8, 8, 10, 10, 10, 10,
	10, 13, 14, 14, 14, 14, 17, 19, 20, 20, 23, 27, 29, 29, 30, 37,
	39, 39, 41, 48, 48, 54, 55, 60, 64, 67, 72, 76, 84, 84, 89, 94,
	101, 107, 113, 117, 117, 127, 129, 131, 139, 139, 148, 161, 167, 190, 215, 229,
}

var picoNoiseGains = [...]uint16{
	1830, 1830, 1830, 1629, 1501, 1360, 1360, 1360, 1360, 1360, 1360, 1360, 1249, 1249, 1249, 1234,
	1234, 1137, 1137, 1130, 1104, 1104, 1005, 959, 947, 935, 904, 841, 804, 804, 795, 739,
	739, 739, 731, 698, 698, 679, 679, 660, 660, 644, 644, 644, 644, 644, 644, 644,
	644, 644, 644, 644, 644, 644, 644, 644, 644, 644, 644, 642, 632, 592, 553, 534,
}

func picoPitchKey(step uint32) int {
	low, high := 0, len(picoPitchPhaseSteps)
	for low < high {
		middle := low + (high-low)/2
		if picoPitchPhaseSteps[middle] < step {
			low = middle + 1
		} else {
			high = middle
		}
	}
	key := low
	if key == len(picoPitchPhaseSteps) {
		key--
	} else if key > 0 && step-picoPitchPhaseSteps[key-1] < picoPitchPhaseSteps[key]-step {
		key--
	}
	return key
}

func picoNoiseParameters(step uint32) (alpha, gain uint16) {
	// Arbitrary runtime frequencies inherit the closest measured PICO key's
	// colored-noise parameters.
	key := picoPitchKey(step)
	return uint16(picoNoiseAlphas[key]), picoNoiseGains[key]
}

func picoNoizPhaseStep(step uint32) uint32 {
	// PICO-8 0.2.6b holds each NOIZ value for exactly 64-key samples at its
	// 22,050 Hz recorder clock. Convert that physical update rate to the target
	// 16,384 Hz Q32 phase clock. Key 63 updates faster than the target can
	// represent, so one fresh value per target sample is the correct bandlimit.
	hold := uint64(64 - picoPitchKey(step))
	phaseStep := (uint64(22050) << 32) / (uint64(MusicSampleRate) * hold)
	if phaseStep > uint64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(phaseStep)
}

func picoNoiseSample(v *Voice) int16 {
	if v.Filter&SFXFilterNoiz != 0 {
		// NOIZ is a pitch-clocked zero-order-hold random source. Captured PICO
		// output changes value every 64-key source samples (28 at key 36);
		// that clock and hold shape account for its sinc-like
		// spectrum despite the editor describing it as pure white noise.
		sample := int16(v.NoiseValue)
		previous := v.Phase2
		v.Phase2 += v.Step2
		if v.Phase2 < previous {
			v.Noise ^= v.Noise << 7
			v.Noise ^= v.Noise >> 9
			v.Noise ^= v.Noise << 8
			v.NoiseValue = (int32(v.Noise&0xff) - 128) / 2
		}
		return sample
	}
	v.Noise ^= v.Noise << 7
	v.Noise ^= v.Noise >> 9
	v.Noise ^= v.Noise << 8
	random := (int32(v.Noise&0xff) - 128) << 8
	v.NoiseValue += (random - v.NoiseValue) * int32(v.NoiseAlpha) >> 8
	return int16(v.NoiseValue * int32(v.NoiseGain) >> 16)
}

func validRequest(r Request) bool {
	if !validKind(r.Kind) || r.Step == 0 || r.Volume > MaxVolume || r.Pan < MinPan || r.Pan > MaxPan || !validSFXFilter(r.Filter) {
		return false
	}
	if r.Filter != 0 && (r.Kind == SourceWavetable || r.Kind == SourcePCM) {
		return false
	}
	stateful := statefulSFXFilter(r.Filter)
	if stateful != (r.filterChannel != 0) || r.filterChannel > SFXChannels {
		return false
	}
	switch r.Kind {
	case SourceWavetable:
		return r.Wavetable != nil && len(r.PCM) == 0 && !r.Loop
	case SourcePCM:
		if r.Wavetable != nil || len(r.PCM) == 0 || uint64(len(r.PCM)) > uint64(^uint32(0)) {
			return false
		}
		if !r.Loop {
			return true
		}
		length := uint32(len(r.PCM))
		if r.LoopStart >= r.LoopEnd || r.LoopEnd > length {
			return false
		}
		loopSamples := uint64(r.LoopEnd - r.LoopStart)
		return uint64(r.Step) < loopSamples<<16
	default:
		return r.Wavetable == nil && len(r.PCM) == 0 && !r.Loop
	}
}

func older(a, b uint32) bool {
	return int32(a-b) < 0
}

// Start uses the first inactive voice, then the lowest-priority eligible
// voice. A request may steal only an unprotected voice whose priority is not
// greater than the new request. Equal-priority ties select the oldest start.
func (m *Mixer) Start(r Request) (int, bool) {
	if m == nil || m.voiceLimit == 0 || !validRequest(r) || r.filterChannel != 0 && m.filterStates == nil {
		if m != nil {
			m.failed++
		}
		return -1, false
	}

	id := -1
	for i := 0; i < int(m.voiceLimit); i++ {
		if m.activeIDs&(uint16(1)<<i) == 0 {
			id = i
			break
		}
	}
	position := int(m.active)
	steal := false
	if id < 0 {
		position = -1
		for i := 0; i < int(m.active); i++ {
			v := &m.voices[i]
			if v.Protected || v.Priority > r.Priority {
				continue
			}
			if position < 0 || v.Priority < m.voices[position].Priority ||
				(v.Priority == m.voices[position].Priority && older(v.Started, m.voices[position].Started)) {
				position = i
			}
		}
		if position < 0 {
			m.failed++
			return -1, false
		}
		id = int(m.voiceIDs[position])
		steal = true
	}

	m.started++
	if m.started == 0 {
		m.started = 1
	}
	noise := r.NoiseSeed
	if noise == 0 {
		noise = 1
	}
	v := Voice{
		Active:        true,
		Protected:     r.Protected,
		Music:         r.Music,
		Kind:          r.Kind,
		Priority:      r.Priority,
		Filter:        r.Filter,
		Detune:        r.Filter.Detune(),
		filterChannel: r.filterChannel,
		Step:          r.Step,
		Volume:        r.Volume,
		Pan:           r.Pan,
		Started:       m.started,
		Noise:         noise,
		Loop:          r.Loop,
		LoopStart:     r.LoopStart,
		LoopEnd:       r.LoopEnd,
		Wavetable:     r.Wavetable,
		PCM:           r.PCM,
	}
	if r.Kind == SourceNoise {
		if r.Filter&SFXFilterNoiz != 0 {
			v.Step2 = picoNoizPhaseStep(r.Step)
		} else {
			v.NoiseAlpha, v.NoiseGain = picoNoiseParameters(r.Step)
		}
	} else if r.Kind == SourcePhaser {
		if v.Detune != 0 {
			v.Step2 = detunePhaseStep(r.Kind, v.Detune, r.Step)
		} else {
			v.Step2 = phaserPhaseStep(r.Step)
		}
	} else if r.Kind == SourceSaw && r.Filter&SFXFilterBuzz != 0 {
		v.Step2 = r.Step / 2
	}
	if detune := r.Filter.Detune(); detune != 0 && r.Kind != SourceNoise && r.Kind != SourcePhaser {
		v.Step3 = detunePhaseStep(r.Kind, detune, r.Step)
		if r.Kind == SourceSaw && r.Filter&SFXFilterBuzz != 0 {
			v.Step4 = v.Step3 / 2
		}
	}
	if r.Kind == SourcePCM && !r.Loop {
		v.LoopEnd = uint32(len(r.PCM))
	}
	if steal {
		if m.voices[position].rampSamples != 0 {
			m.ramps--
		}
		if m.voices[position].attackLeft != 0 {
			m.attacks--
		}
		if m.voices[position].releaseLeft != 0 {
			m.attacks--
		}
		if m.voices[position].Music {
			m.musicVoices--
		}
		if m.voices[position].filterChannel != 0 {
			m.filterVoices--
		}
	}
	m.voices[position] = v
	if r.Music {
		m.musicVoices++
	}
	if r.filterChannel != 0 {
		m.filterVoices++
	}
	if steal {
		m.steals++
	} else {
		m.voiceIDs[position] = uint8(id)
		m.positions[id] = uint8(position)
		m.activeIDs |= uint16(1) << id
		m.active++
		if m.active > m.peak {
			m.peak = m.active
		}
	}
	return id, true
}

func (m *Mixer) hasFilterWork() bool {
	if m == nil || m.filterStates == nil {
		return false
	}
	if m.filterVoices != 0 {
		return true
	}
	for index := range m.filterStates {
		if m.filterStates[index].hasWork() {
			return true
		}
	}
	return false
}

func (m *Mixer) filterState(v *Voice) *sfxFilterState {
	if m == nil || m.filterStates == nil || v == nil || v.filterChannel == 0 || v.filterChannel > SFXChannels {
		return nil
	}
	return &m.filterStates[v.filterChannel-1]
}

func (m *Mixer) removeActiveAt(position int) {
	last := int(m.active) - 1
	id := int(m.voiceIDs[position])
	if m.voices[position].rampSamples != 0 {
		m.ramps--
	}
	if m.voices[position].attackLeft != 0 {
		m.attacks--
	}
	if m.voices[position].releaseLeft != 0 {
		m.attacks--
	}
	if m.voices[position].Music {
		m.musicVoices--
	}
	if m.voices[position].filterChannel != 0 {
		m.filterVoices--
	}
	if position != last {
		m.voices[position] = m.voices[last]
		movedID := m.voiceIDs[last]
		m.voiceIDs[position] = movedID
		m.positions[movedID] = uint8(position)
	}
	m.voices[last] = Voice{}
	m.voiceIDs[last] = 0
	m.positions[id] = 0
	m.activeIDs &^= uint16(1) << id
	m.active--
}

// Stop releases one stable voice ID. Packing may move another voice's state,
// but its ID and Voice lookup remain unchanged.
func (m *Mixer) Stop(id int) bool {
	if m == nil || id < 0 || id >= int(m.voiceLimit) || m.activeIDs&(uint16(1)<<id) == 0 {
		return false
	}
	m.removeActiveAt(int(m.positions[id]))
	return true
}

func (m *Mixer) StopAll() {
	if m == nil {
		return
	}
	for i := 0; i < int(m.voiceLimit); i++ {
		m.voices[i] = Voice{}
		m.voiceIDs[i] = 0
		m.positions[i] = 0
	}
	m.active = 0
	m.activeIDs = 0
	m.ramps = 0
	m.attacks = 0
	m.musicVoices = 0
	m.filterVoices = 0
	if m.filterStates != nil {
		for index := range m.filterStates {
			m.filterStates[index].configure(0, false, 0)
		}
	}
	m.SetMusicVolume(MaxVolume)
}

// Voice returns a copy for editor/debug inspection by stable voice ID.
func (m *Mixer) Voice(id int) (Voice, bool) {
	if m == nil || id < 0 || id >= int(m.voiceLimit) {
		return Voice{}, false
	}
	if m.activeIDs&(uint16(1)<<id) == 0 {
		return Voice{}, true
	}
	return m.voices[m.positions[id]], true
}

// SetVoiceStepVolume updates one owned synth voice without restarting its
// oscillator phase. started is the ownership generation returned by Voice;
// a stolen/reused ID therefore cannot be modulated by its previous owner.
func (m *Mixer) SetVoiceStepVolume(id int, started uint32, step uint32, volume uint16) bool {
	if m == nil || id < 0 || id >= int(m.voiceLimit) || step == 0 || volume > MaxVolume ||
		m.activeIDs&(uint16(1)<<id) == 0 {
		return false
	}
	voice := &m.voices[m.positions[id]]
	if !voice.Active || voice.Started != started || !modulatableSynthKind(voice.Kind) {
		return false
	}
	voice.Step = step
	if voice.Kind == SourceNoise {
		if voice.Filter&SFXFilterNoiz != 0 {
			voice.Step2 = picoNoizPhaseStep(step)
		} else {
			voice.NoiseAlpha, voice.NoiseGain = picoNoiseParameters(step)
		}
	} else if voice.Kind == SourcePhaser {
		if voice.Detune != 0 {
			voice.Step2 = detunePhaseStep(voice.Kind, voice.Detune, step)
		} else {
			voice.Step2 = phaserPhaseStep(step)
		}
	} else if voice.Kind == SourceSaw && voice.Filter&SFXFilterBuzz != 0 {
		voice.Step2 = step / 2
	}
	if detune := voice.Filter.Detune(); detune != 0 && voice.Kind != SourceNoise && voice.Kind != SourcePhaser {
		voice.Step3 = detunePhaseStep(voice.Kind, detune, step)
		if voice.Kind == SourceSaw && voice.Filter&SFXFilterBuzz != 0 {
			voice.Step4 = voice.Step3 / 2
		}
	}
	voice.Volume = volume
	if voice.rampSamples != 0 {
		m.ramps--
	}
	voice.rampVolume = 0
	voice.rampDelta = 0
	voice.rampSamples = 0
	return true
}

// SetVoicePan updates one owned voice without restarting its source. The
// ownership generation prevents a channel from panning a stable ID that was
// stolen and reused by another source.
func (m *Mixer) SetVoicePan(id int, started uint32, pan int16) bool {
	if m == nil || id < 0 || id >= int(m.voiceLimit) || pan < MinPan || pan > MaxPan ||
		m.activeIDs&(uint16(1)<<id) == 0 {
		return false
	}
	voice := &m.voices[m.positions[id]]
	if !voice.Active || voice.Started != started {
		return false
	}
	voice.Pan = pan
	return true
}

// SetVoiceVolumeRamp applies a sample-continuous linear volume ramp to one
// owned synth voice. The first generated sample uses from; after samples have
// been emitted the voice remains at to. Oscillator phase is never reset.
func (m *Mixer) SetVoiceVolumeRamp(id int, started uint32, from, to uint16, samples uint32) bool {
	if m == nil || id < 0 || id >= int(m.voiceLimit) || from > MaxVolume || to > MaxVolume ||
		samples == 0 || samples > uint32(^uint16(0)) || m.activeIDs&(uint16(1)<<id) == 0 {
		return false
	}
	voice := &m.voices[m.positions[id]]
	if !voice.Active || voice.Started != started || !modulatableSynthKind(voice.Kind) {
		return false
	}
	voice.Volume = to
	voice.rampVolume = uint32(from) << 16
	voice.rampDelta = ((int32(to) - int32(from)) << 16) / int32(samples)
	if voice.rampSamples == 0 {
		m.ramps++
	}
	voice.rampSamples = uint16(samples)
	return true
}

// StartVoiceAttack applies PICO-8's short linear anti-click attack to an
// already-owned synth voice. It is independent of the authored note ramp, so
// fade-in/fade-out effects retain their full-step envelope while their first
// edge is still click-free.
func (m *Mixer) StartVoiceAttack(id int, started uint32) bool {
	if m == nil || id < 0 || id >= int(m.voiceLimit) ||
		m.activeIDs&(uint16(1)<<id) == 0 {
		return false
	}
	voice := &m.voices[m.positions[id]]
	if !voice.Active || voice.Started != started || !modulatableSynthKind(voice.Kind) {
		return false
	}
	if voice.attackLeft == 0 {
		m.attacks++
	}
	voice.attackGain = sfxEdgeStep
	voice.attackLeft = sfxEdgeSamples
	return true
}

// StartVoiceRelease detaches a synthesized note through PICO-8's short
// linear release tail. The voice remains active for exactly sfxEdgeSamples,
// advancing its oscillator independently while a replacement voice attacks.
func (m *Mixer) StartVoiceRelease(id int, started uint32) bool {
	if m == nil || id < 0 || id >= int(m.voiceLimit) ||
		m.activeIDs&(uint16(1)<<id) == 0 {
		return false
	}
	voice := &m.voices[m.positions[id]]
	if !voice.Active || voice.Started != started || !modulatableSynthKind(voice.Kind) {
		return false
	}
	if voice.releaseLeft == 0 {
		m.attacks++
	}
	voice.releaseLeft = sfxEdgeSamples
	return true
}

func (m *Mixer) voiceContribution(voice *Voice, sample int16) int32 {
	var contribution int32
	if voice.rampSamples != 0 {
		contribution = int32(sample) * int32(voice.rampVolume) >> 24
		voice.rampSamples--
		if voice.rampSamples != 0 {
			voice.rampVolume = uint32(int32(voice.rampVolume) + voice.rampDelta)
		} else {
			voice.rampVolume = 0
			voice.rampDelta = 0
			m.ramps--
		}
	} else {
		contribution = int32(sample) * int32(voice.Volume) >> 8
	}
	if voice.attackLeft != 0 {
		contribution = contribution * int32(voice.attackGain) >> 16
		voice.attackLeft--
		if voice.attackLeft != 0 {
			voice.attackGain += sfxEdgeStep
		} else {
			voice.attackGain = musicGainUnity
			m.attacks--
		}
	}
	if voice.releaseLeft != 0 {
		releaseGain := uint32(voice.releaseLeft-1) * sfxEdgeStep
		contribution = contribution * int32(releaseGain) >> 16
		voice.releaseLeft--
		if voice.releaseLeft == 0 {
			voice.Active = false
			m.attacks--
		}
	}
	return contribution
}

// SetMusicVolume immediately sets the gain applied to music-owned voices.
// Ordinary mixer users and manually triggered SFX remain at unity gain.
func (m *Mixer) SetMusicVolume(volume uint16) bool {
	if m == nil || volume > MaxVolume {
		return false
	}
	m.musicGain = uint32(volume) << 8
	m.musicGainTarget = m.musicGain
	m.musicGainStep = 0
	m.musicGainRemainder = 0
	m.musicGainError = 0
	m.musicGainSamples = 0
	m.musicGainTotal = 0
	m.musicGainDirection = 0
	return true
}

// RampMusicVolume linearly approaches volume over exactly samples emitted
// target samples. The first sample uses the current gain; after samples have
// been emitted the retained gain is exactly volume.
func (m *Mixer) RampMusicVolume(volume uint16, samples uint32) bool {
	if m == nil || volume > MaxVolume || samples == 0 {
		return false
	}
	target := uint32(volume) << 8
	var delta uint32
	direction := int8(1)
	if target < m.musicGain {
		delta = m.musicGain - target
		direction = -1
	} else {
		delta = target - m.musicGain
	}
	m.musicGainTarget = target
	step := int32(delta / samples)
	if direction < 0 {
		step = -step
	}
	m.musicGainStep = step
	m.musicGainRemainder = delta % samples
	m.musicGainError = 0
	m.musicGainSamples = samples
	m.musicGainTotal = samples
	m.musicGainDirection = direction
	return true
}

func (m *Mixer) advanceMusicGain() {
	if m == nil || m.musicGainSamples == 0 {
		return
	}
	m.musicGainSamples--
	if m.musicGainSamples == 0 {
		m.musicGain = m.musicGainTarget
		m.musicGainStep = 0
		m.musicGainRemainder = 0
		m.musicGainError = 0
		m.musicGainTotal = 0
		m.musicGainDirection = 0
		return
	}
	gain := int32(m.musicGain) + m.musicGainStep
	if m.musicGainRemainder != 0 {
		m.musicGainError += m.musicGainRemainder
		if m.musicGainError >= m.musicGainTotal {
			gain += int32(m.musicGainDirection)
			m.musicGainError -= m.musicGainTotal
		}
	}
	m.musicGain = uint32(gain)
}

// advanceMusicGainRunning is the long-fade hot path. Its caller proves the
// fade cannot end in the current destination segment, so endpoint checks and
// state clearing stay in advanceMusicGain's short-tail path.
func (m *Mixer) advanceMusicGainRunning() {
	m.musicGainSamples--
	gain := int32(m.musicGain) + m.musicGainStep
	if m.musicGainRemainder != 0 {
		m.musicGainError += m.musicGainRemainder
		if m.musicGainError >= m.musicGainTotal {
			gain += int32(m.musicGainDirection)
			m.musicGainError -= m.musicGainTotal
		}
	}
	m.musicGain = uint32(gain)
}

func (m *Mixer) Stats() Stats {
	if m == nil {
		return Stats{}
	}
	return Stats{
		ActiveVoices:     m.active,
		PeakActiveVoices: m.peak,
		VoiceSteals:      m.steals,
		FailedStarts:     m.failed,
	}
}

// packSynthMusicFirst groups active synthesized music voices before manual
// SFX/PCM voices and updates the stable-ID lookup. It runs once per destination
// segment, never per sample. Music created by SFXPlayer is always synthesized;
// a low-level music-owned PCM voice keeps the generic mixed-fade path.
func (m *Mixer) packSynthMusicFirst() bool {
	if m == nil || m.musicVoices == 0 || m.musicVoices == m.active {
		return false
	}
	for position := 0; position < int(m.active); position++ {
		voice := &m.voices[position]
		if voice.Music && voice.Kind == SourcePCM {
			return false
		}
	}
	for musicPosition := 0; musicPosition < int(m.musicVoices); musicPosition++ {
		if m.voices[musicPosition].Music {
			continue
		}
		found := -1
		for position := musicPosition + 1; position < int(m.active); position++ {
			if m.voices[position].Music {
				found = position
				break
			}
		}
		if found < 0 {
			return false
		}
		m.voices[musicPosition], m.voices[found] = m.voices[found], m.voices[musicPosition]
		m.voiceIDs[musicPosition], m.voiceIDs[found] = m.voiceIDs[found], m.voiceIDs[musicPosition]
		m.positions[m.voiceIDs[musicPosition]] = uint8(musicPosition)
		m.positions[m.voiceIDs[found]] = uint8(found)
	}
	return true
}

// nextMonoFiltered is selected only while a stateful PICO filter voice or
// tail exists. Keeping it separate leaves the measured ordinary synth, PCM,
// note-ramp, and music-gain loops unchanged.
//
//go:noinline
//go:section .iwram
func (m *Mixer) nextMonoFiltered() int8 {
	var mixed, musicMixed int32
	musicGainActive := m.musicGainSamples != 0 || m.musicGain != musicGainUnity
	fedMask := uint8(0)
	for position := 0; position < int(m.active); {
		v := &m.voices[position]
		sample, emit := nextStereoVoice(v)
		if emit {
			contribution := m.voiceContribution(v, sample)
			if state := m.filterState(v); state != nil {
				fedMask |= uint8(1) << (v.filterChannel - 1)
				contribution = state.process(contribution)
			}
			if musicGainActive && v.Music {
				musicMixed += contribution
			} else {
				mixed += contribution
			}
		}
		if !v.Active {
			m.removeActiveAt(position)
			continue
		}
		position++
	}
	for index := range m.filterStates {
		if fedMask&(uint8(1)<<index) != 0 || !m.filterStates[index].hasWork() {
			continue
		}
		state := &m.filterStates[index]
		contribution := state.process(0)
		if musicGainActive && state.music {
			musicMixed += contribution
		} else {
			mixed += contribution
		}
	}
	if musicGainActive {
		mixed += musicMixed * int32(m.musicGain) >> 16
		m.advanceMusicGain()
	}
	if mixed > 127 {
		return 127
	}
	if mixed < -128 {
		return -128
	}
	return int8(mixed)
}

// nextMono is the branch-free common path. Source stepping intentionally
// mirrors nextMonoRamped: keeping ramp selection outside the per-voice loop
// avoids charging every ordinary voice for an inactive fade.
//
//go:noinline
//go:section .iwram
func (m *Mixer) nextMono() int8 {
	if m.active == 0 {
		return 0
	}
	var mixed int32
	for position := 0; position < int(m.active); {
		v := &m.voices[position]
		var sample int16
		emit := true
		if v.Kind == SourcePCM {
			end := v.LoopEnd
			if v.Position >= end || v.Position >= uint32(len(v.PCM)) {
				v.Active = false
				emit = false
			} else {
				sample = int16(v.PCM[v.Position])
				whole := v.Step >> 16
				frac := uint32(v.Fraction) + (v.Step & 0xFFFF)
				v.Position += whole + frac>>16
				v.Fraction = uint16(frac)
				if v.Position >= end {
					if v.Loop {
						v.Position = v.LoopStart + (v.Position - end)
					} else {
						v.Active = false
					}
				}
			}
		} else {
			switch v.Kind {
			case SourceSquare:
				sample = squareVoiceSample(v)
			case SourcePulse:
				sample = pulseVoiceSample(v)
			case SourceTriangle:
				sample = triangleVoiceSample(v)
			case SourceTiltedSaw:
				sample = tiltedSawVoiceSample(v)
			case SourceOrgan:
				sample = organVoiceSample(v)
			case SourcePhaser:
				sample = phaserVoiceSample(v)
			case SourceSaw:
				sample = sawVoiceSample(v)
			case SourceNoise:
				sample = picoNoiseSample(v)
			case SourceWavetable:
				sample = int16(v.Wavetable[v.Phase>>27])
			}
			v.Phase += v.Step
			if v.Kind == SourcePhaser || v.Kind == SourceSaw && v.Filter&SFXFilterBuzz != 0 {
				v.Phase2 += v.Step2
			}
		}
		if emit {
			mixed += int32(sample) * int32(v.Volume) >> 8
		}
		if !v.Active {
			m.removeActiveAt(position)
			continue
		}
		position++
	}
	if mixed > 127 {
		return 127
	}
	if mixed < -128 {
		return -128
	}
	return int8(mixed)
}

// mixMonoPlainTriangleWords is the production fast path for the most common
// PICO/Mtile workload: one to four ordinary triangle voices with no filter,
// note-volume ramp, or music-gain ramp. Selecting it once per destination
// segment keeps the expanded eight-instrument switch and detune/filter source
// code out of every sample. Triangle voices cannot expire inside the mixer,
// so the packed active set is stable for the complete segment.
//
//go:noinline
//go:section .iwram
func (m *Mixer) mixMonoPlainTriangleWords(dst []uint32) {
	for wordIndex := range dst {
		var word uint32
		for byteIndex := 0; byteIndex < 4; byteIndex++ {
			var mixed int32
			for position := 0; position < int(m.active); position++ {
				voice := &m.voices[position]
				phase := int32(voice.Phase >> 24)
				var sample int32
				if phase < 128 {
					sample = -127 + phase*2
				} else {
					sample = 127 - (phase-128)*2
				}
				voice.Phase += voice.Step
				mixed += sample * int32(voice.Volume) >> 8
			}
			if mixed > 127 {
				mixed = 127
			} else if mixed < -128 {
				mixed = -128
			}
			word |= uint32(uint8(int8(mixed))) << uint(byteIndex*8)
		}
		dst[wordIndex] = word
	}
}

// mixMonoPlainOrganWords removes the generic source dispatcher from dense
// PICO custom-instrument envelopes. Carts commonly drive three or four
// simultaneous speed-2 organ layers; dispatching the tagged source for every
// voice and sample exceeds one GBA stream-page budget even though the organ
// oscillator itself is comfortably bounded.
//
//go:noinline
//go:section .iwram
func (m *Mixer) mixMonoPlainOrganWords(dst []uint32) {
	for wordIndex := range dst {
		var word uint32
		for byteIndex := 0; byteIndex < 4; byteIndex++ {
			var mixed int32
			for position := 0; position < int(m.active); position++ {
				voice := &m.voices[position]
				sample := int32(organSample(voice.Phase))
				voice.Phase += voice.Step
				mixed += sample * int32(voice.Volume) >> 8
			}
			if mixed > 127 {
				mixed = 127
			} else if mixed < -128 {
				mixed = -128
			}
			word |= uint32(uint8(int8(mixed))) << uint(byteIndex*8)
		}
		dst[wordIndex] = word
	}
}

// mixMonoPlainPICOSynthWords handles mixed deterministic PICO waveforms with
// one source dispatch per voice and packed output word, instead of one source
// dispatch per voice and sample. Custom instruments frequently produce a
// square/triangle/organ mix even though every active voice is otherwise a
// plain oscillator. Noise and phaser remain on the generic stateful paths.
//
//go:noinline
//go:section .iwram
func (m *Mixer) mixMonoPlainPICOSynthWords(dst []uint32) {
	for wordIndex := range dst {
		var mixed0, mixed1, mixed2, mixed3 int32
		for position := 0; position < int(m.active); position++ {
			voice := &m.voices[position]
			phase := voice.Phase
			step := voice.Step
			var sample0, sample1, sample2, sample3 int32
			switch voice.Kind {
			case SourceSquare:
				sample0 = int32(squareFilteredSample(phase, false))
				phase += step
				sample1 = int32(squareFilteredSample(phase, false))
				phase += step
				sample2 = int32(squareFilteredSample(phase, false))
				phase += step
				sample3 = int32(squareFilteredSample(phase, false))
				phase += step
			case SourceTriangle:
				sample0 = int32(triangleSample(phase))
				phase += step
				sample1 = int32(triangleSample(phase))
				phase += step
				sample2 = int32(triangleSample(phase))
				phase += step
				sample3 = int32(triangleSample(phase))
				phase += step
			case SourceSaw:
				sample0 = int32(phase>>24) - 128
				phase += step
				sample1 = int32(phase>>24) - 128
				phase += step
				sample2 = int32(phase>>24) - 128
				phase += step
				sample3 = int32(phase>>24) - 128
				phase += step
			case SourcePulse:
				sample0 = int32(pulseFilteredSample(phase, false))
				phase += step
				sample1 = int32(pulseFilteredSample(phase, false))
				phase += step
				sample2 = int32(pulseFilteredSample(phase, false))
				phase += step
				sample3 = int32(pulseFilteredSample(phase, false))
				phase += step
			case SourceTiltedSaw:
				sample0 = int32(tiltedSawSample(phase))
				phase += step
				sample1 = int32(tiltedSawSample(phase))
				phase += step
				sample2 = int32(tiltedSawSample(phase))
				phase += step
				sample3 = int32(tiltedSawSample(phase))
				phase += step
			case SourceOrgan:
				sample0 = int32(organSample(phase))
				phase += step
				sample1 = int32(organSample(phase))
				phase += step
				sample2 = int32(organSample(phase))
				phase += step
				sample3 = int32(organSample(phase))
				phase += step
			case SourceNoise:
				sample0 = int32(picoNoiseSample(voice))
				phase += step
				sample1 = int32(picoNoiseSample(voice))
				phase += step
				sample2 = int32(picoNoiseSample(voice))
				phase += step
				sample3 = int32(picoNoiseSample(voice))
				phase += step
			case SourcePhaser:
				sample0 = int32(phaserVoiceSample(voice))
				phase += step
				voice.Phase += step
				voice.Phase2 += voice.Step2
				sample1 = int32(phaserVoiceSample(voice))
				phase += step
				voice.Phase += step
				voice.Phase2 += voice.Step2
				sample2 = int32(phaserVoiceSample(voice))
				phase += step
				voice.Phase += step
				voice.Phase2 += voice.Step2
				sample3 = int32(phaserVoiceSample(voice))
				phase += step
				voice.Phase2 += voice.Step2
			}
			voice.Phase = phase
			volume := int32(voice.Volume)
			mixed0 += sample0 * volume >> 8
			mixed1 += sample1 * volume >> 8
			mixed2 += sample2 * volume >> 8
			mixed3 += sample3 * volume >> 8
		}
		if mixed0 > 127 {
			mixed0 = 127
		} else if mixed0 < -128 {
			mixed0 = -128
		}
		if mixed1 > 127 {
			mixed1 = 127
		} else if mixed1 < -128 {
			mixed1 = -128
		}
		if mixed2 > 127 {
			mixed2 = 127
		} else if mixed2 < -128 {
			mixed2 = -128
		}
		if mixed3 > 127 {
			mixed3 = 127
		} else if mixed3 < -128 {
			mixed3 = -128
		}
		dst[wordIndex] = uint32(uint8(int8(mixed0))) |
			uint32(uint8(int8(mixed1)))<<8 |
			uint32(uint8(int8(mixed2)))<<16 |
			uint32(uint8(int8(mixed3)))<<24
	}
}

//go:noinline
//go:section .iwram
func plainPICOSynthWordSamples(voice *Voice) (int32, int32, int32, int32) {
	phase := voice.Phase
	step := voice.Step
	var sample0, sample1, sample2, sample3 int32
	switch voice.Kind {
	case SourceSquare:
		sample0 = int32(squareFilteredSample(phase, false))
		phase += step
		sample1 = int32(squareFilteredSample(phase, false))
		phase += step
		sample2 = int32(squareFilteredSample(phase, false))
		phase += step
		sample3 = int32(squareFilteredSample(phase, false))
		phase += step
	case SourceTriangle:
		sample0 = int32(triangleSample(phase))
		phase += step
		sample1 = int32(triangleSample(phase))
		phase += step
		sample2 = int32(triangleSample(phase))
		phase += step
		sample3 = int32(triangleSample(phase))
		phase += step
	case SourceSaw:
		sample0 = int32(phase>>24) - 128
		phase += step
		sample1 = int32(phase>>24) - 128
		phase += step
		sample2 = int32(phase>>24) - 128
		phase += step
		sample3 = int32(phase>>24) - 128
		phase += step
	case SourcePulse:
		sample0 = int32(pulseFilteredSample(phase, false))
		phase += step
		sample1 = int32(pulseFilteredSample(phase, false))
		phase += step
		sample2 = int32(pulseFilteredSample(phase, false))
		phase += step
		sample3 = int32(pulseFilteredSample(phase, false))
		phase += step
	case SourceTiltedSaw:
		sample0 = int32(tiltedSawSample(phase))
		phase += step
		sample1 = int32(tiltedSawSample(phase))
		phase += step
		sample2 = int32(tiltedSawSample(phase))
		phase += step
		sample3 = int32(tiltedSawSample(phase))
		phase += step
	case SourceOrgan:
		sample0 = int32(organSample(phase))
		phase += step
		sample1 = int32(organSample(phase))
		phase += step
		sample2 = int32(organSample(phase))
		phase += step
		sample3 = int32(organSample(phase))
		phase += step
	case SourceNoise:
		sample0 = int32(picoNoiseSample(voice))
		phase += step
		sample1 = int32(picoNoiseSample(voice))
		phase += step
		sample2 = int32(picoNoiseSample(voice))
		phase += step
		sample3 = int32(picoNoiseSample(voice))
		phase += step
	case SourcePhaser:
		sample0 = int32(phaserVoiceSample(voice))
		phase += step
		voice.Phase += step
		voice.Phase2 += voice.Step2
		sample1 = int32(phaserVoiceSample(voice))
		phase += step
		voice.Phase += step
		voice.Phase2 += voice.Step2
		sample2 = int32(phaserVoiceSample(voice))
		phase += step
		voice.Phase += step
		voice.Phase2 += voice.Step2
		sample3 = int32(phaserVoiceSample(voice))
		phase += step
		voice.Phase2 += voice.Step2
	}
	voice.Phase = phase
	return sample0, sample1, sample2, sample3
}

// mixMonoPlainPICOSynthRampedWords is the mixed-waveform counterpart of the
// triangle ramp fast path. Dispatching once per packed word keeps PICO fade
// effects on organ/phaser/noise arrangements within one target page budget.
// The caller proves that no global music-gain ramp or stateful filter exists.
//
//go:noinline
//go:section .iwram
func (m *Mixer) mixMonoPlainPICOSynthRampedWords(dst []uint32) {
	for wordIndex := range dst {
		var mixed [4]int32
		for position := 0; position < int(m.active); position++ {
			voice := &m.voices[position]
			sample0, sample1, sample2, sample3 := plainPICOSynthWordSamples(voice)
			samples := [4]int32{sample0, sample1, sample2, sample3}
			for sampleIndex, sample := range samples {
				if voice.rampSamples != 0 {
					mixed[sampleIndex] += sample * int32(voice.rampVolume) >> 24
					voice.rampSamples--
					if voice.rampSamples != 0 {
						voice.rampVolume = uint32(int32(voice.rampVolume) + voice.rampDelta)
					} else {
						voice.rampVolume = 0
						voice.rampDelta = 0
						m.ramps--
					}
				} else {
					mixed[sampleIndex] += sample * int32(voice.Volume) >> 8
				}
			}
		}
		for sampleIndex := range mixed {
			if mixed[sampleIndex] > 127 {
				mixed[sampleIndex] = 127
			} else if mixed[sampleIndex] < -128 {
				mixed[sampleIndex] = -128
			}
		}
		dst[wordIndex] = uint32(uint8(int8(mixed[0]))) |
			uint32(uint8(int8(mixed[1])))<<8 |
			uint32(uint8(int8(mixed[2])))<<16 |
			uint32(uint8(int8(mixed[3])))<<24
	}
}

// mixMonoPlainTriangleRampedWords keeps PICO fade-in/fade-out effects on the
// same compact triangle path while preserving the generic mixer's exact Q16
// ramp arithmetic. The caller proves that no global music-gain ramp is live.
//
//go:noinline
//go:section .iwram
func (m *Mixer) mixMonoPlainTriangleRampedWords(dst []uint32) {
	for wordIndex := range dst {
		var word uint32
		for byteIndex := 0; byteIndex < 4; byteIndex++ {
			var mixed int32
			for position := 0; position < int(m.active); position++ {
				voice := &m.voices[position]
				phase := int32(voice.Phase >> 24)
				var sample int32
				if phase < 128 {
					sample = -127 + phase*2
				} else {
					sample = 127 - (phase-128)*2
				}
				voice.Phase += voice.Step
				if voice.rampSamples != 0 {
					mixed += sample * int32(voice.rampVolume) >> 24
					voice.rampSamples--
					if voice.rampSamples != 0 {
						voice.rampVolume = uint32(int32(voice.rampVolume) + voice.rampDelta)
					} else {
						voice.rampVolume = 0
						voice.rampDelta = 0
						m.ramps--
					}
				} else {
					mixed += sample * int32(voice.Volume) >> 8
				}
			}
			if mixed > 127 {
				mixed = 127
			} else if mixed < -128 {
				mixed = -128
			}
			word |= uint32(uint8(int8(mixed))) << uint(byteIndex*8)
		}
		dst[wordIndex] = word
	}
}

// mixMonoPlainTriangleMusicRampedWords is the all-music counterpart of the
// ordinary triangle fast path. The caller proves that the global fade cannot
// end inside dst, allowing endpoint handling to stay outside the sample loop.
//
//go:noinline
//go:section .iwram
func (m *Mixer) mixMonoPlainTriangleMusicRampedWords(dst []uint32) {
	for wordIndex := range dst {
		var word uint32
		for byteIndex := 0; byteIndex < 4; byteIndex++ {
			var mixed int32
			for position := 0; position < int(m.active); position++ {
				voice := &m.voices[position]
				phase := int32(voice.Phase >> 24)
				var sample int32
				if phase < 128 {
					sample = -127 + phase*2
				} else {
					sample = 127 - (phase-128)*2
				}
				voice.Phase += voice.Step
				mixed += sample * int32(voice.Volume) >> 8
			}
			mixed = mixed * int32(m.musicGain) >> 16
			m.advanceMusicGainRunning()
			if mixed > 127 {
				mixed = 127
			} else if mixed < -128 {
				mixed = -128
			}
			word |= uint32(uint8(int8(mixed))) << uint(byteIndex*8)
		}
		dst[wordIndex] = word
	}
}

// mixMonoTriangleVariantWords covers stateless PICO triangle modifiers such
// as buzz and detune. Source selection is performed once per destination
// segment rather than through the full tagged-source switch on every sample.
//
//go:noinline
//go:section .iwram
func (m *Mixer) mixMonoTriangleVariantWords(dst []uint32) {
	for wordIndex := range dst {
		var word uint32
		for byteIndex := 0; byteIndex < 4; byteIndex++ {
			var mixed int32
			for position := 0; position < int(m.active); position++ {
				voice := &m.voices[position]
				sample := int32(triangleVoiceSample(voice))
				voice.Phase += voice.Step
				mixed += sample * int32(voice.Volume) >> 8
			}
			if mixed > 127 {
				mixed = 127
			} else if mixed < -128 {
				mixed = -128
			}
			word |= uint32(uint8(int8(mixed))) << uint(byteIndex*8)
		}
		dst[wordIndex] = word
	}
}

// mixMonoFilteredTriangleWords is selected only when every active voice is a
// stateful-filtered triangle and no stopped channel has a live tail. Each
// logical channel is therefore fed exactly once per sample, eliminating the
// generic source dispatcher and per-sample fed-mask/tail scan. Mixed source
// sets and tails continue through nextMonoFiltered.
//
//go:noinline
//go:section .iwram
func (m *Mixer) mixMonoFilteredTriangleWords(dst []uint32) {
	for wordIndex := range dst {
		var word uint32
		for byteIndex := 0; byteIndex < 4; byteIndex++ {
			var mixed int32
			for position := 0; position < int(m.active); position++ {
				voice := &m.voices[position]
				var sample int32
				buzz := voice.Filter&SFXFilterBuzz != 0
				if voice.Detune == 0 && !buzz {
					phase := int32(voice.Phase >> 24)
					if phase < 128 {
						sample = -127 + phase*2
					} else {
						sample = 127 - (phase-128)*2
					}
				} else {
					primary := triangleFilteredSample(voice.Phase, buzz)
					if voice.Detune == 0 {
						sample = int32(primary)
					} else {
						secondary := triangleFilteredSample(voice.Phase3, buzz)
						voice.Phase3 += voice.Step3
						sample = (int32(primary)*10 + int32(secondary)*5) / 12
					}
				}
				voice.Phase += voice.Step
				contribution := sample * int32(voice.Volume) >> 8
				state := &m.filterStates[voice.filterChannel-1]
				mixed += state.process(contribution)
			}
			if mixed > 127 {
				mixed = 127
			} else if mixed < -128 {
				mixed = -128
			}
			word |= uint32(uint8(int8(mixed))) << uint(byteIndex*8)
		}
		dst[wordIndex] = word
	}
}

func (m *Mixer) hasOnlyPlainTriangles() bool {
	if m == nil || m.active == 0 {
		return false
	}
	for position := 0; position < int(m.active); position++ {
		voice := &m.voices[position]
		if voice.Kind != SourceTriangle || voice.filterChannel != 0 || voice.Detune != 0 ||
			voice.Filter&SFXFilterBuzz != 0 {
			return false
		}
	}
	return true
}

func (m *Mixer) hasOnlyPlainOrgans() bool {
	if m == nil || m.active == 0 {
		return false
	}
	for position := 0; position < int(m.active); position++ {
		voice := &m.voices[position]
		if voice.Kind != SourceOrgan || voice.filterChannel != 0 || voice.Detune != 0 ||
			voice.Filter&SFXFilterBuzz != 0 {
			return false
		}
	}
	return true
}

func plainPICOSynthKind(kind SourceKind) bool {
	return kind == SourceSquare || kind == SourceTriangle || kind == SourceSaw ||
		kind == SourcePulse || kind == SourceTiltedSaw || kind == SourceOrgan ||
		kind == SourceNoise || kind == SourcePhaser
}

func (m *Mixer) hasOnlyPlainPICOSynths() bool {
	if m == nil || m.active == 0 {
		return false
	}
	for position := 0; position < int(m.active); position++ {
		voice := &m.voices[position]
		if !plainPICOSynthKind(voice.Kind) || voice.filterChannel != 0 || voice.Detune != 0 ||
			voice.Filter&SFXFilterBuzz != 0 {
			return false
		}
	}
	return true
}

func (m *Mixer) hasOnlyTriangleVariants() bool {
	if m == nil || m.active == 0 {
		return false
	}
	for position := 0; position < int(m.active); position++ {
		voice := &m.voices[position]
		if voice.Kind != SourceTriangle || voice.filterChannel != 0 {
			return false
		}
	}
	return true
}

func (m *Mixer) canMixFilteredTrianglesFast() bool {
	if m == nil || m.filterStates == nil || m.active == 0 || m.filterVoices != m.active {
		return false
	}
	fedMask := uint8(0)
	for position := 0; position < int(m.active); position++ {
		voice := &m.voices[position]
		if voice.Kind != SourceTriangle || voice.filterChannel == 0 || voice.filterChannel > SFXChannels {
			return false
		}
		bit := uint8(1) << (voice.filterChannel - 1)
		if fedMask&bit != 0 {
			return false
		}
		fedMask |= bit
	}
	for index := range m.filterStates {
		if fedMask&(uint8(1)<<index) == 0 && m.filterStates[index].hasWork() {
			return false
		}
	}
	return true
}

// nextMonoMusicRamped is the all-music global-gain path. MixMono selects it
// only when every active voice is music-owned and no note ramp is live, so the
// sample loop needs neither ownership branches nor a second accumulator.
//
//go:noinline
//go:section .iwram
func (m *Mixer) nextMonoMusicRamped() int8 {
	if m.active == 0 {
		m.advanceMusicGain()
		return 0
	}
	var mixed int32
	for position := 0; position < int(m.active); {
		v := &m.voices[position]
		var sample int16
		emit := true
		if v.Kind == SourcePCM {
			end := v.LoopEnd
			if v.Position >= end || v.Position >= uint32(len(v.PCM)) {
				v.Active = false
				emit = false
			} else {
				sample = int16(v.PCM[v.Position])
				whole := v.Step >> 16
				frac := uint32(v.Fraction) + (v.Step & 0xFFFF)
				v.Position += whole + frac>>16
				v.Fraction = uint16(frac)
				if v.Position >= end {
					if v.Loop {
						v.Position = v.LoopStart + (v.Position - end)
					} else {
						v.Active = false
					}
				}
			}
		} else {
			switch v.Kind {
			case SourceSquare:
				sample = squareVoiceSample(v)
			case SourcePulse:
				sample = pulseVoiceSample(v)
			case SourceTriangle:
				sample = triangleVoiceSample(v)
			case SourceTiltedSaw:
				sample = tiltedSawVoiceSample(v)
			case SourceOrgan:
				sample = organVoiceSample(v)
			case SourcePhaser:
				sample = phaserVoiceSample(v)
			case SourceSaw:
				sample = sawVoiceSample(v)
			case SourceNoise:
				sample = picoNoiseSample(v)
			case SourceWavetable:
				sample = int16(v.Wavetable[v.Phase>>27])
			}
			v.Phase += v.Step
			if v.Kind == SourcePhaser || v.Kind == SourceSaw && v.Filter&SFXFilterBuzz != 0 {
				v.Phase2 += v.Step2
			}
		}
		if emit {
			mixed += int32(sample) * int32(v.Volume) >> 8
		}
		if !v.Active {
			m.removeActiveAt(position)
			continue
		}
		position++
	}
	mixed = mixed * int32(m.musicGain) >> 16
	m.advanceMusicGain()
	if mixed > 127 {
		return 127
	}
	if mixed < -128 {
		return -128
	}
	return int8(mixed)
}

// nextMonoMixedMusicRamped handles a global music fade while manual SFX or
// PCM voices are also active. MixMono selects it only when no per-note volume
// ramp is live, so this common native-sample case avoids the general ramp
// branch and ramp-state work for every voice and output sample.
//
//go:noinline
//go:section .iwram
func (m *Mixer) nextMonoMixedMusicRamped() int8 {
	if m.active == 0 {
		m.advanceMusicGain()
		return 0
	}
	var mixed, musicMixed int32
	for position := 0; position < int(m.active); {
		v := &m.voices[position]
		var sample int16
		emit := true
		if v.Kind == SourcePCM {
			end := v.LoopEnd
			if v.Position >= end || v.Position >= uint32(len(v.PCM)) {
				v.Active = false
				emit = false
			} else {
				sample = int16(v.PCM[v.Position])
				whole := v.Step >> 16
				frac := uint32(v.Fraction) + (v.Step & 0xFFFF)
				v.Position += whole + frac>>16
				v.Fraction = uint16(frac)
				if v.Position >= end {
					if v.Loop {
						v.Position = v.LoopStart + (v.Position - end)
					} else {
						v.Active = false
					}
				}
			}
		} else {
			switch v.Kind {
			case SourceSquare:
				sample = squareVoiceSample(v)
			case SourcePulse:
				sample = pulseVoiceSample(v)
			case SourceTriangle:
				sample = triangleVoiceSample(v)
			case SourceTiltedSaw:
				sample = tiltedSawVoiceSample(v)
			case SourceOrgan:
				sample = organVoiceSample(v)
			case SourcePhaser:
				sample = phaserVoiceSample(v)
			case SourceSaw:
				sample = sawVoiceSample(v)
			case SourceNoise:
				sample = picoNoiseSample(v)
			case SourceWavetable:
				sample = int16(v.Wavetable[v.Phase>>27])
			}
			v.Phase += v.Step
			if v.Kind == SourcePhaser || v.Kind == SourceSaw && v.Filter&SFXFilterBuzz != 0 {
				v.Phase2 += v.Step2
			}
		}
		if emit {
			contribution := int32(sample) * int32(v.Volume) >> 8
			if v.Music {
				musicMixed += contribution
			} else {
				mixed += contribution
			}
		}
		if !v.Active {
			m.removeActiveAt(position)
			continue
		}
		position++
	}
	mixed += musicMixed * int32(m.musicGain) >> 16
	m.advanceMusicGain()
	if mixed > 127 {
		return 127
	}
	if mixed < -128 {
		return -128
	}
	return int8(mixed)
}

// mixMonoPackedMusicWords is the generated sample-cartridge long-fade path
// after packSynthMusicFirst has grouped synthesized music before manual
// voices. One call emits a complete destination segment, amortizing its ARM
// register-save frame across every sample. The caller proves the fade cannot
// end in the segment. The music prefix cannot expire inside the mixer, so it
// avoids PCM, ownership, emit, liveness, and fade-end branches. The manual
// suffix retains every source kind and PCM end/loop behavior.
//
//go:noinline
//go:section .iwram
func (m *Mixer) mixMonoPackedMusicWords(dst []uint32) {
	for wordIndex := range dst {
		var word uint32
		for byteIndex := 0; byteIndex < 4; byteIndex++ {
			var mixed, musicMixed int32
			for position := 0; position < int(m.musicVoices); position++ {
				v := &m.voices[position]
				var sample int16
				switch v.Kind {
				case SourceSquare:
					sample = squareVoiceSample(v)
				case SourcePulse:
					sample = pulseVoiceSample(v)
				case SourceTriangle:
					sample = triangleVoiceSample(v)
				case SourceTiltedSaw:
					sample = tiltedSawVoiceSample(v)
				case SourceOrgan:
					sample = organVoiceSample(v)
				case SourcePhaser:
					sample = phaserVoiceSample(v)
				case SourceSaw:
					sample = sawVoiceSample(v)
				case SourceNoise:
					sample = picoNoiseSample(v)
				case SourceWavetable:
					sample = int16(v.Wavetable[v.Phase>>27])
				}
				v.Phase += v.Step
				if v.Kind == SourcePhaser || v.Kind == SourceSaw && v.Filter&SFXFilterBuzz != 0 {
					v.Phase2 += v.Step2
				}
				musicMixed += int32(sample) * int32(v.Volume) >> 8
			}

			for position := int(m.musicVoices); position < int(m.active); {
				v := &m.voices[position]
				var sample int16
				emit := true
				if v.Kind == SourcePCM {
					end := v.LoopEnd
					if v.Position >= end || v.Position >= uint32(len(v.PCM)) {
						v.Active = false
						emit = false
					} else {
						sample = int16(v.PCM[v.Position])
						whole := v.Step >> 16
						frac := uint32(v.Fraction) + (v.Step & 0xFFFF)
						v.Position += whole + frac>>16
						v.Fraction = uint16(frac)
						if v.Position >= end {
							if v.Loop {
								v.Position = v.LoopStart + (v.Position - end)
							} else {
								v.Active = false
							}
						}
					}
				} else {
					switch v.Kind {
					case SourceSquare:
						sample = squareVoiceSample(v)
					case SourcePulse:
						sample = pulseVoiceSample(v)
					case SourceTriangle:
						sample = triangleVoiceSample(v)
					case SourceTiltedSaw:
						sample = tiltedSawVoiceSample(v)
					case SourceOrgan:
						sample = organVoiceSample(v)
					case SourcePhaser:
						sample = phaserVoiceSample(v)
					case SourceSaw:
						sample = sawVoiceSample(v)
					case SourceNoise:
						sample = picoNoiseSample(v)
					case SourceWavetable:
						sample = int16(v.Wavetable[v.Phase>>27])
					}
					v.Phase += v.Step
					if v.Kind == SourcePhaser || v.Kind == SourceSaw && v.Filter&SFXFilterBuzz != 0 {
						v.Phase2 += v.Step2
					}
				}
				if emit {
					mixed += int32(sample) * int32(v.Volume) >> 8
				}
				if !v.Active {
					m.removeActiveAt(position)
					continue
				}
				position++
			}
			mixed += musicMixed * int32(m.musicGain) >> 16
			m.advanceMusicGainRunning()
			if mixed > 127 {
				mixed = 127
			} else if mixed < -128 {
				mixed = -128
			}
			word |= uint32(uint8(int8(mixed))) << uint(byteIndex*8)
		}
		dst[wordIndex] = word
	}
}

// nextMonoRamped handles note ramps and mixed music/manual global gain. The
// common all-music global-gain case uses nextMonoMusicRamped instead.
//
//go:noinline
//go:section .iwram
func (m *Mixer) nextMonoRamped() int8 {
	if m.active == 0 {
		m.advanceMusicGain()
		return 0
	}
	var mixed, musicMixed int32
	musicGainActive := m.musicGainSamples != 0 || m.musicGain != musicGainUnity
	for position := 0; position < int(m.active); {
		v := &m.voices[position]
		var sample int16
		emit := true
		if v.Kind == SourcePCM {
			end := v.LoopEnd
			if v.Position >= end || v.Position >= uint32(len(v.PCM)) {
				v.Active = false
				emit = false
			} else {
				sample = int16(v.PCM[v.Position])
				whole := v.Step >> 16
				frac := uint32(v.Fraction) + (v.Step & 0xFFFF)
				v.Position += whole + frac>>16
				v.Fraction = uint16(frac)
				if v.Position >= end {
					if v.Loop {
						v.Position = v.LoopStart + (v.Position - end)
					} else {
						v.Active = false
					}
				}
			}
		} else {
			switch v.Kind {
			case SourceSquare:
				sample = squareVoiceSample(v)
			case SourcePulse:
				sample = pulseVoiceSample(v)
			case SourceTriangle:
				sample = triangleVoiceSample(v)
			case SourceTiltedSaw:
				sample = tiltedSawVoiceSample(v)
			case SourceOrgan:
				sample = organVoiceSample(v)
			case SourcePhaser:
				sample = phaserVoiceSample(v)
			case SourceSaw:
				sample = sawVoiceSample(v)
			case SourceNoise:
				sample = picoNoiseSample(v)
			case SourceWavetable:
				sample = int16(v.Wavetable[v.Phase>>27])
			}
			v.Phase += v.Step
			if v.Kind == SourcePhaser || v.Kind == SourceSaw && v.Filter&SFXFilterBuzz != 0 {
				v.Phase2 += v.Step2
			}
		}
		if emit {
			contribution := m.voiceContribution(v, sample)
			if musicGainActive && v.Music {
				musicMixed += contribution
			} else {
				mixed += contribution
			}
		}
		if !v.Active {
			m.removeActiveAt(position)
			continue
		}
		position++
	}
	if musicGainActive {
		mixed += musicMixed * int32(m.musicGain) >> 16
		m.advanceMusicGain()
	}
	if mixed > 127 {
		return 127
	}
	if mixed < -128 {
		return -128
	}
	return int8(mixed)
}

// nextStereoVoice advances one source by one output sample. The mono hot paths
// keep their source code inline because ARM measurements showed that call
// overhead matters there; the provisional stereo path centralizes the same
// state transition until its own target measurements justify specialization.
//
//go:noinline
//go:section .iwram
func nextStereoVoice(v *Voice) (int16, bool) {
	if v.Kind == SourcePCM {
		end := v.LoopEnd
		if v.Position >= end || v.Position >= uint32(len(v.PCM)) {
			v.Active = false
			return 0, false
		}
		sample := int16(v.PCM[v.Position])
		whole := v.Step >> 16
		frac := uint32(v.Fraction) + (v.Step & 0xFFFF)
		v.Position += whole + frac>>16
		v.Fraction = uint16(frac)
		if v.Position >= end {
			if v.Loop {
				v.Position = v.LoopStart + (v.Position - end)
			} else {
				v.Active = false
			}
		}
		return sample, true
	}

	var sample int16
	switch v.Kind {
	case SourceSquare:
		sample = squareVoiceSample(v)
	case SourcePulse:
		sample = pulseVoiceSample(v)
	case SourceTriangle:
		sample = triangleVoiceSample(v)
	case SourceTiltedSaw:
		sample = tiltedSawVoiceSample(v)
	case SourceOrgan:
		sample = organVoiceSample(v)
	case SourcePhaser:
		sample = phaserVoiceSample(v)
	case SourceSaw:
		sample = sawVoiceSample(v)
	case SourceNoise:
		sample = picoNoiseSample(v)
	case SourceWavetable:
		sample = int16(v.Wavetable[v.Phase>>27])
	}
	v.Phase += v.Step
	if v.Kind == SourcePhaser || v.Kind == SourceSaw && v.Filter&SFXFilterBuzz != 0 {
		v.Phase2 += v.Step2
	}
	return sample, true
}

func stereoPanGains(pan int16) (int32, int32) {
	left, right := int32(MaxVolume), int32(MaxVolume)
	if pan > 0 {
		left -= int32(pan)
	} else if pan < 0 {
		right += int32(pan)
	}
	return left, right
}

func clampStereoSample(sample int32) int8 {
	if sample > 127 {
		return 127
	}
	if sample < -128 {
		return -128
	}
	return int8(sample)
}

// nextStereoFiltered mirrors nextMonoFiltered, applying the mono PICO filter
// before Mtile's optional per-channel pan and preserving panned tails.
//
//go:noinline
//go:section .iwram
func (m *Mixer) nextStereoFiltered() uint16 {
	var mixedLeft, mixedRight, musicLeft, musicRight int32
	musicGainActive := m.musicGainSamples != 0 || m.musicGain != musicGainUnity
	fedMask := uint8(0)
	for position := 0; position < int(m.active); {
		v := &m.voices[position]
		sample, emit := nextStereoVoice(v)
		if emit {
			contribution := m.voiceContribution(v, sample)
			if state := m.filterState(v); state != nil {
				fedMask |= uint8(1) << (v.filterChannel - 1)
				contribution = state.process(contribution)
			}
			left, right := contribution, contribution
			if v.Pan != 0 {
				leftGain, rightGain := stereoPanGains(v.Pan)
				left = contribution * leftGain >> 8
				right = contribution * rightGain >> 8
			}
			if musicGainActive && v.Music {
				musicLeft += left
				musicRight += right
			} else {
				mixedLeft += left
				mixedRight += right
			}
		}
		if !v.Active {
			m.removeActiveAt(position)
			continue
		}
		position++
	}
	for index := range m.filterStates {
		if fedMask&(uint8(1)<<index) != 0 || !m.filterStates[index].hasWork() {
			continue
		}
		state := &m.filterStates[index]
		contribution := state.process(0)
		left, right := contribution, contribution
		if state.pan != 0 {
			leftGain, rightGain := stereoPanGains(state.pan)
			left = contribution * leftGain >> 8
			right = contribution * rightGain >> 8
		}
		if musicGainActive && state.music {
			musicLeft += left
			musicRight += right
		} else {
			mixedLeft += left
			mixedRight += right
		}
	}
	if musicGainActive {
		mixedLeft += musicLeft * int32(m.musicGain) >> 16
		mixedRight += musicRight * int32(m.musicGain) >> 16
		m.advanceMusicGain()
	}
	left := uint8(clampStereoSample(mixedLeft))
	right := uint8(clampStereoSample(mixedRight))
	return uint16(left) | uint16(right)<<8
}

// nextStereo is the common path when neither note nor global music gain is
// active. Keeping those checks outside the voice loop mirrors the measured
// mono specialization.
//
//go:noinline
//go:section .iwram
func (m *Mixer) nextStereo() uint16 {
	if m.active == 0 {
		return 0
	}
	var mixedLeft, mixedRight int32
	for position := 0; position < int(m.active); {
		v := &m.voices[position]
		sample, emit := nextStereoVoice(v)
		if emit {
			contribution := int32(sample) * int32(v.Volume) >> 8
			if v.Pan == 0 {
				mixedLeft += contribution
				mixedRight += contribution
			} else {
				leftGain, rightGain := stereoPanGains(v.Pan)
				mixedLeft += contribution * leftGain >> 8
				mixedRight += contribution * rightGain >> 8
			}
		}
		if !v.Active {
			m.removeActiveAt(position)
			continue
		}
		position++
	}
	left := uint8(clampStereoSample(mixedLeft))
	right := uint8(clampStereoSample(mixedRight))
	return uint16(left) | uint16(right)<<8
}

// nextStereoMusicRamped is the all-music global-gain path. Callers select it
// only when every active voice is music-owned and no note ramp is live, so the
// hot loop needs neither ownership branches nor separate manual/music stereo
// accumulators.
//
//go:noinline
//go:section .iwram
func (m *Mixer) nextStereoMusicRamped() uint16 {
	if m.active == 0 {
		m.advanceMusicGain()
		return 0
	}
	var mixedLeft, mixedRight int32
	for position := 0; position < int(m.active); {
		v := &m.voices[position]
		sample, emit := nextStereoVoice(v)
		if emit {
			contribution := int32(sample) * int32(v.Volume) >> 8
			if v.Pan == 0 {
				mixedLeft += contribution
				mixedRight += contribution
			} else {
				leftGain, rightGain := stereoPanGains(v.Pan)
				mixedLeft += contribution * leftGain >> 8
				mixedRight += contribution * rightGain >> 8
			}
		}
		if !v.Active {
			m.removeActiveAt(position)
			continue
		}
		position++
	}
	mixedLeft = mixedLeft * int32(m.musicGain) >> 16
	mixedRight = mixedRight * int32(m.musicGain) >> 16
	m.advanceMusicGain()
	left := uint8(clampStereoSample(mixedLeft))
	right := uint8(clampStereoSample(mixedRight))
	return uint16(left) | uint16(right)<<8
}

// nextStereoRamped handles note ramps and mixed music/manual global gain.
// Centered voices retain unity gain in each channel, so each side remains
// byte-identical to MixMono before independent left/right saturation.
//
//go:noinline
//go:section .iwram
func (m *Mixer) nextStereoRamped() uint16 {
	if m.active == 0 {
		m.advanceMusicGain()
		return 0
	}
	var mixedLeft, mixedRight int32
	var musicLeft, musicRight int32
	musicGainActive := m.musicGainSamples != 0 || m.musicGain != musicGainUnity
	for position := 0; position < int(m.active); {
		v := &m.voices[position]
		sample, emit := nextStereoVoice(v)
		if emit {
			contribution := m.voiceContribution(v, sample)
			left, right := contribution, contribution
			if v.Pan != 0 {
				leftGain, rightGain := stereoPanGains(v.Pan)
				left = contribution * leftGain >> 8
				right = contribution * rightGain >> 8
			}
			if musicGainActive && v.Music {
				musicLeft += left
				musicRight += right
			} else {
				mixedLeft += left
				mixedRight += right
			}
		}
		if !v.Active {
			m.removeActiveAt(position)
			continue
		}
		position++
	}
	if musicGainActive {
		mixedLeft += musicLeft * int32(m.musicGain) >> 16
		mixedRight += musicRight * int32(m.musicGain) >> 16
		m.advanceMusicGain()
	}
	left := uint8(clampStereoSample(mixedLeft))
	right := uint8(clampStereoSample(mixedRight))
	return uint16(left) | uint16(right)<<8
}

// MixMono writes signed 8-bit target samples with explicit saturation.
//
//go:noinline
//go:section .iwram
func (m *Mixer) MixMono(dst []int8) {
	if m == nil {
		for i := range dst {
			dst[i] = 0
		}
		return
	}
	if m.hasFilterWork() {
		for i := range dst {
			dst[i] = m.nextMonoFiltered()
		}
		return
	}
	if m.ramps != 0 || m.attacks != 0 {
		for i := range dst {
			dst[i] = m.nextMonoRamped()
		}
		return
	}
	if m.musicGainSamples != 0 || m.musicGain != musicGainUnity {
		if m.musicVoices == m.active {
			for i := range dst {
				dst[i] = m.nextMonoMusicRamped()
			}
		} else {
			for i := range dst {
				dst[i] = m.nextMonoMixedMusicRamped()
			}
		}
		return
	}
	for i := range dst {
		dst[i] = m.nextMono()
	}
}

// MixMonoWords packs four consecutive signed 8-bit samples per uint32 in the
// FIFO playback order. It avoids a byte-to-word conversion pass on target.
//
//go:noinline
//go:section .iwram
func (m *Mixer) MixMonoWords(dst []uint32) {
	if m == nil {
		for i := range dst {
			dst[i] = 0
		}
		return
	}
	if m.hasFilterWork() {
		if m.ramps == 0 && m.attacks == 0 && m.musicGainSamples == 0 && m.musicGain == musicGainUnity &&
			m.canMixFilteredTrianglesFast() {
			m.mixMonoFilteredTriangleWords(dst)
			return
		}
		for i := range dst {
			dst[i] = uint32(uint8(m.nextMonoFiltered())) |
				uint32(uint8(m.nextMonoFiltered()))<<8 |
				uint32(uint8(m.nextMonoFiltered()))<<16 |
				uint32(uint8(m.nextMonoFiltered()))<<24
		}
		return
	}
	if m.ramps != 0 || m.attacks != 0 {
		if m.attacks == 0 && m.musicGainSamples == 0 && m.musicGain == musicGainUnity && m.hasOnlyPlainTriangles() {
			m.mixMonoPlainTriangleRampedWords(dst)
			return
		}
		if m.attacks == 0 && m.musicGainSamples == 0 && m.musicGain == musicGainUnity && m.hasOnlyPlainPICOSynths() {
			m.mixMonoPlainPICOSynthRampedWords(dst)
			return
		}
		for i := range dst {
			dst[i] = uint32(uint8(m.nextMonoRamped())) |
				uint32(uint8(m.nextMonoRamped()))<<8 |
				uint32(uint8(m.nextMonoRamped()))<<16 |
				uint32(uint8(m.nextMonoRamped()))<<24
		}
		return
	}
	if m.musicGainSamples != 0 || m.musicGain != musicGainUnity {
		if m.musicVoices == m.active {
			if m.musicGainSamples > uint32(len(dst)*4) && m.hasOnlyPlainTriangles() {
				m.mixMonoPlainTriangleMusicRampedWords(dst)
				return
			}
			for i := range dst {
				dst[i] = uint32(uint8(m.nextMonoMusicRamped())) |
					uint32(uint8(m.nextMonoMusicRamped()))<<8 |
					uint32(uint8(m.nextMonoMusicRamped()))<<16 |
					uint32(uint8(m.nextMonoMusicRamped()))<<24
			}
		} else if m.musicGainSamples > uint32(len(dst)*4) && m.packSynthMusicFirst() {
			m.mixMonoPackedMusicWords(dst)
		} else {
			for i := range dst {
				dst[i] = uint32(uint8(m.nextMonoMixedMusicRamped())) |
					uint32(uint8(m.nextMonoMixedMusicRamped()))<<8 |
					uint32(uint8(m.nextMonoMixedMusicRamped()))<<16 |
					uint32(uint8(m.nextMonoMixedMusicRamped()))<<24
			}
		}
		return
	}
	if m.active == 0 {
		for i := range dst {
			dst[i] = 0
		}
		return
	}
	if m.hasOnlyPlainTriangles() {
		m.mixMonoPlainTriangleWords(dst)
		return
	}
	if m.hasOnlyPlainOrgans() {
		m.mixMonoPlainOrganWords(dst)
		return
	}
	if m.hasOnlyPlainPICOSynths() {
		m.mixMonoPlainPICOSynthWords(dst)
		return
	}
	if m.hasOnlyTriangleVariants() {
		m.mixMonoTriangleVariantWords(dst)
		return
	}
	for i := range dst {
		dst[i] = uint32(uint8(m.nextMono())) |
			uint32(uint8(m.nextMono()))<<8 |
			uint32(uint8(m.nextMono()))<<16 |
			uint32(uint8(m.nextMono()))<<24
	}
}

// MixStereo writes separate signed 8-bit left and right target streams. Only
// the shared prefix advances mixer state; any unmatched destination tail is
// cleared. Destinations must not overlap.
func (m *Mixer) MixStereo(left, right []int8) {
	n := min(len(left), len(right))
	if m == nil {
		n = 0
	}
	musicGainActive := m != nil && (m.musicGainSamples != 0 || m.musicGain != musicGainUnity)
	if m != nil && m.hasFilterWork() {
		for index := 0; index < n; index++ {
			pair := m.nextStereoFiltered()
			left[index] = int8(uint8(pair))
			right[index] = int8(uint8(pair >> 8))
		}
	} else if m != nil && m.ramps == 0 && m.attacks == 0 && musicGainActive && m.musicVoices == m.active {
		for index := 0; index < n; index++ {
			pair := m.nextStereoMusicRamped()
			left[index] = int8(uint8(pair))
			right[index] = int8(uint8(pair >> 8))
		}
	} else if m != nil && (m.ramps != 0 || m.attacks != 0 || musicGainActive) {
		for index := 0; index < n; index++ {
			pair := m.nextStereoRamped()
			left[index] = int8(uint8(pair))
			right[index] = int8(uint8(pair >> 8))
		}
	} else {
		for index := 0; index < n; index++ {
			pair := m.nextStereo()
			left[index] = int8(uint8(pair))
			right[index] = int8(uint8(pair >> 8))
		}
	}
	for index := n; index < len(left); index++ {
		left[index] = 0
	}
	for index := n; index < len(right); index++ {
		right[index] = 0
	}
}

// MixStereoWords packs four consecutive samples per word for the dual-FIFO
// backend. Left words feed FIFO A and right words feed FIFO B; unmatched
// destination tails are cleared without advancing.
//
//go:noinline
//go:section .iwram
func (m *Mixer) MixStereoWords(left, right []uint32) {
	n := min(len(left), len(right))
	if m == nil {
		n = 0
	}
	musicGainActive := m != nil && (m.musicGainSamples != 0 || m.musicGain != musicGainUnity)
	if m != nil && m.hasFilterWork() {
		for index := 0; index < n; index++ {
			a := m.nextStereoFiltered()
			b := m.nextStereoFiltered()
			c := m.nextStereoFiltered()
			d := m.nextStereoFiltered()
			left[index] = uint32(uint8(a)) |
				uint32(uint8(b))<<8 |
				uint32(uint8(c))<<16 |
				uint32(uint8(d))<<24
			right[index] = uint32(uint8(a>>8)) |
				uint32(uint8(b>>8))<<8 |
				uint32(uint8(c>>8))<<16 |
				uint32(uint8(d>>8))<<24
		}
	} else if m != nil && m.ramps == 0 && m.attacks == 0 && musicGainActive && m.musicVoices == m.active {
		for index := 0; index < n; index++ {
			a := m.nextStereoMusicRamped()
			b := m.nextStereoMusicRamped()
			c := m.nextStereoMusicRamped()
			d := m.nextStereoMusicRamped()
			left[index] = uint32(uint8(a)) |
				uint32(uint8(b))<<8 |
				uint32(uint8(c))<<16 |
				uint32(uint8(d))<<24
			right[index] = uint32(uint8(a>>8)) |
				uint32(uint8(b>>8))<<8 |
				uint32(uint8(c>>8))<<16 |
				uint32(uint8(d>>8))<<24
		}
	} else if m != nil && (m.ramps != 0 || m.attacks != 0 || musicGainActive) {
		for index := 0; index < n; index++ {
			a := m.nextStereoRamped()
			b := m.nextStereoRamped()
			c := m.nextStereoRamped()
			d := m.nextStereoRamped()
			left[index] = uint32(uint8(a)) |
				uint32(uint8(b))<<8 |
				uint32(uint8(c))<<16 |
				uint32(uint8(d))<<24
			right[index] = uint32(uint8(a>>8)) |
				uint32(uint8(b>>8))<<8 |
				uint32(uint8(c>>8))<<16 |
				uint32(uint8(d>>8))<<24
		}
	} else {
		for index := 0; index < n; index++ {
			a := m.nextStereo()
			b := m.nextStereo()
			c := m.nextStereo()
			d := m.nextStereo()
			left[index] = uint32(uint8(a)) |
				uint32(uint8(b))<<8 |
				uint32(uint8(c))<<16 |
				uint32(uint8(d))<<24
			right[index] = uint32(uint8(a>>8)) |
				uint32(uint8(b>>8))<<8 |
				uint32(uint8(c>>8))<<16 |
				uint32(uint8(d>>8))<<24
		}
	}
	for index := n; index < len(left); index++ {
		left[index] = 0
	}
	for index := n; index < len(right); index++ {
		right[index] = 0
	}
}
