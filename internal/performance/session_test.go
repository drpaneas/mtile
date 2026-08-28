package performance

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	runtimeaudio "github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

func testSoundBank() SoundBank {
	bank := SoundBank{}
	bank.Channels[0] = runtimeaudio.Sound{Kind: runtimeaudio.SoundCH1, Pulse1: hw.Pulse1Config{
		Pulse: hw.PulseConfig{Duty: hw.PulseDuty50, Envelope: hw.HardwareEnvelope{Initial: 15}, Route: hw.SoundRouteBoth},
	}}
	bank.Channels[1] = runtimeaudio.Sound{Kind: runtimeaudio.SoundCH2, Pulse2: hw.PulseConfig{
		Duty: hw.PulseDuty25, Envelope: hw.HardwareEnvelope{Initial: 15}, Route: hw.SoundRouteBoth,
	}}
	wave := PulseWaveform(50)
	bank.Channels[2] = runtimeaudio.Sound{Kind: runtimeaudio.SoundCH3, Wave: hw.WaveConfig{
		Wave: hw.Waveform{Length: 32}, Level: hw.Wave100, Route: hw.SoundRouteBoth,
	}}
	copy(bank.Channels[2].Wave.Wave.Samples[:32], wave[:])
	for index := range bank.Noise {
		bank.Noise[index] = runtimeaudio.Sound{Kind: runtimeaudio.SoundCH4, Noise: hw.NoiseConfig{
			Width: hw.Noise15Bit, Divisor: uint8(index%7 + 1), Shift: uint8(index + 2),
			Envelope: hw.HardwareEnvelope{Initial: 15, Pace: 1}, Route: hw.SoundRouteBoth,
		}}
	}
	bank.Channels[3] = bank.Noise[0]
	return bank
}

func TestLiveStreamRendersAndStopsWithoutPersistentTick(t *testing.T) {
	stream := NewStream(48_000)
	bank := testSoundBank()
	if !stream.Start(1, 69, bank.Channels[0]) {
		t.Fatal("start queue rejected")
	}
	pcm := make([]byte, 480*4)
	if _, err := stream.Read(pcm); err != nil {
		t.Fatal(err)
	}
	nonzero := false
	for i := 0; i < len(pcm); i += 2 {
		if binary.LittleEndian.Uint16(pcm[i:]) != 0 {
			nonzero = true
			break
		}
	}
	if !nonzero {
		t.Fatal("live pulse stream is silent")
	}
	stream.Stop(1)
	_, _ = stream.Read(pcm)
	_, _ = stream.Read(pcm)
	for _, value := range pcm {
		if value != 0 {
			t.Fatal("stopped stream retained a periodic background tick")
		}
	}
	active, _ := stream.ChannelSnapshot()
	if active[0] {
		t.Fatalf("stopped channel snapshot = %v", active)
	}
}

func TestLiveStreamHonorsHardwareRoute(t *testing.T) {
	stream := NewStream(48_000)
	sound := testSoundBank().Channels[0]
	sound.Pulse1.Pulse.Route = hw.SoundRouteLeft
	stream.Start(1, 69, sound)
	pcm := make([]byte, 64*4)
	_, _ = stream.Read(pcm)
	for frame := 0; frame < 64; frame++ {
		if binary.LittleEndian.Uint16(pcm[frame*4+2:]) != 0 {
			t.Fatal("left-routed PSG voice leaked into right output")
		}
	}
}

func TestLiveStreamHonorsGlobalPSGOutput(t *testing.T) {
	render := func(leftLevel, rightLevel, ratio int) (leftPeak, rightPeak int16) {
		stream := NewStream(48_000)
		stream.SetOutput(leftLevel, rightLevel, ratio)
		sound := testSoundBank().Channels[0]
		sound.Pulse1.Pulse.Route = hw.SoundRouteBoth
		stream.Start(1, 69, sound)
		pcm := make([]byte, 64*4)
		_, _ = stream.Read(pcm)
		for frame := 0; frame < 64; frame++ {
			left := absPCM16(int16(binary.LittleEndian.Uint16(pcm[frame*4:])))
			right := absPCM16(int16(binary.LittleEndian.Uint16(pcm[frame*4+2:])))
			leftPeak = max(leftPeak, left)
			rightPeak = max(rightPeak, right)
		}
		return leftPeak, rightPeak
	}

	fullLeft, fullRight := render(7, 7, 100)
	quietLeft, quietRight := render(0, 7, 25)
	if fullLeft == 0 || fullRight == 0 {
		t.Fatalf("full PSG output is silent: left=%d right=%d", fullLeft, fullRight)
	}
	if quietLeft == 0 || quietRight == 0 || quietLeft >= quietRight || quietRight >= fullRight {
		t.Fatalf("PSG output scaling = quiet %d/%d full %d/%d", quietLeft, quietRight, fullLeft, fullRight)
	}
}

func TestLiveStreamHonorsHardwareLengthCutoff(t *testing.T) {
	stream := NewStream(48_000)
	sound := testSoundBank().Channels[0]
	sound.Pulse1.Pulse.UseLength = true
	sound.Pulse1.Pulse.Length = 63 // one 256 Hz hardware-length tick
	stream.Start(1, 69, sound)

	const frames = 512
	pcm := make([]byte, frames*4)
	_, _ = stream.Read(pcm)
	cutoff := int(liveHardwareLengthSamples(sound, 48_000))
	if cutoff < 1 || cutoff >= frames {
		t.Fatalf("test cutoff = %d frames", cutoff)
	}
	for frame := cutoff; frame < frames; frame++ {
		if binary.LittleEndian.Uint32(pcm[frame*4:]) != 0 {
			t.Fatalf("hardware-length voice survived at frame %d (cutoff %d)", frame, cutoff)
		}
	}
	active, _ := stream.ChannelSnapshot()
	if active[0] {
		t.Fatalf("hardware-length channel remained active: %v", active)
	}
}

func TestLiveStreamKeepsCH3PitchAcrossWaveDimensions(t *testing.T) {
	renderFrequency := func(length uint8) (float64, int) {
		stream := NewStream(48_000)
		sound := testSoundBank().Channels[2]
		sound.Wave.Wave.Length = length
		for index := 0; index < int(length); index++ {
			sound.Wave.Wave.Samples[index] = uint8((index / max(1, int(length)/16)) & 15)
		}
		if !stream.Start(3, 57, sound) {
			t.Fatal("CH3 start queue rejected")
		}
		_, _ = stream.Read(make([]byte, 4))
		return stream.voices[2].frequency, stream.voices[2].period
	}
	freq32, period32 := renderFrequency(32)
	freq64, period64 := renderFrequency(64)
	if math.Abs(freq32-freq64) > 0.001 {
		t.Fatalf("CH3 A3 pitch changed with dimension: 32=%f Hz 64=%f Hz", freq32, freq64)
	}
	if period32 != 1750 || period64 != 1899 {
		t.Fatalf("CH3 periods = %d/%d, want 1750/1899", period32, period64)
	}
}

func absPCM16(value int16) int16 {
	if value < 0 {
		return -value
	}
	return value
}

func TestLiveStreamReadAllocatesNothing(t *testing.T) {
	stream := NewStream(48_000)
	stream.Start(1, 69, testSoundBank().Channels[0])
	pcm := make([]byte, 64*4)
	_, _ = stream.Read(pcm) // drain start outside measured loop
	if allocations := testing.AllocsPerRun(100, func() { _, _ = stream.Read(pcm) }); allocations != 0 {
		t.Fatalf("audio Read allocations = %g", allocations)
	}
}

func TestKeyboardAndMIDISourcesShareIdenticalSessionPath(t *testing.T) {
	run := func(source Source) AllocationSnapshot {
		session := NewSession(nil, testSoundBank())
		session.SetMode(ModePoly3, 1, 0)
		session.Arm(true, 0)
		for _, note := range []uint8{60, 64, 67} {
			session.Handle(PerformanceEvent{Kind: NoteOn, Source: source, Note: note})
		}
		return session.Snapshot().Allocation
	}
	keyboard, midi := run(SourceKeyboard), run(SourceMIDI)
	if keyboard != midi {
		t.Fatalf("keyboard allocation %#v != MIDI %#v", keyboard, midi)
	}
}

func TestControllerDrumPadUsesCH4WithoutDisturbingPoly3(t *testing.T) {
	session := NewSession(nil, testSoundBank())
	session.SetMode(ModePoly3, 1, 0)
	session.Arm(true, 0)
	session.Handle(PerformanceEvent{At: time.Millisecond, Kind: NoteOn, Source: SourceMIDI, Note: 60})
	batch := session.Handle(PerformanceEvent{At: 2 * time.Millisecond, Kind: NoteOn, Source: SourceController, Note: 38})
	if batch.Len != 1 || batch.Actions[0].Channel != 4 || batch.Actions[0].Note != 38 {
		t.Fatalf("poly drum pad = %#v", batch)
	}
	allocation := session.Snapshot().Allocation
	if !allocation.Active[0] || allocation.Notes[0] != 60 {
		t.Fatalf("drum pad disturbed poly allocation: %#v", allocation)
	}
}

func TestControllerDrumPadIsStoppedByPolyAllNotesOff(t *testing.T) {
	session := NewSession(nil, testSoundBank())
	session.SetMode(ModePoly3, 1, 0)
	session.Arm(true, 0)
	session.Handle(PerformanceEvent{At: time.Millisecond, Kind: NoteOn, Source: SourceController, Note: 36})
	batch := session.Handle(PerformanceEvent{At: 2 * time.Millisecond, Kind: AllNotesOff})
	if batch.Len != 1 || batch.Actions[0].Kind != ActionStop || batch.Actions[0].Channel != 4 {
		t.Fatalf("poly all-notes-off drum actions = %#v", batch.Slice())
	}
}

func TestControllerDrumPadIsStoppedAndRecordedOnDisconnect(t *testing.T) {
	session := NewSession(nil, testSoundBank())
	session.SetMode(ModePoly3, 1, 0)
	session.Arm(true, 0)
	session.StartRecording(0)
	session.Handle(PerformanceEvent{At: time.Millisecond, Kind: NoteOn, Source: SourceController, Note: 36})
	take, ok := session.Disconnect(2 * time.Millisecond)
	if !ok || len(take.Events) != 2 {
		t.Fatalf("drum disconnect take = %#v ok=%t", take, ok)
	}
	if stop := take.Events[1]; stop.Kind != ActionStop || stop.Channel != 4 {
		t.Fatalf("drum disconnect stop = %#v", stop)
	}
}

func TestLinkedDutyUpdatesPhysicalPulseAndWaveChannels(t *testing.T) {
	session := NewSession(nil, testSoundBank())
	session.SetMode(ModePoly3, 1, 0)
	session.Arm(true, 0)
	batch := session.Handle(PerformanceEvent{Kind: ParameterChange, Parameter: ParameterDuty, Value: 75})
	bank := session.SoundBank()
	if batch.Len != 3 || bank.Channels[0].Pulse1.Pulse.Duty != hw.PulseDuty75 || bank.Channels[1].Pulse2.Duty != hw.PulseDuty75 {
		t.Fatalf("linked pulse duty = batch %#v CH1 %v CH2 %v", batch.Slice(), bank.Channels[0].Pulse1.Pulse.Duty, bank.Channels[1].Pulse2.Duty)
	}
	for index, sample := range bank.Channels[2].Wave.Wave.Samples[:32] {
		want := uint8(0)
		if index < 24 {
			want = 15
		}
		if sample != want {
			t.Fatalf("linked CH3 pulse sample %d = %d, want %d", index, sample, want)
		}
	}
}

func TestParameterSemanticsAreExplicitAndStable(t *testing.T) {
	retrigger := []Parameter{ParameterDuty, ParameterWaveLevel, ParameterNoiseWidth, ParameterNoiseDivisor, ParameterNoiseShift}
	for _, parameter := range retrigger {
		if got := parameter.Semantics(); got != ParameterRetrigger || got.String() != "RETRIGGER" {
			t.Fatalf("parameter %d semantics = %s", parameter, got)
		}
	}
	nextNote := []Parameter{
		ParameterVolume, ParameterEnvelopePace, ParameterEnvelopeDirection,
		ParameterSweepPace, ParameterSweepDirection, ParameterSweepShift,
	}
	for _, parameter := range nextNote {
		if got := parameter.Semantics(); got != ParameterNextNote || got.String() != "NEXT_NOTE" {
			t.Fatalf("parameter %d semantics = %s", parameter, got)
		}
	}
	if got := ParameterImmediate.String(); got != "IMMEDIATE" {
		t.Fatalf("immediate semantics label = %q", got)
	}
}

func TestNextNoteParameterDoesNotRetriggerHeldPreviewVoice(t *testing.T) {
	stream := NewStream(48_000)
	session := NewSession(stream, testSoundBank())
	session.Arm(true, 0)
	session.Handle(PerformanceEvent{At: 0, Kind: NoteOn, Note: 60})
	pcm := make([]byte, 64*4)
	_, _ = stream.Read(pcm)
	before := stream.voices[0].elapsed

	session.Handle(PerformanceEvent{At: time.Millisecond, Kind: ParameterChange, Parameter: ParameterVolume, Value: 7})
	_, _ = stream.Read(pcm)
	if got := stream.voices[0].elapsed; got <= before {
		t.Fatalf("NEXT_NOTE volume reset held voice: elapsed %d -> %d", before, got)
	}
	if got := stream.voices[0].sound.Pulse1.Pulse.Envelope.Initial; got != 15 {
		t.Fatalf("NEXT_NOTE volume changed held sound to %d", got)
	}

	session.Handle(PerformanceEvent{At: 2 * time.Millisecond, Kind: NoteOff, Note: 60})
	_, _ = stream.Read(pcm)
	session.Handle(PerformanceEvent{At: 3 * time.Millisecond, Kind: NoteOn, Note: 62})
	_, _ = stream.Read(pcm)
	if got := stream.voices[0].sound.Pulse1.Pulse.Envelope.Initial; got != 7 {
		t.Fatalf("next trigger volume = %d, want 7", got)
	}
}

func TestRetriggerParameterRestartsHeldPreviewVoice(t *testing.T) {
	stream := NewStream(48_000)
	session := NewSession(stream, testSoundBank())
	session.Arm(true, 0)
	session.Handle(PerformanceEvent{At: 0, Kind: NoteOn, Note: 60})
	pcm := make([]byte, 64*4)
	_, _ = stream.Read(pcm)
	before := stream.voices[0].elapsed

	session.Handle(PerformanceEvent{At: time.Millisecond, Kind: ParameterChange, Parameter: ParameterDuty, Value: 75})
	_, _ = stream.Read(pcm[:4])
	if got := stream.voices[0].elapsed; got >= before {
		t.Fatalf("RETRIGGER duty preserved held phase: elapsed %d -> %d", before, got)
	}
	if got := stream.voices[0].sound.Pulse1.Pulse.Duty; got != hw.PulseDuty75 {
		t.Fatalf("retriggered duty = %v, want 75%%", got)
	}
}

func TestRecorderKeepsOnlyHardwareCompatibleLinkedParameters(t *testing.T) {
	session := NewSession(nil, testSoundBank())
	session.SetMode(ModePoly3, 1, 0)
	session.Arm(true, 0)
	if !session.StartRecording(0) {
		t.Fatal("recording did not start")
	}
	session.Handle(PerformanceEvent{At: time.Millisecond, Kind: ParameterChange, Parameter: ParameterEnvelopePace, Value: 4})
	session.Handle(PerformanceEvent{At: 2 * time.Millisecond, Kind: ParameterChange, Parameter: ParameterSweepShift, Value: 2})
	take, ok := session.StopRecording(3 * time.Millisecond)
	if !ok || len(take.Events) != 3 {
		t.Fatalf("linked parameter take = %#v ok=%t", take, ok)
	}
	want := []struct {
		channel   uint8
		parameter Parameter
	}{{1, ParameterEnvelopePace}, {2, ParameterEnvelopePace}, {1, ParameterSweepShift}}
	for index, event := range take.Events {
		if event.Channel != want[index].channel || event.Parameter != want[index].parameter {
			t.Fatalf("take event %d = %#v, want CH%d parameter %d", index, event, want[index].channel, want[index].parameter)
		}
	}
}

func TestDisconnectStopsVoicesAndFinalizesTake(t *testing.T) {
	session := NewSession(nil, testSoundBank())
	session.Arm(true, 0)
	session.StartRecording(0)
	session.Handle(PerformanceEvent{At: time.Millisecond, Kind: NoteOn, Note: 60})
	take, ok := session.Disconnect(time.Second)
	if !ok || len(take.Events) != 2 || session.Snapshot().Armed || session.Snapshot().Recording {
		t.Fatalf("disconnect = take %#v ok=%t state %#v", take, ok, session.Snapshot())
	}
}
