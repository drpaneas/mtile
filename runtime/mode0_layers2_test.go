//go:build mtile_mode0 && mtile_mode0_layers2

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0TwoLayersUseSeparateScreenBlocksAndPainterPriority(t *testing.T) {
	hw.ResetForTest()
	mapData := []uint16{1, 0, 2, 0}
	c := newMode0TestContext(t, 24, 8, 4, 1, make([]byte, 24*8), mapData, []byte{0, 1, 2})
	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 2, 1, 1)
	c.Map(2, 0, -16, 0, 2, 1, 2)
	c.PresentRender()

	if got := hw.GetReg16(hw.DISPCNT); got != 0x0300 {
		t.Fatalf("DISPCNT = %#04x, want BG0+BG1", got)
	}
	if got := hw.GetReg16(hw.BG0CNT); got != mode0LayerControl(0) || got&3 != 1 {
		t.Fatalf("BG0CNT = %#04x, want back priority 1", got)
	}
	if got := hw.GetReg16(hw.BG1CNT); got != mode0LayerControl(1) || got&3 != 0 {
		t.Fatalf("BG1CNT = %#04x, want front priority 0", got)
	}
	if got := hw.GetReg16(mode0LayerScreenBase(0)); got != 1 {
		t.Fatalf("back layer entry = %d, want 1", got)
	}
	if got := hw.GetReg16(mode0LayerScreenBase(1)); got != 2 {
		t.Fatalf("front layer entry = %d, want 2", got)
	}
	if stats := c.RendererUploadStats(); stats.LastMapBytes != 8192 {
		t.Fatalf("two-layer upload = %+v, want 8192 map bytes", stats)
	}

	c.BeginRenderFrame()
	c.Cls(0)
	c.Map(0, 0, 0, 0, 2, 1, 1)
	c.Map(2, 0, -16, 0, 2, 1, 2)
	c.PresentRender()
	if stats := c.RendererUploadStats(); stats.LastMapBytes != 0 {
		t.Fatalf("stable layers uploaded map data: %+v", stats)
	}
}

func TestMode0MutationsPatchOnlyAffectedLayerShadows(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{1}, []byte{0, 0})
	draw := func() {
		c.BeginRenderFrame()
		c.Cls(0)
		c.Map(0, 0, 0, 0, 1, 1, 1)
		c.Map(0, 0, 0, 0, 1, 1, 2)
		c.PresentRender()
	}
	draw()

	c.Fset(1, 0, true)
	draw()
	if got := hw.GetReg16(mode0LayerScreenBase(0)); got != 1 {
		t.Fatalf("flag-0 layer entry = %d, want 1", got)
	}
	if got := hw.GetReg16(mode0LayerScreenBase(1)); got != 0 {
		t.Fatalf("flag-1 layer entry = %d, want 0", got)
	}
	if stats := c.RendererUploadStats(); stats.LastMapBytes != 4096 {
		t.Fatalf("one affected flag layer uploaded %+v, want 4096 map bytes", stats)
	}

	c.Fset(1, 1, true)
	draw()
	if got := hw.GetReg16(mode0LayerScreenBase(1)); got != 1 {
		t.Fatalf("flag-1 layer entry after fset = %d, want 1", got)
	}
	if stats := c.RendererUploadStats(); stats.LastMapBytes != 4096 {
		t.Fatalf("second affected flag layer uploaded %+v, want 4096 map bytes", stats)
	}

	c.Mset(0, 0, 0)
	draw()
	if got := hw.GetReg16(mode0LayerScreenBase(0)); got != 0 {
		t.Fatalf("back layer after mset = %d, want 0", got)
	}
	if got := hw.GetReg16(mode0LayerScreenBase(1)); got != 0 {
		t.Fatalf("front layer after mset = %d, want 0", got)
	}
	if stats := c.RendererUploadStats(); stats.LastMapBytes != 8192 {
		t.Fatalf("mset affecting both layers uploaded %+v, want 8192 map bytes", stats)
	}
}
