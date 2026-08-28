//go:build !tinygo

package hw

// hostCycleCount lets host tests simulate a measured copy duration; real
// hardware timing only exists under TinyGo. Reset via ResetCycleCounterForTest.
var (
	hostCycleCount      uint32
	hostFrameCycleCount uint32
)

func StartCycleCounter() {}

func StopCycleCounter() uint32 { return hostCycleCount }

func StartFrameCycleCounter() {}

func StopFrameCycleCounter() uint32 { return hostFrameCycleCount }

// SetCycleCounterForTest is a test control, not runtime API. It makes the
// next StopCycleCounter call report a chosen cycle count, so host tests can
// simulate a copy slow enough to wrap one or more full frames.
func SetCycleCounterForTest(cycles uint32) {
	hostCycleCount = cycles
}

// SetFrameCycleCounterForTest is a host-test control for frame-budget
// instrumentation. It is intentionally unavailable to TinyGo callers.
func SetFrameCycleCounterForTest(cycles uint32) {
	hostFrameCycleCount = cycles
}
