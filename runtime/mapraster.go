//go:build !mtile_mode0

package runtime

import "unsafe"

func (c *Context) invalidateMapRaster() {
	c.mapRasterReady = false
	c.mapRasterFailed = false
	c.mapRasterSpanCount = 0
}

// PrepareMapRaster is generated startup work, after the game's Init has
// removed object markers and before profiling/gameplay begins. It changes
// neither framebuffer pixels nor palette/camera/clip state.
func (c *Context) PrepareMapRaster() {
	if c == nil || c.cart == nil {
		return
	}
	cart := c.cart
	w, h := cart.MapW*8, cart.MapH*8
	if w < 1 || w > ScreenWidth || h < 1 || h > ScreenHeight || cart.MapStreaming ||
		!(sheetStorageReadOnly || cart.SheetReadOnly) || len(cart.Map) != cart.MapW*cart.MapH ||
		len(cart.MapRasterPixels) != w*h || len(cart.MapRasterSpans) == 0 || c.mapRasterFailed || c.mapRasterReady {
		return
	}
	for bg := 0; bg < cart.MapLayers; bg++ {
		if cart.MapPlaneMask&(1<<uint(bg)) != 0 {
			c.buildMapRaster(bg, 0)
			return
		}
	}
}

// buildMapRaster caches source indices, never draw-remapped colours. The
// bounded span list preserves every transparent hole and fails closed for
// excessively fragmented maps. No allocation is performed in the runtime.
func (c *Context) buildMapRaster(bg, storageBG int) bool {
	cart := c.cart
	w, h := cart.MapW*8, cart.MapH*8
	pixels := cart.MapRasterPixels
	clear(pixels)
	for y := 0; y < cart.MapH; y++ {
		for x := 0; x < cart.MapW; x++ {
			id := int(cart.Map[storageBG*cart.MapW*cart.MapH+y*cart.MapW+x])
			if id == 0 || id >= cart.SpriteN || id >= len(cart.SheetIndex) {
				continue
			}
			slot := int(cart.SheetIndex[id])
			if slot == 0 {
				continue
			}
			source := (slot - 1) * 64
			for row := 0; row < 8; row++ {
				dest := (y*8+row)*w + x*8
				copy(pixels[dest:dest+8], cart.Sheet[source+row*8:source+row*8+8])
			}
		}
	}
	count := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; {
			if pixels[y*w+x] == 0 {
				x++
				continue
			}
			start := x
			for x < w && pixels[y*w+x] != 0 {
				x++
			}
			if count == len(cart.MapRasterSpans) {
				c.mapRasterFailed = true
				return false
			}
			cart.MapRasterSpans[count] = MapRasterSpan{uint16(y*w + start), uint8(x - start), uint8(y)}
			count++
		}
	}
	c.mapRasterBG = bg
	c.mapRasterSpanCount = count
	c.mapRasterReady = true
	return true
}

// drawMapRaster accepts only identity-palette/key-zero calls (checked by its
// caller). Clip/camera and requested cell rectangles are applied to spans;
// the pixels behind transparent runs are never read or overwritten.
//
//go:noinline
//go:section .iwram
func (c *Context) drawMapRaster(bg, storageBG, cellX, cellY, mx0, mx1, my0, my1, baseX, baseY int) bool {
	cart := c.cart
	w, h := cart.MapW*8, cart.MapH*8
	if w < 1 || w > ScreenWidth || h < 1 || h > ScreenHeight || cart.MapStreaming ||
		!(sheetStorageReadOnly || cart.SheetReadOnly) || len(cart.Map) != cart.MapW*cart.MapH ||
		len(cart.MapRasterPixels) != w*h || len(cart.MapRasterSpans) == 0 || c.mapRasterFailed {
		return false
	}
	if !c.mapRasterReady || c.mapRasterBG != bg {
		if !c.buildMapRaster(bg, storageBG) {
			return false
		}
	}
	originX, originY := baseX-cellX*8, baseY-cellY*8
	x0 := max((cellX+mx0)*8, c.clipX0-originX)
	x1 := min((cellX+mx1)*8, c.clipX1-originX)
	y0 := max((cellY+my0)*8, c.clipY0-originY)
	y1 := min((cellY+my1)*8, c.clipY1-originY)
	src := unsafe.Pointer(&cart.MapRasterPixels[0])
	dst := unsafe.Pointer(&c.framebuffer[0])
	// This also makes the rounded-down loads used by shifted copies safe at
	// the first span. Generated EWRAM arrays are aligned; unusual Go callers
	// may supply an intentionally unaligned byte slice and retain the old path.
	if uintptr(src)&3 != 0 {
		return false
	}
	if w == ScreenWidth && originX == 0 && originY == 0 && x0 == 0 && x1 == w && y0 == 0 && y1 == h && uintptr(dst)&3 == 0 {
		copyMapRasterFull(src, dst, unsafe.Pointer(&cart.MapRasterSpans[0]), c.mapRasterSpanCount)
		return true
	}
	if w == ScreenWidth && uintptr(dst)&3 == 0 {
		copyMapRasterShifted(src, dst, unsafe.Pointer(&cart.MapRasterSpans[0]), c.mapRasterSpanCount,
			originY*ScreenWidth+originX, x0, x1, y0, y1)
		return true
	}
	for i := 0; i < c.mapRasterSpanCount; i++ {
		span := cart.MapRasterSpans[i]
		y := int(span.Y)
		if y < y0 {
			continue
		}
		if y >= y1 {
			break
		}
		x := int(span.Offset) - y*w
		left := max(x, x0)
		right := min(x+int(span.Length), x1)
		if left >= right {
			continue
		}
		source := unsafe.Add(src, y*w+left)
		target := unsafe.Add(dst, (y+originY)*ScreenWidth+left+originX)
		copyMapRasterSpan(source, target, right-left)
	}
	return true
}

// The caller clips both pointers to allocated byte arrays. ARM7TDMI cannot
// load/store an arbitrary unaligned word. Align destination writes, then
// combine two aligned source words for an exact shifted camera copy. The
// source's enclosing raster starts at a four-byte boundary (checked above).
//
//go:noinline
//go:section .iwram
func copyMapRasterSpan(src, dst unsafe.Pointer, n int) {
	for n > 0 && uintptr(dst)&3 != 0 {
		*(*byte)(dst) = *(*byte)(src)
		src = unsafe.Add(src, 1)
		dst = unsafe.Add(dst, 1)
		n--
	}
	if uintptr(src)&3 == 0 {
		for n >= 16 {
			*(*uint32)(dst) = *(*uint32)(src)
			*(*uint32)(unsafe.Add(dst, 4)) = *(*uint32)(unsafe.Add(src, 4))
			*(*uint32)(unsafe.Add(dst, 8)) = *(*uint32)(unsafe.Add(src, 8))
			*(*uint32)(unsafe.Add(dst, 12)) = *(*uint32)(unsafe.Add(src, 12))
			src = unsafe.Add(src, 16)
			dst = unsafe.Add(dst, 16)
			n -= 16
		}
		for n >= 4 {
			*(*uint32)(dst) = *(*uint32)(src)
			src = unsafe.Add(src, 4)
			dst = unsafe.Add(dst, 4)
			n -= 4
		}
	} else if n >= 8 {
		shift := uint(uintptr(src)&3) * 8
		aligned := unsafe.Add(src, -int(uintptr(src)&3))
		low := *(*uint32)(aligned)
		for n >= 8 {
			high := *(*uint32)(unsafe.Add(aligned, 4))
			*(*uint32)(dst) = low>>shift | high<<(32-shift)
			low = high
			aligned = unsafe.Add(aligned, 4)
			src = unsafe.Add(src, 4)
			dst = unsafe.Add(dst, 4)
			n -= 4
		}
	}
	for n > 0 {
		*(*byte)(dst) = *(*byte)(src)
		src = unsafe.Add(src, 1)
		dst = unsafe.Add(dst, 1)
		n--
	}
}
