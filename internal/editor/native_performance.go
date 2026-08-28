package editor

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/performance"
	"github.com/drpaneas/mtile/runtime/audio"
)

// ApplyNativePerformanceTake stores raw timing for later re-quantization and
// writes the quantized result into the selected part's physical CH1-CH4 clips.
func (m *Model) ApplyNativePerformanceTake(sequenceIndex int, raw performance.RawPerformanceTake, clock performance.TrackerClock, quantize performance.Quantize, overdub bool) (int, error) {
	return m.ApplyNativePerformanceTakeSwing(sequenceIndex, raw, clock, quantize, 50, overdub)
}

func (m *Model) ApplyNativePerformanceTakeSwing(sequenceIndex int, raw performance.RawPerformanceTake, clock performance.TrackerClock, quantize performance.Quantize, swing int, overdub bool) (int, error) {
	return m.applyNativePerformanceTake(sequenceIndex, raw, clock, quantize, swing, overdub, true)
}

func (m *Model) applyNativePerformanceTake(sequenceIndex int, raw performance.RawPerformanceTake, clock performance.TrackerClock, quantize performance.Quantize, swing int, overdub, storeRaw bool) (int, error) {
	if raw.Dropped != 0 {
		return -1, fmt.Errorf("performance take dropped %d events; it was not committed", raw.Dropped)
	}
	materialized := performance.MaterializeSwing(raw, clock, quantize, swing)
	takeID := -1
	err := m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if _, err := ensureNativeSongAndPart(candidate, m.NativeSongIndex); err != nil {
			return err
		}
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return errors.New("selected song part does not exist")
		}
		part := &candidate.Music.Parts[partIndex]
		if storeRaw {
			usedTakes := map[int]bool{}
			for _, take := range candidate.Music.Takes {
				usedTakes[take.ID] = true
			}
			var ok bool
			takeID, ok = firstFreeNativeID(usedTakes, audio.NativeMaxParts)
			if !ok {
				return errors.New("the project has no free raw take IDs")
			}
			candidate.Music.Takes = append(candidate.Music.Takes, nativeTakeAsset(takeID, part.ID, raw))
		}

		rangeEnd := int(clock.TickAt(clock.Origin + raw.Duration))
		if rangeEnd <= 0 {
			rangeEnd = 1
		}
		rangeEnd = min(rangeEnd, part.LengthTicks)
		for channel := 0; channel < 4; channel++ {
			recorded := notesForChannel(materialized.Notes, uint8(channel+1), part.LengthTicks)
			if len(recorded) == 0 && overdub {
				continue
			}
			clipIndex := -1
			lane := nativeLane(&part.Lanes, channel)
			if lane != nil && *lane != nil {
				clipIndex = nativeClipIndexByID(&candidate.Music, **lane)
			}
			if clipIndex < 0 && len(recorded) != 0 {
				var err error
				clipIndex, err = ensureNativeClip(candidate, m.NativeSongIndex, sequenceIndex, channel)
				if err != nil {
					return err
				}
			}
			if clipIndex < 0 {
				continue
			}
			clip := &candidate.Music.Clips[clipIndex]
			events := append([]assetsync.NativeEventAsset(nil), clip.Events...)
			if !overdub {
				events = removeNativeEventsInRange(events, 0, rangeEnd)
			}
			for _, note := range recorded {
				events = removeNativeEventsInRange(events, note.StartTick, note.StartTick+note.DurationTicks)
				segments := recordedNoteSegments(note, materialized.Parameters)
				previousSound := -1
				for _, segment := range segments {
					soundID, err := recordedSoundID(candidate, channel, segment, materialized.Parameters)
					if err != nil {
						return err
					}
					events = append(events, assetsync.NativeEventAsset{
						Tick: segment.StartTick, DurationTicks: segment.DurationTicks, Note: int(segment.Note),
						SoundID: soundID, Priority: 128, Legato: previousSound == soundID,
					})
					previousSound = soundID
				}
			}
			sort.SliceStable(events, func(i, j int) bool { return events[i].Tick < events[j].Tick })
			clip.Events = events
		}
		return nil
	})
	return takeID, err
}

// NativeTakesAt returns the retained high-resolution takes for one arranged
// part. Takes are authoring data and are never compiled into the cartridge.
func (m *Model) NativeTakesAt(sequenceIndex int) []assetsync.NativeTakeAsset {
	part, ok := m.NativePartAt(sequenceIndex)
	if !ok {
		return nil
	}
	takes := make([]assetsync.NativeTakeAsset, 0)
	for _, take := range m.NativeAudio.Music.Takes {
		if take.PartID == part.ID {
			takes = append(takes, take)
		}
	}
	return takes
}

// UseNativePerformanceTake re-quantizes a retained raw take without creating a
// duplicate. This lets the editor audition different grids and loop passes.
func (m *Model) UseNativePerformanceTake(sequenceIndex, takeID int, clock performance.TrackerClock, quantize performance.Quantize, overdub bool) error {
	return m.UseNativePerformanceTakeSwing(sequenceIndex, takeID, clock, quantize, 50, overdub)
}

func (m *Model) UseNativePerformanceTakeSwing(sequenceIndex, takeID int, clock performance.TrackerClock, quantize performance.Quantize, swing int, overdub bool) error {
	part, ok := m.NativePartAt(sequenceIndex)
	if !ok {
		return errors.New("selected song part does not exist")
	}
	for _, take := range m.NativeAudio.Music.Takes {
		if take.ID == takeID {
			if take.PartID != part.ID {
				return errors.New("selected take belongs to another song part")
			}
			_, err := m.applyNativePerformanceTake(sequenceIndex, NativeTakeRaw(take), clock, quantize, swing, overdub, false)
			return err
		}
	}
	return errors.New("selected performance take does not exist")
}

// recordedNoteSegments makes every quantized RETRIGGER/IMMEDIATE hardware
// parameter point audible even when it occurs during one held note. NEXT_NOTE
// changes stay in the automation lane and are picked up by the next trigger.
// The runtime stores full hardware sound states, so an audible change becomes
// a contiguous same-pitch segment.
func recordedNoteSegments(note performance.TrackerNote, parameters []performance.TrackerParameter) []performance.TrackerNote {
	end := note.StartTick + note.DurationTicks
	boundaries := make([]int, 0, len(parameters)+2)
	boundaries = append(boundaries, note.StartTick)
	for _, parameter := range parameters {
		if parameter.Channel == note.Channel && parameter.Parameter.Semantics() != performance.ParameterNextNote &&
			parameter.Tick > note.StartTick && parameter.Tick < end {
			boundaries = append(boundaries, parameter.Tick)
		}
	}
	boundaries = append(boundaries, end)
	sort.Ints(boundaries)
	segments := make([]performance.TrackerNote, 0, len(boundaries)-1)
	for index := 0; index+1 < len(boundaries); index++ {
		if index > 0 && boundaries[index] == boundaries[index-1] {
			continue
		}
		start, finish := boundaries[index], boundaries[index+1]
		if finish <= start {
			continue
		}
		segment := note
		segment.StartTick, segment.DurationTicks = start, finish-start
		segments = append(segments, segment)
	}
	return segments
}

func nativeTakeAsset(id, partID int, raw performance.RawPerformanceTake) assetsync.NativeTakeAsset {
	take := assetsync.NativeTakeAsset{
		ID: id, Name: fmt.Sprintf("Take %d", id+1), PartID: partID,
		DurationNanos: raw.Duration.Nanoseconds(), Events: make([]assetsync.NativeRawEventAsset, 0, len(raw.Events)),
	}
	for _, event := range raw.Events {
		var kind string
		switch event.Kind {
		case performance.ActionStop:
			kind = "stop"
		case performance.ActionParameter:
			kind = "parameter"
		default:
			kind = "start"
		}
		take.Events = append(take.Events, assetsync.NativeRawEventAsset{
			OffsetNanos: event.Offset.Nanoseconds(), Kind: kind, Channel: int(event.Channel),
			Note: int(event.Note), Parameter: performanceParameterName(event.Parameter), Value: int(event.Value),
		})
		if kind != "parameter" {
			take.Events[len(take.Events)-1].Parameter = ""
			take.Events[len(take.Events)-1].Value = 0
		}
	}
	return take
}

func performanceParameterName(parameter performance.Parameter) string {
	return [...]string{
		"duty", "volume", "envelope-pace", "envelope-direction", "sweep-pace", "sweep-direction",
		"sweep-shift", "wave-level", "noise-width", "noise-divisor", "noise-shift",
	}[min(int(parameter), int(performance.ParameterNoiseShift))]
}

func notesForChannel(notes []performance.TrackerNote, channel uint8, partTicks int) []performance.TrackerNote {
	result := make([]performance.TrackerNote, 0, len(notes))
	for _, note := range notes {
		if note.Channel != channel || note.StartTick < 0 || note.StartTick >= partTicks {
			continue
		}
		note.DurationTicks = min(note.DurationTicks, partTicks-note.StartTick)
		if note.DurationTicks > 0 {
			result = append(result, note)
		}
	}
	return result
}

func removeNativeEventsInRange(events []assetsync.NativeEventAsset, start, end int) []assetsync.NativeEventAsset {
	kept := events[:0]
	for _, event := range events {
		if event.Tick < end && event.Tick+event.DurationTicks > start {
			continue
		}
		kept = append(kept, event)
	}
	return kept
}

func recordedSoundID(candidate *assetsync.NativeAudioAssets, channel int, note performance.TrackerNote, parameters []performance.TrackerParameter) (int, error) {
	base, ok := recordedBaseSound(candidate, channel, note.Note)
	if !ok {
		return -1, fmt.Errorf("CH%d has no sound for recorded note %d", channel+1, note.Note)
	}
	base = cloneNativeSoundAsset(base)
	changed := false
	for _, parameter := range parameters {
		if int(parameter.Channel) != channel+1 || parameter.Tick > note.StartTick {
			continue
		}
		applyRecordedParameter(&base, parameter.Parameter, parameter.Value)
		changed = true
	}
	if !changed {
		return base.ID, nil
	}
	for _, sound := range candidate.Sounds.Sounds {
		if sameNativeSoundConfig(sound, base) {
			return sound.ID, nil
		}
	}
	used := map[int]bool{}
	for _, sound := range candidate.Sounds.Sounds {
		used[sound.ID] = true
	}
	id, ok := firstFreeNativeID(used, audio.NativeMaxSounds)
	if !ok {
		return -1, errors.New("the project has no free sound IDs for recorded automation")
	}
	base.ID, base.Name = id, fmt.Sprintf("CH%d take sound", channel+1)
	candidate.Sounds.Sounds = append(candidate.Sounds.Sounds, base)
	return id, nil
}

func cloneNativeSoundAsset(sound assetsync.NativeSoundAsset) assetsync.NativeSoundAsset {
	clone := sound
	if sound.Pulse != nil {
		pulse := *sound.Pulse
		if sound.Pulse.Sweep != nil {
			sweep := *sound.Pulse.Sweep
			pulse.Sweep = &sweep
		}
		clone.Pulse = &pulse
	}
	if sound.Wave != nil {
		wave := *sound.Wave
		wave.Samples = append([]int(nil), sound.Wave.Samples...)
		clone.Wave = &wave
	}
	if sound.Noise != nil {
		noise := *sound.Noise
		clone.Noise = &noise
	}
	return clone
}

func recordedBaseSound(candidate *assetsync.NativeAudioAssets, channel int, note uint8) (assetsync.NativeSoundAsset, bool) {
	if channel == 3 && note >= 36 && note <= 43 {
		name := nativeNoisePresetNames[note-36]
		for _, sound := range candidate.Sounds.Sounds {
			if sound.Kind == "ch4" && sound.Name == name {
				return sound, true
			}
		}
		used := map[int]bool{}
		for _, sound := range candidate.Sounds.Sounds {
			used[sound.ID] = true
		}
		id, ok := firstFreeNativeID(used, audio.NativeMaxSounds)
		if !ok {
			return assetsync.NativeSoundAsset{}, false
		}
		sound := defaultNativeNoiseSound(id, int(note-36))
		candidate.Sounds.Sounds = append(candidate.Sounds.Sounds, sound)
		return sound, true
	}
	for _, sound := range candidate.Sounds.Sounds {
		if sound.Kind == nativeKindName(channel) {
			return sound, true
		}
	}
	used := map[int]bool{}
	for _, sound := range candidate.Sounds.Sounds {
		used[sound.ID] = true
	}
	id, ok := firstFreeNativeID(used, audio.NativeMaxSounds)
	if !ok {
		return assetsync.NativeSoundAsset{}, false
	}
	sound := defaultNativeSound(id, channel)
	candidate.Sounds.Sounds = append(candidate.Sounds.Sounds, sound)
	return sound, true
}

func sameNativeSoundConfig(left, right assetsync.NativeSoundAsset) bool {
	left.ID, right.ID = 0, 0
	left.Name, right.Name = "", ""
	return reflect.DeepEqual(left, right)
}

func applyRecordedParameter(sound *assetsync.NativeSoundAsset, parameter performance.Parameter, value int16) {
	if sound == nil {
		return
	}
	clamp := func(value int16, high int16) int { return int(min(max(value, 0), high)) }
	switch parameter {
	case performance.ParameterDuty:
		if sound.Pulse != nil {
			duty := float64(value)
			if value == 12 {
				duty = 12.5
			}
			if duty == 12.5 || duty == 25 || duty == 50 || duty == 75 {
				sound.Pulse.Duty = duty
			}
		} else if sound.Wave != nil {
			wave := performance.PulseWaveform(int(value))
			sound.Wave.Samples = make([]int, 32)
			for index, sample := range wave {
				sound.Wave.Samples[index] = int(sample)
			}
		}
	case performance.ParameterVolume:
		if sound.Pulse != nil {
			sound.Pulse.Envelope.Initial = clamp(value, 15)
		} else if sound.Noise != nil {
			sound.Noise.Envelope.Initial = clamp(value, 15)
		}
	case performance.ParameterEnvelopePace:
		if sound.Pulse != nil {
			sound.Pulse.Envelope.Pace = clamp(value, 7)
		} else if sound.Noise != nil {
			sound.Noise.Envelope.Pace = clamp(value, 7)
		}
	case performance.ParameterEnvelopeDirection:
		direction := "down"
		if value > 0 {
			direction = "up"
		}
		if sound.Pulse != nil {
			sound.Pulse.Envelope.Direction = direction
		} else if sound.Noise != nil {
			sound.Noise.Envelope.Direction = direction
		}
	case performance.ParameterSweepPace:
		if sound.Pulse != nil && sound.Pulse.Sweep != nil {
			sound.Pulse.Sweep.Pace = clamp(value, 7)
		}
	case performance.ParameterSweepDirection:
		if sound.Pulse != nil && sound.Pulse.Sweep != nil {
			sound.Pulse.Sweep.Direction = "down"
			if value > 0 {
				sound.Pulse.Sweep.Direction = "up"
			}
		}
	case performance.ParameterSweepShift:
		if sound.Pulse != nil && sound.Pulse.Sweep != nil {
			sound.Pulse.Sweep.Shift = clamp(value, 7)
		}
	case performance.ParameterWaveLevel:
		if sound.Wave != nil {
			sound.Wave.Level = [...]int{0, 25, 50, 75, 100}[clamp(value, 4)]
		}
	case performance.ParameterNoiseWidth:
		if sound.Noise != nil {
			sound.Noise.Width = 15
			if value != 0 {
				sound.Noise.Width = 7
			}
		}
	case performance.ParameterNoiseDivisor:
		if sound.Noise != nil {
			sound.Noise.Divisor = clamp(value, 7)
		}
	case performance.ParameterNoiseShift:
		if sound.Noise != nil {
			sound.Noise.Shift = clamp(value, 13)
		}
	}
}

// NativeTakeRaw reconstructs the source timing retained in music.json.
func NativeTakeRaw(take assetsync.NativeTakeAsset) performance.RawPerformanceTake {
	raw := performance.RawPerformanceTake{Duration: time.Duration(take.DurationNanos), Events: make([]performance.RawEvent, 0, len(take.Events))}
	for _, event := range take.Events {
		var kind performance.ActionKind
		switch event.Kind {
		case "stop":
			kind = performance.ActionStop
		case "parameter":
			kind = performance.ActionParameter
		default:
			kind = performance.ActionStart
		}
		raw.Events = append(raw.Events, performance.RawEvent{
			Offset: time.Duration(event.OffsetNanos), Kind: kind, Channel: uint8(event.Channel), Note: uint8(event.Note),
			Parameter: performanceParameter(event.Parameter), Value: int16(event.Value),
		})
	}
	return raw
}

func performanceParameter(name string) performance.Parameter {
	names := [...]string{"duty", "volume", "envelope-pace", "envelope-direction", "sweep-pace", "sweep-direction", "sweep-shift", "wave-level", "noise-width", "noise-divisor", "noise-shift"}
	for parameter, candidate := range names {
		if name == candidate {
			return performance.Parameter(parameter)
		}
	}
	return performance.ParameterDuty
}
