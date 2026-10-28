package hw

// Only video memory is valid for the pause backup. A zero-length DMA must not
// reach DMA3 (the hardware interprets zero as 65,536 transfers).
func videoReadRange(source uintptr, words int) bool {
	if words <= 0 || words > 0xffff || source&3 != 0 {
		return false
	}
	size := uintptr(words) * 4
	return source >= VRAM && source <= VRAM+VRAMBytes && size <= VRAM+VRAMBytes-source || source >= OAM && source <= OAM+OAMBytes && size <= OAM+OAMBytes-source
}
