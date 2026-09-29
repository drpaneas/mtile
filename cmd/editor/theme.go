package main

import (
	"image/color"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
)

// effectLevel is retained in the preferences schema so older editor.json files
// remain readable. Application chrome is always rendered at effectsClean.
type effectLevel uint8

const (
	effectsClean effectLevel = iota
	effectsSoft
	effectsFull
	effectLevelCount
)

func (e effectLevel) String() string {
	switch e {
	case effectsClean:
		return "CLEAN"
	case effectsFull:
		return "FULL"
	default:
		return "SOFT"
	}
}

type themeState struct {
	frame uint64
	level effectLevel

	buildPulse int

	cursorX float64
	cursorY float64

	inkPulse int
	inkX     int
	inkY     int

	mapPulse int
	mapX     int
	mapY     int
	mapVX    float64
	mapVY    float64

	codeCaretX      float64
	codeCaretY      float64
	completionFrame int
}

func (g *Game) initTheme() {
	g.theme.level = effectsClean
	mx, my := ebiten.CursorPosition()
	g.theme.cursorX, g.theme.cursorY = float64(mx), float64(my)
}

func (g *Game) updateTheme() {
	g.theme.frame++
	mx, my := ebiten.CursorPosition()
	g.theme.level = effectsClean
	g.theme.cursorX, g.theme.cursorY = float64(mx), float64(my)
	g.theme.buildPulse = 0
	g.theme.inkPulse = 0
	g.theme.mapPulse = 0
	if g.codeCompletionOpen {
		g.theme.completionFrame = min(8, g.theme.completionFrame+1)
	} else {
		g.theme.completionFrame = 0
	}
	if g.mode == modeCode && g.code != nil {
		layout := newDevelopPanelLayout(g.developPanel).Code
		line, column := g.code.LineColumn(g.code.Cursor())
		x := float64(layout.Text.X + 8 + (column-g.codeScrollColumn)*codeCharWidth)
		y := float64(layout.Text.Y + 3 + (line-g.codeScrollLine)*codeLineHeight)
		g.theme.codeCaretX, g.theme.codeCaretY = x, y
	}
}

func modeSurfaceColor(mode editorMode) color.Color {
	switch mode {
	case modeSprites:
		return color.RGBA{R: 17, G: 24, B: 36, A: 255}
	case modeMap:
		return color.RGBA{R: 18, G: 28, B: 31, A: 255}
	case modeMusic:
		return color.RGBA{R: 21, G: 22, B: 61, A: 255}
	case modeCode:
		return color.RGBA{R: 14, G: 16, B: 30, A: 255}
	default:
		return colorBackground
	}
}

func (g *Game) drawThemeBackdrop(screen *ebiten.Image) {
	workspace := editor.Rect{X: 0, Y: editorTabsHeight, W: screenWidth, H: statusBarRect.Y - editorTabsHeight}
	fillRect(screen, workspace, modeSurfaceColor(g.mode))
}

func (g *Game) drawThemeOverlay(_ *ebiten.Image) {}

func drawHardwarePanel(screen *ebiten.Image, rect editor.Rect, fill, accent color.Color) {
	if rect.W <= 0 || rect.H <= 0 {
		return
	}
	_ = accent
	fillRect(screen, rect, fill)
	strokeRect(screen, rect, colorBorder)
}

func rgba(clr color.Color, alpha uint8) color.Color {
	r, g, b, a := clr.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(uint32(a>>8) * uint32(alpha) / 255)}
}

func drawTactileButton(screen *ebiten.Image, rect editor.Rect, label string, active bool, accent color.Color, compact bool) {
	drawTactileButtonWithTypography(screen, rect, label, active, accent, compact, false)
}

func drawDevelopTactileButton(screen *ebiten.Image, rect editor.Rect, label string, active bool, accent color.Color, compact bool) {
	drawTactileButtonWithTypography(screen, rect, label, active, accent, compact, true)
}

func drawDevelopButton(screen *ebiten.Image, rect editor.Rect, label string, active bool, accent color.Color) {
	drawDevelopTactileButton(screen, rect, label, active, accent, false)
}

func drawTactileButtonWithTypography(screen *ebiten.Image, rect editor.Rect, label string, active bool, accent color.Color, compact, develop bool) {
	mx, my := ebiten.CursorPosition()
	pressed := rectContains(rect, mx, my) && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	face := rect
	fill := color.Color(colorButtonFace)
	textColor := color.Color(colorText)
	if active {
		fill = color.RGBA{R: 50, G: 48, B: 104, A: 255}
		textColor = accent
	} else if pressed {
		fill = color.RGBA{R: 39, G: 38, B: 67, A: 255}
	}
	fillRect(screen, face, fill)
	border := color.Color(colorBorder)
	if active {
		border = accent
	} else if rectContains(rect, mx, my) {
		border = accent
	}
	strokeRect(screen, face, border)
	if develop {
		if compact {
			drawDevelopSmallTextCentered(screen, label, face, textColor)
		} else {
			drawDevelopTextCentered(screen, label, face, textColor)
		}
		return
	}
	if compact {
		drawSmallTextCentered(screen, label, face, textColor)
	} else {
		drawTextCentered(screen, label, face, textColor)
	}
}

func (g *Game) flashInk(x, y int) {
	if g.theme.level == effectsClean {
		return
	}
	g.theme.inkPulse, g.theme.inkX, g.theme.inkY = 12, x, y
}

func (g *Game) flashMap(x, y int) {
	if g.theme.level == effectsClean {
		return
	}
	g.theme.mapPulse, g.theme.mapX, g.theme.mapY = 14, x, y
}

func drawWorkspaceHeader(screen *ebiten.Image, r editor.Rect, title, subtitle string, accent color.Color) {
	drawWorkspaceHeaderWithTypography(screen, r, title, subtitle, accent, false)
}

func drawDevelopWorkspaceHeader(screen *ebiten.Image, r editor.Rect, title, subtitle string, accent color.Color) {
	drawWorkspaceHeaderWithTypography(screen, r, title, subtitle, accent, true)
}

func drawWorkspaceHeaderWithTypography(screen *ebiten.Image, r editor.Rect, title, subtitle string, accent color.Color, develop bool) {
	fillRect(screen, r, color.RGBA{R: 29, G: 31, B: 78, A: 255})
	fillRect(screen, editor.Rect{X: r.X, Y: r.Y, W: r.W, H: 3}, accent)
	strokeRect(screen, r, colorBorder)
	titleX := r.X + 260
	if r.W >= 1400 {
		logoY := r.Y + max(8, (r.H-48)/2)
		drawMtileHeaderLogo(screen, r.X+16, logoY, 220, 48)
	} else {
		titleX = r.X + 18
	}
	titleY := r.Y + 4
	if develop {
		drawDevelopTextScale(screen, title, titleX, titleY, 2, accent)
	} else {
		drawTextScale(screen, title, titleX, titleY, 2, accent)
	}
	textHeight := uiTextHeight
	titleHeight := uiTitleHeight
	if develop {
		textHeight = developTextHeight
		titleHeight = developTitleHeight
	}
	subtitleY := r.Y + r.H - textHeight - 4
	if r.W < 1400 && r.H > 84 {
		if develop {
			drawDevelopSmallText(screen, subtitle, titleX, r.Y+titleHeight+2, rgba(accent, 220))
		} else {
			drawSmallText(screen, subtitle, titleX, r.Y+titleHeight+2, rgba(accent, 220))
		}
		return
	}
	if develop {
		drawDevelopText(screen, subtitle, titleX, subtitleY, rgba(accent, 220))
	} else {
		drawText(screen, subtitle, titleX, subtitleY, rgba(accent, 220))
	}
}
