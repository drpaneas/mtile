package audio

import (
	"github.com/drpaneas/mtile/runtime/hw"
	_ "unsafe" // required for //go:section
)

// StereoStreamMaxServiceBlocks bounds one normal-context refill burst. The
// accepted two-hard-panned-voice profile can generate two pairs safely, while
// a four-pair catch-up monopolizes more than two sample pages and causes the
// producer to lose ground while it is refilling.
const StereoStreamMaxServiceBlocks = 2

// StereoStream is the experimental fixed four-page producer/consumer queue
// for FIFO A/DMA1 (left) and FIFO B/DMA2 (right). The two page rings publish,
// redirect, underrun, and recover as one unit so channel timing cannot drift.
// Place target instances in EWRAM; the mixer should remain in IWRAM.
//
// StereoStream deliberately remains separate from MonoStream while Milestone
// 5 is measured. Generated games continue to instantiate MonoStream and do not
// pay for the second page ring or stereo mixer code.
type StereoStream struct {
	leftStorage  [StreamStorageWords]uint32
	rightStorage [StreamStorageWords]uint32
	silence      [StreamPageWords]uint32

	mixer *Mixer
	sfx   *SFXPlayer
	clock hw.DirectSoundClock

	readyMask uint8
	playPage  uint8
	nextPage  uint8
	writePage uint8
	running   bool

	mixedBlocks    uint32
	boundaries     uint32
	underruns      uint32
	lateBoundaries uint32
	maxLateSamples uint16
	switchFailures uint32
}

func stereoPageBlock(storage *[StreamStorageWords]uint32, page uint8) []uint32 {
	start := int(page) * StreamBlockWords
	return storage[start : start+StreamBlockWords]
}

func stereoPageDMA(storage *[StreamStorageWords]uint32, page uint8) []uint32 {
	start := int(page) * StreamBlockWords
	return storage[start : start+StreamPageWords]
}

func (s *StereoStream) leftPageBlock(page uint8) []uint32 {
	return stereoPageBlock(&s.leftStorage, page)
}

func (s *StereoStream) rightPageBlock(page uint8) []uint32 {
	return stereoPageBlock(&s.rightStorage, page)
}

func (s *StereoStream) leftPageDMA(page uint8) []uint32 {
	return stereoPageDMA(&s.leftStorage, page)
}

func (s *StereoStream) rightPageDMA(page uint8) []uint32 {
	return stereoPageDMA(&s.rightStorage, page)
}

func refreshStereoWrap(storage *[StreamStorageWords]uint32, page uint8) {
	wrap := storage[StreamRingWords:]
	switch page {
	case 0:
		copy(wrap[:StreamBlockWords], stereoPageBlock(storage, 0))
	case 1:
		copy(wrap[StreamBlockWords:], stereoPageBlock(storage, 1)[:StreamGuardWords-StreamBlockWords])
	}
}

func (s *StereoStream) refreshWrap(page uint8) {
	refreshStereoWrap(&s.leftStorage, page)
	refreshStereoWrap(&s.rightStorage, page)
}

func clearStereoConsumedPage(storage *[StreamStorageWords]uint32, page uint8) {
	block := stereoPageBlock(storage, page)
	for index := range block {
		block[index] = 0
	}
	wrap := storage[StreamRingWords:]
	switch page {
	case 0:
		for index := 0; index < StreamBlockWords; index++ {
			wrap[index] = 0
		}
	case 1:
		for index := StreamBlockWords; index < StreamGuardWords; index++ {
			wrap[index] = 0
		}
	}
}

func (s *StereoStream) clearConsumedPage(page uint8) {
	clearStereoConsumedPage(&s.leftStorage, page)
	clearStereoConsumedPage(&s.rightStorage, page)
}

// Start pre-generates both four-page rings and starts the provisional 16.384
// kHz dual-FIFO backend. Other candidate rates remain rejected until they are
// measured with stereo refill and coexistence costs.
func (s *StereoStream) Start(mixer *Mixer, rate hw.DirectSoundRate) bool {
	return s.start(mixer, nil, rate)
}

// StartSFX selects the sample-clocked SFX/music/sample producer for stereo
// pages. Pan remains internal engine state; game-facing APIs are unchanged.
func (s *StereoStream) StartSFX(player *SFXPlayer, rate hw.DirectSoundRate) bool {
	if player == nil {
		return false
	}
	return s.start(player.mixer, player, rate)
}

func (s *StereoStream) start(mixer *Mixer, player *SFXPlayer, rate hw.DirectSoundRate) bool {
	if s == nil || mixer == nil || s.running || rate != hw.DirectSoundRate16384 {
		return false
	}
	clock, ok := hw.DirectSoundClockForRate(rate)
	if !ok {
		return false
	}

	state := disableStreamInterrupts()
	if !claimStereoStream(s) {
		restoreStreamInterrupts(state)
		return false
	}
	restoreStreamInterrupts(state)

	*s = StereoStream{
		mixer:    mixer,
		sfx:      player,
		clock:    clock,
		playPage: noStreamPage,
	}
	for page := 0; page < StreamPageCount; page++ {
		if player != nil {
			player.saveCheckpoint(uint8(page))
		}
		s.mixStereoWords(s.leftPageBlock(uint8(page)), s.rightPageBlock(uint8(page)))
		s.readyMask |= uint8(1) << page
		s.mixedBlocks++
	}
	s.refreshWrap(0)
	s.refreshWrap(1)

	s.playPage = 0
	s.nextPage = 1
	s.writePage = 0
	s.readyMask &^= 1

	state = disableStreamInterrupts()
	s.running = true
	if !hw.StartStereoStreamBlock(s.leftPageDMA(0), s.rightPageDMA(0), StreamBlockSamples, s.clock) {
		s.running = false
		releaseStereoStream(s)
		restoreStreamInterrupts(state)
		return false
	}
	if player != nil {
		player.setDeferred(true)
	}
	enableStereoStreamIRQ()
	restoreStreamInterrupts(state)
	return true
}

func (s *StereoStream) mixStereoWords(left, right []uint32) {
	if s.sfx != nil {
		s.sfx.MixStereoWords(left, right)
		return
	}
	s.mixer.MixStereoWords(left, right)
}

func (s *StereoStream) rewindPendingSFX() bool {
	if s == nil || s.sfx == nil || !s.sfx.hasPendingCommands() {
		return false
	}
	state := disableStreamInterrupts()
	defer restoreStreamInterrupts(state)
	if !s.running || s.playPage == noStreamPage || queuedStreamBlocks(s.readyMask) != StreamPageCount-1 {
		return false
	}
	immediate := s.nextPage
	target := nextStreamPage(immediate)
	tail := nextStreamPage(target)
	ready := uint8(1)<<immediate | uint8(1)<<target | uint8(1)<<tail
	if s.readyMask&ready != ready || !s.sfx.restoreCheckpoint(target) {
		return false
	}
	s.sfx.applyPendingCommands(false)
	s.sfx.saveCheckpoint(target)
	s.mixStereoWords(s.leftPageBlock(target), s.rightPageBlock(target))
	s.refreshWrap(target)
	s.clearConsumedPage(tail)
	s.readyMask &^= uint8(1) << tail
	s.writePage = tail
	s.sfx.invalidateCheckpoint(tail)
	s.sfx.rewinds++
	return true
}

// Service generates up to maxBlocks free stereo page pairs in producer order.
// A pair is published only after both channels are complete.
func (s *StereoStream) Service(maxBlocks int) int {
	if s == nil || maxBlocks <= 0 {
		return 0
	}
	if maxBlocks > StereoStreamMaxServiceBlocks {
		maxBlocks = StereoStreamMaxServiceBlocks
	}
	if s.rewindPendingSFX() {
		return 0
	}
	if s.sfx != nil && s.sfx.hasPendingCommands() {
		state := disableStreamInterrupts()
		if !s.running {
			restoreStreamInterrupts(state)
			return 0
		}
		s.sfx.applyPendingCommands(true)
		restoreStreamInterrupts(state)
	}
	produced := 0
	for produced < maxBlocks {
		state := disableStreamInterrupts()
		if !s.running {
			restoreStreamInterrupts(state)
			break
		}
		page := s.writePage
		bit := uint8(1) << page
		if s.readyMask&bit != 0 || s.playPage == page {
			restoreStreamInterrupts(state)
			break
		}
		if s.playPage != noStreamPage && page == s.nextPage &&
			hw.StereoStreamBlockProgress(StreamBlockSamples) >= StreamImmediateRefillCutoffSamples {
			restoreStreamInterrupts(state)
			break
		}
		restoreStreamInterrupts(state)

		if s.sfx != nil {
			s.sfx.saveCheckpoint(page)
		}
		s.mixStereoWords(s.leftPageBlock(page), s.rightPageBlock(page))
		s.refreshWrap(page)

		state = disableStreamInterrupts()
		if !s.running || s.readyMask&bit != 0 || s.playPage == page {
			restoreStreamInterrupts(state)
			break
		}
		s.readyMask |= bit
		s.writePage = nextStreamPage(page)
		s.mixedBlocks++
		produced++
		restoreStreamInterrupts(state)
	}
	return produced
}

//go:noinline
//go:section .iwram
func (s *StereoStream) onBlockBoundary() {
	if s == nil || !s.running {
		return
	}
	progress := hw.StereoStreamBlockProgress(StreamBlockSamples)
	s.boundaries++
	if progress != 0 {
		s.lateBoundaries++
		if progress > s.maxLateSamples {
			s.maxLateSamples = progress
		}
	}
	progress = settleStreamRedirectProgress(progress)

	finished := s.playPage
	page := s.nextPage
	bit := uint8(1) << page
	left, right := s.silence[:], s.silence[:]
	if s.readyMask&bit != 0 {
		s.readyMask &^= bit
		s.playPage = page
		s.nextPage = nextStreamPage(page)
		left, right = s.leftPageDMA(page), s.rightPageDMA(page)
	} else {
		s.playPage = noStreamPage
		s.underruns++
	}
	if !hw.RedirectStereoStreamBlock(left, right, StreamBlockSamples, progress) {
		s.switchFailures++
		hw.StopStereoStream()
		s.running = false
		s.playPage = noStreamPage
		s.readyMask = 0
		releaseStereoStream(s)
	}
	if finished != noStreamPage {
		s.clearConsumedPage(finished)
	}
}

// Stop synchronously masks Timer 1, stops both FIFO DMA channels, and releases
// the singleton audio-stream backend.
func (s *StereoStream) Stop() {
	if s == nil {
		return
	}
	state := disableStreamInterrupts()
	if s.running {
		hw.StopStereoStream()
		s.running = false
		s.playPage = noStreamPage
		s.readyMask = 0
	}
	if s.sfx != nil {
		s.sfx.setDeferred(false)
	}
	releaseStereoStream(s)
	restoreStreamInterrupts(state)
}

// Stats returns synchronized pair-level telemetry. MixedBlocks and
// QueuedBlocks count page pairs, not individual channel pages.
func (s *StereoStream) Stats() StreamStats {
	if s == nil {
		return StreamStats{}
	}
	state := disableStreamInterrupts()
	stats := StreamStats{
		Running:        s.running,
		QueuedBlocks:   queuedStreamBlocks(s.readyMask),
		MixedBlocks:    s.mixedBlocks,
		Boundaries:     s.boundaries,
		Underruns:      s.underruns,
		LateBoundaries: s.lateBoundaries,
		MaxLateSamples: s.maxLateSamples,
		SwitchFailures: s.switchFailures,
	}
	restoreStreamInterrupts(state)
	return stats
}
