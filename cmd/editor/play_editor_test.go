package main

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/drpaneas/mtile/internal/emulator"
)

func TestRuntimeLogsIgnoreReplacedPreviewAndRemainBounded(t *testing.T) {
	g := &Game{runID: 2}
	g.acceptRuntimeLogs(1, []string{"stale cartridge"})
	if len(g.runRuntimeLog) != 0 {
		t.Fatal("old preview wrote into the current run log")
	}
	for i := 0; i < 600; i++ {
		g.acceptRuntimeLogs(2, []string{"current cartridge"})
	}
	if len(g.runRuntimeLog) != 512 {
		t.Fatalf("runtime log length = %d", len(g.runRuntimeLog))
	}
	g.stopPlaySession()
	g.acceptRuntimeLogs(2, []string{"late message after stop"})
	if strings.Contains(strings.Join(g.runRuntimeLog, "\n"), "late message") {
		t.Fatal("stopped preview appended a late message")
	}
}

func TestPreviewHeapErrorsExplainFailureOnceAndClearOnReset(t *testing.T) {
	g := &Game{}
	telemetry := emulator.Telemetry{Valid: true, DebugValid: true, Sequence: 2}
	telemetry.Stats.HeapError = emulator.HeapOOM
	g.acceptPlayTelemetry(telemetry)
	telemetry.Sequence += 2
	g.acceptPlayTelemetry(telemetry)
	if len(g.runRuntimeLog) != 1 || !strings.Contains(g.runError, "Lua memory is full") {
		t.Fatalf("heap failure = %q, logs %q", g.runError, g.runRuntimeLog)
	}
	telemetry.Stats.HeapError = emulator.HeapOK
	g.acceptPlayTelemetry(telemetry)
	if g.runError != "" {
		t.Fatalf("reset retained heap error: %s", g.runError)
	}
	g.runError = "unrelated build error"
	g.acceptPlayTelemetry(telemetry)
	if g.runError != "unrelated build error" {
		t.Fatal("healthy telemetry cleared an unrelated error")
	}
	g.runID = 1 // Old preview is still running while its replacement builds.
	telemetry.Stats.HeapError = emulator.HeapOOM
	g.acceptPlayTelemetry(telemetry)
	if len(g.runRuntimeLog) != 1 {
		t.Fatal("old preview reported a heap failure into the replacement build")
	}
	g.stopPlayback()
	g.playRunID = g.runID
	g.runRuntimeLog, g.runError = nil, ""
	g.acceptPlayTelemetry(telemetry)
	if len(g.runRuntimeLog) != 1 || !strings.Contains(g.runError, "Lua memory is full") {
		t.Fatal("new preview suppressed a repeated heap failure")
	}
}

type failingLogSession struct {
	logs   []string
	closed bool
}

func (s *failingLogSession) Step(uint16, bool) (emulator.Frame, error) {
	return emulator.Frame{}, errors.New("frame read failed")
}

func (s *failingLogSession) Close() error {
	if !s.closed {
		s.logs = append(s.logs, "final cartridge diagnostic")
		s.closed = true
	}
	return nil
}

func (s *failingLogSession) DrainLogs() []string {
	logs := s.logs
	s.logs = nil
	return logs
}

func TestEmulatorFailureDrainsLastDiagnostics(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := &Game{}
		session := &failingLogSession{}
		failures := make(chan error, 1)
		g.runEmulator(session, emulator.NewPCMStream(100), make(chan emulator.Frame, 1), failures, make(chan struct{}), 0)
		if !session.closed || len(failures) != 1 || len(g.runRuntimeLog) != 1 || g.runRuntimeLog[0] != "final cartridge diagnostic" {
			t.Fatalf("missing final diagnostic: %q", g.runRuntimeLog)
		}
	})
}

func TestCartridgePlaybackBufferFitsInteractiveLatencyBudget(t *testing.T) {
	frameDuration := time.Second / 60
	if cartridgePlaybackBufferSize < frameDuration {
		t.Fatalf("cartridge buffer = %s, want at least one 60 Hz frame (%s)", cartridgePlaybackBufferSize, frameDuration)
	}
	if cartridgePlaybackBufferSize > 25*time.Millisecond {
		t.Fatalf("cartridge buffer = %s, want no more than 25ms", cartridgePlaybackBufferSize)
	}
	// Oto can briefly hold almost twice the configured player buffer. Include
	// the device queue so future tuning cannot silently restore perceptible lag.
	hostQueueBudget := 2*cartridgePlaybackBufferSize + editorDeviceBufferSize
	if hostQueueBudget > 100*time.Millisecond {
		t.Fatalf("cartridge host queue budget = %s, want no more than 100ms", hostQueueBudget)
	}
}

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

	stepped, err := g.stepEmulator(stepper, pcm, frames, g.runID)
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
	stepped, err = g.stepEmulator(stepper, pcm, frames, g.runID)
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
	_, err = g.stepEmulator(stepper, pcm, frames, g.runID)
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
