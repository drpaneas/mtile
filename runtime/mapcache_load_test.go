package runtime

import "testing"

// referenceChunkLoad is the original per-cell chunk loader. The row-copy
// implementation must produce exactly the same cache contents.
func referenceChunkLoad(c *Context, layer, chunkX, chunkY int) []uint16 {
	storageLayer, _ := mapPlaneSlot(c.cart.MapPlaneMask, layer, c.cart.MapLayers)
	out := make([]uint16, MapChunkCells)
	for localY := 0; localY < MapChunkHeight; localY++ {
		mapY := chunkY*MapChunkHeight + localY
		for localX := 0; localX < MapChunkWidth; localX++ {
			mapX := chunkX*MapChunkWidth + localX
			value := uint16(0)
			if mapX >= 0 && mapY >= 0 && mapX < c.cart.MapW && mapY < c.cart.MapH {
				index, _ := mapLayerCellIndex(storageLayer, mapX, mapY, c.cart.MapW, c.cart.MapH, mapMaskPopulation(c.cart.MapPlaneMask))
				value = c.mapROM[int(index)]
				if edited, found := c.mapEditLookup(index); found {
					value = edited
				}
			}
			out[localY*MapChunkWidth+localX] = value
		}
	}
	return out
}

func TestStreamingChunkLoadMatchesPerCellReference(t *testing.T) {
	// Dimensions are not chunk multiples, so edge chunks are partly outside
	// the map horizontally and vertically.
	const width, height, layers = 70, 45, 3
	planeCells := width * height
	mapROM := make([]uint16, planeCells*layers)
	for i := range mapROM {
		mapROM[i] = uint16(1 + i%4)
	}
	image := &AssetImage{
		SheetW: 40, SheetH: 8, SpriteCols: 5, SpriteRows: 1, SpriteN: 5,
		MapW: width, MapH: height, MapLayers: layers, MapStreaming: true,
		Sheet: make([]byte, 40*8), Map: mapROM, Flags: make([]byte, 5),
	}
	ram := &AssetMemory{
		SheetW: 40, SheetH: 8, SpriteCols: 5, SpriteRows: 1, SpriteN: 5,
		MapW: width, MapH: height, MapLayers: layers, MapStreaming: true,
		Sheet: make([]byte, 40*8), Map: make([]uint16, MapCacheCells), Flags: make([]byte, 5),
		MapChunks: make([]MapChunk, MapCacheSlots), MapEdits: make([]MapEdit, MapEditCapacity),
	}
	var c Context
	if !c.Init(image, ram) {
		t.Fatal("streaming Init returned false")
	}
	check := func(stage string) {
		t.Helper()
		for layer := 0; layer < layers; layer++ {
			for chunkY := 0; chunkY <= height/MapChunkHeight+1; chunkY++ {
				for chunkX := 0; chunkX <= width/MapChunkWidth+1; chunkX++ {
					want := referenceChunkLoad(&c, layer, chunkX, chunkY)
					slot := c.loadMapLayerChunk(layer, chunkX, chunkY)
					if slot < 0 {
						t.Fatalf("%s: BG%d chunk (%d,%d) did not load", stage, layer, chunkX, chunkY)
					}
					got := c.cart.Map[slot*MapChunkCells : (slot+1)*MapChunkCells]
					for i := range want {
						if got[i] != want[i] {
							t.Fatalf("%s: BG%d chunk (%d,%d) cell (%d,%d) = %d, want %d",
								stage, layer, chunkX, chunkY, i%MapChunkWidth, i/MapChunkWidth, got[i], want[i])
						}
					}
				}
			}
		}
	}
	check("ROM only")
	// Edits on every layer, including zero values, chunk seams, and map edges.
	for _, edit := range []struct{ layer, x, y, id int }{
		{0, 0, 0, 0}, {0, 31, 0, 3}, {0, 32, 0, 2}, {1, 69, 44, 0},
		{1, 33, 31, 4}, {2, 64, 32, 1}, {2, 0, 44, 3},
	} {
		c.MsetLayer(edit.layer, edit.x, edit.y, edit.id)
	}
	if c.mapEditCount == 0 {
		t.Fatal("edits did not reach the sparse overlay")
	}
	check("with edits")
}
