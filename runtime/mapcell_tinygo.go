//go:build tinygo

package runtime

import "unsafe"

// copyMapCell8Key0 requires a contiguous, four-byte-aligned 64-byte source
// and eight fully clipped destination rows with a 240-byte stride. Source
// zero is transparent; destination alignment is unrestricted.
//
//export mtile_copy_map_cell8_key0
func copyMapCell8Key0(src, dst unsafe.Pointer)
