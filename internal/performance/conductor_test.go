package performance

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestClickStreamIsSeparateAudibleAndAllocationFree(t *testing.T) {
	stream := NewClickStream(48_000)
	pcm := make([]byte, 128*4)
	stream.Trigger(true)
	_, _ = stream.Read(pcm)
	nonzero := false
	for offset := 0; offset < len(pcm); offset += 2 {
		if binary.LittleEndian.Uint16(pcm[offset:]) != 0 {
			nonzero = true
			break
		}
	}
	if !nonzero {
		t.Fatal("metronome click is silent")
	}
	if allocations := testing.AllocsPerRun(100, func() { _, _ = stream.Read(pcm) }); allocations != 0 {
		t.Fatalf("click stream Read allocations = %g", allocations)
	}
}

func TestConductorStartsImmediatelyAndReturnsRawTake(t *testing.T) {
	session := NewSession(nil, testSoundBank())
	session.Arm(true, 0)
	conductor := NewConductor(session, nil)
	clock := NewTrackerClock(120, DefaultPartTicks)
	if !conductor.Start(CaptureOptions{Clock: clock}) || !session.Snapshot().Recording {
		t.Fatal("conductor did not start an uncounted recording synchronously")
	}
	session.Handle(PerformanceEvent{At: session.Now(), Kind: NoteOn, Note: 60})
	time.Sleep(time.Millisecond)
	take, ok := conductor.Stop()
	if !ok || len(take.Events) == 0 || session.Snapshot().Recording {
		t.Fatalf("conductor stop = take %#v ok=%t state %#v", take, ok, session.Snapshot())
	}
}

func TestConductorLoopCutsTakesOnTrackerBoundaries(t *testing.T) {
	session := NewSession(nil, testSoundBank())
	session.Arm(true, 0)
	conductor := NewConductor(session, nil)
	clock := NewTrackerClock(400, 4) // 6.25 ms loop for a fast deterministic test.
	if !conductor.Start(CaptureOptions{Clock: clock, Loop: true}) {
		t.Fatal("loop conductor did not start")
	}
	select {
	case event := <-conductor.Events():
		if event.Kind != CaptureStarted {
			t.Fatalf("first event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing capture start event")
	}
	select {
	case event := <-conductor.Events():
		if event.Kind != CaptureTake {
			t.Fatalf("loop event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("loop boundary did not produce a take")
	}
	_, _ = conductor.Stop()
}

func TestConductorNonLoopRecordingContinuesPastPartBoundary(t *testing.T) {
	session := NewSession(nil, testSoundBank())
	session.Arm(true, 0)
	conductor := NewConductor(session, nil)
	clock := NewTrackerClock(400, 4) // one part is 6.25 ms
	if !conductor.Start(CaptureOptions{Clock: clock, Loop: false}) {
		t.Fatal("continuous conductor did not start")
	}
	select {
	case <-conductor.Events(): // CaptureStarted
	case <-time.After(time.Second):
		t.Fatal("missing capture start event")
	}
	time.Sleep(20 * time.Millisecond)
	snapshot := conductor.Snapshot()
	if !snapshot.Active || !snapshot.Recording || snapshot.Tick <= int64(clock.PartTicks) {
		t.Fatalf("capture stopped at part boundary: %#v", snapshot)
	}
	select {
	case event := <-conductor.Events():
		t.Fatalf("non-loop capture emitted boundary event %#v", event)
	default:
	}
	if _, ok := conductor.Stop(); !ok {
		t.Fatal("continuous capture did not return its final take")
	}
}
