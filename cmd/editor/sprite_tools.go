package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// spriteTool is deliberately small and familiar. The toolbar combines these
// persistent modes with two immediate flip transformations. Zero is pencil so
// a newly opened editor behaves exactly like the original single-tool canvas.
type spriteTool uint8

const (
	spriteToolPencil spriteTool = iota
	spriteToolEraser
	spriteToolFill
	spriteToolLine
	spriteToolRect
	spriteToolCircle
	spriteToolMove
)

// Move leads the toolbar because it changes how the canvas pointer behaves,
// while Pencil intentionally remains enum zero and therefore the editor's
// default tool. Toolbar order and persistent state order are kept separate so
// adding a navigation tool cannot silently change the startup behavior.
var spriteToolbarTools = [...]spriteTool{
	spriteToolMove,
	spriteToolPencil,
	spriteToolEraser,
	spriteToolFill,
	spriteToolLine,
	spriteToolRect,
	spriteToolCircle,
}

const (
	spriteToolButtonFlipH = len(spriteToolbarTools)
	spriteToolButtonFlipV = len(spriteToolbarTools) + 1
	spriteToolButtonCount = len(spriteToolbarTools) + 2
)

func (t spriteTool) String() string {
	switch t {
	case spriteToolMove:
		return "MOVE"
	case spriteToolEraser:
		return "ERASER"
	case spriteToolFill:
		return "FILL"
	case spriteToolLine:
		return "LINE"
	case spriteToolRect:
		return "RECTANGLE"
	case spriteToolCircle:
		return "CIRCLE"
	default:
		return "PENCIL"
	}
}

func spriteToolForButton(index int) (spriteTool, bool) {
	if index < 0 || index >= len(spriteToolbarTools) {
		return spriteToolPencil, false
	}
	return spriteToolbarTools[index], true
}

func spriteToolButtonName(index int) string {
	if tool, ok := spriteToolForButton(index); ok {
		return tool.String()
	}
	if index == spriteToolButtonFlipH {
		return "FLIP H · SHIFT+H"
	}
	return "FLIP V · SHIFT+V"
}

func spriteToolButtonRect(index int) editor.Rect {
	const (
		sideInset = 5
		gap       = 4
	)
	buttonW := max(1, (spriteToolsRect.W-2*sideInset-gap*(spriteToolButtonCount-1))/spriteToolButtonCount)
	return editor.Rect{
		X: spriteToolsRect.X + sideInset + index*(buttonW+gap),
		Y: spriteToolsRect.Y,
		W: buttonW,
		H: spriteButtonHeight,
	}
}

func (g *Game) updateSpriteTools(mx, my int) {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	for i := 0; i < spriteToolButtonCount; i++ {
		if !rectContains(spriteToolButtonRect(i), mx, my) {
			continue
		}
		switch i {
		case spriteToolButtonFlipH:
			g.flipSpriteSelection(true)
		case spriteToolButtonFlipV:
			g.flipSpriteSelection(false)
		default:
			tool, ok := spriteToolForButton(i)
			if !ok {
				return
			}
			g.spriteTool = tool
			g.spriteShapeActive = false
			g.statusMsg = "Sprite tool: " + g.spriteTool.String()
		}
		return
	}
}

func (g *Game) updateSpriteToolShortcuts() bool {
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyAlt)
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	if !mod && shift {
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeyH):
			g.flipSpriteSelection(true)
			return true
		case inpututil.IsKeyJustPressed(ebiten.KeyV):
			g.flipSpriteSelection(false)
			return true
		}
	}
	if mod || shift {
		return false
	}
	keys := [...]struct {
		key  ebiten.Key
		tool spriteTool
	}{
		{ebiten.KeyM, spriteToolMove},
		{ebiten.KeyP, spriteToolPencil},
		{ebiten.KeyE, spriteToolEraser},
		{ebiten.KeyF, spriteToolFill},
		{ebiten.KeyL, spriteToolLine},
		{ebiten.KeyR, spriteToolRect},
		{ebiten.KeyC, spriteToolCircle},
	}
	for _, binding := range keys {
		if inpututil.IsKeyJustPressed(binding.key) {
			g.spriteTool = binding.tool
			g.spriteShapeActive = false
			g.statusMsg = "Sprite tool: " + g.spriteTool.String()
			return true
		}
	}
	return false
}

func (g *Game) drawSpriteTools(screen *ebiten.Image) {
	mx, my := ebiten.CursorPosition()
	title := g.spriteTool.String()
	for i := 0; i < spriteToolButtonCount; i++ {
		if rectContains(spriteToolButtonRect(i), mx, my) {
			title = spriteToolButtonName(i)
			break
		}
	}
	drawSpritePanelTitle(screen, spriteToolsPanelRect, "DRAW TOOLS")
	title = truncateTextToWidth(title, spriteToolsRect.W/2, uiSmallFace)
	drawSmallText(screen, title, spriteToolsPanelRect.X+spriteToolsPanelRect.W-spritePanelTitleInsetX-smallTextWidth(title), spritePanelMetaY(spriteToolsPanelRect), colorSprite)
	for i := 0; i < spriteToolButtonCount; i++ {
		r := spriteToolButtonRect(i)
		tool, persistent := spriteToolForButton(i)
		active := persistent && g.spriteTool == tool
		drawTactileButton(screen, r, "", active, colorSprite, true)
		drawSpriteToolIcon(screen, r, i, map[bool]color.Color{false: colorText, true: colorSprite}[active])
	}
}

func drawSpriteToolIcon(screen *ebiten.Image, r editor.Rect, index int, clr color.Color) {
	if screen == nil {
		return
	}
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	if tool, ok := spriteToolForButton(index); ok {
		switch tool {
		case spriteToolMove:
			drawMoveToolIcon(screen, cx, cy, clr)
		case spriteToolPencil:
			drawLine(screen, cx-7, cy+6, cx+6, cy-7, clr)
			drawLine(screen, cx-5, cy+7, cx+8, cy-6, clr)
			fillRect(screen, editor.Rect{X: cx - 8, Y: cy + 5, W: 4, H: 4}, colorHighlight)
		case spriteToolEraser:
			drawEraserToolIcon(screen, cx, cy, clr)
		case spriteToolFill:
			drawFillToolIcon(screen, cx, cy, clr)
		case spriteToolLine:
			drawLine(screen, cx-8, cy+7, cx+8, cy-7, clr)
		case spriteToolRect:
			strokeRoundedRect(screen, editor.Rect{X: cx - 8, Y: cy - 7, W: 16, H: 14}, 2, clr)
		case spriteToolCircle:
			vector.StrokeCircle(screen, float32(cx), float32(cy), 8, 1, clr, true)
		}
		return
	}
	switch index {
	case spriteToolButtonFlipH:
		drawLine(screen, cx, cy-8, cx, cy+8, colorDim)
		drawLine(screen, cx-2, cy, cx-8, cy, clr)
		drawLine(screen, cx-8, cy, cx-5, cy-3, clr)
		drawLine(screen, cx-8, cy, cx-5, cy+3, clr)
		drawLine(screen, cx+2, cy, cx+8, cy, clr)
		drawLine(screen, cx+8, cy, cx+5, cy-3, clr)
		drawLine(screen, cx+8, cy, cx+5, cy+3, clr)
	case spriteToolButtonFlipV:
		drawLine(screen, cx-8, cy, cx+8, cy, colorDim)
		drawLine(screen, cx, cy-2, cx, cy-8, clr)
		drawLine(screen, cx, cy-8, cx-3, cy-5, clr)
		drawLine(screen, cx, cy-8, cx+3, cy-5, clr)
		drawLine(screen, cx, cy+2, cx, cy+8, clr)
		drawLine(screen, cx, cy+8, cx-3, cy+5, clr)
		drawLine(screen, cx, cy+8, cx+3, cy+5, clr)
	}
}

func drawMoveToolIcon(screen *ebiten.Image, cx, cy int, clr color.Color) {
	drawLine(screen, cx-8, cy, cx+8, cy, clr)
	drawLine(screen, cx, cy-8, cx, cy+8, clr)
	drawLine(screen, cx-8, cy, cx-4, cy-3, clr)
	drawLine(screen, cx-8, cy, cx-4, cy+3, clr)
	drawLine(screen, cx+8, cy, cx+4, cy-3, clr)
	drawLine(screen, cx+8, cy, cx+4, cy+3, clr)
	drawLine(screen, cx, cy-8, cx-3, cy-4, clr)
	drawLine(screen, cx, cy-8, cx+3, cy-4, clr)
	drawLine(screen, cx, cy+8, cx-3, cy+4, clr)
	drawLine(screen, cx, cy+8, cx+3, cy+4, clr)
}

func drawFilledIconPath(screen *ebiten.Image, path *vector.Path, fill, outline color.Color) {
	if screen == nil || path == nil {
		return
	}
	fillOp := &vector.DrawPathOptions{}
	fillOp.AntiAlias = true
	fillOp.ColorScale.ScaleWithColor(fill)
	vector.FillPath(screen, path, nil, fillOp)
	strokeOp := &vector.DrawPathOptions{}
	strokeOp.AntiAlias = true
	strokeOp.ColorScale.ScaleWithColor(outline)
	vector.StrokePath(screen, path, &vector.StrokeOptions{Width: 1.5, MiterLimit: 4}, strokeOp)
}

func drawEraserToolIcon(screen *ebiten.Image, cx, cy int, clr color.Color) {
	// A broad two-part parallelogram reads as an eraser before it reads as a
	// decorative diamond. The large coral rubber end makes the tool distinct
	// from the monochrome shape icons next to it.
	var body vector.Path
	body.MoveTo(float32(cx-10), float32(cy+2))
	body.LineTo(float32(cx-3), float32(cy-8))
	body.LineTo(float32(cx+10), float32(cy+1))
	body.LineTo(float32(cx+3), float32(cy+10))
	body.Close()
	drawFilledIconPath(screen, &body, colorText, colorInk)

	var rubber vector.Path
	rubber.MoveTo(float32(cx-10), float32(cy+2))
	rubber.LineTo(float32(cx-3), float32(cy-8))
	rubber.LineTo(float32(cx+2), float32(cy-5))
	rubber.LineTo(float32(cx-5), float32(cy+6))
	rubber.Close()
	drawFilledIconPath(screen, &rubber, colorMusic, colorInk)
	drawLine(screen, cx+2, cy-5, cx-5, cy+6, colorInk)
	drawLine(screen, cx-10, cy+11, cx+9, cy+11, rgba(clr, 150))
}

func drawFillToolIcon(screen *ebiten.Image, cx, cy int, clr color.Color) {
	// A dark, outlined bucket with a visible liquid surface and pouring drop
	// stays legible at 36x30 without becoming a large white block.
	var bucket vector.Path
	bucket.MoveTo(float32(cx-8), float32(cy-4))
	bucket.LineTo(float32(cx+3), float32(cy-7))
	bucket.LineTo(float32(cx+8), float32(cy+3))
	bucket.LineTo(float32(cx-2), float32(cy+8))
	bucket.Close()
	drawFilledIconPath(screen, &bucket, colorPanel, clr)

	var liquid vector.Path
	liquid.MoveTo(float32(cx-6), float32(cy+1))
	liquid.LineTo(float32(cx+6), float32(cy-2))
	liquid.LineTo(float32(cx+8), float32(cy+3))
	liquid.LineTo(float32(cx-2), float32(cy+8))
	liquid.Close()
	drawFilledIconPath(screen, &liquid, colorSprite, colorInk)

	var handle vector.Path
	handle.MoveTo(float32(cx-8), float32(cy-4))
	handle.QuadTo(float32(cx-8), float32(cy-12), float32(cx+2), float32(cy-9))
	handle.QuadTo(float32(cx+7), float32(cy-8), float32(cx+8), float32(cy-3))
	handleStroke := &vector.DrawPathOptions{}
	handleStroke.AntiAlias = true
	handleStroke.ColorScale.ScaleWithColor(clr)
	vector.StrokePath(screen, &handle, &vector.StrokeOptions{Width: 1.5}, handleStroke)
	drawLine(screen, cx-6, cy+1, cx+6, cy-2, colorText)

	var drop vector.Path
	drop.MoveTo(float32(cx+11), float32(cy+4))
	drop.QuadTo(float32(cx+15), float32(cy+9), float32(cx+11), float32(cy+12))
	drop.QuadTo(float32(cx+7), float32(cy+9), float32(cx+11), float32(cy+4))
	drop.Close()
	drawFilledIconPath(screen, &drop, colorSprite, colorInk)
}

func (g *Game) selectionPixelTarget(x, y int) (id, px, py int, ok bool) {
	if x < 0 || y < 0 || x >= g.gridPixelsW() || y >= g.gridPixelsH() {
		return 0, 0, 0, false
	}
	baseRow, baseCol := g.baseSpriteRowCol()
	row, col := baseRow+y/8, baseCol+x/8
	if row < 0 || col < 0 || row >= g.spriteRows() || col >= g.spriteCols() {
		return 0, 0, 0, false
	}
	return row*g.spriteCols() + col, x % 8, y % 8, true
}

func (g *Game) selectionPixelAt(x, y int) (int, bool) {
	id, px, py, ok := g.selectionPixelTarget(x, y)
	if !ok {
		return 0, false
	}
	cols := g.spriteCols()
	return g.pixelAt((id%cols)*8+px, (id/cols)*8+py), true
}

func (g *Game) paintSelectionPixel(x, y, colorIndex int) bool {
	id, px, py, ok := g.selectionPixelTarget(x, y)
	return ok && g.paintPixel(id, px, py, colorIndex)
}

func (g *Game) fillSpriteSelection(startX, startY, replacement int) bool {
	id, _, _, ok := g.selectionPixelTarget(startX, startY)
	if !ok {
		return false
	}
	if id == editor.ReservedSpriteID {
		g.statusMsg = spriteZeroLockedMessage
		return false
	}
	target, _ := g.selectionPixelAt(startX, startY)
	if target == replacement {
		return false
	}
	w, h := g.gridPixelsW(), g.gridPixelsH()
	type point struct{ x, y int }
	queue := []point{{startX, startY}}
	seen := make([]bool, w*h)
	changed := false
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if p.x < 0 || p.y < 0 || p.x >= w || p.y >= h || seen[p.y*w+p.x] {
			continue
		}
		seen[p.y*w+p.x] = true
		pixelID, _, _, valid := g.selectionPixelTarget(p.x, p.y)
		if !valid || pixelID == editor.ReservedSpriteID {
			continue
		}
		value, _ := g.selectionPixelAt(p.x, p.y)
		if value != target {
			continue
		}
		changed = g.paintSelectionPixel(p.x, p.y, replacement) || changed
		queue = append(queue,
			point{p.x - 1, p.y}, point{p.x + 1, p.y},
			point{p.x, p.y - 1}, point{p.x, p.y + 1},
		)
	}
	if changed {
		g.statusMsg = fmt.Sprintf("Filled with color %d", replacement)
	}
	return changed
}

func visitRasterLine(x0, y0, x1, y1 int, visit func(x, y int)) {
	dx, dy := absInt(x1-x0), -absInt(y1-y0)
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		visit(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func visitSpriteShape(tool spriteTool, x0, y0, x1, y1 int, filled bool, visit func(x, y int)) {
	left, right := min(x0, x1), max(x0, x1)
	top, bottom := min(y0, y1), max(y0, y1)
	switch tool {
	case spriteToolLine:
		visitRasterLine(x0, y0, x1, y1, visit)
	case spriteToolRect:
		if filled {
			for y := top; y <= bottom; y++ {
				for x := left; x <= right; x++ {
					visit(x, y)
				}
			}
			return
		}
		visitRasterLine(left, top, right, top, visit)
		visitRasterLine(right, top, right, bottom, visit)
		visitRasterLine(right, bottom, left, bottom, visit)
		visitRasterLine(left, bottom, left, top, visit)
	case spriteToolCircle:
		rx, ry := float64(right-left)/2, float64(bottom-top)/2
		cx, cy := float64(left+right)/2, float64(top+bottom)/2
		if rx == 0 || ry == 0 {
			visitRasterLine(x0, y0, x1, y1, visit)
			return
		}
		if filled {
			for y := top; y <= bottom; y++ {
				for x := left; x <= right; x++ {
					dx := (float64(x) - cx) / (rx + .5)
					dy := (float64(y) - cy) / (ry + .5)
					if dx*dx+dy*dy <= 1 {
						visit(x, y)
					}
				}
			}
			return
		}
		steps := max(16, int(math.Ceil(2*math.Pi*max(rx, ry)*2)))
		prevX, prevY := int(math.Round(cx+rx)), int(math.Round(cy))
		for i := 1; i <= steps; i++ {
			a := 2 * math.Pi * float64(i) / float64(steps)
			x := int(math.Round(cx + rx*math.Cos(a)))
			y := int(math.Round(cy + ry*math.Sin(a)))
			visitRasterLine(prevX, prevY, x, y, visit)
			prevX, prevY = x, y
		}
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (g *Game) commitSpriteShape() bool {
	if g.spriteTool < spriteToolLine || g.spriteTool > spriteToolCircle {
		return false
	}
	changed := false
	visitSpriteShape(g.spriteTool, g.spriteShapeStartX, g.spriteShapeStartY, g.spriteShapeEndX, g.spriteShapeEndY, g.spriteShapeFilled, func(x, y int) {
		changed = g.paintSelectionPixel(x, y, g.currentColor) || changed
	})
	if changed {
		g.statusMsg = g.spriteTool.String() + " drawn"
	}
	return changed
}

func (g *Game) drawSpriteShapePreview(screen *ebiten.Image) {
	if !g.spriteShapeActive || g.spriteTool < spriteToolLine || g.spriteTool > spriteToolCircle {
		return
	}
	preview := rgba(g.swatch(g.currentColor), 174)
	if g.currentColor == eraserIndex {
		preview = rgba(colorDanger, 140)
	}
	visitSpriteShape(g.spriteTool, g.spriteShapeStartX, g.spriteShapeStartY, g.spriteShapeEndX, g.spriteShapeEndY, g.spriteShapeFilled, func(x, y int) {
		if _, _, _, ok := g.selectionPixelTarget(x, y); !ok {
			return
		}
		r, visible := intersectEditorRects(g.canvasPixelBounds(x, y, 1, 1), canvasRect)
		if !visible {
			return
		}
		r = editor.Rect{X: r.X + 1, Y: r.Y + 1, W: max(1, r.W-2), H: max(1, r.H-2)}
		fillRoundedRect(screen, r, min(3, min(r.W, r.H)/4), preview)
	})
}

func (g *Game) flipSpriteSelection(horizontal bool) bool {
	baseRow, baseCol := g.baseSpriteRowCol()
	if baseRow+g.spriteGridRows() > g.spriteRows() || baseCol+g.spriteGridCols() > g.spriteCols() {
		g.statusMsg = "Cannot flip: selection extends outside the spritesheet"
		return false
	}
	if baseRow == 0 && baseCol == 0 {
		g.statusMsg = spriteZeroLockedMessage
		return false
	}
	w, h := g.gridPixelsW(), g.gridPixelsH()
	pixels := make([]int, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pixels[y*w+x], _ = g.selectionPixelAt(x, y)
		}
	}
	changed := false
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sourceX, sourceY := x, y
			if horizontal {
				sourceX = w - 1 - x
			} else {
				sourceY = h - 1 - y
			}
			changed = g.paintSelectionPixel(x, y, pixels[sourceY*w+sourceX]) || changed
		}
	}
	axis := "vertically"
	if horizontal {
		axis = "horizontally"
	}
	if changed {
		g.statusMsg = "Sprite flipped " + axis
	} else {
		g.statusMsg = "Sprite is already symmetrical " + axis
	}
	return changed
}

// drawSpriteLockOverlay is presentation-only. It never touches sprite data;
// it simply makes the immutable transparent eraser cell read as a deliberate
// cartridge lock instead of a broken thumbnail.
func drawSpriteLockOverlay(screen *ebiten.Image, r editor.Rect) {
	if screen == nil || r.W <= 0 || r.H <= 0 {
		return
	}
	inset := max(1, min(r.W, r.H)/18)
	overlay := editor.Rect{X: r.X + inset, Y: r.Y + inset, W: r.W - inset*2, H: r.H - inset*2}
	strokeRoundedRect(screen, overlay, min(8, max(2, min(overlay.W, overlay.H)/8)), rgba(colorSprite, 150))
	size := min(26, max(7, min(overlay.W, overlay.H)/3))
	body := editor.Rect{X: overlay.X + (overlay.W-size)/2, Y: overlay.Y + overlay.H/2 - size/6, W: size, H: max(6, size*2/3)}
	shackle := editor.Rect{X: body.X + size/4, Y: body.Y - size/3, W: max(4, size/2), H: max(5, size/2)}
	strokeRoundedRect(screen, shackle, max(2, shackle.W/2), colorText)
	fillRoundedRect(screen, body, max(2, size/7), colorBorder)
	keyW := max(2, size/8)
	fillRoundedRect(screen, editor.Rect{X: body.X + body.W/2 - keyW/2, Y: body.Y + body.H/3, W: keyW, H: max(3, body.H/3)}, keyW/2, colorInk)
}
