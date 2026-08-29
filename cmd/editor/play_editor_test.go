package main

import (
	"testing"

	"github.com/drpaneas/mtile/internal/emulator"
)

type fakePlayStepper struct {
	calls int
	keys  uint16
	reset bool
}

func (s *fakePlayStepper) Step(keys uint16, reset bool) (emulator.Frame, error) {
	s.calls++
	s.keys = keys
	s.reset = reset
	return emulator.Frame{Number: uint32(s.calls), PCM: []byte{1, 2, 3, 4}}, nil
}

func TestStepEmulatorPausesAndForwardsLiveInput(t *testing.T) {
	g := &Game{
		playPaused:       true,
		playKeys:         emulator.KeyA,
		playPendingKeys:  emulator.KeyLeft,
		playPendingTicks: 2,
		playReset:        true,
	}
	stepper := &fakePlayStepper{}
	pcm := emulator.NewPCMStream(100)
	frames := make(chan emulator.Frame, 1)

	stepped, err := g.stepEmulator(stepper, pcm, frames)
	if err != nil || stepped || stepper.calls != 0 {
		t.Fatalf("paused tick = stepped %v, calls %d, err %v", stepped, stepper.calls, err)
	}
	if !g.playReset {
		t.Fatal("pause consumed the pending reset")
	}
	if g.playPendingTicks != 2 || g.playPendingKeys != emulator.KeyLeft {
		t.Fatal("pause consumed the pending key press")
	}

	g.runMu.Lock()
	g.playPaused = false
	g.playKeys = emulator.KeyRight | emulator.KeyB
	g.runMu.Unlock()
	stepped, err = g.stepEmulator(stepper, pcm, frames)
	if err != nil || !stepped || stepper.calls != 1 {
		t.Fatalf("live tick = stepped %v, calls %d, err %v", stepped, stepper.calls, err)
	}
	if stepper.keys != emulator.KeyRight|emulator.KeyB|emulator.KeyLeft || !stepper.reset {
		t.Fatalf("forwarded input = keys %#x, reset %v", stepper.keys, stepper.reset)
	}
	if g.playLastSentKeys != stepper.keys {
		t.Fatalf("last sent input = %#x, want %#x", g.playLastSentKeys, stepper.keys)
	}
	if g.playReset {
		t.Fatal("live tick did not consume the pending reset")
	}
	if g.playPendingTicks != 1 || g.playPendingKeys != emulator.KeyLeft {
		t.Fatal("first live tick did not retain the key latch for its second tick")
	}
	if frame := <-frames; frame.Number != 1 {
		t.Fatalf("published frame = %d, want 1", frame.Number)
	}
	if queued, _, _ := pcm.Stats(); queued != 1 {
		t.Fatalf("queued PCM frames = %d, want 1", queued)
	}
	_, err = g.stepEmulator(stepper, pcm, frames)
	if err != nil || g.playPendingTicks != 0 || g.playPendingKeys != 0 {
		t.Fatalf("second live tick did not consume key latch: ticks=%d keys=%#x err=%v", g.playPendingTicks, g.playPendingKeys, err)
	}
}

func TestPlayKeyLabel(t *testing.T) {
	if got := playKeyLabel(0); got != "READY" {
		t.Fatalf("empty label = %q", got)
	}
	if got := playKeyLabel(emulator.KeyLeft | emulator.KeyA | emulator.KeyStart); got != "LEFT+A+START" {
		t.Fatalf("combined label = %q", got)
	}
}

func TestHostAudioXRunsStartCountingOnlyWhilePSGIsActive(t *testing.T) {
	pcm := emulator.NewPCMStream(100)
	g := &Game{playPCM: pcm, playCount: 20}
	buffer := make([]byte, 4)
	_, _ = pcm.Read(buffer) // A silent-preview transport underrun.

	g.acceptPlayTelemetry(emulator.Telemetry{Valid: true})
	g.capturePlayAudioBaseline()
	if g.playAudioActive || g.playAudioBaselineReady {
		t.Fatalf("silent telemetry armed host audio: active=%v baseline=%v", g.playAudioActive, g.playAudioBaselineReady)
	}

	g.acceptPlayTelemetry(emulator.Telemetry{Valid: true, PSG: emulator.PSGRegisters{Status: 1}})
	g.capturePlayAudioBaseline()
	if !g.playAudioActive || !g.playAudioBaselineReady {
		t.Fatalf("active PSG did not arm host audio: active=%v baseline=%v", g.playAudioActive, g.playAudioBaselineReady)
	}
	_, _ = pcm.Read(buffer)
	_, underruns, overruns := g.developHostAudioStatus()
	if underruns != 1 || overruns != 0 {
		t.Fatalf("active host audio xruns = under %d over %d, want 1/0", underruns, overruns)
	}

	g.acceptPlayTelemetry(emulator.Telemetry{Valid: true})
	if g.playAudioActive || g.playAudioBaselineReady {
		t.Fatalf("stopped PSG retained host audio counters: active=%v baseline=%v", g.playAudioActive, g.playAudioBaselineReady)
	}
}

func TestPlayTelemetryAudioActivityIncludesPSGAndDirectSound(t *testing.T) {
	if playTelemetryAudioActive(emulator.Telemetry{}) {
		t.Fatal("invalid telemetry reported active audio")
	}
	if playTelemetryAudioActive(emulator.Telemetry{Valid: true}) {
		t.Fatal("silent telemetry reported active audio")
	}
	if !playTelemetryAudioActive(emulator.Telemetry{Valid: true, PSG: emulator.PSGRegisters{Status: 1 << 2}}) {
		t.Fatal("active PSG channel reported silent audio")
	}
	if !playTelemetryAudioActive(emulator.Telemetry{Valid: true, PSG: emulator.PSGRegisters{MixDMA: 1 << 12}}) {
		t.Fatal("routed Direct Sound channel reported silent audio")
	}
}

func TestPlaySessionSuspendsAndResumesWithoutBeingDestroyed(t *testing.T) {
	stop := make(chan struct{})
	g := &Game{playStop: stop, playKeys: emulator.KeyLeft}
	g.suspendPlaySession()
	if !g.playPaused || g.playKeys != 0 || g.playStop != stop {
		t.Fatalf("suspended state = paused %v, keys %#x, stop replaced %v", g.playPaused, g.playKeys, g.playStop != stop)
	}

	g.resumePlaySession()
	if g.playPaused || g.playStop != stop {
		t.Fatalf("resumed state = paused %v, stop replaced %v", g.playPaused, g.playStop != stop)
	}
}
