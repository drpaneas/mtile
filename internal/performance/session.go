package performance

import (
	"sync"
	"time"

	runtimeaudio "github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

type SoundBank struct {
	Channels [4]runtimeaudio.Sound
	Noise    [8]runtimeaudio.Sound
}

type SessionSnapshot struct {
	Allocation AllocationSnapshot
	Armed      bool
	Recording  bool
	StreamDrop uint32
}

// Session serializes keyboard and MIDI producers through one allocator and
// recorder. Its critical section is bounded by four channels and never touches
// the editor model or UI.
type Session struct {
	mu        sync.Mutex
	origin    time.Time
	allocator Allocator
	recorder  *Recorder
	stream    *Stream
	bank      SoundBank
	armed     bool
	recording bool
	drumLive  bool
}

func NewSession(stream *Stream, bank SoundBank) *Session {
	return &Session{
		origin: time.Now(), allocator: NewAllocator(ModeMono, 1),
		recorder: NewRecorder(DefaultTakeCapacity), stream: stream, bank: bank,
	}
}

func (s *Session) Now() time.Duration {
	if s == nil {
		return 0
	}
	return time.Since(s.origin)
}

func (s *Session) SetSoundBank(bank SoundBank) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.bank = bank
	s.mu.Unlock()
}

func (s *Session) Arm(armed bool, at time.Duration) ActionBatch {
	if s == nil {
		return ActionBatch{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.armed == armed {
		return ActionBatch{}
	}
	s.armed = armed
	if armed {
		return ActionBatch{}
	}
	batch := s.allocator.Process(PerformanceEvent{At: at, Kind: AllNotesOff})
	batch = s.stopLiveDrum(batch, at)
	s.submit(batch)
	if s.recording {
		s.recorder.Record(batch)
		s.recording = false
	}
	return batch
}

func (s *Session) SetMode(mode VoiceMode, monoChannel uint8, at time.Duration) ActionBatch {
	if s == nil {
		return ActionBatch{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	batch := s.allocator.SetMode(mode, monoChannel, at)
	batch = s.stopLiveDrum(batch, at)
	s.submit(batch)
	if s.recording {
		s.recorder.Record(batch)
	}
	return batch
}

func (s *Session) Handle(event PerformanceEvent) ActionBatch {
	if s == nil {
		return ActionBatch{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.armed && event.Kind != AllNotesOff {
		return ActionBatch{}
	}
	// Controller drum pads remain a real CH4 instrument while POLY2/POLY3
	// occupies CH1-CH3. The recorder still receives an ordinary physical CH4
	// action, never a synthetic drum or chord event.
	if event.Source == SourceController && event.Kind == NoteOn && event.Note >= 36 && event.Note <= 43 {
		var batch ActionBatch
		batch.append(ChannelAction{At: event.At, Kind: ActionStart, Channel: 4, Note: event.Note})
		s.drumLive = true
		s.submit(batch)
		if s.recording {
			s.recorder.Record(batch)
		}
		return batch
	}
	batch := s.allocator.Process(event)
	if event.Kind == AllNotesOff {
		batch = s.stopLiveDrum(batch, event.At)
	}
	if event.Kind == ParameterChange {
		for _, action := range batch.Slice() {
			applyParameter(&s.bank.Channels[action.Channel-1], action.Parameter, action.Value)
		}
	}
	s.submit(batch)
	if s.recording {
		s.recorder.Record(batch)
	}
	return batch
}

func (s *Session) StartRecording(at time.Duration) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.armed || s.recording {
		return false
	}
	s.recorder.Start(at)
	s.recording = true
	return true
}

func (s *Session) StopRecording(at time.Duration) (RawPerformanceTake, bool) {
	if s == nil {
		return RawPerformanceTake{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.recording {
		return RawPerformanceTake{}, false
	}
	s.recording = false
	return s.recorder.Stop(at), true
}

// RecordingSnapshotInto returns the unfinished raw take for live piano-roll
// feedback. The destination belongs to the caller and can be reused.
func (s *Session) RecordingSnapshotInto(dst []RawEvent, at time.Duration) (RawPerformanceTake, bool) {
	if s == nil {
		return RawPerformanceTake{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.recording {
		return RawPerformanceTake{}, false
	}
	return s.recorder.SnapshotInto(dst, at)
}

// Disconnect is the hotplug safety boundary. It returns the final take when a
// device disappears during recording and always silences every live channel.
func (s *Session) Disconnect(at time.Duration) (RawPerformanceTake, bool) {
	if s == nil {
		return RawPerformanceTake{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	batch := s.allocator.Process(PerformanceEvent{At: at, Kind: AllNotesOff})
	batch = s.stopLiveDrum(batch, at)
	s.submit(batch)
	s.armed = false
	if !s.recording {
		return RawPerformanceTake{}, false
	}
	s.recorder.Record(batch)
	s.recording = false
	return s.recorder.Stop(at), true
}

func (s *Session) stopLiveDrum(batch ActionBatch, at time.Duration) ActionBatch {
	if !s.drumLive {
		return batch
	}
	batch.append(ChannelAction{At: at, Kind: ActionStop, Channel: 4})
	s.drumLive = false
	return batch
}

func (s *Session) Snapshot() SessionSnapshot {
	if s == nil {
		return SessionSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := SessionSnapshot{Allocation: s.allocator.Snapshot(), Armed: s.armed, Recording: s.recording}
	if s.stream != nil {
		snapshot.StreamDrop = s.stream.Dropped()
	}
	return snapshot
}

func (s *Session) SoundBank() SoundBank {
	if s == nil {
		return SoundBank{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bank
}

func (s *Session) submit(batch ActionBatch) {
	if s.stream == nil {
		return
	}
	for _, action := range batch.Slice() {
		switch action.Kind {
		case ActionStart:
			sound := s.bank.Channels[action.Channel-1]
			if action.Channel == 4 && action.Note >= 36 && action.Note <= 43 {
				sound = s.bank.Noise[action.Note-36]
			}
			s.stream.Start(action.Channel, action.Note, sound)
		case ActionStop:
			s.stream.Stop(action.Channel)
		case ActionParameter:
			// NEXT_NOTE controls are still recorded at their exact raw time, but
			// deliberately do not disturb a held preview voice. The next Start
			// reads the already-updated bank. RETRIGGER is explicit and matches
			// the physical tracker event produced from the take.
			if action.Parameter.Semantics() != ParameterNextNote {
				s.stream.Update(action.Channel, s.bank.Channels[action.Channel-1])
			}
		}
	}
}

func applyParameter(sound *runtimeaudio.Sound, parameter Parameter, value int16) {
	if sound == nil {
		return
	}
	clamp := func(value int16, high int16) uint8 { return uint8(min(max(value, 0), high)) }
	switch parameter {
	case ParameterDuty:
		duty := hw.PulseDuty50
		switch value {
		case 12:
			duty = hw.PulseDuty12_5
		case 25:
			duty = hw.PulseDuty25
		case 75:
			duty = hw.PulseDuty75
		}
		switch sound.Kind {
		case runtimeaudio.SoundCH1:
			sound.Pulse1.Pulse.Duty = duty
		case runtimeaudio.SoundCH2:
			sound.Pulse2.Duty = duty
		case runtimeaudio.SoundCH3:
			wave := PulseWaveform(int(value))
			sound.Wave.Wave.Length = 32
			copy(sound.Wave.Wave.Samples[:32], wave[:])
		}
	case ParameterVolume:
		level := clamp(value, 15)
		switch sound.Kind {
		case runtimeaudio.SoundCH1:
			sound.Pulse1.Pulse.Envelope.Initial = level
		case runtimeaudio.SoundCH2:
			sound.Pulse2.Envelope.Initial = level
		case runtimeaudio.SoundCH4:
			sound.Noise.Envelope.Initial = level
		}
	case ParameterEnvelopePace:
		pace := clamp(value, 7)
		switch sound.Kind {
		case runtimeaudio.SoundCH1:
			sound.Pulse1.Pulse.Envelope.Pace = pace
		case runtimeaudio.SoundCH2:
			sound.Pulse2.Envelope.Pace = pace
		case runtimeaudio.SoundCH4:
			sound.Noise.Envelope.Pace = pace
		}
	case ParameterEnvelopeDirection:
		direction := hw.EnvelopeDown
		if value > 0 {
			direction = hw.EnvelopeUp
		}
		switch sound.Kind {
		case runtimeaudio.SoundCH1:
			sound.Pulse1.Pulse.Envelope.Direction = direction
		case runtimeaudio.SoundCH2:
			sound.Pulse2.Envelope.Direction = direction
		case runtimeaudio.SoundCH4:
			sound.Noise.Envelope.Direction = direction
		}
	case ParameterSweepPace:
		if sound.Kind == runtimeaudio.SoundCH1 {
			sound.Pulse1.Sweep.Pace = clamp(value, 7)
		}
	case ParameterSweepDirection:
		if sound.Kind == runtimeaudio.SoundCH1 {
			sound.Pulse1.Sweep.Direction = hw.SweepDown
			if value > 0 {
				sound.Pulse1.Sweep.Direction = hw.SweepUp
			}
		}
	case ParameterSweepShift:
		if sound.Kind == runtimeaudio.SoundCH1 {
			sound.Pulse1.Sweep.Shift = clamp(value, 7)
		}
	case ParameterWaveLevel:
		if sound.Kind == runtimeaudio.SoundCH3 {
			levels := [...]hw.WaveLevel{hw.WaveMute, hw.Wave25, hw.Wave50, hw.Wave75, hw.Wave100}
			sound.Wave.Level = levels[clamp(value, 4)]
		}
	case ParameterNoiseWidth:
		if sound.Kind == runtimeaudio.SoundCH4 {
			sound.Noise.Width = hw.Noise15Bit
			if value != 0 {
				sound.Noise.Width = hw.Noise7Bit
			}
		}
	case ParameterNoiseDivisor:
		if sound.Kind == runtimeaudio.SoundCH4 {
			sound.Noise.Divisor = clamp(value, 7)
		}
	case ParameterNoiseShift:
		if sound.Kind == runtimeaudio.SoundCH4 {
			sound.Noise.Shift = clamp(value, 15)
		}
	}
}
