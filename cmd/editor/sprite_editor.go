package main

import (
	"fmt"
	"image/color"
	"math"
	"runtime"
	"time"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

func (g *Game) updateSpriteMode() {
	mx, my := ebiten.CursorPosition()
	g.updateSpriteTools(mx, my)
	g.updatePixelCanvas(mx, my)
	g.updateFlags(mx, my)
	g.updateSpritesheetPanel(mx, my)
	g.updatePaletteSwatches(mx, my)
	g.updateColorPicker(mx, my)
	g.updateSpriteKeyboard()
	g.updateCopyPaste()
}

func (g *Game) drawSpriteMode(screen *ebiten.Image) {
	g.drawSpriteWorkspaceHeader(screen)
	drawSpriteWorkbenchPanel(screen, spriteCanvasPanelRect)
	drawSpriteWorkbenchPanel(screen, spriteToolsPanelRect)
	drawSpriteWorkbenchPanel(screen, spritePalettePanelRect)
	drawSpriteWorkbenchPanel(screen, spriteFlagsPanelRect)
	if spritePickerPanelRect.W > 0 {
		drawSpriteWorkbenchPanel(screen, spritePickerPanelRect)
	}
	drawSpriteWorkbenchPanel(screen, sheetPanelRect)
	g.drawPixelCanvas(screen)
	g.drawSpriteCanvasChrome(screen)
	g.drawSpriteTools(screen)
	g.drawFlags(screen)
	g.drawSpritesheetPanel(screen)
	g.drawPaletteSwatches(screen)
	g.drawColorPicker(screen)
}

func (g *Game) drawSpriteWorkspaceHeader(screen *ebiten.Image) {
	r := spriteHeaderRect
	fillRect(screen, r, color.RGBA{R: 29, G: 31, B: 78, A: 255})
	fillRect(screen, editor.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, rgba(colorBorder, 190))
	strokeRect(screen, r, colorBorder)
	logoW, _ := drawMtileHeaderLogo(screen, r.X+16, r.Y+13, 190, 38)
	titleX := r.X + 16 + logoW + 24
	drawTextScale(screen, "SPRITE LAB", titleX, r.Y+3, 2, colorSprite)
	drawSmallText(screen, "PIXEL ART · TILESET AUTHORING", titleX, r.Y+39, rgba(colorSprite, 220))
}

func drawSpriteWorkbenchPanel(screen *ebiten.Image, r editor.Rect) {
	if r.W <= 0 || r.H <= 0 {
		return
	}
	fillRoundedRect(screen, r, 7, colorPanel)
	strokeRoundedRect(screen, r, 7, rgba(colorBorder, 190))
}

func spritePanelTitlePosition(panel editor.Rect) (int, int) {
	return panel.X + spritePanelTitleInsetX, panel.Y + spritePanelTitleInsetY
}

func spritePanelMetaY(panel editor.Rect) int {
	_, titleY := spritePanelTitlePosition(panel)
	return titleY + max(0, (uiTextHeight-uiSmallTextHeight)/2)
}

func drawSpritePanelTitle(screen *ebiten.Image, panel editor.Rect, title string) {
	titleX, titleY := spritePanelTitlePosition(panel)
	drawText(screen, title, titleX, titleY, colorText)
}

func (g *Game) drawSpriteCanvasChrome(screen *ebiten.Image) {
	titleX, titleY := spritePanelTitlePosition(spriteCanvasPanelRect)
	metaY := spritePanelMetaY(spriteCanvasPanelRect)
	for _, part := range spriteCanvasTitleParts(g.currentSprite, g.spriteCols(), g.gridPixelsW(), g.gridPixelsH()) {
		clr := color.Color(colorText)
		if part.highlight {
			clr = colorSprite
		}
		if part.secondary {
			drawSmallText(screen, part.text, titleX, metaY, clr)
			titleX += smallTextWidth(part.text)
			continue
		}
		drawText(screen, part.text, titleX, titleY, clr)
		titleX += uiTextWidth(part.text)
	}

	widthText := fmt.Sprintf("%d", g.gridPixelsW())
	heightText := fmt.Sprintf("%d", g.gridPixelsH())
	dimensions := widthText + " × " + heightText + " PX"
	dimensionX := spriteCanvasPanelRect.X + spriteCanvasPanelRect.W - smallTextWidth(dimensions) - 14
	dimensionY := metaY
	drawSmallText(screen, widthText, dimensionX, dimensionY, colorSprite)
	dimensionX += smallTextWidth(widthText)
	drawSmallText(screen, " × ", dimensionX, dimensionY, colorDim)
	dimensionX += smallTextWidth(" × ")
	drawSmallText(screen, heightText, dimensionX, dimensionY, colorSprite)
	dimensionX += smallTextWidth(heightText)
	drawSmallText(screen, " PX", dimensionX, dimensionY, colorDim)

	footerY := canvasRect.Y + canvasRect.H + 9
	drawSmallText(screen, resizeShortcutText(runtime.GOOS), canvasRect.X, footerY, colorDim)
	if g.hoverX >= 0 {
		coordinates := fmt.Sprintf("PIXEL %d, %d", g.hoverX, g.hoverY)
		drawSmallText(screen, coordinates, canvasRect.X+canvasRect.W-smallTextWidth(coordinates), footerY, colorSprite)
	}
}

// gridPixelsW and gridPixelsH are the sprite-pixel span of the current
// selection (8 per sprite cell). Invalid sizes fall back to one 8x8 cell.
func (g *Game) spriteGridCols() int {
	if g == nil || g.gridCols < gridCellsMin {
		return gridCellsMin
	}
	if g.gridCols > gridCellsMax {
		return gridCellsMax
	}
	return g.gridCols
}

func (g *Game) spriteGridRows() int {
	if g == nil || g.gridRows < gridCellsMin {
		return gridCellsMin
	}
	if g.gridRows > gridCellsMax {
		return gridCellsMax
	}
	return g.gridRows
}

func (g *Game) gridPixelsW() int { return 8 * g.spriteGridCols() }

func (g *Game) gridPixelsH() int { return 8 * g.spriteGridRows() }

func (g *Game) canvasPixelScale() float64 {
	gridW, gridH := g.gridPixelsW(), g.gridPixelsH()
	if gridW <= 0 || gridH <= 0 {
		return 1
	}
	return math.Max(1, math.Min(float64(canvasRect.W)/float64(gridW), float64(canvasRect.H)/float64(gridH)))
}

// canvasPixelBounds maps sprite pixels onto shared integer boundaries. A
// fractional scale can therefore distribute remainder pixels across the
// cells while the selection still consumes the full canvas without seams.
func (g *Game) canvasPixelBounds(x, y, width, height int) editor.Rect {
	scale := g.canvasPixelScale()
	x0 := int(math.Floor(float64(canvasRect.X) + float64(x)*scale))
	y0 := int(math.Floor(float64(canvasRect.Y) + float64(y)*scale))
	x1 := int(math.Floor(float64(canvasRect.X) + float64(x+width)*scale))
	y1 := int(math.Floor(float64(canvasRect.Y) + float64(y+height)*scale))
	return editor.Rect{X: x0, Y: y0, W: max(0, x1-x0), H: max(0, y1-y0)}
}

// baseSpriteRowCol returns the top-left sprite of the current multi-sprite
// selection.
func (g *Game) baseSpriteRowCol() (row, col int) {
	cols := g.spriteCols()
	if cols == 0 {
		return 0, 0
	}
	return g.currentSprite / cols, g.currentSprite % cols
}

func (g *Game) updatePixelCanvas(mx, my int) {
	// Shape tools commit on release, even when the drag ends just outside the
	// grid. Clamping the final endpoint gives the familiar paint-app gesture
	// without ever producing an out-of-selection write.
	if g.spriteShapeActive {
		if col, row, ok := g.spriteCanvasCellAt(mx, my, true); ok {
			g.spriteShapeEndX, g.spriteShapeEndY = col, row
		}
		g.spriteShapeFilled = ebiten.IsKeyPressed(ebiten.KeyShift)
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			if g.commitSpriteShape() {
				g.flashInk(mx, my)
			}
			g.spriteShapeActive = false
		}
	}

	col, row, inside := g.spriteCanvasCellAt(mx, my, false)
	if !inside {
		g.hoverX, g.hoverY = -1, -1
		return
	}
	id, px, py, ok := g.selectionPixelTarget(col, row)
	if !ok {
		g.hoverX, g.hoverY = -1, -1
		return
	}
	cols := g.spriteCols()
	g.hoverX, g.hoverY = (id%cols)*8+px, (id/cols)*8+py

	switch {
	case ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight):
		if g.paintPixel(id, px, py, eraserIndex) {
			g.flashInk(mx, my)
		}
	case g.spriteTool == spriteToolPencil && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft):
		if g.paintPixel(id, px, py, g.currentColor) {
			g.flashInk(mx, my)
		}
	case g.spriteTool == spriteToolEraser && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft):
		if g.paintPixel(id, px, py, eraserIndex) {
			g.flashInk(mx, my)
		}
	case g.spriteTool == spriteToolFill && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft):
		if g.fillSpriteSelection(col, row, g.currentColor) {
			g.flashInk(mx, my)
		}
	case g.spriteTool >= spriteToolLine && g.spriteTool <= spriteToolCircle && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft):
		g.spriteShapeActive = true
		g.spriteShapeStartX, g.spriteShapeStartY = col, row
		g.spriteShapeEndX, g.spriteShapeEndY = col, row
		g.spriteShapeFilled = ebiten.IsKeyPressed(ebiten.KeyShift)
	}
}

func (g *Game) spriteCanvasCellAt(mx, my int, clampToGrid bool) (col, row int, ok bool) {
	gridW, gridH := g.gridPixelsW(), g.gridPixelsH()
	used := g.canvasPixelBounds(0, 0, gridW, gridH)
	if used.W <= 0 || used.H <= 0 {
		return 0, 0, false
	}
	if clampToGrid {
		mx = max(used.X, min(used.X+used.W-1, mx))
		my = max(used.Y, min(used.Y+used.H-1, my))
	} else if !rectContains(used, mx, my) {
		return 0, 0, false
	}
	scale := g.canvasPixelScale()
	col = int(math.Ceil(float64(mx-canvasRect.X+1)/scale)) - 1
	row = int(math.Ceil(float64(my-canvasRect.Y+1)/scale)) - 1
	return col, row, col >= 0 && row >= 0 && col < gridW && row < gridH
}

func (g *Game) paintPixel(id, px, py, colorIndex int) bool {
	cols := g.spriteCols()
	if cols == 0 || id < 0 || id >= g.spriteN() {
		return false
	}
	if id == editor.ReservedSpriteID {
		g.statusMsg = spriteZeroLockedMessage
		return false
	}
	if g.pixelAt((id%cols)*8+px, (id/cols)*8+py) == colorIndex {
		return false
	}
	if err := g.model.SetSpritePixel(id, px, py, colorIndex); err != nil {
		g.statusMsg = err.Error()
		return false
	}
	g.dirty = true
	return true
}

func (g *Game) drawPixelCanvas(screen *ebiten.Image) {
	scale := g.canvasPixelScale()
	gridW, gridH := g.gridPixelsW(), g.gridPixelsH()
	baseRow, baseCol := g.baseSpriteRowCol()
	g.drawSpritePixels(screen, float64(canvasRect.X), float64(canvasRect.Y), scale, scale, baseCol*8, baseRow*8, gridW, gridH, false)
	g.drawPixelCanvasGrid(screen, scale, gridW, gridH)
	g.drawSpriteShapePreview(screen)
	g.drawReservedSpriteCanvasLock(screen)
	strokeRoundedRect(screen, editor.Rect{X: canvasRect.X - 1, Y: canvasRect.Y - 1, W: canvasRect.W + 2, H: canvasRect.H + 2}, 5, colorBorder)
	if g.theme.level != effectsClean {
		mx, my := ebiten.CursorPosition()
		if col, row, ok := g.spriteCanvasCellAt(mx, my, false); ok {
			strokeRect(screen, g.canvasPixelBounds(col, row, 1, 1), colorSprite)
		}
		if g.theme.inkPulse > 0 {
			if col, row, ok := g.spriteCanvasCellAt(g.theme.inkX, g.theme.inkY, false); ok {
				expand := (12 - g.theme.inkPulse) / 3
				alpha := uint8(min(190, g.theme.inkPulse*15))
				r := g.canvasPixelBounds(col, row, 1, 1)
				r = editor.Rect{X: r.X - expand, Y: r.Y - expand, W: r.W + expand*2, H: r.H + expand*2}
				strokeRect(screen, r, rgba(colorSprite, alpha))
			}
		}
	}

}

// drawPixelCanvasGrid uses the shared sprite-grid treatment, including minor
// pixel boundaries while the enlarged canvas has enough room for them.
func (g *Game) drawPixelCanvasGrid(screen *ebiten.Image, scale float64, gridW, gridH int) {
	if screen == nil || scale <= 1 || gridW <= 0 || gridH <= 0 {
		return
	}
	drawSpritePixelGrid(screen, float64(canvasRect.X), float64(canvasRect.Y), scale, scale, gridW, gridH, canvasRect, true)
}

// drawSpritePixelGrid renders grid lines in sprite-pixel space. The editing
// canvas can show minor pixel boundaries because its pixels are large; the
// fitted spritesheet uses the same helper with only 8x8 tile boundaries so
// its smaller cells keep the same apparent darkness instead of turning into
// a dense bright mesh.
func drawSpritePixelGrid(screen *ebiten.Image, originX, originY, scaleX, scaleY float64, gridW, gridH int, clip editor.Rect, showMinor bool) {
	// Once a source pixel is smaller than one screen pixel, a per-pixel grid
	// would alias into repeated dark lines. The canvas already makes the same
	// choice for a one-pixel cell, so keep the two views visually consistent.
	if screen == nil || gridW <= 0 || gridH <= 0 || scaleX < 1 || scaleY < 1 || clip.W <= 0 || clip.H <= 0 {
		return
	}
	left := clip.X
	top := clip.Y
	right := clip.X + clip.W
	bottom := clip.Y + clip.H
	firstCol, lastCol := 1, gridW
	if originX < float64(left) {
		firstCol = max(firstCol, int(math.Floor((float64(left)-originX)/scaleX)))
	}
	if originX+float64(gridW)*scaleX > float64(right) {
		lastCol = min(lastCol, int(math.Ceil((float64(right)-originX)/scaleX)))
	}
	for col := firstCol; col < lastCol; col++ {
		clr, visible := spriteGridLineColor(col, showMinor)
		if !visible {
			continue
		}
		x := int(math.Floor(originX + float64(col)*scaleX))
		if x < left || x > right {
			continue
		}
		drawLine(screen, x, max(top, int(math.Floor(originY))), x, min(bottom, int(math.Floor(originY+float64(gridH)*scaleY))), clr)
	}
	firstRow, lastRow := 1, gridH
	if originY < float64(top) {
		firstRow = max(firstRow, int(math.Floor((float64(top)-originY)/scaleY)))
	}
	if originY+float64(gridH)*scaleY > float64(bottom) {
		lastRow = min(lastRow, int(math.Ceil((float64(bottom)-originY)/scaleY)))
	}
	for row := firstRow; row < lastRow; row++ {
		clr, visible := spriteGridLineColor(row, showMinor)
		if !visible {
			continue
		}
		y := int(math.Floor(originY + float64(row)*scaleY))
		if y < top || y > bottom {
			continue
		}
		drawLine(screen, max(left, int(math.Floor(originX))), y, min(right, int(math.Floor(originX+float64(gridW)*scaleX))), y, clr)
	}
}

func spriteGridLineColor(index int, showMinor bool) (color.Color, bool) {
	if index > 0 && index%8 == 0 {
		return spriteCanvasGridColor(44), true
	}
	if index > 0 && showMinor {
		return spriteCanvasGridColor(28), true
	}
	return nil, false
}

// color.RGBA stores premultiplied channels, so pairing its full RGB values
// with a low alpha makes Ebitengine's vector path render unexpectedly bright.
// NRGBA supplies straight-alpha input and gives both sprite views the same
// subdued grid treatment.
func spriteCanvasGridColor(alpha uint8) color.Color {
	return color.NRGBA{R: colorGrid.R, G: colorGrid.G, B: colorGrid.B, A: alpha}
}

// drawReservedSpriteCanvasLock makes the immutable first cell unmistakable
// while still allowing it to remain selected as the map eraser. It also works
// for a multi-cell selection whose top-left constituent includes sprite 0.
func (g *Game) drawReservedSpriteCanvasLock(screen *ebiten.Image) {
	baseRow, baseCol := g.baseSpriteRowCol()
	if baseRow != 0 || baseCol != 0 || g.spriteN() == 0 {
		return
	}
	r := g.canvasPixelBounds(0, 0, 8, 8)
	if r.W < 160 || r.H < 120 {
		drawSpriteLockOverlay(screen, r)
		return
	}
	fillRoundedRect(screen, r, min(8, max(2, min(r.W, r.H)/10)), rgba(colorInk, 184))
	if r.W < 300 {
		cardW := min(208, r.W-24)
		cardH := 116
		card := editor.Rect{X: r.X + (r.W-cardW)/2, Y: r.Y + (r.H-cardH)/2, W: cardW, H: cardH}
		fillRoundedRect(screen, card, 7, color.RGBA{R: 24, G: 25, B: 48, A: 245})
		strokeRoundedRect(screen, card, 7, rgba(colorSprite, 190))
		icon := editor.Rect{X: card.X + (card.W-32)/2, Y: card.Y + 10, W: 32, H: 32}
		drawSpriteLockOverlay(screen, icon)
		drawTextCentered(screen, "SPRITE 0 · RESERVED", editor.Rect{X: card.X, Y: card.Y + 46, W: card.W, H: uiTextHeight}, colorText)
		drawSmallTextCentered(screen, "TRANSPARENT MAP ERASER", editor.Rect{X: card.X, Y: card.Y + 75, W: card.W, H: uiSmallTextHeight}, colorSprite)
		drawSmallTextCentered(screen, "SELECT SPRITE 1 TO DRAW", editor.Rect{X: card.X, Y: card.Y + 96, W: card.W, H: uiSmallTextHeight}, colorDim)
		return
	}
	cardW := min(260, r.W-32)
	card := editor.Rect{X: r.X + (r.W-cardW)/2, Y: r.Y + (r.H-94)/2, W: cardW, H: 94}
	fillRoundedRect(screen, card, 7, color.RGBA{R: 24, G: 25, B: 48, A: 245})
	strokeRoundedRect(screen, card, 7, rgba(colorSprite, 190))
	icon := editor.Rect{X: card.X + 14, Y: card.Y + 20, W: 48, H: 48}
	drawSpriteLockOverlay(screen, icon)
	drawText(screen, "SPRITE 0 · RESERVED", card.X+76, card.Y+17, colorText)
	drawSmallText(screen, "TRANSPARENT MAP ERASER", card.X+76, card.Y+48, colorSprite)
	drawSmallText(screen, "SELECT SPRITE 1 TO DRAW", card.X+76, card.Y+68, colorDim)
}

func spriteNumberText(spriteID int) string {
	return fmt.Sprintf("%03d", spriteID)
}

type spriteCanvasTitlePart struct {
	text      string
	highlight bool
	secondary bool
}

// spriteCanvasTitleParts keeps the ordinary tile ID for an 8x8 selection.
// Larger selections are labeled as metasprites and expose the exact source
// rectangle accepted by sspr(), so the user can copy sx, sy, sw, sh directly
// from the canvas header.
func spriteCanvasTitleParts(spriteID, spriteCols, width, height int) []spriteCanvasTitlePart {
	if width <= 8 && height <= 8 {
		return []spriteCanvasTitlePart{
			{text: "SPRITE"},
			{text: " ", secondary: true},
			{text: spriteNumberText(spriteID), highlight: true, secondary: true},
		}
	}
	sourceX, sourceY := 0, 0
	if spriteCols > 0 {
		sourceX = (spriteID % spriteCols) * 8
		sourceY = (spriteID / spriteCols) * 8
	}
	return []spriteCanvasTitlePart{
		{text: "METASPRITE"},
		{text: " (", secondary: true},
		{text: fmt.Sprintf("%d", sourceX), highlight: true, secondary: true},
		{text: ",", secondary: true},
		{text: fmt.Sprintf("%d", sourceY), highlight: true, secondary: true},
		{text: ",", secondary: true},
		{text: fmt.Sprintf("%d", width), highlight: true, secondary: true},
		{text: ",", secondary: true},
		{text: fmt.Sprintf("%d", height), highlight: true, secondary: true},
		{text: ")", secondary: true},
	}
}

func resizeShortcutText(goos string) string {
	mod := "CTRL"
	if goos == "darwin" {
		mod = "CMD"
	}
	return fmt.Sprintf("%s+[ / ] RESIZE", mod)
}

func (g *Game) fillColor(screen *ebiten.Image, r editor.Rect, index int) {
	if isEraserIndex(index) {
		drawCheckerboardColors(screen, r, colorSpriteCheckerA, colorSpriteCheckerB)
		return
	}
	fillRect(screen, r, g.swatch(index))
}

// swatch returns the displayable color for a palette index, or black if the
// preview isn't ready yet or the index is out of range.
func (g *Game) swatch(index int) color.Color {
	if g.preview == nil || index < 0 || index >= len(g.preview.Colors) {
		return color.Black
	}
	return swatchColor(g.preview.Colors[index])
}

func (g *Game) updateFlags(mx, my int) {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	for i := 0; i < 8; i++ {
		r := flagCheckboxRect(i)
		if rectContains(r, mx, my) {
			g.toggleFlag(i)
			return
		}
	}
}

func flagCheckboxRect(i int) editor.Rect {
	const (
		width = 34
		gap   = 8
	)
	buttonY := flagsRect.Y + 21
	if spriteFlagsPanelRect.H >= spriteFlagsPanelHeight {
		buttonY = flagsRect.Y + 31
	}
	return editor.Rect{X: flagsRect.X + 4 + i*(width+gap), Y: buttonY, W: width, H: 30}
}

func (g *Game) toggleFlag(bit int) {
	// A mixed selection is shown as tri-state. Treat it like the usual editor
	// convention: only an all-set selection toggles off; a mixed or all-clear
	// selection toggles on for every constituent sprite. Looking only at the
	// top-left sprite made the result depend on selection order.
	allTrue, _ := g.flagBitState(bit)
	value := !allTrue
	baseRow, baseCol := g.baseSpriteRowCol()
	changed := false
	for dr := 0; dr < g.spriteGridRows(); dr++ {
		for dc := 0; dc < g.spriteGridCols(); dc++ {
			row, col := baseRow+dr, baseCol+dc
			if row >= g.spriteRows() || col >= g.spriteCols() {
				continue
			}
			id := row*g.spriteCols() + col
			if err := g.model.SetSpriteFlag(id, bit, value); err != nil {
				g.statusMsg = err.Error()
				return
			}
			changed = true
		}
	}
	if !changed {
		return
	}
	g.undo.forcePush(g.snapshot())
}

func (g *Game) flagBitState(bit int) (allTrue, anyTrue bool) {
	baseRow, baseCol := g.baseSpriteRowCol()
	seen := false
	allTrue = true
	for dr := 0; dr < g.spriteGridRows(); dr++ {
		for dc := 0; dc < g.spriteGridCols(); dc++ {
			row, col := baseRow+dr, baseCol+dc
			if row >= g.spriteRows() || col >= g.spriteCols() {
				continue
			}
			seen = true
			id := row*g.spriteCols() + col
			if g.model.SpriteFlagByte(id)&(1<<bit) != 0 {
				anyTrue = true
			} else {
				allTrue = false
			}
		}
	}
	if !seen {
		return false, false
	}
	return allTrue, anyTrue
}

func (g *Game) drawFlags(screen *ebiten.Image) {
	drawSpritePanelTitle(screen, spriteFlagsPanelRect, "SPRITE FLAGS")
	meta := "BITS 0–7"
	drawSmallText(screen, meta, spriteFlagsPanelRect.X+spriteFlagsPanelRect.W-spritePanelTitleInsetX-smallTextWidth(meta), spritePanelMetaY(spriteFlagsPanelRect), colorDim)
	for i := 0; i < 8; i++ {
		r := flagCheckboxRect(i)
		allTrue, anyTrue := g.flagBitState(i)
		fillRoundedRect(screen, r, 4, colorButtonFace)
		strokeRoundedRect(screen, r, 4, rgba(colorBorder, 190))
		switch {
		case allTrue:
			fillRoundedRect(screen, editor.Rect{X: r.X + 2, Y: r.Y + 2, W: r.W - 4, H: r.H - 4}, 3, color.RGBA{R: 47, G: 65, B: 83, A: 255})
			strokeRoundedRect(screen, r, 4, colorSprite)
		case anyTrue:
			fillRect(screen, editor.Rect{X: r.X + 7, Y: r.Y + r.H/2 - 1, W: r.W - 14, H: 3}, colorHighlight)
		}
		clr := color.Color(colorDim)
		if allTrue || anyTrue {
			clr = colorText
		}
		drawSmallTextCentered(screen, fmt.Sprintf("%d", i), r, clr)
	}
}

func (g *Game) sheetGridCanvas() editor.Rect {
	return editor.Rect{
		X: sheetPanelRect.X + 10,
		Y: sheetPanelRect.Y + sheetPanelLabelHeight,
		W: max(1, sheetPanelRect.W-20),
		H: max(1, sheetPanelRect.H-sheetPanelLabelHeight-10),
	}
}

func (g *Game) fitSheetCamera() {
	canvas := g.sheetGridCanvas()
	cols, rows := g.spriteCols(), g.spriteRows()
	if cols <= 0 || rows <= 0 || canvas.W <= 0 || canvas.H <= 0 {
		return
	}
	// The spritesheet is an editing surface, not a letterboxed preview. Fit
	// each axis independently so its final column and row meet the canvas
	// edges exactly. Keep the existing zoom ceiling for tiny sheets, where
	// filling a desktop panel would turn one cell into a giant button.
	g.sheetCam.Zoom = min(float64(canvas.W)/(float64(cols)*editor.BaseTileSize), spriteSheetMaxFitZoom)
	g.sheetCam.ZoomY = min(float64(canvas.H)/(float64(rows)*editor.BaseTileSize), spriteSheetMaxFitZoom)
	g.sheetCam.X, g.sheetCam.Y = 0, 0
	g.clampSheetCamera(canvas)
}

// clampSheetCamera keeps panning bounded, then anchors every fully visible
// axis to the panel's leading edge. A fitted sheet should read like an editor
// grid that starts below its header, not a preview floating in letterboxing.
func (g *Game) clampSheetCamera(canvas editor.Rect) {
	cols, rows := g.spriteCols(), g.spriteRows()
	g.sheetCam.Clamp(cols, rows, canvas)
	spanX, spanY := g.sheetCam.VisibleSpan(canvas)
	if spanX >= float64(cols) {
		g.sheetCam.X = 0
	}
	if spanY >= float64(rows) {
		g.sheetCam.Y = 0
	}
}

func (g *Game) updateSpritesheetPanel(mx, my int) {
	canvas := g.sheetGridCanvas()
	cols, rows := g.spriteCols(), g.spriteRows()

	if rectContains(canvas, mx, my) {
		if _, yoff := ebiten.Wheel(); yoff != 0 {
			g.sheetCam.Pan(0, -yoff, canvas, cols, rows)
		}
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			col, row, ok := g.sheetCam.ScreenToTile(mx, my, canvas)
			if ok && col >= 0 && row >= 0 && col < cols && row < rows {
				g.currentSprite = row*cols + col
			}
		}
	}
	g.clampSheetCamera(canvas)
}

func (g *Game) drawSpritesheetPanel(screen *ebiten.Image) {
	canvas := g.sheetGridCanvas()
	cols, rows := g.spriteCols(), g.spriteRows()
	title := "SPRITESHEET"
	titleX, titleY := spritePanelTitlePosition(sheetPanelRect)
	drawText(screen, title, titleX, titleY, colorText)
	meta := fmt.Sprintf("%d × %d  ·  %d SPRITES", cols, rows, cols*rows)
	metaY := spritePanelMetaY(sheetPanelRect)
	drawSmallText(screen, meta, titleX+uiTextWidth(title)+smallTextWidth("  "), metaY, colorDim)
	fillRoundedRect(screen, canvas, 5, colorInk)
	strokeRoundedRect(screen, canvas, 5, rgba(colorBorder, 180))

	minC, minR, maxC, maxR := g.sheetCam.VisibleBounds(canvas, cols, rows)
	tileW, tileH := g.sheetCam.TileSize(), g.sheetCam.TileSizeY()
	var lockedRect editor.Rect
	lockedVisible := false
	for row := minR; row < maxR; row++ {
		for col := minC; col < maxC; col++ {
			sx, sy := g.sheetCam.WorldToScreen(col, row, canvas)
			id := row*cols + col
			g.drawSpriteThumb(screen, id, sx, sy, tileW, tileH)
			if id == editor.ReservedSpriteID {
				lockedRect = editor.Rect{X: int(sx), Y: int(sy), W: max(1, int(math.Ceil(tileW))), H: max(1, int(math.Ceil(tileH)))}
				lockedVisible = true
			}
		}
	}
	sheetLeft, sheetTop := g.sheetCam.WorldToScreen(0, 0, canvas)
	// Keep the sheet at tile-level resolution. Drawing every source-pixel line
	// at this fitted scale overwhelms the checkerboard and makes the sheet look
	// much brighter than the editing canvas.
	drawSpritePixelGrid(screen, sheetLeft, sheetTop, tileW/8, tileH/8, cols*8, rows*8, canvas, false)
	if lockedVisible {
		drawSpriteLockOverlay(screen, lockedRect)
	}
	g.drawSelectionBorder(screen, canvas)
}

func (g *Game) drawSpriteThumb(screen *ebiten.Image, id int, sx, sy, tileW, tileH float64) {
	cols := g.spriteCols()
	if cols == 0 || id < 0 || id >= g.spriteN() {
		return
	}
	scaleX := tileW / 8
	scaleY := tileH / 8
	baseCol := id % cols
	baseRow := id / cols
	g.drawSpritePixels(screen, sx, sy, scaleX, scaleY, baseCol*8, baseRow*8, 8, 8, true)
}

// drawSpritePixels is the shared indexed-pixel renderer for the editing
// canvas and each fitted spritesheet cell. Keeping the source lookup and
// palette conversion here prevents the two views from drifting visually.
func (g *Game) drawSpritePixels(screen *ebiten.Image, originX, originY, scaleX, scaleY float64, sourceX, sourceY, pixelW, pixelH int, subtleTransparency bool) {
	if screen == nil || scaleX <= 0 || scaleY <= 0 || pixelW <= 0 || pixelH <= 0 {
		return
	}
	for py := 0; py < pixelH; py++ {
		for px := 0; px < pixelW; px++ {
			pixel := g.pixelAt(sourceX+px, sourceY+py)
			x, w := scaledRun(originX, px, scaleX)
			y, h := scaledRun(originY, py, scaleY)
			target := editor.Rect{X: x, Y: y, W: w, H: h}
			if subtleTransparency && isEraserIndex(pixel) {
				drawCheckerboardColors(screen, target, colorSheetCheckerA, colorSheetCheckerB)
			} else {
				g.fillColor(screen, target, pixel)
			}
		}
	}
}

// scaledRun maps one source texel onto an integer screen span so adjacent
// texels share an edge. Truncating each pixel independently to int(scale)
// leaves gaps when the thumbnail scale is not an integer.
func scaledRun(origin float64, i int, scale float64) (start, size int) {
	start = int(math.Floor(origin + float64(i)*scale))
	end := int(math.Floor(origin + float64(i+1)*scale))
	size = end - start
	if size < 1 {
		size = 1
	}
	return start, size
}

func (g *Game) drawSelectionBorder(screen *ebiten.Image, canvas editor.Rect) {
	baseRow, baseCol := g.baseSpriteRowCol()
	x1, y1 := g.sheetCam.WorldToScreen(baseCol, baseRow, canvas)
	x2, y2 := g.sheetCam.WorldToScreen(baseCol+g.spriteGridCols(), baseRow+g.spriteGridRows(), canvas)
	r := editor.Rect{
		X: int(math.Round(x1)) + 3,
		Y: int(math.Round(y1)) + 3,
		W: max(1, int(math.Round(x2-x1))-6),
		H: max(1, int(math.Round(y2-y1))-6),
	}
	if r.W <= 1 || r.H <= 1 {
		return
	}

	// A narrow, still cyan edge matches the restrained selection treatment used
	// by the reference UI. The outer stroke is only a soft separator from the
	// grid—not an animated neon halo competing with the artwork.
	radius := min(8, max(3, min(r.W, r.H)/8))
	strokeRoundedRectWidth(screen, r, radius, 3, rgba(colorSprite, 38))
	strokeRoundedRectWidth(screen, r, radius, 2, rgba(colorSprite, 230))
	inner := editor.Rect{X: r.X + 1, Y: r.Y + 1, W: max(1, r.W-2), H: max(1, r.H-2)}
	strokeRoundedRectWidth(screen, inner, max(2, radius-1), 1, rgba(colorText, 96))
}

func cycleGridCells(n, delta int) int {
	span := gridCellsMax - gridCellsMin + 1
	if n < gridCellsMin || n > gridCellsMax {
		n = gridCellsMin
	}
	n += delta
	n = (n - gridCellsMin) % span
	if n < 0 {
		n += span
	}
	return n + gridCellsMin
}

func (g *Game) nudgeGridCols(delta int) {
	g.gridCols = cycleGridCells(g.spriteGridCols(), delta)
}

func (g *Game) nudgeGridRows(delta int) {
	g.gridRows = cycleGridCells(g.spriteGridRows(), delta)
}

func (g *Game) updateSpriteKeyboard() {
	if g.updateSpriteToolShortcuts() {
		return
	}
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	delta := 1
	if mod {
		delta = -1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft) {
		g.nudgeGridRows(delta)
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketRight) {
		g.nudgeGridCols(delta)
		return
	}

	now := time.Now()
	if now.Sub(g.lastNav) < navDebounce {
		return
	}
	mx, my := ebiten.CursorPosition()
	if _, yoff := ebiten.Wheel(); yoff != 0 && rectContains(canvasRect, mx, my) {
		delta := 1
		if yoff < 0 {
			delta = -1
		}
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			g.nudgeGridRows(delta)
		} else {
			g.nudgeGridCols(delta)
		}
		g.lastNav = now
		return
	}

	if mod {
		return
	}

	var dRow, dCol int
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyLeft):
		dCol = -1
	case inpututil.IsKeyJustPressed(ebiten.KeyRight):
		dCol = 1
	case inpututil.IsKeyJustPressed(ebiten.KeyUp):
		dRow = -1
	case inpututil.IsKeyJustPressed(ebiten.KeyDown):
		dRow = 1
	default:
		return
	}
	baseRow, baseCol := g.baseSpriteRowCol()
	newRow, newCol := baseRow+dRow, baseCol+dCol
	if newRow < 0 || newRow >= g.spriteRows() || newCol < 0 || newCol >= g.spriteCols() {
		return
	}
	g.currentSprite = newRow*g.spriteCols() + newCol
	g.lastNav = now
}

func (g *Game) updateCopyPaste() {
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	if !mod {
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyC) {
		g.copyCurrentSprite()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyV) {
		g.pasteCurrentSprite()
	}
}

func (g *Game) copyCurrentSprite() {
	cols := g.spriteCols()
	if cols == 0 || g.currentSprite < 0 || g.currentSprite >= g.spriteN() {
		g.hasCopied = false
		return
	}
	baseCol, baseRow := g.currentSprite%cols, g.currentSprite/cols
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			g.copiedSprite[y][x] = g.pixelAt(baseCol*8+x, baseRow*8+y)
		}
	}
	g.hasCopied = true
}

// pasteCurrentSprite applies the copy buffer to currentSprite. It validates
// every buffered color index against the active palette before writing any
// pixel: the buffer may have been copied while a wider palette was active
// (up to 256 colors), and pasting under a narrower one without this
// preflight would let paintPixel silently apply only the in-range pixels
// while leaving the rest of the sprite untouched, with no clear indication
// that the paste was incomplete.
func (g *Game) pasteCurrentSprite() {
	if !g.hasCopied {
		return
	}
	if g.currentSprite == editor.ReservedSpriteID {
		g.statusMsg = spriteZeroLockedMessage
		return
	}
	limit := g.paintableColorCount()
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if c := g.copiedSprite[y][x]; c < 0 || c >= limit {
				g.statusMsg = fmt.Sprintf("copied sprite uses color %d, outside the active palette's %d colors", c, limit)
				return
			}
		}
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			g.paintPixel(g.currentSprite, x, y, g.copiedSprite[y][x])
		}
	}
}
