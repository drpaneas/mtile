//go:build mtile_tilecanvas && !mtile_mode0

package runtime

// markCanvasRect records an exact, inclusive screen-space rectangle written
// by a primitive. Sprite and text bounding boxes must never call this: their
// transparent holes need the per-pixel coverage paths below.
func (c *Context) markCanvasRect(x0, y0, x1, y1 int) {
	x0, y0 = max(x0, c.clipX0), max(y0, c.clipY0)
	x1, y1 = min(x1, c.clipX1-1), min(y1, c.clipY1-1)
	if x0 > x1 || y0 > y1 {
		return
	}
	for y := y0; y <= y1; y++ {
		first, last := y*ScreenWidth+x0, y*ScreenWidth+x1
		lo, hi := first>>5, last>>5
		left, right := ^uint32(0)<<uint(first&31), ^uint32(0)>>uint(31-(last&31))
		if lo == hi {
			c.canvasCoverage[lo] |= left & right
			continue
		}
		c.canvasCoverage[lo] |= left
		for word := lo + 1; word < hi; word++ {
			c.canvasCoverage[word] = ^uint32(0)
		}
		c.canvasCoverage[hi] |= right
	}
	mask := uint16((uint32(1)<<uint((x1>>4)+1))-1) &^ uint16((uint32(1)<<uint(x0>>4))-1)
	for row := y0 >> 4; row <= y1>>4; row++ {
		c.canvasCandidates[row] |= mask
	}
	c.canvasHasPixels = true
}

// markCanvasPixelOffset is for callers that have already clipped an actual
// pixel write (for example, a set glyph bit). Draw-time palette remapping can
// produce logical zero, so coverage never depends on the destination color.
func (c *Context) markCanvasPixelOffset(dst int) {
	if dst < 0 || dst >= FramebufferSize {
		return
	}
	c.canvasCoverage[dst>>5] |= 1 << uint(dst&31)
	y := int(uint32(dst) / uint32(ScreenWidth))
	x := dst - y*ScreenWidth
	c.canvasCandidates[y>>4] |= 1 << uint(x>>4)
	c.canvasHasPixels = true
}

// A sampled row is at most 16 pixels wide and already screen-clipped. One
// mask update records all source-alpha decisions without marking sprite holes.
func (c *Context) canvasMarkSampledRow(x, y, width int, mask uint16) {
	if mask == 0 {
		return
	}
	dst := y*ScreenWidth + x
	shift := uint(dst & 31)
	c.canvasCoverage[dst>>5] |= uint32(mask) << shift
	if int(shift)+width > 32 {
		c.canvasCoverage[(dst>>5)+1] |= uint32(mask) >> (32 - shift)
	}
	cells := uint16((uint32(1)<<uint(((x+width-1)>>4)+1))-1) &^ uint16((uint32(1)<<uint(x>>4))-1)
	c.canvasCandidates[y>>4] |= cells
	c.canvasHasPixels = true
}

// canvasMarkSprite mirrors drawSprite's source geometry and alpha test, then
// lets the existing packed blitter write the pixels. Compact sheets permit
// logical metasprite cells to cross an authored row; dense sheets instead
// clip the source rectangle at the physical sheet edges.
func (c *Context) canvasMarkSprite(id, x, y, w, h int, flipX, flipY bool) {
	if c.cart == nil || id < 0 || id >= c.cart.SpriteN || c.cart.SpriteCols <= 0 || w <= 0 || h <= 0 {
		return
	}
	baseX, baseY := x-c.camX, y-c.camY
	cols := c.cart.SpriteCols
	compact := sheetStorageReadOnly || c.cart.SheetReadOnly
	sourceX, sourceY := 0, 0
	if !compact {
		cx, cy := spriteCellSplit(id, cols, spriteCellRecip(cols))
		sourceX, sourceY = cx*8, cy*8
	}
	for cy := 0; cy < h; cy++ {
		for cx := 0; cx < w; cx++ {
			dx, dy := cx, cy
			if flipX {
				dx = w - 1 - cx
			}
			if flipY {
				dy = h - 1 - cy
			}
			screenX, screenY := baseX+dx*8, baseY+dy*8
			if screenX >= c.clipX1 || screenY >= c.clipY1 || screenX+8 <= c.clipX0 || screenY+8 <= c.clipY0 {
				continue
			}
			if compact {
				logical := id + cy*cols + cx
				if logical >= c.cart.SpriteN || logical >= len(c.cart.SheetIndex) {
					continue
				}
				slot := c.cart.SheetIndex[logical]
				if slot == 0 {
					if !c.transp[0] {
						c.markCanvasRect(screenX, screenY, screenX+7, screenY+7)
					}
					continue
				}
				c.canvasMarkSpriteCell(c.cart.Sheet, 8, len(c.cart.Sheet)/8, 0, (int(slot)-1)*8, screenX, screenY, flipX, flipY)
			} else {
				c.canvasMarkSpriteCell(c.cart.Sheet, c.cart.SheetW, c.cart.SheetH, sourceX+cx*8, sourceY+cy*8, screenX, screenY, flipX, flipY)
			}
		}
	}
}

func (c *Context) canvasMarkSpriteCell(sheet []byte, sheetW, sheetH, sourceX, sourceY, x, y int, flipX, flipY bool) {
	x0, y0 := max(x, c.clipX0), max(y, c.clipY0)
	x1, y1 := min(x+8, c.clipX1), min(y+8, c.clipY1)
	for py := y0; py < y1; py++ {
		sy := py - y
		if flipY {
			sy = 7 - sy
		}
		sy += sourceY
		if sy < 0 || sy >= sheetH {
			continue
		}
		var mask uint16
		for px := x0; px < x1; px++ {
			sx := px - x
			if flipX {
				sx = 7 - sx
			}
			sx += sourceX
			if sx >= 0 && sx < sheetW && !c.transp[sheet[sy*sheetW+sx]] {
				mask |= 1 << uint(px-x0)
			}
		}
		c.canvasMarkSampledRow(x0, py, x1-x0, mask)
	}
}

// canvasMarkSSpr uses the same integer remainder progression as SSpr: the
// source sample is exactly floor(destination*sourceSize/destinationSize).
// Divisions occur once at clipped row/column initialization, never per pixel.
func (c *Context) canvasMarkSSpr(sx, sy, sw, sh, dx, dy, dw, dh int, flipX, flipY bool) {
	if c.cart == nil || sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return
	}
	baseX, baseY := dx-c.camX, dy-c.camY
	x0, y0 := max(0, c.clipX0-baseX), max(0, c.clipY0-baseY)
	x1, y1 := min(dw, c.clipX1-baseX), min(dh, c.clipY1-baseY)
	if x0 >= x1 || y0 >= y1 {
		return
	}
	sourceY, yRemainder := y0*sh/dh, (y0*sh)%dh
	firstX, firstRemainder := x0*sw/dw, (x0*sw)%dw
	compact := sheetStorageReadOnly || c.cart.SheetReadOnly
	for oy := y0; oy < y1; oy++ {
		sampleY := sourceY
		if flipY {
			sampleY = sh - 1 - sourceY
		}
		readY := sy + sampleY
		if readY >= 0 && readY < c.cart.SheetH {
			sourceX, xRemainder := firstX, firstRemainder
			for chunk := x0; chunk < x1; chunk += 16 {
				end := min(chunk+16, x1)
				var mask uint16
				for ox := chunk; ox < end; ox++ {
					sampleX := sourceX
					if flipX {
						sampleX = sw - 1 - sourceX
					}
					readX := sx + sampleX
					var color byte
					var valid bool
					if compact {
						color, valid = compactSheetPixel(c.cart.Sheet, c.cart.SheetIndex, c.cart.SpriteCols, readX, readY)
					} else if readX >= 0 && readX < c.cart.SheetW {
						color, valid = c.cart.Sheet[readY*c.cart.SheetW+readX], true
					}
					if valid && !c.transp[color] {
						mask |= 1 << uint(ox-chunk)
					}
					sourceX, xRemainder = advanceNearest(sourceX, xRemainder, sw, dw)
				}
				c.canvasMarkSampledRow(baseX+chunk, baseY+oy, end-chunk, mask)
			}
		}
		sourceY, yRemainder = advanceNearest(sourceY, yRemainder, sh, dh)
	}
}

// The caller already clipped these coordinates. Keeping x/y avoids division
// by the 240-pixel screen width for each glyph or primitive pixel.
func (c *Context) markCanvasPixel(x, y int) {
	dst := y*ScreenWidth + x
	c.canvasCoverage[dst>>5] |= 1 << uint(dst&31)
	c.canvasCandidates[y>>4] |= 1 << uint(x>>4)
	c.canvasHasPixels = true
}
