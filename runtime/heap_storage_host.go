//go:build !tinygo

package runtime

func (h *Heap) initSpaces() {
	if h.localSpaces == nil {
		h.localSpaces = new(heapSpacePair)
	}
}

func (h *Heap) space(which int) []byte {
	h.initSpaces()
	if which == 0 {
		return h.localSpaces.a[:]
	}
	return h.localSpaces.b[:]
}
