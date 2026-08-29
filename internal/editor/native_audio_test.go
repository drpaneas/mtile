package editor

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/performance"
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

func nativeEditorFixture(t *testing.T) *Model {
	t.Helper()
	dir := editorFixture(t)
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
	model, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return model
}

func TestNativeMusicPreviewIsFiniteDeterministicAndTracksSteps(t *testing.T) {
	m := nativeEditorFixture(t)
	for step, note := range []int{60, 64, 67, 72} {
		if err := m.SetNativeNote(0, 0, step, note); err != nil {
			t.Fatal(err)
		}
	}
	song, _ := m.NativeSong()
	first, err := RenderNativeMusicPreview(m.NativeAudio.Library, song.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderNativeMusicPreview(m.NativeAudio.Library, song.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Samples) != NativePreviewSampleRate*2 || !slices.Equal(first.Samples, second.Samples) {
		t.Fatalf("preview samples = %d deterministic=%t", len(first.Samples), slices.Equal(first.Samples, second.Samples))
	}
	nonzero := false
	for _, sample := range first.Samples {
		if sample != 0 {
			nonzero = true
			break
		}
	}
	if !nonzero {
		t.Fatal("native preview is silent")
	}
	sequence, step, ok := first.Position(NativePreviewSampleRate / 2)
	if !ok || sequence != 0 || step != 16 {
		t.Fatalf("position = section %d step %d ok=%t", sequence, step, ok)
	}
}

func TestNativeMusicPreviewMuteAndSoloMasksAreMonitorOnly(t *testing.T) {
	m := nativeEditorFixture(t)
	for channel, note := range []int{60, 64, 48} {
		if err := m.SetNativeNote(0, channel, channel, note); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.SetNativeNoiseHit(0, 3, 1); err != nil {
		t.Fatal(err)
	}
	song, _ := m.NativeSong()
	full, err := RenderNativeMusicPreviewMute(m.NativeAudio.Library, song.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	ch1OnlyMask := NativeMonitorMuteMask(0, 0x01)
	ch1Only, err := RenderNativeMusicPreviewMute(m.NativeAudio.Library, song.ID, ch1OnlyMask)
	if err != nil {
		t.Fatal(err)
	}
	muted, err := RenderNativeMusicPreviewMute(m.NativeAudio.Library, song.ID, 0x0f)
	if err != nil {
		t.Fatal(err)
	}
	if full.MuteMask != 0 || ch1Only.MuteMask != 0x0e || slices.Equal(full.Samples, ch1Only.Samples) {
		t.Fatalf("monitor masks full=%#x solo-CH1=%#x changed=%t", full.MuteMask, ch1Only.MuteMask, !slices.Equal(full.Samples, ch1Only.Samples))
	}
	if !slices.ContainsFunc(ch1Only.Samples, func(sample int8) bool { return sample != 0 }) {
		t.Fatal("soloed CH1 preview is silent")
	}
	if slices.ContainsFunc(muted.Samples, func(sample int8) bool { return sample != 0 }) {
		t.Fatal("all-muted preview is audible")
	}
	if got := NativeMonitorMuteMask(0x01, 0x03); got != 0x0d {
		t.Fatalf("explicit mute plus CH1/CH2 solo mask = %#x, want 0x0d", got)
	}
	if got := NativeMonitorMuteMask(0x10, 0); got != 0 {
		t.Fatalf("out-of-range monitor bits survived: %#x", got)
	}
}

func TestNativeMusicPreviewHonorsStereoRoutesAndMasterLevels(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNote(0, 0, 0, 60); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateNativeSoundAtStep(0, 0, 0, func(sound *assetsync.NativeSoundAsset) {
		sound.Route = "left"
	}); err != nil {
		t.Fatal(err)
	}
	song, _ := m.NativeSong()
	preview, err := RenderNativeMusicPreviewSettings(m.NativeAudio.Library, song.ID, 0, assetsync.NativeAudioSettings{
		Version: assetsync.NativeAudioSchemaVersion, Profile: assetsync.NativeAudioProfile,
		LeftLevel: 7, RightLevel: 7, PSGRatio: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(preview.LeftSamples, func(sample int8) bool { return sample != 0 }) {
		t.Fatal("left-routed note is silent on the left")
	}
	if slices.ContainsFunc(preview.RightSamples, func(sample int8) bool { return sample != 0 }) {
		t.Fatal("left-routed note leaked into the right terminal")
	}
	quiet, err := RenderNativeMusicPreviewSettings(m.NativeAudio.Library, song.ID, 0, assetsync.NativeAudioSettings{
		Version: assetsync.NativeAudioSchemaVersion, Profile: assetsync.NativeAudioProfile,
		LeftLevel: 0, RightLevel: 7, PSGRatio: 25,
	})
	if err != nil {
		t.Fatal(err)
	}
	if quiet.AudioStats().Peak >= preview.AudioStats().Peak {
		t.Fatalf("master level/ratio did not reduce preview: quiet=%d full=%d", quiet.AudioStats().Peak, preview.AudioStats().Peak)
	}
}

func TestNativeOutputSettingsAreTransactionalHardwareControls(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeOutput(3, 5, 50); err != nil {
		t.Fatal(err)
	}
	if got := m.NativeAudio.Settings; got.LeftLevel != 3 || got.RightLevel != 5 || got.PSGRatio != 50 {
		t.Fatalf("PSG output settings = %+v", got)
	}
	if err := m.SetNativeOutput(8, 5, 50); err == nil {
		t.Fatal("invalid PSG terminal level was accepted")
	}
	if got := m.NativeAudio.Settings; got.LeftLevel != 3 || got.RightLevel != 5 || got.PSGRatio != 50 {
		t.Fatalf("rejected output edit changed settings: %+v", got)
	}
	if err := m.SetNativeOutput(3, 5, 75); err == nil {
		t.Fatal("non-hardware PSG ratio was accepted")
	}
}

func TestNativeMusicPreviewExportsAudiblePCM16WAV(t *testing.T) {
	m := nativeEditorFixture(t)
	for step, note := range []int{60, 64, 67, 72} {
		if err := m.SetNativeNote(0, step%3, step*2, note); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.SetNativeNoiseHit(0, 0, 0); err != nil {
		t.Fatal(err)
	}
	song, _ := m.NativeSong()
	preview, err := RenderNativeMusicPreview(m.NativeAudio.Library, song.ID)
	if err != nil {
		t.Fatal(err)
	}
	stats := preview.AudioStats()
	if stats.Frames != len(preview.Samples) || stats.Duration <= 0 || stats.Peak == 0 || stats.RMS <= 0 {
		t.Fatalf("preview stats = %+v", stats)
	}
	wav, err := RenderNativeMusicPreviewWAV(preview)
	if err != nil {
		t.Fatal(err)
	}
	if len(wav) != 44+len(preview.Samples)*2 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatalf("WAV header/size = %q/%d", wav[:12], len(wav))
	}
	if got := binary.LittleEndian.Uint32(wav[24:28]); got != NativePreviewSampleRate {
		t.Fatalf("WAV sample rate = %d", got)
	}
	if got := binary.LittleEndian.Uint16(wav[22:24]); got != 1 {
		t.Fatalf("WAV channels = %d", got)
	}
	nonzeroPCM := false
	for index := 44; index < len(wav); index++ {
		if wav[index] != 0 {
			nonzeroPCM = true
			break
		}
	}
	if !nonzeroPCM {
		t.Fatal("WAV payload is silent")
	}
}

func TestNativeMusicPreviewReportsSilentEmptySong(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.EnsureNativeClip(0, 0); err != nil {
		t.Fatal(err)
	}
	song, _ := m.NativeSong()
	preview, err := RenderNativeMusicPreview(m.NativeAudio.Library, song.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats := preview.AudioStats(); stats.Peak != 0 || stats.RMS != 0 {
		t.Fatalf("empty song stats = %+v", stats)
	}
}

func TestNativeNotePreviewAuditionsEveryHardwareChannel(t *testing.T) {
	m := nativeEditorFixture(t)
	for channel, note := range []int{60, 64, 48} {
		if err := m.SetNativeNote(0, channel, channel, note); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.SetNativeNoiseHit(0, 3, 1); err != nil {
		t.Fatal(err)
	}

	for channel, note := range []int{60, 64, 48, 37} {
		sound, ok := m.NativeSoundAtStep(0, channel, channel)
		if !ok {
			t.Fatalf("CH%d has no sound", channel+1)
		}
		first, err := RenderNativeNotePreview(m.NativeAudio.Library, sound.ID, note)
		if err != nil {
			t.Fatalf("CH%d preview: %v", channel+1, err)
		}
		second, err := RenderNativeNotePreview(m.NativeAudio.Library, sound.ID, note)
		if err != nil {
			t.Fatalf("CH%d second preview: %v", channel+1, err)
		}
		if len(first) != NativeNotePreviewSamples || !slices.Equal(first, second) {
			t.Fatalf("CH%d preview length=%d deterministic=%t", channel+1, len(first), slices.Equal(first, second))
		}
		if !slices.ContainsFunc(first, func(sample int8) bool { return sample != 0 }) {
			t.Fatalf("CH%d audition is silent", channel+1)
		}
	}
}

func TestNativeNotePreviewRejectsMissingSoundAndInvalidPitch(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNote(0, 0, 0, 60); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderNativeNotePreview(m.NativeAudio.Library, 255, 60); err == nil {
		t.Fatal("missing sound preview succeeded")
	}
	sound, ok := m.NativeSoundAtStep(0, 0, 0)
	if !ok {
		t.Fatal("placed note has no sound")
	}
	if _, err := RenderNativeNotePreview(m.NativeAudio.Library, sound.ID, 128); err == nil {
		t.Fatal("out-of-range pitch preview succeeded")
	}
}

func TestNativeDragCellsPaintsCrossedStepsWithoutRepeatingTheCurrentCell(t *testing.T) {
	if got := NativeDragCells(-1, -1, 2, 4); !slices.Equal(got, []NativeGridCell{{Step: 2, Row: 4}}) {
		t.Fatalf("initial drag cells = %#v", got)
	}
	if got := NativeDragCells(2, 4, 2, 4); got != nil {
		t.Fatalf("stationary drag cells = %#v, want nil", got)
	}
	if got := NativeDragCells(2, 4, 2, 7); !slices.Equal(got, []NativeGridCell{{Step: 2, Row: 7}}) {
		t.Fatalf("vertical drag cells = %#v", got)
	}
	wantForward := []NativeGridCell{{Step: 3, Row: 5}, {Step: 4, Row: 6}, {Step: 5, Row: 7}}
	if got := NativeDragCells(2, 4, 5, 7); !slices.Equal(got, wantForward) {
		t.Fatalf("forward drag cells = %#v, want %#v", got, wantForward)
	}
	wantBackward := []NativeGridCell{{Step: 4, Row: 6}, {Step: 3, Row: 5}, {Step: 2, Row: 4}}
	if got := NativeDragCells(5, 7, 2, 4); !slices.Equal(got, wantBackward) {
		t.Fatalf("backward drag cells = %#v, want %#v", got, wantBackward)
	}
}

func TestNativeEditorFirstNoteCreatesHardwareSongGraph(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNote(0, 0, 3, 60); err != nil {
		t.Fatal(err)
	}
	if len(m.NativeAudio.Sounds.Sounds) != 1 || m.NativeAudio.Sounds.Sounds[0].Kind != "ch1" || m.NativeAudio.Sounds.Sounds[0].Pulse.Sweep == nil {
		t.Fatalf("CH1 sound = %#v", m.NativeAudio.Sounds.Sounds)
	}
	clip, ok := m.NativeClipAt(0, 0)
	if !ok || clip.Kind != "ch1" || len(clip.Events) != 1 {
		t.Fatalf("CH1 clip = %#v ok=%t", clip, ok)
	}
	event := clip.Events[0]
	if event.Tick != 3*NativeTicksPerStep || event.DurationTicks != NativeTicksPerStep || event.Note != 60 {
		t.Fatalf("note event = %#v", event)
	}
	song, ok := m.NativeSong()
	if !ok || song.Tempo != 120 || len(song.Sequence) != 1 {
		t.Fatalf("song = %#v ok=%t", song, ok)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(m.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reloaded.NativeClipAt(0, 0); !ok || len(got.Events) != 1 || got.Events[0] != event {
		t.Fatalf("reloaded clip = %#v ok=%t", got, ok)
	}
}

func TestNativeNoteDurationsSelectResizeReplaceAndDeleteSustainedNotes(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNoteDuration(0, 0, 0, 60, 4); err != nil {
		t.Fatal(err)
	}
	event, start, ok := m.NativeNoteEventAtStep(0, 0, 2)
	if !ok || start != 0 || event.Note != 60 || event.DurationTicks != 4*NativeTicksPerStep {
		t.Fatalf("sustained note at step 2 = %#v start=%d ok=%t", event, start, ok)
	}

	// A new note owns its range and trims the sustained note before it.
	if err := m.SetNativeNoteDuration(0, 0, 2, 64, 2); err != nil {
		t.Fatal(err)
	}
	clip, _ := m.NativeClipAt(0, 0)
	if len(clip.Events) != 2 || clip.Events[0].DurationTicks != 2*NativeTicksPerStep ||
		clip.Events[1].Tick != 2*NativeTicksPerStep || clip.Events[1].DurationTicks != 2*NativeTicksPerStep {
		t.Fatalf("replacement events = %#v", clip.Events)
	}

	// Growing into the following note clamps at its start rather than creating
	// an invalid monophonic overlap.
	accepted, err := m.ResizeNativeNote(0, 0, 0, 8)
	if err != nil || accepted != 2 {
		t.Fatalf("resize accepted=%d err=%v", accepted, err)
	}
	clip, _ = m.NativeClipAt(0, 0)
	if clip.Events[0].DurationTicks != 2*NativeTicksPerStep {
		t.Fatalf("clamped first note = %#v", clip.Events[0])
	}

	// Deleting any cell covered by a sustained note removes that note.
	if err := m.DeleteNativeNote(0, 0, 3); err != nil {
		t.Fatal(err)
	}
	clip, _ = m.NativeClipAt(0, 0)
	if len(clip.Events) != 1 || clip.Events[0].Note != 60 {
		t.Fatalf("delete inside sustained note left %#v", clip.Events)
	}
}

func TestNativeNoteDurationClampsAtEndOfPart(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNoteDuration(0, 2, 60, 48, 8); err != nil {
		t.Fatal(err)
	}
	clip, _ := m.NativeClipAt(0, 2)
	if len(clip.Events) != 1 || clip.Events[0].Tick != 60*NativeTicksPerStep || clip.Events[0].DurationTicks != 4*NativeTicksPerStep {
		t.Fatalf("end-clamped note = %#v", clip.Events)
	}
}

func TestNativeSixtyFourthGridPreservesExistingAbsoluteTiming(t *testing.T) {
	if NativeTicksPerStep != 6 || NativeStepsPerPart != 64 || NativePartTicks != audio.NativePPQN*4 {
		t.Fatalf("1/64 grid = %d steps x %d ticks, part=%d", NativeStepsPerPart, NativeTicksPerStep, NativePartTicks)
	}
	m := nativeEditorFixture(t)
	if err := m.SetNativeNoteDuration(0, 0, 2, 60, 4); err != nil {
		t.Fatal(err)
	}
	clip, _ := m.NativeClipAt(0, 0)
	if clip.Events[0].Tick != 12 || clip.Events[0].DurationTicks != 24 {
		t.Fatalf("old 1/32-aligned timing changed: %#v", clip.Events[0])
	}
	event, start, ok := m.NativeNoteEventAtStep(0, 0, 5)
	if !ok || start != 2 || event.Tick != 12 || event.DurationTicks != 24 {
		t.Fatalf("existing timing is not editable on finer grid: event=%#v start=%d ok=%t", event, start, ok)
	}
}

func TestNativeExactTickAuthoringSupportsTupletsAndDottedNotes(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNoteTicksWithSound(0, 0, 32, 60, 32, -1); err != nil {
		t.Fatal(err)
	}
	clip, ok := m.NativeClipAt(0, 0)
	if !ok || len(clip.Events) != 1 || clip.Events[0].Tick != 32 || clip.Events[0].DurationTicks != 32 {
		t.Fatalf("eighth-triplet event = %#v, ok=%t", clip.Events, ok)
	}
	accepted, err := m.ResizeNativeNoteTicks(0, 0, 32, 72)
	if err != nil {
		t.Fatal(err)
	}
	clip, _ = m.NativeClipAt(0, 0)
	if accepted != 72 || clip.Events[0].DurationTicks != 72 {
		t.Fatalf("dotted-eighth resize = accepted %d event %#v", accepted, clip.Events[0])
	}
}

func TestNativeNoteRangeCopyPasteMoveAndDelete(t *testing.T) {
	m := nativeEditorFixture(t)
	for _, event := range []struct{ tick, note int }{{32, 60}, {64, 64}} {
		if err := m.SetNativeNoteTicksWithSound(0, 0, event.tick, event.note, 16, -1); err != nil {
			t.Fatal(err)
		}
	}
	clipboard := m.NativeNoteEventsInRange(0, 0, 30, 90, 60, 64)
	if len(clipboard) != 2 || clipboard[0].Tick != 0 || clipboard[1].Tick != 32 {
		t.Fatalf("relative note clipboard = %#v", clipboard)
	}
	start, end, low, high, ok := m.NativeNoteRangeBounds(0, 0, 0, 96, 50, 70)
	if !ok || start != 32 || end != 80 || low != 60 || high != 64 {
		t.Fatalf("tight range bounds = %d..%d notes %d..%d ok=%t", start, end, low, high, ok)
	}
	if err := m.PasteNativeNoteEvents(0, 0, 128, clipboard); err != nil {
		t.Fatal(err)
	}
	if err := m.MoveNativeNoteRange(0, 0, 128, 180, 60, 64, 16, 1); err != nil {
		t.Fatal(err)
	}
	clip, _ := m.NativeClipAt(0, 0)
	if len(clip.Events) != 4 || clip.Events[2].Tick != 144 || clip.Events[2].Note != 61 || clip.Events[3].Tick != 176 || clip.Events[3].Note != 65 {
		t.Fatalf("moved pasted notes = %#v", clip.Events)
	}
	if err := m.DeleteNativeNoteRange(0, 0, 140, 200, 61, 65); err != nil {
		t.Fatal(err)
	}
	clip, _ = m.NativeClipAt(0, 0)
	if len(clip.Events) != 2 || clip.Events[0].Tick != 32 || clip.Events[1].Tick != 64 {
		t.Fatalf("range delete = %#v", clip.Events)
	}
}

func TestNativeNoteRangeMoveFailureIsAtomic(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNoteTicksWithSound(0, 0, 32, 60, 16, -1); err != nil {
		t.Fatal(err)
	}
	before, _ := m.NativeClipAt(0, 0)
	if err := m.MoveNativeNoteRange(0, 0, 30, 60, 60, 60, -40, 0); err == nil {
		t.Fatal("range move before the clip unexpectedly succeeded")
	}
	after, _ := m.NativeClipAt(0, 0)
	if len(after.Events) != 1 || after.Events[0] != before.Events[0] {
		t.Fatalf("failed range move mutated clip: before=%#v after=%#v", before.Events, after.Events)
	}
	if err := m.MoveNativeNoteRange(0, 0, 30, 60, 60, 60, 0, -40); err == nil {
		t.Fatal("range move below the CH1 hardware pitch range unexpectedly succeeded")
	}
}

func TestNativePartTempoOverridesSongInPreview(t *testing.T) {
	m := nativeEditorFixture(t)
	second, err := m.AddNativePart(0)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetNativePartTempo(second, 240); err != nil {
		t.Fatal(err)
	}
	if tempo, overridden := m.NativePartTempo(second); tempo != 240 || !overridden {
		t.Fatalf("part tempo = %d overridden=%t", tempo, overridden)
	}
	song, _ := m.NativeSong()
	preview, err := RenderNativeMusicPreview(m.NativeAudio.Library, song.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Segments) != 2 || preview.Segments[0].Tempo != int(song.Tempo) || preview.Segments[1].Tempo != 240 {
		t.Fatalf("preview tempo segments = %#v", preview.Segments)
	}
	firstLength := preview.Segments[0].EndSample - preview.Segments[0].StartSample
	secondLength := preview.Segments[1].EndSample - preview.Segments[1].StartSample
	if secondLength*2 != firstLength {
		t.Fatalf("240 BPM part samples = %d, want half of %d", secondLength, firstLength)
	}
	if err := m.SetNativePartTempo(second, 0); err != nil {
		t.Fatal(err)
	}
	if tempo, overridden := m.NativePartTempo(second); tempo != song.Tempo || overridden {
		t.Fatalf("inherited part tempo = %d overridden=%t", tempo, overridden)
	}
}

func TestNativeRawSubCellNoteCanBeSelectedResizedAndDeleted(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNote(0, 0, 0, 60); err != nil {
		t.Fatal(err)
	}
	clip, _ := m.NativeClipAt(0, 0)
	clip.Events[0].Tick = 5
	clip.Events[0].DurationTicks = 3
	m.NativeAudio.Music.Clips[0] = clip
	library, err := assetsync.CompileNativeAudio(m.NativeAudio.Settings, m.NativeAudio.Sounds, m.NativeAudio.Music)
	if err != nil {
		t.Fatal(err)
	}
	m.NativeAudio.Library = library

	event, start, ok := m.NativeNoteEventAtStep(0, 0, 0)
	if !ok || start != 0 || event.Tick != 5 {
		t.Fatalf("raw note was not selectable: event=%#v start=%d ok=%t", event, start, ok)
	}
	accepted, err := m.ResizeNativeNote(0, 0, 0, 2)
	if err != nil || accepted != 2 {
		t.Fatalf("resize raw note accepted=%d err=%v", accepted, err)
	}
	clip, _ = m.NativeClipAt(0, 0)
	if clip.Events[0].Tick != 5 || clip.Events[0].DurationTicks != 2*NativeTicksPerStep {
		t.Fatalf("raw note resize lost its exact start: %#v", clip.Events[0])
	}
	if err := m.DeleteNativeNote(0, 0, 0); err != nil {
		t.Fatal(err)
	}
	clip, _ = m.NativeClipAt(0, 0)
	if len(clip.Events) != 0 {
		t.Fatalf("raw note remained after cell delete: %#v", clip.Events)
	}
}

func TestNativeMusicPreviewHonorsSustainedNoteDuration(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNoteDuration(0, 0, 0, 60, 16); err != nil {
		t.Fatal(err)
	}
	song, _ := m.NativeSong()
	preview, err := RenderNativeMusicPreview(m.NativeAudio.Library, song.ID)
	if err != nil {
		t.Fatal(err)
	}
	stepSamples := nativeTicksToSamples(NativeTicksPerStep, song.Tempo)
	fifteenthStep := preview.Samples[14*stepSamples : 15*stepSamples]
	afterNote := preview.Samples[16*stepSamples : 17*stepSamples]
	if !slices.ContainsFunc(fifteenthStep, func(sample int8) bool { return sample != 0 }) {
		t.Fatal("quarter note became silent before its sixteenth 1/64 cell")
	}
	peak := func(samples []int8) int {
		value := 0
		for _, sample := range samples {
			candidate := int(sample)
			if candidate < 0 {
				candidate = -candidate
			}
			value = max(value, candidate)
		}
		return value
	}
	if peak(afterNote) >= peak(fifteenthStep) {
		t.Fatalf("post-note high-pass tail did not decay: active=%d tail=%d", peak(fifteenthStep), peak(afterNote))
	}
}

func TestNativeEditorChannelsStayHardwareTyped(t *testing.T) {
	m := nativeEditorFixture(t)
	for channel := 0; channel < 4; channel++ {
		if err := m.SetNativeNote(0, channel, channel, 60+channel); err != nil {
			t.Fatalf("CH%d: %v", channel+1, err)
		}
	}
	if len(m.NativeAudio.Sounds.Sounds) != 4 || len(m.NativeAudio.Music.Clips) != 4 {
		t.Fatalf("sounds/clips = %d/%d", len(m.NativeAudio.Sounds.Sounds), len(m.NativeAudio.Music.Clips))
	}
	for channel := 0; channel < 4; channel++ {
		clip, ok := m.NativeClipAt(0, channel)
		if !ok || clip.Kind != nativeKindName(channel) {
			t.Fatalf("CH%d clip = %#v ok=%t", channel+1, clip, ok)
		}
	}
	if err := m.ClearNativeLane(0, 2); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.NativeClipAt(0, 2); ok {
		t.Fatal("clearing CH3 left the clip assigned")
	}
	if _, ok := m.NativeClipAt(0, 1); !ok {
		t.Fatal("clearing CH3 changed CH2")
	}
}

func TestNativeEditorNoiseRowsMaterializeDistinctHardwarePresets(t *testing.T) {
	m := nativeEditorFixture(t)
	for preset := 0; preset < 4; preset++ {
		if err := m.SetNativeNoiseHit(0, preset, preset); err != nil {
			t.Fatalf("preset %d: %v", preset, err)
		}
	}
	clip, ok := m.NativeClipAt(0, 3)
	if !ok || len(clip.Events) != 4 {
		t.Fatalf("noise clip = %#v ok=%t", clip, ok)
	}
	seen := map[int]bool{}
	for preset, event := range clip.Events {
		if event.Note != 36+preset || seen[event.SoundID] {
			t.Fatalf("noise event %d = %#v", preset, event)
		}
		seen[event.SoundID] = true
	}
	if len(seen) != 4 {
		t.Fatalf("noise sound IDs = %#v", seen)
	}
}

func TestNativeEditorArrangementTempoLoopAndRename(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNote(0, 1, 0, 64); err != nil {
		t.Fatal(err)
	}
	if err := m.RenameNativeClip(0, 1, "Verse pulse"); err != nil {
		t.Fatal(err)
	}
	second, err := m.AddNativePart(0)
	if err != nil {
		t.Fatal(err)
	}
	if second != 1 {
		t.Fatalf("second sequence index = %d", second)
	}
	if err := m.SetNativeSongName("Pocket Adventure"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetNativeTempo(144); err != nil {
		t.Fatal(err)
	}
	if err := m.SetNativeSongLoop(true); err != nil {
		t.Fatal(err)
	}
	song, _ := m.NativeSong()
	if song.Name != "Pocket Adventure" || song.Tempo != 144 || len(song.Sequence) != 2 || song.LoopStart == nil || *song.LoopStart != 0 {
		t.Fatalf("song = %#v", song)
	}
	clip, ok := m.NativeClipAt(1, 1)
	if !ok || clip.Name != "Verse pulse" {
		t.Fatalf("duplicated assignment = %#v ok=%t", clip, ok)
	}
	if err := m.SetNativeNote(1, 1, 1, 67); err != nil {
		t.Fatal(err)
	}
	first, _ := m.NativeClipAt(0, 1)
	secondClip, _ := m.NativeClipAt(1, 1)
	if first.ID == secondClip.ID || len(first.Events) != 1 || len(secondClip.Events) != 2 {
		t.Fatalf("section clips were not copied independently: first=%#v second=%#v", first, secondClip)
	}
}

func TestNativeEditorPasteCreatesIndependentClip(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNote(0, 0, 0, 60); err != nil {
		t.Fatal(err)
	}
	first, _ := m.NativeClipAt(0, 0)
	secondSection, err := m.AddNativePart(-1)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.DuplicateNativeClipTo(secondSection, 0, first.ID); err != nil {
		t.Fatal(err)
	}
	pasted, ok := m.NativeClipAt(secondSection, 0)
	if !ok || pasted.ID == first.ID || len(pasted.Events) != 1 {
		t.Fatalf("pasted clip = %#v ok=%t, source = %#v", pasted, ok, first)
	}
	if err := m.SetNativeNote(secondSection, 0, 1, 64); err != nil {
		t.Fatal(err)
	}
	firstAfter, _ := m.NativeClipAt(0, 0)
	pastedAfter, _ := m.NativeClipAt(secondSection, 0)
	if len(firstAfter.Events) != 1 || len(pastedAfter.Events) != 2 {
		t.Fatalf("paste edits leaked into source: source=%#v paste=%#v", firstAfter, pastedAfter)
	}
}

func TestNativeEditorSectionsRenameReorderAndDelete(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.RenameNativePart(0, "Intro"); err != nil {
		t.Fatal(err)
	}
	second, err := m.AddNativePart(-1)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RenameNativePart(second, "Verse"); err != nil {
		t.Fatal(err)
	}
	moved, err := m.MoveNativePart(second, -1)
	if err != nil || moved != 0 {
		t.Fatalf("move = %d, %v", moved, err)
	}
	part, ok := m.NativePartAt(0)
	if !ok || part.Name != "Verse" {
		t.Fatalf("first part after move = %#v ok=%t", part, ok)
	}
	selected, err := m.DeleteNativePart(0)
	if err != nil || selected != 0 {
		t.Fatalf("delete = %d, %v", selected, err)
	}
	part, ok = m.NativePartAt(0)
	if !ok || part.Name != "Intro" {
		t.Fatalf("remaining part = %#v ok=%t", part, ok)
	}
	if _, err := m.DeleteNativePart(0); err == nil {
		t.Fatal("last song section was deleted")
	}
}

func TestNativeInstrumentsAreExplicitReusableAndCopyOnWrite(t *testing.T) {
	m := nativeEditorFixture(t)
	baseID, err := m.CreateNativeSound(0, -1, "Warm lead")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetNativeNoteDurationWithSound(0, 0, 0, 60, 2, baseID); err != nil {
		t.Fatal(err)
	}
	if err := m.SetNativeNoteDurationWithSound(0, 0, 4, 64, 2, baseID); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateNativeSoundAtStep(0, 0, 0, func(sound *assetsync.NativeSoundAsset) {
		sound.Pulse.Duty = 12.5
	}); err != nil {
		t.Fatal(err)
	}
	clip, _ := m.NativeClipAt(0, 0)
	if len(clip.Events) != 2 || clip.Events[0].SoundID == clip.Events[1].SoundID || clip.Events[1].SoundID != baseID {
		t.Fatalf("shared instrument edit was not isolated: %#v", clip.Events)
	}
	variation, ok := m.NativeSoundByID(clip.Events[0].SoundID)
	if !ok || variation.Pulse.Duty != 12.5 || variation.Name != "Warm lead variation" {
		t.Fatalf("isolated variation = %#v ok=%t", variation, ok)
	}
	base, ok := m.NativeSoundByID(baseID)
	if !ok || base.Pulse.Duty == variation.Pulse.Duty {
		t.Fatalf("base instrument was mutated: base=%#v variation=%#v", base, variation)
	}
	if got := m.NativeSoundsForChannel(0); len(got) < 2 {
		t.Fatalf("CH1 instrument list = %#v", got)
	}
}

func TestNativePartReuseAndLoopFromSelectedSection(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.RenameNativePart(0, "Verse"); err != nil {
		t.Fatal(err)
	}
	second, err := m.AddNativePart(-1)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RenameNativePart(second, "Chorus"); err != nil {
		t.Fatal(err)
	}
	reused, err := m.ReuseNativePart(0, second)
	if err != nil || reused != 2 {
		t.Fatalf("reuse inserted=%d err=%v", reused, err)
	}
	song, _ := m.NativeSong()
	if len(song.Sequence) != 3 || song.Sequence[0] != song.Sequence[2] || song.Sequence[0] == song.Sequence[1] {
		t.Fatalf("song order did not reuse Verse: %#v", song.Sequence)
	}
	if err := m.SetNativeSongLoopStart(1); err != nil {
		t.Fatal(err)
	}
	song, _ = m.NativeSong()
	if song.LoopStart == nil || *song.LoopStart != 1 {
		t.Fatalf("loop start = %#v", song.LoopStart)
	}
	if _, err := m.DeleteNativePart(0); err != nil {
		t.Fatal(err)
	}
	song, _ = m.NativeSong()
	verse, ok := m.NativePartAt(1)
	if len(song.Sequence) != 2 || !ok || verse.Name != "Verse" || song.LoopStart == nil || *song.LoopStart != 0 {
		t.Fatalf("deleting one reference damaged reused part: %#v", song.Sequence)
	}
}

func TestNativePartMeterResizesGridClipsAndPreview(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativePartMeter(0, 2, 3, 4); err != nil {
		t.Fatal(err)
	}
	part, ok := m.NativePartAt(0)
	if !ok {
		t.Fatal("resized part is missing")
	}
	bars, beats, unit, ticksPerBar := NativePartMeter(part)
	if bars != 2 || beats != 3 || unit != 4 || ticksPerBar != 288 || part.LengthTicks != 576 {
		t.Fatalf("meter = %d bars %d/%d, ticks/bar=%d length=%d", bars, beats, unit, ticksPerBar, part.LengthTicks)
	}
	if got := m.NativePartBarStepCount(0); got != 48 {
		t.Fatalf("bar grid steps = %d, want 48", got)
	}
	if err := m.SetNativeNoteDuration(0, 0, 80, 60, 16); err != nil {
		t.Fatal(err)
	}
	song, _ := m.NativeSong()
	preview, err := RenderNativeMusicPreview(m.NativeAudio.Library, song.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Samples) != NativePreviewSampleRate*3 {
		t.Fatalf("2 bars of 3/4 at 120 BPM = %d samples, want %d", len(preview.Samples), NativePreviewSampleRate*3)
	}
	_, step, ok := preview.Position(NativePreviewSampleRate * 2)
	if !ok || step != 64 {
		t.Fatalf("multi-bar preview position = step %d ok=%t, want 64", step, ok)
	}
	if err := m.SetNativePartMeter(0, 1, 3, 4); err != nil {
		t.Fatal(err)
	}
	clip, _ := m.NativeClipAt(0, 0)
	if len(clip.Events) != 0 || clip.LengthTicks != 288 {
		t.Fatalf("shrunk clip = %#v", clip)
	}
}

func TestNativePartMeterSupportsCompoundOddAndCustomValues(t *testing.T) {
	m := nativeEditorFixture(t)
	for _, meter := range []struct {
		beats int
		unit  int
		ticks int
	}{{6, 8, 288}, {11, 8, 528}, {13, 16, 312}, {3, 2, 576}} {
		if err := m.SetNativePartMeter(0, 1, meter.beats, meter.unit); err != nil {
			t.Fatalf("set 1 bar of %d/%d: %v", meter.beats, meter.unit, err)
		}
		part, ok := m.NativePartAt(0)
		if !ok || part.LengthTicks != meter.ticks || part.BeatsPerBar != meter.beats || part.BeatUnit != meter.unit {
			t.Fatalf("meter %d/%d = %#v, ok=%t", meter.beats, meter.unit, part, ok)
		}
	}
}

func TestNativeMultipleSongsHaveIndependentOrdersAndSelection(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNote(0, 0, 0, 60); err != nil {
		t.Fatal(err)
	}
	firstSong, _ := m.NativeSong()
	second, err := m.AddNativeSong("Battle")
	if err != nil || second != 1 || m.NativeSongIndex != 1 {
		t.Fatalf("add song index=%d selected=%d err=%v", second, m.NativeSongIndex, err)
	}
	if err := m.SetNativeTempo(180); err != nil {
		t.Fatal(err)
	}
	if err := m.SetNativeNote(0, 1, 0, 48); err != nil {
		t.Fatal(err)
	}
	battle, ok := m.NativeSong()
	if !ok || battle.Name != "Battle" || battle.Tempo != 180 || battle.ID == firstSong.ID {
		t.Fatalf("second song = %#v ok=%t", battle, ok)
	}
	if err := m.SelectNativeSong(0); err != nil {
		t.Fatal(err)
	}
	firstAgain, _ := m.NativeSong()
	firstClip, firstClipOK := m.NativeClipAt(0, 0)
	if firstAgain.ID != firstSong.ID || firstAgain.Tempo == battle.Tempo || !firstClipOK || len(firstClip.Events) != 1 {
		t.Fatalf("first song changed: song=%#v clip=%#v ok=%t", firstAgain, firstClip, firstClipOK)
	}
	selected, err := m.DeleteNativeSong(0)
	if err != nil || selected != 0 || len(m.NativeSongs()) != 1 {
		t.Fatalf("delete first song selected=%d songs=%d err=%v", selected, len(m.NativeSongs()), err)
	}
	remaining, _ := m.NativeSong()
	if remaining.Name != "Battle" {
		t.Fatalf("remaining song = %#v", remaining)
	}
	if _, err := m.DeleteNativeSong(0); err == nil {
		t.Fatal("last song was deleted")
	}
}

func TestNativeStandaloneSFXUsesStableClipIDAndExactNoisePreset(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNote(0, 0, 0, 60); err != nil {
		t.Fatal(err)
	}
	effectID, err := m.CreateNativeSFX(3, "Door crash")
	if err != nil {
		t.Fatal(err)
	}
	if effectID == 0 {
		t.Fatal("standalone effect reused the song clip ID")
	}
	if err := m.SetNativeSFXNoiseHitDuration(effectID, 0, 1, 4); err != nil {
		t.Fatal(err)
	}
	effects := m.NativeSFXClips()
	if len(effects) != 1 || effects[0].ID != effectID || effects[0].Name != "Door crash" || len(effects[0].Events) != 1 {
		t.Fatalf("standalone effects = %#v", effects)
	}
	if err := m.AssignNativeClip(0, 3, effectID); err == nil {
		t.Fatal("standalone SFX was assignable as a song lane")
	}
	if err := m.ClearNativeLane(0, 0); err != nil {
		t.Fatal(err)
	}
	if effects = m.NativeSFXClips(); len(effects) != 1 || effects[0].ID != effectID {
		t.Fatalf("detached song clip leaked into SFX library: %#v", effects)
	}
	event := effects[0].Events[0]
	sound, ok := m.NativeSoundByID(event.SoundID)
	if !ok || sound.Noise == nil || sound.Name != "Snare" || sound.Noise.Divisor != 3 || sound.Noise.Shift != 4 {
		t.Fatalf("snare preset = %#v ok=%t", sound, ok)
	}
	preview, err := RenderNativeSFXPreview(m.NativeAudio.Library, effectID, 120, 0, m.NativeAudio.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(preview.Samples, func(sample int8) bool { return sample != 0 }) {
		t.Fatal("standalone PSG effect preview is silent")
	}
	muted, err := RenderNativeSFXPreview(m.NativeAudio.Library, effectID, 120, 1<<3, m.NativeAudio.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(muted.Samples, func(sample int8) bool { return sample != 0 }) {
		t.Fatal("muted standalone PSG effect is audible")
	}
	if runtimeClip := nativeRuntimeClip(m.NativeAudio.Library.Clips, uint8(effectID)); runtimeClip == nil || runtimeClip.ID != uint8(effectID) {
		t.Fatal("compiled runtime library lost the stable sfx(id)")
	}
	if err := m.DeleteNativeSFX(effectID); err != nil {
		t.Fatal(err)
	}
	if len(m.NativeSFXClips()) != 0 {
		t.Fatal("standalone effect was not deleted")
	}
}

func TestNativeNoteLegatoIsAuthoredForPitchedChannelsOnly(t *testing.T) {
	m := nativeEditorFixture(t)
	if err := m.SetNativeNoteDuration(0, 0, 0, 60, 4); err != nil {
		t.Fatal(err)
	}
	if err := m.SetNativeNoteDuration(0, 0, 4, 64, 4); err != nil {
		t.Fatal(err)
	}
	if err := m.SetNativeNoteLegato(0, 0, 4, true); err != nil {
		t.Fatal(err)
	}
	clip, _ := m.NativeClipAt(0, 0)
	if len(clip.Events) != 2 || !clip.Events[1].Legato || !m.NativeAudio.Library.Clips[0].NoteEvents[1].Legato {
		t.Fatalf("legato event = %#v", clip.Events)
	}
	if err := m.SetNativeNoiseHit(0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := m.SetNativeNoteLegato(0, 3, 0, true); err == nil {
		t.Fatal("CH4 accepted legato")
	}
}

func TestNativePreviewKeepsPhaseAndEnvelopeAcrossTiedNotes(t *testing.T) {
	tied := nativeEditorFixture(t)
	if err := tied.SetNativeNoteDuration(0, 0, 0, 60, 4); err != nil {
		t.Fatal(err)
	}
	if err := tied.SetNativeNoteDuration(0, 0, 4, 60, 4); err != nil {
		t.Fatal(err)
	}
	if err := tied.SetNativeNoteLegato(0, 0, 4, true); err != nil {
		t.Fatal(err)
	}
	tiedSong, _ := tied.NativeSong()
	tiedPreview, err := RenderNativeMusicPreview(tied.NativeAudio.Library, tiedSong.ID)
	if err != nil {
		t.Fatal(err)
	}

	sustained := nativeEditorFixture(t)
	if err := sustained.SetNativeNoteDuration(0, 0, 0, 60, 8); err != nil {
		t.Fatal(err)
	}
	sustainedSong, _ := sustained.NativeSong()
	sustainedPreview, err := RenderNativeMusicPreview(sustained.NativeAudio.Library, sustainedSong.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tiedPreview.Samples, sustainedPreview.Samples) {
		t.Fatal("tied preview retriggered oscillator or envelope at the note boundary")
	}
}

func TestNativePreviewAppliesCH1SweepAndHardwareLength(t *testing.T) {
	base := audio.Sound{Kind: audio.SoundCH1, Pulse1: hw.Pulse1Config{
		Pulse: hw.PulseConfig{
			Duty: hw.PulseDuty50, Envelope: hw.HardwareEnvelope{Initial: 15},
			Route: hw.SoundRouteBoth,
		},
	}}
	plain := make([]int16, NativePreviewSampleRate/2)
	swept := make([]int16, len(plain))
	nativeRenderEvent(plain, 0, len(plain), 57, base)
	base.Pulse1.Sweep = hw.HardwareSweep{Pace: 1, Direction: hw.SweepUp, Shift: 2}
	nativeRenderEvent(swept, 0, len(swept), 57, base)
	if slices.Equal(plain, swept) {
		t.Fatal("CH1 sweep did not change preview waveform")
	}

	base.Pulse1.Pulse.UseLength = true
	base.Pulse1.Pulse.Length = 63
	limited := make([]int16, NativePreviewSampleRate/4)
	nativeRenderEvent(limited, 0, len(limited), 57, base)
	lastNonzero := -1
	for index, sample := range limited {
		if sample != 0 {
			lastNonzero = index
		}
	}
	if lastNonzero < 0 || lastNonzero >= NativePreviewSampleRate/256 {
		t.Fatalf("hardware length stopped at sample %d", lastNonzero)
	}
}

func TestNativePreviewNoiseClockMatchesNR43HardwareFormula(t *testing.T) {
	for _, test := range []struct {
		divisor uint8
		shift   uint8
		want    float64
	}{
		{divisor: 0, shift: 0, want: 524288},
		{divisor: 1, shift: 0, want: 262144},
		{divisor: 2, shift: 1, want: 65536},
		{divisor: 7, shift: 13, want: 262144.0 / (7 * 8192)},
	} {
		if got := nativeNoiseFrequency(test.divisor, test.shift); math.Abs(got-test.want) > 1e-9 {
			t.Fatalf("noise divisor=%d shift=%d frequency = %f, want %f", test.divisor, test.shift, got, test.want)
		}
	}
}

func TestNativePreviewBandlimitsPSGEdgesAtHostRate(t *testing.T) {
	if NativePreviewSampleRate != 48_000 {
		t.Fatalf("native preview sample rate = %d, want host-rate 48000", NativePreviewSampleRate)
	}
	for _, duty := range []float64{0.125, 0.25, 0.5, 0.75} {
		got := nativePulseAverage(0, 1, duty)
		want := 2*duty - 1
		if math.Abs(got-want) > 1e-12 {
			t.Fatalf("one-cycle %.3f pulse average = %.12f, want %.12f", duty, got, want)
		}
	}

	state := nativePreviewVoiceState{lfsr: 0x7fff}
	got := nativeNoiseAverage(&state, 16, hw.Noise15Bit)
	if got < -1 || got > 1 || state.lfsr == 0x7fff {
		t.Fatalf("16-transition noise integration = %.3f state %#04x", got, state.lfsr)
	}
}

func TestEveryFactoryPresetMatchesBetweenClickAndLiveAudition(t *testing.T) {
	m := nativeEditorFixture(t)
	const note = uint8(60)
	for channel := 0; channel < 4; channel++ {
		for preset, factory := range NativeSoundPresets(channel) {
			id, _, err := m.EnsureNativeSoundPreset(channel, preset)
			if err != nil {
				t.Fatal(err)
			}
			var sound *audio.Sound
			for index := range m.NativeAudio.Library.Sounds {
				if int(m.NativeAudio.Library.Sounds[index].ID) == id {
					sound = &m.NativeAudio.Library.Sounds[index]
					break
				}
			}
			if sound == nil {
				t.Fatalf("CH%d preset %q was not compiled", channel+1, factory.Name)
			}
			click, err := RenderNativeNotePreview(m.NativeAudio.Library, id, int(note))
			if err != nil {
				t.Fatal(err)
			}
			stream := performance.NewStream(NativePreviewSampleRate)
			if !stream.Start(uint8(channel+1), note, *sound) {
				t.Fatalf("CH%d preset %q live start rejected", channel+1, factory.Name)
			}
			pcm := make([]byte, len(click)*4)
			if _, err := stream.Read(pcm); err != nil {
				t.Fatal(err)
			}
			var totalDifference float64
			peakDifference := 0.0
			peakIndex := 0
			comparisonSamples := min(len(click), NativePreviewSampleRate/20)
			for index, want := range click[:comparisonSamples] {
				live := float64(int16(binary.LittleEndian.Uint16(pcm[index*4:]))) / 250.0
				difference := math.Abs(live - float64(want))
				totalDifference += difference
				if difference > peakDifference {
					peakDifference = difference
					peakIndex = index
				}
			}
			meanDifference := totalDifference / float64(comparisonSamples)
			if meanDifference > 1.5 || peakDifference > 6 {
				t.Fatalf("CH%d preset %q click/live mismatch: mean %.3f peak %.3f at sample %d", channel+1, factory.Name, meanDifference, peakDifference, peakIndex)
			}
		}
	}
}
