package main

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

func (g *Game) updateSpriteMode() {
	if g.poseVariants.open {
		g.updatePoseVariants()
		return
	}
	if g.updatePoseNameTyping() {
		mx, my := editorCursorPosition()
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && !g.poseNameEditClickKeepsFocus(mx, my) {
			g.commitPoseNameEdit()
			if g.poseNameEditing {
				return
			}
		} else {
			return
		}
	}
	mx, my := editorCursorPosition()
	if g.updateSpriteBankToggle(mx, my) {
		return
	}
	if g.animationWorkspaceActive() {
		g.updateAnimationWorkspace(mx, my)
		return
	}
	if g.objEditing && g.updateObjPageHeader(mx, my) {
		return
	}
	if g.spritePaste != nil {
		g.updateSpritesheetPanel(mx, my)
		g.updateCopyPaste()
		return
	}
	g.updateSpriteCanvasView(mx, my)
	g.updateSpriteTools(mx, my)
	g.updatePixelCanvas(mx, my)
	if !g.objEditing {
		g.updateFlags(mx, my)
		g.updateTileLinkControls(mx, my)
	} else {
		g.updateObjTileReuse(mx, my)
	}
	g.updateSpritesheetPanel(mx, my)
	g.updatePoseStrip(mx, my)
	g.updatePaletteSwatches(mx, my)
	g.updateColorPicker(mx, my)
	g.updateSpriteKeyboard()
	g.updateCopyPaste()
}

func (g *Game) drawSpriteMode(screen *ebiten.Image) {
	g.drawSpriteWorkspaceHeader(screen)
	if g.animationWorkspaceActive() {
		g.drawAnimationWorkspace(screen)
		return
	}
	// OBJ sheets are stored sparsely. Flatten once per requested draw rather
	// than searching thousands of records for each displayed pixel.
	if g.objEditing && g.model != nil && g.model.ObjSheet != nil {
		g.objDrawPixels = g.objectTileUsage().pixels
		defer func() { g.objDrawPixels = nil }()
	}
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
	if !g.objEditing {
		g.drawFlags(screen)
		g.drawTileLinkControls(screen)
	} else {
		g.drawObjTileReuse(screen)
	}
	g.drawSpritesheetPanel(screen)
	g.drawPoseStrip(screen)
	g.drawPaletteSwatches(screen)
	g.drawColorPicker(screen)
	if g.poseVariants.open {
		g.drawPoseVariants(screen)
	}
}

func (g *Game) drawSpriteWorkspaceHeader(screen *ebiten.Image) {
	r := spriteHeaderRect
	titleX := r.X + 230
	subtitle := "SHARED TILES · map / spr / sspr"
	if g.objEditing {
		subtitle = "POSES · DRAW / PASTE → REGISTER POSE"
		if g.animationWorkspaceActive() {
			subtitle = "ANIMATIONS · CHOOSE POSES → PREVIEW → REGISTER CLIP"
		}
	}
	bg, _, _ := spriteWorkspaceTabRects()
	subtitle = truncateUIText(subtitle, bg.X-titleX-16)
	drawWorkspaceHeader(screen, r, "SPRITE LAB", subtitle, colorSprite)
	g.drawSpriteBankToggle(screen)
	g.drawPoseBankHint(screen)
}

func spriteBankToggleRects() (bg, obj editor.Rect) {
	bg, obj, _ = spriteWorkspaceTabRects()
	return bg, obj
}

func spriteWorkspaceTabRects() (shared, poses, animations editor.Rect) {
	r := spriteHeaderRect
	animations = editor.Rect{X: r.X + r.W - 152, Y: r.Y + 22, W: 132, H: 28}
	poses = editor.Rect{X: animations.X - 112, Y: animations.Y, W: 104, H: 28}
	shared = editor.Rect{X: poses.X - 140, Y: poses.Y, W: 132, H: 28}
	return shared, poses, animations
}

func (g *Game) drawSpriteBankToggle(screen *ebiten.Image) {
	bg, poses, animations := spriteWorkspaceTabRects()
	drawBankToggle(screen, bg, "SHARED TILES", !g.objEditing)
	drawBankToggle(screen, poses, "POSES", g.objEditing && !g.animationWorkspaceActive())
	drawBankToggle(screen, animations, "ANIMATIONS", g.animationWorkspaceActive())
}

func drawBankToggle(screen *ebiten.Image, r editor.Rect, label string, active bool) {
	drawTactileButton(screen, r, label, active, colorSelection, true)
}

func (g *Game) updateSpriteBankToggle(mx, my int) bool {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return false
	}
	bg, poses, animations := spriteWorkspaceTabRects()
	switch {
	case rectContains(bg, mx, my):
		g.setAnimationWorkspaceActive(false)
		_ = g.setObjEditing(false)
	case rectContains(poses, mx, my), rectContains(animations, mx, my):
		if !g.objEditing {
			creating := g.model != nil && g.model.ObjSheet == nil
			if err := g.setObjEditing(true); err != nil {
				g.statusMsg = err.Error()
				return true
			}
			if creating {
				// Creating the sheet remaps pose parts. Record it as its own
				// undo step so undoing a later edit cannot revert it too.
				g.undo.forcePush(g.snapshot())
			}
		}
		g.setAnimationWorkspaceActive(rectContains(animations, mx, my))
	default:
		return false
	}
	g.spritePaste = nil
	g.copiedSprite.cut = nil
	g.sheetSelectionDragging = false
	g.objPageNameEditing = false
	g.cancelPoseNameEdit()
	g.requestRender()
	return true
}

func drawSpriteWorkbenchPanel(screen *ebiten.Image, r editor.Rect) {
	if r.W <= 0 || r.H <= 0 {
		return
	}
	fillRoundedRect(screen, r, 7, colorPanel)
	strokeRoundedRect(screen, r, 7, colorSectionBorder)
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

	footerY := canvasRect.Y + canvasRect.H + spriteCanvasFooterGap
	g.drawSpriteCanvasZoomControls(screen)
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
	limit := g.spriteSelectionLimitCols()
	if g.objEditing && g.spriteCols() > 0 {
		p := g.currentObjPageRect()
		limit = min(limit, (p.X+p.W)/8-g.currentSprite%g.spriteCols())
	}
	return max(1, min(g.gridCols, limit))
}

func (g *Game) spriteGridRows() int {
	if g == nil || g.gridRows < gridCellsMin {
		return gridCellsMin
	}
	limit := g.spriteSelectionLimitRows()
	if g.objEditing && g.spriteCols() > 0 {
		p := g.currentObjPageRect()
		limit = min(limit, (p.Y+p.H)/8-g.currentSprite/g.spriteCols())
	}
	return max(1, min(g.gridRows, limit))
}

func (g *Game) spriteSelectionLimitCols() int {
	if g != nil && g.objEditing {
		return max(gridCellsMin, g.currentObjPageRect().W/8)
	}
	return gridCellsMax
}

func (g *Game) spriteSelectionLimitRows() int {
	if g != nil && g.objEditing {
		return max(gridCellsMin, g.currentObjPageRect().H/8)
	}
	return gridCellsMax
}

func (g *Game) gridPixelsW() int { return 8 * g.spriteGridCols() }

func (g *Game) gridPixelsH() int { return 8 * g.spriteGridRows() }

// selectionPixelBlock is the in-sheet pixel rectangle of the current
// multi-sprite selection. Bracket-key grids can outrun the sheet edge;
// copy, paste, and erase use only the cells that still exist.
func (g *Game) selectionPixelBlock() (startCol, startRow, width, height int, ok bool) {
	if g == nil || g.spriteN() <= 0 || g.currentSprite < 0 || g.currentSprite >= g.spriteN() {
		return 0, 0, 0, 0, false
	}
	startRow, startCol = g.baseSpriteRowCol()
	tileCols := min(g.spriteGridCols(), g.spriteCols()-startCol)
	tileRows := min(g.spriteGridRows(), g.spriteRows()-startRow)
	if tileCols <= 0 || tileRows <= 0 {
		return 0, 0, 0, 0, false
	}
	return startCol, startRow, tileCols * 8, tileRows * 8, true
}

func (g *Game) canvasPixelScale() float64 {
	gridW, gridH := g.gridPixelsW(), g.gridPixelsH()
	if gridW <= 0 || gridH <= 0 {
		return 1
	}
	fit := math.Max(1, math.Min(float64(canvasRect.W)/float64(gridW), float64(canvasRect.H)/float64(gridH)))
	return fit * spriteCanvasZoomFactor(g.spriteCanvasZoomLevel)
}

const (
	spriteCanvasZoomMin = 0
	spriteCanvasZoomMax = 4
)

func spriteCanvasZoomFactor(level int) float64 {
	switch min(spriteCanvasZoomMax, max(spriteCanvasZoomMin, level)) {
	case 1:
		return 1.5
	case 2:
		return 2
	case 3:
		return 3
	case 4:
		return 4
	default:
		return 1
	}
}

func (g *Game) clampSpriteCanvasPan() {
	scale := g.canvasPixelScale()
	if scale <= 0 {
		g.spriteCanvasPanX, g.spriteCanvasPanY = 0, 0
		return
	}
	spanX := float64(canvasRect.W) / scale
	spanY := float64(canvasRect.H) / scale
	maxX := max(0, float64(g.gridPixelsW())-spanX)
	maxY := max(0, float64(g.gridPixelsH())-spanY)
	g.spriteCanvasPanX = min(max(g.spriteCanvasPanX, 0), maxX)
	g.spriteCanvasPanY = min(max(g.spriteCanvasPanY, 0), maxY)
}

func (g *Game) spriteCanvasOrigin() (float64, float64) {
	scale := g.canvasPixelScale()
	return float64(canvasRect.X) - g.spriteCanvasPanX*scale,
		float64(canvasRect.Y) - g.spriteCanvasPanY*scale
}

func (g *Game) changeSpriteCanvasZoom(delta int) bool {
	oldLevel := min(spriteCanvasZoomMax, max(spriteCanvasZoomMin, g.spriteCanvasZoomLevel))
	newLevel := min(spriteCanvasZoomMax, max(spriteCanvasZoomMin, oldLevel+delta))
	if newLevel == oldLevel {
		return false
	}
	oldScale := g.canvasPixelScale()
	focusX := g.spriteCanvasPanX + float64(canvasRect.W)/(2*oldScale)
	focusY := g.spriteCanvasPanY + float64(canvasRect.H)/(2*oldScale)
	g.spriteCanvasZoomLevel = newLevel
	newScale := g.canvasPixelScale()
	g.spriteCanvasPanX = focusX - float64(canvasRect.W)/(2*newScale)
	g.spriteCanvasPanY = focusY - float64(canvasRect.H)/(2*newScale)
	g.clampSpriteCanvasPan()
	g.spriteTool = spriteToolMove
	g.spriteShapeActive = false
	g.statusMsg = fmt.Sprintf("canvas zoom: %d%%", int(spriteCanvasZoomFactor(newLevel)*100))
	return true
}

func spriteCanvasZoomButtonRect(delta int) editor.Rect {
	index := 0
	if delta > 0 {
		index = 1
	}
	return editor.Rect{
		X: canvasRect.X + index*36,
		Y: canvasRect.Y + canvasRect.H + spriteCanvasFooterGap,
		W: 30,
		H: max(18, uiSmallTextHeight),
	}
}

func (g *Game) updateSpriteCanvasView(mx, my int) {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		switch {
		case rectContains(spriteCanvasZoomButtonRect(-1), mx, my):
			g.changeSpriteCanvasZoom(-1)
		case rectContains(spriteCanvasZoomButtonRect(1), mx, my):
			g.changeSpriteCanvasZoom(1)
		}
	}
	middleDown := ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle)
	moveDown := g.spriteTool == spriteToolMove && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	middleStarted := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle)
	moveStarted := g.spriteTool == spriteToolMove && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	if (middleStarted || moveStarted) && rectContains(canvasRect, mx, my) {
		g.spriteCanvasPanning = true
		g.spriteCanvasLastMouseX, g.spriteCanvasLastMouseY = mx, my
	}
	if (middleDown || moveDown) && g.spriteCanvasPanning {
		g.panSpriteCanvasBy(mx-g.spriteCanvasLastMouseX, my-g.spriteCanvasLastMouseY)
		g.spriteCanvasLastMouseX, g.spriteCanvasLastMouseY = mx, my
	}
	if !middleDown && !moveDown {
		g.spriteCanvasPanning = false
	}
	// Grid dimensions can change through the existing keyboard and wheel
	// shortcuts; keep a retained zoom view inside the new selection bounds.
	g.clampSpriteCanvasPan()
}

func (g *Game) panSpriteCanvasBy(screenDX, screenDY int) {
	scale := g.canvasPixelScale()
	if scale <= 0 {
		return
	}
	g.spriteCanvasPanX -= float64(screenDX) / scale
	g.spriteCanvasPanY -= float64(screenDY) / scale
	g.clampSpriteCanvasPan()
}

func (g *Game) drawSpriteCanvasZoomControls(screen *ebiten.Image) {
	for _, control := range []struct {
		delta int
		label string
	}{{-1, "-"}, {1, "+"}} {
		enabled := control.delta < 0 && g.spriteCanvasZoomLevel > spriteCanvasZoomMin ||
			control.delta > 0 && g.spriteCanvasZoomLevel < spriteCanvasZoomMax
		r := spriteCanvasZoomButtonRect(control.delta)
		if enabled {
			drawTactileButton(screen, r, control.label, false, colorSprite, true)
			continue
		}
		fillRoundedRect(screen, r, 4, colorButtonFace)
		strokeRoundedRect(screen, r, 4, rgba(colorBorder, 90))
		drawSmallTextCentered(screen, control.label, r, rgba(colorDim, 120))
	}
}

// canvasPixelBounds maps sprite pixels onto shared integer boundaries. A
// fractional scale can therefore distribute remainder pixels across the
// cells while the selection still consumes the full canvas without seams.
func (g *Game) canvasPixelBounds(x, y, width, height int) editor.Rect {
	scale := g.canvasPixelScale()
	originX, originY := g.spriteCanvasOrigin()
	x0 := int(math.Floor(originX + float64(x)*scale))
	y0 := int(math.Floor(originY + float64(y)*scale))
	x1 := int(math.Floor(originX + float64(x+width)*scale))
	y1 := int(math.Floor(originY + float64(y+height)*scale))
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
		visible, ok := intersectEditorRects(used, canvasRect)
		if !ok {
			return 0, 0, false
		}
		mx = max(visible.X, min(visible.X+visible.W-1, mx))
		my = max(visible.Y, min(visible.Y+visible.H-1, my))
	} else if !rectContains(canvasRect, mx, my) || !rectContains(used, mx, my) {
		return 0, 0, false
	}
	scale := g.canvasPixelScale()
	originX, originY := g.spriteCanvasOrigin()
	col = int(math.Ceil((float64(mx)-originX+1)/scale)) - 1
	row = int(math.Ceil((float64(my)-originY+1)/scale)) - 1
	return col, row, col >= 0 && row >= 0 && col < gridW && row < gridH
}

func (g *Game) paintPixel(id, px, py, colorIndex int) bool {
	cols := g.spriteCols()
	if cols == 0 || id < 0 || id >= g.spriteN() {
		return false
	}
	if g.spriteIsReserved(id) {
		g.statusMsg = reservedSpriteLockedMessage(id)
		return false
	}
	if g.pixelAt((id%cols)*8+px, (id/cols)*8+py) == colorIndex {
		return false
	}
	var err error
	if g.objEditing {
		err = g.model.SetObjSpritePixel(id, px, py, colorIndex)
	} else {
		err = g.model.SetSpritePixel(id, px, py, colorIndex)
	}
	if err != nil {
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
	originX, originY := g.spriteCanvasOrigin()
	drawCheckerboardColors(screen, canvasRect, colorSpriteCheckerA, colorSpriteCheckerB)
	g.drawSpritePixels(screen, originX, originY, scale, scale, baseCol*8, baseRow*8, gridW, gridH, true, canvasRect)
	g.drawPixelCanvasGrid(screen, originX, originY, scale, gridW, gridH)
	g.drawSpriteShapePreview(screen)
	g.drawReservedSpriteCanvasLock(screen)
	strokeRoundedRect(screen, editor.Rect{X: canvasRect.X - 1, Y: canvasRect.Y - 1, W: canvasRect.W + 2, H: canvasRect.H + 2}, 5, colorBorder)
	if g.theme.level != effectsClean {
		mx, my := editorCursorPosition()
		if col, row, ok := g.spriteCanvasCellAt(mx, my, false); ok {
			if r, visible := intersectEditorRects(g.canvasPixelBounds(col, row, 1, 1), canvasRect); visible {
				strokeRect(screen, r, colorSprite)
			}
		}
		if g.theme.inkPulse > 0 {
			if col, row, ok := g.spriteCanvasCellAt(g.theme.inkX, g.theme.inkY, false); ok {
				expand := (12 - g.theme.inkPulse) / 3
				alpha := uint8(min(190, g.theme.inkPulse*15))
				r := g.canvasPixelBounds(col, row, 1, 1)
				r = editor.Rect{X: r.X - expand, Y: r.Y - expand, W: r.W + expand*2, H: r.H + expand*2}
				r, visible := intersectEditorRects(r, canvasRect)
				if !visible {
					return
				}
				strokeRect(screen, r, rgba(colorSprite, alpha))
			}
		}
	}

}

// drawPixelCanvasGrid uses the shared sprite-grid treatment, including minor
// pixel boundaries while the enlarged canvas has enough room for them.
func (g *Game) drawPixelCanvasGrid(screen *ebiten.Image, originX, originY, scale float64, gridW, gridH int) {
	if screen == nil || scale <= 1 || gridW <= 0 || gridH <= 0 {
		return
	}
	drawSpritePixelGrid(screen, originX, originY, scale, scale, gridW, gridH, canvasRect, true)
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

// drawReservedSpriteCanvasLock makes immutable cells unmistakable while still
// allowing sprite 0 to remain selected as the map eraser.
func (g *Game) drawReservedSpriteCanvasLock(screen *ebiten.Image) {
	baseRow, baseCol := g.baseSpriteRowCol()
	cols := g.spriteCols()
	if cols == 0 || g.spriteN() == 0 {
		return
	}
	id := baseRow*cols + baseCol
	if !g.objEditing && id == editor.ReservedSpriteID {
		r := g.canvasPixelBounds(0, 0, 8, 8)
		var visible bool
		r, visible = intersectEditorRects(r, canvasRect)
		if !visible {
			return
		}
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
		return
	}
	if !g.spriteIsReserved(id) || g.spriteGridRows() != 1 || g.spriteGridCols() != 1 {
		return
	}
	r := g.canvasPixelBounds(0, 0, 8, 8)
	var visible bool
	r, visible = intersectEditorRects(r, canvasRect)
	if !visible {
		return
	}
	drawSpriteLockOverlay(screen, r)
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
	return editor.Rect{X: flagsRect.X + 4 + i*(width+gap), Y: flagsRect.Y, W: width, H: spriteButtonHeight}
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
	drawSpritePanelTitle(screen, spriteFlagsPanelRect, "SPRITE FLAGS · LINK")
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
	height := max(1, sheetPanelRect.H-spritePanelContentOffset()-10)
	y := spritePanelContentY(sheetPanelRect)
	if g.objEditing {
		height = max(1, height-animationDockHeight-poseStripGap-objPageBarHeight)
		y += objPageBarHeight
	} else if g.showsPoseStrip() {
		height = max(1, height-poseStripHeight()-poseStripGap)
	}
	return editor.Rect{
		X: sheetPanelRect.X + 10,
		Y: y,
		W: max(1, sheetPanelRect.W-20),
		H: height,
	}
}

func (g *Game) fitSheetCamera() {
	if g.objEditing {
		g.fitObjPageCamera()
		return
	}
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
	if g.objEditing {
		g.clampObjPageCamera(canvas)
		return
	}
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

// spritesheetDragSelection turns an anchor and the current pointer tile into
// the rectangular multi-sprite selection used by the editing canvas. The
// cursor can be outside the sheet while dragging; both axes stay bounded by
// the sheet and by the editor's 16x16 selection limit.
func spritesheetDragSelection(anchorCol, anchorRow, cursorCol, cursorRow, sheetCols, sheetRows int) (col, row, cols, rows int, ok bool) {
	return spritesheetDragSelectionLimit(anchorCol, anchorRow, cursorCol, cursorRow, sheetCols, sheetRows, gridCellsMax, gridCellsMax)
}

func spritesheetDragSelectionLimit(anchorCol, anchorRow, cursorCol, cursorRow, sheetCols, sheetRows, limitCols, limitRows int) (col, row, cols, rows int, ok bool) {
	if sheetCols <= 0 || sheetRows <= 0 || anchorCol < 0 || anchorCol >= sheetCols || anchorRow < 0 || anchorRow >= sheetRows {
		return 0, 0, 0, 0, false
	}
	cursorCol = min(max(cursorCol, max(0, anchorCol-limitCols+1)), min(sheetCols-1, anchorCol+limitCols-1))
	cursorRow = min(max(cursorRow, max(0, anchorRow-limitRows+1)), min(sheetRows-1, anchorRow+limitRows-1))
	col, row = min(anchorCol, cursorCol), min(anchorRow, cursorRow)
	return col, row, max(anchorCol, cursorCol) - col + 1, max(anchorRow, cursorRow) - row + 1, true
}

func (g *Game) selectSpritesheetDrag(cursorCol, cursorRow, sheetCols, sheetRows int) {
	if g.objEditing {
		p := g.currentObjPageRect()
		cursorCol = min(max(cursorCol, p.X/8), (p.X+p.W)/8-1)
		cursorRow = min(max(cursorRow, p.Y/8), (p.Y+p.H)/8-1)
	}
	col, row, cols, rows, ok := spritesheetDragSelectionLimit(
		g.sheetSelectionAnchorCol,
		g.sheetSelectionAnchorRow,
		cursorCol,
		cursorRow,
		sheetCols,
		sheetRows,
		g.spriteSelectionLimitCols(), g.spriteSelectionLimitRows(),
	)
	if !ok {
		return
	}
	g.currentSprite = row*sheetCols + col
	g.gridCols, g.gridRows = cols, rows
	g.clampSpriteCanvasPan()
	g.syncPoseSelectionFromSprite(g.currentSprite)
}

func (g *Game) updateSpritesheetPanel(mx, my int) {
	canvas := g.sheetGridCanvas()
	cols, rows := g.spriteCols(), g.spriteRows()

	if rectContains(canvas, mx, my) {
		if _, yoff := ebiten.Wheel(); yoff != 0 {
			g.sheetCam.Y -= yoff
			g.clampSheetCamera(canvas)
		}
		if g.spritePaste != nil {
			col, row, ok := g.sheetCam.ScreenToTile(mx, my, canvas)
			if ok && col >= 0 && row >= 0 && col < cols && row < rows && (!g.objEditing || g.objPageContains(col, row)) {
				g.moveSpritePastePlacement(col, row)
				if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
					_ = g.commitSpritePastePlacement()
				}
			}
			g.sheetSelectionDragging = false
			g.clampSheetCamera(canvas)
			return
		}
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			col, row, ok := g.sheetCam.ScreenToTile(mx, my, canvas)
			if ok && col >= 0 && row >= 0 && col < cols && row < rows && (!g.objEditing || g.objPageContains(col, row)) {
				if g.showsPoseStrip() && ebiten.IsKeyPressed(ebiten.KeyShift) {
					g.togglePoseTile(row*cols + col)
					g.sheetSelectionDragging = false
					g.clampSheetCamera(canvas)
					return
				}
				g.sheetSelectionDragging = true
				g.sheetSelectionAnchorCol, g.sheetSelectionAnchorRow = col, row
				g.selectSpritesheetDrag(col, row, cols, rows)
			}
		}
	}
	if g.sheetSelectionDragging && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		// Keep extending the selection when the pointer briefly leaves the
		// canvas, matching native marquee-selection behavior at an edge.
		selectionX := min(max(mx, canvas.X), canvas.X+canvas.W-1)
		selectionY := min(max(my, canvas.Y), canvas.Y+canvas.H-1)
		if col, row, ok := g.sheetCam.ScreenToTile(selectionX, selectionY, canvas); ok {
			g.selectSpritesheetDrag(col, row, cols, rows)
		}
	}
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.sheetSelectionDragging = false
	}
	g.clampSheetCamera(canvas)
}

func (g *Game) drawSpritesheetPanel(screen *ebiten.Image) {
	canvas := g.sheetGridCanvas()
	cols, rows := g.spriteCols(), g.spriteRows()
	title := "SHARED TILES"
	if g.showsPoseStrip() {
		title = fmt.Sprintf("TILE SHEET  ·  %d FRAMES", len(g.authoredPoses()))
	}
	titleX, titleY := spritePanelTitlePosition(sheetPanelRect)
	meta := fmt.Sprintf("%d × %d  ·  %d TILES", cols, rows, cols*rows)
	metaY := spritePanelMetaY(sheetPanelRect)
	if g.objEditing {
		g.drawObjPageHeader(screen)
	} else {
		drawText(screen, title, titleX, titleY, colorText)
		drawSmallText(screen, meta, titleX+uiTextWidth(title)+smallTextWidth("  "), metaY, colorDim)
	}
	fillRoundedRect(screen, canvas, 5, colorInk)
	strokeRoundedRect(screen, canvas, 5, rgba(colorBorder, 180))
	board := g.sheetContentClip(canvas)
	drawCheckerboardColors(screen, board, colorSheetCheckerA, colorSheetCheckerB)

	minC, minR, maxC, maxR := g.sheetCam.VisibleBounds(canvas, cols, rows)
	if g.objEditing {
		p := g.currentObjPageRect()
		minC, minR = max(minC, p.X/8), max(minR, p.Y/8)
		maxC, maxR = min(maxC, (p.X+p.W)/8), min(maxR, (p.Y+p.H)/8)
	}
	tileW, tileH := g.sheetCam.TileSize(), g.sheetCam.TileSizeY()
	for row := minR; row < maxR; row++ {
		for col := minC; col < maxC; col++ {
			cell := sheetCameraRect(g.sheetCam, canvas, col, row, 1, 1)
			id := row*cols + col
			sx, sy := g.sheetCam.WorldToScreen(col, row, canvas)
			g.drawSpriteThumb(screen, id, sx, sy, tileW, tileH, cell)
			if !g.objEditing && spriteSheetCellShowsLock(id) {
				drawSpriteLockOverlay(screen, cell)
			}
			if !g.objEditing && g.tileLinked(id) {
				drawTileLinkMarker(screen, cell)
			}
		}
	}
	sheetLeft, sheetTop := g.sheetCam.WorldToScreen(0, 0, canvas)
	gridW, gridH := cols*8, rows*8
	if g.objEditing {
		p := g.currentObjPageRect()
		sheetLeft, sheetTop = g.sheetCam.WorldToScreen(p.X/8, p.Y/8, canvas)
		gridW, gridH = p.W, p.H
	}
	// Keep the sheet at tile-level resolution. Drawing every source-pixel line
	// at this fitted scale overwhelms the checkerboard and makes the sheet look
	// much brighter than the editing canvas.
	drawSpritePixelGrid(screen, sheetLeft, sheetTop, tileW/8, tileH/8, gridW, gridH, board, false)
	g.drawPoseSheetOverlays(screen, canvas)
	g.drawSpritePastePreview(screen, canvas)
	g.drawSelectionBorder(screen, canvas)
	g.drawObjTileMatches(screen, canvas)
	g.drawSpriteCutSource(screen, canvas)
}

// drawSpritePastePreview renders the clipboard image at its prospective sheet
// position. A translucent veil keeps transparent source pixels visible as part
// of the affected area; the source colors remain readable underneath a strong
// valid/invalid outline.
func (g *Game) drawSpritePastePreview(screen *ebiten.Image, canvas editor.Rect) {
	placement := g.spritePaste
	if screen == nil || placement == nil {
		return
	}
	region := sheetCameraRect(g.sheetCam, canvas, placement.startCol, placement.startRow, placement.tileCols(), placement.tileRows())
	clip := g.sheetContentClip(canvas)
	visible, ok := intersectEditorRects(region, clip)
	if !ok {
		return
	}
	accent := color.Color(colorSprite)
	if g.spritePasteTargetError(placement, placement.startCol, placement.startRow) != nil {
		accent = colorDanger
	}
	fillRect(screen, visible, rgba(accent, 48))

	originX, originY := g.sheetCam.WorldToScreen(placement.startCol, placement.startRow, canvas)
	scaleX, scaleY := g.sheetCam.TileSize()/8, g.sheetCam.TileSizeY()/8
	for py := 0; py < placement.pixelHeight; py++ {
		for px := 0; px < placement.pixelWidth; px++ {
			pixel := placement.pixels[py*placement.pixelWidth+px]
			if isEraserIndex(pixel) {
				continue
			}
			x, w := scaledRun(originX, px, scaleX)
			y, h := scaledRun(originY, py, scaleY)
			target, targetVisible := intersectEditorRects(editor.Rect{X: x, Y: y, W: w, H: h}, clip)
			if targetVisible {
				fillRect(screen, target, rgba(g.swatch(pixel), 185))
			}
		}
	}
	if outline, outlineVisible := spritesheetSelectionOutlineRect(region, clip); outlineVisible {
		strokeRect(screen, outline, rgba(accent, 245))
		if outline.W > 4 && outline.H > 4 {
			strokeRect(screen, editor.Rect{X: outline.X + 1, Y: outline.Y + 1, W: outline.W - 2, H: outline.H - 2}, rgba(accent, 115))
		}
	}
}

func (g *Game) drawSpriteThumb(screen *ebiten.Image, id int, sx, sy, tileW, tileH float64, clips ...editor.Rect) {
	cols := g.spriteCols()
	if cols == 0 || id < 0 || id >= g.spriteN() {
		return
	}
	scaleX := tileW / 8
	scaleY := tileH / 8
	baseCol := id % cols
	baseRow := id / cols
	clip := editor.Rect{X: 0, Y: 0, W: screenWidth, H: screenHeight}
	if len(clips) > 0 {
		clip = clips[0]
	}
	var visible bool
	clip, visible = intersectEditorRects(clip, spriteThumbRect(sx, sy, tileW, tileH))
	if !visible {
		return
	}
	g.drawSpritePixels(screen, sx, sy, scaleX, scaleY, baseCol*8, baseRow*8, 8, 8, true, clip)
}

// spriteThumbRect converts fractional camera geometry into shared integer
// edges. Adjacent cells therefore abut exactly and every thumbnail, lock
// overlay, and selection marker can use the same ownership boundary.
func spriteThumbRect(sx, sy, tileW, tileH float64) editor.Rect {
	left, top := int(math.Floor(sx)), int(math.Floor(sy))
	right, bottom := int(math.Floor(sx+tileW)), int(math.Floor(sy+tileH))
	return editor.Rect{X: left, Y: top, W: max(0, right-left), H: max(0, bottom-top)}
}

func sheetCameraRect(camera editor.Camera, canvas editor.Rect, col, row, cols, rows int) editor.Rect {
	x1, y1 := camera.WorldToScreen(col, row, canvas)
	x2, y2 := camera.WorldToScreen(col+cols, row+rows, canvas)
	return spriteThumbRect(x1, y1, x2-x1, y2-y1)
}

// drawSpritePixels is the shared indexed-pixel renderer for the editing
// canvas and each fitted spritesheet cell. Keeping the source lookup and
// palette conversion here prevents the two views from drifting visually.
func (g *Game) drawSpritePixels(screen *ebiten.Image, originX, originY, scaleX, scaleY float64, sourceX, sourceY, pixelW, pixelH int, subtleTransparency bool, clip editor.Rect) {
	if screen == nil || scaleX <= 0 || scaleY <= 0 || pixelW <= 0 || pixelH <= 0 {
		return
	}
	for py := 0; py < pixelH; py++ {
		for px := 0; px < pixelW; px++ {
			pixel := g.pixelAt(sourceX+px, sourceY+py)
			x, w := scaledRun(originX, px, scaleX)
			y, h := scaledRun(originY, py, scaleY)
			target := editor.Rect{X: x, Y: y, W: w, H: h}
			var visible bool
			target, visible = intersectEditorRects(target, clip)
			if !visible {
				continue
			}
			if subtleTransparency && isEraserIndex(pixel) {
				// The caller paints one checkerboard under the complete surface.
				// Leaving transparent pixels untouched replaces hundreds of
				// thousands of tiny vector draws on a large empty spritesheet.
				continue
			}
			g.fillColor(screen, target, pixel)
		}
	}
}

func intersectEditorRects(a, b editor.Rect) (editor.Rect, bool) {
	left := max(a.X, b.X)
	top := max(a.Y, b.Y)
	right := min(a.X+a.W, b.X+b.W)
	bottom := min(a.Y+a.H, b.Y+b.H)
	if right <= left || bottom <= top {
		return editor.Rect{}, false
	}
	return editor.Rect{X: left, Y: top, W: right - left, H: bottom - top}, true
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
	selection := sheetCameraRect(g.sheetCam, canvas, baseCol, baseRow, g.spriteGridCols(), g.spriteGridRows())
	r, ok := spritesheetSelectionOutlineRect(selection, g.sheetContentClip(canvas))
	if !ok {
		return
	}
	strokeRect(screen, r, rgba(colorSprite, 235))
}

func spritesheetSelectionOutlineRect(selection, canvas editor.Rect) (editor.Rect, bool) {
	visible, ok := intersectEditorRects(selection, canvas)
	if !ok || visible.W <= 2 || visible.H <= 2 {
		return editor.Rect{}, false
	}
	return editor.Rect{X: visible.X + 1, Y: visible.Y + 1, W: visible.W - 2, H: visible.H - 2}, true
}

func cycleGridCells(n, delta int) int { return cycleGridCellsLimit(n, delta, gridCellsMax) }

func cycleGridCellsLimit(n, delta, limit int) int {
	span := limit - gridCellsMin + 1
	if n < gridCellsMin || n > limit {
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
	g.gridCols = cycleGridCellsLimit(g.spriteGridCols(), delta, g.spriteSelectionLimitCols())
}

func (g *Game) nudgeGridRows(delta int) {
	g.gridRows = cycleGridCellsLimit(g.spriteGridRows(), delta, g.spriteSelectionLimitRows())
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
	mx, my := editorCursorPosition()
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
	if g.objEditing && !g.objPageContains(newCol, newRow) {
		return
	}
	g.currentSprite = newRow*g.spriteCols() + newCol
	g.syncPoseSelectionFromSprite(g.currentSprite)
	g.lastNav = now
}

func (g *Game) updateCopyPaste() {
	if (g.spritePaste != nil || g.copiedSprite.cut != nil) && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.cancelSpritePastePlacement()
		return
	}
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	if g.spritePaste != nil && !mod && !ebiten.IsKeyPressed(ebiten.KeyAlt) && inpututil.IsKeyJustPressed(ebiten.KeyD) {
		g.toggleSpritePasteDither()
		return
	}
	if !mod {
		if g.spritePaste == nil && (inpututil.IsKeyJustPressed(ebiten.KeyDelete) || inpututil.IsKeyJustPressed(ebiten.KeyBackspace)) {
			g.eraseCurrentSpriteSelection()
		}
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyC) {
		g.spritePaste = nil
		g.statusMsg = ""
		g.copyCurrentSprite()
	}
	if g.objEditing && inpututil.IsKeyJustPressed(ebiten.KeyX) {
		g.cutCurrentSprite()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyV) || (g.objEditing && inpututil.IsKeyJustPressed(ebiten.KeyP)) {
		g.pasteSpriteClipboard()
	}
}

func (g *Game) copyCurrentSprite() {
	g.spritePaste = nil
	sheet := g.activeSheet()
	startCol, startRow, width, height, ok := g.selectionPixelBlock()
	if sheet == nil || !ok {
		g.hasCopied = false
		g.copiedSprite = spriteCopyBuffer{}
		return
	}
	pixels := spriteSelectionPixels(sheet, startCol, startRow, width, height)
	g.copiedSprite = spriteCopyBuffer{width: width, height: height, pixels: pixels}
	g.hasCopied = true
	g.captureSpriteCopyClipboard()
	if g.objEditing {
		g.statusMsg = "Tiles copied · Cmd/Ctrl+P or V, then click a destination · Esc cancels"
		g.requestRender()
	}
}

func (g *Game) pasteCurrentSprite() {
	if !g.hasCopied || g.copiedSprite.width <= 0 || g.copiedSprite.height <= 0 {
		return
	}
	startRow, startCol := g.baseSpriteRowCol()
	placement := &spritePastePlacement{
		pixels:      append([]int(nil), g.copiedSprite.pixels...),
		pixelWidth:  g.copiedSprite.width,
		pixelHeight: g.copiedSprite.height,
		startCol:    startCol,
		startRow:    startRow,
	}
	if err := g.spritePasteTargetError(placement, startCol, startRow); err != nil {
		id := startRow*g.spriteCols() + startCol
		if g.spriteIsReserved(id) {
			g.statusMsg = reservedSpriteLockedMessage(id)
			return
		}
		g.statusMsg = err.Error()
		return
	}
	changed, err := g.setActiveSpritePixelBlock(startCol, startRow, placement.pixelWidth, placement.pixelHeight, placement.pixels)
	if err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.gridCols, g.gridRows = placement.tileCols(), placement.tileRows()
	g.spriteShapeActive = false
	g.statusMsg = ""
	if !changed {
		return
	}
	g.dirty = true
	if err := g.refreshPreview(); err != nil {
		g.statusMsg = err.Error()
	}
}

func (g *Game) eraseCurrentSpriteSelection() {
	startCol, startRow, width, height, ok := g.selectionPixelBlock()
	if !ok {
		return
	}
	changed, err := g.model.EraseSpritePixelBlock(g.objEditing, startCol, startRow, width, height)
	if err != nil {
		id := startRow*g.spriteCols() + startCol
		if g.spriteIsReserved(id) {
			g.statusMsg = reservedSpriteLockedMessage(id)
			return
		}
		g.statusMsg = err.Error()
		return
	}
	g.statusMsg = ""
	if !changed {
		return
	}
	g.dirty = true
	if err := g.refreshPreview(); err != nil {
		g.statusMsg = err.Error()
	}
}
