package main

import (
	"fmt"
	"image/color"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type mapTool uint8

const (
	mapToolPaint mapTool = iota
	mapToolMoveArea
	mapToolCopyArea
)

func (tool mapTool) String() string {
	switch tool {
	case mapToolMoveArea:
		return "move area"
	case mapToolCopyArea:
		return "copy area"
	default:
		return "paint"
	}
}

func (tool mapTool) isAreaTool() bool {
	return tool == mapToolMoveArea || tool == mapToolCopyArea
}

func (tool mapTool) areaVerb() string {
	if tool == mapToolCopyArea {
		return "copy"
	}
	return "move"
}

func (tool mapTool) areaTitle() string {
	if tool == mapToolCopyArea {
		return "Copy Area"
	}
	return "Move Area"
}

type mapAreaSelection struct {
	layer         int
	x, y          int
	width, height int
}

func (selection *mapAreaSelection) contains(x, y int) bool {
	return selection != nil &&
		x >= selection.x && x < selection.x+selection.width &&
		y >= selection.y && y < selection.y+selection.height
}

func mapAreaFromDrag(layer, anchorX, anchorY, cursorX, cursorY, mapW, mapH int) *mapAreaSelection {
	if mapW <= 0 || mapH <= 0 || anchorX < 0 || anchorY < 0 || anchorX >= mapW || anchorY >= mapH {
		return nil
	}
	cursorX = min(max(cursorX, 0), mapW-1)
	cursorY = min(max(cursorY, 0), mapH-1)
	x, y := min(anchorX, cursorX), min(anchorY, cursorY)
	return &mapAreaSelection{
		layer:  layer,
		x:      x,
		y:      y,
		width:  max(anchorX, cursorX) - x + 1,
		height: max(anchorY, cursorY) - y + 1,
	}
}

func mapModifierSelectionRequested(tool mapTool, modifierDown, leftJustPressed, overMap, layerVisible bool) bool {
	return tool == mapToolPaint && modifierDown && leftJustPressed && overMap && layerVisible
}

func (g *Game) resetMapAreaInteraction() {
	if g == nil {
		return
	}
	g.mapArea = nil
	g.mapAreaSelecting = false
	g.mapAreaPlacing = false
	g.mapAreaAnchorX, g.mapAreaAnchorY = 0, 0
	g.mapAreaGrabX, g.mapAreaGrabY = 0, 0
	g.mapAreaTargetLayer = g.mapActiveLayer
	g.mapAreaTargetX, g.mapAreaTargetY = 0, 0
}

func (g *Game) mapAreaToolPrompt() string {
	if !g.mapLayerVisible[g.mapActiveLayer] {
		return fmt.Sprintf("BG%d is hidden: show it before selecting an area", g.mapActiveLayer)
	}
	return fmt.Sprintf("%s on BG%d: drag to select, or Cmd/Ctrl+A for the entire layer", g.mapTool.areaTitle(), g.mapActiveLayer)
}

func (g *Game) handleMapAreaTool(layout mapLayout) {
	mx, my := ebiten.CursorPosition()
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	leftJustPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	requested := mapToolPaint
	switch {
	case !mod && inpututil.IsKeyJustPressed(ebiten.KeyM):
		requested = mapToolMoveArea
	case !mod && inpututil.IsKeyJustPressed(ebiten.KeyC):
		requested = mapToolCopyArea
	case leftJustPressed && rectContains(layout.MoveArea, mx, my):
		requested = mapToolMoveArea
	case leftJustPressed && rectContains(layout.CopyArea, mx, my):
		requested = mapToolCopyArea
	}
	if requested.isAreaTool() {
		if g.mapTool == requested {
			g.mapTool = mapToolPaint
			g.resetMapAreaInteraction()
			g.statusMsg = "Map tool: paint"
		} else {
			g.mapTool = requested
			g.mapAreaSelecting = false
			g.mapAreaPlacing = false
			if g.mapArea == nil {
				g.mapAreaTargetLayer = g.mapActiveLayer
				g.statusMsg = g.mapAreaToolPrompt()
			} else {
				g.statusMsg = g.mapAreaSelectionStatus()
			}
		}
		g.requestRender()
		return
	}
	_, _, overMap := g.mapTargetTile(layout, mx, my)
	if mapModifierSelectionRequested(g.mapTool, mod, leftJustPressed, overMap, g.mapLayerVisible[g.mapActiveLayer]) {
		g.mapTool = mapToolMoveArea
		g.resetMapAreaInteraction()
		g.statusMsg = fmt.Sprintf("Selecting BG%d area", g.mapActiveLayer)
		g.requestRender()
		return
	}
	if g.mapTool.isAreaTool() && mod && inpututil.IsKeyJustPressed(ebiten.KeyA) {
		g.selectWholeMapArea()
		return
	}
	if g.mapTool.isAreaTool() && g.mapArea != nil &&
		(inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter)) {
		g.finishMapAreaPlacement(ebiten.IsKeyPressed(ebiten.KeyShift))
		return
	}
	if !g.mapTool.isAreaTool() || !inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return
	}

	switch {
	case g.mapAreaPlacing:
		g.mapAreaPlacing = false
		g.statusMsg = fmt.Sprintf("%s canceled; BG%d area remains selected", g.mapTool.areaTitle(), g.mapActiveLayer)
	case g.mapAreaSelecting || g.mapArea != nil:
		g.resetMapAreaInteraction()
		g.statusMsg = g.mapAreaToolPrompt()
	default:
		g.mapTool = mapToolPaint
		g.statusMsg = "Map tool: paint"
	}
	g.requestRender()
}

func (g *Game) selectWholeMapArea() {
	if g == nil || !g.mapTool.isAreaTool() || !g.mapLayerVisible[g.mapActiveLayer] {
		return
	}
	w, h := g.mapBounds()
	if w <= 0 || h <= 0 {
		return
	}
	g.mapArea = &mapAreaSelection{layer: g.mapActiveLayer, width: w, height: h}
	g.mapAreaSelecting = false
	g.mapAreaPlacing = false
	g.mapAreaTargetLayer = g.mapActiveLayer
	g.mapAreaTargetX, g.mapAreaTargetY = 0, 0
	g.statusMsg = fmt.Sprintf("Entire BG%d selected: choose a target BG, then press Enter or drag", g.mapActiveLayer)
	g.requestRender()
}

func (g *Game) setMapAreaTargetLayer(layer int) bool {
	if g == nil || g.mapArea == nil || !g.mapTool.isAreaTool() || layer < 0 || layer >= len(g.mapLayerVisible) {
		return false
	}
	g.mapAreaTargetLayer = layer
	g.mapAreaTargetX, g.mapAreaTargetY = g.mapArea.x, g.mapArea.y
	g.mapAreaSelecting = false
	g.mapAreaPlacing = false
	g.statusMsg = g.mapAreaSelectionStatus()
	g.requestRender()
	return true
}

func (g *Game) mapAreaSelectionStatus() string {
	selection := g.mapArea
	if selection == nil {
		return g.mapAreaToolPrompt()
	}
	if g.mapAreaTargetLayer != selection.layer {
		if !g.mapLayerVisible[g.mapAreaTargetLayer] {
			return fmt.Sprintf("Target BG%d is hidden: show it before %s", g.mapAreaTargetLayer, g.mapTool.areaVerb())
		}
		if collisions := g.mapAreaCollisionCount(); collisions > 0 {
			return fmt.Sprintf("BG%d has %d occupied target tile(s): hold Shift with Enter or release to overwrite", g.mapAreaTargetLayer, collisions)
		}
		return fmt.Sprintf("BG%d > BG%d selected: Enter keeps position, or drag to reposition", selection.layer, g.mapAreaTargetLayer)
	}
	return fmt.Sprintf("BG%d area %dx%d selected: drag to %s; click another BG for cross-layer transfer", selection.layer, selection.width, selection.height, g.mapTool.areaVerb())
}

func (g *Game) mapDragTargetTile(layout mapLayout, mx, my int) (int, int, bool) {
	left, top, right, bottom, ok := g.mapGuideClip(layout)
	if !ok {
		return 0, 0, false
	}
	mx = min(max(mx, left), right)
	my = min(max(my, top), bottom)
	x, y, ok := g.mapCam.ScreenToTile(mx, my, layout.Canvas)
	if !ok {
		return 0, 0, false
	}
	w, h := g.mapBounds()
	return min(max(x, 0), w-1), min(max(y, 0), h-1), true
}

func (g *Game) handleMapAreaInteraction(layout mapLayout) {
	if !g.mapTool.isAreaTool() {
		return
	}
	if !g.mapLayerVisible[g.mapActiveLayer] {
		return
	}
	mx, my := ebiten.CursorPosition()
	justPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	justReleased := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)
	leftDown := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)

	if justPressed {
		x, y, ok := g.mapTargetTile(layout, mx, my)
		if !ok {
			return
		}
		if g.mapArea != nil && g.mapArea.layer == g.mapActiveLayer && g.mapArea.contains(x, y) {
			g.mapAreaPlacing = true
			g.mapAreaSelecting = false
			g.mapAreaGrabX, g.mapAreaGrabY = x-g.mapArea.x, y-g.mapArea.y
			g.mapAreaTargetX, g.mapAreaTargetY = g.mapArea.x, g.mapArea.y
			g.statusMsg = fmt.Sprintf("Drag the selected area; release to %s, Esc to cancel", g.mapTool.areaVerb())
		} else {
			g.mapAreaPlacing = false
			g.mapAreaSelecting = true
			g.mapAreaAnchorX, g.mapAreaAnchorY = x, y
			g.mapArea = mapAreaFromDrag(g.mapActiveLayer, x, y, x, y, g.model.Map.Width, g.model.Map.Height)
			g.mapAreaTargetLayer = g.mapActiveLayer
			g.mapAreaTargetX, g.mapAreaTargetY = x, y
			g.statusMsg = fmt.Sprintf("Selecting BG%d area", g.mapActiveLayer)
		}
	}

	if (leftDown || justReleased) && (g.mapAreaSelecting || g.mapAreaPlacing) {
		if x, y, ok := g.mapDragTargetTile(layout, mx, my); ok {
			if g.mapAreaSelecting {
				g.mapArea = mapAreaFromDrag(g.mapActiveLayer, g.mapAreaAnchorX, g.mapAreaAnchorY, x, y, g.model.Map.Width, g.model.Map.Height)
				if g.mapArea != nil {
					g.mapAreaTargetLayer = g.mapArea.layer
					g.mapAreaTargetX, g.mapAreaTargetY = g.mapArea.x, g.mapArea.y
				}
			} else if g.mapArea != nil {
				g.mapAreaTargetX = x - g.mapAreaGrabX
				g.mapAreaTargetY = y - g.mapAreaGrabY
				g.statusMsg = g.mapAreaPlacementStatus()
			}
		}
	}

	if !justReleased {
		return
	}
	if g.mapAreaSelecting {
		g.mapAreaSelecting = false
		g.statusMsg = g.mapAreaSelectionStatus()
		return
	}
	if g.mapAreaPlacing {
		g.finishMapAreaPlacement(ebiten.IsKeyPressed(ebiten.KeyShift))
	}
}

func (g *Game) mapAreaDestinationError() error {
	if g == nil || g.mapArea == nil {
		return fmt.Errorf("no map area is selected")
	}
	if g.mapAreaTargetLayer < 0 || g.mapAreaTargetLayer >= len(g.mapLayerVisible) {
		return fmt.Errorf("target BG%d is outside BG0..BG3", g.mapAreaTargetLayer)
	}
	if !g.mapLayerVisible[g.mapAreaTargetLayer] {
		return fmt.Errorf("target BG%d is hidden; show it before %s", g.mapAreaTargetLayer, g.mapTool.areaVerb())
	}
	w, h := g.mapBounds()
	if g.mapAreaTargetX < 0 || g.mapAreaTargetY < 0 ||
		g.mapAreaTargetX+g.mapArea.width > w || g.mapAreaTargetY+g.mapArea.height > h {
		return fmt.Errorf("the %dx%d area would leave the %dx%d map", g.mapArea.width, g.mapArea.height, w, h)
	}
	return nil
}

func (g *Game) mapAreaCollisionCount() int {
	if g == nil {
		return 0
	}
	selection := g.mapArea
	if selection == nil || g.mapAreaTargetLayer == selection.layer {
		return 0
	}
	count := 0
	for y := 0; y < selection.height; y++ {
		for x := 0; x < selection.width; x++ {
			if g.mapCellAtLayer(selection.layer, selection.x+x, selection.y+y) != 0 &&
				g.mapCellAtLayer(g.mapAreaTargetLayer, g.mapAreaTargetX+x, g.mapAreaTargetY+y) != 0 {
				count++
			}
		}
	}
	return count
}

func (g *Game) mapAreaPreviewActive() bool {
	if g == nil {
		return false
	}
	selection := g.mapArea
	return selection != nil &&
		g.mapAreaTargetLayer >= 0 && g.mapAreaTargetLayer < len(g.mapLayerVisible) &&
		g.mapLayerVisible[g.mapAreaTargetLayer] &&
		(g.mapAreaPlacing || g.mapAreaTargetLayer != selection.layer)
}

func (g *Game) mapAreaPlacementStatus() string {
	if err := g.mapAreaDestinationError(); err != nil {
		return fmt.Sprintf("Cannot %s here: %s", g.mapTool.areaVerb(), err)
	}
	if collisions := g.mapAreaCollisionCount(); collisions > 0 {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			return fmt.Sprintf("Release to overwrite %d occupied tile(s) on BG%d", collisions, g.mapAreaTargetLayer)
		}
		return fmt.Sprintf("BG%d has %d occupied target tile(s): hold Shift while releasing to overwrite", g.mapAreaTargetLayer, collisions)
	}
	return fmt.Sprintf("Release to %s BG%d area onto BG%d at %d,%d; Esc cancels", g.mapTool.areaVerb(), g.mapArea.layer, g.mapAreaTargetLayer, g.mapAreaTargetX, g.mapAreaTargetY)
}

func (g *Game) finishMapAreaPlacement(overwrite bool) {
	if g == nil || g.mapArea == nil {
		return
	}
	g.mapAreaPlacing = false
	if err := g.mapAreaDestinationError(); err != nil {
		g.statusMsg = fmt.Sprintf("Cannot %s here: %s", g.mapTool.areaVerb(), err)
		g.requestRender()
		return
	}
	selection := g.mapArea
	sourceLayer := selection.layer
	destinationLayer := g.mapAreaTargetLayer
	var changed bool
	var err error
	switch {
	case g.mapTool == mapToolCopyArea && sourceLayer != destinationLayer:
		changed, err = g.model.CopyMapAreaToLayer(sourceLayer, destinationLayer, selection.x, selection.y, selection.width, selection.height, g.mapAreaTargetX, g.mapAreaTargetY, overwrite)
	case g.mapTool == mapToolMoveArea && sourceLayer != destinationLayer:
		changed, err = g.model.MoveMapAreaToLayer(sourceLayer, destinationLayer, selection.x, selection.y, selection.width, selection.height, g.mapAreaTargetX, g.mapAreaTargetY, overwrite)
	case g.mapTool == mapToolCopyArea:
		changed, err = g.model.CopyMapArea(selection.layer, selection.x, selection.y, selection.width, selection.height, g.mapAreaTargetX, g.mapAreaTargetY)
	default:
		changed, err = g.model.MoveMapArea(selection.layer, selection.x, selection.y, selection.width, selection.height, g.mapAreaTargetX, g.mapAreaTargetY)
	}
	if err != nil {
		if g.mapAreaCollisionCount() > 0 && !overwrite {
			g.statusMsg = fmt.Sprintf("BG%d has occupied target tiles: hold Shift with Enter or release to overwrite", destinationLayer)
		} else {
			g.statusMsg = g.mapTool.areaTitle() + " failed: " + err.Error()
		}
		g.requestRender()
		return
	}
	selection.layer = destinationLayer
	selection.x, selection.y = g.mapAreaTargetX, g.mapAreaTargetY
	g.mapActiveLayer = destinationLayer
	g.mapAreaTargetLayer = destinationLayer
	if changed {
		g.undo.forcePush(g.snapshot())
		verb := "Moved"
		if g.mapTool == mapToolCopyArea {
			verb = "Copied"
		}
		g.statusMsg = fmt.Sprintf("%s BG%d area %dx%d to BG%d at %d,%d", verb, sourceLayer, selection.width, selection.height, destinationLayer, selection.x, selection.y)
	} else if sourceLayer != destinationLayer {
		g.statusMsg = fmt.Sprintf("No map cells changed between BG%d and BG%d", sourceLayer, destinationLayer)
	} else {
		g.statusMsg = fmt.Sprintf("BG%d area remains at %d,%d", selection.layer, selection.x, selection.y)
	}
	g.requestRender()
}

func (g *Game) mapCellForDisplay(layer, x, y int) int {
	id := g.mapCellAtLayer(layer, x, y)
	selection := g.mapArea
	if selection == nil || !g.mapAreaPreviewActive() {
		return id
	}
	if g.mapTool == mapToolMoveArea && layer == selection.layer && selection.contains(x, y) {
		id = 0
	}
	if layer == g.mapAreaTargetLayer {
		dx, dy := x-g.mapAreaTargetX, y-g.mapAreaTargetY
		if dx >= 0 && dy >= 0 && dx < selection.width && dy < selection.height {
			sourceID := g.mapCellAtLayer(selection.layer, selection.x+dx, selection.y+dy)
			if selection.layer == g.mapAreaTargetLayer || sourceID != 0 {
				id = sourceID
			}
		}
	}
	return id
}

func mapAreaScreenRect(camera editor.Camera, canvas editor.Rect, x, y, width, height int) editor.Rect {
	x0, y0 := camera.WorldToScreen(x, y, canvas)
	x1, y1 := camera.WorldToScreen(x+width, y+height, canvas)
	return spriteThumbRect(x0, y0, x1-x0, y1-y0)
}

func drawMapAreaOutline(screen *ebiten.Image, area, clip editor.Rect, clr color.Color) {
	if screen == nil || area.W <= 0 || area.H <= 0 {
		return
	}
	left, top := area.X, area.Y
	right, bottom := area.X+area.W-1, area.Y+area.H-1
	clipRight, clipBottom := clip.X+clip.W-1, clip.Y+clip.H-1
	lineTop, lineBottom := max(top, clip.Y), min(bottom, clipBottom)
	lineLeft, lineRight := max(left, clip.X), min(right, clipRight)
	if left >= clip.X && left <= clipRight && lineTop <= lineBottom {
		drawLine(screen, left, lineTop, left, lineBottom, clr)
	}
	if right >= clip.X && right <= clipRight && lineTop <= lineBottom {
		drawLine(screen, right, lineTop, right, lineBottom, clr)
	}
	if top >= clip.Y && top <= clipBottom && lineLeft <= lineRight {
		drawLine(screen, lineLeft, top, lineRight, top, clr)
	}
	if bottom >= clip.Y && bottom <= clipBottom && lineLeft <= lineRight {
		drawLine(screen, lineLeft, bottom, lineRight, bottom, clr)
	}
}

func (g *Game) drawMapAreaSelection(screen *ebiten.Image, layout mapLayout) {
	selection := g.mapArea
	if screen == nil || selection == nil {
		return
	}
	source := mapAreaScreenRect(g.mapCam, layout.Canvas, selection.x, selection.y, selection.width, selection.height)
	if visible, ok := intersectEditorRects(source, layout.Canvas); ok {
		fillRect(screen, visible, rgba(colorMap, 36))
	}
	sourceColor := color.Color(colorMap)
	if g.mapAreaPreviewActive() && g.mapTool == mapToolMoveArea {
		sourceColor = rgba(colorMap, 130)
	}
	drawMapAreaOutline(screen, source, layout.Canvas, sourceColor)

	if !g.mapAreaPreviewActive() {
		return
	}
	target := mapAreaScreenRect(g.mapCam, layout.Canvas, g.mapAreaTargetX, g.mapAreaTargetY, selection.width, selection.height)
	accent := color.Color(colorMap)
	if g.mapAreaTargetLayer != selection.layer {
		accent = colorHighlight
	}
	if g.mapAreaDestinationError() != nil || (g.mapAreaCollisionCount() > 0 && !ebiten.IsKeyPressed(ebiten.KeyShift)) {
		accent = colorDanger
	}
	if visible, ok := intersectEditorRects(target, layout.Canvas); ok {
		fillRect(screen, visible, rgba(accent, 52))
	}
	drawMapAreaOutline(screen, target, layout.Canvas, rgba(accent, 245))
}
