//go:build !mtile_mode0 && !mtile_tilecanvas

package runtime

import "unsafe"

// Uniform runs store their source color in Offset and need no pixel payload.
const mapPackedUniformFlag uint16 = 0x8000

func (c *Context) invalidateMapPacked(bg int) {
	if bg >= 0 && bg < 4 {
		c.mapPackedReady[bg] = false
		c.mapPackedFailed[bg] = false
		c.mapPackedSpanCount[bg] = 0
		return
	}
	c.mapPackedReady = [4]bool{}
	c.mapPackedFailed = [4]bool{}
	c.mapPackedSpanCount = [4]int{}
}

// PrepareMapPacked builds optional source caches after game initialization.
// It leaves framebuffer pixels and all draw state unchanged. Runtime map
// mutations disable only the affected plane until this method runs again;
// drawing never rebuilds a cache and therefore cannot cause a cold-build spike.
func (c *Context) PrepareMapPacked() {
	if c == nil || c.cart == nil {
		return
	}
	for bg := 0; bg < c.cart.MapLayers && bg < 4; bg++ {
		if slot, ok := mapPlaneSlot(c.cart.MapPlaneMask, bg, c.cart.MapLayers); ok {
			c.buildMapPacked(bg, slot)
		}
	}
}

func packedMapColor(cart *AssetMemory, planeBase, x, y int) byte {
	id := int(cart.Map[planeBase+(y>>3)*cart.MapW+(x>>3)])
	if id == 0 || id >= cart.SpriteN || id >= len(cart.SheetIndex) {
		return 0
	}
	slot := int(cart.SheetIndex[id])
	if slot == 0 {
		return 0
	}
	return cart.Sheet[(slot-1)*64+(y&7)*8+(x&7)]
}

// buildMapPacked performs no allocation. Every layer has independent bounded
// storage; an edit that exceeds either capacity retains the live cell renderer.
//
//go:noinline
func (c *Context) buildMapPacked(bg, storageBG int) bool {
	cart := c.cart
	if cart == nil || bg < 0 || bg >= 4 || cart.MapStreaming || (!sheetStorageReadOnly && !cart.SheetReadOnly) || len(cart.MapPackedPixels) == 0 || len(cart.MapPackedSpans) == 0 {
		return false
	}
	if c.mapPackedFailed[bg] {
		return false
	}
	if c.mapPackedReady[bg] {
		return true
	}
	layer := cart.MapPackedLayers[bg]
	if layer.PixelStart >= layer.PixelEnd || layer.SpanStart >= layer.SpanEnd || int(layer.PixelEnd) > len(cart.MapPackedPixels) || int(layer.SpanEnd) > len(cart.MapPackedSpans) {
		return false
	}
	w, h := cart.MapW*8, cart.MapH*8
	if w <= 0 || h <= 0 || w > 32767 || h > 65535 {
		return false
	}
	pos, count := int(layer.PixelStart), int(layer.SpanStart)
	planeBase := storageBG * cart.MapW * cart.MapH
	for y := 0; y < h; y++ {
		for x := 0; x < w; {
			if packedMapColor(cart, planeBase, x, y) == 0 {
				x++
				continue
			}
			// Inspect the complete run before reserving pixels: a long solid
			// run fits even when its plane has only the small edit allowance.
			start := x
			first := packedMapColor(cart, planeBase, x, y)
			uniform := true
			for x < w {
				color := packedMapColor(cart, planeBase, x, y)
				if color == 0 {
					break
				}
				if color != first {
					uniform = false
				}
				x++
			}
			if count >= int(layer.SpanEnd) {
				c.mapPackedFailed[bg] = true
				return false
			}
			length := uint16(x - start)
			offset := uint16(first)
			if uniform {
				length |= mapPackedUniformFlag
			} else {
				pos = (pos + 3) &^ 3
				offset = uint16(pos)
				if x-start > int(layer.PixelEnd)-pos {
					c.mapPackedFailed[bg] = true
					return false
				}
				for sx := start; sx < x; sx++ {
					cart.MapPackedPixels[pos] = packedMapColor(cart, planeBase, sx, y)
					pos++
				}
			}
			cart.MapPackedSpans[count] = MapPackedSpan{uint16(start), uint16(y), length, offset}
			count++
		}
	}
	c.mapPackedSpanCount[bg] = count - int(layer.SpanStart)
	c.mapPackedReady[bg] = true
	return true
}

// drawMapPacked accepts only identity palette and source-zero transparency
// calls. Its caller has intersected the cell rectangle with map and clip bounds.
//
//go:noinline
//go:section .iwram
func (c *Context) drawMapPacked(bg, storageBG, cellX, cellY, mx0, mx1, my0, my1, baseX, baseY int) bool {
	if bg < 0 || bg >= 4 || !c.mapPackedReady[bg] {
		return false
	}
	if len(c.cart.MapPackedPixels) == 0 {
		return false
	}
	originX, originY := baseX-cellX*8, baseY-cellY*8
	x0 := max((cellX+mx0)*8, c.clipX0-originX)
	x1 := min((cellX+mx1)*8, c.clipX1-originX)
	y0 := max((cellY+my0)*8, c.clipY0-originY)
	y1 := min((cellY+my1)*8, c.clipY1-originY)
	src := unsafe.Pointer(&c.cart.MapPackedPixels[0])
	dst := unsafe.Pointer(&c.framebuffer[0])
	if uintptr(src)&3 != 0 {
		return false
	}
	start := int(c.cart.MapPackedLayers[bg].SpanStart)
	copyMapPackedSpans(src, dst, unsafe.Pointer(&c.cart.MapPackedSpans[start]), c.mapPackedSpanCount[bg], originY*ScreenWidth+originX, x0, x1, y0, y1)
	return true
}
