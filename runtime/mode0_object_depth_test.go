//go:build mtile_mode0 && mtile_mode0_layers4 && mtile_mode0_obj

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0ObjectDepthFollowsInterleavedMapPainterOrder(t *testing.T) {
	requireMode0LayerCount(t, 4)
	hw.ResetForTest()
	c := newMode0TestContext(t, 40, 8, 1, 1, make([]byte, 40*8), []uint16{1, 1, 1, 1}, make([]byte, 5))

	c.BeginRenderFrame()
	c.Cls(0)
	for layer := 0; layer < 4; layer++ {
		c.MapBGAt(layer, layer, 0, 0, 0, 0, 1, 1, 0)
		c.Spr(layer+1, 16+layer*16, 24, 1, 1, false, false)
	}
	c.PresentRender()

	for entry := 0; entry < 4; entry++ {
		// OAM painter order is reversed: the last/front sprite is entry 0.
		logicalID := 4 - entry
		wantPriority := entry
		want := uint16(logicalID*2) | uint16(wantPriority)<<10
		if got := hw.GetReg16(hw.OAM + uintptr(entry*8+4)); got != want {
			t.Fatalf("OAM%d attr2 = %#04x, want tile %d with priority %d (%#04x)", entry, got, logicalID, wantPriority, want)
		}
	}
}
