//go:build !mtile_mode0

package runtime

import "testing"

func TestAxisPrimitivesMatchGeometry(t *testing.T) {
	shapes := []struct {
		name           string
		rect           bool
		x0, y0, x1, y1 int
	}{
		{"horizontal", false, 1, 7, 17, 7},
		{"vertical", false, 7, 1, 7, 17},
		{"point", false, 7, 7, 7, 7},
		{"horizontal_screen_edges", false, -12, 0, 251, 0},
		{"vertical_screen_edges", false, 239, -12, 239, 171},
		{"offscreen_horizontal", false, -20, -9, 260, -9},
		{"offscreen_vertical", false, -9, -20, -9, 180},
		{"rectangle", true, 2, 3, 18, 19},
		{"rectangle_partial_edges", true, -5, -6, 12, 13},
		{"rectangle_enclosing_clip", true, -1, -1, 240, 160},
		{"rectangle_last_pixel", true, 230, 150, 239, 159},
		{"rectangle_row", true, 1, 7, 17, 7},
		{"rectangle_column", true, 7, 1, 7, 17},
		{"rectangle_point", true, 7, 7, 7, 7},
	}
	states := []struct {
		name       string
		camX, camY int
		x, y, w, h int
	}{
		{"screen", 0, 0, 0, 0, ScreenWidth, ScreenHeight},
		{"clip", 0, 0, 4, 5, 8, 9},
		{"positive_camera", 5, 6, 0, 0, ScreenWidth, ScreenHeight},
		{"negative_camera_clip", -4, -3, 4, 5, 8, 9},
		{"clip_past_screen", 0, 0, -3, -4, 247, 168},
		{"empty_width", 0, 0, 5, 5, 0, 10},
		{"negative_height", 0, 0, 5, 5, 10, -1},
		{"clip_offscreen_left", 0, 0, -20, 0, 10, 160},
		{"clip_offscreen_right", 0, 0, 250, 0, 10, 160},
		{"clip_offscreen_below", 0, 0, 0, 170, 240, 10},
	}
	colors := []struct {
		name           string
		logical, pixel int
	}{
		{"opaque_zero", 0, 0},
		{"remapped_color", 3, 9},
		{"remapped_to_zero", 7, 0},
	}
	for _, shape := range shapes {
		for _, state := range states {
			for _, color := range colors {
				for _, reversed := range []bool{false, true} {
					name := shape.name + "/" + state.name + "/" + color.name
					if reversed {
						name += "/reversed"
					}
					t.Run(name, func(t *testing.T) {
						var c Context
						for i := range c.framebuffer {
							c.framebuffer[i] = byte(32 + i%13)
						}
						c.Color(14)
						c.Pal(color.logical, color.pixel, 0)
						c.Pal(color.pixel, 24, 1) // Display remapping must not affect stored indices.
						c.PaltColor(color.logical, true)
						c.PaltColor(color.pixel, true)
						c.Camera(state.camX, state.camY)
						c.Clip(state.x, state.y, state.w, state.h)
						x0, y0, x1, y1 := shape.x0, shape.y0, shape.x1, shape.y1
						if reversed {
							x0, y0, x1, y1 = x1, y1, x0, y0
						}
						if shape.rect {
							c.Rect(x0, y0, x1, y1, color.logical)
						} else {
							c.Line(x0, y0, x1, y1, color.logical)
						}
						if got := c.currentDrawColor(); got != color.logical {
							t.Fatalf("current draw color = %d, want logical color %d", got, color.logical)
						}
						// Decide independently whether each screen pixel belongs to the
						// world-space geometric boundary. Do not reproduce span clipping,
						// endpoint normalization, or the optimized storage loops here.
						for i, got := range c.framebuffer {
							sx, sy := i%ScreenWidth, i/ScreenWidth
							wx, wy := sx+state.camX, sy+state.camY
							inside := axisBetween(wx, x0, x1) && axisBetween(wy, y0, y1)
							boundary := !shape.rect || wx == x0 || wx == x1 || wy == y0 || wy == y1
							inClip := state.w > 0 && state.h > 0 && sx >= state.x && sy >= state.y && sx < state.x+state.w && sy < state.y+state.h
							want := byte(32 + i%13)
							if inside && boundary && inClip {
								want = byte(color.pixel)
							}
							if got != want {
								t.Fatalf("screen pixel (%d,%d) = %d, want %d", sx, sy, got, want)
							}
						}
					})
				}
			}
		}
	}
}

func axisBetween(value, endpoint0, endpoint1 int) bool {
	return value >= endpoint0 && value <= endpoint1 || value >= endpoint1 && value <= endpoint0
}

func TestAxisPrimitivesRejectInvalidColor(t *testing.T) {
	for _, rect := range []bool{false, true} {
		for _, color := range []int{-1, 256} {
			var c Context
			c.Color(14)
			c.framebuffer[7*ScreenWidth+7] = 42
			before := c.framebuffer
			if rect {
				c.Rect(7, 1, 7, 17, color)
			} else {
				c.Line(7, 1, 7, 17, color)
			}
			if c.framebuffer != before || c.currentDrawColor() != 14 {
				t.Fatalf("rect=%v color=%d changed framebuffer or current draw color", rect, color)
			}
		}
	}
}
