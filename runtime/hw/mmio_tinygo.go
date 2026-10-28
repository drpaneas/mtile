//go:build tinygo

package hw

import (
	"runtime/volatile"
	"unsafe"
)

func GetReg16(addr uintptr) uint16 {
	return volatile.LoadUint16((*uint16)(unsafe.Pointer(addr)))
}

func SetReg16(addr uintptr, value uint16) {
	volatile.StoreUint16((*uint16)(unsafe.Pointer(addr)), value)
}

func GetReg32(addr uintptr) uint32 {
	return volatile.LoadUint32((*uint32)(unsafe.Pointer(addr)))
}

func SetReg32(addr uintptr, value uint32) {
	volatile.StoreUint32((*uint32)(unsafe.Pointer(addr)), value)
}

func SetVRAM32(addr uintptr, value uint32) {
	SetReg32(addr, value)
}

func ReadVRAM32(addr uintptr) uint32 {
	return GetReg32(addr)
}

func SetPaletteRAM16(index int, value uint16) {
	if index < 0 || index >= 512 {
		return
	}
	SetReg16(PALRAM+uintptr(index*2), value)
}

// SetPaletteRAM32 writes two adjacent 16-bit palette entries with one
// 32-bit store. index must be even; an odd index is a misaligned PALRAM write.
func SetPaletteRAM32(index int, lo, hi uint16) {
	if index < 0 || index >= 511 || index&1 != 0 {
		return
	}
	SetReg32(PALRAM+uintptr(index*2), uint32(lo)|uint32(hi)<<16)
}

func ReadPaletteRAM16(index int) uint16 {
	if index < 0 || index >= 512 {
		return 0
	}
	return GetReg16(PALRAM + uintptr(index*2))
}

// CopyToVRAM32DMA uses immediate DMA3. The transfer is 32-bit and increments
// both addresses, so it is safe for Mode 4 indices and does not incur 9,600
// function-call/store iterations in the VBlank window. A zero count must not
// reach DMA3CNT: on GBA that encoding means 65536 transfers. Destinations
// must be word-aligned; misaligned destinations leave memory unchanged. An
// unaligned byte source uses assembled CPU words to preserve the exact bytes.
func CopyToVRAM32DMA(src []byte, dest uintptr, words int) {
	if words <= 0 || words > 0xFFFF || len(src) < words*4 || dest&3 != 0 {
		return
	}
	source := uintptr(unsafe.Pointer(&src[0]))
	if source&3 != 0 {
		copyBytesToVRAM32(src, dest, words)
		return
	}
	copyToVRAM32DMA(source, dest, words)
}

// CopyWordsToVRAM32DMA is the aligned-ROM counterpart used by generated
// uint32 asset tables. The element type guarantees DMA3's source alignment.
// A misaligned destination is rejected before any write.
func CopyWordsToVRAM32DMA(src []uint32, dest uintptr, words int) {
	if words <= 0 || words > 0xFFFF || len(src) < words || dest&3 != 0 {
		return
	}
	copyToVRAM32DMA(uintptr(unsafe.Pointer(&src[0])), dest, words)
}

// Bound each CPU-stalling transfer below one 65,536-cycle audio timer slot.
// An EWRAM-to-VRAM word costs eight cycles; 4,096 words leave room for the
// 16,384-cycle audio IRQ budget and avoid merging two pending timer overflows.
func copyToVRAM32DMA(src, dest uintptr, words int) {
	for words > 4096 {
		dmaCopyWords(src, dest, 4096)
		src += 4096 * 4
		dest += 4096 * 4
		words -= 4096
	}
	dmaCopyWords(src, dest, words)
}

func dmaCopyWords(src, dest uintptr, words int) {
	SetReg32(DMA3SAD, uint32(src))
	SetReg32(DMA3DAD, uint32(dest))
	SetReg16(DMA3CNT, uint16(words))
	SetReg16(DMA3CNT+2, 0x8400)
	for GetReg16(DMA3CNT+2)&0x8000 != 0 {
	}
}

// ReadVideoWords32DMA captures VRAM/OAM during VBlank into aligned EWRAM.
// It uses the same bounded DMA3 path as video writes, so a backup cannot turn
// hundreds of volatile read calls into a transfer spanning active scanout.
func ReadVideoWords32DMA(dst []uint32, source uintptr) {
	if !videoReadRange(source, len(dst)) {
		return
	}
	copyToVRAM32DMA(source, uintptr(unsafe.Pointer(&dst[0])), len(dst))
}
