package runtime

import (
	"bytes"
	"fmt"
	"testing"
)

func literalPlanForTest(arrayCap, hashCap, slot, count int) uint32 {
	return uint32(arrayCap) | uint32(hashCap)<<9 | uint32(slot)<<17 | uint32(count)<<24
}

func assertLiteralTablesEqual(t *testing.T, c *Context, got, want Value) {
	t.Helper()
	gd, ge := c.tableData(got)
	wd, we := c.tableData(want)
	if ge != HeapOK || we != HeapOK || len(gd) != len(wd) {
		t.Fatalf("table data mismatch: errors %v/%v lengths %d/%d", ge, we, len(gd), len(wd))
	}
	for i := range gd {
		if gd[i] != wd[i] {
			t.Fatalf("table layout byte %d differs: got %d want %d", i, gd[i], wd[i])
		}
	}
}

func TestTableInitStaticPreservesExactLayoutAndValues(t *testing.T) {
	for _, capacity := range []int{1, 2, 3, 4, 5, 8, 17, 28, 32, 128} {
		t.Run(fmt.Sprintf("capacity-%d", capacity), func(t *testing.T) {
			var c Context
			got, err := c.NewTableWithCaps(3, capacity)
			if err != HeapOK {
				t.Fatal(err)
			}
			want, err := c.NewTableWithCaps(3, capacity)
			if err != HeapOK {
				t.Fatal(err)
			}
			occupied := make([]bool, capacity)
			for i := 0; i < capacity; i++ {
				key := c.InternStatic(fmt.Sprintf("key_%d", i))
				slot := hashBucket(c.hashValue(key), uint16(capacity))
				for occupied[slot] {
					slot = (slot + 1) % capacity
				}
				occupied[slot] = true
				value := IntVal(i - 40)
				if i%3 == 0 {
					value = BoolVal(i%2 == 0)
				}
				if err := c.TableInitStatic(got, key.SRef, value, literalPlanForTest(3, capacity, slot, i)); err != HeapOK {
					t.Fatal(err)
				}
				if err := c.TableSet(want, key, value); err != HeapOK {
					t.Fatal(err)
				}
				assertLiteralTablesEqual(t, &c, got, want)
				if actual := c.TableGet(got, key); actual != value {
					t.Fatalf("key %v got %#v want %#v", i, actual, value)
				}
			}
		})
	}
}

func TestTableInitStaticFallsBackOnUnexpectedConstructorState(t *testing.T) {
	for _, issue := range []string{"nil", "array-cap", "hash-cap", "count", "occupied", "tombstone"} {
		t.Run(issue, func(t *testing.T) {
			var c Context
			got, _ := c.NewTableWithCaps(2, 4)
			want, _ := c.NewTableWithCaps(2, 4)
			key := c.InternStatic("key")
			value := IntVal(42)
			slot := hashBucket(c.hashValue(key), 4)
			plan := literalPlanForTest(2, 4, slot, 0)
			switch issue {
			case "nil":
				value = NilVal()
			case "array-cap":
				plan = literalPlanForTest(1, 4, slot, 0)
			case "hash-cap":
				plan = literalPlanForTest(2, 3, slot, 0)
			case "count":
				plan = literalPlanForTest(2, 4, slot, 1)
			case "occupied", "tombstone":
				_ = c.TableSet(got, key, BoolVal(false))
				_ = c.TableSet(want, key, BoolVal(false))
				plan = literalPlanForTest(2, 4, slot, 1)
				if issue == "tombstone" {
					_ = c.TableSet(got, key, NilVal())
					_ = c.TableSet(want, key, NilVal())
					plan = literalPlanForTest(2, 4, slot, 0)
				}
			}
			if err := c.TableInitStatic(got, key.SRef, value, plan); err != HeapOK {
				t.Fatal(err)
			}
			if err := c.TableSet(want, key, value); err != HeapOK {
				t.Fatal(err)
			}
			assertLiteralTablesEqual(t, &c, got, want)
		})
	}
}

func TestTableInitStaticRejectsInvalidTableBeforeWriting(t *testing.T) {
	var c Context
	key := c.InternStatic("key")
	for _, table := range []Value{NilVal(), IntVal(0), {Kind: KindTable, Ref: invalidRef}, {Kind: KindTable, Ref: 500}} {
		if got := c.TableInitStatic(table, key.SRef, IntVal(1), literalPlanForTest(1, 1, 0, 0)); got == HeapOK {
			t.Fatalf("accepted invalid table %#v", table)
		}
	}
}

func TestTableLiteralViewPreservesLayoutCanariesAndAliases(t *testing.T) {
	for _, capacity := range []int{2, 3, 4, 5, 8, 17, 28, 32, 128} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			var c Context
			table, err := c.NewTableWithCaps(3, capacity)
			if err != HeapOK {
				t.Fatal(err)
			}
			space := c.activeSpace()
			header := c.heap.handles[table.Ref]
			for i := int(header.Offset) + int(header.Size); i < len(space); i++ {
				space[i] = 0xa5
			}
			cursor, handles, lastErr := c.heap.cursor, c.heap.handleN, c.LastHeapError()
			view, alias := c.BorrowTableLiteral(table), c.BorrowTableLiteral(table)
			if c.heap.cursor != cursor || c.heap.handleN != handles || c.LastHeapError() != lastErr {
				t.Fatal("borrow changed allocation/error state")
			}
			occupied := make([]bool, capacity)
			for i := 0; i < capacity; i++ {
				key := c.InternStatic(fmt.Sprintf("literal_%d", i))
				slot := hashBucket(c.hashValue(key), uint16(capacity))
				for occupied[slot] {
					slot = (slot + 1) % capacity
				}
				occupied[slot] = true
				value := IntVal(i - 21)
				before := append([]byte(nil), space...)
				if err := c.TableSet(table, key, value); err != HeapOK {
					t.Fatal(err)
				}
				wantBytes := append([]byte(nil), space...)
				copy(space, before)
				current := &view
				if i%2 != 0 {
					current = &alias
				}
				if err := current.InitStatic(key.SRef, value, literalPlanForTest(3, capacity, slot, i)); err != HeapOK {
					t.Fatal(err)
				}
				if !bytes.Equal(space, wantBytes) {
					t.Fatalf("field %d differs from TableSet or changed neighboring bytes", i)
				}
			}
		})
	}
}

func TestTableLiteralViewInvalidatesBeforeFallbackAndGrowth(t *testing.T) {
	for _, reason := range []string{"nil", "wrong-plan", "occupied", "invalid-key"} {
		t.Run(reason, func(t *testing.T) {
			var c Context
			got, _ := c.NewTableWithCaps(1, 1)
			want, _ := c.NewTableWithCaps(1, 1)
			view := c.BorrowTableLiteral(got)
			key, value := c.InternStatic("first"), IntVal(7)
			plan := literalPlanForTest(1, 1, 0, 0)
			switch reason {
			case "nil":
				value = NilVal()
			case "wrong-plan":
				plan = ^uint32(0)
			case "occupied":
				_ = c.TableSet(got, key, IntVal(2))
				_ = c.TableSet(want, key, IntVal(2))
			case "invalid-key":
				key.SRef = c.staticN
			}
			if err, wantErr := view.InitStatic(key.SRef, value, plan), c.TableSet(want, key, value); err != wantErr {
				t.Fatalf("fallback error %v want %v", err, wantErr)
			}
			if view.data != nil {
				t.Fatal("fallback retained a borrowed heap pointer")
			}
			assertLiteralTablesEqual(t, &c, got, want)
			for i := 0; i < 10; i++ {
				key = c.InternStatic(fmt.Sprintf("later_%d", i))
				value = IntVal(i)
				if err, wantErr := view.InitStatic(key.SRef, value, plan), c.TableSet(want, key, value); err != wantErr {
					t.Fatalf("post-growth error %v want %v", err, wantErr)
				}
				assertLiteralTablesEqual(t, &c, got, want)
			}
		})
	}
}

func TestTableLiteralViewPreservesInvalidObjectErrors(t *testing.T) {
	for _, name := range []string{"nil", "invalid-ref", "free", "wrong-object", "short-header", "bad-capacity", "offset-past-end", "size-past-end", "terminal"} {
		t.Run(name, func(t *testing.T) {
			var c Context
			table, _ := c.NewTableWithCaps(1, 2)
			key := c.InternStatic("key")
			header := &c.heap.handles[table.Ref]
			switch name {
			case "nil":
				table = NilVal()
			case "invalid-ref":
				table.Ref = invalidRef
			case "free":
				header.SetState(HandleFree)
			case "wrong-object":
				header.KindState = uint8(ObjString)<<2 | uint8(HandleAllocated)
			case "short-header":
				header.Size = 3
			case "bad-capacity":
				writeU16(c.activeSpace(), header.Offset+6, 65535)
			case "offset-past-end":
				header.Offset = heapSize + 1
			case "size-past-end":
				header.Offset = heapSize - 1
			case "terminal":
				c.heap.terminal = HeapOOM
			}
			view := c.BorrowTableLiteral(table)
			for _, value := range []Value{NilVal(), IntVal(3)} {
				if got, want := view.InitStatic(key.SRef, value, 1025), c.TableSet(table, key, value); got != want {
					t.Fatalf("error %v want %v", got, want)
				}
			}
		})
	}
}

func TestTableLiteralViewReborrowsAfterCollection(t *testing.T) {
	f := new(gcFixture)
	c := &f.c
	key := c.InternStatic("key")
	for pass := 0; pass < 4; pass++ {
		table, err := c.NewTableWithCaps(1, 2)
		if err != HeapOK {
			t.Fatal(err)
		}
		f.root(0, table)
		if err := c.Collect(); err != HeapOK {
			t.Fatal(err)
		}
		view := c.BorrowTableLiteral(f.slots[0])
		plan := literalPlanForTest(1, 2, hashBucket(c.hashValue(key), 2), 0)
		if err := view.InitStatic(key.SRef, IntVal(pass), plan); err != HeapOK {
			t.Fatal(err)
		}
		if err := c.Collect(); err != HeapOK {
			t.Fatal(err)
		}
		if c.TableGet(f.slots[0], key) != IntVal(pass) {
			t.Fatal("field lost after collection")
		}
	}
}
