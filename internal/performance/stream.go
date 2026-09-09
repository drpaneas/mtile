package performance

import (
	"encoding/binary"
	"io"
	"math"
	"sync"
	"sync/atomic"

	"github.com/drpaneas/mtile/internal/gbapu"
	runtimeaudio "github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

const (
	liveQueueSize            = 256
	liveQueueMask            = liveQueueSize - 1
	liveMonitorReleaseMillis = 5
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
	active bool
	note   uint8
	sound  runtimeaudio.Sound

	frequency float64

	lfsr           uint16
	elapsed        uint64
	sweepEvery     uint64
	period         int
	noiseDivisor   uint8
	noiseShift     uint8
	macroStep      uint8
	macroRemainder uint64
	macroActive    bool
	macroBaseNote  uint8
	maxSamples     uint64
}

// Stream is an infinite 48 kHz-style PCM reader. Producers enqueue complete
// physical channel commands; the audio reader owns oscillator state and never
// locks or allocates while filling a buffer.
type Stream struct {
	sampleRate     int
	queue          [liveQueueSize]liveCommand
	head           atomic.Uint32
	tail           atomic.Uint32
	producerMu     sync.Mutex
	dropped        atomic.Uint32
	output         atomic.Uint32
	closed         atomic.Bool
	voices         [4]liveVoice
	snapshot       [4]atomic.Uint32
	renderer       *gbapu.Renderer
	rendererOutput uint32
	renderLeft     [1]int16
	renderRight    [1]int16
}

func NewStream(sampleRate int) *Stream {
	if sampleRate < 8_000 {
		sampleRate = 48_000
	}
	stream := &Stream{
		sampleRate:     sampleRate,
		renderer:       gbapu.NewRenderer(sampleRate),
		rendererOutput: ^uint32(0),
	}
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

// syncRendererOutput is called exclusively by the audio reader. SetOutput is
// allowed from the editor goroutine, but the renderer and its blip buffers
// remain single-owner state throughout playback.
func (s *Stream) syncRendererOutput() {
	packed := s.output.Load()
	if s.renderer == nil || packed == s.rendererOutput {
		return
	}
	ratio := 100
	switch (packed >> 6) & 3 {
	case 0:
		ratio = 25
	case 1:
		ratio = 50
	}
	s.renderer.SetOutput(int(packed&7), int((packed>>3)&7), ratio)
	s.rendererOutput = packed
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
	if voice.active {
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
			if s.renderer != nil {
				s.renderer.StopAll()
			}
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
	*voice = liveVoice{
		active: true, note: note, sound: sound, lfsr: 0x7fff,
		noiseDivisor: sound.Noise.Divisor, noiseShift: sound.Noise.Shift,
		macroActive: len(sound.Macro.Steps) != 0, macroBaseNote: note,
	}
	voice.maxSamples = liveHardwareLengthSamples(sound, s.sampleRate)
	if !runtimeaudio.NativeSoundNoteValid(sound, note) {
		voice.active = false
		s.publish(channel)
		return
	}
	if s.renderer != nil && !s.renderer.Start(uint8(channel+1), note, sound) {
		voice.active = false
		s.publish(channel)
		return
	}
	if sound.Kind != runtimeaudio.SoundCH4 {
		period, ok := runtimeaudio.NativeSoundPeriod(sound, note)
		if !ok {
			voice.active = false
			s.publish(channel)
			return
		}
		voice.period = int(period)
	}
	if !liveApplyMacroStep(voice) {
		voice.active = false
		s.publish(channel)
		return
	}
	liveUpdateToneFrequency(voice)
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
	if s.renderer != nil {
		// Note-off timing remains exact in the allocator and recorder. Only the
		// desktop monitor receives this tiny de-click tail; cartridge playback
		// and explicit PSG Stop calls keep their hardware behavior.
		frames := max(1, s.sampleRate*liveMonitorReleaseMillis/1000)
		s.renderer.Release(uint8(channel+1), frames)
	}
	voice.active = false
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
		s.syncRendererOutput()
		// Keep the public live-state snapshot and parameter semantics moving in
		// lockstep with the audio callback. Audible samples come from the shared
		// cycle-timed AGB renderer below.
		for channel := range s.voices {
			s.advanceVoiceState(channel)
		}
		s.renderLeft[0], s.renderRight[0] = 0, 0
		if s.renderer != nil {
			s.renderer.Render(s.renderLeft[:], s.renderRight[:])
			for channel := range s.voices {
				if s.voices[channel].active && !s.renderer.Active(uint8(channel+1)) {
					s.voices[channel].active = false
					s.publish(channel)
				}
			}
		}
		offset := frame * 4
		binary.LittleEndian.PutUint16(target[offset:], uint16(s.renderLeft[0]))
		binary.LittleEndian.PutUint16(target[offset+2:], uint16(s.renderRight[0]))
	}
	for i := frames * 4; i < len(target); i++ {
		target[i] = 0
	}
	return len(target), nil
}

var _ io.Reader = (*Stream)(nil)

func (s *Stream) advanceVoiceState(channel int) {
	if channel < 0 || channel >= len(s.voices) {
		return
	}
	voice := &s.voices[channel]
	if !voice.active {
		return
	}
	if voice.maxSamples != 0 && voice.elapsed >= voice.maxSamples {
		voice.active = false
		s.publish(channel)
		return
	}
	if voice.sweepEvery != 0 && voice.elapsed != 0 && voice.elapsed%voice.sweepEvery == 0 {
		delta := voice.period >> voice.sound.Pulse1.Sweep.Shift
		if voice.sound.Pulse1.Sweep.Direction == hw.SweepDown {
			voice.period -= delta
		} else {
			voice.period += delta
		}
		if voice.period < 0 || voice.period > 2047 {
			voice.active = false
			s.publish(channel)
			return
		}
		liveUpdateToneFrequency(voice)
	}
	voice.elapsed++
	if !liveAdvanceMacro(voice, s.sampleRate) {
		voice.active = false
		s.publish(channel)
	}
}

func liveUpdateToneFrequency(voice *liveVoice) {
	if voice == nil || voice.sound.Kind == runtimeaudio.SoundCH4 || voice.period < 0 || voice.period > 2047 {
		return
	}
	clock := 131072.0
	if voice.sound.Kind == runtimeaudio.SoundCH3 {
		waveLength := max(1, int(voice.sound.Wave.Wave.Length))
		clock = 2097152.0 / float64(waveLength)
	}
	voice.frequency = clock / float64(2048-voice.period)
}

func liveAdvanceMacro(voice *liveVoice, sampleRate int) bool {
	if voice == nil || !voice.macroActive {
		return true
	}
	voice.macroRemainder += uint64(hw.GBAClockHz)
	threshold := uint64(sampleRate) * uint64(runtimeaudio.NativeMacroClockCycles)
	if voice.macroRemainder < threshold {
		return true
	}
	voice.macroRemainder -= threshold
	return liveApplyMacroStep(voice)
}

func liveApplyMacroStep(voice *liveVoice) bool {
	if voice == nil || !voice.macroActive {
		return true
	}
	if int(voice.macroStep) >= len(voice.sound.Macro.Steps) {
		voice.macroActive = false
		return true
	}
	step := voice.sound.Macro.Steps[voice.macroStep]
	if step.PeriodDelta != 0 {
		voice.period += int(step.PeriodDelta)
		if voice.period < 0 || voice.period > 2047 {
			voice.macroActive = false
			return false
		}
		liveUpdateToneFrequency(voice)
	}
	if step.HasNoise {
		voice.noiseDivisor = step.NoiseDivisor
		voice.noiseShift = step.NoiseShift
	}
	if step.HasNoiseNote {
		divisor, shift, ok := runtimeaudio.NativeGBStudioNoisePolynomial(voice.macroBaseNote, step.NoiseNoteOffset)
		if !ok {
			voice.macroActive = false
			return false
		}
		voice.noiseDivisor = divisor
		voice.noiseShift = shift
	}
	next := int(voice.macroStep) + 1
	if next >= len(voice.sound.Macro.Steps) {
		if voice.sound.Macro.LoopStart < 0 {
			voice.macroActive = false
			return true
		}
		next = int(voice.sound.Macro.LoopStart)
	}
	voice.macroStep = uint8(next)
	return true
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

func liveNoiseFrequency(divisor, shift uint8) float64 {
	ratio := .5
	if divisor != 0 {
		ratio = float64(divisor)
	}
	return 262144 / (ratio * math.Pow(2, float64(shift)))
}
