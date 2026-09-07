//go:build mtile_tilecanvas && !mtile_mode0

package runtime

func (c *Context) canvasFullMap(slot int) bool { return c.mode0MapFullCache[slot] }
