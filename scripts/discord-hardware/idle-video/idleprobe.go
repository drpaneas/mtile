//go:build tinygo

package main

import (
	"github.com/drpaneas/mtile/runtime/hw"
	"runtime/volatile"
)

//go:section .ewram
var probe [2048]volatile.Register32

func main() {
	hw.SetReg16(hw.DISPCNT, 3|1<<10)
	hw.SetReg16(hw.INTERRUPT_IME, 1)
	// Check entry at every scanline. Current VBlank must return immediately.
	for line := uint16(0); line < 228; line++ {
		for hw.GetReg16(hw.VCOUNT) != line {
		}
		hw.StartFrameCycleCounter()
		hw.WaitForPresentWindow()
		probe[int(line)*2].Set(uint32(hw.GetReg16(hw.VCOUNT)))
		probe[int(line)*2+1].Set(hw.ReadFrameCycleCounter())
	}
	// Two-scan pacing, including current VBlank and late-work inputs.
	for n := 0; n < 120; n++ {
		hw.WaitForPresentWindow()
		hw.WaitVDraw()
		hw.StartFrameCycleCounter()
		if n%3 == 1 {
			for hw.ReadFrameCycleCounter() < 300000 {
			}
		}
		if n%3 == 2 {
			for hw.ReadFrameCycleCounter() < 480000 {
			}
		}
		hw.PaceWaitUntil(280896)
		hw.WaitForPresentWindow()
		probe[500+n*3].Set(hw.ReadFrameCycleCounter())
		probe[501+n*3].Set(uint32(hw.GetReg16(hw.VCOUNT)))
		hw.WaitVDraw()
		probe[502+n*3].Set(hw.ReadFrameCycleCounter())
	}
	probe[2047].Set(0x600d600d)
	for {
		hw.WaitForPresentWindow()
		hw.WaitVDraw()
	}
}
