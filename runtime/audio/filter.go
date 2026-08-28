package audio

const (
	// PICO-8's captured reverb delays are 366 and 732 samples at 22,050 Hz.
	// At the fixed 16,384 Hz target clock those delays round to 272 and 544
	// samples, preserving the physical 16.60/33.20 ms echo cadence.
	sfxReverbDelayOne = 272
	sfxReverbDelayTwo = 544
	// A producer rewind can move from three generated pages ahead to the page
	// after next. Retaining one full stream horizon in addition to the longest
	// delay keeps every pre-rewind feedback sample available without copying
	// the history into each page checkpoint.
	sfxReverbHistorySamples = sfxReverbDelayTwo + StreamBlockSamples*StreamPageCount
	sfxReverbFractionBits   = 6
	sfxReverbFeedbackOneQ6  = 33
	sfxReverbFeedbackTwoQ6  = 32
	sfxDampenFractionBits   = 12
	sfxDampenUnity          = 1 << sfxDampenFractionBits
	sfxDampenOnePole        = 1611
	sfxDampenTwoPole        = 2781
)

// sfxFilterState belongs to one logical SFX channel, not one oscillator
// voice. A definition can therefore move between notes without cutting its
// reverb history, and the tail remains audible after the final voice stops.
// The ring stores Q6 pre-clamp filtered contribution because PICO applies
// feedback at higher precision than the final mixed-output saturation. Q6
// retains the measured decay while keeping every channel history in int16.
type sfxFilterState struct {
	reverbBuffer   [sfxReverbHistorySamples]int16
	reverbPosition uint16
	reverbNonZero  uint16
	reverbSamples  uint16
	dampenValue    int32
	filter         SFXFilter
	reverbFeedback uint8
	pan            int16
	reverbDelay    uint16
	dampenPole     uint16
	music          bool
}

type sfxFilterCheckpoint struct {
	reverbPosition uint16
	reverbNonZero  uint16
	reverbSamples  uint16
	dampenValue    int32
	filter         SFXFilter
	pan            int16
	music          bool
}

func statefulSFXFilter(filter SFXFilter) bool {
	return filter.Reverb() != 0 || filter.Dampen() != 0
}

func (state *sfxFilterState) configure(filter SFXFilter, music bool, pan int16) {
	// Keep the append-only history and cursor. reverbSamples prevents stale
	// values from a previous definition from being read before the new delay
	// has elapsed, so starting an SFX is constant-time on target.
	state.reverbNonZero = 0
	state.reverbSamples = 0
	state.dampenValue = 0
	state.filter = filter
	state.reverbDelay = 0
	state.reverbFeedback = 0
	switch filter.Reverb() {
	case 1:
		state.reverbDelay = sfxReverbDelayOne
		state.reverbFeedback = sfxReverbFeedbackOneQ6
	case 2:
		state.reverbDelay = sfxReverbDelayTwo
		state.reverbFeedback = sfxReverbFeedbackTwoQ6
	}
	state.dampenPole = 0
	switch filter.Dampen() {
	case 1:
		state.dampenPole = sfxDampenOnePole
	case 2:
		state.dampenPole = sfxDampenTwoPole
	}
	state.music = music
	state.pan = pan
}

func (state *sfxFilterState) checkpoint() sfxFilterCheckpoint {
	return sfxFilterCheckpoint{
		reverbPosition: state.reverbPosition,
		reverbNonZero:  state.reverbNonZero,
		reverbSamples:  state.reverbSamples,
		dampenValue:    state.dampenValue,
		filter:         state.filter,
		pan:            state.pan,
		music:          state.music,
	}
}

func (state *sfxFilterState) restore(checkpoint sfxFilterCheckpoint) {
	state.reverbPosition = checkpoint.reverbPosition
	state.reverbNonZero = checkpoint.reverbNonZero
	state.reverbSamples = checkpoint.reverbSamples
	state.dampenValue = checkpoint.dampenValue
	state.filter = checkpoint.filter
	state.reverbDelay = 0
	state.reverbFeedback = 0
	switch checkpoint.filter.Reverb() {
	case 1:
		state.reverbDelay = sfxReverbDelayOne
		state.reverbFeedback = sfxReverbFeedbackOneQ6
	case 2:
		state.reverbDelay = sfxReverbDelayTwo
		state.reverbFeedback = sfxReverbFeedbackTwoQ6
	}
	state.dampenPole = 0
	switch checkpoint.filter.Dampen() {
	case 1:
		state.dampenPole = sfxDampenOnePole
	case 2:
		state.dampenPole = sfxDampenTwoPole
	}
	state.pan = checkpoint.pan
	state.music = checkpoint.music
}

func (state *sfxFilterState) hasWork() bool {
	return state != nil && (state.reverbNonZero != 0 || state.dampenValue != 0)
}

//go:noinline
//go:section .iwram
func (state *sfxFilterState) process(input int32) int32 {
	if state == nil {
		return input
	}
	output := input
	delay := state.reverbDelay
	if delay != 0 {
		position := state.reverbPosition
		delayed := int16(0)
		if state.reverbSamples >= delay {
			delayedPosition := int(position) - int(delay)
			if delayedPosition < 0 {
				delayedPosition += sfxReverbHistorySamples
			}
			delayed = state.reverbBuffer[delayedPosition]
		}
		feedbackQ6 := int32(delayed) * int32(state.reverbFeedback) / (1 << sfxReverbFractionBits)
		outputQ6 := input<<sfxReverbFractionBits + feedbackQ6
		if delayed != 0 {
			state.reverbNonZero--
		}
		stored := int16(outputQ6)
		state.reverbBuffer[position] = stored
		if stored != 0 {
			state.reverbNonZero++
		}
		position++
		if position == sfxReverbHistorySamples {
			position = 0
		}
		state.reverbPosition = position
		if state.reverbSamples < delay {
			state.reverbSamples++
		}
		if outputQ6 >= 0 {
			output = (outputQ6 + 1<<(sfxReverbFractionBits-1)) >> sfxReverbFractionBits
		} else {
			output = -((-outputQ6 + 1<<(sfxReverbFractionBits-1)) >> sfxReverbFractionBits)
		}
	}

	pole := int32(state.dampenPole)
	if pole == 0 {
		return output
	}
	// The filtered input is at most one signed 8-bit voice contribution.
	// Both weighted Q12 terms therefore fit int32 exactly, including their
	// sum. Avoiding target-emulated int64 multiplication cuts the dampen hot
	// path substantially on ARM7TDMI while retaining bit-identical arithmetic.
	value := (output<<sfxDampenFractionBits)*(sfxDampenUnity-pole) +
		state.dampenValue*pole
	if value >= 0 {
		state.dampenValue = (value + 1<<(sfxDampenFractionBits-1)) >> sfxDampenFractionBits
	} else {
		state.dampenValue = -((-value + 1<<(sfxDampenFractionBits-1)) >> sfxDampenFractionBits)
	}
	if output == 0 && state.dampenValue >= -1 && state.dampenValue <= 1 {
		state.dampenValue = 0
	}
	if state.dampenValue >= 0 {
		return (state.dampenValue + 1<<(sfxDampenFractionBits-1)) >> sfxDampenFractionBits
	}
	return -((-state.dampenValue + 1<<(sfxDampenFractionBits-1)) >> sfxDampenFractionBits)
}
