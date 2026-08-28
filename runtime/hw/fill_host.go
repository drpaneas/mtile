//go:build !tinygo

package hw

import "unsafe"

// Fill32 writes words copies of value as packed 32-bit stores. Host tests
// use a CPU fill so they do not depend on DMA registers.
func Fill32(dst []byte, value uint32, words int) {
	if words <= 0 || words > 0xFFFF || len(dst) < words*4 {
		return
	}
	for i := 0; i < words; i++ {
		*(*uint32)(unsafe.Pointer(&dst[i*4])) = value
	}
}
