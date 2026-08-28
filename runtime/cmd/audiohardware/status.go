package main

const (
	soakTicks       = 10 * 60 * 30
	musicCycleTicks = 300
	soakFadePairs   = soakTicks / musicCycleTicks
	statusRunning   = 0xA0A00001
	statusPassed    = 0xA0A00002
	statusStalled   = 0xA0A00003
	statusStopped   = 0xA0A00004
	statusFailed    = 0xDEADA0A0
)

func soakStatus(failed, running, service bool, ticks uint32) uint32 {
	switch {
	case failed:
		return statusFailed
	case !running:
		return statusStopped
	case !service:
		return statusStalled
	case ticks >= soakTicks:
		return statusPassed
	default:
		return statusRunning
	}
}
