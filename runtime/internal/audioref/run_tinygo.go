//go:build tinygo

package audioref

import (
	rt "github.com/drpaneas/mtile/runtime"
	"runtime/volatile"
	_ "unsafe" // required for //go:section
)

//go:section .ewram
var context rt.Context

// Debugger-visible capture identity.
//
//go:section .ewram
var referenceKind volatile.Register32

//go:section .ewram
var referenceCase volatile.Register32

//go:section .ewram
var referenceRunning volatile.Register32

func draw(kind Kind, index int, running bool) {
	context.Cls(1)
	context.Print("MTILE AUDIO TRUTH", 8, 12, 15)
	context.Print(KindName(kind), 8, 32, 18)
	context.Print(CaseName(kind, index), 8, 52, 19)
	context.Print("A/RIGHT: NEXT", 8, 76, 6)
	context.Print("LEFT: PREVIOUS", 8, 90, 6)
	context.Print("B: AUTO/MANUAL", 8, 104, 6)
	context.Print("START: RESTART", 8, 118, 6)
	context.Print("SELECT: STOP", 8, 132, 6)
	if running {
		context.Print("STATE: PLAYING", 8, 146, 11)
	} else {
		context.Print("STATE: STOPPED", 8, 146, 21)
	}
}

// Run is the common cartridge loop; audio setup remains the direct-register
// Start implementation above and never enters the production audio runtime.
func Run(kind Kind) {
	context.InitHardware()
	index := 0
	running := startVisibleCase(kind, index)
	auto := true
	autoTicks := 0
	referenceKind.Set(uint32(kind))
	referenceCase.Set(uint32(index))
	if running {
		referenceRunning.Set(1)
	}
	for {
		context.BeginFramePace()
		context.PollInput()
		context.AdvanceTick()
		switch {
		case context.Btnp(rt.ButtonA) || context.Btnp(rt.ButtonRight):
			index = (index + 1) % CaseCount(kind)
			running = startVisibleCase(kind, index)
			autoTicks = 0
		case context.Btnp(rt.ButtonLeft):
			index--
			if index < 0 {
				index = CaseCount(kind) - 1
			}
			running = startVisibleCase(kind, index)
			autoTicks = 0
		case context.Btnp(rt.ButtonB):
			auto = !auto
			autoTicks = 0
		case context.Btnp(rt.ButtonStart):
			running = startVisibleCase(kind, index)
			autoTicks = 0
		case context.Btnp(rt.ButtonSelect):
			Stop()
			running = false
			auto = false
			autoTicks = 0
		default:
			if auto {
				autoTicks++
				if autoTicks >= 60 {
					index = (index + 1) % CaseCount(kind)
					running = startVisibleCase(kind, index)
					autoTicks = 0
				}
			}
		}
		referenceCase.Set(uint32(index))
		if running {
			referenceRunning.Set(1)
		} else {
			referenceRunning.Set(0)
		}
		draw(kind, index, running)
		context.PrepareHiddenPage()
		context.PaceFrame30()
		context.WaitForPresentWindow()
		context.ApplyDisplayPalette()
		context.FlipPage()
		context.EndFramePace30()
		context.WaitVDraw()
	}
}

func startVisibleCase(kind Kind, index int) bool {
	if !Start(kind, index) {
		return false
	}
	return !((kind == FIFOA || kind == FIFOB) && index == 5)
}
