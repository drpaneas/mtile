package runtime

func (c *Context) ClsV(color Value) {
	// CLS defaults to black; unlike a primitive colour argument it does not
	// change the current draw colour.
	c.Cls(color.IntOr(0))
}

func (c *Context) PsetV(x, y, color Value) {
	c.Pset(x.Int(), y.Int(), c.optionalDrawColor(color))
}

func (c *Context) PgetV(x, y Value) Value {
	return IntVal(int(c.Pget(x.Int(), y.Int())))
}

func (c *Context) LineV(x0, y0, x1, y1, color Value) {
	c.Line(x0.Int(), y0.Int(), x1.Int(), y1.Int(), c.optionalDrawColor(color))
}

func (c *Context) RectV(x0, y0, x1, y1, color Value) {
	c.Rect(x0.Int(), y0.Int(), x1.Int(), y1.Int(), c.optionalDrawColor(color))
}

func (c *Context) RectfillV(x0, y0, x1, y1, color Value) {
	c.Rectfill(x0.Int(), y0.Int(), x1.Int(), y1.Int(), c.optionalDrawColor(color))
}

func (c *Context) CircV(cx, cy, r, color Value) {
	c.Circ(cx.Int(), cy.Int(), r.Int(), c.optionalDrawColor(color))
}

func (c *Context) CircfillV(cx, cy, r, color Value) {
	c.Circfill(cx.Int(), cy.Int(), r.Int(), c.optionalDrawColor(color))
}

func (c *Context) SprV(n, x, y, w, h, flipX, flipY Value) {
	c.Spr(
		n.Int(),
		x.Int(),
		y.Int(),
		w.IntOr(1),
		h.IntOr(1),
		flipX.IsTrue(),
		flipY.IsTrue(),
	)
}

func (c *Context) PSprV(id, x, y, flipX, flipY Value) {
	c.PSpr(
		id.Int(),
		x.Int(),
		y.Int(),
		flipX.IsTrue(),
		flipY.IsTrue(),
	)
}

func (c *Context) SSprV(sx, sy, sw, sh, dx, dy, dw, dh, flipX, flipY Value) {
	c.SSpr(
		sx.Int(),
		sy.Int(),
		sw.Int(),
		sh.Int(),
		dx.Int(),
		dy.Int(),
		dw.IntOr(sw.Int()),
		dh.IntOr(sh.Int()),
		flipX.IsTrue(),
		flipY.IsTrue(),
	)
}

// MapBGAtV is the generated-code entry point for the new map contract. slot
// is a compile-time dense allocation; a is the stable logical BG0..BG3 ID.
func (c *Context) MapBGAtV(slot int, a, b, c0, d, e, f, g, h Value) {
	c.MapBGAt(slot, a.Int(), b.Int(), c0.Int(), d.Int(), e.Int(), f.Int(), g.Int(), h.Int())
}

func (c *Context) CameraV(x, y Value) {
	c.Camera(x.Int(), y.Int())
}

func (c *Context) ClipV(x, y, w, h Value) {
	c.Clip(x.Int(), y.Int(), w.IntOr(ScreenWidth), h.IntOr(ScreenHeight))
}

func (c *Context) ColorV(color Value) {
	// COLOR() resets the current colour to this runtime's default (index 12).
	c.Color(color.IntOr(defaultDrawColor))
}

func (c *Context) CursorV(x, y, color Value) {
	c.Cursor(x.Int(), y.Int(), color.IntOr(-1))
}

func (c *Context) PalV(from, to, layer Value) {
	c.Pal(from.Int(), to.Int(), layer.Int())
}

func (c *Context) PaltV(color, transparent Value) {
	c.PaltColor(color.Int(), transparent.IsTrue())
}

func (c *Context) BtnV(index Value) Value {
	return BoolVal(c.Btn(index.Int()))
}

func (c *Context) BtnpV(index Value) Value {
	return BoolVal(c.Btnp(index.Int()))
}

func (c *Context) RndV(n Value) Value {
	return NumVal(c.Rnd(n.NumberOr(One)))
}

func (c *Context) SrandV(seed Value) {
	c.Srand(seed.NumberOr(0))
}

func (c *Context) TimeV() Value {
	return NumVal(c.Time())
}

func (c *Context) StatV(n Value) Value {
	return NumVal(c.Stat(n.Int()))
}
