//go:build mtile_mode0 && !mtile_mode0_obj

package runtime

type mode0ObjectState struct{}

func (c *Context) initMode0Objects()                                  {}
func (c *Context) beginMode0Objects()                                 {}
func (c *Context) presentMode0Objects() uint32                        { return 0 }
func (c *Context) uploadMode0ObjectTile(id int, tile []byte) uint32   { return 0 }
func (c *Context) mode0ObjectTileMutated(id int)                      {}
func (c *Context) dirtyMode0PackedObject(id int)                      {}
func mode0ObjectDisplayBits() uint16                                  { return 0 }
func (c *Context) renderSprite(id, x, y, w, h int, flipX, flipY bool) {}
func (c *Context) renderSSpr(sx, sy, sw, sh, dx, dy, dw, dh int, flipX, flipY bool) bool {
	return true
}
