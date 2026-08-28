package emulator

import "sync"

// PCMStream is a bounded producer/consumer bridge from emulated frames to the
// host audio callback. The callback is never allowed to block the UI; a rare
// underrun emits silence and an overrun drops the oldest samples to keep audio
// synchronized with the newest video frame.
type PCMStream struct {
	mu        sync.Mutex
	data      []byte
	head      int
	size      int
	underruns uint64
	overruns  uint64
	closed    bool
}

func NewPCMStream(milliseconds int) *PCMStream {
	if milliseconds < 100 {
		milliseconds = 100
	}
	capacity := SampleRate * 4 * milliseconds / 1000
	capacity -= capacity % 4
	return &PCMStream{data: make([]byte, capacity)}
}

func (s *PCMStream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(s.data) == 0 {
		return len(p), nil
	}
	original := len(p)
	if len(p) > len(s.data) {
		p = p[len(p)-len(s.data):]
		s.overruns++
	}
	for s.size+len(p) > len(s.data) {
		drop := min(4, s.size)
		s.head = (s.head + drop) % len(s.data)
		s.size -= drop
		s.overruns++
	}
	tail := (s.head + s.size) % len(s.data)
	first := min(len(p), len(s.data)-tail)
	copy(s.data[tail:], p[:first])
	copy(s.data, p[first:])
	s.size += len(p)
	return original, nil
}

func (s *PCMStream) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range p {
		p[index] = 0
	}
	if s.closed || s.size == 0 {
		if !s.closed {
			s.underruns++
		}
		return len(p), nil
	}
	count := min(len(p), s.size)
	first := min(count, len(s.data)-s.head)
	copy(p, s.data[s.head:s.head+first])
	copy(p[first:], s.data[:count-first])
	s.head = (s.head + count) % len(s.data)
	s.size -= count
	if count < len(p) {
		s.underruns++
	}
	return len(p), nil
}

func (s *PCMStream) Stats() (queuedFrames int, underruns, overruns uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.size / 4, s.underruns, s.overruns
}

// Discard removes audio queued before a preview pause. Resuming from the
// newest emulated frame must not replay stale sound from the previous tab.
func (s *PCMStream) Discard() {
	s.mu.Lock()
	s.head = 0
	s.size = 0
	s.mu.Unlock()
}

func (s *PCMStream) Close() error {
	s.mu.Lock()
	s.closed = true
	s.size = 0
	s.mu.Unlock()
	return nil
}
