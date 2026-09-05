//go:build !tinygo && !mtile_mode0

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
