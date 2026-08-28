//go:build !tinygo

package audio

import (
	"testing"
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

func startTestStereoStream(t *testing.T, stream *StereoStream, mixer *Mixer) {
	t.Helper()
	hw.ResetForTest()
	if !stream.Start(mixer, hw.DirectSoundRate16384) {
		t.Fatal("StereoStream.Start failed")
	}
	t.Cleanup(stream.Stop)
}

func startTestStereoSFXStream(t *testing.T, stream *StereoStream, player *SFXPlayer) {
	t.Helper()
	hw.ResetForTest()
	if !stream.StartSFX(player, hw.DirectSoundRate16384) {
		t.Fatal("StereoStream.StartSFX failed")
	}
	t.Cleanup(stream.Stop)
}

func TestStereoStreamPrimesContinuousPackedPagePairs(t *testing.T) {
	mixer := newMixer(t, 4)
	expectedMixer := newMixer(t, 4)
	requests := []Request{
		{Kind: SourceSaw, Step: 0x1234567, Volume: 96, Pan: MinPan},
		{Kind: SourceTriangle, Step: 0x0765432, Volume: 80, Pan: MaxPan},
	}
	for _, request := range requests {
		start(t, mixer, request)
		start(t, expectedMixer, request)
	}

	var stream StereoStream
	startTestStereoStream(t, &stream, mixer)
	var expectedLeft, expectedRight [StreamPageCount * StreamBlockWords]uint32
	expectedMixer.MixStereoWords(expectedLeft[:], expectedRight[:])
	for page := 0; page < StreamPageCount; page++ {
		left := stream.leftPageBlock(uint8(page))
		right := stream.rightPageBlock(uint8(page))
		for word := 0; word < StreamBlockWords; word++ {
			wantIndex := page*StreamBlockWords + word
			if left[word] != expectedLeft[wantIndex] || right[word] != expectedRight[wantIndex] {
				t.Fatalf("page %d word %d = (%#08x,%#08x), want (%#08x,%#08x)",
					page, word, left[word], right[word], expectedLeft[wantIndex], expectedRight[wantIndex])
			}
		}
		leftDMA, rightDMA := stream.leftPageDMA(uint8(page)), stream.rightPageDMA(uint8(page))
		for word := 0; word < StreamGuardWords; word++ {
			wantIndex := ((page+1)*StreamBlockWords + word) % len(expectedLeft)
			if leftDMA[StreamBlockWords+word] != expectedLeft[wantIndex] ||
				rightDMA[StreamBlockWords+word] != expectedRight[wantIndex] {
				t.Fatalf("page %d guard word %d is not a synchronized continuation", page, word)
			}
		}
	}
	if stats := stream.Stats(); !stats.Running || stats.QueuedBlocks != 3 || stats.MixedBlocks != StreamPageCount {
		t.Fatalf("initial stereo stats = %+v", stats)
	}
}

func TestStereoStreamCanRewindSampleClockedSFX(t *testing.T) {
	_, player := sfxFixture(t)
	var stream StereoStream
	startTestStereoSFXStream(t, &stream, player)

	immediate := stream.nextPage
	target := nextStreamPage(immediate)
	if _, ok := player.Play(2, 0, 0, 0); !ok {
		t.Fatal("deferred stereo SFX Play failed")
	}
	if got := stream.Service(StreamPageCount); got != 0 {
		t.Fatalf("stereo rewind produced %d pages, want deferred tail refill", got)
	}
	if !blockIsSilent(stream.leftPageBlock(immediate)) || !blockIsSilent(stream.rightPageBlock(immediate)) {
		t.Fatal("stereo rewind changed the immediate-next safety pair")
	}
	left, right := stream.leftPageBlock(target), stream.rightPageBlock(target)
	if blockIsSilent(left) || blockIsSilent(right) {
		t.Fatal("stereo rewind did not mix the queued centered effect")
	}
	for index := range left {
		if left[index] != right[index] {
			t.Fatalf("centered SFX page word %d differs left=%#08x right=%#08x", index, left[index], right[index])
		}
	}
	if stats := player.Stats(); stats.PendingCommands != 0 || stats.Rewinds != 1 || stats.FallbackCommands != 0 {
		t.Fatalf("stereo rewind SFX stats = %+v", stats)
	}
}

func TestStereoStreamBoundaryRedirectsBothDMAChannelsTogether(t *testing.T) {
	mixer := newMixer(t, 4)
	start(t, mixer, Request{Kind: SourceTriangle, Step: 1 << 25, Volume: 128, Pan: MinPan})
	start(t, mixer, Request{Kind: SourceSaw, Step: 1 << 24, Volume: 96, Pan: MaxPan})
	var stream StereoStream
	startTestStereoStream(t, &stream, mixer)

	stream.onBlockBoundary()
	if stream.playPage != 1 || stream.nextPage != 2 {
		t.Fatalf("handoff state play=%d next=%d, want 1/2", stream.playPage, stream.nextPage)
	}
	if got, want := hw.GetReg32(hw.DMA1SAD), uint32(uintptr(unsafe.Pointer(&stream.leftPageDMA(1)[5]))); got != want {
		t.Fatalf("DMA1SAD after handoff = %#08x, want %#08x", got, want)
	}
	if got, want := hw.GetReg32(hw.DMA2SAD), uint32(uintptr(unsafe.Pointer(&stream.rightPageDMA(1)[5]))); got != want {
		t.Fatalf("DMA2SAD after handoff = %#08x, want %#08x", got, want)
	}
	if got := stream.Service(1); got != 1 {
		t.Fatalf("Service produced %d stereo pairs, want one", got)
	}
	if stats := stream.Stats(); stats.Boundaries != 1 || stats.Underruns != 0 || stats.QueuedBlocks != 3 || stats.MixedBlocks != 5 {
		t.Fatalf("post-refill stereo stats = %+v", stats)
	}
}

func TestStereoStreamBoundsOneServiceBurst(t *testing.T) {
	mixer := newMixer(t, 4)
	start(t, mixer, Request{Kind: SourceTriangle, Step: 1 << 25, Volume: 64, Pan: MinPan})
	var stream StereoStream
	startTestStereoStream(t, &stream, mixer)
	for index := 0; index < StreamPageCount-1; index++ {
		stream.onBlockBoundary()
	}
	if got := stream.Service(StreamPageCount); got != StereoStreamMaxServiceBlocks {
		t.Fatalf("Service produced %d pairs, want bounded burst of %d", got, StereoStreamMaxServiceBlocks)
	}
	if stats := stream.Stats(); stats.QueuedBlocks != StereoStreamMaxServiceBlocks {
		t.Fatalf("post-burst stats = %+v, want %d queued pairs", stats, StereoStreamMaxServiceBlocks)
	}
}

func TestStereoStreamUnderrunSilencesAndRecoversBothChannels(t *testing.T) {
	mixer := newMixer(t, 4)
	start(t, mixer, Request{Kind: SourceSquare, Step: 1 << 31, Volume: 64, Pan: MinPan})
	var stream StereoStream
	startTestStereoStream(t, &stream, mixer)

	for index := 0; index < StreamPageCount; index++ {
		stream.onBlockBoundary()
	}
	if stats := stream.Stats(); stats.Underruns != 1 || stream.playPage != noStreamPage || stream.nextPage != 0 {
		t.Fatalf("stereo underrun state stats=%+v play=%d next=%d", stats, stream.playPage, stream.nextPage)
	}
	wantSilence := uint32(uintptr(unsafe.Pointer(&stream.silence[5])))
	if got := hw.GetReg32(hw.DMA1SAD); got != wantSilence {
		t.Fatalf("underrun DMA1SAD = %#08x, want silence %#08x", got, wantSilence)
	}
	if got := hw.GetReg32(hw.DMA2SAD); got != wantSilence {
		t.Fatalf("underrun DMA2SAD = %#08x, want silence %#08x", got, wantSilence)
	}
	if !blockIsSilent(stream.leftPageBlock(0)) || !blockIsSilent(stream.rightPageBlock(0)) {
		t.Fatal("consumed stereo page pair was not cleared")
	}

	if got := stream.Service(1); got != 1 {
		t.Fatalf("stereo recovery produced %d pairs, want one", got)
	}
	stream.onBlockBoundary()
	if stream.playPage != 0 || stream.nextPage != 1 || stream.Stats().Underruns != 1 {
		t.Fatalf("stereo recovery state play=%d next=%d stats=%+v", stream.playPage, stream.nextPage, stream.Stats())
	}
}

func TestMonoAndStereoStreamsShareExclusiveHardwareOwnership(t *testing.T) {
	hw.ResetForTest()
	monoMixer := newMixer(t, 4)
	stereoMixer := newMixer(t, 4)
	var mono MonoStream
	var stereo StereoStream
	if !mono.Start(monoMixer, hw.DirectSoundRate16384) {
		t.Fatal("mono start failed")
	}
	if stereo.Start(stereoMixer, hw.DirectSoundRate16384) {
		stereo.Stop()
		t.Fatal("stereo stream claimed hardware while mono was active")
	}
	mono.Stop()
	if !stereo.Start(stereoMixer, hw.DirectSoundRate16384) {
		t.Fatal("stereo stream could not claim released hardware")
	}
	if mono.Start(monoMixer, hw.DirectSoundRate16384) {
		mono.Stop()
		t.Fatal("mono stream claimed hardware while stereo was active")
	}
	stereo.Stop()
}

func TestStereoStreamInvalidBoundaryStopsBothChannels(t *testing.T) {
	mixer := newMixer(t, 4)
	var stream StereoStream
	startTestStereoStream(t, &stream, mixer)
	hw.SetReg16(hw.TM1CNT_L, 0)
	stream.onBlockBoundary()
	if stats := stream.Stats(); stats.Running || stats.SwitchFailures != 1 {
		t.Fatalf("invalid-progress stereo stats = %+v", stats)
	}
	for _, address := range []uintptr{hw.DMA1CNT + 2, hw.DMA2CNT + 2, hw.SOUNDCNT_X} {
		if got := hw.GetReg16(address); got != 0 {
			t.Fatalf("invalid progress left register %#x at %#04x", address, got)
		}
	}
}

func TestStereoStreamServiceAllocatesNothing(t *testing.T) {
	mixer := newMixer(t, 4)
	start(t, mixer, Request{Kind: SourceTriangle, Step: 1 << 25, Volume: 64, Pan: MinPan})
	var stream StereoStream
	startTestStereoStream(t, &stream, mixer)
	if got := testing.AllocsPerRun(100, func() {
		stream.onBlockBoundary()
		stream.Service(1)
	}); got != 0 {
		t.Fatalf("stereo boundary/refill allocations = %v, want 0", got)
	}
}
