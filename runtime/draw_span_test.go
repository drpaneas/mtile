//go:build !mtile_mode0

package runtime

import "testing"

// Exhaust every alignment and length, including final-screen words and tiny
// spans without an aligned middle. Derive expected pixels independently.
func TestFillRowWordAllAlignmentsAndLengths(t *testing.T) {
	var c Context
	const color byte = 173
	const word uint32 = 0xadadadad
	for _, y := range []int{0, 79, 159} {
		for i := range c.framebuffer {
			c.framebuffer[i] = 37
		}
		row := c.framebuffer[y*ScreenWidth : (y+1)*ScreenWidth]
		for left := 0; left < ScreenWidth; left++ {
			for right := left; right < ScreenWidth; right++ {
				for i := range row {
					row[i] = 37
				}
				c.fillRowWord(y, left, right, color, word)
				for x, got := range row {
					want := byte(37)
					if x >= left && x <= right {
						want = color
					}
					if got != want {
						t.Fatalf("row%d [%d,%d] x%d got%d want%d", y, left, right, x, got, want)
					}
				}
			}
		}
		// Only the selected row was reset between spans. Any stray store
		// elsewhere survives and is detected here.
		for i, got := range c.framebuffer {
			if i/ScreenWidth != y && got != 37 {
				t.Fatalf("row%d clobbered pixel%d with%d", y, i, got)
			}
		}
	}
}
