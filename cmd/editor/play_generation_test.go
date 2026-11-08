package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/emulator"
)

func TestPreviewBuildStopsWhenAssetsCannotBeSaved(t *testing.T) {
	g := newTestGame(t, 16, 16, 8, 8)
	t.Cleanup(g.closeNativePerformance)
	// A directory cannot be replaced by the file Model.Save writes here.
	projectFile := filepath.Join(g.model.Dir, "project.json")
	if err := os.RemoveAll(projectFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(projectFile, 0o755); err != nil {
		t.Fatal(err)
	}
	g.runID, g.playRunID, g.runReportID = 1, 1, 1
	g.startRun()
	status, _ := g.developPreviewStatus()
	log := strings.Join(g.developPanelLines(developBuild), "\n")
	if g.runBusy || g.runStatus != "BUILD BLOCKED" || status != "PREVIOUS BUILD" || !strings.Contains(log, "project.json") {
		t.Fatalf("busy %v, preview %q, build log %q", g.runBusy, status, log)
	}
}

func TestFailedReplacementKeepsMatchingPreviewReport(t *testing.T) {
	g := memoryTestGame()
	g.runID, g.runBusy = 2, true
	g.acceptPlayLaunch(playLaunch{id: 2, result: build.Result{ROMPath: "replacement.gba", ROMSize: 512}, err: errors.New("mGBA could not launch")})
	status, _ := g.developPreviewStatus()
	memory := strings.Join(g.developMemoryLines(), "\n")
	if status != "PREVIOUS BUILD" || g.runStatus != "PLAYER FAILED" || g.runROMSize != 32768 || !strings.Contains(memory, "idle_2 (#2)") {
		t.Fatalf("preview %q, run %q, memory %q", status, g.runStatus, memory)
	}
	g.playErrors = make(chan error, 1)
	g.playErrors <- errors.New("previous preview exited")
	g.pumpPlaySession()
	if g.playRunID != 0 || g.runError != "mGBA could not launch" {
		t.Fatalf("failed old preview remained active or replaced the build error: %d, %q", g.playRunID, g.runError)
	}
}

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
