//go:build mtile_mode0 && mtile_mode0_layers3

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0ThreeLayersPackSparseLogicalBackgrounds(t *testing.T) {
	hw.ResetForTest()
	image := AssetImage{
		SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1, SpriteN: 4,
		MapW: 1, MapH: 1, MapLayers: 4, MapPlaneMask: 0x0d,
		Sheet: make([]byte, 32*8), Map: []uint16{1, 2, 3}, Flags: make([]byte, 4),
	}
	ram := &AssetMemory{
		SheetW: 32, SheetH: 8, SpriteCols: 4, SpriteRows: 1, SpriteN: 4,
		MapW: 1, MapH: 1, MapLayers: 4, MapPlaneMask: 0x0d,
		Sheet: make([]byte, 32*8), Map: make([]uint16, 3), Flags: make([]byte, 4),
	}
	var c Context
	if !c.Init(&image, ram) {
		t.Fatal("Init rejected compact BG0/BG2/BG3 map")
	}
	c.InitHardware()
	c.BeginRenderFrame()
	c.Cls(0)
	c.MapAll(0x0d)
	c.PresentRender()

	if got, want := hw.GetReg16(hw.DISPCNT), mode0LayerDisplay(0x07); got != want {
		t.Fatalf("DISPCNT = %#04x, want three packed BGs/optional-OBJ display %#04x", got, want)
	}
	wantSources := [3]uint8{0, 2, 3}
	for slot := 0; slot < 3; slot++ {
		if got := hw.GetReg16(hw.BG0CNT + uintptr(slot*2)); got != mode0LayerControl(slot) {
			t.Fatalf("slot %d control = %#04x, want %#04x", slot, got, mode0LayerControl(slot))
		}
		if got := hw.GetReg16(mode0LayerScreenBase(slot)); got != uint16(slot+1) {
			t.Fatalf("slot %d entry = %d, want %d", slot, got, slot+1)
		}
		if got := c.mode0MapSource[slot]; got != wantSources[slot] {
			t.Fatalf("slot %d source = BG%d, want BG%d", slot, got, wantSources[slot])
		}
	}
}
