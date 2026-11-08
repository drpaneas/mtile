package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/drpaneas/mtile/internal/emulator"
)

func TestDevelopPlaytestNative(t *testing.T) {
	if os.Getenv("MTILE_VERIFY_PLAYTEST") == "" {
		t.Skip("set MTILE_VERIFY_PLAYTEST to exercise TinyGo and mGBA")
	}
	g := newTestGame(t, 16, 16, 8, 8)
	t.Cleanup(g.closeNativePerformance)
	t.Cleanup(g.closePlayMode)
	g.mode = modeCode
	source := "x = 20\nfunction _update() if btn(1) then x += 2 end end\nfunction _draw() cls(1) rectfill(x, 80, x + 5, 85, 7) end\n"
	setSource := func(text string) {
		g.code.SelectAll()
		g.code.Insert(text)
	}
	setSource(source)
	var latest emulator.Frame
	drain := func() {
		for {
			select {
			case frame := <-g.playFrames:
				latest = frame
			default:
				return
			}
		}
	}
	wait := func(description string, condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			g.pumpDevelopPlaytest()
			drain()
			if condition() {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		t.Fatalf("%s: status %+v, message %q, frame %d", description, g.playtest.status, g.playtest.message, latest.Number)
	}
	launch := func() {
		t.Helper()
		g.startRun()
		select {
		case ready := <-g.runReady:
			if ready.err != nil {
				ready.discard()
				t.Fatal(ready.err)
			}
			g.acceptPlayLaunch(ready)
		case <-time.After(90 * time.Second):
			t.Fatal("preview build timed out")
		}
		latest = emulator.Frame{}
		g.developPanel = developPlaytest
		wait("boot checkpoint and video", func() bool { return g.playtest.status.HasBoot && latest.Number >= 3 })
	}
	click := func(action developPlaytestAction) {
		t.Helper()
		layout := newDevelopPanelLayout(developPlaytest)
		for _, control := range g.developPlaytestControls(layout) {
			if control.action == action && control.enabled && g.clickDevelopPlaytest(layout, control.rect.X+1, control.rect.Y+1) {
				return
			}
		}
		t.Fatalf("action %d was unavailable: %+v", action, g.playtest.status)
	}
	launch()
	click(developPlaytestToggleRecord)
	wait("start recording", func() bool { return !g.playtest.pending && g.playtest.status.Recording })
	g.runMu.Lock()
	g.playKeys = emulator.KeyRight
	g.runMu.Unlock()
	wait("recorded movement", func() bool { return g.playtest.status.RecordedFrames >= 24 })
	click(developPlaytestToggleRecord)
	wait("recording ready", func() bool { return !g.playtest.pending && g.playtest.status.Paused && len(g.playtest.recorded) >= 24 })
	recordedFrame := latest
	if len(recordedFrame.RGBA) == 0 || recordedFrame.Number == 0 || !slices.Contains(g.playtest.recorded, emulator.KeyRight) {
		t.Fatal("recording did not capture input and a video frame")
	}
	click(developPlaytestCapture)
	wait("checkpoint saved", func() bool { return !g.playtest.pending && g.playtest.status.HasCheckpoint })
	click(developPlaytestStep)
	wait("exactly one stepped frame", func() bool { return !g.playtest.pending && latest.Number == recordedFrame.Number+1 })
	click(developPlaytestRestore)
	wait("checkpoint restored without stepping", func() bool { return !g.playtest.pending && latest.Number == recordedFrame.Number })
	if !bytes.Equal(latest.RGBA, recordedFrame.RGBA) {
		t.Fatal("restore did not show the checkpoint's pixels")
	}
	replay := func() {
		click(developPlaytestReplay)
		wait("replay completed", func() bool {
			return !g.playtest.pending && g.playtest.status.Paused && g.playtest.status.ReplayFrame == len(g.playtest.recorded)
		})
	}
	replay()
	if latest.Number != recordedFrame.Number || !bytes.Equal(latest.RGBA, recordedFrame.RGBA) {
		t.Fatal("replay diverged from the original input recording")
	}

	setSource(string(bytes.Replace([]byte(source), []byte("cls(1)"), []byte("cls(2)"), 1)))
	launch()
	if g.playtest.status.HasCheckpoint {
		t.Fatal("the new ROM inherited the previous checkpoint")
	}
	replay()
	if latest.Number != recordedFrame.Number || bytes.Equal(latest.RGBA, recordedFrame.RGBA) {
		t.Fatal("the new ROM did not replay the same number of frames with the edited background")
	}
	if _, err := os.Stat(filepath.Join(g.model.Dir, "game.gba")); !os.IsNotExist(err) {
		t.Fatalf("playtesting created the release ROM: %v", err)
	}
}
