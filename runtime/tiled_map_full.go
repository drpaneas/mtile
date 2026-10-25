//go:build mtile_mode0 || mtile_tilecanvas

package runtime

import "github.com/drpaneas/mtile/runtime/hw"

// tryMode0FullMap caches the complete finite call rectangle in the existing
// 64x32 screen shadow when hardware wrapping cannot expose a repeated copy.
// An unchanged rectangle then needs only scroll-register updates, including
// whole-tile camera movement. No extra tilemap storage is allocated.
//
// The hardware repeats every 512x256 pixels. The viewport must start after
// the end of the preceding copy (w*8-512, h*8-256), and end before the next
// copy (512, 256). Outside those bounds the viewport materializer preserves
// the finite map() semantics. Custom clips use that materializer as well.
func (c *Context) tryMode0FullMap(slot, bg, cx, cy, x, y, w, h, flags int) bool {
	left, top := c.camX-x-cx*8, c.camY-y-cy*8
	if c.clipX0 != 0 || c.clipY0 != 0 || c.clipX1 != ScreenWidth || c.clipY1 != ScreenHeight ||
		left < w*8-512 || left+ScreenWidth > 512 || top < h*8-256 || top+ScreenHeight > 256 {
		if c.mode0MapFullCache[slot] {
			c.mode0MapSigSet[slot] = false
		}
		c.mode0MapFullCache[slot] = false
		return false
	}
	c.mode0MapCalls++
	c.mode0MapSeen |= 1 << uint(slot)
	c.mode0ScrollX[slot], c.mode0ScrollY[slot] = uint16(left&511), uint16(top&255)
	var mask byte
	if flags > 0 && flags <= 255 {
		mask = byte(flags)
	}
	if c.mode0MapFullCache[slot] && c.mode0MapSigSet[slot] && c.mode0MapSource[slot] == uint8(bg) &&
		c.mode0MapCellX[slot] == cx && c.mode0MapCellY[slot] == cy &&
		c.mode0MapCellsW[slot] == w && c.mode0MapCellsH[slot] == h && c.mode0MapLayer[slot] == mask {
		return true
	}
	screen := c.mode0Screen[slot][:]
	hw.Fill32(screen, 0, len(screen)/4)
	storage, present := mapPlaneSlot(c.cart.MapPlaneMask, bg, c.cart.MapLayers)
	streaming := mapStorageStreaming || !mapStorageFixed && c.cart.MapStreaming
	if present && !streaming {
		// A regular 64x32 BG is two separate 32x32 screen blocks. Split each
		// clipped row at column 32; each half then becomes a contiguous copy.
		mapW, mapH := c.cart.MapW, c.cart.MapH
		left, right := max(0, -cx), min(w, mapW-cx)
		top, bottom := max(0, -cy), min(h, mapH-cy)
		limit := min(c.cart.SpriteN, mode0TileLimit)
		cells, flags := c.cart.Map, c.cart.Flags
		for my := top; my < bottom; my++ {
			row := (storage*mapH+cy+my)*mapW + cx
			if end := min(right, 32); left < end {
				entry := mode0ScreenEntry(left, my) * 2
				mode0CopyMapSpan(screen[entry:entry+(end-left)*2], cells[row+left:row+end], flags, limit, mask)
				c.hideMode0LinkedSpan(screen[entry:entry+(end-left)*2], cells[row+left:row+end])
			}
			if start := max(left, 32); start < right {
				entry := mode0ScreenEntry(start, my) * 2
				mode0CopyMapSpan(screen[entry:entry+(right-start)*2], cells[row+start:row+right], flags, limit, mask)
				c.hideMode0LinkedSpan(screen[entry:entry+(right-start)*2], cells[row+start:row+right])
			}
		}
	} else if present {
		cur := c.newMapCursor(bg)
		for my := max(0, -cy); my < min(h, c.cart.MapH-cy); my++ {
			for mx := max(0, -cx); mx < min(w, c.cart.MapW-cx); mx++ {
				id := c.mapCursorCell(&cur, cx+mx, cy+my)
				if id == 0 || int(id) >= c.cart.SpriteN || id >= mode0TileLimit || mask != 0 && c.cart.Flags[id]&mask != mask || c.tileLinkOf(id) != 0 {
					continue
				}
				entry := mode0ScreenEntry(mx, my) * 2
				screen[entry], screen[entry+1] = byte(id), byte(id>>8)
			}
		}
	}
	c.mode0MapFullCache[slot] = true
	c.mode0MapCellX[slot], c.mode0MapCellY[slot] = cx, cy
	c.mode0MapTileX[slot], c.mode0MapTileY[slot] = 0, 0
	c.mode0MapCellsW[slot], c.mode0MapCellsH[slot] = w, h
	c.mode0MapLayer[slot], c.mode0MapSource[slot] = mask, uint8(bg)
	c.mode0MapSigSet[slot], c.mode0MapReady[slot], c.mode0MapDirty[slot] = true, true, true
	return true
}

// hideMode0LinkedSpan blanks linked tiles after a span copy; their objects
// are drawn by drawTileLinks. Carts without links skip the pass entirely.
func (c *Context) hideMode0LinkedSpan(dst []byte, src []uint16) {
	if len(c.cart.TileLinkOf) == 0 {
		return
	}
	for i, id := range src {
		if c.tileLinkOf(id) != 0 {
			dst[i*2], dst[i*2+1] = 0, 0
		}
	}
}
