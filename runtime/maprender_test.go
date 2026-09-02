//go:build !mtile_mode0

package runtime

import "testing"

func TestMapDefaultSizeCoversClip(t *testing.T) {
	sheet := make([]byte, 16*8)
	sheet[8+1] = 7
	img := &AssetImage{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1,
		MapW: 4, MapH: 2, SpriteN: 2,
		Sheet: sheet, Map: []uint16{1, 1, 1, 1, 1, 1, 1, 1}, Flags: []byte{0, 1},
	}
	ram := &AssetMemory{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1,
		MapW: 4, MapH: 2, SpriteN: 2,
		Sheet: make([]byte, 128), Map: make([]uint16, 8), Flags: make([]byte, 2),
	}
	copy(ram.Sheet, sheet)
	copy(ram.Map, img.Map)
	copy(ram.Flags, img.Flags)
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.PaltColor(0, false)
	c.Clip(0, 0, 16, 8)
	c.Map(0, 0, 0, 0, 0, 0, 0)
	if c.Pget(1, 0) != 7 {
		t.Fatal("default map size skipped the first clip cell")
	}
	if c.Pget(9, 0) != 7 {
		t.Fatal("default map size skipped the second clip cell")
	}
}

func TestMapDefaultSizeCoversClipAfterWholeTileCameraOffset(t *testing.T) {
	sheet := make([]byte, 16*8)
	for y := 0; y < 8; y++ {
		for x := 8; x < 16; x++ {
			sheet[y*16+x] = 7
		}
	}
	mapData := make([]uint16, 40*30)
	for i := range mapData {
		mapData[i] = 1
	}
	img := &AssetImage{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1,
		MapW: 40, MapH: 30, SpriteN: 2,
		Sheet: sheet, Map: mapData, Flags: make([]byte, 2),
	}
	ram := &AssetMemory{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1,
		MapW: 40, MapH: 30, SpriteN: 2,
		Sheet: append([]byte(nil), sheet...), Map: append([]uint16(nil), mapData...), Flags: make([]byte, 2),
	}
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.PaltColor(0, false)
	c.Cls(3)
	c.Camera(16, 16)
	c.Map(0, 0, 0, 0, 0, 0, 0)
	// Pget is camera-relative; reset it so these assertions inspect the
	// framebuffer's physical right and bottom edges.
	c.CameraReset()
	if got := c.Pget(239, 0); got != 7 {
		t.Fatalf("right edge after camera offset = %d, want 7", got)
	}
	if got := c.Pget(0, 159); got != 7 {
		t.Fatalf("bottom edge after camera offset = %d, want 7", got)
	}
}

func TestMapSkipsSpriteZeroByDefault(t *testing.T) {
	img, ram := spriteFixture()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.Cls(3)
	c.PaltColor(0, false)
	c.Map(0, 0, 0, 0, 1, 1, 0)
	if c.Pget(1, 0) != 3 {
		t.Fatal("map drew sprite 0 instead of treating it as empty")
	}
}

func TestMapLayerFiltersFlags(t *testing.T) {
	sheet := make([]byte, 16*8)
	sheet[8+1] = 7
	img := &AssetImage{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1,
		MapW: 1, MapH: 1, SpriteN: 2,
		Sheet: sheet, Map: []uint16{1}, Flags: []byte{0, 0x01},
	}
	ram := &AssetMemory{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1,
		MapW: 1, MapH: 1, SpriteN: 2,
		Sheet: append([]byte(nil), sheet...), Map: []uint16{1}, Flags: []byte{0, 0x01},
	}
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.PaltColor(0, false)
	c.Map(0, 0, 0, 8, 1, 1, 2)
	if c.Pget(1, 8) != 0 {
		t.Fatal("map layer 2 drew a sprite without that flag")
	}
	c.Map(0, 0, 0, 8, 1, 1, 1)
	if c.Pget(1, 8) != 7 {
		t.Fatal("map layer 1 skipped a sprite that has flag 0")
	}

	ram.Flags[1] = 0x01
	c.Cls(0)
	c.Map(0, 0, 0, 8, 1, 1, 0x05)
	if c.Pget(1, 8) != 0 {
		t.Fatal("map layer 0x05 drew a sprite that had only one requested flag")
	}
	ram.Flags[1] = 0x05
	c.Map(0, 0, 0, 8, 1, 1, 0x05)
	if c.Pget(1, 8) != 7 {
		t.Fatal("map layer 0x05 skipped a sprite with all requested flags")
	}
}

func TestExplicitMapBackgroundReadsIndependentPlanesBackToFront(t *testing.T) {
	sheet := make([]byte, 24*8)
	for y := 0; y < 8; y++ {
		for x := 8; x < 16; x++ {
			sheet[y*24+x] = 4
		}
		for x := 16; x < 24; x++ {
			sheet[y*24+x] = 9
		}
	}
	mapData := []uint16{1, 0, 2, 0}
	img := &AssetImage{
		SheetW: 24, SheetH: 8, SpriteCols: 3, SpriteRows: 1, SpriteN: 3,
		MapW: 1, MapH: 1, MapLayers: 4,
		Sheet: sheet, Map: mapData, Flags: make([]byte, 3),
	}
	ram := &AssetMemory{
		SheetW: 24, SheetH: 8, SpriteCols: 3, SpriteRows: 1, SpriteN: 3,
		MapW: 1, MapH: 1, MapLayers: 4,
		Sheet: make([]byte, len(sheet)), Map: make([]uint16, len(mapData)), Flags: make([]byte, 3),
	}
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init rejected layered map")
	}
	c.Cls(1)
	c.MapBG(0, 0, 0, 0, 0, 1, 1, 0)
	c.MapBG(2, 0, 0, 0, 0, 1, 1, 0)
	if got := c.Pget(0, 0); got != 9 {
		t.Fatalf("front BG2 pixel = %d, want 9", got)
	}
	c.Cls(1)
	c.MapBG(0, 0, 0, 0, 0, 1, 1, 0)
	if got := c.Pget(0, 0); got != 4 {
		t.Fatalf("BG0 pixel = %d, want 4", got)
	}
	if c.Mget(0, 0) != 1 {
		t.Fatalf("legacy Mget no longer addresses BG0")
	}
}

func TestMapAllDrawsSparseLogicalPlanesFromCompactStorage(t *testing.T) {
	sheet := make([]byte, 24*8)
	for i := 8; i < 16; i++ {
		sheet[i] = 4
	}
	for i := 16; i < 24; i++ {
		sheet[i] = 9
	}
	img := &AssetImage{
		SheetW: 24, SheetH: 8, SpriteCols: 3, SpriteRows: 1, SpriteN: 3,
		MapW: 1, MapH: 1, MapLayers: 4, MapPlaneMask: 0x0a,
		Sheet: sheet, Map: []uint16{1, 2}, Flags: make([]byte, 3),
	}
	ram := &AssetMemory{
		SheetW: 24, SheetH: 8, SpriteCols: 3, SpriteRows: 1, SpriteN: 3,
		MapW: 1, MapH: 1, MapLayers: 4, MapPlaneMask: 0x0a,
		Sheet: make([]byte, len(sheet)), Map: make([]uint16, 2), Flags: make([]byte, 3),
	}
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init rejected compact BG1/BG3 map")
	}
	c.Cls(0)
	c.MapAll(0x0a)
	if got := c.Pget(0, 0); got != 9 {
		t.Fatalf("BG3 composite pixel = %d, want 9", got)
	}
	if got := c.mapGetLayer(1, 0, 0); got != 1 {
		t.Fatalf("BG1 compact cell = %d, want 1", got)
	}
	if got := c.mapGetLayer(3, 0, 0); got != 2 {
		t.Fatalf("BG3 compact cell = %d, want 2", got)
	}
	if got := c.mapGetLayer(0, 0, 0); got != 0 {
		t.Fatalf("absent BG0 cell = %d, want 0", got)
	}
}

func TestMapWithCameraAndClipEdgeCells(t *testing.T) {
	sheet := make([]byte, 16*16)
	for i := range sheet {
		sheet[i] = 5
	}
	img := &AssetImage{
		SheetW: 16, SheetH: 16, SpriteCols: 2, SpriteRows: 2,
		MapW: 3, MapH: 3, SpriteN: 4,
		Sheet: sheet, Map: []uint16{1, 2, 3, 1, 2, 3, 1, 2, 3}, Flags: []byte{0, 1, 1, 1},
	}
	ram := &AssetMemory{
		SheetW: 16, SheetH: 16, SpriteCols: 2, SpriteRows: 2,
		MapW: 3, MapH: 3, SpriteN: 4,
		Sheet: append([]byte(nil), sheet...), Map: append([]uint16(nil), img.Map...), Flags: append([]byte(nil), img.Flags...),
	}
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.PaltColor(0, false)
	c.Camera(3, 2)
	c.Cls(0)
	c.Clip(4, 4, 12, 10)
	c.Map(0, 0, 0, 0, 3, 3, 0)

	// Clip is screen-space [4,4)-(16,14). Camera is (3,2).
	// Map draws at screen positions (screenX+mx*8-camX, screenY+my*8-camY)
	// = (mx*8-3, my*8-2). 3x3 cells = 24x24 world pixels starting at (0,0).
	// Screen coverage: (-3,-2) to (21,22), clipped to (4,4)-(16,14).

	checks := []struct {
		x, y int
		want byte
		desc string
	}{
		{7, 6, 5, "clip top-left corner (4,4)"},
		{6, 6, 0, "one left of clip (3,4)"},
		{7, 5, 0, "one above clip (4,3)"},
		{18, 15, 5, "inside clip near bottom-right (15,13)"},
		{19, 16, 0, "just outside clip bottom-right (16,14)"},
		{10, 10, 5, "center of map in clip"},
		{3, 2, 0, "screen origin (0,0) outside clip"},
	}
	for _, tc := range checks {
		if got := c.Pget(tc.x, tc.y); got != tc.want {
			t.Fatalf("Pget(%d,%d) [%s] = %d, want %d", tc.x, tc.y, tc.desc, got, tc.want)
		}
	}
}

// TestMapNonZeroCellXY verifies that cellX/cellY offsets are properly
// applied when they are non-zero. Without the fix for the bug where
// baseXOrigin = screenX - camX (missing cellX*8) and
// baseY = screenY + my*8 - camY (missing cellY*8),
// calling map(3, 1, ...) would draw cells from column 0, row 0 where
// column 3, row 1 was requested.
func TestMapNonZeroCellXY(t *testing.T) {
	sheet := make([]byte, 32*8)
	for id := 0; id < 4; id++ {
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				sheet[id*8+y*32+x] = byte(id*10 + 5)
			}
		}
	}
	mapData := make([]uint16, 32)
	for i := range mapData {
		mapData[i] = uint16(i % 4)
	}
	img := &AssetImage{
		SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1,
		MapW: 8, MapH: 4, SpriteN: 4,
		Sheet: sheet, Map: mapData, Flags: []byte{0, 1, 1, 1},
	}
	ram := &AssetMemory{
		SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1,
		MapW: 8, MapH: 4, SpriteN: 4,
		Sheet: append([]byte(nil), sheet...), Map: append([]uint16(nil), mapData...), Flags: append([]byte(nil), img.Flags...),
	}
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.PaltColor(0, false)
	c.Cls(0)

	// Draw 3x2 cells starting at map cell (3, 1), placed at screen (8, 8).
	// Expected world cells: (3,1)=3, (4,1)=0, (5,1)=1, (3,2)=3, (4,2)=0, (5,2)=1
	// Screen positions: cell at (cellX, cellY) goes to screenX + (cellX+mx)*8 - camX
	// Here: screenX=8, camX=0, cellX=3, so mx=0 -> screen pos 8+3*8=32
	//                                 mx=1 -> screen pos 8+4*8=40
	//                                 mx=2 -> screen pos 8+5*8=48
	//                screenY=8, camY=0, cellY=1, so my=0 -> screen pos 8+1*8=16
	//                                 my=1 -> screen pos 8+2*8=24
	c.Map(3, 1, 8, 8, 3, 2, 0)

	checks := []struct {
		sx, sy int
		want   byte
		desc   string
	}{
		{32, 16, 35, "cell (3,1) = sprite 3 at (32,16)"},
		{40, 16, 0, "cell (4,1) = sprite 0 at (40,16) (empty)"},
		{48, 16, 15, "cell (5,1) = sprite 1 at (48,16)"},
		{32, 24, 35, "cell (3,2) = sprite 3 at (32,24)"},
		{40, 24, 0, "cell (4,2) = sprite 0 at (40,24) (empty)"},
		{48, 24, 15, "cell (5,2) = sprite 1 at (48,24)"},
	}
	for _, tc := range checks {
		got := c.Pget(tc.sx, tc.sy)
		if got != tc.want {
			t.Fatalf("Pget(%d,%d) [%s] = %d, want %d", tc.sx, tc.sy, tc.desc, got, tc.want)
		}
	}
}

func mapSprCompareFixture() (*AssetImage, func() *AssetMemory) {
	sheet := make([]byte, 16*8)
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			if (row+col)%2 == 0 {
				sheet[row*16+8+col] = 13
			}
		}
	}
	img := &AssetImage{
		SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1,
		MapW: 1, MapH: 1, SpriteN: 2,
		Sheet: sheet, Map: []uint16{1}, Flags: []byte{0, 1},
	}
	newRAM := func() *AssetMemory {
		return &AssetMemory{
			SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1,
			MapW: 1, MapH: 1, SpriteN: 2,
			Sheet: append([]byte(nil), sheet...), Map: []uint16{1}, Flags: []byte{0, 1},
		}
	}
	return img, newRAM
}

func assertFramesMatch(t *testing.T, got, want *Context, label string) {
	t.Helper()
	for y := 0; y < ScreenHeight; y++ {
		for x := 0; x < ScreenWidth; x++ {
			if g, w := got.pixelRaw(x, y), want.pixelRaw(x, y); g != w {
				t.Fatalf("%s framebuffer mismatch at screen (%d,%d): map %d, spr %d", label, x, y, g, w)
			}
		}
	}
}

func TestMapPackedAlignedMatchesSpr(t *testing.T) {
	img, newRAM := mapSprCompareFixture()
	var mapped, sprite Context
	if !mapped.Init(img, newRAM()) || !sprite.Init(img, newRAM()) {
		t.Fatal("Init returned false")
	}
	mapped.Cls(3)
	sprite.Cls(3)
	mapped.Map(0, 0, 80, 40, 1, 1, 0)
	sprite.Spr(1, 80, 40, 1, 1, false, false)
	assertFramesMatch(t, &mapped, &sprite, "aligned packed map")
}

func TestMapYClipMatchesSpr(t *testing.T) {
	img, newRAM := mapSprCompareFixture()
	var mapped, sprite Context
	if !mapped.Init(img, newRAM()) || !sprite.Init(img, newRAM()) {
		t.Fatal("Init returned false")
	}
	mapped.Camera(0, 2)
	sprite.Camera(0, 2)
	mapped.Cls(3)
	sprite.Cls(3)
	mapped.Map(0, 0, 16, 0, 1, 1, 0)
	sprite.Spr(1, 16, 0, 1, 1, false, false)
	assertFramesMatch(t, &mapped, &sprite, "y-clipped map")
}

func TestMapXClipMatchesSpr(t *testing.T) {
	img, newRAM := mapSprCompareFixture()
	var mapped, sprite Context
	if !mapped.Init(img, newRAM()) || !sprite.Init(img, newRAM()) {
		t.Fatal("Init returned false")
	}
	mapped.Clip(20, 0, 10, 16)
	sprite.Clip(20, 0, 10, 16)
	mapped.Cls(3)
	sprite.Cls(3)
	mapped.Map(0, 0, 16, 0, 1, 1, 0)
	sprite.Spr(1, 16, 0, 1, 1, false, false)
	assertFramesMatch(t, &mapped, &sprite, "x-clipped map")
}

func TestSSprNearestNeighborAndCamera(t *testing.T) {
	img, ram := spriteFixture()
	var c Context
	if !c.Init(img, ram) {
		t.Fatal("Init returned false")
	}
	c.SSpr(1, 0, 1, 1, 10, 10, 2, 1, false, false)
	if c.Pget(10, 10) != 7 || c.Pget(11, 10) != 7 {
		t.Fatal("sspr did not stretc the opaque source pioxel")
	}
	c.Camera(3, 0)
	c.SSpr(1, 0, 1, 1, 3, 20, 1, 1, false, false)
	if c.Pget(3, 20) != 7 {
		t.Fatal("sspr did not apply camera")
	}
}

func TestSSprMatchesNearestNeighborReferenceWithClippingAndFlips(t *testing.T) {
	sheet := make([]byte, 8*8)
	for i := range sheet {
		sheet[i] = byte(i + 1)
	}
	img := &AssetImage{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		Sheet: sheet, Map: []uint16{}, Flags: []byte{0},
	}
	ram := &AssetMemory{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		MapW: 0, MapH: 0,
		Sheet: append([]byte(nil), sheet...), Map: []uint16{}, Flags: []byte{0},
	}
	for _, flipX := range []bool{false, true} {
		for _, flipY := range []bool{false, true} {
			t.Run(flipName(flipX, flipY), func(t *testing.T) {
				var c Context
				if !c.Init(img, ram) {
					t.Fatal("Init returned false")
				}
				c.Cls(200)
				c.SSpr(1, 2, 5, 4, -2, 3, 9, 7, flipX, flipY)

				for y := 0; y < ScreenHeight; y++ {
					for x := 0; x < ScreenWidth; x++ {
						want := byte(200)
						ox, oy := x+2, y-3
						if ox >= 0 && ox < 9 && oy >= 0 && oy < 7 {
							sourceX := ox * 5 / 9
							sourceY := oy * 4 / 7
							if flipX {
								sourceX = 4 - sourceX
							}
							if flipY {
								sourceY = 3 - sourceY
							}
							want = sheet[(2+sourceY)*8+1+sourceX]
						}
						if got := c.Pget(x, y); got != want {
							t.Fatalf("pixel (%d,%d) = %d, want %d", x, y, got, want)
						}
					}
				}
			})
		}
	}
}

func flipName(flipX, flipY bool) string {
	if flipX {
		if flipY {
			return "flip_xy"
		}
		return "flip_x"
	}
	if flipY {
		return "flip_y"
	}
	return "normal"
}
