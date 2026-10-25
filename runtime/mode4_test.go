//go:build !mtile_mode0 && !mtile_tilecanvas

package runtime

import (
	"testing"
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

func pageWord(c *Context, page, offset int) uint32 {
	p := c.mode4Page(page)
	return uint32(p[offset]) | uint32(p[offset+1])<<8 | uint32(p[offset+2])<<16 | uint32(p[offset+3])<<24
}

func psetRow(c *Context) {
	c.Pset(0, 0, 0x12)
	c.Pset(1, 0, 0x34)
	c.Pset(2, 0, 0x56)
	c.Pset(3, 0, 0x78)
}

func TestMode4InitHardwareKeepsLogicalImageOnHiddenPage(t *testing.T) {
	hw.ResetForTest()
	var c Context
	psetRow(&c)
	c.InitHardware()
	if got := hw.GetReg16(hw.DISPCNT); got != 0x0404 {
		t.Fatalf("DISPCNT = %#x, want Mode 4 BG2 only, page A", got)
	}
	if got := hw.GetReg16(hw.BG2PA); got != 0x0100 {
		t.Fatalf("BG2PA = %#x, want identity", got)
	}
	if got := pageWord(&c, 0, 0); got != 0xFFFFFFFF {
		t.Fatalf("visible page A = %#x, want 0xFFFFFFFF", got)
	}
	if got := pageWord(&c, 1, 0); got != 0x78563412 {
		t.Fatalf("hidden page B = %#x, want pixels drawn before init", got)
	}
	if got := pageWord(&c, 1, 4); got != 0 {
		t.Fatalf("undrawn hidden pixels = %#x, want the zero initial image", got)
	}
}

func TestMode4DrawsOnHiddenPageAndFlipShowsIt(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.InitHardware()
	hw.SetVCountForTest(10)
	psetRow(&c)
	c.PrepareRender()
	if got := pageWord(&c, 0, 0); got != 0xFFFFFFFF {
		t.Fatalf("drawing touched the visible page: %#x", got)
	}
	if got := pageWord(&c, 1, 0); got != 0x78563412 {
		t.Fatalf("hidden page = %#x, want %#x", got, uint32(0x78563412))
	}
	hw.SetVCountForTest(hw.ScreenHeight)
	c.PresentRender()
	if hw.GetReg16(hw.DISPCNT)&pageBit == 0 {
		t.Fatal("PresentRender did not show page B")
	}
}

func TestMode4FirstPresentShowsUndrawnImage(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.InitHardware()
	c.PresentRender()
	if hw.GetReg16(hw.DISPCNT)&pageBit == 0 {
		t.Fatal("first present must show the logical image even without draws")
	}
}

func TestMode4TickWithoutDrawsKeepsVisiblePage(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.InitHardware()
	c.Pset(0, 0, 9)
	c.PresentRender()
	shown := hw.GetReg16(hw.DISPCNT) & pageBit
	c.PresentRender()
	if got := hw.GetReg16(hw.DISPCNT) & pageBit; got != shown {
		t.Fatal("a tick without draws flipped to the stale page")
	}
	if got := c.Pget(0, 0); got != 9 {
		t.Fatalf("Pget after an idle tick = %d, want 9", got)
	}
}

func TestMode4FirstWriteAfterFlipKeepsFramebufferPersistent(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.InitHardware()
	c.Pset(0, 0, 1)
	c.PresentRender() // page B shown; page A holds the image from before init
	if got := c.Pget(0, 0); got != 1 {
		t.Fatalf("Pget reads %d from the visible page, want 1", got)
	}
	c.Pset(1, 0, 2)
	if got := pageWord(&c, 0, 0) & 0xFFFF; got != 0x0201 {
		t.Fatalf("hidden page A = %#x, want previous pixel 1 plus new pixel 2", got)
	}
	c.PresentRender()
	if hw.GetReg16(hw.DISPCNT)&pageBit != 0 {
		t.Fatal("second present did not return to page A")
	}
	c.Pset(2, 0, 3)
	if got := pageWord(&c, 1, 0) & 0xFFFFFF; got != 0x030201 {
		t.Fatalf("hidden page B = %#x, want all three pixels", got)
	}
}

func TestMode4ClsAfterFlipReplacesStalePage(t *testing.T) {
	hw.ResetForTest()
	var c Context
	c.InitHardware()
	c.Pset(5, 5, 9)
	c.PresentRender()
	c.Cls(3)
	for i, got := range c.mode4Page(0) {
		if got != 3 {
			t.Fatalf("cleared hidden page byte %d = %d, want 3", i, got)
		}
	}
	if got := c.Pget(5, 5); got != 3 {
		t.Fatalf("Pget after cls = %d, want 3", got)
	}
	if got := c.mode4Page(1)[5*ScreenWidth+5]; got != 9 {
		t.Fatalf("cls touched the visible page: %d", got)
	}
}

func TestFramebufferByteStoreKeepsHalfwordNeighbour(t *testing.T) {
	// Mode 4 cannot rely on 8-bit VRAM stores, so each pixel is merged into its
	// aligned halfword. Both neighbours must survive.
	var words [2]uint16
	words[0], words[1] = 0xAABB, 0xCCDD
	base := unsafe.Pointer(&words[0])
	storeFramebufferByte(base, 0x11)
	storeFramebufferByte(unsafe.Add(base, 3), 0x22)
	if words[0] != 0xAA11 || words[1] != 0x22DD {
		t.Fatalf("halfwords = %#04x %#04x, want 0xaa11 0x22dd", words[0], words[1])
	}
}

func TestMode4PagesStayInByteMergeableBGVRAM(t *testing.T) {
	// Above 0x06014000 (OBJ VRAM in bitmap modes) hardware ignores 8-bit
	// stores entirely; the halfword merge only needs BG VRAM, but both pages
	// must sit below the boundary for the 32-bit page operations as well.
	const objVRAM = 0x06014000
	if end := hw.Page1 + FramebufferSize; end > objVRAM {
		t.Fatalf("page B ends at %#x, beyond BG VRAM", end)
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
