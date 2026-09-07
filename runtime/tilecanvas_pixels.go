//go:build mtile_tilecanvas && !mtile_mode0

package runtime

import "unsafe"

// packTileCanvas converts a 240x160 indexed framebuffer to 600 tile-major
// tiles in an existing RAM backing. dst needs 38400 bytes for 8bpp or 19200
// for 4bpp. Source/backing must not overlap; bits must be nonnil, and 4bpp
// requires pal. Larger slices are allowed; bytes outside these extents stay
// untouched. As with the framebuffer itself, undersized storage is a caller
// error and panics at the initial slice checks.
//
// Dirty bits accumulate: callers clear them after uploading pending tiles.
// The result counts tiles changed by this call, including already-dirty tiles.
// In 4bpp, every source index is mapped through pal, with even x in the low
// nibble. Only the low four bits of each mapping are stored. 8bpp ignores pal.
// No allocation is performed.
//
//go:noinline
//go:section .iwram
func packTileCanvas(src []byte, dst []byte, bits *[19]uint32, pal *[256]byte, fourBPP bool) (changedTiles int) {
	return packTileCanvasRows(src, dst, bits, pal, fourBPP, nil)
}

// packTileCanvasRows has the same packing and dirty-bit contracts as
// packTileCanvas, but visits only candidate tiles selected by rows. Bit x of
// rows[y] selects tile (x,y); the upper two bits are ignored. A nil mask visits
// all tiles. The mask is not modified, and unselected backing tiles are left
// untouched. Callers must include both old and new drawing bounds to erase
// moved/disappeared pixels, plus any tiles affected by palette changes.
//
//go:noinline
//go:section .iwram
func packTileCanvasRows(src []byte, dst []byte, bits *[19]uint32, pal *[256]byte, fourBPP bool, rows *[20]uint32) (changedTiles int) {
	src = src[:FramebufferSize]
	tileBytes := 64
	if fourBPP {
		tileBytes = 32
	}
	dst = dst[:600*tileBytes]
	sourceBase := unsafe.Pointer(&src[0])
	targetBase := unsafe.Pointer(&dst[0])
	if (uintptr(sourceBase)|uintptr(targetBase))&3 != 0 {
		return packTileCanvasRowsUnaligned(src, dst, bits, pal, fourBPP, rows)
	}
	var zero4 uint32
	if fourBPP {
		zero4 = uint32(pal[0]&15) * 0x11111111
	}
	for ty := 0; ty < 20; ty++ {
		rowMask := uint32(1<<30) - 1
		if rows != nil {
			rowMask &= rows[ty]
		}
		for tx := 0; rowMask != 0; tx++ {
			selected := rowMask & 1
			rowMask >>= 1
			if selected == 0 {
				continue
			}
			tile := ty*30 + tx
			sourceOffset := ty*8*ScreenWidth + tx*8
			targetOffset := tile * tileBytes
			changed := false
			if !fourBPP {
				for row := 0; row < 8; row++ {
					s := unsafe.Add(sourceBase, sourceOffset+row*ScreenWidth)
					d := unsafe.Add(targetBase, targetOffset+row*8)
					a, b := *(*uint32)(s), *(*uint32)(unsafe.Add(s, 4))
					if *(*uint32)(d) != a || *(*uint32)(unsafe.Add(d, 4)) != b {
						*(*uint32)(d) = a
						*(*uint32)(unsafe.Add(d, 4)) = b
						changed = true
					}
				}
			} else {
				for row := 0; row < 8; row++ {
					s := unsafe.Add(sourceBase, sourceOffset+row*ScreenWidth)
					d := (*uint32)(unsafe.Add(targetBase, targetOffset+row*4))
					a, b := *(*uint32)(s), *(*uint32)(unsafe.Add(s, 4))
					var packed uint32
					if a|b == 0 {
						// Transparent rear rows avoid eight palette reads.
						packed = zero4
					} else if a == b && (a^(a>>8))<<8 == 0 {
						// Solid cloud rows also need only one palette read.
						packed = uint32(pal[byte(a)]&15) * 0x11111111
					} else {
						packed = uint32(pal[byte(a)]&15) |
							uint32(pal[byte(a>>8)]&15)<<4 |
							uint32(pal[byte(a>>16)]&15)<<8 |
							uint32(pal[byte(a>>24)]&15)<<12 |
							uint32(pal[byte(b)]&15)<<16 |
							uint32(pal[byte(b>>8)]&15)<<20 |
							uint32(pal[byte(b>>16)]&15)<<24 |
							uint32(pal[byte(b>>24)]&15)<<28
					}
					if *d != packed {
						*d = packed
						changed = true
					}
				}
			}
			if changed {
				bits[tile>>5] |= uint32(1) << uint(tile&31)
				changedTiles++
			}
		}
	}
	return changedTiles
}

// Go callers may supply unaligned slices. ARM7 cannot use its word loads for
// those, so retain byte-based conversion in ROM. Keeping this uncommon path
// out of the IWRAM packer reserves fast memory for the aligned framebuffers.
// Slices have already been checked by packTileCanvasRows.
//
//go:noinline
func packTileCanvasRowsUnaligned(src []byte, dst []byte, bits *[19]uint32, pal *[256]byte, fourBPP bool, rows *[20]uint32) (changedTiles int) {
	tileBytes := 64
	if fourBPP {
		tileBytes = 32
	}
	for ty := 0; ty < 20; ty++ {
		rowMask := uint32(1<<30) - 1
		if rows != nil {
			rowMask &= rows[ty]
		}
		for tx := 0; rowMask != 0; tx++ {
			selected := rowMask & 1
			rowMask >>= 1
			if selected == 0 {
				continue
			}
			tile := ty*30 + tx
			sourceOffset := ty*8*ScreenWidth + tx*8
			targetOffset := tile * tileBytes
			changed := false
			for row := 0; row < 8; row++ {
				s := sourceOffset + row*ScreenWidth
				if fourBPP {
					d := targetOffset + row*4
					for x := 0; x < 4; x++ {
						v := pal[src[s+x*2]]&15 | (pal[src[s+x*2+1]]&15)<<4
						if dst[d+x] != v {
							dst[d+x] = v
							changed = true
						}
					}
				} else {
					d := targetOffset + row*8
					for x := 0; x < 8; x++ {
						if dst[d+x] != src[s+x] {
							dst[d+x] = src[s+x]
							changed = true
						}
					}
				}
			}
			if changed {
				bits[tile>>5] |= uint32(1) << uint(tile&31)
				changedTiles++
			}
		}
	}
	return changedTiles
}
