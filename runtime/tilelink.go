package runtime

// TileLink draws every map occurrence of one shared tile as a registered pose
// or animation clip. ID is a pose ID, or an AnimationClips index when Clip is
// set. The map keeps the tile ID, so Mget and Fget still observe it.
type TileLink struct {
	Tile uint16
	Clip bool
	ID   uint16
}

// TileLinkInstance is one build-time map cell holding a linked tile. Rows
// index instances by logical layer and map row; each row is sorted by X.
type TileLinkInstance struct {
	X, Y  uint16
	Layer uint8
}

// TileLinkReach is how far, in pixels, any linked object extends beyond its
// 8x8 cell. Map draws widen their visible cell range by it, so an object whose
// cell is just off screen still draws its visible part.
type TileLinkReach struct {
	Left, Top, Right, Bottom int16
}

// SetTileLinkSites gives the next map draw's linked objects their compiled
// Mode 0 object allocation: up to `count` sites starting at `base`, with pose
// windows `stride` tiles apart from `window`. Mode 4 never calls it.
func (c *Context) SetTileLinkSites(base, window, stride int) {
	c.tileLinkSite, c.tileLinkWindow, c.tileLinkStride = base, window, stride
}

// tileLinkOf returns the link index plus one for a shared tile, or zero.
func (c *Context) tileLinkOf(id uint16) uint8 {
	if int(id) >= len(c.cart.TileLinkOf) {
		return 0
	}
	return c.cart.TileLinkOf[id]
}

// drawTileLink draws a linked object with the same semantics as a pspr or
// anim call at that point: camera, clip, pal, palt, and pvariant apply, and
// clips follow the default update clock.
func (c *Context) drawTileLink(link uint8, worldX, worldY int) {
	l := c.cart.TileLinks[link-1]
	if l.Clip {
		c.animSiteFromBase(c.tileLinkSite, c.tileLinkWindow, c.tileLinkStride, int(l.ID), worldX, worldY, false, false, Value{})
		return
	}
	c.PSprSiteFromBase(c.tileLinkSite, c.tileLinkWindow, c.tileLinkStride, int(l.ID), worldX, worldY, false, false)
}

// drawTileLinks draws the linked objects of one map draw after its tiles, in
// row-major cell order. Cell (cellX+mx, cellY+my) is drawn at world position
// (screenX+(cellX+mx)*8, screenY+(cellY+my)*8) before the camera applies.
func (c *Context) drawTileLinks(bg, cellX, cellY, cellsW, cellsH, screenX, screenY, layer int) {
	if c.cart == nil || len(c.cart.TileLinks) == 0 || c.clipX0 >= c.clipX1 || c.clipY0 >= c.clipY1 {
		return
	}
	reach := c.cart.TileLinkReach
	baseX := screenX + cellX*8 - c.camX
	baseY := screenY + cellY*8 - c.camY
	// Default spans have no explicit cell boundary. Intersect the remaining
	// map with the widened clip below so anchors beyond the right/bottom edge
	// can still draw a pose that reaches back into view. Positive counts keep
	// their explicit source rectangle.
	if cellsW <= 0 {
		cellsW = c.cart.MapW - cellX
	}
	if cellsH <= 0 {
		cellsH = c.cart.MapH - cellY
	}
	mx0, mx1 := mapVisibleCellRange(cellX, cellsW, c.cart.MapW, baseX, c.clipX0-int(reach.Right), c.clipX1+int(reach.Left))
	my0, my1 := mapVisibleCellRange(cellY, cellsH, c.cart.MapH, baseY, c.clipY0-int(reach.Bottom), c.clipY1+int(reach.Top))
	if mx0 >= mx1 || my0 >= my1 {
		return
	}
	worldX, worldY := screenX+cellX*8, screenY+cellY*8
	var mask byte
	if layer > 0 && layer <= 255 {
		mask = byte(layer)
	}
	if c.tileLinkScanLayers&(1<<uint(bg)) != 0 || len(c.cart.TileLinkRows) == 0 {
		c.drawTileLinksScan(bg, cellX, cellY, mx0, mx1, my0, my1, worldX, worldY, mask)
		return
	}
	c.drawTileLinksIndex(bg, cellX, cellY, mx0, mx1, my0, my1, worldX, worldY, mask)
}

// drawTileLinksIndex visits only the build-time instances in visible rows,
// starting each row at a binary-searched column. Instances draw whatever
// linked tile the cell holds now, so Mset can remove or swap them.
//
//go:noinline
//go:section .iwram
func (c *Context) drawTileLinksIndex(bg, cellX, cellY, mx0, mx1, my0, my1, worldX, worldY int, mask byte) {
	rows := c.cart.TileLinkRows
	instances := c.cart.TileLinkInstances
	cur := c.newMapCursor(bg)
	x0, x1 := cellX+mx0, cellX+mx1
	for my := my0; my < my1; my++ {
		y := cellY + my
		r := bg*c.cart.MapH + y
		if r+1 >= len(rows) {
			return
		}
		lo, end := int(rows[r]), int(rows[r+1])
		for hi := end; lo < hi; {
			mid := int(uint(lo+hi) >> 1)
			if int(instances[mid].X) < x0 {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		for i := lo; i < end; i++ {
			x := int(instances[i].X)
			if x >= x1 {
				break
			}
			id := c.mapCursorCell(&cur, x, y)
			if link := c.tileLinkOf(id); link != 0 && (mask == 0 || c.cart.Flags[id]&mask == mask) {
				c.drawTileLink(link, worldX+(x-cellX)*8, worldY+my*8)
			}
		}
	}
}

// drawTileLinksScan reads every visible cell. A layer uses it once Mset has
// placed a linked tile on a cell the build-time index does not cover.
//
//go:noinline
//go:section .iwram
func (c *Context) drawTileLinksScan(bg, cellX, cellY, mx0, mx1, my0, my1, worldX, worldY int, mask byte) {
	cur := c.newMapCursor(bg)
	for my := my0; my < my1; my++ {
		y := cellY + my
		for mx := mx0; mx < mx1; mx++ {
			id := c.mapCursorCell(&cur, cellX+mx, y)
			if link := c.tileLinkOf(id); link != 0 && (mask == 0 || c.cart.Flags[id]&mask == mask) {
				c.drawTileLink(link, worldX+mx*8, worldY+my*8)
			}
		}
	}
}

// tileLinkMutated runs after every map write. Writing a linked tile to a cell
// the build-time index does not list switches that layer to scanning, which
// keeps every placement exact at a small per-draw cost.
func (c *Context) tileLinkMutated(layer, x, y int, id uint16) {
	if c.cart == nil || layer < 0 || layer >= 8 || c.tileLinkOf(id) == 0 || c.tileLinkIndexed(layer, x, y) {
		return
	}
	c.tileLinkScanLayers |= 1 << uint(layer)
}

func (c *Context) tileLinkIndexed(layer, x, y int) bool {
	rows := c.cart.TileLinkRows
	r := layer*c.cart.MapH + y
	if y < 0 || y >= c.cart.MapH || r+1 >= len(rows) {
		return false
	}
	for i := int(rows[r]); i < int(rows[r+1]); i++ {
		if int(c.cart.TileLinkInstances[i].X) == x {
			return true
		}
	}
	return false
}
