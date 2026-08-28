//go:build !tinygo

package audio

import (
	"testing"
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

func startTestStream(t *testing.T, stream *MonoStream, mixer *Mixer) {
	t.Helper()
	hw.ResetForTest()
	if !stream.Start(mixer, hw.DirectSoundRate16384) {
		t.Fatal("MonoStream.Start failed")
	}
	t.Cleanup(stream.Stop)
}

func startTestSFXStream(t *testing.T, stream *MonoStream, player *SFXPlayer) {
	t.Helper()
	hw.ResetForTest()
	if !stream.StartSFX(player, hw.DirectSoundRate16384) {
		t.Fatal("MonoStream.StartSFX failed")
	}
	t.Cleanup(stream.Stop)
}

func blockIsSilent(block []uint32) bool {
	for _, word := range block {
		if word != 0 {
			return false
		}
	}
	return true
}

func TestStreamRedirectProgressSettlesOnlyDMAThresholdRace(t *testing.T) {
	for progress := uint16(0); progress < 64; progress++ {
		want := progress&15 == 4 || progress&15 == 5
		if got := streamRedirectProgressUnsettled(progress); got != want {
			t.Fatalf("progress %d unsettled = %t, want %t", progress, got, want)
		}
	}
}

func TestMonoStreamPrimesContinuousPackedPages(t *testing.T) {
	mixer := newMixer(t, 4)
	expectedMixer := newMixer(t, 4)
	request := Request{Kind: SourceSaw, Step: 0x1234567, Volume: 96}
	start(t, mixer, request)
	start(t, expectedMixer, request)

	var stream MonoStream
	startTestStream(t, &stream, mixer)
	var expected [StreamPageCount * StreamBlockWords]uint32
	expectedMixer.MixMonoWords(expected[:])
	for page := 0; page < StreamPageCount; page++ {
		block := stream.pageBlock(uint8(page))
		for word := 0; word < StreamBlockWords; word++ {
			got := block[word]
			want := expected[page*StreamBlockWords+word]
			if got != want {
				t.Fatalf("page %d word %d = %#08x, want %#08x", page, word, got, want)
			}
		}
		dma := stream.pageDMA(uint8(page))
		for word := 0; word < StreamGuardWords; word++ {
			got := dma[StreamBlockWords+word]
			wantIndex := ((page+1)*StreamBlockWords + word) % len(expected)
			if want := expected[wantIndex]; got != want {
				t.Fatalf("page %d guard word %d = %#08x, want continuation %#08x", page, word, got, want)
			}
		}
	}
	stats := stream.Stats()
	if !stats.Running || stats.QueuedBlocks != 3 || stats.MixedBlocks != StreamPageCount {
		t.Fatalf("initial stream stats = %+v", stats)
	}
}

func TestMonoStreamCanUseSampleClockedSFXProducer(t *testing.T) {
	newPlayer := func() *SFXPlayer {
		mixer := newMixer(t, 4)
		definitions := []SFXDefinition{{
			ID: 0, Speed: 1, StepCount: 2, LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
		}}
		steps := []SFXStep{
			{PhaseStep: 1 << 31, Volume: MaxVolume, Kind: SourceSquare},
			{PhaseStep: 1 << 30, Volume: MaxVolume, Kind: SourcePulse},
		}
		var player SFXPlayer
		if !player.Init(mixer, definitions, steps) {
			t.Fatal("SFXPlayer.Init failed")
		}
		if _, ok := player.Play(0, 0, 0, 0); !ok {
			t.Fatal("SFXPlayer.Play failed")
		}
		return &player
	}
	player := newPlayer()
	expectedPlayer := newPlayer()

	hw.ResetForTest()
	var stream MonoStream
	if !stream.StartSFX(player, hw.DirectSoundRate16384) {
		t.Fatal("MonoStream.StartSFX failed")
	}
	t.Cleanup(stream.Stop)
	var expected [StreamPageCount * StreamBlockWords]uint32
	expectedPlayer.MixMonoWords(expected[:])
	for page := 0; page < StreamPageCount; page++ {
		for word, got := range stream.pageBlock(uint8(page)) {
			if want := expected[page*StreamBlockWords+word]; got != want {
				t.Fatalf("page %d word %d = %#08x, want %#08x", page, word, got, want)
			}
		}
	}
	if stream.StartSFX(nil, hw.DirectSoundRate16384) {
		t.Fatal("nil SFX producer start succeeded")
	}
}

func TestMonoStreamRewindsQueuedSFXIntoPageAfterNext(t *testing.T) {
	_, player := sfxFixture(t)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)

	immediate := stream.nextPage
	target := nextStreamPage(immediate)
	tail := nextStreamPage(target)
	if !blockIsSilent(stream.pageBlock(immediate)) || !blockIsSilent(stream.pageBlock(target)) ||
		!blockIsSilent(stream.pageBlock(tail)) {
		t.Fatal("inactive SFX stream did not start with silent queued pages")
	}
	if channel, ok := player.Play(2, -1, 0, 0); !ok || channel != -1 {
		t.Fatalf("deferred Play channel=%d ok=%t, want accepted unresolved auto channel", channel, ok)
	}
	if stats := player.Stats(); stats.PendingCommands != 1 || stats.QueuedCommands != 1 || stats.Starts != 0 {
		t.Fatalf("pre-service SFX stats = %+v", stats)
	}

	if got := stream.Service(StreamPageCount); got != 0 {
		t.Fatalf("rewind Service produced %d free pages, want deferred tail refill", got)
	}
	if !blockIsSilent(stream.pageBlock(immediate)) {
		t.Fatal("rewind changed the immediate-next safety page")
	}
	if blockIsSilent(stream.pageBlock(target)) {
		t.Fatal("queued effect was not mixed into the page after next")
	}
	if !blockIsSilent(stream.pageBlock(tail)) || stream.readyMask&(uint8(1)<<tail) != 0 || stream.writePage != tail {
		t.Fatalf("stale tail was not discarded: ready=%04b write=%d tail=%d", stream.readyMask, stream.writePage, tail)
	}
	if stats := stream.Stats(); stats.QueuedBlocks != 2 || stats.MixedBlocks != StreamPageCount {
		t.Fatalf("post-rewind stream stats = %+v", stats)
	}
	if stats := player.Stats(); stats.PendingCommands != 0 || stats.Rewinds != 1 || stats.Starts != 1 || stats.FallbackCommands != 0 {
		t.Fatalf("post-rewind SFX stats = %+v", stats)
	}

	if got := stream.Service(StreamPageCount); got != 1 {
		t.Fatalf("tail refill produced %d pages, want one", got)
	}
	if stats := stream.Stats(); stats.QueuedBlocks != 3 || stats.MixedBlocks != StreamPageCount+1 {
		t.Fatalf("post-tail-refill stream stats = %+v", stats)
	}
}

func TestMonoStreamRewindsQueuedMusicWithPatternCheckpoint(t *testing.T) {
	player := musicFixture(t)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)

	immediate := stream.nextPage
	target := nextStreamPage(immediate)
	if !player.Music(0, 0) {
		t.Fatal("deferred Music failed")
	}
	if state := player.MusicState(); state.Active {
		t.Fatalf("queued music mutated speculative state early: %+v", state)
	}
	if got := stream.Service(StreamPageCount); got != 0 {
		t.Fatalf("music rewind produced %d pages, want deferred tail refill", got)
	}
	if !blockIsSilent(stream.pageBlock(immediate)) {
		t.Fatal("music rewind changed immediate-next safety page")
	}
	if blockIsSilent(stream.pageBlock(target)) {
		t.Fatal("queued music was not mixed into page after next")
	}
	state := player.MusicState()
	stats := player.Stats()
	if !state.Active || state.PatternID != 1 || state.SamplesRemaining != 128 || stats.PendingCommands != 0 || stats.Rewinds != 1 || stats.MusicStarts != 1 || stats.MusicTransitions != 1 {
		t.Fatalf("post-rewind music=%+v stats=%+v", state, stats)
	}
}

func TestMonoStreamRewindsQueuedMusicFadeWithGainCheckpoint(t *testing.T) {
	player := musicFixture(t)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)

	immediate := stream.nextPage
	target := nextStreamPage(immediate)
	if !player.Music(0, 1000) {
		t.Fatal("deferred faded Music failed")
	}
	if got := stream.Service(StreamPageCount); got != 0 {
		t.Fatalf("music fade rewind produced %d pages, want deferred tail refill", got)
	}
	if !blockIsSilent(stream.pageBlock(immediate)) {
		t.Fatal("music fade rewind changed immediate-next safety page")
	}
	if blockIsSilent(stream.pageBlock(target)) {
		t.Fatal("faded music was not mixed into the page after next")
	}
	state := player.MusicState()
	if !state.Active || state.FadeSamplesRemaining != MusicSampleRate-StreamBlockSamples {
		t.Fatalf("post-rewind faded music state = %+v", state)
	}
	if stats := player.Stats(); stats.PendingCommands != 0 || stats.Rewinds != 1 || stats.MusicFadeSamplesRemaining != state.FadeSamplesRemaining {
		t.Fatalf("post-rewind faded music stats = %+v", stats)
	}
}

func TestMonoStreamRewindsFullQueueOfMusicFades(t *testing.T) {
	player := musicFixture(t)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)

	immediate := stream.nextPage
	target := nextStreamPage(immediate)
	for index := 0; index < SFXCommandCapacity; index++ {
		if !player.Music(0, 1000) {
			t.Fatalf("deferred faded Music command %d failed", index)
		}
	}
	if got := stream.Service(StreamPageCount); got != 0 {
		t.Fatalf("full music-fade rewind produced %d pages, want deferred tail refill", got)
	}
	if !blockIsSilent(stream.pageBlock(immediate)) || blockIsSilent(stream.pageBlock(target)) {
		t.Fatal("full music-fade rewind changed safety page or left target silent")
	}
	state := player.MusicState()
	stats := player.Stats()
	if state.FadeSamplesRemaining != MusicSampleRate-StreamBlockSamples ||
		stats.PendingCommands != 0 || stats.QueuedCommands != SFXCommandCapacity ||
		stats.MusicStarts != SFXCommandCapacity || stats.Rewinds != 1 || stats.DroppedCommands != 0 {
		t.Fatalf("full music-fade rewind state=%+v stats=%+v", state, stats)
	}
}

func TestMonoStreamRewindPreservesImmediateAudioAndAppliesStop(t *testing.T) {
	_, player := sfxFixture(t)
	if _, ok := player.Play(7, 0, 0, 0); !ok {
		t.Fatal("looped startup Play failed")
	}
	var stream MonoStream
	startTestSFXStream(t, &stream, player)
	immediate := stream.nextPage
	target := nextStreamPage(immediate)
	if blockIsSilent(stream.pageBlock(immediate)) || blockIsSilent(stream.pageBlock(target)) {
		t.Fatal("looped startup effect did not fill queued pages")
	}
	immediateBefore := append([]uint32(nil), stream.pageBlock(immediate)...)

	if !player.Stop(0) {
		t.Fatal("deferred Stop failed")
	}
	stream.Service(StreamPageCount)
	for index, got := range stream.pageBlock(immediate) {
		if got != immediateBefore[index] {
			t.Fatalf("immediate page word %d changed from %#08x to %#08x", index, immediateBefore[index], got)
		}
	}
	if !blockIsSilent(stream.pageBlock(target)) {
		t.Fatal("deferred stop did not silence the page after next")
	}
	if stats := player.Stats(); stats.PendingCommands != 0 || stats.Rewinds != 1 || stats.Stops == 0 {
		t.Fatalf("stop rewind stats = %+v", stats)
	}
}

func TestMonoStreamSFXCommandQueueIsBounded(t *testing.T) {
	_, player := sfxFixture(t)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)
	for index := 0; index < SFXCommandCapacity; index++ {
		if _, ok := player.Play(2, -1, 0, 0); !ok {
			t.Fatalf("queued Play %d failed before capacity", index)
		}
	}
	if _, ok := player.Play(2, -1, 0, 0); ok {
		t.Fatal("command beyond fixed queue capacity succeeded")
	}
	stats := player.Stats()
	if stats.PendingCommands != SFXCommandCapacity || stats.QueuedCommands != SFXCommandCapacity || stats.DroppedCommands != 1 {
		t.Fatalf("bounded queue stats = %+v", stats)
	}
	stream.Service(StreamPageCount)
	if stats := player.Stats(); stats.PendingCommands != 0 || stats.Starts != SFXCommandCapacity || stats.Rewinds != 1 {
		t.Fatalf("applied bounded queue stats = %+v", stats)
	}
}

func TestMonoStreamSFXMissingCheckpointFallsBackToProducerTail(t *testing.T) {
	_, player := sfxFixture(t)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)
	target := nextStreamPage(stream.nextPage)
	player.invalidateCheckpoint(target)
	if _, ok := player.Play(2, 0, 0, 0); !ok {
		t.Fatal("deferred Play failed")
	}
	if got := stream.Service(StreamPageCount); got != 0 {
		t.Fatalf("full-queue fallback produced %d pages, want zero", got)
	}
	if stats := player.Stats(); stats.PendingCommands != 0 || stats.FallbackCommands != 1 || stats.Rewinds != 0 || stats.Starts != 1 {
		t.Fatalf("fallback stats = %+v", stats)
	}
	if stats := stream.Stats(); stats.QueuedBlocks != 3 {
		t.Fatalf("fallback changed full queue = %+v", stats)
	}
}

func TestMonoStreamRewindUsesTwoReadyPagesWithoutFallback(t *testing.T) {
	_, player := sfxFixture(t)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)
	if _, ok := player.Play(2, 0, 0, 0); !ok {
		t.Fatal("deferred Play failed")
	}

	// Model Timer 1 consuming one page after Service observed a full queue.
	// Immediate and page-after-next remain ready, so no refill is required
	// before the rewind.
	stream.onBlockBoundary()
	if got := stream.rewindPendingSFX(); got != rewindComplete {
		t.Fatalf("two-page rewind result = %d, want complete", got)
	}
	if stats := player.Stats(); stats.PendingCommands != 0 || stats.FallbackCommands != 0 || stats.Rewinds != 1 {
		t.Fatalf("two-page rewind state = %+v", stats)
	}
	if stats := stream.Stats(); stats.QueuedBlocks != 2 {
		t.Fatalf("two-page rewind queue = %+v", stats)
	}
}

func TestMonoStreamRewindSnapshotRaceRetriesBelowTwoPages(t *testing.T) {
	_, player := sfxFixture(t)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)
	if _, ok := player.Play(2, 0, 0, 0); !ok {
		t.Fatal("deferred Play failed")
	}
	stream.onBlockBoundary()
	stream.onBlockBoundary()
	if got := stream.rewindPendingSFX(); got != rewindRetry {
		t.Fatalf("one-page rewind result = %d, want retry", got)
	}
	if stats := player.Stats(); stats.PendingCommands != 1 || stats.FallbackCommands != 0 || stats.Rewinds != 0 {
		t.Fatalf("one-page retry mutated command state: %+v", stats)
	}
}

func TestMonoStreamLateRewindWaitsForSafeFullQueueWithoutFallback(t *testing.T) {
	_, player := sfxFixture(t)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)
	if _, ok := player.Play(2, 0, 0, 0); !ok {
		t.Fatal("deferred Play failed")
	}

	hw.SetReg16(hw.TM1CNT_L, 0xFF00+StreamRewindStartCutoffSamples)
	if got := stream.Service(StreamPageCount); got != 0 {
		t.Fatalf("late full-queue Service produced %d pages, want deferred command", got)
	}
	if stats := player.Stats(); stats.PendingCommands != 1 || stats.Rewinds != 0 || stats.FallbackCommands != 0 {
		t.Fatalf("late deferred command stats = %+v", stats)
	}

	stream.onBlockBoundary()
	if got := stream.Service(StreamPageCount); got != 1 {
		t.Fatalf("pending-command refill produced %d pages, want one old-timeline page", got)
	}
	if stats := player.Stats(); stats.PendingCommands != 1 || stats.Rewinds != 0 || stats.FallbackCommands != 0 {
		t.Fatalf("post-refill deferred command stats = %+v", stats)
	}
	if stats := stream.Stats(); stats.QueuedBlocks != StreamPageCount-1 {
		t.Fatalf("post-refill queue = %+v", stats)
	}

	hw.SetReg16(hw.TM1CNT_L, 0xFF00)
	if got := stream.Service(StreamPageCount); got != 0 {
		t.Fatalf("safe deferred rewind produced %d pages, want tail deferred", got)
	}
	if stats := player.Stats(); stats.PendingCommands != 0 || stats.Rewinds != 1 || stats.FallbackCommands != 0 {
		t.Fatalf("safe retry stats = %+v", stats)
	}
}

func TestMonoStreamStopDropsPendingSFXAndRestoresImmediateCalls(t *testing.T) {
	_, player := sfxFixture(t)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)
	if _, ok := player.Play(2, 0, 0, 0); !ok {
		t.Fatal("deferred Play failed")
	}
	stream.Stop()
	if stats := player.Stats(); stats.PendingCommands != 0 || stats.DroppedCommands != 1 || stats.Starts != 0 {
		t.Fatalf("stopped pending-command stats = %+v", stats)
	}
	if channel, ok := player.Play(2, 0, 0, 0); !ok || channel != 0 {
		t.Fatalf("post-stop immediate Play channel=%d ok=%t", channel, ok)
	}
	if stats := player.Stats(); stats.Starts != 1 || stats.PendingCommands != 0 {
		t.Fatalf("post-stop immediate stats = %+v", stats)
	}
}

func TestMonoStreamBoundaryRefillAndDMAHandoff(t *testing.T) {
	mixer := newMixer(t, 4)
	start(t, mixer, Request{Kind: SourceTriangle, Step: 1 << 25, Volume: 128})
	var stream MonoStream
	startTestStream(t, &stream, mixer)

	stream.onBlockBoundary()
	if stream.playPage != 1 || stream.nextPage != 2 {
		t.Fatalf("handoff state play=%d next=%d, want 1/2", stream.playPage, stream.nextPage)
	}
	if got, want := hw.GetReg32(hw.DMA1SAD), uint32(uintptr(unsafe.Pointer(&stream.pageDMA(1)[5]))); got != want {
		t.Fatalf("DMA1SAD after handoff = %#08x, want %#08x", got, want)
	}
	if got := stream.Service(StreamPageCount); got != 1 {
		t.Fatalf("Service produced %d blocks, want one newly free page", got)
	}
	stats := stream.Stats()
	if stats.Boundaries != 1 || stats.Underruns != 0 || stats.QueuedBlocks != 3 || stats.MixedBlocks != 5 {
		t.Fatalf("post-refill stats = %+v", stats)
	}
}

func TestMonoStreamUnderrunUsesSilenceAndRecoversInOrder(t *testing.T) {
	mixer := newMixer(t, 4)
	start(t, mixer, Request{Kind: SourceSquare, Step: 1 << 31, Volume: 64})
	var stream MonoStream
	startTestStream(t, &stream, mixer)

	for i := 0; i < StreamPageCount; i++ {
		stream.onBlockBoundary()
	}
	stats := stream.Stats()
	if stats.Underruns != 1 || stream.playPage != noStreamPage || stream.nextPage != 0 {
		t.Fatalf("underrun state stats=%+v play=%d next=%d", stats, stream.playPage, stream.nextPage)
	}
	if got, want := hw.GetReg32(hw.DMA1SAD), uint32(uintptr(unsafe.Pointer(&stream.silence[5]))); got != want {
		t.Fatalf("underrun DMA1SAD = %#08x, want silence %#08x", got, want)
	}
	for i, word := range stream.pageBlock(0) {
		if word != 0 {
			t.Fatalf("consumed page 0 word %d = %#08x, want cleared fallback", i, word)
		}
	}

	if got := stream.Service(1); got != 1 {
		t.Fatalf("recovery Service produced %d blocks, want 1", got)
	}
	stream.onBlockBoundary()
	if stream.playPage != 0 || stream.nextPage != 1 {
		t.Fatalf("recovery handoff play=%d next=%d, want 0/1", stream.playPage, stream.nextPage)
	}
	if got := stream.Stats().Underruns; got != 1 {
		t.Fatalf("underruns after recovery = %d, want 1", got)
	}
}

func TestMonoStreamTracksLateBoundary(t *testing.T) {
	mixer := newMixer(t, 4)
	var stream MonoStream
	startTestStream(t, &stream, mixer)
	hw.SetReg16(hw.TM1CNT_L, 0xFF09)
	stream.onBlockBoundary()
	stats := stream.Stats()
	if stats.LateBoundaries != 1 || stats.MaxLateSamples != 9 {
		t.Fatalf("late telemetry = %+v, want one boundary at 9 samples", stats)
	}
}

func TestMonoStreamInvalidBoundaryProgressStopsHardware(t *testing.T) {
	mixer := newMixer(t, 4)
	var stream MonoStream
	startTestStream(t, &stream, mixer)
	hw.SetReg16(hw.TM1CNT_L, 0) // wraps to progress 256, outside the block
	stream.onBlockBoundary()
	stats := stream.Stats()
	if stats.Running || stats.SwitchFailures != 1 {
		t.Fatalf("invalid-progress stats = %+v, want stopped with one failure", stats)
	}
	if got := hw.GetReg16(hw.SOUNDCNT_X); got != 0 {
		t.Fatalf("invalid progress left sound master at %#04x", got)
	}
}

func TestMonoStreamRefusesUnsafeImmediateRefill(t *testing.T) {
	mixer := newMixer(t, 12)
	for i := 0; i < 12; i++ {
		start(t, mixer, Request{Kind: SourceTriangle, Step: uint32(1<<23 + i), Volume: 8})
	}
	var stream MonoStream
	startTestStream(t, &stream, mixer)
	for i := 0; i < StreamPageCount-1; i++ {
		stream.onBlockBoundary()
	}
	if stream.playPage != 3 || stream.nextPage != 0 || stream.writePage != 0 {
		t.Fatalf("pre-cutoff state play=%d next=%d write=%d", stream.playPage, stream.nextPage, stream.writePage)
	}

	hw.SetReg16(hw.TM1CNT_L, 0xFF00+StreamImmediateRefillCutoffSamples)
	if got := stream.Service(1); got != 0 {
		t.Fatalf("unsafe immediate refill produced %d blocks, want 0", got)
	}
	for i, word := range stream.pageBlock(0) {
		if word != 0 {
			t.Fatalf("deferred page 0 word %d = %#08x, want cleared silence", i, word)
		}
	}

	hw.SetReg16(hw.TM1CNT_L, 0xFF00+StreamImmediateRefillCutoffSamples-1)
	if got := stream.Service(1); got != 1 {
		t.Fatalf("safe immediate refill produced %d blocks, want 1", got)
	}
}

func TestMonoStreamHasExclusiveHardwareOwnership(t *testing.T) {
	firstMixer := newMixer(t, 4)
	secondMixer := newMixer(t, 4)
	var first, second MonoStream
	startTestStream(t, &first, firstMixer)
	if second.Start(secondMixer, hw.DirectSoundRate16384) {
		second.Stop()
		t.Fatal("second stream claimed FIFO A while first was running")
	}
	first.Stop()
	if !second.Start(secondMixer, hw.DirectSoundRate16384) {
		t.Fatal("second stream could not claim FIFO A after release")
	}
	second.Stop()
}

func TestMonoStreamStopAndInvalidStartsFailClosed(t *testing.T) {
	var stream MonoStream
	if stream.Start(nil, hw.DirectSoundRate16384) {
		t.Fatal("nil mixer start succeeded")
	}
	mixer := newMixer(t, 4)
	if stream.Start(mixer, hw.DirectSoundRate(255)) {
		t.Fatal("unknown rate start succeeded")
	}
	if stream.Start(mixer, hw.DirectSoundRate18157) {
		t.Fatal("unmeasured production rate start succeeded")
	}
	startTestStream(t, &stream, mixer)
	stream.Stop()
	stats := stream.Stats()
	if stats.Running || stats.QueuedBlocks != 0 {
		t.Fatalf("stopped stats = %+v", stats)
	}
	if got := hw.GetReg16(hw.SOUNDCNT_X); got != 0 {
		t.Fatalf("SOUNDCNT_X after stop = %#04x, want disabled", got)
	}
}

func TestMonoStreamServiceAllocatesNothing(t *testing.T) {
	mixer := newMixer(t, 4)
	start(t, mixer, Request{Kind: SourceTriangle, Step: 1 << 25, Volume: 64})
	var stream MonoStream
	startTestStream(t, &stream, mixer)
	if got := testing.AllocsPerRun(100, func() {
		stream.onBlockBoundary()
		stream.Service(1)
	}); got != 0 {
		t.Fatalf("stream boundary/refill allocations = %v, want 0", got)
	}
}

func TestMonoStreamSFXCommandRewindAllocatesNothing(t *testing.T) {
	_, player := sfxFixture(t)
	var stream MonoStream
	startTestSFXStream(t, &stream, player)
	if got := testing.AllocsPerRun(100, func() {
		if _, ok := player.Play(2, 0, 0, 0); !ok {
			panic("deferred Play failed")
		}
		stream.Service(StreamPageCount)
		stream.Service(StreamPageCount)
	}); got != 0 {
		t.Fatalf("SFX command/rewind/refill allocations = %v, want 0", got)
	}
}
