package main

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/performance"
	runtimeaudio "github.com/drpaneas/mtile/runtime/audio"
	"github.com/hajimehoshi/ebiten/v2"
)

func (g *Game) updateNativeCreateMouse(layout nativeMusicLayout, mx, my int) {
	if rectContains(layout.WorkflowMusic, mx, my) {
		g.setNativeSFXMode(false)
		return
	}
	if rectContains(layout.WorkflowSFX, mx, my) {
		g.setNativeSFXMode(true)
		return
	}
	for channel := 0; channel < 4; channel++ {
		if rectContains(nativeChannelMuteRect(layout, channel), mx, my) {
			g.toggleNativeChannelMute(channel)
			return
		}
		if rectContains(nativeChannelSoloRect(layout, channel), mx, my) {
			g.toggleNativeChannelSolo(channel)
			return
		}
		if rectContains(nativeChannelRect(layout, channel), mx, my) {
			if g.nativeSFXMode && channel != g.nativeChannel {
				g.createNativeSFX(channel)
				return
			}
			g.nativeChannel = channel
			g.nativeRangeActive = false
			g.nativeFocus = nativeFocusComposer
			if channel == 3 {
				g.setNativeVoiceMode(performance.ModeNoise)
			} else {
				g.setNativeVoiceMode(performance.ModeMono)
			}
			g.statusMsg = fmt.Sprintf("CH%d selected", channel+1)
			return
		}
	}
	if !g.nativeSFXMode {
		for index, mode := range [...]performance.VoiceMode{performance.ModeMono, performance.ModePoly2, performance.ModePoly3, performance.ModeNoise} {
			if rectContains(nativeVoiceModeRect(layout, index), mx, my) {
				g.setNativeVoiceMode(mode)
				return
			}
		}
		for index := 0; index < 5; index++ {
			if !rectContains(nativeCaptureOptionRect(layout, index), mx, my) {
				continue
			}
			switch index {
			case 0:
				g.nativeQuantize = performance.Quantize((int(g.nativeQuantize) + 1) % int(performance.QuantizeSixtyFourth+1))
				g.statusMsg = "Quantize " + g.nativeQuantize.String()
			case 1:
				g.nativeOverdub = !g.nativeOverdub
			case 2:
				g.nativeMetronome = !g.nativeMetronome
				if g.nativeConductor != nil {
					g.nativeConductor.SetMetronome(g.nativeMetronome)
				}
			case 3:
				g.nativeCountIn = (g.nativeCountIn + 1) % 3
			case 4:
				g.nativeSwing += 4
				if g.nativeSwing > 66 {
					g.nativeSwing = 50
				}
				if g.nativeSwing == 50 {
					g.statusMsg = "Swing straight"
				} else {
					g.statusMsg = fmt.Sprintf("Swing %d%%", g.nativeSwing)
				}
			}
			return
		}
	}
	if rectContains(layout.ClipName, mx, my) {
		if clip, ok := g.nativeCurrentClip(); ok {
			g.nativeNameTarget, g.nativeNameDraft = nativeEditClipName, clip.Name
		}
		return
	}
	if rectContains(layout.BarPrevious, mx, my) {
		g.nudgeNativeBar(-1)
		return
	}
	if rectContains(layout.BarValue, mx, my) || rectContains(layout.BarNext, mx, my) {
		g.nudgeNativeBar(1)
		return
	}
	if rectContains(layout.GridSnap, mx, my) {
		g.nativeGridQuantize = nextNativeGridSnap(g.nativeGridQuantize)
		g.statusMsg = "Piano-roll snap " + g.nativeGridQuantize.String()
		return
	}
	if rectContains(layout.InstrumentPrevious, mx, my) {
		g.cycleNativeInstrument(-1)
		return
	}
	if rectContains(layout.InstrumentNext, mx, my) {
		g.cycleNativeInstrument(1)
		return
	}
	if rectContains(layout.InstrumentName, mx, my) {
		if sound, ok := g.model.NativeSoundByID(g.nativeInstrument[g.nativeChannel]); ok {
			g.nativeNameTarget, g.nativeNameDraft = nativeEditSoundName, sound.Name
		}
		return
	}
	if rectContains(layout.InstrumentDuplicate, mx, my) {
		g.duplicateNativeInstrument()
		return
	}
	if rectContains(layout.InstrumentPreset, mx, my) {
		presets := editor.NativeSoundPresets(g.nativeChannel)
		g.nativePresetMenuOpen = len(presets) != 0
		g.nativePresetMenuCursor = 0
		if sound, ok := g.model.NativeSoundByID(g.nativeInstrument[g.nativeChannel]); ok {
			g.nativePresetMenuCursor = max(0, editor.NativeSoundPresetIndex(g.nativeChannel, sound))
		}
		visible := nativePresetMenuVisibleCount(layout, len(presets))
		g.nativePresetMenuScroll = min(max(0, len(presets)-visible), max(0, g.nativePresetMenuCursor-visible/2))
		return
	}
	if rectContains(layout.Advanced, mx, my) {
		g.nativeAdvanced = !g.nativeAdvanced
		g.statusMsg = "Sound controls: " + map[bool]string{false: "simple", true: "advanced"}[g.nativeAdvanced]
		return
	}
	if g.nativeAdvanced && rectContains(layout.OutputLeft, mx, my) {
		g.nudgeNativeOutput(0, nativePointerDelta(layout.OutputLeft, mx))
		return
	}
	if g.nativeAdvanced && rectContains(layout.OutputRight, mx, my) {
		g.nudgeNativeOutput(1, nativePointerDelta(layout.OutputRight, mx))
		return
	}
	if g.nativeAdvanced && rectContains(layout.OutputMix, mx, my) {
		g.nudgeNativeOutput(2, nativePointerDelta(layout.OutputMix, mx))
		return
	}
	if rectContains(layout.NoteLengthDown, mx, my) {
		g.nudgeNativeNoteLength(-1)
		return
	}
	if rectContains(layout.NoteLengthValue, mx, my) || rectContains(layout.NoteLengthUp, mx, my) {
		g.nudgeNativeNoteLength(1)
		return
	}
	if rectContains(layout.Articulation, mx, my) && g.nativeChannel < 3 {
		event, start, ok := g.nativeEventAtStep(g.nativeSelectedStep)
		if !ok {
			g.statusMsg = "Select a note before changing its attack"
			return
		}
		if err := g.nativeSetNoteLegato(start, !event.Legato); err != nil {
			g.statusMsg = err.Error()
			return
		}
		label := "Retrigger"
		if !event.Legato {
			label = "Tie / legato"
		}
		g.markNativeMusicEdit(label + " selected")
		return
	}
	if rectContains(layout.OctaveDown, mx, my) {
		g.nudgeNativeOctave(layout, -1)
		return
	}
	if rectContains(layout.OctaveUp, mx, my) {
		g.nudgeNativeOctave(layout, 1)
		return
	}
	if rectContains(layout.Grid, mx, my) {
		g.nativeFocus = nativeFocusComposer
		g.beginNativeNoteDrag(layout, mx, my)
		return
	}
	for displayIndex, controlIndex := range g.nativeInspectorControlIndices() {
		if rectContains(nativeInspectorControlRect(layout, g.nativeChannel, displayIndex), mx, my) {
			delta := 1
			control := nativeInspectorControlRect(layout, g.nativeChannel, displayIndex)
			if mx < control.X+control.W/2 {
				delta = -1
			}
			g.nudgeNativeSoundControl(controlIndex, delta)
			return
		}
	}
	g.updateNativeWaveDraw(layout)
}

func (g *Game) nativeGridPosition(layout nativeMusicLayout, mx, my int) (step, row int) {
	_, _, _, barSteps, partSteps, viewStart := g.nativePartView()
	local := min(barSteps-1, max(0, (mx-layout.Grid.X)*barSteps/layout.Grid.W))
	step = min(partSteps-1, viewStart+local)
	rows := nativePianoRows(layout.Grid, g.nativeChannel)
	row = min(rows-1, max(0, (my-layout.Grid.Y)*rows/layout.Grid.H))
	return step, row
}

func (g *Game) nativeGridTick(layout nativeMusicLayout, mx int) int {
	_, _, _, barSteps, partSteps, viewStart := g.nativePartView()
	barTicks := barSteps * editor.NativeTicksPerStep
	localPixel := min(layout.Grid.W-1, max(0, mx-layout.Grid.X))
	raw := viewStart*editor.NativeTicksPerStep + localPixel*barTicks/layout.Grid.W
	clock := performance.NewTrackerClock(120, max(1, partSteps*editor.NativeTicksPerStep))
	tick := int(clock.QuantizeTickSwing(int64(raw), g.nativeGridQuantize, g.nativeSwing))
	return min(partSteps*editor.NativeTicksPerStep-1, max(0, tick))
}

func (g *Game) nativeGridSnapTicks() int {
	if g.nativeGridQuantize == performance.QuantizeOff {
		return 1
	}
	return int(g.nativeGridQuantize.Ticks())
}

// nativePianoRows keeps keys large enough to hit while taking advantage of
// taller displays. A normal desktop shows three octaves; 2K and 4K workspaces
// progressively expose four and five without vertical scrolling.
func nativePianoRows(grid editor.Rect, channel int) int {
	if channel == 3 {
		return 8
	}
	rows := (grid.H / 16 / 12) * 12
	return min(60, max(36, rows))
}

func (g *Game) nativePitchRange() (int, int) {
	if g.nativeChannel == 3 {
		return 36, 43
	}
	soundID := g.nativeInstrument[g.nativeChannel]
	if sound, ok := g.nativeSoundAtStep(g.nativeSelectedStep); ok {
		soundID = sound.ID
	}
	for _, sound := range g.model.NativeAudio.Library.Sounds {
		if int(sound.ID) != soundID || int(sound.Kind) != g.nativeChannel {
			continue
		}
		if low, high, ok := runtimeaudio.NativeSoundNoteRange(sound); ok {
			return int(low), int(high)
		}
	}
	if g.nativeChannel == 2 {
		return 24, 127
	}
	return 36, 127
}

func (g *Game) nativePitchValid(note int) bool {
	low, high := g.nativePitchRange()
	return note >= low && note <= high
}

func (g *Game) nativePitchRangeMessage() string {
	low, high := g.nativePitchRange()
	return fmt.Sprintf("CH%d plays %s to %s", g.nativeChannel+1, nativeMIDINoteName(low), nativeMIDINoteName(high))
}

func (g *Game) nativeOctaveBounds(layout nativeMusicLayout) (int, int) {
	if g.nativeChannel == 3 {
		return 0, 8
	}
	rows := nativePianoRows(layout.Grid, g.nativeChannel)
	low, high := g.nativePitchRange()
	// Keep the lowest view fully inside the concrete instrument's range. The
	// highest view may contain a few disabled keys above MIDI 127 so G9 remains
	// reachable without inventing a partial-octave navigation mode.
	minimum := max(0, (low+rows+11)/12-3)
	maximum := min(8, (high+12)/12-3)
	if minimum > maximum {
		minimum = maximum
	}
	return minimum, maximum
}

func (g *Game) clampNativeOctave(layout nativeMusicLayout) {
	minimum, maximum := g.nativeOctaveBounds(layout)
	g.nativeOctave = min(max(g.nativeOctave, minimum), maximum)
}

func (g *Game) nudgeNativeOctave(layout nativeMusicLayout, delta int) {
	minimum, maximum := g.nativeOctaveBounds(layout)
	g.nativeOctave = min(max(g.nativeOctave+delta, minimum), maximum)
}

func (g *Game) nudgeNativeBar(delta int) {
	bars, _, _, barSteps, _, _ := g.nativePartView()
	g.nativeBar = min(max(g.nativeBar+delta, 0), bars-1)
	g.nativeSelectedStep = g.nativeBar * barSteps
	g.statusMsg = fmt.Sprintf("Bar %d of %d", g.nativeBar+1, bars)
}

func (g *Game) nativeNoteRow(note int) int {
	if g.nativeChannel == 3 {
		return note - 36
	}
	return (g.nativeOctave+3)*12 - 1 - note
}

func (g *Game) nativeGridNote(row int) int {
	if g.nativeChannel == 3 {
		return 36 + row
	}
	return (g.nativeOctave+3)*12 - 1 - row
}

func (g *Game) setNativeRangeFromCells(startStep, startRow, endStep, endRow int) {
	g.nativeRangeStartTick = min(startStep, endStep) * editor.NativeTicksPerStep
	g.nativeRangeEndTick = (max(startStep, endStep) + 1) * editor.NativeTicksPerStep
	startNote, endNote := g.nativeGridNote(startRow), g.nativeGridNote(endRow)
	g.nativeRangeLowNote, g.nativeRangeHighNote = min(startNote, endNote), max(startNote, endNote)
}

func (g *Game) nativeRangeEvents() []assetsync.NativeEventAsset {
	if !g.nativeRangeActive || g.nativeSFXMode {
		return nil
	}
	return g.model.NativeNoteEventsInRange(g.nativeSequence, g.nativeChannel, g.nativeRangeStartTick, g.nativeRangeEndTick, g.nativeRangeLowNote, g.nativeRangeHighNote)
}

func (g *Game) nativeEventRangeSelected(event assetsync.NativeEventAsset) bool {
	return g.nativeRangeActive && event.Tick < g.nativeRangeEndTick && event.Tick+event.DurationTicks > g.nativeRangeStartTick &&
		event.Note >= g.nativeRangeLowNote && event.Note <= g.nativeRangeHighNote
}

func (g *Game) nativeNoteAtCell(step, row int) (assetsync.NativeEventAsset, int, bool) {
	event, start, ok := g.nativeEventAtStep(step)
	if !ok || g.nativeNoteRow(event.Note) != row {
		return assetsync.NativeEventAsset{}, 0, false
	}
	return event, start, true
}

func (g *Game) placeNativeGridNoteTicks(startTick, row, durationTicks int) (int, error) {
	note := (g.nativeOctave+3)*12 - 1 - row
	if g.nativeChannel == 3 {
		note = 36 + row
	}
	if !g.nativePitchValid(note) {
		return note, fmt.Errorf("%s", g.nativePitchRangeMessage())
	}
	return note, g.nativeSetNoteTicks(startTick, note, durationTicks, g.nativeInstrument[g.nativeChannel])
}

func (g *Game) clampNativeInstrument(channel int) {
	if channel < 0 || channel > 3 {
		return
	}
	sounds := g.model.NativeSoundsForChannel(channel)
	for _, sound := range sounds {
		if sound.ID == g.nativeInstrument[channel] {
			return
		}
	}
	g.nativeInstrument[channel] = -1
	if len(sounds) != 0 {
		g.nativeInstrument[channel] = sounds[0].ID
	}
}

func (g *Game) cycleNativeInstrument(delta int) {
	sounds := g.model.NativeSoundsForChannel(g.nativeChannel)
	if len(sounds) == 0 {
		id, err := g.model.CreateNativeSound(g.nativeChannel, -1, "")
		if err != nil {
			g.statusMsg = err.Error()
			return
		}
		g.nativeInstrument[g.nativeChannel] = id
		g.markNativeMusicEdit("Instrument created")
		return
	}
	index := 0
	for i, sound := range sounds {
		if sound.ID == g.nativeInstrument[g.nativeChannel] {
			index = i
			break
		}
	}
	index = (index + delta + len(sounds)) % len(sounds)
	g.nativeInstrument[g.nativeChannel] = sounds[index].ID
	if _, _, ok := g.nativeEventAtStep(g.nativeSelectedStep); ok {
		if err := g.nativeAssignNoteSound(g.nativeSelectedStep, sounds[index].ID); err != nil {
			g.statusMsg = err.Error()
			return
		}
		g.markNativeMusicEdit("Note instrument: " + sounds[index].Name)
		return
	}
	g.refreshNativePerformanceSounds()
	g.statusMsg = "New notes use " + sounds[index].Name
}

func (g *Game) duplicateNativeInstrument() {
	source := g.nativeInstrument[g.nativeChannel]
	id, err := g.model.CreateNativeSound(g.nativeChannel, source, "")
	if err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.nativeInstrument[g.nativeChannel] = id
	if _, _, ok := g.nativeEventAtStep(g.nativeSelectedStep); ok {
		if err := g.nativeAssignNoteSound(g.nativeSelectedStep, id); err != nil {
			g.statusMsg = err.Error()
			return
		}
	}
	g.markNativeMusicEdit("Independent instrument copy created")
}

func (g *Game) selectNativeSoundPreset(index int) {
	presets := editor.NativeSoundPresets(g.nativeChannel)
	if index < 0 || index >= len(presets) {
		g.statusMsg = "Sound preset is unavailable"
		return
	}
	id, created, err := g.model.EnsureNativeSoundPreset(g.nativeChannel, index)
	if err != nil {
		g.statusMsg = err.Error()
		return
	}
	changed := created
	note := 60
	if g.nativeChannel == 2 {
		// GB Studio's C5 test note writes the unshifted hUGE period to CH3,
		// which is one physical octave below CH1/CH2. Mtile labels physical
		// pitch consistently, so MIDI 48 is the matching audition note.
		note = 48
	}
	if event, _, ok := g.nativeEventAtStep(g.nativeSelectedStep); ok {
		note = event.Note
		if event.SoundID != id {
			if err := g.nativeAssignNoteSound(g.nativeSelectedStep, id); err != nil {
				g.statusMsg = err.Error()
				return
			}
			changed = true
		}
	}
	g.nativeInstrument[g.nativeChannel] = id
	message := fmt.Sprintf("CH%d preset: %s", g.nativeChannel+1, presets[index].Name)
	if changed {
		g.markNativeMusicEdit(message)
	} else {
		g.refreshNativePerformanceSounds()
		g.statusMsg = message
	}
	g.playNativeSoundPreview(id, note)
}

func (g *Game) beginNativeNoteDrag(layout nativeMusicLayout, mx, my int) {
	step, row := g.nativeGridPosition(layout, mx, my)
	startTick := g.nativeGridTick(layout, mx)
	g.nativeNoteDrag = true
	g.nativeNoteDragDirty = false
	g.nativeNoteDragStep = step
	g.nativeNoteDragRow = row
	g.nativeNoteDragAnchorStep = step
	g.nativeNoteDragAnchorRow = row
	g.nativeNoteDragTick = startTick
	g.nativeNoteDragAnchorTick = startTick
	g.nativeNoteDragEdits = 0
	if ebiten.IsKeyPressed(ebiten.KeyShift) && !g.nativeSFXMode {
		g.nativeNoteDragMode = nativeNoteDragSelect
		g.nativeRangeActive = true
		g.setNativeRangeFromCells(step, row, step, row)
		g.statusMsg = "Note range selected — drag to extend"
		return
	}
	g.nativeRangeActive = false
	if event, start, ok := g.nativeNoteAtCell(step, row); ok {
		g.nativeNoteDragMode = nativeNoteDragResize
		g.nativeNoteDragAnchorStep = start
		g.nativeNoteDragAnchorTick = event.Tick
		g.nativeNoteDragTick = event.Tick
		g.nativeSelectedStep = start
		g.nativeNoteLength = nativeDurationSteps(event.DurationTicks)
		g.nativeNoteLengthTicks = event.DurationTicks
		g.statusMsg = "Note selected — drag horizontally to resize"
		return
	}

	g.nativeNoteDragMode = nativeNoteDragCreate
	note, err := g.placeNativeGridNoteTicks(startTick, row, g.nativeNoteLengthTicks)
	if err != nil {
		g.nativeNoteDrag = false
		g.nativeNoteDragMode = nativeNoteDragNone
		g.statusMsg = err.Error()
		return
	}
	g.nativeSelectedStep = startTick / editor.NativeTicksPerStep
	g.nativeNoteDragDirty = true
	g.nativeNoteDragEdits = 1
	g.statusMsg = "Note placed (" + nativeNoteLengthLabel(g.nativeNoteLengthTicks) + ")"
	g.playNativeNotePreview(g.nativeSelectedStep, note)
}

func (g *Game) extendNativeNoteDrag(layout nativeMusicLayout, mx, my int) {
	if !g.nativeNoteDrag || !rectContains(layout.Grid, mx, my) {
		return
	}
	step, row := g.nativeGridPosition(layout, mx, my)
	tick := g.nativeGridTick(layout, mx)
	if step == g.nativeNoteDragStep && row == g.nativeNoteDragRow {
		return
	}
	if g.nativeNoteDragMode == nativeNoteDragSelect {
		g.setNativeRangeFromCells(g.nativeNoteDragAnchorStep, g.nativeNoteDragAnchorRow, step, row)
		g.nativeNoteDragStep, g.nativeNoteDragRow = step, row
		g.statusMsg = fmt.Sprintf("Selected %d note(s)", len(g.nativeRangeEvents()))
		return
	}

	if g.nativeNoteDragMode == nativeNoteDragResize ||
		g.nativeNoteDragMode == nativeNoteDragCreate && row == g.nativeNoteDragAnchorRow && step >= g.nativeNoteDragAnchorStep {
		requestedTicks := max(1, tick-g.nativeNoteDragAnchorTick+g.nativeGridSnapTicks())
		accepted, err := g.nativeResizeNoteTicks(g.nativeNoteDragAnchorTick, requestedTicks)
		if err != nil {
			g.statusMsg = err.Error()
			return
		}
		g.nativeNoteLengthTicks = accepted
		g.nativeNoteLength = nativeDurationSteps(accepted)
		g.nativeSelectedStep = g.nativeNoteDragAnchorStep
		g.nativeNoteDragStep, g.nativeNoteDragRow, g.nativeNoteDragTick = step, row, tick
		g.nativeNoteDragDirty = true
		g.nativeNoteDragEdits++
		g.statusMsg = "Note length " + nativeNoteLengthLabel(accepted)
		return
	}
	if g.nativeNoteDragMode == nativeNoteDragCreate {
		g.nativeNoteDragMode = nativeNoteDragPaint
	}
	cells := editor.NativeDragCells(g.nativeNoteDragStep, g.nativeNoteDragRow, step, row)
	if len(cells) == 0 {
		return
	}

	lastStep, lastNote := -1, -1
	for _, cell := range cells {
		cellTick := int(performance.NewTrackerClock(120, g.nativePartTicks()).QuantizeTickSwing(int64(cell.Step*editor.NativeTicksPerStep), g.nativeGridQuantize, g.nativeSwing))
		note, err := g.placeNativeGridNoteTicks(cellTick, cell.Row, g.nativeNoteLengthTicks)
		if err != nil {
			g.statusMsg = err.Error()
			break
		}
		g.nativeNoteDragStep = cell.Step
		g.nativeNoteDragRow = cell.Row
		g.nativeSelectedStep = cell.Step
		g.nativeNoteDragDirty = true
		g.nativeNoteDragEdits++
		lastStep, lastNote = cell.Step, note
	}
	if lastStep < 0 {
		return
	}
	if g.nativeChannel == 3 {
		g.statusMsg = "Drum hit placed"
	} else {
		g.statusMsg = "Note placed"
	}
	g.playNativeNotePreview(lastStep, lastNote)
}

func (g *Game) finishNativeNoteDrag() {
	if !g.nativeNoteDrag {
		return
	}
	dirty, edits := g.nativeNoteDragDirty, g.nativeNoteDragEdits
	mode := g.nativeNoteDragMode
	g.nativeNoteDrag = false
	g.nativeNoteDragMode = nativeNoteDragNone
	g.nativeNoteDragDirty = false
	g.nativeNoteDragStep = -1
	g.nativeNoteDragRow = -1
	g.nativeNoteDragAnchorStep = -1
	g.nativeNoteDragAnchorRow = -1
	g.nativeNoteDragTick = -1
	g.nativeNoteDragAnchorTick = -1
	g.nativeNoteDragEdits = 0
	if mode == nativeNoteDragSelect {
		g.statusMsg = fmt.Sprintf("Selected %d note(s) — arrows move, Cmd/Ctrl+C copies", len(g.nativeRangeEvents()))
		return
	}
	if !dirty {
		return
	}
	message := "Note placed (" + nativeNoteLengthLabel(g.nativeNoteLengthTicks) + ")"
	if g.nativeChannel == 3 {
		message = "Drum hit placed"
	}
	if mode == nativeNoteDragResize || mode == nativeNoteDragCreate && edits > 1 {
		message = "Note length " + nativeNoteLengthLabel(g.nativeNoteLengthTicks)
	} else if edits > 1 {
		message = fmt.Sprintf("Drew %d notes", edits)
	}
	g.markNativeMusicEdit(message)
}
