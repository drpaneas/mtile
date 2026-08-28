package audio

import "testing"

func sampleDefinitions() ([]SampleDefinition, []int8) {
	pcm := []int8{10, 20, 100, -100, 30, 40}
	definitions := []SampleDefinition{
		{ID: 2, DataOffset: 0, DataLength: 2, Volume: MaxVolume, Priority: 32},
		{ID: 7, DataOffset: 2, DataLength: 4, Loop: true, LoopStart: 1, LoopEnd: 4, Volume: MaxVolume, Priority: 48},
	}
	return definitions, pcm
}

func newSamplePlayer(t *testing.T, voices int) (*Mixer, *SFXPlayer) {
	t.Helper()
	mixer := newMixer(t, voices)
	var player SFXPlayer
	if !player.Init(mixer, nil, nil) {
		t.Fatal("SFXPlayer.Init failed")
	}
	definitions, pcm := sampleDefinitions()
	if !player.InitSamples(definitions, pcm) {
		t.Fatal("InitSamples failed")
	}
	return mixer, &player
}

func TestSamplePlaybackUsesDefaultsOverridesAndLoopPoints(t *testing.T) {
	_, player := newSamplePlayer(t, 6)
	if !player.Sample(2, -1, -1) {
		t.Fatal("default sample start failed")
	}
	dst := make([]int8, 3)
	player.MixMono(dst)
	requireSamples(t, dst, []int8{10, 20, 0})
	if stats := player.Stats(); stats.SampleStarts != 1 || stats.ActiveSampleVoices != 0 {
		t.Fatalf("finished sample stats = %+v", stats)
	}

	if !player.Sample(7, 128, 200) {
		t.Fatal("looping sample override start failed")
	}
	dst = make([]int8, 7)
	player.MixMono(dst)
	requireSamples(t, dst, []int8{50, -50, 15, 20, -50, 15, 20})
	if stats := player.Stats(); stats.SampleStarts != 2 || stats.ActiveSampleVoices != 1 {
		t.Fatalf("looping sample stats = %+v", stats)
	}
}

func TestSamplePlaybackAppliesMasterVolumeToDefaultsAndOverrides(t *testing.T) {
	_, player := newSamplePlayer(t, 6)
	if !player.SetMasterVolume(MaxVolume / 2) {
		t.Fatal("SetMasterVolume failed")
	}
	if !player.Sample(2, -1, -1) {
		t.Fatal("default sample start failed")
	}
	dst := make([]int8, 2)
	player.MixMono(dst)
	requireSamples(t, dst, []int8{5, 10})

	if !player.Sample(2, 128, -1) {
		t.Fatal("overridden sample start failed")
	}
	dst = make([]int8, 2)
	player.MixMono(dst)
	requireSamples(t, dst, []int8{2, 5})
}

func TestSampleStopLeavesSynthVoicesAndTracksFailures(t *testing.T) {
	mixer, player := newSamplePlayer(t, 4)
	start(t, mixer, Request{Kind: SourceSquare, Step: 1, Volume: 1, Protected: true})
	if !player.Sample(7, -1, -1) {
		t.Fatal("sample start failed")
	}
	if !player.Sample(-1, -1, -1) {
		t.Fatal("sample stop-all failed")
	}
	if stats := player.Stats(); stats.ActiveSampleVoices != 0 || stats.SampleStops != 1 {
		t.Fatalf("sample stop stats = %+v", stats)
	}
	if stats := mixer.Stats(); stats.ActiveVoices != 1 {
		t.Fatalf("sample stop removed synth: %+v", stats)
	}

	for mixer.Stats().ActiveVoices < 4 {
		start(t, mixer, Request{Kind: SourceSquare, Step: 1, Volume: 1, Protected: true})
	}
	if player.Sample(2, -1, -1) {
		t.Fatal("sample unexpectedly started against four protected voices")
	}
	if stats := player.Stats(); stats.FailedSampleStarts != 1 {
		t.Fatalf("failed sample stats = %+v", stats)
	}
}

func TestSampleCallsUseStreamCheckpointRewind(t *testing.T) {
	mixer, player := newSamplePlayer(t, 6)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)
	defer stream.Stop()

	if !player.Sample(7, -1, -1) {
		t.Fatal("deferred sample start failed")
	}
	if produced := stream.Service(StreamPageCount); produced != 0 {
		t.Fatalf("rewind produced %d pages, want deferred tail refill", produced)
	}
	stats := player.Stats()
	if stats.PendingCommands != 0 || stats.Rewinds != 1 || stats.SampleStarts != 1 || stats.ActiveSampleVoices != 1 {
		t.Fatalf("sample rewind stats = %+v", stats)
	}
	if produced := stream.Service(StreamPageCount); produced != 1 {
		t.Fatalf("tail refill produced %d pages, want 1", produced)
	}
	if !player.Sample(-1, -1, -1) || stream.Service(StreamPageCount) != 0 {
		t.Fatal("deferred sample stop rewind failed")
	}
	stats = player.Stats()
	if stats.Rewinds != 2 || stats.SampleStops != 1 || stats.ActiveSampleVoices != 0 || stats.FallbackCommands != 0 {
		t.Fatalf("sample stop rewind stats = %+v", stats)
	}
	_ = mixer
}

func TestInitSamplesRejectsMalformedRecords(t *testing.T) {
	mixer := newMixer(t, 6)
	var player SFXPlayer
	if !player.Init(mixer, nil, nil) {
		t.Fatal("player init failed")
	}
	pcm := []int8{1, 2, 3, 4}
	invalid := [][]SampleDefinition{
		{{ID: 0, DataLength: 0, Volume: 1}},
		{{ID: 0, DataOffset: 3, DataLength: 2, Volume: 1}},
		{{ID: 0, DataLength: 4, Volume: MaxVolume + 1}},
		{{ID: 0, DataLength: 4, LoopStart: 1, LoopEnd: 3, Volume: 1}},
		{{ID: 0, DataLength: 4, Loop: true, LoopStart: 1, LoopEnd: 2, Volume: 1}},
		{{ID: 1, DataLength: 4, Volume: 1}, {ID: 1, DataLength: 4, Volume: 1}},
	}
	for _, definitions := range invalid {
		if player.InitSamples(definitions, pcm) {
			t.Fatalf("malformed definitions accepted: %#v", definitions)
		}
	}
	if player.InitSamples(nil, pcm) {
		t.Fatal("orphan PCM data accepted")
	}
}

func TestSampleHotPathAllocatesNothing(t *testing.T) {
	_, player := newSamplePlayer(t, 6)
	dst := make([]int8, 16)
	if got := testing.AllocsPerRun(100, func() {
		player.Sample(7, -1, -1)
		player.MixMono(dst)
		player.Sample(-1, -1, -1)
	}); got != 0 {
		t.Fatalf("sample path allocations = %v, want 0", got)
	}
}
