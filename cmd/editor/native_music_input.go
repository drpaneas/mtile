package main

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

func (g *Game) updateMusicMode() {
	g.clampNativeMusicSelection()
	// Selecting an existing note can select its instrument without going
	// through an edit action. Keep the armed Launchkey bank identical to the
	// note/preset currently shown by the editor before dispatching MIDI.
	g.syncNativePerformanceSounds()
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
	if g.nativePresetMenuOpen {
		presets := editor.NativeSoundPresets(g.nativeChannel)
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			g.nativePresetMenuOpen = false
			return
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
			g.nativePresetMenuCursor = max(0, g.nativePresetMenuCursor-1)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
			g.nativePresetMenuCursor = min(len(presets)-1, g.nativePresetMenuCursor+1)
		}
		visible := nativePresetMenuVisibleCount(newNativeMusicLayout(), len(presets))
		g.nativePresetMenuScroll = min(g.nativePresetMenuScroll, g.nativePresetMenuCursor)
		g.nativePresetMenuScroll = max(g.nativePresetMenuScroll, g.nativePresetMenuCursor-visible+1)
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) && len(presets) != 0 {
			g.selectNativeSoundPreset(g.nativePresetMenuCursor)
			g.nativePresetMenuOpen = false
		}
		return
	}
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
	if g.nativePresetMenuOpen {
		presets := editor.NativeSoundPresets(g.nativeChannel)
		visible := nativePresetMenuVisibleCount(layout, len(presets))
		maxScroll := max(0, len(presets)-visible)
		if _, wheelY := ebiten.Wheel(); wheelY != 0 {
			g.nativePresetMenuScroll = min(maxScroll, max(0, g.nativePresetMenuScroll-int(wheelY)))
			g.nativePresetMenuCursor = min(len(presets)-1, max(g.nativePresetMenuScroll, g.nativePresetMenuCursor))
		}
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			g.nativePresetMenuOpen = false
			return
		}
		mx, my := ebiten.CursorPosition()
		for row := 0; row < visible; row++ {
			index := g.nativePresetMenuScroll + row
			if index < len(presets) && rectContains(nativePresetMenuItemRect(layout, len(presets), row), mx, my) {
				g.nativePresetMenuCursor = index
				break
			}
		}
		if !leftJustPressed {
			return
		}
		if rectContains(layout.InstrumentPreset, mx, my) {
			g.nativePresetMenuOpen = false
			return
		}
		for row := 0; row < visible; row++ {
			index := g.nativePresetMenuScroll + row
			if index < len(presets) && rectContains(nativePresetMenuItemRect(layout, len(presets), row), mx, my) {
				g.nativePresetMenuCursor = index
				g.selectNativeSoundPreset(index)
				g.nativePresetMenuOpen = false
				return
			}
		}
		g.nativePresetMenuOpen = false
		return
	}
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
