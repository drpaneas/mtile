package audio

import (
	"testing"
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

func nativeTestWave() hw.WaveConfig {
	var wave hw.Waveform
	wave.Length = 32
	for i := 0; i < 32; i++ {
		wave.Samples[i] = uint8(i & 15)
	}
	return hw.WaveConfig{Wave: wave, Level: hw.Wave100, Route: hw.SoundRouteBoth}
}

func nativeTestLibrary() Library {
	envelope := hw.HardwareEnvelope{Initial: 15, Direction: hw.EnvelopeDown}
	sounds := []Sound{
		{ID: 10, Kind: SoundCH1, Pulse1: hw.Pulse1Config{Pulse: hw.PulseConfig{Duty: hw.PulseDuty50, Envelope: envelope, Route: hw.SoundRouteBoth}}},
		{ID: 11, Kind: SoundCH2, Pulse2: hw.PulseConfig{Duty: hw.PulseDuty25, Envelope: envelope, Route: hw.SoundRouteBoth}},
		{ID: 12, Kind: SoundCH3, Wave: nativeTestWave()},
		{ID: 13, Kind: SoundCH4, Noise: hw.NoiseConfig{Width: hw.Noise15Bit, Divisor: 3, Shift: 5, Envelope: envelope, Route: hw.SoundRouteBoth}},
	}
	clips := []Clip{
		{ID: 20, Kind: SoundCH1, LengthTicks: 4, NoteEvents: []NoteEvent{
			{Tick: 0, DurationTicks: 2, Note: 69, SoundID: 10, Priority: 100},
			{Tick: 2, DurationTicks: 2, Note: 72, SoundID: 10, Priority: 100},
		}},
		{ID: 21, Kind: SoundCH2, LengthTicks: 4, NoteEvents: []NoteEvent{
			{Tick: 1, DurationTicks: 2, Note: 57, SoundID: 11, Priority: 80},
		}},
		{ID: 22, Kind: SoundCH4, LengthTicks: 4, NoteEvents: []NoteEvent{
			{Tick: 0, DurationTicks: 1, SoundID: 13, Priority: 90},
		}},
	}
	part := Part{ID: 30, LengthTicks: 4, PSGClips: [4]int16{20, 21, -1, 22}}
	return Library{Sounds: sounds, Clips: clips, Parts: []Part{part}, Songs: []Song{{ID: 40, Tempo: 120, Sequence: []uint8{30}, LoopStart: -1}}}
}

func initNativeTestController(t *testing.T) *Controller {
	t.Helper()
	hw.ResetForTest()
	var controller Controller
	if !controller.Init(ProfilePSG) {
		t.Fatal("controller init failed")
	}
	if !controller.Bind(nativeTestLibrary()) {
		t.Fatal("controller bind failed")
	}
	return &controller
}

func TestNativeControllerRunsTypedSongAtRationalClock(t *testing.T) {
	c := initNativeTestController(t)
	if !c.PlaySong(40) {
		t.Fatal("PlaySong enqueue failed")
	}
	c.Service()
	stats := c.Stats()
	if !stats.RunningSong || stats.PartTick != 0 || !stats.Channels[0].Active || !stats.Channels[3].Active {
		t.Fatalf("tick-zero state = %+v", stats)
	}
	if got := hw.GetReg16(hw.SOUND1CNT_X) & 0x07FF; got != 1750 {
		t.Errorf("A4 period = %d, want 1750", got)
	}

	// At 120 BPM and 96 PPQN, 256 Hz commits advance exactly three ticks
	// every four Service calls. One call was used to start at tick zero.
	for i := 0; i < 4; i++ {
		c.Service()
	}
	stats = c.Stats()
	if stats.AbsoluteTick != 3 || stats.PartTick != 3 {
		t.Fatalf("after four clock services tick = %d/%d, want 3/3", stats.AbsoluteTick, stats.PartTick)
	}
	if !stats.Channels[0].Active || stats.Channels[0].Note != 72 {
		t.Errorf("second CH1 event not active: %+v", stats.Channels[0])
	}

	for i := 0; i < 2; i++ {
		c.Service()
	}
	stats = c.Stats()
	if stats.RunningSong || stats.CompletedSongs != 1 {
		t.Fatalf("ending song did not stop exactly: %+v", stats)
	}
}

func TestNativeControllerLoopsAtPartBoundaryWithoutDrift(t *testing.T) {
	library := nativeTestLibrary()
	library.Songs[0].LoopStart = 0
	hw.ResetForTest()
	var c Controller
	c.Init(ProfilePSG)
	if !c.Bind(library) || !c.PlaySong(40) {
		t.Fatal("loop fixture setup failed")
	}
	c.Service()
	for i := 0; i < 16; i++ {
		c.Service()
	}
	stats := c.Stats()
	if !stats.RunningSong || stats.AbsoluteTick != 12 || stats.PartTick != 0 || stats.SequenceIndex != 0 {
		t.Fatalf("three exact loops = %+v", stats)
	}
}

func TestNativeControllerAppliesPartTempoAtBoundary(t *testing.T) {
	library := nativeTestLibrary()
	library.Parts = append(library.Parts, Part{ID: 31, LengthTicks: 4, Tempo: 240, PSGClips: [4]int16{-1, -1, -1, -1}})
	library.Parts = append(library.Parts, Part{ID: 32, LengthTicks: 1, PSGClips: [4]int16{-1, -1, -1, -1}})
	library.Songs[0].Sequence = []uint8{30, 31, 32}
	hw.ResetForTest()
	var c Controller
	c.Init(ProfilePSG)
	if !c.Bind(library) || !c.PlaySong(40) {
		t.Fatal("part-tempo fixture setup failed")
	}
	c.Service()
	if c.tempo != 120 {
		t.Fatalf("inherited first-part tempo = %d", c.tempo)
	}
	for range 4 {
		c.advanceTick()
	}
	if c.sequencePos != 1 || c.tempo != 240 {
		t.Fatalf("second part position/tempo = %d/%d", c.sequencePos, c.tempo)
	}
	for range 4 {
		c.advanceTick()
	}
	if c.sequencePos != 2 || c.tempo != 120 {
		t.Fatalf("inherited third-part position/tempo = %d/%d", c.sequencePos, c.tempo)
	}
	c.advanceTick()
	if c.songActive || c.tempo != NativeDefaultTempo {
		t.Fatalf("ended song state/tempo = active %t tempo %d", c.songActive, c.tempo)
	}
}

func TestNativePartTempoUsesExistingTargetRecordPadding(t *testing.T) {
	if got := unsafe.Sizeof(Part{}); got != 16 {
		t.Fatalf("native part record = %d bytes, want 16", got)
	}
}

func TestNativeLegatoCrossesPartBoundaryWithoutStopOrRetrigger(t *testing.T) {
	library := nativeTestLibrary()
	library.Clips[0].LengthTicks = 1
	library.Clips[0].NoteEvents = []NoteEvent{{Tick: 0, DurationTicks: 1, Note: 69, SoundID: 10, Priority: 100}}
	library.Parts[0].LengthTicks = 1
	library.Parts[0].PSGClips = [4]int16{20, -1, -1, -1}
	library.Clips = append(library.Clips, Clip{ID: 23, Kind: SoundCH1, LengthTicks: 1, NoteEvents: []NoteEvent{
		{Tick: 0, DurationTicks: 1, Note: 72, SoundID: 10, Priority: 100, Legato: true},
	}})
	library.Parts = append(library.Parts, Part{ID: 31, LengthTicks: 1, PSGClips: [4]int16{23, -1, -1, -1}})
	library.Songs[0].Sequence = []uint8{30, 31}

	hw.ResetForTest()
	var c Controller
	c.Init(ProfilePSG)
	if !c.Bind(library) || !c.PlaySong(40) {
		t.Fatal("legato boundary fixture setup failed")
	}
	c.Service()
	if got := c.Stats().EventsApplied; got != 1 {
		t.Fatalf("first note events applied = %d, want 1", got)
	}
	c.advanceTick()
	stats := c.Stats()
	if !stats.Channels[0].Active || stats.Channels[0].Note != 72 || stats.SequenceIndex != 1 {
		t.Fatalf("boundary legato state = %+v", stats)
	}
	if stats.EventsApplied != 2 {
		t.Fatalf("legato boundary inserted a stop/retrigger: events=%d, want 2", stats.EventsApplied)
	}
}

func TestNativeOwnershipAndExplicitStopsAreIsolated(t *testing.T) {
	c := initNativeTestController(t)
	c.PlaySong(40)
	c.Service()

	// Lower-priority SFX cannot steal the active song note.
	c.PlaySFX(20, 50)
	c.Service()
	if got := c.Stats(); got.PriorityRejects != 1 || got.Channels[0].Owner != OwnerSong {
		t.Fatalf("low-priority SFX policy = %+v", got)
	}
	// A direct note always replaces the song owner and StopSong preserves it.
	c.NoteOn(1, 76, 10, 0)
	c.Service()
	c.StopSong()
	c.Service()
	if got := c.Stats().Channels[0]; !got.Active || got.Owner != OwnerDirect || got.Note != 76 {
		t.Fatalf("StopSong changed direct owner: %+v", got)
	}
	c.StopChannel(1)
	c.Service()
	if c.Stats().Channels[0].Active {
		t.Fatal("explicit native stop did not stop direct note")
	}
}

func TestNativeLibraryRejectsWrongKindsOverlapAndBadProfiles(t *testing.T) {
	hw.ResetForTest()
	var c Controller
	if c.Init(Profile(99)) {
		t.Fatal("invalid profile accepted")
	}
	c.Init(ProfilePSG)
	library := nativeTestLibrary()
	library.Clips[0].NoteEvents[0].SoundID = 11
	if c.Bind(library) {
		t.Fatal("CH1 clip accepted CH2 sound")
	}
	library = nativeTestLibrary()
	library.Clips[0].NoteEvents[1].Tick = 1
	if c.Bind(library) {
		t.Fatal("overlapping PSG notes accepted")
	}
	if c.Stats().InvalidAssets != 2 {
		t.Errorf("invalid asset telemetry = %d, want 2", c.Stats().InvalidAssets)
	}
}

func TestNativeCommandQueueIsBounded(t *testing.T) {
	c := initNativeTestController(t)
	for i := 0; i < NativeMaxCommandQueue; i++ {
		if !c.NoteOn(1, 69, 10, 1) {
			t.Fatalf("command %d unexpectedly rejected", i)
		}
	}
	if c.NoteOn(1, 69, 10, 1) {
		t.Fatal("queue overflow accepted")
	}
	if c.Stats().QueueOverflows != 1 {
		t.Errorf("queue overflow telemetry = %d", c.Stats().QueueOverflows)
	}
}

func TestNativePitchTablesCoverHardwareRanges(t *testing.T) {
	if _, ok := NativeTonePeriod(SoundCH1, 35); ok {
		t.Error("CH1 accepted note below pulse range")
	}
	if got, ok := NativeTonePeriod(SoundCH1, 69); !ok || got != 1750 {
		t.Errorf("CH1 A4 = %d/%v, want 1750/true", got, ok)
	}
	if got, ok := NativeTonePeriod(SoundCH3, 57); !ok || got != 1750 {
		t.Errorf("CH3 A3 = %d/%v, want 1750/true", got, ok)
	}
	wave64 := Sound{Kind: SoundCH3, Wave: nativeTestWave()}
	wave64.Wave.Wave.Length = 64
	if got, ok := NativeSoundPeriod(wave64, 57); !ok || got != 1899 {
		t.Errorf("64-sample CH3 A3 = %d/%v, want 1899/true", got, ok)
	}
	if got, ok := NativeSoundPeriod(wave64, 12); !ok || got != 44 {
		t.Errorf("64-sample CH3 C0 = %d/%v, want 44/true", got, ok)
	}
	if got, ok := NativeSoundPeriod(wave64, 127); !ok || got != 2045 {
		t.Errorf("64-sample CH3 G9 = %d/%v, want 2045/true", got, ok)
	}
	if low, high, ok := NativeSoundNoteRange(Sound{Kind: SoundCH1}); !ok || low != 36 || high != 127 {
		t.Errorf("pulse range = %d..%d/%v, want 36..127/true", low, high, ok)
	}
	wave32 := Sound{Kind: SoundCH3, Wave: nativeTestWave()}
	if low, high, ok := NativeSoundNoteRange(wave32); !ok || low != 24 || high != 127 {
		t.Errorf("32-sample CH3 range = %d..%d/%v, want 24..127/true", low, high, ok)
	}
	if low, high, ok := NativeSoundNoteRange(wave64); !ok || low != 12 || high != 127 {
		t.Errorf("64-sample CH3 range = %d..%d/%v, want 12..127/true", low, high, ok)
	}
	if _, _, ok := NativeSoundNoteRange(Sound{Kind: SoundCH4}); ok {
		t.Error("noise channel reported a pitched range")
	}
}

func TestNativeControllerPrograms64SampleWaveAtMusicalPitch(t *testing.T) {
	library := nativeTestLibrary()
	library.Sounds[2].Wave.Wave.Length = 64
	for index := 32; index < 64; index++ {
		library.Sounds[2].Wave.Wave.Samples[index] = uint8(index & 15)
	}
	hw.ResetForTest()
	var controller Controller
	if !controller.Init(ProfilePSG) || !controller.Bind(library) {
		t.Fatal("64-sample wave library did not bind")
	}
	if !controller.NoteOn(3, 57, 12, 1) {
		t.Fatal("64-sample CH3 note was rejected")
	}
	controller.Service()
	if got := hw.GetReg16(hw.SOUND3CNT_X) & 0x07ff; got != 1899 {
		t.Fatalf("64-sample CH3 A3 period = %d, want 1899", got)
	}
}

func TestNativeControllerOwnsExact256HzTimerAndStopsCleanly(t *testing.T) {
	c := initNativeTestController(t)
	if !c.Start() {
		t.Fatal("Start failed")
	}
	if got := hw.GetReg16(hw.TM0CNT_L); got != 0 {
		t.Fatalf("TM0 reload = %#04x, want 0", got)
	}
	if got := hw.GetReg16(hw.TM0CNT_H); got != nativeTimerEnabled {
		t.Fatalf("TM0 control = %#04x, want %#04x", got, nativeTimerEnabled)
	}
	if got := hw.GetReg16(hw.INTERRUPT_IE) & nativeTimer0IRQ; got == 0 {
		t.Fatal("Timer 0 IRQ was not enabled")
	}
	if !c.PlaySong(40) {
		t.Fatal("PlaySong enqueue failed")
	}
	serviceNativeTimerForTest()
	if got := c.Stats(); !got.ClockRunning || !got.RunningSong || got.ServiceCalls != 1 {
		t.Fatalf("IRQ-driven state = %+v", got)
	}
	// Manual servicing is disabled while the IRQ clock owns the controller.
	c.Service()
	if got := c.Stats().ServiceCalls; got != 1 {
		t.Fatalf("manual Service advanced running clock: %d", got)
	}
	c.Stop()
	if got := hw.GetReg16(hw.TM0CNT_H); got != 0 {
		t.Fatalf("TM0 still running after Stop: %#04x", got)
	}
	if got := hw.GetReg16(hw.INTERRUPT_IE) & nativeTimer0IRQ; got != 0 {
		t.Fatal("Timer 0 IRQ still enabled after Stop")
	}
	if got := c.Stats(); got.ClockRunning || got.RunningSong {
		t.Fatalf("controller still running after Stop: %+v", got)
	}
}

func TestNativeControllerAndLegacyStreamCannotShareTimers(t *testing.T) {
	c := initNativeTestController(t)
	activeMonoStream = &MonoStream{}
	if c.Start() {
		t.Fatal("native controller stole Timer 0 from legacy stream")
	}
	activeMonoStream = nil
	if !c.Start() {
		t.Fatal("native controller failed to claim free timer")
	}
	if claimMonoStream(&MonoStream{}) || claimStereoStream(&StereoStream{}) {
		t.Fatal("legacy stream stole timers from native controller")
	}
	c.Stop()
}

func TestSecondNativeControllerCannotReinitializeOrStopActiveHardware(t *testing.T) {
	first := initNativeTestController(t)
	if !first.Start() {
		t.Fatal("first controller failed to start")
	}
	first.NoteOn(1, 69, 10, 0)
	serviceNativeTimerForTest()
	want := hw.GetReg16(hw.SOUND1CNT_X)

	var second Controller
	if second.Init(ProfilePSG) {
		t.Fatal("second controller initialized while Timer 0 was owned")
	}
	second.Stop()
	if got := hw.GetReg16(hw.TM0CNT_H); got != nativeTimerEnabled {
		t.Fatalf("second controller stopped Timer 0: %#04x", got)
	}
	if got := hw.GetReg16(hw.SOUND1CNT_X); got != want {
		t.Fatalf("second controller changed CH1: %#04x want %#04x", got, want)
	}
	first.Stop()
}

func TestNativeSFXPlaysEveryEventAndFinishesWithoutSong(t *testing.T) {
	c := initNativeTestController(t)
	if !c.PlaySFX(20, 100) {
		t.Fatal("PlaySFX enqueue failed")
	}
	c.Service()
	if got := c.Stats(); !got.SFX[0].Active || got.Channels[0].Note != 69 || got.RunningSong {
		t.Fatalf("SFX tick zero = %+v", got)
	}
	for i := 0; i < 4; i++ {
		c.Service()
	}
	if got := c.Stats(); !got.SFX[0].Active || got.SFX[0].Tick != 3 || got.Channels[0].Note != 72 {
		t.Fatalf("SFX second event = %+v", got)
	}
	c.Service()
	if got := c.Stats(); got.SFX[0].Active || got.Channels[0].Active {
		t.Fatalf("completed SFX remained active = %+v", got)
	}
}

func TestNativeTimedDirectNoteStopsWithoutSong(t *testing.T) {
	c := initNativeTestController(t)
	c.NoteOn(2, 57, 11, 2)
	c.Service()
	if !c.Stats().Channels[1].Active {
		t.Fatal("direct note did not start")
	}
	for i := 0; i < 3; i++ {
		c.Service()
	}
	if got := c.Stats().Channels[1]; got.Active {
		t.Fatalf("timed direct note did not stop: %+v", got)
	}
}

func TestNativeSFXChannelsRunIndependentlyAndDirectCancelsOne(t *testing.T) {
	c := initNativeTestController(t)
	c.PlaySFX(20, 100)
	c.PlaySFX(21, 100)
	c.Service()
	if got := c.Stats(); !got.SFX[0].Active || !got.SFX[1].Active {
		t.Fatalf("independent effects did not start: %+v", got.SFX)
	}
	c.NoteOn(1, 76, 10, 0)
	c.Service()
	if got := c.Stats(); got.SFX[0].Active || !got.SFX[1].Active || got.Channels[0].Owner != OwnerDirect {
		t.Fatalf("direct ownership did not cancel only CH1 SFX: %+v", got)
	}
}

func TestNativeLongLoopKeepsExactRationalTickCount(t *testing.T) {
	library := nativeTestLibrary()
	library.Songs[0].Tempo = 137
	library.Songs[0].LoopStart = 0
	hw.ResetForTest()
	var c Controller
	c.Init(ProfilePSG)
	if !c.Bind(library) || !c.PlaySong(40) {
		t.Fatal("long-loop setup failed")
	}
	const services = 10_000
	for i := 0; i < services; i++ {
		c.Service()
	}
	wantTicks := uint32(uint64(services) * 137 * NativePPQN / (60 * 256))
	if got := c.Stats(); got.AbsoluteTick != wantTicks || got.PartTick != wantTicks%4 {
		t.Fatalf("rational clock = absolute %d part %d, want %d/%d", got.AbsoluteTick, got.PartTick, wantTicks, wantTicks%4)
	}
}

func TestNativeServiceAndPreviewAllocateNothing(t *testing.T) {
	c := initNativeTestController(t)
	var preview NativePreview
	if allocations := testing.AllocsPerRun(1_000, func() {
		c.Service()
		if !c.PreviewInto(&preview) {
			t.Fatal("PreviewInto rejected a valid destination")
		}
	}); allocations != 0 {
		t.Fatalf("Service/PreviewInto allocations = %v, want 0", allocations)
	}
	if got := c.Preview(); got != preview {
		t.Fatal("by-value Preview disagrees with PreviewInto")
	}
	if c.PreviewInto(nil) || (*Controller)(nil).PreviewInto(&preview) {
		t.Fatal("PreviewInto accepted a nil controller or destination")
	}
}

func TestNativeCycleTelemetryIgnoresImpossibleZeroSample(t *testing.T) {
	var c Controller
	c.recordServiceCycles(0)
	if c.measuredServices != 0 || c.minServiceCycles != 0 || c.totalServiceCycles != 0 {
		t.Fatalf("zero sample changed telemetry: %+v", c.statsLocked())
	}
	c.recordServiceCycles(117)
	c.recordServiceCycles(0)
	c.recordServiceCycles(819)
	stats := c.statsLocked()
	if stats.MeasuredServices != 2 || stats.MinServiceCycles != 117 ||
		stats.MeanServiceCycles != 468 || stats.MaxServiceCycles != 819 {
		t.Fatalf("filtered cycle telemetry = %+v", stats)
	}
}
