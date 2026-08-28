//go:build tinygo

package hw

import (
	"runtime/volatile"
	"unsafe"
)

// CopyToVRAM32IWRAM is the same unrolled 32-bit copy, placed in IWRAM so
// instruction fetch is 32-bit sequential during the VBlank window.
//
//go:noinline
//go:section .iwram
func CopyToVRAM32IWRAM(src []byte, dest uintptr, words int) {
	if words <= 0 || len(src) < words*4 {
		return
	}
	srcPtr := unsafe.Pointer(&src[0])
	dstPtr := unsafe.Pointer(dest)
	i := 0
	for i+8 <= words {
		off := i * 4
		volatile.StoreUint32((*uint32)(unsafe.Add(dstPtr, off)), *(*uint32)(unsafe.Add(srcPtr, off)))
		volatile.StoreUint32((*uint32)(unsafe.Add(dstPtr, off+4)), *(*uint32)(unsafe.Add(srcPtr, off+4)))
		volatile.StoreUint32((*uint32)(unsafe.Add(dstPtr, off+8)), *(*uint32)(unsafe.Add(srcPtr, off+8)))
		volatile.StoreUint32((*uint32)(unsafe.Add(dstPtr, off+12)), *(*uint32)(unsafe.Add(srcPtr, off+12)))
		volatile.StoreUint32((*uint32)(unsafe.Add(dstPtr, off+16)), *(*uint32)(unsafe.Add(srcPtr, off+16)))
		volatile.StoreUint32((*uint32)(unsafe.Add(dstPtr, off+20)), *(*uint32)(unsafe.Add(srcPtr, off+20)))
		volatile.StoreUint32((*uint32)(unsafe.Add(dstPtr, off+24)), *(*uint32)(unsafe.Add(srcPtr, off+24)))
		volatile.StoreUint32((*uint32)(unsafe.Add(dstPtr, off+28)), *(*uint32)(unsafe.Add(srcPtr, off+28)))
		i += 8
	}
	for i < words {
		off := i * 4
		volatile.StoreUint32((*uint32)(unsafe.Add(dstPtr, off)), *(*uint32)(unsafe.Add(srcPtr, off)))
		i++
	}
}
