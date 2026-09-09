//go:build !tinygo

package runtime

import "unsafe"

// copyMapCell8Key0 composites one contiguous, four-byte-aligned 8x8 cell
// onto eight fully clipped framebuffer rows. Destination alignment is
// unrestricted; its row stride is ScreenWidth. Source zero is transparent
// and every other source byte is copied without palette remapping.
func copyMapCell8Key0(src, dst unsafe.Pointer) {
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			color := *(*byte)(unsafe.Add(src, y*8+x))
			if color != 0 {
				*(*byte)(unsafe.Add(dst, y*ScreenWidth+x)) = color
			}
		}
	}
}
