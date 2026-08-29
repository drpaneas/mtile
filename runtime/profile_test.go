package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
)

func TestPerformanceProfilingRecordsFramePhasesAndGC(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.SetProfiling(true)
	if !c.ProfilingEnabled() {
		t.Fatal("profiling did not enable")
	}

	c.BeginProfilePhase(ProfileUpdate)
	hw.SetCycleCounterForTest(101)
	c.EndProfilePhase(ProfileUpdate)
	c.BeginProfilePhase(ProfileDraw)
	hw.SetCycleCounterForTest(202)
	c.EndProfilePhase(ProfileDraw)
	hw.SetFrameCycleCounterForTest(300000)
	c.BeginFrameProfile()
	c.EndFrameProfile()

	hw.SetCycleCounterForTest(303)
	if err := c.Collect(); err != HeapOK {
		t.Fatalf("Collect() = %v, want HeapOK", err)
	}

	stats := c.PerformanceStats()
	if stats.Frames != 1 || stats.MissedFrames != 1 {
		t.Fatalf("frame stats = %+v, want one missed 60 Hz frame", stats)
	}
	if stats.LastUpdate != 101 || stats.MaxUpdate != 101 || stats.LastDraw != 202 || stats.MaxDraw != 202 {
		t.Fatalf("phase stats = %+v", stats)
	}
	if stats.GCCount != 1 || stats.LastGC != 303 || stats.MaxGC != 303 {
		t.Fatalf("GC stats = %+v", stats)
	}

	c.SetProfiling(false)
	hw.SetFrameCycleCounterForTest(999999)
	c.BeginFrameProfile()
	c.EndFrameProfile()
	if stats := c.PerformanceStats(); stats.Frames != 1 {
		t.Fatalf("disabled profiler changed frame count: %+v", stats)
	}
}

func TestPaceFrame30RecordsWorkCostBeforePaceWait(t *testing.T) {
	hw.ResetForTest()
	var c Context
	const workCycles = 349_327

	hw.SetFrameCycleCounterForTest(workCycles)
	c.BeginFramePace()
	c.PaceFrame30()

	if c.lastFrameCycles != workCycles {
		t.Fatalf("lastFrameCycles = %d, want %d", c.lastFrameCycles, workCycles)
	}
	if c.maxFrameCycles != workCycles {
		t.Fatalf("maxFrameCycles = %d, want %d", c.maxFrameCycles, workCycles)
	}
	if c.profileFrames != 1 {
		t.Fatalf("profileFrames = %d, want 1", c.profileFrames)
	}
	if got := c.Stat(1); got != cyclesToTickFraction(workCycles) {
		t.Fatalf("Stat(1) = %v, want %v", got, cyclesToTickFraction(workCycles))
	}
}

func TestPostPaceRendererWorkUpdatesCPUAndDeadlineOnce(t *testing.T) {
	hw.ResetForTest()
	var c Context
	const prePresent = 530_000
	hw.SetFrameCycleCounterForTest(prePresent)
	c.BeginFramePace()
	c.PaceFrame30()

	// The renderer used 20k active cycles after pacing and completed after the
	// two-frame wall deadline. Work excludes the deliberate wait, while the
	// missed-frame counter uses elapsed wall time.
	c.accountPostPaceWork(20_000, 570_000)
	if c.lastFrameCycles != 550_000 || c.maxFrameCycles != 550_000 {
		t.Fatalf("post-present cycles = %d/%d", c.lastFrameCycles, c.maxFrameCycles)
	}
	if c.profileMisses != 1 {
		t.Fatalf("misses = %d, want 1", c.profileMisses)
	}

	// A tick already classified late before presentation must not count twice.
	hw.SetFrameCycleCounterForTest(570_000)
	c.BeginFramePace()
	c.PaceFrame30()
	c.accountPostPaceWork(10_000, 590_000)
	if c.profileMisses != 2 {
		t.Fatalf("misses after pre-present miss = %d, want 2", c.profileMisses)
	}
}

func TestStatReportsMaximumTickCost(t *testing.T) {
	hw.ResetForTest()
	var c Context

	hw.SetFrameCycleCounterForTest(100_000)
	c.BeginFramePace()
	c.PaceFrame30()

	hw.SetFrameCycleCounterForTest(352_077)
	c.BeginFramePace()
	c.PaceFrame30()

	if got := c.Stat(1); got != cyclesToTickFraction(352_077) {
		t.Fatalf("Stat(1) = %v, want last tick %v", got, cyclesToTickFraction(352_077))
	}
	if got := c.Stat(2); got != cyclesToTickFraction(352_077) {
		t.Fatalf("Stat(2) = %v, want max %v", got, cyclesToTickFraction(352_077))
	}
	if got := c.Stat(0); got != 0 {
		t.Fatalf("Stat(0) = %v, want 0", got)
	}
}

func TestAudioStatIDsMapAndSaturateCounters(t *testing.T) {
	stats := AudioStats{
		Stream: audio.StreamStats{
			Underruns: 11, QueuedBlocks: 2, MaxLateSamples: 19,
			MaxServiceCycles: 256 * 123, MaxServicePattern: 12, LastUnderrunPattern: 9,
		},
		Mixer: audio.Stats{ActiveVoices: 3, PeakActiveVoices: 4, VoiceSteals: 5, FailedStarts: 6},
		SFX:   audio.SFXStats{Rewinds: 7, FallbackCommands: 8, DroppedCommands: 9},
	}
	want := map[int]int{
		StatAudioUnderruns: 11, StatAudioQueuedBlocks: 2,
		StatAudioActiveVoices: 3, StatAudioPeakVoices: 4,
		StatAudioVoiceSteals: 5, StatAudioFailedStarts: 6,
		StatAudioRewinds: 7, StatAudioFallbacks: 8,
		StatAudioDroppedCommands: 9, StatAudioMaxLateSamples: 19,
		StatAudioMaxServiceCyclesPerSample: 123,
		StatAudioMaxServicePattern:         12, StatAudioLastUnderrunPattern: 9,
	}
	for id, integer := range want {
		if got := audioStatValue(id, stats); got != FromInt(integer) {
			t.Errorf("audioStatValue(%d) = %v, want %v", id, got, FromInt(integer))
		}
	}
	stats.Stream.Underruns = 100_000
	if got := audioStatValue(StatAudioUnderruns, stats); got != FromInt(0x7fff) {
		t.Fatalf("saturated underruns = %v, want %v", got, FromInt(0x7fff))
	}
	if got := audioStatValue(79, stats); got != 0 {
		t.Fatalf("unknown audio stat = %v, want 0", got)
	}
}

func TestNativeAudioStatIDsMapPSGScheduler(t *testing.T) {
	stats := audio.NativeStats{
		ServiceCalls: 11, EventsApplied: 12, ChannelReplaces: 13,
		PriorityRejects: 14, CommandsRejected: 15, QueueOverflows: 16,
		MaxServiceCycles: 17, AbsoluteTick: 18, CompletedSongs: 19,
		MinServiceCycles: 20, MeanServiceCycles: 21, MeasuredServices: 22,
	}
	stats.Channels[0].Active = true
	stats.Channels[2].Active = true
	want := map[int]int{
		80: 11, 81: 12, 82: 2, 83: 13, 84: 14, 85: 15, 86: 16,
		87: 17, 88: 18, 89: 19, 90: 20, 91: 21, 92: 22,
	}
	for id, integer := range want {
		if got := nativeAudioStatValue(id, stats); got != FromInt(integer) {
			t.Errorf("nativeAudioStatValue(%d) = %v, want %v", id, got, FromInt(integer))
		}
	}
	stats.MaxServiceCycles = 100_000
	if got := nativeAudioStatValue(87, stats); got != FromInt(0x7fff) {
		t.Fatalf("saturated max service cycles = %v, want %v", got, FromInt(0x7fff))
	}
}
