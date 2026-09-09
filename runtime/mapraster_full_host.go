//go:build !tinygo && !mtile_mode0 && !mtile_no_map_raster

package runtime

import "unsafe"

// Host oracle for the ARM span-list copier. Both buffers have matching
// four-byte alignment, and each four-byte record describes one opaque run.
func copyMapRasterFull(src, dst, spans unsafe.Pointer, count int) {
	for i := 0; i < count; i++ {
		span := (*MapRasterSpan)(unsafe.Add(spans, i*4))
		offset := int(span.Offset)
		copyMapRasterSpan(unsafe.Add(src, offset), unsafe.Add(dst, offset), int(span.Length))
	}
}

// Host oracle for the width-240 clipped/shifted ARM path. The source bounds
// already include cell subrectangles and the destination camera/clip limits.
func copyMapRasterShifted(src, dst, spans unsafe.Pointer, count, originOffset, x0, x1, y0, y1 int) {
	for i := 0; i < count; i++ {
		span := (*MapRasterSpan)(unsafe.Add(spans, i*4))
		y := int(span.Y)
		if y < y0 {
			continue
		}
		if y >= y1 {
			break
		}
		x := int(span.Offset) - y*ScreenWidth
		left, right := max(x, x0), min(x+int(span.Length), x1)
		if left >= right {
			continue
		}
		offset := y*ScreenWidth + left
		copyMapRasterSpan(unsafe.Add(src, offset), unsafe.Add(dst, offset+originOffset), right-left)
	}
}
