package performance

import (
	"testing"
	"time"
)

func TestRecorderMaterializesPhysicalPolyTransitions(t *testing.T) {
	clock := NewTrackerClock(120, DefaultPartTicks)
	recorder := NewRecorder(32)
	recorder.Start(0)
	a := NewAllocator(ModePoly3, 1)
	for _, event := range []PerformanceEvent{
		{At: 0, Kind: NoteOn, Note: 60},
		{At: 10 * time.Millisecond, Kind: NoteOn, Note: 64},
		{At: 20 * time.Millisecond, Kind: NoteOn, Note: 67},
		{At: 500 * time.Millisecond, Kind: NoteOff, Note: 67},
		{At: 510 * time.Millisecond, Kind: NoteOff, Note: 64},
		{At: 520 * time.Millisecond, Kind: NoteOff, Note: 60},
	} {
		recorder.Record(a.Process(event))
	}
	take := recorder.Stop(520 * time.Millisecond)
	materialized := Materialize(take, clock, QuantizeSixteenth)
	seen := [4]bool{}
	for _, note := range materialized.Notes {
		seen[note.Channel-1] = true
		if note.Channel == 3 && note.Note != 60 {
			t.Fatalf("CH3 did not receive low C: %#v", materialized.Notes)
		}
	}
	if seen != [4]bool{true, true, true, false} {
		t.Fatalf("recorded physical channels = %v notes=%#v", seen, materialized.Notes)
	}
}

func TestRecorderRetainsRawTimingForRequantization(t *testing.T) {
	clock := NewTrackerClock(120, DefaultPartTicks)
	take := RawPerformanceTake{Duration: time.Second, Events: []RawEvent{
		{Offset: 90 * time.Millisecond, Kind: ActionStart, Channel: 1, Note: 60},
		{Offset: 390 * time.Millisecond, Kind: ActionStop, Channel: 1, Note: 60},
	}}
	sixteenth := Materialize(take, clock, QuantizeSixteenth)
	thirtySecond := Materialize(take, clock, QuantizeThirtySecond)
	sixtyFourth := Materialize(take, clock, QuantizeSixtyFourth)
	if sixteenth.Notes[0].StartTick == thirtySecond.Notes[0].StartTick {
		t.Fatalf("requantization did not use raw offsets: 1/16=%#v 1/32=%#v", sixteenth.Notes, thirtySecond.Notes)
	}
	if sixtyFourth.Notes[0].StartTick%6 != 0 || sixtyFourth.Notes[0].DurationTicks%6 != 0 {
		t.Fatalf("1/64 materialization is not on the 6-tick grid: %#v", sixtyFourth.Notes)
	}
}

func TestParameterDedupLastValueWins(t *testing.T) {
	clock := NewTrackerClock(120, DefaultPartTicks)
	take := RawPerformanceTake{Events: []RawEvent{
		{Offset: 1 * time.Millisecond, Kind: ActionParameter, Channel: 1, Parameter: ParameterDuty, Value: 25},
		{Offset: 2 * time.Millisecond, Kind: ActionParameter, Channel: 1, Parameter: ParameterDuty, Value: 75},
	}}
	got := Materialize(take, clock, QuantizeSixteenth)
	if len(got.Parameters) != 1 || got.Parameters[0].Value != 75 {
		t.Fatalf("deduplicated parameters = %#v", got.Parameters)
	}
}

func TestRecorderMaterializesTripletsAndSwingFromRawTake(t *testing.T) {
	clock := NewTrackerClock(120, DefaultPartTicks)
	take := RawPerformanceTake{Duration: time.Second, Events: []RawEvent{
		{Offset: 170 * time.Millisecond, Kind: ActionStart, Channel: 1, Note: 60},
		{Offset: 330 * time.Millisecond, Kind: ActionStop, Channel: 1, Note: 60},
	}}
	triplet := MaterializeSwing(take, clock, QuantizeEighthTriplet, 50)
	if len(triplet.Notes) != 1 || triplet.Notes[0].StartTick%32 != 0 || triplet.Notes[0].DurationTicks%32 != 0 {
		t.Fatalf("triplet materialization = %#v", triplet.Notes)
	}
	swung := MaterializeSwing(take, clock, QuantizeEighth, 66)
	if len(swung.Notes) != 1 || swung.Notes[0].StartTick != 63 {
		t.Fatalf("swing materialization = %#v", swung.Notes)
	}
}

func TestReplaceModePreservesNotesOutsideRecordedRange(t *testing.T) {
	existing := []TrackerNote{
		{Channel: 1, StartTick: 0, DurationTicks: 12, Note: 60},
		{Channel: 1, StartTick: 24, DurationTicks: 12, Note: 62},
		{Channel: 1, StartTick: 96, DurationTicks: 12, Note: 64},
	}
	recorded := []TrackerNote{{Channel: 1, StartTick: 30, DurationTicks: 12, Note: 67}}
	got := MergeNotes(existing, recorded, 24, 48, false)
	if len(got) != 3 || got[0].Note != 60 || got[1].Note != 67 || got[2].Note != 64 {
		t.Fatalf("replace merge = %#v", got)
	}
}

func TestRecorderDropsWithoutAllocatingPastCapacity(t *testing.T) {
	r := NewRecorder(1)
	r.Start(0)
	batch := ActionBatch{Len: 2}
	batch.Actions[0] = ChannelAction{Kind: ActionStart, Channel: 1, Note: 60}
	batch.Actions[1] = ChannelAction{Kind: ActionStop, Channel: 1, Note: 60}
	r.Record(batch)
	take := r.Stop(time.Second)
	if len(take.Events) != 1 || take.Dropped != 1 {
		t.Fatalf("bounded recorder = %#v", take)
	}
}
