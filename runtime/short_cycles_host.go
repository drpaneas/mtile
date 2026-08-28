//go:build !tinygo

package runtime

import "github.com/drpaneas/mtile/runtime/hw"

// Host tests retain the injectable short counter. Target production uses the
// already-running frame counter so Audio v2 remains the sole Timer0 owner.
func startShortCycleMeasurement() uint32 {
	hw.StartCycleCounter()
	return 0
}

func stopShortCycleMeasurement(uint32) uint32 { return hw.StopCycleCounter() }
