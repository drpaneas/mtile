package main

import (
	"fmt"
	"image/color"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/codeeditor"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/drpaneas/mtile/internal/emulator"
	"github.com/drpaneas/mtile/internal/midicontrol"
	"github.com/drpaneas/mtile/internal/performance"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Sprite edit selection in 8x8 cells. Pixel size is 8*cells, so the range is
// 8x8 through 128x128. The pixel canvas still gives a 128px selection at
// least two screen pixels per sprite pixel; going past that would undersample.
const (
	gridCellsMin = 1
	gridCellsMax = 16
)

const navDebounce = 150 * time.Millisecond

// Retained rendering handles the truly static editor. Keep only a very slow
// safety refresh for state changed by an uncommon external source that did not
// explicitly call requestRender; all normal input and asynchronous paths wake
// the UI immediately.
const idleRedrawTicks = 5 * 60

// Game is the editor's ebiten.Game. It owns all UI state; internal/editor's
// Model owns the actual asset data and its persistence/validation rules.
type Game struct {
	// tileLinkChoice indexes tileLinkTargets in the Shared tiles link row;
	// tileLinkSyncedTile is the selection it was last synced to.
	tileLinkChoice         int
	tileLinkSyncedTile     int
	mapRooms               bool
	mapRoomSelected        bool
	mapRoomCol, mapRoomRow int
	poseVariants           poseVariantEditor
	model                  *editor.Model
	theme                  themeState
	preferences            editorPreferences
	preferencesOpen        bool
	fontEditor             fontEditorState
	pcm                    pcmEditorState

	mode                 editorMode
	currentColor         int
	currentSprite        int
	objEditing           bool
	selectedPose         int
	poseStripScroll      int
	poseNameEditing      bool
	poseNameDraft        string
	objPage              int
	objPageFirstTab      int
	objPageNameEditing   bool
	objPageNameSelectAll bool
	objPageNameDraft     string
	objShowReuse         bool
	objUsage             *objTileUsage
	animation            animationEditorState
	gridCols             int
	gridRows             int
	spriteTool           spriteTool
	spriteShapeActive    bool
	spriteShapeStartX    int
	spriteShapeStartY    int
	spriteShapeEndX      int
	spriteShapeEndY      int
	spriteShapeFilled    bool

	hoverX, hoverY                     int // absolute sheet pixel under the cursor, -1 if none
	spriteCanvasZoomLevel              int
	spriteCanvasPanX, spriteCanvasPanY float64
	spriteCanvasLastMouseX             int
	spriteCanvasLastMouseY             int
	spriteCanvasPanning                bool
	sheetSelectionDragging             bool
	sheetSelectionAnchorCol            int
	sheetSelectionAnchorRow            int

	objDrawPixels []byte // temporary flattened animation pixels during one draw
	preview       *editor.Preview
	dirty         bool // set by any edit this Update(); triggers a debounced undo push + preview refresh

	sheetCam editor.Camera
	mapCam   editor.Camera

	mapLastMouseX, mapLastMouseY int
	middleWasDown                bool
	mapActiveLayer               int
	mapLayerVisible              [assetsync.MapLayerCount]bool
	mapTool                      mapTool
	mapArea                      *mapAreaSelection
	mapAreaSelecting             bool
	mapAreaPlacing               bool
	mapAreaAnchorX               int
	mapAreaAnchorY               int
	mapAreaGrabX                 int
	mapAreaGrabY                 int
	mapAreaTargetLayer           int
	mapAreaTargetX               int
	mapAreaTargetY               int

	copiedSprite spriteCopyBuffer
	hasCopied    bool
	spritePaste  *spritePastePlacement

	undo undoStack

	previewPlayer *editorAudioPlayer

	// Audio v2 presents composition, hardware controls, and song order in one
	// workspace. Imported PCM assets live in their separate PCM Editor.
	nativeChannel               int
	nativeSequence              int
	nativeBar                   int
	nativeSelectedStep          int
	nativeOctave                int
	nativeNoteLength            int
	nativeNoteLengthTicks       int
	nativeGridQuantize          performance.Quantize
	nativeInstrument            [4]int
	nativePerformanceInstrument [4]int
	nativePresetMenuOpen        bool
	nativePresetMenuCursor      int
	nativePresetMenuScroll      int
	nativeAdvanced              bool
	nativeSFXMode               bool
	nativeSFXID                 int
	nativeNoteDrag              bool
	nativeNoteDragMode          int
	nativeNoteDragDirty         bool
	nativeNoteDragStep          int
	nativeNoteDragRow           int
	nativeNoteDragAnchorStep    int
	nativeNoteDragAnchorRow     int
	nativeNoteDragTick          int
	nativeNoteDragAnchorTick    int
	nativeNoteDragEdits         int
	nativeRangeActive           bool
	nativeRangeStartTick        int
	nativeRangeEndTick          int
	nativeRangeLowNote          int
	nativeRangeHighNote         int
	nativeNoteClipboard         []assetsync.NativeEventAsset
	nativeNoteClipboardKind     int
	nativeNameTarget            int
	nativeNameDraft             string
	nativeClipboardID           int
	nativeClipboardSet          bool
	nativePreview               *editor.NativeMusicPreview
	nativeAudition              bool
	nativeMuteMask              uint8
	nativeSoloMask              uint8
	nativePreviewOffset         int
	nativeFocus                 int
	nativeStream                *performance.Stream
	nativeSession               *performance.Session
	nativeLivePlayer            *editorAudioPlayer
	nativeClickStream           *performance.ClickStream
	nativeClickPlayer           *editorAudioPlayer
	nativeConductor             *performance.Conductor
	nativeCaptureClock          performance.TrackerClock
	nativeCaptureSequence       int
	nativeCaptureTick           int
	nativeCaptureVisible        bool
	nativeCaptureEvents         []performance.RawEvent
	nativeCaptureNotes          []performance.TrackerNote
	nativeCaptureEventCount     int
	nativeCaptureDropped        uint32
	nativeCaptureQuantize       performance.Quantize
	nativeCaptureSwing          int
	nativeVoiceMode             performance.VoiceMode
	nativeQuantize              performance.Quantize
	nativeSwing                 int
	nativeOverdub               bool
	nativeMetronome             bool
	nativeCountIn               int
	nativeKeyboardHeld          [12]bool
	nativeTakeCount             int
	nativeTakeSelection         int
	nativeMIDI                  *midicontrol.Manager
	nativeMIDICommands          chan midicontrol.Command
	nativeMIDIName              string
	nativeMIDIParam             string
	nativeMIDIValue             string

	// Source documents are separate from the asset undo stack. Each Lua file
	// owns independent text history, selection, cursor, scroll, and diagnostics;
	// the fields below are aliases for the active document so the editing path
	// remains allocation-light.
	code                     *codeeditor.Editor
	codePath                 string
	codeDocuments            map[string]*codeDocument
	codeTabs                 []string
	codeActiveTab            int
	codeProjectEntries       []developProjectEntry
	codeProjectExpanded      map[string]bool
	codeProjectScroll        int
	codeClosePending         string
	codeClosePendingAt       time.Time
	codeReloadPending        string
	codeReloadPendingAt      time.Time
	codeQuickOpen            bool
	codeQuickDraft           string
	codeQuickMatches         []string
	codeQuickIndex           int
	codeScrollLine           int
	codeScrollColumn         int
	codeDragging             bool
	codeHorizontalDragging   bool
	codeHorizontalDragOffset int
	codeVerticalDragging     bool
	codeVerticalDragOffset   int
	codeWheelX               float64
	codeWheelY               float64
	codePointer              codePointerState
	codeSearchOpen           bool
	codeSearchDraft          string
	codeSearchMatches        []int
	codeSearchIndex          int
	codeCompletionOpen       bool
	codeCompletions          []codeCompletion
	codeCompletionIndex      int
	codeDiagnostic           string
	codeDiagnosticLine       int
	codeDiagnosticColumn     int
	codeClipboard            string
	codeLastParsed           string
	codeAnalysis             build.ProjectAnalysis
	codeAnalysisRevision     uint64
	codeAnalysisDue          time.Time
	codeAnalysisBusy         bool
	codeAnalysisResults      chan codeAnalysisResult
	lintShowNotes            bool
	developGameFocus         bool

	// Build/run state is synchronized because TinyGo and the emulator helper
	// must never block Ebitengine's UI thread.
	runMu                      sync.Mutex
	runStatus                  string
	runError                   string
	runBusy                    bool
	runID                      uint64
	runROMPath                 string
	runReportID                uint64
	romExport                  developROMExport
	runROMSize                 int
	runEWRAMUsed               uint64
	runEWRAMFree               uint64
	runIWRAMUsed               uint64
	runMemoryReady             bool
	runPoseMemory              build.PoseMemoryReport
	runEntry                   string
	runSourceFiles             []string
	runGlobalUsage             build.GlobalUsage
	sourceAnalysisError        string
	runGeneratedGo             string
	runRendererRequested       string
	runRendererSelected        string
	runRendererReason          string
	runRendererProfile         string
	runRendererDiagnostic      string
	runRendererLayers          int
	runRendererObjects         bool
	runRendererText            bool
	runBuildEvents             []build.Event
	runRuntimeLog              []string
	developPanel               developPanel
	developScroll              [developPanelCount]int
	developText                [developPanelCount]developTextView
	developTextFocused         bool
	developTextDragging        bool
	developTextScrollDragging  bool
	runReady                   chan playLaunch
	playFrames                 chan emulator.Frame
	playErrors                 chan error
	playStop                   chan struct{}
	playKeys                   uint16
	playPendingKeys            uint16
	playPendingTicks           uint8
	playLastSentKeys           uint16
	playReset                  bool
	playPaused                 bool
	playImage                  *ebiten.Image
	playPCM                    *emulator.PCMStream
	playAudio                  *editorAudioPlayer
	playStart                  time.Time
	playCount                  int
	playFPS                    float64
	playTelemetry              emulator.Telemetry
	playRunID                  uint64
	playTelemetrySeq           uint32
	playCPULoad                [60]float64
	playCPUSamples             int
	playAudioActive            bool
	playAudioBaselineReady     bool
	playAudioBaselineUnderruns uint64
	playAudioBaselineOverruns  uint64

	statusMsg string
	lastNav   time.Time

	// Ebitengine's retained offscreen is redrawn on demand. At a large Retina
	// window, rebuilding every vector, glyph and checker cell at 60 Hz consumes
	// a full CPU core and keeps the GPU permanently busy even when the editor is
	// untouched. Input gets a short redraw tail; genuinely moving surfaces keep
	// their full 60 Hz feedback; otherwise a low-rate safety refresh observes
	// asynchronous build/MIDI status without burning battery.
	rendered           bool
	renderRequested    bool
	renderInputTail    int
	renderIdleTicks    int
	renderDynamicTicks int
	renderCursorX      int
	renderCursorY      int
	renderCursorKnown  bool
	renderKeys         []ebiten.Key
}

func newGame(dir string) (*Game, error) {
	m, err := editor.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("open project %q: %w", dir, err)
	}
	g := &Game{
		model:         m,
		mode:          modeSprites,
		currentColor:  1,
		currentSprite: 0,
		selectedPose:  -1,
		gridCols:      gridCellsMin,
		gridRows:      gridCellsMin,
		hoverX:        -1,
		hoverY:        -1,
		nativeOctave:  4,
		// Keep the beginner default at 1/32 while the editor itself supports
		// precise 1/64 notes through the NOTE LENGTH control.
		nativeNoteLength:            2,
		nativeNoteLengthTicks:       editor.NativeTicksPerStep * 2,
		nativeGridQuantize:          performance.QuantizeSixtyFourth,
		nativeInstrument:            [4]int{-1, -1, -1, -1},
		nativePerformanceInstrument: [4]int{-2, -2, -2, -2},
		nativeSFXID:                 -1,
		nativeNoteDragStep:          -1,
		nativeNoteDragRow:           -1,
		nativeNoteDragAnchorStep:    -1,
		nativeNoteDragAnchorRow:     -1,
		nativeNoteDragTick:          -1,
		nativeNoteDragAnchorTick:    -1,
		nativeClipboardID:           -1,
		nativeQuantize:              performance.QuantizeSixteenth,
		nativeSwing:                 50,
		nativeTakeCount:             len(m.NativeAudio.Music.Takes),
		sheetCam:                    editor.NewCamera(),
		// Maps open at a readable 32px-per-tile scale. In particular, do not
		// fit an entire long level into the window: that can make every tile
		// less than one pixel wide.
		mapCam: editor.Camera{Zoom: editor.MaxZoom},
		// BG0 is the backwards-compatible editing default and the leftmost
		// background in the horizontal back-to-front toolbar.
		mapActiveLayer:      0,
		mapLayerVisible:     [assetsync.MapLayerCount]bool{true, true, true, true},
		codeDocuments:       make(map[string]*codeDocument),
		codeActiveTab:       -1,
		codeProjectExpanded: make(map[string]bool),
		runReady:            make(chan playLaunch, 4),
	}
	entry := m.Project.Entry
	if entry == "" {
		entry = "game.lua"
	}
	if err := g.openCodeDocument(filepath.Join(m.Dir, filepath.FromSlash(entry))); err != nil {
		return nil, fmt.Errorf("open project code: %w", err)
	}
	g.scanDevelopProject()
	// Diagnostics are displayed through sourceAnalysisError; keep the project open.
	_, _ = g.refreshProjectSourceAnalysis()
	g.initTheme()
	g.loadPreferences()
	g.initNativePerformance()
	if err := g.refreshPreview(); err != nil {
		return nil, fmt.Errorf("open project %q: %w", dir, err)
	}
	g.undo.reset(g.snapshot())
	g.ensurePoseSelection()
	g.fitSheetCamera()
	return g, nil
}

func (g *Game) spriteCols() int {
	sheet := g.activeSheet()
	if g == nil || sheet == nil || sheet.Width < 8 {
		return 0
	}
	return sheet.Width / 8
}

func (g *Game) spriteRows() int {
	sheet := g.activeSheet()
	if g == nil || sheet == nil || sheet.Height < 8 {
		return 0
	}
	return sheet.Height / 8
}

func (g *Game) activeSheet() *assetsync.Spritesheet {
	if g == nil || g.model == nil {
		return nil
	}
	if g.objEditing && g.mode != modeMap && g.model.ObjSheet != nil {
		return g.model.ObjSheet
	}
	return &g.model.Sheet
}

func (g *Game) spriteN() int { return g.spriteCols() * g.spriteRows() }

func (g *Game) spriteIsReserved(id int) bool {
	return editor.SpriteIsReservedOn(id, g.objEditing && g.mode != modeMap)
}

func (g *Game) setObjEditing(on bool) error {
	if on != g.objEditing {
		g.spritePaste = nil
		g.copiedSprite.cut = nil
	}
	if on && (g.model == nil || g.model.ObjSheet == nil) {
		if err := g.model.CreateObjSheet(assetsync.DefaultObjSheetW, assetsync.DefaultObjSheetH); err != nil {
			return err
		}
		g.dirty = true
	}
	if on {
		if err := g.model.NormalizeObjPages(); err != nil {
			return err
		}
	}
	g.objEditing = on
	if on {
		g.model.EnsureObjPages()
	}
	g.objPage = 0
	g.objPageNameEditing = false
	g.objUsage = nil
	g.invalidateAnimationPreview()
	g.currentSprite = 0
	g.gridCols = 1
	g.gridRows = 1
	g.cancelPoseNameEdit()
	g.ensurePoseSelection()
	if on {
		g.selectPose(g.selectedPose)
	}
	g.fitSheetCamera()
	return nil
}

// clampCurrentColor keeps the paint color inside the active palette's stored
// entries (a 256-color index 255 is not paint-legal on a 16-color shorthand).
func (g *Game) clampCurrentColor() {
	n := g.paintableColorCount()
	if n <= 0 || g.currentColor < 0 || g.currentColor >= n {
		g.currentColor = 0
	}
}

func (g *Game) mapBounds() (int, int) { return g.model.Map.Width, g.model.Map.Height }

// ensureEditableMap gives a 0x0 map the native 30x20 GBA-screen default so
// the map editor has cells to paint. A sheet with no complete 8x8 sprite
// cannot own a tile map (assetsync), so those projects stay empty.
func (g *Game) ensureEditableMap() {
	w, h := g.mapBounds()
	if w > 0 && h > 0 {
		return
	}
	if g.spriteN() == 0 {
		return
	}
	if err := g.model.ResizeMap(gbaScreenWidthTiles, gbaScreenHeightTiles); err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.dirty = true
}

func (g *Game) prepareMapView() {
	g.ensureEditableMap()
	w, h := g.mapBounds()
	if g.mapCam.Zoom == 0 {
		g.mapCam.Zoom = editor.MaxZoom
	}
	g.mapCam.Clamp(w, h, mapCanvasRect())
}

func (g *Game) refreshPreview() error {
	preview, err := g.model.Preview()
	if err != nil {
		return err
	}
	g.preview = preview
	return nil
}

// pixelAt reads an already-flattened sheet pixel (0 for out-of-range or a
// stale/missing preview, matching an unauthored pixel's stored value).
func (g *Game) pixelAt(x, y int) int {
	if g.objEditing && g.mode != modeMap && g.model != nil && g.model.ObjSheet != nil {
		sheet := g.model.ObjSheet
		if g.objDrawPixels != nil && x >= 0 && y >= 0 && x < sheet.Width && y < sheet.Height {
			return int(g.objDrawPixels[y*sheet.Width+x])
		}
		return spriteSheetPixel(*sheet, x, y)
	}
	if g.preview == nil || x < 0 || y < 0 || x >= g.preview.Width || y >= g.preview.Height {
		return 0
	}
	return int(g.preview.Pixels[y*g.preview.Width+x])
}

func spriteSheetPixel(sheet assetsync.Spritesheet, x, y int) int {
	if x < 0 || y < 0 || x >= sheet.Width || y >= sheet.Height {
		return 0
	}
	for _, sprite := range sheet.Sprites {
		if x < sprite.X || y < sprite.Y || x >= sprite.X+sprite.Width || y >= sprite.Y+sprite.Height {
			continue
		}
		return sprite.Pixels[y-sprite.Y][x-sprite.X]
	}
	return 0
}

func (g *Game) snapshot() undoSnapshot {
	return undoSnapshot{
		Fonts:              g.model.Fonts,
		Sheet:              g.model.Sheet,
		ObjSheet:           g.model.ObjSheet,
		Poses:              g.model.Poses,
		Animation:          g.model.Animation,
		ObjPage:            g.objPage,
		Map:                g.model.Map,
		TileLinks:          g.model.TileLinks,
		Project:            g.model.Project,
		Palette:            g.model.ActivePalette,
		CurrentSprite:      g.currentSprite,
		SelectedPose:       g.selectedPose,
		CurrentColor:       g.currentColor,
		GridCols:           g.gridCols,
		GridRows:           g.gridRows,
		NativeAudio:        g.model.NativeAudio,
		NativeChannel:      g.nativeChannel,
		NativeSong:         g.model.NativeSongIndex,
		NativeSequence:     g.nativeSequence,
		NativeBar:          g.nativeBar,
		NativeSelectedStep: g.nativeSelectedStep,
		NativeOctave:       g.nativeOctave,
		NativeNoteLength:   g.nativeNoteLength,
		NativeNoteTicks:    g.nativeNoteLengthTicks,
		NativeGridQuantize: uint8(g.nativeGridQuantize),
		NativeInstrument:   g.nativeInstrument,
		NativeAdvanced:     g.nativeAdvanced,
		NativeSFXMode:      g.nativeSFXMode,
		NativeSFXID:        g.nativeSFXID,
	}
}

func (g *Game) restore(s undoSnapshot) {
	currentSprite, currentColor := g.currentSprite, g.currentColor
	gridCols, gridRows := g.gridCols, g.gridRows
	preserveMapPaintingSelection := g.mode == modeMap

	g.stopAudioPreview()
	g.resetMapAreaInteraction()
	// Paste indices belong to the palette and sheet present at import time.
	// Undo/redo may replace either, so discard the transient preview together.
	if g.spritePaste != nil || g.copiedSprite.cut != nil {
		g.cancelSpritePastePlacement()
	}
	g.model.Sheet = s.Sheet
	if s.ObjSheet != nil {
		sheet := *s.ObjSheet
		g.model.ObjSheet = &sheet
	} else {
		g.model.ObjSheet = nil
	}
	if g.objEditing && g.model.ObjSheet == nil {
		g.objEditing = false
		g.setAnimationWorkspaceActive(false)
	}
	g.model.Poses = s.Poses
	g.model.Animation = s.Animation
	g.objPage = s.ObjPage
	g.objPageNameEditing = false
	// A draft belongs to the old selection; restoring a different pose must
	// never apply that draft to the newly selected pose on the next Enter.
	g.poseNameEditing, g.poseNameDraft = false, ""
	g.objUsage = nil
	g.resetAnimationEditorAfterRestore()
	g.model.Map = s.Map
	g.model.TileLinks = s.TileLinks
	g.model.NativeAudio = s.NativeAudio
	g.model.Project = s.Project
	g.model.Fonts = s.Fonts
	g.model.ActivePalette = s.Palette
	g.currentSprite = s.CurrentSprite
	g.selectedPose = s.SelectedPose
	g.currentColor = s.CurrentColor
	g.gridCols = s.GridCols
	g.gridRows = s.GridRows
	// In Map Lab this selection is the current paint brush, not authored map
	// data. Undo/redo must not silently switch the tile or stamp footprint.
	if preserveMapPaintingSelection {
		g.currentSprite, g.currentColor = currentSprite, currentColor
		g.gridCols, g.gridRows = gridCols, gridRows
	}
	g.nativeChannel = s.NativeChannel
	g.model.NativeSongIndex = s.NativeSong
	g.nativeSequence = s.NativeSequence
	g.nativeBar = s.NativeBar
	g.nativeSelectedStep = s.NativeSelectedStep
	g.nativeOctave = s.NativeOctave
	g.nativeNoteLength = s.NativeNoteLength
	g.nativeNoteLengthTicks = s.NativeNoteTicks
	g.nativeGridQuantize = performance.Quantize(s.NativeGridQuantize)
	g.nativeInstrument = s.NativeInstrument
	g.nativeAdvanced = s.NativeAdvanced
	g.nativeSFXMode = s.NativeSFXMode
	g.nativeSFXID = s.NativeSFXID
	if g.nativeNoteLength == 0 {
		g.nativeNoteLength = 1
	}
	if g.nativeNoteLengthTicks == 0 {
		g.nativeNoteLengthTicks = g.nativeNoteLength * editor.NativeTicksPerStep
	}
	g.clampCurrentColor()
	// A pasted variant (or a blank page) deliberately has no registered pose
	// selected. Redo must retain that, rather than selecting an unrelated pose.
	if g.selectedPose >= 0 {
		g.ensurePoseSelection()
	}
	g.fitSheetCamera()
	g.requestRender()
	if err := g.refreshPreview(); err != nil {
		g.statusMsg = "preview after undo/redo: " + err.Error()
	}
	if g.mode == modeMusic {
		g.refreshNativeMusicPreview()
		g.refreshNativePerformanceSounds()
	}
}

// persist saves to disk, matching the reference editor's timing: on every
// mode switch, not continuously while dragging pixels/tiles. Failures are
// surfaced in the status bar rather than treated as fatal, since an editing
// session should not be lost over a transient disk error.
func (g *Game) persist() error {
	if err := g.model.Save(); err != nil {
		g.statusMsg = "save failed: " + err.Error()
		log.Printf("save failed: %v", err)
		return err
	}
	g.markAnimationClipsSaved()
	g.queueProjectCheck()
	g.statusMsg = ""
	return nil
}

func (g *Game) Update() error {
	g.dirty = false
	g.pumpDevelopROMExport()
	g.updateRenderDemand()
	g.pollProjectCheck(time.Now())
	g.updateTheme()
	g.advanceAnimationPreview(time.Now())
	if g.fontEditor.open {
		g.updateFontEditor()
		return nil
	}
	if g.pcm.open {
		g.updatePCMEditor()
		return nil
	}
	if !g.preferencesOpen && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := editorCursorPosition()
		if rectContains(fontButtonRect(), mx, my) {
			g.openFontEditor()
			return nil
		}
		if rectContains(pcmButtonRect(), mx, my) {
			g.openPCMEditor()
			return nil
		}
	}
	if g.preferencesOpen {
		g.updatePreferences()
		return nil
	}
	if g.objPageNameEditing {
		g.updateObjPageName()
		return nil
	}
	if (g.objEditing || g.animationWorkspaceActive()) && g.updateAnimationNameTyping() {
		return nil
	}
	if g.animationWorkspaceActive() && g.updateAnimationWorkspaceTyping() {
		return nil
	}
	// Code owns the normal editing shortcuts while its document has focus.
	// Mouse tab changes remain available through handleModeNavigation.
	if g.mode == modeCode {
		if g.handleModeNavigation() {
			return nil
		}
		g.updateDevelopMode()
		return nil
	}
	g.handleUndoRedoKeys()
	g.handleSaveKey()
	if g.handleModeNavigation() {
		return nil
	}
	switch g.mode {
	case modeMap:
		g.updateMapMode()
	case modeMusic:
		g.updateMusicMode()
	default:
		g.updateSpriteMode()
	}
	if g.dirty {
		g.objUsage = nil
		g.invalidateAnimationPreview()
		if err := g.refreshPreview(); err != nil {
			g.statusMsg = err.Error()
		}
		g.undo.push(g.snapshot())
	}
	return nil
}

func (g *Game) handleSaveKey() {
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyS) {
		g.persist()
	}
}

func (g *Game) handleUndoRedoKeys() {
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	if !mod || !inpututil.IsKeyJustPressed(ebiten.KeyZ) {
		return
	}
	g.finishNativeNoteDrag()
	if ebiten.IsKeyPressed(ebiten.KeyShift) {
		if s, ok := g.undo.redo(); ok {
			g.restore(s)
		}
		return
	}
	if s, ok := g.undo.undo(); ok {
		g.restore(s)
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	if !ebiten.IsScreenClearedEveryFrame() && g.rendered && !g.renderRequested {
		return
	}
	g.rendered = true
	g.renderRequested = false
	screen.Fill(colorBackground)
	g.drawThemeBackdrop(screen)
	switch g.mode {
	case modeMap:
		g.drawMapMode(screen)
	case modeMusic:
		g.drawMusicMode(screen)
	case modeCode:
		g.drawDevelopMode(screen)
	default:
		g.drawSpriteMode(screen)
	}
	g.drawThemeOverlay(screen)
	g.drawEditorTabs(screen)
	g.drawStatusBar(screen)
	if g.fontEditor.open {
		g.drawFontEditor(screen)
	}
	if g.pcm.open {
		g.drawPCMEditor(screen)
	}
	if g.preferencesOpen {
		g.drawPreferences(screen)
	}
}

func (g *Game) drawStatusBar(screen *ebiten.Image) {
	fillRect(screen, statusBarRect, colorPanel)
	separator := rgba(modeAccent(g.mode), 155)
	if g.mode == modeSprites {
		separator = rgba(colorBorder, 140)
	}
	fillRect(screen, editor.Rect{X: 0, Y: statusBarRect.Y, W: statusBarRect.W, H: 1}, separator)
	mode := g.mode.label()
	lastSprite := max(0, g.spriteN()-1)
	msg := fmt.Sprintf("[%s]  sprite:%d/%d  tool:%s  (M/P/E/F/L/R/C: tools, Shift+H/V: flip, Cmd/Ctrl+S: save, Cmd/Ctrl+Z: undo)", mode, g.currentSprite, lastSprite, g.spriteTool.String())
	if g.mode == modeSprites && g.showsPoseStrip() {
		if g.selectedPose >= 0 && g.selectedPose < len(g.authoredPoses()) {
			msg = fmt.Sprintf("[%s]  tile:%d/%d  pose:%s  (Register Pose / Update Pose / Delete Pose, click name to rename)",
				mode, g.currentSprite, lastSprite, editor.PoseFrameLabel(g.selectedPose, g.authoredPoses()[g.selectedPose]))
		} else {
			msg = fmt.Sprintf("[%s]  tile:%d/%d  (select artwork, then Register Pose · Cmd/Ctrl+S: save)", mode, g.currentSprite, lastSprite)
		}
	}
	if g.mode == modeSprites && g.animationWorkspaceActive() {
		msg = "[animations]  Add registered poses, arrange their order, set speed, then Register Clip to use it in code."
	}
	if g.mode == modeMap {
		visible := ""
		for layer := 0; layer < assetsync.MapLayerCount; layer++ {
			if g.mapLayerVisible[layer] {
				visible += fmt.Sprintf("%d", layer)
			}
		}
		if visible == "" {
			visible = "none"
		}
		msg = fmt.Sprintf("[map editor]  BG%d  visible:%s  tool:%s  sprite:%d/%d  (1-4: layer/target, M: move, C: copy, Cmd/Ctrl+S: save)",
			g.mapActiveLayer, visible, g.mapTool.String(), g.currentSprite, lastSprite)
	}
	if g.mode == modeMusic {
		sections := 0
		if song, ok := g.model.NativeSong(); ok {
			sections = len(song.Sequence)
		}
		focus := "notes"
		if g.nativeFocus == nativeFocusArrangement {
			focus = "song order"
		}
		msg = fmt.Sprintf("[music lab]  part:%d/%d  CH%d  focus:%s  (Space: play/stop, Del: selection, Cmd/Ctrl+C/V: copy/paste)",
			min(g.nativeSequence+1, max(1, sections)), sections, g.nativeChannel+1, focus)
	}
	if g.mode == modeCode {
		line, column := g.code.LineColumn(g.code.Cursor())
		state := "saved"
		if g.code.Dirty() {
			state = "modified"
		}
		g.runMu.Lock()
		runStatus := g.runStatus
		g.runMu.Unlock()
		if runStatus == "" {
			runStatus = "READY"
		}
		focus := "CODE INPUT"
		if g.developGameFocus {
			focus = "GAME INPUT"
		}
		name := filepath.Base(g.codePath)
		if document := g.codeDocuments[g.codePath]; document != nil {
			name = document.Relative
		}
		msg = fmt.Sprintf("[develop]  %s  Ln %d, Col %d  %s  /  UTF-8  LF  LUA  /  %s  /  %s  (Cmd/Ctrl+P: files, Cmd/Ctrl+R: build)", name, line+1, column+1, state, runStatus, focus)
	}
	if g.mode == modeCode {
		msg = truncateDevelopText(msg, statusBarRect.W-16)
		drawDevelopText(screen, msg, statusBarRect.X+8, statusBarRect.Y+(statusBarRect.H-developTextHeight)/2, colorText)
	} else {
		msg = truncateUIText(msg, statusBarRect.W-16)
		drawText(screen, msg, statusBarRect.X+8, statusBarRect.Y+(statusBarRect.H-uiTextHeight)/2, colorText)
	}
	if g.mode == modeCode {
		message := newDevelopLayout().Message
		if g.statusMsg != "" {
			status := truncateDevelopText(g.statusMsg, message.W-16)
			drawDevelopText(screen, status, message.X+8, message.Y+(message.H-developTextHeight)/2, statusMessageColor(g.statusMsg))
		}
	} else if g.statusMsg != "" {
		status := truncateUIText(g.statusMsg, statusBarRect.W-16)
		drawText(screen, status, statusBarRect.X+8, statusBarRect.Y-uiTextHeight-6, statusMessageColor(g.statusMsg))
	}
}

func statusMessageColor(message string) color.Color {
	lower := strings.ToLower(message)
	for _, word := range []string{"error", "failed", "cannot", "outside", "invalid", "locked"} {
		if strings.Contains(lower, word) {
			return colorDanger
		}
	}
	return colorLCD
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Keep layout in logical window coordinates. editorWindow supplies the
	// physical framebuffer size separately; the two differ on HiDPI displays.
	// Never enlarge this logical layout beyond the available window space.
	width := max(1, outsideWidth)
	height := max(1, outsideHeight)
	if setEditorLayout(width, height) && g != nil && g.model != nil {
		g.requestRender()
		g.fitSheetCamera()
		mapW, mapH := g.mapBounds()
		g.mapCam.Clamp(mapW, mapH, mapCanvasRect())
	}
	return width, height
}

func (g *Game) requestRender() {
	if g != nil {
		g.renderRequested = true
	}
}

// updateRenderDemand separates Ebitengine's 60 Hz Update cadence from costly
// full-window rasterization. Audio/MIDI callbacks and the emulator do not use
// this gate, so reducing idle rendering cannot add audio latency.
func (g *Game) updateRenderDemand() {
	if g == nil {
		return
	}
	mx, my := editorCursorPosition()
	g.renderKeys = inpututil.AppendPressedKeys(g.renderKeys[:0])
	input := !g.renderCursorKnown || mx != g.renderCursorX || my != g.renderCursorY ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) ||
		ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) ||
		len(g.renderKeys) != 0
	g.renderCursorX, g.renderCursorY, g.renderCursorKnown = mx, my, true
	if wheelX, wheelY := ebiten.Wheel(); wheelX != 0 || wheelY != 0 {
		input = true
	}
	if input {
		// Keep one frame after release so tactile buttons and hover treatments
		// cannot remain painted in their pressed state.
		g.renderInputTail = 2
	} else if g.renderInputTail > 0 {
		g.renderInputTail--
	}

	if input || g.renderInputTail > 0 {
		g.renderIdleTicks = 0
		g.renderDynamicTicks = 0
		g.requestRender()
		return
	}
	if active, everyTick := g.dynamicSurfaceActivity(); active {
		g.renderIdleTicks = 0
		g.renderDynamicTicks++
		stride := 2 // 30 FPS is smooth for meters, playheads and REC notes.
		if everyTick {
			stride = 1 // The embedded GBA display must remain at 59.73 FPS.
		}
		if g.renderDynamicTicks >= stride {
			g.renderDynamicTicks = 0
			g.requestRender()
		}
		return
	}
	g.renderDynamicTicks = 0
	g.renderIdleTicks++
	if g.renderIdleTicks >= idleRedrawTicks {
		g.renderIdleTicks = 0
		g.requestRender()
	}
}

func (g *Game) dynamicSurfaceActivity() (active, everyTick bool) {
	if g.mode == modeMusic {
		if g.nativeMusicPreviewPlaying() || g.nativeCaptureVisible || g.nativeNoteDrag {
			return true, false
		}
		if g.nativeConductor != nil && g.nativeConductor.Active() {
			return true, false
		}
		if g.nativeSession != nil {
			snapshot := g.nativeSession.Snapshot()
			if snapshot.Recording {
				return true, false
			}
			for _, allocated := range snapshot.Allocation.Active {
				if allocated {
					return true, false
				}
			}
		}
	}
	if g.mode == modeCode {
		g.runMu.Lock()
		busy := g.runBusy
		g.runMu.Unlock()
		// A running, unpaused mGBA preview publishes a new frame at 59.73 Hz.
		// Keep its visual surface smooth while allowing a stopped editor to idle.
		playing := g.playStop != nil && !g.playPaused
		return active || busy || playing || g.romExport.ready != nil, playing
	}
	return active, false
}
