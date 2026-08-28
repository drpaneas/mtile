package editor

import (
	"errors"
	"math"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

const NativePreviewSampleRate = 16384

// NativeNotePreviewSamples is a deliberately short desktop audition. It is
// long enough to identify pitch and envelope, but short enough that repeated
// note placement still feels immediate.
const NativeNotePreviewSamples = NativePreviewSampleRate / 4

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
			left := nativePreviewTerminalSample(leftMix[index], settings.LeftLevel, settings.PSGRatio)
			right := nativePreviewTerminalSample(rightMix[index], settings.RightLevel, settings.PSGRatio)
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
	for index := range leftMix {
		left := nativePreviewTerminalSample(leftMix[index], settings.LeftLevel, settings.PSGRatio)
		right := nativePreviewTerminalSample(rightMix[index], settings.RightLevel, settings.PSGRatio)
		preview.LeftSamples[index] = left
		preview.RightSamples[index] = right
		preview.Samples[index] = int8((int(left) + int(right)) / 2)
	}
	return preview, nil
}

func nativePreviewTerminalSample(value int16, level, ratio int) int8 {
	level = min(max(level, 0), 7)
	switch ratio {
	case 25, 50, 100:
	default:
		ratio = 100
	}
	scaled := int(value) * (level + 1) * ratio / (4 * 8 * 100)
	return int8(min(max(scaled, -127), 127))
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
	if sound.Kind != audio.SoundCH4 {
		if _, ok := audio.NativeSoundPeriod(*sound, uint8(note)); !ok {
			return nil, errors.New("native note preview pitch is outside the hardware channel range")
		}
	}

	mix := make([]int16, NativeNotePreviewSamples)
	nativeRenderEvent(mix, 0, len(mix), uint8(note), *sound)
	samples := make([]int8, len(mix))
	for index, value := range mix {
		value /= 4 // match one channel's level in the four-channel song monitor
		value = min(max(value, -127), 127)
		samples[index] = int8(value)
	}
	applyNativeNotePreviewEdges(samples)
	return samples, nil
}

func RenderNativeNotePreviewStereo(library audio.Library, soundID, note int, settings assetsync.NativeAudioSettings) ([]int8, []int8, error) {
	mono, err := RenderNativeNotePreview(library, soundID, note)
	if err != nil {
		return nil, nil, err
	}
	sound := nativeRuntimeSound(library.Sounds, uint8(soundID))
	if sound == nil {
		return nil, nil, errors.New("native note preview sound does not exist")
	}
	left := make([]int8, len(mono))
	right := make([]int8, len(mono))
	route := nativeSoundRoute(*sound)
	for index, sample := range mono {
		if route == hw.SoundRouteLeft || route == hw.SoundRouteBoth {
			left[index] = nativePreviewTerminalSample(int16(sample)*4, settings.LeftLevel, settings.PSGRatio)
		}
		if route == hw.SoundRouteRight || route == hw.SoundRouteBoth {
			right[index] = nativePreviewTerminalSample(int16(sample)*4, settings.RightLevel, settings.PSGRatio)
		}
	}
	return left, right, nil
}

// applyNativeNotePreviewEdges removes host-speaker clicks at the artificial
// beginning and end of the short audition. It does not alter song rendering
// or the target's hardware envelope.
func applyNativeNotePreviewEdges(samples []int8) {
	const edgeSamples = NativePreviewSampleRate / 250 // about 4 ms
	edge := min(edgeSamples, len(samples)/2)
	for index := 0; index < edge; index++ {
		samples[index] = int8(int(samples[index]) * index / edge)
		last := len(samples) - 1 - index
		samples[last] = int8(int(samples[last]) * index / edge)
	}
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
	initialized  bool
	tone         bool
	phase        float64
	noisePhase   float64
	lfsr         uint16
	elapsed      int
	sweepElapsed int
	period       int
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
			initialized: true,
			tone:        tone,
			lfsr:        0x7fff,
			period:      int(period),
		}
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
			value = nativePulseValue(phase, nativePulseDutyFraction(sound.Pulse1.Pulse.Duty))
			volume = nativeEnvelopeVolume(state.elapsed, sound.Pulse1.Pulse.Envelope.Initial, sound.Pulse1.Pulse.Envelope.Pace, sound.Pulse1.Pulse.Envelope.Direction == hw.EnvelopeUp)
		case audio.SoundCH2:
			if !state.tone {
				break
			}
			value = nativePulseValue(phase, nativePulseDutyFraction(sound.Pulse2.Duty))
			volume = nativeEnvelopeVolume(state.elapsed, sound.Pulse2.Envelope.Initial, sound.Pulse2.Envelope.Pace, sound.Pulse2.Envelope.Direction == hw.EnvelopeUp)
		case audio.SoundCH3:
			if !state.tone {
				break
			}
			wave := sound.Wave.Wave
			if wave.Length != 0 {
				waveIndex := int(phase*float64(wave.Length)) % int(wave.Length)
				value = (float64(wave.Samples[waveIndex]) - 7.5) / 7.5
			}
			volume = nativeWaveLevel(sound.Wave.Level)
		case audio.SoundCH4:
			frequency = nativeNoiseFrequency(sound.Noise.Divisor, sound.Noise.Shift)
			state.noisePhase += frequency / NativePreviewSampleRate
			for state.noisePhase >= 1 {
				bit := (state.lfsr ^ (state.lfsr >> 1)) & 1
				state.lfsr = (state.lfsr >> 1) | (bit << 14)
				if sound.Noise.Width == hw.Noise7Bit {
					state.lfsr = (state.lfsr &^ (1 << 6)) | (bit << 6)
				}
				state.noisePhase--
			}
			if state.lfsr&1 == 0 {
				value = 1
			} else {
				value = -1
			}
			volume = nativeEnvelopeVolume(state.elapsed, sound.Noise.Envelope.Initial, sound.Noise.Envelope.Pace, sound.Noise.Envelope.Direction == hw.EnvelopeUp)
		}
		target[start+index] += int16(value * volume * 112)
		if sound.Kind != audio.SoundCH4 && state.tone {
			state.phase = math.Mod(state.phase+frequency/NativePreviewSampleRate, 1)
		}
		state.elapsed++
		state.sweepElapsed++
	}
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
