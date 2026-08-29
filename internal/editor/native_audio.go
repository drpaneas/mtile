package editor

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/runtime/audio"
)

const (
	// The authoring grid is 1/64-note resolution. The target scheduler remains
	// 96 PPQN, so every visible cell maps exactly to 6 runtime ticks. Existing
	// projects retain their absolute ticks; the finer grid only makes more of
	// the scheduler's native timing editable and quantizable.
	NativeStepsPerPart = 64
	NativeTicksPerStep = audio.NativePPQN / 16
	NativePartTicks    = NativeStepsPerPart * NativeTicksPerStep
)

const NativeMaxPartBars = audio.NativeMaxPartBars

// NativePartMeter returns musical metadata for an authored part. Older v2
// files did not store meter, so their existing tick length remains valid and
// is presented as one or more 4/4 bars without requiring a migration.
func NativePartMeter(part assetsync.NativePartAsset) (bars, beatsPerBar, beatUnit, ticksPerBar int) {
	beatsPerBar, beatUnit = part.BeatsPerBar, part.BeatUnit
	if beatsPerBar == 0 || beatUnit == 0 {
		beatsPerBar, beatUnit = 4, 4
	}
	ticksPerBar = audio.NativePPQN * 4 * beatsPerBar / beatUnit
	if ticksPerBar < 1 {
		ticksPerBar = NativePartTicks
	}
	bars = part.Bars
	if bars == 0 {
		bars = max(1, (part.LengthTicks+ticksPerBar-1)/ticksPerBar)
	}
	return bars, beatsPerBar, beatUnit, ticksPerBar
}

// NativeGridCell is one piano-roll cell visited by a pointer gesture.
type NativeGridCell struct {
	Step int
	Row  int
}

// NativeDragCells returns the new cells between two sampled pointer
// positions. Piano-roll clips can hold one note per step, so interpolation
// emits exactly one row for every crossed step and always includes the final
// cell. A vertical move within one step replaces that step's note once.
func NativeDragCells(fromStep, fromRow, toStep, toRow int) []NativeGridCell {
	if fromStep < 0 || fromRow < 0 {
		return []NativeGridCell{{Step: toStep, Row: toRow}}
	}
	if fromStep == toStep {
		if fromRow == toRow {
			return nil
		}
		return []NativeGridCell{{Step: toStep, Row: toRow}}
	}

	distance := toStep - fromStep
	direction := 1
	if distance < 0 {
		direction = -1
		distance = -distance
	}
	cells := make([]NativeGridCell, 0, distance)
	for offset := 1; offset <= distance; offset++ {
		row := fromRow + int(math.Round(float64(toRow-fromRow)*float64(offset)/float64(distance)))
		cells = append(cells, NativeGridCell{Step: fromStep + direction*offset, Row: row})
	}
	return cells
}

// NativeAudioClone gives undo/edit transactions a deep copy of every authored
// slice and pointer. The compiled Library is rebuilt after each accepted edit.
func NativeAudioClone(source assetsync.NativeAudioAssets) assetsync.NativeAudioAssets {
	data, err := json.Marshal(source)
	if err != nil {
		return source
	}
	var clone assetsync.NativeAudioAssets
	if err := json.Unmarshal(data, &clone); err != nil {
		return source
	}
	clone.Library = source.Library
	return clone
}

func (m *Model) updateNativeAudio(edit func(*assetsync.NativeAudioAssets) error) error {
	if m == nil || m.AudioVersion != assetsync.NativeAudioSchemaVersion {
		return errors.New("the project does not use four-channel audio v2")
	}
	candidate := NativeAudioClone(m.NativeAudio)
	if err := edit(&candidate); err != nil {
		return err
	}
	library, err := assetsync.CompileNativeAudio(candidate.Settings, candidate.Sounds, candidate.Music)
	if err != nil {
		return err
	}
	candidate.Library = library
	m.NativeAudio = candidate
	return nil
}

func firstFreeNativeID(used map[int]bool, limit int) (int, bool) {
	for id := 0; id < limit; id++ {
		if !used[id] {
			return id, true
		}
	}
	return 0, false
}

func nativeLane(lanes *assetsync.NativeLaneAssets, channel int) **int {
	switch channel {
	case 0:
		return &lanes.CH1
	case 1:
		return &lanes.CH2
	case 2:
		return &lanes.CH3
	case 3:
		return &lanes.CH4
	default:
		return nil
	}
}

func nativeKindName(channel int) string { return fmt.Sprintf("ch%d", channel+1) }

func defaultNativeSound(id, channel int) assetsync.NativeSoundAsset {
	sound := assetsync.NativeSoundAsset{ID: id, Kind: nativeKindName(channel), Route: "both"}
	switch channel {
	case 0:
		sound.Name = "Pulse + sweep"
		sound.Pulse = &assetsync.NativePulseAsset{
			Duty: 50, Envelope: assetsync.NativeEnvelopeAsset{Initial: 12, Direction: "down", Pace: 0},
			Sweep: &assetsync.NativeSweepAsset{Pace: 0, Direction: "down", Shift: 0},
		}
	case 1:
		sound.Name = "Pulse"
		sound.Pulse = &assetsync.NativePulseAsset{
			Duty: 25, Envelope: assetsync.NativeEnvelopeAsset{Initial: 11, Direction: "down", Pace: 0},
		}
	case 2:
		sound.Name = "Wave"
		sound.Wave = &assetsync.NativeWaveAsset{
			Samples: []int{8, 10, 12, 14, 15, 14, 12, 10, 8, 6, 4, 2, 0, 2, 4, 6, 8, 10, 12, 14, 15, 14, 12, 10, 8, 6, 4, 2, 0, 2, 4, 6},
			Level:   100,
		}
	case 3:
		return defaultNativeNoiseSound(id, 0)
	}
	return sound
}

var nativeNoisePresetNames = [...]string{"Kick", "Snare", "Closed hat", "Open hat", "Tom 1", "Tom 2", "Click", "FX"}

func defaultNativeNoiseSound(id, preset int) assetsync.NativeSoundAsset {
	preset = min(max(preset, 0), len(nativeNoisePresetNames)-1)
	sound := assetsync.NativeSoundAsset{ID: id, Name: nativeNoisePresetNames[preset], Kind: "ch4", Route: "both"}
	values := [...]assetsync.NativeNoiseAsset{
		{Width: 15, Divisor: 7, Shift: 7, Envelope: assetsync.NativeEnvelopeAsset{Initial: 15, Direction: "down", Pace: 2}},
		{Width: 15, Divisor: 3, Shift: 4, Envelope: assetsync.NativeEnvelopeAsset{Initial: 14, Direction: "down", Pace: 3}},
		{Width: 7, Divisor: 1, Shift: 2, Envelope: assetsync.NativeEnvelopeAsset{Initial: 10, Direction: "down", Pace: 1}},
		{Width: 7, Divisor: 2, Shift: 3, Envelope: assetsync.NativeEnvelopeAsset{Initial: 12, Direction: "down", Pace: 4}},
		{Width: 15, Divisor: 5, Shift: 6, Envelope: assetsync.NativeEnvelopeAsset{Initial: 14, Direction: "down", Pace: 2}},
		{Width: 15, Divisor: 3, Shift: 6, Envelope: assetsync.NativeEnvelopeAsset{Initial: 13, Direction: "down", Pace: 2}},
		{Width: 7, Divisor: 1, Shift: 8, Envelope: assetsync.NativeEnvelopeAsset{Initial: 11, Direction: "down", Pace: 1}},
		{Width: 15, Divisor: 1, Shift: 3, Envelope: assetsync.NativeEnvelopeAsset{Initial: 15, Direction: "down", Pace: 6}},
	}
	noise := values[preset]
	sound.Noise = &noise
	return sound
}

func ensureNativeSongAndPart(audioAssets *assetsync.NativeAudioAssets, songIndex int) (int, error) {
	music := &audioAssets.Music
	if len(music.Songs) == 0 {
		music.Songs = append(music.Songs, assetsync.NativeSongAsset{ID: 0, Name: "New Song", Tempo: audio.NativeDefaultTempo})
	}
	if songIndex < 0 || songIndex >= len(music.Songs) {
		return 0, errors.New("selected song does not exist")
	}
	song := &music.Songs[songIndex]
	if len(song.Sequence) == 0 {
		used := make(map[int]bool, len(music.Parts))
		for _, part := range music.Parts {
			used[part.ID] = true
		}
		id, ok := firstFreeNativeID(used, audio.NativeMaxParts)
		if !ok {
			return 0, errors.New("the song has no free section IDs")
		}
		music.Parts = append(music.Parts, assetsync.NativePartAsset{ID: id, Name: "Part A", LengthTicks: NativePartTicks, Bars: 1, BeatsPerBar: 4, BeatUnit: 4})
		song.Sequence = append(song.Sequence, id)
	}
	return 0, nil
}

func nativePartIndexForSequence(music *assetsync.NativeMusicLibrary, songIndex, sequenceIndex int) int {
	if songIndex < 0 || songIndex >= len(music.Songs) || sequenceIndex < 0 || sequenceIndex >= len(music.Songs[songIndex].Sequence) {
		return -1
	}
	id := music.Songs[songIndex].Sequence[sequenceIndex]
	for index := range music.Parts {
		if music.Parts[index].ID == id {
			return index
		}
	}
	return -1
}

func nativeClipIndexByID(music *assetsync.NativeMusicLibrary, id int) int {
	for index := range music.Clips {
		if music.Clips[index].ID == id {
			return index
		}
	}
	return -1
}

// cleanupNativeMusicOrphans removes editor-created parts/clips that no song or
// part can reach. Explicit standalone SFX survive by design. Reusable sounds
// are retained even while unused because they are the project's instrument
// library, not disposable event data.
func cleanupNativeMusicOrphans(music *assetsync.NativeMusicLibrary) {
	partUsed := map[int]bool{}
	for _, song := range music.Songs {
		for _, partID := range song.Sequence {
			partUsed[partID] = true
		}
	}
	parts := music.Parts[:0]
	for _, part := range music.Parts {
		if partUsed[part.ID] {
			parts = append(parts, part)
		}
	}
	music.Parts = parts

	clipUsed := map[int]bool{}
	for _, part := range music.Parts {
		for _, lane := range []*int{part.Lanes.CH1, part.Lanes.CH2, part.Lanes.CH3, part.Lanes.CH4} {
			if lane != nil {
				clipUsed[*lane] = true
			}
		}
	}
	clips := music.Clips[:0]
	for _, clip := range music.Clips {
		if clip.SFX || clipUsed[clip.ID] {
			clips = append(clips, clip)
		}
	}
	music.Clips = clips
}

func (m *Model) NativeSong() (assetsync.NativeSongAsset, bool) {
	if m == nil || m.NativeSongIndex < 0 || m.NativeSongIndex >= len(m.NativeAudio.Music.Songs) {
		return assetsync.NativeSongAsset{}, false
	}
	return m.NativeAudio.Music.Songs[m.NativeSongIndex], true
}

func (m *Model) NativeSongs() []assetsync.NativeSongAsset {
	if m == nil {
		return nil
	}
	return append([]assetsync.NativeSongAsset(nil), m.NativeAudio.Music.Songs...)
}

func (m *Model) SelectNativeSong(index int) error {
	if m == nil || index < 0 || index >= len(m.NativeAudio.Music.Songs) {
		return errors.New("selected song does not exist")
	}
	m.NativeSongIndex = index
	return nil
}

func (m *Model) AddNativeSong(name string) (int, error) {
	added := -1
	err := m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if len(candidate.Music.Songs) >= audio.NativeMaxSongs {
			return errors.New("the project has no free song IDs")
		}
		used := map[int]bool{}
		for _, song := range candidate.Music.Songs {
			used[song.ID] = true
		}
		id, ok := firstFreeNativeID(used, audio.NativeMaxSongs)
		if !ok {
			return errors.New("the project has no free song IDs")
		}
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("Song %d", len(candidate.Music.Songs)+1)
		}
		candidate.Music.Songs = append(candidate.Music.Songs, assetsync.NativeSongAsset{ID: id, Name: name, Tempo: audio.NativeDefaultTempo})
		added = len(candidate.Music.Songs) - 1
		_, err := ensureNativeSongAndPart(candidate, added)
		return err
	})
	if err == nil {
		m.NativeSongIndex = added
	}
	return added, err
}

func (m *Model) DeleteNativeSong(index int) (int, error) {
	selected := index
	err := m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if index < 0 || index >= len(candidate.Music.Songs) {
			return errors.New("selected song does not exist")
		}
		if len(candidate.Music.Songs) == 1 {
			return errors.New("a project must keep one song")
		}
		candidate.Music.Songs = append(candidate.Music.Songs[:index], candidate.Music.Songs[index+1:]...)
		cleanupNativeMusicOrphans(&candidate.Music)
		selected = min(index, len(candidate.Music.Songs)-1)
		return nil
	})
	if err == nil {
		m.NativeSongIndex = selected
	}
	return selected, err
}

func (m *Model) NativePartAt(sequenceIndex int) (assetsync.NativePartAsset, bool) {
	if m == nil {
		return assetsync.NativePartAsset{}, false
	}
	index := nativePartIndexForSequence(&m.NativeAudio.Music, m.NativeSongIndex, sequenceIndex)
	if index < 0 {
		return assetsync.NativePartAsset{}, false
	}
	return m.NativeAudio.Music.Parts[index], true
}

// NativePartTempo returns the effective tempo and whether the selected part
// overrides its song. A zero part tempo is the backward-compatible inherit
// representation stored in the asset file and target ROM.
func (m *Model) NativePartTempo(sequenceIndex int) (tempo int, overridden bool) {
	song, songOK := m.NativeSong()
	part, partOK := m.NativePartAt(sequenceIndex)
	if partOK && part.Tempo != 0 {
		return part.Tempo, true
	}
	if songOK {
		return song.Tempo, false
	}
	return audio.NativeDefaultTempo, false
}

func (m *Model) SetNativePartTempo(sequenceIndex, tempo int) error {
	if tempo != 0 && (tempo < 20 || tempo > 400) {
		return errors.New("part tempo must be 20..400 BPM or zero to inherit")
	}
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if _, err := ensureNativeSongAndPart(candidate, m.NativeSongIndex); err != nil {
			return err
		}
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return errors.New("selected song section does not exist")
		}
		candidate.Music.Parts[partIndex].Tempo = tempo
		return nil
	})
}

func (m *Model) NativePartStepCount(sequenceIndex int) int {
	part, ok := m.NativePartAt(sequenceIndex)
	if !ok {
		return NativeStepsPerPart
	}
	return max(1, (part.LengthTicks+NativeTicksPerStep-1)/NativeTicksPerStep)
}

func (m *Model) NativePartBarStepCount(sequenceIndex int) int {
	part, ok := m.NativePartAt(sequenceIndex)
	if !ok {
		return NativeStepsPerPart
	}
	_, _, _, ticksPerBar := NativePartMeter(part)
	return max(1, ticksPerBar/NativeTicksPerStep)
}

func (m *Model) NativeClipAt(sequenceIndex, channel int) (assetsync.NativeClipAsset, bool) {
	part, ok := m.NativePartAt(sequenceIndex)
	if !ok {
		return assetsync.NativeClipAsset{}, false
	}
	lane := nativeLane(&part.Lanes, channel)
	if lane == nil || *lane == nil {
		return assetsync.NativeClipAsset{}, false
	}
	index := nativeClipIndexByID(&m.NativeAudio.Music, **lane)
	if index < 0 {
		return assetsync.NativeClipAsset{}, false
	}
	return m.NativeAudio.Music.Clips[index], true
}

func (m *Model) NativeClipByID(id int) (assetsync.NativeClipAsset, bool) {
	if m == nil {
		return assetsync.NativeClipAsset{}, false
	}
	index := nativeClipIndexByID(&m.NativeAudio.Music, id)
	if index < 0 {
		return assetsync.NativeClipAsset{}, false
	}
	return m.NativeAudio.Music.Clips[index], true
}

func nativeSFXClipIndex(music *assetsync.NativeMusicLibrary, id int) int {
	index := nativeClipIndexByID(music, id)
	if index < 0 || !music.Clips[index].SFX {
		return -1
	}
	return index
}

// NativeSFXClips are explicitly standalone typed PSG clips. The explicit bit
// prevents a detached/deleted song lane from silently becoming a gameplay SFX.
// Their stable clip ID is directly usable with audio.PlaySFX.
func (m *Model) NativeSFXClips() []assetsync.NativeClipAsset {
	if m == nil {
		return nil
	}
	result := make([]assetsync.NativeClipAsset, 0)
	for _, clip := range m.NativeAudio.Music.Clips {
		if clip.SFX {
			copy := clip
			copy.Events = append([]assetsync.NativeEventAsset(nil), clip.Events...)
			result = append(result, copy)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (m *Model) CreateNativeSFX(channel int, name string) (int, error) {
	created := -1
	err := m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if channel < 0 || channel > 3 {
			return errors.New("effect channel must be CH1, CH2, CH3, or CH4")
		}
		used := map[int]bool{}
		for _, clip := range candidate.Music.Clips {
			used[clip.ID] = true
		}
		id, ok := firstFreeNativeID(used, audio.NativeMaxClips)
		if !ok {
			return errors.New("the project has no free SFX clip IDs")
		}
		name = strings.TrimSpace(name)
		if name == "" {
			name = fmt.Sprintf("CH%d effect %02d", channel+1, id)
		}
		candidate.Music.Clips = append(candidate.Music.Clips, assetsync.NativeClipAsset{
			ID: id, Name: name, Kind: nativeKindName(channel), SFX: true, LengthTicks: NativePartTicks,
		})
		created = id
		return nil
	})
	return created, err
}

func (m *Model) DeleteNativeSFX(clipID int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		for _, part := range candidate.Music.Parts {
			for _, lane := range []*int{part.Lanes.CH1, part.Lanes.CH2, part.Lanes.CH3, part.Lanes.CH4} {
				if lane != nil && *lane == clipID {
					return errors.New("song clips cannot be deleted from the SFX library")
				}
			}
		}
		index := nativeClipIndexByID(&candidate.Music, clipID)
		if index < 0 {
			return errors.New("selected effect does not exist")
		}
		if !candidate.Music.Clips[index].SFX {
			return errors.New("song clips cannot be deleted from the SFX library")
		}
		candidate.Music.Clips = append(candidate.Music.Clips[:index], candidate.Music.Clips[index+1:]...)
		return nil
	})
}

func nativeEventAtStep(clip assetsync.NativeClipAsset, step int) (assetsync.NativeEventAsset, int, bool) {
	if step < 0 || step*NativeTicksPerStep >= clip.LengthTicks {
		return assetsync.NativeEventAsset{}, 0, false
	}
	cellStart := step * NativeTicksPerStep
	cellEnd := cellStart + NativeTicksPerStep
	for _, event := range clip.Events {
		if event.Tick < cellEnd && event.Tick+event.DurationTicks > cellStart {
			return event, event.Tick / NativeTicksPerStep, true
		}
	}
	return assetsync.NativeEventAsset{}, 0, false
}

func (m *Model) NativeSFXEventAtStep(clipID, step int) (assetsync.NativeEventAsset, int, bool) {
	clip, ok := m.NativeClipByID(clipID)
	if !ok || !clip.SFX {
		return assetsync.NativeEventAsset{}, 0, false
	}
	return nativeEventAtStep(clip, step)
}

func (m *Model) NativeSFXSoundAtStep(clipID, step int) (assetsync.NativeSoundAsset, bool) {
	clip, ok := m.NativeClipByID(clipID)
	if !ok || !clip.SFX {
		return assetsync.NativeSoundAsset{}, false
	}
	if event, _, exists := nativeEventAtStep(clip, step); exists {
		return m.NativeSoundByID(event.SoundID)
	}
	for _, sound := range m.NativeAudio.Sounds.Sounds {
		if sound.Kind == clip.Kind {
			return sound, true
		}
	}
	return assetsync.NativeSoundAsset{}, false
}

// NativeNoteEventAtStep returns the monophonic note occupying a piano-roll
// step, together with the step where that note starts. Duration is part of the
// authored event, so a sustained note is selectable from any covered cell.
func (m *Model) NativeNoteEventAtStep(sequenceIndex, channel, step int) (assetsync.NativeEventAsset, int, bool) {
	if step < 0 {
		return assetsync.NativeEventAsset{}, 0, false
	}
	clip, ok := m.NativeClipAt(sequenceIndex, channel)
	if !ok {
		return assetsync.NativeEventAsset{}, 0, false
	}
	if step*NativeTicksPerStep >= clip.LengthTicks {
		return assetsync.NativeEventAsset{}, 0, false
	}
	cellStart := step * NativeTicksPerStep
	cellEnd := cellStart + NativeTicksPerStep
	for _, event := range clip.Events {
		// Raw/unquantized MIDI may begin between visible grid lines. Treat any
		// overlap with this 1/64 cell as selectable, including very short notes.
		if event.Tick < cellEnd && event.Tick+event.DurationTicks > cellStart {
			return event, event.Tick / NativeTicksPerStep, true
		}
	}
	return assetsync.NativeEventAsset{}, 0, false
}

func nativeEventInRange(event assetsync.NativeEventAsset, startTick, endTick, lowNote, highNote int) bool {
	return event.Tick < endTick && event.Tick+event.DurationTicks > startTick && event.Note >= lowNote && event.Note <= highNote
}

// NativeNoteEventsInRange returns detached events with ticks relative to the
// earliest selected note. This is the note-level clipboard format used by the
// piano roll and avoids preserving empty marquee space before the first note.
func (m *Model) NativeNoteEventsInRange(sequenceIndex, channel, startTick, endTick, lowNote, highNote int) []assetsync.NativeEventAsset {
	clip, ok := m.NativeClipAt(sequenceIndex, channel)
	if !ok || startTick < 0 || endTick <= startTick {
		return nil
	}
	events := make([]assetsync.NativeEventAsset, 0)
	firstTick := int(^uint(0) >> 1)
	for _, event := range clip.Events {
		if nativeEventInRange(event, startTick, endTick, lowNote, highNote) {
			events = append(events, event)
			firstTick = min(firstTick, event.Tick)
		}
	}
	for index := range events {
		events[index].Tick -= firstTick
	}
	return events
}

// NativeNoteRangeBounds returns the tight bounds of the notes intersecting a
// marquee. Keeping this in the headless model lets range movement validate the
// actual notes rather than empty space around them.
func (m *Model) NativeNoteRangeBounds(sequenceIndex, channel, startTick, endTick, lowNote, highNote int) (start, end, low, high int, ok bool) {
	clip, exists := m.NativeClipAt(sequenceIndex, channel)
	if !exists || startTick < 0 || endTick <= startTick {
		return 0, 0, 0, 0, false
	}
	for _, event := range clip.Events {
		if !nativeEventInRange(event, startTick, endTick, lowNote, highNote) {
			continue
		}
		if !ok {
			start, end, low, high, ok = event.Tick, event.Tick+event.DurationTicks, event.Note, event.Note, true
			continue
		}
		start = min(start, event.Tick)
		end = max(end, event.Tick+event.DurationTicks)
		low, high = min(low, event.Note), max(high, event.Note)
	}
	return start, end, low, high, ok
}

func (m *Model) DeleteNativeNoteRange(sequenceIndex, channel, startTick, endTick, lowNote, highNote int) error {
	if startTick < 0 || endTick <= startTick || lowNote < 0 || highNote > 127 || lowNote > highNote {
		return errors.New("note selection is outside the piano roll")
	}
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return nil
		}
		lane := nativeLane(&candidate.Music.Parts[partIndex].Lanes, channel)
		if lane == nil || *lane == nil {
			return nil
		}
		clipIndex := nativeClipIndexByID(&candidate.Music, **lane)
		if clipIndex < 0 {
			return nil
		}
		clip := &candidate.Music.Clips[clipIndex]
		kept := clip.Events[:0]
		for _, event := range clip.Events {
			if !nativeEventInRange(event, startTick, endTick, lowNote, highNote) {
				kept = append(kept, event)
			}
		}
		clip.Events = kept
		return nil
	})
}

// PasteNativeNoteEvents inserts relative-tick clipboard events and preserves
// their instrument, priority, duration, and legato fields.
func (m *Model) PasteNativeNoteEvents(sequenceIndex, channel, destinationTick int, events []assetsync.NativeEventAsset) error {
	if destinationTick < 0 || len(events) == 0 {
		return errors.New("note clipboard is empty or outside the piano roll")
	}
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		clipIndex, err := ensureNativeClip(candidate, m.NativeSongIndex, sequenceIndex, channel)
		if err != nil {
			return err
		}
		clip := &candidate.Music.Clips[clipIndex]
		for _, event := range events {
			event.Tick += destinationTick
			if event.Tick < 0 || event.Tick >= clip.LengthTicks || event.DurationTicks < 1 || event.Note < 0 || event.Note > 127 {
				return errors.New("pasted notes do not fit in the selected clip")
			}
			validSound := false
			for _, sound := range candidate.Sounds.Sounds {
				if sound.ID == event.SoundID && sound.Kind == clip.Kind {
					validSound = true
					break
				}
			}
			if !validSound {
				return errors.New("pasted note instrument does not match this hardware channel")
			}
			setNativeClipEventAsset(clip, event)
		}
		return nil
	})
}

func (m *Model) MoveNativeNoteRange(sequenceIndex, channel, startTick, endTick, lowNote, highNote, deltaTick, deltaNote int) error {
	if startTick < 0 || endTick <= startTick || lowNote < 0 || highNote > 127 || lowNote > highNote {
		return errors.New("note selection is outside the piano roll")
	}
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return errors.New("selected song section does not exist")
		}
		lane := nativeLane(&candidate.Music.Parts[partIndex].Lanes, channel)
		if lane == nil || *lane == nil {
			return errors.New("note selection is empty")
		}
		clipIndex := nativeClipIndexByID(&candidate.Music, **lane)
		if clipIndex < 0 {
			return errors.New("selected clip does not exist")
		}
		clip := &candidate.Music.Clips[clipIndex]
		moved, kept := make([]assetsync.NativeEventAsset, 0), make([]assetsync.NativeEventAsset, 0, len(clip.Events))
		for _, event := range clip.Events {
			if nativeEventInRange(event, startTick, endTick, lowNote, highNote) {
				event.Tick += deltaTick
				event.Note += deltaNote
				if event.Tick < 0 || event.Tick+event.DurationTicks > clip.LengthTicks || event.Note < 0 || event.Note > 127 {
					return errors.New("moved notes would leave the piano roll")
				}
				moved = append(moved, event)
			} else {
				kept = append(kept, event)
			}
		}
		if len(moved) == 0 {
			return errors.New("note selection is empty")
		}
		clip.Events = kept
		for _, event := range moved {
			setNativeClipEventAsset(clip, event)
		}
		return nil
	})
}

func (m *Model) NativeSoundAt(sequenceIndex, channel int) (assetsync.NativeSoundAsset, bool) {
	return m.NativeSoundAtStep(sequenceIndex, channel, -1)
}

// NativeSoundsForChannel returns stable-ID instrument presets that are valid
// for one physical hardware channel. The returned slice is detached from the
// model so UI selection cannot mutate project data accidentally.
func (m *Model) NativeSoundsForChannel(channel int) []assetsync.NativeSoundAsset {
	if m == nil || channel < 0 || channel > 3 {
		return nil
	}
	kind := nativeKindName(channel)
	result := make([]assetsync.NativeSoundAsset, 0)
	for _, sound := range m.NativeAudio.Sounds.Sounds {
		if sound.Kind == kind {
			result = append(result, cloneNativeSoundAsset(sound))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (m *Model) NativeSoundByID(id int) (assetsync.NativeSoundAsset, bool) {
	if m == nil {
		return assetsync.NativeSoundAsset{}, false
	}
	for _, sound := range m.NativeAudio.Sounds.Sounds {
		if sound.ID == id {
			return cloneNativeSoundAsset(sound), true
		}
	}
	return assetsync.NativeSoundAsset{}, false
}

// NativeSoundAtStep returns the sound used at step when one exists. This is
// important for CH4, where kick/snare/hat/crash are separate real noise
// configurations in a single channel clip.
func (m *Model) NativeSoundAtStep(sequenceIndex, channel, step int) (assetsync.NativeSoundAsset, bool) {
	clip, ok := m.NativeClipAt(sequenceIndex, channel)
	if !ok {
		return assetsync.NativeSoundAsset{}, false
	}
	if step >= 0 {
		cellStart := step * NativeTicksPerStep
		cellEnd := cellStart + NativeTicksPerStep
		for _, event := range clip.Events {
			if event.Tick < cellEnd && event.Tick+event.DurationTicks > cellStart {
				for _, sound := range m.NativeAudio.Sounds.Sounds {
					if sound.ID == event.SoundID {
						return sound, true
					}
				}
			}
		}
	}
	for _, event := range clip.Events {
		for _, sound := range m.NativeAudio.Sounds.Sounds {
			if sound.ID == event.SoundID {
				return sound, true
			}
		}
	}
	for _, sound := range m.NativeAudio.Sounds.Sounds {
		if sound.Kind == clip.Kind {
			return sound, true
		}
	}
	return assetsync.NativeSoundAsset{}, false
}

func ensureNativeClip(audioAssets *assetsync.NativeAudioAssets, songIndex, sequenceIndex, channel int) (int, error) {
	if channel < 0 || channel > 3 {
		return -1, errors.New("channel must be CH1, CH2, CH3, or CH4")
	}
	if _, err := ensureNativeSongAndPart(audioAssets, songIndex); err != nil {
		return -1, err
	}
	partIndex := nativePartIndexForSequence(&audioAssets.Music, songIndex, sequenceIndex)
	if partIndex < 0 {
		return -1, errors.New("selected song section does not exist")
	}
	lane := nativeLane(&audioAssets.Music.Parts[partIndex].Lanes, channel)
	if *lane != nil {
		return nativeClipIndexByID(&audioAssets.Music, **lane), nil
	}

	usedSounds := map[int]bool{}
	hasChannelSound := false
	for _, sound := range audioAssets.Sounds.Sounds {
		usedSounds[sound.ID] = true
		if sound.Kind == nativeKindName(channel) {
			hasChannelSound = true
		}
	}
	if !hasChannelSound {
		soundID, ok := firstFreeNativeID(usedSounds, audio.NativeMaxSounds)
		if !ok {
			return -1, errors.New("the project has no free sound IDs")
		}
		audioAssets.Sounds.Sounds = append(audioAssets.Sounds.Sounds, defaultNativeSound(soundID, channel))
	}

	usedClips := map[int]bool{}
	for _, clip := range audioAssets.Music.Clips {
		usedClips[clip.ID] = true
	}
	clipID, ok := firstFreeNativeID(usedClips, audio.NativeMaxClips)
	if !ok {
		return -1, errors.New("the project has no free clip IDs")
	}
	clip := assetsync.NativeClipAsset{
		ID: clipID, Name: fmt.Sprintf("CH%d clip", channel+1), Kind: nativeKindName(channel),
		LengthTicks: audioAssets.Music.Parts[partIndex].LengthTicks,
	}
	audioAssets.Music.Clips = append(audioAssets.Music.Clips, clip)
	*lane = new(int)
	**lane = clipID
	return len(audioAssets.Music.Clips) - 1, nil
}

func (m *Model) EnsureNativeClip(sequenceIndex, channel int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		_, err := ensureNativeClip(candidate, m.NativeSongIndex, sequenceIndex, channel)
		return err
	})
}

func (m *Model) SetNativeNote(sequenceIndex, channel, step, note int) error {
	return m.SetNativeNoteDuration(sequenceIndex, channel, step, note, 1)
}

// SetNativeNoteDuration places one grid-quantized monophonic note. New notes
// own their complete time range: a preceding overlap is shortened and events
// starting inside the new note are replaced. This keeps the clip valid while
// making pencil edits predictable.
func (m *Model) SetNativeNoteDuration(sequenceIndex, channel, step, note, durationSteps int) error {
	return m.SetNativeNoteDurationWithSound(sequenceIndex, channel, step, note, durationSteps, -1)
}

// SetNativeNoteDurationWithSound places a note using an explicit reusable
// instrument. soundID -1 preserves the beginner default of the first preset
// for that channel.
func (m *Model) SetNativeNoteDurationWithSound(sequenceIndex, channel, step, note, durationSteps, soundID int) error {
	if step < 0 || note < 0 || note > 127 || durationSteps < 1 {
		return errors.New("note position is outside the 1/64-note piano roll")
	}
	return m.SetNativeNoteTicksWithSound(sequenceIndex, channel, step*NativeTicksPerStep, note, durationSteps*NativeTicksPerStep, soundID)
}

// SetNativeNoteTicksWithSound is the exact 96-PPQN authoring path used by
// triplet, dotted, swing, and live-recorded notes. The step-based API above is
// retained as a compatibility wrapper for existing callers and projects.
func (m *Model) SetNativeNoteTicksWithSound(sequenceIndex, channel, startTick, note, durationTicks, soundID int) error {
	if startTick < 0 || note < 0 || note > 127 || durationTicks < 1 {
		return errors.New("note position is outside the piano roll")
	}
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		clipIndex, err := ensureNativeClip(candidate, m.NativeSongIndex, sequenceIndex, channel)
		if err != nil {
			return err
		}
		clip := &candidate.Music.Clips[clipIndex]
		selectedSoundID := soundID
		if selectedSoundID < 0 {
			for _, sound := range candidate.Sounds.Sounds {
				if sound.Kind == clip.Kind {
					selectedSoundID = sound.ID
					break
				}
			}
		}
		validSound := false
		for _, sound := range candidate.Sounds.Sounds {
			if sound.ID == selectedSoundID && sound.Kind == clip.Kind {
				validSound = true
				break
			}
		}
		if !validSound {
			return errors.New("the selected hardware channel has no sound")
		}
		if startTick >= clip.LengthTicks {
			return errors.New("note starts after the selected clip ends")
		}
		setNativeClipEventTicks(clip, startTick, note, selectedSoundID, durationTicks)
		return nil
	})
}

func setNativeClipEventTicks(clip *assetsync.NativeClipAsset, start, note, soundID, durationTicks int) {
	finish := min(clip.LengthTicks, start+durationTicks)
	setNativeClipEventAsset(clip, assetsync.NativeEventAsset{
		Tick: start, DurationTicks: finish - start, Note: note, SoundID: soundID, Priority: 128,
	})
}

func setNativeClipEventAsset(clip *assetsync.NativeClipAsset, inserted assetsync.NativeEventAsset) {
	start := inserted.Tick
	finish := min(clip.LengthTicks, start+inserted.DurationTicks)
	inserted.DurationTicks = finish - start
	events := make([]assetsync.NativeEventAsset, 0, len(clip.Events)+1)
	for _, event := range clip.Events {
		eventFinish := event.Tick + event.DurationTicks
		switch {
		case event.Tick < start && eventFinish > start:
			event.DurationTicks = start - event.Tick
			events = append(events, event)
		case event.Tick >= start && event.Tick < finish:
			// The new note replaces this occupied range.
		default:
			events = append(events, event)
		}
	}
	clip.Events = append(events, inserted)
	sort.Slice(clip.Events, func(i, j int) bool { return clip.Events[i].Tick < clip.Events[j].Tick })
}

func (m *Model) SetNativeSFXNoteDuration(clipID, step, note, durationSteps, soundID int) error {
	if step < 0 || note < 0 || note > 127 || durationSteps < 1 {
		return errors.New("effect note is outside the 1/64-note piano roll")
	}
	return m.SetNativeSFXNoteTicks(clipID, step*NativeTicksPerStep, note, durationSteps*NativeTicksPerStep, soundID)
}

func (m *Model) SetNativeSFXNoteTicks(clipID, startTick, note, durationTicks, soundID int) error {
	if startTick < 0 || note < 0 || note > 127 || durationTicks < 1 {
		return errors.New("effect note is outside the piano roll")
	}
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		clipIndex := nativeSFXClipIndex(&candidate.Music, clipID)
		if clipIndex < 0 {
			return errors.New("selected effect does not exist")
		}
		clip := &candidate.Music.Clips[clipIndex]
		selectedSoundID := soundID
		if selectedSoundID < 0 {
			for _, sound := range candidate.Sounds.Sounds {
				if sound.Kind == clip.Kind {
					selectedSoundID = sound.ID
					break
				}
			}
		}
		if selectedSoundID < 0 {
			used := map[int]bool{}
			for _, sound := range candidate.Sounds.Sounds {
				used[sound.ID] = true
			}
			id, ok := firstFreeNativeID(used, audio.NativeMaxSounds)
			if !ok {
				return errors.New("the project has no free instrument IDs")
			}
			channel := int(clip.Kind[len(clip.Kind)-1] - '1')
			candidate.Sounds.Sounds = append(candidate.Sounds.Sounds, defaultNativeSound(id, channel))
			selectedSoundID = id
		}
		valid := false
		for _, sound := range candidate.Sounds.Sounds {
			if sound.ID == selectedSoundID && sound.Kind == clip.Kind {
				valid = true
				break
			}
		}
		if !valid {
			return errors.New("instrument does not match the selected effect channel")
		}
		if startTick >= clip.LengthTicks {
			return errors.New("effect note starts after the clip ends")
		}
		setNativeClipEventTicks(clip, startTick, note, selectedSoundID, durationTicks)
		return nil
	})
}

func (m *Model) SetNativeSFXNoiseHitDuration(clipID, step, preset, durationSteps int) error {
	if step < 0 || preset < 0 || preset >= len(nativeNoisePresetNames) || durationSteps < 1 {
		return errors.New("noise preset is outside the drum grid")
	}
	return m.SetNativeSFXNoiseHitTicks(clipID, step*NativeTicksPerStep, preset, durationSteps*NativeTicksPerStep)
}

func (m *Model) SetNativeSFXNoiseHitTicks(clipID, startTick, preset, durationTicks int) error {
	if startTick < 0 || preset < 0 || preset >= len(nativeNoisePresetNames) || durationTicks < 1 {
		return errors.New("noise preset is outside the drum grid")
	}
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		clipIndex := nativeSFXClipIndex(&candidate.Music, clipID)
		if clipIndex < 0 {
			return errors.New("selected effect does not exist")
		}
		clip := &candidate.Music.Clips[clipIndex]
		if clip.Kind != "ch4" {
			return errors.New("noise presets require a CH4 effect")
		}
		if startTick >= clip.LengthTicks {
			return errors.New("noise hit starts after the selected effect ends")
		}
		soundID := -1
		for _, sound := range candidate.Sounds.Sounds {
			if sound.Kind == "ch4" && sound.Name == nativeNoisePresetNames[preset] {
				soundID = sound.ID
				break
			}
		}
		if soundID < 0 {
			used := map[int]bool{}
			for _, sound := range candidate.Sounds.Sounds {
				used[sound.ID] = true
			}
			id, ok := firstFreeNativeID(used, audio.NativeMaxSounds)
			if !ok {
				return errors.New("the project has no free sound IDs")
			}
			candidate.Sounds.Sounds = append(candidate.Sounds.Sounds, defaultNativeNoiseSound(id, preset))
			soundID = id
		}
		setNativeClipEventTicks(clip, startTick, 36+preset, soundID, durationTicks)
		return nil
	})
}

func (m *Model) DeleteNativeSFXNote(clipID, step int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		clipIndex := nativeSFXClipIndex(&candidate.Music, clipID)
		if clipIndex < 0 {
			return nil
		}
		clip := &candidate.Music.Clips[clipIndex]
		cellStart, cellEnd := step*NativeTicksPerStep, (step+1)*NativeTicksPerStep
		events := clip.Events[:0]
		for _, event := range clip.Events {
			if event.Tick >= cellEnd || event.Tick+event.DurationTicks <= cellStart {
				events = append(events, event)
			}
		}
		clip.Events = events
		return nil
	})
}

func (m *Model) ResizeNativeSFXNote(clipID, startStep, durationSteps int) (int, error) {
	duration, err := m.ResizeNativeSFXNoteTicks(clipID, startStep*NativeTicksPerStep, durationSteps*NativeTicksPerStep)
	if err != nil {
		return 0, err
	}
	return max(1, (duration+NativeTicksPerStep-1)/NativeTicksPerStep), err
}

func (m *Model) ResizeNativeSFXNoteTicks(clipID, startTick, durationTicks int) (int, error) {
	accepted := 0
	err := m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		clipIndex := nativeSFXClipIndex(&candidate.Music, clipID)
		if clipIndex < 0 {
			return errors.New("selected effect does not exist")
		}
		clip := &candidate.Music.Clips[clipIndex]
		cellStart, cellEnd := startTick, startTick+NativeTicksPerStep
		start, eventIndex, finishLimit := cellStart, -1, clip.LengthTicks
		for index, event := range clip.Events {
			if event.Tick < cellEnd && event.Tick+event.DurationTicks > cellStart {
				eventIndex, start = index, event.Tick
				continue
			}
			if event.Tick > start && event.Tick < finishLimit {
				finishLimit = event.Tick
			}
		}
		if eventIndex < 0 {
			return errors.New("selected effect note does not exist")
		}
		finish := min(finishLimit, start+durationTicks)
		clip.Events[eventIndex].DurationTicks = finish - start
		accepted = finish - start
		return nil
	})
	return accepted, err
}

func (m *Model) SetNativeSFXNoteLegato(clipID, step int, legato bool) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		clipIndex := nativeSFXClipIndex(&candidate.Music, clipID)
		if clipIndex < 0 {
			return errors.New("selected effect does not exist")
		}
		clip := &candidate.Music.Clips[clipIndex]
		if clip.Kind == "ch4" {
			return errors.New("CH4 noise hits always retrigger")
		}
		return setNativeEventLegato(clip, step, legato)
	})
}

func (m *Model) RenameNativeSFX(clipID int, name string) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		index := nativeSFXClipIndex(&candidate.Music, clipID)
		if index < 0 {
			return errors.New("selected effect does not exist")
		}
		candidate.Music.Clips[index].Name = name
		return nil
	})
}

func setNativeEventLegato(clip *assetsync.NativeClipAsset, step int, legato bool) error {
	cellStart, cellEnd := step*NativeTicksPerStep, (step+1)*NativeTicksPerStep
	for index := range clip.Events {
		event := &clip.Events[index]
		if event.Tick < cellEnd && event.Tick+event.DurationTicks > cellStart {
			event.Legato = legato
			return nil
		}
	}
	return errors.New("selected note does not exist")
}

func (m *Model) AssignNativeSFXNoteSound(clipID, step, soundID int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		clipIndex := nativeSFXClipIndex(&candidate.Music, clipID)
		if clipIndex < 0 {
			return errors.New("selected effect does not exist")
		}
		clip := &candidate.Music.Clips[clipIndex]
		valid := false
		for _, sound := range candidate.Sounds.Sounds {
			if sound.ID == soundID && sound.Kind == clip.Kind {
				valid = true
				break
			}
		}
		if !valid {
			return errors.New("instrument does not match the selected effect channel")
		}
		cellStart, cellEnd := step*NativeTicksPerStep, (step+1)*NativeTicksPerStep
		for index := range clip.Events {
			if clip.Events[index].Tick < cellEnd && clip.Events[index].Tick+clip.Events[index].DurationTicks > cellStart {
				clip.Events[index].SoundID = soundID
				return nil
			}
		}
		return errors.New("selected cell has no effect note")
	})
}

func (m *Model) UpdateNativeSFXSoundAtStep(clipID, step int, edit func(*assetsync.NativeSoundAsset)) error {
	if edit == nil {
		return errors.New("sound edit is required")
	}
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		clipIndex := nativeSFXClipIndex(&candidate.Music, clipID)
		if clipIndex < 0 {
			return errors.New("selected effect does not exist")
		}
		clip := &candidate.Music.Clips[clipIndex]
		cellStart, cellEnd := step*NativeTicksPerStep, (step+1)*NativeTicksPerStep
		eventIndex := -1
		for index, event := range clip.Events {
			if event.Tick < cellEnd && event.Tick+event.DurationTicks > cellStart {
				eventIndex = index
				break
			}
		}
		if eventIndex < 0 {
			return errors.New("selected effect cell has no sound")
		}
		soundID := clip.Events[eventIndex].SoundID
		for index := range candidate.Sounds.Sounds {
			if candidate.Sounds.Sounds[index].ID != soundID {
				continue
			}
			if nativeSoundReferenceCount(candidate.Music.Clips, soundID) > 1 {
				used := map[int]bool{}
				for _, sound := range candidate.Sounds.Sounds {
					used[sound.ID] = true
				}
				id, ok := firstFreeNativeID(used, audio.NativeMaxSounds)
				if !ok {
					return errors.New("the project has no free instrument ID for this variation")
				}
				clone := cloneNativeSoundAsset(candidate.Sounds.Sounds[index])
				clone.ID, clone.Name = id, clone.Name+" variation"
				edit(&clone)
				candidate.Sounds.Sounds = append(candidate.Sounds.Sounds, clone)
				clip.Events[eventIndex].SoundID = id
				return nil
			}
			edit(&candidate.Sounds.Sounds[index])
			return nil
		}
		return errors.New("selected effect sound does not exist")
	})
}

// ResizeNativeNote changes the duration of the note starting at startStep.
// A note may grow to the end of the clip or the next note, whichever comes
// first. The returned value is the accepted grid duration after clamping.
func (m *Model) ResizeNativeNote(sequenceIndex, channel, startStep, durationSteps int) (int, error) {
	if startStep < 0 || durationSteps < 1 {
		return 0, errors.New("note duration is outside the 1/64-note piano roll")
	}
	duration, err := m.ResizeNativeNoteTicks(sequenceIndex, channel, startStep*NativeTicksPerStep, durationSteps*NativeTicksPerStep)
	if err != nil {
		return 0, err
	}
	return max(1, (duration+NativeTicksPerStep-1)/NativeTicksPerStep), err
}

// ResizeNativeNoteTicks preserves non-grid-aligned starts from MIDI and tuplets
// while changing their exact scheduler duration.
func (m *Model) ResizeNativeNoteTicks(sequenceIndex, channel, startTick, durationTicks int) (int, error) {
	if startTick < 0 || durationTicks < 1 {
		return 0, errors.New("note duration is outside the piano roll")
	}
	accepted := 0
	err := m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return errors.New("selected song section does not exist")
		}
		lane := nativeLane(&candidate.Music.Parts[partIndex].Lanes, channel)
		if lane == nil || *lane == nil {
			return errors.New("selected channel is empty")
		}
		clipIndex := nativeClipIndexByID(&candidate.Music, **lane)
		if clipIndex < 0 {
			return errors.New("selected clip does not exist")
		}
		clip := &candidate.Music.Clips[clipIndex]
		cellStart := startTick
		cellEnd := cellStart + NativeTicksPerStep
		start := cellStart
		eventIndex := -1
		finishLimit := clip.LengthTicks
		for index, event := range clip.Events {
			if event.Tick < cellEnd && event.Tick+event.DurationTicks > cellStart {
				eventIndex = index
				start = event.Tick
				continue
			}
			if event.Tick > start && event.Tick < finishLimit {
				finishLimit = event.Tick
			}
		}
		if eventIndex < 0 {
			return errors.New("selected note does not exist")
		}
		finish := min(finishLimit, start+durationTicks)
		if finish <= start {
			return errors.New("selected note has no available duration")
		}
		clip.Events[eventIndex].DurationTicks = finish - start
		accepted = finish - start
		return nil
	})
	return accepted, err
}

// SetNativeNoteLegato chooses whether a pitched note retriggers its channel or
// ties into an already sounding note that uses the same instrument. CH4 has no
// frequency-only update path, so noise hits always retrigger.
func (m *Model) SetNativeNoteLegato(sequenceIndex, channel, step int, legato bool) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if channel < 0 || channel > 2 {
			return errors.New("CH4 noise hits always retrigger")
		}
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return errors.New("selected song section does not exist")
		}
		lane := nativeLane(&candidate.Music.Parts[partIndex].Lanes, channel)
		if lane == nil || *lane == nil {
			return errors.New("selected channel is empty")
		}
		clipIndex := nativeClipIndexByID(&candidate.Music, **lane)
		if clipIndex < 0 {
			return errors.New("selected clip does not exist")
		}
		return setNativeEventLegato(&candidate.Music.Clips[clipIndex], step, legato)
	})
}

// SetNativeNoiseHit places one of the eight beginner drum presets on CH4.
// Each preset materializes into an ordinary hardware noise sound, so saved
// projects contain only exact CH4 register fields rather than magic drum IDs.
func (m *Model) SetNativeNoiseHit(sequenceIndex, step, preset int) error {
	return m.SetNativeNoiseHitDuration(sequenceIndex, step, preset, 1)
}

func (m *Model) SetNativeNoiseHitDuration(sequenceIndex, step, preset, durationSteps int) error {
	if step < 0 || preset < 0 || preset >= len(nativeNoisePresetNames) || durationSteps < 1 {
		return errors.New("noise hit is outside the 1/64-note drum grid")
	}
	return m.SetNativeNoiseHitTicks(sequenceIndex, step*NativeTicksPerStep, preset, durationSteps*NativeTicksPerStep)
}

func (m *Model) SetNativeNoiseHitTicks(sequenceIndex, startTick, preset, durationTicks int) error {
	if startTick < 0 || preset < 0 || preset >= len(nativeNoisePresetNames) || durationTicks < 1 {
		return errors.New("noise hit is outside the drum grid")
	}
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		clipIndex, err := ensureNativeClip(candidate, m.NativeSongIndex, sequenceIndex, 3)
		if err != nil {
			return err
		}
		soundID := -1
		for _, sound := range candidate.Sounds.Sounds {
			if sound.Kind == "ch4" && sound.Name == nativeNoisePresetNames[preset] {
				soundID = sound.ID
				break
			}
		}
		if soundID < 0 {
			used := map[int]bool{}
			for _, sound := range candidate.Sounds.Sounds {
				used[sound.ID] = true
			}
			id, ok := firstFreeNativeID(used, audio.NativeMaxSounds)
			if !ok {
				return errors.New("the project has no free sound IDs")
			}
			candidate.Sounds.Sounds = append(candidate.Sounds.Sounds, defaultNativeNoiseSound(id, preset))
			soundID = id
		}
		clip := &candidate.Music.Clips[clipIndex]
		if startTick >= clip.LengthTicks {
			return errors.New("noise hit starts after the selected clip ends")
		}
		setNativeClipEventTicks(clip, startTick, 36+preset, soundID, durationTicks)
		return nil
	})
}

func (m *Model) DeleteNativeNote(sequenceIndex, channel, step int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return nil
		}
		lane := nativeLane(&candidate.Music.Parts[partIndex].Lanes, channel)
		if lane == nil || *lane == nil {
			return nil
		}
		clipIndex := nativeClipIndexByID(&candidate.Music, **lane)
		if clipIndex < 0 {
			return nil
		}
		cellStart := step * NativeTicksPerStep
		cellEnd := cellStart + NativeTicksPerStep
		events := candidate.Music.Clips[clipIndex].Events[:0]
		for _, event := range candidate.Music.Clips[clipIndex].Events {
			if event.Tick >= cellEnd || event.Tick+event.DurationTicks <= cellStart {
				events = append(events, event)
			}
		}
		candidate.Music.Clips[clipIndex].Events = events
		return nil
	})
}

func (m *Model) RenameNativeClip(sequenceIndex, channel int, name string) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return errors.New("selected song section does not exist")
		}
		lane := nativeLane(&candidate.Music.Parts[partIndex].Lanes, channel)
		if lane == nil || *lane == nil {
			return errors.New("selected channel is empty")
		}
		clipIndex := nativeClipIndexByID(&candidate.Music, **lane)
		if clipIndex < 0 {
			return errors.New("selected clip does not exist")
		}
		candidate.Music.Clips[clipIndex].Name = name
		return nil
	})
}

func (m *Model) RenameNativePart(sequenceIndex int, name string) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if _, err := ensureNativeSongAndPart(candidate, m.NativeSongIndex); err != nil {
			return err
		}
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return errors.New("selected song section does not exist")
		}
		candidate.Music.Parts[partIndex].Name = name
		return nil
	})
}

func (m *Model) MoveNativePart(sequenceIndex, delta int) (int, error) {
	moved := sequenceIndex
	err := m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if m.NativeSongIndex < 0 || m.NativeSongIndex >= len(candidate.Music.Songs) || sequenceIndex < 0 || sequenceIndex >= len(candidate.Music.Songs[m.NativeSongIndex].Sequence) {
			return errors.New("selected song section does not exist")
		}
		song := &candidate.Music.Songs[m.NativeSongIndex]
		destination := min(max(sequenceIndex+delta, 0), len(song.Sequence)-1)
		song.Sequence[sequenceIndex], song.Sequence[destination] = song.Sequence[destination], song.Sequence[sequenceIndex]
		moved = destination
		return nil
	})
	return moved, err
}

func (m *Model) DeleteNativePart(sequenceIndex int) (int, error) {
	selected := sequenceIndex
	err := m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if m.NativeSongIndex < 0 || m.NativeSongIndex >= len(candidate.Music.Songs) || sequenceIndex < 0 || sequenceIndex >= len(candidate.Music.Songs[m.NativeSongIndex].Sequence) {
			return errors.New("selected song section does not exist")
		}
		song := &candidate.Music.Songs[m.NativeSongIndex]
		if len(song.Sequence) == 1 {
			return errors.New("a song must keep one section; clear its channel cells instead")
		}
		removedID := song.Sequence[sequenceIndex]
		song.Sequence = append(song.Sequence[:sequenceIndex], song.Sequence[sequenceIndex+1:]...)
		selected = min(sequenceIndex, len(song.Sequence)-1)
		if song.LoopStart != nil {
			switch {
			case sequenceIndex < *song.LoopStart:
				(*song.LoopStart)--
			case sequenceIndex == *song.LoopStart:
				*song.LoopStart = min(sequenceIndex, len(song.Sequence)-1)
			}
		}

		stillUsed := false
		for _, otherSong := range candidate.Music.Songs {
			for _, partID := range otherSong.Sequence {
				if partID == removedID {
					stillUsed = true
					break
				}
			}
		}
		if !stillUsed {
			parts := candidate.Music.Parts[:0]
			for _, part := range candidate.Music.Parts {
				if part.ID != removedID {
					parts = append(parts, part)
				}
			}
			candidate.Music.Parts = parts
		}
		cleanupNativeMusicOrphans(&candidate.Music)
		return nil
	})
	return selected, err
}

func (m *Model) ClearNativeLane(sequenceIndex, channel int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return nil
		}
		lane := nativeLane(&candidate.Music.Parts[partIndex].Lanes, channel)
		if lane != nil {
			*lane = nil
		}
		cleanupNativeMusicOrphans(&candidate.Music)
		return nil
	})
}

func (m *Model) AssignNativeClip(sequenceIndex, channel, clipID int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if _, err := ensureNativeSongAndPart(candidate, m.NativeSongIndex); err != nil {
			return err
		}
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		clipIndex := nativeClipIndexByID(&candidate.Music, clipID)
		if partIndex < 0 || clipIndex < 0 {
			return errors.New("selected section or clip does not exist")
		}
		if candidate.Music.Clips[clipIndex].SFX || candidate.Music.Clips[clipIndex].Kind != nativeKindName(channel) || candidate.Music.Clips[clipIndex].LengthTicks > candidate.Music.Parts[partIndex].LengthTicks {
			return fmt.Errorf("clip %d cannot play on CH%d", clipID, channel+1)
		}
		lane := nativeLane(&candidate.Music.Parts[partIndex].Lanes, channel)
		*lane = new(int)
		**lane = clipID
		return nil
	})
}

// DuplicateNativeClipTo pastes an independent copy of sourceClipID into one
// hardware lane. Sharing the source clip would make a later edit in the paste
// destination silently rewrite the original section too, which is never what
// a user expects from Copy/Paste in the song arranger.
func (m *Model) DuplicateNativeClipTo(sequenceIndex, channel, sourceClipID int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if _, err := ensureNativeSongAndPart(candidate, m.NativeSongIndex); err != nil {
			return err
		}
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		sourceIndex := nativeClipIndexByID(&candidate.Music, sourceClipID)
		if partIndex < 0 || sourceIndex < 0 {
			return errors.New("selected section or copied clip does not exist")
		}
		source := candidate.Music.Clips[sourceIndex]
		if source.Kind != nativeKindName(channel) || source.LengthTicks > candidate.Music.Parts[partIndex].LengthTicks {
			return fmt.Errorf("clip %d cannot play on CH%d", sourceClipID, channel+1)
		}
		used := make(map[int]bool, len(candidate.Music.Clips))
		for _, clip := range candidate.Music.Clips {
			used[clip.ID] = true
		}
		newID, ok := firstFreeNativeID(used, audio.NativeMaxClips)
		if !ok {
			return errors.New("the project has no free clip IDs")
		}
		copy := source
		copy.ID = newID
		copy.Events = append([]assetsync.NativeEventAsset(nil), source.Events...)
		candidate.Music.Clips = append(candidate.Music.Clips, copy)
		lane := nativeLane(&candidate.Music.Parts[partIndex].Lanes, channel)
		*lane = new(int)
		**lane = newID
		return nil
	})
}

// CreateNativeSound creates a reusable instrument for exactly one hardware
// channel. sourceID duplicates an existing preset; -1 starts from the channel's
// musical default. It never rewrites existing notes.
func (m *Model) CreateNativeSound(channel, sourceID int, name string) (int, error) {
	created := -1
	err := m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if channel < 0 || channel > 3 {
			return errors.New("instrument channel must be CH1, CH2, CH3, or CH4")
		}
		used := map[int]bool{}
		for _, sound := range candidate.Sounds.Sounds {
			used[sound.ID] = true
		}
		id, ok := firstFreeNativeID(used, audio.NativeMaxSounds)
		if !ok {
			return errors.New("the project has no free instrument IDs")
		}
		sound := defaultNativeSound(id, channel)
		if sourceID >= 0 {
			found := false
			for _, source := range candidate.Sounds.Sounds {
				if source.ID == sourceID && source.Kind == nativeKindName(channel) {
					sound = cloneNativeSoundAsset(source)
					sound.ID = id
					found = true
					break
				}
			}
			if !found {
				return errors.New("source instrument does not exist on this channel")
			}
		}
		name = strings.TrimSpace(name)
		if name == "" {
			name = sound.Name + " copy"
		}
		sound.Name = name
		candidate.Sounds.Sounds = append(candidate.Sounds.Sounds, sound)
		created = id
		return nil
	})
	return created, err
}

func (m *Model) RenameNativeSound(soundID int, name string) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		for index := range candidate.Sounds.Sounds {
			if candidate.Sounds.Sounds[index].ID == soundID {
				candidate.Sounds.Sounds[index].Name = name
				return nil
			}
		}
		return errors.New("selected instrument does not exist")
	})
}

func (m *Model) AssignNativeNoteSound(sequenceIndex, channel, step, soundID int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return errors.New("selected song section does not exist")
		}
		lane := nativeLane(&candidate.Music.Parts[partIndex].Lanes, channel)
		if lane == nil || *lane == nil {
			return errors.New("selected channel is empty")
		}
		clipIndex := nativeClipIndexByID(&candidate.Music, **lane)
		if clipIndex < 0 {
			return errors.New("selected clip does not exist")
		}
		clip := &candidate.Music.Clips[clipIndex]
		valid := false
		for _, sound := range candidate.Sounds.Sounds {
			if sound.ID == soundID && sound.Kind == clip.Kind {
				valid = true
				break
			}
		}
		if !valid {
			return errors.New("instrument does not match the selected hardware channel")
		}
		cellStart := step * NativeTicksPerStep
		cellEnd := cellStart + NativeTicksPerStep
		for index := range clip.Events {
			event := &clip.Events[index]
			if event.Tick < cellEnd && event.Tick+event.DurationTicks > cellStart {
				event.SoundID = soundID
				return nil
			}
		}
		return errors.New("selected cell has no note")
	})
}

func (m *Model) UpdateNativeSound(sequenceIndex, channel int, edit func(*assetsync.NativeSoundAsset)) error {
	return m.UpdateNativeSoundAtStep(sequenceIndex, channel, -1, edit)
}

func (m *Model) UpdateNativeSoundByID(soundID int, edit func(*assetsync.NativeSoundAsset)) error {
	if edit == nil {
		return errors.New("sound edit is required")
	}
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		for index := range candidate.Sounds.Sounds {
			if candidate.Sounds.Sounds[index].ID == soundID {
				edit(&candidate.Sounds.Sounds[index])
				return nil
			}
		}
		return errors.New("selected instrument does not exist")
	})
}

func (m *Model) UpdateNativeSoundAtStep(sequenceIndex, channel, step int, edit func(*assetsync.NativeSoundAsset)) error {
	if edit == nil {
		return errors.New("sound edit is required")
	}
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		clipIndex, err := ensureNativeClip(candidate, m.NativeSongIndex, sequenceIndex, channel)
		if err != nil {
			return err
		}
		clip := &candidate.Music.Clips[clipIndex]
		soundID := -1
		eventIndex := -1
		if step >= 0 {
			cellStart := step * NativeTicksPerStep
			cellEnd := cellStart + NativeTicksPerStep
			for index, event := range clip.Events {
				if event.Tick < cellEnd && event.Tick+event.DurationTicks > cellStart {
					soundID = event.SoundID
					eventIndex = index
					break
				}
			}
		}
		if soundID < 0 && len(clip.Events) != 0 {
			soundID = clip.Events[0].SoundID
		}
		for index := range candidate.Sounds.Sounds {
			if candidate.Sounds.Sounds[index].Kind == clip.Kind && (soundID < 0 || candidate.Sounds.Sounds[index].ID == soundID) {
				if eventIndex >= 0 && nativeSoundReferenceCount(candidate.Music.Clips, candidate.Sounds.Sounds[index].ID) > 1 {
					used := map[int]bool{}
					for _, sound := range candidate.Sounds.Sounds {
						used[sound.ID] = true
					}
					id, ok := firstFreeNativeID(used, audio.NativeMaxSounds)
					if !ok {
						return errors.New("the project has no free instrument IDs for a unique edit")
					}
					clone := cloneNativeSoundAsset(candidate.Sounds.Sounds[index])
					clone.ID, clone.Name = id, clone.Name+" variation"
					edit(&clone)
					candidate.Sounds.Sounds = append(candidate.Sounds.Sounds, clone)
					clip.Events[eventIndex].SoundID = id
					return nil
				}
				edit(&candidate.Sounds.Sounds[index])
				return nil
			}
		}
		return errors.New("selected channel has no editable sound")
	})
}

func nativeSoundReferenceCount(clips []assetsync.NativeClipAsset, soundID int) int {
	count := 0
	for _, clip := range clips {
		for _, event := range clip.Events {
			if event.SoundID == soundID {
				count++
			}
		}
	}
	return count
}

func (m *Model) SetNativeSongName(name string) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if _, err := ensureNativeSongAndPart(candidate, m.NativeSongIndex); err != nil {
			return err
		}
		candidate.Music.Songs[m.NativeSongIndex].Name = name
		return nil
	})
}

func (m *Model) SetNativeTempo(tempo int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if _, err := ensureNativeSongAndPart(candidate, m.NativeSongIndex); err != nil {
			return err
		}
		candidate.Music.Songs[m.NativeSongIndex].Tempo = tempo
		return nil
	})
}

// SetNativeOutput updates the three project-wide Game Boy PSG terminal
// controls. They map directly to SOUNDCNT_L left/right volume and SOUNDCNT_H
// PSG ratio; no software gain or post-processing stage is introduced.
func (m *Model) SetNativeOutput(leftLevel, rightLevel, psgRatio int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		candidate.Settings.LeftLevel = leftLevel
		candidate.Settings.RightLevel = rightLevel
		candidate.Settings.PSGRatio = psgRatio
		return nil
	})
}

func (m *Model) AddNativePart(duplicateSequence int) (int, error) {
	added := -1
	err := m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if _, err := ensureNativeSongAndPart(candidate, m.NativeSongIndex); err != nil {
			return err
		}
		used := map[int]bool{}
		for _, part := range candidate.Music.Parts {
			used[part.ID] = true
		}
		id, ok := firstFreeNativeID(used, audio.NativeMaxParts)
		if !ok {
			return errors.New("the song has no free section IDs")
		}
		song := &candidate.Music.Songs[m.NativeSongIndex]
		part := assetsync.NativePartAsset{ID: id, Name: fmt.Sprintf("Part %c", 'A'+len(song.Sequence)), LengthTicks: NativePartTicks, Bars: 1, BeatsPerBar: 4, BeatUnit: 4}
		if sourceIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, duplicateSequence); sourceIndex >= 0 {
			source := candidate.Music.Parts[sourceIndex]
			part.Bars, part.BeatsPerBar, part.BeatUnit, part.LengthTicks = source.Bars, source.BeatsPerBar, source.BeatUnit, source.LengthTicks
			usedClips := map[int]bool{}
			for _, clip := range candidate.Music.Clips {
				usedClips[clip.ID] = true
			}
			for channel := 0; channel < 4; channel++ {
				sourceLane := nativeLane(&source.Lanes, channel)
				if sourceLane == nil || *sourceLane == nil {
					continue
				}
				sourceClipIndex := nativeClipIndexByID(&candidate.Music, **sourceLane)
				newClipID, free := firstFreeNativeID(usedClips, audio.NativeMaxClips)
				if sourceClipIndex < 0 || !free {
					return errors.New("the project has no free clip IDs for a new section")
				}
				usedClips[newClipID] = true
				clip := candidate.Music.Clips[sourceClipIndex]
				clip.ID = newClipID
				clip.Events = append([]assetsync.NativeEventAsset(nil), clip.Events...)
				candidate.Music.Clips = append(candidate.Music.Clips, clip)
				destinationLane := nativeLane(&part.Lanes, channel)
				*destinationLane = new(int)
				**destinationLane = newClipID
			}
		}
		candidate.Music.Parts = append(candidate.Music.Parts, part)
		song.Sequence = append(song.Sequence, id)
		added = len(song.Sequence) - 1
		return nil
	})
	return added, err
}

// SetNativePartMeter resizes the selected reusable part and all four of its
// clips to an exact number of bars. Events beyond a shorter boundary are
// removed or clamped; extending preserves every authored event.
func (m *Model) SetNativePartMeter(sequenceIndex, bars, beatsPerBar, beatUnit int) error {
	validUnit := beatUnit == 1 || beatUnit == 2 || beatUnit == 4 || beatUnit == 8 || beatUnit == 16 || beatUnit == 32
	if bars < 1 || bars > NativeMaxPartBars || beatsPerBar < 1 || beatsPerBar > 16 || !validUnit {
		return fmt.Errorf("part meter is outside 1..%d bars and supported note units", NativeMaxPartBars)
	}
	ticksPerBar := audio.NativePPQN * 4 * beatsPerBar / beatUnit
	if ticksPerBar < NativeTicksPerStep || ticksPerBar%NativeTicksPerStep != 0 {
		return errors.New("part meter does not align to the 1/64 editor grid")
	}
	length := bars * ticksPerBar
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if _, err := ensureNativeSongAndPart(candidate, m.NativeSongIndex); err != nil {
			return err
		}
		partIndex := nativePartIndexForSequence(&candidate.Music, m.NativeSongIndex, sequenceIndex)
		if partIndex < 0 {
			return errors.New("selected song section does not exist")
		}
		part := &candidate.Music.Parts[partIndex]
		part.Bars, part.BeatsPerBar, part.BeatUnit, part.LengthTicks = bars, beatsPerBar, beatUnit, length
		for channel := 0; channel < 4; channel++ {
			lane := nativeLane(&part.Lanes, channel)
			if lane == nil || *lane == nil {
				continue
			}
			clipIndex := nativeClipIndexByID(&candidate.Music, **lane)
			if clipIndex < 0 {
				return errors.New("selected part references a missing clip")
			}
			clip := &candidate.Music.Clips[clipIndex]
			clip.LengthTicks = length
			kept := clip.Events[:0]
			for _, event := range clip.Events {
				if event.Tick >= length {
					continue
				}
				event.DurationTicks = min(event.DurationTicks, length-event.Tick)
				if event.DurationTicks > 0 {
					kept = append(kept, event)
				}
			}
			clip.Events = kept
		}
		return nil
	})
}

// ReuseNativePart inserts another reference to an existing part. Editing the
// part later intentionally updates every occurrence; Duplicate remains the
// explicit way to create an independent copy.
func (m *Model) ReuseNativePart(sourceSequence, afterSequence int) (int, error) {
	inserted := -1
	err := m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if _, err := ensureNativeSongAndPart(candidate, m.NativeSongIndex); err != nil {
			return err
		}
		song := &candidate.Music.Songs[m.NativeSongIndex]
		if sourceSequence < 0 || sourceSequence >= len(song.Sequence) {
			return errors.New("source section does not exist")
		}
		if len(song.Sequence) >= audio.NativeMaxSequenceParts {
			return errors.New("song order is full")
		}
		partID := song.Sequence[sourceSequence]
		inserted = min(max(afterSequence+1, 0), len(song.Sequence))
		song.Sequence = append(song.Sequence, 0)
		copy(song.Sequence[inserted+1:], song.Sequence[inserted:])
		song.Sequence[inserted] = partID
		if song.LoopStart != nil && inserted <= *song.LoopStart {
			(*song.LoopStart)++
		}
		return nil
	})
	return inserted, err
}

func (m *Model) SetNativeSongLoop(loop bool) error {
	if loop {
		return m.SetNativeSongLoopStart(0)
	}
	return m.SetNativeSongLoopStart(-1)
}

// SetNativeSongLoopStart sets the sequence entry that follows the song end.
// -1 means play once and end.
func (m *Model) SetNativeSongLoopStart(sequenceIndex int) error {
	return m.updateNativeAudio(func(candidate *assetsync.NativeAudioAssets) error {
		if _, err := ensureNativeSongAndPart(candidate, m.NativeSongIndex); err != nil {
			return err
		}
		if sequenceIndex >= 0 {
			if sequenceIndex >= len(candidate.Music.Songs[m.NativeSongIndex].Sequence) {
				return errors.New("loop start is outside the song order")
			}
			start := sequenceIndex
			candidate.Music.Songs[m.NativeSongIndex].LoopStart = &start
		} else {
			candidate.Music.Songs[m.NativeSongIndex].LoopStart = nil
		}
		return nil
	})
}
