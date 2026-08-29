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
// reach DMA3CNT: on GBA that encoding means 65536 transfers.
func CopyToVRAM32DMA(src []byte, dest uintptr, words int) {
	if len(src) < words*4 || words <= 0 || words > 0xFFFF {
		return
	}
	SetReg32(DMA3SAD, uint32(uintptr(unsafe.Pointer(&src[0]))))
	SetReg32(DMA3DAD, uint32(dest))
	SetReg16(DMA3CNT, uint16(words))
	SetReg16(DMA3CNT+2, 0x8400) // enable | 32-bit | immediate start
	for GetReg16(DMA3CNT+2)&0x8000 != 0 {
	}
}
