package performance

import (
	"encoding/binary"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
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
	synctest.Test(t, func(t *testing.T) {
		session, conductor := newTestConductor(t, nil)
		if !conductor.Start(CaptureOptions{Clock: NewTrackerClock(120, DefaultPartTicks)}) {
			t.Fatal("conductor did not start")
		}
		assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{Active: true, Recording: true})
		captureEvent(t, conductor, CaptureStarted)
		session.Handle(PerformanceEvent{At: session.Now(), Kind: NoteOn, Note: 60})
		synctest.Sleep(250 * time.Millisecond)
		session.Handle(PerformanceEvent{At: session.Now(), Kind: NoteOff, Note: 60})
		synctest.Sleep(250 * time.Millisecond)
		take, ok := conductor.Stop()
		want := []RawEvent{
			{Kind: ActionStart, Channel: 1, Note: 60},
			{Offset: 250 * time.Millisecond, Kind: ActionStop, Channel: 1, Note: 60},
		}
		if !ok || take.Duration != 500*time.Millisecond || !slices.Equal(take.Events, want) {
			t.Fatalf("conductor stop = take %#v ok=%t, want duration 500ms and events %#v", take, ok, want)
		}
		assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{})
		synctest.Sleep(4 * time.Second)
		assertNoCaptureEvent(t, conductor)
		if _, ok := conductor.Stop(); ok {
			t.Fatal("second stop returned the take again")
		}
	})
}

func TestConductorLoopCutsTakesOnTrackerBoundaries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		session, conductor := newTestConductor(t, nil)
		if !conductor.Start(CaptureOptions{Clock: NewTrackerClock(120, DefaultPartTicks), Loop: true}) {
			t.Fatal("loop conductor did not start")
		}
		captureEvent(t, conductor, CaptureStarted)
		for loop := 0; loop < 3; loop++ {
			note := uint8(60 + loop)
			session.Handle(PerformanceEvent{At: session.Now(), Kind: NoteOn, Note: note})
			synctest.Sleep(time.Second)
			session.Handle(PerformanceEvent{At: session.Now(), Kind: NoteOff, Note: note})
			synctest.Sleep(time.Second - time.Nanosecond)
			assertNoCaptureEvent(t, conductor)
			synctest.Sleep(time.Nanosecond)
			take := captureEvent(t, conductor, CaptureTake).Take
			want := []RawEvent{
				{Kind: ActionStart, Channel: 1, Note: note},
				{Offset: time.Second, Kind: ActionStop, Channel: 1, Note: note},
			}
			if take.Duration != 2*time.Second || !slices.Equal(take.Events, want) {
				t.Fatalf("loop %d take = %#v, want duration 2s and events %#v", loop, take, want)
			}
			assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{
				Active: true, Recording: true,
				Elapsed: time.Duration(loop+1) * 2 * time.Second,
				Tick:    int64(loop+1) * DefaultPartTicks,
			})
			assertNoCaptureEvent(t, conductor)
		}
		synctest.Sleep(250 * time.Millisecond)
		if take, ok := conductor.Stop(); !ok || take.Duration != 250*time.Millisecond || len(take.Events) != 0 {
			t.Fatalf("partial final take = %#v ok=%t", take, ok)
		}
		synctest.Sleep(4 * time.Second)
		assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{})
		assertNoCaptureEvent(t, conductor)
	})
}

func TestConductorNonLoopRecordingContinuesPastPartBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		session, conductor := newTestConductor(t, nil)
		if !conductor.Start(CaptureOptions{Clock: NewTrackerClock(120, DefaultPartTicks)}) {
			t.Fatal("continuous conductor did not start")
		}
		captureEvent(t, conductor, CaptureStarted)
		synctest.Sleep(6 * time.Second)
		assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{
			Active: true, Recording: true, Elapsed: 6 * time.Second, Tick: 3 * DefaultPartTicks,
		})
		assertNoCaptureEvent(t, conductor)
		if take, ok := conductor.Stop(); !ok || take.Duration != 6*time.Second {
			t.Fatalf("continuous final take = %#v ok=%t", take, ok)
		}
	})
}

func TestConductorCountInStartsOnDownbeatAndExcludesPreroll(t *testing.T) {
	for _, bars := range []int{1, 2} {
		t.Run(strconv.Itoa(bars)+"_bars", func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				clicks, click := newTestClickLog()
				session, conductor := newTestConductor(t, click)
				// Capture can begin after the session has already been open for a while.
				synctest.Sleep(3 * time.Second)
				if !conductor.Start(CaptureOptions{Clock: NewTrackerClock(120, DefaultPartTicks), CountInBars: bars}) {
					t.Fatal("count-in did not start")
				}
				for beat := 0; beat < bars*4; beat++ {
					synctest.Wait()
					assertClicks(t, clicks, testClick{3*time.Second + time.Duration(beat)*500*time.Millisecond, beat%4 == 0})
					assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{Active: true})
					assertNoCaptureEvent(t, conductor)
					session.Handle(PerformanceEvent{At: session.Now(), Kind: NoteOn, Note: 60})
					session.Handle(PerformanceEvent{At: session.Now(), Kind: NoteOff, Note: 60})
					synctest.Sleep(500*time.Millisecond - time.Nanosecond)
					assertClicks(t, clicks)
					assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{Active: true})
					assertNoCaptureEvent(t, conductor)
					synctest.Sleep(time.Nanosecond)
				}
				captureEvent(t, conductor, CaptureStarted)
				assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{Active: true, Recording: true})
				session.Handle(PerformanceEvent{At: session.Now(), Kind: NoteOn, Note: 62})
				synctest.Sleep(500 * time.Millisecond)
				take, ok := conductor.Stop()
				want := []RawEvent{{Kind: ActionStart, Channel: 1, Note: 62}}
				if !ok || take.Duration != 500*time.Millisecond || !slices.Equal(take.Events, want) {
					t.Fatalf("take includes count-in: %#v ok=%t", take, ok)
				}
				assertClicks(t, clicks) // Count-in clicks must stop when the recording metronome is disabled.
			})
		})
	}
}

func TestConductorMetronomeToggleKeepsBeatAndAccent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clicks, click := newTestClickLog()
		session, conductor := newTestConductor(t, click)
		if !conductor.Start(CaptureOptions{Clock: NewTrackerClock(120, DefaultPartTicks), Metronome: true}) {
			t.Fatal("conductor did not start")
		}
		captureEvent(t, conductor, CaptureStarted)
		synctest.Sleep(2 * time.Second)
		assertClicks(t, clicks,
			testClick{0, true}, testClick{500 * time.Millisecond, false}, testClick{time.Second, false},
			testClick{1500 * time.Millisecond, false}, testClick{2 * time.Second, true},
		)
		conductor.SetMetronome(false)
		synctest.Sleep(time.Second)
		assertClicks(t, clicks)
		conductor.SetMetronome(true)
		synctest.Sleep(time.Second)
		assertClicks(t, clicks, testClick{3500 * time.Millisecond, false}, testClick{4 * time.Second, true})
		conductor.Stop()
		synctest.Sleep(4 * time.Second)
		assertClicks(t, clicks)
		assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{})
		assertNoCaptureEvent(t, conductor)
	})
}

func TestConductorCountInDelaysFirstLoopBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clicks, click := newTestClickLog()
		session, conductor := newTestConductor(t, click)
		if !conductor.Start(CaptureOptions{
			Clock: NewTrackerClock(120, DefaultPartTicks), CountInBars: 1, Loop: true, Metronome: true,
		}) {
			t.Fatal("counted loop did not start")
		}
		synctest.Sleep(2 * time.Second)
		captureEvent(t, conductor, CaptureStarted)
		assertNoCaptureEvent(t, conductor)
		assertClicks(t, clicks,
			testClick{0, true}, testClick{500 * time.Millisecond, false}, testClick{time.Second, false},
			testClick{1500 * time.Millisecond, false}, testClick{2 * time.Second, true},
		)
		synctest.Sleep(2*time.Second - time.Nanosecond)
		assertNoCaptureEvent(t, conductor)
		synctest.Sleep(time.Nanosecond)
		if take := captureEvent(t, conductor, CaptureTake).Take; take.Duration != 2*time.Second {
			t.Fatalf("first loop includes count-in: %#v", take)
		}
		assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{
			Active: true, Recording: true, Elapsed: 2 * time.Second, Tick: DefaultPartTicks,
		})
		assertClicks(t, clicks,
			testClick{2500 * time.Millisecond, false}, testClick{3 * time.Second, false},
			testClick{3500 * time.Millisecond, false}, testClick{4 * time.Second, true},
		)
	})
}

func TestConductorStopCancelsCountInAndAllowsRestart(t *testing.T) {
	for _, stopAt := range []time.Duration{0, time.Second, 2*time.Second - time.Nanosecond} {
		t.Run(stopAt.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var clicks atomic.Int32
				session, conductor := newTestConductor(t, func(bool) { clicks.Add(1) })
				if !conductor.Start(CaptureOptions{Clock: NewTrackerClock(120, DefaultPartTicks), CountInBars: 1}) {
					t.Fatal("count-in did not start")
				}
				if stopAt > 0 {
					synctest.Sleep(stopAt)
				}
				if _, ok := conductor.Stop(); ok {
					t.Fatal("canceling count-in returned a take")
				}
				stoppedClicks := clicks.Load()
				assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{})
				synctest.Sleep(4 * time.Second)
				if got := clicks.Load(); got != stoppedClicks {
					t.Fatalf("count-in clicked after cancellation: %d -> %d", stoppedClicks, got)
				}
				assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{})
				assertNoCaptureEvent(t, conductor)
				if !conductor.Start(CaptureOptions{Clock: NewTrackerClock(120, DefaultPartTicks)}) {
					t.Fatal("conductor could not restart after cancellation")
				}
				captureEvent(t, conductor, CaptureStarted)
				synctest.Sleep(500 * time.Millisecond)
				if take, ok := conductor.Stop(); !ok || take.Duration != 500*time.Millisecond {
					t.Fatalf("restarted take = %#v ok=%t", take, ok)
				}
				assertNoCaptureEvent(t, conductor)
			})
		})
	}
}

func TestConductorDisarmedBeforeRecordingBoundaryBecomesInactive(t *testing.T) {
	for _, test := range []struct {
		name    string
		options CaptureOptions
	}{
		{"count_in", CaptureOptions{CountInBars: 1}},
		{"loop_restart", CaptureOptions{Loop: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				session, conductor := newTestConductor(t, nil)
				test.options.Clock = NewTrackerClock(120, DefaultPartTicks)
				if !conductor.Start(test.options) {
					t.Fatal("conductor did not start")
				}
				if test.options.CountInBars == 0 {
					captureEvent(t, conductor, CaptureStarted)
				}
				synctest.Sleep(time.Second)
				session.Arm(false, session.Now())
				synctest.Sleep(time.Second)
				assertCaptureSnapshot(t, conductor, session, CaptureSnapshot{})
				assertNoCaptureEvent(t, conductor)
				session.Arm(true, session.Now())
				if !conductor.Start(CaptureOptions{Clock: NewTrackerClock(120, DefaultPartTicks)}) {
					t.Fatal("failed recording boundary left conductor unable to restart")
				}
				captureEvent(t, conductor, CaptureStarted)
			})
		})
	}
}

func newTestConductor(t *testing.T, click func(bool)) (*Session, *Conductor) {
	t.Helper()
	session := NewSession(nil, testSoundBank())
	session.Arm(true, 0)
	conductor := NewConductor(session, click)
	t.Cleanup(func() { conductor.Stop() })
	return session, conductor
}

func assertCaptureSnapshot(t *testing.T, conductor *Conductor, session *Session, want CaptureSnapshot) {
	t.Helper()
	if got := conductor.Snapshot(); got != want {
		t.Fatalf("capture snapshot = %#v, want %#v", got, want)
	}
	if got := conductor.Active(); got != want.Active {
		t.Fatalf("capture active = %t, want %t", got, want.Active)
	}
	if got := session.Snapshot().Recording; got != want.Recording {
		t.Fatalf("session recording = %t, want %t", got, want.Recording)
	}
}

func captureEvent(t *testing.T, conductor *Conductor, kind CaptureEventKind) CaptureEvent {
	t.Helper()
	select {
	case event := <-conductor.Events():
		if event.Kind != kind {
			t.Fatalf("capture event = %#v, want kind %d", event, kind)
		}
		return event
	default:
		t.Fatalf("missing capture event of kind %d", kind)
		return CaptureEvent{}
	}
}

func assertNoCaptureEvent(t *testing.T, conductor *Conductor) {
	t.Helper()
	select {
	case event := <-conductor.Events():
		t.Fatalf("unexpected capture event %#v", event)
	default:
	}
}

type testClick struct {
	at     time.Duration
	accent bool
}

func newTestClickLog() (<-chan testClick, func(bool)) {
	origin := time.Now()
	clicks := make(chan testClick, 32)
	return clicks, func(accent bool) { clicks <- testClick{time.Since(origin), accent} }
}

func assertClicks(t *testing.T, clicks <-chan testClick, want ...testClick) {
	t.Helper()
	for _, expected := range want {
		select {
		case got := <-clicks:
			if got != expected {
				t.Fatalf("metronome click = %#v, want %#v", got, expected)
			}
		default:
			t.Fatalf("missing metronome click %#v", expected)
		}
	}
	select {
	case got := <-clicks:
		t.Fatalf("unexpected metronome click %#v", got)
	default:
	}
}
