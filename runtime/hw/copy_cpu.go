package hw

import "unsafe"

// CopyToVRAM32CPU is a ROM-resident unrolled 32-bit copy. It is the slow
// baseline the presentation gate compares against DMA and the IWRAM copy.
//
//go:noinline
func CopyToVRAM32CPU(src []byte, dest uintptr, words int) {
	copyToVRAM32Unrolled(src, dest, words)
}

func copyToVRAM32Unrolled(src []byte, dest uintptr, words int) {
	if words <= 0 || len(src) < words*4 {
		return
	}
	srcPtr := unsafe.Pointer(&src[0])
	i := 0
	for i+8 <= words {
		off := uintptr(i) * 4
		SetVRAM32(dest+off, *(*uint32)(unsafe.Add(srcPtr, off)))
		SetVRAM32(dest+off+4, *(*uint32)(unsafe.Add(srcPtr, off+4)))
		SetVRAM32(dest+off+8, *(*uint32)(unsafe.Add(srcPtr, off+8)))
		SetVRAM32(dest+off+12, *(*uint32)(unsafe.Add(srcPtr, off+12)))
		SetVRAM32(dest+off+16, *(*uint32)(unsafe.Add(srcPtr, off+16)))
		SetVRAM32(dest+off+20, *(*uint32)(unsafe.Add(srcPtr, off+20)))
		SetVRAM32(dest+off+24, *(*uint32)(unsafe.Add(srcPtr, off+24)))
		SetVRAM32(dest+off+28, *(*uint32)(unsafe.Add(srcPtr, off+28)))
		i += 8
	}
	for i < words {
		off := uintptr(i) * 4
		SetVRAM32(dest+off, *(*uint32)(unsafe.Add(srcPtr, off)))
		i++
	}
}
