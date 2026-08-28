package editor

import "testing"

func inDelta(t *testing.T, got, want, delta float64) {
	t.Helper()
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > delta {
		t.Fatalf("got %v, want %v (delta %v)", got, want, delta)
	}
}

func TestCameraTileSize(t *testing.T) {
	c := Camera{Zoom: 2}
	inDelta(t, c.TileSize(), 16.0, 0.001)
}

func TestCameraScreenToTile(t *testing.T) {
	c := Camera{X: 12.5, Y: 7.25, Zoom: 1.5}
	canvas := Rect{X: 10, Y: 10, W: 160, H: 120}

	tileX, tileY, ok := c.ScreenToTile(34, 26, canvas)
	if !ok {
		t.Fatal("expected point inside canvas")
	}
	if tileX != 14 || tileY != 8 {
		t.Fatalf("ScreenToTile = (%d,%d), want (14,8)", tileX, tileY)
	}
}

func TestCameraScreenToTileOutsideCanvasIsRejected(t *testing.T) {
	c := NewCamera()
	canvas := Rect{X: 10, Y: 10, W: 160, H: 120}
	if _, _, ok := c.ScreenToTile(0, 0, canvas); ok {
		t.Fatal("point outside canvas unexpectedly accepted")
	}
}

func TestCameraClamp(t *testing.T) {
	c := Camera{X: 200, Y: 200, Zoom: 1}
	canvas := Rect{X: 10, Y: 10, W: 160, H: 120}

	c.Clamp(128, 128, canvas)

	inDelta(t, c.X, 108.0, 0.001)
	inDelta(t, c.Y, 113.0, 0.001)
}

func TestCameraPan(t *testing.T) {
	c := Camera{X: 10, Y: 20, Zoom: 1}
	canvas := Rect{X: 10, Y: 10, W: 160, H: 120}

	c.Pan(2.5, -3.0, canvas, 128, 128)

	inDelta(t, c.X, 12.5, 0.001)
	inDelta(t, c.Y, 17.0, 0.001)
}

// TestCameraZoomAtKeepsAnchorStable guards the anchored-zoom UX: whatever
// world cell is under the cursor before a scroll-wheel zoom must still be
// under the cursor afterward, otherwise zooming in/out visibly "slides" the
// map instead of zooming into the point the user is pointing at.
func TestCameraZoomAtKeepsAnchorStable(t *testing.T) {
	c := Camera{X: 20, Y: 10, Zoom: 1}
	canvas := Rect{X: 10, Y: 10, W: 160, H: 120}

	beforeWorldX, beforeWorldY, ok := c.ScreenToWorld(90, 70, canvas)
	if !ok {
		t.Fatal("expected point inside canvas")
	}
	beforeX, beforeY, ok := c.ScreenToTile(90, 70, canvas)
	if !ok {
		t.Fatal("expected point inside canvas")
	}

	c.ZoomAt(ZoomStep, 90, 70, canvas, 128, 128)

	afterWorldX, afterWorldY, ok := c.ScreenToWorld(90, 70, canvas)
	if !ok {
		t.Fatal("expected point inside canvas after zoom")
	}
	afterX, afterY, ok := c.ScreenToTile(90, 70, canvas)
	if !ok {
		t.Fatal("expected point inside canvas after zoom")
	}
	inDelta(t, beforeWorldX, afterWorldX, 0.000001)
	inDelta(t, beforeWorldY, afterWorldY, 0.000001)
	if beforeX != afterX || beforeY != afterY {
		t.Fatalf("zoom anchor tile moved: before=(%d,%d) after=(%d,%d)", beforeX, beforeY, afterX, afterY)
	}
}

func TestCameraZoomAtClampsToMinMax(t *testing.T) {
	c := Camera{Zoom: MinZoom}
	canvas := Rect{X: 0, Y: 0, W: 80, H: 80}
	c.ZoomAt(1/ZoomStep, 10, 10, canvas, 128, 128)
	if c.Zoom != MinZoom {
		t.Fatalf("zoom below MinZoom: %v", c.Zoom)
	}

	c = Camera{Zoom: MaxZoom}
	c.ZoomAt(ZoomStep, 10, 10, canvas, 128, 128)
	if c.Zoom != MaxZoom {
		t.Fatalf("zoom above MaxZoom: %v", c.Zoom)
	}
}

func TestCameraVisibleBoundaryLines(t *testing.T) {
	c := Camera{Zoom: 1}
	canvas := Rect{X: 0, Y: 0, W: 8 * 20, H: 8 * 20}
	vertical, horizontal := c.VisibleBoundaryLines(canvas, 100, 100, 16, 16)
	if len(vertical) == 0 || vertical[0].World != 0 {
		t.Fatalf("expected a boundary line at world 0, got %#v", vertical)
	}
	if len(horizontal) == 0 || horizontal[0].World != 0 {
		t.Fatalf("expected a boundary line at world 0, got %#v", horizontal)
	}
}

// TestCameraVisibleBoundaryLinesSupportsAsymmetricSpacing guards the GBA
// screen-preview use case: the real display is 30x20 tiles (240x160px), not
// square, so the two axes must be settable independently.
func TestCameraVisibleBoundaryLinesSupportsAsymmetricSpacing(t *testing.T) {
	c := Camera{Zoom: 1}
	canvas := Rect{X: 0, Y: 0, W: 8 * 64, H: 8 * 40}
	vertical, horizontal := c.VisibleBoundaryLines(canvas, 200, 200, 30, 20)
	wantV := []int{0, 30, 60}
	wantH := []int{0, 20}
	if len(vertical) != len(wantV) {
		t.Fatalf("vertical lines = %#v, want worlds %v", vertical, wantV)
	}
	for i, w := range wantV {
		if vertical[i].World != w {
			t.Fatalf("vertical[%d].World = %d, want %d", i, vertical[i].World, w)
		}
	}
	if len(horizontal) != len(wantH) {
		t.Fatalf("horizontal lines = %#v, want worlds %v", horizontal, wantH)
	}
	for i, w := range wantH {
		if horizontal[i].World != w {
			t.Fatalf("horizontal[%d].World = %d, want %d", i, horizontal[i].World, w)
		}
	}
}

func TestVisibleBoundaryWorldsRejectsDegenerateInputs(t *testing.T) {
	if got := visibleBoundaryWorlds(0, 0, 100, 16); got != nil {
		t.Fatalf("zero span should yield no lines, got %#v", got)
	}
	if got := visibleBoundaryWorlds(0, 10, 0, 16); got != nil {
		t.Fatalf("zero limit should yield no lines, got %#v", got)
	}
	if got := visibleBoundaryWorlds(0, 10, 100, 0); got != nil {
		t.Fatalf("zero boundaryTiles should yield no lines, got %#v", got)
	}
}

// TestCameraFitKeepsSquareCells guards the spritesheet-panel UX: cells stay
// 1:1 even when the panel aspect does not match the sheet. The limiting
// axis fills the canvas; the other letterboxes.
func TestCameraFitKeepsSquareCells(t *testing.T) {
	c := NewCamera()
	canvas := Rect{X: 10, Y: 20, W: 440, H: 600}
	c.Fit(30, 20, canvas)

	inDelta(t, c.TileSize(), c.TileSizeY(), 0.001)
	inDelta(t, c.TileSize()*30, float64(canvas.W), 0.001)
	if c.TileSizeY()*20 > float64(canvas.H)+0.001 {
		t.Fatalf("fitted height %g overflowed canvas %d", c.TileSizeY()*20, canvas.H)
	}

	x, y := c.WorldToScreen(30, 20, canvas)
	inDelta(t, x, float64(canvas.X+canvas.W), 0.001)
	if y > float64(canvas.Y+canvas.H)+0.001 {
		t.Fatalf("fitted bottom %g overflowed canvas bottom %d", y, canvas.Y+canvas.H)
	}
}

// TestCameraFitCapsTinyGridZoom guards the one-sprite project case. Fit may
// zoom below MinZoom for large sheets, but it must not magnify a single tile
// until it fills the spritesheet panel.
func TestCameraFitCapsTinyGridZoom(t *testing.T) {
	c := NewCamera()
	canvas := Rect{X: 10, Y: 20, W: 900, H: 600}
	c.Fit(1, 1, canvas)

	inDelta(t, c.TileSize(), BaseTileSize*MaxZoom, 0.001)
	inDelta(t, c.TileSize(), c.TileSizeY(), 0.001)

	left, top := c.WorldToScreen(0, 0, canvas)
	right, bottom := c.WorldToScreen(1, 1, canvas)
	inDelta(t, left, float64(canvas.X)+(float64(canvas.W)-c.TileSize())/2, 0.001)
	inDelta(t, top, float64(canvas.Y)+(float64(canvas.H)-c.TileSizeY())/2, 0.001)
	inDelta(t, right-left, BaseTileSize*MaxZoom, 0.001)
	inDelta(t, bottom-top, BaseTileSize*MaxZoom, 0.001)
}

func TestCameraFitWithMaxZoomMakesCompactSheetsEditable(t *testing.T) {
	c := NewCamera()
	canvas := Rect{X: 10, Y: 20, W: 900, H: 600}
	c.FitWithMaxZoom(2, 2, canvas, 16)

	inDelta(t, c.TileSize(), 128, 0.001)
	inDelta(t, c.TileSizeY(), 128, 0.001)

	left, top := c.WorldToScreen(0, 0, canvas)
	right, bottom := c.WorldToScreen(2, 2, canvas)
	inDelta(t, right-left, 256, 0.001)
	inDelta(t, bottom-top, 256, 0.001)
	inDelta(t, left, float64(canvas.X)+(float64(canvas.W)-256)/2, 0.001)
	inDelta(t, top, float64(canvas.Y)+(float64(canvas.H)-256)/2, 0.001)
}

func TestCameraFitWithMaxZoomRejectsInvalidMaximum(t *testing.T) {
	c := Camera{Zoom: 2, ZoomY: 3, X: 4, Y: 5}
	c.FitWithMaxZoom(2, 2, Rect{W: 900, H: 600}, 0)
	if c.Zoom != 2 || c.ZoomY != 3 || c.X != 4 || c.Y != 5 {
		t.Fatalf("invalid maximum mutated camera: %+v", c)
	}
}

func TestCameraFitIgnoresDegenerateInputs(t *testing.T) {
	c := Camera{Zoom: 2, ZoomY: 3, X: 4, Y: 5}
	c.Fit(0, 20, Rect{W: 100, H: 100})
	c.Fit(30, 0, Rect{W: 100, H: 100})
	c.Fit(30, 20, Rect{})
	if c.Zoom != 2 || c.ZoomY != 3 || c.X != 4 || c.Y != 5 {
		t.Fatalf("degenerate Fit mutated camera: %+v", c)
	}
}

// TestCameraFitStretchCoversCanvas keeps the stretch primitive honest: it
// still fills the panel even when that means non-square cells.
func TestCameraFitStretchCoversCanvas(t *testing.T) {
	c := NewCamera()
	canvas := Rect{X: 10, Y: 20, W: 440, H: 600}
	c.FitStretch(30, 20, canvas)

	inDelta(t, c.TileSize()*30, float64(canvas.W), 0.001)
	inDelta(t, c.TileSizeY()*20, float64(canvas.H), 0.001)
	inDelta(t, c.X, 0, 0.001)
	inDelta(t, c.Y, 0, 0.001)

	x, y := c.WorldToScreen(30, 20, canvas)
	inDelta(t, x, float64(canvas.X+canvas.W), 0.001)
	inDelta(t, y, float64(canvas.Y+canvas.H), 0.001)

	tileX, tileY, ok := c.ScreenToTile(canvas.X+canvas.W-1, canvas.Y+canvas.H-1, canvas)
	if !ok {
		t.Fatal("bottom-right pixel of a stretched sheet was outside the canvas")
	}
	if tileX != 29 || tileY != 19 {
		t.Fatalf("ScreenToTile(bottom-right) = (%d,%d), want (29,19)", tileX, tileY)
	}
}

func TestCameraFitStretchIgnoresDegenerateInputs(t *testing.T) {
	c := Camera{Zoom: 2, ZoomY: 3, X: 4, Y: 5}
	c.FitStretch(0, 20, Rect{W: 100, H: 100})
	c.FitStretch(30, 0, Rect{W: 100, H: 100})
	c.FitStretch(30, 20, Rect{})
	if c.Zoom != 2 || c.ZoomY != 3 || c.X != 4 || c.Y != 5 {
		t.Fatalf("degenerate FitStretch mutated camera: %+v", c)
	}
}

// TestCameraClampCentersWhenViewIsLargerThanGrid guards letterboxing after
// a fit-to-panel zoom: if the whole grid is visible, it sits in the middle
// of the leftover space rather than sticking to the top-left corner.
func TestCameraClampCentersWhenViewIsLargerThanGrid(t *testing.T) {
	c := NewCamera()                           // zoom 1, 8px cells
	canvas := Rect{X: 0, Y: 0, W: 160, H: 120} // 20 x 15 cells visible
	c.Clamp(10, 10, canvas)
	inDelta(t, c.X, (10.0-20.0)/2.0, 0.001)
	inDelta(t, c.Y, (10.0-15.0)/2.0, 0.001)
}
