package runtime

import "testing"

func TestMapPlaneSlotTableCoversEverySparseMask(t *testing.T) {
	for mask := uint8(0); mask < 16; mask++ {
		wantSlot := 0
		for layer := 0; layer < 4; layer++ {
			got, present := mapPlaneSlot(mask, layer, 4)
			wantPresent := mask&(1<<uint(layer)) != 0
			if present != wantPresent {
				t.Fatalf("mask %#x layer %d present = %t, want %t", mask, layer, present, wantPresent)
			}
			if present {
				if got != wantSlot {
					t.Fatalf("mask %#x layer %d slot = %d, want %d", mask, layer, got, wantSlot)
				}
				wantSlot++
			}
		}
		if got := mapMaskPopulation(mask); got != wantSlot {
			t.Fatalf("mask %#x population = %d, want %d", mask, got, wantSlot)
		}
	}
	if _, ok := mapPlaneSlot(0x10, 0, 4); ok {
		t.Fatal("out-of-contract high mask bit unexpectedly resolved")
	}
}

func TestLayeredMgetMsetCompactSparsePlanes(t *testing.T) {
	const mapW, mapH, layers = 2, 2, 4
	image := &AssetImage{
		SheetW: 80, SheetH: 8, SpriteCols: 10, SpriteRows: 1, SpriteN: 10,
		MapW: mapW, MapH: mapH, MapLayers: layers, MapPlaneMask: 0x0b,
		Sheet: make([]byte, 80*8),
		// Compact storage order is logical BG0, BG1, BG3.
		Map:   []uint16{1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3},
		Flags: make([]byte, 10),
	}
	ram := &AssetMemory{
		SheetW: 80, SheetH: 8, SpriteCols: 10, SpriteRows: 1, SpriteN: 10,
		MapW: mapW, MapH: mapH, MapLayers: layers, MapPlaneMask: 0x0b,
		Sheet: make([]byte, 80*8), Map: make([]uint16, 12), Flags: make([]byte, 10),
	}
	var c Context
	if !c.Init(image, ram) {
		t.Fatal("Init rejected compact layered map")
	}

	if got := c.Mget(1, 1); got != 1 {
		t.Fatalf("legacy BG0 mget = %d, want 1", got)
	}
	for _, test := range []struct {
		bg, want int
	}{{-1, 0}, {0, 1}, {1, 2}, {2, 0}, {3, 3}, {4, 0}} {
		if got := int(c.MgetLayer(test.bg, 1, 1)); got != test.want {
			t.Fatalf("mget BG%d = %d, want %d", test.bg, got, test.want)
		}
	}

	c.MsetLayer(1, 1, 1, 7)
	if got := c.MgetLayer(1, 1, 1); got != 7 {
		t.Fatalf("BG1 after mset = %d, want 7", got)
	}
	if got := c.Mget(1, 1); got != 1 {
		t.Fatalf("BG1 mset changed legacy BG0: %d", got)
	}
	if got := c.MgetLayer(3, 1, 1); got != 3 {
		t.Fatalf("BG1 mset changed BG3: %d", got)
	}

	c.MsetLayer(3, 0, 0, 8)
	if got := c.MgetLayer(3, 0, 0); got != 8 {
		t.Fatalf("BG3 after mset = %d, want 8", got)
	}
	c.MsetLayer(2, 0, 0, 9) // Absent plane fails closed.
	if got := c.MgetLayer(2, 0, 0); got != 0 {
		t.Fatalf("absent BG2 after mset = %d, want 0", got)
	}
	c.MsetLayer(1, -1, 0, 9)
	c.MsetLayer(1, 0, 0, 10)
	if got := c.MgetLayer(1, 0, 0); got != 2 {
		t.Fatalf("invalid layered writes changed BG1: %d", got)
	}

	c.Mset(0, 0, 6)
	if got := c.MgetLayer(0, 0, 0); got != 6 {
		t.Fatalf("legacy mset no longer writes BG0: %d", got)
	}
	c.MsetLayer(0, 0, 0, 5)
	if got := c.Mget(0, 0); got != 5 {
		t.Fatalf("layered BG0 write differs from legacy mget: %d", got)
	}
}

func TestLayeredMsetStreamingSurvivesCrossPlaneEviction(t *testing.T) {
	const mapW, mapH, layers = 96, 80, 4
	const planeCells = mapW * mapH
	mapROM := make([]uint16, planeCells*2) // Compact BG1, BG3.
	for i := 0; i < planeCells; i++ {
		mapROM[i] = 1
		mapROM[planeCells+i] = 2
	}
	image := &AssetImage{
		SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1, SpriteN: 4,
		MapW: mapW, MapH: mapH, MapLayers: layers, MapPlaneMask: 0x0a, MapStreaming: true,
		Sheet: make([]byte, 32*8), Map: mapROM, Flags: make([]byte, 4),
	}
	ram := &AssetMemory{
		SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1, SpriteN: 4,
		MapW: mapW, MapH: mapH, MapLayers: layers, MapPlaneMask: 0x0a, MapStreaming: true,
		Sheet: make([]byte, 32*8), Map: make([]uint16, MapCacheCells), Flags: make([]byte, 4),
		MapChunks: make([]MapChunk, MapCacheSlots), MapEdits: make([]MapEdit, MapEditCapacity),
	}
	var c Context
	if !c.Init(image, ram) {
		t.Fatal("Init rejected streaming layered map")
	}

	c.MsetLayer(1, 1, 1, 3)
	c.MsetLayer(3, 1, 1, 0)
	if got := c.MgetLayer(1, 1, 1); got != 3 {
		t.Fatalf("initial BG1 edit = %d, want 3", got)
	}
	if got := c.MgetLayer(3, 1, 1); got != 0 {
		t.Fatalf("initial BG3 zero edit = %d, want 0", got)
	}

	// More than four distinct logical-layer/chunk keys evicts both edited
	// chunks from the four-slot cache. The sparse overlay remains authoritative.
	for _, p := range [][3]int{{1, 40, 1}, {1, 72, 1}, {1, 1, 40}, {3, 40, 40}, {3, 72, 40}, {3, 40, 72}} {
		_ = c.MgetLayer(p[0], p[1], p[2])
	}
	if got := c.MgetLayer(1, 1, 1); got != 3 {
		t.Fatalf("evicted BG1 edit = %d, want 3", got)
	}
	if got := c.MgetLayer(3, 1, 1); got != 0 {
		t.Fatalf("evicted BG3 zero edit = %d, want 0", got)
	}
	if c.LastMapError() != MapOK {
		t.Fatalf("layered streaming edits set map error %d", c.LastMapError())
	}
}
