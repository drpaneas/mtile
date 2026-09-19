//go:build tinygo

package hw

import "unsafe"

// Fill32 writes aligned destinations with an IWRAM ARM store-multiple loop.
// A full framebuffer costs about 62K cycles, below the fixed-source DMA3
// fill's 67K because the CPU reuses register values instead of source reads.
// Unaligned RAM destinations use byte stores: both DMA32 and ARM word stores
// mask address bits, so either would overwrite bytes before the slice.
func Fill32(dst []byte, value uint32, words int) {
	if words <= 0 || words > 0xFFFF || len(dst) < words*4 {
		return
	}
	if uintptr(unsafe.Pointer(&dst[0]))&3 == 0 {
		fillWords(unsafe.Pointer(&dst[0]), value, words)
		return
	}
	for i := 0; i < words*4; i += 4 {
		dst[i] = byte(value)
		dst[i+1] = byte(value >> 8)
		dst[i+2] = byte(value >> 16)
		dst[i+3] = byte(value >> 24)
	}
}

//export mtile_fill_words
func fillWords(dst unsafe.Pointer, value uint32, words int)
