package main

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/performance"
	runtimeaudio "github.com/drpaneas/mtile/runtime/audio"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	nativeEditNone = iota
	nativeEditSongName
	nativeEditClipName
	nativeEditPartName
	nativeEditSoundName
)

const (
	nativeFocusComposer = iota
	nativeFocusArrangement
)

const (
	nativeNoteDragNone = iota
	nativeNoteDragCreate
	nativeNoteDragPaint
	nativeNoteDragResize
	nativeNoteDragSelect
)

var (
	nativeChannelColors = [4]color.RGBA{
		{R: 255, G: 195, B: 32, A: 255},
		{R: 108, G: 214, B: 52, A: 255},
		{R: 99, G: 137, B: 255, A: 255},
		{R: 255, G: 92, B: 92, A: 255},
	}
	nativeChannelNames = [4]string{"PULSE + SWEEP", "PULSE", "WAVE", "NOISE"}
	nativeChannelUses  = [4]string{"LEAD / MELODY", "HARMONY / BASS", "CUSTOM TONE", "DRUMS / RHYTHM"}
	nativeNoteLengths  = [...]struct {
		ticks int
		label string
	}{
		{6, "1/64"}, {8, "1/32T"}, {12, "1/32"}, {16, "1/16T"}, {18, "1/32."},
		{24, "1/16"}, {32, "1/8T"}, {36, "1/16."}, {48, "1/8"}, {64, "1/4T"},
		{72, "1/8."}, {96, "1/4"}, {144, "1/4."}, {192, "1/2"}, {288, "1/2."}, {384, "4 BEATS"},
	}
	nativeMeters = [...]struct{ beats, unit int }{
		{4, 4}, {3, 4}, {2, 4}, {2, 2}, {3, 8}, {6, 8}, {9, 8}, {12, 8},
		{5, 4}, {7, 4}, {5, 8}, {7, 8}, {11, 8},
	}
	nativeGridSnaps = [...]performance.Quantize{
		performance.QuantizeSixtyFourth, performance.QuantizeThirtySecond,
		performance.QuantizeSixteenth, performance.QuantizeEighth, performance.QuantizeQuarter,
		performance.QuantizeSixteenthTriplet, performance.QuantizeEighthTriplet, performance.QuantizeQuarterTriplet,
		performance.QuantizeOff,
	}
)

func nativeNoteLengthLabel(ticks int) string {
	for _, length := range nativeNoteLengths {
		if length.ticks == ticks {
			return length.label
		}
	}
	return fmt.Sprintf("%d TICKS", max(1, ticks))
}

func nativeDurationSteps(durationTicks int) int {
	return max(1, (durationTicks+editor.NativeTicksPerStep-1)/editor.NativeTicksPerStep)
}

func nextNativeGridSnap(current performance.Quantize) performance.Quantize {
	for index, snap := range nativeGridSnaps {
		if snap == current {
			return nativeGridSnaps[(index+1)%len(nativeGridSnaps)]
		}
	}
	return performance.QuantizeSixtyFourth
}

func nativeSnapBoundaries(start, end int, grid performance.Quantize, swing int) []int {
	if grid == performance.QuantizeOff || end <= start {
		return nil
	}
	step := int(grid.Ticks())
	boundaries := make([]int, 0, (end-start)/max(1, step)+2)
	if grid.Swingable() && swing != 50 {
		pair := step * 2
		offbeat := (pair*min(75, max(50, swing)) + 50) / 100
		for base := start / pair * pair; base <= end; base += pair {
			for _, tick := range [...]int{base, base + offbeat} {
				if tick >= start && tick <= end {
					boundaries = append(boundaries, tick)
				}
			}
		}
		return boundaries
	}
	for tick := start / step * step; tick <= end; tick += step {
		if tick >= start {
			boundaries = append(boundaries, tick)
		}
	}
	return boundaries
}

func (g *Game) nativePartView() (bars, beats, unit, barSteps, partSteps, viewStart int) {
	if g.nativeSFXMode {
		steps := editor.NativeStepsPerPart
		if clip, ok := g.model.NativeClipByID(g.nativeSFXID); ok {
			steps = max(1, (clip.LengthTicks+editor.NativeTicksPerStep-1)/editor.NativeTicksPerStep)
		}
		return 1, 4, 4, steps, steps, 0
	}
	part, ok := g.model.NativePartAt(g.nativeSequence)
	if !ok {
		return 1, 4, 4, editor.NativeStepsPerPart, editor.NativeStepsPerPart, 0
	}
	bars, beats, unit, _ = editor.NativePartMeter(part)
	barSteps = g.model.NativePartBarStepCount(g.nativeSequence)
	partSteps = g.model.NativePartStepCount(g.nativeSequence)
	bar := min(max(g.nativeBar, 0), bars-1)
	return bars, beats, unit, barSteps, partSteps, bar * barSteps
}

func nativeChannelForKind(kind string) int {
	switch kind {
	case "ch2":
		return 1
	case "ch3":
		return 2
	case "ch4":
		return 3
	default:
		return 0
	}
}

func (g *Game) nativeCurrentClip() (assetsync.NativeClipAsset, bool) {
	if g.nativeSFXMode {
		return g.model.NativeClipByID(g.nativeSFXID)
	}
	return g.model.NativeClipAt(g.nativeSequence, g.nativeChannel)
}

func (g *Game) nativeEventAtStep(step int) (assetsync.NativeEventAsset, int, bool) {
	if g.nativeSFXMode {
		return g.model.NativeSFXEventAtStep(g.nativeSFXID, step)
	}
	return g.model.NativeNoteEventAtStep(g.nativeSequence, g.nativeChannel, step)
}

func (g *Game) nativeSoundAtStep(step int) (assetsync.NativeSoundAsset, bool) {
	if g.nativeSFXMode {
		return g.model.NativeSFXSoundAtStep(g.nativeSFXID, step)
	}
	return g.model.NativeSoundAtStep(g.nativeSequence, g.nativeChannel, step)
}

func (g *Game) nativeSetNote(step, note, duration, soundID int) error {
	return g.nativeSetNoteTicks(step*editor.NativeTicksPerStep, note, duration*editor.NativeTicksPerStep, soundID)
}

func (g *Game) nativeSetNoteTicks(startTick, note, durationTicks, soundID int) error {
	if g.nativeSFXMode {
		if g.nativeChannel == 3 {
			return g.model.SetNativeSFXNoiseHitTicks(g.nativeSFXID, startTick, note-36, durationTicks)
		}
		return g.model.SetNativeSFXNoteTicks(g.nativeSFXID, startTick, note, durationTicks, soundID)
	}
	if g.nativeChannel == 3 {
		return g.model.SetNativeNoiseHitTicks(g.nativeSequence, startTick, note-36, durationTicks)
	}
	return g.model.SetNativeNoteTicksWithSound(g.nativeSequence, g.nativeChannel, startTick, note, durationTicks, soundID)
}

func (g *Game) nativeDeleteNote(step int) error {
	if g.nativeSFXMode {
		return g.model.DeleteNativeSFXNote(g.nativeSFXID, step)
	}
	return g.model.DeleteNativeNote(g.nativeSequence, g.nativeChannel, step)
}

func (g *Game) nativeResizeNoteTicks(startTick, durationTicks int) (int, error) {
	if g.nativeSFXMode {
		return g.model.ResizeNativeSFXNoteTicks(g.nativeSFXID, startTick, durationTicks)
	}
	return g.model.ResizeNativeNoteTicks(g.nativeSequence, g.nativeChannel, startTick, durationTicks)
}

func (g *Game) nativeSetNoteLegato(step int, legato bool) error {
	if g.nativeSFXMode {
		return g.model.SetNativeSFXNoteLegato(g.nativeSFXID, step, legato)
	}
	return g.model.SetNativeNoteLegato(g.nativeSequence, g.nativeChannel, step, legato)
}

func (g *Game) nativeAssignNoteSound(step, soundID int) error {
	if g.nativeSFXMode {
		return g.model.AssignNativeSFXNoteSound(g.nativeSFXID, step, soundID)
	}
	return g.model.AssignNativeNoteSound(g.nativeSequence, g.nativeChannel, step, soundID)
}

func (g *Game) nativeUpdateSelectedSound(step int, edit func(*assetsync.NativeSoundAsset)) error {
	if g.nativeSFXMode {
		return g.model.UpdateNativeSFXSoundAtStep(g.nativeSFXID, step, edit)
	}
	return g.model.UpdateNativeSoundAtStep(g.nativeSequence, g.nativeChannel, step, edit)
}

func (g *Game) nativePartTicks() int {
	if part, ok := g.model.NativePartAt(g.nativeSequence); ok {
		return max(1, part.LengthTicks)
	}
	return editor.NativePartTicks
}

func (g *Game) selectNativeSequence(sequence int) {
	g.nativeSequence = sequence
	g.nativeBar = 0
	g.nativeSelectedStep = 0
	g.nativeRangeActive = false
}

type nativeMusicLayout = editor.NativeMusicLayout

func newNativeMusicLayout() nativeMusicLayout {
	return editor.NewNativeMusicLayout(screenWidth, editorTabsHeight, statusBarRect.Y)
}

func nativeChannelRect(layout nativeMusicLayout, channel int) editor.Rect {
	return layout.ChannelRect(channel)
}

func nativeChannelMuteRect(layout nativeMusicLayout, channel int) editor.Rect {
	return layout.ChannelMuteRect(channel)
}

func nativeChannelSoloRect(layout nativeMusicLayout, channel int) editor.Rect {
	return layout.ChannelSoloRect(channel)
}

func nativeVoiceModeRect(layout nativeMusicLayout, index int) editor.Rect {
	return layout.VoiceModeRect(index)
}

func nativeCaptureOptionRect(layout nativeMusicLayout, index int) editor.Rect {
	return layout.CaptureOptionRect(index)
}

func nativeArrangementVisibleRange(layout nativeMusicLayout, total, selected int) (start, end int) {
	visible := max(1, (layout.Arrangement.W-20)/148)
	selected = min(max(selected, 0), max(0, total-1))
	start = selected / visible * visible
	end = min(total, start+visible)
	return start, end
}

func nativeArrangementCardRect(layout nativeMusicLayout, visibleColumn int) editor.Rect {
	return layout.ArrangementCardRect(visibleColumn)
}

func nativeArrangementLaneRect(layout nativeMusicLayout, visibleColumn, channel int) editor.Rect {
	return layout.ArrangementLaneRect(visibleColumn, channel)
}

func nativeInspectorControlCount(channel int) int {
	return [...]int{9, 6, 5, 8}[min(max(channel, 0), 3)]
}

func nativeInspectorControlRect(layout nativeMusicLayout, channel, index int) editor.Rect {
	return layout.InspectorControlRect(channel, index)
}

func (g *Game) nativeInspectorControlIndices() []int {
	if g.nativeAdvanced {
		count := nativeInspectorControlCount(g.nativeChannel)
		indices := make([]int, count)
		for index := range indices {
			indices[index] = index
		}
		return indices
	}
	switch g.nativeChannel {
	case 0:
		return []int{0, 4, 5}
	case 1:
		return []int{0, 1, 2}
	case 2:
		return []int{0}
	default:
		return []int{3, 4}
	}
}

func (g *Game) clampNativeMusicSelection() {
	g.nativeFocus = min(max(g.nativeFocus, nativeFocusComposer), nativeFocusArrangement)
	g.nativeChannel = min(max(g.nativeChannel, 0), 3)
	g.nativeOctave = min(max(g.nativeOctave, 0), 8)
	if g.nativeSFXMode {
		effects := g.model.NativeSFXClips()
		found := false
		for _, clip := range effects {
			if clip.ID == g.nativeSFXID {
				g.nativeChannel = nativeChannelForKind(clip.Kind)
				found = true
				break
			}
		}
		if !found {
			g.nativeSFXID = -1
			if len(effects) != 0 {
				g.nativeSFXID = effects[0].ID
				g.nativeChannel = nativeChannelForKind(effects[0].Kind)
			}
		}
		g.nativeFocus = nativeFocusComposer
		_, _, _, _, partSteps, _ := g.nativePartView()
		g.nativeSelectedStep = min(max(g.nativeSelectedStep, 0), partSteps-1)
		g.nativeBar = 0
		g.nativeNoteLength = min(max(g.nativeNoteLength, 1), partSteps)
		if event, _, ok := g.nativeEventAtStep(g.nativeSelectedStep); ok {
			g.nativeNoteLength = nativeDurationSteps(event.DurationTicks)
			g.nativeNoteLengthTicks = event.DurationTicks
			g.nativeInstrument[g.nativeChannel] = event.SoundID
		}
		g.clampNativeInstrument(g.nativeChannel)
		return
	}
	song, ok := g.model.NativeSong()
	if !ok || len(song.Sequence) == 0 {
		g.nativeSequence = 0
		g.nativeBar = 0
		g.nativeSelectedStep = 0
		return
	}
	g.nativeSequence = min(max(g.nativeSequence, 0), len(song.Sequence)-1)
	bars, _, _, barSteps, partSteps, _ := g.nativePartView()
	g.nativeSelectedStep = min(max(g.nativeSelectedStep, 0), partSteps-1)
	g.nativeBar = min(max(g.nativeSelectedStep/barSteps, 0), bars-1)
	g.nativeNoteLength = min(max(g.nativeNoteLength, 1), partSteps)
	if event, _, ok := g.nativeEventAtStep(g.nativeSelectedStep); ok {
		g.nativeNoteLength = nativeDurationSteps(event.DurationTicks)
		g.nativeNoteLengthTicks = event.DurationTicks
		g.nativeInstrument[g.nativeChannel] = event.SoundID
	}
	g.clampNativeInstrument(g.nativeChannel)
	takes := g.model.NativeTakesAt(g.nativeSequence)
	g.nativeTakeSelection = min(max(g.nativeTakeSelection, 0), max(0, len(takes)-1))
}

func (g *Game) markNativeMusicEdit(message string) {
	g.clampNativeMusicSelection()
	g.refreshNativeMusicPreview()
	g.refreshNativePerformanceSounds()
	g.undo.forcePush(g.snapshot())
	g.statusMsg = message
}

func (g *Game) updateMusicMode() {
	g.clampNativeMusicSelection()
	g.clampNativeOctave(newNativeMusicLayout())
	g.updateNativeMusicPreviewLifecycle()
	g.updateNativeMIDI()
	g.updateNativeCapture()
	if g.updateNativeNameEdit() {
		return
	}
	performanceHandled := g.updateNativePerformanceKeyboard()
	g.updateNativeMusicKeyboard(performanceHandled)
	g.updateNativeMusicMouse(newNativeMusicLayout())
}

func (g *Game) updateNativeNameEdit() bool {
	if g.nativeNameTarget == nativeEditNone {
		return false
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		if unicode.IsControl(r) || len(g.nativeNameDraft)+utf8.RuneLen(r) > assetsync.NativeAudioMaxNameBytes {
			continue
		}
		g.nativeNameDraft += string(r)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && g.nativeNameDraft != "" {
		_, size := utf8.DecodeLastRuneInString(g.nativeNameDraft)
		g.nativeNameDraft = g.nativeNameDraft[:len(g.nativeNameDraft)-size]
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.nativeNameTarget = nativeEditNone
		g.statusMsg = ""
		return true
	}
	if !inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		return true
	}
	name := strings.TrimSpace(g.nativeNameDraft)
	var err error
	switch g.nativeNameTarget {
	case nativeEditSongName:
		if g.nativeSFXMode {
			err = g.model.RenameNativeSFX(g.nativeSFXID, name)
		} else {
			err = g.model.SetNativeSongName(name)
		}
	case nativeEditPartName:
		err = g.model.RenameNativePart(g.nativeSequence, name)
	case nativeEditSoundName:
		err = g.model.RenameNativeSound(g.nativeInstrument[g.nativeChannel], name)
	default:
		if g.nativeSFXMode {
			err = g.model.RenameNativeSFX(g.nativeSFXID, name)
		} else {
			err = g.model.RenameNativeClip(g.nativeSequence, g.nativeChannel, name)
		}
	}
	if err != nil {
		g.statusMsg = err.Error()
		return true
	}
	g.nativeNameTarget = nativeEditNone
	g.markNativeMusicEdit("Name saved")
	return true
}

func (g *Game) updateNativeMusicKeyboard(performanceHandled bool) {
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) && g.nativeRangeActive {
		g.nativeRangeActive = false
		g.statusMsg = "Note selection cleared"
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.toggleNativeMusicPreview()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyComma) {
		g.nudgeNativeOctave(newNativeMusicLayout(), -1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPeriod) {
		g.nudgeNativeOctave(newNativeMusicLayout(), 1)
	}
	if g.nativeFocus == nativeFocusArrangement && shift && inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
		g.moveNativeSection(-1)
		return
	}
	if g.nativeFocus == nativeFocusArrangement && shift && inpututil.IsKeyJustPressed(ebiten.KeyRight) {
		g.moveNativeSection(1)
		return
	}
	if g.nativeFocus == nativeFocusComposer && shift && g.nativeRangeActive {
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeyLeft):
			g.moveNativeRange(-g.nativeGridSnapTicks(), 0)
			return
		case inpututil.IsKeyJustPressed(ebiten.KeyRight):
			g.moveNativeRange(g.nativeGridSnapTicks(), 0)
			return
		case inpututil.IsKeyJustPressed(ebiten.KeyUp):
			g.moveNativeRange(0, 1)
			return
		case inpututil.IsKeyJustPressed(ebiten.KeyDown):
			g.moveNativeRange(0, -1)
			return
		}
	}
	if g.nativeFocus == nativeFocusArrangement && mod && inpututil.IsKeyJustPressed(ebiten.KeyD) {
		index, err := g.model.AddNativePart(g.nativeSequence)
		if err != nil {
			g.statusMsg = err.Error()
		} else {
			g.selectNativeSequence(index)
			g.markNativeMusicEdit("Section duplicated")
		}
		return
	}
	if g.nativeFocus == nativeFocusArrangement && shift && (inpututil.IsKeyJustPressed(ebiten.KeyDelete) || inpututil.IsKeyJustPressed(ebiten.KeyBackspace)) {
		selected, err := g.model.DeleteNativePart(g.nativeSequence)
		if err != nil {
			g.statusMsg = err.Error()
		} else {
			g.selectNativeSequence(selected)
			g.markNativeMusicEdit("Section deleted")
		}
		return
	}
	if g.nativeFocus == nativeFocusArrangement && inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
		g.selectNativeSequence(max(0, g.nativeSequence-1))
	} else if inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
		g.nativeSelectedStep = max(0, g.nativeSelectedStep-1)
	}
	if g.nativeFocus == nativeFocusArrangement && inpututil.IsKeyJustPressed(ebiten.KeyRight) {
		if song, ok := g.model.NativeSong(); ok {
			g.selectNativeSequence(min(len(song.Sequence)-1, g.nativeSequence+1))
		}
	} else if inpututil.IsKeyJustPressed(ebiten.KeyRight) {
		_, _, _, _, partSteps, _ := g.nativePartView()
		g.nativeSelectedStep = min(partSteps-1, g.nativeSelectedStep+1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		g.nativeChannel = max(0, g.nativeChannel-1)
		g.nativeRangeActive = false
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		g.nativeChannel = min(3, g.nativeChannel+1)
		g.nativeRangeActive = false
	}
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyC) {
		if g.nativeRangeActive && !g.nativeSFXMode {
			events := g.nativeRangeEvents()
			if len(events) == 0 {
				g.statusMsg = "No notes in selection"
				return
			}
			g.nativeNoteClipboard = append(g.nativeNoteClipboard[:0], events...)
			g.nativeNoteClipboardKind = g.nativeChannel
			g.nativeClipboardSet = false
			g.statusMsg = fmt.Sprintf("%d note(s) copied", len(events))
			return
		}
		if clip, ok := g.nativeCurrentClip(); ok {
			g.nativeClipboardID, g.nativeClipboardSet = clip.ID, true
			g.nativeNoteClipboard = nil
			g.statusMsg = "Clip copied"
		}
		return
	}
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyV) && len(g.nativeNoteClipboard) != 0 {
		if g.nativeSFXMode || g.nativeNoteClipboardKind != g.nativeChannel {
			g.statusMsg = "Paste notes into the same Game Boy hardware channel"
			return
		}
		destination := g.nativeSelectedStep * editor.NativeTicksPerStep
		if err := g.model.PasteNativeNoteEvents(g.nativeSequence, g.nativeChannel, destination, g.nativeNoteClipboard); err != nil {
			g.statusMsg = err.Error()
			return
		}
		g.selectPastedNativeRange(destination, g.nativeNoteClipboard)
		g.markNativeMusicEdit(fmt.Sprintf("%d note(s) pasted", len(g.nativeNoteClipboard)))
		return
	}
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyV) && g.nativeClipboardSet {
		if g.nativeSFXMode {
			g.statusMsg = "Select MUSIC to paste a clip into a part"
			return
		}
		if err := g.model.DuplicateNativeClipTo(g.nativeSequence, g.nativeChannel, g.nativeClipboardID); err != nil {
			g.statusMsg = err.Error()
		} else {
			g.markNativeMusicEdit("Clip pasted")
		}
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDelete) || inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		g.deleteNativeSelection()
		return
	}
	if performanceHandled {
		return
	}
	if note, ok := g.nativeKeyboardNote(); ok {
		if !g.nativePitchValid(note) {
			g.statusMsg = g.nativePitchRangeMessage()
			return
		}
		step := g.nativeSelectedStep
		startTick := step * editor.NativeTicksPerStep
		err := g.nativeSetNoteTicks(startTick, note, g.nativeNoteLengthTicks, g.nativeInstrument[g.nativeChannel])
		if err != nil {
			g.statusMsg = err.Error()
		} else {
			_, _, _, _, partSteps, _ := g.nativePartView()
			g.nativeSelectedStep = min(partSteps-1, (startTick+g.nativeNoteLengthTicks)/editor.NativeTicksPerStep)
			g.markNativeMusicEdit("Note placed (" + nativeNoteLengthLabel(g.nativeNoteLengthTicks) + ")")
			g.playNativeNotePreview(step, note)
		}
	}
}

func (g *Game) deleteNativeSelection() {
	var err error
	message := ""
	switch g.nativeFocus {
	case nativeFocusComposer:
		if g.nativeRangeActive && !g.nativeSFXMode {
			err = g.model.DeleteNativeNoteRange(g.nativeSequence, g.nativeChannel, g.nativeRangeStartTick, g.nativeRangeEndTick, g.nativeRangeLowNote, g.nativeRangeHighNote)
			message = "Selected notes erased"
			g.nativeRangeActive = false
		} else {
			err = g.nativeDeleteNote(g.nativeSelectedStep)
			message = "Note erased"
		}
	case nativeFocusArrangement:
		err = g.model.ClearNativeLane(g.nativeSequence, g.nativeChannel)
		message = "Selected channel clip removed"
	default:
		return
	}
	if err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.markNativeMusicEdit(message)
}

func (g *Game) moveNativeRange(deltaTick, deltaNote int) {
	actualStart, actualEnd, actualLow, actualHigh, ok := g.model.NativeNoteRangeBounds(
		g.nativeSequence, g.nativeChannel, g.nativeRangeStartTick, g.nativeRangeEndTick,
		g.nativeRangeLowNote, g.nativeRangeHighNote,
	)
	if !ok {
		g.statusMsg = "No notes in selection"
		return
	}
	low, high := g.nativePitchRange()
	if actualLow+deltaNote < low || actualHigh+deltaNote > high {
		g.statusMsg = g.nativePitchRangeMessage()
		return
	}
	if err := g.model.MoveNativeNoteRange(g.nativeSequence, g.nativeChannel, g.nativeRangeStartTick, g.nativeRangeEndTick,
		g.nativeRangeLowNote, g.nativeRangeHighNote, deltaTick, deltaNote); err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.nativeRangeStartTick = actualStart + deltaTick
	g.nativeRangeEndTick = actualEnd + deltaTick
	g.nativeRangeLowNote = actualLow + deltaNote
	g.nativeRangeHighNote = actualHigh + deltaNote
	g.nativeSelectedStep = max(0, g.nativeRangeStartTick/editor.NativeTicksPerStep)
	g.markNativeMusicEdit("Selected notes moved")
}

func (g *Game) selectPastedNativeRange(destination int, events []assetsync.NativeEventAsset) {
	if len(events) == 0 {
		return
	}
	start, end := destination+events[0].Tick, destination+events[0].Tick+events[0].DurationTicks
	low, high := events[0].Note, events[0].Note
	for _, event := range events[1:] {
		start = min(start, destination+event.Tick)
		end = max(end, destination+event.Tick+event.DurationTicks)
		low, high = min(low, event.Note), max(high, event.Note)
	}
	g.nativeRangeActive = true
	g.nativeRangeStartTick, g.nativeRangeEndTick = start, end
	g.nativeRangeLowNote, g.nativeRangeHighNote = low, high
}

func (g *Game) moveNativeSection(delta int) {
	selected, err := g.model.MoveNativePart(g.nativeSequence, delta)
	if err != nil {
		g.statusMsg = err.Error()
		return
	}
	if selected == g.nativeSequence {
		return
	}
	g.selectNativeSequence(selected)
	g.markNativeMusicEdit("Section moved")
}

func (g *Game) nativeKeyboardNote() (int, bool) {
	if g.nativeChannel == 3 {
		keys := [...]ebiten.Key{ebiten.KeyZ, ebiten.KeyX, ebiten.KeyC, ebiten.KeyV, ebiten.KeyB, ebiten.KeyN, ebiten.KeyM, ebiten.KeySlash}
		for preset, key := range keys {
			if inpututil.IsKeyJustPressed(key) {
				return 36 + preset, true
			}
		}
		return 0, false
	}
	keys := [...]ebiten.Key{ebiten.KeyZ, ebiten.KeyS, ebiten.KeyX, ebiten.KeyD, ebiten.KeyC, ebiten.KeyV, ebiten.KeyG, ebiten.KeyB, ebiten.KeyH, ebiten.KeyN, ebiten.KeyJ, ebiten.KeyM}
	for semitone, key := range keys {
		if inpututil.IsKeyJustPressed(key) {
			return (g.nativeOctave+1)*12 + semitone, true
		}
	}
	return 0, false
}

func (g *Game) updateNativeMusicMouse(layout nativeMusicLayout) {
	leftPressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	leftJustPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	if g.nativeNoteDrag && (!leftPressed || leftJustPressed) {
		g.finishNativeNoteDrag()
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		mx, my := ebiten.CursorPosition()
		if !g.nativeSFXMode && rectContains(layout.Meter, mx, my) {
			delta := 1
			if mx < layout.Meter.X+layout.Meter.W/2 {
				delta = -1
			}
			g.nudgeNativeMeterUnit(delta)
			return
		}
		if rectContains(layout.Grid, mx, my) {
			step, _ := g.nativeGridPosition(layout, mx, my)
			if err := g.nativeDeleteNote(step); err != nil {
				g.statusMsg = err.Error()
			} else {
				g.nativeSelectedStep = step
				g.markNativeMusicEdit("Note erased")
			}
		}
		return
	}
	if !leftJustPressed {
		if leftPressed {
			if g.nativeNoteDrag {
				mx, my := ebiten.CursorPosition()
				g.extendNativeNoteDrag(layout, mx, my)
				return
			}
			g.updateNativeWaveDraw(layout)
		}
		return
	}
	mx, my := ebiten.CursorPosition()
	switch {
	case rectContains(layout.SongPrevious, mx, my):
		g.cycleNativeSong(-1)
	case rectContains(layout.SongName, mx, my):
		name := "New Song"
		if g.nativeSFXMode {
			if clip, ok := g.model.NativeClipByID(g.nativeSFXID); ok {
				name = clip.Name
			}
		} else if song, ok := g.model.NativeSong(); ok {
			name = song.Name
		}
		g.nativeNameTarget, g.nativeNameDraft = nativeEditSongName, name
	case rectContains(layout.SongNext, mx, my):
		g.cycleNativeSong(1)
	case rectContains(layout.SongAdd, mx, my):
		g.addNativeSong()
	case rectContains(layout.SongDelete, mx, my):
		g.deleteNativeSong()
	case rectContains(layout.TempoDown, mx, my):
		g.nudgeNativeTempo(-1)
	case rectContains(layout.TempoValue, mx, my):
		g.toggleNativeTempoScope()
	case rectContains(layout.TempoUp, mx, my):
		g.nudgeNativeTempo(1)
	case rectContains(layout.Play, mx, my):
		g.playNativeMusicPreview()
	case rectContains(layout.Stop, mx, my):
		g.stopNativeTransport()
	case rectContains(layout.Arm, mx, my):
		if !g.nativeSFXMode {
			armed := g.nativeSession != nil && g.nativeSession.Snapshot().Armed
			g.setNativeArmed(!armed)
		}
	case rectContains(layout.Record, mx, my):
		if !g.nativeSFXMode {
			g.toggleNativeRecording()
		}
	}
	g.updateNativeCreateMouse(layout, mx, my)
	g.updateNativeSongMouse(layout, mx, my)
}

func (g *Game) cycleNativeSong(delta int) {
	if g.nativeSFXMode {
		g.cycleNativeSFX(delta)
		return
	}
	songs := g.model.NativeSongs()
	if len(songs) == 0 {
		return
	}
	index := (g.model.NativeSongIndex + delta + len(songs)) % len(songs)
	if err := g.model.SelectNativeSong(index); err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.stopNativeTransport()
	g.selectNativeSequence(0)
	g.clampNativeMusicSelection()
	g.refreshNativeMusicPreview()
	g.statusMsg = fmt.Sprintf("Song %d of %d: %s", index+1, len(songs), songs[index].Name)
}

func (g *Game) addNativeSong() {
	if g.nativeSFXMode {
		g.createNativeSFX(g.nativeChannel)
		return
	}
	index, err := g.model.AddNativeSong("")
	if err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.stopNativeTransport()
	g.selectNativeSequence(0)
	g.markNativeMusicEdit(fmt.Sprintf("Song %d created", index+1))
}

func (g *Game) deleteNativeSong() {
	if g.nativeSFXMode {
		g.deleteNativeSFX()
		return
	}
	index, err := g.model.DeleteNativeSong(g.model.NativeSongIndex)
	if err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.stopNativeTransport()
	g.selectNativeSequence(0)
	g.markNativeMusicEdit(fmt.Sprintf("Song deleted — now editing song %d", index+1))
}

func (g *Game) setNativeSFXMode(enabled bool) {
	if g.nativeSFXMode == enabled {
		return
	}
	g.stopNativeTransport()
	if enabled && g.nativeSession != nil {
		g.nativeSession.Arm(false, g.nativeSession.Now())
	}
	g.nativeSFXMode = enabled
	g.nativeSelectedStep = 0
	g.nativeBar = 0
	if enabled {
		effects := g.model.NativeSFXClips()
		if len(effects) == 0 {
			g.createNativeSFX(g.nativeChannel)
			return
		}
		g.nativeSFXID = effects[0].ID
		g.nativeChannel = nativeChannelForKind(effects[0].Kind)
		g.statusMsg = "SFX workspace — use the clip ID with sfx(id)"
	} else {
		g.statusMsg = "Music workspace"
	}
	g.clampNativeMusicSelection()
	g.refreshNativeMusicPreview()
}

func (g *Game) cycleNativeSFX(delta int) {
	effects := g.model.NativeSFXClips()
	if len(effects) == 0 {
		g.createNativeSFX(g.nativeChannel)
		return
	}
	index := 0
	for i, clip := range effects {
		if clip.ID == g.nativeSFXID {
			index = i
			break
		}
	}
	index = (index + delta + len(effects)) % len(effects)
	g.nativeSFXID = effects[index].ID
	g.nativeChannel = nativeChannelForKind(effects[index].Kind)
	g.nativeSelectedStep = 0
	g.clampNativeMusicSelection()
	g.refreshNativeMusicPreview()
	g.statusMsg = fmt.Sprintf("SFX %d/%d — sfx(%d)", index+1, len(effects), g.nativeSFXID)
}

func (g *Game) createNativeSFX(channel int) {
	id, err := g.model.CreateNativeSFX(channel, "")
	if err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.nativeSFXMode = true
	g.nativeSFXID = id
	g.nativeChannel = channel
	g.nativeSelectedStep = 0
	g.markNativeMusicEdit(fmt.Sprintf("CH%d effect created — call sfx(%d)", channel+1, id))
}

func (g *Game) deleteNativeSFX() {
	if g.nativeSFXID < 0 {
		return
	}
	if err := g.model.DeleteNativeSFX(g.nativeSFXID); err != nil {
		g.statusMsg = err.Error()
		return
	}
	effects := g.model.NativeSFXClips()
	g.nativeSFXID = -1
	if len(effects) != 0 {
		g.nativeSFXID = effects[0].ID
		g.nativeChannel = nativeChannelForKind(effects[0].Kind)
	}
	g.markNativeMusicEdit("Effect deleted")
}

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

func (g *Game) updateNativeSongMouse(layout nativeMusicLayout, mx, my int) {
	if g.nativeSFXMode {
		effects := g.model.NativeSFXClips()
		if len(effects) == 0 {
			return
		}
		selected := 0
		for index, clip := range effects {
			if clip.ID == g.nativeSFXID {
				selected = index
				break
			}
		}
		start, end := nativeArrangementVisibleRange(layout, len(effects), selected)
		for index := start; index < end; index++ {
			if !rectContains(nativeArrangementCardRect(layout, index-start), mx, my) {
				continue
			}
			g.nativeSFXID = effects[index].ID
			g.nativeChannel = nativeChannelForKind(effects[index].Kind)
			g.nativeSelectedStep = 0
			g.nativeFocus = nativeFocusComposer
			g.clampNativeMusicSelection()
			g.refreshNativeMusicPreview()
			g.statusMsg = fmt.Sprintf("Editing %s — sfx(%d)", effects[index].Name, effects[index].ID)
			return
		}
		return
	}
	if rectContains(layout.AddSection, mx, my) {
		index, err := g.model.AddNativePart(-1)
		if err != nil {
			g.statusMsg = err.Error()
		} else {
			g.selectNativeSequence(index)
			g.markNativeMusicEdit("Section added")
		}
		return
	}
	if rectContains(layout.ReuseSection, mx, my) {
		index, err := g.model.ReuseNativePart(g.nativeSequence, g.nativeSequence)
		if err != nil {
			g.statusMsg = err.Error()
		} else {
			g.selectNativeSequence(index)
			g.markNativeMusicEdit("Part reused — edits update every occurrence")
		}
		return
	}
	if rectContains(layout.Loop, mx, my) {
		song, _ := g.model.NativeSong()
		loopStart := g.nativeSequence
		if song.LoopStart != nil && *song.LoopStart == g.nativeSequence {
			loopStart = -1
		}
		if err := g.model.SetNativeSongLoopStart(loopStart); err != nil {
			g.statusMsg = err.Error()
		} else {
			g.markNativeMusicEdit("Song ending changed")
		}
		return
	}
	if rectContains(layout.PartBarsDown, mx, my) {
		g.nudgeNativePartBars(-1)
		return
	}
	if rectContains(layout.PartBarsValue, mx, my) || rectContains(layout.PartBarsUp, mx, my) {
		g.nudgeNativePartBars(1)
		return
	}
	if rectContains(layout.Meter, mx, my) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			delta := 1
			if mx < layout.Meter.X+layout.Meter.W/2 {
				delta = -1
			}
			g.nudgeNativeMeterBeats(delta)
		} else {
			g.cycleNativeMeter()
		}
		return
	}
	takes := g.model.NativeTakesAt(g.nativeSequence)
	if rectContains(layout.TakePrevious, mx, my) && len(takes) != 0 {
		g.nativeTakeSelection = (g.nativeTakeSelection - 1 + len(takes)) % len(takes)
		g.statusMsg = fmt.Sprintf("Take %d of %d selected", g.nativeTakeSelection+1, len(takes))
		return
	}
	if rectContains(layout.TakeNext, mx, my) && len(takes) != 0 {
		g.nativeTakeSelection = (g.nativeTakeSelection + 1) % len(takes)
		g.statusMsg = fmt.Sprintf("Take %d of %d selected", g.nativeTakeSelection+1, len(takes))
		return
	}
	if rectContains(layout.TakeUse, mx, my) && len(takes) != 0 {
		tempo, _ := g.model.NativePartTempo(g.nativeSequence)
		clock := performance.NewTrackerClock(tempo, g.nativePartTicks())
		if err := g.model.UseNativePerformanceTakeSwing(g.nativeSequence, takes[g.nativeTakeSelection].ID, clock, g.nativeQuantize, g.nativeSwing, g.nativeOverdub); err != nil {
			g.statusMsg = err.Error()
		} else {
			g.markNativeMusicEdit(fmt.Sprintf("Take %d applied (%s)", g.nativeTakeSelection+1, g.nativeQuantize.String()))
		}
		return
	}
	song, ok := g.model.NativeSong()
	if !ok {
		return
	}
	start, end := nativeArrangementVisibleRange(layout, len(song.Sequence), g.nativeSequence)
	for sequence := start; sequence < end; sequence++ {
		column := sequence - start
		card := nativeArrangementCardRect(layout, column)
		header := editor.Rect{X: card.X, Y: card.Y, W: card.W, H: 27}
		if rectContains(header, mx, my) {
			g.selectNativeSequence(sequence)
			g.nativeFocus = nativeFocusArrangement
			if part, ok := g.model.NativePartAt(sequence); ok {
				g.nativeNameTarget, g.nativeNameDraft = nativeEditPartName, part.Name
			}
			return
		}
		for channel := 0; channel < 4; channel++ {
			if rectContains(nativeArrangementLaneRect(layout, column, channel), mx, my) {
				g.selectNativeSequence(sequence)
				g.nativeChannel = channel
				g.nativeFocus = nativeFocusArrangement
				if clip, ok := g.model.NativeClipAt(sequence, channel); ok {
					g.nativeNameTarget, g.nativeNameDraft = nativeEditClipName, clip.Name
				} else {
					if err := g.model.EnsureNativeClip(sequence, channel); err != nil {
						g.statusMsg = err.Error()
						return
					}
					g.markNativeMusicEdit("Clip added")
				}
				return
			}
		}
	}
}

func (g *Game) nudgeNativePartBars(delta int) {
	bars, beats, unit, _, _, _ := g.nativePartView()
	bars = min(max(bars+delta, 1), editor.NativeMaxPartBars)
	if err := g.model.SetNativePartMeter(g.nativeSequence, bars, beats, unit); err != nil {
		g.statusMsg = err.Error()
		return
	}
	if g.nativeBar >= bars {
		g.nativeBar = bars - 1
	}
	g.nativeSelectedStep = g.nativeBar * g.model.NativePartBarStepCount(g.nativeSequence)
	g.markNativeMusicEdit(fmt.Sprintf("Part length: %d bar(s)", bars))
}

func (g *Game) cycleNativeMeter() {
	bars, beats, unit, _, _, _ := g.nativePartView()
	index := -1
	for i, meter := range nativeMeters {
		if meter.beats == beats && meter.unit == unit {
			index = i
			break
		}
	}
	next := nativeMeters[(index+1)%len(nativeMeters)]
	if err := g.model.SetNativePartMeter(g.nativeSequence, bars, next.beats, next.unit); err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.nativeBar = min(g.nativeBar, bars-1)
	g.nativeSelectedStep = g.nativeBar * g.model.NativePartBarStepCount(g.nativeSequence)
	g.markNativeMusicEdit(fmt.Sprintf("Time signature: %d/%d", next.beats, next.unit))
}

func (g *Game) nudgeNativeMeterBeats(delta int) {
	bars, beats, unit, _, _, _ := g.nativePartView()
	g.setNativeMeter(bars, min(16, max(1, beats+delta)), unit)
}

func (g *Game) nudgeNativeMeterUnit(delta int) {
	bars, beats, unit, _, _, _ := g.nativePartView()
	units := [...]int{1, 2, 4, 8, 16, 32}
	index := 0
	for i, candidate := range units {
		if candidate == unit {
			index = i
			break
		}
	}
	g.setNativeMeter(bars, beats, units[(index+delta+len(units))%len(units)])
}

func (g *Game) setNativeMeter(bars, beats, unit int) {
	if err := g.model.SetNativePartMeter(g.nativeSequence, bars, beats, unit); err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.nativeBar = min(g.nativeBar, bars-1)
	g.nativeSelectedStep = g.nativeBar * g.model.NativePartBarStepCount(g.nativeSequence)
	g.markNativeMusicEdit(fmt.Sprintf("Time signature: %d/%d", beats, unit))
}

func (g *Game) nudgeNativeTempo(delta int) {
	tempo, overridden := g.model.NativePartTempo(g.nativeSequence)
	if g.nativeSFXMode {
		overridden = false
		if song, ok := g.model.NativeSong(); ok {
			tempo = song.Tempo
		}
	}
	tempo = min(max(tempo+delta, 20), 400)
	var err error
	if !g.nativeSFXMode && overridden {
		err = g.model.SetNativePartTempo(g.nativeSequence, tempo)
	} else {
		err = g.model.SetNativeTempo(tempo)
	}
	if err != nil {
		g.statusMsg = err.Error()
	} else {
		scope := "song"
		if overridden {
			scope = "part"
		}
		g.markNativeMusicEdit(fmt.Sprintf("%s tempo %d BPM", scope, tempo))
	}
}

func (g *Game) toggleNativeTempoScope() {
	if g.nativeSFXMode {
		g.statusMsg = "Effects use the song tempo for preview"
		return
	}
	tempo, overridden := g.model.NativePartTempo(g.nativeSequence)
	target := tempo
	message := fmt.Sprintf("Part tempo %d BPM", tempo)
	if overridden {
		target = 0
		message = "Part inherits song tempo"
	}
	if err := g.model.SetNativePartTempo(g.nativeSequence, target); err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.markNativeMusicEdit(message)
}

func nextNativeNoteLength(current, delta int) int {
	if delta < 0 {
		for index := len(nativeNoteLengths) - 1; index >= 0; index-- {
			if nativeNoteLengths[index].ticks < current {
				return nativeNoteLengths[index].ticks
			}
		}
		return nativeNoteLengths[0].ticks
	}
	for _, length := range nativeNoteLengths {
		if length.ticks > current {
			return length.ticks
		}
	}
	return nativeNoteLengths[len(nativeNoteLengths)-1].ticks
}

func (g *Game) nudgeNativeNoteLength(delta int) {
	target := nextNativeNoteLength(g.nativeNoteLengthTicks, delta)
	g.nativeNoteLengthTicks = target
	g.nativeNoteLength = nativeDurationSteps(target)
	event, start, ok := g.nativeEventAtStep(g.nativeSelectedStep)
	if !ok {
		g.statusMsg = "New note length " + nativeNoteLengthLabel(target)
		return
	}
	accepted, err := g.nativeResizeNoteTicks(event.Tick, target)
	if err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.nativeSelectedStep = start
	g.nativeNoteLengthTicks = accepted
	g.nativeNoteLength = nativeDurationSteps(accepted)
	g.markNativeMusicEdit("Note length " + nativeNoteLengthLabel(accepted))
}

func cycleNativeDuty(current float64, delta int) float64 {
	values := [...]float64{12.5, 25, 50, 75}
	index := 0
	for i, value := range values {
		if current == value {
			index = i
		}
	}
	index = (index + delta + len(values)) % len(values)
	return values[index]
}

func cycleNativeDirection(current string, delta int) string {
	values := [...]string{"down", "up"}
	index := 0
	if current == "up" {
		index = 1
	}
	return values[(index+delta+len(values))%len(values)]
}

func cycleNativeRoute(current string, delta int) string {
	values := [...]string{"none", "right", "left", "both"}
	index := 0
	for i, value := range values {
		if current == value {
			index = i
		}
	}
	return values[(index+delta+len(values))%len(values)]
}

func nudgeNativeLength(value *int, enabled *bool, delta, maximum int) {
	current := -1
	if *enabled {
		current = *value
	}
	current = min(max(current+delta, -1), maximum)
	*enabled = current >= 0
	if current >= 0 {
		*value = current
	}
}

func (g *Game) nudgeNativeSoundControl(control, delta int) {
	edit := func(sound *assetsync.NativeSoundAsset) {
		switch g.nativeChannel {
		case 0:
			switch control {
			case 0:
				sound.Pulse.Duty = cycleNativeDuty(sound.Pulse.Duty, delta)
			case 1:
				sound.Pulse.Sweep.Pace = min(max(sound.Pulse.Sweep.Pace+delta, 0), 7)
			case 2:
				sound.Pulse.Sweep.Shift = min(max(sound.Pulse.Sweep.Shift+delta, 0), 7)
			case 3:
				sound.Pulse.Sweep.Direction = cycleNativeDirection(sound.Pulse.Sweep.Direction, delta)
			case 4:
				sound.Pulse.Envelope.Initial = min(max(sound.Pulse.Envelope.Initial+delta, 0), 15)
			case 5:
				sound.Pulse.Envelope.Pace = min(max(sound.Pulse.Envelope.Pace+delta, 0), 7)
			case 6:
				sound.Pulse.Envelope.Direction = cycleNativeDirection(sound.Pulse.Envelope.Direction, delta)
			case 7:
				nudgeNativeLength(&sound.Pulse.Length, &sound.Pulse.UseLength, delta, 63)
			case 8:
				sound.Route = cycleNativeRoute(sound.Route, delta)
			}
		case 1:
			switch control {
			case 0:
				sound.Pulse.Duty = cycleNativeDuty(sound.Pulse.Duty, delta)
			case 1:
				sound.Pulse.Envelope.Initial = min(max(sound.Pulse.Envelope.Initial+delta, 0), 15)
			case 2:
				sound.Pulse.Envelope.Pace = min(max(sound.Pulse.Envelope.Pace+delta, 0), 7)
			case 3:
				sound.Pulse.Envelope.Direction = cycleNativeDirection(sound.Pulse.Envelope.Direction, delta)
			case 4:
				nudgeNativeLength(&sound.Pulse.Length, &sound.Pulse.UseLength, delta, 63)
			case 5:
				sound.Route = cycleNativeRoute(sound.Route, delta)
			}
		case 2:
			levels := [...]int{0, 25, 50, 75, 100}
			switch control {
			case 0:
				index := 0
				for i, value := range levels {
					if sound.Wave.Level == value {
						index = i
					}
				}
				sound.Wave.Level = levels[(index+delta+len(levels))%len(levels)]
			case 1:
				sound.Wave.StartBank = (sound.Wave.StartBank + delta + 2) % 2
			case 2:
				if len(sound.Wave.Samples) == 32 {
					sound.Wave.Samples = append(sound.Wave.Samples, append([]int(nil), sound.Wave.Samples...)...)
				} else {
					sound.Wave.Samples = append([]int(nil), sound.Wave.Samples[:32]...)
				}
			case 3:
				nudgeNativeLength(&sound.Wave.Length, &sound.Wave.UseLength, delta, 255)
			case 4:
				sound.Route = cycleNativeRoute(sound.Route, delta)
			}
		case 3:
			switch control {
			case 0:
				if sound.Noise.Width == 7 {
					sound.Noise.Width = 15
				} else {
					sound.Noise.Width = 7
				}
			case 1:
				sound.Noise.Divisor = min(max(sound.Noise.Divisor+delta, 0), 7)
			case 2:
				sound.Noise.Shift = min(max(sound.Noise.Shift+delta, 0), 13)
			case 3:
				sound.Noise.Envelope.Initial = min(max(sound.Noise.Envelope.Initial+delta, 0), 15)
			case 4:
				sound.Noise.Envelope.Pace = min(max(sound.Noise.Envelope.Pace+delta, 0), 7)
			case 5:
				sound.Noise.Envelope.Direction = cycleNativeDirection(sound.Noise.Envelope.Direction, delta)
			case 6:
				nudgeNativeLength(&sound.Noise.Length, &sound.Noise.UseLength, delta, 63)
			case 7:
				sound.Route = cycleNativeRoute(sound.Route, delta)
			}
		}
	}
	var err error
	if _, _, selected := g.nativeEventAtStep(g.nativeSelectedStep); selected {
		err = g.nativeUpdateSelectedSound(g.nativeSelectedStep, edit)
	} else {
		err = g.model.UpdateNativeSoundByID(g.nativeInstrument[g.nativeChannel], edit)
	}
	if err != nil {
		g.statusMsg = err.Error()
	} else {
		g.markNativeMusicEdit("Hardware sound updated")
	}
}

func nativePointerDelta(rect editor.Rect, x int) int {
	if x < rect.X+rect.W/2 {
		return -1
	}
	return 1
}

func (g *Game) nudgeNativeOutput(control, delta int) {
	settings := g.model.NativeAudio.Settings
	switch control {
	case 0:
		settings.LeftLevel = min(max(settings.LeftLevel+delta, 0), 7)
	case 1:
		settings.RightLevel = min(max(settings.RightLevel+delta, 0), 7)
	case 2:
		ratios := [...]int{25, 50, 100}
		index := len(ratios) - 1
		for candidate, ratio := range ratios {
			if settings.PSGRatio == ratio {
				index = candidate
				break
			}
		}
		index = min(max(index+delta, 0), len(ratios)-1)
		settings.PSGRatio = ratios[index]
	}
	if err := g.model.SetNativeOutput(settings.LeftLevel, settings.RightLevel, settings.PSGRatio); err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.markNativeMusicEdit("PSG hardware output updated")
}

func nativeWaveEditorRect(layout nativeMusicLayout) editor.Rect {
	return editor.Rect{X: layout.Inspector.X + 10, Y: layout.Inspector.Y + 234, W: layout.Inspector.W - 20, H: layout.Inspector.H - 244}
}

func (g *Game) updateNativeWaveDraw(layout nativeMusicLayout) {
	if g.nativeChannel != 2 || !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		return
	}
	mx, my := ebiten.CursorPosition()
	waveRect := nativeWaveEditorRect(layout)
	if !rectContains(waveRect, mx, my) {
		return
	}
	sound, ok := g.nativeSoundAtStep(g.nativeSelectedStep)
	if !ok {
		sound, ok = g.model.NativeSoundByID(g.nativeInstrument[2])
	}
	if !ok || sound.Wave == nil {
		return
	}
	sampleCount := len(sound.Wave.Samples)
	index := min(sampleCount-1, (mx-waveRect.X)*sampleCount/waveRect.W)
	value := 15 - min(15, (my-waveRect.Y)*16/waveRect.H)
	edit := func(sound *assetsync.NativeSoundAsset) {
		if index >= 0 && index < len(sound.Wave.Samples) {
			sound.Wave.Samples[index] = value
		}
	}
	var err error
	if _, _, selected := g.nativeEventAtStep(g.nativeSelectedStep); selected {
		err = g.nativeUpdateSelectedSound(g.nativeSelectedStep, edit)
	} else {
		err = g.model.UpdateNativeSoundByID(g.nativeInstrument[2], edit)
	}
	if err != nil {
		g.statusMsg = err.Error()
	} else {
		g.markNativeMusicEdit("Wave shape updated")
	}
}

func (g *Game) drawMusicMode(screen *ebiten.Image) {
	layout := newNativeMusicLayout()
	g.drawNativeToolbar(screen, layout)
	g.drawNativeWorkbench(screen, layout)
}

func drawNativeButton(screen *ebiten.Image, rect editor.Rect, label string, active bool, accent color.Color) {
	drawTactileButton(screen, rect, label, active, accent, false)
}

func drawNativeMiniButton(screen *ebiten.Image, rect editor.Rect, label string, active bool, accent color.Color) {
	drawTactileButton(screen, rect, label, active, accent, true)
}

func (g *Game) drawNativeToolbar(screen *ebiten.Image, layout nativeMusicLayout) {
	drawWorkspaceHeader(screen, layout.Header, "MUSIC LAB", "REAL 4-CHANNEL GAME BOY SOUND", colorMusic)
	nameLabel := "SONG"
	if g.nativeSFXMode {
		nameLabel = "EFFECT"
	}
	drawSmallText(screen, nameLabel, layout.SongName.X, layout.SongName.Y-uiSmallTextHeight-2, colorDim)
	name := "New Song"
	tempo := 120
	tempoScope := "SONG"
	if song, ok := g.model.NativeSong(); ok {
		name, tempo = song.Name, song.Tempo
	}
	if !g.nativeSFXMode {
		if effective, overridden := g.model.NativePartTempo(g.nativeSequence); effective != 0 {
			tempo = effective
			if overridden {
				tempoScope = "PART"
			}
		}
	}
	if g.nativeSFXMode {
		name = "New Effect"
		if clip, ok := g.model.NativeClipByID(g.nativeSFXID); ok {
			name = clip.Name
		}
	}
	if g.nativeNameTarget == nativeEditSongName {
		name = g.nativeNameDraft + "_"
	}
	drawNativeButton(screen, layout.SongPrevious, "<", false, colorMusic)
	fillRect(screen, layout.SongName, colorBackground)
	strokeRect(screen, layout.SongName, colorBorder)
	drawTextCentered(screen, truncateUIText(name, layout.SongName.W-16), layout.SongName, colorText)
	drawNativeButton(screen, layout.SongNext, ">", false, colorMusic)
	drawNativeButton(screen, layout.SongAdd, "+", false, colorMusic)
	drawNativeButton(screen, layout.SongDelete, "X", false, colorMusic)
	drawSmallText(screen, "TEMPO / "+tempoScope, layout.TempoDown.X, layout.TempoDown.Y-uiSmallTextHeight-2, colorDim)
	drawNativeButton(screen, layout.TempoDown, "-", false, colorMusic)
	strokeRect(screen, layout.TempoValue, colorBorder)
	drawTextCentered(screen, fmt.Sprintf("%d BPM", tempo), layout.TempoValue, colorText)
	drawNativeButton(screen, layout.TempoUp, "+", false, colorMusic)
	armed, recording := false, false
	if g.nativeSession != nil {
		snapshot := g.nativeSession.Snapshot()
		armed, recording = snapshot.Armed, snapshot.Recording
	}
	if g.nativeConductor != nil && g.nativeConductor.Active() {
		recording = true
	}
	if g.nativeSFXMode {
		drawNativeButton(screen, layout.Arm, fmt.Sprintf("sfx(%d)", g.nativeSFXID), true, colorHighlight)
		drawNativeButton(screen, layout.Record, "PSG", false, colorDanger)
	} else {
		drawNativeButton(screen, layout.Arm, "ARM", armed, colorHighlight)
		drawNativeButton(screen, layout.Record, "REC  O", recording, colorDanger)
	}
	drawNativeButton(screen, layout.Play, "PLAY  >", g.nativeMusicPreviewPlaying(), color.RGBA{R: 100, G: 214, B: 70, A: 255})
	drawNativeButton(screen, layout.Stop, "STOP  []", false, colorDanger)
}

func (g *Game) drawNativeChannelRail(screen *ebiten.Image, layout nativeMusicLayout) {
	fillRect(screen, layout.Channels, colorPanel)
	strokeRect(screen, layout.Channels, colorBorder)
	drawNativeButton(screen, layout.WorkflowMusic, "MUSIC", !g.nativeSFXMode, colorHighlight)
	drawNativeButton(screen, layout.WorkflowSFX, "SFX", g.nativeSFXMode, colorDanger)
	for channel := 0; channel < 4; channel++ {
		r := nativeChannelRect(layout, channel)
		if channel == g.nativeChannel {
			fillRect(screen, r, color.RGBA{R: 31, G: 36, B: 78, A: 255})
		}
		stroke := color.Color(colorBorder)
		if channel == g.nativeChannel {
			stroke = nativeChannelColors[channel]
		}
		strokeRect(screen, r, stroke)
		fillRect(screen, editor.Rect{X: r.X, Y: r.Y, W: 6, H: r.H}, nativeChannelColors[channel])
		drawNativeChannelIcon(screen, editor.Rect{X: r.X + 12, Y: r.Y + 9, W: 40, H: 40}, channel, nativeChannelColors[channel])
		textX := r.X + 60
		textWidth := nativeChannelMuteRect(layout, channel).X - textX - 5
		titleWidth := r.X + r.W - textX - 20
		drawSmallText(screen, truncateTextToWidth(fmt.Sprintf("CH%d  %s", channel+1, nativeChannelNames[channel]), titleWidth, uiSmallFace), textX, r.Y+10, colorText)
		drawSmallText(screen, truncateTextToWidth(nativeChannelUses[channel], textWidth, uiSmallFace), textX, r.Y+38, nativeChannelColors[channel])
		hasClip := false
		if g.nativeSFXMode {
			hasClip = channel == g.nativeChannel && g.nativeSFXID >= 0
		} else {
			_, hasClip = g.model.NativeClipAt(g.nativeSequence, channel)
		}
		if hasClip {
			drawText(screen, "*", r.X+r.W-14, r.Y+10, nativeChannelColors[channel])
		}
		if hasClip && g.nativeMusicPreviewPlaying() && g.nativeMuteMask&(1<<channel) == 0 {
			level := 3 + int(3*(1+math.Sin(float64(g.theme.frame)*.16+float64(channel)*1.7))/2)
			for segment := 0; segment < 6; segment++ {
				clr := rgba(nativeChannelColors[channel], 70)
				if segment < level {
					clr = nativeChannelColors[channel]
				}
				fillRect(screen, editor.Rect{X: r.X + r.W - 55 + segment*6, Y: r.Y + 42, W: 4, H: 7}, clr)
			}
		}
		drawNativeMiniButton(screen, nativeChannelMuteRect(layout, channel), "M", g.nativeMuteMask&(1<<channel) != 0, colorDanger)
		drawNativeMiniButton(screen, nativeChannelSoloRect(layout, channel), "S", g.nativeSoloMask&(1<<channel) != 0, colorHighlight)
	}
	if g.nativeSFXMode {
		drawSmallText(screen, "ONE EFFECT = ONE PSG CHANNEL", layout.Channels.X+12, layout.Channels.Y+326, colorText)
		drawSmallText(screen, "Select another channel to create", layout.Channels.X+12, layout.Channels.Y+350, colorDim)
		drawSmallText(screen, "a new hardware sound effect.", layout.Channels.X+12, layout.Channels.Y+370, colorDim)
		drawSmallText(screen, fmt.Sprintf("CALL FROM GAME:  sfx(%d)", g.nativeSFXID), layout.Channels.X+12, layout.Channels.Y+410, colorHighlight)
		drawSmallText(screen, "SPACE: PLAY / STOP", layout.Channels.X+12, layout.Channels.Y+438, colorDim)
		drawSmallText(screen, "PSG ONLY / PCM OFF / CH1-CH4", layout.Channels.X+12, layout.Channels.Y+layout.Channels.H-uiSmallTextHeight-6, colorHighlight)
		return
	}
	drawSmallText(screen, "PLAY MODE", layout.Channels.X+12, layout.Channels.Y+326, colorDim)
	modes := [...]string{"MONO", "POLY 2", "POLY 3", "DRUMS"}
	for index, label := range modes {
		active := int(g.nativeVoiceMode) == index
		drawNativeButton(screen, nativeVoiceModeRect(layout, index), label, active, colorHighlight)
	}
	drawSmallText(screen, "ARM is separate from REC.", layout.Channels.X+12, layout.Channels.Y+424, colorText)
	controller := "A-W-S-E... / MIDI READY"
	if g.nativeMIDIName != "" {
		controller = "LAUNCHKEY CONNECTED"
	}
	drawSmallText(screen, controller, layout.Channels.X+12, layout.Channels.Y+447, colorDim)
	drawNativeButton(screen, nativeCaptureOptionRect(layout, 0), "QUANTIZE  "+g.nativeQuantize.String(), true, colorHighlight)
	drawNativeButton(screen, nativeCaptureOptionRect(layout, 1), "OVERDUB", g.nativeOverdub, colorHighlight)
	drawNativeButton(screen, nativeCaptureOptionRect(layout, 2), "CLICK", g.nativeMetronome, colorHighlight)
	drawNativeButton(screen, nativeCaptureOptionRect(layout, 3), fmt.Sprintf("COUNT-IN  %d BAR", g.nativeCountIn), g.nativeCountIn > 0, colorHighlight)
	swingLabel := "SWING  STRAIGHT"
	if g.nativeSwing != 50 {
		swingLabel = fmt.Sprintf("SWING  %d%%", g.nativeSwing)
	}
	drawNativeButton(screen, nativeCaptureOptionRect(layout, 4), swingLabel, g.nativeSwing != 50, colorHighlight)
	drawSmallText(screen, "PSG ONLY / PCM OFF / CH1-CH4", layout.Channels.X+12, layout.Channels.Y+layout.Channels.H-uiSmallTextHeight-6, colorHighlight)
}

func drawNativeChannelIcon(screen *ebiten.Image, r editor.Rect, channel int, clr color.Color) {
	fillRect(screen, r, clr)
	dark := color.RGBA{R: 28, G: 31, B: 67, A: 255}
	if channel == 3 {
		for i := 0; i < 14; i++ {
			x := r.X + 5 + i*max(1, (r.W-10)/14)
			y := r.Y + r.H/2 + ((i*17)%13 - 6)
			fillRect(screen, editor.Rect{X: x, Y: y, W: 2, H: 2}, dark)
		}
		return
	}
	if channel < 2 {
		x := r.X + 5
		y := r.Y + 10
		for i := 0; i < 4; i++ {
			next := x + (r.W-10)/4
			drawLine(screen, x, y, next, y, dark)
			if i < 3 {
				nextY := r.Y + r.H - 10
				if y == nextY {
					nextY = r.Y + 10
				}
				drawLine(screen, next, y, next, nextY, dark)
				y = nextY
			}
			x = next
		}
		return
	}
	for i := 0; i < 15; i++ {
		x0 := r.X + 5 + i*(r.W-10)/15
		x1 := r.X + 5 + (i+1)*(r.W-10)/15
		y0 := r.Y + r.H/2 + int(14*math.Sin(float64(i)*math.Pi/4))
		y1 := r.Y + r.H/2 + int(14*math.Sin(float64(i+1)*math.Pi/4))
		drawLine(screen, x0, y0, x1, y1, dark)
	}
}

func (g *Game) drawNativeWorkbench(screen *ebiten.Image, layout nativeMusicLayout) {
	g.drawNativeChannelRail(screen, layout)
	fillRect(screen, layout.Main, color.RGBA{R: 29, G: 31, B: 72, A: 255})
	strokeRect(screen, layout.Main, colorBorder)
	fillRect(screen, layout.Allocation, color.RGBA{R: 22, G: 25, B: 58, A: 255})
	strokeRect(screen, layout.Allocation, nativeChannelColors[g.nativeChannel])
	allocation := "LIVE  " + g.nativeAllocationText()
	if g.nativeSFXMode {
		allocation = fmt.Sprintf("EFFECT  sfx(%d)  /  CH%d HARDWARE", g.nativeSFXID, g.nativeChannel+1)
	}
	if g.nativeAdvanced {
		allocation = "PSG OUTPUT"
	}
	drawTextCentered(screen, truncateUIText(allocation, layout.Allocation.W-16), layout.Allocation, nativeChannelColors[g.nativeChannel])
	if g.nativeAdvanced {
		settings := g.model.NativeAudio.Settings
		drawNativeButton(screen, layout.OutputLeft, fmt.Sprintf("L  %d", settings.LeftLevel), false, colorMusic)
		drawNativeButton(screen, layout.OutputRight, fmt.Sprintf("R  %d", settings.RightLevel), false, colorMusic)
		drawNativeButton(screen, layout.OutputMix, fmt.Sprintf("MIX %d%%", settings.PSGRatio), false, colorMusic)
	}
	if g.nativeChannel < 3 {
		legato := false
		if event, _, ok := g.nativeEventAtStep(g.nativeSelectedStep); ok {
			legato = event.Legato
		}
		label := "RETRIGGER"
		if legato {
			label = "TIE / LEGATO"
		}
		drawNativeButton(screen, layout.Articulation, label, legato, nativeChannelColors[g.nativeChannel])
	}
	drawText(screen, fmt.Sprintf("CH%d", g.nativeChannel+1), layout.Main.X+12, layout.ClipName.Y+(layout.ClipName.H-uiTextHeight)/2, nativeChannelColors[g.nativeChannel])
	drawSmallText(screen, "CLIP", layout.ClipName.X, layout.ClipName.Y-uiSmallTextHeight-2, colorDim)
	clipName := "Click grid to start"
	if clip, ok := g.nativeCurrentClip(); ok {
		clipName = clip.Name
	}
	if g.nativeNameTarget == nativeEditClipName {
		clipName = g.nativeNameDraft + "_"
	}
	fillRect(screen, layout.ClipName, colorBackground)
	strokeRect(screen, layout.ClipName, colorBorder)
	drawTextCentered(screen, truncateUIText(clipName, layout.ClipName.W-14), layout.ClipName, colorText)
	bars, _, _, _, _, _ := g.nativePartView()
	drawSmallText(screen, "BAR", layout.BarPrevious.X, layout.BarPrevious.Y-uiSmallTextHeight-2, colorDim)
	drawNativeButton(screen, layout.BarPrevious, "<", false, nativeChannelColors[g.nativeChannel])
	fillRect(screen, layout.BarValue, colorBackground)
	strokeRect(screen, layout.BarValue, colorBorder)
	barLabel := fmt.Sprintf("%d / %d", g.nativeBar+1, bars)
	drawTextCentered(screen, barLabel, layout.BarValue, colorText)
	drawNativeButton(screen, layout.BarNext, ">", false, nativeChannelColors[g.nativeChannel])
	drawNativeButton(screen, layout.GridSnap, "SNAP  "+g.nativeGridQuantize.String(), true, nativeChannelColors[g.nativeChannel])
	drawSmallText(screen, "NOTE LENGTH", layout.NoteLengthDown.X, layout.NoteLengthDown.Y-uiSmallTextHeight-2, colorDim)
	drawNativeButton(screen, layout.NoteLengthDown, "-", false, nativeChannelColors[g.nativeChannel])
	fillRect(screen, layout.NoteLengthValue, colorBackground)
	strokeRect(screen, layout.NoteLengthValue, colorBorder)
	lengthLabel := nativeNoteLengthLabel(g.nativeNoteLengthTicks)
	drawTextCentered(screen, lengthLabel, layout.NoteLengthValue, colorText)
	drawNativeButton(screen, layout.NoteLengthUp, "+", false, nativeChannelColors[g.nativeChannel])
	drawSmallText(screen, "OCTAVE", layout.OctaveDown.X, layout.OctaveDown.Y-uiSmallTextHeight-2, colorDim)
	drawNativeButton(screen, layout.OctaveDown, "-", false, nativeChannelColors[g.nativeChannel])
	oct := editor.Rect{X: layout.OctaveDown.X + 32, Y: layout.OctaveDown.Y, W: 72, H: 28}
	strokeRect(screen, oct, colorBorder)
	drawTextCentered(screen, fmt.Sprintf("%d", g.nativeOctave), oct, colorText)
	drawNativeButton(screen, layout.OctaveUp, "+", false, nativeChannelColors[g.nativeChannel])
	g.drawNativePianoGrid(screen, layout.Grid)
	g.drawNativeInspector(screen, layout)
	g.drawNativeArrangement(screen, layout)
}

func (g *Game) drawNativePianoGrid(screen *ebiten.Image, grid editor.Rect) {
	fillRect(screen, grid, color.RGBA{R: 24, G: 27, B: 66, A: 255})
	strokeRect(screen, grid, colorBorder)
	_, beats, unit, barSteps, _, viewStart := g.nativePartView()
	viewEnd := viewStart + barSteps
	stepsPerBeat := max(1, (editor.NativePartTicks/4*4/unit)/editor.NativeTicksPerStep)
	rows := nativePianoRows(grid, g.nativeChannel)
	for column := 0; column <= barSteps; column++ {
		x := grid.X + column*grid.W/barSteps
		line := color.RGBA{R: 63, G: 67, B: 119, A: 255}
		if column%stepsPerBeat == 0 {
			line = colorBorder
		} else if column%max(1, stepsPerBeat/2) == 0 {
			line = color.RGBA{R: 85, G: 89, B: 143, A: 255}
		}
		drawLine(screen, x, grid.Y, x, grid.Y+grid.H, line)
	}
	viewStartTick := viewStart * editor.NativeTicksPerStep
	viewEndTick := viewEnd * editor.NativeTicksPerStep
	if g.nativeGridQuantize.Ticks()%editor.NativeTicksPerStep != 0 || g.nativeGridQuantize.Swingable() && g.nativeSwing != 50 {
		for _, tick := range nativeSnapBoundaries(viewStartTick, viewEndTick, g.nativeGridQuantize, g.nativeSwing) {
			x := grid.X + (tick-viewStartTick)*grid.W/(barSteps*editor.NativeTicksPerStep)
			drawLine(screen, x, grid.Y, x, grid.Y+grid.H, color.RGBA{R: 118, G: 122, B: 181, A: 255})
		}
	}
	for row := 0; row <= rows; row++ {
		y := grid.Y + row*grid.H/rows
		drawLine(screen, grid.X, y, grid.X+grid.W, y, color.RGBA{R: 53, G: 57, B: 105, A: 255})
	}
	if g.nativeChannel == 3 {
		labels := [...]string{"KICK", "SNARE", "CL HAT", "OP HAT", "TOM 1", "TOM 2", "CLICK", "FX"}
		for row, label := range labels {
			drawSmallText(screen, label, grid.X-50, grid.Y+row*grid.H/8+(grid.H/8-uiSmallTextHeight)/2, colorDim)
		}
	}
	if g.nativeSelectedStep >= viewStart && g.nativeSelectedStep < viewEnd {
		selectedX := grid.X + (g.nativeSelectedStep-viewStart)*grid.W/barSteps
		strokeRect(screen, editor.Rect{X: selectedX + 1, Y: grid.Y + 1, W: max(1, grid.W/barSteps-2), H: grid.H - 2}, nativeChannelColors[g.nativeChannel])
	}
	if g.nativeChannel != 3 {
		selectedNote, selectedOK := g.nativeNoteAtStep(g.nativeSelectedStep)
		playingStep, playing := g.nativePreviewStep()
		playingNote, playingOK := g.nativeNoteAtStep(playingStep)
		for row := 0; row < rows; row++ {
			note := (g.nativeOctave+3)*12 - 1 - row
			key := editor.Rect{X: grid.X - 50, Y: grid.Y + row*grid.H/rows, W: 46, H: max(1, grid.H/rows)}
			accidental := note%12 == 1 || note%12 == 3 || note%12 == 6 || note%12 == 8 || note%12 == 10
			highlighted := selectedOK && note == selectedNote || playing && playingOK && note == playingNote
			valid := g.nativePitchValid(note)
			if !valid {
				fillRect(screen, key, color.RGBA{R: 39, G: 40, B: 52, A: 255})
			} else if highlighted {
				fillRect(screen, key, nativeChannelColors[g.nativeChannel])
			} else if accidental {
				fillRect(screen, key, color.RGBA{R: 24, G: 25, B: 37, A: 255})
			} else {
				fillRect(screen, key, color.RGBA{R: 220, G: 221, B: 230, A: 255})
			}
			strokeRect(screen, key, colorBorder)
			if valid && note%12 == 0 {
				drawSmallText(screen, nativeMIDINoteName(note), key.X+5, key.Y+(key.H-uiSmallTextHeight)/2, colorBackground)
			}
		}
	}
	if g.nativeRangeActive {
		viewStartTick := viewStart * editor.NativeTicksPerStep
		viewEndTick := viewEnd * editor.NativeTicksPerStep
		startTick := max(viewStartTick, g.nativeRangeStartTick)
		endTick := min(viewEndTick, g.nativeRangeEndTick)
		if endTick > startTick {
			barTicks := barSteps * editor.NativeTicksPerStep
			topRow := min(g.nativeNoteRow(g.nativeRangeLowNote), g.nativeNoteRow(g.nativeRangeHighNote))
			bottomRow := max(g.nativeNoteRow(g.nativeRangeLowNote), g.nativeNoteRow(g.nativeRangeHighNote))
			topRow, bottomRow = max(0, topRow), min(rows-1, bottomRow)
			if bottomRow >= topRow {
				r := editor.Rect{
					X: grid.X + (startTick-viewStartTick)*grid.W/barTicks,
					Y: grid.Y + topRow*grid.H/rows,
					W: max(1, (endTick-startTick)*grid.W/barTicks),
					H: max(1, (bottomRow-topRow+1)*grid.H/rows),
				}
				strokeRect(screen, r, colorText)
			}
		}
	}
	if clip, ok := g.nativeCurrentClip(); ok {
		viewStartTick := viewStart * editor.NativeTicksPerStep
		viewEndTick := viewEnd * editor.NativeTicksPerStep
		barTicks := barSteps * editor.NativeTicksPerStep
		for _, event := range clip.Events {
			step := event.Tick / editor.NativeTicksPerStep
			eventFinish := event.Tick + event.DurationTicks
			if eventFinish <= viewStartTick || event.Tick >= viewEndTick {
				continue
			}
			row := 0
			if g.nativeChannel == 3 {
				row = min(7, max(0, event.Note-36))
			} else {
				row = (g.nativeOctave+3)*12 - 1 - event.Note
				if row < 0 || row >= rows {
					continue
				}
			}
			startX := grid.X + (max(event.Tick, viewStartTick)-viewStartTick)*grid.W/barTicks
			finishX := grid.X + (min(eventFinish, viewEndTick)-viewStartTick)*grid.W/barTicks
			r := editor.Rect{X: startX + 3, Y: grid.Y + row*grid.H/rows + 3, W: max(4, finishX-startX-6), H: max(4, grid.H/rows-6)}
			fillRect(screen, r, nativeChannelColors[g.nativeChannel])
			selectedByCursor := false
			if selected, selectedStart, ok := g.nativeEventAtStep(g.nativeSelectedStep); ok {
				selectedByCursor = selectedStart == step && selected.Note == event.Note
			}
			if selectedByCursor || g.nativeEventRangeSelected(event) {
				strokeRect(screen, r, colorText)
				handleX := max(r.X, r.X+r.W-3)
				fillRect(screen, editor.Rect{X: handleX, Y: r.Y, W: 3, H: r.H}, colorText)
			}
		}
	}
	if step, ok := g.nativePreviewStep(); ok {
		if step >= viewStart && step < viewEnd {
			x := grid.X + (step-viewStart)*grid.W/barSteps
			if g.theme.level != effectsClean {
				drawLine(screen, x-2, grid.Y, x-2, grid.Y+grid.H, rgba(colorHighlight, 42))
				drawLine(screen, x+2, grid.Y, x+2, grid.Y+grid.H, rgba(colorHighlight, 42))
			}
			drawLine(screen, x, grid.Y, x, grid.Y+grid.H, colorHighlight)
		}
	}
	for beat := 0; beat < beats; beat++ {
		label := fmt.Sprint(beat + 1)
		if beat == 0 {
			label = "BEAT 1"
		}
		drawSmallText(screen, label, grid.X+beat*stepsPerBeat*grid.W/barSteps+3, grid.Y-uiSmallTextHeight-2, colorDim)
	}
}

func (g *Game) nativeNoteAtStep(step int) (int, bool) {
	event, _, ok := g.nativeEventAtStep(step)
	return event.Note, ok
}

func nativeMIDINoteName(note int) string {
	names := [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	if note < 0 {
		return "-"
	}
	return fmt.Sprintf("%s%d", names[note%12], note/12-1)
}

func (g *Game) drawNativeInspector(screen *ebiten.Image, layout nativeMusicLayout) {
	fillRect(screen, layout.Inspector, colorPanel)
	strokeRect(screen, layout.Inspector, nativeChannelColors[g.nativeChannel])
	drawSmallText(screen, fmt.Sprintf("CH%d SOUND CONTROLS", g.nativeChannel+1), layout.Inspector.X+12, layout.Inspector.Y+10, nativeChannelColors[g.nativeChannel])
	drawNativeButton(screen, layout.Advanced, map[bool]string{false: "SIMPLE", true: "ADV"}[g.nativeAdvanced], g.nativeAdvanced, nativeChannelColors[g.nativeChannel])
	g.clampNativeInstrument(g.nativeChannel)
	drawSmallText(screen, "INSTRUMENT", layout.InstrumentName.X, layout.InstrumentName.Y-uiSmallTextHeight-2, colorDim)
	drawNativeButton(screen, layout.InstrumentPrevious, "<", false, nativeChannelColors[g.nativeChannel])
	instrumentName := "NEW INSTRUMENT"
	if selected, exists := g.model.NativeSoundByID(g.nativeInstrument[g.nativeChannel]); exists {
		instrumentName = selected.Name
		if g.nativeNameTarget == nativeEditSoundName {
			instrumentName = g.nativeNameDraft + "_"
		}
	}
	fillRect(screen, layout.InstrumentName, colorBackground)
	strokeRect(screen, layout.InstrumentName, colorBorder)
	drawTextCentered(screen, truncateUIText(instrumentName, layout.InstrumentName.W-12), layout.InstrumentName, colorText)
	drawNativeButton(screen, layout.InstrumentNext, ">", false, nativeChannelColors[g.nativeChannel])
	drawNativeButton(screen, layout.InstrumentDuplicate, "DUP", false, nativeChannelColors[g.nativeChannel])
	sound, ok := g.nativeSoundAtStep(g.nativeSelectedStep)
	if !ok {
		sound, ok = g.model.NativeSoundByID(g.nativeInstrument[g.nativeChannel])
	}
	if !ok {
		drawSmallText(screen, "Place a note to create", layout.Inspector.X+18, layout.Inspector.Y+104, colorDim)
		drawSmallText(screen, "this channel's sound.", layout.Inspector.X+18, layout.Inspector.Y+124, colorDim)
		return
	}
	labels := make([]string, nativeInspectorControlCount(g.nativeChannel))
	values := make([]string, len(labels))
	lengthText := func(enabled bool, value int) string {
		if !enabled {
			return "OFF"
		}
		return fmt.Sprint(value)
	}
	switch g.nativeChannel {
	case 0:
		labels = []string{"DUTY", "SWEEP TIME", "SWEEP SHIFT", "SWEEP DIR", "VOLUME", "ENV PACE", "ENV DIR", "HW CUTOFF", "ROUTE"}
		values = []string{fmt.Sprintf("%g%%", sound.Pulse.Duty), fmt.Sprint(sound.Pulse.Sweep.Pace), fmt.Sprint(sound.Pulse.Sweep.Shift), strings.ToUpper(sound.Pulse.Sweep.Direction), fmt.Sprint(sound.Pulse.Envelope.Initial), fmt.Sprint(sound.Pulse.Envelope.Pace), strings.ToUpper(sound.Pulse.Envelope.Direction), lengthText(sound.Pulse.UseLength, sound.Pulse.Length), strings.ToUpper(sound.Route)}
	case 1:
		labels = []string{"DUTY", "VOLUME", "ENV PACE", "ENV DIR", "HW CUTOFF", "ROUTE"}
		values = []string{fmt.Sprintf("%g%%", sound.Pulse.Duty), fmt.Sprint(sound.Pulse.Envelope.Initial), fmt.Sprint(sound.Pulse.Envelope.Pace), strings.ToUpper(sound.Pulse.Envelope.Direction), lengthText(sound.Pulse.UseLength, sound.Pulse.Length), strings.ToUpper(sound.Route)}
	case 2:
		labels = []string{"LEVEL", "WAVE BANK", "WAVE SIZE", "HW CUTOFF", "ROUTE"}
		values = []string{fmt.Sprintf("%d%%", sound.Wave.Level), fmt.Sprint(sound.Wave.StartBank), fmt.Sprintf("%d", len(sound.Wave.Samples)), lengthText(sound.Wave.UseLength, sound.Wave.Length), strings.ToUpper(sound.Route)}
	case 3:
		labels = []string{"WIDTH", "DIVISOR", "SHIFT", "VOLUME", "ENV PACE", "ENV DIR", "HW CUTOFF", "ROUTE"}
		values = []string{fmt.Sprintf("%d-BIT", sound.Noise.Width), fmt.Sprint(sound.Noise.Divisor), fmt.Sprint(sound.Noise.Shift), fmt.Sprint(sound.Noise.Envelope.Initial), fmt.Sprint(sound.Noise.Envelope.Pace), strings.ToUpper(sound.Noise.Envelope.Direction), lengthText(sound.Noise.UseLength, sound.Noise.Length), strings.ToUpper(sound.Route)}
	}
	for displayIndex, controlIndex := range g.nativeInspectorControlIndices() {
		r := nativeInspectorControlRect(layout, g.nativeChannel, displayIndex)
		label := labels[controlIndex]
		if !g.nativeAdvanced && (label == "ENV PACE" || label == "SWEEP TIME") {
			label = "DECAY"
		}
		drawSmallText(screen, label, r.X, r.Y-uiSmallTextHeight-2, colorDim)
		strokeRect(screen, r, colorBorder)
		drawSmallTextCentered(screen, "-", editor.Rect{X: r.X, Y: r.Y, W: 28, H: r.H}, colorText)
		drawSmallTextCentered(screen, truncateTextToWidth(values[controlIndex], r.W-62, uiSmallFace), editor.Rect{X: r.X + 28, Y: r.Y, W: r.W - 56, H: r.H}, nativeChannelColors[g.nativeChannel])
		drawSmallTextCentered(screen, "+", editor.Rect{X: r.X + r.W - 28, Y: r.Y, W: 28, H: r.H}, colorText)
	}
	if g.nativeChannel == 2 && sound.Wave != nil {
		waveRect := nativeWaveEditorRect(layout)
		fillRect(screen, waveRect, colorBackground)
		strokeRect(screen, waveRect, colorBorder)
		for i, sample := range sound.Wave.Samples {
			x := waveRect.X + i*waveRect.W/len(sound.Wave.Samples)
			h := max(1, sample*waveRect.H/15)
			fillRect(screen, editor.Rect{X: x + 1, Y: waveRect.Y + waveRect.H - h, W: max(1, waveRect.W/len(sound.Wave.Samples)-2), H: h}, nativeChannelColors[2])
		}
	}
}

func (g *Game) drawNativeArrangement(screen *ebiten.Image, layout nativeMusicLayout) {
	fillRect(screen, layout.Arrangement, color.RGBA{R: 24, G: 27, B: 63, A: 255})
	accent := color.Color(colorBorder)
	if g.nativeFocus == nativeFocusArrangement {
		accent = colorHighlight
	}
	strokeRect(screen, layout.Arrangement, accent)
	if g.nativeSFXMode {
		g.drawNativeSFXLibrary(screen, layout)
		return
	}
	drawText(screen, "SONG ORDER", layout.Arrangement.X+10, layout.Arrangement.Y+10, colorText)
	drawSmallText(screen, "Parts play left to right", layout.Arrangement.X+22+uiTextWidth("SONG ORDER"), layout.Arrangement.Y+14, colorDim)
	drawNativeButton(screen, layout.AddSection, "+ NEW", false, colorHighlight)
	drawNativeButton(screen, layout.ReuseSection, "+ REUSE", false, colorHighlight)
	song, ok := g.model.NativeSong()
	if !ok {
		drawText(screen, "Place a note to create the first part.", layout.Arrangement.X+22, layout.Arrangement.Y+62, colorDim)
		return
	}
	looping := song.LoopStart != nil
	loopLabel := "PLAY ONCE"
	if looping {
		loopLabel = fmt.Sprintf("LOOP %02d", *song.LoopStart+1)
	}
	drawNativeButton(screen, layout.Loop, loopLabel, looping, colorHighlight)
	bars, beats, unit, _, _, _ := g.nativePartView()
	drawNativeButton(screen, layout.PartBarsDown, "-", false, colorHighlight)
	fillRect(screen, layout.PartBarsValue, colorBackground)
	strokeRect(screen, layout.PartBarsValue, colorBorder)
	barsLabel := fmt.Sprintf("%d BAR", bars)
	if bars != 1 {
		barsLabel += "S"
	}
	drawSmallTextCentered(screen, barsLabel, layout.PartBarsValue, colorText)
	drawNativeButton(screen, layout.PartBarsUp, "+", false, colorHighlight)
	drawNativeButton(screen, layout.Meter, fmt.Sprintf("%d/%d", beats, unit), true, colorHighlight)
	takes := g.model.NativeTakesAt(g.nativeSequence)
	if len(takes) == 0 {
		drawSmallText(screen, "TAKES  0", layout.TakePrevious.X+8, layout.TakePrevious.Y+(layout.TakePrevious.H-uiSmallTextHeight)/2, colorDim)
	} else {
		g.nativeTakeSelection = min(max(g.nativeTakeSelection, 0), len(takes)-1)
		drawNativeButton(screen, layout.TakePrevious, "<", false, colorHighlight)
		drawSmallText(screen, fmt.Sprintf("TAKE %d/%d", g.nativeTakeSelection+1, len(takes)), layout.TakePrevious.X+36, layout.TakePrevious.Y+(layout.TakePrevious.H-uiSmallTextHeight)/2, colorText)
		drawNativeButton(screen, layout.TakeNext, ">", false, colorHighlight)
		drawNativeButton(screen, layout.TakeUse, "USE", false, colorHighlight)
	}
	start, end := nativeArrangementVisibleRange(layout, len(song.Sequence), g.nativeSequence)
	for sequence := start; sequence < end; sequence++ {
		column := sequence - start
		card := nativeArrangementCardRect(layout, column)
		fillRect(screen, card, color.RGBA{R: 29, G: 31, B: 72, A: 255})
		stroke := color.Color(colorBorder)
		if sequence == g.nativeSequence {
			stroke = colorHighlight
		}
		strokeRect(screen, card, stroke)
		header := editor.Rect{X: card.X, Y: card.Y, W: card.W, H: 27}
		if sequence == g.nativeSequence {
			fillRect(screen, header, color.RGBA{R: 50, G: 54, B: 104, A: 255})
		}
		name := fmt.Sprintf("PART %02d", sequence+1)
		if part, ok := g.model.NativePartAt(sequence); ok {
			name = part.Name
		}
		if g.nativeNameTarget == nativeEditPartName && sequence == g.nativeSequence {
			name = g.nativeNameDraft + "_"
		}
		titleWidth := header.W - 12
		if song.LoopStart != nil && sequence == *song.LoopStart {
			loopLabel := "LOOP"
			titleWidth -= smallTextWidth(loopLabel) + 8
			drawSmallText(screen, loopLabel, header.X+header.W-smallTextWidth(loopLabel)-6, header.Y+(header.H-uiSmallTextHeight)/2, colorHighlight)
		}
		titleRect := editor.Rect{X: header.X + 4, Y: header.Y, W: titleWidth, H: header.H}
		drawSmallTextCentered(screen, truncateTextToWidth(fmt.Sprintf("%02d  %s", sequence+1, name), titleRect.W-8, uiSmallFace), titleRect, colorHighlight)
		for channel := 0; channel < 4; channel++ {
			r := nativeArrangementLaneRect(layout, column, channel)
			if sequence == g.nativeSequence && channel == g.nativeChannel && g.nativeFocus == nativeFocusArrangement {
				fillRect(screen, r, color.RGBA{R: 50, G: 54, B: 104, A: 255})
			}
			fillRect(screen, editor.Rect{X: r.X, Y: r.Y, W: 4, H: r.H}, nativeChannelColors[channel])
			label := "+ ADD"
			if clip, ok := g.model.NativeClipAt(sequence, channel); ok {
				label = clip.Name
			}
			drawSmallTextCentered(screen, truncateTextToWidth(fmt.Sprintf("CH%d  %s", channel+1, label), r.W-12, uiSmallFace), r, nativeChannelColors[channel])
		}
	}
	showing := fmt.Sprintf("SHOWING %d-%d / %d", start+1, end, len(song.Sequence))
	showingY := layout.Arrangement.Y + 14
	if layout.Arrangement.W < 1000 {
		showingY = layout.Arrangement.Y + 50
	}
	drawSmallText(screen, showing, layout.Arrangement.X+layout.Arrangement.W-smallTextWidth(showing)-12, showingY, colorDim)
}

func (g *Game) drawNativeSFXLibrary(screen *ebiten.Image, layout nativeMusicLayout) {
	drawText(screen, "SFX LIBRARY", layout.Arrangement.X+10, layout.Arrangement.Y+12, colorText)
	drawSmallText(screen, truncateTextToWidth("Standalone hardware clips; use their stable sfx(id) in your game.", layout.Arrangement.W-270, uiSmallFace), layout.Arrangement.X+92, layout.Arrangement.Y+14, colorDim)
	effects := g.model.NativeSFXClips()
	if len(effects) == 0 {
		drawText(screen, "Click + above or select a channel to create your first sound effect.", layout.Arrangement.X+22, layout.Arrangement.Y+62, colorDim)
		return
	}
	selected := 0
	for index, clip := range effects {
		if clip.ID == g.nativeSFXID {
			selected = index
			break
		}
	}
	start, end := nativeArrangementVisibleRange(layout, len(effects), selected)
	for index := start; index < end; index++ {
		column := index - start
		clip := effects[index]
		channel := nativeChannelForKind(clip.Kind)
		card := nativeArrangementCardRect(layout, column)
		fillRect(screen, card, color.RGBA{R: 29, G: 31, B: 72, A: 255})
		stroke := color.Color(colorBorder)
		if clip.ID == g.nativeSFXID {
			stroke = nativeChannelColors[channel]
		}
		strokeRect(screen, card, stroke)
		fillRect(screen, editor.Rect{X: card.X, Y: card.Y, W: 6, H: card.H}, nativeChannelColors[channel])
		drawSmallText(screen, fmt.Sprintf("sfx(%d)", clip.ID), card.X+12, card.Y+10, nativeChannelColors[channel])
		drawSmallText(screen, truncateTextToWidth(clip.Name, card.W-24, uiSmallFace), card.X+12, card.Y+32, colorText)
		drawSmallText(screen, truncateTextToWidth(fmt.Sprintf("CH%d  %s", channel+1, nativeChannelNames[channel]), card.W-24, uiSmallFace), card.X+12, card.Y+54, colorDim)
		drawSmallText(screen, fmt.Sprintf("%d EVENT(S)", len(clip.Events)), card.X+12, card.Y+76, colorDim)
	}
	showing := fmt.Sprintf("SHOWING %d-%d / %d", start+1, end, len(effects))
	drawSmallText(screen, showing, layout.Arrangement.X+layout.Arrangement.W-smallTextWidth(showing)-12, layout.Arrangement.Y+14, colorDim)
}
