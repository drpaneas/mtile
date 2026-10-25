package runtime

import "unsafe"

// storeFramebufferByte writes one indexed pixel. Mode 4 renders straight into
// the hidden VRAM page, where the hardware turns an 8-bit store into the same
// byte in both halves of its halfword. Mode 4 therefore merges the byte into
// its aligned halfword; RAM-backed framebuffers store the byte directly.
func storeFramebufferByte(p unsafe.Pointer, value byte) {
	if !framebufferInVRAM {
		*(*byte)(p) = value
		return
	}
	odd := uintptr(p) & 1
	half := (*uint16)(unsafe.Add(p, -int(odd)))
	if odd == 0 {
		*half = *half&0xFF00 | uint16(value)
	} else {
		*half = *half&0x00FF | uint16(value)<<8
	}
}

// setFramebufferByte is storeFramebufferByte for a checked slice index.
func setFramebufferByte(fb []byte, index int, value byte) {
	storeFramebufferByte(unsafe.Pointer(&fb[index]), value)
}
