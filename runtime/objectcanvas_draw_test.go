//go:build mtile_tilecanvas && !mtile_mode0

package runtime

import (
	"fmt"
	"testing"
)

func objectCanvasDrawContext(compact bool) *Context {
	const width, height = 32, 16
	sheet := make([]byte, width*height)
	colors := [...]byte{0, 11, 255, 96, 42}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if x/8+y/8*4 != 1 {
				sheet[y*width+x] = colors[(x+y*3)%len(colors)]
			}
		}
	}
	c := new(Context)
	c.cart = &AssetMemory{SheetW: width, SheetH: height, SpriteCols: 4, SpriteRows: 2, SpriteN: 8, Sheet: sheet, SheetReadOnly: compact}
	if compact {
		c.cart.Sheet = make([]byte, len(sheet))
		c.cart.SheetIndex = make([]uint16, 8)
		for id := 0; id < 8; id++ {
			if id == 1 {
				continue
			}
			slot := 8 - id
			c.cart.SheetIndex[id] = uint16(slot)
			for y := 0; y < 8; y++ {
				copy(c.cart.Sheet[(slot-1)*64+y*8:(slot-1)*64+y*8+8], sheet[(id/4*8+y)*width+id%4*8:(id/4*8+y)*width+id%4*8+8])
			}
		}
	}
	c.ensureDrawState()
	return c
}

// The original pixel sampler is the oracle. Remap every visible source to
// zero against a nonzero sentinel so an actual opaque-zero write is distinct
// from an untouched destination, irrespective of the source artwork colors.
func objectCanvasAlphaOracle(c *Context) {
	for i := range c.framebuffer {
		c.framebuffer[i] = 254
	}
	for i := range c.drawPal {
		c.drawPal[i] = 0
	}
	c.drawPalIdentity = false
}
func assertObjectCanvasCoverage(t *testing.T, c, oracle *Context) {
	t.Helper()
	any := false
	for offset, color := range oracle.framebuffer {
		want := color == 0
		got := c.canvasCoverage[offset>>5]&(1<<uint(offset&31)) != 0
		if got != want {
			t.Fatalf("coverage pixel(%d,%d)=%t want%t", offset%ScreenWidth, offset/ScreenWidth, got, want)
		}
		if want {
			any = true
			x, y := offset%ScreenWidth, offset/ScreenWidth
			if c.canvasCandidates[y/16]&(1<<uint(x/16)) == 0 {
				t.Fatalf("covered pixel(%d,%d) missing candidate", x, y)
			}
		}
	}
	if any != c.canvasHasPixels {
		t.Fatalf("hasPixels=%t want%t", c.canvasHasPixels, any)
	}
	for _, row := range c.canvasCandidates {
		if row&0x8000 != 0 {
			t.Fatal("candidate outside screen grid")
		}
	}
}

func TestObjectCanvasSpriteCoverageMatchesOriginalSampler(t *testing.T) {
	for _, compact := range []bool{false, true} {
		for flip := 0; flip < 4; flip++ {
			for _, id := range []int{0, 1, 3, 7} {
				for _, size := range [][2]int{{1, 1}, {2, 2}, {3, 1}} {
					for variant := 0; variant < 3; variant++ {
						t.Run(fmt.Sprintf("compact%t/flip%d/id%d/size%v/variant%d", compact, flip, id, size, variant), func(t *testing.T) {
							c, oracle := objectCanvasDrawContext(compact), objectCanvasDrawContext(compact)
							x, y := 5, 7
							for _, ctx := range []*Context{c, oracle} {
								if variant == 1 {
									ctx.Camera(2, -1)
									ctx.Clip(7, 5, 30, 27)
									ctx.PaltColor(0, false)
									x, y = -3, -2
								}
								if variant == 2 {
									ctx.PaltColor(255, true)
									ctx.PaltColor(42, true)
									x, y = 236, 155
								}
							}
							objectCanvasAlphaOracle(oracle)
							c.canvasMarkSprite(id, x, y, size[0], size[1], flip&1 != 0, flip&2 != 0)
							oracle.drawSprite(id, x, y, size[0], size[1], flip&1 != 0, flip&2 != 0)
							assertObjectCanvasCoverage(t, c, oracle)
						})
					}
				}
			}
		}
	}
}

func TestObjectCanvasSSprCoverageMatchesNearestOriginalSampler(t *testing.T) {
	sources := [][4]int{{0, 0, 8, 8}, {7, 3, 19, 11}, {-3, -2, 10, 10}, {30, 14, 8, 8}}
	for _, compact := range []bool{false, true} {
		for flip := 0; flip < 4; flip++ {
			for _, source := range sources {
				for _, size := range [][2]int{{13, 17}, {31, 9}, {5, 3}} {
					for variant := 0; variant < 2; variant++ {
						t.Run(fmt.Sprintf("compact%t/flip%d/source%v/size%v/v%d", compact, flip, source, size, variant), func(t *testing.T) {
							c, oracle := objectCanvasDrawContext(compact), objectCanvasDrawContext(compact)
							x, y := 5, 7
							for _, ctx := range []*Context{c, oracle} {
								if variant == 1 {
									ctx.Camera(2, -1)
									ctx.Clip(4, 3, 28, 19)
									ctx.PaltColor(0, false)
									ctx.PaltColor(255, true)
									x, y = -3, -2
								}
							}
							objectCanvasAlphaOracle(oracle)
							c.canvasMarkSSpr(source[0], source[1], source[2], source[3], x, y, size[0], size[1], flip&1 != 0, flip&2 != 0)
							oracle.SSpr(source[0], source[1], source[2], source[3], x, y, size[0], size[1], flip&1 != 0, flip&2 != 0)
							assertObjectCanvasCoverage(t, c, oracle)
						})
					}
				}
			}
		}
	}
}

func TestObjectCanvasExactRectangleCoverageAndPixelOffsets(t *testing.T) {
	c, oracle := objectCanvasDrawContext(false), objectCanvasDrawContext(false)
	objectCanvasAlphaOracle(oracle)
	c.Clip(3, 5, 233, 153)
	for _, rect := range [][4]int{{-4, -2, 4, 9}, {31, 6, 65, 8}, {229, 150, 245, 165}, {16, 31, 33, 49}} {
		c.markCanvasRect(rect[0], rect[1], rect[2], rect[3])
		for y := max(rect[1], 5); y <= min(rect[3], 157); y++ {
			for x := max(rect[0], 3); x <= min(rect[2], 235); x++ {
				oracle.framebuffer[y*ScreenWidth+x] = 0
			}
		}
	}
	for _, dst := range []int{-1, FramebufferSize, 0, 15, 16, 239, 240, FramebufferSize - 1} {
		c.markCanvasPixelOffset(dst)
		if dst >= 0 && dst < FramebufferSize {
			oracle.framebuffer[dst] = 0
		}
	}
	assertObjectCanvasCoverage(t, c, oracle)
}

func TestObjectCanvasInvisibleCallsDoNotCreateForeground(t *testing.T) {
	c := objectCanvasDrawContext(true)
	c.canvasMarkSprite(1, 10, 10, 1, 1, false, false) // Implicit zero tile.
	c.canvasMarkSprite(0, ScreenWidth, ScreenHeight, 1, 1, false, false)
	c.canvasMarkSSpr(0, 0, 8, 8, ScreenWidth, ScreenHeight, 16, 16, false, false)
	c.markCanvasRect(300, 200, 400, 300)
	if c.canvasHasPixels || c.canvasCoverage != ([objectCanvasCoverageWords]uint32{}) || c.canvasCandidates != ([objectCanvasRows]uint16{}) {
		t.Fatal("invisible operations started the foreground painter pass")
	}
	c.PaltColor(0, false)
	c.canvasMarkSprite(1, 10, 10, 1, 1, false, false)
	if !c.canvasHasPixels {
		t.Fatal("opaque source zero failed to begin foreground")
	}
}

func TestObjectCanvasCoveragePathsAllocateNothing(t *testing.T) {
	c := objectCanvasDrawContext(true)
	if n := testing.AllocsPerRun(10, func() {
		clear(c.canvasCoverage[:])
		clear(c.canvasCandidates[:])
		c.canvasHasPixels = false
		c.markCanvasRect(3, 5, 89, 17)
		c.markCanvasPixelOffset(239)
		c.canvasMarkSprite(0, 20, 30, 2, 2, true, true)
		c.canvasMarkSSpr(3, 2, 19, 11, 31, 27, 29, 17, false, true)
	}); n != 0 {
		t.Fatalf("coverage paths allocated %g times", n)
	}
}
