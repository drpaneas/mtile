package runtime

import "testing"

func TestHeapAllocationClearsOnlyRoundedPayload(t *testing.T) {
	for _, shift := range []uint32{0, 1, 2, 3} {
		for _, size := range []uint16{0, 1, 2, 3, 4, 7, 15, 16, 17, 63, 64, 65, 392, 820, 3080} {
			var c Context
			c.ensureHeap()
			space := c.activeSpace()
			for i := range space {
				space[i] = 0xa5
			}
			start := uint32(16) + shift
			c.heap.cursor = start
			ref, err := c.heap.allocObject(ObjString, size)
			if err != HeapOK {
				t.Fatalf("shift%d size%d: %v", shift, size, err)
			}
			rounded := alignUp4(uint32(size))
			header := c.heap.handles[ref]
			if header.Offset != start || header.Size != size || c.heap.cursor != start+rounded {
				t.Fatalf("allocation metadata changed shift%d size%d", shift, size)
			}
			for i, b := range space {
				want := byte(0xa5)
				if uint32(i) >= start && uint32(i) < start+rounded {
					want = 0
				}
				if b != want {
					t.Fatalf("shift%d size%d byte%d=%02x want%02x", shift, size, i, b, want)
				}
			}
		}
	}
}

func TestHeapZeroLengthAllocationAtSpaceEnd(t *testing.T) {
	var c Context
	c.ensureHeap()
	c.heap.cursor = heapSize
	if _, err := c.heap.allocObject(ObjString, 0); err != HeapOK {
		t.Fatal(err)
	}
	if c.heap.cursor != heapSize {
		t.Fatal("zero-byte allocation changed cursor")
	}
}
