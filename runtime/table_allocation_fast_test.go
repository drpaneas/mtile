package runtime

import "testing"

func TestFreshTableAllocationsKeepPayloadAndErrorContract(t *testing.T) {
	for _, arrayCap := range []int{0, 1, 16, maxArrayCap} {
		for _, hashCap := range []int{0, 1, 8, maxHashCap} {
			for shift := uint32(0); shift < 4; shift++ {
				var c Context
				c.ensureHeap()
				space := c.activeSpace()
				for i := range space {
					space[i] = 0xa5
				}
				c.heap.cursor = 16 + shift
				table, err := c.NewTableWithCaps(arrayCap, hashCap)
				if err != HeapOK {
					t.Fatal(err)
				}
				data, err := c.tableData(table)
				if err != HeapOK || int(readU16(data, 2)) != arrayCap || int(readU16(data, 6)) != hashCap {
					t.Fatalf("fresh header changed caps%d/%d shift%d", arrayCap, hashCap, shift)
				}
				for i, v := range data {
					if i >= 2 && i <= 3 || i >= 6 && i <= 7 {
						continue
					}
					if v != 0 {
						t.Fatalf("fresh payload byte%d=%x", i, v)
					}
				}
			}
		}
	}
	for _, terminal := range []HeapError{HeapOOM, HeapCorrupt, HeapCorruptRef, HeapBusy} {
		var c Context
		c.ensureHeap()
		c.heap.terminal = terminal
		if value, err := c.NewTableWithCaps(1, 1); value != NilVal() || err != terminal {
			t.Fatal("fresh allocation did not preserve terminal error")
		}
		if value, err := c.NewNumberArray(1); value != NilVal() || err != terminal {
			t.Fatal("numeric allocation did not preserve terminal error")
		}
	}
}

func BenchmarkFreshTableAllocation(b *testing.B) {
	var c Context
	c.ensureHeap()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.heap.cursor, c.heap.handleN, c.heap.freeHead = 0, 0, 0
		c.heap.handles[0] = ObjectHeader{Offset: uint32(invalidRef)}
		if _, err := c.NewTableWithCaps(0, 6); err != HeapOK {
			b.Fatal(err)
		}
	}
}
