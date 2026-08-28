package runtime

import (
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

// ProfilePhase identifies one generated-game phase in the optional production
// frame profiler. Profiling is off by default, so normal carts pay only the
// branch cost of the instrumentation calls.
type ProfilePhase uint8

const (
	ProfileUpdate ProfilePhase = iota
	ProfileDraw
)

const (
	StatAudioUnderruns = 80 + iota
	StatAudioQueuedBlocks
	StatAudioActiveVoices
	StatAudioPeakVoices
	StatAudioVoiceSteals
	StatAudioFailedStarts
	StatAudioRewinds
	StatAudioFallbacks
	StatAudioDroppedCommands
	StatAudioMaxLateSamples
	StatAudioMaxServiceCyclesPerSample
	StatAudioMaxServicePattern
	StatAudioLastUnderrunPattern
)

// PerformanceStats is a snapshot of the most recent measured frame and its
// update/draw/GC components. Cycle counts are ARM7TDMI cycles on hardware;
// host tests may inject deterministic values through runtime/hw.
type PerformanceStats struct {
	Frames          uint32
	MissedFrames    uint32
	LastFrame       uint32
	MaxFrame        uint32
	LastUpdate      uint32
	MaxUpdate       uint32
	LastDraw        uint32
	MaxDraw         uint32
	GCCount         uint32
	LastGC          uint32
	MaxGC           uint32
	LastBlit        uint32
	PresentStart    uint16
	PresentEnd      uint16
	DiagnosticSkips uint32
}

func (c *Context) SetProfiling(enabled bool) {
	if c == nil {
		return
	}
	if c.profileEnabled == enabled {
		return
	}
	c.profileEnabled = enabled
	if !enabled {
		if c.heapReady {
			c.heap.measureCollections = false
		}
		return
	}
	c.profileFrames = 0
	c.profileMisses = 0
	c.lastFrameCycles = 0
	c.maxFrameCycles = 0
	c.lastUpdateCycles = 0
	c.maxUpdateCycles = 0
	c.lastDrawCycles = 0
	c.maxDrawCycles = 0
	c.lastPresentStart = 0
	c.lastPresentEnd = 0
	if c.heapReady {
		c.heap.measureCollections = true
		c.heap.collectCount = 0
		c.heap.lastCollectCycles = 0
		c.heap.maxCollectCycles = 0
	}
}

func (c *Context) ProfilingEnabled() bool {
	return c != nil && c.profileEnabled
}

// BeginFrameProfile and EndFrameProfile bracket one generated 60 Hz loop
// iteration. The frame deadline is one complete 228-line GBA frame.
func (c *Context) BeginFrameProfile() {
	if c != nil && c.profileEnabled {
		hw.StartFrameCycleCounter()
	}
}

func (c *Context) EndFrameProfile() {
	if c == nil || !c.profileEnabled {
		return
	}
	cycles := hw.StopFrameCycleCounter()
	c.profileFrames++
	c.lastFrameCycles = cycles
	if cycles > c.maxFrameCycles {
		c.maxFrameCycles = cycles
	}
	if cycles >= cyclesPerFrame {
		c.profileMisses++
	}
}

// 30 Hz pacing. The GBA LCD always scans at 59.73 Hz; a stable 30 Hz game
// flips its page on every second VBlank so each image is shown for exactly
// two scans. The only way 30 Hz can look bad is cadence jitter: an
// iteration that finishes fast and flips one VBlank early alternates
// 1-frame/2-frame presentation. BeginFramePace anchors the free-running
// TM2/TM3 cycle counter at VDraw start; PaceFrame30 burns until one whole
// display frame has elapsed since that anchor, so the following
// WaitForPresentWindow always lands in the second VBlank - never the
// first - regardless of how fast update/draw ran.
func (c *Context) BeginFramePace() {
	hw.StartFrameCycleCounter()
}

// PaceFrame30 samples the true work cost (everything since BeginFramePace:
// input, update, draw, blit, GC) *before* burning the leftover, so stat()
// reports real load rather than the always-~85% post-pace reading. It
// records unconditionally - one timer read and three stores per tick -
// so stat(1) works without arming the profiler.
func (c *Context) PaceFrame30() {
	if c != nil {
		cycles := hw.ReadFrameCycleCounter()
		c.profileFrames++
		c.lastFrameCycles = cycles
		if cycles > c.maxFrameCycles {
			c.maxFrameCycles = cycles
		}
		if cycles >= 2*cyclesPerFrame {
			c.profileMisses++
		}
	}
	hw.PaceWaitUntil(cyclesPerFrame)
}

// EndFramePace30 is kept as the loop's post-flip hook. Work-cost sampling
// happens in PaceFrame30 (before the pace burn); nothing is left to record
// here, but the generated loop calls it so future flip-timing diagnostics
// have a stable place to live.
func (c *Context) EndFramePace30() {
}

// Stat is the PICO-8-style stat() builtin. stat(1) is CPU load for the
// last tick, where 1.0 means the whole 30 Hz tick (two display frames)
// was consumed by update+draw+blit; stat(2) is the maximum since boot.
// stat(80)..stat(92) expose bounded audio counters; unknown ids return 0.
func (c *Context) Stat(n int) Number {
	if c == nil {
		return 0
	}
	// Audio v2 owns only the four Game Boy PSG channels. Keep the bounded
	// stat(80)..stat(92) surface, but report its hardware scheduler instead of
	// the archived v1 Direct Sound stream whenever a native controller exists.
	if c.nativeAudio != nil && n >= StatAudioUnderruns && n <= StatAudioLastUnderrunPattern {
		return nativeAudioStatValue(n, c.nativeAudio.Stats())
	}
	switch n {
	case 1:
		return cyclesToTickFraction(c.lastFrameCycles)
	case 2:
		return cyclesToTickFraction(c.maxFrameCycles)
	case StatAudioUnderruns, StatAudioQueuedBlocks, StatAudioActiveVoices,
		StatAudioPeakVoices, StatAudioVoiceSteals, StatAudioFailedStarts,
		StatAudioRewinds, StatAudioFallbacks, StatAudioDroppedCommands,
		StatAudioMaxLateSamples, StatAudioMaxServiceCyclesPerSample,
		StatAudioMaxServicePattern, StatAudioLastUnderrunPattern:
		return legacyAudioStatValue(n, c)
	}
	return 0
}

func nativeAudioStatValue(n int, stats audio.NativeStats) Number {
	var value uint32
	switch n {
	case 80:
		value = stats.ServiceCalls
	case 81:
		value = stats.EventsApplied
	case 82:
		for _, channel := range stats.Channels {
			if channel.Active {
				value++
			}
		}
	case 83:
		value = stats.ChannelReplaces
	case 84:
		value = stats.PriorityRejects
	case 85:
		value = stats.CommandsRejected
	case 86:
		value = stats.QueueOverflows
	case 87:
		value = stats.MaxServiceCycles
	case 88:
		value = stats.AbsoluteTick
	case 89:
		value = stats.CompletedSongs
	case 90:
		value = stats.MinServiceCycles
	case 91:
		value = stats.MeanServiceCycles
	case 92:
		value = stats.MeasuredServices
	default:
		return 0
	}
	return boundedStatCounter(value)
}

// legacyAudioStatValue keeps archived v1 diagnostics out of the PSG-only
// generated path. In particular it avoids constructing the old aggregate
// AudioStats value from stat(), which is large enough to escape under
// TinyGo -gc=none even when no Direct Sound engine is attached.
func legacyAudioStatValue(n int, c *Context) Number {
	var value uint32
	switch n {
	case StatAudioUnderruns:
		if c.audioStream != nil {
			value = c.audioStream.Stats().Underruns
		}
	case StatAudioQueuedBlocks:
		if c.audioStream != nil {
			value = uint32(c.audioStream.Stats().QueuedBlocks)
		}
	case StatAudioActiveVoices:
		if c.sfxPlayer != nil {
			value = uint32(c.sfxPlayer.MixerStats().ActiveVoices)
		}
	case StatAudioPeakVoices:
		if c.sfxPlayer != nil {
			value = uint32(c.sfxPlayer.MixerStats().PeakActiveVoices)
		}
	case StatAudioVoiceSteals:
		if c.sfxPlayer != nil {
			value = c.sfxPlayer.MixerStats().VoiceSteals
		}
	case StatAudioFailedStarts:
		if c.sfxPlayer != nil {
			value = c.sfxPlayer.MixerStats().FailedStarts
		}
	case StatAudioRewinds:
		if c.sfxPlayer != nil {
			value = c.sfxPlayer.Stats().Rewinds
		}
	case StatAudioFallbacks:
		if c.sfxPlayer != nil {
			value = c.sfxPlayer.Stats().FallbackCommands
		}
	case StatAudioDroppedCommands:
		if c.sfxPlayer != nil {
			value = c.sfxPlayer.Stats().DroppedCommands
		}
	case StatAudioMaxLateSamples:
		if c.audioStream != nil {
			value = uint32(c.audioStream.Stats().MaxLateSamples)
		}
	case StatAudioMaxServiceCyclesPerSample:
		if c.audioStream != nil {
			value = c.audioStream.Stats().MaxServiceCycles / 256
		}
	case StatAudioMaxServicePattern:
		if c.audioStream != nil {
			value = uint32(c.audioStream.Stats().MaxServicePattern)
		}
	case StatAudioLastUnderrunPattern:
		if c.audioStream != nil {
			value = uint32(c.audioStream.Stats().LastUnderrunPattern)
		}
	default:
		return 0
	}
	return boundedStatCounter(value)
}

func boundedStatCounter(value uint32) Number {
	const maxStatInteger = uint32(0x7fff)
	if value > maxStatInteger {
		value = maxStatInteger
	}
	return FromInt(int(value))
}

func audioStatValue(n int, stats AudioStats) Number {
	var value uint32
	switch n {
	case StatAudioUnderruns:
		value = stats.Stream.Underruns
	case StatAudioQueuedBlocks:
		value = uint32(stats.Stream.QueuedBlocks)
	case StatAudioActiveVoices:
		value = uint32(stats.Mixer.ActiveVoices)
	case StatAudioPeakVoices:
		value = uint32(stats.Mixer.PeakActiveVoices)
	case StatAudioVoiceSteals:
		value = stats.Mixer.VoiceSteals
	case StatAudioFailedStarts:
		value = stats.Mixer.FailedStarts
	case StatAudioRewinds:
		value = stats.SFX.Rewinds
	case StatAudioFallbacks:
		value = stats.SFX.FallbackCommands
	case StatAudioDroppedCommands:
		value = stats.SFX.DroppedCommands
	case StatAudioMaxLateSamples:
		value = uint32(stats.Stream.MaxLateSamples)
	case StatAudioMaxServiceCyclesPerSample:
		value = stats.Stream.MaxServiceCycles / 256
	case StatAudioMaxServicePattern:
		value = uint32(stats.Stream.MaxServicePattern)
	case StatAudioLastUnderrunPattern:
		value = uint32(stats.Stream.LastUnderrunPattern)
	default:
		return 0
	}
	return boundedStatCounter(value)
}

// cyclesToTickFraction converts cycles to a 16:16 fraction of the 561,792
// cycle tick. FromInt(561792) would overflow 16:16, so divide in int64.
func cyclesToTickFraction(cycles uint32) Number {
	return Number(int64(cycles) << 16 / (2 * cyclesPerFrame))
}

func (c *Context) BeginProfilePhase(_ ProfilePhase) {
	if c != nil && c.profileEnabled {
		c.profilePhaseTick = startShortCycleMeasurement()
	}
}

func (c *Context) EndProfilePhase(phase ProfilePhase) {
	if c == nil || !c.profileEnabled {
		return
	}
	cycles := stopShortCycleMeasurement(c.profilePhaseTick)
	switch phase {
	case ProfileUpdate:
		c.lastUpdateCycles = cycles
		if cycles > c.maxUpdateCycles {
			c.maxUpdateCycles = cycles
		}
	case ProfileDraw:
		c.lastDrawCycles = cycles
		if cycles > c.maxDrawCycles {
			c.maxDrawCycles = cycles
		}
	}
}

func (c *Context) PerformanceStats() PerformanceStats {
	if c == nil {
		return PerformanceStats{}
	}
	stats := PerformanceStats{
		Frames:          c.profileFrames,
		MissedFrames:    c.profileMisses,
		LastFrame:       c.lastFrameCycles,
		MaxFrame:        c.maxFrameCycles,
		LastUpdate:      c.lastUpdateCycles,
		MaxUpdate:       c.maxUpdateCycles,
		LastDraw:        c.lastDrawCycles,
		MaxDraw:         c.maxDrawCycles,
		LastBlit:        c.lastBlitCycles,
		PresentStart:    c.lastPresentStart,
		PresentEnd:      c.lastPresentEnd,
		DiagnosticSkips: c.frameSkips,
	}
	if c.heapReady {
		stats.GCCount = c.heap.collectCount
		stats.LastGC = c.heap.lastCollectCycles
		stats.MaxGC = c.heap.maxCollectCycles
	}
	return stats
}
