package runtime

import "testing"

const (
	tileLinkMarker      = 1  // linked to pose 0
	tileLinkClipMarker  = 2  // linked to clip "blink"
	tileLinkMarkerColor = 3  // pixels of both marker tiles
	tileLinkTopColor    = 9  // pose 0 top part (sheet tile 2)
	tileLinkBottomColor = 10 // pose 0 bottom part, and pose 1 (sheet tile 3)
)

// tileLinkFixture is a 6x6 map with tile 1 at (2,1). Tile 1 links to pose 0,
// an 8x16 pose hanging one cell below its cell; tile 2 links to clip "blink",
// which alternates pose 1 (one bottom-colored tile) and pose 0.
func tileLinkFixture(t *testing.T) *Context {
	t.Helper()
	sheet := make([]byte, 4*64)
	for i := 0; i < 64; i++ {
		sheet[64+i] = tileLinkMarkerColor
		sheet[2*64+i] = tileLinkTopColor
		sheet[3*64+i] = tileLinkBottomColor
	}
	// The sheet is 32x8 pixels: four cells side by side, row-major pixels.
	pixels := make([]byte, 32*8)
	for cell := 0; cell < 4; cell++ {
		for y := 0; y < 8; y++ {
			copy(pixels[y*32+cell*8:y*32+cell*8+8], sheet[cell*64+y*8:cell*64+y*8+8])
		}
	}
	mapData := make([]uint16, 6*6)
	mapData[1*6+2] = tileLinkMarker
	image := &AssetImage{
		SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1, SpriteN: 4,
		MapW: 6, MapH: 6, MapLayers: 1, Sheet: pixels, Map: mapData, Flags: make([]byte, 4),
		PoseIndex:       []PoseRange{{Start: 0, Count: 2, FlipOffsetX: 8, FlipOffsetY: 16}, {Start: 2, Count: 1, FlipOffsetX: 8, FlipOffsetY: 8}},
		PoseParts:       []PosePart{{Tile: 2, X: 0, Y: 0}, {Tile: 3, X: 0, Y: 8}, {Tile: 3, X: 0, Y: 0}},
		AnimationClips:  []AnimationClip{{Name: "blink", Count: 2, FPS: 30, Loop: true, FlipOffsetX: 8, FlipOffsetY: 16}},
		AnimationFrames: []uint16{1, 0},
	}
	ram := &AssetMemory{
		SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1, SpriteN: 4,
		MapW: 6, MapH: 6, MapLayers: 1, Sheet: make([]byte, len(pixels)), Map: make([]uint16, len(mapData)), Flags: make([]byte, 4),
	}
	c := new(Context)
	if !c.Init(image, ram) {
		t.Fatal("init tile link fixture")
	}
	c.cart.TileLinkOf = []uint8{0, 1, 2, 0}
	c.cart.TileLinks = []TileLink{{Tile: tileLinkMarker, ID: 0}, {Tile: tileLinkClipMarker, Clip: true, ID: 0}}
	c.cart.TileLinkInstances = []TileLinkInstance{{X: 2, Y: 1}}
	c.cart.TileLinkRows = []uint32{0, 0, 1, 1, 1, 1, 1}
	c.cart.TileLinkReach = TileLinkReach{Bottom: 8}
	c.SetTickRate(30)
	return c
}
