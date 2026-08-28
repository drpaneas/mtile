package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestPaletteHas256EntriesAndSeparateLayers(t *testing.T) {
	hw.ResetForTest()
	var c Context
	var colors [256]uint16
	colors[7] = 0x1234
	colors[200] = 0x5678
	c.LoadPalette(&colors)
	c.Pal(1, 200, 0)
	c.Pal(1, 7, 1)
	c.PaltColor(200, true)
	c.ApplyDisplayPalette()
	if got := c.PaletteEntry(200); got != 0x5678 {
		t.Fatalf("palette[200] = %#x, want %#x", got, uint16(0x5678))
	}
	if got := hw.ReadPaletteRAM16(1); got != 0x1234 {
		t.Fatalf("display remap palette[1] = %#x, want %#x", got, uint16(0x1234))
	}
	if !c.transp[200] || !c.transp[0] {
		// PaltColor(200,true) is independent; color 0 remains transparent by
		// default.
		t.Fatal("unexpected transparency state")
	}
}

func TestDefaultPaletteUsesFullAuroraTable(t *testing.T) {
	var c Context
	if got := c.PaletteEntry(0); got != 0 {
		t.Fatalf("aurora[0] = %#x, want 0", got)
	}
	if got := c.PaletteEntry(1); got != 0x0842 {
		t.Fatalf("aurora[1] = %#x, want %#x", got, uint16(0x0842))
	}
	if c.PaletteEntry(16) == c.PaletteEntry(0) {
		t.Fatal("aurora index 16 aliased color 0; the default table must occupy all 256 slots")
	}
	if c.PaletteEntry(255) == c.PaletteEntry(0) {
		t.Fatal("aurora index 255 aliased color 0; the default table must occupy all 256 slots")
	}
}

func TestPaletteResetRestoresDefaultTransparency(t *testing.T) {
	var c Context
	c.PaltColor(0, false)
	c.PaltColor(9, true)
	c.PalReset()
	if !c.transp[0] || c.transp[9] {
		t.Fatal("PalReset did not restore only color 0 transparency")
	}
	if c.drawPal[17] != 17 || c.displayPal[17] != 17 {
		t.Fatal("PalReset did not restore identity remaps")
	}
}

func TestPaltResetRestoresOnlyColorZero(t *testing.T) {
	t.Parallel()
	var c Context
	c.PaltColor(0, false)
	c.PaltColor(4, true)
	c.PaltReset()
	if !c.transp[0] || c.transp[4] {
		t.Fatal("PaltReset did not restore only color 0 transparency")
	}
}

func TestPalLayerResetAndRejectedLayer(t *testing.T) {
	t.Parallel()
	var c Context
	c.Pal(3, 9, 0)
	c.Pal(4, 10, 1)
	c.Pal(5, 11, 2)
	if c.drawPal[3] != 9 {
		t.Fatal("valid draw remap was rejected")
	}
	if c.drawPal[5] != 5 {
		t.Fatal("pal layer 2 was not rejected")
	}
	if c.displayPal[4] != 10 {
		t.Fatal("valid display remap was rejected")
	}
	c.PalLayerReset(2)
	if c.drawPal[3] != 9 || c.displayPal[4] != 10 {
		t.Fatal("PalLayerReset(2) mutated a valid layer")
	}
	c.PalLayerReset(0)
	if c.drawPal[3] != 3 {
		t.Fatal("PalLayerReset(0) did not restore draw identity")
	}
	if c.displayPal[4] != 10 {
		t.Fatal("PalLayerReset(0) reset the display layer")
	}
	c.PalLayerReset(1)
	if c.displayPal[4] != 4 {
		t.Fatal("PalLayerReset(1) did not restore display identity")
	}
}
