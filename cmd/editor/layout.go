package main

import "github.com/drpaneas/mtile/internal/editor"

// The editor starts with a large desktop workspace and reflows when the user
// resizes it. Returning the real window dimensions from Game.Layout is what
// makes a 2K/4K window provide more editing room instead of merely stretching
// the same fixed canvas.
const (
	defaultScreenWidth  = 1920
	defaultScreenHeight = 1200
	minimumScreenWidth  = 1024
	minimumScreenHeight = 900
)

var (
	screenWidth  = defaultScreenWidth
	screenHeight = defaultScreenHeight
)

const (
	editorTabsHeight      = 36
	workspaceHeaderHeight = 72
	spriteHeaderHeight    = 64
)

// The map camera deliberately stops at 4x, but that makes compact 1x1 and
// 2x2 spritesheets only 32px and 64px wide in a large workspace. The sheet
// editor is a visual picker, so allow up to 16x (128px per 8x8 sprite cell)
// while retaining square pixels and fit-to-panel behavior.
const spriteSheetMaxFitZoom = 16.0

// GBA screen size in pixels/tiles, used only for the map editor's "native
// display preview" boundary overlay (see mapEditor.draw). It has nothing to
// do with a project's own sheet/map dimensions, which are project-defined
// and uncapped.
const (
	gbaScreenWidthTiles  = 30 // 240 / 8
	gbaScreenHeightTiles = 20 // 160 / 8
)

// Sprite editor layout (all fixed logical-pixel rects).
//
// The color tools sit under the sprite canvas. The spritesheet uses
// the full remaining width and a 3:2 grid canvas, so a native 30x20 sheet
// fills it with square cells exactly. Non-native sheet aspects are
// letterboxed rather than stretched.
const (
	spriteRailX            = 12
	spriteRailWidth        = 360
	spriteWorkspaceGap     = 12
	spritePanelPadding     = 12
	spriteSectionGap       = 10
	spritePanelTitleInsetX = 14
	spritePanelTitleInsetY = 10
	spritePanelContentGap  = 8
	spriteButtonHeight     = 30
	spriteCanvasFooterGap  = 8
)

// Every section starts its content after the measured heading line and the
// same breathing room. Panel heights use this offset too, so a larger text
// preference moves the controls without clipping them or shrinking the gap.
func spritePanelContentOffset() int {
	return spritePanelTitleInsetY + uiTextHeight + spritePanelContentGap
}

func spritePanelContentY(panel editor.Rect) int {
	return panel.Y + spritePanelContentOffset()
}

var (
	editorTabsRect         editor.Rect
	spriteHeaderRect       editor.Rect
	spriteRailRect         editor.Rect
	spriteCanvasPanelRect  editor.Rect
	spriteToolsPanelRect   editor.Rect
	spritePalettePanelRect editor.Rect
	spriteFlagsPanelRect   editor.Rect
	spritePickerPanelRect  editor.Rect
	canvasRect             editor.Rect
	spriteToolsRect        editor.Rect
	paletteRect            editor.Rect
	flagsRect              editor.Rect
	pickerRect             editor.Rect
	sheetPanelRect         editor.Rect
	statusBarRect          editor.Rect
	paletteSwatchWidth     = 21
	paletteSwatchHeight    = 16
)

func init() {
	setEditorLayout(defaultScreenWidth, defaultScreenHeight)
}

// setEditorLayout rebuilds every screen-relative rectangle. It returns true
// when the logical size changed so Game.Layout can refit only the views that
// are intended to follow the window (the sheet) and preserve map zoom/pan.
func setEditorLayout(width, height int) bool {
	// Keep the logical canvas identical to the actual window. Returning a
	// larger minimum canvas from Layout makes Ebitengine fractionally shrink
	// the complete frame, which blurs even perfectly hinted bitmap text.
	width = max(1, width)
	height = max(1, height)
	changed := width != screenWidth || height != screenHeight || editorTabsRect.W == 0
	screenWidth, screenHeight = width, height

	editorTabsRect = editor.Rect{X: 0, Y: 0, W: width, H: editorTabsHeight}
	statusBarHeight := max(28, uiTextHeight+10)
	statusBarRect = editor.Rect{X: 0, Y: height - statusBarHeight, W: width, H: statusBarHeight}
	spriteHeaderRect = editor.Rect{X: 0, Y: editorTabsHeight, W: width, H: spriteHeaderHeight}
	contentY := spriteHeaderRect.Y + spriteHeaderRect.H + 10
	spriteRailRect = editor.Rect{X: spriteRailX, Y: contentY, W: spriteRailWidth, H: max(1, statusBarRect.Y-contentY-10)}

	// Choose the rail topology from the actual space required by the desktop
	// stack. A raw 1000px breakpoint let intermediate windows create a nearly
	// zero-height CHANGE COLOR panel even though the compact composition fits.
	contentOffset := spritePanelContentOffset()
	canvasFooterHeight := spriteCanvasFooterGap + uiSmallTextHeight + spritePanelPadding
	buttonPanelHeight := contentOffset + spriteButtonHeight + spritePanelPadding
	pickerMinHeight := contentOffset + 2*pickerSliderRowHeight + pickerSliderHeight + spritePanelPadding
	desktopChromeHeight := contentOffset + canvasFooterHeight + 4*spriteSectionGap +
		2*buttonPanelHeight + (contentOffset + paletteSwatchColumns*16 + spritePanelPadding) + pickerMinHeight
	compactSpriteLayout := spriteRailRect.H < 320+desktopChromeHeight
	// Preserve a large desktop canvas at every text size. Use a slightly
	// smaller whole-tile canvas before falling back to the compact topology.
	canvasSize := min(336, (spriteRailRect.H-desktopChromeHeight)/8*8)
	paletteSwatchWidth = 21
	paletteSwatchHeight = 16
	if compactSpriteLayout {
		paletteSwatchWidth = 12
		paletteSwatchHeight = 12
		compactChromeHeight := contentOffset + canvasFooterHeight + 3*spriteSectionGap +
			2*buttonPanelHeight + contentOffset + paletteSwatchColumns*paletteSwatchHeight + spritePanelPadding
		canvasSize = max(8, min(240, (spriteRailRect.H-compactChromeHeight)/8*8))
	}

	spriteCanvasPanelRect = editor.Rect{X: spriteRailRect.X, Y: spriteRailRect.Y, W: spriteRailRect.W, H: contentOffset + canvasSize + canvasFooterHeight}
	canvasRect = editor.Rect{
		X: spriteCanvasPanelRect.X + spritePanelPadding,
		Y: spritePanelContentY(spriteCanvasPanelRect),
		W: canvasSize,
		H: canvasSize,
	}

	spriteToolsPanelRect = editor.Rect{
		X: spriteRailRect.X,
		Y: spriteCanvasPanelRect.Y + spriteCanvasPanelRect.H + spriteSectionGap,
		W: spriteRailRect.W,
		H: buttonPanelHeight,
	}
	spriteToolsRect = editor.Rect{
		X: spriteToolsPanelRect.X + spritePanelPadding,
		Y: spritePanelContentY(spriteToolsPanelRect),
		W: spriteToolsPanelRect.W - spritePanelPadding*2,
		H: spriteButtonHeight,
	}

	paletteGridWidth := paletteSwatchColumns * paletteSwatchWidth
	paletteGridHeight := paletteSwatchColumns * paletteSwatchHeight
	spritePalettePanelRect = editor.Rect{
		X: spriteRailRect.X,
		Y: spriteToolsPanelRect.Y + spriteToolsPanelRect.H + spriteSectionGap,
		W: spriteRailRect.W,
		H: contentOffset + paletteGridHeight + spritePanelPadding,
	}
	paletteRect = editor.Rect{
		X: spritePalettePanelRect.X + spritePanelPadding,
		Y: spritePanelContentY(spritePalettePanelRect),
		W: paletteGridWidth,
		H: paletteGridHeight,
	}

	if compactSpriteLayout {
		pickerX := paletteRect.X + paletteRect.W + 12
		pickerRect = editor.Rect{
			X: pickerX,
			Y: spritePalettePanelRect.Y + spritePanelTitleInsetY,
			W: max(1, spritePalettePanelRect.X+spritePalettePanelRect.W-pickerX-spritePanelPadding),
			H: spritePalettePanelRect.H - 2*spritePanelTitleInsetY,
		}
		spritePickerPanelRect = editor.Rect{}
		spriteFlagsPanelRect = editor.Rect{
			X: spriteRailRect.X,
			Y: spritePalettePanelRect.Y + spritePalettePanelRect.H + spriteSectionGap,
			W: spriteRailRect.W,
			H: max(buttonPanelHeight, statusBarRect.Y-(spritePalettePanelRect.Y+spritePalettePanelRect.H)-spriteSectionGap-10),
		}
	} else {
		spriteFlagsPanelRect = editor.Rect{
			X: spriteRailRect.X,
			Y: spritePalettePanelRect.Y + spritePalettePanelRect.H + spriteSectionGap,
			W: spriteRailRect.W,
			H: buttonPanelHeight,
		}
		spritePickerPanelRect = editor.Rect{
			X: spriteRailRect.X,
			Y: spriteFlagsPanelRect.Y + spriteFlagsPanelRect.H + spriteSectionGap,
			W: spriteRailRect.W,
			H: max(1, statusBarRect.Y-(spriteFlagsPanelRect.Y+spriteFlagsPanelRect.H)-spriteSectionGap-10),
		}
		pickerRect = editor.Rect{
			X: spritePickerPanelRect.X + spritePanelTitleInsetX,
			Y: spritePickerPanelRect.Y + spritePanelTitleInsetY,
			W: spritePickerPanelRect.W - spritePanelTitleInsetX*2,
			H: max(1, spritePickerPanelRect.H-2*spritePanelTitleInsetY),
		}
	}
	flagsRect = editor.Rect{
		X: spriteFlagsPanelRect.X + spritePanelPadding,
		Y: spritePanelContentY(spriteFlagsPanelRect),
		W: spriteFlagsPanelRect.W - spritePanelPadding*2,
		H: max(1, spriteFlagsPanelRect.H-contentOffset-spritePanelPadding),
	}

	sheetPanelX := spriteRailRect.X + spriteRailRect.W + spriteWorkspaceGap
	sheetPanelRect = editor.Rect{
		X: sheetPanelX,
		Y: contentY,
		W: max(1, width-sheetPanelX-12),
		H: max(1, statusBarRect.Y-contentY-10),
	}
	return changed
}

// Map editor layout.
const (
	mapPadding           = 10
	mapGap               = 8
	mapStripHeight       = 52
	mapPreviewSize       = 28
	mapLayerToolbarWidth = 560
	mapLayerLabelWidth   = 150
	mapLayerButtonGap    = 6
	mapMoveToolGap       = 12
)

type mapLayout struct {
	Header       editor.Rect
	Canvas       editor.Rect
	Strip        editor.Rect
	SelectedTile editor.Rect
	MoveArea     editor.Rect
	CopyArea     editor.Rect
	Coordinates  editor.Rect
	Layers       editor.Rect
	LayerRows    [4]editor.Rect
	LayerEyes    [4]editor.Rect
}

func newMapLayout(screenW, screenH int) mapLayout {
	header := editor.Rect{X: 0, Y: editorTabsHeight, W: screenW, H: workspaceHeaderHeight}
	usableH := screenH - statusBarRect.H - editorTabsHeight - workspaceHeaderHeight
	canvasH := usableH - (mapPadding * 2) - mapGap - mapStripHeight
	canvas := editor.Rect{X: mapPadding, Y: header.Y + header.H + mapPadding, W: screenW - mapPadding*2, H: canvasH}
	strip := editor.Rect{X: mapPadding, Y: canvas.Y + canvas.H + mapGap, W: canvas.W, H: mapStripHeight}
	layersW := min(mapLayerToolbarWidth, max(500, screenW/2))
	layers := editor.Rect{X: strip.X + strip.W - layersW - 6, Y: strip.Y + 6, W: layersW, H: strip.H - 12}
	selected := editor.Rect{X: strip.X + mapGap, Y: strip.Y + mapGap, W: mapPreviewSize, H: mapPreviewSize}
	areaToolW := max(128, max(uiTextWidth("MOVE AREA [M]"), uiTextWidth("COPY AREA [C]"))+20)
	moveArea := editor.Rect{X: selected.X + selected.W + mapMoveToolGap, Y: selected.Y, W: areaToolW, H: selected.H}
	copyArea := editor.Rect{X: moveArea.X + moveArea.W + mapLayerButtonGap, Y: moveArea.Y, W: areaToolW, H: moveArea.H}
	coordX := copyArea.X + copyArea.W + mapMoveToolGap
	coords := editor.Rect{X: coordX, Y: strip.Y + (strip.H-20)/2, W: max(1, layers.X-coordX-mapGap), H: 20}
	layout := mapLayout{Header: header, Canvas: canvas, Strip: strip, SelectedTile: selected, MoveArea: moveArea, CopyArea: copyArea, Coordinates: coords, Layers: layers}
	buttonsX := layers.X + mapLayerLabelWidth
	buttonW := (layers.W - mapLayerLabelWidth - 3*mapLayerButtonGap) / 4
	for bg := 0; bg < 4; bg++ {
		row := editor.Rect{X: buttonsX + bg*(buttonW+mapLayerButtonGap), Y: layers.Y, W: buttonW, H: layers.H}
		layout.LayerRows[bg] = row
		layout.LayerEyes[bg] = editor.Rect{X: row.X + 5, Y: row.Y + (row.H-24)/2, W: 26, H: 24}
	}
	return layout
}
