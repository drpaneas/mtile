package performance

import (
	"encoding/binary"
	"io"
	"math"
	"sync/atomic"
)

// ClickStream is a separate authoring-only metronome output. It never consumes
// CH4, so percussion remains available while the musician records POLY2/3.
type ClickStream struct {
	sampleRate int
	pending    atomic.Uint32
	remaining  int
	total      int
	phase      float64
	frequency  float64
	closed     atomic.Bool
}

func NewClickStream(sampleRate int) *ClickStream {
	if sampleRate <= 0 {
		sampleRate = 48_000
	}
	total := max(1, sampleRate/40) // 25 ms.
	return &ClickStream{sampleRate: sampleRate, total: total, frequency: 1320}
}

func (s *ClickStream) Trigger(accent bool) {
	if s == nil || s.closed.Load() {
		return
	}
	value := uint32(1)
	if accent {
		value = 2
	}
	s.pending.Store(value)
}

func (s *ClickStream) Read(target []byte) (int, error) {
	if s == nil || s.closed.Load() {
		clear(target)
		return len(target), nil
	}
	frames := len(target) / 4
	for frame := 0; frame < frames; frame++ {
		if pending := s.pending.Swap(0); pending != 0 {
			s.remaining, s.phase, s.frequency = s.total, 0, 1320
			if pending == 2 {
				s.frequency = 1760
			}
		}
		value := int16(0)
		if s.remaining > 0 {
			envelope := float64(s.remaining) / float64(s.total)
			value = int16(math.Sin(s.phase*2*math.Pi) * envelope * 16_000)
			s.phase += s.frequency / float64(s.sampleRate)
			s.phase -= math.Floor(s.phase)
			s.remaining--
		}
		offset := frame * 4
		binary.LittleEndian.PutUint16(target[offset:], uint16(value))
		binary.LittleEndian.PutUint16(target[offset+2:], uint16(value))
	}
	clear(target[frames*4:])
	return len(target), nil
}

func (s *ClickStream) Close() error {
	if s != nil {
		s.closed.Store(true)
	}
	return nil
}

var _ io.ReadCloser = (*ClickStream)(nil)
