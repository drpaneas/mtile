package runtime

import (
	"bytes"
	"testing"
)

func TestCollectionPreservesOddAndLargePayloadsAcrossSpaceFlips(t *testing.T) {
	var c Context
	var roots [67]Value
	var payloads [67][]byte
	for i := range roots {
		size := i
		if i == len(roots)-1 {
			size = MaxStringBytes - 1
		}
		payloads[i] = make([]byte, size)
		for j := range payloads[i] {
			payloads[i][j] = byte((i*37 + j*19) & 255)
		}
		var err HeapError
		roots[i], err = c.NewString(payloads[i])
		if err != HeapOK {
			t.Fatal(err)
		}
		c.RegisterGlobal(&roots[i])
	}
	for pass := 0; pass < 4; pass++ {
		inactive := c.heap.space(1 - c.heap.active)
		for i := range inactive {
			inactive[i] = 0xa5
		}
		if err := c.Collect(); err != HeapOK {
			t.Fatalf("collection %d: %v", pass, err)
		}
		for i, value := range roots {
			if !bytes.Equal(c.stringBytes(value), payloads[i]) {
				t.Fatalf("collection %d changed payload %d (%d bytes)", pass, i, len(payloads[i]))
			}
			if c.heap.handles[value.Ref].State() != HandleAllocated {
				t.Fatalf("collection %d retained forwarding state for %d", pass, i)
			}
			header := c.heap.handles[value.Ref]
			for at := uint32(header.Size); at < alignUp4(uint32(header.Size)); at++ {
				if c.activeSpace()[header.Offset+at] != 0xa5 {
					t.Fatalf("collection %d copied padding after payload %d", pass, i)
				}
			}
		}
	}
}

func TestCollectionRetainsUnalignedPayloadFallback(t *testing.T) {
	var c Context
	c.ensureHeap()
	ref, err := c.heap.allocObject(ObjString, 7)
	if err != HeapOK {
		t.Fatal(err)
	}
	// Real allocations are aligned. Keep the previous collector's behavior
	// for an otherwise in-bounds header pointing at an unaligned payload.
	c.heap.handles[ref].Offset++
	data := c.activeSpace()[1:8]
	writeU16(data, 0, 5)
	copy(data[2:], []byte{1, 0, 255, 3, 4})
	root := Value{Kind: KindString, Ref: ref}
	c.RegisterGlobal(&root)
	if err := c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	if !bytes.Equal(c.stringBytes(root), []byte{1, 0, 255, 3, 4}) {
		t.Fatal("unaligned payload changed")
	}
}

func TestCollectionPreservesMixedHandleFreeListOrder(t *testing.T) {
	var c Context
	var values [8]Value
	for i := range values {
		var err HeapError
		values[i], err = c.NewString(bytes.Repeat([]byte{byte(i + 1)}, i+1))
		if err != HeapOK {
			t.Fatal(err)
		}
	}
	for _, i := range []int{1, 4, 6} {
		c.RegisterGlobal(&values[i])
	}
	for pass := 0; pass < 2; pass++ {
		if err := c.Collect(); err != HeapOK {
			t.Fatal(err)
		}
		head := c.heap.freeHead
		for _, want := range []int{7, 5, 3, 2, 0, 8} {
			if head != Ref(want) || c.heap.handles[head].State() != HandleFree {
				t.Fatalf("collection %d free-list head=%d, want %d", pass, head, want)
			}
			head = Ref(c.heap.handles[head].Offset)
		}
		for _, i := range []int{1, 4, 6} {
			if !bytes.Equal(c.stringBytes(values[i]), bytes.Repeat([]byte{byte(i + 1)}, i+1)) {
				t.Fatalf("collection %d changed live handle %d", pass, i)
			}
		}
	}
	for _, want := range []int{7, 5, 3, 2, 0} {
		v, err := c.NewString([]byte("reused"))
		if err != HeapOK || v.Ref != Ref(want) {
			t.Fatalf("reused ref=%d error=%v, want %d", v.Ref, err, want)
		}
	}
}
