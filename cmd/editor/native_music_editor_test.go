package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/midicontrol"
	"github.com/drpaneas/mtile/internal/performance"
	"github.com/drpaneas/mtile/runtime/hw"
)

func newNativeMusicTestGame(t *testing.T) *Game {
	t.Helper()
	dir := t.TempDir()
	if err := assetsync.SaveSpritesheet(dir, &assetsync.Spritesheet{Width: 16, Height: 16}); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveMap(dir, &assetsync.Map{Width: 2, Height: 2, Cells: make([]int, 4)}); err != nil {
		t.Fatal(err)
	}
	settings := assetsync.DefaultNativeAudioSettings()
	sounds := assetsync.DefaultNativeSoundLibrary()
	music := assetsync.DefaultNativeMusicLibrary()
	if err := assetsync.SaveNativeAudioSettings(dir, &settings); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveNativeSounds(dir, &sounds); err != nil {
		t.Fatal(err)
	}
	if err := assetsync.SaveNativeMusic(dir, sounds, &music); err != nil {
		t.Fatal(err)
	}
	writeTestEntry(t, dir)
	g, err := newGame(dir)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestNativeMIDICommandsDriveSameModeAndLinkedHardwareParameters(t *testing.T) {
	g := newNativeMusicTestGame(t)
	g.applyNativeMIDICommand(midicontrol.Command{Kind: midicontrol.CommandMode, Mode: performance.ModePoly3})
	g.nativeSession.Arm(true, 0)
	g.applyNativeMIDICommand(midicontrol.Command{
		Kind: midicontrol.CommandParameterDelta, Parameter: performance.ParameterDuty, Delta: 1,
	})
	bank := g.nativeSession.SoundBank()
	if g.nativeVoiceMode != performance.ModePoly3 || bank.Channels[0].Pulse1.Pulse.Duty != hw.PulseDuty75 || bank.Channels[1].Pulse2.Duty != hw.PulseDuty75 {
		t.Fatalf("MIDI linked duty = mode %v CH1 %v CH2 %v", g.nativeVoiceMode, bank.Channels[0].Pulse1.Pulse.Duty, bank.Channels[1].Pulse2.Duty)
	}
	wave := performance.PulseWaveform(75)
	for index := range wave {
		if bank.Channels[2].Wave.Wave.Samples[index] != wave[index] {
			t.Fatalf("MIDI linked CH3 wave sample %d = %d, want %d", index, bank.Channels[2].Wave.Wave.Samples[index], wave[index])
		}
	}
	g.applyNativeMIDICommand(midicontrol.Command{
		Kind: midicontrol.CommandParameterDelta, Parameter: performance.ParameterEnvelopeDirection, Delta: 1,
	})
	bank = g.nativeSession.SoundBank()
	if bank.Channels[0].Pulse1.Pulse.Envelope.Direction != hw.EnvelopeUp || bank.Channels[1].Pulse2.Envelope.Direction != hw.EnvelopeUp {
		t.Fatalf("MIDI linked envelope direction = CH1 %v CH2 %v", bank.Channels[0].Pulse1.Pulse.Envelope.Direction, bank.Channels[1].Pulse2.Envelope.Direction)
	}
}

func TestNativeControllerShowsRecordingDuringCountIn(t *testing.T) {
	g := newNativeMusicTestGame(t)
	g.nativeSession.Arm(true, 0)
	if !g.nativeConductor.Start(performance.CaptureOptions{
		Clock: performance.NewTrackerClock(120, editor.NativePartTicks), CountInBars: 2,
	}) {
		t.Fatal("count-in did not start")
	}
	if state := g.nativeControllerState(); !state.Armed || !state.Recording {
		t.Fatalf("controller state during count-in = %#v", state)
	}
	_, _ = g.nativeConductor.Stop()
}

func TestNativeRecordingIsVisibleAndContinuesPastLoopingPart(t *testing.T) {
	g := newNativeMusicTestGame(t)
	if err := g.model.SetNativePartMeter(0, 1, 1, 32); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetNativePartTempo(0, 400); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetNativeSongLoop(true); err != nil {
		t.Fatal(err)
	}
	g.nativeSession.Arm(true, g.nativeSession.Now())
	g.toggleNativeRecording()
	g.nativeSession.Handle(performance.PerformanceEvent{At: g.nativeSession.Now(), Kind: performance.NoteOn, Note: 60})
	time.Sleep(30 * time.Millisecond) // more than two 12-tick bars at 400 BPM
	g.updateNativeCapture()

	snapshot := g.nativeConductor.Snapshot()
	if !snapshot.Recording || snapshot.Tick <= 12 {
		t.Fatalf("recording stopped at looping part boundary: %#v", snapshot)
	}
	if !g.nativeCaptureVisible || len(g.nativeCaptureNotes) != 1 || g.nativeBar < 1 {
		t.Fatalf("live recording feedback = visible %t notes %#v bar %d", g.nativeCaptureVisible, g.nativeCaptureNotes, g.nativeBar)
	}
	if takes := g.model.NativeTakesAt(0); len(takes) != 0 {
		t.Fatalf("recording committed before STOP: %#v", takes)
	}

	g.nativeSession.Handle(performance.PerformanceEvent{At: g.nativeSession.Now(), Kind: performance.NoteOff, Note: 60})
	g.toggleNativeRecording()
	part, ok := g.model.NativePartAt(0)
	if !ok || part.Bars < 2 || len(g.model.NativeTakesAt(0)) != 1 {
		t.Fatalf("STOP did not commit the continuous take: part %#v ok=%t takes=%#v", part, ok, g.model.NativeTakesAt(0))
	}
}

func TestNativeDeleteTargetsNoteOrArrangementLaneByFocus(t *testing.T) {
	g := newNativeMusicTestGame(t)
	if err := g.model.SetNativeNote(0, 0, 0, 60); err != nil {
		t.Fatal(err)
	}
	if err := g.model.SetNativeNote(0, 0, 1, 64); err != nil {
		t.Fatal(err)
	}

	g.nativeFocus = nativeFocusComposer
	g.nativeSelectedStep = 1
	g.deleteNativeSelection()
	clip, ok := g.model.NativeClipAt(0, 0)
	if !ok || len(clip.Events) != 1 || clip.Events[0].Note != 60 {
		t.Fatalf("composer delete changed the wrong scope: clip=%#v ok=%t", clip, ok)
	}

	g.nativeFocus = nativeFocusArrangement
	g.deleteNativeSelection()
	if _, ok := g.model.NativeClipAt(0, 0); ok {
		t.Fatal("arrangement delete left the selected channel clip assigned")
	}
}

func TestNativeNoteLengthLabelsPresetsAndSelectedNoteResize(t *testing.T) {
	for ticks, want := range map[int]string{6: "1/64", 8: "1/32T", 12: "1/32", 16: "1/16T", 18: "1/32.", 32: "1/8T", 72: "1/8.", 144: "1/4.", 384: "4 BEATS", 5: "5 TICKS"} {
		if got := nativeNoteLengthLabel(ticks); got != want {
			t.Errorf("nativeNoteLengthLabel(%d) = %q, want %q", ticks, got, want)
		}
	}
	if got := nextNativeNoteLength(17, -1); got != 16 {
		t.Errorf("previous length from 17 = %d, want 16", got)
	}
	if got := nextNativeNoteLength(17, 1); got != 18 {
		t.Errorf("next length from 17 = %d, want 18", got)
	}

	g := newNativeMusicTestGame(t)
	if g.nativeNoteLengthTicks != 12 || nativeNoteLengthLabel(g.nativeNoteLengthTicks) != "1/32" {
		t.Fatalf("beginner note-length default = %d ticks (%s), want 12 (1/32)", g.nativeNoteLengthTicks, nativeNoteLengthLabel(g.nativeNoteLengthTicks))
	}
	if err := g.model.SetNativeNoteDuration(0, 0, 0, 60, 4); err != nil {
		t.Fatal(err)
	}
	g.nativeSelectedStep = 0
	g.nativeNoteLength = 4
	g.nativeNoteLengthTicks = 24
	g.nudgeNativeNoteLength(-1)
	clip, ok := g.model.NativeClipAt(0, 0)
	if !ok || len(clip.Events) != 1 || clip.Events[0].DurationTicks != 18 || g.nativeNoteLengthTicks != 18 {
		t.Fatalf("selected dotted note resize = clip=%#v ok=%t length=%d", clip, ok, g.nativeNoteLengthTicks)
	}
}

func TestNativePianoRollSnapIncludesTupletsAndSwingBoundaries(t *testing.T) {
	got := performance.QuantizeSixtyFourth
	want := []performance.Quantize{
		performance.QuantizeThirtySecond, performance.QuantizeSixteenth, performance.QuantizeEighth,
		performance.QuantizeQuarter, performance.QuantizeSixteenthTriplet,
		performance.QuantizeEighthTriplet, performance.QuantizeQuarterTriplet, performance.QuantizeOff,
		performance.QuantizeSixtyFourth,
	}
	for index, expected := range want {
		got = nextNativeGridSnap(got)
		if got != expected {
			t.Fatalf("snap cycle %d = %s, want %s", index, got, expected)
		}
	}
	triplets := nativeSnapBoundaries(0, 96, performance.QuantizeEighthTriplet, 50)
	if fmt.Sprint(triplets) != "[0 32 64 96]" {
		t.Fatalf("triplet boundaries = %v", triplets)
	}
	swung := nativeSnapBoundaries(0, 96, performance.QuantizeEighth, 66)
	if fmt.Sprint(swung) != "[0 63 96]" {
		t.Fatalf("swing boundaries = %v", swung)
	}
}

func TestNativeAdvancedOutputControlsEditRealPSGSettings(t *testing.T) {
	g := newNativeMusicTestGame(t)
	g.nudgeNativeOutput(0, -1)
	g.nudgeNativeOutput(1, -1)
	g.nudgeNativeOutput(2, -1)
	settings := g.model.NativeAudio.Settings
	if settings.LeftLevel != 6 || settings.RightLevel != 6 || settings.PSGRatio != 50 {
		t.Fatalf("advanced PSG output controls = %+v", settings)
	}
	g.nudgeNativeOutput(2, -1)
	if got := g.model.NativeAudio.Settings.PSGRatio; got != 25 {
		t.Fatalf("second hardware mix step = %d, want 25", got)
	}
}

func TestNativeSFXWorkflowUsesSameComposerAndStableRuntimeID(t *testing.T) {
	g := newNativeMusicTestGame(t)
	g.createNativeSFX(2)
	if !g.nativeSFXMode || g.nativeSFXID < 0 || g.nativeChannel != 2 {
		t.Fatalf("SFX selection = mode %t id %d channel %d", g.nativeSFXMode, g.nativeSFXID, g.nativeChannel)
	}
	if err := g.nativeSetNote(0, 60, 4, g.nativeInstrument[2]); err != nil {
		t.Fatal(err)
	}
	clip, ok := g.nativeCurrentClip()
	if !ok || clip.ID != g.nativeSFXID || clip.Kind != "ch3" || len(clip.Events) != 1 {
		t.Fatalf("shared composer effect = %#v ok=%t", clip, ok)
	}
	g.setNativeSFXMode(false)
	if g.nativeSFXMode {
		t.Fatal("MUSIC workflow did not restore")
	}
}

func TestNativePerformanceUsesSelectedReusableInstrument(t *testing.T) {
	g := newNativeMusicTestGame(t)
	first, err := g.model.CreateNativeSound(0, -1, "First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := g.model.CreateNativeSound(0, first, "Second")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.model.UpdateNativeSoundByID(second, func(sound *assetsync.NativeSoundAsset) {
		sound.Pulse.Duty = 75
	}); err != nil {
		t.Fatal(err)
	}
	g.nativeInstrument[0] = second
	bank := g.nativePerformanceSoundBank()
	if bank.Channels[0].ID != uint8(second) || bank.Channels[0].Pulse1.Pulse.Duty != hw.PulseDuty75 {
		t.Fatalf("live bank ignored selected instrument: id=%d duty=%v", bank.Channels[0].ID, bank.Channels[0].Pulse1.Pulse.Duty)
	}
}

func TestNativePerformanceSynchronizesInstrumentSelectedWithoutEdit(t *testing.T) {
	g := newNativeMusicTestGame(t)
	first, err := g.model.CreateNativeSound(0, -1, "First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := g.model.CreateNativeSound(0, first, "Second")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.model.UpdateNativeSoundByID(second, func(sound *assetsync.NativeSoundAsset) {
		sound.Pulse.Duty = 75
	}); err != nil {
		t.Fatal(err)
	}

	// This is the path used when selection lands on an existing note: the UI
	// changes instrument ID, but there is no edit action to refresh the bank.
	g.nativeInstrument[0] = second
	g.syncNativePerformanceSounds()
	bank := g.nativeSession.SoundBank()
	if bank.Channels[0].ID != uint8(second) || bank.Channels[0].Pulse1.Pulse.Duty != hw.PulseDuty75 {
		t.Fatalf("armed MIDI bank stayed stale: id=%d duty=%v", bank.Channels[0].ID, bank.Channels[0].Pulse1.Pulse.Duty)
	}
}

func TestNativePerformanceKeepsSelectedCH4Instrument(t *testing.T) {
	g := newNativeMusicTestGame(t)
	id, _, err := g.model.EnsureNativeSoundPreset(3, 12) // Explosion
	if err != nil {
		t.Fatal(err)
	}
	g.nativeInstrument[3] = id
	bank := g.nativePerformanceSoundBank()
	if bank.Channels[3].ID != uint8(id) || bank.Channels[3].Kind != 3 {
		t.Fatalf("selected CH4 instrument was replaced: id=%d kind=%d", bank.Channels[3].ID, bank.Channels[3].Kind)
	}
}

func TestNativePerformanceMapsAuthoredGBStudioDrumsToLaunchkeyPads(t *testing.T) {
	g := newNativeMusicTestGame(t)
	bass, _, err := g.model.EnsureNativeSoundPreset(3, 10)
	if err != nil {
		t.Fatal(err)
	}
	snare, _, err := g.model.EnsureNativeSoundPreset(3, 6)
	if err != nil {
		t.Fatal(err)
	}
	bank := g.nativePerformanceSoundBank()
	if bank.Noise[0].ID != uint8(bass) || bank.Noise[1].ID != uint8(snare) {
		t.Fatalf("Launchkey drum map = bass %d snare %d, want %d/%d", bank.Noise[0].ID, bank.Noise[1].ID, bass, snare)
	}
}

func TestNativeMusicLayoutKeepsComposerInspectorAndOrderVisible(t *testing.T) {
	layout := newNativeMusicLayout()
	for name, rect := range map[string]struct{ x, y, w, h int }{
		"grid":        {layout.Grid.X, layout.Grid.Y, layout.Grid.W, layout.Grid.H},
		"inspector":   {layout.Inspector.X, layout.Inspector.Y, layout.Inspector.W, layout.Inspector.H},
		"arrangement": {layout.Arrangement.X, layout.Arrangement.Y, layout.Arrangement.W, layout.Arrangement.H},
		"take use":    {layout.TakeUse.X, layout.TakeUse.Y, layout.TakeUse.W, layout.TakeUse.H},
	} {
		if rect.w <= 0 || rect.h <= 0 || rect.x < 0 || rect.y < editorTabsHeight || rect.x+rect.w > screenWidth || rect.y+rect.h > statusBarRect.Y {
			t.Fatalf("%s is outside the single Music Lab workspace: %#v", name, rect)
		}
	}
	if layout.Grid.Y+layout.Grid.H > layout.Arrangement.Y || layout.Inspector.Y+layout.Inspector.H > layout.Arrangement.Y {
		t.Fatalf("composer/inspector overlaps order strip: grid=%#v inspector=%#v order=%#v", layout.Grid, layout.Inspector, layout.Arrangement)
	}
}

func TestNativePresetMenuStaysInsideWorkspaceAndRowsDoNotOverlap(t *testing.T) {
	layout := newNativeMusicLayout()
	for channel := 0; channel < 4; channel++ {
		presets := editor.NativeSoundPresets(channel)
		menu := nativePresetMenuRect(layout, len(presets))
		if menu.X < layout.Main.X || menu.Y <= layout.InstrumentPreset.Y || menu.X+menu.W > layout.Main.X+layout.Main.W || menu.Y+menu.H > layout.Arrangement.Y {
			t.Fatalf("CH%d preset menu escapes Music Lab: menu=%#v main=%#v arrangement=%#v", channel+1, menu, layout.Main, layout.Arrangement)
		}
		visible := nativePresetMenuVisibleCount(layout, len(presets))
		for row := 1; row < visible; row++ {
			previous := nativePresetMenuItemRect(layout, len(presets), row-1)
			current := nativePresetMenuItemRect(layout, len(presets), row)
			if previous.Y+previous.H > current.Y {
				t.Fatalf("CH%d preset rows %d and %d overlap: %#v %#v", channel+1, row, row+1, previous, current)
			}
		}
	}
}

func TestNativePianoShowsMoreOctavesAsWorkspaceGrows(t *testing.T) {
	if got := nativePianoRows(editor.Rect{H: 500}, 0); got != 36 {
		t.Fatalf("desktop piano rows = %d, want 36 (three octaves)", got)
	}
	if got := nativePianoRows(editor.Rect{H: 900}, 0); got != 48 {
		t.Fatalf("2K piano rows = %d, want 48 (four octaves)", got)
	}
	if got := nativePianoRows(editor.Rect{H: 1600}, 0); got != 60 {
		t.Fatalf("4K piano rows = %d, want 60 (five octaves)", got)
	}
	if got := nativePianoRows(editor.Rect{H: 1600}, 3); got != 8 {
		t.Fatalf("noise rows = %d, want the eight drum lanes", got)
	}
}

func TestNativePitchNavigationUsesRealChannelRange(t *testing.T) {
	g := newNativeMusicTestGame(t)
	layout := newNativeMusicLayout()
	g.nativeChannel = 0
	g.nativeOctave = 0
	g.clampNativeOctave(layout)
	minimum, maximum := g.nativeOctaveBounds(layout)
	if minimum < 2 || maximum != 8 || g.nativeOctave != minimum {
		t.Fatalf("CH1 octave bounds/default clamp = %d..%d octave=%d", minimum, maximum, g.nativeOctave)
	}
	if g.nativePitchValid(35) || !g.nativePitchValid(36) || !g.nativePitchValid(127) {
		t.Fatalf("CH1 pitch validation did not match 36..127")
	}
	g.nativeOctave = 8
	if note := (g.nativeOctave+3)*12 - 1 - 4; note != 127 || !g.nativePitchValid(note) {
		t.Fatalf("highest legal MIDI note is not reachable: %d", note)
	}
}

func TestEnteringMusicLabInitializesGraphicsOnlyProjectAsV2(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	if g.model.AudioVersion == assetsync.NativeAudioSchemaVersion {
		t.Fatal("fixture unexpectedly started with authored native audio")
	}
	g.setMode(modeMusic)
	if g.mode != modeMusic || g.model.AudioVersion != assetsync.NativeAudioSchemaVersion {
		t.Fatalf("Music Lab mode/version = %v/%d", g.mode, g.model.AudioVersion)
	}
	if len(g.undo.past) != 1 || g.undo.past[0].NativeAudio.Settings.Version != assetsync.NativeAudioSchemaVersion {
		t.Fatalf("native undo baseline = %#v", g.undo.past)
	}
}

func TestEnteringMusicLabRejectsAuthoredLegacyAudio(t *testing.T) {
	g := newTestGame(t, 16, 16, 2, 2)
	if _, err := g.model.AddSFX(); err != nil {
		t.Fatal(err)
	}
	g.setMode(modeMusic)
	if g.mode == modeMusic || g.model.AudioVersion == assetsync.NativeAudioSchemaVersion {
		t.Fatalf("legacy project entered native Music Lab: mode=%v version=%d", g.mode, g.model.AudioVersion)
	}
	if g.statusMsg == "" {
		t.Fatal("legacy project did not explain why Music Lab was unavailable")
	}
}
