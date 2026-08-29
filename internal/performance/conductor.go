package performance

import (
	"sync"
	"sync/atomic"
	"time"
)

type RecordingSession interface {
	Now() time.Duration
	StartRecording(time.Duration) bool
	StopRecording(time.Duration) (RawPerformanceTake, bool)
}

type CaptureEventKind uint8

const (
	CaptureStarted CaptureEventKind = iota
	CaptureTake
)

type CaptureEvent struct {
	Kind CaptureEventKind
	Take RawPerformanceTake
}

type CaptureOptions struct {
	Clock       TrackerClock
	CountInBars int
	Loop        bool
	Metronome   bool
}

// CaptureSnapshot is the editor-visible state of an active capture. Tick is
// measured from the first recorded sample, so count-in time never advances the
// recording playhead.
type CaptureSnapshot struct {
	Active    bool
	Recording bool
	Tick      int64
	Elapsed   time.Duration
}

// Conductor schedules count-in, recording boundaries, loop takes, and the
// metronome from one TrackerClock. Timers call Session directly, independent of
// the Ebitengine Update cadence.
type Conductor struct {
	session RecordingSession
	click   func(bool)
	events  chan CaptureEvent

	mu         sync.Mutex
	cancel     chan struct{}
	done       chan struct{}
	active     bool
	recording  bool
	metronome  atomic.Bool
	recordAt   time.Duration
	loopLength time.Duration
	clock      TrackerClock
}

func NewConductor(session RecordingSession, click func(bool)) *Conductor {
	return &Conductor{session: session, click: click, events: make(chan CaptureEvent, 32)}
}

func (c *Conductor) Events() <-chan CaptureEvent { return c.events }

func (c *Conductor) Active() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

// Snapshot reports a monotonic recording position without tying transport to
// Ebitengine's Update cadence. During count-in Active is true and Recording is
// false; Tick remains zero until capture actually starts.
func (c *Conductor) Snapshot() CaptureSnapshot {
	if c == nil || c.session == nil {
		return CaptureSnapshot{}
	}
	c.mu.Lock()
	active, recording, recordAt, clock := c.active, c.recording, c.recordAt, c.clock
	c.mu.Unlock()
	result := CaptureSnapshot{Active: active, Recording: recording}
	if !recording {
		return result
	}
	now := c.session.Now()
	result.Elapsed = max(time.Duration(0), now-recordAt)
	result.Tick = max(int64(0), clock.TickAt(now))
	return result
}

func (c *Conductor) SetMetronome(enabled bool) {
	if c != nil {
		c.metronome.Store(enabled)
	}
}

func (c *Conductor) Start(options CaptureOptions) bool {
	if c == nil || c.session == nil {
		return false
	}
	c.mu.Lock()
	if c.active {
		c.mu.Unlock()
		return false
	}
	now := c.session.Now()
	clock := options.Clock
	clock.Origin = now
	clock.CountInBars = min(max(options.CountInBars, 0), 2)
	recordAt := now + clock.CountInDuration()
	loopLength := clock.TimeAtTick(int64(clock.PartTicks)) - clock.Origin
	recordClock := options.Clock
	recordClock.Origin, recordClock.CountInBars = recordAt, 0
	c.cancel, c.done, c.active = make(chan struct{}), make(chan struct{}), true
	c.recordAt, c.loopLength, c.clock, c.recording = recordAt, loopLength, recordClock, false
	c.metronome.Store(options.Metronome)
	cancel, done := c.cancel, c.done
	c.mu.Unlock()
	if clock.CountInBars == 0 {
		if !c.session.StartRecording(recordAt) {
			c.mu.Lock()
			c.active = false
			close(done)
			c.mu.Unlock()
			return false
		}
		c.setRecording(true)
		c.emit(CaptureEvent{Kind: CaptureStarted})
	}
	go c.run(clock, options.Loop, cancel, done)
	return true
}

func (c *Conductor) run(clock TrackerClock, loop bool, cancel <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	beatLength := clock.TimeAtTick(PPQN) - clock.Origin
	if beatLength <= 0 {
		beatLength = time.Second / 2
	}
	now := clock.Origin
	countInBeats := clock.CountInBars * 4
	for beat := 0; beat < countInBeats; beat++ {
		if !c.waitUntil(now+time.Duration(beat)*beatLength, cancel) {
			return
		}
		if c.click != nil {
			c.click(beat%4 == 0)
		}
	}
	if clock.CountInBars > 0 {
		if !c.waitUntil(c.recordAt, cancel) || !c.session.StartRecording(c.recordAt) {
			return
		}
		c.setRecording(true)
		c.emit(CaptureEvent{Kind: CaptureStarted})
	}
	nextBeat := c.recordAt
	nextBoundary := time.Duration(1<<63 - 1)
	if loop {
		nextBoundary = c.recordAt + c.loopLength
	}
	beat := 0
	for {
		target := nextBeat
		if nextBoundary < target {
			target = nextBoundary
		}
		if !c.waitUntil(target, cancel) {
			return
		}
		if target == nextBeat {
			if c.metronome.Load() && c.click != nil {
				c.click(beat%4 == 0)
			}
			beat++
			nextBeat += beatLength
		}
		if target == nextBoundary {
			c.setRecording(false)
			if take, ok := c.session.StopRecording(nextBoundary); ok {
				c.emit(CaptureEvent{Kind: CaptureTake, Take: take})
			}
			select {
			case <-cancel:
				return
			default:
			}
			if !c.session.StartRecording(nextBoundary) {
				return
			}
			c.setRecording(true)
			nextBoundary += c.loopLength
		}
	}
}

func (c *Conductor) waitUntil(target time.Duration, cancel <-chan struct{}) bool {
	delay := target - c.session.Now()
	if delay <= 0 {
		select {
		case <-cancel:
			return false
		default:
			return true
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-cancel:
		return false
	}
}

func (c *Conductor) Stop() (RawPerformanceTake, bool) {
	if c == nil || c.session == nil {
		return RawPerformanceTake{}, false
	}
	c.mu.Lock()
	if !c.active {
		c.mu.Unlock()
		return RawPerformanceTake{}, false
	}
	c.active = false
	c.recording = false
	cancel, done := c.cancel, c.done
	close(cancel)
	c.mu.Unlock()
	<-done
	return c.session.StopRecording(c.session.Now())
}

func (c *Conductor) setRecording(recording bool) {
	c.mu.Lock()
	c.recording = recording
	c.mu.Unlock()
}

func (c *Conductor) emit(event CaptureEvent) {
	select {
	case c.events <- event:
	default:
	}
}
