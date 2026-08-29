package main

import (
	"errors"
	"image/color"
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/build"
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
			"gutter": layout.Gutter, "text": layout.Text, "inspector": layout.Inspector,
			"diagnostics": layout.Diagnostics, "save": layout.Save, "run": layout.Run,
		} {
			if !rectInsideIDE(rect, workspace) {
				t.Fatalf("%dx%d %s outside workspace: %#v in %#v", size[0], size[1], name, rect, workspace)
			}
		}
		for _, pair := range [][2]editor.Rect{
			{layout.Sidebar, layout.Editor}, {layout.Editor, layout.Inspector},
			{layout.Editor, layout.Diagnostics}, {layout.Save, layout.Run},
		} {
			if rectsOverlapIDE(pair[0], pair[1]) {
				t.Fatalf("%dx%d code panels overlap: %#v and %#v", size[0], size[1], pair[0], pair[1])
			}
		}
		if !rectInsideIDE(layout.Gutter, layout.Editor) || !rectInsideIDE(layout.Text, layout.Editor) {
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
		if !rectInsideIDE(layout.LCD, layout.Inspector) {
			t.Fatalf("%dx%d LCD control escapes inspector: %#v", size[0], size[1], layout.LCD)
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
			"source": layout.Code.Editor, "gutter": layout.Code.Gutter, "text": layout.Code.Text,
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
		} {
			if rectsOverlapIDE(pair[0], pair[1]) {
				t.Fatalf("%dx%d Develop panels overlap: %#v and %#v", size.width, size.height, pair[0], pair[1])
			}
		}
		if !rectInsideIDE(layout.Code.Gutter, layout.Code.Editor) || !rectInsideIDE(layout.Code.Text, layout.Code.Editor) {
			t.Fatalf("%dx%d code children escape the source panel", size.width, size.height)
		}
		if !rectInsideIDE(layout.Play.Screen, layout.Play.Bezel) || !rectInsideIDE(layout.Play.Bezel, layout.Play.Stage) {
			t.Fatalf("%dx%d live GBA screen/bezel escapes preview", size.width, size.height)
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

func TestEffectLevelsCycleThroughAllPresentationModes(t *testing.T) {
	g := &Game{}
	g.theme.level = effectsClean
	g.cycleEffects()
	if g.theme.level != effectsSoft {
		t.Fatalf("clean should cycle to soft, got %v", g.theme.level)
	}
	g.cycleEffects()
	if g.theme.level != effectsFull {
		t.Fatalf("soft should cycle to full, got %v", g.theme.level)
	}
	g.cycleEffects()
	if g.theme.level != effectsClean {
		t.Fatalf("full should cycle to clean, got %v", g.theme.level)
	}
}

func TestLCDShaderCompiles(t *testing.T) {
	if _, err := playLCDShader(); err != nil {
		t.Fatalf("compile LCD shader: %v", err)
	}
}

func TestStatusMessageColorSeparatesInformationFromErrors(t *testing.T) {
	if got := statusMessageColor("visual effects: SOFT"); got != color.Color(colorLCD) {
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

func TestDevelopBuildNoticeTracksTheBuildLifecycle(t *testing.T) {
	g := &Game{}
	if label, _ := g.developBuildNotice(); label != "READY  /  BUILD + RELOAD TO START" {
		t.Fatalf("ready notice = %q", label)
	}
	g.runBusy = true
	g.runBuildEvents = []build.Event{{Phase: build.PhaseMemory, Message: "Checking EWRAM"}}
	if label, _ := g.developBuildNotice(); label != "BUILDING  /  MEMORY  /  Checking EWRAM" {
		t.Fatalf("building notice = %q", label)
	}
	g.runBusy = false
	g.runStatus = "RUNNING ON mGBA CORE"
	if label, _ := g.developBuildNotice(); label != "BUILD COMPLETE  /  LIVE PREVIEW RUNNING" {
		t.Fatalf("running notice = %q", label)
	}
	g.runError = "link failed"
	g.runStatus = "BUILD FAILED"
	if label, _ := g.developBuildNotice(); label != "BUILD FAILED  /  SEE BUILD LOG BELOW" {
		t.Fatalf("failure notice = %q", label)
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
	if width := uiTextWidth(wideLabel); width > wideAvailable {
		t.Fatalf("wide memory health label %q uses %d px, available %d", wideLabel, width, wideAvailable)
	}

	setEditorLayout(minimumScreenWidth, minimumScreenHeight)
	compact := newDevelopLayout().Play.Inspector
	compactAvailable := compact.W/2 - 24
	compactLabel := developEWRAMCompactLabel(build.EWRAMCapacity)
	if width := smallTextWidth(compactLabel); width > compactAvailable {
		t.Fatalf("compact memory health label %q uses %d px, available %d", compactLabel, width, compactAvailable)
	}
}

func TestDevelopTelemetryUsesTheAvailablePixelWidth(t *testing.T) {
	label := "CPU  96.9%  544.0K cyc"
	columnWidth := uiTextWidth(label) + 24
	if got := truncateDevelopTelemetryText(label, columnWidth); got != label {
		t.Fatalf("exact-fit telemetry label = %q, want %q", got, label)
	}

	narrowWidth := uiTextWidth("CPU  96.9%") + 24
	got := truncateDevelopTelemetryText(label, narrowWidth)
	if got == label || !strings.HasSuffix(got, "...") {
		t.Fatalf("narrow telemetry label = %q, want truncated label", got)
	}
	if width := uiTextWidth(got); width > narrowWidth-24 {
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
	if uiFace != comfort.face || uiSmallFace != comfort.smallFace || uiTitleFace != comfort.titleFace || codeFace != comfort.codeFace || uiCharWidth != comfort.charWidth || uiSmallTextHeight != comfort.smallTextHeight || codeCharWidth != comfort.codeCharWidth || codeLineHeight != comfort.lineHeight {
		t.Fatalf("comfort metrics = ui %d code %d line %d", uiCharWidth, codeCharWidth, codeLineHeight)
	}
	setInterfaceTextSize(textLarge)
	large := uiFontStyles[textLarge]
	if uiFace != large.face || uiSmallFace != large.smallFace || uiTitleFace != large.titleFace || codeFace != large.codeFace || uiCharWidth != large.charWidth || uiSmallTextHeight != large.smallTextHeight || codeCharWidth != large.codeCharWidth || codeLineHeight != large.lineHeight {
		t.Fatalf("large metrics = ui %d code %d line %d", uiCharWidth, codeCharWidth, codeLineHeight)
	}
	if large.size <= comfort.size || large.charWidth <= comfort.charWidth || large.textHeight <= comfort.textHeight {
		t.Fatalf("large font metrics do not grow: comfort=%+v large=%+v", comfort, large)
	}
}
