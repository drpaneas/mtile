//go:build tinygo

// Command audiocoexist runs the audio/graphics coexistence matrix with a
// compile-time-selected production profile. The default uses four
// music-owned synthesized voices, the pcm profile adds one looping native
// sample to a six-voice mixer, and the experimental stereo profile uses two
// hard-panned music voices. One long music fade stays active across the
// complete matrix after a checkpointed setup command. Timer 0 and Timer 1
// remain owned by Direct Sound; Timer 2 and Timer 3 measure the complete
// normal-context work of each 30 Hz tick.
package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/volatile"
	_ "unsafe" // required for //go:section
)

const (
	sheetW  = 128
	sheetH  = 128
	spriteN = (sheetW / 8) * (sheetH / 8)
	mapW    = 64
	mapH    = 96

	sparseBaseY = 64

	caseCount       = 8
	normalCaseCount = 7
	caseTicks       = 16
	recoveryTicks   = 32
	coexistFadeMS   = 30000

	// Six audio blocks are 96 ms at 16.384 kHz. With service deliberately
	// withheld, this exceeds the current page plus all three queued pages.
	starvationCycles = 6 * 262144

	statusMeasuring = 0xA0C00001
	statusDone      = 0xA0C00002
	statusFailed    = 0xDEADA0C0
)

func makeSheetROM() (sheet [sheetW * sheetH]byte) {
	for y := 0; y < sheetH; y++ {
		for x := 0; x < sheetW; x++ {
			i := y*sheetW + x
			if y < 8 || (i/4)%2 == 0 {
				sheet[i] = byte(1 + i%15)
			}
		}
	}
	return sheet
}

//go:section .rodata
var sheetROM = makeSheetROM()

//go:section .rodata
var mapROM = [mapW * mapH]uint16{}

//go:section .rodata
var flagROM = [spriteN]byte{}

//go:section .ewram
var sheetRAM [sheetW * sheetH]byte

//go:section .ewram
var mapRAM [mapW * mapH]uint16

//go:section .ewram
var flagRAM [spriteN]byte

//go:section .ewram
var cartData rt.AssetMemory

//go:section .ewram
var ctx rt.Context

//go:section .iwram
var mixer audio.Mixer

//go:section .ewram
var sfxPlayer audio.SFXPlayer

var musicDefinitions = [...]audio.SFXDefinition{{
	ID: 0, Speed: 8, StepCount: 1, LoopStart: 0, LoopEnd: 1,
}}

var musicSteps = [...]audio.SFXStep{{
	PhaseStep: 1 << 24, Volume: 28, Kind: audio.SourceTriangle,
}}

var musicPatterns = [...]audio.MusicPattern{{
	ID: 0, Channels: coexistMusicChannels, Next: 0, DurationSamples: 4096,
}}

// coexistStatus is measuring, done, or failed. coexistCount reports the
// number of populated entries in each case array below.
//
//go:section .ewram
var coexistStatus volatile.Register32

//go:section .ewram
var coexistCount volatile.Register32

// coexistMaxCycles includes audio Service, the named graphics workload, and
// the Mode 4 DMA blit. It excludes frame pacing and the VBlank page flip.
//
//go:section .ewram
var coexistMaxCycles [caseCount]volatile.Register32

// coexistMaxServiceCycles isolates the slowest single Service call observed in
// each case. coexistMaxServiceBlocks records how many pages that call produced.

//go:section .ewram
var coexistMaxServiceCycles [caseCount]volatile.Register32

//go:section .ewram
var coexistMaxServiceBlocks [caseCount]volatile.Register32

//go:section .ewram
var coexistBoundaries [caseCount]volatile.Register32

//go:section .ewram
var coexistUnderruns [caseCount]volatile.Register32

//go:section .ewram
var coexistLateBoundaries [caseCount]volatile.Register32

// coexistMaxLateSamples is the cumulative high-water mark after each case.
//
//go:section .ewram
var coexistMaxLateSamples [caseCount]volatile.Register32

// The music telemetry proves that the setup rewind remained the only rewind,
// and that its long fade stayed active through every graphics case. Rewinds
// and fallbacks are cumulative high-water marks.

//go:section .ewram
var coexistFadeRemaining [caseCount]volatile.Register32

//go:section .ewram
var coexistRewinds [caseCount]volatile.Register32

//go:section .ewram
var coexistFallbacks [caseCount]volatile.Register32

// Native-sample telemetry is zero for the synth-only profiles. The PCM
// profile requires exactly one looping voice and one successful start through
// every matrix case.

//go:section .ewram
var coexistSampleVoices [caseCount]volatile.Register32

//go:section .ewram
var coexistSampleStarts [caseCount]volatile.Register32

//go:section .ewram
var coexistFailedSampleStarts [caseCount]volatile.Register32

//go:section .ewram
var coexistInvalidSampleCalls [caseCount]volatile.Register32

// The recovery gate starts only after the forced-starvation case has
// repopulated all three queued pages.
//
//go:section .ewram
var coexistRecoveryQueued volatile.Register32

//go:section .ewram
var coexistRecoveryBoundaries volatile.Register32

//go:section .ewram
var coexistRecoveryUnderruns volatile.Register32

//go:section .ewram
var coexistFinalUnderruns volatile.Register32

func authorAssets() {
	// Dense rows 0-31 use the half-transparent sheet. Rows 48-63 are a
	// half-empty PICO-class room. The sparse 30x20 mapdemo-class room has a
	// disjoint region starting at sparseBaseY so both retain their intended
	// density throughout the matrix.
	for y := 0; y < mapH; y++ {
		for x := 0; x < mapW; x++ {
			switch {
			case y >= 48:
				if (x+y)&1 == 0 {
					ctx.Mset(x, y, 1+(y*mapW+x)%(spriteN-1))
				}
			default:
				ctx.Mset(x, y, 1+(y*mapW+x)%(spriteN-1))
			}
		}
	}
	authorSparseGBAScreen()
}

func authorSparseGBAScreen() {
	put := func(x, y, tile int) {
		if x >= 0 && x < 30 && y >= 0 && y < 20 {
			ctx.Mset(x, sparseBaseY+y, tile)
		}
	}
	for y := 0; y < 20; y++ {
		for x := 0; x < 30; x++ {
			put(x, y, 0)
		}
	}
	for x := 0; x < 30; x++ {
		put(x, 18, 1)
		put(x, 19, 2)
	}
	for x := 22; x < 30; x++ {
		put(x, 17, 1)
		if x > 25 {
			put(x, 16, 1)
		}
	}
	for x := 4; x < 9; x++ {
		put(x, 13, 3)
	}
	for x := 12; x < 16; x++ {
		put(x, 10, 3)
	}
	for x := 19; x < 23; x++ {
		put(x, 7, 3)
	}
	for y := 14; y < 18; y++ {
		put(2, y, 4)
	}
	for x := 9; x < 12; x++ {
		put(x, 15, 1)
	}
	for _, p := range [][2]int{{5, 12}, {7, 12}, {13, 9}, {14, 9}, {20, 6}, {3, 17}, {10, 17}, {15, 17}, {24, 16}, {27, 15}} {
		put(p[0], p[1], 16)
	}
	for _, p := range [][2]int{{6, 12}, {21, 6}, {12, 17}, {25, 16}} {
		put(p[0], p[1], 17)
	}
}

func initAssets() bool {
	image := rt.AssetImage{
		SheetW: sheetW, SheetH: sheetH,
		SpriteCols: sheetW / 8, SpriteRows: sheetH / 8, SpriteN: spriteN,
		MapW: mapW, MapH: mapH,
		Sheet: sheetROM[:], Map: mapROM[:], Flags: flagROM[:],
	}
	cartData.SheetW, cartData.SheetH = sheetW, sheetH
	cartData.SpriteCols, cartData.SpriteRows, cartData.SpriteN = sheetW/8, sheetH/8, spriteN
	cartData.MapW, cartData.MapH = mapW, mapH
	cartData.SheetReadOnly = false
	cartData.Sheet, cartData.Map, cartData.Flags = sheetRAM[:], mapRAM[:], flagRAM[:]
	return ctx.Init(&image, &cartData)
}

func startAudio() bool {
	if !mixer.Init(coexistMixerVoices) ||
		!sfxPlayer.Init(&mixer, musicDefinitions[:], musicSteps[:]) ||
		!sfxPlayer.InitMusic(musicPatterns[:]) || !configureCoexistChannels() ||
		!sfxPlayer.Music(0, coexistFadeMS) || !configureCoexistProfile() {
		return false
	}
	return stream.StartSFX(&sfxPlayer, hw.DirectSoundRate16384)
}

func drawCase(index int) {
	switch index {
	case 0:
		// Empty update/draw. The production DMA blit still runs.
	case 1:
		ctx.Cls(1)
		ctx.Print("CPU  AUDIO  MAP", 4, 4, 7)
	case 2:
		for s := 0; s < 100; s++ {
			ctx.Spr(1+s%32, (s*16)%232, (s*8)%152, 1, 1, false, false)
		}
	case 3:
		ctx.Map(0, sparseBaseY, 0, 0, 30, 20, 0)
	case 4:
		ctx.Map(0, 0, 0, 0, 16, 16, 0)
	case 5:
		ctx.Cls(1)
		ctx.Map(0, 48, 0, 0, 16, 16, 0)
		for s := 0; s < 30; s++ {
			ctx.Spr(1+s%32, (s*16)%232, (s*8)%152, 1, 1, s&3 == 0, false)
		}
		for p := 0; p < 24; p++ {
			x := (p * 9) % 230
			y := (p * 13) % 150
			ctx.Rectfill(x, y, x+1, y+1, 7)
		}
		ctx.Print("3200 M", 4, 4, 7)
	case 6:
		ctx.Cls(1)
		ctx.Map(0, sparseBaseY, 0, 0, 30, 20, 0)
		ctx.Spr(1, 60, 136, 1, 1, false, false)
		ctx.Print("cpu 62%", 4, 4, 7)
		ctx.Print("arrows move", 4, 150, 7)
	}
}

func finishFrame() {
	ctx.PaceFrame30()
	ctx.WaitForPresentWindow()
	ctx.ApplyDisplayPalette()
	ctx.FlipPage()
	ctx.EndFramePace30()
	ctx.WaitVDraw()
}

type tickTiming struct {
	workCycles       uint32
	maxServiceCycles uint32
	maxServiceBlocks uint32
}

func timedService() (uint32, uint32) {
	start := hw.ReadFrameCycleCounter()
	blocks := stream.Service(audio.StreamPageCount)
	return hw.ReadFrameCycleCounter() - start, uint32(blocks)
}

func runTick(index int) tickTiming {
	ctx.BeginFramePace()
	maxServiceCycles, maxServiceBlocks := timedService()
	drawCase(index)
	serviceCycles, serviceBlocks := serviceAfterDraw()
	if serviceCycles > maxServiceCycles {
		maxServiceCycles, maxServiceBlocks = serviceCycles, serviceBlocks
	}
	ctx.PrepareHiddenPage()
	serviceCycles, serviceBlocks = timedService()
	if serviceCycles > maxServiceCycles {
		maxServiceCycles, maxServiceBlocks = serviceCycles, serviceBlocks
	}
	cycles := hw.ReadFrameCycleCounter()
	finishFrame()
	return tickTiming{
		workCycles: cycles, maxServiceCycles: maxServiceCycles, maxServiceBlocks: maxServiceBlocks,
	}
}

func recordCase(index int, before, after audio.StreamStats, afterSFX audio.SFXStats, timing tickTiming) {
	coexistMaxCycles[index].Set(timing.workCycles)
	coexistMaxServiceCycles[index].Set(timing.maxServiceCycles)
	coexistMaxServiceBlocks[index].Set(timing.maxServiceBlocks)
	coexistBoundaries[index].Set(after.Boundaries - before.Boundaries)
	coexistUnderruns[index].Set(after.Underruns - before.Underruns)
	coexistLateBoundaries[index].Set(after.LateBoundaries - before.LateBoundaries)
	coexistMaxLateSamples[index].Set(uint32(after.MaxLateSamples))
	coexistFadeRemaining[index].Set(afterSFX.MusicFadeSamplesRemaining)
	coexistRewinds[index].Set(afterSFX.Rewinds)
	coexistFallbacks[index].Set(afterSFX.FallbackCommands)
	coexistSampleVoices[index].Set(uint32(afterSFX.ActiveSampleVoices))
	coexistSampleStarts[index].Set(afterSFX.SampleStarts)
	coexistFailedSampleStarts[index].Set(afterSFX.FailedSampleStarts)
	coexistInvalidSampleCalls[index].Set(afterSFX.InvalidSampleCalls)
	coexistCount.Set(uint32(index + 1))
}

func fillQueue() bool {
	for attempts := 0; attempts < 8; attempts++ {
		if stream.Stats().QueuedBlocks == audio.StreamPageCount-1 {
			return true
		}
		stream.Service(audio.StreamPageCount)
	}
	return stream.Stats().QueuedBlocks == audio.StreamPageCount-1
}

func primeMusicFade() bool {
	beforeStream := stream.Stats()
	beforeSFX := sfxPlayer.Stats()
	if !sfxPlayer.Music(0, coexistFadeMS) {
		return false
	}
	// Apply one live deferred command and refill its discarded tail before the
	// matrix begins. The isolated benchmark owns its cycle measurement; this
	// gate verifies that the real stream path rewinds without an underrun.
	stream.Service(audio.StreamPageCount)
	filled := fillQueue()
	afterStream := stream.Stats()
	afterSFX := sfxPlayer.Stats()
	return filled && afterStream.Running &&
		afterStream.QueuedBlocks == audio.StreamPageCount-1 &&
		afterStream.Underruns == beforeStream.Underruns &&
		afterStream.SwitchFailures == beforeStream.SwitchFailures &&
		afterSFX.MusicActive && afterSFX.ActiveChannels == coexistActiveChannels &&
		afterSFX.MusicFadeSamplesRemaining != 0 && !afterSFX.MusicFadingOut &&
		coexistProfileStateValid(afterSFX) &&
		afterSFX.PendingCommands == 0 && afterSFX.Rewinds == beforeSFX.Rewinds+1 &&
		afterSFX.FallbackCommands == beforeSFX.FallbackCommands &&
		afterSFX.DroppedCommands == beforeSFX.DroppedCommands
}

func runNormalCase(index int) bool {
	before := stream.Stats()
	beforeSFX := sfxPlayer.Stats()
	var maximum tickTiming
	for tick := 0; tick < caseTicks; tick++ {
		timing := runTick(index)
		if timing.workCycles > maximum.workCycles {
			maximum.workCycles = timing.workCycles
		}
		if timing.maxServiceCycles > maximum.maxServiceCycles {
			maximum.maxServiceCycles = timing.maxServiceCycles
			maximum.maxServiceBlocks = timing.maxServiceBlocks
		}
	}
	after := stream.Stats()
	afterSFX := sfxPlayer.Stats()
	recordCase(index, before, after, afterSFX, maximum)
	return after.Running && after.Underruns == before.Underruns &&
		after.SwitchFailures == before.SwitchFailures &&
		afterSFX.MusicActive && afterSFX.ActiveChannels == coexistActiveChannels &&
		afterSFX.MusicFadeSamplesRemaining != 0 && !afterSFX.MusicFadingOut &&
		coexistProfileStateValid(afterSFX) &&
		afterSFX.PendingCommands == 0 && afterSFX.Rewinds == beforeSFX.Rewinds &&
		afterSFX.FallbackCommands == beforeSFX.FallbackCommands &&
		afterSFX.DroppedCommands == beforeSFX.DroppedCommands
}

func runStarvationCase() bool {
	// Arrive with the normal producer full before intentionally withholding it.
	if !fillQueue() {
		return false
	}
	before := stream.Stats()
	ctx.BeginFramePace()
	ctx.Cls(1)
	for hw.ReadFrameCycleCounter() < starvationCycles {
		for s := 0; s < 100; s++ {
			ctx.Spr(1+s%32, (s*16)%232, (s*8)%152, 1, 1, false, false)
		}
	}
	ctx.PrepareHiddenPage()
	cycles := hw.ReadFrameCycleCounter()
	finishFrame()

	// Refill attempts can initially be cutoff-bound near an immediate page.
	// Establish the recovery baseline only once normal Service calls have
	// repopulated all three queued pages.
	recoveredFull := fillQueue()
	for attempts := 0; attempts < 8 && !recoveredFull; attempts++ {
		runTick(0)
		recoveredFull = fillQueue()
	}
	recovered := stream.Stats()
	recoveredSFX := sfxPlayer.Stats()
	recordCase(caseCount-1, before, recovered, recoveredSFX, tickTiming{workCycles: cycles})
	coexistRecoveryQueued.Set(uint32(recovered.QueuedBlocks))

	for tick := 0; tick < recoveryTicks; tick++ {
		runTick(0)
	}
	final := stream.Stats()
	finalSFX := sfxPlayer.Stats()
	coexistRecoveryBoundaries.Set(final.Boundaries - recovered.Boundaries)
	coexistRecoveryUnderruns.Set(final.Underruns - recovered.Underruns)
	coexistFinalUnderruns.Set(final.Underruns)
	return recoveredFull && recovered.Running && recovered.QueuedBlocks == audio.StreamPageCount-1 &&
		recovered.Underruns > before.Underruns &&
		final.Underruns == recovered.Underruns &&
		final.SwitchFailures == recovered.SwitchFailures &&
		finalSFX.MusicActive && finalSFX.ActiveChannels == coexistActiveChannels &&
		finalSFX.MusicFadeSamplesRemaining != 0 && !finalSFX.MusicFadingOut &&
		coexistProfileStateValid(recoveredSFX) && coexistProfileStateValid(finalSFX) &&
		finalSFX.PendingCommands == 0 && finalSFX.FallbackCommands == 0 &&
		finalSFX.DroppedCommands == 0
}

func showResult(passed bool) {
	ctx.Cls(0)
	ctx.Print(coexistTitle, 8, 24, 15)
	ctx.Print(coexistProfileText, 8, 48, 18)
	ctx.Print("1 FORCED STARVATION CASE", 8, 64, 18)
	if passed {
		ctx.Print("EMULATOR GATE: PASS", 8, 96, 19)
		ctx.Print("REAL HARDWARE STILL REQUIRED", 8, 120, 20)
	} else {
		ctx.Print("EMULATOR GATE: FAIL", 8, 96, 21)
	}
	ctx.PrepareHiddenPage()
	ctx.WaitForPresentWindow()
	ctx.FlipPage()
}

func halt() {
	for {
	}
}

func main() {
	if !initAssets() {
		coexistStatus.Set(statusFailed)
		halt()
	}
	ctx.InitHardware()
	authorAssets()
	if !startAudio() {
		coexistStatus.Set(statusFailed)
		halt()
	}
	if !primeMusicFade() {
		coexistStatus.Set(statusFailed)
		halt()
	}
	coexistStatus.Set(statusMeasuring)
	ctx.WaitVDraw()

	passed := true
	for index := 0; index < normalCaseCount; index++ {
		if !runNormalCase(index) {
			passed = false
		}
	}
	if !runStarvationCase() {
		passed = false
	}
	stream.Stop()
	showResult(passed)
	if passed {
		coexistStatus.Set(statusDone)
	} else {
		coexistStatus.Set(statusFailed)
	}
	halt()
}
