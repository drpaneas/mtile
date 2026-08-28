package audio

import (
	"github.com/drpaneas/mtile/runtime/hw"
	_ "unsafe" // required for //go:section
)

const (
	// StreamBlockSamples gives Timer 1 a 15.625 ms cadence at 16.384 kHz.
	// Four pages provide 62.5 ms of fixed-memory scheduling tolerance.
	StreamBlockSamples = 256
	StreamPageCount    = 4
	// The guard covers one complete following block plus three FIFO bursts.
	// This tolerates a delayed IRQ while RedirectFIFOAStreamBlock accounts for
	// samples already consumed and prefetched.
	StreamGuardSamples = StreamBlockSamples + 48

	StreamBlockWords   = StreamBlockSamples / 4
	StreamRingWords    = StreamPageCount * StreamBlockWords
	StreamGuardWords   = StreamGuardSamples / 4
	StreamPageWords    = StreamBlockWords + StreamGuardWords
	StreamStorageWords = StreamRingWords + StreamGuardWords

	// Starting an immediate-next refill after this point cannot guarantee that
	// the slowest measured mixer profile publishes before the boundary.
	StreamImmediateRefillCutoffSamples = 32
	// A command rewind regenerates the page after next, so it has one
	// additional page of scheduling distance. Starting in the first half of
	// the current page avoids the deterministic low-queue phase observed by
	// the long PCM/music soak while still permitting steady-state commands to
	// make progress from either two or three ready pages. Keep this separate
	// from the stricter immediate-page gate.
	StreamRewindStartCutoffSamples = StreamBlockSamples / 2

	noStreamPage uint8 = 0xFF

	rewindUnavailable uint8 = 0
	rewindRetry       uint8 = 1
	rewindComplete    uint8 = 2
)

// StreamStats is a bounded snapshot shared with runtime diagnostics. An
// underrun means the next logical page was not ready and a zero-filled page
// was selected instead. LateBoundaries records IRQs observed after Timer 1 had
// already advanced beyond the exact block boundary.
type StreamStats struct {
	Running             bool
	QueuedBlocks        uint8
	MixedBlocks         uint32
	Boundaries          uint32
	Underruns           uint32
	LateBoundaries      uint32
	MaxLateSamples      uint16
	SwitchFailures      uint32
	MaxServiceCycles    uint32
	MaxServicePattern   uint8
	LastUnderrunPattern uint8
}

// MonoStream is a fixed four-page producer/consumer queue for FIFO A. Place a
// target instance in EWRAM; its mixer should be a separate IWRAM object. Pages
// are contiguous and followed by a wrap duplicate, so DMA continues with the
// next logical samples while a Timer 1 IRQ is delayed. Consumed pages are
// cleared to make a missed refill produce silence rather than stale audio.
//
// Only the Timer 1 handler switches pages. Mixing always happens through
// Service in normal code.
type MonoStream struct {
	storage [StreamStorageWords]uint32
	silence [StreamPageWords]uint32

	mixer *Mixer
	sfx   *SFXPlayer
	clock hw.DirectSoundClock

	readyMask uint8
	playPage  uint8
	nextPage  uint8
	writePage uint8
	running   bool

	mixedBlocks         uint32
	boundaries          uint32
	underruns           uint32
	lateBoundaries      uint32
	maxLateSamples      uint16
	switchFailures      uint32
	maxServiceCycles    uint32
	maxServicePattern   uint8
	lastUnderrunPattern uint8
}

func nextStreamPage(page uint8) uint8 {
	return (page + 1) & (StreamPageCount - 1)
}

func streamRedirectProgressUnsettled(progress uint16) bool {
	remainder := progress & (16 - 1)
	return remainder == 4 || remainder == 5
}

func (s *MonoStream) pageBlock(page uint8) []uint32 {
	start := int(page) * StreamBlockWords
	return s.storage[start : start+StreamBlockWords]
}

func (s *MonoStream) pageDMA(page uint8) []uint32 {
	start := int(page) * StreamBlockWords
	return s.storage[start : start+StreamPageWords]
}

func (s *MonoStream) refreshWrap(page uint8) {
	wrap := s.storage[StreamRingWords:]
	switch page {
	case 0:
		copy(wrap[:StreamBlockWords], s.pageBlock(0))
	case 1:
		copy(wrap[StreamBlockWords:], s.pageBlock(1)[:StreamGuardWords-StreamBlockWords])
	}
}

func (s *MonoStream) clearConsumedPage(page uint8) {
	block := s.pageBlock(page)
	for i := range block {
		block[i] = 0
	}
	wrap := s.storage[StreamRingWords:]
	switch page {
	case 0:
		for i := 0; i < StreamBlockWords; i++ {
			wrap[i] = 0
		}
	case 1:
		for i := StreamBlockWords; i < StreamGuardWords; i++ {
			wrap[i] = 0
		}
	}
}

// Start pre-generates all four pages, claims the singleton FIFO A/Timer 1
// backend, and starts the provisional 16.384 kHz clock. Other candidate rates
// remain rejected until their refill cutoffs and coexistence budgets are
// measured. Target instances must live in IWRAM or EWRAM because DMA 1 cannot
// read Game Pak ROM.
func (s *MonoStream) Start(mixer *Mixer, rate hw.DirectSoundRate) bool {
	return s.start(mixer, nil, rate)
}

// StartSFX selects the sample-clocked SFX sequencer as the page producer.
func (s *MonoStream) StartSFX(player *SFXPlayer, rate hw.DirectSoundRate) bool {
	if player == nil {
		return false
	}
	return s.start(player.mixer, player, rate)
}

func (s *MonoStream) start(mixer *Mixer, player *SFXPlayer, rate hw.DirectSoundRate) bool {
	if s == nil || mixer == nil || s.running || rate != hw.DirectSoundRate16384 {
		return false
	}
	clock, ok := hw.DirectSoundClockForRate(rate)
	if !ok {
		return false
	}

	state := disableStreamInterrupts()
	if !claimMonoStream(s) {
		restoreStreamInterrupts(state)
		return false
	}
	restoreStreamInterrupts(state)

	*s = MonoStream{
		mixer:    mixer,
		sfx:      player,
		clock:    clock,
		playPage: noStreamPage,
	}
	for page := 0; page < StreamPageCount; page++ {
		if player != nil {
			player.saveCheckpoint(uint8(page))
		}
		s.mixMonoWords(s.pageBlock(uint8(page)))
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
	if !hw.StartFIFOAStreamBlock(s.pageDMA(0), StreamBlockSamples, s.clock) {
		s.running = false
		releaseMonoStream(s)
		restoreStreamInterrupts(state)
		return false
	}
	if player != nil {
		player.setDeferred(true)
	}
	enableMonoStreamIRQ()
	restoreStreamInterrupts(state)
	return true
}

func (s *MonoStream) mixMonoWords(dst []uint32) {
	if s.sfx != nil {
		s.sfx.MixMonoWords(dst)
		return
	}
	s.mixer.MixMonoWords(dst)
}

// rewindPendingSFX applies queued commands to the page after the immediate
// next page. The current page and immediate-next page stay untouched. A stale
// third ready page is discarded when present; a two-page queue needs no tail
// discard. Page roles are captured with interrupts masked, then verified again
// after the bounded checkpoint restore and one-page remix before publication.
func (s *MonoStream) rewindPendingSFX() uint8 {
	if s == nil || s.sfx == nil || !s.sfx.hasPendingCommands() {
		return rewindUnavailable
	}
	state := disableStreamInterrupts()
	queued := queuedStreamBlocks(s.readyMask)
	if !s.running || s.playPage == noStreamPage || queued < 2 {
		restoreStreamInterrupts(state)
		// Timer 1 may consume pages between Service's initial queue
		// snapshot and this second protected snapshot. That is a normal retry,
		// not evidence that the target checkpoint is damaged.
		return rewindRetry
	}
	immediate := s.nextPage
	target := nextStreamPage(immediate)
	tail := nextStreamPage(target)
	immediateAndTarget := uint8(1)<<immediate | uint8(1)<<target
	if s.readyMask&immediateAndTarget != immediateAndTarget {
		restoreStreamInterrupts(state)
		return rewindUnavailable
	}
	tailWasReady := s.readyMask&(uint8(1)<<tail) != 0
	if tailWasReady != (queued == StreamPageCount-1) {
		restoreStreamInterrupts(state)
		return rewindUnavailable
	}
	restoreStreamInterrupts(state)

	// The measured restore/apply/remix path is shorter than one 256-sample
	// page. Keep Timer 1 enabled while rebuilding the page after next: at most
	// the immediate page can become current, so target cannot be selected for
	// playback before it is complete. This avoids adding an almost-page-long
	// IRQ critical section to the entry latency already covered by the guard.
	if !s.sfx.restoreCheckpoint(target) {
		return rewindUnavailable
	}
	s.sfx.applyPendingCommands(false)
	s.sfx.saveCheckpoint(target)
	s.mixMonoWords(s.pageBlock(target))

	state = disableStreamInterrupts()
	defer restoreStreamInterrupts(state)
	if !s.running || s.playPage == target || s.readyMask&(uint8(1)<<target) == 0 ||
		tailWasReady && s.readyMask&(uint8(1)<<tail) == 0 {
		// Reaching target would violate the measured one-page producer bound.
		// Expose the impossible timing breach instead of publishing partial
		// audio or pretending the command can be retried after state mutation.
		s.switchFailures++
		return rewindComplete
	}
	s.refreshWrap(target)
	if tailWasReady {
		s.clearConsumedPage(tail)
		s.readyMask &^= uint8(1) << tail
	}
	s.writePage = tail
	s.sfx.invalidateCheckpoint(tail)
	s.sfx.rewinds++
	return rewindComplete
}

// Service generates up to maxBlocks free pages in producer order. It is safe
// for Timer 1 to interrupt the mix: a page is published only after every word
// is complete, and the handler emits silence if it reaches an unpublished
// page. A successful SFX rewind returns without filling its discarded tail;
// the next service point performs that bounded second piece of work.
func (s *MonoStream) Service(maxBlocks int) int {
	if s == nil || maxBlocks <= 0 {
		return 0
	}
	pending := s.sfx != nil && s.sfx.hasPendingCommands()
	queued := uint8(0)
	if pending {
		state := disableStreamInterrupts()
		if !s.running {
			restoreStreamInterrupts(state)
			return 0
		}
		queued = queuedStreamBlocks(s.readyMask)
		progress := hw.FIFOAStreamBlockProgress(StreamBlockSamples)
		restoreStreamInterrupts(state)

		if queued >= 2 {
			// A page regeneration can consume almost one complete sample page.
			// Starting after the normal immediate-refill cutoff creates a
			// deterministic low-queue phase two ticks later. Keep the command
			// pending until a handoff/refill cycle presents a safe phase; do not
			// misreport this intentional defer as a fallback.
			if progress >= StreamRewindStartCutoffSamples {
				if queued == StreamPageCount-1 {
					return 0
				}
			} else {
				switch s.rewindPendingSFX() {
				case rewindComplete:
					return 0
				case rewindRetry:
					// A handoff raced the initial queue snapshot. Leave the
					// command pending, refill below, and retry from the next
					// safe two-page state.
					break
				default:
					// A contiguous safe queue can otherwise fail rewind only
					// when its target checkpoint is unavailable. Preserve the
					// explicit producer-tail fallback for damaged state.
					s.sfx.applyPendingCommands(true)
				}
			}
		}
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
			hw.FIFOAStreamBlockProgress(StreamBlockSamples) >= StreamImmediateRefillCutoffSamples {
			restoreStreamInterrupts(state)
			break
		}
		restoreStreamInterrupts(state)

		serviceStart := hw.ReadFrameCycleCounter()
		if s.sfx != nil {
			s.sfx.saveCheckpoint(page)
		}
		s.mixMonoWords(s.pageBlock(page))
		s.refreshWrap(page)
		serviceCycles := hw.ReadFrameCycleCounter() - serviceStart
		if serviceCycles > s.maxServiceCycles {
			s.maxServiceCycles = serviceCycles
			if s.sfx != nil {
				s.maxServicePattern = s.sfx.MusicState().PatternID
			}
		}

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
func (s *MonoStream) onBlockBoundary() {
	if s == nil || !s.running {
		return
	}
	progress := hw.FIFOAStreamBlockProgress(StreamBlockSamples)
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
	block := s.silence[:]
	if s.readyMask&bit != 0 {
		s.readyMask &^= bit
		s.playPage = page
		s.nextPage = nextStreamPage(page)
		block = s.pageDMA(page)
	} else {
		s.playPage = noStreamPage
		s.underruns++
		if s.sfx != nil {
			s.lastUnderrunPattern = s.sfx.MusicState().PatternID
		}
	}
	if !hw.RedirectFIFOAStreamBlock(block, StreamBlockSamples, progress) {
		s.switchFailures++
		hw.StopFIFOAStream()
		s.running = false
		s.playPage = noStreamPage
		s.readyMask = 0
		releaseMonoStream(s)
	}
	if finished != noStreamPage {
		s.clearConsumedPage(finished)
	}
}

// Stop synchronously masks Timer 1, stops FIFO DMA, and releases ownership.
func (s *MonoStream) Stop() {
	if s == nil {
		return
	}
	state := disableStreamInterrupts()
	if s.running {
		hw.StopFIFOAStream()
		s.running = false
		s.playPage = noStreamPage
		s.readyMask = 0
	}
	if s.sfx != nil {
		s.sfx.setDeferred(false)
	}
	releaseMonoStream(s)
	restoreStreamInterrupts(state)
}

func queuedStreamBlocks(mask uint8) uint8 {
	var count uint8
	for mask != 0 {
		count += mask & 1
		mask >>= 1
	}
	return count
}

func (s *MonoStream) Stats() StreamStats {
	if s == nil {
		return StreamStats{}
	}
	state := disableStreamInterrupts()
	stats := StreamStats{
		Running:             s.running,
		QueuedBlocks:        queuedStreamBlocks(s.readyMask),
		MixedBlocks:         s.mixedBlocks,
		Boundaries:          s.boundaries,
		Underruns:           s.underruns,
		LateBoundaries:      s.lateBoundaries,
		MaxLateSamples:      s.maxLateSamples,
		SwitchFailures:      s.switchFailures,
		MaxServiceCycles:    s.maxServiceCycles,
		MaxServicePattern:   s.maxServicePattern,
		LastUnderrunPattern: s.lastUnderrunPattern,
	}
	restoreStreamInterrupts(state)
	return stats
}
