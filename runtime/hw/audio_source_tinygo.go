//go:build tinygo

package hw

import "unsafe"

func validFIFOSourceRange(samples []uint32) bool {
	if len(samples) == 0 {
		return false
	}
	start := uintptr(unsafe.Pointer(&samples[0]))
	end := start + uintptr(len(samples))*4
	return start >= 0x02000000 && end <= 0x02040000 ||
		start >= 0x03000000 && end <= 0x03008000
}
