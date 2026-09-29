//go:build mtile_mode0 && !mtile_mode0_obj

package runtime

type mode0ObjectState struct{}

func (c *Context) hasMode0PackedUploads() bool { return false }
func (c *Context) hasMode0PsprUploads() bool   { return false }
func (c *Context) prepareMode0PsprStreams()    {}
func (c *Context) prepareMode0PackedObjects()  {}
func (c *Context) initMode0ResidentPoses()     {}

func (c *Context) initMode0Objects()                                  {}
func (c *Context) applyMode0Palettes()                                { c.ApplyDisplayPalette() }
func (c *Context) beginMode0Objects()                                 {}
func (c *Context) presentMode0Objects() uint32                        { return 0 }
func (c *Context) uploadMode0ObjectTile(id int, tile []byte) uint32   { return 0 }
func (c *Context) mode0ObjectTileMutated(id int)                      {}
func (c *Context) dirtyMode0PackedObject(id int)                      {}
func mode0ObjectDisplayBits() uint16                                  { return 0 }
func (c *Context) renderSprite(id, x, y, w, h int, flipX, flipY bool) {}
func (c *Context) mode0PSprPart(id, x, y int, flipX, flipY bool)      {}
func (c *Context) mode0PSpr(site, windowBase, id, x, y int, flipX, flipY bool) {
}
func (c *Context) takeMode0PsprSite(base int) int { return base }
func (c *Context) presentMode0PsprStreams() uint32 {
	return 0
}
func (c *Context) renderSSpr(sx, sy, sw, sh, dx, dy, dw, dh int, flipX, flipY bool) bool {
	return true
}
