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
	developWatches
	developMemory
	developGenerated
	developPanelCount
)

func (p developPanel) label() string {
	return [...]string{"PROBLEMS", "OUTPUT", "BUILD", "RUNTIME", "WATCHES", "MEMORY", "GENERATED GO"}[p]
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
	Message    editor.Rect
	Code       codeLayout
	Play       playLayout
}

func newDevelopLayout() developLayout {
	return newDevelopPanelLayout(developProblems)
}

func newDevelopPanelLayout(panel developPanel) developLayout {
	const (
		headerH                  = 72
		sidebarW                 = 228
		padding                  = 12
		minimumCodeW             = 600
		previewBezelW            = 76
		previewBezelH            = 86
		previewTitleH            = 30
		previewVerticalClearance = 2
	)
	bottomH := 176
	if panel == developMemory {
		// Give the comparison room without covering the running cartridge.
		// Compact windows retain enough stage height for a 1x GBA preview.
		bottomH = min(336, 176+max(0, screenHeight-840))
	}
	contentY := editorTabsHeight
	compact := screenWidth < 1400
	resolvedSidebarW := sidebarW
	resolvedMinimumCodeW := minimumCodeW
	if compact {
		resolvedSidebarW = 0
		resolvedMinimumCodeW = 500
	}
	header := editor.Rect{X: 0, Y: contentY, W: screenWidth, H: headerH}
	// Reserve the message line even when empty, so notifications cannot
	// overlap the sidebar hint or diagnostics, or move the workspace around.
	messageH := developTextHeight + 12
	message := editor.Rect{X: 0, Y: statusBarRect.Y - messageH, W: screenWidth, H: messageH}
	sidebar := editor.Rect{X: 0, Y: contentY + headerH, W: resolvedSidebarW, H: message.Y - contentY - headerH}
	bodyY := contentY + headerH + padding
	// Take message space from diagnostics, preserving the source viewport
	// and the cartridge preview's integer scale.
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
	gutter, textRect, horizontal, vertical := codeViewportLayout(codeRect)
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
		fileX = 420
		fileW = min(140, screenWidth/4)
		stopW, resetW, buildW = 82, 82, 132
	}
	fileRect := editor.Rect{X: fileX, Y: contentY + 17, W: fileW, H: 38}
	stop := editor.Rect{X: screenWidth - stopW - 12, Y: contentY + 17, W: stopW, H: 38}
	reset := editor.Rect{X: stop.X - resetW - 8, Y: stop.Y, W: resetW, H: stop.H}
	build := editor.Rect{X: reset.X - buildW - 8, Y: reset.Y, W: buildW, H: reset.H}
	sourceX := fileRect.X + fileRect.W + 8
	sourceMode := editor.Rect{X: sourceX, Y: fileRect.Y, W: max(1, build.X-sourceX-8), H: fileRect.H}
	search := editor.Rect{X: textRect.X + textRect.W - 390, Y: textRect.Y + 8, W: 370, H: 34}
	diagnostics := editor.Rect{X: codeRect.X, Y: mainBottom + padding, W: screenWidth - codeRect.X - padding, H: message.Y - mainBottom - padding*2}
	code := codeLayout{
		Header: header, Sidebar: sidebar, File: fileRect,
		Tabs: tabs, Editor: codeRect, Gutter: gutter, Text: textRect, Horizontal: horizontal, Vertical: vertical, Format: codeFormatRect(tabs),
		Diagnostics: diagnostics, Search: search,
	}
	play := playLayout{
		Header: header, Reload: build, Reset: reset, Stop: stop,
		Stage: stage, Bezel: bezel, Screen: screen, Inspector: telemetry,
	}
	return developLayout{Header: header, SourceMode: sourceMode, Build: build, Reset: reset, Stop: stop, Message: message, Code: code, Play: play}
}

func (g *Game) updateDevelopMode() {
	if g.code == nil {
		return
	}
	g.pumpPlaySession()
	layout := newDevelopPanelLayout(g.developPanel)
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
			if g.developPanel == developMemory {
				g.scrollDevelopMemory(layout, -int(wheelY*3))
			} else {
				lines := g.developPanelLines(g.developPanel)
				visible := max(1, (layout.Code.Diagnostics.H-45)/codeLineHeight)
				g.developScroll[g.developPanel] = min(max(0, g.developScroll[g.developPanel]-int(wheelY*3)), max(0, len(lines)-visible))
			}
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
	layout := newDevelopPanelLayout(g.developPanel)
	fillRect(screen, editor.Rect{X: 0, Y: editorTabsHeight, W: screenWidth, H: statusBarRect.Y - editorTabsHeight}, colorInk)
	// Paint this before the adjacent panels: their border strokes straddle
	// the shared edge and must remain visible above the message background.
	fillRect(screen, layout.Message, colorPanel)
	// Close the dark workspace below its padded panes, joining the sidebar
	// separator without drawing across the separate message row.
	fillRect(screen, editor.Rect{X: layout.Code.Sidebar.W, Y: layout.Message.Y - 1, W: screenWidth - layout.Code.Sidebar.W, H: 1}, colorBorder)
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
	drawDevelopWorkspaceHeader(screen, layout.Header, "DEVELOP", "CODE + LIVE CARTRIDGE", colorCode)
	fillRect(screen, layout.Code.File, colorBackground)
	strokeRect(screen, layout.Code.File, colorSectionBorder)
	name := filepath.Base(g.codePath)
	if g.code.Dirty() {
		name += "  *"
	}
	fileLabel := truncateDevelopText("ACTIVE FILE  ·  "+name, layout.Code.File.W-24)
	drawDevelopText(screen, fileLabel, layout.Code.File.X+12, layout.Code.File.Y+(layout.Code.File.H-developTextHeight)/2, colorText)
	// This is build-pipeline status, not a selectable input. A compact badge
	// keeps it visually distinct from the editable ACTIVE FILE field.
	g.runMu.Lock()
	globalUsage := cloneGlobalUsage(g.runGlobalUsage)
	g.runMu.Unlock()
	globalLabel := "GLOBALS  -- / 256"
	if globalUsage.Limit != 0 {
		globalLabel = fmt.Sprintf("GLOBALS  %d / %d  ·  %s", globalUsage.Count, globalUsage.Limit, developGlobalUsageState(globalUsage))
	}
	fillRoundedRect(screen, layout.SourceMode, 6, color.RGBA{R: 25, G: 31, B: 51, A: 255})
	globalColor := developGlobalUsageColor(globalUsage)
	if globalUsage.Limit != 0 && globalUsage.Count >= globalUsage.Limit {
		strokeRoundedRect(screen, layout.SourceMode, 6, globalColor)
	}
	drawDevelopSmallTextCentered(screen, truncateTextToWidth(globalLabel, layout.SourceMode.W-12, developSmallFace), layout.SourceMode, globalColor)

	g.runMu.Lock()
	busy := g.runBusy
	g.runMu.Unlock()
	buildLabel := "BUILD + RELOAD"
	if busy {
		buildLabel = "BUILDING..."
	}
	drawDevelopButton(screen, layout.Build, buildLabel, true, colorRun)
	drawDevelopButton(screen, layout.Reset, "RESET", false, colorSelection)
	drawDevelopButton(screen, layout.Stop, "STOP", false, colorDanger)
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
	left := truncateTextToWidth("GBA PREVIEW  /  mGBA CORE", layout.Play.Stage.W/2-20, developSmallFace)
	right := truncateTextToWidth(label, layout.Play.Stage.W/2-20, developSmallFace)
	drawDevelopSmallText(screen, left, layout.Play.Stage.X+12, layout.Play.Stage.Y+10, colorCode)
	drawDevelopSmallText(screen, right, layout.Play.Stage.X+layout.Play.Stage.W-12-developSmallTextWidth(right), layout.Play.Stage.Y+10, border)
	g.drawDevelopPreviewStatus(screen, layout, left, right)
	g.drawGBABezel(screen, layout.Play)
	if g.developGameFocus {
		strokeRoundedRect(screen, editor.Rect{X: layout.Play.Screen.X - 12, Y: layout.Play.Screen.Y - 12, W: layout.Play.Screen.W + 24, H: layout.Play.Screen.H + 24}, 5, rgba(colorRun, 210))
	}
}

func (g *Game) developPreviewStatus() (string, color.Color) {
	g.runMu.Lock()
	busy, status, runError := g.runBusy, g.runStatus, g.runError
	g.runMu.Unlock()
	if busy {
		return "BUILDING", colorHighlight
	}
	if runError != "" {
		return "FAILED", colorDanger
	}
	switch status {
	case "RUNNING ON mGBA CORE":
		return "LIVE", colorRun
	case "BUILD BLOCKED":
		return "BLOCKED", colorHighlight
	case "PLAYER STOPPED":
		return "STOPPED", colorDim
	default:
		return "READY", colorDim
	}
}

func developPreviewStatusRect(layout developLayout, left, right string) editor.Rect {
	leftEdge := layout.Play.Stage.X + 28 + developSmallTextWidth(left)
	rightEdge := layout.Play.Stage.X + layout.Play.Stage.W - 28 - developSmallTextWidth(right)
	return editor.Rect{X: leftEdge, Y: layout.Play.Stage.Y + 5, W: max(0, rightEdge-leftEdge), H: 20}
}

func (g *Game) drawDevelopPreviewStatus(screen *ebiten.Image, layout developLayout, left, right string) {
	available := developPreviewStatusRect(layout, left, right)
	if available.W < 64 {
		return
	}
	label, accent := g.developPreviewStatus()
	label = truncateTextToWidth(label, available.W-30, developSmallFace)
	w := min(available.W, developSmallTextWidth(label)+30)
	r := editor.Rect{X: available.X + (available.W-w)/2, Y: available.Y, W: w, H: available.H}
	fillRoundedRect(screen, r, 5, color.RGBA{R: 11, G: 12, B: 25, A: 238})
	strokeRoundedRect(screen, r, 5, rgba(accent, 150))
	fillRect(screen, editor.Rect{X: r.X + 8, Y: r.Y + 7, W: 6, H: 6}, accent)
	drawDevelopSmallText(screen, label, r.X+20, r.Y+(r.H-developSmallTextHeight)/2, accent)
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
		return truncateDevelopTelemetryText(text, colW)
	}
	for column := 1; column < 3; column++ {
		x := r.X + column*colW
		drawLine(screen, x, r.Y+10, x, r.Y+r.H-10, colorBorder)
	}

	x := r.X + 12
	drawDevelopText(screen, columnText("CARTRIDGE"), x, r.Y+14, colorDim)
	drawDevelopText(screen, columnText(filepath.Base(g.model.Dir)), x, r.Y+40, colorText)
	drawDevelopText(screen, columnText("240 x 160  /  59.73 Hz"), x, r.Y+66, colorLCD)
	drawDevelopText(screen, columnText(fmt.Sprintf("PREVIEW  %.1f FPS", g.playFPS)), x, r.Y+92, colorText)
	g.runMu.Lock()
	romSize := g.runROMSize
	inputKeys := g.playKeys | g.playPendingKeys
	g.runMu.Unlock()
	if romSize != 0 {
		drawDevelopText(screen, columnText("ROM  "+formatDevelopBytes(uint64(romSize))), x, r.Y+118, colorText)
	}
	if r.H >= 205 {
		drawDevelopText(screen, columnText("INPUT  "+playKeyLabel(inputKeys)), x, r.Y+146, playInputColor(inputKeys))
	}
	x = r.X + colW + 12
	drawDevelopText(screen, columnText("GAME BOY PSG  /  HW"), x, r.Y+14, colorDim)
	if !g.playTelemetry.Valid {
		drawDevelopText(screen, columnText("Waiting for mGBA telemetry"), x, r.Y+44, colorDim)
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
			drawDevelopText(screen, columnText(fmt.Sprintf("CH%d  %s  REG %04X", channel+1, state, registers[channel])), x, r.Y+42+channel*24, clr)
		}
		drawDevelopText(screen, columnText(fmt.Sprintf("MIX %04X  STATUS %04X", g.playTelemetry.PSG.MixPSG, g.playTelemetry.PSG.Status)), x, r.Y+142, colorLCD)
	}
	if r.H >= 220 && g.playPCM != nil {
		clr := colorDim
		label := "SILENT / IDLE"
		if g.playAudioActive {
			queuedMS, underruns, overruns := g.developHostAudioStatus()
			label = formatDevelopHostAudio(queuedMS, underruns, overruns)
			if underruns > 0 || overruns > 0 {
				clr = colorHighlight
			}
		}
		drawDevelopText(screen, columnText("HOST AUDIO"), x, r.Y+r.H-54, colorDim)
		drawDevelopText(screen, columnText(label), x, r.Y+r.H-30, clr)
	}

	x = r.X + colW*2 + 12
	drawDevelopText(screen, columnText("DEBUG / INSPECTOR"), x, r.Y+14, colorDim)
	g.runMu.Lock()
	ewramUsed, ewramFree, iwramUsed := g.runEWRAMUsed, g.runEWRAMFree, g.runIWRAMUsed
	rendererRequested, rendererSelected := g.runRendererRequested, g.runRendererSelected
	g.runMu.Unlock()
	if rendererRequested == "" && g.model != nil {
		rendererRequested = g.model.Project.Renderer
	}
	rendererLabel := developRendererLabel(rendererRequested, rendererSelected)
	drawDevelopText(screen, columnText(rendererLabel), x, r.Y+64, developRendererColor(rendererSelected))
	if !g.playTelemetry.DebugValid {
		drawDevelopText(screen, columnText("CPU  waiting for live preview"), x, r.Y+40, colorDim)
		if ewramUsed != 0 {
			drawDevelopText(screen, columnText(developEWRAMLabel(ewramUsed)), x, r.Y+88, developEWRAMColor(ewramUsed))
			drawDevelopText(screen, columnText(developMemoryHeadroomLabel(ewramFree, iwramUsed)), x, r.Y+112, colorLCD)
		} else {
			drawDevelopText(screen, columnText("RAM  available after build"), x, r.Y+88, colorDim)
		}
		return
	}
	stats := g.playTelemetry.Stats
	load := float64(stats.LastFrame) / 561792.0
	loadColor := developUtilizationColor(load)
	drawDevelopText(screen, columnText(fmt.Sprintf("CPU  %.1f%%  %s cyc", load*100, formatDevelopCount(stats.LastFrame))), x, r.Y+40, loadColor)
	if ewramUsed != 0 {
		drawDevelopText(screen, columnText(developEWRAMLabel(ewramUsed)), x, r.Y+88, developEWRAMColor(ewramUsed))
	} else {
		drawDevelopText(screen, columnText("EWRAM  build data unavailable"), x, r.Y+88, colorDim)
	}
	heapVisible := stats.HeapCapacity != 0
	compact := r.H < 220
	frameY, updateY := r.Y+112, r.Y+136
	if heapVisible {
		drawDevelopText(screen, columnText(developHeapLabel(stats)), x, r.Y+112, developHeapColor(stats))
		frameY, updateY = r.Y+136, r.Y+160
		compact = r.H < 240
		if r.H >= 300 {
			drawDevelopText(screen, columnText(developHeapAuxLabel(stats)), x, r.Y+136, developHeapColor(stats))
			frameY, updateY = r.Y+160, r.Y+184
		}
	}
	if !compact {
		drawDevelopText(screen, columnText(fmt.Sprintf("UPDATE %s  DRAW %s", formatDevelopCount(stats.LastUpdate), formatDevelopCount(stats.LastDraw))), x, updateY, colorText)
	}
	drawDevelopText(screen, columnText(fmt.Sprintf("FRAME %d  MISSED %d", stats.Frames, stats.MissedFrames)), x, frameY, colorText)
	if r.H >= 250 && (!heapVisible || r.H >= 300) {
		g.drawDevelopCPUGraph(screen, editor.Rect{X: x, Y: r.Y + r.H - 42, W: colW - 24, H: 28})
	}
}

func truncateDevelopTelemetryText(value string, columnWidth int) string {
	return truncateDevelopText(value, max(0, columnWidth-24))
}

func developRendererLabel(requested, selected string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	selected = strings.ToLower(strings.TrimSpace(selected))
	if selected == "" {
		if requested == "" {
			return "RENDERER  BUILD TO SELECT"
		}
		return "RENDERER  " + developRendererName(requested) + " / NOT BUILT"
	}
	if requested == "" || requested == selected {
		return "RENDERER  " + developRendererName(selected)
	}
	return "RENDERER  " + developRendererName(requested) + " -> " + developRendererName(selected)
}

func developRendererName(renderer string) string {
	switch strings.ToLower(strings.TrimSpace(renderer)) {
	case "mode0":
		return "MODE 0"
	case "mode4":
		return "MODE 4"
	case "auto":
		return "AUTO"
	default:
		return strings.ToUpper(strings.TrimSpace(renderer))
	}
}

func developRendererColor(selected string) color.Color {
	if strings.EqualFold(strings.TrimSpace(selected), "mode0") {
		return colorLCD
	}
	if strings.TrimSpace(selected) == "" {
		return colorDim
	}
	return colorCode
}

func (g *Game) drawDevelopCompactTelemetry(screen *ebiten.Image, layout developLayout) {
	r := layout.Play.Inspector
	colW := max(1, r.W/2)
	drawLine(screen, r.X+colW, r.Y+10, r.X+colW, r.Y+r.H-10, colorBorder)
	cell := func(column, row int, value string, clr color.Color) {
		x := r.X + column*colW + 12
		y := r.Y + 12 + row*22
		drawDevelopSmallText(screen, truncateTextToWidth(value, colW-24, developSmallFace), x, y, clr)
	}

	g.runMu.Lock()
	romSize := g.runROMSize
	inputKeys := g.playKeys | g.playPendingKeys
	ewramUsed := g.runEWRAMUsed
	rendererRequested, rendererSelected := g.runRendererRequested, g.runRendererSelected
	g.runMu.Unlock()
	if rendererRequested == "" && g.model != nil {
		rendererRequested = g.model.Project.Renderer
	}
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
	cell(1, 0, "DEBUG / RUNTIME", colorDim)
	cell(1, 1, developRendererLabel(rendererRequested, rendererSelected), developRendererColor(rendererSelected))
	if !g.playTelemetry.Valid {
		cell(1, 2, "mGBA  ·  WAITING", colorDim)
		cell(1, 3, "CPU  WAITING", colorDim)
		if ewramUsed != 0 {
			cell(1, 4, developEWRAMCompactLabel(ewramUsed), developEWRAMColor(ewramUsed))
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
	cell(1, 2, fmt.Sprintf("PSG  %d/4 CHANNELS", activeChannels), colorLCD)
	if !g.playTelemetry.DebugValid {
		cell(1, 3, "CPU  WAITING", colorDim)
		if ewramUsed != 0 {
			cell(1, 4, developEWRAMCompactLabel(ewramUsed), developEWRAMColor(ewramUsed))
		}
		return
	}
	stats := g.playTelemetry.Stats
	load := float64(stats.LastFrame) / 561792.0
	loadColor := developUtilizationColor(load)
	cell(1, 3, fmt.Sprintf("CPU  %.1f%%  %s CYC", load*100, formatDevelopCount(stats.LastFrame)), loadColor)
	if stats.HeapError != emulator.HeapOK {
		cell(1, 4, developHeapCompactLabel(stats), colorDanger)
	} else if ewramUsed != 0 {
		cell(1, 4, developEWRAMCompactLabel(ewramUsed), developEWRAMColor(ewramUsed))
	}
	if r.H >= 150 {
		cell(1, 5, fmt.Sprintf("FRAME %d  MISSED %d", stats.Frames, stats.MissedFrames), colorText)
	}
	if stats.HeapCapacity != 0 && stats.HeapError == emulator.HeapOK && r.H >= 170 {
		cell(1, 6, developHeapCompactLabel(stats), developHeapColor(stats))
	}
	if stats.HeapCapacity != 0 && r.H >= 195 {
		cell(1, 7, developHeapAuxLabel(stats), developHeapColor(stats))
	}
}

func (g *Game) developHostAudioStatus() (queuedMilliseconds int, underruns, overruns uint64) {
	if g == nil || g.playPCM == nil {
		return 0, 0, 0
	}
	queued, totalUnderruns, totalOverruns := g.playPCM.Stats()
	queuedMilliseconds = queued * 1000 / emulator.SampleRate
	if !g.playAudioBaselineReady {
		return queuedMilliseconds, 0, 0
	}
	if totalUnderruns >= g.playAudioBaselineUnderruns {
		underruns = totalUnderruns - g.playAudioBaselineUnderruns
	}
	if totalOverruns >= g.playAudioBaselineOverruns {
		overruns = totalOverruns - g.playAudioBaselineOverruns
	}
	return queuedMilliseconds, underruns, overruns
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

func formatDevelopHostAudio(queuedMilliseconds int, underruns, overruns uint64) string {
	if underruns > 0 || overruns > 0 {
		return fmt.Sprintf("%dms  UNDER %d  OVER %d", queuedMilliseconds, underruns, overruns)
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
	load := float64(used) / float64(build.EWRAMCapacity)
	return fmt.Sprintf("EWRAM %.0f%% %s %sK", load*100, developUtilizationState(load), formatDevelopKiB(used))
}

func developEWRAMCompactLabel(used uint64) string {
	load := float64(used) / float64(build.EWRAMCapacity)
	return fmt.Sprintf("EWRAM %.0f%% %s", load*100, developUtilizationState(load))
}

func developEWRAMColor(used uint64) color.Color {
	return developUtilizationColor(float64(used) / float64(build.EWRAMCapacity))
}

func developHeapLabel(stats emulator.DebugStats) string {
	if stats.HeapCapacity == 0 {
		return "HEAP  TELEMETRY UNAVAILABLE"
	}
	if stats.HeapError != emulator.HeapOK {
		return fmt.Sprintf("HEAP %s  %sK LIMIT", developHeapErrorLabel(stats.HeapError), formatDevelopKiB(uint64(stats.HeapCapacity)))
	}
	live := "--"
	if stats.GCCount != 0 {
		live = formatDevelopKiB(uint64(stats.HeapLive)) + "K"
	}
	return fmt.Sprintf("HEAP %sK/%sK  LIVE %s", formatDevelopKiB(uint64(stats.HeapAllocated)), formatDevelopKiB(uint64(stats.HeapCapacity)), live)
}

func developHeapCompactLabel(stats emulator.DebugStats) string {
	return developHeapLabel(stats)
}

func developHeapAuxLabel(stats emulator.DebugStats) string {
	peak := "--"
	if stats.GCCount != 0 {
		peak = formatDevelopKiB(uint64(stats.HeapPeakLive)) + "K"
	}
	return fmt.Sprintf("HANDLES %d/%d  PEAK %s  GC %d", stats.HeapHandles, stats.HeapHandleLimit, peak, stats.GCCount)
}

func developHeapColor(stats emulator.DebugStats) color.Color {
	if stats.HeapError != emulator.HeapOK {
		return colorDanger
	}
	if stats.HeapCapacity == 0 {
		return colorDim
	}
	used := stats.HeapAllocated
	if stats.GCCount != 0 {
		used = stats.HeapLive
	}
	return developUtilizationColor(float64(used) / float64(stats.HeapCapacity))
}

func developHeapErrorLabel(err emulator.HeapError) string {
	switch err {
	case emulator.HeapOOM:
		return "OOM"
	case emulator.HeapCorrupt:
		return "CORRUPT"
	case emulator.HeapCorruptRef:
		return "BAD REF"
	case emulator.HeapBusy:
		return "BUSY"
	default:
		return fmt.Sprintf("ERROR %d", err)
	}
}

func developUtilizationColor(load float64) color.Color {
	if load >= .90 {
		return colorDanger
	}
	if load >= .70 {
		return colorHighlight
	}
	return colorRun
}

func developGlobalUsageState(usage build.GlobalUsage) string {
	if usage.Limit == 0 {
		return "PENDING"
	}
	if usage.Count > usage.Limit {
		return "BLOCKED"
	}
	if usage.Count == usage.Limit {
		return "FULL"
	}
	if usage.Count*8 >= usage.Limit*7 {
		return "WARNING"
	}
	return "OK"
}

func developGlobalUsageColor(usage build.GlobalUsage) color.Color {
	switch developGlobalUsageState(usage) {
	case "BLOCKED":
		return colorDanger
	case "FULL", "WARNING":
		return colorHighlight
	case "OK":
		return colorRun
	default:
		return colorDim
	}
}

func developUtilizationState(load float64) string {
	if load >= .90 {
		return "CRITICAL"
	}
	if load >= .70 {
		return "HIGH"
	}
	return "OK"
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
		drawDevelopTextCentered(screen, label, tab, clr)
	}
	drawLine(screen, r.X+8, r.Y+34, r.X+r.W-8, r.Y+34, colorBorder)
	if g.developPanel == developMemory {
		g.drawDevelopMemory(screen, layout)
		return
	}
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
	widths := [...]int{112, 96, 92, 106, 112, 104, 150}
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
	runError, sourceError := g.runError, g.sourceAnalysisError
	usage := g.runGlobalUsage
	g.runMu.Unlock()
	return g.codeDiagnosticLine != 0 || runError != "" || sourceError != "" || (usage.Limit != 0 && usage.Count > usage.Limit)
}

func (g *Game) developPanelLines(panel developPanel) []string {
	if panel == developMemory {
		return g.developMemoryLines()
	}
	g.runMu.Lock()
	status, runError, sourceError := g.runStatus, g.runError, g.sourceAnalysisError
	romPath, romSize := g.runROMPath, g.runROMSize
	ewramUsed, ewramFree, iwramUsed := g.runEWRAMUsed, g.runEWRAMFree, g.runIWRAMUsed
	entry := g.runEntry
	sources := append([]string(nil), g.runSourceFiles...)
	globalUsage := cloneGlobalUsage(g.runGlobalUsage)
	generated := g.runGeneratedGo
	rendererRequested, rendererSelected := g.runRendererRequested, g.runRendererSelected
	rendererReason, rendererProfile, rendererDiagnostic := g.runRendererReason, g.runRendererProfile, g.runRendererDiagnostic
	rendererLayers, rendererObjects, rendererText := g.runRendererLayers, g.runRendererObjects, g.runRendererText
	events := append([]build.Event(nil), g.runBuildEvents...)
	runtimeLog := append([]string(nil), g.runRuntimeLog...)
	g.runMu.Unlock()
	telemetryValid, debugValid := g.playTelemetry.Valid, g.playTelemetry.DebugValid
	watches := append([]emulator.Watch(nil), g.playTelemetry.Watches...)
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
		if sourceError != "" && sourceError != runError {
			lines = append(lines, sourceError)
		}
		if globalUsage.Limit != 0 && globalUsage.Count > globalUsage.Limit {
			firstExcess := min(globalUsage.Limit, len(globalUsage.Symbols))
			for _, symbol := range globalUsage.Symbols[firstExcess:] {
				lines = append(lines, fmt.Sprintf("%s:%d:%d  global %q exceeds the persistent-state slot budget", symbol.Source, symbol.Line, symbol.Column, symbol.Name))
			}
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
		if globalUsage.Limit != 0 {
			lines = append(lines, fmt.Sprintf("GLOBALS %d / %d  %s", globalUsage.Count, globalUsage.Limit, developGlobalUsageState(globalUsage)))
		}
		if romPath != "" {
			lines = append(lines,
				fmt.Sprintf("ROM     %s  /  %s", filepath.Base(romPath), formatDevelopBytes(uint64(romSize))),
				fmt.Sprintf("RENDER  requested %s  /  selected %s  /  %s", rendererRequested, rendererSelected, rendererReason),
				fmt.Sprintf("EWRAM   %s used  /  %s free  /  %s total", formatDevelopBytes(ewramUsed), formatDevelopBytes(ewramFree), formatDevelopBytes(build.EWRAMCapacity)),
				fmt.Sprintf("IWRAM   %s used  /  %s total", formatDevelopBytes(iwramUsed), formatDevelopBytes(build.IWRAMCapacity)),
			)
			if rendererSelected == "mode0" {
				lines = append(lines, fmt.Sprintf("MODE 0  %d BG layer(s)  /  OBJ %t  /  text %t", rendererLayers, rendererObjects, rendererText))
			}
			if rendererProfile != "" {
				lines = append(lines, "AUTO    profile "+rendererProfile)
			}
			if rendererDiagnostic != "" {
				lines = append(lines, "FALLBACK  "+rendererDiagnostic)
			}
		}
		lines = append(lines,
			fmt.Sprintf("SPRITES %d  /  MAP %d x %d", g.spriteN(), g.model.Map.Width, g.model.Map.Height),
			"AUDIO   4 Game Boy PSG channels  /  PCM OFF",
		)
		for _, symbol := range globalUsage.Symbols {
			lines = append(lines, fmt.Sprintf("GLOBAL  %s:%d:%d  %s", symbol.Source, symbol.Line, symbol.Column, symbol.Name))
		}
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
	case developWatches:
		if !telemetryValid {
			return []string{"Waiting for live mGBA telemetry. Build and start the cartridge preview."}
		}
		if !debugValid {
			return []string{"Waiting for debug data from the preview ROM."}
		}
		if len(watches) == 0 {
			return []string{"No top-level globals are available in this build."}
		}
		total := max(globalUsage.Count, len(watches))
		lines := []string{fmt.Sprintf("LIVE WATCHES  %d / %d GLOBALS", len(watches), total)}
		if total > len(watches) {
			lines[0] += "  /  DEBUG MAILBOX SHOWS THE FIRST 16"
		}
		for _, watch := range watches {
			lines = append(lines, fmt.Sprintf("%-15s = %s", watch.Name, developWatchValue(watch)))
		}
		return lines
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
		g.keepCodeCursorVisible(newDevelopPanelLayout(g.developPanel).Code)
		return
	}
	g.runMu.Lock()
	message := g.runError
	if message == "" {
		message = g.sourceAnalysisError
	}
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
	g.keepCodeCursorVisible(newDevelopPanelLayout(g.developPanel).Code)
}
