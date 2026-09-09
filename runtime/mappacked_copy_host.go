//go:build !tinygo && !mtile_mode0 && !mtile_tilecanvas

package runtime

import "unsafe"

func copyMapPackedSpans(src, dst, spans unsafe.Pointer, count, originOffset, x0, x1, y0, y1 int) {
	for i := 0; i < count; i++ {
		span := (*MapPackedSpan)(unsafe.Add(spans, i*8))
		y := int(span.Y)
		if y < y0 {
			continue
		}
		if y >= y1 {
			break
		}
		x := int(span.X)
		left, right := max(x, x0), min(x+int(span.Length&(mapPackedUniformFlag-1)), x1)
		for sx := left; sx < right; sx++ {
			color := byte(span.Offset)
			if span.Length&mapPackedUniformFlag == 0 {
				color = *(*byte)(unsafe.Add(src, int(span.Offset)+sx-x))
			}
			*(*byte)(unsafe.Add(dst, y*ScreenWidth+sx+originOffset)) = color
		}
	}
}
