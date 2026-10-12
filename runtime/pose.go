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
	Start uint32
	Count uint16
	// Build-time sums of the minimum and exclusive maximum tile bounds. A
	// flipped draw adds these before mirroring around the authored origin,
	// keeping the same local bounding rectangle even with negative offsets.
	FlipOffsetX, FlipOffsetY int32
}

// PSpr draws a multi-part pose, flipping within its authored tile bounds.
// The compiler supplies the bounds, so drawing needs no bounds scan or extra
// pixel storage. Each part's own flip flags are XORed with flipX/flipY.
// Unflipped draws retain their authored offsets from (x,y). An out-of-range
// id, a cart without pose tables, or a pose whose range escapes PoseParts is
// a silent no-op, matching Spr's own out-of-range behavior.
func (c *Context) PSpr(id, x, y int, flipX, flipY bool) {
	c.PSprSite(0, 0, id, x, y, flipX, flipY)
}

func (c *Context) PSprSite(site, windowBase, id, x, y int, flipX, flipY bool) {
	if (flipX || flipY) && c != nil && c.cart != nil && uint(id) < uint(len(c.cart.PoseIndex)) {
		pose := &c.cart.PoseIndex[id]
		if flipX {
			x += int(pose.FlipOffsetX)
		}
		if flipY {
			y += int(pose.FlipOffsetY)
		}
	}
	c.drawPoseSite(site, windowBase, id, x, y, flipX, flipY)
}

// drawPoseSite is the origin-mirroring primitive shared by standalone poses
// and animations. Its caller has already applied either pose or clip bounds.
func (c *Context) drawPoseSite(site, windowBase, id, x, y int, flipX, flipY bool) {
	if c != nil && c.cart != nil && c.cart.PoseVariantBaseCount > 0 {
		if id < 0 || id >= c.cart.PoseVariantBaseCount {
			return
		}
		id += c.poseVariant * c.cart.PoseVariantBaseCount
	}
	if c != nil && c.psprOnHardwareOBJ() {
		c.mode0PSpr(site, windowBase, id, x, y, flipX, flipY)
		return
	}
	c.drawPose(id, x, y, flipX, flipY)
}

func (c *Context) PSprSiteFromBase(base, windowBase, windowStride, id, x, y int, flipX, flipY bool) {
	site := c.TakePSprSite(base)
	window := windowBase + (site-base)*windowStride
	c.PSprSite(site, window, id, x, y, flipX, flipY)
}

func (c *Context) TakePSprSite(base int) int {
	if c != nil && c.psprOnHardwareOBJ() {
		return c.takeMode0PsprSite(base)
	}
	return base
}

func (c *Context) drawPose(id, x, y int, flipX, flipY bool) {
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
		c.drawPosePart(int(part.Tile), x+ox, y+oy, partFlipX, partFlipY)
	}
}

func (c *Context) drawPosePart(tile, x, y int, flipX, flipY bool) {
	if c == nil || c.cart == nil {
		return
	}
	if c.psprOnHardwareOBJ() {
		c.mode0PSprPart(tile, x, y, flipX, flipY)
		return
	}
	if c.cart.ObjSpriteN > 0 {
		c.drawObjectCell(tile, x, y, flipX, flipY)
		return
	}
	c.Spr(tile, x, y, 1, 1, flipX, flipY)
}
