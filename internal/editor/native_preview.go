package editor

import (
	"errors"
	"math"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/gbapu"
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

// NativePreviewSampleRate is intentionally the desktop monitor rate, not a
// Direct Sound FIFO rate. Native Audio v2 uses the four PSG channels directly;
// rendering them at 16,384 Hz and then linearly stretching that signal to the
// host rate aliases narrow pulse waves and high-frequency noise badly. GB
// Studio likewise asks its Game Boy emulator to render at the browser audio
// context's rate.
const NativePreviewSampleRate = 48000

// NativeNotePreviewSamples matches GB Studio's half-second instrument test.
const NativeNotePreviewSamples = NativePreviewSampleRate / 2

type NativePreviewSegment struct {
	SequenceIndex int
	PartID        int
	Tempo         int
	StartSample   int
	EndSample     int
}

type NativeMusicPreview struct {
	// Samples remains the mono fold-down used by deterministic reports and WAV
	// export. Desktop playback uses LeftSamples/RightSamples so authored GBA
	// routing is audible instead of being silently collapsed to mono.
	Samples      []int8
	LeftSamples  []int8
	RightSamples []int8
	Segments     []NativePreviewSegment
	Tempo        int
	MuteMask     uint8
}

func (p *NativeMusicPreview) Position(sample int) (sequence, step int, ok bool) {
	if p == nil || sample < 0 {
		return 0, 0, false
	}
	for _, segment := range p.Segments {
		if sample >= segment.StartSample && sample < segment.EndSample {
			local := sample - segment.StartSample
			tempo := segment.Tempo
			if tempo == 0 {
				tempo = p.Tempo
			}
			ticks := int((int64(local)*int64(tempo)*audio.NativePPQN + NativePreviewSampleRate*30) / (NativePreviewSampleRate * 60))
			return segment.SequenceIndex, ticks / NativeTicksPerStep, true
		}
	}
	return 0, 0, false
}

func RenderNativeMusicPreview(library audio.Library, songID int) (*NativeMusicPreview, error) {
	return RenderNativeMusicPreviewMute(library, songID, 0)
}

// RenderNativeMusicPreviewMute applies an editor-only four-bit monitor mask.
// It changes neither authored assets nor target playback data.
func RenderNativeMusicPreviewMute(library audio.Library, songID int, muteMask uint8) (*NativeMusicPreview, error) {
	return RenderNativeMusicPreviewSettings(library, songID, muteMask, assetsync.DefaultNativeAudioSettings())
}

// RenderNativeMusicPreviewSettings simulates the GBA PSG terminal routing,
// left/right master levels, and PSG mix ratio for the desktop monitor.
func RenderNativeMusicPreviewSettings(library audio.Library, songID int, muteMask uint8, settings assetsync.NativeAudioSettings) (*NativeMusicPreview, error) {
	if !audio.ValidateLibrary(library) {
		return nil, errors.New("native audio library is invalid")
	}
	var song *audio.Song
	for index := range library.Songs {
		if int(library.Songs[index].ID) == songID {
			song = &library.Songs[index]
			break
		}
	}
	if song == nil {
		return nil, errors.New("song does not exist")
	}
	preview := &NativeMusicPreview{Tempo: int(song.Tempo), MuteMask: muteMask & 0x0f}
	partStart := 0
	var voiceStates [4]nativePreviewVoiceState
	var previousEnd [4]int
	var previousSound [4]uint8
	var previousValid [4]bool
	var leftFilter, rightFilter nativePreviewHighPass
	for sequenceIndex, partID := range song.Sequence {
		part := nativeRuntimePart(library.Parts, partID)
		if part == nil {
			return nil, errors.New("song references a missing part")
		}
		partTempo := int(part.Tempo)
		if partTempo == 0 {
			partTempo = int(song.Tempo)
		}
		partSamples := nativeTicksToSamples(int(part.LengthTicks), partTempo)
		if partSamples < 1 {
			partSamples = 1
		}
		if len(preview.Samples)+partSamples > NativePreviewSampleRate*120 {
			return nil, errors.New("editor preview is longer than two minutes")
		}
		start := len(preview.Samples)
		preview.Samples = append(preview.Samples, make([]int8, partSamples)...)
		preview.LeftSamples = append(preview.LeftSamples, make([]int8, partSamples)...)
		preview.RightSamples = append(preview.RightSamples, make([]int8, partSamples)...)
		leftMix := make([]int16, partSamples)
		rightMix := make([]int16, partSamples)
		for channel, clipID := range part.PSGClips {
			if preview.MuteMask&(1<<channel) != 0 {
				continue
			}
			if clipID < 0 {
				previousValid[channel] = false
				voiceStates[channel] = nativePreviewVoiceState{}
				continue
			}
			clip := nativeRuntimeClip(library.Clips, uint8(clipID))
			if clip == nil || int(clip.Kind) != channel {
				continue
			}
			for _, event := range clip.NoteEvents {
				eventStart := nativeTicksToSamples(int(event.Tick), partTempo)
				eventLength := nativeTicksToSamples(int(event.DurationTicks), partTempo)
				sound := nativeRuntimeSound(library.Sounds, event.SoundID)
				if sound == nil || eventStart >= partSamples {
					continue
				}
				length := min(eventLength, partSamples-eventStart)
				eventMix := make([]int16, length)
				globalStart := start + eventStart
				continued := event.Legato && previousValid[channel] && previousSound[channel] == event.SoundID && previousEnd[channel] == globalStart
				nativeRenderEventState(eventMix, 0, length, event.Note, *sound, &voiceStates[channel], continued)
				previousEnd[channel] = globalStart + length
				previousSound[channel] = event.SoundID
				previousValid[channel] = true
				route := nativeSoundRoute(*sound)
				for offset, value := range eventMix {
					if route == hw.SoundRouteLeft || route == hw.SoundRouteBoth {
						leftMix[eventStart+offset] += value
					}
					if route == hw.SoundRouteRight || route == hw.SoundRouteBoth {
						rightMix[eventStart+offset] += value
					}
				}
			}
		}
		for index := range leftMix {
			left := leftFilter.terminalSample(leftMix[index], settings.LeftLevel, settings.PSGRatio)
			right := rightFilter.terminalSample(rightMix[index], settings.RightLevel, settings.PSGRatio)
			preview.LeftSamples[start+index] = left
			preview.RightSamples[start+index] = right
			preview.Samples[start+index] = int8((int(left) + int(right)) / 2)
		}
		preview.Segments = append(preview.Segments, NativePreviewSegment{
			SequenceIndex: sequenceIndex, PartID: int(part.ID), Tempo: partTempo, StartSample: partStart, EndSample: partStart + partSamples,
		})
		partStart += partSamples
	}
	return preview, nil
}

// RenderNativeSFXPreview renders one standalone PSG clip. Standalone clips use
// their stable clip ID as the runtime sfx(id), and deliberately retain the
// same channel routing/master settings as music preview.
func RenderNativeSFXPreview(library audio.Library, clipID, tempo int, muteMask uint8, settings assetsync.NativeAudioSettings) (*NativeMusicPreview, error) {
	if !audio.ValidateLibrary(library) {
		return nil, errors.New("native audio library is invalid")
	}
	clip := nativeRuntimeClip(library.Clips, uint8(clipID))
	if clip == nil || int(clip.ID) != clipID {
		return nil, errors.New("effect does not exist")
	}
	if tempo < 20 || tempo > 400 {
		tempo = audio.NativeDefaultTempo
	}
	length := nativeTicksToSamples(int(clip.LengthTicks), tempo)
	if length < 1 {
		length = 1
	}
	if length > NativePreviewSampleRate*120 {
		return nil, errors.New("editor preview is longer than two minutes")
	}
	preview := &NativeMusicPreview{
		Samples:      make([]int8, length),
		LeftSamples:  make([]int8, length),
		RightSamples: make([]int8, length),
		Tempo:        tempo,
		MuteMask:     muteMask & 0x0f,
		Segments: []NativePreviewSegment{{
			SequenceIndex: 0, PartID: -1, Tempo: tempo, StartSample: 0, EndSample: length,
		}},
	}
	if preview.MuteMask&(1<<int(clip.Kind)) != 0 {
		return preview, nil
	}
	leftMix := make([]int16, length)
	rightMix := make([]int16, length)
	state := nativePreviewVoiceState{}
	previousEnd := 0
	previousSound := uint8(0)
	previousValid := false
	for _, event := range clip.NoteEvents {
		eventStart := nativeTicksToSamples(int(event.Tick), tempo)
		eventLength := nativeTicksToSamples(int(event.DurationTicks), tempo)
		sound := nativeRuntimeSound(library.Sounds, event.SoundID)
		if sound == nil || sound.Kind != clip.Kind || eventStart >= length {
			continue
		}
		eventLength = min(eventLength, length-eventStart)
		eventMix := make([]int16, eventLength)
		continued := event.Legato && previousValid && previousSound == event.SoundID && previousEnd == eventStart
		nativeRenderEventState(eventMix, 0, eventLength, event.Note, *sound, &state, continued)
		previousEnd = eventStart + eventLength
		previousSound = event.SoundID
		previousValid = true
		route := nativeSoundRoute(*sound)
		for offset, value := range eventMix {
			if route == hw.SoundRouteLeft || route == hw.SoundRouteBoth {
				leftMix[eventStart+offset] += value
			}
			if route == hw.SoundRouteRight || route == hw.SoundRouteBoth {
				rightMix[eventStart+offset] += value
			}
		}
	}
	var leftFilter, rightFilter nativePreviewHighPass
	for index := range leftMix {
		left := leftFilter.terminalSample(leftMix[index], settings.LeftLevel, settings.PSGRatio)
		right := rightFilter.terminalSample(rightMix[index], settings.RightLevel, settings.PSGRatio)
		preview.LeftSamples[index] = left
		preview.RightSamples[index] = right
		preview.Samples[index] = int8((int(left) + int(right)) / 2)
	}
	return preview, nil
}

// nativePreviewHighPass models the AC-coupled output used by Game Boy
// emulators and hardware. The 511/512 coefficient is the same bass-shift=9
// integrator leak used by binjgb/mGBA's blip buffer and removes the large DC
// component of 12.5%, 25%, and 75% pulse waves.
type nativePreviewHighPass struct {
	previousInput  float64
	previousOutput float64
}

func (f *nativePreviewHighPass) process(value int) int {
	input := float64(value)
	output := input - f.previousInput + f.previousOutput*(511.0/512.0)
	f.previousInput = input
	f.previousOutput = output
	return int(math.Round(output))
}

func (f *nativePreviewHighPass) terminalSample(value int16, level, ratio int) int8 {
	level = min(max(level, 0), 7)
	switch ratio {
	case 25, 50, 100:
	default:
		ratio = 100
	}
	scaled := int(value) * (level + 1) * ratio / (4 * 8 * 100)
	filtered := f.process(scaled)
	return int8(min(max(filtered, -127), 127))
}

// NativeMonitorMuteMask combines explicit Mute and Solo controls. At least
// one Solo means every non-soloed channel is monitored as muted; an explicit
// Mute still wins when the same channel is also soloed.
func NativeMonitorMuteMask(muteMask, soloMask uint8) uint8 {
	muteMask &= 0x0f
	soloMask &= 0x0f
	if soloMask != 0 {
		muteMask |= (^soloMask) & 0x0f
	}
	return muteMask
}

// RenderNativeNotePreview renders one compiled PSG sound in isolation. The
// editor uses it to audition a piano-roll click through the exact CH1-CH4
// configuration that will be written to the cartridge.
func RenderNativeNotePreview(library audio.Library, soundID, note int) ([]int8, error) {
	if !audio.ValidateLibrary(library) {
		return nil, errors.New("native audio library is invalid")
	}
	if soundID < 0 || soundID > 255 || note < 0 || note > 127 {
		return nil, errors.New("native note preview selection is invalid")
	}
	sound := nativeRuntimeSound(library.Sounds, uint8(soundID))
	if sound == nil {
		return nil, errors.New("native note preview sound does not exist")
	}
	if !audio.NativeSoundNoteValid(*sound, uint8(note)) {
		return nil, errors.New("native note preview pitch is outside the hardware channel range")
	}

	monitorSound := nativeSoundWithRoute(*sound, hw.SoundRouteBoth)
	renderer := gbapu.NewRenderer(NativePreviewSampleRate)
	if !renderer.Start(uint8(sound.Kind)+1, uint8(note), monitorSound) {
		return nil, errors.New("native note preview could not start the hardware renderer")
	}
	left := make([]int16, NativeNotePreviewSamples)
	right := make([]int16, NativeNotePreviewSamples)
	renderer.Render(left, right)
	samples := make([]int8, len(left))
	for index, value := range left {
		samples[index] = int8(min(max(int(value)/256, -127), 127))
	}
	return samples, nil
}

func RenderNativeNotePreviewStereo(library audio.Library, soundID, note int, settings assetsync.NativeAudioSettings) ([]int8, []int8, error) {
	if !audio.ValidateLibrary(library) || soundID < 0 || soundID > 255 || note < 0 || note > 127 {
		return nil, nil, errors.New("native note preview selection is invalid")
	}
	sound := nativeRuntimeSound(library.Sounds, uint8(soundID))
	if sound == nil || !audio.NativeSoundNoteValid(*sound, uint8(note)) {
		return nil, nil, errors.New("native note preview sound does not exist")
	}
	renderer := gbapu.NewRenderer(NativePreviewSampleRate)
	renderer.SetOutput(settings.LeftLevel, settings.RightLevel, settings.PSGRatio)
	if !renderer.Start(uint8(sound.Kind)+1, uint8(note), *sound) {
		return nil, nil, errors.New("native note preview could not start the hardware renderer")
	}
	leftPCM := make([]int16, NativeNotePreviewSamples)
	rightPCM := make([]int16, NativeNotePreviewSamples)
	renderer.Render(leftPCM, rightPCM)
	left := make([]int8, NativeNotePreviewSamples)
	right := make([]int8, NativeNotePreviewSamples)
	for index := range left {
		left[index] = int8(min(max(int(leftPCM[index])/256, -127), 127))
		right[index] = int8(min(max(int(rightPCM[index])/256, -127), 127))
	}
	return left, right, nil
}

func nativeSoundWithRoute(sound audio.Sound, route hw.SoundRoute) audio.Sound {
	switch sound.Kind {
	case audio.SoundCH1:
		sound.Pulse1.Pulse.Route = route
	case audio.SoundCH2:
		sound.Pulse2.Route = route
	case audio.SoundCH3:
		sound.Wave.Route = route
	case audio.SoundCH4:
		sound.Noise.Route = route
	}
	return sound
}

func nativeTicksToSamples(ticks, tempo int) int {
	return int((int64(ticks)*NativePreviewSampleRate*60 + int64(tempo*audio.NativePPQN)/2) / int64(tempo*audio.NativePPQN))
}

func nativeRuntimePart(parts []audio.Part, id uint8) *audio.Part {
	for index := range parts {
		if parts[index].ID == id {
			return &parts[index]
		}
	}
	return nil
}

func nativeRuntimeClip(clips []audio.Clip, id uint8) *audio.Clip {
	for index := range clips {
		if clips[index].ID == id {
			return &clips[index]
		}
	}
	return nil
}

func nativeRuntimeSound(sounds []audio.Sound, id uint8) *audio.Sound {
	for index := range sounds {
		if sounds[index].ID == id {
			return &sounds[index]
		}
	}
	return nil
}

type nativePreviewVoiceState struct {
	initialized    bool
	tone           bool
	phase          float64
	noisePhase     float64
	lfsr           uint16
	elapsed        int
	sweepElapsed   int
	period         int
	noiseDivisor   uint8
	noiseShift     uint8
	macroStep      uint8
	macroRemainder uint64
	macroActive    bool
	macroBaseNote  uint8
}

func nativeRenderEvent(target []int16, start, length int, note uint8, sound audio.Sound) {
	if nativeSoundRoute(sound) == hw.SoundRouteNone {
		return
	}
	state := nativePreviewVoiceState{}
	nativeRenderEventState(target, start, length, note, sound, &state, false)
}

// nativeRenderEventState keeps the oscillator, hardware envelope, length
// counter, and CH1 sweep alive across an explicitly tied note. This mirrors a
// period write without the trigger bit on the Game Boy APU. A normal note
// resets all of that state, just like a hardware retrigger.
func nativeRenderEventState(target []int16, start, length int, note uint8, sound audio.Sound, state *nativePreviewVoiceState, continued bool) {
	if length <= 0 {
		return
	}
	if state == nil {
		return
	}
	if start < 0 || start >= len(target) {
		return
	}
	length = min(length, len(target)-start)
	period, tone := audio.NativeSoundPeriod(sound, note)
	if !continued || !state.initialized {
		*state = nativePreviewVoiceState{
			initialized:   true,
			tone:          tone,
			lfsr:          0x7fff,
			period:        int(period),
			noiseDivisor:  sound.Noise.Divisor,
			noiseShift:    sound.Noise.Shift,
			macroActive:   len(sound.Macro.Steps) != 0,
			macroBaseNote: note,
		}
		nativePreviewApplyMacroStep(sound, state)
	} else if state.tone && tone {
		// Legato changes pitch but deliberately does not restart phase,
		// envelope, length, or sweep timing.
		state.period = int(period)
	}
	if hardwareLength := nativeHardwareLengthSamples(sound); hardwareLength > 0 {
		remaining := hardwareLength - state.elapsed
		if remaining <= 0 {
			return
		}
		length = min(length, remaining)
	}
	sweepInterval := 0
	if sound.Kind == audio.SoundCH1 && sound.Pulse1.Sweep.Pace != 0 {
		sweepInterval = max(1, NativePreviewSampleRate*int(sound.Pulse1.Sweep.Pace)/128)
	}
	for index := 0; index < length; index++ {
		if sweepInterval != 0 && state.sweepElapsed != 0 && state.sweepElapsed%sweepInterval == 0 {
			delta := state.period >> sound.Pulse1.Sweep.Shift
			if sound.Pulse1.Sweep.Direction == hw.SweepDown {
				state.period -= delta
			} else {
				state.period += delta
			}
			if state.period < 0 || state.period > 2047 {
				state.tone = false
				sweepInterval = 0
			}
		}
		frequency := 0.0
		if state.tone {
			clock := 131072.0
			if sound.Kind == audio.SoundCH3 {
				waveLength := max(1, int(sound.Wave.Wave.Length))
				clock = 2097152.0 / float64(waveLength)
			}
			frequency = clock / float64(2048-state.period)
		}
		phase := state.phase
		value, volume := 0.0, 1.0
		switch sound.Kind {
		case audio.SoundCH1:
			if !state.tone {
				break
			}
			advance := frequency / NativePreviewSampleRate
			value = nativePulseAverage(phase, advance, nativePulseDutyFraction(sound.Pulse1.Pulse.Duty))
			state.phase = math.Mod(phase+advance, 1)
			volume = nativeEnvelopeVolume(state.elapsed, sound.Pulse1.Pulse.Envelope.Initial, sound.Pulse1.Pulse.Envelope.Pace, sound.Pulse1.Pulse.Envelope.Direction == hw.EnvelopeUp)
		case audio.SoundCH2:
			if !state.tone {
				break
			}
			advance := frequency / NativePreviewSampleRate
			value = nativePulseAverage(phase, advance, nativePulseDutyFraction(sound.Pulse2.Duty))
			state.phase = math.Mod(phase+advance, 1)
			volume = nativeEnvelopeVolume(state.elapsed, sound.Pulse2.Envelope.Initial, sound.Pulse2.Envelope.Pace, sound.Pulse2.Envelope.Direction == hw.EnvelopeUp)
		case audio.SoundCH3:
			if !state.tone {
				break
			}
			wave := sound.Wave.Wave
			if wave.Length != 0 {
				advance := frequency / NativePreviewSampleRate
				value = nativeWaveAverage(phase, advance, wave)
				state.phase = math.Mod(phase+advance, 1)
			}
			volume = nativeWaveLevel(sound.Wave.Level)
		case audio.SoundCH4:
			frequency = nativeNoiseFrequency(state.noiseDivisor, state.noiseShift)
			value = nativeNoiseAverage(state, frequency/NativePreviewSampleRate, sound.Noise.Width)
			volume = nativeEnvelopeVolume(state.elapsed, sound.Noise.Envelope.Initial, sound.Noise.Envelope.Pace, sound.Noise.Envelope.Direction == hw.EnvelopeUp)
		}
		target[start+index] += int16(value * volume * 112)
		state.elapsed++
		state.sweepElapsed++
		nativePreviewAdvanceMacro(sound, state)
	}
}

func nativePreviewAdvanceMacro(sound audio.Sound, state *nativePreviewVoiceState) {
	if state == nil || !state.macroActive {
		return
	}
	state.macroRemainder += uint64(hw.GBAClockHz)
	threshold := uint64(NativePreviewSampleRate) * uint64(audio.NativeMacroClockCycles)
	if state.macroRemainder < threshold {
		return
	}
	state.macroRemainder -= threshold
	nativePreviewApplyMacroStep(sound, state)
}

func nativePreviewApplyMacroStep(sound audio.Sound, state *nativePreviewVoiceState) {
	if state == nil || !state.macroActive || int(state.macroStep) >= len(sound.Macro.Steps) {
		return
	}
	step := sound.Macro.Steps[state.macroStep]
	if step.PeriodDelta != 0 {
		state.period += int(step.PeriodDelta)
		if state.period < 0 || state.period > 2047 {
			state.tone = false
			state.macroActive = false
			return
		}
	}
	if step.HasNoise {
		state.noiseDivisor = step.NoiseDivisor
		state.noiseShift = step.NoiseShift
	}
	if step.HasNoiseNote {
		divisor, shift, ok := audio.NativeGBStudioNoisePolynomial(state.macroBaseNote, step.NoiseNoteOffset)
		if !ok {
			state.macroActive = false
			return
		}
		state.noiseDivisor = divisor
		state.noiseShift = shift
	}
	next := int(state.macroStep) + 1
	if next >= len(sound.Macro.Steps) {
		if sound.Macro.LoopStart < 0 {
			state.macroActive = false
			return
		}
		next = int(sound.Macro.LoopStart)
	}
	state.macroStep = uint8(next)
}

func nativeSoundRoute(sound audio.Sound) hw.SoundRoute {
	switch sound.Kind {
	case audio.SoundCH1:
		return sound.Pulse1.Pulse.Route
	case audio.SoundCH2:
		return sound.Pulse2.Route
	case audio.SoundCH3:
		return sound.Wave.Route
	case audio.SoundCH4:
		return sound.Noise.Route
	default:
		return hw.SoundRouteNone
	}
}

func nativeHardwareLengthSamples(sound audio.Sound) int {
	switch sound.Kind {
	case audio.SoundCH1:
		if sound.Pulse1.Pulse.UseLength {
			return max(1, (64-int(sound.Pulse1.Pulse.Length))*NativePreviewSampleRate/256)
		}
	case audio.SoundCH2:
		if sound.Pulse2.UseLength {
			return max(1, (64-int(sound.Pulse2.Length))*NativePreviewSampleRate/256)
		}
	case audio.SoundCH3:
		if sound.Wave.UseLength {
			return max(1, (256-int(sound.Wave.Length))*NativePreviewSampleRate/256)
		}
	case audio.SoundCH4:
		if sound.Noise.UseLength {
			return max(1, (64-int(sound.Noise.Length))*NativePreviewSampleRate/256)
		}
	}
	return 0
}

func nativePulseValue(phase, duty float64) float64 {
	if phase < duty {
		return 1
	}
	return -1
}

// nativePulseAverage integrates one hardware pulse over an output sample.
// Point-sampling a 262 kHz PSG clock aliases into false audible pitches;
// integrating edge coverage is the compact equivalent of the low-pass stage
// used by an emulator's blip buffer.
func nativePulseAverage(phase, advance, duty float64) float64 {
	if advance <= 0 {
		return nativePulseValue(phase, duty)
	}
	positive := nativePeriodicIntervalCoverage(phase, advance, duty)
	return positive*2/advance - 1
}

func nativePeriodicIntervalCoverage(phase, advance, positiveEnd float64) float64 {
	phase = math.Mod(phase, 1)
	if phase < 0 {
		phase++
	}
	remaining := advance
	covered := 0.0
	for remaining > 0 {
		span := math.Min(remaining, 1-phase)
		if phase < positiveEnd {
			covered += math.Min(span, positiveEnd-phase)
		}
		remaining -= span
		phase += span
		if phase >= 1-1e-12 {
			phase = 0
		}
	}
	return covered
}

func nativeWaveAverage(phase, advance float64, wave hw.Waveform) float64 {
	if wave.Length == 0 {
		return 0
	}
	if advance <= 0 {
		index := int(math.Mod(phase, 1)*float64(wave.Length)) % int(wave.Length)
		return (float64(wave.Samples[index]) - 7.5) / 7.5
	}
	remaining := advance
	position := math.Mod(phase, 1) * float64(wave.Length)
	weighted := 0.0
	for remaining > 0 {
		index := int(position) % int(wave.Length)
		toBoundary := (float64(index+1) - position) / float64(wave.Length)
		if toBoundary <= 1e-12 {
			toBoundary = 1 / float64(wave.Length)
		}
		span := math.Min(remaining, toBoundary)
		weighted += ((float64(wave.Samples[index]) - 7.5) / 7.5) * span
		remaining -= span
		position += span * float64(wave.Length)
		if position >= float64(wave.Length)-1e-12 {
			position = 0
		}
	}
	return weighted / advance
}

func nativeNoiseValue(lfsr uint16) float64 {
	if lfsr&1 == 0 {
		return 1
	}
	return -1
}

func nativeNoiseShift(state *nativePreviewVoiceState, width hw.NoiseWidth) {
	bit := (state.lfsr ^ (state.lfsr >> 1)) & 1
	state.lfsr = (state.lfsr >> 1) | (bit << 14)
	if width == hw.Noise7Bit {
		state.lfsr = (state.lfsr &^ (1 << 6)) | (bit << 6)
	}
}

func nativeNoiseAverage(state *nativePreviewVoiceState, advance float64, width hw.NoiseWidth) float64 {
	if state == nil || advance <= 0 {
		if state == nil {
			return 0
		}
		return nativeNoiseValue(state.lfsr)
	}
	remaining := 1.0
	weighted := 0.0
	for remaining > 0 {
		toShift := (1 - state.noisePhase) / advance
		if toShift >= remaining {
			weighted += nativeNoiseValue(state.lfsr) * remaining
			state.noisePhase += advance * remaining
			break
		}
		weighted += nativeNoiseValue(state.lfsr) * toShift
		remaining -= toShift
		state.noisePhase = 0
		nativeNoiseShift(state, width)
	}
	if state.noisePhase >= 1-1e-12 {
		state.noisePhase = 0
		nativeNoiseShift(state, width)
	}
	return weighted
}

func nativePulseDutyFraction(duty hw.PulseDuty) float64 {
	switch duty {
	case hw.PulseDuty12_5:
		return .125
	case hw.PulseDuty25:
		return .25
	case hw.PulseDuty50:
		return .5
	default:
		return .75
	}
}

func nativeEnvelopeVolume(sample int, initial, pace uint8, up bool) float64 {
	level := int(initial)
	if pace != 0 {
		steps := sample / max(1, NativePreviewSampleRate*int(pace)/64)
		if up {
			level = min(15, level+steps)
		} else {
			level = max(0, level-steps)
		}
	}
	return float64(level) / 15
}

func nativeWaveLevel(level hw.WaveLevel) float64 {
	switch level {
	case 1:
		return 1
	case 2:
		return .5
	case 3:
		return .25
	case 4:
		return .75
	default:
		return 0
	}
}

func nativeNoiseFrequency(divisor, shift uint8) float64 {
	ratio := .5
	if divisor != 0 {
		ratio = float64(divisor)
	}
	return 262144 / (ratio * math.Pow(2, float64(shift)))
}
