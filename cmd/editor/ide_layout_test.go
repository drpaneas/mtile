package main

import (
	"errors"
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/codeeditor"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/emulator"
)

func rectInsideIDE(inner, outer editor.Rect) bool {
	return inner.W > 0 && inner.H > 0 && inner.X >= outer.X && inner.Y >= outer.Y &&
		inner.X+inner.W <= outer.X+outer.W && inner.Y+inner.H <= outer.Y+outer.H
}

func rectsOverlapIDE(a, b editor.Rect) bool {
	return a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y
}

func TestGraphicsWorkspacesReserveHeadersWithoutCrowdingEditors(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1920, 1200}, {2560, 1440}, {3840, 2160}} {
		setEditorLayout(size[0], size[1])
		workspace := editor.Rect{X: 0, Y: editorTabsHeight, W: screenWidth, H: statusBarRect.Y - editorTabsHeight}
		if !rectInsideIDE(spriteHeaderRect, workspace) {
			t.Fatalf("%dx%d sprite header outside workspace: %#v", size[0], size[1], spriteHeaderRect)
		}
		for name, rect := range map[string]editor.Rect{
			"sprite canvas": canvasRect, "sprite tools": spriteToolsRect,
			"palette": paletteRect, "flags": flagsRect, "picker": pickerRect,
			"spritesheet": sheetPanelRect,
		} {
			if rect.Y < spriteHeaderRect.Y+spriteHeaderRect.H {
				t.Fatalf("%dx%d %s intrudes into sprite header: %#v", size[0], size[1], name, rect)
			}
			if rect.Y+rect.H > statusBarRect.Y {
				t.Fatalf("%dx%d %s overlaps status bar: %#v", size[0], size[1], name, rect)
			}
		}

		layout := newMapLayout(size[0], size[1])
		if !rectInsideIDE(layout.Header, workspace) {
			t.Fatalf("%dx%d map header outside workspace: %#v", size[0], size[1], layout.Header)
		}
		if layout.Canvas.Y < layout.Header.Y+layout.Header.H {
			t.Fatalf("%dx%d map canvas intrudes into header: %#v", size[0], size[1], layout.Canvas)
		}
		if layout.Strip.Y+layout.Strip.H > statusBarRect.Y {
			t.Fatalf("%dx%d map strip overlaps status bar: %#v", size[0], size[1], layout.Strip)
		}
		if !rectInsideIDE(layout.Layers, layout.Strip) {
			t.Fatalf("%dx%d map layer toolbar escapes strip: layers=%#v strip=%#v", size[0], size[1], layout.Layers, layout.Strip)
		}
		if !rectInsideIDE(layout.MoveArea, layout.Strip) || !rectInsideIDE(layout.CopyArea, layout.Strip) ||
			rectsOverlapIDE(layout.MoveArea, layout.SelectedTile) || rectsOverlapIDE(layout.MoveArea, layout.CopyArea) ||
			rectsOverlapIDE(layout.CopyArea, layout.Coordinates) {
			t.Fatalf("%dx%d map area tools escape or overlap strip content: move=%#v copy=%#v selected=%#v coordinates=%#v", size[0], size[1], layout.MoveArea, layout.CopyArea, layout.SelectedTile, layout.Coordinates)
		}
		for bg, row := range layout.LayerRows {
			if !rectInsideIDE(row, layout.Layers) || !rectInsideIDE(layout.LayerEyes[bg], row) {
				t.Fatalf("%dx%d BG%d controls escape layer toolbar: row=%#v eye=%#v layers=%#v", size[0], size[1], bg, row, layout.LayerEyes[bg], layout.Layers)
			}
		}
	}
}

func TestCodeWorkspaceReflowsWithoutPanelOverlap(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1920, 1200}, {2560, 1440}} {
		setEditorLayout(size[0], size[1])
		layout := newCodeLayout()
		workspace := editor.Rect{X: 0, Y: editorTabsHeight, W: screenWidth, H: statusBarRect.Y - editorTabsHeight}
		for name, rect := range map[string]editor.Rect{
			"header": layout.Header, "sidebar": layout.Sidebar, "editor": layout.Editor,
			"gutter": layout.Gutter, "text": layout.Text, "horizontal scroll": layout.Horizontal, "inspector": layout.Inspector,
			"diagnostics": layout.Diagnostics, "save": layout.Save, "run": layout.Run,
		} {
			if !rectInsideIDE(rect, workspace) {
				t.Fatalf("%dx%d %s outside workspace: %#v in %#v", size[0], size[1], name, rect, workspace)
			}
		}
		for _, pair := range [][2]editor.Rect{
			{layout.Sidebar, layout.Editor}, {layout.Editor, layout.Inspector},
			{layout.Editor, layout.Diagnostics}, {layout.Save, layout.Run},
			{layout.Text, layout.Horizontal},
		} {
			if rectsOverlapIDE(pair[0], pair[1]) {
				t.Fatalf("%dx%d code panels overlap: %#v and %#v", size[0], size[1], pair[0], pair[1])
			}
		}
		if !rectInsideIDE(layout.Gutter, layout.Editor) || !rectInsideIDE(layout.Text, layout.Editor) || !rectInsideIDE(layout.Horizontal, layout.Editor) {
			t.Fatalf("%dx%d editor children escape the source panel", size[0], size[1])
		}
	}
}

func TestPlayWorkspaceKeepsScreenAndControlsSeparated(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1920, 1200}, {2560, 1440}} {
		setEditorLayout(size[0], size[1])
		layout := newPlayLayout()
		workspace := editor.Rect{X: 0, Y: editorTabsHeight, W: screenWidth, H: statusBarRect.Y - editorTabsHeight}
		for name, rect := range map[string]editor.Rect{
			"header": layout.Header, "stage": layout.Stage, "inspector": layout.Inspector,
			"bezel": layout.Bezel, "screen": layout.Screen,
		} {
			if !rectInsideIDE(rect, workspace) {
				t.Fatalf("%dx%d %s outside workspace: %#v", size[0], size[1], name, rect)
			}
		}
		if !rectInsideIDE(layout.Screen, layout.Bezel) || !rectInsideIDE(layout.Bezel, layout.Stage) {
			t.Fatalf("%dx%d screen/bezel escapes the emulator stage", size[0], size[1])
		}
		if rectsOverlapIDE(layout.Stage, layout.Inspector) {
			t.Fatalf("%dx%d emulator stage overlaps inspector", size[0], size[1])
		}
		for _, button := range []editor.Rect{layout.Reload, layout.Reset, layout.Stop} {
			if !rectInsideIDE(button, layout.Header) {
				t.Fatalf("%dx%d run control escapes header: %#v", size[0], size[1], button)
			}
		}
	}
}

func TestDevelopWorkspaceCombinesCodeAndLiveGBAWithoutOverlap(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range []struct {
		width, height int
		previewScale  int
	}{
		{minimumScreenWidth, minimumScreenHeight, 1},
		{1920, 1200, 3},
		{2560, 1440, 4},
		{3840, 2160, 7},
	} {
		setEditorLayout(size.width, size.height)
		layout := newDevelopLayout()
		workspace := editor.Rect{X: 0, Y: editorTabsHeight, W: screenWidth, H: statusBarRect.Y - editorTabsHeight}
		for name, rect := range map[string]editor.Rect{
			"header": layout.Header, "sidebar": layout.Code.Sidebar,
			"source": layout.Code.Editor, "gutter": layout.Code.Gutter, "text": layout.Code.Text, "horizontal scroll": layout.Code.Horizontal,
			"preview": layout.Play.Stage, "bezel": layout.Play.Bezel, "screen": layout.Play.Screen,
			"telemetry": layout.Play.Inspector, "diagnostics": layout.Code.Diagnostics,
			"active file": layout.Code.File, "source mode": layout.SourceMode,
			"build": layout.Build, "reset": layout.Reset, "stop": layout.Stop,
		} {
			if name == "sidebar" && rect.W == 0 {
				continue
			}
			if !rectInsideIDE(rect, workspace) {
				t.Fatalf("%dx%d %s outside Develop workspace: %#v in %#v", size.width, size.height, name, rect, workspace)
			}
		}
		for _, pair := range [][2]editor.Rect{
			{layout.Code.Sidebar, layout.Code.Editor},
			{layout.Code.Editor, layout.Play.Stage},
			{layout.Code.Editor, layout.Code.Diagnostics},
			{layout.Play.Stage, layout.Play.Inspector},
			{layout.Play.Inspector, layout.Code.Diagnostics},
			{layout.Code.File, layout.SourceMode},
			{layout.SourceMode, layout.Build},
			{layout.Build, layout.Reset},
			{layout.Reset, layout.Stop},
			{layout.Code.Text, layout.Code.Horizontal},
		} {
			if rectsOverlapIDE(pair[0], pair[1]) {
				t.Fatalf("%dx%d Develop panels overlap: %#v and %#v", size.width, size.height, pair[0], pair[1])
			}
		}
		if !rectInsideIDE(layout.Code.Gutter, layout.Code.Editor) || !rectInsideIDE(layout.Code.Text, layout.Code.Editor) || !rectInsideIDE(layout.Code.Horizontal, layout.Code.Editor) {
			t.Fatalf("%dx%d code children escape the source panel", size.width, size.height)
		}
		if !rectInsideIDE(layout.Play.Screen, layout.Play.Bezel) || !rectInsideIDE(layout.Play.Bezel, layout.Play.Stage) {
			t.Fatalf("%dx%d live GBA screen/bezel escapes preview", size.width, size.height)
		}
		status := developPreviewStatusRect(layout)
		if status.W < developSmallTextWidth("PREVIOUS BUILD")+30 || !rectInsideIDE(status, layout.Play.Stage) || rectsOverlapIDE(status, layout.Play.Bezel) {
			t.Fatalf("%dx%d preview status does not fit its header: %#v", size.width, size.height, status)
		}
		if layout.Code.Editor.W < 600 {
			t.Fatalf("%dx%d code viewport is too narrow: %d", size.width, size.height, layout.Code.Editor.W)
		}
		if layout.Code.Editor.W < layout.Play.Stage.W {
			t.Fatalf("%dx%d preview became wider than source: source %d, preview %d", size.width, size.height, layout.Code.Editor.W, layout.Play.Stage.W)
		}
		if size.width >= 1400 && layout.Code.File.X < 490 {
			t.Fatalf("%dx%d active-file controls intrude into the Mtile/workspace identity: %#v", size.width, size.height, layout.Code.File)
		}
		wantW, wantH := emulator.Width*size.previewScale, emulator.Height*size.previewScale
		if layout.Play.Screen.W != wantW || layout.Play.Screen.H != wantH {
			t.Fatalf("%dx%d preview = %dx%d, want pixel-perfect %dx%d", size.width, size.height, layout.Play.Screen.W, layout.Play.Screen.H, wantW, wantH)
		}
	}
}

func TestDevelopStatusMessageDoesNotOverlapWorkspace(t *testing.T) {
	t.Cleanup(func() {
		setInterfaceTextSize(textComfort)
		setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	})
	for textSize := textCompact; textSize < interfaceTextSizeCount; textSize++ {
		for _, size := range [][2]int{{1024, 900}, {1399, 900}, {1400, 900}, {1920, 1200}, {2560, 1440}, {3840, 2160}} {
			t.Run(fmt.Sprintf("%s/%dx%d", textSize, size[0], size[1]), func(t *testing.T) {
				setInterfaceTextSize(textSize)
				setEditorLayout(size[0], size[1])
				layout := newDevelopLayout()
				// The dark workspace must surround the bottom pane with the
				// same margin used at its right edge, before the message row.
				diagnostics := layout.Code.Diagnostics
				wantMargin := screenWidth - (diagnostics.X + diagnostics.W)
				if got := layout.Message.Y - (diagnostics.Y + diagnostics.H); got != wantMargin || got <= 0 {
					t.Errorf("bottom workspace margin = %d, want right margin %d", got, wantMargin)
				}
				// The message sits six pixels above the main status bar. Its
				// entire line must be reserved, including beneath diagnostics.
				message := editor.Rect{X: 8, Y: statusBarRect.Y - developTextHeight - 6, W: statusBarRect.W - 16, H: developTextHeight}
				for name, rect := range map[string]editor.Rect{
					"sidebar": layout.Code.Sidebar, "diagnostics": layout.Code.Diagnostics,
					"source": layout.Code.Editor, "preview": layout.Play.Stage,
					"telemetry": layout.Play.Inspector, "status bar": statusBarRect,
				} {
					if rect.W > 0 && rectsOverlapIDE(message, rect) {
						t.Errorf("message overlaps %s: %#v and %#v", name, message, rect)
					}
				}
			})
		}
	}
}

func TestPreferencesLayoutKeepsEveryControlInsideItsPanel(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1920, 1200}, {2560, 1440}, {3840, 2160}} {
		setEditorLayout(size[0], size[1])
		layout := newPreferencesLayout()
		screen := editor.Rect{X: 0, Y: 0, W: screenWidth, H: screenHeight}
		if !rectInsideIDE(layout.Panel, screen) {
			t.Fatalf("%dx%d preferences panel outside screen: %#v", size[0], size[1], layout.Panel)
		}
		for name, control := range map[string]editor.Rect{"close": layout.Close, "defaults": layout.Defaults} {
			if !rectInsideIDE(control, layout.Panel) {
				t.Fatalf("%dx%d %s outside preferences panel: %#v", size[0], size[1], name, control)
			}
		}
		for index, row := range layout.Rows {
			if !rectInsideIDE(row, layout.Panel) {
				t.Fatalf("%dx%d row %d outside preferences panel: %#v", size[0], size[1], index, row)
			}
			if !rectInsideIDE(layout.Minus[index], row) || !rectInsideIDE(layout.Plus[index], row) {
				t.Fatalf("%dx%d row %d controls escape row", size[0], size[1], index)
			}
			if rectsOverlapIDE(layout.Minus[index], layout.Plus[index]) {
				t.Fatalf("%dx%d row %d decrement/increment controls overlap", size[0], size[1], index)
			}
			if index > 0 && rectsOverlapIDE(layout.Rows[index-1], row) {
				t.Fatalf("%dx%d preference rows %d and %d overlap", size[0], size[1], index-1, index)
			}
		}
		if rectsOverlapIDE(layout.Rows[len(layout.Rows)-1], layout.Defaults) {
			t.Fatalf("%dx%d final preference row overlaps defaults button", size[0], size[1])
		}
	}
}

func TestApplicationChromeAlwaysUsesCleanPresentation(t *testing.T) {
	g := &Game{theme: themeState{level: effectsFull, buildPulse: 8, inkPulse: 8, mapPulse: 8}}
	g.updateTheme()
	if g.theme.level != effectsClean || g.theme.buildPulse != 0 || g.theme.inkPulse != 0 || g.theme.mapPulse != 0 {
		t.Fatalf("theme should normalize to a still clean UI, got %+v", g.theme)
	}
}

func TestStatusMessageColorSeparatesInformationFromErrors(t *testing.T) {
	if got := statusMessageColor("preferences restored"); got != color.Color(colorLCD) {
		t.Fatalf("informational message should use LCD color, got %v", got)
	}
	if got := statusMessageColor("save failed: disk full"); got != color.Color(colorDanger) {
		t.Fatalf("failure should use danger color, got %v", got)
	}
}

func TestEditorTabsStayInsideTopBarAtEverySupportedWindowSize(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	for _, size := range [][2]int{{minimumScreenWidth, minimumScreenHeight}, {1920, 1200}, {2560, 1440}, {3840, 2160}} {
		setEditorLayout(size[0], size[1])
		for _, mode := range primaryEditorModes {
			tab := editorTabRect(mode)
			if !rectInsideIDE(tab, editorTabsRect) {
				t.Fatalf("%dx%d %s tab outside top bar: %#v", size[0], size[1], mode.label(), tab)
			}
		}
	}
}

func TestEveryWorkspaceHasItsOwnAccent(t *testing.T) {
	seen := map[color.Color]editorMode{}
	for _, mode := range primaryEditorModes {
		accent := modeAccent(mode)
		if previous, exists := seen[accent]; exists {
			t.Fatalf("%s and %s share the same accent", previous.label(), mode.label())
		}
		seen[accent] = mode
	}
}

func TestStalePlayBuildCannotReplaceCurrentRun(t *testing.T) {
	g := &Game{runID: 4, runBusy: true, runStatus: "BUILDING CARTRIDGE..."}
	g.acceptPlayLaunch(playLaunch{id: 3, err: errors.New("stale failure")})
	if !g.runBusy || g.runStatus != "BUILDING CARTRIDGE..." || g.runError != "" {
		t.Fatalf("stale build changed current state: busy=%v status=%q error=%q", g.runBusy, g.runStatus, g.runError)
	}
	g.stopPlaySession()
	if g.runBusy || g.runID != 5 || g.runStatus != "PLAYER STOPPED" {
		t.Fatalf("stop did not invalidate pending build: busy=%v id=%d status=%q", g.runBusy, g.runID, g.runStatus)
	}
}

func TestDevelopPreviewStatusTracksTheBuildLifecycle(t *testing.T) {
	g := &Game{}
	if label, _ := g.developPreviewStatus(); label != "READY" {
		t.Fatalf("ready status = %q", label)
	}
	g.runBusy = true
	if label, _ := g.developPreviewStatus(); label != "BUILDING" {
		t.Fatalf("building status = %q", label)
	}
	g.runBusy = false
	g.runStatus = "RUNNING ON mGBA CORE"
	if label, _ := g.developPreviewStatus(); label != "LIVE" {
		t.Fatalf("running status = %q", label)
	}
	g.runError = "link failed"
	g.runStatus = "BUILD FAILED"
	if label, _ := g.developPreviewStatus(); label != "FAILED" {
		t.Fatalf("failure status = %q", label)
	}
}

func TestDevelopWatchesPanelShowsEveryMailboxWatch(t *testing.T) {
	watches := make([]emulator.Watch, 16)
	for index := range watches {
		watches[index] = emulator.Watch{
			Name: fmt.Sprintf("global_%02d", index),
			Kind: 2,
			Raw:  int32(index) * 65536,
		}
	}
	g := &Game{
		playTelemetry:  emulator.Telemetry{Valid: true, DebugValid: true, Watches: watches},
		runGlobalUsage: build.GlobalUsage{Count: 20, Limit: 256},
	}
	lines := g.developPanelLines(developWatches)
	if len(lines) != len(watches)+1 {
		t.Fatalf("watch panel has %d lines, want %d: %#v", len(lines), len(watches)+1, lines)
	}
	if !strings.Contains(lines[0], "16 / 20 GLOBALS") || !strings.Contains(lines[0], "FIRST 16") {
		t.Fatalf("watch capacity is not explained: %q", lines[0])
	}
	for index, watch := range watches {
		if !strings.Contains(lines[index+1], watch.Name) || !strings.Contains(lines[index+1], fmt.Sprint(index)) {
			t.Fatalf("watch %q missing from line %q", watch.Name, lines[index+1])
		}
	}
}

func TestDevelopGlobalUsageStateTracksBudgetPressure(t *testing.T) {
	for _, test := range []struct {
		count int
		want  string
	}{
		{count: 2, want: "OK"},
		{count: 224, want: "WARNING"},
		{count: 256, want: "FULL"},
		{count: 257, want: "BLOCKED"},
	} {
		if got := developGlobalUsageState(build.GlobalUsage{Count: test.count, Limit: 256}); got != test.want {
			t.Fatalf("global state for %d = %q, want %q", test.count, got, test.want)
		}
	}
}

func TestDevelopProblemsListEveryExcessGlobalWithSource(t *testing.T) {
	symbols := make([]build.GlobalSymbol, 257)
	for index := range symbols {
		symbols[index] = build.GlobalSymbol{Name: fmt.Sprintf("g%d", index), Source: "game.lua", Line: index + 1, Column: 1}
	}
	g := &Game{runGlobalUsage: build.GlobalUsage{Count: 257, Limit: 256, Symbols: symbols}}
	lines := g.developPanelLines(developProblems)
	if len(lines) != 1 || lines[0] != `game.lua:257:1  global "g256" exceeds the persistent-state slot budget` {
		t.Fatalf("global limit problem lines = %#v", lines)
	}
}

func TestFailedBuildKeepsPartialGlobalUsageVisible(t *testing.T) {
	g := &Game{runID: 4, runBusy: true}
	usage := build.GlobalUsage{Count: 257, Limit: 256, Symbols: []build.GlobalSymbol{{Name: "overflow", Source: "game.lua", Line: 8, Column: 1}}}
	g.acceptPlayLaunch(playLaunch{
		id:     4,
		result: build.Result{Entry: "game.lua", SourceFiles: []string{"game.lua"}, GlobalUsage: usage},
		err:    errors.New("global limit"),
	})
	if g.runBusy || g.runEntry != "game.lua" || g.runGlobalUsage.Count != 257 || g.runStatus != "BUILD FAILED" {
		t.Fatalf("failed build metadata = busy %v entry %q globals %#v status %q", g.runBusy, g.runEntry, g.runGlobalUsage, g.runStatus)
	}
}

func TestDevelopMemoryAndCycleLabelsStayCompact(t *testing.T) {
	if got := formatDevelopCount(153773); got != "153.8K" {
		t.Fatalf("cycle label = %q", got)
	}
	if got := formatDevelopBytes(build.EWRAMCapacity); got != "255.5 KiB" {
		t.Fatalf("EWRAM capacity label = %q", got)
	}
	label := developEWRAMLabel(128 * 1024)
	if label != "EWRAM 50% OK 128K" {
		t.Fatalf("EWRAM label = %q", label)
	}
	if got := formatDevelopHostAudio(160, 80, 3); got != "160ms  UNDER 80  OVER 3" {
		t.Fatalf("host audio xrun label = %q", got)
	}
	if got := formatDevelopHostAudio(160, 0, 0); got != "160ms QUEUED" {
		t.Fatalf("host audio queued label = %q", got)
	}
}

func TestDevelopHeapTelemetryLabelsExposeLivePressureAndFailure(t *testing.T) {
	stats := emulator.DebugStats{
		GCCount: 7, HeapCapacity: 56 * 1024, HeapAllocated: 41 * 1024,
		HeapLive: 24 * 1024, HeapPeakLive: 28 * 1024,
		HeapHandles: 321, HeapHandleLimit: 2048,
	}
	if got := developHeapLabel(stats); got != "HEAP 41K/56K  LIVE 24K" {
		t.Fatalf("heap label = %q", got)
	}
	if got := developHeapAuxLabel(stats); got != "HANDLES 321/2048  PEAK 28K  GC 7" {
		t.Fatalf("heap auxiliary label = %q", got)
	}
	if got := developHeapColor(stats); got != colorRun {
		t.Fatalf("low live-set color = %v", got)
	}

	stats.HeapLive = 40 * 1024
	if got := developHeapColor(stats); got != colorHighlight {
		t.Fatalf("high live-set color = %v", got)
	}
	stats.HeapError = emulator.HeapOOM
	if got := developHeapLabel(stats); got != "HEAP OOM  56K LIMIT" {
		t.Fatalf("OOM label = %q", got)
	}
	if got := developHeapColor(stats); got != colorDanger {
		t.Fatalf("OOM color = %v", got)
	}
}

func TestDevelopUtilizationHealthUsesVisibleThresholds(t *testing.T) {
	tests := []struct {
		load      float64
		wantState string
		wantColor color.Color
	}{
		{load: 0, wantState: "OK", wantColor: colorRun},
		{load: .6999, wantState: "OK", wantColor: colorRun},
		{load: .70, wantState: "HIGH", wantColor: colorHighlight},
		{load: .8999, wantState: "HIGH", wantColor: colorHighlight},
		{load: .90, wantState: "CRITICAL", wantColor: colorDanger},
		{load: 1.05, wantState: "CRITICAL", wantColor: colorDanger},
	}
	for _, test := range tests {
		if got := developUtilizationState(test.load); got != test.wantState {
			t.Errorf("developUtilizationState(%v) = %q, want %q", test.load, got, test.wantState)
		}
		if got := developUtilizationColor(test.load); got != test.wantColor {
			t.Errorf("developUtilizationColor(%v) = %v, want %v", test.load, got, test.wantColor)
		}
	}
}

func TestDevelopMemoryHealthFitsWideAndCompactInspectors(t *testing.T) {
	defer setEditorLayout(defaultScreenWidth, defaultScreenHeight)

	setEditorLayout(defaultScreenWidth, defaultScreenHeight)
	wide := newDevelopLayout().Play.Inspector
	wideAvailable := wide.W/3 - 24
	wideLabel := developEWRAMLabel(build.EWRAMCapacity)
	if width := developTextWidth(wideLabel); width > wideAvailable {
		t.Fatalf("wide memory health label %q uses %d px, available %d", wideLabel, width, wideAvailable)
	}

	setEditorLayout(minimumScreenWidth, minimumScreenHeight)
	compact := newDevelopLayout().Play.Inspector
	compactAvailable := compact.W/2 - 24
	compactLabel := developEWRAMCompactLabel(build.EWRAMCapacity)
	if width := developSmallTextWidth(compactLabel); width > compactAvailable {
		t.Fatalf("compact memory health label %q uses %d px, available %d", compactLabel, width, compactAvailable)
	}

	heap := emulator.DebugStats{
		GCCount: 12, HeapCapacity: 56 * 1024, HeapAllocated: 55 * 1024,
		HeapLive: 48 * 1024, HeapPeakLive: 50 * 1024,
		HeapHandles: 2047, HeapHandleLimit: 2048,
	}
	if label := developHeapLabel(heap); developTextWidth(label) > wideAvailable {
		t.Fatalf("wide heap label %q does not fit in %d px", label, wideAvailable)
	}
	if label := developHeapCompactLabel(heap); developSmallTextWidth(label) > compactAvailable {
		t.Fatalf("compact heap label %q does not fit in %d px", label, compactAvailable)
	}
}

func TestDevelopTelemetryUsesTheAvailablePixelWidth(t *testing.T) {
	label := "CPU  96.9%  544.0K cyc"
	columnWidth := developTextWidth(label) + 24
	if got := truncateDevelopTelemetryText(label, columnWidth); got != label {
		t.Fatalf("exact-fit telemetry label = %q, want %q", got, label)
	}

	narrowWidth := developTextWidth("CPU  96.9%") + 24
	got := truncateDevelopTelemetryText(label, narrowWidth)
	if got == label || !strings.HasSuffix(got, "...") {
		t.Fatalf("narrow telemetry label = %q, want truncated label", got)
	}
	if width := developTextWidth(got); width > narrowWidth-24 {
		t.Fatalf("truncated telemetry label width = %d, available = %d", width, narrowWidth-24)
	}
}

func TestDevelopRendererLabelShowsPolicyAndFinalSelection(t *testing.T) {
	tests := []struct {
		requested string
		selected  string
		want      string
	}{
		{requested: "auto", selected: "mode0", want: "RENDERER  AUTO -> MODE 0"},
		{requested: "auto", selected: "mode4", want: "RENDERER  AUTO -> MODE 4"},
		{requested: "mode4", selected: "mode4", want: "RENDERER  MODE 4"},
		{requested: "mode0", want: "RENDERER  MODE 0 / NOT BUILT"},
		{want: "RENDERER  BUILD TO SELECT"},
	}
	for _, test := range tests {
		if got := developRendererLabel(test.requested, test.selected); got != test.want {
			t.Fatalf("developRendererLabel(%q, %q) = %q, want %q", test.requested, test.selected, got, test.want)
		}
	}
}

func TestInterfaceTextSizeUpdatesMatchingEditorMetrics(t *testing.T) {
	defer setInterfaceTextSize(textCompact)
	setInterfaceTextSize(textComfort)
	comfort := uiFontStyles[textComfort]
	if uiFace != comfort.face || uiSmallFace != comfort.smallFace || uiTitleFace != comfort.titleFace || codeFace != comfort.codeFace || developFace != comfort.developFace || developSmallFace != comfort.developSmallFace || developTitleFace != comfort.developTitleFace || uiCharWidth != comfort.charWidth || uiSmallTextHeight != comfort.smallTextHeight || codeCharWidth != comfort.codeCharWidth || developCharWidth != comfort.developCharWidth || codeLineHeight != comfort.lineHeight {
		t.Fatalf("comfort metrics = ui %d code %d line %d", uiCharWidth, codeCharWidth, codeLineHeight)
	}
	setInterfaceTextSize(textLarge)
	large := uiFontStyles[textLarge]
	if uiFace != large.face || uiSmallFace != large.smallFace || uiTitleFace != large.titleFace || codeFace != large.codeFace || developFace != large.developFace || developSmallFace != large.developSmallFace || developTitleFace != large.developTitleFace || uiCharWidth != large.charWidth || uiSmallTextHeight != large.smallTextHeight || codeCharWidth != large.codeCharWidth || developCharWidth != large.developCharWidth || codeLineHeight != large.lineHeight {
		t.Fatalf("large metrics = ui %d code %d line %d", uiCharWidth, codeCharWidth, codeLineHeight)
	}
	if large.size <= comfort.size || large.charWidth <= comfort.charWidth || large.textHeight <= comfort.textHeight {
		t.Fatalf("large font metrics do not grow: comfort=%+v large=%+v", comfort, large)
	}
}

func TestCodeTypographyPresetsStayDenseAndReadable(t *testing.T) {
	wants := [...]struct {
		size       float64
		lineHeight int
	}{
		{size: 12, lineHeight: 18},
		{size: 14, lineHeight: 20},
		{size: 16, lineHeight: 23},
	}
	for index, want := range wants {
		style := uiFontStyles[index]
		if style.codeSize != want.size || style.lineHeight != want.lineHeight {
			t.Fatalf("preset %d code typography = %.0f/%d, want %.0f/%d", index, style.codeSize, style.lineHeight, want.size, want.lineHeight)
		}
		if style.codeTextHeight > style.lineHeight {
			t.Fatalf("preset %d glyph height %d exceeds line height %d", index, style.codeTextHeight, style.lineHeight)
		}
	}
}

func TestCodeHorizontalViewportClampsAndMapsScrollbar(t *testing.T) {
	defer setInterfaceTextSize(textCompact)
	setInterfaceTextSize(textComfort)
	g := &Game{code: codeeditor.New(strings.Repeat("x", 40))}
	layout := codeLayout{
		Text:       editor.Rect{X: 50, Y: 10, W: codeCharWidth*10 + 16, H: 200},
		Horizontal: editor.Rect{X: 50, Y: 210, W: 240, H: codeHorizontalScrollbarHeight},
	}
	if got, want := g.codeHorizontalScrollLimit(layout), 31; got != want {
		t.Fatalf("horizontal limit = %d, want %d", got, want)
	}
	left := g.codeHorizontalThumb(layout)
	if left.X != layout.Horizontal.X+3 {
		t.Fatalf("left thumb starts at %d, want %d", left.X, layout.Horizontal.X+3)
	}
	g.codeScrollColumn = g.codeHorizontalScrollLimit(layout)
	right := g.codeHorizontalThumb(layout)
	if got, want := right.X+right.W, layout.Horizontal.X+layout.Horizontal.W-3; got != want {
		t.Fatalf("right thumb ends at %d, want %d", got, want)
	}

	g.code.SetText("short")
	g.keepCodeCursorVisible(layout)
	if g.codeScrollColumn != 0 {
		t.Fatalf("horizontal scroll after shortening = %d, want 0", g.codeScrollColumn)
	}
}

func TestCodeWheelAccumulatesFineTrackpadMotion(t *testing.T) {
	var remainder float64
	position := 10
	for i := 0; i < 3; i++ {
		position = codeWheelAxis(position, 100, -0.1, &remainder)
		if position != 10 {
			t.Fatalf("tiny movement jumped a line at event %d", i)
		}
	}
	position = codeWheelAxis(position, 100, -0.1, &remainder)
	if position != 11 {
		t.Fatalf("fractional movement was lost: %d", position)
	}
	remainder = 0
	if got := codeWheelAxis(position, 100, -2, &remainder); got != 17 {
		t.Fatalf("mouse wheel movement = %d, want 17", got)
	}
	position = codeWheelAxis(99, 100, -2, &remainder)
	if position != 100 || remainder != 0 {
		t.Fatal("wheel did not clamp at end")
	}
}
