package runtime

import (
	"unsafe"

	"github.com/drpaneas/mtile/runtime/hw"
)

// ensurePaletteState lazily initializes zero-valued Contexts. This keeps the
// generated .ewram globals zero-initialized while making a stack Context useful
// in host tests immediately.
func (c *Context) ensurePaletteState() {
	if c.paletteReady {
		return
	}
	for i := 0; i < 256; i++ {
		c.drawPal[i] = byte(i)
		c.displayPal[i] = byte(i)
	}
	copy(c.palette[:], defaultPalette[:])
	c.transp[0] = true
	c.paletteReady = true
	c.paletteDirty = true
	c.transpKey0 = true
	c.drawPalIdentity = true
}

func (c *Context) pixelRaw(x, y int) byte {
	if x < 0 || x >= ScreenWidth || y < 0 || y >= ScreenHeight {
		return 0
	}
	return c.framebuffer[y*ScreenWidth+x]
}

// Pset writes an opaque framebuffer pixel after draw-time palette remapping.
// Palette and transparency state do not affect primitive opacity. Its exact
// composed-image semantics are Mode 4-only; see docs/render-modes.md.
func (c *Context) Pset(x, y, color int) {
	if color < 0 || color > 255 {
		return
	}
	c.ensureDrawState()
	c.cursor.color = byte(color)
	c.setWorldPixel(x, y, c.drawPal[color])
}

// Pget reads the logical index currently stored in the software framebuffer.
// Camera applies; clip does not.
func (c *Context) Pget(x, y int) byte {
	return c.pixelRaw(x-c.camX, y-c.camY)
}

// Cls explicitly clears the persistent framebuffer, restores the default
// clip rectangle, and resets the print cursor to (0,0). It deliberately
// preserves the current draw colour. There is no implicit clear during
// presentation. Hardware uses DMA3 with a fixed source so a 60 Hz cls() is
// one fill, not 9,600 ROM-resident word stores.
// A future Mode 0 backend may preserve this only for a proven frame-initial
// clear; later or persistent-framebuffer clears require Mode 4.
func (c *Context) Cls(color int) {
	if color < 0 || color > 255 {
		return
	}
	c.clearRender(color)
}

func (c *Context) clearMode4(color int) {
	c.ensureDrawState()
	c.ClipReset()
	c.cursor.x, c.cursor.y = 0, 0
	value := c.drawPal[color]
	word := uint32(value) | uint32(value)<<8 | uint32(value)<<16 | uint32(value)<<24
	hw.Fill32(c.framebuffer[:], word, FramebufferSize/4)
}

// Rectfill draws an inclusive rectangle, normalizing reversed corners and
// clipping to the current clip rectangle after the camera offset.
func (c *Context) Rectfill(x0, y0, x1, y1, color int) {
	if color < 0 || color > 255 {
		return
	}
	c.ensureDrawState()
	c.cursor.color = byte(color)
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	c.hlineSpan(x0, x1, y0, y1, c.drawPal[color])
}

func (c *Context) hlineSpan(x0, x1, y0, y1 int, value byte) {
	sx0 := x0 - c.camX
	sx1 := x1 - c.camX
	sy0 := y0 - c.camY
	sy1 := y1 - c.camY
	if sx0 < c.clipX0 {
		sx0 = c.clipX0
	}
	if sy0 < c.clipY0 {
		sy0 = c.clipY0
	}
	if sx1 >= c.clipX1 {
		sx1 = c.clipX1 - 1
	}
	if sy1 >= c.clipY1 {
		sy1 = c.clipY1 - 1
	}
	if sx0 > sx1 || sy0 > sy1 {
		return
	}
	word := uint32(value) | uint32(value)<<8 | uint32(value)<<16 | uint32(value)<<24
	// Full-width spans are one contiguous framebuffer run, so a single DMA3
	// fill replaces the per-row word loop. This makes a full-screen rectfill
	// cost the same as cls (~67k cycles) instead of ~183k. ScreenWidth is a
	// multiple of 4, so the run is always word-sized, and the largest
	// possible run (38,400 bytes = 9,600 words) fits DMA3's 16-bit count.
	if sx0 == 0 && sx1 == ScreenWidth-1 {
		start := sy0 * ScreenWidth
		hw.Fill32(c.framebuffer[start:], word, (sy1-sy0+1)*ScreenWidth/4)
		return
	}
	for y := sy0; y <= sy1; y++ {
		c.fillRowWord(y, sx0, sx1, value, word)
	}
}

func (c *Context) fillRow(y, x0, x1 int, value byte) {
	word := uint32(value) | uint32(value)<<8 | uint32(value)<<16 | uint32(value)<<24
	c.fillRowWord(y, x0, x1, value, word)
}

// fillRowWord executes from IWRAM: it is the span workhorse under
// Rectfill, Circfill, Rect's horizontal edges, and print's background,
// and its word loop runs 60 iterations per full-width row. See
// blitUnflippedCell8 for why IWRAM placement matters on this bus.
//
//go:noinline
//go:section .iwram
func (c *Context) fillRowWord(y, x0, x1 int, value byte, word uint32) {
	row := y * ScreenWidth
	for x := x0; x <= x1 && x&3 != 0; x++ {
		c.framebuffer[row+x] = value
	}
	for x := x0 + ((4 - (x0 & 3)) & 3); x+3 <= x1; x += 4 {
		*(*uint32)(unsafe.Pointer(&c.framebuffer[row+x])) = word
	}
	for x := x1 - ((x1 + 1) & 3) + 1; x <= x1; x++ {
		if x >= x0 {
			c.framebuffer[row+x] = value
		}
	}
}

// Line draws an inclusive Bresenham segment. Color 0 is opaque.
func (c *Context) Line(x0, y0, x1, y1, color int) {
	if color < 0 || color > 255 {
		return
	}
	c.ensureDrawState()
	c.cursor.color = byte(color)
	value := c.drawPal[color]
	dx := x1 - x0
	if dx < 0 {
		dx = -dx
	}
	dy := y1 - y0
	if dy < 0 {
		dy = -dy
	}
	stepX := 1
	if x0 > x1 {
		stepX = -1
	}
	stepY := 1
	if y0 > y1 {
		stepY = -1
	}
	x, y := x0, y0
	c.setWorldPixel(x, y, value)
	if dx > dy {
		err := dx / 2
		for x != x1 {
			x += stepX
			err -= dy
			if err < 0 {
				y += stepY
				err += dx
			}
			c.setWorldPixel(x, y, value)
		}
		return
	}
	err := dy / 2
	for y != y1 {
		y += stepY
		err -= dx
		if err < 0 {
			x += stepX
			err += dy
		}
		c.setWorldPixel(x, y, value)
	}
}

// Rect draws an inclusive 1-pixel outline.
func (c *Context) Rect(x0, y0, x1, y1, color int) {
	if color < 0 || color > 255 {
		return
	}
	c.ensureDrawState()
	c.cursor.color = byte(color)
	value := c.drawPal[color]
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	c.hlineWorld(x0, x1, y0, value)
	c.hlineWorld(x0, x1, y1, value)
	for y := y0 + 1; y < y1; y++ {
		c.setWorldPixel(x0, y, value)
		c.setWorldPixel(x1, y, value)
	}
}

func (c *Context) circPlot(cx, cy, x, y int, value byte) {
	c.setWorldPixel(cx+x, cy+y, value)
	c.setWorldPixel(cx-x, cy+y, value)
	c.setWorldPixel(cx+x, cy-y, value)
	c.setWorldPixel(cx-x, cy-y, value)
	c.setWorldPixel(cx+y, cy+x, value)
	c.setWorldPixel(cx-y, cy+x, value)
	c.setWorldPixel(cx+y, cy-x, value)
	c.setWorldPixel(cx-y, cy-x, value)
}

func (c *Context) circFillSpans(cx, cy, x, y int, value byte) {
	c.hlineWorld(cx-x, cx+x, cy+y, value)
	c.hlineWorld(cx-x, cx+x, cy-y, value)
	c.hlineWorld(cx-y, cx+y, cy+x, value)
	c.hlineWorld(cx-y, cx+y, cy-x, value)
}

// Circ draws a midpoint circle outline. Color 0 is opaque. r < 0 is a no-op.
func (c *Context) Circ(cx, cy, r, color int) {
	if color < 0 || color > 255 || r < 0 {
		return
	}
	c.ensureDrawState()
	c.cursor.color = byte(color)
	value := c.drawPal[color]
	if r == 0 {
		c.setWorldPixel(cx, cy, value)
		return
	}
	x, y := 0, r
	d := 1 - r
	c.circPlot(cx, cy, x, y, value)
	for x < y {
		if d < 0 {
			d += 2*x + 3
		} else {
			d += 2*(x-y) + 5
			y--
		}
		x++
		c.circPlot(cx, cy, x, y, value)
	}
}

// Circfill fills a midpoint circle. Color 0 is opaque. r < 0 is a no-op.
func (c *Context) Circfill(cx, cy, r, color int) {
	if color < 0 || color > 255 || r < 0 {
		return
	}
	c.ensureDrawState()
	c.cursor.color = byte(color)
	value := c.drawPal[color]
	if r == 0 {
		c.setWorldPixel(cx, cy, value)
		return
	}
	c.hlineWorld(cx-r, cx+r, cy, value)
	x, y := 0, r
	d := 1 - r
	c.circFillSpans(cx, cy, x, y, value)
	for x < y {
		if d < 0 {
			d += 2*x + 3
		} else {
			d += 2*(x-y) + 5
			y--
		}
		x++
		c.circFillSpans(cx, cy, x, y, value)
	}
}
