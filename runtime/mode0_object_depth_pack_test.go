//go:build mtile_mode0 && mtile_mode0_layers2 && mtile_mode0_obj && mtile_mode0_objpack

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0PackedObjectKeepsDerivedBackgroundDepth(t *testing.T) {
	requireMode0LayerCount(t, 2)
	hw.ResetForTest()
	c := newMode0TestContext(t, 240, 24, 1, 1, make([]byte, 240*24), []uint16{1, 1}, make([]byte, 90))

	c.BeginRenderFrame()
	c.Cls(0)
	c.MapBGAt(0, 0, 0, 0, 0, 0, 1, 1, 0)
	c.Spr(32, 24, 24, 2, 2, false, false)
	c.MapBGAt(1, 1, 0, 0, 0, 0, 1, 1, 0)
	c.PresentRender()

	if c.mode0ObjectCount != 1 {
		t.Fatalf("packed object count = %d, want one", c.mode0ObjectCount)
	}
	want := uint16(mode0PackedObjectTileBase*2) | 1<<10
	if got := hw.GetReg16(hw.OAM + 4); got != want {
		t.Fatalf("packed attr2 = %#04x, want tile %d at priority 1 (%#04x)", got, mode0PackedObjectTileBase, want)
	}
}
