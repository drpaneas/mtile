package runtime

import (
	"bytes"
	"testing"
)

func TestTableGetCachedNumberMatchesStableAfterUpdatesAndGC(t *testing.T) {
	f := new(gcFixture)
	c := &f.c
	table, err := c.NewTableWithCaps(2, 3)
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(0, table)
	keys := []Value{IntVal(1), c.InternStatic("x"), c.InternStatic("y"), c.InternStatic("timer")}
	caches := make([]TableFieldCache, len(keys))
	for i, key := range keys {
		if err := c.TableSet(table, key, IntVal(i-2)); err != HeapOK {
			t.Fatal(err)
		}
		_ = c.TableGetStableNumber(table, key, &caches[i])
		if caches[i] == 0 {
			t.Fatal("test did not establish stable slot")
		}
	}
	for pass := 0; pass < 4; pass++ {
		for i, key := range keys {
			value := Number(0x1234567 * (pass + i + 1))
			if i%2 == 0 {
				value = -value
			}
			c.TableSetCachedNumber(f.slots[0].Ref, caches[i], value)
			before := append([]byte(nil), c.activeSpace()...)
			if got, want := c.TableGetCachedNumber(f.slots[0].Ref, caches[i]), c.TableGetStableNumber(f.slots[0], key, &caches[i]); got != want || got != value {
				t.Fatalf("pass%d field%d got%v want%v", pass, i, got, want)
			}
			if !bytes.Equal(before, c.activeSpace()) {
				t.Fatal("warm read wrote to heap memory")
			}
		}
		if err := c.Collect(); err != HeapOK {
			t.Fatal(err)
		}
	}
}

func TestTableGetStableNumberWarmIgnoresNonTable(t *testing.T) {
	var c Context
	decoy, err := c.NewTableWithCaps(0, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	key := c.InternStatic("x")
	if err := c.TableSet(decoy, key, IntVal(99)); err != HeapOK {
		t.Fatal(err)
	}
	table, err := c.NewTableWithCaps(0, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableSet(table, key, IntVal(7)); err != HeapOK {
		t.Fatal(err)
	}
	var cache TableFieldCache
	if got := c.TableGetStableNumber(table, key, &cache); got != FromInt(7) || cache == 0 {
		t.Fatalf("warm setup got%v cache%v", got, cache)
	}
	if got := c.TableGetStableNumber(NilVal(), key, &cache); got != 0 {
		t.Fatalf("nil base read %v from handle 0", got)
	}
	if got := c.TableGetStableNumber(IntVal(0), key, &cache); got != 0 {
		t.Fatalf("number base read %v from handle 0", got)
	}
}

func TestTableSetStableWarmIgnoresNonTable(t *testing.T) {
	var c Context
	decoy, err := c.NewTableWithCaps(0, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	key := c.InternStatic("x")
	if err := c.TableSet(decoy, key, IntVal(99)); err != HeapOK {
		t.Fatal(err)
	}
	table, err := c.NewTableWithCaps(0, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableSet(table, key, IntVal(7)); err != HeapOK {
		t.Fatal(err)
	}
	var cache TableFieldCache
	if err := c.TableSetStable(table, key, IntVal(8), &cache); err != HeapOK || cache == 0 {
		t.Fatalf("warm setup err%v cache%v", err, cache)
	}
	if err := c.TableSetStable(NilVal(), key, IntVal(3), &cache); err != HeapOK && err != HeapCorruptRef {
		t.Fatalf("nil base store %v", err)
	}
	if got := c.TableGet(decoy, key); got != IntVal(99) {
		t.Fatalf("nil store smashed decoy %#v", got)
	}
}
