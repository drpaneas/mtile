//go:build !tinygo

package hw

// Host unit tests inject counters; LCD events do not progress on the host.
func BeginFramePace()                    { StartFrameCycleCounter() }
func WaitFramePace()                     {}
func WaitFrameStart()                    {}
func SetDisplayAudio(vblank, mix func()) {}
