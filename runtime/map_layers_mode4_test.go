//go:build !mtile_mode0

package runtime

import "testing"

func TestLayeredMsetIsVisibleToLaterMode4MapPass(t *testing.T) {
	sheet := make([]byte, 24*8)
	for x := 8; x < 16; x++ {
		sheet[x] = 4
	}
	for x := 16; x < 24; x++ {
		sheet[x] = 9
	}
	image := &AssetImage{
		SheetW: 24, SheetH: 8, SpriteCols: 3, SpriteRows: 1, SpriteN: 3,
		MapW: 1, MapH: 1, MapLayers: 2, MapPlaneMask: 0x02,
		Sheet: sheet, Map: []uint16{1}, Flags: make([]byte, 3),
	}
	ram := &AssetMemory{
		SheetW: 24, SheetH: 8, SpriteCols: 3, SpriteRows: 1, SpriteN: 3,
		MapW: 1, MapH: 1, MapLayers: 2, MapPlaneMask: 0x02,
		Sheet: make([]byte, len(sheet)), Map: make([]uint16, 1), Flags: make([]byte, 3),
	}
	var c Context
	if !c.Init(image, ram) {
		t.Fatal("Init rejected visible layered mutation fixture")
	}
	c.Cls(0)
	c.MapBG(1, 0, 0, 0, 0, 1, 1, 0)
	if got := c.Pget(0, 0); got != 4 {
		t.Fatalf("BG1 before mutation pixel = %d, want 4", got)
	}
	c.MsetLayer(1, 0, 0, 2)
	c.Cls(0)
	c.MapBG(1, 0, 0, 0, 0, 1, 1, 0)
	if got := c.Pget(0, 0); got != 9 {
		t.Fatalf("BG1 after mutation pixel = %d, want 9", got)
	}
}
