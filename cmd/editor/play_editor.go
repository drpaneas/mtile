package main

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
	"time"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type playLaunch struct {
	id      uint64
	session *emulator.Session
	result  build.Result
	helper  string
	err     error
}

type playStepper interface {
	Step(keys uint16, reset bool) (emulator.Frame, error)
}

type playSession interface {
	playStepper
	Close() error
}

type playLayout struct {
	Header    editor.Rect
	Reload    editor.Rect
	Reset     editor.Rect
	Stop      editor.Rect
	LCD       editor.Rect
	Stage     editor.Rect
	Bezel     editor.Rect
	Screen    editor.Rect
	Inspector editor.Rect
}

func newPlayLayout() playLayout {
	y := editorTabsHeight
	header := editor.Rect{X: 0, Y: y, W: screenWidth, H: 72}
	inspectorW := 340
	stage := editor.Rect{X: 0, Y: y + header.H, W: screenWidth - inspectorW, H: statusBarRect.Y - y - header.H}
	inspector := editor.Rect{X: stage.W, Y: stage.Y, W: inspectorW, H: stage.H}
	maximumW, maximumH := stage.W-100, stage.H-112
	scale := max(1, min(maximumW/emulator.Width, maximumH/emulator.Height))
	screen := editor.Rect{W: emulator.Width * scale, H: emulator.Height * scale}
	screen.X = stage.X + (stage.W-screen.W)/2
	screen.Y = stage.Y + (stage.H-screen.H)/2 - 8
	bezel := editor.Rect{X: screen.X - 38, Y: screen.Y - 38, W: screen.W + 76, H: screen.H + 86}
	return playLayout{
		Header: header,
		Reload: editor.Rect{X: screenWidth - 458, Y: y + 17, W: 142, H: 38},
		Reset:  editor.Rect{X: screenWidth - 304, Y: y + 17, W: 132, H: 38},
		Stop:   editor.Rect{X: screenWidth - 160, Y: y + 17, W: 142, H: 38},
		LCD:    editor.Rect{X: inspector.X + inspector.W - 148, Y: inspector.Y + 100, W: 128, H: 30},
		Stage:  stage, Bezel: bezel, Screen: screen, Inspector: inspector,
	}
}

func (g *Game) startRun() {
	if g == nil {
		return
	}
	// A build request always opens its own log. This makes mouse and
	// Cmd/Ctrl+R builds land in the same, predictable place.
	g.developPanel = developBuild
	g.developScroll[developBuild] = 0
	if !g.prepareCodeForBuild() {
		g.runMu.Lock()
		g.runError = ""
		g.runStatus = "BUILD BLOCKED"
		g.runBuildEvents = []build.Event{{Phase: build.PhaseSource, Message: g.statusMsg}}
		g.runMu.Unlock()
		return
	}
	g.persist()
	g.runMu.Lock()
	if g.runBusy {
		g.runMu.Unlock()
		return
	}
	g.runBusy = true
	g.runID++
	runID := g.runID
	g.runError = ""
	g.runStatus = "BUILDING CARTRIDGE..."
	g.runBuildEvents = g.runBuildEvents[:0]
	g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog[:0], "Build requested")
	g.runGeneratedGo = ""
	g.runEntry = ""
	g.runSourceFiles = nil
	g.runRendererRequested = ""
	g.runRendererSelected = ""
	g.runRendererReason = ""
	g.runRendererProfile = ""
	g.runRendererDiagnostic = ""
	g.runRendererLayers = 0
	g.runRendererObjects = false
	g.runRendererText = false
	g.runEWRAMUsed = 0
	g.runEWRAMFree = 0
	g.runIWRAMUsed = 0
	g.runMu.Unlock()
	g.theme.buildPulse = 48
	projectDir := g.model.Dir
	go func() {
		service, err := build.NewService("", "")
		if err != nil {
			g.deliverPlayLaunch(playLaunch{id: runID, err: err})
			return
		}
		service.Trace = func(event build.Event) {
			g.appendBuildEvent(runID, event)
		}
		service.Debug = true
		service.UseProjectRenderer = true
		result, err := service.Build(projectDir, "")
		if err != nil {
			g.deliverPlayLaunch(playLaunch{id: runID, err: err})
			return
		}
		if !g.currentRun(runID) {
			return
		}
		helper, err := emulator.ResolveHelper()
		if err != nil {
			g.deliverPlayLaunch(playLaunch{id: runID, result: result, err: err})
			return
		}
		session, err := emulator.Start(helper, result.ROMPath)
		g.deliverPlayLaunch(playLaunch{id: runID, session: session, result: result, helper: helper, err: err})
	}()
}

func appendBoundedLog(log []string, message string) []string {
	if strings.TrimSpace(message) == "" {
		return log
	}
	const maximum = 512
	log = append(log, message)
	if len(log) > maximum {
		copy(log, log[len(log)-maximum:])
		log = log[:maximum]
	}
	return log
}

func (g *Game) appendBuildEvent(id uint64, event build.Event) {
	g.runMu.Lock()
	defer g.runMu.Unlock()
	if id != g.runID {
		return
	}
	const maximum = 256
	g.runBuildEvents = append(g.runBuildEvents, event)
	if len(g.runBuildEvents) > maximum {
		copy(g.runBuildEvents, g.runBuildEvents[len(g.runBuildEvents)-maximum:])
		g.runBuildEvents = g.runBuildEvents[:maximum]
	}
}

func (g *Game) appendRuntimeLog(message string) {
	g.runMu.Lock()
	g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, message)
	g.runMu.Unlock()
}

func (g *Game) currentRun(id uint64) bool {
	g.runMu.Lock()
	current := id == g.runID
	g.runMu.Unlock()
	return current
}

func (g *Game) deliverPlayLaunch(launch playLaunch) {
	if !g.currentRun(launch.id) {
		if launch.session != nil {
			_ = launch.session.Close()
		}
		return
	}
	g.runReady <- launch
}

func (g *Game) pumpPlaySession() {
	select {
	case launch := <-g.runReady:
		g.acceptPlayLaunch(launch)
	default:
	}
	if g.playFrames != nil {
		for {
			select {
			case frame := <-g.playFrames:
				if g.playImage == nil {
					g.playImage = ebiten.NewImage(emulator.Width, emulator.Height)
				}
				if g.playHistory == nil {
					g.playHistory = ebiten.NewImage(emulator.Width, emulator.Height)
				}
				g.playHistory.DrawImage(g.playImage, nil)
				g.playImage.ReplacePixels(frame.RGBA)
				g.acceptPlayTelemetry(frame.Telemetry)
				g.playCount++
				g.capturePlayAudioBaseline()
				elapsed := time.Since(g.playStart).Seconds()
				if elapsed >= .5 {
					g.playFPS = float64(g.playCount) / elapsed
				}
			default:
				goto framesDone
			}
		}
	}
framesDone:
	if g.playErrors != nil {
		select {
		case err := <-g.playErrors:
			g.runMu.Lock()
			g.runError = err.Error()
			g.runStatus = "PLAYER STOPPED"
			g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, "mGBA: "+err.Error())
			g.runMu.Unlock()
			g.stopPlayback()
		default:
		}
	}
}

func (g *Game) acceptPlayTelemetry(telemetry emulator.Telemetry) {
	if !telemetry.Valid {
		return
	}
	g.playTelemetry = telemetry
	g.setPlayAudioActive(playTelemetryAudioActive(telemetry))
	if !telemetry.DebugValid || telemetry.Sequence == g.playTelemetrySeq {
		return
	}
	g.playTelemetrySeq = telemetry.Sequence
	const cyclesPerThirtyHzTick = 561792.0
	load := float64(telemetry.Stats.LastFrame) / cyclesPerThirtyHzTick
	copy(g.playCPULoad[:], g.playCPULoad[1:])
	g.playCPULoad[len(g.playCPULoad)-1] = load
	g.playCPUSamples = min(len(g.playCPULoad), g.playCPUSamples+1)
}

func playTelemetryAudioActive(telemetry emulator.Telemetry) bool {
	// NR52 exposes the four PSG channel-on flags. Preserve legacy Direct Sound
	// previews too: SOUNDCNT_H route bits mean FIFO A or B owns host output even
	// though the PSG status flags remain clear.
	const directSoundRouteMask uint16 = 0x3300
	return telemetry.Valid && (telemetry.PSG.Status&0x000f != 0 || telemetry.PSG.MixDMA&directSoundRouteMask != 0)
}

func (g *Game) setPlayAudioActive(active bool) {
	if active == g.playAudioActive {
		return
	}
	g.playAudioActive = active
	g.playAudioBaselineReady = false
	g.playAudioBaselineUnderruns = 0
	g.playAudioBaselineOverruns = 0
}

func (g *Game) capturePlayAudioBaseline() {
	if !g.playAudioActive || g.playAudioBaselineReady || g.playCount < 10 || g.playPCM == nil {
		return
	}
	_, g.playAudioBaselineUnderruns, g.playAudioBaselineOverruns = g.playPCM.Stats()
	g.playAudioBaselineReady = true
}

func (g *Game) updatePlayMode() {
	g.pumpPlaySession()

	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.startRun()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.stopPlaySession()
	}
	layout := newPlayLayout()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		switch {
		case rectContains(layout.Reload, mx, my):
			g.startRun()
		case rectContains(layout.Reset, mx, my):
			g.runMu.Lock()
			g.playReset = true
			g.runMu.Unlock()
		case rectContains(layout.Stop, mx, my):
			g.stopPlaySession()
		case rectContains(layout.LCD, mx, my):
			g.theme.playLCD = !g.theme.playLCD
			g.preferences.LCDPreview = g.theme.playLCD
			g.savePreferences()
			g.statusMsg = map[bool]string{false: "clean display", true: "LCD display"}[g.theme.playLCD]
		}
	}
	keys := playKeyMask()
	pressed := playJustPressedKeyMask()
	g.runMu.Lock()
	g.playKeys = keys
	if pressed != 0 {
		g.playPendingKeys |= pressed
		g.playPendingTicks = 2
	}
	g.runMu.Unlock()
}

func (g *Game) acceptPlayLaunch(launch playLaunch) {
	g.runMu.Lock()
	if launch.id != g.runID {
		g.runMu.Unlock()
		if launch.session != nil {
			_ = launch.session.Close()
		}
		return
	}
	g.runBusy = false
	if launch.result.ROMPath != "" {
		g.runROMPath = launch.result.ROMPath
		g.runROMSize = launch.result.ROMSize
		g.runEWRAMUsed = launch.result.EWRAMUsed
		g.runEWRAMFree = launch.result.EWRAMFree
		g.runIWRAMUsed = launch.result.IWRAMUsed
		g.runEntry = launch.result.Entry
		g.runSourceFiles = append([]string(nil), launch.result.SourceFiles...)
		g.runGeneratedGo = launch.result.GeneratedGo
		g.runRendererRequested = launch.result.RendererRequested.String()
		g.runRendererSelected = launch.result.Renderer.String()
		g.runRendererReason = launch.result.RendererReason
		g.runRendererProfile = launch.result.RendererProfile
		g.runRendererDiagnostic = launch.result.RendererDiagnostic
		g.runRendererLayers = launch.result.RendererLayers
		g.runRendererObjects = launch.result.RendererObjects
		g.runRendererText = launch.result.RendererText
	}
	if launch.err != nil {
		g.runError = launch.err.Error()
		g.runStatus = "BUILD FAILED"
		if launch.result.ROMPath != "" {
			g.runStatus = "PLAYER FAILED"
		}
		g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, g.runStatus+": "+launch.err.Error())
		g.runMu.Unlock()
		return
	}
	g.runError = ""
	g.runStatus = "RUNNING ON mGBA CORE"
	g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, "mGBA helper started: "+filepath.Base(launch.helper))
	g.runMu.Unlock()
	g.stopPlayback()

	stream := emulator.NewPCMStream(500)
	context, err := editorAudioContext()
	if err != nil {
		_ = launch.session.Close()
		g.runMu.Lock()
		g.runError = err.Error()
		g.runStatus = "AUDIO DEVICE ERROR"
		g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, "audio device: "+err.Error())
		g.runMu.Unlock()
		return
	}
	player, err := context.NewPlayer(stream)
	if err != nil {
		_ = launch.session.Close()
		g.runMu.Lock()
		g.runError = err.Error()
		g.runStatus = "AUDIO STREAM ERROR"
		g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, "audio stream: "+err.Error())
		g.runMu.Unlock()
		return
	}
	player.SetBufferSize(250 * time.Millisecond)
	g.playPCM = stream
	g.playAudio = player
	g.setPreviewPlayerVolume(g.playAudio)
	g.playFrames = make(chan emulator.Frame, 2)
	g.playErrors = make(chan error, 1)
	g.playStop = make(chan struct{})
	g.playStart = time.Now()
	g.playCount = 0
	g.playFPS = 0
	g.playAudioActive = false
	g.playAudioBaselineReady = false
	g.playAudioBaselineUnderruns = 0
	g.playAudioBaselineOverruns = 0
	g.runMu.Lock()
	g.playPaused = g.mode != modeCode
	paused := g.playPaused
	g.runMu.Unlock()
	if !paused {
		player.Play()
	}
	go g.runEmulator(launch.session, g.playPCM, g.playFrames, g.playErrors, g.playStop)
}

func (g *Game) runEmulator(session playSession, pcm *emulator.PCMStream, frames chan emulator.Frame, failures chan error, stop <-chan struct{}) {
	defer session.Close()
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		_, err := g.stepEmulator(session, pcm, frames)
		if err != nil {
			select {
			case failures <- err:
			default:
			}
			return
		}
	}
}

// stepEmulator performs one deterministic preview tick. Keeping the pause and
// input hand-off here makes the live-player lifecycle directly testable without
// launching a native helper process.
func (g *Game) stepEmulator(session playStepper, pcm *emulator.PCMStream, frames chan emulator.Frame) (bool, error) {
	g.runMu.Lock()
	paused := g.playPaused
	keys := g.playKeys | g.playPendingKeys
	reset := g.playReset
	if !paused {
		g.playReset = false
		if g.playPendingTicks > 0 {
			g.playPendingTicks--
			if g.playPendingTicks == 0 {
				g.playPendingKeys = 0
			}
		}
	}
	g.runMu.Unlock()
	if paused {
		return false, nil
	}
	g.runMu.Lock()
	g.playLastSentKeys = keys
	g.runMu.Unlock()
	frame, err := session.Step(keys, reset)
	if err != nil {
		return false, err
	}
	_, _ = pcm.Write(frame.PCM)
	frame.PCM = nil
	select {
	case frames <- frame:
	default:
		select {
		case <-frames:
		default:
		}
		select {
		case frames <- frame:
		default:
		}
	}
	return true, nil
}

var playKeyBindings = [...]struct {
	key  ebiten.Key
	mask uint16
}{
	{ebiten.KeyZ, emulator.KeyA},
	{ebiten.KeyX, emulator.KeyB},
	{ebiten.KeyBackspace, emulator.KeySelect},
	{ebiten.KeyEnter, emulator.KeyStart},
	{ebiten.KeyArrowRight, emulator.KeyRight},
	{ebiten.KeyArrowLeft, emulator.KeyLeft},
	{ebiten.KeyArrowUp, emulator.KeyUp},
	{ebiten.KeyArrowDown, emulator.KeyDown},
	{ebiten.KeyS, emulator.KeyR},
	{ebiten.KeyA, emulator.KeyL},
}

func playKeyMask() uint16 {
	var keys uint16
	for _, binding := range playKeyBindings {
		if ebiten.IsKeyPressed(binding.key) {
			keys |= binding.mask
		}
	}
	return keys
}

func playJustPressedKeyMask() uint16 {
	var keys uint16
	for _, binding := range playKeyBindings {
		if inpututil.IsKeyJustPressed(binding.key) {
			keys |= binding.mask
		}
	}
	return keys
}

func (g *Game) stopPlaySession() {
	g.runMu.Lock()
	g.runID++
	g.runBusy = false
	g.runStatus = "PLAYER STOPPED"
	g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, "Player stopped")
	g.playKeys = 0
	g.playPendingKeys = 0
	g.playPendingTicks = 0
	g.playLastSentKeys = 0
	g.playReset = false
	g.playPaused = false
	g.runMu.Unlock()
	g.stopPlayback()
}

// suspendPlaySession freezes the current cartridge when the developer opens
// another workspace. The helper remains alive so returning to Develop is instant
// and resumes the exact same game state instead of leaving a stale framebuffer.
func (g *Game) suspendPlaySession() {
	g.runMu.Lock()
	g.playKeys = 0
	g.playPendingKeys = 0
	g.playPendingTicks = 0
	g.playLastSentKeys = 0
	g.playPaused = true
	g.runMu.Unlock()
	if g.playAudio != nil {
		g.playAudio.Pause()
	}
	if g.playPCM != nil {
		g.playPCM.Discard()
	}
}

// resumePlaySession continues an existing cartridge, or asynchronously builds
// one when Develop is opened for the first time. The tab itself changes instantly.
func (g *Game) resumePlaySession() {
	g.runMu.Lock()
	g.playKeys = 0
	g.playPendingKeys = 0
	g.playPendingTicks = 0
	g.playLastSentKeys = 0
	g.playPaused = false
	busy := g.runBusy
	g.runMu.Unlock()
	if g.playStop != nil {
		g.playStart = time.Now()
		g.playCount = 0
		g.playFPS = 0
	}
	if g.playPCM != nil {
		g.playPCM.Discard()
	}
	if g.playAudio != nil {
		g.playAudio.Play()
	}
	if g.playStop == nil && !busy {
		g.startRun()
	}
}

// stopPlayback tears down only the active helper/audio stream. It is separate
// from stopPlaySession so replacing a running cartridge cannot invalidate the
// freshly completed build generation.
func (g *Game) stopPlayback() {
	g.runMu.Lock()
	g.playKeys = 0
	g.playPendingKeys = 0
	g.playPendingTicks = 0
	g.playLastSentKeys = 0
	g.playReset = false
	g.playPaused = false
	g.runMu.Unlock()
	if g.playStop != nil {
		close(g.playStop)
		g.playStop = nil
	}
	if g.playAudio != nil {
		g.playAudio.Pause()
		_ = g.playAudio.Close()
		g.playAudio = nil
	}
	if g.playPCM != nil {
		_ = g.playPCM.Close()
		g.playPCM = nil
	}
	g.playFrames = nil
	g.playErrors = nil
	g.playAudioActive = false
	g.playAudioBaselineReady = false
	g.playAudioBaselineUnderruns = 0
	g.playAudioBaselineOverruns = 0
}

func (g *Game) closePlayMode() { g.stopPlaySession() }

func (g *Game) drawPlayMode(screen *ebiten.Image) {
	layout := newPlayLayout()
	fillRect(screen, editor.Rect{X: 0, Y: editorTabsHeight, W: screenWidth, H: statusBarRect.Y - editorTabsHeight}, colorInk)
	fillRect(screen, layout.Header, colorIndigo)
	drawTextScale(screen, "PLAY", 20, layout.Header.Y+4, 2, colorText)
	drawText(screen, "CARTRIDGE PREVIEW  /  REAL mGBA CORE", 20, layout.Header.Y+layout.Header.H-uiTextHeight-4, colorLCD)
	g.runMu.Lock()
	status, runError, busy := g.runStatus, g.runError, g.runBusy
	g.runMu.Unlock()
	if status == "" {
		status = "PRESS RUN TO BUILD YOUR CARTRIDGE"
	}
	statusColor := colorRun
	if runError != "" {
		statusColor = colorDanger
	} else if busy {
		statusColor = colorHighlight
	}
	statusRect := editor.Rect{X: 250, Y: layout.Header.Y + 17, W: max(1, layout.Reload.X-262), H: 38}
	drawSmallTextCentered(screen, truncateTextToWidth(status, statusRect.W-16, uiSmallFace), statusRect, statusColor)
	drawNativeButton(screen, layout.Reload, "BUILD + RELOAD", busy, colorRun)
	drawNativeButton(screen, layout.Reset, "RESET", false, colorHighlight)
	drawNativeButton(screen, layout.Stop, "STOP", false, colorDanger)

	fillRect(screen, layout.Stage, colorBackground)
	g.drawGBABezel(screen, layout)
	fillRect(screen, layout.Inspector, colorPanel)
	strokeRect(screen, layout.Inspector, colorBorder)
	drawText(screen, "CARTRIDGE", layout.Inspector.X+20, layout.Inspector.Y+24, colorDim)
	drawText(screen, filepath.Base(g.model.Dir), layout.Inspector.X+20, layout.Inspector.Y+54, colorText)
	drawLine(screen, layout.Inspector.X+16, layout.Inspector.Y+84, layout.Inspector.X+layout.Inspector.W-16, layout.Inspector.Y+84, colorBorder)
	drawText(screen, "DISPLAY", layout.Inspector.X+20, layout.Inspector.Y+110, colorDim)
	displayLabel := "LCD  ON"
	if !g.theme.playLCD {
		displayLabel = "CLEAN"
	}
	drawNativeButton(screen, layout.LCD, displayLabel, g.theme.playLCD, colorCode)
	drawText(screen, "240 x 160  /  59.73 Hz", layout.Inspector.X+20, layout.Inspector.Y+138, colorLCD)
	drawText(screen, fmt.Sprintf("PREVIEW  %.1f FPS", g.playFPS), layout.Inspector.X+20, layout.Inspector.Y+164, colorText)
	drawText(screen, "AUDIO", layout.Inspector.X+20, layout.Inspector.Y+210, colorDim)
	drawText(screen, "48 kHz  /  STEREO", layout.Inspector.X+20, layout.Inspector.Y+238, colorLCD)
	if g.playPCM != nil {
		queued, underruns, overruns := g.playPCM.Stats()
		drawText(screen, fmt.Sprintf("BUFFER  %d frames", queued), layout.Inspector.X+20, layout.Inspector.Y+264, colorText)
		drawText(screen, fmt.Sprintf("XRUN  %d / %d", underruns, overruns), layout.Inspector.X+20, layout.Inspector.Y+288, colorDim)
	}
	drawLine(screen, layout.Inspector.X+16, layout.Inspector.Y+330, layout.Inspector.X+layout.Inspector.W-16, layout.Inspector.Y+330, colorBorder)
	drawText(screen, "CONTROLS", layout.Inspector.X+20, layout.Inspector.Y+356, colorDim)
	for index, label := range []string{"ARROWS   D-PAD", "Z / X    A / B", "ENTER    START", "BKSP     SELECT", "A / S    L / R"} {
		drawText(screen, label, layout.Inspector.X+20, layout.Inspector.Y+388+index*26, colorText)
	}
	g.runMu.Lock()
	inputKeys := g.playKeys | g.playPendingKeys
	sentKeys := g.playLastSentKeys
	g.runMu.Unlock()
	drawText(screen, "INPUT", layout.Inspector.X+20, layout.Inspector.Y+536, colorDim)
	drawText(screen, playKeyLabel(inputKeys), layout.Inspector.X+86, layout.Inspector.Y+536, playInputColor(inputKeys))
	drawText(screen, "CORE", layout.Inspector.X+20, layout.Inspector.Y+562, colorDim)
	drawText(screen, playKeyLabel(sentKeys), layout.Inspector.X+86, layout.Inspector.Y+562, playInputColor(sentKeys))
	if g.runROMPath != "" {
		drawText(screen, fmt.Sprintf("ROM  %s  /  %d bytes", filepath.Base(g.runROMPath), g.runROMSize), layout.Inspector.X+20, layout.Inspector.Y+layout.Inspector.H-46, colorLCD)
	}
}

func playInputColor(keys uint16) color.Color {
	if keys == 0 {
		return colorCommentMuted()
	}
	return colorRun
}

func playKeyLabel(keys uint16) string {
	if keys == 0 {
		return "READY"
	}
	labels := ""
	for _, binding := range [...]struct {
		mask  uint16
		label string
	}{
		{emulator.KeyLeft, "LEFT"}, {emulator.KeyRight, "RIGHT"},
		{emulator.KeyUp, "UP"}, {emulator.KeyDown, "DOWN"},
		{emulator.KeyA, "A"}, {emulator.KeyB, "B"},
		{emulator.KeyStart, "START"}, {emulator.KeySelect, "SELECT"},
		{emulator.KeyL, "L"}, {emulator.KeyR, "R"},
	} {
		if keys&binding.mask == 0 {
			continue
		}
		if labels != "" {
			labels += "+"
		}
		labels += binding.label
	}
	return labels
}

func (g *Game) drawGBABezel(screen *ebiten.Image, layout playLayout) {
	// A restrained cartridge/GBA silhouette gives the runtime a recognizable
	// identity without turning the rest of the IDE into a toy shell.
	drawHardwarePanel(screen, layout.Bezel, colorIndigo, colorRun)
	drawLine(screen, layout.Bezel.X+20, layout.Bezel.Y+12, layout.Bezel.X+layout.Bezel.W-20, layout.Bezel.Y+12, colorCode)
	fillRect(screen, editor.Rect{X: layout.Screen.X - 10, Y: layout.Screen.Y - 10, W: layout.Screen.W + 20, H: layout.Screen.H + 20}, color.RGBA{R: 13, G: 18, B: 22, A: 255})
	if g.playImage != nil {
		g.drawPlayDisplay(screen, layout)
	} else {
		fillRect(screen, layout.Screen, color.RGBA{R: 33, G: 45, B: 39, A: 255})
		drawTextScale(screen, "MTILE", layout.Screen.X+layout.Screen.W/2-56, layout.Screen.Y+layout.Screen.H/2-22, 2, colorLCD)
		drawText(screen, "BUILD A CARTRIDGE TO PLAY", layout.Screen.X+layout.Screen.W/2-94, layout.Screen.Y+layout.Screen.H/2+18, colorCommentMuted())
	}
	drawText(screen, "DOT MATRIX WITH STEREO SOUND", layout.Bezel.X+24, layout.Bezel.Y+layout.Bezel.H-24, colorLCD)
	fillRect(screen, editor.Rect{X: layout.Bezel.X + layout.Bezel.W - 44, Y: layout.Bezel.Y + layout.Bezel.H - 30, W: 10, H: 10}, colorRun)
	for index := 0; index < 7; index++ {
		x := layout.Bezel.X + layout.Bezel.W - 118 + index*10
		drawLine(screen, x, layout.Bezel.Y+layout.Bezel.H-28, x+5, layout.Bezel.Y+layout.Bezel.H-33, colorShellBorder)
	}
}

func (g *Game) drawPlayDisplay(screen *ebiten.Image, layout playLayout) {
	scale := float64(layout.Screen.W) / emulator.Width
	if g.theme.playLCD && g.playHistory != nil && g.theme.level != effectsClean {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(float64(layout.Screen.X), float64(layout.Screen.Y))
		op.Filter = ebiten.FilterNearest
		alpha := float32(.015)
		if g.theme.level == effectsFull {
			alpha = .035
		}
		op.ColorScale.ScaleAlpha(alpha)
		screen.DrawImage(g.playHistory, op)
	}
	if g.theme.playLCD {
		if shader, err := playLCDShader(); err == nil {
			strength := float32(.32)
			if g.theme.level == effectsClean {
				strength = .20
			} else if g.theme.level == effectsFull {
				strength = .60
			}
			op := &ebiten.DrawRectShaderOptions{}
			op.GeoM.Scale(scale, scale)
			op.GeoM.Translate(float64(layout.Screen.X), float64(layout.Screen.Y))
			op.Images[0] = g.playImage
			if g.playLCDUniforms == nil {
				g.playLCDUniforms = make(map[string]any, 2)
			}
			g.playLCDUniforms["Strength"] = strength
			g.playLCDUniforms["Time"] = float32(g.theme.frame) / 60
			op.Uniforms = g.playLCDUniforms
			screen.DrawRectShader(emulator.Width, emulator.Height, shader, op)
			return
		}
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(layout.Screen.X), float64(layout.Screen.Y))
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(g.playImage, op)
}

func colorCommentMuted() color.Color { return color.RGBA{R: 116, G: 135, B: 123, A: 255} }
