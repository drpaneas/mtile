package performance

import (
	"sort"
	"sync"
	"time"
)

const DefaultTakeCapacity = 8192

type RawEvent struct {
	Offset    time.Duration
	Kind      ActionKind
	Channel   uint8
	Note      uint8
	Parameter Parameter
	Value     int16
}

type RawPerformanceTake struct {
	Events   []RawEvent
	Duration time.Duration
	Dropped  uint32
}

// Recorder uses a fixed-capacity buffer while recording. Stop allocates the
// immutable take after the real-time path has ended.
type Recorder struct {
	mu        sync.Mutex
	events    []RawEvent
	start     time.Duration
	recording bool
	dropped   uint32
}

func NewRecorder(capacity int) *Recorder {
	if capacity <= 0 {
		capacity = DefaultTakeCapacity
	}
	return &Recorder{events: make([]RawEvent, 0, capacity)}
}

func (r *Recorder) Start(at time.Duration) {
	r.mu.Lock()
	r.events = r.events[:0]
	r.start, r.recording, r.dropped = at, true, 0
	r.mu.Unlock()
}

func (r *Recorder) Record(batch ActionBatch) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.recording {
		return
	}
	for _, action := range batch.Slice() {
		if len(r.events) == cap(r.events) {
			r.dropped++
			continue
		}
		offset := action.At - r.start
		if offset < 0 {
			offset = 0
		}
		r.events = append(r.events, RawEvent{
			Offset: offset, Kind: action.Kind, Channel: action.Channel, Note: action.Note,
			Parameter: action.Parameter, Value: action.Value,
		})
	}
}

func (r *Recorder) Stop(at time.Duration) RawPerformanceTake {
	r.mu.Lock()
	defer r.mu.Unlock()
	duration := at - r.start
	if duration < 0 {
		duration = 0
	}
	r.recording = false
	return RawPerformanceTake{Events: append([]RawEvent(nil), r.events...), Duration: duration, Dropped: r.dropped}
}

func (r *Recorder) Recording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recording
}

type TrackerNote struct {
	Channel       uint8
	StartTick     int
	DurationTicks int
	Note          uint8
}

type TrackerParameter struct {
	Channel   uint8
	Tick      int
	Parameter Parameter
	Value     int16
}

type MaterializedTake struct {
	Notes      []TrackerNote
	Parameters []TrackerParameter
	StartTick  int
	EndTick    int
	Dropped    uint32
}

type openNote struct {
	open  bool
	note  uint8
	start int64
}

func Materialize(take RawPerformanceTake, clock TrackerClock, grid Quantize) MaterializedTake {
	return MaterializeSwing(take, clock, grid, 50)
}

// MaterializeSwing converts a retained raw performance using the selected
// straight/triplet grid and swing amount. The raw take remains untouched, so
// changing either control is always a non-destructive re-quantization.
func MaterializeSwing(take RawPerformanceTake, clock TrackerClock, grid Quantize, swing int) MaterializedTake {
	out := MaterializedTake{Dropped: take.Dropped}
	open := [4]openNote{}
	closeNote := func(channel int, end int64) {
		state := &open[channel]
		if !state.open {
			return
		}
		start := clock.QuantizeTickSwing(state.start, grid, swing)
		end = clock.QuantizeTickSwing(end, grid, swing)
		minimum := int64(1)
		if grid != QuantizeOff {
			minimum = grid.Ticks()
		}
		if end <= start {
			end = start + minimum
		}
		out.Notes = append(out.Notes, TrackerNote{Channel: uint8(channel + 1), StartTick: int(start), DurationTicks: int(end - start), Note: state.note})
		state.open = false
	}

	for _, event := range take.Events {
		if event.Channel < 1 || event.Channel > 4 {
			continue
		}
		channel := int(event.Channel - 1)
		tick := clock.TickAt(clock.Origin + event.Offset)
		switch event.Kind {
		case ActionStart:
			closeNote(channel, tick)
			open[channel] = openNote{open: true, note: event.Note, start: tick}
			if channel == 3 {
				// CH4 drum hits are one quantize cell unless explicitly replaced
				// by another hit sooner.
				closeNote(channel, tick+max(1, grid.Ticks()))
			}
		case ActionStop:
			closeNote(channel, tick)
		case ActionParameter:
			out.Parameters = append(out.Parameters, TrackerParameter{
				Channel: event.Channel, Tick: int(clock.QuantizeTickSwing(tick, grid, swing)), Parameter: event.Parameter, Value: event.Value,
			})
		}
	}
	endTick := clock.TickAt(clock.Origin + take.Duration)
	for channel := range open {
		closeNote(channel, endTick)
	}
	sort.SliceStable(out.Notes, func(i, j int) bool {
		if out.Notes[i].Channel != out.Notes[j].Channel {
			return out.Notes[i].Channel < out.Notes[j].Channel
		}
		return out.Notes[i].StartTick < out.Notes[j].StartTick
	})
	// Quantization can collapse rapid allocator reassignments onto one tick.
	// Keep the latest transition at that position and trim its predecessor so
	// every physical channel remains a valid non-overlapping tracker lane.
	normalized := out.Notes[:0]
	for _, note := range out.Notes {
		if n := len(normalized); n > 0 && normalized[n-1].Channel == note.Channel {
			previous := &normalized[n-1]
			if previous.StartTick == note.StartTick {
				*previous = note
				continue
			}
			if previous.StartTick+previous.DurationTicks > note.StartTick {
				previous.DurationTicks = note.StartTick - previous.StartTick
			}
		}
		normalized = append(normalized, note)
	}
	out.Notes = normalized
	sort.SliceStable(out.Parameters, func(i, j int) bool {
		if out.Parameters[i].Tick != out.Parameters[j].Tick {
			return out.Parameters[i].Tick < out.Parameters[j].Tick
		}
		if out.Parameters[i].Channel != out.Parameters[j].Channel {
			return out.Parameters[i].Channel < out.Parameters[j].Channel
		}
		return out.Parameters[i].Parameter < out.Parameters[j].Parameter
	})
	// At one tracker position, the latest controller value wins.
	parameters := out.Parameters[:0]
	for _, parameter := range out.Parameters {
		if n := len(parameters); n > 0 && parameters[n-1].Tick == parameter.Tick && parameters[n-1].Channel == parameter.Channel && parameters[n-1].Parameter == parameter.Parameter {
			parameters[n-1] = parameter
		} else {
			parameters = append(parameters, parameter)
		}
	}
	out.Parameters = parameters
	if len(out.Notes) != 0 {
		out.StartTick = out.Notes[0].StartTick
		out.EndTick = out.StartTick
		for _, note := range out.Notes {
			out.StartTick = min(out.StartTick, note.StartTick)
			out.EndTick = max(out.EndTick, note.StartTick+note.DurationTicks)
		}
	}
	return out
}

// MergeNotes implements loop overdub/replace semantics. In replace mode only
// notes intersecting [start,end) are removed; data outside the take survives.
func MergeNotes(existing, recorded []TrackerNote, start, end int, overdub bool) []TrackerNote {
	result := make([]TrackerNote, 0, len(existing)+len(recorded))
	for _, note := range existing {
		intersects := note.StartTick < end && note.StartTick+note.DurationTicks > start
		if overdub || !intersects {
			result = append(result, note)
		}
	}
	result = append(result, recorded...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Channel != result[j].Channel {
			return result[i].Channel < result[j].Channel
		}
		return result[i].StartTick < result[j].StartTick
	})
	return result
}
