package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

func mapCanvasRect() editor.Rect {
	return newMapLayout(screenWidth, screenHeight).Canvas
}

func (g *Game) mapCellAt(x, y int) int {
	return g.mapCellAtLayer(g.mapActiveLayer, x, y)
}

func (g *Game) mapCellAtLayer(layer, x, y int) int {
	return g.model.MapCell(layer, x, y)
}

func (g *Game) setMapCell(x, y, id int) bool {
	w, h := g.mapBounds()
	if x < 0 || y < 0 || x >= w || y >= h || id < 0 || id >= g.spriteN() {
		return false
	}
	changed, err := g.model.SetMapCell(g.mapActiveLayer, x, y, id)
	if err != nil {
		g.statusMsg = err.Error()
		return false
	}
	return changed
}

func (g *Game) updateMapMode() {
	layout := newMapLayout(screenWidth, screenHeight)
	if g.mapCam.Zoom == 0 {
		g.mapCam.Zoom = editor.MaxZoom
	}
	mapW, mapH := g.mapBounds()
	g.mapCam.Clamp(mapW, mapH, layout.Canvas)

	if g.updateMapRoomToolbar(layout) {
		return
	}
	if g.mapRooms {
		g.handleMapLayerPanel(layout)
		g.handleMapPan(layout)
		g.handleMapMouseDrag(layout)
		g.handleMapZoom(layout)
		g.updateMapRooms(layout)
		return
	}
	g.handleMapResizeKeys()
	g.handleMapLayerPanel(layout)
	g.handleMapAreaTool(layout)
	g.handleMapPan(layout)
	g.handleMapMouseDrag(layout)
	g.handleMapZoom(layout)
	mapW, mapH = g.mapBounds()
	g.mapCam.Clamp(mapW, mapH, layout.Canvas)
	if g.mapTool.isAreaTool() {
		g.handleMapAreaInteraction(layout)
		return
	}
	g.handleMapPlaceErase(layout)
}

func (g *Game) selectMapLayer(layer int) bool {
	if layer < 0 || layer >= len(g.mapLayerVisible) {
		return false
	}
	changed := layer != g.mapActiveLayer
	g.mapActiveLayer = layer
	if changed {
		g.resetMapAreaInteraction()
	}
	if g.mapTool.isAreaTool() {
		g.statusMsg = g.mapAreaToolPrompt()
	} else {
		g.statusMsg = fmt.Sprintf("BG%d selected", layer)
	}
	return true
}

func mapLayerPanelHit(layout mapLayout, x, y int) (layer int, visibility bool, ok bool) {
	for bg := 0; bg < 4; bg++ {
		if rectContains(layout.LayerEyes[bg], x, y) {
			return bg, true, true
		}
		if rectContains(layout.LayerRows[bg], x, y) {
			return bg, false, true
		}
	}
	return 0, false, false
}

func (g *Game) handleMapLayerPanel(layout mapLayout) {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	mx, my := ebiten.CursorPosition()
	layer, visibility, ok := mapLayerPanelHit(layout, mx, my)
	if !ok {
		return
	}
	if visibility {
		g.mapLayerVisible[layer] = !g.mapLayerVisible[layer]
		if layer == g.mapActiveLayer && !g.mapLayerVisible[layer] {
			g.resetMapAreaInteraction()
		}
		state := "hidden"
		if g.mapLayerVisible[layer] {
			state = "visible"
		}
		g.statusMsg = fmt.Sprintf("BG%d %s", layer, state)
		return
	}
	if g.mapTool.isAreaTool() && g.mapArea != nil {
		g.setMapAreaTargetLayer(layer)
		return
	}
	g.selectMapLayer(layer)
}

func (g *Game) handleMapPan(layout mapLayout) {
	step := 1.0 / max(g.mapCam.Zoom, editor.MinZoom)
	var dx, dy float64
	if ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl) {
		return
	}
	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		dx -= step
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		dx += step
	}
	if ebiten.IsKeyPressed(ebiten.KeyUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		dy -= step
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		dy += step
	}
	if dx != 0 || dy != 0 {
		w, h := g.mapBounds()
		g.mapCam.Pan(dx, dy, layout.Canvas, w, h)
	}
}

func (g *Game) handleMapMouseDrag(layout mapLayout) {
	mx, my := ebiten.CursorPosition()
	middleDown := ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle)
	wasDown := g.middleWasDown
	if middleDown && !g.middleWasDown {
		g.mapLastMouseX, g.mapLastMouseY = mx, my
	}
	if middleDown {
		tile := g.mapCam.TileSize()
		if tile > 0 {
			w, h := g.mapBounds()
			dx := -float64(mx-g.mapLastMouseX) / tile
			dy := -float64(my-g.mapLastMouseY) / tile
			g.mapCam.Pan(dx, dy, layout.Canvas, w, h)
			g.theme.mapVX, g.theme.mapVY = dx, dy
		}
		g.mapLastMouseX, g.mapLastMouseY = mx, my
	} else if wasDown && g.theme.level == effectsClean {
		g.theme.mapVX, g.theme.mapVY = 0, 0
	} else if g.theme.level != effectsClean && (math.Abs(g.theme.mapVX) > .002 || math.Abs(g.theme.mapVY) > .002) {
		w, h := g.mapBounds()
		g.mapCam.Pan(g.theme.mapVX, g.theme.mapVY, layout.Canvas, w, h)
		g.theme.mapVX *= .82
		g.theme.mapVY *= .82
	}
	g.middleWasDown = middleDown
}

func (g *Game) handleMapResizeKeys() {
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	delta := 1
	if mod {
		delta = -1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft) {
		g.nudgeMapSize(0, delta)
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketRight) {
		g.nudgeMapSize(delta, 0)
	}
}

func (g *Game) nudgeMapSize(dScreensX, dScreensY int) {
	w, h := g.mapBounds()
	newW := max(1, w+dScreensX*gbaScreenWidthTiles)
	newH := max(1, h+dScreensY*gbaScreenHeightTiles)
	if newW == w && newH == h {
		return
	}
	if err := g.model.ResizeMap(newW, newH); err != nil {
		g.statusMsg = err.Error()
		return
	}
	g.resetMapAreaInteraction()
	// Resizing changes the level bounds, not the artist's chosen view. Keep
	// the current readable zoom and only clamp the camera to the new bounds.
	g.mapCam.Clamp(newW, newH, mapCanvasRect())
	g.undo.forcePush(g.snapshot())
}

func (g *Game) handleMapZoom(layout mapLayout) {
	mx, my := ebiten.CursorPosition()
	if !rectContains(layout.Canvas, mx, my) {
		return
	}
	_, yoff := ebiten.Wheel()
	w, h := g.mapBounds()
	switch {
	case yoff > 0:
		g.mapCam.ZoomAt(editor.ZoomStep, mx, my, layout.Canvas, w, h)
	case yoff < 0:
		g.mapCam.ZoomAt(1/editor.ZoomStep, mx, my, layout.Canvas, w, h)
	}
}

func (g *Game) mapTargetTile(layout mapLayout, mx, my int) (int, int, bool) {
	tileX, tileY, ok := g.mapCam.ScreenToTile(mx, my, layout.Canvas)
	if !ok {
		return 0, 0, false
	}
	w, h := g.mapBounds()
	if tileX < 0 || tileX >= w || tileY < 0 || tileY >= h {
		return 0, 0, false
	}
	return tileX, tileY, true
}

func (g *Game) handleMapPlaceErase(layout mapLayout) {
	mx, my := ebiten.CursorPosition()
	tileX, tileY, ok := g.mapTargetTile(layout, mx, my)
	if !ok {
		return
	}
	switch {
	case ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight):
		if g.setMapCell(tileX, tileY, 0) {
			g.dirty = true
			g.flashMap(mx, my)
		}
	case ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft):
		if g.placeGridSprites(tileX, tileY) {
			g.flashMap(mx, my)
		}
	}
}

// placeGridSprites stamps the current multi-sprite selection onto the map
// starting at (x,y), one map cell per constituent sprite id. A 2x1
// selection places two sprite ids side by side; a 2x2 places four.
func (g *Game) placeGridSprites(x, y int) bool {
	baseRow, baseCol := g.baseSpriteRowCol()
	changed := false
	for dy := 0; dy < g.spriteGridRows(); dy++ {
		for dx := 0; dx < g.spriteGridCols(); dx++ {
			row, col := baseRow+dy, baseCol+dx
			if row >= g.spriteRows() || col >= g.spriteCols() {
				continue
			}
			id := row*g.spriteCols() + col
			if g.setMapCell(x+dx, y+dy, id) {
				changed = true
			}
		}
	}
	if changed {
		g.dirty = true
	}
	return changed
}

func (g *Game) drawMapMode(screen *ebiten.Image) {
	layout := newMapLayout(screenWidth, screenHeight)
	drawWorkspaceHeader(screen, layout.Header, "MAP LAB", "TILEMAPS + GAME BOY WORLDS", colorMap)
	drawHardwarePanel(screen, editor.Rect{X: layout.Canvas.X - 4, Y: layout.Canvas.Y - 4, W: layout.Canvas.W + 8, H: layout.Canvas.H + 8}, colorPanel, colorMap)
	drawHardwarePanel(screen, editor.Rect{X: layout.Strip.X - 4, Y: layout.Strip.Y - 4, W: layout.Strip.W + 8, H: layout.Strip.H + 8}, colorPanel, colorMap)
	fillRect(screen, layout.Canvas, colorBackground)

	g.drawMapTiles(screen, layout)
	g.drawMapBounds(screen, layout)
	mx, my := ebiten.CursorPosition()
	tileX, tileY, ok := g.mapTargetTile(layout, mx, my)
	if g.mapTool.isAreaTool() {
		g.drawMapAreaSelection(screen, layout)
	} else if !g.mapRooms {
		g.drawMapHover(screen, layout, tileX, tileY, ok)
	}
	g.drawSceneBoundaries(screen, layout)
	g.drawMapBottomStrip(screen, layout, tileX, tileY, ok)
	g.drawMapLayersPanel(screen, layout)
	g.drawMapRoomToolbar(screen, layout)
	if g.mapRooms {
		g.drawMapRooms(screen, layout)
	}
}

func (g *Game) drawMapTiles(screen *ebiten.Image, layout mapLayout) {
	w, h := g.mapBounds()
	minX, minY, maxX, maxY := g.mapCam.VisibleBounds(layout.Canvas, w, h)
	tile := g.mapCam.TileSize()
	// Match the runtime's source-pixel transparency: color 0 leaves whatever
	// is behind the map untouched. In the editor that underlying surface is a
	// checkerboard, painted once for the visible map instead of once per tile.
	if left, top, right, bottom, ok := g.mapGuideClip(layout); ok {
		drawCheckerboard(screen, editor.Rect{X: left, Y: top, W: right - left + 1, H: bottom - top + 1})
	}
	for layer := 0; layer < 4; layer++ {
		if !g.mapLayerVisible[layer] {
			continue
		}
		for y := minY; y < maxY; y++ {
			for x := minX; x < maxX; x++ {
				sx, sy := g.mapCam.WorldToScreen(x, y, layout.Canvas)
				id := g.mapCellForDisplay(layer, x, y)
				if isEraserIndex(id) {
					continue
				}
				g.drawSpriteThumb(screen, id, sx, sy, tile, tile)
				if g.tileLinked(id) {
					drawTileLinkMarker(screen, editor.Rect{X: int(sx), Y: int(sy), W: max(1, int(tile)), H: max(1, int(tile))})
				}
			}
		}
	}
}

func (g *Game) drawMapLayersPanel(screen *ebiten.Image, layout mapLayout) {
	drawText(screen, "LAYERS", layout.Layers.X+4, layout.Layers.Y+1, colorText)
	layerStatus := fmt.Sprintf("ACTIVE BG%d  /  1-4", g.mapActiveLayer)
	if g.mapTool.isAreaTool() && g.mapArea != nil {
		layerStatus = fmt.Sprintf("SOURCE BG%d  >  TARGET BG%d", g.mapArea.layer, g.mapAreaTargetLayer)
	}
	drawSmallText(screen, layerStatus, layout.Layers.X+4, layout.Layers.Y+layout.Layers.H-uiSmallTextHeight-1, colorMap)
	for bg := 0; bg < 4; bg++ {
		row := layout.LayerRows[bg]
		if bg == g.mapActiveLayer {
			fillRect(screen, row, color.RGBA{R: 32, G: 62, B: 48, A: 255})
			fillRect(screen, editor.Rect{X: row.X, Y: row.Y + row.H - 3, W: row.W, H: 3}, colorMap)
		}
		if g.mapTool.isAreaTool() && g.mapArea != nil && bg == g.mapAreaTargetLayer && bg != g.mapArea.layer {
			fillRect(screen, row, rgba(colorHighlight, 28))
			fillRect(screen, editor.Rect{X: row.X, Y: row.Y, W: row.W, H: 3}, colorHighlight)
		}
		strokeRect(screen, row, rgba(colorBorder, 150))
		g.drawMapLayerVisibility(screen, layout.LayerEyes[bg], g.mapLayerVisible[bg])
		labelColor := colorText
		if !g.mapLayerVisible[bg] {
			labelColor = colorDim
		}
		role := "MID"
		switch bg {
		case 0:
			role = "BACK"
		case 3:
			role = "FRONT"
		}
		textX := row.X + 36
		drawText(screen, fmt.Sprintf("BG%d", bg), textX, row.Y+1, labelColor)
		drawSmallText(screen, role, textX, row.Y+row.H-uiSmallTextHeight-2, colorDim)
	}
}

func (g *Game) drawMapLayerVisibility(screen *ebiten.Image, rect editor.Rect, visible bool) {
	color := colorDim
	if visible {
		color = colorText
	}
	cx, cy := rect.X+rect.W/2, rect.Y+rect.H/2
	drawLine(screen, rect.X+4, cy, cx, rect.Y+5, color)
	drawLine(screen, cx, rect.Y+5, rect.X+rect.W-4, cy, color)
	drawLine(screen, rect.X+rect.W-4, cy, cx, rect.Y+rect.H-5, color)
	drawLine(screen, cx, rect.Y+rect.H-5, rect.X+4, cy, color)
	if visible {
		fillRect(screen, editor.Rect{X: cx - 2, Y: cy - 2, W: 5, H: 5}, colorMap)
	} else {
		drawLine(screen, rect.X+5, rect.Y+4, rect.X+rect.W-5, rect.Y+rect.H-4, colorDanger)
	}
}

func (g *Game) drawMapBounds(screen *ebiten.Image, layout mapLayout) {
	w, h := g.mapBounds()
	if w <= 0 || h <= 0 {
		return
	}
	x0, y0 := g.mapCam.WorldToScreen(0, 0, layout.Canvas)
	x1, y1 := g.mapCam.WorldToScreen(w, h, layout.Canvas)
	strokeRect(screen, editor.Rect{
		X: int(x0),
		Y: int(y0),
		W: int(x1 - x0),
		H: int(y1 - y0),
	}, colorBorder)
}

func (g *Game) drawMapHover(screen *ebiten.Image, layout mapLayout, tileX, tileY int, ok bool) {
	if !ok {
		return
	}
	w, h := g.mapBounds()
	tile := g.mapCam.TileSize()
	for dy := 0; dy < g.spriteGridRows(); dy++ {
		for dx := 0; dx < g.spriteGridCols(); dx++ {
			tx, ty := tileX+dx, tileY+dy
			if tx < 0 || tx >= w || ty < 0 || ty >= h {
				continue
			}
			sx, sy := g.mapCam.WorldToScreen(tx, ty, layout.Canvas)
			strokeRect(screen, editor.Rect{X: int(sx), Y: int(sy), W: int(tile), H: int(tile)}, colorHighlight)
		}
	}
	if g.theme.mapPulse > 0 {
		expand := (14 - g.theme.mapPulse) / 2
		alpha := uint8(min(180, g.theme.mapPulse*12))
		strokeRect(screen, editor.Rect{X: g.theme.mapX - int(tile)/2 - expand, Y: g.theme.mapY - int(tile)/2 - expand, W: int(tile) + expand*2, H: int(tile) + expand*2}, rgba(colorMap, alpha))
	}
}

// drawSceneBoundaries overlays a reference grid at the GBA's real 240x160
// (30x20-tile) screen size inside the map. Lines are clipped to the map
// rectangle so zooming out does not draw a crosshair across empty canvas.
func (g *Game) drawSceneBoundaries(screen *ebiten.Image, layout mapLayout) {
	w, h := g.mapBounds()
	left, top, right, bottom, ok := g.mapGuideClip(layout)
	if !ok {
		return
	}
	vertical, horizontal := g.mapCam.VisibleBoundaryLines(layout.Canvas, w, h, gbaScreenWidthTiles, gbaScreenHeightTiles)
	for _, line := range vertical {
		if line.World == 0 || line.World == w || line.Screen < left || line.Screen > right {
			continue
		}
		drawLine(screen, line.Screen, top, line.Screen, bottom, colorBorder)
	}
	for _, line := range horizontal {
		if line.World == 0 || line.World == h || line.Screen < top || line.Screen > bottom {
			continue
		}
		drawLine(screen, left, line.Screen, right, line.Screen, colorBorder)
	}
}

func (g *Game) mapGuideClip(layout mapLayout) (left, top, right, bottom int, ok bool) {
	w, h := g.mapBounds()
	if w <= 0 || h <= 0 {
		return 0, 0, 0, 0, false
	}
	x0, y0 := g.mapCam.WorldToScreen(0, 0, layout.Canvas)
	x1, y1 := g.mapCam.WorldToScreen(w, h, layout.Canvas)
	left = max(layout.Canvas.X, int(x0))
	top = max(layout.Canvas.Y, int(y0))
	right = min(layout.Canvas.X+layout.Canvas.W-1, int(x1)-1)
	bottom = min(layout.Canvas.Y+layout.Canvas.H-1, int(y1)-1)
	if left > right || top > bottom {
		return 0, 0, 0, 0, false
	}
	return left, top, right, bottom, true
}

func (g *Game) drawMapBottomStrip(screen *ebiten.Image, layout mapLayout, tileX, tileY int, ok bool) {
	fillRect(screen, layout.Strip, colorPanel)
	strokeRect(screen, layout.Strip, colorBorder)

	fillRect(screen, layout.SelectedTile, colorBackground)
	if isEraserIndex(g.currentSprite) {
		drawEraserSwatch(screen, layout.SelectedTile)
	} else {
		g.drawSpriteThumb(screen, g.currentSprite, float64(layout.SelectedTile.X), float64(layout.SelectedTile.Y), float64(layout.SelectedTile.W), float64(layout.SelectedTile.H))
	}
	strokeRect(screen, layout.SelectedTile, colorBorder)
	drawTactileButton(screen, layout.MoveArea, "MOVE AREA [M]", g.mapTool == mapToolMoveArea, colorMap, true)
	drawTactileButton(screen, layout.CopyArea, "COPY AREA [C]", g.mapTool == mapToolCopyArea, colorMap, true)

	hoverText := "tile: -,- sprite: -"
	if ok {
		id := g.mapCellAt(tileX, tileY)
		hoverText = fmt.Sprintf("BG%d tile: %d,%d sprite: %d", g.mapActiveLayer, tileX, tileY, id)
		if link, linked := g.model.TileLinkFor(id); linked && id > 0 {
			hoverText += " (draws " + tileLinkLabel(link) + ")"
		}
	}
	mw, mh := g.mapBounds()
	hoverText += fmt.Sprintf("  map: %dx%d (%dx%d screens)", mw, mh, mapScreenCount(mw, gbaScreenWidthTiles), mapScreenCount(mh, gbaScreenHeightTiles))
	selected := fmt.Sprintf("selected: %d", g.currentSprite)
	if isEraserIndex(g.currentSprite) {
		selected += " eraser"
	}
	drawText(screen, truncateUIText(hoverText, layout.Coordinates.W), layout.Coordinates.X, layout.Coordinates.Y-6, colorText)
	help := fmt.Sprintf("%s  (left paint, right erase, Cmd/Ctrl+drag select, WASD pan, wheel zoom)", selected)
	if g.mapRooms {
		help = "Click a room to select; + adds at an edge; Cmd/Ctrl+Z undoes"
	}
	if g.mapTool.isAreaTool() {
		help = fmt.Sprintf("%s: drag select; Cmd/Ctrl+A whole layer; choose target BG0-BG3 (keys 1-4)", g.mapTool.areaTitle())
		if g.mapArea != nil {
			help = fmt.Sprintf("BG%d > BG%d: drag to reposition or Enter; Shift confirms overwrite", g.mapArea.layer, g.mapAreaTargetLayer)
		}
	}
	drawText(screen, truncateUIText(help, layout.Coordinates.W), layout.Coordinates.X, layout.Coordinates.Y+8, colorDim)
}

func mapScreenCount(tiles, screen int) int {
	if tiles <= 0 || screen <= 0 {
		return 0
	}
	return (tiles + screen - 1) / screen
}
