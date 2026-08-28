//go:build tinygo

// Command audiohardware is the physical-GBA sign-off harness for the
// version-1 mono profiles. The default build runs two music voices and one
// looping PCM sample; the standard build tag runs four music voices. Both
// variants exercise deferred fades and Mode 4 DMA3 presentation for ten
// uninterrupted minutes. The screen and EWRAM telemetry remain useful
// without a debugger.
package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/volatile"
	_ "unsafe" // required for //go:section
)

const (
	fadeMS        = 1000
	progressWidth = 216
)

//go:section .ewram
var ctx rt.Context

//go:section .ewram
var stream audio.MonoStream

//go:section .iwram
var mixer audio.Mixer

//go:section .ewram
var player audio.SFXPlayer

var definitions = [...]audio.SFXDefinition{{
	ID: 0, Speed: 8, StepCount: 1, LoopStart: 0, LoopEnd: 1,
}}

var steps = [...]audio.SFXStep{{
	PhaseStep: 1 << 24, Volume: 28, Kind: audio.SourceTriangle,
}}

// Debugger-visible, fixed-address sign-off telemetry.
//
//go:section .ewram
var hardwareStatus volatile.Register32

//go:section .ewram
var hardwareTicks volatile.Register32

//go:section .ewram
var hardwareBoundaries volatile.Register32

//go:section .ewram
var hardwareUnderruns volatile.Register32

//go:section .ewram
var hardwareMaxLateSamples volatile.Register32

//go:section .ewram
var hardwareSwitchFailures volatile.Register32

//go:section .ewram
var hardwareQueued volatile.Register32

//go:section .ewram
var hardwareRewinds volatile.Register32

//go:section .ewram
var hardwareFallbacks volatile.Register32

//go:section .ewram
var hardwareDrops volatile.Register32

//go:section .ewram
var hardwareSampleVoices volatile.Register32

//go:section .ewram
var hardwareCommandProgress volatile.Register32

//go:section .ewram
var hardwarePendingCommands volatile.Register32

//go:section .ewram
var hardwareMusicStarts volatile.Register32

//go:section .ewram
var hardwareMusicStops volatile.Register32

//go:section .ewram
var hardwareFadeChecks volatile.Register32

//go:section .ewram
var serviceEnabled volatile.Register32

var ticks uint32
var fadeChecks uint32
var failed bool

func startAudio() bool {
	stream.Stop()
	if !mixer.Init(profileMixerVoices) ||
		!player.Init(&mixer, definitions[:], steps[:]) ||
		!initAudioProfile() || !startAudioProfile() ||
		!stream.StartSFX(&player, hw.DirectSoundRate16384) {
		failed = true
		hardwareStatus.Set(statusFailed)
		return false
	}
	ticks = 0
	fadeChecks = 0
	failed = false
	serviceEnabled.Set(1)
	hardwareStatus.Set(statusRunning)
	return true
}

func service() {
	if serviceEnabled.Get() != 0 {
		stream.Service(audio.StreamPageCount)
	}
}

func scheduleMusicFade() {
	phase := ticks % musicCycleTicks
	switch phase {
	case 150:
		stats := player.Stats()
		if stats.PendingCommands != 0 || !stats.MusicActive || stats.MusicFadingOut {
			failed = true
			return
		}
		hardwareCommandProgress.Set(uint32(hw.FIFOAStreamBlockProgress(audio.StreamBlockSamples)))
		if !player.Music(-1, fadeMS) {
			failed = true
		}
	case 210:
		stats := player.Stats()
		if stats.PendingCommands != 0 || stats.MusicActive {
			failed = true
			return
		}
		fadeChecks++
		hardwareCommandProgress.Set(uint32(hw.FIFOAStreamBlockProgress(audio.StreamBlockSamples)))
		if !player.Music(0, fadeMS) {
			failed = true
		}
	}
}

func publishTelemetry() {
	streamStats := stream.Stats()
	playerStats := player.Stats()
	hardwareTicks.Set(ticks)
	hardwareBoundaries.Set(streamStats.Boundaries)
	hardwareUnderruns.Set(streamStats.Underruns)
	hardwareMaxLateSamples.Set(uint32(streamStats.MaxLateSamples))
	hardwareSwitchFailures.Set(streamStats.SwitchFailures)
	hardwareQueued.Set(uint32(streamStats.QueuedBlocks))
	hardwareRewinds.Set(playerStats.Rewinds)
	hardwareFallbacks.Set(playerStats.FallbackCommands)
	hardwareDrops.Set(playerStats.DroppedCommands)
	hardwareSampleVoices.Set(uint32(playerStats.ActiveSampleVoices))
	hardwarePendingCommands.Set(uint32(playerStats.PendingCommands))
	hardwareMusicStarts.Set(playerStats.MusicStarts)
	hardwareMusicStops.Set(playerStats.MusicStops)
	hardwareFadeChecks.Set(fadeChecks)
	if serviceEnabled.Get() != 0 && (!streamStats.Running || streamStats.Underruns != 0 ||
		streamStats.SwitchFailures != 0 || playerStats.ActiveSampleVoices != profileSampleVoices ||
		playerStats.FailedSampleStarts != 0 || playerStats.InvalidSampleCalls != 0 ||
		playerStats.FallbackCommands != 0 || playerStats.DroppedCommands != 0 ||
		ticks >= soakTicks && (fadeChecks != soakFadePairs || playerStats.PendingCommands != 0 ||
			playerStats.MusicStarts != soakFadePairs+1)) {
		failed = true
	}
	hardwareStatus.Set(soakStatus(failed, streamStats.Running, serviceEnabled.Get() != 0, ticks))
}

func draw() {
	ctx.Cls(1)
	ctx.Print("MTILE AUDIO HARDWARE", 8, 12, 15)
	ctx.Print(profileLabel, 8, 28, 18)
	ctx.Print("MODE 4 + DMA3 / 10 MIN", 8, 44, 18)

	progress := int(ticks * progressWidth / soakTicks)
	if progress > progressWidth {
		progress = progressWidth
	}
	ctx.Rect(8, 68, 8+progressWidth, 78, 6)
	if progress > 0 {
		ctx.Rectfill(9, 69, 8+progress, 77, 11)
	}
	marker := int(ticks % progressWidth)
	ctx.Rectfill(8+marker, 88, 11+marker, 99, 10)

	switch hardwareStatus.Get() {
	case statusPassed:
		ctx.Print("HARDWARE SOAK: PASS", 8, 112, 19)
	case statusFailed:
		ctx.Print("HARDWARE SOAK: FAIL", 8, 112, 21)
	case statusStalled:
		ctx.Print("INTENTIONAL STALL", 8, 112, 20)
	case statusStopped:
		ctx.Print("AUDIO STOPPED", 8, 112, 20)
	default:
		ctx.Print("HARDWARE SOAK: RUNNING", 8, 112, 15)
	}
	ctx.Print("A: STALL  START: RESTART", 8, 136, 6)
	ctx.Print("SELECT: STOP", 8, 148, 6)
}

func runFrame() {
	ctx.BeginFramePace()
	ctx.PollInput()
	ctx.AdvanceTick()

	switch {
	case ctx.Btnp(rt.ButtonA):
		if serviceEnabled.Get() == 0 {
			startAudio()
		} else {
			serviceEnabled.Set(0)
			hardwareStatus.Set(statusStalled)
		}
	case ctx.Btnp(rt.ButtonStart):
		startAudio()
	case ctx.Btnp(rt.ButtonSelect):
		stream.Stop()
		serviceEnabled.Set(0)
		hardwareStatus.Set(statusStopped)
	}

	service()
	if stream.Stats().Running && serviceEnabled.Get() != 0 {
		ticks++
		scheduleMusicFade()
	}
	service()
	publishTelemetry()
	draw()
	service()
	ctx.PrepareHiddenPage()
	service()
	ctx.PaceFrame30()
	ctx.WaitForPresentWindow()
	ctx.ApplyDisplayPalette()
	ctx.FlipPage()
	ctx.EndFramePace30()
	ctx.WaitVDraw()
}

func main() {
	ctx.InitHardware()
	startAudio()
	for {
		runFrame()
	}
}
