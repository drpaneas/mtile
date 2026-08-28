//go:build tinygo

package hw

import "unsafe"

// dmaFillWord lives in IWRAM BSS so DMA3 can use a fixed 32-bit source
// without fetching that word from ROM on every beat.
var dmaFillWord uint32

// Fill32 writes words copies of value through DMA3 with a fixed source.
// Control 0x8500 is enable | 32-bit | source-fixed. A zero count must not
// reach DMA3CNT: on GBA that encoding means 65536 transfers.
func Fill32(dst []byte, value uint32, words int) {
	if words <= 0 || words > 0xFFFF || len(dst) < words*4 {
		return
	}
	dmaFillWord = value
	SetReg32(DMA3SAD, uint32(uintptr(unsafe.Pointer(&dmaFillWord))))
	SetReg32(DMA3DAD, uint32(uintptr(unsafe.Pointer(&dst[0]))))
	SetReg16(DMA3CNT, uint16(words))
	SetReg16(DMA3CNT+2, 0x8500)
	for GetReg16(DMA3CNT+2)&0x8000 != 0 {
	}
}
