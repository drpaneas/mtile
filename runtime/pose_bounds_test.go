package runtime

// Handcrafted runtime fixtures must supply the same derived metadata as the
// asset compiler; initialization deliberately does not scan pose parts.
func poseRangeForTest(start uint32, parts []PosePart) PoseRange {
	r := PoseRange{Start: start, Count: uint16(len(parts))}
	if len(parts) == 0 {
		return r
	}
	minX, minY := int32(parts[0].X), int32(parts[0].Y)
	maxX, maxY := minX+8, minY+8
	for _, part := range parts[1:] {
		minX, minY = min(minX, int32(part.X)), min(minY, int32(part.Y))
		maxX, maxY = max(maxX, int32(part.X)+8), max(maxY, int32(part.Y)+8)
	}
	r.FlipOffsetX, r.FlipOffsetY = minX+maxX, minY+maxY
	return r
}

// Mirror a rendered image within a known rectangle, independently of pose
// offsets, object plans, source tiles, or per-part flip calculations.
func reflectedFrameForTest(source []byte, left, top, width, height int, flipX, flipY bool) []byte {
	out := append([]byte(nil), source...)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sx, sy := x, y
			if flipX {
				sx = width - 1 - x
			}
			if flipY {
				sy = height - 1 - y
			}
			out[(top+y)*ScreenWidth+left+x] = source[(top+sy)*ScreenWidth+left+sx]
		}
	}
	return out
}

type animationFlipCallForTest struct {
	name string
	draw func(*Context, int, Value, bool, bool, Value)
}

func animationFlipCallsForTest() []animationFlipCallForTest {
	return []animationFlipCallForTest{
		{"name", func(c *Context, _ int, name Value, x, y bool, elapsed Value) {
			c.AnimV(name, IntVal(80), IntVal(64), BoolVal(x), BoolVal(y), elapsed)
		}},
		{"name-site", func(c *Context, _ int, name Value, x, y bool, elapsed Value) {
			c.AnimSiteV(IntVal(0), IntVal(0), name, IntVal(80), IntVal(64), BoolVal(x), BoolVal(y), elapsed)
		}},
		{"name-base", func(c *Context, _ int, name Value, x, y bool, elapsed Value) {
			c.AnimSiteFromBaseV(IntVal(0), IntVal(0), IntVal(4), name, IntVal(80), IntVal(64), BoolVal(x), BoolVal(y), elapsed)
		}},
		{"id", func(c *Context, id int, _ Value, x, y bool, elapsed Value) {
			c.AnimIDV(id, IntVal(80), IntVal(64), BoolVal(x), BoolVal(y), elapsed)
		}},
		{"id-site", func(c *Context, id int, _ Value, x, y bool, elapsed Value) {
			c.AnimIDSiteV(IntVal(0), IntVal(0), id, IntVal(80), IntVal(64), BoolVal(x), BoolVal(y), elapsed)
		}},
		{"id-base", func(c *Context, id int, _ Value, x, y bool, elapsed Value) {
			c.AnimIDSiteFromBaseV(IntVal(0), IntVal(0), IntVal(4), id, IntVal(80), IntVal(64), BoolVal(x), BoolVal(y), elapsed)
		}},
		{"static", func(c *Context, _ int, name Value, x, y bool, elapsed Value) {
			c.AnimStaticV(name, IntVal(80), IntVal(64), BoolVal(x), BoolVal(y), elapsed)
		}},
		{"static-site", func(c *Context, _ int, name Value, x, y bool, elapsed Value) {
			c.AnimStaticSiteV(IntVal(0), IntVal(0), name, IntVal(80), IntVal(64), BoolVal(x), BoolVal(y), elapsed)
		}},
		{"static-base", func(c *Context, _ int, name Value, x, y bool, elapsed Value) {
			c.AnimStaticSiteFromBaseV(IntVal(0), IntVal(0), IntVal(4), name, IntVal(80), IntVal(64), BoolVal(x), BoolVal(y), elapsed)
		}},
	}
}
