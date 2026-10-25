package main

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
)

func (g *Game) drawMusicMode(screen *ebiten.Image) {
	layout := newNativeMusicLayout()
	g.drawNativeToolbar(screen, layout)
	g.drawNativeWorkbench(screen, layout)
}

// Yellow used to mark ordinary controls as well as the playhead. Keep it
// for playback timing; selections use the editor's common selection color.
func nativeControlAccent(accent color.Color) color.Color {
	if accent == colorHighlight {
		return colorSelection
	}
	return accent
}

func drawNativeButton(screen *ebiten.Image, rect editor.Rect, label string, active bool, accent color.Color) {
	drawTactileButton(screen, rect, label, active, nativeControlAccent(accent), false)
}

func drawNativeMiniButton(screen *ebiten.Image, rect editor.Rect, label string, active bool, accent color.Color) {
	drawTactileButton(screen, rect, label, active, nativeControlAccent(accent), true)
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
	strokeRect(screen, layout.Channels, colorSectionBorder)
	drawNativeButton(screen, layout.WorkflowMusic, "MUSIC", !g.nativeSFXMode, colorHighlight)
	drawNativeButton(screen, layout.WorkflowSFX, "SFX", g.nativeSFXMode, colorSelection)
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
		drawSmallText(screen, "PSG ONLY / PCM OFF / CH1-CH4", layout.Channels.X+12, layout.Channels.Y+layout.Channels.H-uiSmallTextHeight-6, colorDim)
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
	drawSmallText(screen, "PSG ONLY / PCM OFF / CH1-CH4", layout.Channels.X+12, layout.Channels.Y+layout.Channels.H-uiSmallTextHeight-6, colorDim)
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
	fillRect(screen, layout.Allocation, color.RGBA{R: 22, G: 25, B: 58, A: 255})
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
	if g.nativePresetMenuOpen {
		g.drawNativePresetMenu(screen, layout)
	}
}

func (g *Game) drawNativePianoGrid(screen *ebiten.Image, grid editor.Rect) {
	fillRect(screen, grid, color.RGBA{R: 24, G: 27, B: 66, A: 255})
	_, beats, unit, barSteps, _, viewStart := g.nativePartView()
	viewEnd := viewStart + barSteps
	stepsPerBeat := max(1, (editor.NativePartTicks/4*4/unit)/editor.NativeTicksPerStep)
	rows := nativePianoRows(grid, g.nativeChannel)
	for column := 0; column <= barSteps; column++ {
		x := grid.X + column*grid.W/barSteps
		line, width := nativePianoGridLineStyle(column, barSteps, stepsPerBeat)
		lineX := min(grid.X+grid.W-width, max(grid.X, x-width/2))
		fillRect(screen, editor.Rect{X: lineX, Y: grid.Y, W: width, H: grid.H}, line)
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
	// Paint the outer frame last so the stronger inner bar edge reads as a
	// deliberate measure boundary instead of replacing the panel border.
	strokeRect(screen, grid, colorBorder)
	if g.nativeChannel == 3 {
		labels := [8]string{"PAD 1", "PAD 2", "PAD 3", "PAD 4", "PAD 5", "PAD 6", "PAD 7", "PAD 8"}
		if clip, ok := g.nativeCurrentClip(); ok {
			for _, event := range clip.Events {
				row := event.Note - 36
				if row < 0 || row >= len(labels) {
					continue
				}
				if sound, exists := g.model.NativeSoundByID(event.SoundID); exists {
					labels[row] = sound.Name
				}
			}
		}
		for row, label := range labels {
			label = truncateTextToWidth(label, 48, uiSmallFace)
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
	// The recorder owns an unfinished take until STOP. Paint its materialized
	// notes directly over the piano roll so performance feedback is immediate,
	// without mutating the project or creating undo entries every frame.
	if g.nativeCaptureVisible && g.nativeCaptureSequence == g.nativeSequence {
		viewStartTick := viewStart * editor.NativeTicksPerStep
		viewEndTick := viewEnd * editor.NativeTicksPerStep
		barTicks := barSteps * editor.NativeTicksPerStep
		for _, note := range g.nativeCaptureNotes {
			if int(note.Channel) != g.nativeChannel+1 {
				continue
			}
			noteEnd := note.StartTick + note.DurationTicks
			if noteEnd <= viewStartTick || note.StartTick >= viewEndTick {
				continue
			}
			row := 0
			if g.nativeChannel == 3 {
				row = min(7, max(0, int(note.Note)-36))
			} else {
				row = (g.nativeOctave+3)*12 - 1 - int(note.Note)
				if row < 0 || row >= rows {
					continue
				}
			}
			startX := grid.X + (max(note.StartTick, viewStartTick)-viewStartTick)*grid.W/barTicks
			finishX := grid.X + (min(noteEnd, viewEndTick)-viewStartTick)*grid.W/barTicks
			r := editor.Rect{X: startX + 3, Y: grid.Y + row*grid.H/rows + 3, W: max(4, finishX-startX-6), H: max(4, grid.H/rows-6)}
			fillRect(screen, r, nativeChannelColors[g.nativeChannel])
			strokeRect(screen, r, colorHighlight)
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
	drawLine(screen, layout.Inspector.X, layout.Inspector.Y, layout.Inspector.X, layout.Inspector.Y+layout.Inspector.H, colorSectionBorder)
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
	drawNativeButton(screen, layout.InstrumentPreset, "PRESET LIBRARY  v", g.nativePresetMenuOpen, nativeChannelColors[g.nativeChannel])
	sound, ok := g.nativeSoundAtStep(g.nativeSelectedStep)
	if !ok {
		sound, ok = g.model.NativeSoundByID(g.nativeInstrument[g.nativeChannel])
	}
	if !ok {
		drawSmallText(screen, "Choose a preset or place a note", layout.Inspector.X+18, layout.Inspector.Y+148, colorDim)
		drawSmallText(screen, "to create this channel's sound.", layout.Inspector.X+18, layout.Inspector.Y+168, colorDim)
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

func (g *Game) drawNativePresetMenu(screen *ebiten.Image, layout nativeMusicLayout) {
	presets := editor.NativeSoundPresets(g.nativeChannel)
	if len(presets) == 0 {
		return
	}
	visible := nativePresetMenuVisibleCount(layout, len(presets))
	maxScroll := max(0, len(presets)-visible)
	g.nativePresetMenuScroll = min(maxScroll, max(0, g.nativePresetMenuScroll))
	menu := nativePresetMenuRect(layout, len(presets))
	fillRect(screen, menu, color.RGBA{R: 17, G: 19, B: 48, A: 255})
	strokeRect(screen, menu, nativeChannelColors[g.nativeChannel])
	drawSmallText(screen, fmt.Sprintf("CH%d  PRESET LIBRARY", g.nativeChannel+1), menu.X+10, menu.Y+9, nativeChannelColors[g.nativeChannel])
	for row := 0; row < visible; row++ {
		index := g.nativePresetMenuScroll + row
		if index >= len(presets) {
			break
		}
		item := nativePresetMenuItemRect(layout, len(presets), row)
		if index == g.nativePresetMenuCursor {
			fillRect(screen, item, color.RGBA{R: 48, G: 51, B: 98, A: 255})
			strokeRect(screen, item, nativeChannelColors[g.nativeChannel])
		} else if row%2 != 0 {
			fillRect(screen, item, color.RGBA{R: 24, G: 27, B: 63, A: 255})
		}
		label := fmt.Sprintf("%02d  %s", index+1, presets[index].Name)
		drawSmallText(screen, truncateTextToWidth(label, item.W-18, uiSmallFace), item.X+9, item.Y+(item.H-uiSmallTextHeight)/2, colorText)
	}
	footerY := menu.Y + menu.H - nativePresetMenuFooterHeight
	footer := fmt.Sprintf("%d-%d / %d   WHEEL OR ARROWS + ENTER", g.nativePresetMenuScroll+1, min(len(presets), g.nativePresetMenuScroll+visible), len(presets))
	drawSmallText(screen, truncateTextToWidth(footer, menu.W-20, uiSmallFace), menu.X+10, footerY+6, colorDim)
}

func (g *Game) drawNativeArrangement(screen *ebiten.Image, layout nativeMusicLayout) {
	fillRect(screen, layout.Arrangement, color.RGBA{R: 24, G: 27, B: 63, A: 255})
	accent := color.Color(colorSectionBorder)
	if g.nativeFocus == nativeFocusArrangement {
		accent = colorSelection
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
			stroke = colorSelection
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
