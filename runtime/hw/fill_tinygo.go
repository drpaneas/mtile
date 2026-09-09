//go:build tinygo

package hw

import "unsafe"

// dmaFillWord lives in IWRAM BSS so DMA3 can use a fixed 32-bit source
// without fetching that word from ROM on every beat.
var dmaFillWord uint32

// Fill32 writes aligned destinations with an IWRAM ARM store-multiple loop.
// A full framebuffer costs about 62K cycles, below the fixed-source DMA3
// fill's 67K because the CPU reuses register values instead of source reads.
// Unaligned destinations retain the DMA3 fallback (0x8500: enable, 32-bit,
// fixed source). Zero must not reach DMA3CNT: it means 65536 transfers.
func Fill32(dst []byte, value uint32, words int) {
	if words <= 0 || words > 0xFFFF || len(dst) < words*4 {
		return
	}
	if uintptr(unsafe.Pointer(&dst[0]))&3 == 0 {
		fillWords(unsafe.Pointer(&dst[0]), value, words)
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

//export mtile_fill_words
func fillWords(dst unsafe.Pointer, value uint32, words int)
