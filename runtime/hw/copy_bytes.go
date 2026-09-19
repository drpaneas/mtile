package hw

// copyBytesToVRAM32 requires validated lengths and a word-aligned destination.
// Reading individual source bytes avoids ARM7 unaligned word-load rotation.
func copyBytesToVRAM32(src []byte, dest uintptr, words int) {
	for i := 0; i < words; i++ {
		offset := i * 4
		word := uint32(src[offset]) | uint32(src[offset+1])<<8 |
			uint32(src[offset+2])<<16 | uint32(src[offset+3])<<24
		SetVRAM32(dest+uintptr(offset), word)
	}
}
