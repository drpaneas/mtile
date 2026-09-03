package audio

import "github.com/drpaneas/mtile/runtime/hw"

type nativeCommandKind uint8

const (
	commandNote nativeCommandKind = iota
	commandStopChannel
	commandPlaySFX
	commandStopSFX
	commandPlaySong
	commandStopSong
)

type nativeCommand struct {
	kind     nativeCommandKind
	a        uint16
	b        uint16
	c        uint16
	d        uint16
	priority uint8
}

// Controller is the allocation-free native GBA event scheduler. Service is
// called at the profile's documented commit boundary and is the only method
// that commits queued public commands to hardware.
type Controller struct {
	profile Profile
	library Library
	bound   bool
	running bool

	commands    [NativeMaxCommandQueue]nativeCommand
	commandHead uint8
	commandLen  uint8

	channels [4]ChannelState

	songActive   bool
	songID       uint8
	songIndex    int16
	sequencePos  uint8
	partID       uint8
	partIndex    int16
	partTick     uint32
	absoluteTick uint32
	tempo        uint16
	clipIndex    [4]int16
	clipCursor   [4]uint16
	sfx          [4]nativeSFXPlayback
	macros       [4]nativeMacroPlayback

	// Clock accumulation is expressed in CPU cycles so both the exact 256 Hz
	// profiles and the 280,896-cycle display frame profile retain fractional
	// authoring ticks without drift.
	commitCycles  uint32
	tickRemainder uint64

	serviceCalls       uint32
	measuredServices   uint32
	lastServiceCycles  uint32
	minServiceCycles   uint32
	totalServiceCycles uint32
	maxServiceCycles   uint32
	eventsApplied      uint32
	commandsRejected   uint32
	queueOverflows     uint32
	invalidAssets      uint32
	channelReplaces    uint32
	priorityRejects    uint32
	completedSongs     uint32
}

type nativeSFXPlayback struct {
	active    bool
	clipID    uint8
	clipIndex int16
	tick      uint32
	cursor    uint16
	priority  uint8
}

type nativeMacroPlayback struct {
	active         bool
	nextStep       uint8
	period         uint16
	note           uint8
	cycleRemainder uint32
}

func profileCommitCycles(profile Profile) uint32 {
	return hw.GBAClockHz / 256
}

func profileCommitMilliHz(profile Profile) uint32 {
	cycles := profileCommitCycles(profile)
	return uint32((uint64(hw.GBAClockHz)*1000 + uint64(cycles)/2) / uint64(cycles))
}

// Init resets the controller and establishes one measured playback profile.
func (c *Controller) Init(profile Profile) bool {
	if c == nil || c.running || activeNativeController != nil && activeNativeController != c || !profile.Valid() {
		return false
	}
	*c = Controller{
		profile: profile, commitCycles: profileCommitCycles(profile), tempo: NativeDefaultTempo,
		songIndex: -1, partIndex: -1,
	}
	for i := range c.clipIndex {
		c.clipIndex[i] = -1
		c.sfx[i].clipIndex = -1
	}
	hw.StopAllPSG()
	hw.ConfigurePSGOutput(7, 7, hw.PSGMix100)
	return true
}

// Bind validates and installs generated static v2 data.
func (c *Controller) Bind(library Library) bool {
	if c == nil || c.running || !validateNativeLibrary(library) {
		if c != nil {
			c.invalidAssets++
		}
		return false
	}
	c.library = library
	c.bound = true
	return true
}

// ConfigureOutput sets the PSG terminal levels and hardware mix ratio without
// exposing shared-register writes to generated game code.
func (c *Controller) ConfigureOutput(leftLevel, rightLevel uint8, ratio hw.PSGMixRatio) bool {
	if c == nil || c.running {
		return false
	}
	return hw.ConfigurePSGOutput(leftLevel, rightLevel, ratio)
}

func validateNativeLibrary(l Library) bool {
	if len(l.Sounds) > NativeMaxSounds || len(l.Clips) > NativeMaxClips || len(l.Parts) > NativeMaxParts || len(l.Songs) > NativeMaxSongs {
		return false
	}
	events := 0
	for i := range l.Sounds {
		sound := &l.Sounds[i]
		if sound.Kind > SoundCH4 {
			return false
		}
		switch sound.Kind {
		case SoundCH1:
			if !sound.Pulse1.Pulse.Valid() || !sound.Pulse1.Sweep.Valid() {
				return false
			}
		case SoundCH2:
			if !sound.Pulse2.Valid() {
				return false
			}
		case SoundCH3:
			if !sound.Wave.Valid() {
				return false
			}
		case SoundCH4:
			if !sound.Noise.Valid() {
				return false
			}
		}
		if len(sound.Macro.Steps) > NativeMaxMacroSteps || len(sound.Macro.Steps) == 0 && sound.Macro.LoopStart != 0 ||
			len(sound.Macro.Steps) != 0 && (sound.Macro.LoopStart < -1 || int(sound.Macro.LoopStart) >= len(sound.Macro.Steps)) {
			return false
		}
		for _, step := range sound.Macro.Steps {
			if step.PeriodDelta < -2047 || step.PeriodDelta > 2047 || step.NoiseDivisor > 7 || step.NoiseShift > 15 ||
				step.NoiseNoteOffset < -36 || step.NoiseNoteOffset > 35 || step.HasNoise && step.HasNoiseNote ||
				sound.Kind == SoundCH4 && step.PeriodDelta != 0 || sound.Kind != SoundCH4 && (step.HasNoise || step.HasNoiseNote) ||
				!step.HasNoise && (step.NoiseDivisor != 0 || step.NoiseShift != 0) || !step.HasNoiseNote && step.NoiseNoteOffset != 0 {
				return false
			}
		}
		for j := 0; j < i; j++ {
			if l.Sounds[i].ID == l.Sounds[j].ID {
				return false
			}
		}
	}
	for i := range l.Clips {
		clip := &l.Clips[i]
		if clip.Kind > SoundCH4 || clip.LengthTicks == 0 {
			return false
		}
		for j := 0; j < i; j++ {
			if clip.ID == l.Clips[j].ID {
				return false
			}
		}
		var previousEnd uint32
		for j, event := range clip.NoteEvents {
			if event.Tick >= clip.LengthTicks || event.DurationTicks == 0 || event.Tick+uint32(event.DurationTicks) > clip.LengthTicks ||
				(j > 0 && event.Tick < previousEnd) {
				return false
			}
			sound := findSoundIn(l.Sounds, event.SoundID)
			if sound == nil || sound.Kind != clip.Kind || !nativeNoteValid(sound, event.Note) {
				return false
			}
			if clip.Kind == SoundCH4 && event.Legato {
				return false
			}
			previousEnd = event.Tick + uint32(event.DurationTicks)
		}
		events += len(clip.NoteEvents)
	}
	if events > NativeMaxEvents {
		return false
	}
	for i := range l.Parts {
		part := &l.Parts[i]
		if part.LengthTicks == 0 || part.Tempo != 0 && (part.Tempo < 20 || part.Tempo > 400) {
			return false
		}
		for j := 0; j < i; j++ {
			if part.ID == l.Parts[j].ID {
				return false
			}
		}
		for channel, id := range part.PSGClips {
			if id < -1 {
				return false
			}
			if id >= 0 {
				clip := findClipInOnly(l.Clips, uint8(id))
				if clip == nil || clip.Kind != SoundKind(channel) || clip.LengthTicks > part.LengthTicks {
					return false
				}
			}
		}
	}
	for i := range l.Songs {
		song := &l.Songs[i]
		if song.Tempo < 20 || song.Tempo > 400 || len(song.Sequence) == 0 || len(song.Sequence) > NativeMaxSequenceParts ||
			song.LoopStart < -1 || song.LoopStart >= int16(len(song.Sequence)) {
			return false
		}
		for j := 0; j < i; j++ {
			if song.ID == l.Songs[j].ID {
				return false
			}
		}
		for _, partID := range song.Sequence {
			if findPartIn(l.Parts, partID) == nil {
				return false
			}
		}
	}
	return true
}

// ValidateLibrary checks generated static assets without touching hardware or
// controller state. Host compilers use the same final invariant gate as Bind.
func ValidateLibrary(l Library) bool { return validateNativeLibrary(l) }

func findSoundIn(sounds []Sound, id uint8) *Sound {
	for i := range sounds {
		if sounds[i].ID == id {
			return &sounds[i]
		}
	}
	return nil
}

func findClipIn(clips []Clip, id uint8) (*Clip, int16) {
	for i := range clips {
		if clips[i].ID == id {
			return &clips[i], int16(i)
		}
	}
	return nil, -1
}

func findClipInOnly(clips []Clip, id uint8) *Clip {
	clip, _ := findClipIn(clips, id)
	return clip
}

func findPartIn(parts []Part, id uint8) *Part {
	for i := range parts {
		if parts[i].ID == id {
			return &parts[i]
		}
	}
	return nil
}

func findPartIndex(parts []Part, id uint8) int16 {
	for i := range parts {
		if parts[i].ID == id {
			return int16(i)
		}
	}
	return -1
}

func findSongIndex(songs []Song, id uint8) int16 {
	for i := range songs {
		if songs[i].ID == id {
			return int16(i)
		}
	}
	return -1
}

func nativeNoteValid(sound *Sound, note uint8) bool {
	return nativeSoundNoteValid(sound, note)
}

func (c *Controller) enqueue(command nativeCommand) bool {
	if c == nil {
		return false
	}
	state := disableStreamInterrupts()
	defer restoreStreamInterrupts(state)
	if c.commandLen == NativeMaxCommandQueue {
		if c != nil {
			c.queueOverflows++
		}
		return false
	}
	index := (int(c.commandHead) + int(c.commandLen)) & (NativeMaxCommandQueue - 1)
	c.commands[index] = command
	c.commandLen++
	return true
}

// NoteOn schedules one native channel note. durationTicks zero sustains until
// StopChannel; otherwise it is an explicit 96-PPQN duration.
func (c *Controller) NoteOn(channel, note, soundID uint8, durationTicks uint16) bool {
	if channel < 1 || channel > 4 || (channel != 4 && note > 127) {
		if c != nil {
			c.commandsRejected++
		}
		return false
	}
	return c.enqueue(nativeCommand{kind: commandNote, a: uint16(channel), b: uint16(note), c: uint16(soundID), d: durationTicks})
}

func (c *Controller) StopChannel(channel uint8) bool {
	if channel < 1 || channel > 4 {
		if c != nil {
			c.commandsRejected++
		}
		return false
	}
	return c.enqueue(nativeCommand{kind: commandStopChannel, a: uint16(channel)})
}

func (c *Controller) PlaySFX(id uint8, priority uint8) bool {
	return c.enqueue(nativeCommand{kind: commandPlaySFX, a: uint16(id), priority: priority})
}

func (c *Controller) StopSFX() bool { return c.enqueue(nativeCommand{kind: commandStopSFX}) }

func (c *Controller) PlaySong(id uint8) bool {
	return c.enqueue(nativeCommand{kind: commandPlaySong, a: uint16(id)})
}

func (c *Controller) StopSong() bool { return c.enqueue(nativeCommand{kind: commandStopSong}) }

// Service applies queued commands and advances the rational 96-PPQN clock by
// one profile commit quantum.
func (c *Controller) Service() {
	if c == nil || c.running || activeNativeController != nil {
		return
	}
	c.serviceCommit()
}

// Start claims Timer 0 and advances this controller from a 256 Hz IRQ. Audio
// v2 performs no per-sample work: the interrupt only applies queued commands,
// advances the 96-PPQN clock, and writes changed CH1-CH4 registers.
func (c *Controller) Start() bool {
	if c == nil || !c.bound || c.running {
		return false
	}
	state := disableStreamInterrupts()
	defer restoreStreamInterrupts(state)
	if !claimNativeController(c) {
		return false
	}
	c.running = true
	startNativeClock()
	return true
}

// Stop synchronously releases Timer 0 and silences only the four PSG channels
// owned by Audio v2.
func (c *Controller) Stop() {
	if c == nil {
		return
	}
	state := disableStreamInterrupts()
	defer restoreStreamInterrupts(state)
	if activeNativeController != nil && activeNativeController != c {
		return
	}
	if activeNativeController == c {
		stopNativeClock()
		releaseNativeController(c)
	}
	c.running = false
	c.commandHead = 0
	c.commandLen = 0
	c.stopSongNow()
	c.stopSFXNow()
	c.stopOwner(OwnerDirect)
	hw.StopAllPSG()
}

func (c *Controller) serviceCommit() {
	cycleStart := hw.ReadFrameCycleCounter()
	c.serviceCalls++
	c.applyCommands()
	c.advanceSoundMacros()
	if !c.hasTimedWork() || c.tempo == 0 {
		c.tickRemainder = 0
		c.finishServiceCycles(cycleStart)
		return
	}
	c.tickRemainder += uint64(c.tempo) * NativePPQN * uint64(c.commitCycles)
	threshold := uint64(60) * uint64(hw.GBAClockHz)
	for c.tickRemainder >= threshold && c.hasTimedWork() {
		c.tickRemainder -= threshold
		c.advanceTick()
	}
	c.finishServiceCycles(cycleStart)
}

func (c *Controller) advanceSoundMacros() {
	for channel := range c.macros {
		playback := &c.macros[channel]
		if !playback.active || !c.channels[channel].Active {
			continue
		}
		playback.cycleRemainder += c.commitCycles
		if playback.cycleRemainder < NativeMacroClockCycles {
			continue
		}
		playback.cycleRemainder -= NativeMacroClockCycles
		sound := findSoundIn(c.library.Sounds, c.channels[channel].SoundID)
		if sound == nil || !c.applySoundMacroStep(channel, sound, playback) {
			playback.active = false
		}
	}
}

func (c *Controller) startSoundMacro(channel int, sound *Sound, period uint16, note uint8) {
	playback := &c.macros[channel]
	*playback = nativeMacroPlayback{period: period, note: note}
	if sound == nil || len(sound.Macro.Steps) == 0 {
		return
	}
	playback.active = true
	c.applySoundMacroStep(channel, sound, playback)
}

func (c *Controller) applySoundMacroStep(channel int, sound *Sound, playback *nativeMacroPlayback) bool {
	if sound == nil || playback == nil || int(playback.nextStep) >= len(sound.Macro.Steps) {
		return false
	}
	step := sound.Macro.Steps[playback.nextStep]
	ok := true
	if step.PeriodDelta != 0 {
		period := int(playback.period) + int(step.PeriodDelta)
		if period < 0 || period > 2047 {
			return false
		}
		playback.period = uint16(period)
		switch sound.Kind {
		case SoundCH1:
			ok = hw.SetPulse1Period(playback.period, sound.Pulse1.Pulse.UseLength)
		case SoundCH2:
			ok = hw.SetPulse2Period(playback.period, sound.Pulse2.UseLength)
		case SoundCH3:
			ok = hw.SetWavePeriod(playback.period, sound.Wave.UseLength)
		}
	}
	if step.HasNoise {
		ok = ok && hw.SetNoisePolynomial(step.NoiseDivisor, step.NoiseShift, sound.Noise.Width)
	}
	if step.HasNoiseNote {
		divisor, shift, valid := NativeGBStudioNoisePolynomial(playback.note, step.NoiseNoteOffset)
		ok = ok && valid && hw.SetNoisePolynomial(divisor, shift, sound.Noise.Width)
	}
	if !ok {
		return false
	}
	next := int(playback.nextStep) + 1
	if next >= len(sound.Macro.Steps) {
		if sound.Macro.LoopStart < 0 {
			playback.active = false
			return true
		}
		next = int(sound.Macro.LoopStart)
	}
	playback.nextStep = uint8(next)
	return true
}

// nativeNoiseTriggerConfig returns the CH4 state that must exist at the exact
// trigger edge. hUGEDriver triggers table-driven noise instruments from the
// played note, then applies subpattern row zero without retriggering. Keeping
// those two writes distinct preserves the LFSR/timer attack on real hardware.
func nativeNoiseTriggerConfig(sound *Sound, note uint8) (hw.NoiseConfig, bool) {
	if sound == nil {
		return hw.NoiseConfig{}, false
	}
	config := sound.Noise
	for _, step := range sound.Macro.Steps {
		if !step.HasNoiseNote {
			continue
		}
		divisor, shift, ok := NativeGBStudioNoisePolynomial(note, 0)
		if !ok {
			return config, false
		}
		config.Divisor = divisor
		config.Shift = shift
		return config, true
	}
	return config, true
}

func (c *Controller) finishServiceCycles(start uint32) {
	end := hw.ReadFrameCycleCounter()
	// The graphics pacer resets TM2/TM3 once per 30 Hz frame. If that reset
	// happened during this interrupt, discard the ambiguous sample instead of
	// treating unsigned underflow as a multi-billion-cycle service call.
	if end >= start {
		c.recordServiceCycles(end - start)
	}
}

func (c *Controller) recordServiceCycles(cycles uint32) {
	// A real service cannot consume zero ARM cycles. A zero sample means the
	// shared frame counter was stopped/reset around the read (observed when an
	// emulator runs unthrottled), so exclude it from min/mean telemetry.
	if cycles == 0 {
		return
	}
	c.lastServiceCycles = cycles
	if c.measuredServices == 0 || cycles < c.minServiceCycles {
		c.minServiceCycles = cycles
	}
	c.measuredServices++
	if ^uint32(0)-c.totalServiceCycles < cycles {
		c.totalServiceCycles = ^uint32(0)
	} else {
		c.totalServiceCycles += cycles
	}
	if cycles > c.maxServiceCycles {
		c.maxServiceCycles = cycles
	}
}

func (c *Controller) applyCommands() {
	for c.commandLen > 0 {
		command := c.commands[c.commandHead]
		c.commandHead = (c.commandHead + 1) & (NativeMaxCommandQueue - 1)
		c.commandLen--
		switch command.kind {
		case commandNote:
			c.playNote(uint8(command.a), uint8(command.b), uint8(command.c), command.d, OwnerDirect, 255, false)
		case commandStopChannel:
			channel := uint8(command.a)
			c.cancelSFXChannel(channel, false)
			c.stopChannel(channel, OwnerDirect, false)
		case commandPlaySFX:
			c.playSFXNow(uint8(command.a), command.priority)
		case commandStopSFX:
			c.stopSFXNow()
		case commandPlaySong:
			c.startSong(uint8(command.a))
		case commandStopSong:
			c.stopSongNow()
		}
	}
}

func (c *Controller) playSFXNow(id, priority uint8) {
	clip, clipIndex := findClipIn(c.library.Clips, id)
	if !c.bound || clip == nil || len(clip.NoteEvents) == 0 {
		c.commandsRejected++
		return
	}
	channelIndex := int(clip.Kind)
	channel := uint8(channelIndex + 1)
	current := &c.channels[channelIndex]
	playback := &c.sfx[channelIndex]
	if current.Active && (current.Owner == OwnerDirect || current.Owner == OwnerSong && priority < current.Priority) ||
		playback.active && priority < playback.priority {
		c.priorityRejects++
		return
	}
	if playback.active {
		c.cancelSFXChannel(channel, true)
	}
	*playback = nativeSFXPlayback{
		active: true, clipID: id, clipIndex: clipIndex, priority: priority,
	}
	c.processSFXTick(channelIndex, 0)
}

func (c *Controller) startSong(id uint8) {
	if !c.bound {
		c.commandsRejected++
		return
	}
	index := findSongIndex(c.library.Songs, id)
	if index < 0 {
		c.commandsRejected++
		return
	}
	hadTimedWork := c.hasTimedWork()
	c.stopOwner(OwnerSong)
	song := &c.library.Songs[index]
	c.songActive = true
	c.songID = id
	c.songIndex = index
	c.sequencePos = 0
	if !hadTimedWork {
		c.tickRemainder = 0
	}
	c.tempo = song.Tempo
	if !c.enterPart(song.Sequence[0]) {
		c.stopSongNow()
		c.invalidAssets++
		return
	}
	c.processPartTick(0)
}

func (c *Controller) enterPart(id uint8) bool {
	index := findPartIndex(c.library.Parts, id)
	if index < 0 {
		return false
	}
	c.partID = id
	c.partIndex = index
	c.partTick = 0
	part := &c.library.Parts[index]
	if part.Tempo != 0 {
		c.tempo = part.Tempo
	} else if c.songIndex >= 0 {
		c.tempo = c.library.Songs[c.songIndex].Tempo
	}
	for channel := range c.clipIndex {
		c.clipIndex[channel] = -1
		c.clipCursor[channel] = 0
		if part.PSGClips[channel] >= 0 {
			_, clipIndex := findClipIn(c.library.Clips, uint8(part.PSGClips[channel]))
			c.clipIndex[channel] = clipIndex
			// A part boundary is the documented point where the song reclaims
			// assigned hardware channels from SFX or direct ownership.
			if c.channels[channel].Active && c.channels[channel].Owner != OwnerSong {
				if c.channels[channel].Owner == OwnerSFX {
					c.cancelSFXChannel(uint8(channel+1), false)
				}
				c.stopChannel(uint8(channel+1), c.channels[channel].Owner, true)
			}
		}
	}
	return true
}

func (c *Controller) advanceTick() {
	c.absoluteTick++
	for channel := range c.sfx {
		playback := &c.sfx[channel]
		if !playback.active {
			continue
		}
		clip := &c.library.Clips[playback.clipIndex]
		if playback.tick+1 >= clip.LengthTicks {
			c.cancelSFXChannel(uint8(channel+1), true)
			continue
		}
		playback.tick++
		c.processSFXTick(channel, playback.tick)
	}
	if c.songActive {
		part := &c.library.Parts[c.partIndex]
		if c.partTick+1 >= part.LengthTicks {
			c.advancePart()
		} else {
			c.partTick++
			c.processPartTick(c.partTick)
		}
	}
	// Apply note-ons for this exact tick before expiring the previous event.
	// A legato event at a part boundary can therefore update the current period
	// without an audible stop/retrigger gap. If no replacement arrived, the old
	// scheduled stop still wins here.
	for channel := range c.channels {
		state := &c.channels[channel]
		if state.Active && state.HasStop && c.absoluteTick >= state.StopTick {
			c.stopChannel(uint8(channel+1), state.Owner, true)
		}
	}
}

func (c *Controller) processSFXTick(channel int, tick uint32) {
	playback := &c.sfx[channel]
	clip := &c.library.Clips[playback.clipIndex]
	cursor := int(playback.cursor)
	for cursor < len(clip.NoteEvents) && clip.NoteEvents[cursor].Tick == tick {
		event := clip.NoteEvents[cursor]
		c.playNote(uint8(channel+1), event.Note, event.SoundID, event.DurationTicks, OwnerSFX, playback.priority, event.Legato)
		cursor++
	}
	playback.cursor = uint16(cursor)
}

func (c *Controller) cancelSFXChannel(channel uint8, stopOwnedNote bool) {
	playback := &c.sfx[channel-1]
	if !playback.active {
		return
	}
	if stopOwnedNote {
		c.stopChannel(channel, OwnerSFX, false)
	}
	*playback = nativeSFXPlayback{clipIndex: -1}
}

func (c *Controller) stopSFXNow() {
	for channel := uint8(1); channel <= 4; channel++ {
		c.cancelSFXChannel(channel, true)
	}
}

func (c *Controller) hasTimedWork() bool {
	if c.songActive {
		return true
	}
	for channel := range c.channels {
		if c.sfx[channel].active || c.channels[channel].Active && c.channels[channel].HasStop {
			return true
		}
	}
	return false
}

func (c *Controller) advancePart() {
	song := &c.library.Songs[c.songIndex]
	next := int(c.sequencePos) + 1
	if next >= len(song.Sequence) {
		if song.LoopStart < 0 {
			c.completedSongs++
			c.stopSongNow()
			return
		}
		next = int(song.LoopStart)
	}
	c.sequencePos = uint8(next)
	if !c.enterPart(song.Sequence[next]) {
		c.invalidAssets++
		c.stopSongNow()
		return
	}
	c.processPartTick(0)
}

func (c *Controller) processPartTick(tick uint32) {
	for channel := 0; channel < 4; channel++ {
		clipIndex := c.clipIndex[channel]
		if clipIndex < 0 {
			continue
		}
		clip := &c.library.Clips[clipIndex]
		cursor := int(c.clipCursor[channel])
		for cursor < len(clip.NoteEvents) && clip.NoteEvents[cursor].Tick == tick {
			event := clip.NoteEvents[cursor]
			c.playNote(uint8(channel+1), event.Note, event.SoundID, event.DurationTicks, OwnerSong, event.Priority, event.Legato)
			cursor++
		}
		c.clipCursor[channel] = uint16(cursor)
	}
}

func (c *Controller) playNote(channel, note, soundID uint8, duration uint16, owner NativeOwner, priority uint8, legato bool) bool {
	if !c.bound && owner != OwnerDirect {
		c.commandsRejected++
		return false
	}
	sound := findSoundIn(c.library.Sounds, soundID)
	kind := SoundKind(channel - 1)
	if sound == nil || sound.Kind != kind || !nativeNoteValid(sound, note) {
		c.commandsRejected++
		return false
	}
	state := &c.channels[channel-1]
	if owner == OwnerDirect {
		c.cancelSFXChannel(channel, false)
	}
	if state.Active {
		// Within a part, song events do not unexpectedly overwrite gameplay
		// or explicit native control. The next part boundary reclaims the lane.
		if owner == OwnerSong && state.Owner != OwnerSong {
			c.priorityRejects++
			return false
		}
		if owner == OwnerSFX {
			if state.Owner == OwnerDirect || state.Owner == OwnerSong && priority < state.Priority {
				c.priorityRejects++
				return false
			}
		}
		state.Replacements++
		c.channelReplaces++
	}
	triggered := false
	period := uint16(0)
	if kind != SoundCH4 {
		period, _ = nativeSoundPeriod(sound, note)
	}
	if legato && state.Active && state.SoundID == soundID {
		switch kind {
		case SoundCH1:
			triggered = hw.SetPulse1Period(period, sound.Pulse1.Pulse.UseLength)
		case SoundCH2:
			triggered = hw.SetPulse2Period(period, sound.Pulse2.UseLength)
		case SoundCH3:
			triggered = hw.SetWavePeriod(period, sound.Wave.UseLength)
		}
	}
	if !triggered {
		switch kind {
		case SoundCH1:
			triggered = hw.TriggerPulse1(period, sound.Pulse1)
		case SoundCH2:
			triggered = hw.TriggerPulse2(period, sound.Pulse2)
		case SoundCH3:
			triggered = hw.TriggerWaveRef(period, &sound.Wave)
		case SoundCH4:
			noise, ok := nativeNoiseTriggerConfig(sound, note)
			triggered = ok && hw.TriggerNoise(noise)
		}
	}
	if !triggered {
		c.commandsRejected++
		return false
	}
	c.startSoundMacro(int(channel-1), sound, period, note)
	*state = ChannelState{
		Active: true, Owner: owner, Priority: priority, Note: note, SoundID: soundID,
		StartedTick: c.absoluteTick, Replacements: state.Replacements,
	}
	if duration > 0 {
		state.HasStop = true
		state.StopTick = c.absoluteTick + uint32(duration)
	}
	c.eventsApplied++
	return true
}

func (c *Controller) stopChannel(channel uint8, owner NativeOwner, force bool) bool {
	state := &c.channels[channel-1]
	if !state.Active || (!force && owner != OwnerDirect && state.Owner != owner) {
		return false
	}
	hw.StopPSGChannel(channel)
	c.macros[channel-1] = nativeMacroPlayback{}
	replacements := state.Replacements
	*state = ChannelState{Replacements: replacements}
	c.eventsApplied++
	return true
}

func (c *Controller) stopOwner(owner NativeOwner) {
	for channel := uint8(1); channel <= 4; channel++ {
		state := &c.channels[channel-1]
		if state.Active && state.Owner == owner {
			c.stopChannel(channel, owner, true)
		}
	}
}

func (c *Controller) stopSongNow() {
	c.stopOwner(OwnerSong)
	c.songActive = false
	c.songIndex = -1
	c.partIndex = -1
	c.tempo = NativeDefaultTempo
	if !c.hasTimedWork() {
		c.tickRemainder = 0
	}
}

func (c *Controller) Stats() NativeStats {
	if c == nil {
		return NativeStats{}
	}
	state := disableStreamInterrupts()
	defer restoreStreamInterrupts(state)
	return c.statsLocked()
}

func (c *Controller) statsLocked() NativeStats {
	var stats NativeStats
	c.statsIntoLocked(&stats)
	return stats
}

func (c *Controller) statsIntoLocked(stats *NativeStats) {
	meanServiceCycles := uint32(0)
	if c.measuredServices != 0 {
		meanServiceCycles = c.totalServiceCycles / c.measuredServices
	}
	*stats = NativeStats{
		ClockRunning: c.running, RunningSong: c.songActive, SongID: c.songID, SequenceIndex: c.sequencePos,
		PartID: c.partID, PartTick: c.partTick, AbsoluteTick: c.absoluteTick,
		ServiceCalls: c.serviceCalls, MeasuredServices: c.measuredServices,
		LastServiceCycles: c.lastServiceCycles, MinServiceCycles: c.minServiceCycles,
		MeanServiceCycles: meanServiceCycles, MaxServiceCycles: c.maxServiceCycles,
		EventsApplied:    c.eventsApplied,
		CommandsRejected: c.commandsRejected, QueueOverflows: c.queueOverflows,
		InvalidAssets: c.invalidAssets, ChannelReplaces: c.channelReplaces,
		PriorityRejects: c.priorityRejects, CompletedSongs: c.completedSongs,
		Profile: c.profile, CommitRateMilliHz: profileCommitMilliHz(c.profile), Channels: c.channels,
	}
	for channel := range c.sfx {
		stats.SFX[channel] = SFXPlaybackState{
			Active: c.sfx[channel].active, ClipID: c.sfx[channel].clipID,
			Tick: c.sfx[channel].tick, Priority: c.sfx[channel].priority,
		}
	}
}

// PreviewInto writes one coherent playback snapshot into caller-owned memory.
// Target code should prefer this form: it avoids returning the comparatively
// large snapshot through TinyGo's interruptible aggregate-return path.
func (c *Controller) PreviewInto(preview *NativePreview) bool {
	if c == nil || preview == nil {
		return false
	}
	state := disableStreamInterrupts()
	defer restoreStreamInterrupts(state)
	c.statsIntoLocked(&preview.Stats)
	preview.PSG = hw.ManagedPSGSnapshot()
	preview.Control = hw.ManagedSoundControlSnapshot()
	return true
}

func (c *Controller) Preview() NativePreview {
	var preview NativePreview
	c.PreviewInto(&preview)
	return preview
}
