package transpiler

// solidSheetColors annotates immutable compact cells once, at build time.
// A zero entry deliberately makes no claim about that cell. Omit the whole
// array when no cell can use the map renderer's solid-color path.
func solidSheetColors(sheet []byte) []uint16 {
	colors := make([]uint16, len(sheet)/64)
	anySolid := false
	for slot := range colors {
		pixels := sheet[slot*64 : (slot+1)*64]
		uniform := true
		for _, pixel := range pixels[1:] {
			if pixel != pixels[0] {
				uniform = false
				break
			}
		}
		if uniform {
			colors[slot] = uint16(pixels[0]) + 1
			anySolid = true
		}
	}
	if !anySolid {
		return nil
	}
	return colors
}
