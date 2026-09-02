//go:build mtile_mode0 && mtile_mode0_layers4

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

// The low-level positional Map helper is retained for runtime characterization:
// first through fourth calls own physical slots BG0 through BG3. Generated Lua
// map calls use MapBGAt/MapAll and densely assign logical planes instead.
func TestMode0FourLayersUseIndependentBackgroundsAndScroll(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(
		t,
		40, 8,
		4, 1,
		make([]byte, 40*8),
		[]uint16{1, 2, 3, 4},
		make([]byte, 5),
	)

	draw := func(scroll [4]int) {
		c.BeginRenderFrame()
		c.Cls(0)
		for layer := 0; layer < 4; layer++ {
			c.Camera(scroll[layer], 0)
			// Cancel the source-region world offset so every one-cell region
			// occupies the same screen position on its own hardware BG.
			c.Map(layer, 0, -layer*8, 0, 1, 1, 0)
		}
		c.PresentRender()
	}

	draw([4]int{1, 2, 3, 4})
	if got, want := hw.GetReg16(hw.DISPCNT), mode0LayerDisplay(0x0f); got != want {
		t.Fatalf("DISPCNT = %#04x, want BG0+BG1+BG2+BG3/optional-OBJ display %#04x", got, want)
	}
	for layer := 0; layer < 4; layer++ {
		control := hw.GetReg16(hw.BG0CNT + uintptr(layer*2))
		if want := mode0LayerControl(layer); control != want {
			t.Fatalf("BG%dCNT = %#04x, want %#04x", layer, control, want)
		}
		if priority := int(control & 3); priority != 3-layer {
			t.Fatalf("BG%d priority = %d, want %d", layer, priority, 3-layer)
		}
		if got := hw.GetReg16(mode0LayerScreenBase(layer)); got != uint16(layer+1) {
			t.Fatalf("BG%d entry = %d, want tile %d", layer, got, layer+1)
		}
		if got := hw.GetReg16(hw.BG0HOFS + uintptr(layer*4)); got != uint16(layer+1) {
			t.Fatalf("BG%dHOFS = %d, want %d", layer, got, layer+1)
		}
	}
	if stats := c.RendererUploadStats(); stats.LastMapBytes != 4*4096 {
		t.Fatalf("four-layer upload = %+v, want %d map bytes", stats, 4*4096)
	}

	// Independent sub-tile parallax is register-only. No layer crosses an
	// eight-pixel boundary, so none of the four screen shadows is uploaded.
	draw([4]int{2, 3, 4, 5})
	if stats := c.RendererUploadStats(); stats.LastMapBytes != 0 {
		t.Fatalf("sub-tile parallax uploaded map data: %+v", stats)
	}
	for layer := 0; layer < 4; layer++ {
		if got := hw.GetReg16(hw.BG0HOFS + uintptr(layer*4)); got != uint16(layer+2) {
			t.Fatalf("updated BG%dHOFS = %d, want %d", layer, got, layer+2)
		}
	}
}

func TestMode0MapBGAtReadsSparseLogicalPlanesIntoDenseSlots(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(
		t,
		40, 8,
		1, 1,
		make([]byte, 40*8),
		[]uint16{1, 2, 3, 4},
		make([]byte, 5),
	)
	c.BeginRenderFrame()
	c.Cls(0)
	c.MapBGAt(0, 0, 0, 0, 0, 0, 1, 1, 0)
	c.MapBGAt(1, 3, 0, 0, 0, 0, 1, 1, 0)
	c.PresentRender()
	if got, want := hw.GetReg16(hw.DISPCNT), mode0LayerDisplay(0x03); got != want {
		t.Fatalf("DISPCNT = %#04x, want two dense BGs/optional-OBJ display %#04x", got, want)
	}
	if got := hw.GetReg16(mode0LayerScreenBase(0)); got != 1 {
		t.Fatalf("BG0 entry = %d, want authored BG0 tile 1", got)
	}
	if got := hw.GetReg16(mode0LayerScreenBase(1)); got != 4 {
		t.Fatalf("dense slot 1 entry = %d, want authored BG3 tile 4", got)
	}
}
