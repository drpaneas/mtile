package main

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/drpaneas/mtile/internal/emulator"
)

type playtestCheckpointStub struct {
	saved   []byte
	loaded  [][]byte
	saveErr error
	loadErr error
}

func (s *playtestCheckpointStub) SaveState() ([]byte, error) {
	return bytes.Clone(s.saved), s.saveErr
}

func (s *playtestCheckpointStub) LoadState(data []byte) error {
	if s.loadErr != nil {
		return s.loadErr
	}
	s.loaded = append(s.loaded, bytes.Clone(data))
	return nil
}

func TestDevelopPlaytestStepConsumesOnlyRequestedFrames(t *testing.T) {
	state := newDevelopPlaytestState(nil, nil)
	if err := state.command(developPlaytestTogglePause, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, step := state.next(emulator.KeyRight, false); step {
		t.Fatal("paused preview advanced")
	}
	for range 2 {
		if err := state.command(developPlaytestStep, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []uint16{emulator.KeyA, emulator.KeyLeft} {
		keys, reset, step := state.next(want, false)
		if !step || reset || keys != want {
			t.Fatalf("frame request = %#x, reset %v, step %v", keys, reset, step)
		}
		state.stepped(keys)
	}
	if _, _, step := state.next(0, false); step || !state.status().Paused {
		t.Fatal("frame steps did not leave the preview paused")
	}
}

func TestDevelopPlaytestRestoreUsesCheckpointAndStaysPaused(t *testing.T) {
	boot, checkpoint := []byte("boot"), []byte("checkpoint")
	state := newDevelopPlaytestState(nil, boot)
	session := &playtestCheckpointStub{saved: checkpoint}
	if err := state.command(developPlaytestCapture, session); err != nil {
		t.Fatal(err)
	}
	if status := state.status(); !status.HasCheckpoint || !status.Paused {
		t.Fatalf("capture status = %+v", status)
	}
	if err := state.command(developPlaytestStep, nil); err != nil {
		t.Fatal(err)
	}
	if err := state.command(developPlaytestRestore, session); err != nil {
		t.Fatal(err)
	}
	if len(session.loaded) != 1 || !bytes.Equal(session.loaded[0], checkpoint) {
		t.Fatalf("restored %q, want %q", session.loaded, checkpoint)
	}
	if _, _, step := state.next(0, false); step || !state.status().Paused {
		t.Fatal("restore advanced past the checkpoint")
	}
	session.saveErr = errors.New("checkpoint read failed")
	if err := state.command(developPlaytestCapture, session); err == nil || !state.status().HasCheckpoint {
		t.Fatal("failed capture replaced the prior checkpoint")
	}
}

func TestDevelopPlaytestRecordingReplaysExactDisplayInputsAcrossBuilds(t *testing.T) {
	oldBoot, newBoot := []byte("old boot"), []byte("new boot")
	recording := newDevelopPlaytestState(nil, oldBoot)
	oldSession := &playtestCheckpointStub{saved: []byte("old checkpoint")}
	if err := recording.command(developPlaytestCapture, oldSession); err != nil {
		t.Fatal(err)
	}
	if err := recording.command(developPlaytestToggleRecord, oldSession); err != nil {
		t.Fatal(err)
	}
	want := []uint16{emulator.KeyA | emulator.KeyRight, emulator.KeyRight, emulator.KeyRight, 0, emulator.KeyLeft | emulator.KeyRight}
	for _, mask := range want {
		keys, reset, step := recording.next(mask, false)
		if !step || reset || keys != mask {
			t.Fatalf("record frame = %#x, reset %v, step %v", keys, reset, step)
		}
		recording.stepped(keys)
	}
	if err := recording.command(developPlaytestToggleRecord, oldSession); err != nil {
		t.Fatal(err)
	}
	if len(oldSession.loaded) != 1 || !bytes.Equal(oldSession.loaded[0], oldBoot) {
		t.Fatal("recording did not begin from the original boot")
	}
	saved := recording.recordedInput()
	if !slices.Equal(saved, want) || recording.status().RecordingRevision != 1 {
		t.Fatalf("recorded input = %v, want %v", saved, want)
	}

	replay := newDevelopPlaytestState(saved, newBoot)
	saved[0] = 0 // The new preview owns its own recording.
	if replay.status().HasCheckpoint {
		t.Fatal("a checkpoint survived a preview rebuild")
	}
	newSession := &playtestCheckpointStub{}
	if err := replay.command(developPlaytestReplay, newSession); err != nil {
		t.Fatal(err)
	}
	for _, mask := range want {
		keys, reset, step := replay.next(emulator.KeyStart, false)
		if !step || reset || keys != mask {
			t.Fatalf("replay frame = %#x, reset %v, step %v, want %#x", keys, reset, step, mask)
		}
		replay.stepped(keys)
	}
	if len(newSession.loaded) != 1 || !bytes.Equal(newSession.loaded[0], newBoot) {
		t.Fatal("replay reused the previous ROM's boot checkpoint")
	}
	if status := replay.status(); status.Replaying || !status.Paused || status.ReplayFrame != len(want) {
		t.Fatalf("finished replay status = %+v", status)
	}
}

func TestDevelopPlaytestRecordingCapAndEmptyRecording(t *testing.T) {
	state := newDevelopPlaytestState([]uint16{emulator.KeyStart}, []byte("boot"))
	session := &playtestCheckpointStub{}
	for range 2 {
		if err := state.command(developPlaytestToggleRecord, session); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(state.recordedInput(), []uint16{emulator.KeyStart}) || state.status().RecordingRevision != 0 {
		t.Fatal("empty recording replaced a completed recording")
	}
	if err := state.command(developPlaytestToggleRecord, session); err != nil {
		t.Fatal(err)
	}
	for range developPlaytestFrameLimit {
		keys, _, step := state.next(0, false)
		if !step {
			t.Fatal("recording stopped before its frame limit")
		}
		state.stepped(keys)
	}
	if status := state.status(); status.Recording || !status.Paused || status.RecordedFrames != developPlaytestFrameLimit || status.RecordingRevision != 1 {
		t.Fatalf("bounded recording status = %+v", status)
	}
}
