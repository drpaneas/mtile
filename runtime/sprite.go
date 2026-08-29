package runtime

import "unsafe"

// emptySpriteCell backs an implicit compact-sheet slot when color 0 has been
// made opaque with palt(0,false). In the default transparent configuration
// empty slots reject before touching it.
var emptySpriteCell [64]byte

// spriteCellRecip returns m = ceil(2^32 / cols) for the divide-free cell
// split below. ARM7TDMI has no divide instruction; TinyGo lowers % and /
// to a ROM-resident shift-subtract loop costing hundreds of cycles, paid
// per map cell and per sprite. One umull against this reciprocal replaces
// both. Exact for id < 2^16 and cols < 2^16: the approximation error is
// id*e/(cols*2^32) < 2^-16, and a true quotient's fractional part is at
// most (cols-1)/cols <= 1 - 2^-16, so the floor can never be bumped.
// cols == 1 would overflow the uint32; callers special-case it.
func spriteCellRecip(cols int) uint32 {
	return uint32(((1 << 32) + uint64(cols) - 1) / uint64(cols))
}

// spriteCellSplit converts sprite id to (cellX, cellY) using the
// reciprocal from spriteCellRecip.
func spriteCellSplit(id int, cols int, recip uint32) (int, int) {
	if cols == 1 {
		return 0, id
	}
	cellY := int(uint32((uint64(uint32(id)) * uint64(recip)) >> 32))
	return id - cellY*cols, cellY
}

// Spr is an immediate software blit from one or more 8x8 sprite cells.
// Source transparency is tested before draw remapping (palt). A sprite ID is
// an int at the API boundary, but asset validation limits IDs to the uint16
// range. A future Mode 0 backend may lower compatible calls to hardware
// OBJ/metasprites only after proving the constraints in docs/render-modes.md.
func (c *Context) Spr(id, x, y, w, h int, flipX, flipY bool) {
	if c == nil || c.cart == nil || id < 0 || id >= c.cart.SpriteN ||
		c.cart.SpriteCols <= 0 || w <= 0 || h <= 0 {
		return
	}
	c.ensureDrawState()
	c.renderSprite(id, x, y, w, h, flipX, flipY)
}

// packedHasZeroByte reports whether any of v's four little-endian bytes is
// zero. The 8x8 identity blit uses this to memcpy opaque words and fall back
// per byte when color 0 must keep the destination.
func packedHasZeroByte(v uint32) bool {
	return (v-0x01010101)&^v&0x80808080 != 0
}

// blitPackedKey0 copies one 8-pixel row as two 32-bit words. It takes raw
// base pointers instead of slices so the inner loop carries no per-word
// bounds checks; blitSpriteRow's callers guarantee src+8 <= len(sheet) and
// dst+8 <= len(fb) via the cell clamps in blitUnflippedSpriteCells / Map.
func blitPackedKey0(sheetPtr, fbPtr unsafe.Pointer, src, dst int) {
	srcW := unsafe.Add(sheetPtr, src)
	dstW := unsafe.Add(fbPtr, dst)
	w0 := *(*uint32)(srcW)
	if !packedHasZeroByte(w0) {
		*(*uint32)(dstW) = w0
	} else {
		for col := 0; col < 4; col++ {
			if source := *(*byte)(unsafe.Add(srcW, col)); source != 0 {
				*(*byte)(unsafe.Add(dstW, col)) = source
			}
		}
	}
	w1 := *(*uint32)(unsafe.Add(srcW, 4))
	if !packedHasZeroByte(w1) {
		*(*uint32)(unsafe.Add(dstW, 4)) = w1
	} else {
		for col := 4; col < 8; col++ {
			if source := *(*byte)(unsafe.Add(srcW, col)); source != 0 {
				*(*byte)(unsafe.Add(dstW, col)) = source
			}
		}
	}
}

// blitSpriteRow writes one full 8-pixel row. The per-byte loops use raw
// pointers: TinyGo emits a bounds check per slice store, and on the GBA
// that check runs once per pixel in the hottest loop the runtime has. The
// callers guarantee src+8 <= len(sheet) and dst+8 <= len(fb) (full-row
// calls only happen after the cell clamps).
func blitSpriteRow(sheet, fb []byte, pal *[256]byte, transp *[256]bool, src, dst int, identity, key0, packed bool) {
	if packed {
		blitPackedKey0(unsafe.Pointer(&sheet[0]), unsafe.Pointer(&fb[0]), src, dst)
		return
	}
	srcP := unsafe.Add(unsafe.Pointer(&sheet[0]), src)
	dstP := unsafe.Add(unsafe.Pointer(&fb[0]), dst)
	if key0 && identity {
		// The dominant configuration (default palt, no pal remap): dedicated
		// loop so the per-pixel path tests one register, not two flags.
		for col := 0; col < 8; col++ {
			if source := *(*byte)(unsafe.Add(srcP, col)); source != 0 {
				*(*byte)(unsafe.Add(dstP, col)) = source
			}
		}
		return
	}
	for col := 0; col < 8; col++ {
		source := *(*byte)(unsafe.Add(srcP, col))
		if key0 {
			if source == 0 {
				continue
			}
		} else if transp[source] {
			continue
		}
		if identity {
			*(*byte)(unsafe.Add(dstP, col)) = source
		} else {
			*(*byte)(unsafe.Add(dstP, col)) = pal[source]
		}
	}
}

func compactSheetPixel(sheet []byte, index []uint16, cols, x, y int) (byte, bool) {
	cell := (y/8)*cols + x/8
	if cell < 0 || cell >= len(index) {
		return 0, false
	}
	slot := index[cell]
	if slot == 0 {
		return 0, true
	}
	return sheet[(int(slot)-1)*64+(y&7)*8+(x&7)], true
}

func writeSpritePixel(fb []byte, pal *[256]byte, transp *[256]bool, dst int, source byte, identity, key0 bool) {
	if key0 {
		if source == 0 {
			return
		}
	} else if transp[source] {
		return
	}
	if identity {
		fb[dst] = source
		return
	}
	fb[dst] = pal[source]
}

// blitUnflippedCell8 copies one unflipped 8x8 cell from (srcX0,srcY0) onto
// the framebuffer at screen (baseX,baseY). rowY0/rowY1 and colX0/colX1 are
// half-open cell-local ranges already clipped to [0,8). packed is ignored
// unless the full 8-wide row is written; callers must only set it when
// source and destination X are 4-byte aligned.
//
// This is the pixel workhorse for both spr() and map(). It executes from
// IWRAM: TinyGo emits 32-bit ARM opcodes, and fetching those through the
// 16-bit GamePak bus costs two ROM accesses per instruction, which
// benchspike measured at ~77 cycles per blitted pixel for the whole map()
// path. IWRAM is 32-bit zero-wait, so the same loops fetch in one cycle.
// The noinline directive keeps the body out of ROM-resident callers; the
// leaf helpers it calls are small enough that TinyGo inlines them here.
//
//go:noinline
//go:section .iwram
func blitUnflippedCell8(sheet, fb []byte, pal *[256]byte, transp *[256]bool, sheetW, srcX0, srcY0, baseX, baseY, rowY0, rowY1, colX0, colX1 int, identity, key0, packed bool) {
	fullX := colX0 == 0 && colX1 == 8
	usePacked := packed && fullX
	for row := rowY0; row < rowY1; row++ {
		src := (srcY0+row)*sheetW + srcX0
		dst := (baseY+row)*ScreenWidth + baseX
		if fullX {
			blitSpriteRow(sheet, fb, pal, transp, src, dst, identity, key0, usePacked)
			continue
		}
		for col := colX0; col < colX1; col++ {
			writeSpritePixel(fb, pal, transp, dst+col, sheet[src+col], identity, key0)
		}
	}
}

// blitFlippedCell8 executes from IWRAM for the same reason as
// blitUnflippedCell8; it is the whole-cell fast path for flipped spr().
//
//go:noinline
//go:section .iwram
func blitFlippedCell8(sheet, fb []byte, pal *[256]byte, transp *[256]bool, sheetW, srcX0, srcY0, baseX, baseY, rowY0, rowY1, colX0, colX1 int, identity, key0, flipX, flipY bool) {
	for row := rowY0; row < rowY1; row++ {
		sy := row
		if flipY {
			sy = 7 - row
		}
		src := (srcY0+sy)*sheetW + srcX0
		dst := (baseY+row)*ScreenWidth + baseX
		for col := colX0; col < colX1; col++ {
			sx := col
			if flipX {
				sx = 7 - col
			}
			writeSpritePixel(fb, pal, transp, dst+col, sheet[src+sx], identity, key0)
		}
	}
}

// blitUnflippedSpriteCells walks the cell grid for spr() and clamps each
// cell against the sheet and clip bounds. It executes from IWRAM because
// benchspike showed the ROM-resident walk cost as much as the pixel work:
// an 8x8 spr() spent ~2,000 of its ~3,000 cycles outside the blit core.
//
//go:noinline
//go:section .iwram
func blitUnflippedSpriteCells(sheet, fb []byte, pal *[256]byte, transp *[256]bool, sheetW, sheetH, srcX0, srcY0, baseX, baseY, drawX0, drawX1, drawY0, drawY1, w, h int, identity, key0 bool) {
	for cy := 0; cy < h; cy++ {
		spriteY := cy * 8
		cellSrcY := srcY0 + spriteY
		if cellSrcY >= sheetH {
			continue
		}
		rowY0, rowY1 := 0, 8
		if cellSrcY+8 > sheetH {
			rowY1 = sheetH - cellSrcY
		}
		if drawY0 > spriteY {
			if drawY0 >= spriteY+8 {
				continue
			}
			if v := drawY0 - spriteY; v > rowY0 {
				rowY0 = v
			}
		}
		if drawY1 < spriteY+8 {
			if drawY1 <= spriteY {
				continue
			}
			if v := drawY1 - spriteY; v < rowY1 {
				rowY1 = v
			}
		}
		if rowY0 >= rowY1 {
			continue
		}
		for cx := 0; cx < w; cx++ {
			spriteX := cx * 8
			cellSrcX := srcX0 + spriteX
			if cellSrcX >= sheetW {
				continue
			}
			colX0, colX1 := 0, 8
			if cellSrcX+8 > sheetW {
				colX1 = sheetW - cellSrcX
			}
			if drawX0 > spriteX {
				if drawX0 >= spriteX+8 {
					continue
				}
				if v := drawX0 - spriteX; v > colX0 {
					colX0 = v
				}
			}
			if drawX1 < spriteX+8 {
				if drawX1 <= spriteX {
					continue
				}
				if v := drawX1 - spriteX; v < colX1 {
					colX1 = v
				}
			}
			if colX0 >= colX1 {
				continue
			}
			cellBaseX := baseX + spriteX
			packed := identity && key0 && cellSrcX&3 == 0 && sheetW&3 == 0 && cellBaseX&3 == 0 &&
				colX0 == 0 && colX1 == 8
			blitUnflippedCell8(sheet, fb, pal, transp, sheetW, cellSrcX, cellSrcY, cellBaseX, baseY+spriteY,
				rowY0, rowY1, colX0, colX1, identity, key0, packed)
		}
	}
}

func advanceNearest(pos, remainder, delta, den int) (int, int) {
	remainder += delta
	for remainder >= den {
		remainder -= den
		pos++
	}
	return pos, remainder
}

// drawSprite is the inner implementation for Spr. Unflipped rectangles are a
// grid of blitUnflippedCell8 calls, so spr(n,x,y,2,2) shares the map() 8x8
// path. Flipped 8x8 cells have a dedicated row loop; larger flipped rects
// keep the general sampler because flip is over the whole rectangle, not
// each cell.
//
// Executes from IWRAM: the id->cell divides (ARM7TDMI has no divide
// instruction; % and / call a software routine) and the eight clip clamps
// run on every spr() call before a single pixel moves.
//
//go:noinline
//go:section .iwram
func (c *Context) drawSprite(id, x, y, w, h int, flipX, flipY bool) {
	cols := c.cart.SpriteCols
	srcX0, srcY0 := 0, 0
	if !c.cart.SheetReadOnly {
		cellX, cellY := spriteCellSplit(id, cols, spriteCellRecip(cols))
		srcX0 = cellX * 8
		srcY0 = cellY * 8
	}
	srcW := w * 8
	srcH := h * 8
	baseX := x - c.camX
	baseY := y - c.camY
	drawX0, drawX1 := 0, srcW
	drawY0, drawY1 := 0, srcH
	if baseX < c.clipX0 {
		drawX0 = c.clipX0 - baseX
	}
	if baseX+srcW > c.clipX1 {
		drawX1 = c.clipX1 - baseX
	}
	if baseY < c.clipY0 {
		drawY0 = c.clipY0 - baseY
	}
	if baseY+srcH > c.clipY1 {
		drawY1 = c.clipY1 - baseY
	}
	if drawX0 < 0 {
		drawX0 = 0
	}
	if drawY0 < 0 {
		drawY0 = 0
	}
	if drawX1 > srcW {
		drawX1 = srcW
	}
	if drawY1 > srcH {
		drawY1 = srcH
	}
	if drawX0 >= drawX1 || drawY0 >= drawY1 {
		return
	}

	sheet := c.cart.Sheet
	sheetW := c.cart.SheetW
	sheetH := c.cart.SheetH
	if c.cart.SheetReadOnly {
		// Compact immutable sheets store one contiguous 8x8 cell per authored
		// ID. Multi-cell spr() rectangles are not contiguous there, so walk
		// logical cells and reuse the single-cell path. Empty slots are free.
		if w > 1 || h > 1 {
			for cy := 0; cy < h; cy++ {
				for cx := 0; cx < w; cx++ {
					logical := id + cy*cols + cx
					if logical >= c.cart.SpriteN {
						continue
					}
					cellX, cellY := cx, cy
					if flipX {
						cellX = w - 1 - cx
					}
					if flipY {
						cellY = h - 1 - cy
					}
					c.drawSprite(logical, x+cellX*8, y+cellY*8, 1, 1, flipX, flipY)
				}
			}
			return
		}
		if id >= len(c.cart.SheetIndex) {
			return
		}
		slot := c.cart.SheetIndex[id]
		if slot == 0 {
			if c.transpKey0 {
				return
			}
			sheet = emptySpriteCell[:]
			sheetW, sheetH, srcY0 = 8, 8, 0
		} else {
			sheetW = 8
			sheetH = len(sheet)
			srcY0 = (int(slot) - 1) * 8
		}
	}
	fb := c.framebuffer[:]
	pal := &c.drawPal
	transp := &c.transp
	identity := c.drawPalIdentity
	key0 := c.transpKey0

	if !flipX && !flipY {
		blitUnflippedSpriteCells(sheet, fb, pal, transp, sheetW, sheetH, srcX0, srcY0, baseX, baseY, drawX0, drawX1, drawY0, drawY1, w, h, identity, key0)
		return
	}
	if w == 1 && h == 1 && srcX0 >= 0 && srcY0 >= 0 && srcX0+8 <= sheetW && srcY0+8 <= sheetH {
		blitFlippedCell8(sheet, fb, pal, transp, sheetW, srcX0, srcY0, baseX, baseY,
			drawY0, drawY1, drawX0, drawX1, identity, key0, flipX, flipY)
		return
	}

	for dy := drawY0; dy < drawY1; dy++ {
		sy := dy
		if flipY {
			sy = srcH - 1 - dy
		}
		srcY := srcY0 + sy
		if srcY < 0 || srcY >= sheetH {
			continue
		}
		row := srcY * sheetW
		dstRow := (baseY + dy) * ScreenWidth
		for dx := drawX0; dx < drawX1; dx++ {
			sx := dx
			if flipX {
				sx = srcW - 1 - dx
			}
			srcX := srcX0 + sx
			if srcX < 0 || srcX >= sheetW {
				continue
			}
			writeSpritePixel(fb, pal, transp, dstRow+baseX+dx, sheet[row+srcX], identity, key0)
		}
	}
}

// SSpr blits a source pixel rectangle, optionally stretching it to a
// destination rectangle using nearest-neighbor sampling.
//
// Executes from IWRAM: the sampler runs per destination pixel with no
// packed fast path, so instruction fetch dominates even more than in the
// cell blitters. Stretching stays the most expensive way to put pixels on
// screen; see docs/performance.md.
//
//go:noinline
//go:section .iwram
func (c *Context) SSpr(sx, sy, sw, sh, dx, dy, dw, dh int, flipX, flipY bool) {
	if c == nil || c.cart == nil || sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return
	}
	c.ensureDrawState()
	if c.renderSSpr(sx, sy, sw, sh, dx, dy, dw, dh, flipX, flipY) {
		return
	}
	baseX := dx - c.camX
	baseY := dy - c.camY
	drawX0, drawX1 := 0, dw
	drawY0, drawY1 := 0, dh
	if baseX < c.clipX0 {
		drawX0 = c.clipX0 - baseX
	}
	if baseX+dw > c.clipX1 {
		drawX1 = c.clipX1 - baseX
	}
	if baseY < c.clipY0 {
		drawY0 = c.clipY0 - baseY
	}
	if baseY+dh > c.clipY1 {
		drawY1 = c.clipY1 - baseY
	}
	if drawX0 < 0 {
		drawX0 = 0
	}
	if drawY0 < 0 {
		drawY0 = 0
	}
	if drawX1 > dw {
		drawX1 = dw
	}
	if drawY1 > dh {
		drawY1 = dh
	}
	if drawX0 >= drawX1 || drawY0 >= drawY1 {
		return
	}

	sheet := c.cart.Sheet
	sheetW := c.cart.SheetW
	sheetH := c.cart.SheetH
	fb := c.framebuffer[:]
	pal := &c.drawPal
	transp := &c.transp
	identity := c.drawPalIdentity
	key0 := c.transpKey0

	// Preserve the original dense-sheet sampler as a separate hot path. A
	// shared callback here made TinyGo generate substantially slower code even
	// when SheetReadOnly was false.
	if !c.cart.SheetReadOnly {
		sourceY := drawY0 * sh / dh
		yRemainder := (drawY0 * sh) % dh
		for oy := drawY0; oy < drawY1; oy++ {
			sampleY := sourceY
			if flipY {
				sampleY = sh - 1 - sourceY
			}
			readY := sy + sampleY
			if readY >= 0 && readY < sheetH {
				sourceX := drawX0 * sw / dw
				xRemainder := (drawX0 * sw) % dw
				row := readY * sheetW
				dstRow := (baseY + oy) * ScreenWidth
				for ox := drawX0; ox < drawX1; ox++ {
					sampleX := sourceX
					if flipX {
						sampleX = sw - 1 - sourceX
					}
					readX := sx + sampleX
					if readX >= 0 && readX < sheetW {
						writeSpritePixel(fb, pal, transp, dstRow+baseX+ox, sheet[row+readX], identity, key0)
					}
					sourceX, xRemainder = advanceNearest(sourceX, xRemainder, sw, dw)
				}
			}
			sourceY, yRemainder = advanceNearest(sourceY, yRemainder, sh, dh)
		}
		return
	}

	// Generate nearest-neighbour source coordinates with an integer remainder
	// accumulator. This is exactly floor(ox*sw/dw) and floor(oy*sh/dh). The
	// common 1:1 and 2x cases overflow by one denominator per pixel, so a
	// subtract loop avoids a division on every destination pixel.
	sourceY := drawY0 * sh / dh
	yRemainder := (drawY0 * sh) % dh
	for oy := drawY0; oy < drawY1; oy++ {
		sampleY := sourceY
		if flipY {
			sampleY = sh - 1 - sourceY
		}
		readY := sy + sampleY
		if readY >= 0 && readY < sheetH {
			sourceX := drawX0 * sw / dw
			xRemainder := (drawX0 * sw) % dw
			dstRow := (baseY + oy) * ScreenWidth
			for ox := drawX0; ox < drawX1; ox++ {
				sampleX := sourceX
				if flipX {
					sampleX = sw - 1 - sourceX
				}
				readX := sx + sampleX
				if pixel, ok := compactSheetPixel(sheet, c.cart.SheetIndex, c.cart.SpriteCols, readX, readY); ok {
					writeSpritePixel(fb, pal, transp, dstRow+baseX+ox, pixel, identity, key0)
				}
				sourceX, xRemainder = advanceNearest(sourceX, xRemainder, sw, dw)
			}
		}
		sourceY, yRemainder = advanceNearest(sourceY, yRemainder, sh, dh)
	}
}
