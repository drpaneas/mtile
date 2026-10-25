package runtime

func (c *Context) ensureDrawState() {
	c.ensurePaletteState()
	if c.drawReady {
		return
	}
	c.clipX1 = ScreenWidth
	c.clipY1 = ScreenHeight
	c.cursor.color = defaultDrawColor
	c.drawReady = true
}

// Camera sets the world-to-screen offset. Drawn world (x,y) appears at
// screen (x-camX, y-camY).
func (c *Context) Camera(x, y int) {
	c.camX = x
	c.camY = y
}

func (c *Context) CameraReset() {
	c.camX = 0
	c.camY = 0
}

func (c *Context) CameraPos() (x, y int) {
	return c.camX, c.camY
}

// Clip sets the screen-space clip rectangle. It is independent of camera.
// w or h < 0 is treated as 0. The rectangle is intersected with 240x160.
func (c *Context) Clip(x, y, w, h int) {
	c.ensureDrawState()
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	x0, y0 := x, y
	x1, y1 := x+w, y+h
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > ScreenWidth {
		x1 = ScreenWidth
	}
	if y1 > ScreenHeight {
		y1 = ScreenHeight
	}
	if x1 < x0 {
		x1 = x0
	}
	if y1 < y0 {
		y1 = y0
	}
	c.clipX0, c.clipY0, c.clipX1, c.clipY1 = x0, y0, x1, y1
}

func (c *Context) ClipReset() {
	c.ensureDrawState()
	c.clipX0, c.clipY0 = 0, 0
	c.clipX1, c.clipY1 = ScreenWidth, ScreenHeight
}

func (c *Context) ClipRect() (x, y, w, h int) {
	c.ensureDrawState()
	return c.clipX0, c.clipY0, c.clipX1 - c.clipX0, c.clipY1 - c.clipY0
}

func (c *Context) Color(color int) {
	if color < 0 || color > 255 {
		return
	}
	c.ensureDrawState()
	c.cursor.color = byte(color)
}

func (c *Context) Cursor(x, y, color int) {
	c.ensureDrawState()
	c.cursor.x, c.cursor.y = x, y
	if color >= 0 && color <= 255 {
		c.cursor.color = byte(color)
	}
}

// currentDrawColor returns the logical colour used when an optional draw
// colour is omitted. The caller may then pass it to a primitive, which also
// records an explicitly supplied colour in the shared draw state.
func (c *Context) currentDrawColor() int {
	c.ensureDrawState()
	return int(c.cursor.color)
}

// optionalDrawColor implements the [COL] form of the primitive APIs.
func (c *Context) optionalDrawColor(color Value) int {
	return color.IntOr(c.currentDrawColor())
}

func (c *Context) setWorldPixel(x, y int, color byte) {
	c.plotClipped(x-c.camX, y-c.camY, color)
}

func (c *Context) plotClipped(sx, sy int, color byte) {
	if sx < c.clipX0 || sx >= c.clipX1 || sy < c.clipY0 || sy >= c.clipY1 {
		return
	}
	if tileCanvasExperiment {
		c.markCanvasPixel(sx, sy)
	}
	setFramebufferByte(c.writableFramebuffer(), sy*ScreenWidth+sx, color)
}

// hlineWorld is the inner span-fill helper shared by Rect and Circfill's
// horizontal spans. Both callers already call ensureDrawState once before
// entering their plotting loop (Rect twice, Circfill's circFillSpans up to
// four times per midpoint-circle step), so it must not redo that check here
// - this used to run on every span instead of once per shape.
func (c *Context) hlineWorld(x0, x1, y int, color byte) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	sy := y - c.camY
	if sy < c.clipY0 || sy >= c.clipY1 {
		return
	}
	sx0 := x0 - c.camX
	sx1 := x1 - c.camX
	if sx0 < c.clipX0 {
		sx0 = c.clipX0
	}
	if sx1 >= c.clipX1 {
		sx1 = c.clipX1 - 1
	}
	if sx0 > sx1 {
		return
	}
	c.fillRow(sy, sx0, sx1, color)
}

// vlineWorld transforms and clips once before filling an inclusive vertical
// span. Its callers initialize draw state and remap the color before calling.
func (c *Context) vlineWorld(x, y0, y1 int, color byte) {
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	sx := x - c.camX
	if sx < c.clipX0 || sx >= c.clipX1 {
		return
	}
	sy0 := y0 - c.camY
	sy1 := y1 - c.camY
	if sy0 < c.clipY0 {
		sy0 = c.clipY0
	}
	if sy1 >= c.clipY1 {
		sy1 = c.clipY1 - 1
	}
	if sy0 > sy1 {
		return
	}
	c.fillColumn(sx, sy0, sy1, color)
}
