//go:build mtile_mode0 && mtile_mode0_obj && !tinygo

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0MovingObjectsDoNotRewriteUnchangedPalette(t *testing.T) {
	hw.ResetForTest()
	sheet := make([]byte, 16*8)
	sheet[8] = 1
	c := newMode0TestContext(t, 16, 8, 1, 1, sheet, []uint16{0}, []byte{0, 0})
	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(1, 10, 20, 1, 1, false, false)
	c.PresentRender()
	if c.paletteDirty {
		t.Fatal("palette remained dirty after first present")
	}
	// A sentinel in simulated MMIO detects writes without adding a production
	// counter. A palette upload would overwrite it with the Context's color.
	const sentinel = 0x7ffe
	hw.SetPaletteRAM16(257, sentinel)
	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(1, 11, 20, 1, 1, false, false)
	c.PresentRender()
	if got := hw.ReadPaletteRAM16(257); got != sentinel {
		t.Fatalf("unchanged OBJ palette was rewritten: entry 1 = %#x", got)
	}
	if got := hw.GetReg16(hw.OAM + 2); got != 11 {
		t.Fatalf("sprite movement was not committed: x = %d", got)
	}
	if got := c.RendererUploadStats().LastOAMBytes; got != hw.OAMBytes {
		t.Fatalf("OAM upload = %d, want %d", got, hw.OAMBytes)
	}
}

func TestMode0PaletteMutationsReachBothBanksWithoutObjects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Context)
	}{
		{"native-palette", func(c *Context) {
			colors := c.palette
			colors[2] = 0x3456
			c.LoadPalette(&colors)
		}},
		{"draw-remap", func(c *Context) { c.Pal(1, 2, 0) }},
		{"display-remap", func(c *Context) { c.Pal(1, 2, 1) }},
		{"draw-layer-reset", func(c *Context) { c.Pal(1, 2, 0); c.PalLayerReset(0) }},
		{"display-layer-reset", func(c *Context) { c.Pal(1, 2, 1); c.PalLayerReset(1) }},
		{"full-reset", func(c *Context) { c.Pal(1, 2, 0); c.Pal(2, 3, 1); c.PalReset() }},
		{"composed-remaps", func(c *Context) { c.Pal(1, 2, 0); c.Pal(2, 3, 1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hw.ResetForTest()
			c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, []byte{0, 0})
			// Poison every simulated entry, so resets are checked even when
			// their final mapping is the identity seen before the mutation.
			for index := 0; index < 512; index++ {
				hw.SetPaletteRAM16(index, 0xffff)
			}
			tc.mutate(c)
			c.BeginRenderFrame()
			c.Cls(5)
			c.PresentRender()
			for index := 0; index < 256; index++ {
				color := c.palette[c.displayPal[c.drawPal[index]]]
				if got := hw.ReadPaletteRAM16(256 + index); got != color {
					t.Fatalf("OBJ entry %d = %#x, want %#x", index, got, color)
				}
				if index == 0 {
					color = c.palette[c.displayPal[c.mode0Clear]]
				}
				if got := hw.ReadPaletteRAM16(index); got != color {
					t.Fatalf("BG entry %d = %#x, want %#x", index, got, color)
				}
			}
			if c.paletteDirty {
				t.Fatal("palette dirty flag was not consumed")
			}
			if got := c.RendererUploadStats().LastOAMBytes; got != 0 {
				t.Fatalf("palette-only frame uploaded %d OAM bytes", got)
			}
		})
	}
}

func TestMode0PaletteMutationAfterInvisibleFrameAppliesToNextSprite(t *testing.T) {
	hw.ResetForTest()
	sheet := make([]byte, 16*8)
	sheet[8] = 1
	c := newMode0TestContext(t, 16, 8, 1, 1, sheet, []uint16{0}, []byte{0, 0})
	c.Pal(1, 2, 0)
	c.BeginRenderFrame()
	c.Cls(0)
	c.PresentRender()
	c.BeginRenderFrame()
	c.Cls(0)
	c.Spr(1, 4, 5, 1, 1, false, false)
	c.PresentRender()
	if got := hw.ReadPaletteRAM16(257); got != c.palette[2] {
		t.Fatalf("new sprite sees OBJ color %#x, want remapped %#x", got, c.palette[2])
	}
}

func TestMode0InitPaletteMutationCommitsWhileForceBlanked(t *testing.T) {
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, []byte{0, 0})
	// Generated lifecycle: hardware init, user _init mutations, finalize
	// tile/palette uploads while force-blanked, then the first visible commit.
	if got := hw.GetReg16(hw.DISPCNT); got != mode0ForceBlank {
		t.Fatalf("init display = %#x, want force blank", got)
	}
	c.Pal(1, 2, 0)
	c.FinalizeInitHardware()
	if c.paletteDirty {
		t.Fatal("finalization left the initialization palette upload pending")
	}
	for _, index := range []int{1, 257} {
		if got := hw.ReadPaletteRAM16(index); got != c.palette[2] {
			t.Fatalf("finalized palette entry %d = %#x, want %#x", index, got, c.palette[2])
		}
	}
	if got := hw.GetReg16(hw.DISPCNT); got != mode0ForceBlank {
		t.Fatalf("finalization enabled display before first present: %#x", got)
	}
	const sentinel = 0x7ffe
	hw.SetPaletteRAM16(1, sentinel)
	hw.SetPaletteRAM16(257, sentinel)
	c.BeginRenderFrame()
	c.Cls(0)
	c.PresentRender()
	for _, index := range []int{1, 257} {
		if got := hw.ReadPaletteRAM16(index); got != sentinel {
			t.Fatalf("first present repeated finalized palette upload at entry %d", index)
		}
	}
}
