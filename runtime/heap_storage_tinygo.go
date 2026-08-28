//go:build tinygo

package runtime

import _ "unsafe"

// Semispaces are package-level EWRAM, not Context fields, and they use
// .ewram_heap so gba.ld places them after ctx. A leading 112 KB heap would
// slide ctx to 0x0201C080, which ARM7TDMI cannot encode as one rotated
// immediate.
//
//go:section .ewram_heap
var heapSpaceA [heapSize]byte

//go:section .ewram_heap
var heapSpaceB [heapSize]byte

func (h *Heap) initSpaces() {}

func (h *Heap) space(which int) []byte {
	if which == 0 {
		return heapSpaceA[:]
	}
	return heapSpaceB[:]
}
