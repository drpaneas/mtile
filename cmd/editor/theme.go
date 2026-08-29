package main

import (
	"image/color"
	"sync"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
)

// effectLevel keeps the nostalgic presentation adjustable. The theme itself
// remains consistent in every mode; this setting controls only motion and
// display treatment so a developer can always choose a completely still UI.
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

	playLCD bool
}

func (g *Game) initTheme() {
	g.theme.level = effectsSoft
	g.theme.playLCD = true
	mx, my := ebiten.CursorPosition()
	g.theme.cursorX, g.theme.cursorY = float64(mx), float64(my)
}

func (g *Game) updateTheme() {
	g.theme.frame++
	mx, my := ebiten.CursorPosition()
	if g.theme.level == effectsClean {
		g.theme.cursorX, g.theme.cursorY = float64(mx), float64(my)
	} else {
		g.theme.cursorX += (float64(mx) - g.theme.cursorX) * .18
		g.theme.cursorY += (float64(my) - g.theme.cursorY) * .18
	}

	if g.theme.buildPulse > 0 {
		g.theme.buildPulse--
	}
	if g.theme.inkPulse > 0 {
		g.theme.inkPulse--
	}
	if g.theme.mapPulse > 0 {
		g.theme.mapPulse--
	}
	if g.codeCompletionOpen {
		g.theme.completionFrame = min(8, g.theme.completionFrame+1)
	} else {
		g.theme.completionFrame = 0
	}
	if g.mode == modeCode && g.code != nil {
		layout := newDevelopLayout().Code
		line, column := g.code.LineColumn(g.code.Cursor())
		x := float64(layout.Text.X + 8 + (column-g.codeScrollColumn)*codeCharWidth)
		y := float64(layout.Text.Y + 3 + (line-g.codeScrollLine)*codeLineHeight)
		if g.theme.codeCaretX == 0 && g.theme.codeCaretY == 0 || g.theme.level == effectsClean {
			g.theme.codeCaretX, g.theme.codeCaretY = x, y
		} else {
			g.theme.codeCaretX += (x - g.theme.codeCaretX) * .55
			g.theme.codeCaretY += (y - g.theme.codeCaretY) * .55
		}
	}
}

func (g *Game) cycleEffects() {
	g.theme.level = (g.theme.level + 1) % effectLevelCount
	g.preferences.Effects = g.theme.level
	if g.preferences.Version == editorPreferencesVersion {
		g.savePreferences()
	}
	g.statusMsg = "visual effects: " + g.theme.level.String()
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
	// Sprite Lab's panel borders already define its workspace. Keep its outer
	// shell flat so the canvas remains the visual focus.
	if g.mode != modeSprites {
		fillRect(screen, editor.Rect{X: 0, Y: workspace.Y, W: 5, H: workspace.H}, modeAccent(g.mode))
		fillRect(screen, editor.Rect{X: 5, Y: workspace.Y, W: 3, H: workspace.H}, colorShellGlow)
		fillRect(screen, editor.Rect{X: 0, Y: statusBarRect.Y - 2, W: screenWidth, H: 2}, colorShellGlow)
	}
	if g.theme.level == effectsClean {
		return
	}
	// Cursor-reactive edge light. It is deliberately confined to the shell,
	// never drawn over code, notes, sprites, or labels.
	lightX := max(8, min(screenWidth-168, int(g.theme.cursorX)-80))
	fillRect(screen, editor.Rect{X: lightX, Y: editorTabsHeight, W: 160, H: 1}, rgba(colorShellHighlight, 72))
}

func (g *Game) drawThemeOverlay(screen *ebiten.Image) {
	if g.theme.buildPulse > 0 {
		g.drawBuildBus(screen)
	}
}

func (g *Game) drawBuildBus(screen *ebiten.Image) {
	develop := editorTabRect(modeCode)
	y := editorTabsHeight + 8
	x0 := develop.X + 8
	x1 := develop.X + develop.W - 8
	drawLine(screen, x0, y, x1, y, rgba(colorRun, 118))
	progress := 1 - float64(g.theme.buildPulse)/48
	progress = max(0.0, min(1.0, progress))
	x := x0 + int(float64(x1-x0)*progress)
	fillRect(screen, editor.Rect{X: x - 4, Y: y - 4, W: 9, H: 9}, colorRun)
	fillRect(screen, editor.Rect{X: x - 8, Y: y - 1, W: 17, H: 3}, rgba(colorRun, 92))
}

func drawHardwarePanel(screen *ebiten.Image, rect editor.Rect, fill, accent color.Color) {
	if rect.W <= 0 || rect.H <= 0 {
		return
	}
	fillRoundedRect(screen, editor.Rect{X: rect.X + 4, Y: rect.Y + 5, W: rect.W, H: rect.H}, 9, color.RGBA{R: 6, G: 7, B: 15, A: 210})
	fillRoundedRect(screen, rect, 9, fill)
	strokeRoundedRect(screen, rect, 9, colorShellBorder)
	drawLine(screen, rect.X+9, rect.Y+1, rect.X+rect.W-10, rect.Y+1, rgba(accent, 150))
	drawLine(screen, rect.X+9, rect.Y+rect.H-2, rect.X+rect.W-10, rect.Y+rect.H-2, colorShellShadow)
	if rect.W >= 180 && rect.H >= 90 {
		drawThemeScrew(screen, rect.X+8, rect.Y+8)
		drawThemeScrew(screen, rect.X+rect.W-9, rect.Y+rect.H-9)
	}
}

func drawThemeScrew(screen *ebiten.Image, x, y int) {
	fillRect(screen, editor.Rect{X: x - 2, Y: y - 2, W: 5, H: 5}, colorShellShadow)
	drawLine(screen, x-1, y, x+1, y, colorShellHighlight)
}

func rgba(clr color.Color, alpha uint8) color.Color {
	r, g, b, a := clr.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(uint32(a>>8) * uint32(alpha) / 255)}
}

func drawTactileButton(screen *ebiten.Image, rect editor.Rect, label string, active bool, accent color.Color, compact bool) {
	mx, my := ebiten.CursorPosition()
	pressed := rectContains(rect, mx, my) && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	offset := 0
	if pressed {
		offset = 2
	}
	face := editor.Rect{X: rect.X, Y: rect.Y + offset, W: rect.W, H: rect.H - offset}
	fillRoundedRect(screen, editor.Rect{X: rect.X + 2, Y: rect.Y + 3, W: rect.W, H: rect.H}, 6, colorShellShadow)
	fill := color.Color(colorButtonFace)
	textColor := color.Color(colorText)
	if active {
		fill = color.RGBA{R: 50, G: 48, B: 104, A: 255}
		textColor = accent
	}
	fillRoundedRect(screen, face, 6, fill)
	border := color.Color(colorBorder)
	if active {
		border = accent
	} else if rectContains(rect, mx, my) {
		border = rgba(accent, 165)
	}
	strokeRoundedRect(screen, face, 6, border)
	if !pressed {
		drawLine(screen, face.X+1, face.Y+1, face.X+face.W-2, face.Y+1, rgba(colorShellHighlight, 120))
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
	drawTextScale(screen, title, titleX, titleY, 2, accent)
	subtitleY := r.Y + r.H - uiTextHeight - 4
	if r.W < 1400 && r.H > 84 {
		drawSmallText(screen, subtitle, titleX, r.Y+uiTitleHeight+2, rgba(accent, 220))
		return
	}
	drawText(screen, subtitle, titleX, subtitleY, rgba(accent, 220))
}

const lcdShaderSource = `//kage:unit pixels

package main

var Strength float
var Time float

func Fragment(dstPos vec4, srcPos vec2, colorScale vec4) vec4 {
	origin := imageSrc0Origin()
	size := imageSrc0Size()
	p := srcPos - origin
	uv := p / size
	samplePos := floor(p) + origin + vec2(0.5)
	c := imageSrc0At(samplePos)

	scan := 1.0 - Strength * (0.035 + 0.025*cos(p.y*3.14159265))
	column := mod(floor(p.x), 3.0)
	mask := vec3(0.94)
	if column < 1.0 {
		mask.r = 1.0
	} else if column < 2.0 {
		mask.g = 1.0
	} else {
		mask.b = 1.0
	}
	d := uv - vec2(0.5)
	vignette := 1.0 - Strength * 0.20 * dot(d, d) * 2.5
	flicker := 1.0 + Strength * 0.006 * sin(Time*2.1 + p.y*0.17)
	c.rgb *= scan * mix(vec3(1.0), mask, Strength*0.45) * vignette * flicker
	return c * colorScale
}
`

var (
	lcdShaderOnce sync.Once
	lcdShader     *ebiten.Shader
	lcdShaderErr  error
)

func playLCDShader() (*ebiten.Shader, error) {
	lcdShaderOnce.Do(func() {
		lcdShader, lcdShaderErr = ebiten.NewShader([]byte(lcdShaderSource))
	})
	return lcdShader, lcdShaderErr
}
