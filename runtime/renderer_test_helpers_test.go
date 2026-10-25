package runtime

import "testing"

// requireSoftwareFramebuffer keeps the few mixed test files honest under the
// full renderer-tag matrix. Mode 0 deliberately has no 38,400-byte software
// framebuffer; its drawing behavior is covered by the hardware-state tests.
func requireSoftwareFramebuffer(t *testing.T) {
	t.Helper()
	var c Context
	if len(c.fbForTest()) == 0 {
		t.Skip("test exercises the Mode 4 software framebuffer")
	}
}

// fbForTest returns the logical software framebuffer image. Mode 4 binds its
// hidden page on first use, so tests that seed pixels need this accessor.
func (c *Context) fbForTest() []byte {
	c.ensureFramebuffer()
	return c.framebuffer[:]
}
