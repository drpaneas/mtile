//go:build tinygo

package hw

import (
	"runtime/volatile"
	"unsafe"
)

// Word aligned, padded SRAM_Vnnn detection signature. A volatile reference
// keeps it in saving ROMs, while unused save support is dead stripped.
//
//go:section .rodata
var sramSignature = [3]uint32{0x4d415253, 0x6e6e565f, 0x0000006e}

func PrepareSRAM() {
	_ = volatile.LoadUint32(&sramSignature[0])
	const waitcnt = 0x04000204
	SetReg16(waitcnt, GetReg16(waitcnt)|3) // eight SRAM wait states
}

// SRAM has an 8-bit bus. GBATEK requires reads from WRAM; noinline prevents
// LLVM moving this load into its ROM-resident caller.
//
//go:noinline
//go:section .iwram
func ReadSRAMByte(offset int) byte {
	if uint(offset) >= 32768 {
		return 0xff
	}
	return volatile.LoadUint8((*uint8)(unsafe.Pointer(uintptr(0x0e000000) + uintptr(offset))))
}

func WriteSRAMByte(offset int, value byte) {
	if uint(offset) < 32768 {
		volatile.StoreUint8((*uint8)(unsafe.Pointer(uintptr(0x0e000000)+uintptr(offset))), value)
	}
}
