package main

import (
	"testing"

	"github.com/drpaneas/mtile/internal/emulator"
)

func TestRetiredEmulatorCannotConsumeActivePreviewInput(t *testing.T) {
	for _, activeRunID := range []uint64{0, 2} {
		name := "replaced"
		if activeRunID == 0 {
			name = "stopped"
		}
		t.Run(name, func(t *testing.T) {
			// A tick and a closed stop channel can both be ready. Model the old
			// worker entering its last tick after stop/replacement completed.
			g := &Game{
				runID:            2,
				playRunID:        activeRunID,
				playKeys:         emulator.KeyA,
				playPendingKeys:  emulator.KeyLeft,
				playPendingTicks: 1,
				playLastSentKeys: emulator.KeyRight,
				playReset:        true,
			}
			stepper := &fakePlayStepper{}
			pcm := emulator.NewPCMStream(100)
			frames := make(chan emulator.Frame, 1)

			stepped, err := g.stepEmulator(stepper, pcm, frames, 1)
			if err != nil || stepped || stepper.calls != 0 {
				t.Errorf("retired tick = stepped %v, calls %d, err %v", stepped, stepper.calls, err)
			}
			if g.playKeys != emulator.KeyA || g.playPendingKeys != emulator.KeyLeft ||
				g.playPendingTicks != 1 || g.playLastSentKeys != emulator.KeyRight || !g.playReset {
				t.Error("retired preview changed active preview input or consumed its reset")
			}
			if queued, _, _ := pcm.Stats(); queued != 0 || len(frames) != 0 {
				t.Errorf("retired preview published output: PCM frames %d, video frames %d", queued, len(frames))
			}
		})
	}
}

func TestActivePreviewKeepsRunningWhileReplacementBuilds(t *testing.T) {
	g := &Game{runID: 2, playRunID: 1, playKeys: emulator.KeyA, playReset: true}
	stepper := &fakePlayStepper{}
	pcm := emulator.NewPCMStream(100)
	frames := make(chan emulator.Frame, 1)

	stepped, err := g.stepEmulator(stepper, pcm, frames, 1)
	if err != nil || !stepped || stepper.calls != 1 {
		t.Fatalf("active tick during build = stepped %v, calls %d, err %v", stepped, stepper.calls, err)
	}
	if stepper.keys != emulator.KeyA || !stepper.reset || g.playReset {
		t.Fatal("active preview did not receive and consume its input during the replacement build")
	}
}
