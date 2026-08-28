//go:build !tinygo

package hw

// Host tests do not model scanline progression. The default simulated VCOUNT
// is the start of VBlank, so a blit can be tested deterministically.
func WaitForPresentWindow() {}
func WaitVDraw()            {}
