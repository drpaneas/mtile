package main

import (
	"fmt"
	"image/color"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type developPanel uint8

const (
	developProblems developPanel = iota
	developOutput
	developBuild
	developRuntime
	developGenerated
	developPanelCount
)

func (p developPanel) label() string {
	return [...]string{"PROBLEMS", "OUTPUT", "BUILD", "RUNTIME", "GENERATED GO"}[p]
}

var developDiagnosticLocation = regexp.MustCompile(`(?:^|[ (])([^ :()]+\.lua):(\d+):(\d+)`)

// developLayout deliberately gives source and cartridge preview equal vertical
// authority while keeping source wider. The code is the primary authoring
// surface; the live GBA is persistent feedback rather than a destination tab.
type developLayout struct {
	Header     editor.Rect
	SourceMode editor.Rect
	Build      editor.Rect
	Reset      editor.Rect
	Stop       editor.Rect
	Code       codeLayout
	Play       playLayout
}

func newDevelopLayout() developLayout {
	const (
		headerH                  = 72
		sidebarW                 = 228
		padding                  = 12
		bottomH                  = 176
		minimumCodeW             = 600
		previewBezelW            = 76
		previewBezelH            = 86
		previewTitleH            = 30
		previewVerticalClearance = 2
	)
	contentY := editorTabsHeight
	compact := screenWidth < 1400
	resolvedSidebarW := sidebarW
	resolvedMinimumCodeW := minimumCodeW
	if compact {
		resolvedSidebarW = 0
		resolvedMinimumCodeW = 500
	}
	header := editor.Rect{X: 0, Y: contentY, W: screenWidth, H: headerH}
	sidebar := editor.Rect{X: 0, Y: contentY + headerH, W: resolvedSidebarW, H: statusBarRect.Y - contentY - headerH}
	bodyY := contentY + headerH + padding
	mainBottom := statusBarRect.Y - bottomH - padding
	mainH := max(1, mainBottom-bodyY)

	telemetryH := min(250, max(172, mainH*28/100))
	stageH := mainH - telemetryH - padding
	contentW := screenWidth - resolvedSidebarW - padding*3
	// Size the preview from the largest pixel-perfect GBA scale that fits the
	// stage instead of an unrelated percentage. Keeping it within half of the
	// main surface preserves source authority while 12 px of side clearance
	// prevents the bezel from crowding its panel.
	previewLimitW := min(contentW/2, contentW-resolvedMinimumCodeW)
	maxScaleW := max(1, (previewLimitW-previewBezelW-padding*2)/emulator.Width)
	maxScaleH := max(1, (stageH-previewTitleH-previewBezelH-previewVerticalClearance)/emulator.Height)
	previewScale := max(1, min(maxScaleW, maxScaleH))
	previewW := emulator.Width*previewScale + previewBezelW + padding*2
	codeW := contentW - previewW
	codeArea := editor.Rect{X: resolvedSidebarW + padding, Y: bodyY, W: codeW, H: mainH}
	tabs := editor.Rect{X: codeArea.X, Y: codeArea.Y, W: codeArea.W, H: 34}
	codeRect := editor.Rect{X: codeArea.X, Y: tabs.Y + tabs.H, W: codeArea.W, H: codeArea.H - tabs.H}
	gutterWidth := codeCharWidth*4 + 20
	gutter := editor.Rect{X: codeRect.X, Y: codeRect.Y, W: gutterWidth, H: codeRect.H}
	textRect := editor.Rect{X: gutter.X + gutter.W, Y: codeRect.Y, W: codeRect.W - gutter.W, H: codeRect.H}
	previewX := codeArea.X + codeArea.W + padding
	stage := editor.Rect{X: previewX, Y: bodyY, W: previewW, H: stageH}
	telemetry := editor.Rect{X: previewX, Y: stage.Y + stage.H + padding, W: previewW, H: telemetryH}

	screen := editor.Rect{W: emulator.Width * previewScale, H: emulator.Height * previewScale}
	bezel := editor.Rect{W: screen.W + previewBezelW, H: screen.H + previewBezelH}
	bezel.X = stage.X + (stage.W-bezel.W)/2
	bezel.Y = stage.Y + previewTitleH + (stage.H-previewTitleH-bezel.H)/2
	screen.X, screen.Y = bezel.X+38, bezel.Y+38

	fileW := min(480, screenWidth/4)
	fileX := 490
	stopW, resetW, buildW := 124, 124, 164
	if compact {
		fileX = 220
		fileW = min(230, screenWidth/4)
		stopW, resetW, buildW = 82, 82, 132
	}
	fileRect := editor.Rect{X: fileX, Y: contentY + 17, W: fileW, H: 38}
	stop := editor.Rect{X: screenWidth - stopW - 12, Y: contentY + 17, W: stopW, H: 38}
	reset := editor.Rect{X: stop.X - resetW - 8, Y: stop.Y, W: resetW, H: stop.H}
	build := editor.Rect{X: reset.X - buildW - 8, Y: reset.Y, W: buildW, H: reset.H}
	sourceX := fileRect.X + fileRect.W + 8
	sourceMode := editor.Rect{X: sourceX, Y: fileRect.Y, W: max(1, build.X-sourceX-8), H: fileRect.H}
	search := editor.Rect{X: textRect.X + textRect.W - 390, Y: textRect.Y + 8, W: 370, H: 34}
	diagnostics := editor.Rect{X: codeRect.X, Y: mainBottom + padding, W: screenWidth - codeRect.X - padding, H: statusBarRect.Y - mainBottom - padding}
	telemetryColumns := 3
	if compact {
		telemetryColumns = 2
	}
	colW := max(1, telemetry.W/telemetryColumns)
	lcd := editor.Rect{X: telemetry.X + 12, Y: telemetry.Y + telemetry.H - 40, W: colW - 24, H: 28}

	code := codeLayout{
		Header: header, Sidebar: sidebar, File: fileRect,
		Tabs: tabs, Editor: codeRect, Gutter: gutter, Text: textRect,
		Diagnostics: diagnostics, Search: search,
	}
	play := playLayout{
		Header: header, Reload: build, Reset: reset, Stop: stop,
		LCD: lcd, Stage: stage, Bezel: bezel, Screen: screen, Inspector: telemetry,
	}
	return developLayout{Header: header, SourceMode: sourceMode, Build: build, Reset: reset, Stop: stop, Code: code, Play: play}
}

func (g *Game) updateDevelopMode() {
	if g.code == nil {
		return
	}
	g.pumpPlaySession()
	layout := newDevelopLayout()
	mx, my := ebiten.CursorPosition()
	clicked := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	if clicked {
		switch {
		case g.updateDevelopPanelClick(layout, mx, my):
			g.releaseDevelopGameInput()
		case rectContains(layout.Build, mx, my):
			g.startRun()
		case rectContains(layout.Reset, mx, my):
			g.runMu.Lock()
			g.playReset = true
			g.runMu.Unlock()
		case rectContains(layout.Stop, mx, my):
			g.releaseDevelopGameInput()
			g.stopPlaySession()
		case rectContains(layout.Play.LCD, mx, my):
			g.theme.playLCD = !g.theme.playLCD
			g.preferences.LCDPreview = g.theme.playLCD
			g.savePreferences()
			g.statusMsg = map[bool]string{false: "clean display", true: "LCD display"}[g.theme.playLCD]
		case rectContains(layout.Play.Screen, mx, my):
			g.developGameFocus = true
			g.statusMsg = "game input active; press Esc to return to code"
		default:
			g.releaseDevelopGameInput()
		}
	}
	if rectContains(layout.Code.Diagnostics, mx, my) {
		_, wheelY := ebiten.Wheel()
		if wheelY != 0 {
			lines := g.developPanelLines(g.developPanel)
			visible := max(1, (layout.Code.Diagnostics.H-45)/codeLineHeight)
			g.developScroll[g.developPanel] = min(max(0, g.developScroll[g.developPanel]-int(wheelY*3)), max(0, len(lines)-visible))
		}
	}

	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	if mod && ebiten.IsKeyPressed(ebiten.KeyShift) && inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.reloadActiveCodeDocument()
		return
	}
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.startRun()
		return
	}
	if g.developGameFocus {
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			g.releaseDevelopGameInput()
			g.statusMsg = "code input active"
			return
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
		return
	}
	g.updateCodeWorkspace(layout.Code)
}

func (g *Game) releaseDevelopGameInput() {
	g.developGameFocus = false
	g.runMu.Lock()
	g.playKeys = 0
	g.playPendingKeys = 0
	g.playPendingTicks = 0
	g.runMu.Unlock()
}

func (g *Game) drawDevelopMode(screen *ebiten.Image) {
	layout := newDevelopLayout()
	fillRect(screen, editor.Rect{X: 0, Y: editorTabsHeight, W: screenWidth, H: statusBarRect.Y - editorTabsHeight}, colorInk)
	g.drawDevelopHeader(screen, layout)
	if layout.Code.Sidebar.W > 0 {
		g.drawCodeSidebar(screen, layout.Code)
	}
	g.drawCodeTabs(screen, layout.Code)
	g.drawCodeDocument(screen, layout.Code)
	g.drawDevelopPreview(screen, layout)
	g.drawDevelopTelemetry(screen, layout)
	g.drawDevelopDiagnostics(screen, layout)
	g.drawCodeOverlays(screen, layout.Code)
}

func (g *Game) drawDevelopHeader(screen *ebiten.Image, layout developLayout) {
	drawWorkspaceHeader(screen, layout.Header, "DEVELOP", "CODE + LIVE CARTRIDGE", colorCode)
	fillRect(screen, layout.Code.File, colorBackground)
	strokeRect(screen, layout.Code.File, colorBorder)
	name := filepath.Base(g.codePath)
	if g.code.Dirty() {
		name += "  *"
	}
	fileLabel := truncateUIText("ACTIVE FILE  ·  "+name, layout.Code.File.W-24)
	drawText(screen, fileLabel, layout.Code.File.X+12, layout.Code.File.Y+(layout.Code.File.H-uiTextHeight)/2, colorText)
	// This is build-pipeline status, not a selectable input. A compact badge
	// keeps it visually distinct from the editable ACTIVE FILE field.
	fillRoundedRect(screen, layout.SourceMode, 6, color.RGBA{R: 25, G: 31, B: 51, A: 255})
	strokeRoundedRect(screen, layout.SourceMode, 6, rgba(colorCode, 150))
	drawSmallTextCentered(screen, truncateTextToWidth("PIPELINE  ·  LUA TO GO", layout.SourceMode.W-12, uiSmallFace), layout.SourceMode, colorLCD)

	g.runMu.Lock()
	busy := g.runBusy
	g.runMu.Unlock()
	drawNativeButton(screen, layout.Build, "BUILD + RELOAD", busy, colorRun)
	drawNativeButton(screen, layout.Reset, "RESET", false, colorHighlight)
	drawNativeButton(screen, layout.Stop, "STOP", false, colorDanger)
}

func (g *Game) drawDevelopPreview(screen *ebiten.Image, layout developLayout) {
	fillRect(screen, layout.Play.Stage, colorBackground)
	border := colorBorder
	label := "CLICK SCREEN TO PLAY"
	if g.developGameFocus {
		border = colorRun
		label = "GAME INPUT ACTIVE  /  ESC RELEASE"
	}
	strokeRect(screen, layout.Play.Stage, border)
	left := truncateTextToWidth("GBA PREVIEW  /  mGBA CORE", layout.Play.Stage.W/2-20, uiSmallFace)
	right := truncateTextToWidth(label, layout.Play.Stage.W/2-20, uiSmallFace)
	drawSmallText(screen, left, layout.Play.Stage.X+12, layout.Play.Stage.Y+10, colorCode)
	drawSmallText(screen, right, layout.Play.Stage.X+layout.Play.Stage.W-12-smallTextWidth(right), layout.Play.Stage.Y+10, border)
	g.drawGBABezel(screen, layout.Play)
	g.drawDevelopBuildNotice(screen, layout)
	if g.developGameFocus {
		strokeRoundedRect(screen, editor.Rect{X: layout.Play.Screen.X - 12, Y: layout.Play.Screen.Y - 12, W: layout.Play.Screen.W + 24, H: layout.Play.Screen.H + 24}, 5, rgba(colorRun, 210))
	}
}

func (g *Game) developBuildNotice() (string, color.Color) {
	g.runMu.Lock()
	busy, status, runError := g.runBusy, g.runStatus, g.runError
	events := append([]build.Event(nil), g.runBuildEvents...)
	g.runMu.Unlock()
	if busy {
		message := "PREPARING CARTRIDGE"
		if len(events) != 0 {
			last := events[len(events)-1]
			message = strings.ToUpper(string(last.Phase)) + "  /  " + last.Message
		}
		return "BUILDING  /  " + message, colorHighlight
	}
	if runError != "" {
		if status == "PLAYER FAILED" || strings.Contains(status, "AUDIO") {
			return "PREVIEW FAILED  /  SEE BUILD OR RUNTIME LOG", colorDanger
		}
		return "BUILD FAILED  /  SEE BUILD LOG BELOW", colorDanger
	}
	switch status {
	case "RUNNING ON mGBA CORE":
		return "BUILD COMPLETE  /  LIVE PREVIEW RUNNING", colorRun
	case "BUILD BLOCKED":
		return "BUILD BLOCKED  /  SAVE SOURCE FILES", colorHighlight
	case "PLAYER STOPPED":
		return "PREVIEW STOPPED  /  BUILD + RELOAD TO START", colorDim
	default:
		return "READY  /  BUILD + RELOAD TO START", colorDim
	}
}

func (g *Game) drawDevelopBuildNotice(screen *ebiten.Image, layout developLayout) {
	label, accent := g.developBuildNotice()
	label = truncateUIText(label, layout.Play.Bezel.W-42)
	w := min(layout.Play.Bezel.W-28, uiTextWidth(label)+28)
	r := editor.Rect{X: layout.Play.Bezel.X + (layout.Play.Bezel.W-w)/2, Y: layout.Play.Bezel.Y + 8, W: w, H: uiTextHeight + 10}
	fillRoundedRect(screen, r, 5, color.RGBA{R: 11, G: 12, B: 25, A: 238})
	strokeRoundedRect(screen, r, 5, rgba(accent, 190))
	drawTextCentered(screen, label, r, accent)
}

func (g *Game) drawDevelopTelemetry(screen *ebiten.Image, layout developLayout) {
	r := layout.Play.Inspector
	fillRect(screen, r, colorPanel)
	strokeRect(screen, r, colorBorder)
	if r.W < 480 {
		g.drawDevelopCompactTelemetry(screen, layout)
		return
	}
	colW := max(1, r.W/3)
	columnText := func(text string) string {
		return truncateMusicEditText(text, max(1, (colW-24)/codeCharWidth))
	}
	for column := 1; column < 3; column++ {
		x := r.X + column*colW
		drawLine(screen, x, r.Y+10, x, r.Y+r.H-10, colorBorder)
	}

	x := r.X + 12
	drawText(screen, columnText("CARTRIDGE"), x, r.Y+14, colorDim)
	drawText(screen, columnText(filepath.Base(g.model.Dir)), x, r.Y+40, colorText)
	drawText(screen, columnText("240 x 160  /  59.73 Hz"), x, r.Y+66, colorLCD)
	drawText(screen, columnText(fmt.Sprintf("PREVIEW  %.1f FPS", g.playFPS)), x, r.Y+92, colorText)
	g.runMu.Lock()
	romSize := g.runROMSize
	inputKeys := g.playKeys | g.playPendingKeys
	g.runMu.Unlock()
	if romSize != 0 {
		drawText(screen, columnText("ROM  "+formatDevelopBytes(uint64(romSize))), x, r.Y+118, colorText)
	}
	if r.H >= 205 {
		drawText(screen, columnText("INPUT  "+playKeyLabel(inputKeys)), x, r.Y+146, playInputColor(inputKeys))
	}
	drawNativeButton(screen, layout.Play.LCD, map[bool]string{false: "CLEAN", true: "LCD ON"}[g.theme.playLCD], g.theme.playLCD, colorCode)

	x = r.X + colW + 12
	drawText(screen, columnText("GAME BOY PSG  /  HW"), x, r.Y+14, colorDim)
	if !g.playTelemetry.Valid {
		drawText(screen, columnText("Waiting for mGBA telemetry"), x, r.Y+44, colorDim)
	} else {
		registers := [...]uint16{g.playTelemetry.PSG.CH1Frequency, g.playTelemetry.PSG.CH2Frequency, g.playTelemetry.PSG.CH3Frequency, g.playTelemetry.PSG.CH4Frequency}
		for channel := 0; channel < 4; channel++ {
			active := g.playTelemetry.PSG.Status&(1<<channel) != 0
			state := "OFF"
			clr := colorDim
			if active {
				state = "ON "
				clr = nativeChannelColors[channel]
			}
			drawText(screen, columnText(fmt.Sprintf("CH%d  %s  REG %04X", channel+1, state, registers[channel])), x, r.Y+42+channel*24, clr)
		}
		drawText(screen, columnText(fmt.Sprintf("MIX %04X  STATUS %04X", g.playTelemetry.PSG.MixPSG, g.playTelemetry.PSG.Status)), x, r.Y+142, colorLCD)
	}
	if r.H >= 220 && g.playPCM != nil {
		queuedMS, dropouts := g.developHostAudioStatus()
		label := formatDevelopHostAudio(queuedMS, dropouts)
		clr := colorDim
		if dropouts > 0 {
			clr = colorHighlight
		}
		drawText(screen, columnText("HOST AUDIO"), x, r.Y+r.H-54, colorDim)
		drawText(screen, columnText(label), x, r.Y+r.H-30, clr)
	}

	x = r.X + colW*2 + 12
	drawText(screen, columnText("DEBUG / INSPECTOR"), x, r.Y+14, colorDim)
	g.runMu.Lock()
	ewramUsed, ewramFree, iwramUsed := g.runEWRAMUsed, g.runEWRAMFree, g.runIWRAMUsed
	g.runMu.Unlock()
	if !g.playTelemetry.DebugValid {
		drawText(screen, columnText("CPU  waiting for live preview"), x, r.Y+40, colorDim)
		if ewramUsed != 0 {
			drawText(screen, columnText(developEWRAMLabel(ewramUsed)), x, r.Y+64, colorText)
			drawText(screen, columnText(developMemoryHeadroomLabel(ewramFree, iwramUsed)), x, r.Y+88, colorLCD)
		} else {
			drawText(screen, columnText("RAM  available after build"), x, r.Y+64, colorDim)
		}
		return
	}
	stats := g.playTelemetry.Stats
	load := float64(stats.LastFrame) / 561792.0
	loadColor := colorRun
	if load >= .90 {
		loadColor = colorDanger
	} else if load >= .70 {
		loadColor = colorHighlight
	}
	drawText(screen, columnText(fmt.Sprintf("CPU  %.1f%%  %s cyc", load*100, formatDevelopCount(stats.LastFrame))), x, r.Y+40, loadColor)
	if ewramUsed != 0 {
		drawText(screen, columnText(developEWRAMLabel(ewramUsed)), x, r.Y+64, colorText)
	} else {
		drawText(screen, columnText("EWRAM  build data unavailable"), x, r.Y+64, colorDim)
	}
	compact := r.H < 220
	frameY, watchY := r.Y+112, r.Y+164
	if compact {
		frameY, watchY = r.Y+88, r.Y+116
	} else {
		drawText(screen, columnText(developMemoryHeadroomLabel(ewramFree, iwramUsed)), x, r.Y+88, colorLCD)
	}
	drawText(screen, columnText(fmt.Sprintf("FRAME %d  MISSED %d", stats.Frames, stats.MissedFrames)), x, frameY, colorText)
	if !compact {
		drawText(screen, columnText(fmt.Sprintf("UPDATE %s  DRAW %s", formatDevelopCount(stats.LastUpdate), formatDevelopCount(stats.LastDraw))), x, r.Y+136, colorText)
	}
	drawText(screen, columnText("WATCH / VARIABLES"), x, watchY, colorDim)
	watchRows := min(1, len(g.playTelemetry.Watches))
	for row := 0; row < watchRows; row++ {
		watch := g.playTelemetry.Watches[row]
		label := fmt.Sprintf("%s = %s", watch.Name, developWatchValue(watch))
		drawText(screen, columnText(label), x, watchY+24+row*22, colorText)
	}
	if watchRows == 0 {
		drawText(screen, columnText("No top-level globals"), x, watchY+24, colorDim)
	}
	if r.H >= 238 {
		g.drawDevelopCPUGraph(screen, editor.Rect{X: x, Y: r.Y + r.H - 42, W: colW - 24, H: 28})
	}
}

func (g *Game) drawDevelopCompactTelemetry(screen *ebiten.Image, layout developLayout) {
	r := layout.Play.Inspector
	colW := max(1, r.W/2)
	drawLine(screen, r.X+colW, r.Y+10, r.X+colW, r.Y+r.H-10, colorBorder)
	cell := func(column, row int, value string, clr color.Color) {
		x := r.X + column*colW + 12
		y := r.Y + 12 + row*22
		drawSmallText(screen, truncateTextToWidth(value, colW-24, uiSmallFace), x, y, clr)
	}

	g.runMu.Lock()
	romSize := g.runROMSize
	inputKeys := g.playKeys | g.playPendingKeys
	ewramUsed := g.runEWRAMUsed
	g.runMu.Unlock()
	cell(0, 0, "CARTRIDGE", colorDim)
	cell(0, 1, filepath.Base(g.model.Dir), colorText)
	cell(0, 2, "240 x 160  ·  59.73 Hz", colorLCD)
	cell(0, 3, fmt.Sprintf("PREVIEW  %.1f FPS", g.playFPS), colorText)
	if romSize != 0 {
		cell(0, 4, "ROM  "+formatDevelopBytes(uint64(romSize)), colorText)
	} else {
		cell(0, 4, "ROM  BUILD TO LOAD", colorDim)
	}
	if r.H >= 195 {
		cell(0, 5, "INPUT  "+playKeyLabel(inputKeys), playInputColor(inputKeys))
	}
	drawNativeButton(screen, layout.Play.LCD, map[bool]string{false: "CLEAN", true: "LCD ON"}[g.theme.playLCD], g.theme.playLCD, colorCode)

	cell(1, 0, "RUNTIME", colorDim)
	if !g.playTelemetry.Valid {
		cell(1, 1, "mGBA  ·  WAITING", colorDim)
		cell(1, 2, "PSG  WAITING", colorDim)
		cell(1, 3, "CPU  WAITING", colorDim)
		if ewramUsed != 0 {
			cell(1, 4, developEWRAMLabel(ewramUsed), colorText)
		} else {
			cell(1, 4, "RAM  AFTER BUILD", colorDim)
		}
		return
	}
	activeChannels := 0
	for channel := 0; channel < 4; channel++ {
		if g.playTelemetry.PSG.Status&(1<<channel) != 0 {
			activeChannels++
		}
	}
	cell(1, 1, fmt.Sprintf("PSG  %d/4 CHANNELS", activeChannels), colorLCD)
	if !g.playTelemetry.DebugValid {
		cell(1, 2, "CPU  WAITING", colorDim)
		return
	}
	stats := g.playTelemetry.Stats
	load := float64(stats.LastFrame) / 561792.0
	loadColor := colorRun
	if load >= .90 {
		loadColor = colorDanger
	} else if load >= .70 {
		loadColor = colorHighlight
	}
	cell(1, 2, fmt.Sprintf("CPU  %.1f%%  %s CYC", load*100, formatDevelopCount(stats.LastFrame)), loadColor)
	if ewramUsed != 0 {
		cell(1, 3, developEWRAMLabel(ewramUsed), colorText)
	}
	cell(1, 4, fmt.Sprintf("FRAME %d  MISSED %d", stats.Frames, stats.MissedFrames), colorText)
	if r.H >= 195 {
		cell(1, 5, "WATCH  "+fmt.Sprint(len(g.playTelemetry.Watches)), colorDim)
	}
}

func (g *Game) developHostAudioStatus() (queuedMilliseconds int, dropouts uint64) {
	if g == nil || g.playPCM == nil {
		return 0, 0
	}
	queued, underruns, overruns := g.playPCM.Stats()
	queuedMilliseconds = queued * 1000 / emulator.SampleRate
	if !g.playAudioBaselineReady {
		return queuedMilliseconds, 0
	}
	if underruns >= g.playAudioBaselineUnderruns {
		dropouts += underruns - g.playAudioBaselineUnderruns
	}
	if overruns >= g.playAudioBaselineOverruns {
		dropouts += overruns - g.playAudioBaselineOverruns
	}
	return queuedMilliseconds, dropouts
}

func formatDevelopCount(value uint32) string {
	switch {
	case value >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(value)/1_000_000)
	case value >= 1_000:
		return fmt.Sprintf("%.1fK", float64(value)/1_000)
	default:
		return strconv.FormatUint(uint64(value), 10)
	}
}

func formatDevelopHostAudio(queuedMilliseconds int, dropouts uint64) string {
	if dropouts > 0 {
		return fmt.Sprintf("%dms  /  DROPS %d", queuedMilliseconds, dropouts)
	}
	return fmt.Sprintf("%dms QUEUED", queuedMilliseconds)
}

func formatDevelopBytes(value uint64) string {
	const kib = 1024
	if value < kib {
		return fmt.Sprintf("%d B", value)
	}
	if value%kib == 0 {
		return fmt.Sprintf("%d KiB", value/kib)
	}
	return fmt.Sprintf("%.1f KiB", float64(value)/kib)
}

func developEWRAMLabel(used uint64) string {
	percent := float64(used) * 100 / float64(build.EWRAMCapacity)
	return fmt.Sprintf("EWRAM %s/%sK  %.0f%%", formatDevelopKiB(used), formatDevelopKiB(build.EWRAMCapacity), percent)
}

func developMemoryHeadroomLabel(free, iwramUsed uint64) string {
	if iwramUsed == 0 {
		return "FREE " + formatDevelopKiB(free) + "K"
	}
	return fmt.Sprintf("FREE %sK  IWRAM %sK", formatDevelopKiB(free), formatDevelopKiB(iwramUsed))
}

func formatDevelopKiB(value uint64) string {
	if value%1024 == 0 {
		return strconv.FormatUint(value/1024, 10)
	}
	return fmt.Sprintf("%.1f", float64(value)/1024)
}

func developWatchValue(watch emulator.Watch) string {
	switch watch.Kind {
	case 0:
		return "nil"
	case 1:
		if watch.Raw != 0 {
			return "true"
		}
		return "false"
	case 2:
		value := float64(watch.Raw) / 65536.0
		if value == float64(int(value)) {
			return strconv.Itoa(int(value))
		}
		return fmt.Sprintf("%.3f", value)
	case 3, 5:
		return "<string>"
	case 4:
		return "<table>"
	default:
		return fmt.Sprintf("<kind %d>", watch.Kind)
	}
}

func (g *Game) drawDevelopCPUGraph(screen *ebiten.Image, r editor.Rect) {
	fillRect(screen, r, colorBackground)
	strokeRect(screen, r, colorBorder)
	if g.playCPUSamples < 2 {
		return
	}
	start := len(g.playCPULoad) - g.playCPUSamples
	for index := start + 1; index < len(g.playCPULoad); index++ {
		x0 := r.X + (index-start-1)*(r.W-2)/max(1, g.playCPUSamples-1)
		x1 := r.X + (index-start)*(r.W-2)/max(1, g.playCPUSamples-1)
		y0 := r.Y + r.H - 2 - int(min(1.0, g.playCPULoad[index-1])*float64(r.H-4))
		y1 := r.Y + r.H - 2 - int(min(1.0, g.playCPULoad[index])*float64(r.H-4))
		drawLine(screen, x0+1, y0, x1+1, y1, colorRun)
	}
}

func (g *Game) drawDevelopDiagnostics(screen *ebiten.Image, layout developLayout) {
	r := layout.Code.Diagnostics
	fillRect(screen, r, colorPanel)
	strokeRect(screen, r, colorBorder)
	for panel, tab := range g.developPanelTabRects(layout) {
		active := developPanel(panel) == g.developPanel
		if active {
			fillRect(screen, tab, colorBackground)
			fillRect(screen, editor.Rect{X: tab.X, Y: tab.Y + tab.H - 3, W: tab.W, H: 3}, colorCode)
		}
		label := developPanel(panel).label()
		clr := colorDim
		if active {
			clr = colorText
		}
		if panel == int(developProblems) && g.hasDevelopProblems() {
			clr = colorDanger
			label += " !"
		}
		drawTextCentered(screen, label, tab, clr)
	}
	drawLine(screen, r.X+8, r.Y+34, r.X+r.W-8, r.Y+34, colorBorder)
	lines := g.developPanelLines(g.developPanel)
	visible := max(1, (r.H-45)/codeLineHeight)
	start := min(g.developScroll[g.developPanel], max(0, len(lines)-visible))
	g.developScroll[g.developPanel] = start
	for row := 0; row < visible && start+row < len(lines); row++ {
		line := lines[start+row]
		y := r.Y + 40 + row*codeLineHeight
		clr := colorText
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
			clr = colorDanger
		} else if strings.HasPrefix(line, "OK") || strings.Contains(lower, "completed successfully") {
			clr = colorRun
		} else if strings.HasPrefix(line, "[") {
			clr = colorLCD
		}
		if g.developPanel == developGenerated {
			x := r.X + 56
			drawCodeText(screen, fmt.Sprintf("%4d", start+row+1), r.X+12, y, colorDim)
			for _, segment := range highlightGoLine(truncateMusicEditText(line, max(1, (r.W-70)/codeCharWidth))) {
				drawCodeText(screen, segment.text, x, y, segment.color)
				x += len([]rune(segment.text)) * codeCharWidth
			}
		} else {
			drawCodeText(screen, truncateCodeText(line, r.W-30), r.X+14, y, clr)
		}
	}
}

func (g *Game) developPanelTabRects(layout developLayout) []editor.Rect {
	r := layout.Code.Diagnostics
	widths := [...]int{112, 96, 92, 106, 150}
	tabs := make([]editor.Rect, 0, len(widths))
	x := r.X + 1
	for _, width := range widths {
		tabs = append(tabs, editor.Rect{X: x, Y: r.Y + 1, W: width, H: 33})
		x += width
	}
	return tabs
}

func (g *Game) updateDevelopPanelClick(layout developLayout, x, y int) bool {
	r := layout.Code.Diagnostics
	if !rectContains(r, x, y) {
		return false
	}
	for panel, tab := range g.developPanelTabRects(layout) {
		if rectContains(tab, x, y) {
			g.developPanel = developPanel(panel)
			return true
		}
	}
	if g.developPanel == developProblems && y >= r.Y+34 {
		g.openDevelopDiagnostic()
	}
	return true
}

func (g *Game) hasDevelopProblems() bool {
	g.runMu.Lock()
	runError := g.runError
	g.runMu.Unlock()
	return g.codeDiagnosticLine != 0 || runError != ""
}

func (g *Game) developPanelLines(panel developPanel) []string {
	g.runMu.Lock()
	status, runError := g.runStatus, g.runError
	romPath, romSize := g.runROMPath, g.runROMSize
	ewramUsed, ewramFree, iwramUsed := g.runEWRAMUsed, g.runEWRAMFree, g.runIWRAMUsed
	entry := g.runEntry
	sources := append([]string(nil), g.runSourceFiles...)
	generated := g.runGeneratedGo
	events := append([]build.Event(nil), g.runBuildEvents...)
	runtimeLog := append([]string(nil), g.runRuntimeLog...)
	g.runMu.Unlock()
	switch panel {
	case developProblems:
		var lines []string
		if g.codeDiagnosticLine != 0 {
			name := filepath.Base(g.codePath)
			if document := g.codeDocuments[g.codePath]; document != nil {
				name = document.Relative
			}
			lines = append(lines, fmt.Sprintf("%s:%d:%d  %s", name, g.codeDiagnosticLine, g.codeDiagnosticColumn, g.codeDiagnostic))
		}
		if runError != "" {
			lines = append(lines, runError)
		}
		if len(lines) == 0 {
			lines = append(lines, "OK  No source or build problems detected.")
		}
		return lines
	case developOutput:
		if status == "" {
			status = "READY TO BUILD"
		}
		lines := []string{"STATUS  " + status}
		if entry != "" {
			lines = append(lines, "ENTRY   "+entry, fmt.Sprintf("SOURCES %d  %s", len(sources), strings.Join(sources, ", ")))
		}
		if romPath != "" {
			lines = append(lines,
				fmt.Sprintf("ROM     %s  /  %s", filepath.Base(romPath), formatDevelopBytes(uint64(romSize))),
				fmt.Sprintf("EWRAM   %s used  /  %s free  /  %s total", formatDevelopBytes(ewramUsed), formatDevelopBytes(ewramFree), formatDevelopBytes(build.EWRAMCapacity)),
				fmt.Sprintf("IWRAM   %s used  /  %s total", formatDevelopBytes(iwramUsed), formatDevelopBytes(build.IWRAMCapacity)),
			)
		}
		lines = append(lines,
			fmt.Sprintf("SPRITES %d  /  MAP %d x %d", g.spriteN(), g.model.Map.Width, g.model.Map.Height),
			"AUDIO   4 Game Boy PSG channels  /  PCM OFF",
		)
		return lines
	case developBuild:
		if len(events) == 0 && runError == "" {
			return []string{"Build log is empty. Press Build + Reload."}
		}
		lines := make([]string, 0, len(events)+1)
		for _, event := range events {
			lines = append(lines, fmt.Sprintf("[%s] %s", event.Phase, event.Message))
		}
		if runError != "" {
			lines = append(lines, "ERROR  "+runError)
		}
		return lines
	case developRuntime:
		if len(runtimeLog) == 0 {
			return []string{"Runtime log is empty. Start the cartridge preview."}
		}
		return runtimeLog
	case developGenerated:
		if generated == "" {
			return []string{"Generated Go is available after a successful build."}
		}
		return strings.Split(generated, "\n")
	default:
		return nil
	}
}

func (g *Game) openDevelopDiagnostic() {
	if g.codeDiagnosticLine != 0 {
		g.code.SetCursor(g.code.Offset(max(0, g.codeDiagnosticLine-1), max(0, g.codeDiagnosticColumn-1)), false)
		g.keepCodeCursorVisible(newDevelopLayout().Code)
		return
	}
	g.runMu.Lock()
	message := g.runError
	g.runMu.Unlock()
	match := developDiagnosticLocation.FindStringSubmatch(message)
	if match == nil {
		return
	}
	line, _ := strconv.Atoi(match[2])
	column, _ := strconv.Atoi(match[3])
	path := match[1]
	if !filepath.IsAbs(path) {
		path = filepath.Join(g.model.Dir, filepath.FromSlash(path))
	}
	if err := g.openCodeDocument(path); err != nil {
		g.statusMsg = "open diagnostic: " + err.Error()
		return
	}
	g.code.SetCursor(g.code.Offset(max(0, line-1), max(0, column-1)), false)
	g.keepCodeCursorVisible(newDevelopLayout().Code)
}
