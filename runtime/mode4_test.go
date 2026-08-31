//go:build !mtile_mode0

package runtime

import (
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode4InitializationAndHiddenPageBlit(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.Pset(0, 0, 0x12)
	c.Pset(1, 0, 0x34)
	c.Pset(2, 0, 0x56)
	c.Pset(3, 0, 0x78)
	c.InitHardware()
	if got := hw.GetReg16(hw.DISPCNT); got != 0x0404 {
		t.Fatalf("DISPCNT = %#x, want Mode 4 BG2 only", got)
	}
	if got := hw.GetReg16(hw.BG2PA); got != 0x0100 {
		t.Fatalf("BG2PA = %#x, want identity", got)
	}
	if got := hw.ReadVRAM32(hw.Page1); got != 0xFFFFFFFF {
		t.Fatalf("page B clear = %#x, want 0xFFFFFFFF", got)
	}
	if !c.BlitToHiddenPage() {
		t.Fatal("blit unexpectedly missed the simulated VBlank")
	}
	if got := hw.ReadVRAM32(hw.Page1); got != 0x78563412 {
		t.Fatalf("packed page-B pixels = %#x, want %#x", got, uint32(0x78563412))
	}
	c.FlipPage()
	if hw.GetReg16(hw.DISPCNT)&0x0010 == 0 {
		t.Fatal("FlipPage did not select page B")
	}
}

func TestPrepareHiddenPageWritesDuringVDraw(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.Pset(0, 0, 0x12)
	c.Pset(1, 0, 0x34)
	c.Pset(2, 0, 0x56)
	c.Pset(3, 0, 0x78)
	c.InitHardware()
	// Unlike BlitToHiddenPage, PrepareHiddenPage must succeed while still in
	// VDraw (VCOUNT < ScreenHeight): the destination page is never the one
	// currently on screen, so there is nothing to protect against here.
	hw.SetVCountForTest(50)
	c.PrepareHiddenPage()
	if got := hw.ReadVRAM32(hw.Page1); got != 0x78563412 {
		t.Fatalf("packed page-B pixels = %#x, want %#x", got, uint32(0x78563412))
	}
	start, end := c.LastBlitWindow()
	if start != 50 || end != 50 {
		t.Fatalf("LastBlitWindow = %d..%d, want the VDraw VCOUNT (50) on both ends", start, end)
	}
}

func TestPrepareHiddenPageThenFlipShowsNewContent(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.InitHardware() // Page0 shown, Page1 (hidden) cleared to 0xFF
	c.Pset(0, 0, 0x12)
	c.Pset(1, 0, 0x34)
	c.Pset(2, 0, 0x56)
	c.Pset(3, 0, 0x78)
	hw.SetVCountForTest(10) // still VDraw, well before VBlank
	c.PrepareHiddenPage()   // writes the new frame into Page1 while Page0 is on screen
	if got := hw.ReadVRAM32(hw.Page0); got != 0xFFFFFFFF {
		t.Fatalf("PrepareHiddenPage must not touch the visible page; Page0 = %#x", got)
	}
	if got := hw.ReadVRAM32(hw.Page1); got != 0x78563412 {
		t.Fatalf("hidden Page1 = %#x, want %#x", got, uint32(0x78563412))
	}
	hw.SetVCountForTest(hw.ScreenHeight) // now in VBlank; safe to flip
	c.FlipPage()
	if hw.GetReg16(hw.DISPCNT)&0x0010 == 0 {
		t.Fatal("FlipPage did not select Page1, which PrepareHiddenPage just wrote")
	}
}

func TestMode4BlitRejectsVDraw(t *testing.T) {
	hw.ResetForTest()
	var c Context
	hw.SetVCountForTest(159)
	if c.BlitToHiddenPage() {
		t.Fatal("blit accepted a VDraw start")
	}
	if c.FrameSkips() != 1 {
		t.Fatalf("frame skips = %d, want 1", c.FrameSkips())
	}
}

func TestMode4BlitLeavesStridePadding(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.Pset(0, 0, 0x12)
	c.InitHardware()
	if !c.BlitToHiddenPage() {
		t.Fatal("blit unexpectedly missed the simulated VBlank")
	}
	if got := hw.ReadVRAM32(hw.Page1 + hw.VisibleBytes); got != 0xFFFFFFFF {
		t.Fatalf("padding after visible page = %#x, want 0xFFFFFFFF", got)
	}
	start, end := c.LastBlitWindow()
	if start != hw.ScreenHeight || end != hw.ScreenHeight {
		t.Fatalf("host blit window = %d..%d, want VBlank start", start, end)
	}
}

func TestMode4BlitKindsWriteTheSamePixels(t *testing.T) {
	for _, kind := range []BlitKind{BlitDMA, BlitCPU, BlitIWRAM} {
		t.Run(kindName(kind), func(t *testing.T) {
			hw.ResetForTest()
			var c Context
			c.SetBlitKind(kind)
			c.Pset(0, 0, 0x12)
			c.Pset(1, 0, 0x34)
			c.Pset(2, 0, 0x56)
			c.Pset(3, 0, 0x78)
			c.InitHardware()
			if !c.BlitToHiddenPage() {
				t.Fatal("missed simulated VBlank")
			}
			if got := hw.ReadVRAM32(hw.Page1); got != 0x78563412 {
				t.Fatalf("packed pixels = %#x, want %#x", got, uint32(0x78563412))
			}
		})
	}
}

// TestMode4BlitRejectsFullFrameWraparound guards against a copy so slow it
// wraps VCOUNT one or more full 228-line frames and happens to land back in
// the 160-227 range, aliasing as "still in VBlank" under a VCOUNT-only check.
// This reproduces the false positive measured for the ROM-resident CPU copy
// on real hardware: 314,098 cycles (over one full 280,896-cycle frame) left
// VCOUNT at 187, which a naive end<start-or-end<ScreenHeight check accepts.
func TestMode4BlitRejectsFullFrameWraparound(t *testing.T) {
	hw.ResetForTest()
	var c Context
	hw.SetVCountForTest(160)
	hw.SetCycleCounterForTest(314098)
	if c.BlitToHiddenPage() {
		t.Fatal("blit accepted a copy that wrapped a full frame")
	}
	if c.FrameSkips() != 1 {
		t.Fatalf("frame skips = %d, want 1", c.FrameSkips())
	}
}

func kindName(kind BlitKind) string {
	switch kind {
	case BlitCPU:
		return "cpu"
	case BlitIWRAM:
		return "iwram"
	default:
		return "dma"
	}
}
