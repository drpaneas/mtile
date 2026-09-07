package runtime

import (
	"bytes"
	"fmt"
	"testing"
)

func TestNumberArrayViewMatchesCheckedAccessAndAliasing(t *testing.T) {
	for _, shift := range []uint32{0, 1, 2, 3} {
		for _, length := range []int{0, 1, 3, 204, 256} {
			t.Run(fmt.Sprintf("shift%d/length%d", shift, length), func(t *testing.T) {
				var c Context
				array, err := c.NewNumberArray(length)
				if err != HeapOK {
					t.Fatal(err)
				}
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
				view := c.BorrowNumberArray(array)
				alias := c.BorrowNumberArray(array)
				maxInt := int(^uint(0) >> 1)
				for _, index := range []int{-maxInt - 1, -maxInt, -1, 0, 1, length / 2, length, length + 1, maxInt} {
					for _, value := range []Number{0, 1, -1, Number(-2147483648), Number(2147483647), 0x12345678} {
						before := append([]byte(nil), space...)
						wantErr := c.NumberArraySet(array, index, value)
						wantBytes := append([]byte(nil), space...)
						wantGet := c.NumberArrayGet(array, index)
						copy(space, before)
						if gotErr := view.Set(index, value); gotErr != wantErr {
							t.Fatalf("index%d error%d want%d", index, gotErr, wantErr)
						}
						if !bytes.Equal(space, wantBytes) {
							t.Fatalf("index%d changed neighboring bytes", index)
						}
						if view.Get(index) != wantGet || alias.Get(index) != wantGet {
							t.Fatalf("index%d views differ from checked read %d", index, wantGet)
						}
					}
				}
			})
		}
	}
}

func TestNumberArrayViewPreservesInvalidObjectErrors(t *testing.T) {
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
			view := c.BorrowNumberArray(array)
			for _, index := range []int{-1, 0, 1, 4} {
				if got, want := view.Get(index), c.NumberArrayGet(array, index); got != want {
					t.Fatalf("index%d value%d want%d", index, got, want)
				}
				if got, want := view.Set(index, 7), c.NumberArraySet(array, index, 7); got != want {
					t.Fatalf("index%d error%d want%d", index, got, want)
				}
			}
		})
	}
}

func TestNumberArrayViewReborrowsAfterCollection(t *testing.T) {
	f := new(gcFixture)
	c := &f.c
	array, err := c.NewNumberArray(3)
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(0, array)
	for pass := 0; pass < 4; pass++ {
		// A borrow ends before each collection. The compiler's loop effect
		// proof is responsible for this boundary; retain the rooted handle.
		view := c.BorrowNumberArray(f.slots[0])
		for i := 1; i <= 3; i++ {
			if pass != 0 && view.Get(i) != Number((pass-1)*10+i) {
				t.Fatal("reborrow did not follow relocated array")
			}
			if err := view.Set(i, Number(pass*10+i)); err != HeapOK {
				t.Fatal(err)
			}
		}
		if err := c.Collect(); err != HeapOK {
			t.Fatal(err)
		}
	}
}

func BenchmarkNumberArrayViewRegion(b *testing.B) {
	var c Context
	array, err := c.NewNumberArray(204)
	if err != HeapOK {
		b.Fatal(err)
	}
	for _, borrow := range []bool{false, true} {
		b.Run(fmt.Sprint(borrow), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if borrow {
					view := c.BorrowNumberArray(array)
					for j := 1; j <= 204; j++ {
						_ = view.Set(j, view.Get(j)+1)
					}
				} else {
					for j := 1; j <= 204; j++ {
						_ = c.NumberArraySet(array, j, c.NumberArrayGet(array, j)+1)
					}
				}
			}
		})
	}
}
