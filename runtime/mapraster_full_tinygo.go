//go:build tinygo && !mtile_mode0

package runtime

import "unsafe"

// The ARM routine reads Offset/Length/Y with this four-byte record stride.
var _ [4 - unsafe.Sizeof(MapRasterSpan{})]byte
var _ [unsafe.Sizeof(MapRasterSpan{}) - 4]byte

//export mtile_copy_map_raster_full
func copyMapRasterFull(src, dst, spans unsafe.Pointer, count int)
