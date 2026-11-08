package main

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/hajimehoshi/ebiten/v2"
)

type developPlaytestUI struct {
	commands chan developPlaytestAction
	updates  chan developPlaytestUpdate
	status   developPlaytestStatus
	recorded []uint16
	message  string
	pending  bool
}

type developPlaytestUpdate struct {
	status   developPlaytestStatus
	recorded []uint16
	message  string
	command  bool
}

type developPlaytestWorker struct {
	commands   <-chan developPlaytestAction
	updates    chan<- developPlaytestUpdate
	session    developPlaytestCheckpointSession
	state      *developPlaytestState
	last       developPlaytestStatus
	lastFrame  emulator.Frame
	savedFrame emulator.Frame
	message    string
	commandAck bool
}

func (g *Game) beginDevelopPlaytest() *developPlaytestWorker {
	ui := &g.playtest
	ui.commands = make(chan developPlaytestAction, developPlaytestQueueCapacity)
	ui.updates = make(chan developPlaytestUpdate, 8)
	return &developPlaytestWorker{
		commands: ui.commands, updates: ui.updates,
		state: newDevelopPlaytestState(ui.recorded, nil),
	}
}

func (g *Game) clearDevelopPlaytest() {
	recorded := g.playtest.recorded
	g.playtest = developPlaytestUI{recorded: recorded}
}

func (w *developPlaytestWorker) initialize(session playSession) {
	checkpointSession, ok := session.(developPlaytestCheckpointSession)
	if !ok {
		return
	}
	w.session = checkpointSession
	boot, err := checkpointSession.SaveState()
	if err != nil {
		w.publish("Starting checkpoint unavailable: "+err.Error(), false)
		return
	}
	w.state = newDevelopPlaytestState(w.state.recordedInput(), boot)
	w.publish("", false)
}

func (w *developPlaytestWorker) command(pcm *emulator.PCMStream, frames chan emulator.Frame) bool {
	select {
	case action := <-w.commands:
		if action == developPlaytestCapture && len(w.lastFrame.RGBA) == 0 {
			w.publish("Play at least one frame before capturing a checkpoint.", true)
			return false
		}
		before := w.state.status()
		err := w.state.command(action, w.session)
		message := ""
		if err != nil {
			message = err.Error()
		} else {
			switch action {
			case developPlaytestCapture:
				w.savedFrame = w.lastFrame
				message = "Checkpoint captured."
			case developPlaytestRestore:
				w.lastFrame = w.savedFrame
				if len(w.lastFrame.RGBA) != 0 {
					publishPreviewFrame(frames, w.lastFrame)
				}
				message = "Checkpoint restored."
			}
		}
		if before.Paused != w.state.status().Paused || action == developPlaytestRestore || action == developPlaytestReplay || action == developPlaytestToggleRecord && !before.Recording {
			pcm.Discard()
		}
		w.publish(message, true)
		return err == nil && (action == developPlaytestRestore || action == developPlaytestReplay || action == developPlaytestToggleRecord && !before.Recording)
	default:
		return false
	}
}

func (w *developPlaytestWorker) publish(message string, command bool) {
	if command || message != "" {
		w.message = message
	}
	w.commandAck = w.commandAck || command
	status := w.state.status()
	if status == w.last && w.message == "" && !w.commandAck {
		return
	}
	update := developPlaytestUpdate{status: status, message: w.message, command: w.commandAck}
	if status.RecordingRevision != w.last.RecordingRevision {
		update.recorded = w.state.recordedInput()
	}
	select {
	case w.updates <- update:
		w.last = status
		w.message, w.commandAck = "", false
	default:
	}
}

func (g *Game) pumpDevelopPlaytest() {
	ui := &g.playtest
	for {
		select {
		case update := <-ui.updates:
			if update.command {
				ui.pending = false
				ui.message = update.message
			} else if update.message != "" {
				ui.message = update.message
			}
			if update.recorded != nil {
				ui.recorded = update.recorded
			}
			if update.status.Paused != ui.status.Paused && g.playAudio != nil {
				if update.status.Paused {
					g.playAudio.Pause()
				} else if g.mode == modeCode {
					g.playAudio.Play()
				}
			}
			ui.status = update.status
			g.requestRender()
		default:
			return
		}
	}
}

type developPlaytestControl struct {
	label   string
	rect    editor.Rect
	action  developPlaytestAction
	enabled bool
}

func (g *Game) developPlaytestControls(layout developLayout) []developPlaytestControl {
	ui := &g.playtest
	status := ui.status
	g.runMu.Lock()
	building := g.runBusy
	g.runMu.Unlock()
	active := ui.commands != nil && !ui.pending && !building
	idle := active && !status.Recording && !status.Replaying
	pauseLabel, recordLabel, replayLabel := "PAUSE", "RECORD INPUT", "REPLAY FROM START"
	if status.Paused {
		pauseLabel = "RESUME"
	}
	if status.Recording {
		recordLabel = "STOP RECORDING"
	}
	if status.Replaying {
		replayLabel = "STOP REPLAY"
	}
	controls := []developPlaytestControl{
		{label: pauseLabel, action: developPlaytestTogglePause, enabled: idle},
		{label: "STEP FRAME", action: developPlaytestStep, enabled: idle && status.Paused},
		{label: "CHECKPOINT", action: developPlaytestCapture, enabled: idle && status.HasBoot},
		{label: "RESTORE", action: developPlaytestRestore, enabled: idle && status.HasCheckpoint},
		{label: recordLabel, action: developPlaytestToggleRecord, enabled: active && status.HasBoot && !status.Replaying},
		{label: replayLabel, action: developPlaytestReplay, enabled: active && status.HasBoot && !status.Recording && len(ui.recorded) != 0},
	}
	r := layout.Code.Diagnostics
	x, y := r.X+12, r.Y+46
	for i := range controls {
		control := &controls[i]
		w := max(82, developSmallTextWidth(control.label)+24)
		if x+w > r.X+r.W-12 {
			x, y = r.X+12, y+34
		}
		control.rect = editor.Rect{X: x, Y: y, W: w, H: 28}
		x += w + 8
	}
	return controls
}

func (g *Game) clickDevelopPlaytest(layout developLayout, x, y int) bool {
	if g.developPanel != developPlaytest {
		return false
	}
	for _, control := range g.developPlaytestControls(layout) {
		if !rectContains(control.rect, x, y) {
			continue
		}
		if control.enabled {
			g.releaseDevelopGameInput()
			select {
			case g.playtest.commands <- control.action:
				g.playtest.pending = true
				if control.action == developPlaytestToggleRecord && !g.playtest.status.Recording {
					g.developGameFocus = true
				}
			default:
			}
		}
		return true
	}
	return false
}

func (g *Game) drawDevelopPlaytest(screen *ebiten.Image, layout developLayout) {
	controls := g.developPlaytestControls(layout)
	for _, control := range controls {
		if control.enabled {
			drawDevelopTactileButton(screen, control.rect, control.label, false, colorRun, true)
		} else {
			drawDisabledTactileButton(screen, control.rect, control.label, true)
		}
	}
	ui := &g.playtest
	status := ui.status
	label := "Build + Reload to start a cartridge."
	if ui.commands != nil {
		label = fmt.Sprintf("Recorded input: %d frames. Capture a checkpoint to return to this spot.", len(ui.recorded))
		switch {
		case status.Recording:
			label = fmt.Sprintf("Recording from startup · %d frames · click Stop Recording to keep the input.", status.RecordedFrames)
		case status.Replaying:
			label = fmt.Sprintf("Replaying from startup · %d / %d frames", status.ReplayFrame, len(ui.recorded))
		case status.Paused:
			label = fmt.Sprintf("Paused · %d recorded frames", len(ui.recorded))
		}
	}
	r := layout.Code.Diagnostics
	y := controls[len(controls)-1].rect.Y + 38
	drawDevelopText(screen, truncateDevelopText(label, r.W-24), r.X+12, y, colorText)
	if ui.message != "" {
		drawDevelopSmallText(screen, truncateTextToWidth(ui.message, r.W-24, developSmallFace), r.X+12, y+25, colorLCD)
	}
}
