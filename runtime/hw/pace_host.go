//go:build !tinygo

package hw

// ReadFrameCycleCounter mirrors StopFrameCycleCounter's host stub: it
// reports the value injected with SetFrameCycleCounterForTest. The default
// zero means host tests take the "might still be in the first VBlank"
// branch of the 30 Hz pacer, whose waits are also no-ops here.
func ReadFrameCycleCounter() uint32 { return hostFrameCycleCount }

// PaceWaitUntil is a no-op on host: there is no cycle progression to wait
// for, and spinning on the injected constant would hang tests.
func PaceWaitUntil(deadline uint32) {}
