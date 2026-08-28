package audio

import "testing"

func musicFixture(t *testing.T) *SFXPlayer {
	t.Helper()
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{
		{ID: 2, Speed: 1, StepOffset: 0, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
		{ID: 7, Speed: 2, StepOffset: 1, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
	}
	steps := []SFXStep{
		{PhaseStep: 1, Volume: MaxVolume, Kind: SourceSquare},
		{PhaseStep: 1 << 30, Volume: MaxVolume, Kind: SourceTriangle},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) {
		t.Fatal("SFXPlayer.Init failed")
	}
	patterns := []MusicPattern{
		{ID: 0, Channels: [SFXChannels]int8{2, -1, -1, -1}, Next: 1, DurationSamples: 128},
		{ID: 1, Channels: [SFXChannels]int8{-1, 7, -1, -1}, Next: 0, DurationSamples: 256},
	}
	if !player.InitMusic(patterns) {
		t.Fatal("SFXPlayer.InitMusic failed")
	}
	return &player
}

func TestMusicTransitionsAndLoopsAtExactSampleBoundaries(t *testing.T) {
	player := musicFixture(t)
	if !player.Music(0, 0) {
		t.Fatal("Music(0) failed")
	}
	state := player.MusicState()
	channel, _ := player.Channel(0)
	if !state.Active || state.PatternID != 0 || state.SamplesRemaining != 128 || !channel.Active || !channel.Music || channel.SFXID != 2 {
		t.Fatalf("initial music=%+v channel=%+v", state, channel)
	}

	player.MixMono(make([]int8, 128))
	if got := player.MusicState(); got.PatternID != 0 || got.SamplesRemaining != 0 {
		t.Fatalf("state at exact boundary = %+v", got)
	}
	player.MixMono(make([]int8, 1))
	state = player.MusicState()
	channel0, _ := player.Channel(0)
	channel1, _ := player.Channel(1)
	if state.PatternID != 1 || state.SamplesRemaining != 255 || channel0.Active || !channel1.Active || !channel1.Music || channel1.SFXID != 7 {
		t.Fatalf("first transition music=%+v channels=%+v/%+v", state, channel0, channel1)
	}

	player.MixMono(make([]int8, 255))
	player.MixMono(make([]int8, 1))
	state = player.MusicState()
	if state.PatternID != 0 || state.SamplesRemaining != 127 {
		t.Fatalf("loop transition = %+v", state)
	}
	stats := player.Stats()
	if !stats.MusicActive || stats.MusicStarts != 1 || stats.MusicTransitions != 2 {
		t.Fatalf("music stats = %+v", stats)
	}
}

func TestMusicTransitionPreservesUnchangedLoopingSFX(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 3, Speed: 1, StepCount: 1, LoopStart: 0, LoopEnd: 1,
	}}
	steps := []SFXStep{{
		PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourceTriangle,
	}}
	patterns := []MusicPattern{
		{ID: 0, Channels: [SFXChannels]int8{3, -1, -1, -1}, Next: 1, DurationSamples: 128},
		{ID: 1, Channels: [SFXChannels]int8{3, -1, -1, -1}, Next: -1, DurationSamples: 256},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) || !player.InitMusic(patterns) || !player.Music(0, 0) {
		t.Fatal("unchanged music-channel fixture failed")
	}
	started := player.channels[0].started
	player.MixMono(make([]int8, 129))
	state := player.MusicState()
	channel, _ := player.Channel(0)
	if state.PatternID != 1 || !channel.Active || !channel.Music ||
		player.channels[0].started != started || player.Stats().Starts != 1 {
		t.Fatalf("unchanged channel retriggered: music=%+v channel=%+v starts=%d/%d",
			state, channel, player.channels[0].started, player.Stats().Starts)
	}
}

func TestMusicTransitionRetriggersMatchingFiniteActiveSFX(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 3, Speed: 1, StepCount: 4, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourceTriangle},
		{PhaseStep: picoPitchPhaseSteps[25], Volume: MaxVolume, Kind: SourceTriangle},
		{PhaseStep: picoPitchPhaseSteps[26], Volume: MaxVolume, Kind: SourceTriangle},
		{PhaseStep: picoPitchPhaseSteps[27], Volume: MaxVolume, Kind: SourceTriangle},
	}
	duration := SFXSamplesForUnits(1)
	patterns := []MusicPattern{
		{ID: 0, Channels: [SFXChannels]int8{3, -1, -1, -1}, Next: 1, DurationSamples: duration},
		{ID: 1, Channels: [SFXChannels]int8{3, -1, -1, -1}, Next: -1, DurationSamples: duration},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) || !player.InitMusic(patterns) || !player.Music(0, 0) {
		t.Fatal("finite active matching music-channel fixture failed")
	}
	player.MixMono(make([]int8, int(duration)+1))
	state := player.MusicState()
	channel, _ := player.Channel(0)
	if state.PatternID != 1 || !channel.Active || channel.Step != 0 || player.Stats().Starts != 2 {
		t.Fatalf("matching finite active SFX did not retrigger: music=%+v channel=%+v starts=%d", state, channel, player.Stats().Starts)
	}
}

func TestMusicTransitionRetriggersMatchingFiniteSFXAtExactEnd(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{
		ID: 3, Speed: 1, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
	}}
	steps := []SFXStep{{
		PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourceTriangle,
	}}
	duration := SFXSamplesForUnits(1)
	patterns := []MusicPattern{
		{ID: 0, Channels: [SFXChannels]int8{3, -1, -1, -1}, Next: 1, DurationSamples: duration},
		{ID: 1, Channels: [SFXChannels]int8{3, -1, -1, -1}, Next: -1, DurationSamples: duration},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) || !player.InitMusic(patterns) || !player.Music(0, 0) {
		t.Fatal("finite matching music-channel fixture failed")
	}
	player.MixMono(make([]int8, int(duration)+1))
	state := player.MusicState()
	channel, _ := player.Channel(0)
	if state.PatternID != 1 || !channel.Active || channel.SFXID != 3 || player.Stats().Starts != 2 {
		t.Fatalf("matching finite SFX did not retrigger: music=%+v channel=%+v starts=%d", state, channel, player.Stats().Starts)
	}
}

func TestMusicTransitionImmediatelyReplacesChangedSFX(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{
		{ID: 2, Speed: 1, StepOffset: 0, StepCount: 4, LoopStart: 0, LoopEnd: 4},
		{ID: 7, Speed: 1, StepOffset: 4, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop},
	}
	steps := []SFXStep{
		{PhaseStep: picoPitchPhaseSteps[19], Volume: MaxVolume, Kind: SourcePhaser},
		{PhaseStep: picoPitchPhaseSteps[21], Volume: MaxVolume, Kind: SourcePhaser},
		{PhaseStep: picoPitchPhaseSteps[24], Volume: MaxVolume, Kind: SourcePhaser},
		{PhaseStep: picoPitchPhaseSteps[28], Volume: MaxVolume, Kind: SourcePhaser},
		{PhaseStep: picoPitchPhaseSteps[31], Volume: MaxVolume, Kind: SourcePhaser},
	}
	patterns := []MusicPattern{
		{ID: 0, Channels: [SFXChannels]int8{2, -1, -1, -1}, Next: 1, DurationSamples: 1},
		{ID: 1, Channels: [SFXChannels]int8{7, -1, -1, -1}, Next: -1, DurationSamples: SFXSamplesForUnits(8)},
	}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) || !player.InitMusic(patterns) || !player.Music(0, 0) {
		t.Fatal("changed music-channel fixture failed")
	}
	player.MixMono(make([]int8, 2))
	state := player.MusicState()
	channel, _ := player.Channel(0)
	if state.PatternID != 1 || !channel.Active || !channel.Music || channel.SFXID != 7 {
		t.Fatalf("changed assignment was not replaced at the pattern boundary: music=%+v channel=%+v", state, channel)
	}
}

func TestManualSFXInterruptsMusicUntilNextPatternAndSurvivesStop(t *testing.T) {
	player := musicFixture(t)
	if !player.Music(0, 0) {
		t.Fatal("Music failed")
	}
	if _, ok := player.Play(7, 1, 0, 0); !ok {
		t.Fatal("manual SFX failed")
	}
	manual, _ := player.Channel(1)
	if !manual.Active || manual.Music {
		t.Fatalf("manual channel = %+v", manual)
	}

	player.MixMono(make([]int8, 129))
	reclaimed, _ := player.Channel(1)
	if !reclaimed.Active || !reclaimed.Music || reclaimed.SFXID != 7 {
		t.Fatalf("next music pattern did not reclaim channel: %+v", reclaimed)
	}
	if _, ok := player.Play(2, 0, 0, 0); !ok {
		t.Fatal("second manual SFX failed")
	}
	if !player.Music(-1, 0) {
		t.Fatal("music stop failed")
	}
	channel0, _ := player.Channel(0)
	channel1, _ := player.Channel(1)
	if !channel0.Active || channel0.Music || channel1.Active || player.MusicState().Active {
		t.Fatalf("stop ownership channels=%+v/%+v music=%+v", channel0, channel1, player.MusicState())
	}
}

func TestMusicRejectsMalformedDataAndInvalidFade(t *testing.T) {
	player := musicFixture(t)
	if player.Music(0, -1) || player.Music(0, MaxMusicFadeMS+1) || player.Music(9, 0) {
		t.Fatal("invalid fade or missing pattern was accepted")
	}
	if got := player.Stats().InvalidMusicCalls; got != 3 {
		t.Fatalf("invalid music calls = %d, want 3", got)
	}

	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{ID: 2, Speed: 1, StepCount: 1, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop}}
	steps := []SFXStep{{PhaseStep: 1, Volume: 1, Kind: SourceSquare}}
	for _, patterns := range [][]MusicPattern{
		{{ID: 0, Channels: [SFXChannels]int8{3, -1, -1, -1}, Next: -1, DurationSamples: 1}},
		{{ID: 0, Channels: [SFXChannels]int8{2, -1, -1, -1}, Next: 1, DurationSamples: 1}},
	} {
		var candidate SFXPlayer
		if !candidate.Init(mixer, definitions, steps) {
			t.Fatal("candidate Init failed")
		}
		if candidate.InitMusic(patterns) {
			t.Fatalf("malformed patterns accepted: %#v", patterns)
		}
	}
}

func TestZeroDurationMusicPatternPlaysIndefinitely(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := []SFXDefinition{{ID: 2, Speed: 1, StepCount: 1, LoopStart: 0, LoopEnd: 1}}
	steps := []SFXStep{{PhaseStep: 1, Volume: MaxVolume, Kind: SourceSquare}}
	var player SFXPlayer
	if !player.Init(mixer, definitions, steps) || !player.InitMusic([]MusicPattern{{
		ID: 0, Channels: [SFXChannels]int8{2, -1, -1, -1}, Next: -1, DurationSamples: 0,
	}}) || !player.Music(0, 0) {
		t.Fatal("failed to start infinite music pattern")
	}
	player.MixMono(make([]int8, 4096))
	state := player.MusicState()
	channel, _ := player.Channel(0)
	if !state.Active || state.PatternID != 0 || state.SamplesRemaining != 0 || !channel.Active {
		t.Fatalf("infinite pattern stopped or transitioned: music=%+v channel=%+v", state, channel)
	}
}

func TestMusicFadeInAndOutUseExactSampleClock(t *testing.T) {
	player := musicFixture(t)
	if !player.Music(0, 1) {
		t.Fatal("one-millisecond fade-in failed")
	}
	if state := player.MusicState(); state.FadeSamplesRemaining != 20 || state.FadingOut {
		t.Fatalf("initial fade-in state = %+v", state)
	}
	samples := make([]int8, 21)
	player.MixMono(samples)
	// The global music fade and PICO's per-voice 48-sample anti-click attack
	// multiply while both are active.
	if samples[0] != 0 || samples[10] != 14 || samples[19] != 49 || samples[20] != 55 {
		t.Fatalf("fade-in samples [0,10,19,20] = [%d,%d,%d,%d]", samples[0], samples[10], samples[19], samples[20])
	}
	if state := player.MusicState(); state.FadeSamplesRemaining != 0 || state.FadingOut {
		t.Fatalf("completed fade-in state = %+v", state)
	}

	if !player.Music(-1, 1) {
		t.Fatal("one-millisecond fade-out failed")
	}
	samples = make([]int8, 20)
	player.MixMono(samples)
	if samples[0] != 58 || samples[10] != 42 || samples[19] != 5 {
		t.Fatalf("fade-out samples [0,10,19] = [%d,%d,%d]", samples[0], samples[10], samples[19])
	}
	if state := player.MusicState(); !state.Active || state.FadeSamplesRemaining != 0 || !state.FadingOut {
		t.Fatalf("state at exact fade-out boundary = %+v", state)
	}
	tail := make([]int8, 1)
	player.MixMono(tail)
	if tail[0] != 0 || player.MusicState().Active {
		t.Fatalf("fade-out tail=%v state=%+v", tail, player.MusicState())
	}
}

func TestMusicFadeOutPreservesManualSFX(t *testing.T) {
	player := musicFixture(t)
	if !player.Music(0, 0) {
		t.Fatal("Music failed")
	}
	if _, ok := player.Play(7, 1, 0, 0); !ok {
		t.Fatal("manual SFX failed")
	}
	if !player.Music(-1, 1) {
		t.Fatal("music fade-out failed")
	}
	player.MixMono(make([]int8, 21))
	manual, _ := player.Channel(1)
	if !manual.Active || manual.Music || player.MusicState().Active {
		t.Fatalf("fade-out ownership manual=%+v music=%+v", manual, player.MusicState())
	}
}

func TestMusicFadeWorksThroughWordMixer(t *testing.T) {
	player := musicFixture(t)
	if !player.Music(0, 1) {
		t.Fatal("Music fade-in failed")
	}
	player.MixMonoWords(make([]uint32, 5))
	if state := player.MusicState(); !state.Active || state.FadeSamplesRemaining != 0 {
		t.Fatalf("word-mixed fade state = %+v", state)
	}
}

func TestMusicFadeClockSpansPatternTransitions(t *testing.T) {
	player := musicFixture(t)
	if !player.Music(0, 100) {
		t.Fatal("long music fade-in failed")
	}
	player.MixMono(make([]int8, 129))
	state := player.MusicState()
	if state.PatternID != 1 || state.FadeSamplesRemaining != 1511 || state.FadingOut {
		t.Fatalf("fade-in after transition = %+v", state)
	}
	if !player.Music(-1, 100) {
		t.Fatal("long music fade-out failed")
	}
	player.MixMono(make([]int8, 256))
	state = player.MusicState()
	if !state.Active || !state.FadingOut || state.FadeSamplesRemaining != 1384 {
		t.Fatalf("fade-out after transitions = %+v", state)
	}
}

func TestMusicMixAllocatesNothing(t *testing.T) {
	player := musicFixture(t)
	if !player.Music(0, 0) {
		t.Fatal("Music failed")
	}
	buffer := make([]int8, 512)
	if got := testing.AllocsPerRun(100, func() { player.MixMono(buffer) }); got != 0 {
		t.Fatalf("music mix allocations = %v, want 0", got)
	}
}

func TestMusicFourChannelPatternGoldenPCM(t *testing.T) {
	mixer := newMixer(t, 4)
	definitions := make([]SFXDefinition, SFXChannels)
	steps := make([]SFXStep, SFXChannels)
	var channels [SFXChannels]int8
	for index := 0; index < SFXChannels; index++ {
		definitions[index] = SFXDefinition{
			ID: uint8(index), Speed: 1, StepOffset: uint16(index), StepCount: 1,
			LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
		}
		steps[index] = SFXStep{PhaseStep: 1 << 31, Volume: 64, Kind: SourceSquare}
		channels[index] = int8(index)
	}
	var player SFXPlayer
	patterns := []MusicPattern{{ID: 0, Channels: channels, Next: -1, DurationSamples: 128}}
	if !player.Init(mixer, definitions, steps) || !player.InitMusic(patterns) || !player.Music(0, 0) {
		t.Fatal("four-channel music setup failed")
	}
	got := make([]int8, 16)
	player.MixMono(got)
	want := []int8{0, -8, 4, -12, 12, -16, 16, -24, 20, -28, 28, -32, 32, -40, 36, -44}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("golden sample %d = %d, want %d; block=%v", index, got[index], want[index], got)
		}
	}
	if stats := player.Stats(); stats.ActiveChannels != SFXChannels || !stats.MusicActive {
		t.Fatalf("four-channel music stats = %+v", stats)
	}
}
