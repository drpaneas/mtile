package performance

import (
	"encoding/binary"
	"io"
	"math"
	"sync"
	"sync/atomic"

	runtimeaudio "github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

const (
	liveQueueSize   = 256
	liveQueueMask   = liveQueueSize - 1
	liveReleaseMS   = 1
	liveMixHeadroom = 4
)

type liveCommandKind uint8

const (
	liveStart liveCommandKind = iota
	liveStop
	liveUpdate
	liveStopAll
)

type liveCommand struct {
	kind    liveCommandKind
	channel uint8
	note    uint8
	sound   runtimeaudio.Sound
}

type liveVoice struct {
	active        bool
	note          uint8
	sound         runtimeaudio.Sound
	phase         float64
	frequency     float64
	noisePhase    float64
	lfsr          uint16
	elapsed       uint64
	sweepEvery    uint64
	period        int
	release       int
	releaseLength int
	maxSamples    uint64
}

// Stream is an infinite 48 kHz-style PCM reader. Producers enqueue complete
// physical channel commands; the audio reader owns oscillator state and never
// locks or allocates while filling a buffer.
type Stream struct {
	sampleRate int
	queue      [liveQueueSize]liveCommand
	head       atomic.Uint32
	tail       atomic.Uint32
	producerMu sync.Mutex
	dropped    atomic.Uint32
	output     atomic.Uint32
	closed     atomic.Bool
	voices     [4]liveVoice
	snapshot   [4]atomic.Uint32
}

func NewStream(sampleRate int) *Stream {
	if sampleRate < 8_000 {
		sampleRate = 48_000
	}
	stream := &Stream{sampleRate: sampleRate}
	stream.SetOutput(7, 7, 100)
	return stream
}

// SetOutput applies the same project-wide SOUNDCNT_L/H terminal levels used
// by the cartridge. Packing them in one atomic keeps the callback lock-free.
func (s *Stream) SetOutput(leftLevel, rightLevel, psgRatio int) {
	if s == nil {
		return
	}
	leftLevel = min(max(leftLevel, 0), 7)
	rightLevel = min(max(rightLevel, 0), 7)
	ratioCode := uint32(2)
	switch psgRatio {
	case 25:
		ratioCode = 0
	case 50:
		ratioCode = 1
	}
	s.output.Store(uint32(leftLevel) | uint32(rightLevel)<<3 | ratioCode<<6)
}

func (s *Stream) enqueue(command liveCommand) bool {
	if s == nil || s.closed.Load() {
		return false
	}
	s.producerMu.Lock()
	defer s.producerMu.Unlock()
	tail := s.tail.Load()
	next := (tail + 1) & liveQueueMask
	if next == s.head.Load() {
		s.dropped.Add(1)
		return false
	}
	s.queue[tail] = command
	s.tail.Store(next)
	return true
}

func (s *Stream) Start(channel, note uint8, sound runtimeaudio.Sound) bool {
	return s.enqueue(liveCommand{kind: liveStart, channel: channel, note: note, sound: sound})
}

func (s *Stream) Stop(channel uint8) bool {
	return s.enqueue(liveCommand{kind: liveStop, channel: channel})
}

func (s *Stream) Update(channel uint8, sound runtimeaudio.Sound) bool {
	return s.enqueue(liveCommand{kind: liveUpdate, channel: channel, sound: sound})
}

func (s *Stream) StopAll() bool { return s.enqueue(liveCommand{kind: liveStopAll}) }

func (s *Stream) Close() error {
	if s != nil {
		s.closed.Store(true)
	}
	return nil
}

func (s *Stream) Dropped() uint32 {
	if s == nil {
		return 0
	}
	return s.dropped.Load()
}

func (s *Stream) ChannelSnapshot() (active [4]bool, notes [4]uint8) {
	if s == nil {
		return active, notes
	}
	for channel := range s.snapshot {
		packed := s.snapshot[channel].Load()
		active[channel] = packed&(1<<31) != 0
		notes[channel] = uint8(packed)
	}
	return active, notes
}

func (s *Stream) publish(channel int) {
	voice := &s.voices[channel]
	packed := uint32(voice.note)
	if voice.active || voice.release > 0 {
		packed |= 1 << 31
	}
	s.snapshot[channel].Store(packed)
}

func (s *Stream) drain() {
	for {
		head := s.head.Load()
		if head == s.tail.Load() {
			return
		}
		command := s.queue[head]
		s.head.Store((head + 1) & liveQueueMask)
		if command.kind == liveStopAll {
			for channel := range s.voices {
				s.release(channel)
			}
			continue
		}
		if command.channel < 1 || command.channel > 4 {
			continue
		}
		channel := int(command.channel - 1)
		switch command.kind {
		case liveStart:
			s.start(channel, command.note, command.sound)
		case liveStop:
			s.release(channel)
		case liveUpdate:
			voice := &s.voices[channel]
			if voice.active {
				s.start(channel, voice.note, command.sound)
			} else {
				voice.sound = command.sound
			}
		}
	}
}

func (s *Stream) start(channel int, note uint8, sound runtimeaudio.Sound) {
	voice := &s.voices[channel]
	*voice = liveVoice{active: true, note: note, sound: sound, lfsr: 0x7fff}
	voice.releaseLength = max(1, s.sampleRate*liveReleaseMS/1000)
	voice.maxSamples = liveHardwareLengthSamples(sound, s.sampleRate)
	if sound.Kind != runtimeaudio.SoundCH4 {
		period, ok := runtimeaudio.NativeSoundPeriod(sound, note)
		if !ok {
			voice.active = false
			s.publish(channel)
			return
		}
		voice.period = int(period)
		clock := 131072.0
		if sound.Kind == runtimeaudio.SoundCH3 {
			waveLength := max(1, int(sound.Wave.Wave.Length))
			clock = 2097152.0 / float64(waveLength)
		}
		voice.frequency = clock / float64(2048-voice.period)
	}
	if sound.Kind == runtimeaudio.SoundCH1 && sound.Pulse1.Sweep.Pace != 0 {
		voice.sweepEvery = uint64(max(1, s.sampleRate*int(sound.Pulse1.Sweep.Pace)/128))
	}
	s.publish(channel)
}

func (s *Stream) release(channel int) {
	voice := &s.voices[channel]
	if !voice.active {
		return
	}
	voice.active = false
	voice.release = voice.releaseLength
	s.publish(channel)
}

func (s *Stream) Read(target []byte) (int, error) {
	if s == nil || s.closed.Load() {
		for i := range target {
			target[i] = 0
		}
		return len(target), nil
	}
	frames := len(target) / 4
	for frame := 0; frame < frames; frame++ {
		// Command polling every frame keeps callback-to-state latency below one
		// sample once the host requests the next buffer.
		s.drain()
		left, right := int32(0), int32(0)
		for channel := range s.voices {
			value := s.sample(channel)
			route := liveSoundRoute(s.voices[channel].sound)
			if route == hw.SoundRouteLeft || route == hw.SoundRouteBoth {
				left += int32(value)
			}
			if route == hw.SoundRouteRight || route == hw.SoundRouteBoth {
				right += int32(value)
			}
		}
		left, right = left/liveMixHeadroom, right/liveMixHeadroom
		output := s.output.Load()
		leftLevel := int32(output & 7)
		rightLevel := int32((output >> 3) & 7)
		ratio := int32(100)
		switch (output >> 6) & 3 {
		case 0:
			ratio = 25
		case 1:
			ratio = 50
		}
		left = left * (leftLevel + 1) * ratio / (8 * 100)
		right = right * (rightLevel + 1) * ratio / (8 * 100)
		left = min(max(left, -32767), 32767)
		right = min(max(right, -32767), 32767)
		offset := frame * 4
		binary.LittleEndian.PutUint16(target[offset:], uint16(int16(left)))
		binary.LittleEndian.PutUint16(target[offset+2:], uint16(int16(right)))
	}
	for i := frames * 4; i < len(target); i++ {
		target[i] = 0
	}
	return len(target), nil
}

var _ io.Reader = (*Stream)(nil)

func (s *Stream) sample(channel int) int16 {
	voice := &s.voices[channel]
	if !voice.active && voice.release <= 0 {
		return 0
	}
	if voice.maxSamples != 0 && voice.elapsed >= voice.maxSamples {
		voice.active = false
		voice.release = 0
		s.publish(channel)
		return 0
	}
	if voice.sweepEvery != 0 && voice.elapsed != 0 && voice.elapsed%voice.sweepEvery == 0 {
		delta := voice.period >> voice.sound.Pulse1.Sweep.Shift
		if voice.sound.Pulse1.Sweep.Direction == hw.SweepDown {
			voice.period -= delta
		} else {
			voice.period += delta
		}
		if voice.period < 0 || voice.period > 2047 {
			voice.active, voice.release = false, 0
			s.publish(channel)
			return 0
		}
		voice.frequency = 131072 / float64(2048-voice.period)
	}
	voice.phase += voice.frequency / float64(s.sampleRate)
	voice.phase -= math.Floor(voice.phase)
	value, volume := 0.0, 1.0
	switch voice.sound.Kind {
	case runtimeaudio.SoundCH1:
		value = livePulse(voice.phase, voice.sound.Pulse1.Pulse.Duty)
		volume = liveEnvelope(voice.elapsed, s.sampleRate, voice.sound.Pulse1.Pulse.Envelope)
	case runtimeaudio.SoundCH2:
		value = livePulse(voice.phase, voice.sound.Pulse2.Duty)
		volume = liveEnvelope(voice.elapsed, s.sampleRate, voice.sound.Pulse2.Envelope)
	case runtimeaudio.SoundCH3:
		wave := voice.sound.Wave.Wave
		if wave.Length != 0 {
			index := int(voice.phase*float64(wave.Length)) % int(wave.Length)
			value = (float64(wave.Samples[index]) - 7.5) / 7.5
		}
		volume = liveWaveLevel(voice.sound.Wave.Level)
	case runtimeaudio.SoundCH4:
		frequency := liveNoiseFrequency(voice.sound.Noise.Divisor, voice.sound.Noise.Shift)
		voice.noisePhase += frequency / float64(s.sampleRate)
		for voice.noisePhase >= 1 {
			bit := (voice.lfsr ^ (voice.lfsr >> 1)) & 1
			voice.lfsr = (voice.lfsr >> 1) | (bit << 14)
			if voice.sound.Noise.Width == hw.Noise7Bit {
				voice.lfsr = (voice.lfsr &^ (1 << 6)) | (bit << 6)
			}
			voice.noisePhase--
		}
		if voice.lfsr&1 == 0 {
			value = 1
		} else {
			value = -1
		}
		volume = liveEnvelope(voice.elapsed, s.sampleRate, voice.sound.Noise.Envelope)
	}
	if voice.release > 0 {
		volume *= float64(voice.release) / float64(voice.releaseLength)
		voice.release--
		if voice.release == 0 {
			s.publish(channel)
		}
	}
	voice.elapsed++
	return int16(value * volume * 28_000)
}

func liveHardwareLengthSamples(sound runtimeaudio.Sound, sampleRate int) uint64 {
	switch sound.Kind {
	case runtimeaudio.SoundCH1:
		if sound.Pulse1.Pulse.UseLength {
			return uint64(max(1, (64-int(sound.Pulse1.Pulse.Length))*sampleRate/256))
		}
	case runtimeaudio.SoundCH2:
		if sound.Pulse2.UseLength {
			return uint64(max(1, (64-int(sound.Pulse2.Length))*sampleRate/256))
		}
	case runtimeaudio.SoundCH3:
		if sound.Wave.UseLength {
			return uint64(max(1, (256-int(sound.Wave.Length))*sampleRate/256))
		}
	case runtimeaudio.SoundCH4:
		if sound.Noise.UseLength {
			return uint64(max(1, (64-int(sound.Noise.Length))*sampleRate/256))
		}
	}
	return 0
}

func livePulse(phase float64, duty hw.PulseDuty) float64 {
	fraction := .75
	switch duty {
	case hw.PulseDuty12_5:
		fraction = .125
	case hw.PulseDuty25:
		fraction = .25
	case hw.PulseDuty50:
		fraction = .5
	}
	if phase < fraction {
		return 1
	}
	return -1
}

func liveEnvelope(sample uint64, sampleRate int, envelope hw.HardwareEnvelope) float64 {
	level := int(envelope.Initial)
	if envelope.Pace != 0 {
		steps := int(sample / uint64(max(1, sampleRate*int(envelope.Pace)/64)))
		if envelope.Direction == hw.EnvelopeUp {
			level = min(15, level+steps)
		} else {
			level = max(0, level-steps)
		}
	}
	return float64(level) / 15
}

func liveWaveLevel(level hw.WaveLevel) float64 {
	switch level {
	case hw.Wave100:
		return 1
	case hw.Wave50:
		return .5
	case hw.Wave25:
		return .25
	case hw.Wave75:
		return .75
	default:
		return 0
	}
}

func liveNoiseFrequency(divisor, shift uint8) float64 {
	ratio := .5
	if divisor != 0 {
		ratio = float64(divisor)
	}
	return 262144 / (ratio * math.Pow(2, float64(shift+1)))
}

func liveSoundRoute(sound runtimeaudio.Sound) hw.SoundRoute {
	switch sound.Kind {
	case runtimeaudio.SoundCH1:
		return sound.Pulse1.Pulse.Route
	case runtimeaudio.SoundCH2:
		return sound.Pulse2.Route
	case runtimeaudio.SoundCH3:
		return sound.Wave.Route
	case runtimeaudio.SoundCH4:
		return sound.Noise.Route
	default:
		return hw.SoundRouteNone
	}
}
