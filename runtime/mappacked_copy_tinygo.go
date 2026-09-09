//go:build tinygo && !mtile_mode0 && !mtile_tilecanvas

package runtime

import "unsafe"

// copyMapPackedSpans composites Y-sorted opaque runs from a word-aligned
// packed source. Ordinary pixel offsets are word-aligned; a high Length bit
// instead fills the run with the source color held in Offset. The bounds are
// source coordinates already intersected with the framebuffer after applying
// originOffset; source and destination buffers must cover those clipped runs.
//
//export mtile_copy_map_packed_spans
func copyMapPackedSpans(src, dst, spans unsafe.Pointer, count, originOffset, x0, x1, y0, y1 int)
