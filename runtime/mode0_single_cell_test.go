//go:build mtile_mode0 && mtile_mode0_obj

package runtime

import (
	"fmt"
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func TestMode0SingleCellMatchesSourceGridBounds(t *testing.T) {
	// Include non-power-of-two sheets and both the logical sheet edge and
	// physical OBJ tile limit. The reference deliberately uses the original
	// row/column calculation, independent of the optimized linear bound.
	for _, grid := range [][2]int{{1, 4}, {13, 3}, {30, 20}, {64, 9}} {
		t.Run(fmt.Sprintf("%dx%d", grid[0], grid[1]), func(t *testing.T) {
			hw.ResetForTest()
			cols, rows := grid[0], grid[1]
			c := newMode0TestContext(t, cols*8, rows*8, 1, 1,
				make([]byte, cols*rows*64), []uint16{0}, make([]byte, cols*rows))
			c.Cls(0)
			c.Camera(3, 4)
			for _, zeroVisible := range []bool{false, true} {
				c.mode0ObjectZeroVisible = zeroVisible
				for id := -1; id <= cols*rows+1; id++ {
					c.mode0ObjectCount = 0
					c.mode0OAMDirty = false
					c.renderSprite(id, 1, 3, 1, 1, true, true)
					want := id >= 0 && id < c.cart.SpriteN && id < mode0ObjectTileLimit &&
						id%cols+1 <= cols && id/cols+1 <= rows && (id != 0 || zeroVisible)
					if got := c.mode0ObjectCount == 1; got != want {
						t.Fatalf("id=%d opaqueZero=%t: emitted=%t want=%t", id, zeroVisible, got, want)
					}
					if c.mode0OAMDirty != want {
						t.Fatalf("id=%d: dirty=%t want=%t", id, c.mode0OAMDirty, want)
					}
					if !want {
						continue
					}
					data := c.mode0OAM[127*8:]
					get := func(i int) uint16 { return uint16(data[i]) | uint16(data[i+1])<<8 }
					if get(0) != 0x20ff || get(2) != 0x31fe || get(4) != uint16(id*2) {
						t.Fatalf("id=%d: camera/flips/tile attributes = %04x %04x %04x", id, get(0), get(2), get(4))
					}
				}
			}
		})
	}
}
