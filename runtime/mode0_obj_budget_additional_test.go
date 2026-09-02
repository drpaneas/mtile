//go:build mtile_mode0 && mtile_mode0_obj

package runtime

import (
	"fmt"
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0OAMCompactionPreservesPriorityAcrossOverlapSizes(t *testing.T) {
	requireMode0LayerCount(t, 1)
	for _, count := range []int{1, 63, 64, 65, 100, 127, 128} {
		t.Run(fmt.Sprintf("count-%d", count), func(t *testing.T) {
			hw.ResetForTest()
			c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))
			c.BeginRenderFrame()
			c.Cls(0)
			for index := 0; index < count; index++ {
				c.Spr(1, index, 0, 1, 1, false, false)
			}
			c.PresentRender()

			for entry := 0; entry < count; entry++ {
				got := int(hw.GetReg16(hw.OAM+uintptr(entry*8)+2) & 0x01ff)
				if want := count - 1 - entry; got != want {
					t.Fatalf("entry %d x = %d, want reverse painter order %d", entry, got, want)
				}
			}
			if count < mode0ObjectLimit {
				if got := hw.GetReg16(hw.OAM + uintptr(count*8)); got != mode0ObjectDisable {
					t.Fatalf("first inactive entry %d attr0 = %#04x, want disabled", count, got)
				}
			}
		})
	}
}

func TestMode0OAMOverflowRejectsWholeMetasprite(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	c := newMode0TestContext(t, 24, 8, 1, 1, make([]byte, 24*8), []uint16{0}, make([]byte, 3))
	c.BeginRenderFrame()
	c.Cls(0)
	for index := 0; index < 127; index++ {
		c.Spr(1, index, 0, 1, 1, false, false)
	}
	c.Spr(1, 200, 0, 2, 1, false, false)
	if c.mode0ObjectCount != 127 {
		t.Fatalf("object count = %d, want the overflowing two-cell metasprite rejected atomically", c.mode0ObjectCount)
	}
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM + uintptr(127*8)); got != mode0ObjectDisable {
		t.Fatalf("OAM entry 127 attr0 = %#04x, want disabled after whole-metasprite rejection", got)
	}
}

func TestMode0OAMCompactionClearsShrinkingAndGrowingFrames(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	c := newMode0TestContext(t, 16, 8, 1, 1, make([]byte, 16*8), []uint16{0}, make([]byte, 2))

	for frame, count := range []int{100, 28, 100, 0, 65, 1} {
		c.BeginRenderFrame()
		c.Cls(0)
		for index := 0; index < count; index++ {
			c.Spr(1, index, frame, 1, 1, false, false)
		}
		c.PresentRender()

		for entry := 0; entry < mode0ObjectLimit; entry++ {
			attr0 := hw.GetReg16(hw.OAM + uintptr(entry*8))
			if entry >= count {
				if attr0 != mode0ObjectDisable {
					t.Fatalf("frame %d entry %d attr0 = %#04x, want stale entry disabled", frame, entry, attr0)
				}
				continue
			}
			if got := int(attr0 & 0x00ff); got != frame {
				t.Fatalf("frame %d entry %d y = %d, want active current-frame object", frame, entry, got)
			}
		}
	}
}

func TestMode0ObjectsCullWrappedOffscreenCoordinates(t *testing.T) {
	requireMode0LayerCount(t, 1)
	hw.ResetForTest()
	c := newMode0TestContext(t, 24, 8, 1, 1, make([]byte, 24*8), []uint16{0}, make([]byte, 3))
	c.BeginRenderFrame()
	c.Cls(0)

	// These coordinates alias (0,0) after the GBA OBJ attribute masks, but
	// Mode 4 clips both sprites completely away.
	c.Spr(1, 512, 0, 1, 1, false, false)
	c.Spr(1, 0, 256, 1, 1, false, false)
	if c.mode0ObjectCount != 0 {
		t.Fatalf("wrapped offscreen sprites allocated %d OAM entries, want none", c.mode0ObjectCount)
	}

	// Cull metasprite cells individually so an offscreen leading cell does not
	// hide the adjacent cell that still intersects the viewport.
	c.Spr(1, -8, 0, 2, 1, false, false)
	if c.mode0ObjectCount != 1 {
		t.Fatalf("partially visible metasprite allocated %d OAM entries, want one", c.mode0ObjectCount)
	}
	c.PresentRender()
	if got := hw.GetReg16(hw.OAM+2) & 0x01ff; got != 0 {
		t.Fatalf("visible metasprite cell x = %d, want 0", got)
	}
}
