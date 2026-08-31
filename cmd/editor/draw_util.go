package main

import (
	"image"
	"image/color"
	"math"
	"strings"
	"sync"

	fontassets "github.com/drpaneas/mtile/fonts"
	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func truncateMusicEditText(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

// UI chrome colors. These are the tool's own interface colors, independent
// of the project's active palette, so the editor stays readable no matter
// which game palette is selected.
var (
	colorBackground     = color.RGBA{R: 16, G: 15, B: 28, A: 255}
	colorPanel          = color.RGBA{R: 35, G: 33, B: 57, A: 255}
	colorBorder         = color.RGBA{R: 119, G: 113, B: 162, A: 255}
	colorText           = color.RGBA{R: 235, G: 231, B: 250, A: 255}
	colorDim            = color.RGBA{R: 178, G: 171, B: 207, A: 255}
	colorHighlight      = color.RGBA{R: 255, G: 207, B: 69, A: 255}
	colorSprite         = color.RGBA{R: 82, G: 217, B: 208, A: 255}
	colorMap            = color.RGBA{R: 127, G: 190, B: 101, A: 255}
	colorMusic          = color.RGBA{R: 255, G: 133, B: 112, A: 255}
	colorCode           = color.RGBA{R: 143, G: 169, B: 255, A: 255}
	colorRun            = color.RGBA{R: 104, G: 219, B: 142, A: 255}
	colorInk            = color.RGBA{R: 9, G: 10, B: 20, A: 255}
	colorIndigo         = color.RGBA{R: 35, G: 33, B: 93, A: 255}
	colorLCD            = color.RGBA{R: 183, G: 212, B: 143, A: 255}
	colorDanger         = color.RGBA{R: 255, G: 95, B: 104, A: 255}
	colorCheckerA       = color.RGBA{R: 48, G: 46, B: 67, A: 255}
	colorCheckerB       = color.RGBA{R: 78, G: 73, B: 103, A: 255}
	colorSpriteCheckerA = color.RGBA{R: 18, G: 19, B: 33, A: 255}
	colorSpriteCheckerB = color.RGBA{R: 24, G: 24, B: 40, A: 255}
	colorSheetCheckerA  = color.RGBA{R: 12, G: 13, B: 23, A: 255}
	colorSheetCheckerB  = color.RGBA{R: 16, G: 16, B: 29, A: 255}
	colorGrid           = color.RGBA{R: 176, G: 174, B: 188, A: 255}
	colorShellBorder    = color.RGBA{R: 105, G: 91, B: 153, A: 255}
	colorButtonFace     = color.RGBA{R: 28, G: 27, B: 52, A: 255}
)

type uiFontStyle struct {
	face                   *text.GoXFace
	smallFace              *text.GoXFace
	titleFace              *text.GoXFace
	codeFace               *text.GoXFace
	developFace            *text.GoXFace
	developSmallFace       *text.GoXFace
	developTitleFace       *text.GoXFace
	size                   float64
	smallSize              float64
	titleSize              float64
	codeSize               float64
	charWidth              int
	textHeight             int
	smallTextHeight        int
	titleHeight            int
	codeCharWidth          int
	codeTextHeight         int
	developCharWidth       int
	developTextHeight      int
	developSmallTextHeight int
	developTitleHeight     int
	lineHeight             int
}

var (
	uiFontStyles           = loadUIFonts()
	uiFace                 = uiFontStyles[textComfort].face
	uiSmallFace            = uiFontStyles[textComfort].smallFace
	uiTitleFace            = uiFontStyles[textComfort].titleFace
	codeFace               = uiFontStyles[textComfort].codeFace
	developFace            = uiFontStyles[textComfort].developFace
	developSmallFace       = uiFontStyles[textComfort].developSmallFace
	developTitleFace       = uiFontStyles[textComfort].developTitleFace
	uiCharWidth            = uiFontStyles[textComfort].charWidth
	uiTextHeight           = uiFontStyles[textComfort].textHeight
	uiSmallTextHeight      = uiFontStyles[textComfort].smallTextHeight
	uiTitleHeight          = uiFontStyles[textComfort].titleHeight
	codeTextHeight         = uiFontStyles[textComfort].codeTextHeight
	developCharWidth       = uiFontStyles[textComfort].developCharWidth
	developTextHeight      = uiFontStyles[textComfort].developTextHeight
	developSmallTextHeight = uiFontStyles[textComfort].developSmallTextHeight
	developTitleHeight     = uiFontStyles[textComfort].developTitleHeight
)

// loadUIFonts gives each typography role its own measured face. Application
// chrome uses the neutral Go Sans family with the font rasterizer's native
// grayscale output. Only source code uses JetBrains Mono.
func loadUIFonts() [interfaceTextSizeCount]uiFontStyle {
	uiFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		panic("parse embedded Go Sans UI font: " + err.Error())
	}
	uiBoldFont, err := opentype.Parse(gobold.TTF)
	if err != nil {
		panic("parse embedded bold Go Sans UI font: " + err.Error())
	}
	codeFont, err := opentype.Parse(fontassets.JetBrainsMonoRegular)
	if err != nil {
		panic("parse embedded JetBrains Mono font: " + err.Error())
	}
	specs := [...]struct {
		uiSize, smallSize, titleSize, codeSize float64
		lineHeight                             int
	}{
		{uiSize: 12, smallSize: 10, titleSize: 18, codeSize: 12, lineHeight: 18},
		{uiSize: 13, smallSize: 11, titleSize: 20, codeSize: 14, lineHeight: 20},
		{uiSize: 14, smallSize: 12, titleSize: 22, codeSize: 16, lineHeight: 23},
	}
	var styles [interfaceTextSizeCount]uiFontStyle
	for index, spec := range specs {
		face, charWidth, textHeight := newCleanUIFontFace(uiFont, spec.uiSize)
		smallFace, _, smallTextHeight := newCleanUIFontFace(uiFont, spec.smallSize)
		titleFace, _, titleHeight := newCleanUIFontFace(uiBoldFont, spec.titleSize)
		codeFace, codeCharWidth, codeTextHeight := newHintedMonoFace(codeFont, spec.codeSize)
		developFace, developCharWidth, developTextHeight := newCleanUIFontFace(uiFont, spec.uiSize)
		developSmallFace, _, developSmallTextHeight := newCleanUIFontFace(uiFont, spec.smallSize)
		developTitleFace, _, developTitleHeight := newCleanUIFontFace(uiBoldFont, spec.titleSize)
		styles[index] = uiFontStyle{
			face:                   face,
			smallFace:              smallFace,
			titleFace:              titleFace,
			codeFace:               codeFace,
			developFace:            developFace,
			developSmallFace:       developSmallFace,
			developTitleFace:       developTitleFace,
			size:                   spec.uiSize,
			smallSize:              spec.smallSize,
			titleSize:              spec.titleSize,
			codeSize:               spec.codeSize,
			charWidth:              charWidth,
			textHeight:             textHeight,
			smallTextHeight:        smallTextHeight,
			titleHeight:            titleHeight,
			codeCharWidth:          codeCharWidth,
			codeTextHeight:         codeTextHeight,
			developCharWidth:       developCharWidth,
			developTextHeight:      developTextHeight,
			developSmallTextHeight: developSmallTextHeight,
			developTitleHeight:     developTitleHeight,
			lineHeight:             spec.lineHeight,
		}
	}
	return styles
}

func newHintedMonoFace(parsed *opentype.Font, size float64) (*text.GoXFace, int, int) {
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		panic("create hinted JetBrains Mono face: " + err.Error())
	}
	integerFace := &integerFontFace{Face: face}
	charWidth := font.MeasureString(integerFace, "M").Ceil()
	textHeight := integerFace.Metrics().Height.Ceil()
	return text.NewGoXFace(integerFace), charWidth, textHeight
}

// integerFontFace keeps the code editor's measured cell grid exact while
// preserving the font rasterizer's full grayscale coverage.
type integerFontFace struct {
	font.Face
}

func (f *integerFontFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	dot.X = fixed.I(dot.X.Round())
	dot.Y = fixed.I(dot.Y.Round())
	drawRect, mask, maskPoint, advance, ok := f.Face.Glyph(dot, r)
	return drawRect, mask, maskPoint, fixed.I(advance.Round()), ok
}

func (f *integerFontFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	bounds, advance, ok := f.Face.GlyphBounds(r)
	return bounds, fixed.I(advance.Round()), ok
}

func (f *integerFontFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	advance, ok := f.Face.GlyphAdvance(r)
	return fixed.I(advance.Round()), ok
}

func (f *integerFontFace) Kern(r0, r1 rune) fixed.Int26_6 {
	return fixed.I(f.Face.Kern(r0, r1).Round())
}

func newCleanUIFontFace(parsed *opentype.Font, size float64) (*text.GoXFace, int, int) {
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		panic("create Go Sans UI face: " + err.Error())
	}
	charWidth := font.MeasureString(face, "M").Ceil()
	textHeight := face.Metrics().Height.Ceil()
	return text.NewGoXFace(face), charWidth, textHeight
}

func setInterfaceTextSize(size interfaceTextSize) {
	if size >= interfaceTextSizeCount {
		size = textComfort
	}
	style := uiFontStyles[size]
	uiFace = style.face
	uiSmallFace = style.smallFace
	uiTitleFace = style.titleFace
	codeFace = style.codeFace
	developFace = style.developFace
	developSmallFace = style.developSmallFace
	developTitleFace = style.developTitleFace
	uiCharWidth = style.charWidth
	uiTextHeight = style.textHeight
	uiSmallTextHeight = style.smallTextHeight
	uiTitleHeight = style.titleHeight
	codeTextHeight = style.codeTextHeight
	developCharWidth = style.developCharWidth
	developTextHeight = style.developTextHeight
	developSmallTextHeight = style.developSmallTextHeight
	developTitleHeight = style.developTitleHeight
	codeLineHeight = style.lineHeight
	codeCharWidth = style.codeCharWidth
	setEditorLayout(screenWidth, screenHeight)
}

func drawText(screen *ebiten.Image, s string, x, y int, clr color.Color) {
	drawTextFace(screen, s, x, y, uiFace, clr)
}

func drawCodeText(screen *ebiten.Image, s string, x, y int, clr color.Color) {
	drawTextFace(screen, s, x, y, codeFace, clr)
}

func drawDevelopText(screen *ebiten.Image, s string, x, y int, clr color.Color) {
	drawTextFace(screen, s, x, y, developFace, clr)
}

func drawDevelopSmallText(screen *ebiten.Image, s string, x, y int, clr color.Color) {
	drawTextFace(screen, s, x, y, developSmallFace, clr)
}

func drawSmallText(screen *ebiten.Image, s string, x, y int, clr color.Color) {
	drawTextFace(screen, s, x, y, uiSmallFace, clr)
}

func drawTextFace(screen *ebiten.Image, s string, x, y int, face text.Face, clr color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(clr)
	text.Draw(screen, s, face, op)
}

func drawTextScale(screen *ebiten.Image, s string, x, y, scale int, clr color.Color) {
	if scale <= 1 {
		drawText(screen, s, x, y, clr)
		return
	}
	drawTextFace(screen, s, x, y, uiTitleFace, clr)
}

func drawDevelopTextScale(screen *ebiten.Image, s string, x, y, scale int, clr color.Color) {
	if scale <= 1 {
		drawDevelopText(screen, s, x, y, clr)
		return
	}
	drawTextFace(screen, s, x, y, developTitleFace, clr)
}

func uiTextWidth(value string) int {
	return int(math.Ceil(text.Advance(value, uiFace)))
}

func codeTextWidth(value string) int {
	return int(math.Ceil(text.Advance(value, codeFace)))
}

func developTextWidth(value string) int {
	return int(math.Ceil(text.Advance(value, developFace)))
}

func developSmallTextWidth(value string) int {
	return int(math.Ceil(text.Advance(value, developSmallFace)))
}

func smallTextWidth(value string) int {
	return int(math.Ceil(text.Advance(value, uiSmallFace)))
}

func drawTextCentered(screen *ebiten.Image, value string, rect editor.Rect, clr color.Color) {
	drawText(screen, value, rect.X+(rect.W-uiTextWidth(value))/2, rect.Y+(rect.H-uiTextHeight)/2, clr)
}

func drawCodeTextCentered(screen *ebiten.Image, value string, rect editor.Rect, clr color.Color) {
	drawCodeText(screen, value, rect.X+(rect.W-codeTextWidth(value))/2, rect.Y+(rect.H-codeTextHeight)/2, clr)
}

func drawDevelopTextCentered(screen *ebiten.Image, value string, rect editor.Rect, clr color.Color) {
	drawDevelopText(screen, value, rect.X+(rect.W-developTextWidth(value))/2, rect.Y+(rect.H-developTextHeight)/2, clr)
}

func drawDevelopSmallTextCentered(screen *ebiten.Image, value string, rect editor.Rect, clr color.Color) {
	drawDevelopSmallText(screen, value, rect.X+(rect.W-developSmallTextWidth(value))/2, rect.Y+(rect.H-developSmallTextHeight)/2, clr)
}

func drawSmallTextCentered(screen *ebiten.Image, value string, rect editor.Rect, clr color.Color) {
	drawSmallText(screen, value, rect.X+(rect.W-smallTextWidth(value))/2, rect.Y+(rect.H-uiSmallTextHeight)/2, clr)
}

func truncateUIText(value string, width int) string {
	return truncateTextToWidth(value, width, uiFace)
}

func truncateCodeText(value string, width int) string {
	return truncateTextToWidth(value, width, codeFace)
}

func truncateDevelopText(value string, width int) string {
	return truncateTextToWidth(value, width, developFace)
}

func truncateDevelopSmallText(value string, width int) string {
	return truncateTextToWidth(value, width, developSmallFace)
}

func truncateTextToWidth(value string, width int, face text.Face) string {
	value = strings.TrimSpace(value)
	if width <= 0 {
		return ""
	}
	if text.Advance(value, face) <= float64(width) {
		return value
	}
	runes := []rune(value)
	for len(runes) > 0 {
		candidate := string(runes) + "..."
		if text.Advance(candidate, face) <= float64(width) {
			return candidate
		}
		runes = runes[:len(runes)-1]
	}
	return ""
}

func fillRect(screen *ebiten.Image, r editor.Rect, clr color.Color) {
	if r.W <= 0 || r.H <= 0 {
		return
	}
	vector.FillRect(screen, float32(r.X), float32(r.Y), float32(r.W), float32(r.H), clr, false)
}

func strokeRect(screen *ebiten.Image, r editor.Rect, clr color.Color) {
	if r.W <= 0 || r.H <= 0 {
		return
	}
	vector.StrokeRect(screen, float32(r.X), float32(r.Y), float32(r.W), float32(r.H), 1, clr, false)
}

func fillRoundedRect(screen *ebiten.Image, r editor.Rect, radius int, clr color.Color) {
	_ = radius
	fillRect(screen, r, clr)
}

func strokeRoundedRect(screen *ebiten.Image, r editor.Rect, radius int, clr color.Color) {
	strokeRoundedRectWidth(screen, r, radius, 1, clr)
}

// Rounded-rectangle call sites are deliberately rendered as sharp desktop UI
// rectangles. Keeping the helper names avoids scattering presentation policy
// throughout the individual editors.
func strokeRoundedRectWidth(screen *ebiten.Image, r editor.Rect, radius, width int, clr color.Color) {
	_ = radius
	if screen == nil || r.W <= 0 || r.H <= 0 {
		return
	}
	for inset := 0; inset < max(1, width); inset++ {
		inner := editor.Rect{X: r.X + inset, Y: r.Y + inset, W: r.W - inset*2, H: r.H - inset*2}
		if inner.W <= 0 || inner.H <= 0 {
			break
		}
		strokeRect(screen, inner, clr)
	}
}

func drawLine(screen *ebiten.Image, x0, y0, x1, y1 int, clr color.Color) {
	vector.StrokeLine(screen, float32(x0), float32(y0), float32(x1), float32(y1), 1, clr, false)
}

func rectContains(r editor.Rect, x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// eraserIndex is palette color 0 and map sprite 0: transparent / erase.
const eraserIndex = 0

func isEraserIndex(id int) bool { return id == eraserIndex }

func checkerCellSize(r editor.Rect) int {
	n := min(r.W, r.H)
	switch {
	case n <= 0:
		return 1
	case n <= 2:
		return 1
	case n <= 8:
		return max(1, n/2)
	default:
		return 4
	}
}

func drawCheckerboard(screen *ebiten.Image, r editor.Rect) {
	drawCheckerboardColors(screen, r, colorCheckerA, colorCheckerB)
}

func drawCheckerboardColors(screen *ebiten.Image, r editor.Rect, checkerA, checkerB color.Color) {
	if screen == nil || r.W <= 0 || r.H <= 0 {
		return
	}
	cell := checkerCellSize(r)
	if shader, _ := checkerboardFillShader(); shader != nil {
		op := &ebiten.DrawRectShaderOptions{}
		op.GeoM.Translate(float64(r.X), float64(r.Y))
		op.Uniforms = map[string]any{
			"Cell":   float32(cell),
			"ColorA": shaderColor(checkerA),
			"ColorB": shaderColor(checkerB),
		}
		screen.DrawRectShader(r.W, r.H, shader, op)
		return
	}
	for y := r.Y; y < r.Y+r.H; y += cell {
		h := min(cell, r.Y+r.H-y)
		row := (y - r.Y) / cell
		for x := r.X; x < r.X+r.W; x += cell {
			w := min(cell, r.X+r.W-x)
			clr := checkerA
			if (row+(x-r.X)/cell)&1 == 0 {
				clr = checkerB
			}
			fillRect(screen, editor.Rect{X: x, Y: y, W: w, H: h}, clr)
		}
	}
}

const checkerboardShaderSource = `//kage:unit pixels

package main

var Cell float
var ColorA vec4
var ColorB vec4

func Fragment(dstPos vec4, srcPos vec2, colorScale vec4) vec4 {
	checker := mod(floor(dstPos.x/Cell)+floor(dstPos.y/Cell), 2)
	if checker < 1 {
		return ColorA
	}
	return ColorB
}
`

var (
	checkerShaderOnce sync.Once
	checkerShader     *ebiten.Shader
	checkerShaderErr  error
)

func checkerboardFillShader() (*ebiten.Shader, error) {
	checkerShaderOnce.Do(func() {
		// Keep the scalar fallback below for unusual graphics backends where
		// shader compilation is unavailable; desktop Metal takes this one-draw
		// path for every checker surface.
		checkerShader, checkerShaderErr = ebiten.NewShader([]byte(checkerboardShaderSource))
	})
	return checkerShader, checkerShaderErr
}

func shaderColor(clr color.Color) []float32 {
	r, g, b, a := clr.RGBA()
	return []float32{float32(r) / 0xffff, float32(g) / 0xffff, float32(b) / 0xffff, float32(a) / 0xffff}
}

// drawEraserSwatch is the palette/map-preview mark: checkerboard plus an X
// so index 0 does not look like a real black color or a real tile.
func drawEraserSwatch(screen *ebiten.Image, r editor.Rect) {
	drawCheckerboard(screen, r)
	if min(r.W, r.H) < 8 {
		return
	}
	drawLine(screen, r.X+1, r.Y+1, r.X+r.W-2, r.Y+r.H-2, colorDim)
	drawLine(screen, r.X+r.W-2, r.Y+1, r.X+1, r.Y+r.H-2, colorDim)
}

func drawEraserBadge(screen *ebiten.Image, r editor.Rect) {
	s := max(4, min(r.W, r.H)/4)
	badge := editor.Rect{X: r.X, Y: r.Y, W: s, H: s}
	drawEraserSwatch(screen, badge)
	strokeRect(screen, badge, colorHighlight)
}

// swatchColor converts a stored RGB15 (0..31 per channel) into a displayable
// RGBA by scaling each 5-bit component up to 8 bits.
func swatchColor(c assetsync.RGB15) color.Color {
	scale := func(v int) uint8 {
		if v < 0 {
			v = 0
		}
		if v > 31 {
			v = 31
		}
		return uint8(v * 255 / 31)
	}
	return color.RGBA{R: scale(c.R), G: scale(c.G), B: scale(c.B), A: 255}
}
