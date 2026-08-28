package audio

const (
	SFXChannels                    = 4
	MaxSFXDefinitions              = 64
	MaxSFXSteps                    = 32
	MaxCustomInstrumentDepth       = 2
	NoSFXLoop                uint8 = 0xff

	// PICO-8 exports exactly 183 samples at 22,050 Hz per speed unit. The
	// 16,384 Hz target clock represents that duration with a rational
	// accumulator; 136 is the rounded first-unit duration for callers that
	// need a display value rather than a cumulative boundary.
	SFXSpeedUnitSamples      uint32 = 136
	sfxSpeedNumerator        uint64 = 499712
	sfxSpeedDenominator      uint64 = 3675
	sfxSpeedWholeSamples     uint32 = 135
	sfxSpeedRemainder        uint32 = 3587
	sfxSpeedInitialRemainder uint16 = 1837
	SFXPriority              uint8  = 128
	SFXCommandCapacity              = 8
	MaxMusicPatterns                = 64
	NoMusicPattern           int8   = -1
	MaxMusicFadeMS                  = 32767
	MusicSampleRate                 = 16384
	MaxSampleDefinitions            = 64
)

type SFXEffect uint8

// SFXFilter is PICO-8's compact per-SFX filter byte with the editor-mode bit
// removed. Keeping the native representation makes generated ROM records
// compact and lets each DSP stage decode only the setting it needs.
type SFXFilter uint8

const (
	SFXFilterNoiz SFXFilter = 2
	SFXFilterBuzz SFXFilter = 4
	maxSFXFilter  SFXFilter = 214
)

func validSFXFilter(filter SFXFilter) bool {
	return filter&1 == 0 && filter <= maxSFXFilter
}

func (filter SFXFilter) Detune() uint8 { return uint8(filter/8) % 3 }
func (filter SFXFilter) Reverb() uint8 { return uint8(filter/24) % 3 }
func (filter SFXFilter) Dampen() uint8 { return uint8(filter/72) % 3 }

const (
	SFXEffectNone SFXEffect = iota
	// SFXEffectSlide linearly approaches the current step's pitch and volume
	// from the previous note at the exact SFX tick cadence.
	SFXEffectSlide
	// SFXEffectVibrato applies a 16-tick, quarter-tone pitch LFO without
	// retriggering the oscillator.
	SFXEffectVibrato
	// SFXEffectDrop linearly lowers oscillator frequency toward zero over the
	// current step without changing volume or retriggering the oscillator.
	SFXEffectDrop
	// SFXEffectFadeIn raises volume sample-continuously from silence to the
	// authored level over the current step.
	SFXEffectFadeIn
	// SFXEffectFadeOut lowers volume sample-continuously from the authored
	// level to silence over the current step.
	SFXEffectFadeOut
	// SFXEffectArpeggioFast selects pitch from the current aligned four-step
	// group every four ticks, or every two ticks when speed is at most eight.
	SFXEffectArpeggioFast
	// SFXEffectArpeggioSlow uses the same pitch-only sequencer at an eight-tick
	// cadence, or four ticks when speed is at most eight.
	SFXEffectArpeggioSlow
)

// sfxVibratoDeltaQ12 is PICO's 16-tick triangle LFO in Q12. Its extrema are
// the linear-frequency offsets produced by +/-0.25 of one semitone's ratio
// (approximately +/-25 cents). Target updates need only one bounded multiply.
var sfxVibratoDeltaQ12 = [...]int8{
	0, 15, 30, 46, 61, 46, 30, 15,
	0, -15, -30, -46, -61, -46, -30, -15,
}

// SFXDefinition is the compact generated-ROM description of one effect.
// StepOffset addresses the shared step array. Loop bounds are half-open and
// relative to this definition; both are NoSFXLoop when looping is disabled.
type SFXDefinition struct {
	StepOffset uint16
	ID         uint8
	Speed      uint8
	StepCount  uint8
	LoopStart  uint8
	LoopEnd    uint8
	Filters    SFXFilter
}

type SFXStep struct {
	PhaseStep uint32
	Volume    uint16
	Kind      SourceKind
	Effect    SFXEffect
}

func customInstrumentID(kind SourceKind) (int, bool) {
	if kind < SourceCustom0 || kind > SourceCustom7 {
		return 0, false
	}
	return int(kind - SourceCustom0), true
}

// MusicPattern is the compact generated-ROM description of one PICO-style
// pattern. Each channel is an SFX ID or -1 for no music assignment. Next is a
// stable pattern ID or -1 to stop after DurationSamples.
type MusicPattern struct {
	DurationSamples uint32
	Channels        [SFXChannels]int8
	ID              uint8
	Next            int8
}

// SampleDefinition addresses one target-ready signed 8-bit PCM range in the
// generated shared sample data. Loop points are half-open and relative to the
// range; both are zero when Loop is false.
type SampleDefinition struct {
	DataOffset uint32
	DataLength uint32
	LoopStart  uint32
	LoopEnd    uint32
	Volume     uint16
	ID         uint8
	Priority   uint8
	Loop       bool
}

type sfxChannel struct {
	samplesRemaining       uint32
	effectSamplesRemaining uint32
	voiceStarted           uint32
	started                uint32
	definition             uint8
	step                   uint8
	start                  uint8
	end                    uint8
	timelineTick           uint8
	effectTick             uint8
	previousPhaseStep      uint32
	previousVolume         uint16
	clockRemainder         uint16
	effectClockRemainder   uint16
	voiceID                int8
	customDepth            uint8
	active                 bool
	music                  bool
	custom                 [MaxCustomInstrumentDepth]customSFXState
}

// customSFXState is one independently clocked SFX-instrument layer beneath a
// logical channel. The corpus requires at most two layers and Init rejects
// deeper or cyclic graphs, keeping checkpoint size and target work bounded.
type customSFXState struct {
	samplesRemaining       uint32
	effectSamplesRemaining uint32
	previousPhaseStep      uint32
	previousVolume         uint16
	clockRemainder         uint16
	effectClockRemainder   uint16
	definition             uint8
	step                   uint8
	timelineTick           uint8
	effectTick             uint8
	configured             bool
	active                 bool
}

func (state *customSFXState) nextEffectTickSamples() uint32 {
	samples, remainder := sfxClockSamples(1, state.effectClockRemainder)
	state.effectClockRemainder = remainder
	return samples
}

func sfxClockSamples(units uint32, remainder uint16) (uint32, uint16) {
	// 499712/3675 is exactly 135 + 3587/3675. Splitting the
	// quotient keeps the hot custom-instrument boundary clock on 32-bit
	// arithmetic; ARM7TDMI otherwise pays for a software uint64 divide on
	// every inner step. The maximum supported 8,160 units stays well inside
	// uint32 for both terms.
	fraction := units*sfxSpeedRemainder + uint32(remainder)
	denominator := uint32(sfxSpeedDenominator)
	return units*sfxSpeedWholeSamples + fraction/denominator, uint16(fraction % denominator)
}

// SFXSamplesForUnits returns the nearest cumulative target-sample boundary
// for PICO-8 speed units measured from a newly started SFX timeline.
func SFXSamplesForUnits(units uint32) uint32 {
	samples, _ := sfxClockSamples(units, sfxSpeedInitialRemainder)
	return samples
}

func (channel *sfxChannel) nextEffectTickSamples() uint32 {
	samples, remainder := sfxClockSamples(1, channel.effectClockRemainder)
	channel.effectClockRemainder = remainder
	return samples
}

type sfxCommandKind uint8

const (
	sfxCommandPlay sfxCommandKind = iota
	sfxCommandStop
	sfxCommandMusic
	sfxCommandSample
	sfxCommandStopSamples
)

type sfxCommand struct {
	fadeMS   uint16
	id       int8
	channel  int8
	offset   uint8
	length   uint8
	kind     sfxCommandKind
	priority uint8
}

type musicState struct {
	samplesRemaining     uint32
	fadeSamplesRemaining uint32
	pattern              uint8
	active               bool
	fadingOut            bool
}

// sfxCheckpoint captures the complete speculative producer timeline before a
// physical stream page. Definitions, pending commands, and cumulative command
// telemetry are deliberately not part of the checkpoint.
type sfxCheckpoint struct {
	mixer    Mixer
	channels [SFXChannels]sfxChannel
	filters  [SFXChannels]sfxFilterCheckpoint
	music    musicState
	started  uint32
	valid    bool
}

type SFXStats struct {
	ActiveChannels            uint8
	PendingCommands           uint8
	Starts                    uint32
	Stops                     uint32
	InvalidCalls              uint32
	FailedVoiceStarts         uint32
	QueuedCommands            uint32
	DroppedCommands           uint32
	FallbackCommands          uint32
	Rewinds                   uint32
	MusicActive               bool
	MusicPattern              uint8
	MusicStarts               uint32
	MusicStops                uint32
	MusicTransitions          uint32
	InvalidMusicCalls         uint32
	MusicFadeSamplesRemaining uint32
	MusicFadingOut            bool
	ActiveSampleVoices        uint8
	SampleStarts              uint32
	SampleStops               uint32
	FailedSampleStarts        uint32
	InvalidSampleCalls        uint32
}

type SFXChannelState struct {
	Active           bool
	SFXID            uint8
	Step             uint8
	Effect           SFXEffect
	EffectTick       uint8
	VoiceID          int8
	SamplesRemaining uint32
	Music            bool
}

type MusicState struct {
	Active               bool
	PatternID            uint8
	SamplesRemaining     uint32
	FadeSamplesRemaining uint32
	FadingOut            bool
}

// SFXPlayer is a fixed four-channel, sample-clocked PICO-style sequencer.
// It owns no sample buffer and allocates nothing; MonoStream asks it to mix
// complete target pages through the shared Mixer.
type SFXPlayer struct {
	mixer        *Mixer
	masterVolume uint16
	definitions  []SFXDefinition
	steps        []SFXStep
	patterns     []MusicPattern
	samples      []SampleDefinition
	samplePCM    []int8
	channels     [SFXChannels]sfxChannel
	channelPan   [SFXChannels]int16
	filters      [SFXChannels]sfxFilterState
	music        musicState
	commands     [SFXCommandCapacity]sfxCommand
	checkpoints  [StreamPageCount]sfxCheckpoint

	commandCount uint8
	deferred     bool

	started            uint32
	starts             uint32
	stops              uint32
	invalidCalls       uint32
	failedVoiceStarts  uint32
	queuedCommands     uint32
	droppedCommands    uint32
	fallbackCommands   uint32
	rewinds            uint32
	musicStarts        uint32
	musicStops         uint32
	musicTransitions   uint32
	invalidMusicCalls  uint32
	sampleStarts       uint32
	sampleStops        uint32
	failedSampleStarts uint32
	invalidSampleCalls uint32
}

func validSFXDefinition(definition SFXDefinition, stepN int) bool {
	if definition.Speed == 0 || definition.StepCount == 0 || definition.StepCount > MaxSFXSteps || !validSFXFilter(definition.Filters) {
		return false
	}
	end := uint32(definition.StepOffset) + uint32(definition.StepCount)
	if end > uint32(stepN) {
		return false
	}
	if definition.LoopStart == NoSFXLoop || definition.LoopEnd == NoSFXLoop {
		return definition.LoopStart == NoSFXLoop && definition.LoopEnd == NoSFXLoop
	}
	return definition.LoopStart < definition.LoopEnd && definition.LoopEnd <= definition.StepCount
}

func validSFXStep(step SFXStep) bool {
	if _, custom := customInstrumentID(step.Kind); custom {
		return step.PhaseStep != 0 && step.Volume <= 7 && step.Effect <= SFXEffectArpeggioSlow
	}
	return step.PhaseStep != 0 && step.Volume <= MaxVolume && modulatableSynthKind(step.Kind) && step.Effect <= SFXEffectArpeggioSlow
}

func validateCustomInstrumentGraph(definitions []SFXDefinition, steps []SFXStep) bool {
	find := func(id int) int {
		for index := range definitions {
			if int(definitions[index].ID) == id {
				return index
			}
		}
		return -1
	}
	for _, step := range steps {
		instrument, custom := customInstrumentID(step.Kind)
		if custom && step.Volume != 0 && find(instrument) < 0 {
			return false
		}
	}
	// An ordinary note may enter one custom SFX, whose notes may enter one
	// more. A custom reference in that second definition would require a third
	// layer (or form a cycle), both of which are rejected deterministically.
	for instrument := 0; instrument < 8; instrument++ {
		definitionIndex := find(instrument)
		if definitionIndex < 0 {
			continue
		}
		definition := definitions[definitionIndex]
		for offset := 0; offset < int(definition.StepCount); offset++ {
			step := steps[int(definition.StepOffset)+offset]
			target, custom := customInstrumentID(step.Kind)
			if !custom || step.Volume == 0 {
				continue
			}
			targetIndex := find(target)
			if targetIndex < 0 || target == instrument {
				return false
			}
			targetDefinition := definitions[targetIndex]
			for child := 0; child < int(targetDefinition.StepCount); child++ {
				childStep := steps[int(targetDefinition.StepOffset)+child]
				if _, nested := customInstrumentID(childStep.Kind); nested && childStep.Volume != 0 {
					return false
				}
			}
		}
	}
	return true
}

func (p *SFXPlayer) Init(mixer *Mixer, definitions []SFXDefinition, steps []SFXStep) bool {
	if p == nil || mixer == nil || mixer.voiceLimit < SFXChannels || len(definitions) > MaxSFXDefinitions {
		return false
	}
	var seen uint64
	previousID := -1
	for _, definition := range definitions {
		if int(definition.ID) <= previousID || definition.ID >= MaxSFXDefinitions ||
			!validSFXDefinition(definition, len(steps)) {
			return false
		}
		bit := uint64(1) << definition.ID
		if seen&bit != 0 {
			return false
		}
		seen |= bit
		previousID = int(definition.ID)
	}
	for _, step := range steps {
		if !validSFXStep(step) {
			return false
		}
	}
	if !validateCustomInstrumentGraph(definitions, steps) {
		return false
	}
	*p = SFXPlayer{mixer: mixer, masterVolume: MaxVolume, definitions: definitions, steps: steps}
	if !mixer.attachSFXFilters(&p.filters) {
		return false
	}
	for index := range p.channels {
		p.channels[index].voiceID = -1
	}
	return true
}

// SetMasterVolume configures a fixed project-wide 0..256 gain before audio
// starts. It scales voice starts and note-volume changes, so unity adds no
// work to the per-sample mixer loops. Once voices or deferred stream pages
// exist, changing it would split the timeline and is rejected.
func (p *SFXPlayer) SetMasterVolume(volume uint16) bool {
	if p == nil || p.mixer == nil || volume > MaxVolume || p.deferred || p.mixer.Stats().ActiveVoices != 0 {
		return false
	}
	p.masterVolume = volume
	return true
}

func (p *SFXPlayer) scaledVolume(volume uint16) uint16 {
	return uint16((uint32(volume)*uint32(p.masterVolume) + uint32(MaxVolume)/2) / uint32(MaxVolume))
}

// SetChannelPan configures persistent stereo placement for one logical
// SFX/music channel. It is a low-level engine setting, not yet a Lua or editor
// API. Configuration is rejected after a stream enters deferred mode so
// already-generated pages cannot disagree with future pages.
func (p *SFXPlayer) SetChannelPan(channel int, pan int16) bool {
	if p == nil || p.mixer == nil || p.deferred || channel < 0 || channel >= SFXChannels ||
		pan < MinPan || pan > MaxPan {
		return false
	}
	state := &p.channels[channel]
	if state.voiceID >= 0 && !p.mixer.SetVoicePan(int(state.voiceID), state.voiceStarted, pan) {
		return false
	}
	p.channelPan[channel] = pan
	p.filters[channel].pan = pan
	return true
}

// InitMusic validates and attaches generated music patterns. It is separate
// from Init so SFX-only cartridges and existing host users keep the small API.
func (p *SFXPlayer) InitMusic(patterns []MusicPattern) bool {
	if p == nil || p.mixer == nil || len(patterns) > MaxMusicPatterns {
		return false
	}
	previousID := -1
	for _, pattern := range patterns {
		if int(pattern.ID) <= previousID || pattern.ID >= MaxMusicPatterns {
			return false
		}
		previousID = int(pattern.ID)
		for _, id := range pattern.Channels {
			if id != -1 && p.findDefinition(int(id)) < 0 {
				return false
			}
		}
	}
	for _, pattern := range patterns {
		if pattern.Next != NoMusicPattern && findMusicPattern(patterns, int(pattern.Next)) < 0 {
			return false
		}
	}
	p.patterns = patterns
	p.music = musicState{}
	p.mixer.SetMusicVolume(MaxVolume)
	return true
}

// InitSamples validates and attaches target-ready PCM records. It remains
// separate from Init so SFX/music-only cartridges retain the same state path.
func (p *SFXPlayer) InitSamples(definitions []SampleDefinition, pcm []int8) bool {
	if p == nil || p.mixer == nil || len(definitions) > MaxSampleDefinitions || uint64(len(pcm)) > uint64(^uint32(0)) {
		return false
	}
	if len(definitions) == 0 {
		if len(pcm) != 0 {
			return false
		}
		p.samples = nil
		p.samplePCM = nil
		return true
	}
	previousID := -1
	for _, definition := range definitions {
		end := uint64(definition.DataOffset) + uint64(definition.DataLength)
		if int(definition.ID) <= previousID || definition.ID >= MaxSampleDefinitions ||
			definition.DataLength == 0 || end > uint64(len(pcm)) || definition.Volume > MaxVolume {
			return false
		}
		if definition.Loop {
			if definition.LoopStart >= definition.LoopEnd || definition.LoopEnd > definition.DataLength ||
				definition.LoopEnd-definition.LoopStart < 2 {
				return false
			}
		} else if definition.LoopStart != 0 || definition.LoopEnd != 0 {
			return false
		}
		previousID = int(definition.ID)
	}
	p.samples = definitions
	p.samplePCM = pcm
	return true
}

func findMusicPattern(patterns []MusicPattern, id int) int {
	for index := range patterns {
		candidate := int(patterns[index].ID)
		if candidate == id {
			return index
		}
		if candidate > id {
			break
		}
	}
	return -1
}

func (p *SFXPlayer) findMusicPattern(id int) int {
	if p == nil {
		return -1
	}
	return findMusicPattern(p.patterns, id)
}

func (p *SFXPlayer) findDefinition(id int) int {
	for index := range p.definitions {
		candidate := int(p.definitions[index].ID)
		if candidate == id {
			return index
		}
		if candidate > id {
			break
		}
	}
	return -1
}

func (p *SFXPlayer) findSampleDefinition(id int) int {
	for index := range p.samples {
		candidate := int(p.samples[index].ID)
		if candidate == id {
			return index
		}
		if candidate > id {
			break
		}
	}
	return -1
}

func (p *SFXPlayer) stopOwnedVoice(channel *sfxChannel) {
	if p == nil || p.mixer == nil || channel.voiceID < 0 {
		return
	}
	voice, ok := p.mixer.Voice(int(channel.voiceID))
	if ok && voice.Active && voice.Started == channel.voiceStarted {
		p.mixer.Stop(int(channel.voiceID))
	}
	channel.voiceID = -1
	channel.voiceStarted = 0
}

// releaseOwnedVoice leaves the completed oscillator alive for PICO-8's
// measured 64-at-22,050-Hz release tail. Generated cartridges reserve four
// transient slots, one per logical channel. Legacy four/six-voice low-level
// profiles retain their original immediate-stop behavior.
func (p *SFXPlayer) releaseOwnedVoice(channel *sfxChannel) {
	if p == nil || p.mixer == nil || channel.voiceID < 0 {
		return
	}
	voiceID := int(channel.voiceID)
	voice, ok := p.mixer.Voice(voiceID)
	if ok && voice.Active && voice.Started == channel.voiceStarted &&
		voice.Volume != 0 && p.mixer.voiceLimit >= SFXMixerVoices &&
		p.mixer.StartVoiceRelease(voiceID, channel.voiceStarted) {
		channel.voiceID = -1
		channel.voiceStarted = 0
		return
	}
	p.stopOwnedVoice(channel)
}

// continueOwnedVoice updates a compatible oscillator in place. PICO-8 keeps
// one oscillator phase per logical channel across ordinary note cells and
// explicit sfx() retriggers; restarting phase zero at every cell turns the
// short edge blend into an audible periodic pulse on sustained phrases.
func (p *SFXPlayer) continueOwnedVoice(channel *sfxChannel, kind SourceKind, filters SFXFilter, phaseStep uint32, volume uint16) (bool, uint16) {
	if p == nil || p.mixer == nil || channel == nil || channel.voiceID < 0 {
		return false, 0
	}
	voiceID := int(channel.voiceID)
	voice, ok := p.mixer.Voice(voiceID)
	if !ok || !voice.Active || voice.Started != channel.voiceStarted ||
		voice.Kind != kind || voice.Filter != filters || voice.Music != channel.music ||
		!p.mixer.SetVoiceStepVolume(voiceID, channel.voiceStarted, phaseStep, volume) {
		return false, 0
	}
	return true, voice.Volume
}

func (p *SFXPlayer) stopChannel(index int) bool {
	if p == nil || index < 0 || index >= SFXChannels || !p.channels[index].active {
		return false
	}
	channel := &p.channels[index]
	p.releaseOwnedVoice(channel)
	*channel = sfxChannel{voiceID: -1}
	p.stops++
	return true
}

func (p *SFXPlayer) stopChannelImmediate(index int) bool {
	if p == nil || index < 0 || index >= SFXChannels || !p.channels[index].active {
		return false
	}
	channel := &p.channels[index]
	p.stopOwnedVoice(channel)
	*channel = sfxChannel{voiceID: -1}
	p.stops++
	return true
}

func (p *SFXPlayer) stopNow(channel int) bool {
	if p == nil {
		return false
	}
	if channel == -1 {
		stopped := false
		for index := range p.channels {
			stopped = p.stopChannel(index) || stopped
		}
		return stopped
	}
	if channel < 0 || channel >= SFXChannels {
		p.invalidCalls++
		return false
	}
	return p.stopChannel(channel)
}

func (p *SFXPlayer) enqueue(command sfxCommand) bool {
	if p.commandCount >= SFXCommandCapacity {
		p.droppedCommands++
		return false
	}
	p.commands[p.commandCount] = command
	p.commandCount++
	p.queuedCommands++
	return true
}

// Stop releases one logical channel. Once a MonoStream is running, stop
// commands are deferred to the bounded page-rewind service point.
func (p *SFXPlayer) Stop(channel int) bool {
	if p == nil || channel < -1 || channel >= SFXChannels {
		if p != nil {
			p.invalidCalls++
		}
		return false
	}
	if p.deferred {
		return p.enqueue(sfxCommand{kind: sfxCommandStop, channel: int8(channel)})
	}
	return p.stopNow(channel)
}

func (p *SFXPlayer) chooseChannel() int {
	for index := range p.channels {
		if !p.channels[index].active {
			return index
		}
	}
	oldest := 0
	for index := 1; index < len(p.channels); index++ {
		if older(p.channels[index].started, p.channels[oldest].started) {
			oldest = index
		}
	}
	return oldest
}

func combineSFXFilters(left, right SFXFilter) SFXFilter {
	noiz := left&SFXFilterNoiz != 0 || right&SFXFilterNoiz != 0
	buzz := left&SFXFilterBuzz != 0 || right&SFXFilterBuzz != 0
	detune := left.Detune()
	if right.Detune() > detune {
		detune = right.Detune()
	}
	reverb := left.Reverb()
	if right.Reverb() > reverb {
		reverb = right.Reverb()
	}
	dampen := left.Dampen()
	if right.Dampen() > dampen {
		dampen = right.Dampen()
	}
	filter := SFXFilter(detune*8 + reverb*24 + dampen*72)
	if noiz {
		filter += SFXFilterNoiz
	}
	if buzz {
		filter += SFXFilterBuzz
	}
	return filter
}

func customEffectActive(effect SFXEffect) bool {
	// Drop is the custom-instrument retrigger inversion control, not a pitch
	// trajectory, when it appears on a custom note.
	return effect == SFXEffectSlide || effect == SFXEffectVibrato ||
		effect == SFXEffectFadeIn || effect == SFXEffectFadeOut || isSFXArpeggio(effect)
}

func ordinaryEffectActive(effect SFXEffect) bool {
	return effect == SFXEffectSlide || effect == SFXEffectVibrato || effect == SFXEffectDrop || isSFXArpeggio(effect)
}

func (p *SFXPlayer) controlledStep(
	definition SFXDefinition,
	stepIndex uint8,
	previousPhaseStep uint32,
	previousVolume uint16,
	timelineTick uint8,
	effectTick uint8,
	custom bool,
) (uint32, uint32) {
	step := p.steps[int(definition.StepOffset)+int(stepIndex)]
	phaseStep := step.PhaseStep
	// Keep eight fractional bits while custom-instrument envelopes are
	// composed.  Custom-note volumes are only 0..7, so interpolating a quiet
	// fade directly in that authored range would round most of the note to
	// exact silence before the child instrument gain is applied.
	volume := uint32(step.Volume) << 8
	switch step.Effect {
	case SFXEffectSlide:
		phaseStep = slidePhaseStep(previousPhaseStep, step.PhaseStep, effectTick, definition.Speed)
		if previousVolume != 0 {
			volume = lerpU32(uint32(previousVolume)<<8, uint32(step.Volume)<<8, effectTick, definition.Speed)
		}
	case SFXEffectVibrato:
		phaseStep = vibratoPhaseStep(step.PhaseStep, effectTick)
	case SFXEffectDrop:
		if !custom {
			phaseStep = dropPhaseStep(step.PhaseStep, effectTick, definition.Speed)
		}
	case SFXEffectFadeIn:
		volume = lerpU32(0, uint32(step.Volume)<<8, effectTick, definition.Speed)
	case SFXEffectFadeOut:
		volume = lerpU32(uint32(step.Volume)<<8, 0, effectTick, definition.Speed)
	case SFXEffectArpeggioFast, SFXEffectArpeggioSlow:
		member := ((timelineTick + effectTick) >> arpeggioCadenceShift(step.Effect, definition.Speed)) & 3
		groupStep := (stepIndex &^ 3) + member
		if groupStep < definition.StepCount {
			phaseStep = p.steps[int(definition.StepOffset)+int(groupStep)].PhaseStep
		}
	}
	return phaseStep, volume
}

func (p *SFXPlayer) clearCustomFrom(channel *sfxChannel, depth int) {
	if depth < 0 {
		depth = 0
	}
	if depth > MaxCustomInstrumentDepth {
		depth = MaxCustomInstrumentDepth
	}
	for index := depth; index < MaxCustomInstrumentDepth; index++ {
		channel.custom[index] = customSFXState{}
	}
	if int(channel.customDepth) > depth {
		channel.customDepth = uint8(depth)
	}
}

func customRetrigger(previousPhaseStep uint32, previousVolume uint16, step SFXStep, configured bool) bool {
	retrigger := !configured || previousVolume == 0 || previousPhaseStep != step.PhaseStep
	if step.Effect == SFXEffectDrop {
		return !retrigger
	}
	return retrigger
}

func (p *SFXPlayer) customVoiceControl(index int) (SourceKind, uint32, uint16, SFXFilter, bool) {
	channel := &p.channels[index]
	if channel.customDepth == 0 {
		return SourceNone, 0, 0, 0, false
	}
	rootDefinition := p.definitions[channel.definition]
	rootStep := p.steps[int(rootDefinition.StepOffset)+int(channel.step)]
	if _, custom := customInstrumentID(rootStep.Kind); !custom || rootStep.Volume == 0 {
		return SourceNone, 0, 0, 0, false
	}
	filters := rootDefinition.Filters
	deepest := int(channel.customDepth) - 1
	for depth := 0; depth <= deepest; depth++ {
		state := &channel.custom[depth]
		if !state.configured || !state.active {
			return SourceNone, 0, 0, filters, false
		}
		filters = combineSFXFilters(filters, p.definitions[state.definition].Filters)
	}
	deepState := &channel.custom[deepest]
	deepDefinition := p.definitions[deepState.definition]
	deepStep := p.steps[int(deepDefinition.StepOffset)+int(deepState.step)]
	if _, custom := customInstrumentID(deepStep.Kind); custom || deepStep.Volume == 0 {
		return SourceNone, 0, 0, filters, false
	}
	phaseStep, volume := p.controlledStep(
		deepDefinition, deepState.step, deepState.previousPhaseStep, deepState.previousVolume,
		deepState.timelineTick, deepState.effectTick, false,
	)
	for depth := deepest - 1; depth >= 0; depth-- {
		state := &channel.custom[depth]
		definition := p.definitions[state.definition]
		customPhase, customVolume := p.controlledStep(
			definition, state.step, state.previousPhaseStep, state.previousVolume,
			state.timelineTick, state.effectTick, true,
		)
		phaseStep = transposedPICOPhaseStep(phaseStep, customPhase)
		volume = uint32((uint64(volume)*uint64(customVolume) + 7*128) / (7 * 256))
	}
	rootPhase, rootVolume := p.controlledStep(
		rootDefinition, channel.step, channel.previousPhaseStep, channel.previousVolume,
		channel.timelineTick, channel.effectTick, true,
	)
	phaseStep = transposedPICOPhaseStep(phaseStep, rootPhase)
	volume = uint32((uint64(volume)*uint64(rootVolume) + 7*128) / (7 * 256))
	return deepStep.Kind, phaseStep, p.scaledVolume(uint16((volume + 128) >> 8)), filters, true
}

func (p *SFXPlayer) updateCustomVoice(index int, restart bool) {
	channel := &p.channels[index]
	kind, phaseStep, volume, filters, ok := p.customVoiceControl(index)
	if !ok {
		p.stopOwnedVoice(channel)
		return
	}
	if !restart && channel.voiceID >= 0 {
		voice, owned := p.mixer.Voice(int(channel.voiceID))
		if owned && voice.Active && voice.Started == channel.voiceStarted && voice.Kind == kind && voice.Filter == filters {
			if p.mixer.SetVoiceStepVolume(int(channel.voiceID), channel.voiceStarted, phaseStep, volume) {
				return
			}
		}
	}
	p.releaseOwnedVoice(channel)
	if p.filters[index].filter != filters {
		p.filters[index].configure(filters, channel.music, p.channelPan[index])
	}
	filterChannel := uint8(0)
	if statefulSFXFilter(filters) {
		filterChannel = uint8(index + 1)
	}
	voiceID, started := p.mixer.Start(Request{
		Kind: kind, Step: phaseStep, Volume: volume, Pan: p.channelPan[index], Priority: SFXPriority, Music: channel.music,
		Filter: filters, NoiseSeed: uint16(1 + int(p.definitions[channel.definition].ID)*SFXChannels + index + int(channel.step)),
		filterChannel: filterChannel,
	})
	if !started {
		p.failedVoiceStarts++
		return
	}
	channel.voiceID = int8(voiceID)
	voice, _ := p.mixer.Voice(voiceID)
	channel.voiceStarted = voice.Started
	if !p.mixer.StartVoiceAttack(voiceID, channel.voiceStarted) {
		p.stopOwnedVoice(channel)
		p.failedVoiceStarts++
	}
}

func (p *SFXPlayer) beginCustomLayerStep(index, depth int, forceRetrigger bool) {
	channel := &p.channels[index]
	state := &channel.custom[depth]
	definition := p.definitions[state.definition]
	step := p.steps[int(definition.StepOffset)+int(state.step)]
	state.effectClockRemainder = state.clockRemainder
	state.samplesRemaining, state.clockRemainder = sfxClockSamples(uint32(definition.Speed), state.clockRemainder)
	state.effectSamplesRemaining = 0
	state.effectTick = 0
	if (customEffectActive(step.Effect) && step.Kind >= SourceCustom0) ||
		(ordinaryEffectActive(step.Effect) && step.Kind < SourceCustom0) ||
		step.Effect == SFXEffectFadeIn || step.Effect == SFXEffectFadeOut {
		state.effectSamplesRemaining = state.nextEffectTickSamples()
	}
	instrument, custom := customInstrumentID(step.Kind)
	if !custom {
		p.clearCustomFrom(channel, depth+1)
		// PICO keeps the channel oscillator phase across consecutive notes of
		// the same built-in waveform inside a custom instrument. Updating the
		// owned voice avoids a click at every short instrument-envelope step;
		// updateCustomVoice still replaces it when the waveform or filters
		// actually change.
		p.updateCustomVoice(index, false)
		return
	}
	if step.Volume == 0 {
		p.clearCustomFrom(channel, depth+1)
		p.stopOwnedVoice(channel)
		return
	}
	deeperConfigured := depth+1 < int(channel.customDepth) && channel.custom[depth+1].configured &&
		p.definitions[channel.custom[depth+1].definition].ID == uint8(instrument)
	retrigger := forceRetrigger || customRetrigger(state.previousPhaseStep, state.previousVolume, step, deeperConfigured)
	if retrigger {
		p.initCustomLayer(index, depth+1, instrument)
		return
	}
	p.updateCustomVoice(index, false)
}

func (p *SFXPlayer) initCustomLayer(index, depth, instrument int) {
	channel := &p.channels[index]
	if depth < 0 || depth >= MaxCustomInstrumentDepth {
		p.clearCustomFrom(channel, depth)
		p.stopOwnedVoice(channel)
		return
	}
	definitionIndex := p.findDefinition(instrument)
	if definitionIndex < 0 {
		p.clearCustomFrom(channel, depth)
		p.stopOwnedVoice(channel)
		return
	}
	p.clearCustomFrom(channel, depth)
	channel.custom[depth] = customSFXState{
		definition: uint8(definitionIndex), previousPhaseStep: picoPitchPhaseSteps[24],
		clockRemainder: sfxSpeedInitialRemainder, configured: true, active: true,
	}
	channel.customDepth = uint8(depth + 1)
	p.beginCustomLayerStep(index, depth, true)
}

func (p *SFXPlayer) startOuterCustomStep(index int, step SFXStep) {
	channel := &p.channels[index]
	if step.Volume == 0 {
		p.clearCustomFrom(channel, 0)
		p.stopOwnedVoice(channel)
		return
	}
	instrument, _ := customInstrumentID(step.Kind)
	configured := channel.customDepth != 0 && channel.custom[0].configured &&
		p.definitions[channel.custom[0].definition].ID == uint8(instrument)
	if customRetrigger(channel.previousPhaseStep, channel.previousVolume, step, configured) {
		p.initCustomLayer(index, 0, instrument)
		return
	}
	p.updateCustomVoice(index, false)
}

func (p *SFXPlayer) startStep(index int) {
	channel := &p.channels[index]
	definition := p.definitions[channel.definition]
	step := p.steps[int(definition.StepOffset)+int(channel.step)]
	channel.effectClockRemainder = channel.clockRemainder
	channel.samplesRemaining, channel.clockRemainder = sfxClockSamples(uint32(definition.Speed), channel.clockRemainder)
	channel.effectSamplesRemaining = 0
	channel.effectTick = 0
	if _, custom := customInstrumentID(step.Kind); custom {
		if customEffectActive(step.Effect) {
			channel.effectSamplesRemaining = channel.nextEffectTickSamples()
		}
		p.startOuterCustomStep(index, step)
		return
	}
	p.clearCustomFrom(channel, 0)
	stepVolume := p.scaledVolume(step.Volume)
	if stepVolume == 0 {
		p.releaseOwnedVoice(channel)
		return
	}
	phaseStep := step.PhaseStep
	slides := step.Effect == SFXEffectSlide
	arpeggio := isSFXArpeggio(step.Effect)
	effectActive := slides || step.Effect == SFXEffectVibrato || step.Effect == SFXEffectDrop || arpeggio
	if slides {
		phaseStep = channel.previousPhaseStep
		if channel.previousVolume != 0 {
			stepVolume = p.scaledVolume(channel.previousVolume)
		}
	} else if step.Effect == SFXEffectVibrato {
		phaseStep = vibratoPhaseStep(step.PhaseStep, 0)
	} else if arpeggio {
		phaseStep = p.arpeggioPhaseStep(channel, definition, step, channel.timelineTick)
	}
	continued, previousVoiceVolume := false, uint16(0)
	// Fade-in deliberately starts a second oscillator at silence so the old
	// note can release underneath it. Other built-in cells retain channel
	// phase; volume-only edges use a short in-place ramp below.
	if step.Effect != SFXEffectFadeIn {
		continued, previousVoiceVolume = p.continueOwnedVoice(channel, step.Kind, definition.Filters, phaseStep, stepVolume)
	}
	if !continued {
		p.releaseOwnedVoice(channel)
	}
	filterChannel := uint8(0)
	if statefulSFXFilter(definition.Filters) {
		filterChannel = uint8(index + 1)
	}
	voiceID := int(channel.voiceID)
	if !continued {
		var ok bool
		voiceID, ok = p.mixer.Start(Request{
			Kind: step.Kind, Step: phaseStep, Volume: stepVolume, Pan: p.channelPan[index], Priority: SFXPriority, Music: channel.music,
			Filter:        definition.Filters,
			NoiseSeed:     uint16(1 + int(definition.ID)*SFXChannels + index + int(channel.step)),
			filterChannel: filterChannel,
		})
		if !ok {
			p.failedVoiceStarts++
			return
		}
		channel.voiceID = int8(voiceID)
		voice, _ := p.mixer.Voice(voiceID)
		channel.voiceStarted = voice.Started
		if !p.mixer.StartVoiceAttack(voiceID, channel.voiceStarted) {
			p.stopOwnedVoice(channel)
			p.failedVoiceStarts++
			return
		}
	} else if step.Effect == SFXEffectFadeOut && previousVoiceVolume == 0 {
		// Consecutive fade-out cells jump from the preceding cell's zero target
		// back to the authored level. Keep phase but reopen the edge smoothly.
		if !p.mixer.StartVoiceAttack(voiceID, channel.voiceStarted) {
			p.stopOwnedVoice(channel)
			p.failedVoiceStarts++
			return
		}
	} else if step.Effect != SFXEffectFadeOut && previousVoiceVolume != stepVolume {
		if !p.mixer.SetVoiceVolumeRamp(voiceID, channel.voiceStarted, previousVoiceVolume, stepVolume, SFXEdgeSamples) {
			p.stopOwnedVoice(channel)
			p.failedVoiceStarts++
			return
		}
	}
	if step.Effect == SFXEffectFadeIn || step.Effect == SFXEffectFadeOut {
		from, to := uint16(0), stepVolume
		if step.Effect == SFXEffectFadeOut {
			from, to = stepVolume, 0
		}
		if !p.mixer.SetVoiceVolumeRamp(
			voiceID, channel.voiceStarted, from, to, channel.samplesRemaining,
		) {
			p.stopOwnedVoice(channel)
			p.failedVoiceStarts++
			return
		}
	}
	if effectActive {
		channel.effectSamplesRemaining = channel.nextEffectTickSamples()
	}
}

func (p *SFXPlayer) nextStep(channel *sfxChannel, definition SFXDefinition) (int, bool) {
	next := int(channel.step) + 1
	if definition.LoopStart != NoSFXLoop && next == int(definition.LoopEnd) &&
		int(definition.LoopStart) >= int(channel.start) && int(definition.LoopEnd) <= int(channel.end) {
		next = int(definition.LoopStart)
	}
	return next, next < int(channel.end)
}

func slidePhaseStep(from, to uint32, tick, ticks uint8) uint32 {
	if ticks == 0 || tick >= ticks {
		return to
	}
	// PICO interpolates the numeric key, not oscillator frequency. Recover the
	// nearest authored endpoints, interpolate in Q8 semitones, then use the two
	// surrounding captured pitch steps. Linear interpolation within one
	// semitone stays within two cents of the exponential ideal and avoids a
	// runtime floating-point or division-heavy power function.
	fromKey := picoPitchKey(from)
	toKey := picoPitchKey(to)
	position := ((fromKey*int(ticks) + (toKey-fromKey)*int(tick)) << 8) / int(ticks)
	key := position >> 8
	fraction := uint32(position & 0xff)
	if key >= len(picoPitchPhaseSteps)-1 {
		return picoPitchPhaseSteps[len(picoPitchPhaseSteps)-1]
	}
	low := picoPitchPhaseSteps[key]
	high := picoPitchPhaseSteps[key+1]
	return low + uint32((uint64(high-low)*uint64(fraction)+128)>>8)
}

func lerpU16(from, to uint16, tick, ticks uint8) uint16 {
	return uint16(int32(from) + (int32(to)-int32(from))*int32(tick)/int32(ticks))
}

func lerpU32(from, to uint32, tick, ticks uint8) uint32 {
	return uint32(int64(from) + (int64(to)-int64(from))*int64(tick)/int64(ticks))
}

func vibratoPhaseStep(base uint32, tick uint8) uint32 {
	delta := int32(sfxVibratoDeltaQ12[tick&15])
	if delta == 0 {
		return base
	}
	if delta > 0 {
		adjustment := (base >> 6) * uint32(delta) >> 6
		if ^uint32(0)-base < adjustment {
			return ^uint32(0)
		}
		return base + adjustment
	}
	adjustment := (base >> 6) * uint32(-delta) >> 6
	return base - adjustment
}

func dropPhaseStep(base uint32, tick, duration uint8) uint32 {
	if duration == 0 || tick >= duration {
		return 0
	}
	// PICO's drop follows a linear frequency ramp over the note duration. The
	// division happens only at the exact rational-clock effect boundary; truncating before
	// multiplying introduces less than duration phase units of error.
	return (base / uint32(duration)) * uint32(duration-tick)
}

func isSFXArpeggio(effect SFXEffect) bool {
	return effect == SFXEffectArpeggioFast || effect == SFXEffectArpeggioSlow
}

func arpeggioCadenceShift(effect SFXEffect, speed uint8) uint8 {
	shift := uint8(2) // fast: 1 << 2 = 4 ticks
	if effect == SFXEffectArpeggioSlow {
		shift = 3 // slow: 1 << 3 = 8 ticks
	}
	if speed <= 8 {
		shift--
	}
	return shift
}

func (p *SFXPlayer) arpeggioPhaseStep(channel *sfxChannel, definition SFXDefinition, current SFXStep, tick uint8) uint32 {
	member := (tick >> arpeggioCadenceShift(current.Effect, definition.Speed)) & 3
	groupStep := (channel.step &^ 3) + member
	if groupStep >= definition.StepCount {
		// The authored schema may end with an incomplete four-step group. PICO
		// always has 32 slots; keep the current pitch when that source slot does
		// not exist in the compact Mtile definition.
		return current.PhaseStep
	}
	return p.steps[int(definition.StepOffset)+int(groupStep)].PhaseStep
}

func (p *SFXPlayer) advanceEffect(index int) {
	channel := &p.channels[index]
	if !channel.active || channel.samplesRemaining == 0 || channel.effectSamplesRemaining != 0 {
		return
	}
	definition := p.definitions[channel.definition]
	step := p.steps[int(definition.StepOffset)+int(channel.step)]
	if _, custom := customInstrumentID(step.Kind); custom {
		channel.effectTick++
		if isSFXArpeggio(step.Effect) {
			shift := arpeggioCadenceShift(step.Effect, definition.Speed)
			if (channel.timelineTick+channel.effectTick)&((1<<shift)-1) != 0 {
				channel.effectSamplesRemaining = channel.nextEffectTickSamples()
				return
			}
		}
		p.updateCustomVoice(index, false)
		channel.effectSamplesRemaining = channel.nextEffectTickSamples()
		return
	}
	if channel.voiceID < 0 {
		return
	}
	var phaseStep uint32
	volume := step.Volume
	switch step.Effect {
	case SFXEffectSlide:
		channel.effectTick++
		phaseStep = slidePhaseStep(channel.previousPhaseStep, step.PhaseStep, channel.effectTick, definition.Speed)
		if channel.previousVolume != 0 {
			volume = lerpU16(channel.previousVolume, step.Volume, channel.effectTick, definition.Speed)
		}
	case SFXEffectVibrato:
		channel.effectTick++
		phaseStep = vibratoPhaseStep(step.PhaseStep, channel.effectTick)
	case SFXEffectDrop:
		channel.effectTick++
		phaseStep = dropPhaseStep(step.PhaseStep, channel.effectTick, definition.Speed)
	case SFXEffectArpeggioFast, SFXEffectArpeggioSlow:
		channel.effectTick++
		timelineTick := channel.timelineTick + channel.effectTick
		shift := arpeggioCadenceShift(step.Effect, definition.Speed)
		if timelineTick&((1<<shift)-1) != 0 {
			channel.effectSamplesRemaining = channel.nextEffectTickSamples()
			return
		}
		phaseStep = p.arpeggioPhaseStep(channel, definition, step, timelineTick)
	default:
		return
	}
	volume = p.scaledVolume(volume)
	if !p.mixer.SetVoiceStepVolume(int(channel.voiceID), channel.voiceStarted, phaseStep, volume) {
		channel.voiceID = -1
		channel.voiceStarted = 0
		return
	}
	channel.effectSamplesRemaining = channel.nextEffectTickSamples()
}

func (p *SFXPlayer) advanceCustomLayer(index, depth int) {
	channel := &p.channels[index]
	if depth >= int(channel.customDepth) {
		return
	}
	state := &channel.custom[depth]
	if !state.configured || !state.active || state.samplesRemaining != 0 {
		return
	}
	definition := p.definitions[state.definition]
	next := int(state.step) + 1
	if definition.LoopStart != NoSFXLoop && next == int(definition.LoopEnd) {
		next = int(definition.LoopStart)
	}
	if next >= int(definition.StepCount) {
		state.active = false
		p.clearCustomFrom(channel, depth+1)
		p.stopOwnedVoice(channel)
		return
	}
	step := p.steps[int(definition.StepOffset)+int(state.step)]
	state.previousPhaseStep = step.PhaseStep
	state.previousVolume = step.Volume
	state.timelineTick += definition.Speed
	state.step = uint8(next)
	p.beginCustomLayerStep(index, depth, false)
}

func (p *SFXPlayer) advanceDueCustomLayers() {
	for index := range p.channels {
		for depth := 0; depth < MaxCustomInstrumentDepth; depth++ {
			p.advanceCustomLayer(index, depth)
		}
	}
}

func (p *SFXPlayer) advanceCustomEffect(index, depth int) {
	channel := &p.channels[index]
	if depth >= int(channel.customDepth) {
		return
	}
	state := &channel.custom[depth]
	if !state.configured || !state.active || state.samplesRemaining == 0 || state.effectSamplesRemaining != 0 {
		return
	}
	definition := p.definitions[state.definition]
	step := p.steps[int(definition.StepOffset)+int(state.step)]
	state.effectTick++
	if isSFXArpeggio(step.Effect) {
		shift := arpeggioCadenceShift(step.Effect, definition.Speed)
		if (state.timelineTick+state.effectTick)&((1<<shift)-1) != 0 {
			state.effectSamplesRemaining = state.nextEffectTickSamples()
			return
		}
	}
	p.updateCustomVoice(index, false)
	state.effectSamplesRemaining = state.nextEffectTickSamples()
}

func (p *SFXPlayer) advanceDueCustomEffects() {
	for index := range p.channels {
		for depth := 0; depth < MaxCustomInstrumentDepth; depth++ {
			p.advanceCustomEffect(index, depth)
		}
	}
}

func (p *SFXPlayer) validatePlay(id, channel, offset, length int) (int, bool) {
	if p == nil || p.mixer == nil || channel < -1 || channel >= SFXChannels || offset < 0 || length < 0 {
		if p != nil {
			p.invalidCalls++
		}
		return -1, false
	}
	definitionIndex := p.findDefinition(id)
	if definitionIndex < 0 {
		p.invalidCalls++
		return -1, false
	}
	definition := p.definitions[definitionIndex]
	if offset >= int(definition.StepCount) {
		p.invalidCalls++
		return -1, false
	}
	if length > 0 {
		end := offset + length
		if end < offset || end > int(definition.StepCount) {
			p.invalidCalls++
			return -1, false
		}
	}
	return definitionIndex, true
}

func (p *SFXPlayer) playNowOwned(definitionIndex, channel, offset, length int, music bool) (int, bool) {
	definition := p.definitions[definitionIndex]
	end := int(definition.StepCount)
	if length > 0 {
		end = offset + length
	}
	if channel == -1 {
		channel = p.chooseChannel()
	}
	voiceID := int8(-1)
	voiceStarted := uint32(0)
	current := &p.channels[channel]
	if current.active {
		if current.voiceID >= 0 {
			voice, ok := p.mixer.Voice(int(current.voiceID))
			if ok && voice.Active && voice.Started == current.voiceStarted {
				voiceID = current.voiceID
				voiceStarted = current.voiceStarted
			}
		}
		p.stops++
	}
	p.filters[channel].configure(definition.Filters, music, p.channelPan[channel])
	p.started++
	if p.started == 0 {
		p.started = 1
	}
	p.channels[channel] = sfxChannel{
		definition: uint8(definitionIndex), step: uint8(offset), start: uint8(offset), end: uint8(end),
		previousPhaseStep: picoPitchPhaseSteps[24], previousVolume: 0,
		clockRemainder: sfxSpeedInitialRemainder,
		voiceID:        voiceID, voiceStarted: voiceStarted,
		active: true, started: p.started, music: music,
	}
	p.starts++
	p.startStep(channel)
	return channel, true
}

func (p *SFXPlayer) playNow(definitionIndex, channel, offset, length int) (int, bool) {
	return p.playNowOwned(definitionIndex, channel, offset, length, false)
}

// Play starts an effect by stable ID. Channel -1 chooses the first inactive
// logical channel, then the oldest. Length 0 means all remaining steps.
// ID -1 stops the requested channel; channel -1 with ID -1 stops all. Once a
// MonoStream is running, valid calls are queued and applied to a bounded
// page-after-next rewind instead of mutating already-generated future audio.
func (p *SFXPlayer) Play(id, channel, offset, length int) (int, bool) {
	if p == nil || p.mixer == nil {
		return -1, false
	}
	if id == -1 {
		return channel, p.Stop(channel)
	}
	definitionIndex, ok := p.validatePlay(id, channel, offset, length)
	if !ok {
		return -1, false
	}
	if p.deferred {
		command := sfxCommand{kind: sfxCommandPlay, id: int8(id), channel: int8(channel), offset: uint8(offset), length: uint8(length)}
		return channel, p.enqueue(command)
	}
	return p.playNow(definitionIndex, channel, offset, length)
}

func (p *SFXPlayer) stopMusicNow() bool {
	if p == nil || !p.music.active {
		return false
	}
	fadedToSilence := p.music.fadingOut
	for index := range p.channels {
		if p.channels[index].music {
			if fadedToSilence {
				p.stopChannelImmediate(index)
			} else {
				p.stopChannel(index)
			}
		}
	}
	p.music = musicState{}
	p.mixer.SetMusicVolume(MaxVolume)
	p.musicStops++
	return true
}

func (p *SFXPlayer) startMusicPattern(patternIndex int, preserveMatching bool) {
	pattern := p.patterns[patternIndex]
	p.music.active = true
	p.music.pattern = uint8(patternIndex)
	p.music.samplesRemaining = pattern.DurationSamples
	for channel, id := range pattern.Channels {
		if id < 0 {
			if p.channels[channel].music {
				p.stopChannel(channel)
			}
			continue
		}
		definitionIndex := p.findDefinition(int(id))
		if preserveMatching {
			current := &p.channels[channel]
			if current.active && current.music {
				if p.definitions[current.definition].ID == uint8(id) {
					definition := p.definitions[current.definition]
					if definition.LoopStart != NoSFXLoop {
						// PICO lets an unchanged looping assignment retain its
						// independent clock across music rows. Finite assignments are
						// triggered again at every row boundary, even when they still
						// have authored steps remaining.
						continue
					}
				}
			}
		}
		p.playNowOwned(definitionIndex, channel, 0, 0, true)
	}
}

func (p *SFXPlayer) musicNow(patternIndex int, fadeSamples uint32) bool {
	if patternIndex < 0 {
		if fadeSamples == 0 {
			return p.stopMusicNow()
		}
		if !p.music.active {
			return false
		}
		p.music.fadeSamplesRemaining = fadeSamples
		p.music.fadingOut = true
		p.mixer.RampMusicVolume(0, fadeSamples)
		return true
	}
	p.music = musicState{}
	if fadeSamples == 0 {
		p.mixer.SetMusicVolume(MaxVolume)
	} else {
		p.mixer.SetMusicVolume(0)
		p.mixer.RampMusicVolume(MaxVolume, fadeSamples)
	}
	p.startMusicPattern(patternIndex, false)
	p.music.fadeSamplesRemaining = fadeSamples
	p.musicStarts++
	return true
}

func musicFadeSamples(fadeMS int) uint32 {
	if fadeMS <= 0 {
		return 0
	}
	// Hardware writes four samples per FIFO word. Round positive millisecond
	// lengths up to a complete word so every sequencer boundary remains valid
	// for both byte-oriented host mixing and target MixMonoWords.
	samples := (uint32(fadeMS)*MusicSampleRate + 999) / 1000
	return (samples + 3) &^ 3
}

// Music starts a stable pattern ID, or stops music with ID -1. A positive
// fade starts the new pattern at silence or lets current music reach silence
// before releasing its owned channels. Milliseconds are rounded up to a
// complete four-sample FIFO word at the fixed target rate.
func (p *SFXPlayer) Music(id, fadeMS int) bool {
	if p == nil || p.mixer == nil || fadeMS < 0 || fadeMS > MaxMusicFadeMS || id < -1 {
		if p != nil {
			p.invalidMusicCalls++
		}
		return false
	}
	patternIndex := -1
	if id >= 0 {
		patternIndex = p.findMusicPattern(id)
		if patternIndex < 0 {
			p.invalidMusicCalls++
			return false
		}
	}
	if p.deferred {
		return p.enqueue(sfxCommand{kind: sfxCommandMusic, id: int8(id), fadeMS: uint16(fadeMS)})
	}
	return p.musicNow(patternIndex, musicFadeSamples(fadeMS))
}

func (p *SFXPlayer) stopSamplesNow() bool {
	if p == nil || p.mixer == nil {
		return false
	}
	stopped := uint32(0)
	for id := 0; id < int(p.mixer.voiceLimit); id++ {
		voice, ok := p.mixer.Voice(id)
		if ok && voice.Active && voice.Kind == SourcePCM && p.mixer.Stop(id) {
			stopped++
		}
	}
	p.sampleStops += stopped
	return stopped != 0
}

func (p *SFXPlayer) sampleNow(definitionIndex int, volume uint16, priority uint8) bool {
	if p == nil || p.mixer == nil || definitionIndex < 0 || definitionIndex >= len(p.samples) {
		return false
	}
	definition := p.samples[definitionIndex]
	start := definition.DataOffset
	end := start + definition.DataLength
	_, ok := p.mixer.Start(Request{
		Kind: SourcePCM, Step: 1 << 16, Volume: p.scaledVolume(volume), Priority: priority,
		PCM: p.samplePCM[start:end], Loop: definition.Loop,
		LoopStart: definition.LoopStart, LoopEnd: definition.LoopEnd,
	})
	if !ok {
		p.failedSampleStarts++
		return false
	}
	p.sampleStarts++
	return true
}

// Sample starts target-ready PCM by stable ID. Volume and priority use -1 for
// manifest defaults; explicit values are 0..256 and 0..255. ID -1 stops all
// active PCM voices. Streamed calls share the bounded page-rewind queue.
func (p *SFXPlayer) Sample(id, volume, priority int) bool {
	if p == nil || p.mixer == nil || id < -1 || volume < -1 || volume > int(MaxVolume) || priority < -1 || priority > 255 {
		if p != nil {
			p.invalidSampleCalls++
		}
		return false
	}
	if id == -1 {
		if p.deferred {
			return p.enqueue(sfxCommand{kind: sfxCommandStopSamples})
		}
		return p.stopSamplesNow()
	}
	definitionIndex := p.findSampleDefinition(id)
	if definitionIndex < 0 {
		p.invalidSampleCalls++
		return false
	}
	definition := p.samples[definitionIndex]
	if volume < 0 {
		volume = int(definition.Volume)
	}
	if priority < 0 {
		priority = int(definition.Priority)
	}
	if p.deferred {
		return p.enqueue(sfxCommand{
			kind: sfxCommandSample, id: int8(id), fadeMS: uint16(volume), priority: uint8(priority),
		})
	}
	return p.sampleNow(definitionIndex, uint16(volume), uint8(priority))
}

func (p *SFXPlayer) hasPendingCommands() bool {
	return p != nil && p.commandCount != 0
}

// applyNonPlayCommand keeps the less common music/sample/stop dispatcher out
// of the eight-command SFX rewind path. Native sample commands expanded the
// command set, but ordinary PICO sfx() bursts should retain their measured
// bounded latency.
//
//go:noinline
func (p *SFXPlayer) applyNonPlayCommand(command *sfxCommand) {
	switch command.kind {
	case sfxCommandStop:
		p.stopNow(int(command.channel))
	case sfxCommandMusic:
		patternIndex := -1
		if command.id >= 0 {
			patternIndex = p.findMusicPattern(int(command.id))
		}
		p.musicNow(patternIndex, musicFadeSamples(int(command.fadeMS)))
	case sfxCommandSample:
		definitionIndex := p.findSampleDefinition(int(command.id))
		p.sampleNow(definitionIndex, command.fadeMS, command.priority)
	case sfxCommandStopSamples:
		p.stopSamplesNow()
	}
}

func (p *SFXPlayer) applyPendingCommands(fallback bool) int {
	if p == nil {
		return 0
	}
	count := int(p.commandCount)
	for index := 0; index < count; index++ {
		command := &p.commands[index]
		if command.kind == sfxCommandPlay {
			definitionIndex := p.findDefinition(int(command.id))
			p.playNow(definitionIndex, int(command.channel), int(command.offset), int(command.length))
		} else {
			p.applyNonPlayCommand(command)
		}
		*command = sfxCommand{}
	}
	p.commandCount = 0
	if fallback {
		p.fallbackCommands += uint32(count)
	}
	return count
}

func (p *SFXPlayer) saveCheckpoint(page uint8) bool {
	if p == nil || p.mixer == nil || page >= StreamPageCount {
		return false
	}
	checkpoint := &p.checkpoints[page]
	if !p.mixer.copyStateTo(&checkpoint.mixer) {
		return false
	}
	checkpoint.channels = p.channels
	for index := range p.filters {
		checkpoint.filters[index] = p.filters[index].checkpoint()
	}
	checkpoint.music = p.music
	checkpoint.started = p.started
	checkpoint.valid = true
	return true
}

func (p *SFXPlayer) restoreCheckpoint(page uint8) bool {
	if p == nil || p.mixer == nil || page >= StreamPageCount || !p.checkpoints[page].valid {
		return false
	}
	checkpoint := &p.checkpoints[page]
	if !checkpoint.mixer.copyStateTo(p.mixer) {
		return false
	}
	p.channels = checkpoint.channels
	for index := range p.filters {
		p.filters[index].restore(checkpoint.filters[index])
	}
	p.music = checkpoint.music
	p.started = checkpoint.started
	return true
}

func (p *SFXPlayer) advanceMusic() {
	if p == nil || !p.music.active || p.music.samplesRemaining != 0 || p.patterns[p.music.pattern].DurationSamples == 0 {
		return
	}
	pattern := p.patterns[p.music.pattern]
	if pattern.Next == NoMusicPattern {
		p.stopMusicNow()
		return
	}
	next := p.findMusicPattern(int(pattern.Next))
	p.startMusicPattern(next, true)
	p.musicTransitions++
}

func (p *SFXPlayer) advanceMusicFade() {
	if p == nil || !p.music.active || p.music.fadeSamplesRemaining != 0 {
		return
	}
	if p.music.fadingOut {
		p.stopMusicNow()
		return
	}
	p.music.fadingOut = false
}

func (p *SFXPlayer) invalidateCheckpoint(page uint8) {
	if p != nil && page < StreamPageCount {
		p.checkpoints[page].valid = false
	}
}

func (p *SFXPlayer) setDeferred(enabled bool) {
	if p == nil {
		return
	}
	p.deferred = enabled
	if enabled {
		return
	}
	if p.commandCount != 0 {
		p.droppedCommands += uint32(p.commandCount)
		for index := uint8(0); index < p.commandCount; index++ {
			p.commands[index] = sfxCommand{}
		}
		p.commandCount = 0
	}
	for index := range p.checkpoints {
		p.checkpoints[index].valid = false
	}
}

func (p *SFXPlayer) advanceChannel(index int) {
	channel := &p.channels[index]
	if !channel.active || channel.samplesRemaining != 0 {
		return
	}
	definition := p.definitions[channel.definition]
	next, ok := p.nextStep(channel, definition)
	if !ok {
		p.stopChannel(index)
		return
	}
	step := p.steps[int(definition.StepOffset)+int(channel.step)]
	channel.previousPhaseStep = step.PhaseStep
	channel.previousVolume = step.Volume
	channel.timelineTick += definition.Speed
	channel.step = uint8(next)
	p.startStep(index)
}

func (p *SFXPlayer) advanceDueChannels() {
	for index := range p.channels {
		p.advanceChannel(index)
	}
}

func (p *SFXPlayer) advanceDueEffects() {
	for index := range p.channels {
		p.advanceEffect(index)
	}
}

func (p *SFXPlayer) nextSegmentSamples(limit int) int {
	samples := limit
	if p.music.active && p.music.samplesRemaining != 0 && p.music.samplesRemaining < uint32(samples) {
		samples = int(p.music.samplesRemaining)
	}
	if p.music.active && p.music.fadeSamplesRemaining != 0 && p.music.fadeSamplesRemaining < uint32(samples) {
		samples = int(p.music.fadeSamplesRemaining)
	}
	for index := range p.channels {
		channel := &p.channels[index]
		if channel.active && channel.samplesRemaining < uint32(samples) {
			samples = int(channel.samplesRemaining)
		}
		if channel.active && channel.effectSamplesRemaining != 0 && channel.effectSamplesRemaining < uint32(samples) {
			samples = int(channel.effectSamplesRemaining)
		}
		for depth := 0; depth < int(channel.customDepth); depth++ {
			state := &channel.custom[depth]
			if state.configured && state.active && state.samplesRemaining < uint32(samples) {
				samples = int(state.samplesRemaining)
			}
			if state.configured && state.active && state.effectSamplesRemaining != 0 && state.effectSamplesRemaining < uint32(samples) {
				samples = int(state.effectSamplesRemaining)
			}
		}
	}
	return samples
}

func (p *SFXPlayer) consumeSamples(samples uint32) {
	if p.music.active {
		if p.music.samplesRemaining != 0 {
			p.music.samplesRemaining -= samples
		}
		if p.music.fadeSamplesRemaining != 0 {
			p.music.fadeSamplesRemaining -= samples
		}
	}
	for index := range p.channels {
		channel := &p.channels[index]
		if channel.active {
			channel.samplesRemaining -= samples
			if channel.effectSamplesRemaining != 0 {
				channel.effectSamplesRemaining -= samples
			}
			for depth := 0; depth < int(channel.customDepth); depth++ {
				state := &channel.custom[depth]
				if !state.configured || !state.active {
					continue
				}
				state.samplesRemaining -= samples
				if state.effectSamplesRemaining != 0 {
					state.effectSamplesRemaining -= samples
				}
			}
		}
	}
}

func (p *SFXPlayer) MixMono(dst []int8) {
	if p == nil || p.mixer == nil {
		for index := range dst {
			dst[index] = 0
		}
		return
	}
	for len(dst) > 0 {
		p.advanceMusicFade()
		p.advanceMusic()
		p.advanceDueChannels()
		p.advanceDueCustomLayers()
		p.advanceDueEffects()
		p.advanceDueCustomEffects()
		samples := p.nextSegmentSamples(len(dst))
		p.mixer.MixMono(dst[:samples])
		p.consumeSamples(uint32(samples))
		dst = dst[samples:]
	}
}

func (p *SFXPlayer) MixMonoWords(dst []uint32) {
	if p == nil || p.mixer == nil {
		for index := range dst {
			dst[index] = 0
		}
		return
	}
	for len(dst) > 0 {
		p.advanceMusicFade()
		p.advanceMusic()
		p.advanceDueChannels()
		p.advanceDueCustomLayers()
		p.advanceDueEffects()
		p.advanceDueCustomEffects()
		samples := p.nextSegmentSamples(len(dst) * 4)
		words := samples / 4
		if words != 0 {
			p.mixer.MixMonoWords(dst[:words])
			p.consumeSamples(uint32(words * 4))
			dst = dst[words:]
		}
		if samples&3 != 0 && len(dst) != 0 {
			dst[0] = p.mixMonoBoundaryWord()
			dst = dst[1:]
		}
	}
}

// mixMonoBoundaryWord bridges a sequencer boundary that is not aligned to the
// FIFO's four-sample packing unit. Exact PICO timing alternates 136- and
// 137-sample ticks, so rounding the timeline to whole words would drift while
// asking the word mixer for zero words would make no progress. This slow path
// runs only for the single word crossing a boundary; ordinary spans continue
// through MixMonoWords above.
func (p *SFXPlayer) mixMonoBoundaryWord() uint32 {
	var samples [4]int8
	filled := 0
	for filled < len(samples) {
		p.advanceMusicFade()
		p.advanceMusic()
		p.advanceDueChannels()
		p.advanceDueCustomLayers()
		p.advanceDueEffects()
		p.advanceDueCustomEffects()
		count := p.nextSegmentSamples(len(samples) - filled)
		p.mixer.MixMono(samples[filled : filled+count])
		p.consumeSamples(uint32(count))
		filled += count
	}
	return uint32(uint8(samples[0])) |
		uint32(uint8(samples[1]))<<8 |
		uint32(uint8(samples[2]))<<16 |
		uint32(uint8(samples[3]))<<24
}

// MixStereoWords advances the same sample-clocked sequencer as MixMonoWords
// while emitting synchronized FIFO A/FIFO B pages. Only the shared prefix is
// mixed; unmatched destination words are cleared without advancing time.
func (p *SFXPlayer) MixStereoWords(left, right []uint32) {
	n := min(len(left), len(right))
	if p == nil || p.mixer == nil {
		n = 0
	}
	leftMixed, rightMixed := left[:n], right[:n]
	for len(leftMixed) > 0 {
		p.advanceMusicFade()
		p.advanceMusic()
		p.advanceDueChannels()
		p.advanceDueCustomLayers()
		p.advanceDueEffects()
		p.advanceDueCustomEffects()
		samples := p.nextSegmentSamples(len(leftMixed) * 4)
		words := samples / 4
		if words != 0 {
			p.mixer.MixStereoWords(leftMixed[:words], rightMixed[:words])
			p.consumeSamples(uint32(words * 4))
			leftMixed = leftMixed[words:]
			rightMixed = rightMixed[words:]
		}
		if samples&3 != 0 && len(leftMixed) != 0 {
			leftMixed[0], rightMixed[0] = p.mixStereoBoundaryWord()
			leftMixed = leftMixed[1:]
			rightMixed = rightMixed[1:]
		}
	}
	for index := n; index < len(left); index++ {
		left[index] = 0
	}
	for index := n; index < len(right); index++ {
		right[index] = 0
	}
}

func (p *SFXPlayer) mixStereoBoundaryWord() (uint32, uint32) {
	var left, right [4]int8
	filled := 0
	for filled < len(left) {
		p.advanceMusicFade()
		p.advanceMusic()
		p.advanceDueChannels()
		p.advanceDueCustomLayers()
		p.advanceDueEffects()
		p.advanceDueCustomEffects()
		count := p.nextSegmentSamples(len(left) - filled)
		p.mixer.MixStereo(left[filled:filled+count], right[filled:filled+count])
		p.consumeSamples(uint32(count))
		filled += count
	}
	return uint32(uint8(left[0])) |
			uint32(uint8(left[1]))<<8 |
			uint32(uint8(left[2]))<<16 |
			uint32(uint8(left[3]))<<24,
		uint32(uint8(right[0])) |
			uint32(uint8(right[1]))<<8 |
			uint32(uint8(right[2]))<<16 |
			uint32(uint8(right[3]))<<24
}

func (p *SFXPlayer) Stats() SFXStats {
	if p == nil {
		return SFXStats{}
	}
	var active, activeSamples uint8
	for index := range p.channels {
		if p.channels[index].active {
			active++
		}
	}
	if p.mixer != nil {
		for id := 0; id < int(p.mixer.voiceLimit); id++ {
			voice, ok := p.mixer.Voice(id)
			if ok && voice.Active && voice.Kind == SourcePCM {
				activeSamples++
			}
		}
	}
	stats := SFXStats{
		ActiveChannels: active, PendingCommands: p.commandCount, Starts: p.starts, Stops: p.stops,
		InvalidCalls: p.invalidCalls, FailedVoiceStarts: p.failedVoiceStarts,
		QueuedCommands: p.queuedCommands, DroppedCommands: p.droppedCommands,
		FallbackCommands: p.fallbackCommands, Rewinds: p.rewinds,
		MusicActive: p.music.active, MusicStarts: p.musicStarts, MusicStops: p.musicStops,
		MusicTransitions: p.musicTransitions, InvalidMusicCalls: p.invalidMusicCalls,
		MusicFadeSamplesRemaining: p.music.fadeSamplesRemaining, MusicFadingOut: p.music.fadingOut,
		ActiveSampleVoices: activeSamples, SampleStarts: p.sampleStarts, SampleStops: p.sampleStops,
		FailedSampleStarts: p.failedSampleStarts, InvalidSampleCalls: p.invalidSampleCalls,
	}
	if p.music.active {
		stats.MusicPattern = p.patterns[p.music.pattern].ID
	}
	return stats
}

// MixerStats exposes the bounded shared voice-pool counters without exposing
// the mixer pointer itself. It is used by runtime diagnostics and stat().
func (p *SFXPlayer) MixerStats() Stats {
	if p == nil || p.mixer == nil {
		return Stats{}
	}
	return p.mixer.Stats()
}

func (p *SFXPlayer) Channel(index int) (SFXChannelState, bool) {
	if p == nil || index < 0 || index >= SFXChannels {
		return SFXChannelState{}, false
	}
	channel := p.channels[index]
	if !channel.active {
		return SFXChannelState{}, true
	}
	definition := p.definitions[channel.definition]
	step := p.steps[int(definition.StepOffset)+int(channel.step)]
	return SFXChannelState{
		Active: true, SFXID: definition.ID, Step: channel.step, Effect: step.Effect, EffectTick: channel.effectTick, VoiceID: channel.voiceID,
		SamplesRemaining: channel.samplesRemaining, Music: channel.music,
	}, true
}

func (p *SFXPlayer) MusicState() MusicState {
	if p == nil || !p.music.active {
		return MusicState{}
	}
	return MusicState{
		Active: true, PatternID: p.patterns[p.music.pattern].ID,
		SamplesRemaining: p.music.samplesRemaining, FadeSamplesRemaining: p.music.fadeSamplesRemaining,
		FadingOut: p.music.fadingOut,
	}
}
