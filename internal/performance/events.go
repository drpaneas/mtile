// Package performance turns live keyboard and MIDI gestures into exact
// Game Boy PSG channel transitions. It is an authoring layer only: cartridges
// store the resulting CH1-CH4 tracker events, never MIDI or polyphonic voices.
package performance

import "time"

type EventKind uint8

const (
	NoteOn EventKind = iota
	NoteOff
	ParameterChange
	AllNotesOff
)

type Source uint8

const (
	SourceKeyboard Source = iota
	SourceMIDI
	SourceController
)

type Parameter uint8

const (
	ParameterDuty Parameter = iota
	ParameterVolume
	ParameterEnvelopePace
	ParameterEnvelopeDirection
	ParameterSweepPace
	ParameterSweepDirection
	ParameterSweepShift
	ParameterWaveLevel
	ParameterNoiseWidth
	ParameterNoiseDivisor
	ParameterNoiseShift
)

// ParameterSemantics defines when a hardware parameter becomes audible. The
// names are deliberately part of the authoring contract: the live preview and
// the recorded tracker lane must make the same choice.
type ParameterSemantics uint8

const (
	// ParameterImmediate changes an already-running channel without restarting
	// its phase or envelope. It is reserved for controls backed by a safe PSG
	// register write; none of the current performance knobs use it yet.
	ParameterImmediate ParameterSemantics = iota
	// ParameterRetrigger restarts an active hardware channel at the automation
	// tick. This is required for waveform/polynomial changes in the current
	// bounded tracker format.
	ParameterRetrigger
	// ParameterNextNote updates the sound bank but leaves held notes untouched.
	// Envelope and sweep registers have hardware side effects when rewritten
	// live, so their deterministic meaning is "the next trigger".
	ParameterNextNote
)

func (s ParameterSemantics) String() string {
	switch s {
	case ParameterRetrigger:
		return "RETRIGGER"
	case ParameterNextNote:
		return "NEXT_NOTE"
	default:
		return "IMMEDIATE"
	}
}

// Semantics returns the deterministic PSG behavior for one authoring control.
// Duty/wave/noise shape changes are materialized as a same-pitch retrigger.
// Envelope and sweep changes are staged for the next note, avoiding the
// hardware's undocumented/variant-sensitive live-write behavior.
func (p Parameter) Semantics() ParameterSemantics {
	switch p {
	case ParameterDuty, ParameterWaveLevel, ParameterNoiseWidth, ParameterNoiseDivisor, ParameterNoiseShift:
		return ParameterRetrigger
	default:
		return ParameterNextNote
	}
}

// AppliesToChannel reports whether a parameter maps to a real register on the
// one-based physical PSG channel. The recorder uses this to avoid persisting
// synthetic no-op automation (for example CH1 sweep on CH2 or envelope on
// CH3) when one linked controller gesture targets a poly performance.
func (p Parameter) AppliesToChannel(channel uint8) bool {
	switch p {
	case ParameterDuty:
		return channel >= 1 && channel <= 3
	case ParameterVolume, ParameterEnvelopePace, ParameterEnvelopeDirection:
		return channel == 1 || channel == 2 || channel == 4
	case ParameterSweepPace, ParameterSweepDirection, ParameterSweepShift:
		return channel == 1
	case ParameterWaveLevel:
		return channel == 3
	case ParameterNoiseWidth, ParameterNoiseDivisor, ParameterNoiseShift:
		return channel == 4
	default:
		return false
	}
}

type PerformanceEvent struct {
	At        time.Duration
	Kind      EventKind
	Source    Source
	Note      uint8
	Parameter Parameter
	Value     int16
}

type VoiceMode uint8

const (
	ModeMono VoiceMode = iota
	ModePoly2
	ModePoly3
	ModeNoise
)

func (m VoiceMode) String() string {
	switch m {
	case ModePoly2:
		return "POLY2"
	case ModePoly3:
		return "POLY3"
	case ModeNoise:
		return "NOISE"
	default:
		return "MONO"
	}
}

type ActionKind uint8

const (
	ActionStart ActionKind = iota
	ActionStop
	ActionParameter
)

// ChannelAction is the physical result of one performance gesture. Channel is
// one-based (CH1=1 ... CH4=4), matching both the UI and the runtime API.
type ChannelAction struct {
	At        time.Duration
	Kind      ActionKind
	Channel   uint8
	Note      uint8
	Parameter Parameter
	Value     int16
}

const MaxActionsPerEvent = 8

// ActionBatch avoids allocation in keyboard/MIDI callbacks.
type ActionBatch struct {
	Actions [MaxActionsPerEvent]ChannelAction
	Len     uint8
}

func (b *ActionBatch) append(action ChannelAction) {
	if int(b.Len) >= len(b.Actions) {
		return
	}
	b.Actions[b.Len] = action
	b.Len++
}

func (b ActionBatch) Slice() []ChannelAction { return b.Actions[:b.Len] }

type AllocationSnapshot struct {
	Mode        VoiceMode
	MonoChannel uint8
	Armed       [4]bool
	Active      [4]bool
	Notes       [4]uint8
	Held        uint8
}
