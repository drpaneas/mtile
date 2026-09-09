package runtime

import "testing"

// A one-bucket starting table forces mixed kinds through the same probe chain.
// Compare lookup with a linear scan that retains generic equality throughout
// growth, tombstone reuse, overwrite through equivalent string storage and GC.
func TestNumericHashProbeMatchesMixedKeyScan(t *testing.T) {
	for _, capacity := range []int{1, 7, 32} {
		c := new(Context)
		table, err := c.NewTableWithCaps(0, capacity)
		if err != HeapOK {
			t.Fatal(err)
		}
		c.RegisterGlobal(&table)
		object, err := c.NewTableWithCaps(0, 0)
		if err != HeapOK {
			t.Fatal(err)
		}
		c.RegisterGlobal(&object)
		text, err := c.NewString([]byte("1"))
		if err != HeapOK {
			t.Fatal(err)
		}
		c.RegisterGlobal(&text)
		keys := []Value{NumVal(0), NumVal(1), IntVal(-1), NumVal(-1), IntVal(1),
			NumVal(Number(-2147483648)), NumVal(Number(2147483647)),
			BoolVal(false), BoolVal(true), c.InternStatic("1"), c.InternStatic(""), object, text}
		queries := append(append([]Value(nil), keys...), NilVal(), IntVal(321), NumVal(3), c.InternStatic("absent"))
		check := func() {
			t.Helper()
			for _, key := range queries {
				want := NilVal()
				for slot := 0; slot < c.TableSlotCount(table); slot++ {
					stored := c.TableKeyAt(table, slot)
					if stored.Kind != KindNil && c.ValuesEqual(stored, key) {
						want = c.TableValueAt(table, slot)
						break
					}
				}
				if got := c.TableGet(table, key); got != want {
					t.Fatalf("initial capacity=%d key=%+v got=%+v want=%+v", capacity, key, got, want)
				}
			}
		}
		for index, key := range keys {
			if err := c.TableSet(table, key, IntVal(index+1)); err != HeapOK {
				t.Fatal(err)
			}
			check()
		}
		for index, key := range keys {
			if index%2 == 0 {
				if err := c.TableSet(table, key, NilVal()); err != HeapOK {
					t.Fatal(err)
				}
				check()
			}
		}
		if err := c.Collect(); err != HeapOK {
			t.Fatal(err)
		}
		check()
		for index, key := range keys {
			if err := c.TableSet(table, key, IntVal(100+index)); err != HeapOK {
				t.Fatal(err)
			}
			check()
		}
	}
}
