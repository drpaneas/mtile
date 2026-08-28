package main

import "testing"

func TestSoakStatusThresholdAndFailurePrecedence(t *testing.T) {
	if soakFadePairs != 60 {
		t.Fatalf("soak fade pairs = %d, want 60", soakFadePairs)
	}
	tests := []struct {
		name                     string
		failed, running, service bool
		ticks                    uint32
		want                     uint32
	}{
		{name: "running", running: true, service: true, ticks: soakTicks - 1, want: statusRunning},
		{name: "passed", running: true, service: true, ticks: soakTicks, want: statusPassed},
		{name: "stalled", running: true, ticks: soakTicks, want: statusStalled},
		{name: "stopped", service: true, ticks: soakTicks, want: statusStopped},
		{name: "failed", failed: true, running: true, service: true, ticks: soakTicks, want: statusFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := soakStatus(test.failed, test.running, test.service, test.ticks); got != test.want {
				t.Fatalf("soakStatus() = %#x, want %#x", got, test.want)
			}
		})
	}
}
