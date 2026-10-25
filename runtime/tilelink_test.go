//go:build !mtile_mode0 && !mtile_tilecanvas

package runtime

import "testing"

func drawTileLinkMap(c *Context, camX, camY int) []byte {
	c.Cls(0)
	c.Camera(camX, camY)
	c.Map(0, 0, 0, 0, 0, 0, 0)
	c.Camera(0, 0)
	return c.fbForTest()
}

func pixelAt(fb []byte, x, y int) byte { return fb[y*ScreenWidth+x] }

func countColor(fb []byte, color byte) int {
	n := 0
	for _, p := range fb {
		if p == color {
			n++
		}
	}
	return n
}

func TestTileLinkDrawsPoseInsteadOfTile(t *testing.T) {
	c := tileLinkFixture(t)
	fb := drawTileLinkMap(c, 0, 0)
	if got := pixelAt(fb, 16, 8); got != tileLinkTopColor {
		t.Fatalf("pose top at the linked cell = %d, want %d", got, tileLinkTopColor)
	}
	if got := pixelAt(fb, 23, 23); got != tileLinkBottomColor {
		t.Fatalf("pose bottom below the cell = %d, want %d", got, tileLinkBottomColor)
	}
	if n := countColor(fb, tileLinkMarkerColor); n != 0 {
		t.Fatalf("the linked tile itself drew %d pixels", n)
	}
	if got := c.Mget(2, 1); got != tileLinkMarker {
		t.Fatalf("mget at the linked cell = %d, want the tile %d", got, tileLinkMarker)
	}
}

func TestTileLinkMsetRemovesAndSwapsObjects(t *testing.T) {
	c := tileLinkFixture(t)
	c.Mset(2, 1, 0)
	fb := drawTileLinkMap(c, 0, 0)
	if countColor(fb, tileLinkTopColor)+countColor(fb, tileLinkBottomColor) != 0 {
		t.Fatal("object remains after mset cleared its tile")
	}
	// Writing another linked tile on the same cell draws that link instead.
	// At update tick 0 the clip shows its first frame, pose 1.
	c.Mset(2, 1, tileLinkClipMarker)
	fb = drawTileLinkMap(c, 0, 0)
	if got := pixelAt(fb, 16, 8); got != tileLinkBottomColor {
		t.Fatalf("swapped clip frame at the cell = %d, want %d", got, tileLinkBottomColor)
	}
	if c.tileLinkScanLayers != 0 {
		t.Fatal("a cell in the build-time index must not switch the layer to scanning")
	}
}

func TestTileLinkMsetOnNewCellSwitchesLayerToScan(t *testing.T) {
	c := tileLinkFixture(t)
	c.Mset(4, 3, tileLinkMarker)
	if c.tileLinkScanLayers&1 == 0 {
		t.Fatal("placing a linked tile on an unindexed cell did not switch BG0 to scanning")
	}
	fb := drawTileLinkMap(c, 0, 0)
	if got := pixelAt(fb, 32, 24); got != tileLinkTopColor {
		t.Fatalf("spawned object = %d, want %d", got, tileLinkTopColor)
	}
	if got := pixelAt(fb, 16, 8); got != tileLinkTopColor {
		t.Fatalf("original object = %d, want %d", got, tileLinkTopColor)
	}
}

func TestTileLinkReachDrawsObjectWhoseCellIsOffScreen(t *testing.T) {
	c := tileLinkFixture(t)
	// The linked cell spans world y 8..15, entirely above the screen; the
	// pose's bottom part (world y 16..23) is the top screen row band.
	fb := drawTileLinkMap(c, 0, 16)
	if got := pixelAt(fb, 16, 0); got != tileLinkBottomColor {
		t.Fatalf("visible part of an object from an off-screen cell = %d, want %d", got, tileLinkBottomColor)
	}
}

func TestTileLinkClipFollowsUpdateClock(t *testing.T) {
	c := tileLinkFixture(t)
	c.Mset(2, 1, tileLinkClipMarker)
	c.updateTick = 1 // second frame: pose 0
	fb := drawTileLinkMap(c, 0, 0)
	if got := pixelAt(fb, 16, 8); got != tileLinkTopColor {
		t.Fatalf("clip frame 1 at tick 1 = %d, want pose 0's top %d", got, tileLinkTopColor)
	}
}

func TestTileLinkIndexAndScanDrawIdentically(t *testing.T) {
	index := tileLinkFixture(t)
	scan := tileLinkFixture(t)
	scan.tileLinkScanLayers = 1
	for _, cam := range [][2]int{{0, 0}, {-5, 3}, {9, -7}, {0, 16}, {20, 20}} {
		a := append([]byte(nil), drawTileLinkMap(index, cam[0], cam[1])...)
		b := drawTileLinkMap(scan, cam[0], cam[1])
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("camera %v pixel %d: index %d, scan %d", cam, i, a[i], b[i])
			}
		}
	}
}

func tileLinkFrameForTest(t *testing.T, c *Context, camX, camY, w, h, mask int) []byte {
	t.Helper()
	c.Cls(0)
	c.Camera(camX, camY)
	c.Map(0, 0, 0, 0, w, h, mask)
	return c.fbForTest()
}

func TestTileLinkIndexDrawsAfterUint16Boundary(t *testing.T) {
	c := tileLinkFixture(t)
	const width, height = 512, 129
	c.cart.MapW, c.cart.MapH = width, height
	c.cart.Map = make([]uint16, width*height)
	c.cart.TileLinkRows = make([]uint32, height+1)
	c.cart.TileLinkInstances = make([]TileLinkInstance, width*height)
	for y := 0; y < height; y++ {
		c.cart.TileLinkRows[y] = uint32(y * width)
		for x := 0; x < width; x++ {
			c.cart.Map[y*width+x] = tileLinkMarker
			c.cart.TileLinkInstances[y*width+x] = TileLinkInstance{X: uint16(x), Y: uint16(y)}
		}
	}
	c.cart.TileLinkRows[height] = uint32(width * height)
	fb := drawTileLinkMap(c, 0, 128*8)
	if got := pixelAt(fb, 0, 0); got != tileLinkTopColor {
		t.Fatalf("object at instance 65536 = %d, want %d", got, tileLinkTopColor)
	}
}
