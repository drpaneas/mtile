package runtime

import (
	"fmt"
	"testing"
)

func originalTableLen(c *Context, table Value) int {
	data, err := c.tableData(table)
	if err != HeapOK {
		return 0
	}
	arrayCap := int(readU16(data, 2))
	n := 0
	for n < arrayCap && Kind(data[arraySlotOff(n)]) != KindNil {
		n++
	}
	return n
}

func TestTableListLengthPreservesEveryNilBoundary(t *testing.T) {
	var c Context
	table, err := c.NewTableWithCaps(maxArrayCap, 0)
	if err != HeapOK {
		t.Fatal(err)
	}
	data, _ := c.tableData(table)
	for boundary := 0; boundary <= maxArrayCap; boundary++ {
		for i := 0; i < maxArrayCap; i++ {
			value := IntVal(i)
			if i == boundary {
				value = NilVal()
			}
			writeValue(data, arraySlotOff(i), value)
		}
		if got, want := c.TableLen(table), originalTableLen(&c, table); got != want || got != boundary {
			t.Fatalf("boundary%d length%d want%d", boundary, got, want)
		}
	}
}

func TestTableListOperationsPreserveNilHolesAndHiddenElements(t *testing.T) {
	var c Context
	table, err := c.NewTableWithCaps(8, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	for i, value := range []Value{IntVal(10), NilVal(), IntVal(30), IntVal(40)} {
		if err = c.TableSet(table, IntVal(i+1), value); err != HeapOK {
			t.Fatal(err)
		}
	}
	if err = c.TableAdd(table, IntVal(20), IntVal(-1)); err != HeapOK || c.TableLen(table) != 4 {
		t.Fatal("invalid insertion index did not append at the first nil")
	}
	if c.TableDeli(table, IntVal(2)).Int() != 20 || c.TableLen(table) != 3 {
		t.Fatal("indexed removal did not shift the prefix")
	}
	if err = c.TableAdd(table, NilVal(), IntVal(2)); err != HeapOK || c.TableLen(table) != 1 {
		t.Fatal("nil insertion did not split the prefix")
	}
	if c.TableDel(table, IntVal(30)) != NilVal() || c.TableGetInt(table, 3).Int() != 30 {
		t.Fatal("del searched array elements beyond the first nil")
	}
	if c.TableDeli(table, NilVal()).Int() != 10 || c.TableLen(table) != 0 || c.TableGetInt(table, 4).Int() != 40 {
		t.Fatal("default removal changed hidden array elements")
	}
}

func TestTableIndexedInsertGrowsWithoutOverwritingHash(t *testing.T) {
	for _, hashCap := range []int{0, 2} {
		t.Run(fmt.Sprint(hashCap), func(t *testing.T) {
			var c Context
			table, err := c.NewTableWithCaps(2, hashCap)
			if err != HeapOK {
				t.Fatal(err)
			}
			c.RegisterGlobal(&table)
			if err = c.TableAdd(table, IntVal(10), NilVal()); err != HeapOK {
				t.Fatal(err)
			}
			if err = c.TableAdd(table, IntVal(30), NilVal()); err != HeapOK {
				t.Fatal(err)
			}
			key := c.InternStatic("keep")
			if hashCap > 0 {
				if err = c.TableSet(table, key, IntVal(99)); err != HeapOK {
					t.Fatal(err)
				}
			}
			value, err := c.NewString([]byte("inserted"))
			if err != HeapOK {
				t.Fatal(err)
			}
			// Allocation must keep both the table and pending string rooted.
			c.heap.cursor = heapSize
			if err = c.TableAdd(table, value, IntVal(2)); err != HeapOK {
				t.Fatal(err)
			}
			if c.TableLen(table) != 3 || c.TableGetInt(table, 1).Int() != 10 || c.TableGetInt(table, 3).Int() != 30 ||
				string(c.stringBytes(c.TableGetInt(table, 2))) != "inserted" {
				t.Fatal("indexed insertion lost or reordered array elements")
			}
			if hashCap > 0 && c.TableGet(table, key).Int() != 99 {
				t.Fatal("indexed insertion overwrote hash storage")
			}
		})
	}
}

func TestTableIndexedInsertAccountsForPromotedIntegerKeys(t *testing.T) {
	var c Context
	table, err := c.NewTableWithCaps(1, 4)
	if err != HeapOK {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		if err = c.TableSet(table, IntVal(i), IntVal(i*10)); err != HeapOK {
			t.Fatal(err)
		}
	}
	if err = c.TableAdd(table, IntVal(5), IntVal(1)); err != HeapOK {
		t.Fatal(err)
	}
	for i, want := range []int{5, 10, 20, 30, 40} {
		if got := c.TableGetInt(table, i+1).Int(); got != want {
			t.Fatalf("element%d=%d want%d", i+1, got, want)
		}
	}
}

func TestTableIndexedInsertFailsClosedAtMaximumCapacity(t *testing.T) {
	var c Context
	table, err := c.NewTableWithCaps(maxArrayCap, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	for i := 1; i <= maxArrayCap; i++ {
		if err = c.TableSet(table, IntVal(i), IntVal(i)); err != HeapOK {
			t.Fatal(err)
		}
	}
	if err = c.TableAdd(table, IntVal(-1), IntVal(1)); err != HeapOOM {
		t.Fatalf("full insertion error%d want HeapOOM", err)
	}
	for i := 1; i <= maxArrayCap; i++ {
		if c.TableGetInt(table, i).Int() != i {
			t.Fatal("failed insertion changed array contents")
		}
	}
}

func BenchmarkDenseListAppend(b *testing.B) {
	var c Context
	table, err := c.NewTableWithCaps(32, 0)
	if err != HeapOK {
		b.Fatal(err)
	}
	for i := 0; i < 24; i++ {
		if err = c.TableAdd(table, IntVal(i+1), NilVal()); err != HeapOK {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err = c.TableAdd(table, IntVal(25), NilVal()); err != HeapOK {
			b.Fatal(err)
		}
		if c.TableDeli(table, NilVal()).Int() != 25 {
			b.Fatal("append/remove mismatch")
		}
	}
}
