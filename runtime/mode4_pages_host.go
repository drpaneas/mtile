//go:build !tinygo && !mtile_mode0 && !mtile_tilecanvas

package runtime

// Host tests give every Context its own pair of Mode 4 pages, so independent
// Contexts never share the simulated VRAM.
type mode4PageStorage [2][FramebufferSize]byte

func (c *Context) mode4Page(index int) []byte { return c.pages[index][:] }

func copyMode4Page(dst, src []byte) { copy(dst, src) }

func fillMode4Page(page []byte, value byte) {
	for i := range page {
		page[i] = value
	}
}
