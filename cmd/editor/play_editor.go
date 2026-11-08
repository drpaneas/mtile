package main

import (
	"errors"
	"image/color"
	"os"
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
	romDir  string
	lesson  string
	err     error
}

func (launch playLaunch) discard() {
	if launch.session != nil {
		_ = launch.session.Close()
	}
	if launch.romDir != "" {
		_ = os.RemoveAll(launch.romDir)
	}
}

type playStepper interface {
	Step(keys uint16, reset bool) (emulator.Frame, error)
}

type playSession interface {
	playStepper
	Close() error
}

// The live helper delivers about one video frame (16.7 ms) of PCM per tick.
// Keep Oto's read-ahead just above that chunk: its streaming buffer can briefly
// approach twice the configured size, so the old 250 ms value produced roughly
// half a second of sound latency in interactive cartridge previews.
const cartridgePlaybackBufferSize = 20 * time.Millisecond

type playLayout struct {
	Header    editor.Rect
	Reload    editor.Rect
	Reset     editor.Rect
	Stop      editor.Rect
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
		Stage:  stage, Bezel: bezel, Screen: screen, Inspector: inspector,
	}
}

func (g *Game) startRun() {
	if g == nil {
		return
	}
	if g.lessonEntry != "" {
		g.startPreviewBuild(g.lessonEntry, g.projectSourceOverlay(), false)
		return
	}
	g.startPreviewBuild("", nil, true)
}

func (g *Game) startPreviewBuild(lesson string, overlay map[string]string, save bool) bool {
	g.pumpDevelopPlaytest()
	if g.playtest.pending {
		g.statusMsg = "Wait for the current playtest action to finish."
		return false
	}
	if g.playtest.status.Recording {
		g.statusMsg = "Finish recording input before starting another build."
		return false
	}
	g.runMu.Lock()
	busy := g.runBusy
	g.runMu.Unlock()
	if busy || g.romExport.ready != nil {
		return false
	}
	// A build request always opens its own log. This makes mouse and
	// Cmd/Ctrl+R builds land in the same, predictable place.
	g.developPanel = developBuild
	g.developScroll[developBuild] = 0
	if save && !g.prepareCodeForBuild() {
		g.blockPreviewBuild(build.PhaseSource)
		return false
	}
	if save {
		if err := g.persist(); err != nil {
			g.blockPreviewBuild(build.PhaseAssets)
			return false
		}
	}
	g.runMu.Lock()
	g.runBusy = true
	g.runID++
	runID := g.runID
	g.runError = ""
	g.runStatus = "BUILDING CARTRIDGE..."
	g.runBuildEvents = g.runBuildEvents[:0]
	if lesson != "" {
		g.runBuildEvents = append(g.runBuildEvents, build.Event{Phase: build.PhaseSource, Message: "Lesson: " + lesson})
	}
	g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog[:0], "Build requested")
	g.runEntry = ""
	g.runSourceFiles = nil
	g.runGlobalUsage = build.GlobalUsage{}
	g.sourceAnalysisError = ""
	g.runMu.Unlock()
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
		service.EntryOverride, service.SourceOverlay = lesson, overlay
		outputPath, err := developPreviewROMPath(projectDir)
		if err != nil {
			g.deliverPlayLaunch(playLaunch{id: runID, err: err})
			return
		}
		romDir := filepath.Dir(outputPath)
		result, err := service.Build(projectDir, outputPath)
		if err != nil {
			g.deliverPlayLaunch(playLaunch{id: runID, romDir: romDir, err: err})
			return
		}
		if !g.currentRun(runID) {
			_ = os.RemoveAll(romDir)
			return
		}
		helper, err := emulator.ResolveHelper()
		if err != nil {
			g.deliverPlayLaunch(playLaunch{id: runID, result: result, romDir: romDir, err: err})
			return
		}
		session, err := emulator.Start(helper, result.ROMPath)
		g.deliverPlayLaunch(playLaunch{id: runID, session: session, result: result, helper: helper, romDir: romDir, lesson: lesson, err: err})
	}()
	return true
}

func (g *Game) blockPreviewBuild(phase build.Phase) {
	g.runMu.Lock()
	defer g.runMu.Unlock()
	g.runID++
	g.runError = g.statusMsg
	g.runStatus = "BUILD BLOCKED"
	g.runBuildEvents = []build.Event{{Phase: phase, Message: "Could not save changes before build."}}
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

func (g *Game) currentRun(id uint64) bool {
	g.runMu.Lock()
	current := id == g.runID
	g.runMu.Unlock()
	return current
}

func (g *Game) deliverPlayLaunch(launch playLaunch) {
	if !g.currentRun(launch.id) {
		launch.discard()
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
				g.playImage.WritePixels(frame.RGBA)
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
			if g.playRunID == g.runID {
				g.runError = err.Error()
				g.runStatus = "PLAYER STOPPED"
			}
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
	previous := g.playTelemetry
	g.runMu.Lock()
	if g.playRunID == g.runID && telemetry.DebugValid && (!previous.DebugValid || telemetry.Stats.HeapError != previous.Stats.HeapError) {
		message := runtimeHeapErrorMessage(telemetry.Stats.HeapError)
		if message != "" {
			g.runError = message
			g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, message)
		} else if g.runError == runtimeHeapErrorMessage(previous.Stats.HeapError) {
			g.runError = ""
		}
	}
	g.runMu.Unlock()
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

func runtimeHeapErrorMessage(err emulator.HeapError) string {
	switch err {
	case emulator.HeapOK:
		return ""
	case emulator.HeapOOM:
		return "Lua memory is full. Reuse tables and strings, release unused objects, then reset the preview."
	case emulator.HeapCorrupt, emulator.HeapCorruptRef:
		return "Lua memory is damaged. Reset the preview; if this repeats, report it with the project and steps to reproduce."
	default:
		return "Lua memory could not be updated. Reset the preview; if this repeats, report it with the project and steps to reproduce."
	}
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

func (g *Game) acceptPlayLaunch(launch playLaunch) {
	g.runMu.Lock()
	if launch.id != g.runID {
		g.runMu.Unlock()
		launch.discard()
		return
	}
	g.runBusy = false
	if launch.result.Entry != "" {
		g.runEntry = launch.result.Entry
		g.runSourceFiles = append([]string(nil), launch.result.SourceFiles...)
		g.runGlobalUsage = cloneGlobalUsage(launch.result.GlobalUsage)
		g.sourceAnalysisError = ""
	}
	if launch.result.ROMPath != "" && g.playRunID == 0 {
		g.setPreviewBuildReport(launch)
	}
	if launch.err != nil {
		g.runError = launch.err.Error()
		g.runStatus = "BUILD FAILED"
		if launch.result.ROMPath != "" {
			g.runStatus = "PLAYER FAILED"
		}
		g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, g.runStatus+": "+launch.err.Error())
		g.runMu.Unlock()
		launch.discard()
		return
	}
	g.runMu.Unlock()

	stream := emulator.NewPCMStream(500)
	context, err := editorAudioContext()
	if err != nil {
		launch.discard()
		g.runMu.Lock()
		g.runError = err.Error()
		g.runStatus = "AUDIO DEVICE ERROR"
		g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, "audio device: "+err.Error())
		g.runMu.Unlock()
		return
	}
	player, err := context.NewPlayer(stream)
	if err != nil {
		launch.discard()
		g.runMu.Lock()
		g.runError = err.Error()
		g.runStatus = "AUDIO STREAM ERROR"
		g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, "audio stream: "+err.Error())
		g.runMu.Unlock()
		return
	}
	player.SetBufferSize(cartridgePlaybackBufferSize)
	g.stopPlayback()
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
	g.setPreviewBuildReport(launch)
	g.runError = ""
	g.runStatus = "RUNNING ON mGBA CORE"
	g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, "mGBA helper started: "+filepath.Base(launch.helper))
	g.playRunID = launch.id
	g.playLessonEntry = launch.lesson
	g.playPaused = g.mode != modeCode
	paused := g.playPaused
	g.runMu.Unlock()
	if !paused {
		player.Play()
	}
	pcm, frames, failures, stop := g.playPCM, g.playFrames, g.playErrors, g.playStop
	playtest := g.beginDevelopPlaytest()
	go func() {
		g.runEmulator(launch.session, pcm, frames, failures, stop, launch.id, playtest)
		if launch.romDir != "" {
			_ = os.RemoveAll(launch.romDir)
		}
	}()
}

// Hold runMu while publishing the report that belongs to a built ROM.
func (g *Game) setPreviewBuildReport(launch playLaunch) {
	result := launch.result
	g.runReportID = launch.id
	g.runROMPath = result.ROMPath
	g.runROMSize = result.ROMSize
	g.runEWRAMUsed = result.EWRAMUsed
	g.runEWRAMFree = result.EWRAMFree
	g.runIWRAMUsed = result.IWRAMUsed
	g.runMemoryReady = true
	g.runPoseMemory = result.PoseMemory
	g.runPoseMemory.PoseNames = append([]string(nil), result.PoseMemory.PoseNames...)
	g.runGeneratedGo = result.GeneratedGo
	g.runRendererRequested = result.RendererRequested.String()
	g.runRendererSelected = result.Renderer.String()
	g.runRendererReason = result.RendererReason
	g.runRendererProfile = result.RendererProfile
	g.runRendererDiagnostic = result.RendererDiagnostic
	g.runRendererLayers = result.RendererLayers
	g.runRendererObjects = result.RendererObjects
	g.runRendererText = result.RendererText
}

func (g *Game) runEmulator(session playSession, pcm *emulator.PCMStream, frames chan emulator.Frame, failures chan error, stop <-chan struct{}, runID uint64, playtest *developPlaytestWorker) {
	defer func() { _ = session.Close() }()
	if playtest != nil {
		playtest.initialize(session)
	}
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		if playtest != nil {
			g.runMu.Lock()
			active := g.playRunID == runID && !g.playPaused
			g.runMu.Unlock()
			if active && playtest.command(pcm, frames) {
				g.runMu.Lock()
				if g.playRunID == runID {
					g.playReset = false
				}
				g.runMu.Unlock()
			}
		}
		_, err := g.stepEmulator(session, pcm, frames, runID, playtest)
		if playtest != nil {
			playtest.publish("", false)
		}
		if err != nil {
			// Wait for the helper's stderr pump before draining its final lines.
			// A frame EOF can arrive before the diagnostic explaining the exit.
			err = errors.Join(err, session.Close())
			if source, ok := session.(interface{ DrainLogs() []string }); ok {
				g.acceptRuntimeLogs(runID, source.DrainLogs())
			}
			g.runMu.Lock()
			active := g.playRunID == runID
			g.runMu.Unlock()
			if !active {
				return
			}
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
func (g *Game) stepEmulator(session playStepper, pcm *emulator.PCMStream, frames chan emulator.Frame, runID uint64, playtest *developPlaytestWorker) (bool, error) {
	g.runMu.Lock()
	// Closing playStop does not join the worker: its ticker can already be
	// ready. Retired previews must not consume a replacement's input. Compare
	// the active preview, since runID can advance while its replacement builds.
	if runID != g.playRunID || g.playPaused {
		g.runMu.Unlock()
		return false, nil
	}
	keys := g.playKeys | g.playPendingKeys
	reset := g.playReset
	muted := false
	if playtest != nil {
		var step bool
		keys, reset, step = playtest.state.next(keys, reset)
		if !step {
			g.runMu.Unlock()
			return false, nil
		}
		muted = playtest.state.status().Paused
	}
	g.playReset = false
	if g.playPendingTicks > 0 {
		g.playPendingTicks--
		if g.playPendingTicks == 0 {
			g.playPendingKeys = 0
		}
	}
	g.playLastSentKeys = keys
	g.runMu.Unlock()
	frame, err := session.Step(keys, reset)
	if source, ok := session.(interface{ DrainLogs() []string }); ok {
		g.acceptRuntimeLogs(runID, source.DrainLogs())
	}
	if err != nil {
		return false, err
	}
	if !muted {
		_, _ = pcm.Write(frame.PCM)
	}
	frame.PCM = nil
	if playtest != nil {
		playtest.lastFrame = frame
		playtest.state.stepped(keys)
	}
	publishPreviewFrame(frames, frame)
	return true, nil
}

func publishPreviewFrame(frames chan emulator.Frame, frame emulator.Frame) {
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
}

func (g *Game) acceptRuntimeLogs(runID uint64, lines []string) {
	if len(lines) == 0 {
		return
	}
	g.runMu.Lock()
	defer g.runMu.Unlock()
	if runID != g.runID {
		return
	}
	for _, line := range lines {
		g.runRuntimeLog = appendBoundedLog(g.runRuntimeLog, line)
	}
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
	if g.playAudio != nil && !g.playtest.status.Paused {
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
	g.clearDevelopPlaytest()
	g.runMu.Lock()
	g.playKeys = 0
	g.playPendingKeys = 0
	g.playPendingTicks = 0
	g.playLastSentKeys = 0
	g.playReset = false
	g.playPaused = false
	g.playRunID = 0
	g.playLessonEntry = ""
	g.runMu.Unlock()
	g.playTelemetry = emulator.Telemetry{}
	g.playTelemetrySeq = 0
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

func (g *Game) closePlayMode() {
	if g.romExport.cancel != nil {
		g.romExport.cancel()
	}
	g.stopPlaySession()
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
}

func (g *Game) drawPlayDisplay(screen *ebiten.Image, layout playLayout) {
	scale := float64(layout.Screen.W) / emulator.Width
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(layout.Screen.X), float64(layout.Screen.Y))
	op.GeoM.Scale(editorDrawScale, editorDrawScale)
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(g.playImage, op)
}

func colorCommentMuted() color.Color { return color.RGBA{R: 116, G: 135, B: 123, A: 255} }
