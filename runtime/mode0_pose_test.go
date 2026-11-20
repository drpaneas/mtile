//go:build mtile_mode0 && mtile_mode0_obj && !mtile_mode0_no_pspr

package runtime

import (
	"fmt"
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func mode0PsprFixture(t *testing.T) *Context {
	t.Helper()
	hw.ResetForTest()
	objCell := make([]byte, 64)
	objCell[0] = 11
	sheet := make([]byte, 64)
	sheet[0] = 3
	image := AssetImage{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		Sheet: sheet, MapW: 1, MapH: 1, Map: []uint16{0}, Flags: []byte{0},
		PoseIndex: []PoseRange{{Start: 0, Count: 1, FlipOffsetX: 8, FlipOffsetY: 8}},
		PoseParts: []PosePart{{Tile: 0, X: 0, Y: 0}},
		ObjSheetW: 8, ObjSheetH: 8, ObjSpriteCols: 1, ObjSpriteRows: 1, ObjSpriteN: 1,
		ObjSheet: objCell, ObjSheetIndex: []uint32{1},
	}
	ram := &AssetMemory{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		MapW: 1, MapH: 1,
		Sheet: make([]byte, 64), Map: make([]uint16, 1), Flags: make([]byte, 1),
	}
	var c Context
	if !c.Init(&image, ram) {
		t.Fatal("init")
	}
	c.cart.PoseOBJStage = make([]uint32, mode0PsprWindowLimit*16)
	c.InitHardware()
	return &c
}

func TestMode0PSprRemapsOAMTileAndUploadsOnChange(t *testing.T) {
	requireMode0LayerCount(t, 1)
	c := mode0PsprFixture(t)
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 0, 0, 10, 20, false, false)
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM + 4); got&0x3ff != 0 {
		t.Fatalf("OAM attr2 tile units = %d, want remapped slot 0", got&0x3ff)
	}
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM)); got != 11 {
		t.Fatalf("streamed OBJ pixel = %d, want object-sheet 11", got)
	}
	composed := composeMode0Objects(t)
	if composed[20*ScreenWidth+10] != 11 {
		t.Fatalf("composed pixel at (10,20) = %d, want 11", composed[20*ScreenWidth+10])
	}
	stats := c.RendererUploadStats()
	if stats.LastOBJTileBytes < 64 {
		t.Fatalf("pose change uploaded %d OBJ bytes, want at least 64", stats.LastOBJTileBytes)
	}

	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 0, 0, 10, 20, false, false)
	c.PresentRender()
	if stats := c.RendererUploadStats(); stats.LastOBJTileBytes != 0 {
		t.Fatalf("unchanged pose re-uploaded %d OBJ bytes", stats.LastOBJTileBytes)
	}
}

func TestMode0PSprBackgroundMutationPreservesResidentPose(t *testing.T) {
	requireMode0LayerCount(t, 1)
	c := mode0PsprFixture(t)
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 0, 0, 10, 20, false, false)
	c.PrepareRender()
	c.PresentRender()

	c.Sset(0, 0, 7)
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 0, 0, 10, 20, false, false)
	c.PrepareRender()
	c.PresentRender()
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM)); got != 11 {
		t.Fatalf("background mutation replaced resident pose pixel: got %d, want 11", got)
	}
	if got := c.Sget(0, 0); got != 7 {
		t.Fatalf("background mutation was lost: got %d, want 7", got)
	}
}

func TestMode0PSprMutableSheetSourceRefreshesRelocatedPose(t *testing.T) {
	c := mode0PsprFixture(t)
	c.cart.ObjSpriteN = 0
	c.cart.PoseOBJStageBase = 400
	c.cart.PoseOBJStage = make([]uint32, 16)
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 400, 0, 10, 20, false, false)
	c.PrepareRender()
	c.PresentRender()
	c.Sset(0, 0, 7)
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 400, 0, 10, 20, false, false)
	c.PrepareRender()
	c.PresentRender()
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM + 400*64)); got != 7 {
		t.Fatalf("relocated pose did not refresh its mutated source: got %d, want 7", got)
	}
}

func TestMode0PSprFlipXMatchesMode4Pixels(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	cell0 := make([]byte, 64)
	cell1 := make([]byte, 64)
	cell0[0] = 7
	cell1[1] = 9
	compact := append(cell0, cell1...)
	image := AssetImage{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		Sheet: make([]byte, 64), MapW: 1, MapH: 1, Map: []uint16{0}, Flags: []byte{0},
		PoseIndex: []PoseRange{{Start: 0, Count: 2, FlipOffsetX: 16, FlipOffsetY: 8}},
		PoseParts: []PosePart{
			{Tile: 0, X: 0, Y: 0},
			{Tile: 1, X: 8, Y: 0, Flags: PoseFlipX},
		},
		ObjSheetW: 16, ObjSheetH: 8, ObjSpriteCols: 2, ObjSpriteRows: 1, ObjSpriteN: 2,
		ObjSheet: compact, ObjSheetIndex: []uint32{1, 2},
	}
	ram := &AssetMemory{
		SheetW: 8, SheetH: 8, SpriteCols: 1, SpriteRows: 1, SpriteN: 1,
		MapW: 1, MapH: 1,
		Sheet: make([]byte, 64), Map: make([]uint16, 1), Flags: make([]byte, 1),
	}
	var mode0 Context
	if !mode0.Init(&image, ram) {
		t.Fatal("mode0 init")
	}
	mode0.InitHardware()
	mode0.BeginRenderFrame()
	mode0.Cls(0)
	mode0.PSprSite(0, 0, 0, 10, 20, true, false)
	mode0.PresentRender()

	want := composeMode0Objects(t)
	if want[20*ScreenWidth+25] != 7 {
		t.Fatalf("composed flipX pixel at (25,20) = %d, want 7", want[20*ScreenWidth+25])
	}
	flipped := false
	for slot := 0; slot < 128; slot++ {
		attr0 := hw.GetReg16(hw.OAM + uintptr(slot*8))
		if attr0&mode0ObjectDisable != 0 {
			continue
		}
		attr1 := hw.GetReg16(hw.OAM + uintptr(slot*8+2))
		if attr1&(1<<12) != 0 {
			flipped = true
		}
	}
	if !flipped {
		t.Fatal("no OAM entry had flipX set")
	}
}

func composeMode0Objects(t *testing.T) []byte {
	t.Helper()
	screen := make([]byte, ScreenWidth*ScreenHeight)
	for slot := 127; slot >= 0; slot-- {
		attr0 := hw.GetReg16(hw.OAM + uintptr(slot*8))
		if attr0&mode0ObjectDisable != 0 {
			continue
		}
		attr1 := hw.GetReg16(hw.OAM + uintptr(slot*8+2))
		attr2 := hw.GetReg16(hw.OAM + uintptr(slot*8+4))
		y := int(attr0 & 0xff)
		x := int(attr1 & 0x1ff)
		if y >= ScreenHeight {
			y -= 256
		}
		if x >= ScreenWidth {
			x -= 512
		}
		dimensions := [3][4][2]int{
			{{8, 8}, {16, 16}, {32, 32}, {64, 64}},
			{{16, 8}, {32, 8}, {32, 16}, {64, 32}},
			{{8, 16}, {8, 32}, {16, 32}, {32, 64}},
		}
		shape, size := int(attr0>>14), int(attr1>>14)
		if shape >= len(dimensions) {
			continue
		}
		width, height := dimensions[shape][size][0], dimensions[shape][size][1]
		flipX := attr1&(1<<12) != 0
		flipY := attr1&(1<<13) != 0
		tile := int(attr2&0x3ff) / 2
		for py := 0; py < height; py++ {
			for px := 0; px < width; px++ {
				sx, sy := px, py
				if flipX {
					sx = width - 1 - px
				}
				if flipY {
					sy = height - 1 - py
				}
				cell := tile + (sy/8)*(width/8) + sx/8
				address := hw.OBJVRAM + uintptr(cell*64+(sy&7)*8+(sx&7))
				color := byte(hw.ReadVRAM32(address&^3) >> ((address & 3) * 8))
				if color == 0 {
					continue
				}
				dx, dy := x+px, y+py
				if dx < 0 || dy < 0 || dx >= ScreenWidth || dy >= ScreenHeight {
					continue
				}
				screen[dy*ScreenWidth+dx] = color
			}
		}
	}
	return screen
}

func TestMode0PSprDMABenchOnePoseChange(t *testing.T) {
	requireMode0LayerCount(t, 1)
	c := mode0PsprFixture(t)
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 0, 0, 10, 10, false, false)
	c.PresentRender()
	changed := c.RendererUploadStats().LastOBJTileBytes
	if changed != 64 {
		t.Fatalf("one unique-tile pose change uploaded %d bytes, want 64", changed)
	}
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 0, 0, 10, 10, false, false)
	c.PresentRender()
	if got := c.RendererUploadStats().LastOBJTileBytes; got != 0 {
		t.Fatalf("steady pose uploaded %d bytes", got)
	}
}

func plannedMode0PoseFixture(t *testing.T, parts []PosePart) *Context {
	t.Helper()
	c := mode0PsprFixture(t)
	const cells = 128
	c.cart.ObjSheet = make([]byte, cells*64)
	c.cart.ObjSheetIndex = make([]uint32, cells)
	c.cart.ObjSpriteN = cells
	for tile := 0; tile < cells; tile++ {
		c.cart.ObjSheetIndex[tile] = uint32(tile + 1)
		for pixel := 0; pixel < 64; pixel++ {
			if pixel%7 != 0 {
				c.cart.ObjSheet[tile*64+pixel] = byte((tile*13+pixel)%254 + 1)
			}
		}
	}
	objects, tiles := BuildPoseOBJPlan(parts)
	c.cart.PoseParts = parts
	c.cart.PoseIndex = []PoseRange{poseRangeForTest(0, parts)}
	c.cart.PoseOBJParts, c.cart.PoseOBJTiles = objects, tiles
	c.cart.PoseOBJIndex = []PoseOBJRange{{Count: uint32(len(objects)), TileCount: uint32(len(tiles))}}
	return c
}

// Independently paint the authored cells instead of reusing the object plan.
func authoredPosePixels(c *Context, parts []PosePart, x, y int, flipX, flipY bool) []byte {
	want := make([]byte, ScreenWidth*ScreenHeight)
	bounds := poseRangeForTest(0, parts)
	if flipX {
		x += int(bounds.FlipOffsetX)
	}
	if flipY {
		y += int(bounds.FlipOffsetY)
	}
	for _, part := range parts {
		ox, oy := int(part.X), int(part.Y)
		fx, fy := part.Flags&PoseFlipX != 0, part.Flags&PoseFlipY != 0
		if flipX {
			ox, fx = -ox-8, !fx
		}
		if flipY {
			oy, fy = -oy-8, !fy
		}
		for py := 0; py < 8; py++ {
			for px := 0; px < 8; px++ {
				sx, sy := px, py
				if fx {
					sx = 7 - sx
				}
				if fy {
					sy = 7 - sy
				}
				color := c.cart.ObjSheet[int(part.Tile)*64+sy*8+sx]
				dx, dy := x-c.camX+ox+px, y-c.camY+oy+py
				if color != 0 && dx >= 0 && dx < ScreenWidth && dy >= 0 && dy < ScreenHeight {
					want[dy*ScreenWidth+dx] = color
				}
			}
		}
	}
	return want
}

func TestMode0PSprPackedPixelParity(t *testing.T) {
	shapes := [][2]int{{8, 8}, {8, 4}, {4, 8}, {4, 4}, {4, 2}, {2, 4}, {4, 1}, {1, 4}, {2, 2}, {2, 1}, {1, 2}, {1, 1}, {5, 10}}
	for _, shape := range shapes {
		for flip := 0; flip < 4; flip++ {
			t.Run(fmt.Sprintf("%dx%d-flip%d", shape[0]*8, shape[1]*8, flip), func(t *testing.T) {
				parts := rectangularPose(shape[0], shape[1])
				c := plannedMode0PoseFixture(t, parts)
				c.Camera(3, 5)
				c.BeginRenderFrame()
				c.Cls(0)
				x, y := ScreenWidth-35, ScreenHeight-37 // Larger objects cross a screen edge in either orientation.
				c.PSprSite(7, 4, 0, x, y, flip&1 != 0, flip&2 != 0)
				if c.mode0ObjectCount > len(c.cart.PoseOBJParts) {
					t.Fatal("plan emitted extra objects")
				}
				c.PrepareRender()
				c.PresentRender()
				got, want := composeMode0Objects(t), authoredPosePixels(c, parts, x, y, flip&1 != 0, flip&2 != 0)
				for i := range want {
					if got[i] != want[i] {
						t.Fatalf("pixel (%d,%d)=%d want %d", i%ScreenWidth, i/ScreenWidth, got[i], want[i])
					}
				}
			})
		}
	}
	for _, parts := range [][]PosePart{
		{{Tile: 1}, {Tile: 2, X: 4}, {Tile: 3, X: 8, Flags: PoseFlipX}},
		{{Tile: 1}, {Tile: 2, X: 8, Flags: PoseFlipX | PoseFlipY}, {Tile: 3, Y: 8}, {Tile: 4, X: 8, Y: 8}},
	} {
		for flip := 0; flip < 4; flip++ {
			c := plannedMode0PoseFixture(t, parts)
			c.BeginRenderFrame()
			c.Cls(0)
			c.PSprSite(0, 0, 0, 40, 40, flip&1 != 0, flip&2 != 0)
			c.PrepareRender()
			c.PresentRender()
			got, want := composeMode0Objects(t), authoredPosePixels(c, parts, 40, 40, flip&1 != 0, flip&2 != 0)
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("overlap/authored flip pixel%d=%d want%d", i, got[i], want[i])
				}
			}
		}
	}
}

func TestMode0PSprManySitesMixWithResidentSprites(t *testing.T) {
	c := plannedMode0PoseFixture(t, rectangularPose(8, 8))
	c.cart.PoseOBJStageBase = 1
	c.cart.PoseOBJStage = make([]uint32, 3*64*16)
	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(0, 0, 0, 1, 1, false, false)
	for i := 0; i < 3; i++ {
		c.PSprSiteFromBase(0, 1, 64, 0, 70+i*65, 70, false, false)
	}
	if c.mode0ObjectCount != 4 {
		t.Fatalf("mixed scene used%d objects, want4", c.mode0ObjectCount)
	}
	c.PrepareRender()
	c.PresentRender()
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM)); got != 3 {
		t.Fatalf("stream overwrote static sprite: %d", got)
	}
	for i := 0; i < 3; i++ {
		if c.mode0PsprLastID[i] != 0 {
			t.Fatalf("site%d was not uploaded", i)
		}
	}
	if got := c.RendererUploadStats().LastOBJTileBytes; got != 3*64*64 {
		t.Fatalf("uploaded%d bytes, want%d", got, 3*64*64)
	}
}

func TestMode0PSprCoalescedUploadPreservesUnchangedMiddlePose(t *testing.T) {
	parts := rectangularPose(8, 8)
	c := plannedMode0PoseFixture(t, parts)
	other := append([]PosePart(nil), parts...)
	for i := range other {
		other[i].Tile += 64
	}
	objects, tiles := BuildPoseOBJPlan(other)
	c.cart.PoseIndex = append(c.cart.PoseIndex, PoseRange{Start: uint32(len(parts)), Count: uint16(len(other))})
	c.cart.PoseParts = append(c.cart.PoseParts, other...)
	c.cart.PoseOBJIndex = append(c.cart.PoseOBJIndex, PoseOBJRange{Start: uint32(len(c.cart.PoseOBJParts)), Count: uint32(len(objects)), TileStart: uint32(len(c.cart.PoseOBJTiles)), TileCount: uint32(len(tiles))})
	c.cart.PoseOBJParts = append(c.cart.PoseOBJParts, objects...)
	c.cart.PoseOBJTiles = append(c.cart.PoseOBJTiles, tiles...)
	c.cart.PoseOBJStageBase = 1
	c.cart.PoseOBJStage = make([]uint32, 3*64*16)
	for frame := 0; frame < 2; frame++ {
		c.BeginRenderFrame()
		c.Cls(0)
		for site := 0; site < 3; site++ {
			id := 0
			if frame == 1 && site != 1 {
				id = 1
			}
			c.PSprSite(site, 1+site*64, id, 10+site*65, 10, false, false)
		}
		c.PrepareRender()
		c.PresentRender()
	}
	for site := 0; site < 3; site++ {
		address := hw.OBJVRAM + uintptr((1+site*64)*64)
		got := byte(hw.ReadVRAM32(address) >> 8)
		tile := 64
		if site == 1 {
			tile = 0
		}
		if want := c.cart.ObjSheet[tile*64+1]; got != want {
			t.Fatalf("site%d pixel=%d want%d", site, got, want)
		}
	}
}

func TestMode0PSprCommitReservesUploadWindow(t *testing.T) {
	c := plannedMode0PoseFixture(t, rectangularPose(8, 8))
	c.mode0ColdCycles = 0
	c.paletteDirty = false
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 0, 0, 0, 0, false, false)
	if !c.hasMode0PsprUploads() || c.mode0RequiredCommitCycles() != mode0CommitReserve {
		t.Fatal("pending pose uploads need the full commit window")
	}
	c.PrepareRender()
	if !c.hasMode0PsprUploads() {
		t.Fatal("preparation lost the pending VRAM transfer")
	}
	c.PresentRender()
	if c.hasMode0PsprUploads() {
		t.Fatal("completed pose upload remains pending")
	}
}

func TestMode0PSprPreparedUploadSurvivesSkippedPresent(t *testing.T) {
	c := mode0PsprFixture(t)
	if len(c.mode0PoseStageWords()) == 0 {
		t.Skip("requires staged pose backend")
	}
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 0, 0, 10, 20, false, false)
	c.PrepareRender()
	if !c.hasMode0PsprUploads() {
		t.Fatal("prepared upload is not pending")
	}
	// Start another draw and repeat preparation before either frame presents.
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 0, 0, 20, 20, false, false)
	c.PrepareRender()
	c.PresentRender()
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM)); got != 11 {
		t.Fatalf("skipped present lost upload: %d", got)
	}
	if c.hasMode0PsprUploads() {
		t.Fatal("completed transfer remains pending")
	}
}

func TestMode0PSprReprepareUsesLatestMutablePixels(t *testing.T) {
	c := mode0PsprFixture(t)
	if len(c.mode0PoseStageWords()) == 0 {
		t.Skip("requires staged pose backend")
	}
	c.cart.ObjSpriteN = 0
	c.BeginRenderFrame()
	c.Cls(0)
	c.PSprSite(0, 400, 0, 10, 20, false, false)
	c.PrepareRender()
	c.Sset(0, 0, 7)
	c.PrepareRender()
	c.PresentRender()
	if got := byte(hw.ReadVRAM32(hw.OBJVRAM + 400*64)); got != 7 {
		t.Fatalf("reprepare retained stale pixels: %d", got)
	}
}
