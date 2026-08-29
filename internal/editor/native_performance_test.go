package editor

import (
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/performance"
)

func recordedPolyTake() performance.RawPerformanceTake {
	return performance.RawPerformanceTake{Duration: time.Second, Events: []performance.RawEvent{
		{Offset: 0, Kind: performance.ActionStart, Channel: 1, Note: 67},
		{Offset: 0, Kind: performance.ActionStart, Channel: 2, Note: 64},
		{Offset: 0, Kind: performance.ActionStart, Channel: 3, Note: 60},
		{Offset: 500 * time.Millisecond, Kind: performance.ActionStop, Channel: 1, Note: 67},
		{Offset: 500 * time.Millisecond, Kind: performance.ActionStop, Channel: 2, Note: 64},
		{Offset: 500 * time.Millisecond, Kind: performance.ActionStop, Channel: 3, Note: 60},
	}}
}

func TestNativePerformanceTakePersistsRawAndPhysicalLanes(t *testing.T) {
	m := nativeEditorFixture(t)
	clock := performance.NewTrackerClock(120, NativePartTicks)
	id, err := m.ApplyNativePerformanceTake(0, recordedPolyTake(), clock, performance.QuantizeSixteenth, false)
	if err != nil {
		t.Fatal(err)
	}
	if id != 0 || len(m.NativeAudio.Music.Takes) != 1 || len(m.NativeAudio.Music.Takes[0].Events) != 6 {
		t.Fatalf("stored take id=%d takes=%#v", id, m.NativeAudio.Music.Takes)
	}
	want := [...]int{67, 64, 60}
	for channel, note := range want {
		clip, ok := m.NativeClipAt(0, channel)
		if !ok || len(clip.Events) != 1 || clip.Events[0].Note != note || clip.Events[0].DurationTicks != 96 {
			t.Fatalf("CH%d physical recording = %#v ok=%t", channel+1, clip, ok)
		}
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(m.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.NativeAudio.Music.Takes) != 1 || len(NativeTakeRaw(reloaded.NativeAudio.Music.Takes[0]).Events) != 6 {
		t.Fatalf("reloaded raw take = %#v", reloaded.NativeAudio.Music.Takes)
	}
}

func TestNativePerformanceAutomationCreatesRealSoundVariants(t *testing.T) {
	m := nativeEditorFixture(t)
	take := recordedPolyTake()
	take.Events = append([]performance.RawEvent{{
		Offset: 0, Kind: performance.ActionParameter, Channel: 1,
		Parameter: performance.ParameterDuty, Value: 75,
	}}, take.Events...)
	clock := performance.NewTrackerClock(120, NativePartTicks)
	if _, err := m.ApplyNativePerformanceTake(0, take, clock, performance.QuantizeSixteenth, false); err != nil {
		t.Fatal(err)
	}
	clip, _ := m.NativeClipAt(0, 0)
	if len(clip.Events) != 1 {
		t.Fatalf("recorded CH1 clip = %#v", clip)
	}
	var duty float64
	for _, sound := range m.NativeAudio.Sounds.Sounds {
		if sound.ID == clip.Events[0].SoundID {
			duty = sound.Pulse.Duty
		}
	}
	if duty != 75 {
		t.Fatalf("recorded duty = %g sounds=%#v", duty, m.NativeAudio.Sounds.Sounds)
	}
}

func TestNativePerformanceAutomationSplitsHeldNoteAtParameterTick(t *testing.T) {
	m := nativeEditorFixture(t)
	take := performance.RawPerformanceTake{Duration: 500 * time.Millisecond, Events: []performance.RawEvent{
		{Offset: 0, Kind: performance.ActionStart, Channel: 1, Note: 60},
		{Offset: 250 * time.Millisecond, Kind: performance.ActionParameter, Channel: 1, Parameter: performance.ParameterDuty, Value: 75},
		{Offset: 500 * time.Millisecond, Kind: performance.ActionStop, Channel: 1, Note: 60},
	}}
	clock := performance.NewTrackerClock(120, NativePartTicks)
	if _, err := m.ApplyNativePerformanceTake(0, take, clock, performance.QuantizeOff, false); err != nil {
		t.Fatal(err)
	}
	clip, ok := m.NativeClipAt(0, 0)
	if !ok || len(clip.Events) != 2 {
		t.Fatalf("recorded CH1 automation clip = %#v ok=%t", clip, ok)
	}
	if got, want := clip.Events[0].Tick, 0; got != want {
		t.Fatalf("first segment tick = %d, want %d", got, want)
	}
	if got, want := clip.Events[0].DurationTicks, 48; got != want {
		t.Fatalf("first segment duration = %d, want %d", got, want)
	}
	if got, want := clip.Events[1].Tick, 48; got != want {
		t.Fatalf("second segment tick = %d, want %d", got, want)
	}
	if got, want := clip.Events[1].DurationTicks, 48; got != want {
		t.Fatalf("second segment duration = %d, want %d", got, want)
	}
	if clip.Events[0].SoundID == clip.Events[1].SoundID {
		t.Fatalf("parameter automation reused sound %d across both segments: events=%#v sounds=%#v materialized=%#v", clip.Events[0].SoundID, clip.Events, m.NativeAudio.Sounds.Sounds, performance.Materialize(take, clock, performance.QuantizeOff))
	}
}

func TestNativePerformanceNextNoteAutomationDoesNotSplitHeldNote(t *testing.T) {
	m := nativeEditorFixture(t)
	take := performance.RawPerformanceTake{Duration: 750 * time.Millisecond, Events: []performance.RawEvent{
		{Offset: 0, Kind: performance.ActionStart, Channel: 1, Note: 60},
		{Offset: 250 * time.Millisecond, Kind: performance.ActionParameter, Channel: 1, Parameter: performance.ParameterVolume, Value: 7},
		{Offset: 500 * time.Millisecond, Kind: performance.ActionStop, Channel: 1, Note: 60},
		{Offset: 500 * time.Millisecond, Kind: performance.ActionStart, Channel: 1, Note: 62},
		{Offset: 750 * time.Millisecond, Kind: performance.ActionStop, Channel: 1, Note: 62},
	}}
	clock := performance.NewTrackerClock(120, NativePartTicks)
	if _, err := m.ApplyNativePerformanceTake(0, take, clock, performance.QuantizeOff, false); err != nil {
		t.Fatal(err)
	}
	clip, ok := m.NativeClipAt(0, 0)
	if !ok || len(clip.Events) != 2 {
		t.Fatalf("NEXT_NOTE automation clip = %#v ok=%t", clip, ok)
	}
	if clip.Events[0].Tick != 0 || clip.Events[0].DurationTicks != 96 || clip.Events[1].Tick != 96 {
		t.Fatalf("NEXT_NOTE automation split held note: %#v", clip.Events)
	}
	if clip.Events[0].SoundID == clip.Events[1].SoundID {
		t.Fatalf("next note did not receive changed volume: %#v", clip.Events)
	}
	for _, sound := range m.NativeAudio.Sounds.Sounds {
		if sound.ID == clip.Events[0].SoundID && sound.Pulse.Envelope.Initial != 12 {
			t.Fatalf("held note volume changed to %d", sound.Pulse.Envelope.Initial)
		}
		if sound.ID == clip.Events[1].SoundID && sound.Pulse.Envelope.Initial != 7 {
			t.Fatalf("next note volume = %d, want 7", sound.Pulse.Envelope.Initial)
		}
	}
}

func TestNativePerformanceRetainedTakeCanBeRequantizedWithoutDuplication(t *testing.T) {
	m := nativeEditorFixture(t)
	take := performance.RawPerformanceTake{Duration: 400 * time.Millisecond, Events: []performance.RawEvent{
		{Offset: 130 * time.Millisecond, Kind: performance.ActionStart, Channel: 1, Note: 60},
		{Offset: 390 * time.Millisecond, Kind: performance.ActionStop, Channel: 1, Note: 60},
	}}
	clock := performance.NewTrackerClock(120, NativePartTicks)
	id, err := m.ApplyNativePerformanceTake(0, take, clock, performance.QuantizeOff, false)
	if err != nil {
		t.Fatal(err)
	}
	clip, _ := m.NativeClipAt(0, 0)
	if got, want := clip.Events[0].Tick, 25; got != want {
		t.Fatalf("unquantized tick = %d, want %d", got, want)
	}
	if err := m.UseNativePerformanceTake(0, id, clock, performance.QuantizeSixteenth, false); err != nil {
		t.Fatal(err)
	}
	clip, _ = m.NativeClipAt(0, 0)
	if got, want := clip.Events[0].Tick, 24; got != want {
		t.Fatalf("requantized tick = %d, want %d", got, want)
	}
	if takes := m.NativeTakesAt(0); len(takes) != 1 || takes[0].ID != id {
		t.Fatalf("retained takes = %#v", takes)
	}
}

func TestNativePerformanceRejectsDroppedTake(t *testing.T) {
	m := nativeEditorFixture(t)
	if _, err := m.ApplyNativePerformanceTake(0, performance.RawPerformanceTake{Dropped: 1}, performance.NewTrackerClock(120, NativePartTicks), performance.QuantizeSixteenth, false); err == nil {
		t.Fatal("dropped take was committed")
	}
}

func TestNativePerformanceTakeExtendsPartToWholeBars(t *testing.T) {
	m := nativeEditorFixture(t)
	take := performance.RawPerformanceTake{Duration: 5 * time.Second, Events: []performance.RawEvent{
		{Offset: 4250 * time.Millisecond, Kind: performance.ActionStart, Channel: 1, Note: 60},
		{Offset: 4500 * time.Millisecond, Kind: performance.ActionStop, Channel: 1, Note: 60},
	}}
	clock := performance.NewTrackerClock(120, NativePartTicks)
	if _, err := m.ApplyNativePerformanceTake(0, take, clock, performance.QuantizeOff, false); err != nil {
		t.Fatal(err)
	}
	part, ok := m.NativePartAt(0)
	if !ok || part.Bars != 3 || part.LengthTicks != 3*NativePartTicks {
		t.Fatalf("extended recorded part = %#v ok=%t", part, ok)
	}
	clip, ok := m.NativeClipAt(0, 0)
	if !ok || clip.LengthTicks != part.LengthTicks || len(clip.Events) != 1 || clip.Events[0].Tick <= NativePartTicks {
		t.Fatalf("late recorded note was clipped to the first bar: %#v ok=%t", clip, ok)
	}
}
