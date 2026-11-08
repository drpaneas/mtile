package main

import (
	"errors"
	"slices"
)

const (
	developPlaytestFrameLimit    = 3600
	developPlaytestQueueCapacity = 16
)

type developPlaytestAction uint8

const (
	developPlaytestTogglePause developPlaytestAction = iota
	developPlaytestStep
	developPlaytestCapture
	developPlaytestRestore
	developPlaytestToggleRecord
	developPlaytestReplay
)

type developPlaytestCheckpointSession interface {
	SaveState() ([]byte, error)
	LoadState([]byte) error
}

type developPlaytestStatus struct {
	Paused, Recording, Replaying bool
	HasCheckpoint, HasBoot       bool
	RecordedFrames, ReplayFrame  int
	RecordingRevision            uint64
}

// The preview worker owns this state. Checkpoints belong to that worker's
// ROM. Only the completed input sequence can be reused in another worker.
type developPlaytestState struct {
	paused                   bool
	steps                    int
	bootCheckpoint           []byte
	checkpoint               []byte
	recording, replaying     bool
	recordingInput, recorded []uint16
	replayFrame              int
	recordingRevision        uint64
}

func newDevelopPlaytestState(recorded []uint16, bootCheckpoint []byte) *developPlaytestState {
	return &developPlaytestState{
		bootCheckpoint: bootCheckpoint,
		recorded:       slices.Clone(recorded[:min(len(recorded), developPlaytestFrameLimit)]),
	}
}

func (s *developPlaytestState) status() developPlaytestStatus {
	recordedFrames := len(s.recorded)
	if s.recording {
		recordedFrames = len(s.recordingInput)
	}
	return developPlaytestStatus{
		Paused: s.paused, Recording: s.recording, Replaying: s.replaying,
		HasCheckpoint: len(s.checkpoint) != 0, HasBoot: len(s.bootCheckpoint) != 0,
		RecordedFrames: recordedFrames, ReplayFrame: s.replayFrame,
		RecordingRevision: s.recordingRevision,
	}
}

func (s *developPlaytestState) recordedInput() []uint16 {
	return slices.Clone(s.recorded)
}

func (s *developPlaytestState) setPaused(paused bool) {
	s.paused, s.steps = paused, 0
}

func (s *developPlaytestState) command(action developPlaytestAction, session developPlaytestCheckpointSession) error {
	switch action {
	case developPlaytestTogglePause:
		s.setPaused(!s.paused)
	case developPlaytestStep:
		if s.steps >= developPlaytestQueueCapacity {
			return errors.New("too many pending frame steps")
		}
		s.paused = true
		s.steps++
	case developPlaytestCapture:
		if s.recording || s.replaying {
			return errors.New("finish recording or replay before capturing a checkpoint")
		}
		if session == nil {
			return errors.New("this preview cannot capture checkpoints")
		}
		data, err := session.SaveState()
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return errors.New("the preview returned an empty checkpoint")
		}
		s.checkpoint = data
		s.setPaused(true)
	case developPlaytestRestore:
		if s.recording || s.replaying {
			return errors.New("finish recording or replay before restoring a checkpoint")
		}
		if session == nil || len(s.checkpoint) == 0 {
			return errors.New("capture a checkpoint for this preview first")
		}
		if err := session.LoadState(s.checkpoint); err != nil {
			return err
		}
		s.setPaused(true)
	case developPlaytestToggleRecord:
		if s.recording {
			s.finishRecording()
			s.setPaused(true)
			return nil
		}
		if s.replaying {
			return errors.New("stop replay before recording")
		}
		if err := s.loadBoot(session); err != nil {
			return err
		}
		s.recording = true
		s.replayFrame = 0
		s.setPaused(false)
	case developPlaytestReplay:
		if s.replaying {
			s.replaying = false
			s.setPaused(true)
			return nil
		}
		if s.recording {
			return errors.New("finish recording before starting replay")
		}
		if len(s.recorded) == 0 {
			return errors.New("record some input before starting replay")
		}
		if err := s.loadBoot(session); err != nil {
			return err
		}
		s.replaying = true
		s.replayFrame = 0
		s.setPaused(false)
	default:
		return errors.New("unknown playtest action")
	}
	return nil
}

func (s *developPlaytestState) loadBoot(session developPlaytestCheckpointSession) error {
	if session == nil || len(s.bootCheckpoint) == 0 {
		return errors.New("the preview has no boot checkpoint")
	}
	return session.LoadState(s.bootCheckpoint)
}

// next returns one display-frame request. stepped must only be called once
// that request succeeds so a failed emulator tick cannot enter the recording.
func (s *developPlaytestState) next(liveKeys uint16, reset bool) (keys uint16, resetFrame bool, step bool) {
	if reset {
		s.finishRecording()
		s.replaying = false
		s.replayFrame = 0
		s.steps = 0
		return liveKeys, true, true
	}
	if s.paused && s.steps == 0 {
		return 0, false, false
	}
	if s.replaying {
		return s.recorded[s.replayFrame], false, true
	}
	return liveKeys, false, true
}

func (s *developPlaytestState) stepped(keys uint16) {
	if s.steps > 0 {
		s.steps--
	}
	if s.recording {
		s.recordingInput = append(s.recordingInput, keys)
		if len(s.recordingInput) == developPlaytestFrameLimit {
			s.finishRecording()
			s.setPaused(true)
		}
	}
	if s.replaying {
		s.replayFrame++
		if s.replayFrame == len(s.recorded) {
			s.replaying = false
			s.setPaused(true)
		}
	}
}

func (s *developPlaytestState) finishRecording() {
	if len(s.recordingInput) != 0 {
		s.recorded = s.recordingInput
		s.recordingRevision++
	}
	s.recording, s.recordingInput = false, nil
}
