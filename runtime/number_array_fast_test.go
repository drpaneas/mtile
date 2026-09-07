package runtime

import (
	"bytes"
	"fmt"
	"testing"
)

func bytewiseNumberArrayGet(c *Context, array Value, index int) Number {
	data, length, err := c.numberArrayData(array)
	if err != HeapOK || index < 1 || index > length {
		return 0
	}
	offset := numberArrayHeader + (index-1)*4
	return Number(int32(uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24))
}

func bytewiseNumberArraySet(c *Context, array Value, index int, value Number) HeapError {
	data, length, err := c.numberArrayData(array)
	if err != HeapOK {
		return err
	}
	if index < 1 || index > length {
		return HeapOK
	}
	offset := numberArrayHeader + (index-1)*4
	raw := uint32(value)
	data[offset], data[offset+1], data[offset+2], data[offset+3] = byte(raw), byte(raw>>8), byte(raw>>16), byte(raw>>24)
	return HeapOK
}

func TestNumberArrayAccessMatchesBytesAndPreservesSurroundingPayload(t *testing.T) {
	for _, shift := range []uint32{0, 1, 2, 3} {
		for _, length := range []int{0, 1, 3, 204, 256} {
			t.Run(fmt.Sprintf("shift%d/length%d", shift, length), func(t *testing.T) {
				var c Context
				array, err := c.NewNumberArray(length)
				if err != HeapOK {
					t.Fatal(err)
				}
				// Real payloads are aligned. Also retain the old accessor's behavior
				// for otherwise in-bounds headers with unusual unaligned offsets.
				c.heap.handles[array.Ref].Offset = 16 + shift
				space := c.activeSpace()
				for i := range space {
					space[i] = 0xa5
				}
				data, err := c.objectBytes(array.Ref)
				if err != HeapOK {
					t.Fatal(err)
				}
				writeU16(data, 0, uint16(length))
				writeU16(data, 2, 0)
				indices := []int{-2147483647, -1, 0, 1, length / 2, length, length + 1, 2147483647}
				for _, index := range indices {
					for _, value := range []Number{0, 1, -1, Number(-2147483648), Number(2147483647), Number(0x12345678)} {
						before := append([]byte(nil), space...)
						wantErr := bytewiseNumberArraySet(&c, array, index, value)
						wantBytes := append([]byte(nil), space...)
						copy(space, before)
						if gotErr := c.NumberArraySet(array, index, value); gotErr != wantErr {
							t.Fatalf("index%d error%d want%d", index, gotErr, wantErr)
						}
						if !bytes.Equal(space, wantBytes) {
							t.Fatalf("index%d value%x changed neighboring bytes", index, value)
						}
						if got, want := c.NumberArrayGet(array, index), bytewiseNumberArrayGet(&c, array, index); got != want {
							t.Fatalf("index%d got%x want%x", index, got, want)
						}
					}
				}
			})
		}
	}
}

func TestNumberArrayInvalidHeadersKeepAccessorErrors(t *testing.T) {
	for _, name := range []string{"nil", "invalid-ref", "free", "wrong-object", "short-header", "bad-length", "offset-past-end", "size-past-end", "terminal"} {
		t.Run(name, func(t *testing.T) {
			var c Context
			array, err := c.NewNumberArray(3)
			if err != HeapOK {
				t.Fatal(err)
			}
			header := &c.heap.handles[array.Ref]
			switch name {
			case "nil":
				array = NilVal()
			case "invalid-ref":
				array.Ref = invalidRef
			case "free":
				header.SetState(HandleFree)
			case "wrong-object":
				header.KindState = uint8(ObjTable)<<2 | uint8(HandleAllocated)
			case "short-header":
				header.Size = 3
			case "bad-length":
				writeU16(c.activeSpace(), header.Offset, 65535)
			case "offset-past-end":
				header.Offset = heapSize + 1
			case "size-past-end":
				header.Offset = heapSize - 1
			case "terminal":
				c.heap.terminal = HeapOOM
			}
			for _, index := range []int{-1, 0, 1, 4} {
				if got, want := c.NumberArrayGet(array, index), bytewiseNumberArrayGet(&c, array, index); got != want {
					t.Fatalf("index%d got%d want%d", index, got, want)
				}
				if got, want := c.NumberArraySet(array, index, 7), bytewiseNumberArraySet(&c, array, index, 7); got != want {
					t.Fatalf("index%d error%d want%d", index, got, want)
				}
			}
		})
	}
}

func BenchmarkNumberArrayAccess(b *testing.B) {
	var c Context
	array, err := c.NewNumberArray(204)
	if err != HeapOK {
		b.Fatal(err)
	}
	b.Run("get", func(b *testing.B) {
		var sum Number
		for i := 0; i < b.N; i++ {
			sum += c.NumberArrayGet(array, i%204+1)
		}
		if sum != 0 {
			b.Fatal(sum)
		}
	})
	b.Run("set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err := c.NumberArraySet(array, i%204+1, Number(i)); err != HeapOK {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkHeapAllocateClearedPayload(b *testing.B) {
	for _, size := range []uint16{16, 64, 392, 820, 3080} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			var c Context
			c.ensureHeap()
			h := &c.heap
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.cursor = 0
				h.freeHead = 0
				h.handleN = 0
				h.handles[0].SetState(HandleFree)
				h.handles[0].Offset = uint32(invalidRef)
				if _, err := h.allocObject(ObjTable, size); err != HeapOK {
					b.Fatal(err)
				}
			}
		})
	}
}
