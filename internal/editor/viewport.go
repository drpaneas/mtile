package editor

import "math"

// BaseTileSize is the pixel size of one map/sheet cell before zoom, matching
// the runtime's fixed 8x8 sprite cell. It is a package constant (not project
// data) because it is a rendering convention, not something assetsync
// validates or a project can change.
const BaseTileSize = 8.0

const (
	MinZoom  = 0.5
	MaxZoom  = 4.0
	ZoomStep = 1.25
)

// Rect is a screen-space rectangle (pixels), used to describe where a
// scrollable/zoomable panel (map canvas or spritesheet panel) is drawn.
type Rect struct {
	X, Y, W, H int
}

// Camera is the pan/zoom state for one scrollable grid of 8x8 cells (the map
// canvas, or the spritesheet panel after Fit). It has no windowing
// dependency: screen/world conversions take the panel's Rect as an explicit
// parameter rather than reading any global display state.
type Camera struct {
	X     float64 // world-space camera position, in cells
	Y     float64
	Zoom  float64
	ZoomY float64 // 0 means uniform (Y uses Zoom); non-zero is independent Y zoom
}

// NewCamera returns a camera at the origin with no zoom applied.
func NewCamera() Camera {
	return Camera{Zoom: 1}
}

func (c Camera) zoomX() float64 {
	z := c.Zoom
	if z == 0 {
		z = 1
	}
	if c.ZoomY == 0 {
		return min(max(z, MinZoom), MaxZoom)
	}
	if z < 0 {
		return 0
	}
	return z
}

func (c Camera) zoomY() float64 {
	if c.ZoomY == 0 {
		return c.zoomX()
	}
	if c.ZoomY < 0 {
		return 0
	}
	return c.ZoomY
}

// TileSize is the on-screen pixel width of one cell. For a uniform camera
// (ZoomY == 0) it is also the height. After FitStretch the height is TileSizeY.
func (c Camera) TileSize() float64 {
	return BaseTileSize * c.zoomX()
}

// TileSizeY is the on-screen pixel height of one cell.
func (c Camera) TileSizeY() float64 {
	return BaseTileSize * c.zoomY()
}

// VisibleSpan is how many cells are visible across canvas's width/height at
// the current zoom.
func (c Camera) VisibleSpan(canvas Rect) (float64, float64) {
	tileX, tileY := c.TileSize(), c.TileSizeY()
	if tileX == 0 || tileY == 0 {
		return 0, 0
	}
	return float64(canvas.W) / tileX, float64(canvas.H) / tileY
}

// VisibleBounds returns the [minX,maxX) x [minY,maxY) cell range visible in
// canvas, clamped to the gridW x gridH grid.
func (c Camera) VisibleBounds(canvas Rect, gridW, gridH int) (minX, minY, maxX, maxY int) {
	spanX, spanY := c.VisibleSpan(canvas)
	minX = max(0, int(math.Floor(c.X)))
	minY = max(0, int(math.Floor(c.Y)))
	maxX = min(gridW, int(math.Ceil(c.X+spanX)))
	maxY = min(gridH, int(math.Ceil(c.Y+spanY)))
	return minX, minY, maxX, maxY
}

// Clamp keeps the camera from panning past the edges of a gridW x gridH
// grid, given the current canvas size. When the whole grid fits in the
// canvas, the camera is centered so leftover space is split around the
// sheet instead of pooling in the bottom-right.
func (c *Camera) Clamp(gridW, gridH int, canvas Rect) {
	spanX, spanY := c.VisibleSpan(canvas)
	c.X = clampAxis(c.X, spanX, gridW)
	c.Y = clampAxis(c.Y, spanY, gridH)
}

func clampAxis(camera, span float64, grid int) float64 {
	if grid <= 0 {
		return 0
	}
	limit := float64(grid)
	if span >= limit {
		return (limit - span) / 2
	}
	return min(max(camera, 0), limit-span)
}

// Fit sets a uniform zoom so the full gridW x gridH sheet sits inside canvas
// with square cells. Leftover space is letterboxed (Clamp centers it). The
// minimum zoom is skipped so a large sheet can still fit. The maximum is
// retained so a tiny sheet does not turn one cell into a panel-sized tile.
func (c *Camera) Fit(gridW, gridH int, canvas Rect) {
	c.FitWithMaxZoom(gridW, gridH, canvas, MaxZoom)
}

// FitWithMaxZoom behaves like Fit, but lets a caller choose the magnification
// ceiling. This keeps the map camera's conservative interactive zoom limit
// separate from the spritesheet editor, where a 1x1 or 2x2 sheet still needs
// to be large enough to edit comfortably.
func (c *Camera) FitWithMaxZoom(gridW, gridH int, canvas Rect, maxZoom float64) {
	if gridW <= 0 || gridH <= 0 || canvas.W <= 0 || canvas.H <= 0 {
		return
	}
	if maxZoom <= 0 {
		return
	}
	zx := float64(canvas.W) / (float64(gridW) * BaseTileSize)
	zy := float64(canvas.H) / (float64(gridH) * BaseTileSize)
	z := min(min(zx, zy), maxZoom)
	c.Zoom = z
	c.ZoomY = z
	c.X, c.Y = 0, 0
	c.Clamp(gridW, gridH, canvas)
}

// FitStretch sets independent X/Y zoom so the full gridW x gridH sheet
// covers canvas exactly. Cells become non-square when canvas's aspect ratio
// differs from the sheet's. Zoom limits used by the map editor are skipped
// so a tall panel can still be filled.
func (c *Camera) FitStretch(gridW, gridH int, canvas Rect) {
	if gridW <= 0 || gridH <= 0 || canvas.W <= 0 || canvas.H <= 0 {
		return
	}
	c.Zoom = float64(canvas.W) / (float64(gridW) * BaseTileSize)
	c.ZoomY = float64(canvas.H) / (float64(gridH) * BaseTileSize)
	c.X, c.Y = 0, 0
	c.Clamp(gridW, gridH, canvas)
}

// Pan moves the camera by dx/dy cells and re-clamps it.
func (c *Camera) Pan(dx, dy float64, canvas Rect, gridW, gridH int) {
	c.X += dx
	c.Y += dy
	c.Clamp(gridW, gridH, canvas)
}

// ZoomAt changes the zoom level by multiplier while keeping the world point
// under screenX/screenY stable on screen (so scroll-wheel zoom feels
// anchored to the cursor, not the canvas corner).
func (c *Camera) ZoomAt(multiplier float64, screenX, screenY int, canvas Rect, gridW, gridH int) {
	worldX, worldY, ok := c.ScreenToWorld(screenX, screenY, canvas)
	if !ok || multiplier <= 0 {
		return
	}
	c.Zoom = min(max(c.zoomX()*multiplier, MinZoom), MaxZoom)
	c.ZoomY = 0
	c.X = worldX - float64(screenX-canvas.X)/c.TileSize()
	c.Y = worldY - float64(screenY-canvas.Y)/c.TileSizeY()
	c.Clamp(gridW, gridH, canvas)
}

// ScreenToWorld converts a screen pixel to fractional cell coordinates. ok is
// false when the point is outside canvas.
func (c Camera) ScreenToWorld(screenX, screenY int, canvas Rect) (worldX, worldY float64, ok bool) {
	if screenX < canvas.X || screenX >= canvas.X+canvas.W || screenY < canvas.Y || screenY >= canvas.Y+canvas.H {
		return 0, 0, false
	}
	worldX = c.X + float64(screenX-canvas.X)/c.TileSize()
	worldY = c.Y + float64(screenY-canvas.Y)/c.TileSizeY()
	return worldX, worldY, true
}

// ScreenToTile converts a screen pixel to the integer cell it falls in.
func (c Camera) ScreenToTile(screenX, screenY int, canvas Rect) (tileX, tileY int, ok bool) {
	worldX, worldY, ok := c.ScreenToWorld(screenX, screenY, canvas)
	if !ok {
		return 0, 0, false
	}
	return int(math.Floor(worldX)), int(math.Floor(worldY)), true
}

// WorldToScreen converts an integer cell coordinate to its top-left screen
// pixel.
func (c Camera) WorldToScreen(tileX, tileY int, canvas Rect) (screenX, screenY float64) {
	screenX = float64(canvas.X) + (float64(tileX)-c.X)*c.TileSize()
	screenY = float64(canvas.Y) + (float64(tileY)-c.Y)*c.TileSizeY()
	return screenX, screenY
}

// BoundaryLine is one grid line to draw as a visual scene-boundary guide
// (every boundaryTiles cells), in both world and screen coordinates.
type BoundaryLine struct {
	World  int
	Screen int
}

func visibleBoundaryWorlds(camera, span float64, limit, boundaryTiles int) []int {
	if span <= 0 || limit <= 0 || boundaryTiles <= 0 {
		return nil
	}
	visibleEnd := min(float64(limit), camera+span)
	first := int(math.Ceil(camera/float64(boundaryTiles))) * boundaryTiles
	lines := make([]int, 0, int(span/float64(boundaryTiles))+1)
	for world := first; float64(world) < visibleEnd; world += boundaryTiles {
		lines = append(lines, world)
	}
	return lines
}

// VisibleBoundaryLines returns the vertical and horizontal scene-boundary
// grid lines currently visible in canvas, spaced boundaryTilesX/Y cells
// apart on each axis. The GBA's real 240x160 display is 30x20 tiles, not
// square, so the two axes need independent spacing rather than a uniform
// 16x16 screen grid.
func (c Camera) VisibleBoundaryLines(canvas Rect, gridW, gridH, boundaryTilesX, boundaryTilesY int) (vertical, horizontal []BoundaryLine) {
	spanX, spanY := c.VisibleSpan(canvas)
	for _, world := range visibleBoundaryWorlds(c.X, spanX, gridW, boundaryTilesX) {
		screenX, _ := c.WorldToScreen(world, 0, canvas)
		vertical = append(vertical, BoundaryLine{World: world, Screen: int(math.Round(screenX))})
	}
	for _, world := range visibleBoundaryWorlds(c.Y, spanY, gridH, boundaryTilesY) {
		_, screenY := c.WorldToScreen(0, world, canvas)
		horizontal = append(horizontal, BoundaryLine{World: world, Screen: int(math.Round(screenY))})
	}
	return vertical, horizontal
}
