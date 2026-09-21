package runtime

// PoseFlipX and PoseFlipY are the PosePart.Flags bits. A pose part's
// authored flip state is XORed with PSpr's flipX/flipY arguments, mirroring
// how a single sprite's own facing composes with a caller-supplied flip.
const (
	PoseFlipX uint8 = 1 << 0
	PoseFlipY uint8 = 1 << 1
)

// PosePart is one fixed-offset sprite cell within a multi-part pose. X/Y are
// offsets from the pose's own (x,y) origin, in the same units as spr()'s
// pixel coordinates.
type PosePart struct {
	Tile  uint16
	X, Y  int16
	Flags uint8
}

// PoseRange names a pose's contiguous span within AssetImage/AssetMemory's
// shared PoseParts array. Start/Count index PoseParts directly; they are not
// byte offsets.
type PoseRange struct {
	Start uint16
	Count uint16
}

// PSpr draws a multi-part pose as a sequence of 1x1 Spr calls. Each part's
// own offset and flip flags are composed with the caller's flipX/flipY: a
// horizontal flip negates the part's X offset around the pose origin (an
// 8-pixel-wide cell, hence the -part.X-8 mirror point) and XORs the part's
// authored FlipX bit, and flipY behaves symmetrically on Y. An out-of-range
// id, a cart without pose tables, or a pose whose range escapes PoseParts is
// a silent no-op, matching Spr's own out-of-range behavior.
func (c *Context) PSpr(id, x, y int, flipX, flipY bool) {
	if c == nil || c.cart == nil || id < 0 || id >= len(c.cart.PoseIndex) {
		return
	}
	r := c.cart.PoseIndex[id]
	end := int(r.Start) + int(r.Count)
	if int(r.Start) > len(c.cart.PoseParts) || end > len(c.cart.PoseParts) {
		return
	}
	for _, part := range c.cart.PoseParts[r.Start:end] {
		ox, oy := int(part.X), int(part.Y)
		partFlipX := part.Flags&PoseFlipX != 0
		partFlipY := part.Flags&PoseFlipY != 0
		if flipX {
			ox = -int(part.X) - 8
			partFlipX = !partFlipX
		}
		if flipY {
			oy = -int(part.Y) - 8
			partFlipY = !partFlipY
		}
		c.Spr(int(part.Tile), x+ox, y+oy, 1, 1, partFlipX, partFlipY)
	}
}
