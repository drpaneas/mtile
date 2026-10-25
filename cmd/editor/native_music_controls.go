package main

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/performance"
	"github.com/hajimehoshi/ebiten/v2"
)

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
				sound.Noise.Shift = min(max(sound.Noise.Shift+delta, 0), 15)
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
