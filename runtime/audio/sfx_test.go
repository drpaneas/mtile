package audio

import (
	"slices"
	"testing"
	"unsafe"
)

func sfxFixture(t *testing.T) (*Mixer, *SFXPlayer) {
	t.Helper()
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{
		{ID: 2, Speed: 1, StepOffset: 0, StepCount: 3, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
		{ID: 7, Speed: 2, StepOffset: 3, StepCount: 3, LoopStart: 1, LoopEnd: 3},
	}
	steps := []SFXStep{
		{PhaseStep: 1 << 31, Volume: MaxVolume, Kind: SourceSquare},
		{PhaseStep: 1 << 30, Volume: MaxVolume, Kind: SourcePulse},
		{PhaseStep: 1 << 30, Volume: 0, Kind: SourceTriangle},
		{PhaseStep: 1 << 30, Volume: MaxVolume, Kind: SourceTriangle, Effect: SFXEffectSlide},
		{PhaseStep: 1 << 30, Volume: MaxVolume, Kind: SourceSaw},
		{PhaseStep: 1 << 30, Volume: MaxVolume, Kind: SourceNoise},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("SFXPlayer.Init failed")
	}
	return mixer, &player
}

func reverbFixture(t *testing.T, level uint8) (*Mixer, *SFXPlayer) {
	t.Helper()
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 0, Speed: 1, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
		Filters: SFXFilter(level * 24),
	}}
	steps := []SFXStep{{PhaseStep: 1, Volume: MaxVolume, Kind: SourceSquare}}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("reverb SFXPlayer.Init failed")
	}
	if _, ok := player.Play(0, 0, 0, 0); !ok {
		t.Fatal("reverb SFXPlayer.Play failed")
	}
	return mixer, &player
}

func TestPICOReverbUsesCapturedDelayAndFeedback(t *testing.T) {
	for _, test := range []struct {
		level     uint8
		delay     int
		firstEcho int8
		lastEcho  int8
	}{
		{1, sfxReverbDelayOne, 1, 1},
		{2, sfxReverbDelayTwo, 1, 1},
	} {
		_, player := reverbFixture(t, test.level)
		samples := make([]int8, test.delay*2+1)
		player.MixMono(samples)
		if samples[0] != 2 || samples[SFXSpeedUnitSamples-1] != 127 || samples[SFXSpeedUnitSamples] != 0 {
			t.Fatalf("reverb %d dry/note boundary = (%d,%d,%d), want (2,127,0)",
				test.level, samples[0], samples[SFXSpeedUnitSamples-1], samples[SFXSpeedUnitSamples])
		}
		if samples[test.delay-1] != 0 || samples[test.delay] != test.firstEcho || samples[test.delay*2] != test.lastEcho {
			t.Fatalf("reverb %d echoes = (%d,%d,%d), want (0,%d,%d)",
				test.level, samples[test.delay-1], samples[test.delay], samples[test.delay*2],
				test.firstEcho, test.lastEcho)
		}
	}
}

func TestPICOReverbCheckpointRestoresTail(t *testing.T) {
	_, player := reverbFixture(t, 1)
	player.MixMono(make([]int8, 173))
	if !player.saveCheckpoint(0) {
		t.Fatal("saveCheckpoint failed")
	}
	want := make([]int8, 700)
	player.MixMono(want)
	if !player.restoreCheckpoint(0) {
		t.Fatal("restoreCheckpoint failed")
	}
	got := make([]int8, len(want))
	player.MixMono(got)
	if !slices.Equal(got, want) {
		t.Fatal("restored reverb tail diverged from checkpointed timeline")
	}
}

func TestSFXPlayerAdvancesAtExactSampleBoundaries(t *testing.T) {
	_, player := sfxFixture(t)
	if channel, ok := player.Play(2, 0, 0, 0); !ok || channel != 0 {
		t.Fatalf("Play channel=%d ok=%t", channel, ok)
	}
	first := make([]int8, SFXSamplesForUnits(1))
	player.MixMono(first)
	if first[0] != 2 || first[1] != -6 {
		t.Fatalf("first step prefix = %v", first[:4])
	}
	state, _ := player.Channel(0)
	if state.Step != 0 || state.SamplesRemaining != 0 {
		t.Fatalf("state at boundary = %+v", state)
	}
	second := make([]int8, SFXSamplesForUnits(2)-SFXSamplesForUnits(1))
	player.MixMono(second)
	state, _ = player.Channel(0)
	if state.Step != 1 || state.SamplesRemaining != 0 || second[0] != 2 || second[1] != 5 || second[2] != -8 {
		t.Fatalf("second step state=%+v prefix=%v", state, second[:4])
	}
	third := make([]int8, SFXSamplesForUnits(3)-SFXSamplesForUnits(2))
	player.MixMono(third)
	for index, sample := range third {
		if sample != 0 {
			t.Fatalf("rest sample %d = %d", index, sample)
		}
	}
	player.MixMono(make([]int8, 1))
	if state, _ := player.Channel(0); state.Active {
		t.Fatalf("channel remains active after final step: %+v", state)
	}
}

func customInstrumentFixture(t *testing.T, secondEffect SFXEffect) (*Mixer, *SFXPlayer) {
	t.Helper()
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{
		{ID: 0, Speed: 3, StepOffset: 0, StepCount: 2, LoopStart: 0, LoopEnd: 2},
		{ID: 8, Speed: 8, StepOffset: 2, StepCount: 2, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
	}
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourceTriangle},
		{PhaseStep: picoPitchPhaseSteps[31], Volume: MaxVolume / 2, Kind: SourceSquare},
		{PhaseStep: picoPitchPhaseSteps[24], Volume: 7, Kind: SourceCustom0},
		{PhaseStep: picoPitchPhaseSteps[24], Volume: 7, Kind: SourceCustom0, Effect: secondEffect},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("custom SFXPlayer.Init failed")
	}
	return mixer, &player
}

func TestCustomInstrumentHasIndependentLoopClockAndTransposition(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{
		{ID: 0, Speed: 4, StepOffset: 0, StepCount: 2, LoopStart: 0, LoopEnd: 2},
		{ID: 8, Speed: 32, StepOffset: 2, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
	}
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourceTriangle},
		{PhaseStep: picoPitchPhaseSteps[31], Volume: MaxVolume / 2, Kind: SourceSquare},
		{PhaseStep: picoPitchPhaseSteps[30], Volume: 7, Kind: SourceCustom0},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("custom SFXPlayer.Init failed")
	}
	if _, ok := player.Play(8, 0, 0, 0); !ok {
		t.Fatal("custom SFXPlayer.Play failed")
	}
	state, _ := player.Channel(0)
	voice, _ := mixer.Voice(int(state.VoiceID))
	wantFirst := picoPitchPhaseSteps[30]
	if voice.Kind != SourceTriangle || voice.Step != wantFirst || voice.Volume != MaxVolume {
		t.Fatalf("first custom voice = %+v, want triangle step %d volume %d", voice, wantFirst, MaxVolume)
	}

	player.MixMono(make([]int8, SFXSamplesForUnits(4)+1))
	state, _ = player.Channel(0)
	voice, _ = mixer.Voice(int(state.VoiceID))
	wantSecond := picoPitchPhaseSteps[37]
	if player.channels[0].custom[0].step != 1 || voice.Kind != SourceSquare || voice.Step != wantSecond || voice.Volume != MaxVolume/2 {
		t.Fatalf("second custom state=%+v voice=%+v, want square step %d volume %d", player.channels[0].custom[0], voice, wantSecond, MaxVolume/2)
	}
}

func TestCustomInstrumentSustainsAcrossSamePitchAndDropInvertsRetrigger(t *testing.T) {
	for _, test := range []struct {
		name      string
		effect    SFXEffect
		retrigger bool
	}{
		{"ordinary", SFXEffectNone, false},
		{"drop", SFXEffectDrop, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, player := customInstrumentFixture(t, test.effect)
			if _, ok := player.Play(8, 0, 0, 0); !ok {
				t.Fatal("custom Play failed")
			}
			player.MixMono(make([]int8, SFXSamplesForUnits(8)))
			before := player.channels[0].custom[0].samplesRemaining
			if before == 0 {
				t.Fatal("inner clock unexpectedly landed on its boundary")
			}
			player.MixMono(make([]int8, 1))
			after := player.channels[0].custom[0].samplesRemaining
			if test.retrigger {
				if player.channels[0].custom[0].step != 0 || after <= before {
					t.Fatalf("drop did not retrigger custom state: before=%d after=%d state=%+v", before, after, player.channels[0].custom[0])
				}
			} else if after+1 != before {
				t.Fatalf("same-pitch note reset custom clock: before=%d after=%d", before, after)
			}
		})
	}
}

func TestCustomInstrumentChangeRetriggersAtSamePitch(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{
		{ID: 0, Speed: 8, StepOffset: 0, StepCount: 1, LoopStart: 0, LoopEnd: 1},
		{ID: 1, Speed: 8, StepOffset: 1, StepCount: 1, LoopStart: 0, LoopEnd: 1},
		{ID: 8, Speed: 4, StepOffset: 2, StepCount: 2, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
	}
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourceTriangle},
		{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourceSquare},
		{PhaseStep: picoPitchPhaseSteps[30], Volume: 7, Kind: SourceCustom0},
		{PhaseStep: picoPitchPhaseSteps[30], Volume: 7, Kind: SourceCustom1},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("custom instrument-change SFXPlayer.Init failed")
	}
	if _, ok := player.Play(8, 0, 0, 0); !ok {
		t.Fatal("custom instrument-change Play failed")
	}
	player.MixMono(make([]int8, SFXSamplesForUnits(4)+1))
	state, _ := player.Channel(0)
	voice, _ := mixer.Voice(int(state.VoiceID))
	if state.Step != 1 || player.definitions[player.channels[0].custom[0].definition].ID != 1 || voice.Kind != SourceSquare {
		t.Fatalf("instrument change state=%+v custom=%+v voice=%+v, want custom 1 square", state, player.channels[0].custom[0], voice)
	}
}

func TestCustomInstrumentKeepsOscillatorPhaseAcrossEnvelopeSteps(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{
		{ID: 0, Speed: 2, StepOffset: 0, StepCount: 2, LoopStart: 0, LoopEnd: 2},
		{ID: 8, Speed: 8, StepOffset: 2, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
	}
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourceOrgan},
		{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume / 2, Kind: SourceOrgan},
		{PhaseStep: picoPitchPhaseSteps[30], Volume: 7, Kind: SourceCustom0},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("custom SFXPlayer.Init failed")
	}
	if _, ok := player.Play(8, 0, 0, 0); !ok {
		t.Fatal("custom Play failed")
	}
	state, _ := player.Channel(0)
	first, _ := mixer.Voice(int(state.VoiceID))
	player.MixMono(make([]int8, SFXSamplesForUnits(2)+1))
	state, _ = player.Channel(0)
	second, _ := mixer.Voice(int(state.VoiceID))
	if second.Started != first.Started || second.Kind != SourceOrgan || second.Volume != MaxVolume/2 {
		t.Fatalf("custom envelope step restarted oscillator: first=%+v second=%+v", first, second)
	}
}

func TestCustomInstrumentFadeRetainsFractionalLowVolume(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{
		{ID: 0, Speed: 4, StepOffset: 0, StepCount: 1, LoopStart: 0, LoopEnd: 1},
		{ID: 8, Speed: 4, StepOffset: 1, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
	}
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[24], Volume: 167, Kind: SourceTriangle},
		{PhaseStep: picoPitchPhaseSteps[24], Volume: 1, Kind: SourceCustom0, Effect: SFXEffectFadeIn},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("custom fade SFXPlayer.Init failed")
	}
	if _, ok := player.Play(8, 0, 0, 0); !ok {
		t.Fatal("custom fade Play failed")
	}
	state, _ := player.Channel(0)
	voice, _ := mixer.Voice(int(state.VoiceID))
	if voice.Volume != 0 {
		t.Fatalf("custom fade initial volume = %d, want 0", voice.Volume)
	}
	player.MixMono(make([]int8, SFXSamplesForUnits(1)+1))
	state, _ = player.Channel(0)
	voice, _ = mixer.Voice(int(state.VoiceID))
	if voice.Volume != 6 {
		t.Fatalf("custom fade tick-one volume = %d, want 6", voice.Volume)
	}
}

func TestCustomInstrumentSupportsTwoBoundedLayers(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{
		{ID: 0, Speed: 2, StepOffset: 0, StepCount: 1, LoopStart: 0, LoopEnd: 1},
		{ID: 1, Speed: 2, StepOffset: 1, StepCount: 1, LoopStart: 0, LoopEnd: 1},
		{ID: 8, Speed: 8, StepOffset: 2, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
	}
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[26], Volume: 7, Kind: SourceCustom1},
		{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourceTriangle},
		{PhaseStep: picoPitchPhaseSteps[30], Volume: 7, Kind: SourceCustom0},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("two-layer custom SFXPlayer.Init failed")
	}
	if _, ok := player.Play(8, 0, 0, 0); !ok {
		t.Fatal("two-layer custom Play failed")
	}
	state, _ := player.Channel(0)
	voice, _ := mixer.Voice(int(state.VoiceID))
	want := picoPitchPhaseSteps[32]
	if player.channels[0].customDepth != 2 || voice.Kind != SourceTriangle || voice.Step != want {
		t.Fatalf("nested custom depth=%d voice=%+v, want triangle step %d", player.channels[0].customDepth, voice, want)
	}
}

func TestCustomInstrumentRejectsThirdLayer(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{
		{ID: 0, Speed: 1, StepOffset: 0, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
		{ID: 1, Speed: 1, StepOffset: 1, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
		{ID: 2, Speed: 1, StepOffset: 2, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
	}
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[24], Volume: 7, Kind: SourceCustom1},
		{PhaseStep: picoPitchPhaseSteps[24], Volume: 7, Kind: SourceCustom2},
		{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourceTriangle},
	}
	var player SFXPlayer
	if player.Init(mixer, definitions, steps) {
		t.Fatal("SFXPlayer accepted a third custom-instrument layer")
	}
}

func TestCustomInstrumentAllowsSilentSelfReference(t *testing.T) {
	definitions := []SFXDefinition{{
		ID: 0, Speed: 1, StepCount: 2, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{
		{Kind: SourceTriangle, PhaseStep: picoPitchPhaseSteps[24], Volume: 167},
		{Kind: SourceCustom0, PhaseStep: picoPitchPhaseSteps[24], Volume: 0},
	}
	var mixer Mixer
	if !mixer.Init(SFXChannels) {
		t.Fatal("mixer init failed")
	}
	var player SFXPlayer
	if !player.Init(&mixer, definitions, steps) {
		t.Fatal("silent custom self-reference rejected")
	}
}

func TestSFXClockMatchesEveryPICO8Speed(t *testing.T) {
	for speed := uint32(1); speed <= 255; speed++ {
		got := SFXSamplesForUnits(speed)
		// PICO-8 exports 183 source samples per speed unit. Cross-multiply
		// instead of using floating point: the rounded target boundary must be
		// within half one 16,384 Hz sample of that physical duration.
		delta := int64(got)*22050 - int64(speed)*183*MusicSampleRate
		if delta < 0 {
			delta = -delta
		}
		if delta*2 > 22050 {
			t.Fatalf("speed %d duration = %d target samples, rounding error numerator %d", speed, got, delta)
		}

		remainder := sfxSpeedInitialRemainder
		var accumulated uint32
		for step := 0; step < MaxSFXSteps; step++ {
			samples, next := sfxClockSamples(speed, remainder)
			accumulated += samples
			remainder = next
		}
		if want := SFXSamplesForUnits(speed * MaxSFXSteps); accumulated != want {
			t.Fatalf("speed %d cumulative 32-step duration = %d, want %d", speed, accumulated, want)
		}
	}
}

func TestSFXChannelPanPersistsAcrossVoiceRestarts(t *testing.T) {
	mixer, player := sfxFixture(t)
	if !player.SetChannelPan(0, MinPan) {
		t.Fatal("SetChannelPan rejected valid pre-stream configuration")
	}
	if _, ok := player.Play(2, 0, 0, 0); !ok {
		t.Fatal("Play failed")
	}
	state, _ := player.Channel(0)
	voice, _ := mixer.Voice(int(state.VoiceID))
	if voice.Pan != MinPan {
		t.Fatalf("first voice pan = %d, want %d", voice.Pan, MinPan)
	}

	player.MixMono(make([]int8, SFXSamplesForUnits(1)+1))
	state, _ = player.Channel(0)
	voice, _ = mixer.Voice(int(state.VoiceID))
	if state.Step != 1 || voice.Pan != MinPan {
		t.Fatalf("restarted channel state=%+v voice pan=%d, want step 1 hard left", state, voice.Pan)
	}
	if player.SetChannelPan(-1, 0) || player.SetChannelPan(SFXChannels, 0) || player.SetChannelPan(0, MaxPan+1) {
		t.Fatal("SetChannelPan accepted invalid channel or pan")
	}
	player.deferred = true
	if player.SetChannelPan(0, 0) {
		t.Fatal("SetChannelPan mutated configuration after deferred streaming began")
	}
}

func TestSFXPlayerMasterVolumeScalesStartsAndLocksWithActiveAudio(t *testing.T) {
	mixer, player := sfxFixture(t)
	if !player.SetMasterVolume(MaxVolume / 2) {
		t.Fatal("SetMasterVolume rejected valid pre-start gain")
	}
	if _, ok := player.Play(2, 0, 0, 0); !ok {
		t.Fatal("Play failed")
	}
	state, _ := player.Channel(0)
	voice, _ := mixer.Voice(int(state.VoiceID))
	if voice.Volume != MaxVolume/2 {
		t.Fatalf("scaled voice volume = %d, want %d", voice.Volume, MaxVolume/2)
	}
	if player.SetMasterVolume(MaxVolume) {
		t.Fatal("SetMasterVolume changed gain with an active voice")
	}
	if player.SetMasterVolume(MaxVolume + 1) {
		t.Fatal("SetMasterVolume accepted out-of-range gain")
	}
}

func TestSFXPlayerLoopOffsetLengthAndStop(t *testing.T) {
	_, player := sfxFixture(t)
	if _, ok := player.Play(7, 1, 0, 0); !ok {
		t.Fatal("looped Play failed")
	}
	player.MixMono(make([]int8, SFXSamplesForUnits(6)))
	state, _ := player.Channel(1)
	if !state.Active || state.Step != 2 {
		t.Fatalf("looped state = %+v, want step 2 active", state)
	}
	player.MixMono(make([]int8, 1))
	state, _ = player.Channel(1)
	if state.Step != 1 {
		t.Fatalf("loop did not return to step 1: %+v", state)
	}
	if !player.Stop(1) {
		t.Fatal("Stop(1) failed")
	}
	if state, _ := player.Channel(1); state.Active {
		t.Fatal("stopped channel remains active")
	}

	if _, ok := player.Play(7, 2, 1, 1); !ok {
		t.Fatal("sliced Play failed")
	}
	player.MixMono(make([]int8, SFXSamplesForUnits(2)+1))
	if state, _ := player.Channel(2); state.Active {
		t.Fatal("length-limited channel looped instead of stopping")
	}
}

func TestSFXSlideInterpolatesPreviousPitchAndVolumeAtTickBoundaries(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 1, Speed: 4, StepCount: 2, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[24], Volume: 256, Kind: SourceSquare},
		{PhaseStep: picoPitchPhaseSteps[48], Volume: 128, Kind: SourceSquare, Effect: SFXEffectSlide},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("slide player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("slide Play failed")
	}
	player.MixMono(make([]int8, SFXSamplesForUnits(4)))
	state, _ := player.Channel(0)
	voice, _ := mixer.Voice(int(state.VoiceID))
	if voice.Step != picoPitchPhaseSteps[24] || voice.Volume != 256 {
		t.Fatalf("slide start voice = %+v", voice)
	}

	player.MixMono(make([]int8, SFXSamplesForUnits(1)))
	for tick, want := range []struct {
		step   uint32
		volume uint16
	}{{picoPitchPhaseSteps[30], 224}, {picoPitchPhaseSteps[36], 192}, {picoPitchPhaseSteps[42], 160}} {
		player.MixMono(make([]int8, 1))
		state, _ = player.Channel(0)
		voice, _ = mixer.Voice(int(state.VoiceID))
		if state.Effect != SFXEffectSlide || state.EffectTick != uint8(tick+1) || voice.Step != want.step || voice.Volume != want.volume {
			t.Fatalf("slide tick %d state=%+v voice=%+v, want step=%d volume=%d", tick+1, state, voice, want.step, want.volume)
		}
		player.MixMono(make([]int8, SFXSamplesForUnits(1)-1))
	}
}

func TestSFXSlideFirstStepUsesDefaultC2Source(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 1, Speed: 2, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	base := picoPitchPhaseSteps[24]
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[48], Volume: 256, Kind: SourceSquare, Effect: SFXEffectSlide},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("slide player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("slide Play failed")
	}
	state, _ := player.Channel(0)
	voice, _ := mixer.Voice(int(state.VoiceID))
	if voice.Step != base || voice.Volume != 256 {
		t.Fatalf("first slide source state=%+v voice=%+v", state, voice)
	}
	player.MixMono(make([]int8, SFXSamplesForUnits(1)+1))
	state, _ = player.Channel(0)
	voice, _ = mixer.Voice(int(state.VoiceID))
	if state.EffectTick != 1 || voice.Step != picoPitchPhaseSteps[36] || voice.Volume != 256 {
		t.Fatalf("first slide tick state=%+v voice=%+v", state, voice)
	}
}

func TestSFXSlideCannotModulateStolenVoice(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 1, Speed: 2, StepCount: 2, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{
		{PhaseStep: 100, Volume: 256, Kind: SourceSquare, Effect: SFXEffectSlide},
		{PhaseStep: 500, Volume: 0, Kind: SourceSquare},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("slide player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("slide Play failed")
	}
	for index := 0; index < 3; index++ {
		start(t, mixer, Request{Kind: SourceSquare, Step: uint32(700 + index), Volume: 1, Priority: SFXPriority})
	}
	stolenID := start(t, mixer, Request{Kind: SourceTriangle, Step: 999, Volume: 77, Priority: SFXPriority})
	if stolenID != 0 {
		t.Fatalf("stolen voice ID = %d, want SFX-owned ID 0", stolenID)
	}
	player.MixMono(make([]int8, SFXSamplesForUnits(1)+1))
	voice, _ := mixer.Voice(stolenID)
	state, _ := player.Channel(0)
	if voice.Step != 999 || voice.Volume != 77 || state.VoiceID != -1 {
		t.Fatalf("slide changed stolen voice=%+v channel=%+v", voice, state)
	}
}

func TestSFXVibratoUsesQuarterToneSixteenTickCycle(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 1, Speed: 32, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	const base = uint32(1 << 24)
	steps := []SFXStep{{
		PhaseStep: base, Volume: 256, Kind: SourceSquare, Effect: SFXEffectVibrato,
	}}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("vibrato player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("vibrato Play failed")
	}
	state, _ := player.Channel(0)
	voice, _ := mixer.Voice(int(state.VoiceID))
	if state.Effect != SFXEffectVibrato || state.EffectTick != 0 || voice.Step != base || voice.Volume != 256 {
		t.Fatalf("vibrato start state=%+v voice=%+v", state, voice)
	}

	player.MixMono(make([]int8, SFXSamplesForUnits(1)))
	wants := map[int]uint32{
		1: 16838656, 4: 17027072, 8: base, 12: 16527360, 16: base,
	}
	for tick := 1; tick <= 16; tick++ {
		player.MixMono(make([]int8, 1))
		state, _ = player.Channel(0)
		voice, _ = mixer.Voice(int(state.VoiceID))
		if state.EffectTick != uint8(tick) {
			t.Fatalf("vibrato effect tick = %d, want %d", state.EffectTick, tick)
		}
		if want, ok := wants[tick]; ok && voice.Step != want {
			t.Fatalf("vibrato tick %d step = %d, want %d", tick, voice.Step, want)
		}
		if voice.Volume != 256 {
			t.Fatalf("vibrato tick %d changed volume to %d", tick, voice.Volume)
		}
		player.MixMono(make([]int8, SFXSamplesForUnits(1)-1))
	}
}

func TestSFXDropLinearlyLowersFrequencyOverStepDuration(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 1, Speed: 4, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{{
		PhaseStep: 1000, Volume: 256, Kind: SourceSquare, Effect: SFXEffectDrop,
	}}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("drop player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("drop Play failed")
	}
	state, _ := player.Channel(0)
	voice, _ := mixer.Voice(int(state.VoiceID))
	if state.Effect != SFXEffectDrop || state.EffectTick != 0 || voice.Step != 1000 || voice.Volume != 256 {
		t.Fatalf("drop start state=%+v voice=%+v", state, voice)
	}

	player.MixMono(make([]int8, SFXSamplesForUnits(1)))
	for tick, want := range []uint32{750, 500, 250} {
		player.MixMono(make([]int8, 1))
		state, _ = player.Channel(0)
		voice, _ = mixer.Voice(int(state.VoiceID))
		if state.EffectTick != uint8(tick+1) || voice.Step != want || voice.Volume != 256 {
			t.Fatalf("drop tick %d state=%+v voice=%+v, want step=%d", tick+1, state, voice, want)
		}
		player.MixMono(make([]int8, SFXSamplesForUnits(1)-1))
	}
}

func TestSFXArpeggioUsesMeasuredFastAndSlowCadences(t *testing.T) {
	tests := []struct {
		name    string
		effect  SFXEffect
		speed   uint8
		cadence int
	}{
		{"fast", SFXEffectArpeggioFast, 18, 4},
		{"slow", SFXEffectArpeggioSlow, 18, 8},
		{"fast speed eight", SFXEffectArpeggioFast, 8, 2},
		{"slow speed eight", SFXEffectArpeggioSlow, 8, 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mixer := newMixer(t, 4)
			definition := []SFXDefinition{{
				ID: 1, Speed: test.speed, StepCount: 4, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
			}}
			steps := []SFXStep{
				{PhaseStep: 100, Volume: 77, Kind: SourceSquare, Effect: test.effect},
				{PhaseStep: 200, Volume: 88, Kind: SourceTriangle},
				{PhaseStep: 300, Volume: 99, Kind: SourceSaw},
				{PhaseStep: 400, Volume: 111, Kind: SourcePulse},
			}
			var player SFXPlayer
			if !player.Init(mixer, definition, steps) {
				t.Fatal("arpeggio player Init failed")
			}
			if _, ok := player.Play(1, 0, 0, 0); !ok {
				t.Fatal("arpeggio Play failed")
			}
			player.MixMono(make([]int8, SFXSamplesForUnits(uint32(test.cadence))))
			state, _ := player.Channel(0)
			voice, _ := mixer.Voice(int(state.VoiceID))
			if voice.Step != 100 {
				t.Fatalf("pitch changed before cadence: voice=%+v state=%+v", voice, state)
			}
			player.MixMono(make([]int8, 1))
			state, _ = player.Channel(0)
			voice, _ = mixer.Voice(int(state.VoiceID))
			if state.EffectTick != uint8(test.cadence) || voice.Step != 200 || voice.Volume != 77 || voice.Kind != SourceSquare {
				t.Fatalf("cadence boundary state=%+v voice=%+v", state, voice)
			}
		})
	}
}

func TestSFXArpeggioPhaseContinuesAcrossAlignedGroupBoundary(t *testing.T) {
	for _, test := range []struct {
		name   string
		effect SFXEffect
		want   uint32
	}{
		{"fast", SFXEffectArpeggioFast, 700},
		{"slow", SFXEffectArpeggioSlow, 600},
	} {
		t.Run(test.name, func(t *testing.T) {
			mixer := newMixer(t, 4)
			definition := []SFXDefinition{{
				ID: 1, Speed: 18, StepCount: 8, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
			}}
			steps := make([]SFXStep, 8)
			for index := range steps {
				steps[index] = SFXStep{
					PhaseStep: uint32((index + 1) * 100), Volume: uint16(40 + index),
					Kind: SourceSquare, Effect: test.effect,
				}
			}
			steps[4].Volume = 77
			steps[4].Kind = SourceTriangle
			var player SFXPlayer
			if !player.Init(mixer, definition, steps) {
				t.Fatal("arpeggio player Init failed")
			}
			if _, ok := player.Play(1, 0, 0, 0); !ok {
				t.Fatal("arpeggio Play failed")
			}
			player.MixMono(make([]int8, SFXSamplesForUnits(72)))
			player.MixMono(make([]int8, 1))
			state, _ := player.Channel(0)
			voice, _ := mixer.Voice(int(state.VoiceID))
			if state.Step != 4 || state.EffectTick != 0 || voice.Step != test.want || voice.Volume != 77 || voice.Kind != SourceTriangle {
				t.Fatalf("group boundary state=%+v voice=%+v, want pitch %d", state, voice, test.want)
			}
		})
	}
}

func TestSFXArpeggioSliceUsesDefinitionAlignedPitchGroup(t *testing.T) {
	mixer := newMixer(t, 4)
	definition := []SFXDefinition{{
		ID: 1, Speed: 18, StepCount: 8, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := make([]SFXStep, 8)
	for index := range steps {
		steps[index] = SFXStep{PhaseStep: uint32((index + 1) * 100), Volume: 64, Kind: SourceSquare}
	}
	steps[5].Effect = SFXEffectArpeggioFast
	var player SFXPlayer
	if !player.Init(mixer, definition, steps) {
		t.Fatal("arpeggio player Init failed")
	}
	if _, ok := player.Play(1, 0, 5, 1); !ok {
		t.Fatal("sliced arpeggio Play failed")
	}
	state, _ := player.Channel(0)
	voice, _ := mixer.Voice(int(state.VoiceID))
	if voice.Step != 500 {
		t.Fatalf("sliced arpeggio starts at pitch %d, want aligned group pitch 500", voice.Step)
	}
}

func TestSFXArpeggioPhaseContinuesAcrossLoopWrap(t *testing.T) {
	for _, test := range []struct {
		effect SFXEffect
		want   uint32
	}{
		{SFXEffectArpeggioFast, 300},
		{SFXEffectArpeggioSlow, 200},
	} {
		mixer := newMixer(t, 4)
		definition := []SFXDefinition{{
			ID: 1, Speed: 18, StepCount: 4, LoopStart: 0, LoopEnd: 4,
		}}
		steps := []SFXStep{
			{PhaseStep: 100, Volume: 64, Kind: SourceSquare, Effect: test.effect},
			{PhaseStep: 200, Volume: 64, Kind: SourceSquare, Effect: test.effect},
			{PhaseStep: 300, Volume: 64, Kind: SourceSquare, Effect: test.effect},
			{PhaseStep: 400, Volume: 64, Kind: SourceSquare, Effect: test.effect},
		}
		var player SFXPlayer
		if !player.Init(mixer, definition, steps) {
			t.Fatal("looping arpeggio player Init failed")
		}
		if _, ok := player.Play(1, 0, 0, 0); !ok {
			t.Fatal("looping arpeggio Play failed")
		}
		player.MixMono(make([]int8, SFXSamplesForUnits(72)))
		player.MixMono(make([]int8, 1))
		state, _ := player.Channel(0)
		voice, _ := mixer.Voice(int(state.VoiceID))
		if state.Step != 0 || voice.Step != test.want {
			t.Fatalf("loop wrap state=%+v voice=%+v, want pitch %d", state, voice, test.want)
		}
	}
}

func TestSFXFadeInRampsVolumeAcrossEverySample(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 1, Speed: 1, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{{
		PhaseStep: 1, Volume: 256, Kind: SourceSquare, Effect: SFXEffectFadeIn,
	}}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("fade-in player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("fade-in Play failed")
	}
	samples := make([]int8, SFXSamplesForUnits(1))
	player.MixMono(samples)
	if samples[0] != 0 || samples[34] != 22 || samples[68] != 63 || samples[102] != 95 || samples[135] != 126 {
		t.Fatalf("fade-in samples [0,34,68,102,135] = [%d,%d,%d,%d,%d]", samples[0], samples[34], samples[68], samples[102], samples[135])
	}
	state, _ := player.Channel(0)
	if state.Effect != SFXEffectFadeIn || state.EffectTick != 0 || state.SamplesRemaining != 0 {
		t.Fatalf("fade-in state at step boundary = %+v", state)
	}
}

func TestSFXFadeOutRampsVolumeAcrossEverySample(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 1, Speed: 1, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{{
		PhaseStep: 1, Volume: 256, Kind: SourceSquare, Effect: SFXEffectFadeOut,
	}}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("fade-out player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("fade-out Play failed")
	}
	samples := make([]int8, SFXSamplesForUnits(1))
	player.MixMono(samples)
	if samples[0] != 2 || samples[34] != 69 || samples[68] != 63 || samples[102] != 31 || samples[135] != 0 {
		t.Fatalf("fade-out samples [0,34,68,102,135] = [%d,%d,%d,%d,%d]", samples[0], samples[34], samples[68], samples[102], samples[135])
	}
	state, _ := player.Channel(0)
	if state.Effect != SFXEffectFadeOut || state.EffectTick != 0 || state.SamplesRemaining != 0 {
		t.Fatalf("fade-out state at step boundary = %+v", state)
	}
}

func TestSFXFadeOutRetriggerUsesPICOAntiClickAttack(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 1, Speed: 1, StepCount: 2, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{
		{PhaseStep: 1, Volume: MaxVolume, Kind: SourceSquare, Effect: SFXEffectFadeOut},
		{PhaseStep: 1, Volume: MaxVolume, Kind: SourceSquare, Effect: SFXEffectFadeOut},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("fade-out retrigger player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("fade-out retrigger Play failed")
	}
	boundary := int(SFXSamplesForUnits(1))
	samples := make([]int8, SFXSamplesForUnits(2))
	player.MixMono(samples)
	if samples[boundary-1] != 0 || samples[boundary] != 2 || samples[boundary+1] != 5 {
		t.Fatalf("anti-click boundary [%d,%d,%d] = [%d,%d,%d]", boundary-1, boundary, boundary+1,
			samples[boundary-1], samples[boundary], samples[boundary+1])
	}
	if samples[47] != 82 || samples[48] != 82 {
		// The attack reaches unity after 48 samples while the authored fade-out
		// continues across the complete note.
		t.Fatalf("attack/fade product samples [47,48] = [%d,%d]", samples[47], samples[48])
	}
}

func TestSFXStepBoundaryPreservesPICOChannelPhase(t *testing.T) {
	mixer := newMixer(t, SFXMixerVoices)
	definitions := []SFXDefinition{{
		ID: 1, Speed: 1, StepCount: 2, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[18], Volume: 128, Kind: SourceOrgan},
		{PhaseStep: picoPitchPhaseSteps[22], Volume: 128, Kind: SourceOrgan},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("crossfade player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("phase-continuous Play failed")
	}
	started := player.channels[0].voiceStarted
	boundary := int(SFXSamplesForUnits(1))
	samples := make([]int8, boundary+SFXEdgeSamples)
	player.MixMono(samples)
	for sample := boundary - 1; sample < len(samples)-1; sample++ {
		delta := int(samples[sample+1]) - int(samples[sample])
		if delta < 0 {
			delta = -delta
		}
		if delta > 32 {
			t.Fatalf("click-sized delta %d at crossfade sample %d: %d -> %d", delta, sample, samples[sample], samples[sample+1])
		}
	}
	stats := mixer.Stats()
	if player.channels[0].voiceStarted != started || stats.PeakActiveVoices != 1 || stats.ActiveVoices != 1 {
		t.Fatalf("phase-continuous state = generation %d, stats %+v; want generation %d and one voice", player.channels[0].voiceStarted, stats, started)
	}
}

func TestSFXExplicitChannelRetriggerPreservesPICOPhase(t *testing.T) {
	mixer := newMixer(t, SFXMixerVoices)
	definitions := []SFXDefinition{{
		ID: 1, Speed: 8, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{{PhaseStep: picoPitchPhaseSteps[18], Volume: 128, Kind: SourceOrgan}}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("explicit-retrigger player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("initial explicit Play failed")
	}
	player.MixMono(make([]int8, 96))
	before := player.channels[0]
	voiceBefore, ok := mixer.Voice(int(before.voiceID))
	if !ok || !voiceBefore.Active {
		t.Fatalf("initial owned voice = %+v, %v", voiceBefore, ok)
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("explicit retrigger Play failed")
	}
	after := player.channels[0]
	voiceAfter, ok := mixer.Voice(int(after.voiceID))
	if !ok || !voiceAfter.Active || after.voiceID != before.voiceID ||
		after.voiceStarted != before.voiceStarted || voiceAfter.Phase != voiceBefore.Phase {
		t.Fatalf("retrigger reset phase: before channel=%+v voice=%+v, after channel=%+v voice=%+v", before, voiceBefore, after, voiceAfter)
	}
	if stats := mixer.Stats(); stats.PeakActiveVoices != 1 || stats.ActiveVoices != 1 {
		t.Fatalf("explicit-retrigger mixer stats = %+v, want one continuous voice", stats)
	}
}

func TestSFXFinalNoteKeepsMeasuredReleaseTail(t *testing.T) {
	mixer := newMixer(t, SFXMixerVoices)
	definitions := []SFXDefinition{{
		ID: 1, Speed: 1, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{{PhaseStep: picoPitchPhaseSteps[18], Volume: 128, Kind: SourceOrgan}}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("release-tail player Init failed")
	}
	if _, ok := player.Play(1, 0, 0, 0); !ok {
		t.Fatal("release-tail Play failed")
	}
	authored := int(SFXSamplesForUnits(1))
	samples := make([]int8, authored+SFXEdgeSamples)
	player.MixMono(samples)
	nonzero := false
	for _, sample := range samples[authored:] {
		nonzero = nonzero || sample != 0
	}
	if !nonzero || samples[len(samples)-1] != 0 {
		t.Fatalf("release tail is missing or does not close at zero: %v", samples[authored:])
	}
	if stats := mixer.Stats(); stats.ActiveVoices != 0 {
		t.Fatalf("release tail left active mixer state: %+v", stats)
	}
	if state, _ := player.Channel(0); state.Active {
		t.Fatalf("logical SFX channel remained active after release: %+v", state)
	}
}

func TestSFXPlayerAutoChannelRestartAndInvalidCalls(t *testing.T) {
	_, player := sfxFixture(t)
	for want := 0; want < SFXChannels; want++ {
		if got, ok := player.Play(2, -1, 0, 0); !ok || got != want {
			t.Fatalf("auto channel = %d ok=%t, want %d", got, ok, want)
		}
	}
	if got, ok := player.Play(7, -1, 0, 0); !ok || got != 0 {
		t.Fatalf("oldest replacement = %d ok=%t, want 0", got, ok)
	}
	if _, ok := player.Play(99, -1, 0, 0); ok {
		t.Fatal("invalid ID was accepted")
	}
	if _, ok := player.Play(2, 4, 0, 0); ok {
		t.Fatal("invalid channel was accepted")
	}
	stats := player.Stats()
	if stats.ActiveChannels != 4 || stats.Starts != 5 || stats.InvalidCalls != 2 {
		t.Fatalf("stats = %+v", stats)
	}
	if _, ok := player.Play(-1, -1, 0, 0); !ok {
		t.Fatal("stop-all call failed")
	}
	if got := player.Stats().ActiveChannels; got != 0 {
		t.Fatalf("active channels after stop all = %d", got)
	}
}

func TestSFXPlayerRejectsMalformedTargetData(t *testing.T) {
	mixer := newMixer(t, 4)
	validStep := []SFXStep{{PhaseStep: 1, Volume: 1, Kind: SourceSquare}}
	invalidDefinitions := [][]SFXDefinition{
		{{ID: 0, Speed: 0, StepCount: 1}},
		{{ID: 0, Speed: 1, StepCount: 2}},
		{{ID: 0, Speed: 1, StepCount: 1, LoopStart: 0, LoopEnd: NoSFXLoop}},
		{{ID: 1, Speed: 1, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop}, {ID: 0, Speed: 1, StepCount: 1}},
	}
	for _, definitions := range invalidDefinitions {
		var player SFXPlayer
		if player.Init(mixer, definitions, validStep) {
			t.Fatalf("malformed definitions accepted: %#v", definitions)
		}
	}
	for _, step := range []SFXStep{
		{}, {PhaseStep: 1, Volume: MaxVolume + 1, Kind: SourceSquare},
		{PhaseStep: 1, Volume: 1, Kind: SourcePCM},
		{PhaseStep: 1, Volume: 1, Kind: SourceSquare, Effect: SFXEffectArpeggioSlow + 1},
	} {
		var player SFXPlayer
		if player.Init(mixer, []SFXDefinition{{ID: 0, Speed: 1, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop}}, []SFXStep{step}) {
			t.Fatalf("malformed step accepted: %#v", step)
		}
	}
}

func TestSFXPlayerMixAllocatesNothing(t *testing.T) {
	_, player := sfxFixture(t)
	if _, ok := player.Play(7, 0, 0, 0); !ok {
		t.Fatal("Play failed")
	}
	bytes := make([]int8, 256)
	words := make([]uint32, 64)
	if got := testing.AllocsPerRun(100, func() {
		player.MixMono(bytes)
		player.MixMonoWords(words)
	}); got != 0 {
		t.Fatalf("sequencer mix allocations = %v, want 0", got)
	}
}

func TestSFXWordMixersPreserveUnalignedExactClockBoundaries(t *testing.T) {
	newPlayer := func() *SFXPlayer {
		mixer := newMixer(t, 4)
		definition := []SFXDefinition{{
			ID: 0, Speed: 1, StepCount: 4, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
			Filters: SFXFilter(24 + 72),
		}}
		steps := []SFXStep{
			{PhaseStep: 1 << 27, Volume: 180, Kind: SourceTriangle, Effect: SFXEffectVibrato},
			{PhaseStep: 1 << 28, Volume: 160, Kind: SourceTriangle, Effect: SFXEffectSlide},
			{PhaseStep: 1 << 29, Volume: 140, Kind: SourceTriangle, Effect: SFXEffectFadeOut},
			{PhaseStep: 1 << 30, Volume: 120, Kind: SourceTriangle, Effect: SFXEffectNone},
		}
		var player SFXPlayer
		if !player.Init(mixer, definition, steps) {
			t.Fatal("SFXPlayer.Init failed")
		}
		if !player.SetChannelPan(0, -73) {
			t.Fatal("SetChannelPan failed")
		}
		if _, ok := player.Play(0, 0, 0, 0); !ok {
			t.Fatal("Play failed")
		}
		return &player
	}
	mixStereoSamples := func(player *SFXPlayer, left, right []int8) {
		for len(left) > 0 {
			player.advanceMusicFade()
			player.advanceMusic()
			player.advanceDueChannels()
			player.advanceDueEffects()
			samples := player.nextSegmentSamples(len(left))
			player.mixer.MixStereo(left[:samples], right[:samples])
			player.consumeSamples(uint32(samples))
			left, right = left[samples:], right[samples:]
		}
	}

	const wordCount = 512
	monoBytes := make([]int8, wordCount*4)
	monoWords := make([]uint32, wordCount)
	newPlayer().MixMono(monoBytes)
	newPlayer().MixMonoWords(monoWords)
	for sample, want := range monoBytes {
		word := monoWords[sample/4]
		got := int8(uint8(word >> (8 * (sample & 3))))
		if got != want {
			t.Fatalf("mono sample %d = %d, want %d", sample, got, want)
		}
	}

	leftBytes, rightBytes := make([]int8, wordCount*4), make([]int8, wordCount*4)
	leftWords, rightWords := make([]uint32, wordCount), make([]uint32, wordCount)
	mixStereoSamples(newPlayer(), leftBytes, rightBytes)
	newPlayer().MixStereoWords(leftWords, rightWords)
	for sample := range leftBytes {
		shift := 8 * (sample & 3)
		gotLeft := int8(uint8(leftWords[sample/4] >> shift))
		gotRight := int8(uint8(rightWords[sample/4] >> shift))
		if gotLeft != leftBytes[sample] || gotRight != rightBytes[sample] {
			t.Fatalf(
				"stereo sample %d = (%d,%d), want (%d,%d)",
				sample, gotLeft, gotRight, leftBytes[sample], rightBytes[sample],
			)
		}
	}
}

func TestCustomMusicByteAndWordMixersAgreeAcrossUnalignedBoundaries(t *testing.T) {
	newPlayer := func() *SFXPlayer {
		mixer := newMixer(t, 4)
		definitions := []SFXDefinition{{
			ID: 0, Speed: 2, StepOffset: 0, StepCount: 2, LoopStart: 0, LoopEnd: 2,
		}}
		steps := []SFXStep{
			{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourceOrgan},
			{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume / 2, Kind: SourceOrgan},
		}
		for channel := 0; channel < SFXChannels; channel++ {
			definitions = append(definitions, SFXDefinition{
				ID: uint8(8 + channel), Speed: 32, StepOffset: uint16(len(steps)), StepCount: 4,
				LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
			})
			for note := 0; note < 4; note++ {
				steps = append(steps, SFXStep{
					PhaseStep: picoPitchPhaseSteps[20+channel*4+note], Volume: 7, Kind: SourceCustom0,
				})
			}
		}
		pattern := MusicPattern{
			ID: 0, DurationSamples: SFXSamplesForUnits(128), Next: NoMusicPattern,
			Channels: [SFXChannels]int8{8, 9, 10, 11},
		}
		var player SFXPlayer
		if !player.Init(mixer, definitions, steps) || !player.InitMusic([]MusicPattern{pattern}) || !player.Music(0, 0) {
			t.Fatal("custom music player initialization failed")
		}
		return &player
	}
	sampleN := int(SFXSamplesForUnits(128)+128+3) &^ 3
	bytes := make([]int8, sampleN)
	words := make([]uint32, sampleN/4)
	newPlayer().MixMono(bytes)
	newPlayer().MixMonoWords(words)
	for sample, want := range bytes {
		got := int8(uint8(words[sample/4] >> (8 * (sample & 3))))
		if got != want {
			t.Fatalf("custom music sample %d = %d, want %d", sample, got, want)
		}
	}
}

func TestEveryPICO8InstrumentAndEffectCombinationRunsToCompletion(t *testing.T) {
	instruments := []struct {
		name string
		kind SourceKind
	}{
		{"triangle", SourceTriangle}, {"tilted-saw", SourceTiltedSaw},
		{"saw", SourceSaw}, {"square", SourceSquare}, {"pulse", SourcePulse},
		{"organ", SourceOrgan}, {"noise", SourceNoise}, {"phaser", SourcePhaser},
	}
	effects := []struct {
		name   string
		effect SFXEffect
	}{
		{"none", SFXEffectNone}, {"slide", SFXEffectSlide},
		{"vibrato", SFXEffectVibrato}, {"drop", SFXEffectDrop},
		{"fade-in", SFXEffectFadeIn}, {"fade-out", SFXEffectFadeOut},
		{"arpeggio-fast", SFXEffectArpeggioFast},
		{"arpeggio-slow", SFXEffectArpeggioSlow},
	}
	for _, instrument := range instruments {
		for _, effect := range effects {
			t.Run(instrument.name+"/"+effect.name, func(t *testing.T) {
				mixer := newMixer(t, 4)
				definition := []SFXDefinition{{
					ID: 0, Speed: 8, StepCount: 4,
					LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
				}}
				steps := make([]SFXStep, 4)
				for index := range steps {
					steps[index] = SFXStep{
						PhaseStep: picoPitchPhaseSteps[24+index*4], Volume: 96,
						Kind: instrument.kind, Effect: effect.effect,
					}
				}
				var player SFXPlayer
				if !player.Init(mixer, definition, steps) {
					t.Fatal("SFXPlayer.Init failed")
				}
				if _, ok := player.Play(0, 0, 0, 0); !ok {
					t.Fatal("Play failed")
				}
				sampleN := SFXSamplesForUnits(32)
				words := make([]uint32, (sampleN+3)/4)
				player.MixMonoWords(words)
				stats := player.Stats()
				if stats.ActiveChannels != 0 || stats.FailedVoiceStarts != 0 {
					t.Fatalf("terminal stats = %+v", stats)
				}
				nonzero := false
				for _, word := range words {
					nonzero = nonzero || word != 0
				}
				if !nonzero {
					t.Fatal("combination rendered only silence")
				}
			})
		}
	}
}

func TestSFXStepKeepsCompactEightByteTargetRecord(t *testing.T) {
	if got := unsafe.Sizeof(SFXDefinition{}); got != 8 {
		t.Fatalf("SFXDefinition size = %d, want 8", got)
	}
	if got := unsafe.Sizeof(SFXStep{}); got != 8 {
		t.Fatalf("SFXStep size = %d, want 8", got)
	}
}
