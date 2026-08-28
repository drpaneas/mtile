package performance

import (
	"slices"
	"testing"
	"time"
)

func notesByChannel(snapshot AllocationSnapshot) [4]int {
	result := [4]int{-1, -1, -1, -1}
	for channel := range snapshot.Active {
		if snapshot.Active[channel] {
			result[channel] = int(snapshot.Notes[channel])
		}
	}
	return result
}

func TestMonoLastNotePriorityRestoresHeldNote(t *testing.T) {
	a := NewAllocator(ModeMono, 2)
	a.Process(PerformanceEvent{Kind: NoteOn, Note: 60})
	batch := a.Process(PerformanceEvent{At: time.Millisecond, Kind: NoteOn, Note: 64})
	if got := notesByChannel(a.Snapshot()); got != [4]int{-1, 64, -1, -1} {
		t.Fatalf("second mono note = %v", got)
	}
	if len(batch.Slice()) != 2 || batch.Actions[0].Kind != ActionStop || batch.Actions[1].Kind != ActionStart {
		t.Fatalf("mono replacement actions = %#v", batch.Slice())
	}
	a.Process(PerformanceEvent{At: 2 * time.Millisecond, Kind: NoteOff, Note: 64})
	if got := notesByChannel(a.Snapshot()); got != [4]int{-1, 60, -1, -1} {
		t.Fatalf("released mono note did not restore held key: %v", got)
	}
}

func TestPoly2UsesNewestSetThenSortsHighLow(t *testing.T) {
	a := NewAllocator(ModePoly2, 1)
	for index, note := range []uint8{60, 67, 64} {
		a.Process(PerformanceEvent{At: time.Duration(index) * time.Millisecond, Kind: NoteOn, Note: note})
	}
	if got := notesByChannel(a.Snapshot()); got != [4]int{67, 64, -1, -1} {
		t.Fatalf("POLY2 overflow = %v, want newest active set sorted high/low", got)
	}
	a.Process(PerformanceEvent{At: 4 * time.Millisecond, Kind: NoteOff, Note: 64})
	if got := notesByChannel(a.Snapshot()); got != [4]int{67, 60, -1, -1} {
		t.Fatalf("POLY2 held restoration = %v", got)
	}
}

func TestPoly3CEGMapsPhysicalHighMiddleLow(t *testing.T) {
	a := NewAllocator(ModePoly3, 1)
	for _, note := range []uint8{60, 64, 67} {
		a.Process(PerformanceEvent{Kind: NoteOn, Note: note})
	}
	if got := notesByChannel(a.Snapshot()); got != [4]int{67, 64, 60, -1} {
		t.Fatalf("C-E-G allocation = %v", got)
	}
	if got := a.Snapshot().Armed; got != [4]bool{true, true, true, false} {
		t.Fatalf("POLY3 armed channels = %v", got)
	}
}

func TestParameterChangesMaterializeOnReservedChannels(t *testing.T) {
	a := NewAllocator(ModePoly3, 1)
	batch := a.Process(PerformanceEvent{Kind: ParameterChange, Parameter: ParameterDuty, Value: 75})
	if batch.Len != 3 {
		t.Fatalf("linked parameter actions = %d", batch.Len)
	}
	channels := []uint8{batch.Actions[0].Channel, batch.Actions[1].Channel, batch.Actions[2].Channel}
	if !slices.Equal(channels, []uint8{1, 2, 3}) {
		t.Fatalf("linked parameter channels = %v", channels)
	}
}

func TestLinkedParametersEmitOnlyRealHardwareTargets(t *testing.T) {
	tests := []struct {
		parameter Parameter
		channels  []uint8
	}{
		{ParameterDuty, []uint8{1, 2, 3}},
		{ParameterVolume, []uint8{1, 2}},
		{ParameterEnvelopePace, []uint8{1, 2}},
		{ParameterEnvelopeDirection, []uint8{1, 2}},
		{ParameterSweepPace, []uint8{1}},
		{ParameterSweepDirection, []uint8{1}},
		{ParameterSweepShift, []uint8{1}},
		{ParameterWaveLevel, []uint8{3}},
		{ParameterNoiseWidth, nil},
	}
	for _, test := range tests {
		a := NewAllocator(ModePoly3, 1)
		batch := a.Process(PerformanceEvent{Kind: ParameterChange, Parameter: test.parameter, Value: 1})
		channels := make([]uint8, 0, batch.Len)
		for _, action := range batch.Slice() {
			channels = append(channels, action.Channel)
		}
		if !slices.Equal(channels, test.channels) {
			t.Fatalf("parameter %d targets %v, want %v", test.parameter, channels, test.channels)
		}
	}

	noise := NewAllocator(ModeNoise, 1)
	if batch := noise.Process(PerformanceEvent{Kind: ParameterChange, Parameter: ParameterNoiseDivisor, Value: 3}); batch.Len != 1 || batch.Actions[0].Channel != 4 {
		t.Fatalf("noise parameter targets = %#v", batch.Slice())
	}
}

func TestNoiseModeAcceptsOnlyEightHardwarePads(t *testing.T) {
	a := NewAllocator(ModeNoise, 1)
	if got := a.Process(PerformanceEvent{Kind: NoteOn, Note: 35}); got.Len != 0 {
		t.Fatalf("out-of-range drum emitted %#v", got.Slice())
	}
	got := a.Process(PerformanceEvent{Kind: NoteOn, Note: 43})
	if got.Len != 1 || got.Actions[0].Channel != 4 || got.Actions[0].Note != 43 {
		t.Fatalf("noise pad = %#v", got.Slice())
	}
}

func TestAllocatorRejectsOutOfRangeMIDIDataWithoutPanicking(t *testing.T) {
	a := NewAllocator(ModePoly3, 1)
	if batch := a.Process(PerformanceEvent{Kind: NoteOn, Note: 255}); batch.Len != 0 {
		t.Fatalf("invalid MIDI note emitted %#v", batch.Slice())
	}
	if batch := a.Process(PerformanceEvent{Kind: NoteOff, Note: 255}); batch.Len != 0 {
		t.Fatalf("invalid MIDI note-off emitted %#v", batch.Slice())
	}
}

func TestAllNotesOffStopsEveryAssignedChannel(t *testing.T) {
	a := NewAllocator(ModePoly3, 1)
	for _, note := range []uint8{60, 64, 67} {
		a.Process(PerformanceEvent{Kind: NoteOn, Note: note})
	}
	batch := a.Process(PerformanceEvent{At: time.Second, Kind: AllNotesOff})
	if batch.Len != 3 || a.Snapshot().Held != 0 || a.Snapshot().Active != [4]bool{} {
		t.Fatalf("all notes off = actions %#v snapshot %#v", batch.Slice(), a.Snapshot())
	}
}
