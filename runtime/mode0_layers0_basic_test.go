//go:build mtile_mode0 && mtile_mode0_layers0

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0ZeroLayersPresentsBackdrop(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 0, 0, make([]byte, 16*8), nil, make([]byte, 2))
	var palette [256]uint16
	palette[5] = 0x1234
	c.LoadPalette(&palette)
	c.BeginRenderFrame()
	c.Cls(5)
	c.PresentRender()
	if got := hw.GetReg16(hw.DISPCNT); got != mode0ObjectDisplayBits() {
		t.Fatalf("DISPCNT = %#04x, want backdrop/optional-OBJ display %#04x", got, mode0ObjectDisplayBits())
	}
	if got := hw.ReadPaletteRAM16(0); got != palette[5] {
		t.Fatalf("backdrop palette = %#04x, want %#04x", got, palette[5])
	}
}

func TestMode0ZeroLayersSkipsUnusedBGInitialization(t *testing.T) {
	hw.ResetForTest()
	const sentinel = uint32(0xdecafbad)
	hw.SetVRAM32(hw.VRAM+32*1024, sentinel)

	c := newMode0TestContext(t, 16, 8, 0, 0, make([]byte, 16*8), nil, make([]byte, 2))
	if got := hw.ReadVRAM32(hw.VRAM + 32*1024); got != sentinel {
		t.Fatalf("unused BG VRAM = %#08x, want untouched %#08x", got, sentinel)
	}
	if got := c.RendererUploadStats().ColdTileBytes; got != 0 {
		t.Fatalf("cold BG tile upload = %d bytes, want 0", got)
	}
}
