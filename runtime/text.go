package runtime

// Print renders a native 4x5 PICO-8 wide glyph in a 6x8 cell. The cell size
// deliberately divides the native 240x160 display into exactly 40 columns
// and 20 rows, leaving the final row of each cell as spacing. Glyphs cover
// ASCII 32-126. Basic print() control codes are handled without allocating;
// unsupported controls are ignored. Wrap uses the current clip width.

const (
	glyphW = 6
	glyphH = 8

	glyphBitmapW = 4
	glyphBitmapH = 5
)

// font4x5 is generated from fonts/pico-8-wide.otf/pico-8-wide.otf at its native
// 4x5 pixel size; antialiased pixels at >=50% coverage are kept. The 6x8 cell
// preserves mtile's 40-column/20-row print grid.
//
//go:section .rodata
var font4x5 = [95][4]byte{
	// 32-47: ASCII 32 through 47.
	{0x00, 0x00, 0x00, 0x00},
	{0x00, 0x17, 0x00, 0x00},
	{0x03, 0x00, 0x03, 0x00},
	{0x1f, 0x0a, 0x1f, 0x00},
	{0x0b, 0x1f, 0x0d, 0x00},
	{0x19, 0x04, 0x13, 0x00},
	{0x1f, 0x17, 0x1c, 0x00},
	{0x02, 0x01, 0x00, 0x00},
	{0x0e, 0x11, 0x00, 0x00},
	{0x00, 0x11, 0x0e, 0x00},
	{0x15, 0x0e, 0x15, 0x00},
	{0x04, 0x0e, 0x04, 0x00},
	{0x10, 0x08, 0x00, 0x00},
	{0x04, 0x04, 0x04, 0x00},
	{0x00, 0x10, 0x00, 0x00},
	{0x10, 0x0e, 0x01, 0x00},
	// 48-63: ASCII 48 through 63.
	{0x1f, 0x11, 0x1f, 0x00},
	{0x11, 0x1f, 0x10, 0x00},
	{0x1d, 0x15, 0x17, 0x00},
	{0x11, 0x15, 0x1f, 0x00},
	{0x07, 0x04, 0x1f, 0x00},
	{0x17, 0x15, 0x1d, 0x00},
	{0x1f, 0x14, 0x1c, 0x00},
	{0x01, 0x01, 0x1f, 0x00},
	{0x1f, 0x15, 0x1f, 0x00},
	{0x07, 0x05, 0x1f, 0x00},
	{0x00, 0x0a, 0x00, 0x00},
	{0x10, 0x0a, 0x00, 0x00},
	{0x04, 0x0a, 0x11, 0x00},
	{0x0a, 0x0a, 0x0a, 0x00},
	{0x11, 0x0a, 0x04, 0x00},
	{0x01, 0x15, 0x07, 0x00},
	// 64-79: ASCII 64 through 79.
	{0x0e, 0x11, 0x16, 0x00},
	{0x1f, 0x05, 0x1f, 0x00},
	{0x1f, 0x15, 0x1b, 0x00},
	{0x0e, 0x11, 0x11, 0x00},
	{0x1f, 0x11, 0x1e, 0x00},
	{0x1f, 0x15, 0x11, 0x00},
	{0x1f, 0x05, 0x01, 0x00},
	{0x1e, 0x11, 0x19, 0x00},
	{0x1f, 0x04, 0x1f, 0x00},
	{0x11, 0x1f, 0x11, 0x00},
	{0x11, 0x1f, 0x01, 0x00},
	{0x1f, 0x04, 0x1b, 0x00},
	{0x1f, 0x10, 0x10, 0x00},
	{0x1f, 0x03, 0x1f, 0x00},
	{0x1f, 0x01, 0x1e, 0x00},
	{0x1e, 0x11, 0x0f, 0x00},
	// 80-95: ASCII 80 through 95.
	{0x1f, 0x05, 0x07, 0x00},
	{0x0e, 0x19, 0x16, 0x00},
	{0x1f, 0x05, 0x1b, 0x00},
	{0x16, 0x15, 0x0d, 0x00},
	{0x01, 0x1f, 0x01, 0x00},
	{0x0f, 0x10, 0x1f, 0x00},
	{0x0f, 0x18, 0x0f, 0x00},
	{0x1f, 0x18, 0x1f, 0x00},
	{0x1b, 0x04, 0x1b, 0x00},
	{0x17, 0x14, 0x1f, 0x00},
	{0x19, 0x15, 0x13, 0x00},
	{0x1f, 0x11, 0x00, 0x00},
	{0x01, 0x0e, 0x10, 0x00},
	{0x00, 0x11, 0x1f, 0x00},
	{0x02, 0x01, 0x02, 0x00},
	{0x10, 0x10, 0x10, 0x00},
	// 96-111: ASCII 96 through 111.
	{0x00, 0x01, 0x02, 0x00},
	{0x1e, 0x0a, 0x1e, 0x00},
	{0x1e, 0x16, 0x18, 0x00},
	{0x1e, 0x12, 0x12, 0x00},
	{0x1e, 0x12, 0x0c, 0x00},
	{0x1e, 0x16, 0x12, 0x00},
	{0x1e, 0x06, 0x02, 0x00},
	{0x1e, 0x12, 0x1a, 0x00},
	{0x1e, 0x08, 0x1e, 0x00},
	{0x12, 0x1e, 0x12, 0x00},
	{0x12, 0x1e, 0x02, 0x00},
	{0x1e, 0x04, 0x1a, 0x00},
	{0x1e, 0x10, 0x10, 0x00},
	{0x1e, 0x06, 0x1e, 0x00},
	{0x1e, 0x02, 0x1c, 0x00},
	{0x1c, 0x12, 0x0e, 0x00},
	// 112-126: ASCII 112 through 126.
	{0x1e, 0x0a, 0x0e, 0x00},
	{0x0c, 0x1a, 0x14, 0x00},
	{0x1e, 0x0a, 0x16, 0x00},
	{0x14, 0x12, 0x0a, 0x00},
	{0x02, 0x1e, 0x02, 0x00},
	{0x0e, 0x10, 0x1e, 0x00},
	{0x0e, 0x18, 0x0e, 0x00},
	{0x1e, 0x18, 0x1e, 0x00},
	{0x1a, 0x04, 0x1a, 0x00},
	{0x16, 0x14, 0x1e, 0x00},
	{0x1a, 0x12, 0x16, 0x00},
	{0x04, 0x1f, 0x11, 0x00},
	{0x00, 0x1f, 0x00, 0x00},
	{0x11, 0x1f, 0x04, 0x00},
	{0x0c, 0x04, 0x06, 0x00},
}

func glyphColumns(ch byte) [4]byte {
	if ch < 32 || ch > 126 {
		ch = '?'
	}
	return font4x5[ch-32]
}

func gluTextParam(ch byte) int {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0')
	case ch >= 'a' && ch <= 'z':
		return int(ch-'a') + 10
	case ch >= 'A' && ch <= 'Z':
		return int(ch-'A') + 10
	default:
		return 0
	}
}

func (c *Context) Print(s string, x, y, color int) int {
	if color < 0 || color > 255 {
		return x
	}
	c.ensureDrawState()
	c.cursor.color = byte(color)
	return c.printSeq(s, nil, x, y, c.drawPal[c.cursor.color], false, true)
}

func (c *Context) PrintCursor(s string) int {
	c.ensureDrawState()
	return c.printSeq(s, nil, c.cursor.x, c.cursor.y, c.drawPal[c.cursor.color], true, false)
}

// PrintStaticV is the generated-code path for a proven literal string. It is
// intentionally separate from PrintV so a heapless target has no reachable
// Tostr/NewString branch and TinyGo can omit the Lua semispaces completely.
// Invalid/non-static input fails closed; the transpiler emits this call only
// for values initialized through InternStatic.
func (c *Context) PrintStaticV(s, x, y, color Value) int {
	if result, handled := c.hardwarePrintStatic(s, x, y, color); handled {
		return result
	}
	c.ensureDrawState()
	implicitPosition := x.Kind != KindNumber || y.Kind != KindNumber
	if x.Kind == KindNumber && y.Kind != KindNumber && color.Kind != KindNumber {
		color = x
		implicitPosition = true
		x = NilVal()
		y = NilVal()
	}
	col := int(c.cursor.color)
	if color.Kind == KindNumber {
		col = color.Int()
	}
	if col < 0 || col > 255 {
		return x.Int()
	}
	c.cursor.color = byte(col)
	if s.Kind != KindStaticString || int(s.SRef) >= int(c.staticN) {
		return x.Int()
	}
	bytes := c.staticStringBytes(s)
	if implicitPosition {
		return c.printSeq("", bytes, c.cursor.x, c.cursor.y, c.drawPal[c.cursor.color], true, false)
	}
	return c.printSeq("", bytes, x.Int(), y.Int(), c.drawPal[c.cursor.color], false, true)
}

func (c *Context) PrintV(s, x, y, color Value) int {
	if result, handled := c.hardwarePrint(s, x, y, color); handled {
		return result
	}
	c.ensureDrawState()
	// Mtile's two-argument form is print(str, color), while the three- and
	// four-argument forms are print(str, x, y [, color]). The transpiler fills
	// omitted arguments with nil Values, so identify the short form here.
	implicitPosition := x.Kind != KindNumber || y.Kind != KindNumber
	if x.Kind == KindNumber && y.Kind != KindNumber && color.Kind != KindNumber {
		color = x
		implicitPosition = true
		x = NilVal()
		y = NilVal()
	}
	col := int(c.cursor.color)
	if color.Kind == KindNumber {
		col = color.Int()
	}
	if col < 0 || col > 255 {
		return x.Int()
	}
	c.cursor.color = byte(col)
	if !isStringKind(s.Kind) {
		if !c.PushTemp(s) {
			return x.Int()
		}
		str, err := c.Tostr(s, false)
		c.PopTemp()
		if err != HeapOK {
			return x.Int()
		}
		s = str
	}
	if implicitPosition {
		return c.printSeq("", c.stringBytes(s), c.cursor.x, c.cursor.y, c.drawPal[c.cursor.color], true, false)
	}
	return c.printSeq("", c.stringBytes(s), x.Int(), y.Int(), c.drawPal[c.cursor.color], false, true)
}

func printSeqLen(s string, b []byte) int {
	if b != nil {
		return len(b)
	}
	return len(s)
}

func printSeqByte(s string, b []byte, i int) byte {
	if b != nil {
		return b[i]
	}
	return s[i]
}

func (c *Context) printSeq(s string, b []byte, x, y int, value byte, autoNewline, resetCursorToStart bool) int {
	c.ensureDrawState()
	sx := x - c.camX
	sy := y - c.camY
	left := sx
	if left < c.clipX0 {
		left = c.clipX0
	}
	background := byte(0)
	hasBackground := false
	terminated := false
	rightMost := sx
	n := printSeqLen(s, b)
	for i := 0; i < n; i++ {
		ch := printSeqByte(s, b, i)
		switch ch {
		case 0:
			terminated = true
			i = n
			continue
		case 1:
			if i+2 >= n {
				continue
			}
			count := gluTextParam(printSeqByte(s, b, i+1))
			repeated := printSeqByte(s, b, i+2)
			i += 2
			for repeat := 0; repeat < count; repeat++ {
				c.printGlyph(repeated, &sx, &sy, left, value, background, hasBackground)
				if sx > rightMost {
					rightMost = sx
				}
			}
			continue
		case 2:
			if i+1 < n {
				i++
				background = byte(gluTextParam(printSeqByte(s, b, i)))
				hasBackground = true
			}
			continue
		case 3:
			if i+1 < n {
				i++
				sx += gluTextParam(printSeqByte(s, b, i)) - 16
			}
			continue
		case 4:
			if i+1 < n {
				i++
				sy += gluTextParam(printSeqByte(s, b, i)) - 16
			}
			continue
		case 5:
			if i+2 < n {
				sx += gluTextParam(printSeqByte(s, b, i+1)) - 16
				sy += gluTextParam(printSeqByte(s, b, i+2)) - 16
				i += 2
			}
			continue
		case 8:
			sx -= glyphW
			if sx < left {
				sx = left
			}
			continue
		case '\t':
			rel := sx - left
			if rel < 0 {
				rel = 0
			}
			sx = left + (rel/(glyphW*4)+1)*glyphW*4
			continue
		case '\n':
			sx = left
			sy += glyphH
			continue
		case '\r':
			sx = left
			continue
		case 12:
			if i+1 < n {
				i++
				c.cursor.color = byte(gluTextParam(printSeqByte(s, b, i)))
				value = c.drawPal[c.cursor.color]
			}
			continue
		case 6, 7, 11, 14, 15:
			continue
		default:
			c.printGlyph(ch, &sx, &sy, left, value, background, hasBackground)
			if sx > rightMost {
				rightMost = sx
			}
		}
	}
	if autoNewline && !terminated {
		// An implicit print appends a newline. Keep the cursor's logical x at
		// the line's original left edge, even if rendering was clipped.
		c.cursor.x = x
		c.cursor.y = sy + c.camY + glyphH
	} else if resetCursorToStart {
		// An explicitly positioned print establishes the cursor's left
		// margin and y position for the following implicit print.
		c.cursor.x = x
		c.cursor.y = y
	} else {
		c.cursor.x = sx + c.camX
		c.cursor.y = sy + c.camY
	}
	return rightMost + c.camX
}

func (c *Context) printGlyph(ch byte, sx, sy *int, left int, value, background byte, hasBackground bool) {
	if *sx+glyphW > c.clipX1 && *sx > left {
		*sx = left
		*sy += glyphH
	}
	if hasBackground {
		if *sx >= c.clipX0 && *sx+glyphW <= c.clipX1 && *sy >= c.clipY0 && *sy+glyphH <= c.clipY1 {
			for row := 0; row < glyphH; row++ {
				c.fillRow(*sy+row, *sx, *sx+glyphW-1, background)
			}
		} else {
			for row := 0; row < glyphH; row++ {
				for col := 0; col < glyphW; col++ {
					c.plotClipped(*sx+col, *sy+row, background)
				}
			}
		}
	}
	c.drawGlyph(ch, *sx, *sy, value)
	*sx += glyphW
}

// drawGlyph executes from IWRAM: HUD text redraws every frame, and a full
// 40-character row was the single most expensive workload benchspike
// measured before this placement (376k cycles, 134% of a frame).
//
//go:noinline
//go:section .iwram
func (c *Context) drawGlyph(ch byte, sx, sy int, value byte) {
	columns := glyphColumns(ch)
	// Fast path: when the whole 4x5 PICO-8 wide glyph box is inside the clip rectangle
	// (the overwhelmingly common case for HUD text), skip the per-pixel clip
	// test and write straight into the framebuffer. The bits loop also exits
	// as soon as a column has no remaining set pixels.
	if sx >= c.clipX0 && sx+glyphBitmapW <= c.clipX1 && sy >= c.clipY0 && sy+glyphBitmapH <= c.clipY1 {
		base := sy*ScreenWidth + sx
		for col := 0; col < glyphBitmapW; col++ {
			// Mask to the 5 bitmap rows.
			bits := columns[col] & 0x1F
			dst := base + col
			for ; bits != 0; bits >>= 1 {
				if bits&1 != 0 {
					c.framebuffer[dst] = value
				}
				dst += ScreenWidth
			}
		}
		return
	}
	for col := 0; col < glyphBitmapW; col++ {
		bits := columns[col]
		for row := 0; row < glyphBitmapH; row++ {
			if bits&(1<<row) != 0 {
				c.plotClipped(sx+col, sy+row, value)
			}
		}
	}
}
