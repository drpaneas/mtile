package audio

import "github.com/drpaneas/mtile/runtime/hw"

const (
	NativePPQN             = 96
	NativeDefaultTempo     = 120
	NativeMaxSounds        = 256
	NativeMaxClips         = 256
	NativeMaxParts         = 128
	NativeMaxSongs         = 64
	NativeMaxEvents        = 8192
	NativeMaxCommandQueue  = 32
	NativeMaxSequenceParts = 128
	NativeMaxMacroSteps    = 64
	// NativeEditorNotePriority is the priority Music Lab writes for new notes.
	NativeEditorNotePriority = 128
	// NativeDefaultSFXPriority is used when Lua calls sfx(id) without a second
	// argument. It sits above NativeEditorNotePriority so a gameplay effect
	// replaces an active Music Lab song note on that channel.
	NativeDefaultSFXPriority = 160
	// NativeMaxPartBars keeps one continuous performance in one reusable part
	// while bounding authored tick positions and note durations to target-safe
	// integer ranges. At 120 BPM this is more than four minutes of recording.
	NativeMaxPartBars = 128
	// NativeMacroClockCycles is one original Game Boy display frame. The
	// stock GB Studio player calls hUGEDriver once per frame, so instrument
	// subpatterns advance at 4,194,304 / 70,224 ~= 59.73 Hz.
	NativeMacroClockCycles = 280896
	// NativeServicePeriodCycles is one 256 Hz Timer 0 slot at the GBA CPU
	// clock: 16,777,216 / 256 = 65,536. NativeServiceCycleBudget is the
	// documented per-IRQ CPU bound; docs/audio/performance.md records a
	// 11,439-cycle measured maximum under this 16,384-cycle budget.
	NativeServicePeriodCycles = 65536
	NativeServiceCycleBudget  = 16384
)

// Profile is one measured native GBA playback configuration.
type Profile uint8

const (
	ProfilePSG Profile = iota
)

func (p Profile) Valid() bool { return p == ProfilePSG }

// SoundKind identifies the one hardware family a sound may configure.
type SoundKind uint8

const (
	SoundCH1 SoundKind = iota
	SoundCH2
	SoundCH3
	SoundCH4
)

// Sound is a fixed tagged union. Exactly the field selected by Kind is used.
// IDs are stable asset IDs, not slice positions.
type Sound struct {
	ID     uint8
	Kind   SoundKind
	Pulse1 hw.Pulse1Config
	Pulse2 hw.PulseConfig
	Wave   hw.WaveConfig
	Noise  hw.NoiseConfig
	Macro  SoundMacro
}

// SoundMacroStep is one hUGEDriver-compatible instrument subpattern row.
// PeriodDelta implements the 1xx/2xx pitch effects used by the stock duty
// presets. HasNoise replaces NR43's divisor/shift fields without retriggering
// CH4, matching a noise-table note update.
type SoundMacroStep struct {
	PeriodDelta     int16
	NoiseDivisor    uint8
	NoiseShift      uint8
	HasNoise        bool
	NoiseNoteOffset int8
	HasNoiseNote    bool
}

// SoundMacro is optional per-tick hardware automation attached to a sound.
// LoopStart is -1 for a one-shot table or an index into Steps.
type SoundMacro struct {
	Steps     []SoundMacroStep
	LoopStart int8
}

// NoteEvent is one bounded PSG note in a typed clip.
type NoteEvent struct {
	Tick          uint32
	DurationTicks uint16
	Note          uint8
	SoundID       uint8
	Priority      uint8
	Legato        bool
}

// Clip targets exactly one of the four PSG hardware channels.
type Clip struct {
	ID          uint8
	Kind        SoundKind
	LengthTicks uint32
	NoteEvents  []NoteEvent
}

// Part assigns at most one clip to each fixed PSG channel. A lane value of -1
// is empty.
type Part struct {
	LengthTicks uint32
	PSGClips    [4]int16
	// Tempo is zero when the part inherits its song tempo.
	Tempo uint16
	ID    uint8
}

// Song is an ordered arrangement of part IDs. LoopStart is -1 for an ending
// song or an index into Sequence.
type Song struct {
	ID        uint8
	Tempo     uint16
	Sequence  []uint8
	LoopStart int16
}

// Library is generated static target data. Bind validates it before any
// command can touch hardware.
type Library struct {
	Sounds []Sound
	Clips  []Clip
	Parts  []Part
	Songs  []Song
}

type NativeOwner uint8

const (
	OwnerNone NativeOwner = iota
	OwnerSong
	OwnerSFX
	OwnerDirect
)

// ChannelState is register-equivalent scheduler state for one fixed PSG
// channel. It is also the shared preview/playhead truth.
type ChannelState struct {
	Active       bool
	Owner        NativeOwner
	Priority     uint8
	Note         uint8
	SoundID      uint8
	StopTick     uint32
	HasStop      bool
	StartedTick  uint32
	Replacements uint32
}

// SFXPlaybackState is the bounded playhead for one hardware channel's effect
// clip. At most one effect clip can run per channel.
type SFXPlaybackState struct {
	Active   bool
	ClipID   uint8
	Tick     uint32
	Priority uint8
}

// NativeStats contains deterministic scheduler and ownership telemetry.
type NativeStats struct {
	ClockRunning      bool
	RunningSong       bool
	SongID            uint8
	SequenceIndex     uint8
	PartID            uint8
	PartTick          uint32
	AbsoluteTick      uint32
	ServiceCalls      uint32
	MeasuredServices  uint32
	LastServiceCycles uint32
	MinServiceCycles  uint32
	MeanServiceCycles uint32
	MaxServiceCycles  uint32
	EventsApplied     uint32
	CommandsRejected  uint32
	QueueOverflows    uint32
	InvalidAssets     uint32
	ChannelReplaces   uint32
	PriorityRejects   uint32
	CompletedSongs    uint32
	Profile           Profile
	CommitRateMilliHz uint32
	Channels          [4]ChannelState
	SFX               [4]SFXPlaybackState
}

// NativePreview is the complete editor-visible playback snapshot.
type NativePreview struct {
	Stats   NativeStats
	PSG     hw.PSGSnapshot
	Control hw.SoundControlSnapshot
}
