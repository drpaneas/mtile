package main

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/editor"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

func mapRoomsToolButton(layout mapLayout) editor.Rect {
	return editor.Rect{X: layout.Header.W - 308, Y: layout.Header.Y + 20, W: 132, H: 32}
}
func mapDeleteRoomButton(layout mapLayout) editor.Rect {
	return editor.Rect{X: layout.Header.W - 168, Y: layout.Header.Y + 20, W: 152, H: 32}
}
func (g *Game) mapRoomDeleteLabel() string {
	w, h := g.mapBounds()
	if w > editor.MapRoomWidth && h > editor.MapRoomHeight || w <= editor.MapRoomWidth && h <= editor.MapRoomHeight {
		return "CLEAR ROOM"
	}
	return "DELETE ROOM"
}
func (g *Game) updateMapRoomToolbar(layout mapLayout) bool {
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	x, y := ebiten.CursorPosition()
	left := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	if g.mapRooms && ((!mod && (inpututil.IsKeyJustPressed(ebiten.KeyM) || inpututil.IsKeyJustPressed(ebiten.KeyC))) || left && (rectContains(layout.MoveArea, x, y) || rectContains(layout.CopyArea, x, y))) {
		g.mapRooms = false
	}
	if !left {
		return false
	}
	if rectContains(mapRoomsToolButton(layout), x, y) {
		g.mapRooms = !g.mapRooms
		g.resetMapAreaInteraction()
		g.mapTool = mapToolPaint
		if g.mapRooms {
			w, h := g.mapBounds()
			g.mapCam.Fit(w, h, layout.Canvas)
			g.statusMsg = "Rooms: click a room, use + to add, or delete the selection"
		}
		g.requestRender()
		return true
	}
	if g.mapRooms && g.mapRoomSelected && rectContains(mapDeleteRoomButton(layout), x, y) {
		g.deleteSelectedMapRoom()
		return true
	}
	return false
}
func (g *Game) commitMapRoomEdit(err error) {
	if err != nil {
		g.statusMsg = err.Error()
		g.requestRender()
		return
	}
	g.dirty = true
	g.resetMapAreaInteraction()
	w, h := g.mapBounds()
	g.mapRoomCol = min(g.mapRoomCol, mapScreenCount(w, editor.MapRoomWidth)-1)
	g.mapRoomRow = min(g.mapRoomRow, mapScreenCount(h, editor.MapRoomHeight)-1)
	g.mapCam.Fit(w, h, mapCanvasRect())
	g.undo.forcePush(g.snapshot())
	g.requestRender()
}
func (g *Game) deleteSelectedMapRoom() {
	label := g.mapRoomDeleteLabel()
	err := g.model.DeleteMapRoom(g.mapRoomCol, g.mapRoomRow)
	g.commitMapRoomEdit(err)
	if err == nil {
		g.statusMsg = label + ": all backgrounds updated · Cmd/Ctrl+Z to undo"
	}
}
func (g *Game) mapRoomEdgeButtons(layout mapLayout) [4]editor.Rect {
	w, h := g.mapBounds()
	x0, y0 := g.mapCam.WorldToScreen(0, 0, layout.Canvas)
	x1, y1 := g.mapCam.WorldToScreen(w, h, layout.Canvas)
	c := layout.Canvas
	button := func(x, y int) editor.Rect {
		return editor.Rect{X: min(max(x, c.X+8), c.X+c.W-100), Y: min(max(y, c.Y+8), c.Y+c.H-38), W: 92, H: 30}
	}
	return [4]editor.Rect{button(int(x0)-100, int((y0+y1)/2)-15), button(int(x1)+8, int((y0+y1)/2)-15), button(int((x0+x1)/2)-46, int(y0)-38), button(int((x0+x1)/2)-46, int(y1)+8)}
}
func (g *Game) updateMapRooms(layout mapLayout) {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	x, y := ebiten.CursorPosition()
	for i, r := range g.mapRoomEdgeButtons(layout) {
		if rectContains(r, x, y) {
			directions := [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
			d := directions[i]
			err := g.model.AddMapRoomEdge(d[0], d[1])
			if err == nil {
				w, h := g.mapBounds()
				g.mapRoomSelected = true
				if d[0] < 0 {
					g.mapRoomCol = 0
				}
				if d[0] > 0 {
					g.mapRoomCol = mapScreenCount(w, editor.MapRoomWidth) - 1
				}
				if d[1] < 0 {
					g.mapRoomRow = 0
				}
				if d[1] > 0 {
					g.mapRoomRow = mapScreenCount(h, editor.MapRoomHeight) - 1
				}
			}
			g.commitMapRoomEdit(err)
			if err == nil {
				g.statusMsg = "Added room space · Cmd/Ctrl+Z to undo"
			}
			return
		}
	}
	tx, ty, ok := g.mapTargetTile(layout, x, y)
	if !ok {
		return
	}
	g.mapRoomCol, g.mapRoomRow = tx/editor.MapRoomWidth, ty/editor.MapRoomHeight
	g.mapRoomSelected = true
	g.statusMsg = fmt.Sprintf("Room %d,%d selected · %s · Cmd/Ctrl+Z undoes changes", g.mapRoomCol+1, g.mapRoomRow+1, g.mapRoomDeleteLabel())
	g.requestRender()
}
func (g *Game) drawMapRoomToolbar(screen *ebiten.Image, layout mapLayout) {
	drawTactileButton(screen, mapRoomsToolButton(layout), "ROOMS", g.mapRooms, colorMap, true)
	if g.mapRooms {
		drawTactileButton(screen, mapDeleteRoomButton(layout), g.mapRoomDeleteLabel(), false, colorMap, g.mapRoomSelected)
	}
}
func (g *Game) drawMapRooms(screen *ebiten.Image, layout mapLayout) {
	w, h := g.mapBounds()
	cols, rows := mapScreenCount(w, editor.MapRoomWidth), mapScreenCount(h, editor.MapRoomHeight)
	minX, minY, maxX, maxY := g.mapCam.VisibleBounds(layout.Canvas, w, h)
	for row := minY / editor.MapRoomHeight; row < min(rows, (maxY+editor.MapRoomHeight-1)/editor.MapRoomHeight); row++ {
		for col := minX / editor.MapRoomWidth; col < min(cols, (maxX+editor.MapRoomWidth-1)/editor.MapRoomWidth); col++ {
			sx, sy := g.mapCam.WorldToScreen(col*editor.MapRoomWidth, row*editor.MapRoomHeight, layout.Canvas)
			ex, ey := g.mapCam.WorldToScreen(min(w, (col+1)*editor.MapRoomWidth), min(h, (row+1)*editor.MapRoomHeight), layout.Canvas)
			r, ok := intersectEditorRects(editor.Rect{X: int(sx), Y: int(sy), W: int(ex - sx), H: int(ey - sy)}, layout.Canvas)
			if !ok {
				continue
			}
			selected := g.mapRoomSelected && g.mapRoomCol == col && g.mapRoomRow == row
			if selected {
				strokeRect(screen, r, colorMap)
			}
			label := editor.Rect{X: r.X + 6, Y: r.Y + 6, W: 112, H: 26}
			if r.W > 124 && r.H > 40 {
				drawTactileButton(screen, label, fmt.Sprintf("ROOM %d,%d", col+1, row+1), selected, colorMap, true)
			}
		}
	}
	labels := [4]string{"+ LEFT", "+ RIGHT", "+ ABOVE", "+ BELOW"}
	for i, r := range g.mapRoomEdgeButtons(layout) {
		drawTactileButton(screen, r, labels[i], false, colorMap, true)
	}
	drawSmallText(screen, "+ adds a whole row/column. In a 2D grid, Clear Room preserves its neighbors.", layout.Canvas.X+8, layout.Canvas.Y+layout.Canvas.H-18, colorText)
}
