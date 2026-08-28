package performance

import (
	"sort"
	"time"
)

type heldNote struct {
	down  bool
	order uint64
}

// Allocator owns the authoring-only mono/poly policy. Its assignments are
// always physical channels; no synthetic poly voice survives this boundary.
type Allocator struct {
	mode        VoiceMode
	monoChannel uint8
	held        [128]heldNote
	serial      uint64
	assigned    [4]int16
}

func NewAllocator(mode VoiceMode, monoChannel uint8) Allocator {
	a := Allocator{}
	for i := range a.assigned {
		a.assigned[i] = -1
	}
	a.SetMode(mode, monoChannel, 0)
	return a
}

func (a *Allocator) SetMode(mode VoiceMode, monoChannel uint8, at time.Duration) ActionBatch {
	batch := a.stopAssigned(at)
	a.clearHeld()
	if mode > ModeNoise {
		mode = ModeMono
	}
	if monoChannel < 1 || monoChannel > 3 {
		monoChannel = 1
	}
	a.mode, a.monoChannel = mode, monoChannel
	return batch
}

func (a *Allocator) Process(event PerformanceEvent) ActionBatch {
	switch event.Kind {
	case AllNotesOff:
		batch := a.stopAssigned(event.At)
		a.clearHeld()
		return batch
	case ParameterChange:
		return a.parameterActions(event)
	case NoteOn:
		if event.Note >= uint8(len(a.held)) {
			return ActionBatch{}
		}
		if a.mode == ModeNoise {
			var batch ActionBatch
			if event.Note >= 36 && event.Note <= 43 {
				batch.append(ChannelAction{At: event.At, Kind: ActionStart, Channel: 4, Note: event.Note})
			}
			return batch
		}
		a.serial++
		a.held[event.Note] = heldNote{down: true, order: a.serial}
		return a.reassign(event.At)
	case NoteOff:
		if event.Note >= uint8(len(a.held)) {
			return ActionBatch{}
		}
		if a.mode == ModeNoise {
			return ActionBatch{}
		}
		a.held[event.Note].down = false
		return a.reassign(event.At)
	default:
		return ActionBatch{}
	}
}

func (a *Allocator) parameterActions(event PerformanceEvent) ActionBatch {
	var batch ActionBatch
	armed := a.armedChannels()
	for channel, ok := range armed {
		physical := uint8(channel + 1)
		if ok && event.Parameter.AppliesToChannel(physical) {
			batch.append(ChannelAction{At: event.At, Kind: ActionParameter, Channel: physical, Parameter: event.Parameter, Value: event.Value})
		}
	}
	return batch
}

func (a *Allocator) reassign(at time.Duration) ActionBatch {
	var candidates [128]uint8
	n := 0
	for note, state := range a.held {
		if state.down {
			candidates[n] = uint8(note)
			n++
		}
	}
	capacity := 1
	channels := [3]uint8{a.monoChannel, 0, 0}
	switch a.mode {
	case ModePoly2:
		capacity, channels = 2, [3]uint8{1, 2, 0}
	case ModePoly3:
		capacity, channels = 3, [3]uint8{1, 2, 3}
	}
	// Voice stealing is recency based. Pitch sorting is applied only after the
	// active set is chosen, giving stable high/middle/low channel roles.
	sort.Slice(candidates[:n], func(i, j int) bool {
		return a.held[candidates[i]].order > a.held[candidates[j]].order
	})
	if n > capacity {
		n = capacity
	}
	sort.Slice(candidates[:n], func(i, j int) bool { return candidates[i] > candidates[j] })

	next := [4]int16{-1, -1, -1, -1}
	for i := 0; i < n; i++ {
		next[channels[i]-1] = int16(candidates[i])
	}
	var batch ActionBatch
	for channel := range a.assigned {
		if a.assigned[channel] >= 0 && a.assigned[channel] != next[channel] {
			batch.append(ChannelAction{At: at, Kind: ActionStop, Channel: uint8(channel + 1), Note: uint8(a.assigned[channel])})
		}
	}
	for channel := range next {
		if next[channel] >= 0 && a.assigned[channel] != next[channel] {
			batch.append(ChannelAction{At: at, Kind: ActionStart, Channel: uint8(channel + 1), Note: uint8(next[channel])})
		}
	}
	a.assigned = next
	return batch
}

func (a *Allocator) stopAssigned(at time.Duration) ActionBatch {
	var batch ActionBatch
	for channel, note := range a.assigned {
		if note >= 0 {
			batch.append(ChannelAction{At: at, Kind: ActionStop, Channel: uint8(channel + 1), Note: uint8(note)})
			a.assigned[channel] = -1
		}
	}
	return batch
}

func (a *Allocator) clearHeld() {
	for i := range a.held {
		a.held[i] = heldNote{}
	}
}

func (a *Allocator) armedChannels() [4]bool {
	var channels [4]bool
	switch a.mode {
	case ModePoly2:
		channels[0], channels[1] = true, true
	case ModePoly3:
		channels[0], channels[1], channels[2] = true, true, true
	case ModeNoise:
		channels[3] = true
	default:
		channels[a.monoChannel-1] = true
	}
	return channels
}

func (a *Allocator) Snapshot() AllocationSnapshot {
	snapshot := AllocationSnapshot{Mode: a.mode, MonoChannel: a.monoChannel, Armed: a.armedChannels()}
	for _, state := range a.held {
		if state.down {
			snapshot.Held++
		}
	}
	for channel, note := range a.assigned {
		if note >= 0 {
			snapshot.Active[channel], snapshot.Notes[channel] = true, uint8(note)
		}
	}
	return snapshot
}
