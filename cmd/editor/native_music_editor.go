package main

import (
	"fmt"
	"image/color"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/performance"
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

const (
	nativePresetMenuHeaderHeight = 30
	nativePresetMenuRowHeight    = 27
	nativePresetMenuFooterHeight = 24
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

// nativePianoGridLineStyle keeps the time hierarchy legible at a glance:
// subdivisions are quiet, beats are stronger, and a bar owns a distinct
// two-pixel boundary. The piano roll currently pages one bar at a time, so
// both edges of the viewport are bar boundaries.
func nativePianoGridLineStyle(column, barSteps, stepsPerBeat int) (color.RGBA, int) {
	barSteps = max(1, barSteps)
	stepsPerBeat = max(1, stepsPerBeat)
	if column%barSteps == 0 {
		return color.RGBA{R: 164, G: 158, B: 196, A: 255}, 2
	}
	if column%stepsPerBeat == 0 {
		return colorBorder, 1
	}
	if column%max(1, stepsPerBeat/2) == 0 {
		return color.RGBA{R: 85, G: 89, B: 143, A: 255}, 1
	}
	return color.RGBA{R: 63, G: 67, B: 119, A: 255}, 1
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
	var ticksPerBar int
	bars, beats, unit, ticksPerBar = editor.NativePartMeter(part)
	barSteps = g.model.NativePartBarStepCount(g.nativeSequence)
	partSteps = g.model.NativePartStepCount(g.nativeSequence)
	if g.nativeCaptureVisible && g.nativeCaptureSequence == g.nativeSequence {
		captureBars := g.nativeCaptureTick/max(1, ticksPerBar) + 1
		bars = max(bars, captureBars)
		partSteps = max(partSteps, bars*barSteps)
	}
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
		return g.model.SetNativeSFXNoteTicks(g.nativeSFXID, startTick, note, durationTicks, soundID)
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

func nativePresetMenuVisibleCount(layout nativeMusicLayout, total int) int {
	available := layout.Inspector.Y + layout.Inspector.H - layout.InstrumentPreset.Y - layout.InstrumentPreset.H - 12
	rows := (available - nativePresetMenuHeaderHeight - nativePresetMenuFooterHeight) / nativePresetMenuRowHeight
	return min(total, max(4, min(12, rows)))
}

func nativePresetMenuRect(layout nativeMusicLayout, total int) editor.Rect {
	visible := nativePresetMenuVisibleCount(layout, total)
	width := max(360, layout.Inspector.W)
	x := max(layout.Main.X+8, layout.Inspector.X+layout.Inspector.W-width)
	return editor.Rect{
		X: x, Y: layout.InstrumentPreset.Y + layout.InstrumentPreset.H + 4, W: width,
		H: nativePresetMenuHeaderHeight + visible*nativePresetMenuRowHeight + nativePresetMenuFooterHeight,
	}
}

func nativePresetMenuItemRect(layout nativeMusicLayout, total, visibleIndex int) editor.Rect {
	menu := nativePresetMenuRect(layout, total)
	return editor.Rect{
		X: menu.X + 6, Y: menu.Y + nativePresetMenuHeaderHeight + visibleIndex*nativePresetMenuRowHeight,
		W: menu.W - 12, H: nativePresetMenuRowHeight - 2,
	}
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
