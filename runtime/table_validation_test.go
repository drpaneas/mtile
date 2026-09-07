package runtime

import (
	"testing"
	"unsafe"
)

// Keep the original accessor as an independent reference for validation and
// its error precedence. Header reads deliberately retain the byte decoder.
func originalTableData(c *Context, table Value) ([]byte, HeapError) {
	if table.Kind != KindTable {
		return nil, HeapCorruptRef
	}
	data, err := c.objectBytes(table.Ref)
	if err != HeapOK {
		return nil, err
	}
	if c.heap.handles[table.Ref].Kind() != ObjTable {
		return nil, HeapCorruptRef
	}
	if len(data) < 8 {
		return nil, HeapCorrupt
	}
	arrayCap, hashCap := readU16(data, 2), readU16(data, 6)
	need, _, ok := tableLayoutSize(arrayCap, hashCap)
	if !ok || uint32(len(data)) < need || readU16(data, 0) > arrayCap || readU16(data, 4) > hashCap {
		return nil, HeapCorrupt
	}
	return data, HeapOK
}

func compareTableData(t *testing.T, c *Context, table Value) {
	t.Helper()
	want, wantErr := originalTableData(c, table)
	got, gotErr := c.tableData(table)
	if gotErr != wantErr || len(got) != len(want) || unsafe.SliceData(got) != unsafe.SliceData(want) {
		t.Fatalf("table%+v header%+v terminal%d: data length%d error%d want length%d error%d", table,
			c.heap.handles[0], c.heap.terminal, len(got), gotErr, len(want), wantErr)
	}
}

func TestTableValidationMatchesOriginalErrorPrecedence(t *testing.T) {
	var c Context
	c.ensureHeap()
	// Combine invalid kinds, refs, object states, ranges, and terminal errors.
	// For example an invalid payload range must still precede a wrong object
	// kind, but a wrong Lua value kind must precede an existing terminal error.
	for _, terminal := range []HeapError{HeapOK, HeapOOM, HeapCorrupt, HeapCorruptRef, HeapBusy} {
		c.heap.terminal = terminal
		for state := uint8(0); state < 16; state++ {
			for _, offset := range []uint32{0, 1, heapSize - 8, heapSize - 1, heapSize, heapSize + 1, ^uint32(0)} {
				for _, size := range []uint16{0, 7, 8, 32, 65535} {
					c.heap.handles[0] = ObjectHeader{KindState: state, Offset: offset, Size: size}
					for _, ref := range []Ref{0, Ref(maxHandles), invalidRef} {
						for kind := KindNil; kind <= KindStaticString; kind++ {
							compareTableData(t, &c, Value{Kind: kind, Ref: ref})
						}
					}
				}
			}
		}
	}
}

func TestTableValidationMatchesOriginalAlignedAndUnalignedHeaders(t *testing.T) {
	var c Context
	c.ensureHeap()
	space := c.activeSpace()
	table := Value{Kind: KindTable}
	for shift := uint32(0); shift < 4; shift++ {
		c.heap.handles[0] = ObjectHeader{KindState: uint8(ObjTable)<<2 | uint8(HandleAllocated), Offset: 16 + shift}
		data := space[16+shift:]
		for _, arrayCap := range []uint16{0, 1, 8, 255, 256, 257, 65535} {
			for _, hashCap := range []uint16{0, 1, 8, 127, 128, 129, 65535} {
				writeU16(data, 2, arrayCap)
				writeU16(data, 6, hashCap)
				for _, arrayLen := range []uint16{0, arrayCap, arrayCap + 1} {
					writeU16(data, 0, arrayLen)
					for _, hashLen := range []uint16{0, hashCap, hashCap + 1} {
						writeU16(data, 4, hashLen)
						for _, size := range []uint16{0, 7, 8, tableDataSize(arrayCap, hashCap) - 1, tableDataSize(arrayCap, hashCap), 6152} {
							c.heap.handles[0].Size = size
							compareTableData(t, &c, table)
						}
					}
				}
			}
		}
	}
}

func BenchmarkTableValidation(b *testing.B) {
	var c Context
	table, err := c.NewTableWithCaps(0, 8)
	if err != HeapOK {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.tableData(table); err != HeapOK {
			b.Fatal(err)
		}
	}
}
