package runtime

import (
	"bytes"
	"fmt"
	"testing"
)

func TestFreshTableLiteralMatchesEveryCapacity(t *testing.T) {
	for capacity := 1; capacity <= maxHashCap; capacity++ {
		var c Context
		arrayCap := capacity % 17
		got, _ := c.NewTableWithCaps(arrayCap, capacity)
		want, _ := c.NewTableWithCaps(arrayCap, capacity)
		view := c.BorrowFreshTableLiteral(got, arrayCap, capacity)
		if view.data == nil || view.err != HeapOK {
			t.Fatalf("capacity %d: fresh borrow failed", capacity)
		}
		occupied := make([]bool, capacity)
		for i := 0; i < capacity; i++ {
			key := c.InternStatic(fmt.Sprintf("fresh_%d", i))
			slot := hashBucket(c.hashValue(key), uint16(capacity))
			for occupied[slot] {
				slot = (slot + 1) % capacity
			}
			occupied[slot] = true
			value := IntVal(i - 63)
			switch i % 4 {
			case 1:
				value = BoolVal(i%2 == 0)
			case 2:
				value = key
			}
			before := append([]byte(nil), c.activeSpace()...)
			err := view.InitKnown(key.SRef, value, literalPlanForTest(arrayCap, capacity, slot, i))
			if err != HeapOK {
				t.Fatal(err)
			}
			header := c.heap.handles[got.Ref]
			space := c.activeSpace()
			if !bytes.Equal(before[:header.Offset], space[:header.Offset]) ||
				!bytes.Equal(before[header.Offset+uint32(header.Size):], space[header.Offset+uint32(header.Size):]) {
				t.Fatalf("capacity %d field %d changed neighboring bytes", capacity, i)
			}
			if err := c.TableSet(want, key, value); err != HeapOK {
				t.Fatal(err)
			}
			assertLiteralTablesEqual(t, &c, got, want)
		}
	}
}

func TestFreshTableLiteralFallbackInvalidatesBeforeLaterGrowth(t *testing.T) {
	for _, reason := range []string{"nil", "invalid-key", "caps", "occupied", "nil-table", "terminal"} {
		t.Run(reason, func(t *testing.T) {
			var c Context
			got, _ := c.NewTableWithCaps(1, 1)
			want, _ := c.NewTableWithCaps(1, 1)
			key, value := c.InternStatic("key"), IntVal(5)
			arrayCap := 1
			switch reason {
			case "nil":
				value = NilVal()
			case "invalid-key":
				key.SRef = c.staticN
			case "caps":
				arrayCap = 2
			case "occupied":
				_ = c.TableSet(got, key, IntVal(4))
				_ = c.TableSet(want, key, IntVal(4))
			case "nil-table":
				got, want = NilVal(), NilVal()
			case "terminal":
				c.heap.terminal = HeapOOM
			}
			view := c.BorrowFreshTableLiteral(got, arrayCap, 1)
			plan := literalPlanForTest(arrayCap, 1, 0, 0)
			if ge, we := view.InitKnown(key.SRef, value, plan), c.TableSet(want, key, value); ge != we {
				t.Fatalf("errors %v / %v", ge, we)
			}
			if view.data != nil {
				t.Fatal("fallback retained a borrowed heap pointer")
			}
			if reason == "nil-table" || reason == "terminal" {
				return
			}
			assertLiteralTablesEqual(t, &c, got, want)
			for i := 0; i < 12; i++ {
				key = c.InternStatic(fmt.Sprintf("later_%d", i))
				if ge, we := view.InitKnown(key.SRef, IntVal(i), plan), c.TableSet(want, key, IntVal(i)); ge != we {
					t.Fatalf("post-fallback errors %v / %v", ge, we)
				}
				assertLiteralTablesEqual(t, &c, got, want)
			}
		})
	}
}

func TestFreshTableLiteralReborrowAfterCollection(t *testing.T) {
	f := new(gcFixture)
	c := &f.c
	key := c.InternStatic("fresh")
	for pass := 0; pass < 4; pass++ {
		table, err := c.NewTableWithCaps(0, 1)
		if err != HeapOK {
			t.Fatal(err)
		}
		f.root(0, table)
		if err := c.Collect(); err != HeapOK {
			t.Fatal(err)
		}
		view := c.BorrowFreshTableLiteral(table, 0, 1)
		if err := view.InitKnown(key.SRef, BoolVal(false), literalPlanForTest(0, 1, 0, 0)); err != HeapOK {
			t.Fatal(err)
		}
		if err := c.Collect(); err != HeapOK {
			t.Fatal(err)
		}
		if c.TableGet(table, key) != BoolVal(false) {
			t.Fatal("field lost after collection")
		}
	}
}
