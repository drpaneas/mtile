package audio

// Sound IDs span one byte, while valid libraries may contain all 256 IDs in
// arbitrary order. A separate presence bit keeps slice position 255 usable.
// Generated library slices are immutable for the complete binding lifetime.
type nativeSoundIndex struct {
	slots   [NativeMaxSounds]uint8
	present [(NativeMaxSounds + 7) / 8]byte
}

func (index *nativeSoundIndex) insert(id uint8, slot int) bool {
	mask := byte(1 << uint(id&7))
	if index.present[id>>3]&mask != 0 {
		return false
	}
	index.present[id>>3] |= mask
	index.slots[id] = uint8(slot)
	return true
}

func (index *nativeSoundIndex) find(sounds []Sound, id uint8) *Sound {
	if index.present[id>>3]&(1<<uint(id&7)) == 0 {
		return nil
	}
	return &sounds[index.slots[id]]
}
