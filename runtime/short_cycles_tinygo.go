//go:build tinygo

package runtime

import "github.com/drpaneas/mtile/runtime/hw"

func startShortCycleMeasurement() uint32 { return hw.ReadFrameCycleCounter() }

func stopShortCycleMeasurement(start uint32) uint32 {
	return hw.ReadFrameCycleCounter() - start
}
