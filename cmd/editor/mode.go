package main

import (
	"fmt"
	"image/color"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type editorMode uint8

const (
	modeSprites editorMode = iota
	modeMap
	modeMusic
	modeCode
	editorModeCount
)

var primaryEditorModes = [...]editorMode{modeSprites, modeMap, modeMusic, modeCode}

func (m editorMode) label() string {
	switch m {
	case modeMap:
		return "map editor"
	case modeMusic:
		return "music lab"
	case modeCode:
		return "develop"
	default:
		return "sprite editor"
	}
}

func primaryModeLabel(mode editorMode) string {
	switch mode {
	case modeMap:
		return "map editor"
	case modeMusic:
		return "audio studio"
	case modeCode:
		return "develop"
	default:
		return "sprite editor"
	}
}

func primaryMode(mode editorMode) editorMode {
	return mode
}

func primaryModeIndex(mode editorMode) int {
	mode = primaryMode(mode)
	for index, candidate := range primaryEditorModes {
		if mode == candidate {
			return index
		}
	}
	return 0
}

func modeAccent(mode editorMode) color.Color {
	if mode == modeSprites {
		return colorSprite
	}
	if mode == modeMap {
		return colorMap
	}
	if mode == modeMusic {
		return colorMusic
	}
	if mode == modeCode {
		return colorCode
	}
	return colorHighlight
}

func nextEditorMode(mode editorMode) editorMode {
	index := primaryModeIndex(mode)
	return primaryEditorModes[(index+1)%len(primaryEditorModes)]
}

func editorTabRect(mode editorMode) editor.Rect {
	width := max(132, uiTextWidth("4  DEVELOP")+24)
	return editor.Rect{X: 126 + primaryModeIndex(mode)*width, Y: 5, W: width - 6, H: editorTabsHeight - 10}
}

func (g *Game) setMode(mode editorMode) {
	if mode >= editorModeCount {
		return
	}
	if mode == g.mode {
		if mode == modeCode {
			g.resumePlaySession()
		}
		return
	}
	if g.mode == modeMusic {
		g.finishNativeNoteDrag()
		g.stopAudioPreview()
		g.setNativeArmed(false)
	}
	if g.mode == modeCode {
		g.saveAllCode()
		g.developGameFocus = false
		g.suspendPlaySession()
	}
	if mode == modeMusic {
		wasNative := g.model.AudioVersion == assetsync.NativeAudioSchemaVersion
		if err := g.model.EnableNativeAudioV2(); err != nil {
			g.statusMsg = err.Error()
			return
		}
		if !wasNative {
			// The old graphics-only snapshot contains no valid v2 documents.
			// Make the initialized PSG model the new undo baseline so Undo can
			// never restore an invalid zero-value audio schema.
			g.undo.reset(g.snapshot())
		}
	}
	g.persist()
	g.mode = mode
	if mode == modeMap {
		g.prepareMapView()
	}
	if mode == modeMusic {
		g.clampNativeMusicSelection()
		g.refreshNativeMusicPreview()
		g.refreshNativePerformanceSounds()
		g.initNativeMIDI()
	}
	if mode == modeCode {
		g.resumePlaySession()
	}
}

func (g *Game) handleModeNavigation() bool {
	mode := editorModeCount
	handled := false
	textEntry := g.nativeNameTarget != 0 || g.mode == modeCode
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyF6):
		g.cycleEffects()
		return true
	case textEntry && !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft):
		return false
	case inpututil.IsKeyJustPressed(ebiten.Key1):
		mode = modeSprites
		handled = true
	case inpututil.IsKeyJustPressed(ebiten.Key2):
		mode = modeMap
		handled = true
	case inpututil.IsKeyJustPressed(ebiten.Key3):
		mode = modeMusic
		handled = true
	case inpututil.IsKeyJustPressed(ebiten.Key4):
		mode = modeCode
		handled = true
	case inpututil.IsKeyJustPressed(ebiten.KeyTab):
		mode = nextEditorMode(g.mode)
		handled = true
	case inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft):
		mx, my := ebiten.CursorPosition()
		if rectContains(themeEffectsRect(), mx, my) {
			g.preferencesOpen = true
			return true
		}
		for _, candidate := range primaryEditorModes {
			if rectContains(editorTabRect(candidate), mx, my) {
				mode = candidate
				handled = true
				break
			}
		}
	}
	if mode < editorModeCount {
		g.setMode(mode)
	}
	return handled
}

func (g *Game) drawEditorTabs(screen *ebiten.Image) {
	fillRect(screen, editorTabsRect, color.RGBA{R: 25, G: 22, B: 46, A: 255})
	fillRect(screen, editor.Rect{X: 0, Y: 0, W: editorTabsRect.W, H: 2}, colorShellHighlight)
	drawLine(screen, 0, editorTabsRect.H-1, editorTabsRect.W, editorTabsRect.H-1, colorBorder)
	brand := editor.Rect{X: 12, Y: 5, W: 104, H: editorTabsHeight - 10}
	fillRect(screen, brand, colorIndigo)
	strokeRect(screen, brand, colorShellBorder)
	drawTextCentered(screen, "MTILE", brand, colorText)
	fillRect(screen, editor.Rect{X: brand.X + brand.W - 13, Y: brand.Y + 8, W: 7, H: 7}, colorRun)
	for index, mode := range primaryEditorModes {
		r := editorTabRect(mode)
		if primaryMode(mode) == primaryMode(g.mode) {
			fillRect(screen, r, colorButtonFace)
		}
		strokeRect(screen, r, colorBorder)
		drawTextCentered(screen, fmt.Sprintf("%d  %s", index+1, primaryModeLabel(mode)), r, colorText)
	}
	active := editorTabRect(g.mode)
	fillRect(screen, editor.Rect{X: active.X, Y: active.Y + active.H - 3, W: active.W, H: 3}, modeAccent(g.mode))
	project := themeProjectRect()
	fillRect(screen, project, color.RGBA{R: 31, G: 28, B: 56, A: 255})
	strokeRect(screen, project, colorShellBorder)
	drawTextCentered(screen, truncateUIText(g.projectBadgeLabel(), project.W-16), project, colorDim)
	fx := themeEffectsRect()
	fillRect(screen, fx, colorButtonFace)
	strokeRect(screen, fx, colorShellHighlight)
	drawTextCentered(screen, "SETTINGS  ·  "+g.theme.level.String(), fx, colorText)
}
