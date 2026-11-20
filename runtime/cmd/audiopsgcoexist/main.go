//go:build tinygo

// audiopsgcoexist is the production Audio v2 graphics/CPU gate. It loops all
// four Game Boy hardware channels while drawing a dense Mode 4 scene. Debugger
// visible EWRAM words report the maximum 256 Hz scheduler service cost and a
// self-scored pass/fail result. It never configures Direct Sound, FIFO, DMA
// audio, or a software sample mixer.
package main

import (
	rt "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/audio"
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/volatile"
	"unsafe"
)

const (
	psgCoexistMeasuring = 0x50534701
	psgCoexistPassed    = 0x50534702
	psgCoexistFailed    = 0xDEAD5053
	psgServiceBudget    = 16384
)

const (
	psgFailureCycles uint32 = 1 << iota
	psgFailureEvents
	psgFailureQueue
	psgFailureRejected
	psgFailureFrames
	psgFailureChannels
)

//go:section .rodata
var sheetROM = [64]byte{}

//go:section .rodata
var mapROM = [1]uint16{}

//go:section .rodata
var flagROM = [1]byte{}

var image = rt.AssetImage{
	SheetW: 8, SheetH: 8,
	SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
	MapW: 1, MapH: 1,
	Sheet: sheetROM[:], Map: mapROM[:], Flags: flagROM[:],
}

//go:section .ewram
var sheetRAM [64]byte

//go:section .ewram
var mapRAM [1]uint16

//go:section .ewram
var flagRAM [1]byte

//go:section .ewram
var cartData rt.AssetMemory

//go:section .ewram
var ctx rt.Context

//go:section .ewram
var controller audio.Controller

//go:section .ewram
var psgCoexistPreview audio.NativePreview

//go:section .ewram
var psgCoexistStatus volatile.Register32

//go:section .ewram
var psgCoexistMaxServiceCycles volatile.Register32

//go:section .ewram
var psgCoexistEvents volatile.Register32

//go:section .ewram
var psgCoexistFrameSkips volatile.Register32

//go:section .ewram
var psgCoexistServiceCalls volatile.Register32

//go:section .ewram
var psgCoexistMinServiceCycles volatile.Register32

//go:section .ewram
var psgCoexistMeanServiceCycles volatile.Register32

//go:section .ewram
var psgCoexistSeenHardware volatile.Register32

//go:section .ewram
var psgCoexistQueueOverflows volatile.Register32

//go:section .ewram
var psgCoexistCommandsRejected volatile.Register32

//go:section .ewram
var psgCoexistFailureFlags volatile.Register32

//go:section .ewram
var psgCoexistFirstFailureService volatile.Register32

var metricBuf [40]byte

var sounds = [...]audio.Sound{
	{ID: 0, Kind: audio.SoundCH1, Pulse1: hw.Pulse1Config{Pulse: hw.PulseConfig{Duty: hw.PulseDuty50, Envelope: hw.HardwareEnvelope{Initial: 12, Direction: hw.EnvelopeDown, Pace: 2}, Route: hw.SoundRouteBoth}, Sweep: hw.HardwareSweep{Pace: 0, Direction: hw.SweepDown, Shift: 0}}},
	{ID: 1, Kind: audio.SoundCH2, Pulse2: hw.PulseConfig{Duty: hw.PulseDuty25, Envelope: hw.HardwareEnvelope{Initial: 9, Direction: hw.EnvelopeDown, Pace: 3}, Route: hw.SoundRouteBoth}},
	{ID: 2, Kind: audio.SoundCH3, Wave: hw.WaveConfig{Wave: hw.Waveform{Length: 32, Samples: [64]uint8{8, 10, 12, 14, 15, 14, 12, 10, 8, 6, 4, 2, 0, 2, 4, 6, 8, 10, 12, 14, 15, 14, 12, 10, 8, 6, 4, 2, 0, 2, 4, 6}}, Level: hw.Wave50, Route: hw.SoundRouteBoth}},
	{ID: 3, Kind: audio.SoundCH4, Noise: hw.NoiseConfig{Width: hw.Noise15Bit, Divisor: 3, Shift: 4, Envelope: hw.HardwareEnvelope{Initial: 13, Direction: hw.EnvelopeDown, Pace: 2}, Route: hw.SoundRouteBoth}},
}

var ch1Events = [...]audio.NoteEvent{
	{Tick: 0, DurationTicks: 96, Note: 60, SoundID: 0, Priority: 128},
	{Tick: 96, DurationTicks: 96, Note: 64, SoundID: 0, Priority: 128},
	{Tick: 192, DurationTicks: 96, Note: 67, SoundID: 0, Priority: 128},
	{Tick: 288, DurationTicks: 96, Note: 72, SoundID: 0, Priority: 128},
}

var ch2Events = [...]audio.NoteEvent{
	{Tick: 0, DurationTicks: 96, Note: 55, SoundID: 1, Priority: 112},
	{Tick: 96, DurationTicks: 96, Note: 57, SoundID: 1, Priority: 112},
	{Tick: 192, DurationTicks: 96, Note: 59, SoundID: 1, Priority: 112},
	{Tick: 288, DurationTicks: 96, Note: 55, SoundID: 1, Priority: 112},
}

var ch3Events = [...]audio.NoteEvent{
	{Tick: 0, DurationTicks: 96, Note: 36, SoundID: 2, Priority: 104},
	{Tick: 96, DurationTicks: 96, Note: 41, SoundID: 2, Priority: 104},
	{Tick: 192, DurationTicks: 96, Note: 43, SoundID: 2, Priority: 104},
	{Tick: 288, DurationTicks: 96, Note: 40, SoundID: 2, Priority: 104},
}

var ch4Events = [...]audio.NoteEvent{
	{Tick: 0, DurationTicks: 24, Note: 36, SoundID: 3, Priority: 120},
	{Tick: 96, DurationTicks: 24, Note: 37, SoundID: 3, Priority: 120},
	{Tick: 192, DurationTicks: 24, Note: 36, SoundID: 3, Priority: 120},
	{Tick: 288, DurationTicks: 24, Note: 37, SoundID: 3, Priority: 120},
}

var clips = [...]audio.Clip{
	{ID: 0, Kind: audio.SoundCH1, LengthTicks: 384, NoteEvents: ch1Events[:]},
	{ID: 1, Kind: audio.SoundCH2, LengthTicks: 384, NoteEvents: ch2Events[:]},
	{ID: 2, Kind: audio.SoundCH3, LengthTicks: 384, NoteEvents: ch3Events[:]},
	{ID: 3, Kind: audio.SoundCH4, LengthTicks: 384, NoteEvents: ch4Events[:]},
}

var parts = [...]audio.Part{{ID: 0, LengthTicks: 384, PSGClips: [4]int16{0, 1, 2, 3}}}
var songs = [...]audio.Song{{ID: 0, Tempo: 120, Sequence: []uint8{0}, LoopStart: 0}}

func halt() {
	for {
	}
}

func resetTelemetry() {
	psgCoexistStatus.Set(psgCoexistMeasuring)
	psgCoexistMaxServiceCycles.Set(0)
	psgCoexistEvents.Set(0)
	psgCoexistFrameSkips.Set(0)
	psgCoexistServiceCalls.Set(0)
	psgCoexistMinServiceCycles.Set(0)
	psgCoexistMeanServiceCycles.Set(0)
	psgCoexistSeenHardware.Set(0)
	psgCoexistQueueOverflows.Set(0)
	psgCoexistCommandsRejected.Set(0)
	psgCoexistFailureFlags.Set(0)
	psgCoexistFirstFailureService.Set(0)
}

func initRuntime() bool {
	cartData.SheetW, cartData.SheetH = 8, 8
	cartData.SpriteCols, cartData.SpriteRows, cartData.SpriteN = 1, 1, 1
	cartData.MapW, cartData.MapH = 1, 1
	cartData.Sheet, cartData.Map, cartData.Flags = sheetRAM[:], mapRAM[:], flagRAM[:]
	if !ctx.Init(&image, &cartData) {
		return false
	}
	ctx.InitHardware()
	ctx.SetTickRate(30)
	return controller.Init(audio.ProfilePSG) &&
		controller.ConfigureOutput(7, 7, hw.PSGMix100) &&
		controller.Bind(audio.Library{Sounds: sounds[:], Clips: clips[:], Parts: parts[:], Songs: songs[:]}) &&
		controller.PlaySong(0) && controller.Start()
}

func utoa(buf []byte, value uint32) int {
	if value == 0 {
		buf[0] = '0'
		return 1
	}
	n := 0
	for value > 0 {
		buf[n] = byte('0' + value%10)
		value /= 10
		n++
	}
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return n
}

func printMetric(label string, value uint32, y, color int) {
	n := copy(metricBuf[:], label)
	n += utoa(metricBuf[n:], value)
	ctx.Print(unsafe.String(&metricBuf[0], n), 12, y, color)
}

func drawFrame(frame uint32, preview *audio.NativePreview) {
	stats := &preview.Stats
	ctx.Cls(1)
	for i := 0; i < 96; i++ {
		x := (i*29 + int(frame)) % 232
		y := (i*17 + int(frame>>1)) % 152
		ctx.Rectfill(x, y, x+7, y+3, 2+i%14)
	}
	ctx.Rectfill(0, 0, 239, 30, 1)
	ctx.Print("AUDIO V2  CH1 CH2 CH3 CH4", 8, 5, 10)
	ctx.Print("GAME BOY PSG - PCM OFF", 8, 17, 7)
	ctx.Rectfill(6, 34, 233, 137, 1)
	status := psgCoexistStatus.Get()
	switch status {
	case psgCoexistPassed:
		ctx.Print("SELF TEST: PASS", 12, 39, 11)
	case psgCoexistFailed:
		ctx.Print("SELF TEST: FAIL", 12, 39, 21)
	default:
		ctx.Print("SELF TEST: MEASURING", 12, 39, 10)
	}
	printMetric("MAX IRQ CYCLES: ", stats.MaxServiceCycles, 54, 7)
	printMetric("MEAN IRQ CYCLES: ", stats.MeanServiceCycles, 66, 7)
	printMetric("EVENTS: ", stats.EventsApplied, 78, 7)
	printMetric("FRAME SKIPS: ", ctx.FrameSkips(), 90, 7)
	printMetric("HW CHANNELS SEEN: ", psgCoexistSeenHardware.Get(), 102, 7)
	ctx.Print("LISTEN: ALL 4 CHANNELS", 12, 117, 10)
	ctx.Print("NO DROPS / STUCK NOTES / CLICKS", 12, 129, 6)
	ctx.Print("RUN 10 MINUTES FOR SIGN-OFF", 8, 145, 11)
}

func main() {
	resetTelemetry()
	if !initRuntime() {
		psgCoexistStatus.Set(psgCoexistFailed)
		halt()
	}
	var frame uint32
	for {
		ctx.BeginFramePace()
		if !controller.PreviewInto(&psgCoexistPreview) {
			psgCoexistStatus.Set(psgCoexistFailed)
			halt()
		}
		stats := &psgCoexistPreview.Stats
		psgCoexistSeenHardware.Set(psgCoexistSeenHardware.Get() | uint32(psgCoexistPreview.PSG.HardwareActive))
		psgCoexistServiceCalls.Set(stats.ServiceCalls)
		if stats.QueueOverflows > psgCoexistQueueOverflows.Get() {
			psgCoexistQueueOverflows.Set(stats.QueueOverflows)
		}
		if stats.CommandsRejected > psgCoexistCommandsRejected.Get() {
			psgCoexistCommandsRejected.Set(stats.CommandsRejected)
		}
		drawFrame(frame, &psgCoexistPreview)
		ctx.PrepareHiddenPage()
		if stats.ServiceCalls >= 2048 {
			psgCoexistMaxServiceCycles.Set(stats.MaxServiceCycles)
			psgCoexistMinServiceCycles.Set(stats.MinServiceCycles)
			psgCoexistMeanServiceCycles.Set(stats.MeanServiceCycles)
			psgCoexistEvents.Set(stats.EventsApplied)
			psgCoexistFrameSkips.Set(ctx.FrameSkips())
			failureFlags := uint32(0)
			if stats.MaxServiceCycles > psgServiceBudget {
				failureFlags |= psgFailureCycles
			}
			if stats.EventsApplied < 64 {
				failureFlags |= psgFailureEvents
			}
			if stats.QueueOverflows != 0 {
				failureFlags |= psgFailureQueue
			}
			if stats.CommandsRejected != 0 {
				failureFlags |= psgFailureRejected
			}
			if ctx.FrameSkips() != 0 {
				failureFlags |= psgFailureFrames
			}
			if psgCoexistSeenHardware.Get() != 0x0F {
				failureFlags |= psgFailureChannels
			}
			psgCoexistFailureFlags.Set(psgCoexistFailureFlags.Get() | failureFlags)
			if psgCoexistFailureFlags.Get() != 0 {
				if psgCoexistFirstFailureService.Get() == 0 {
					psgCoexistFirstFailureService.Set(stats.ServiceCalls)
				}
				psgCoexistStatus.Set(psgCoexistFailed)
			} else if psgCoexistStatus.Get() != psgCoexistFailed {
				// PASS remains live for the full soak. Any later failure above is
				// latched permanently instead of being hidden by an early pass.
				psgCoexistStatus.Set(psgCoexistPassed)
			}
		}
		ctx.PaceFrame30()
		ctx.WaitForPresentWindow()
		ctx.ApplyDisplayPalette()
		ctx.FlipPage()
		ctx.EndFramePace30()
		ctx.WaitVDraw()
		frame++
	}
}
