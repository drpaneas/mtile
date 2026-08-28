package main

import (
	"image"
	"image/color"
	"math"
	"strings"

	fontassets "github.com/drpaneas/mtile/fonts"
	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"
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
	colorShellGlow      = color.RGBA{R: 89, G: 63, B: 144, A: 255}
	colorShellBorder    = color.RGBA{R: 105, G: 91, B: 153, A: 255}
	colorShellHighlight = color.RGBA{R: 191, G: 172, B: 238, A: 255}
	colorShellShadow    = color.RGBA{R: 7, G: 6, B: 17, A: 255}
	colorButtonFace     = color.RGBA{R: 28, G: 27, B: 52, A: 255}
)

type uiFontStyle struct {
	face            *text.GoXFace
	smallFace       *text.GoXFace
	titleFace       *text.GoXFace
	codeFace        *text.GoXFace
	size            float64
	smallSize       float64
	titleSize       float64
	codeSize        float64
	charWidth       int
	textHeight      int
	smallTextHeight int
	titleHeight     int
	codeCharWidth   int
	codeTextHeight  int
	lineHeight      int
}

var (
	uiFontStyles      = loadUIFonts()
	uiFace            = uiFontStyles[textComfort].face
	uiSmallFace       = uiFontStyles[textComfort].smallFace
	uiTitleFace       = uiFontStyles[textComfort].titleFace
	codeFace          = uiFontStyles[textComfort].codeFace
	uiCharWidth       = uiFontStyles[textComfort].charWidth
	uiTextHeight      = uiFontStyles[textComfort].textHeight
	uiSmallTextHeight = uiFontStyles[textComfort].smallTextHeight
	uiTitleHeight     = uiFontStyles[textComfort].titleHeight
	codeTextHeight    = uiFontStyles[textComfort].codeTextHeight
)

// loadUIFonts gives each typography role its own measured face. Chrome uses
// proportional OpenDyslexic Bold, while source and logs use OpenDyslexic Mono.
// Every face is rasterized at its final size with full hinting; display text
// is never produced by scaling a smaller glyph atlas.
func loadUIFonts() [interfaceTextSizeCount]uiFontStyle {
	uiFont, err := opentype.Parse(fontassets.OpenDyslexicUIBold)
	if err != nil {
		panic("parse embedded bold OpenDyslexic UI font: " + err.Error())
	}
	codeFont, err := opentype.Parse(fontassets.OpenDyslexic)
	if err != nil {
		panic("parse embedded OpenDyslexic Mono font: " + err.Error())
	}
	specs := [...]struct {
		uiSize, smallSize, titleSize, codeSize float64
		lineHeight                             int
	}{
		{uiSize: 12, smallSize: 10, titleSize: 18, codeSize: 16, lineHeight: 30},
		{uiSize: 13, smallSize: 11, titleSize: 20, codeSize: 18, lineHeight: 34},
		{uiSize: 14, smallSize: 12, titleSize: 22, codeSize: 20, lineHeight: 38},
	}
	var styles [interfaceTextSizeCount]uiFontStyle
	for index, spec := range specs {
		face, charWidth, textHeight := newHintedUIFontFace(uiFont, spec.uiSize, 1.15)
		smallFace, _, smallTextHeight := newHintedUIFontFace(uiFont, spec.smallSize, 1.2)
		titleFace, _, titleHeight := newHintedUIFontFace(uiFont, spec.titleSize, 1.1)
		codeFace, codeCharWidth, codeTextHeight := newHintedUIFontFace(codeFont, spec.codeSize, 1.3)
		styles[index] = uiFontStyle{
			face:            face,
			smallFace:       smallFace,
			titleFace:       titleFace,
			codeFace:        codeFace,
			size:            spec.uiSize,
			smallSize:       spec.smallSize,
			titleSize:       spec.titleSize,
			codeSize:        spec.codeSize,
			charWidth:       charWidth,
			textHeight:      textHeight,
			smallTextHeight: smallTextHeight,
			titleHeight:     titleHeight,
			codeCharWidth:   codeCharWidth,
			codeTextHeight:  codeTextHeight,
			lineHeight:      spec.lineHeight,
		}
	}
	return styles
}

func newHintedUIFontFace(parsed *opentype.Font, size, contrast float64) (*text.GoXFace, int, int) {
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		panic("create hinted OpenDyslexic face: " + err.Error())
	}
	charWidth := font.MeasureString(face, "M").Ceil()
	textHeight := face.Metrics().Height.Ceil()
	return text.NewGoXFace(newBitmapFontFace(face, contrast)), charWidth, textHeight
}

// bitmapFontFace turns an outline face into an integer-grid bitmap face. The
// four alpha levels retain intentional curve smoothing without the washed-out
// fringe produced by unrestricted grayscale antialiasing at desktop scale.
type bitmapFontFace struct {
	font.Face
	alpha [256]uint8
}

func newBitmapFontFace(face font.Face, contrast float64) font.Face {
	result := &bitmapFontFace{Face: face}
	for index := range result.alpha {
		value := (float64(index)/255-0.5)*contrast + 0.5
		value = math.Max(0, math.Min(1, value))
		level := int(math.Round(value*255/85)) * 85
		result.alpha[index] = uint8(min(255, max(0, level)))
	}
	return result
}

func (f *bitmapFontFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	dot.X = fixed.I(dot.X.Round())
	dot.Y = fixed.I(dot.Y.Round())
	drawRect, mask, maskPoint, advance, ok := f.Face.Glyph(dot, r)
	if !ok || mask == nil {
		return drawRect, mask, maskPoint, fixed.I(advance.Round()), ok
	}
	bounds := mask.Bounds()
	sharpened := image.NewAlpha(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := mask.At(x, y).RGBA()
			sharpened.SetAlpha(x, y, color.Alpha{A: f.alpha[uint8(alpha>>8)]})
		}
	}
	return drawRect, sharpened, maskPoint, fixed.I(advance.Round()), ok
}

func (f *bitmapFontFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	bounds, advance, ok := f.Face.GlyphBounds(r)
	return bounds, fixed.I(advance.Round()), ok
}

func (f *bitmapFontFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	advance, ok := f.Face.GlyphAdvance(r)
	return fixed.I(advance.Round()), ok
}

func (f *bitmapFontFace) Kern(r0, r1 rune) fixed.Int26_6 {
	return fixed.I(f.Face.Kern(r0, r1).Round())
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
	uiCharWidth = style.charWidth
	uiTextHeight = style.textHeight
	uiSmallTextHeight = style.smallTextHeight
	uiTitleHeight = style.titleHeight
	codeTextHeight = style.codeTextHeight
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

func uiTextWidth(value string) int {
	return int(math.Ceil(text.Advance(value, uiFace)))
}

func codeTextWidth(value string) int {
	return int(math.Ceil(text.Advance(value, codeFace)))
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

func drawSmallTextCentered(screen *ebiten.Image, value string, rect editor.Rect, clr color.Color) {
	drawSmallText(screen, value, rect.X+(rect.W-smallTextWidth(value))/2, rect.Y+(rect.H-uiSmallTextHeight)/2, clr)
}

func truncateUIText(value string, width int) string {
	return truncateTextToWidth(value, width, uiFace)
}

func truncateCodeText(value string, width int) string {
	return truncateTextToWidth(value, width, codeFace)
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

// roundedPath returns a compact rounded rectangle path. The radius is
// clamped so the helper is safe for tiny icon buttons as well as large
// workspace panels.
func roundedPath(r editor.Rect, radius int) vector.Path {
	var path vector.Path
	if r.W <= 0 || r.H <= 0 {
		return path
	}
	radius = max(0, min(radius, min(r.W, r.H)/2))
	if radius == 0 {
		path.MoveTo(float32(r.X), float32(r.Y))
		path.LineTo(float32(r.X+r.W), float32(r.Y))
		path.LineTo(float32(r.X+r.W), float32(r.Y+r.H))
		path.LineTo(float32(r.X), float32(r.Y+r.H))
		path.Close()
		return path
	}
	x, y := float32(r.X), float32(r.Y)
	w, h, rad := float32(r.W), float32(r.H), float32(radius)
	path.MoveTo(x+rad, y)
	path.LineTo(x+w-rad, y)
	path.QuadTo(x+w, y, x+w, y+rad)
	path.LineTo(x+w, y+h-rad)
	path.QuadTo(x+w, y+h, x+w-rad, y+h)
	path.LineTo(x+rad, y+h)
	path.QuadTo(x, y+h, x, y+h-rad)
	path.LineTo(x, y+rad)
	path.QuadTo(x, y, x+rad, y)
	path.Close()
	return path
}

func fillRoundedRect(screen *ebiten.Image, r editor.Rect, radius int, clr color.Color) {
	if screen == nil || r.W <= 0 || r.H <= 0 {
		return
	}
	path := roundedPath(r, radius)
	op := &vector.DrawPathOptions{}
	op.AntiAlias = true
	op.ColorScale.ScaleWithColor(clr)
	vector.FillPath(screen, &path, nil, op)
}

func strokeRoundedRect(screen *ebiten.Image, r editor.Rect, radius int, clr color.Color) {
	strokeRoundedRectWidth(screen, r, radius, 1, clr)
}

// strokeRoundedRectWidth is used for focus rings and glows where a one-pixel
// outline does not create enough separation from dense editor content.
func strokeRoundedRectWidth(screen *ebiten.Image, r editor.Rect, radius, width int, clr color.Color) {
	if screen == nil || r.W <= 0 || r.H <= 0 {
		return
	}
	path := roundedPath(r, radius)
	stroke := &vector.StrokeOptions{Width: float32(max(1, width)), MiterLimit: 10}
	op := &vector.DrawPathOptions{}
	op.AntiAlias = true
	op.ColorScale.ScaleWithColor(clr)
	vector.StrokePath(screen, &path, stroke, op)
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
