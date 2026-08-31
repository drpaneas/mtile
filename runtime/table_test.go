package runtime

import "testing"

func TestNumberArrayUsesCompactNumericStorageAndSurvivesCollection(t *testing.T) {
	var c Context
	array, err := c.NewNumberArray(64)
	if err != HeapOK {
		t.Fatal(err)
	}
	if got, want := c.heap.cursor, uint32(numberArrayHeader+64*4); got != want {
		t.Fatalf("allocated bytes = %d, want %d", got, want)
	}
	if array.Kind != KindTable || c.heap.handles[array.Ref].Kind() != ObjNumberArray {
		t.Fatalf("number array identity = %#v kind=%d", array, c.heap.handles[array.Ref].Kind())
	}
	want := FromInt(7) + One/2
	if err := c.NumberArraySet(array, 37, want); err != HeapOK {
		t.Fatal(err)
	}
	if got := c.NumberArrayGet(array, 37); got != want {
		t.Fatalf("value = %d, want %d", got, want)
	}
	if c.NumberArrayLen(array) != 64 || c.NumberArrayGet(array, 0) != 0 || c.NumberArrayGet(array, 65) != 0 {
		t.Fatal("number array bounds/length contract failed")
	}
	if err := c.NumberArraySet(array, 65, FromInt(99)); err != HeapOK {
		t.Fatal(err)
	}

	root := array
	c.RegisterGlobal(&root)
	if err := c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	if got := c.NumberArrayGet(root, 37); got != want {
		t.Fatalf("value after collection = %d, want %d", got, want)
	}
}

func TestNumberArrayRejectsInvalidLengthsAndGenericObjects(t *testing.T) {
	var c Context
	if _, err := c.NewNumberArray(-1); err != HeapOOM {
		t.Fatalf("negative length error = %d", err)
	}
	if _, err := c.NewNumberArray(maxArrayCap + 1); err != HeapOOM {
		t.Fatalf("oversized length error = %d", err)
	}
	table, err := c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	if got := c.NumberArrayGet(table, 1); got != 0 {
		t.Fatalf("generic table numeric read = %d", got)
	}
	if err := c.NumberArraySet(table, 1, One); err != HeapCorruptRef {
		t.Fatalf("generic table numeric write error = %d", err)
	}
}

func TestTableAddDelDeliCount(t *testing.T) {
	t.Parallel()
	c := new(Context)
	tab, err := c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableAdd(tab, IntVal(10), NilVal()); err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableAdd(tab, IntVal(20), NilVal()); err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableAdd(tab, IntVal(15), IntVal(2)); err != HeapOK {
		t.Fatal(err)
	}
	if c.TableLen(tab) != 3 || c.TableGetInt(tab, 2).Int() != 15 {
		t.Fatalf("add insert: len=%d i2=%d", c.TableLen(tab), c.TableGetInt(tab, 2).Int())
	}
	if c.TableCount(tab, IntVal(10)) != 1 {
		t.Fatal("count value")
	}
	if c.TableCount(tab, NilVal()) != 3 {
		t.Fatal("count length")
	}
	got := c.TableDel(tab, IntVal(15))
	if got.Int() != 15 || c.TableGetInt(tab, 2).Int() != 20 {
		t.Fatal("del did not shift the array part")
	}
	last := c.TableDeli(tab, NilVal())
	if last.Int() != 20 || c.TableLen(tab) != 1 {
		t.Fatal("deli last")
	}
}

func TestTableHashTombstoneDoesNotPin(t *testing.T) {
	t.Parallel()
	f := new(gcFixture)
	tab, err := f.c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(0, tab)
	s, err := f.c.NewString([]byte("gone"))
	if err != HeapOK {
		t.Fatal(err)
	}
	key := f.c.InternStatic("k")
	if err := f.c.TableSet(tab, key, s); err != HeapOK {
		t.Fatal(err)
	}
	if err := f.c.TableSet(tab, key, NilVal()); err != HeapOK {
		t.Fatal(err)
	}
	dead := s.Ref
	if err := f.c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	if f.c.heap.handles[dead].State() != HandleFree {
		t.Fatal("tombstone pinned the deleted string")
	}
}

func TestTablePairsCursorSkipsNil(t *testing.T) {
	t.Parallel()
	c := new(Context)
	tab, err := c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableSet(tab, c.InternStatic("a"), IntVal(1)); err != HeapOK {
		t.Fatal(err)
	}
	found := false
	n := c.TableSlotCount(tab)
	for i := 0; i < n; i++ {
		k := c.TableKeyAt(tab, i)
		if k.Kind == KindNil {
			continue
		}
		if c.ValuesEqual(k, c.InternStatic("a")) && c.TableValueAt(tab, i).Int() == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("pairs cursor missed the hash key")
	}
}

func TestTableAddGrowsArrayPastExactCap(t *testing.T) {
	t.Parallel()
	c := new(Context)
	tab, err := c.NewTableWithCaps(1, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableAdd(tab, IntVal(1), NilVal()); err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableAdd(tab, IntVal(2), NilVal()); err != HeapOK {
		t.Fatalf("second add on cap-1 table: %d", err)
	}
	if c.TableLen(tab) != 2 || c.TableGetInt(tab, 1).Int() != 1 || c.TableGetInt(tab, 2).Int() != 2 {
		t.Fatalf("len=%d t[1]=%d t[2]=%d", c.TableLen(tab), c.TableGetInt(tab, 1).Int(), c.TableGetInt(tab, 2).Int())
	}
}

func TestTableSetGrowsHashPastExactCap(t *testing.T) {
	t.Parallel()
	c := new(Context)
	tab, err := c.NewTableWithCaps(1, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	k := c.InternStatic("k")
	n := c.InternStatic("n")
	if err := c.TableSet(tab, k, IntVal(2)); err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableSet(tab, n, IntVal(3)); err != HeapOK {
		t.Fatalf("second hash set on cap-1 table: %d", err)
	}
	if c.TableGet(tab, k).Int() != 2 || c.TableGet(tab, n).Int() != 3 {
		t.Fatalf("k=%d n=%d", c.TableGet(tab, k).Int(), c.TableGet(tab, n).Int())
	}
}

func TestTableGrowKeepsSameRefAcrossCollect(t *testing.T) {
	t.Parallel()
	f := new(gcFixture)
	tab, err := f.c.NewTableWithCaps(1, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(0, tab)
	ref := tab.Ref
	for i := 1; i <= 20; i++ {
		if err := f.c.TableAdd(tab, IntVal(i), NilVal()); err != HeapOK {
			t.Fatalf("add %d: %d", i, err)
		}
	}
	if err := f.c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	live := f.slots[0]
	if live.Ref != ref {
		t.Fatalf("grow changed ref %d -> %d", ref, live.Ref)
	}
	if f.c.TableLen(live) != 20 || f.c.TableGetInt(live, 20).Int() != 20 {
		t.Fatalf("len=%d t[20]=%d after collect", f.c.TableLen(live), f.c.TableGetInt(live, 20).Int())
	}
}

func TestTableGrowthRootsPendingHeapValues(t *testing.T) {
	t.Parallel()
	t.Run("hash key and value", func(t *testing.T) {
		c := new(Context)
		tab, err := c.NewTableWithCaps(1, 1)
		if err != HeapOK {
			t.Fatal(err)
		}
		if !c.PushLocal(tab) {
			t.Fatal("could not root table")
		}
		defer c.PopLocals(1)
		if err := c.TableSet(tab, c.InternStatic("first"), IntVal(1)); err != HeapOK {
			t.Fatal(err)
		}
		key, err := c.NewString([]byte("pending-key"))
		if err != HeapOK {
			t.Fatal(err)
		}
		value, err := c.NewString([]byte("pending-value"))
		if err != HeapOK {
			t.Fatal(err)
		}
		// Force the next table-growth allocation to collect while key/value
		// are still pending and have not yet been written into the table.
		c.heap.cursor = heapSize
		if err := c.TableSet(tab, key, value); err != HeapOK {
			t.Fatal(err)
		}
		got := c.TableGet(tab, key)
		if got.Kind != KindString || string(c.stringBytes(got)) != "pending-value" {
			t.Fatalf("pending hash value = %#v %q", got, c.stringBytes(got))
		}
	})

	t.Run("array value", func(t *testing.T) {
		c := new(Context)
		tab, err := c.NewTableWithCaps(1, 1)
		if err != HeapOK {
			t.Fatal(err)
		}
		if !c.PushLocal(tab) {
			t.Fatal("could not root table")
		}
		defer c.PopLocals(1)
		if err := c.TableAdd(tab, IntVal(1), NilVal()); err != HeapOK {
			t.Fatal(err)
		}
		value, err := c.NewString([]byte("pending-array-value"))
		if err != HeapOK {
			t.Fatal(err)
		}
		c.heap.cursor = heapSize
		if err := c.TableAdd(tab, value, NilVal()); err != HeapOK {
			t.Fatal(err)
		}
		got := c.TableGetInt(tab, 2)
		if got.Kind != KindString || string(c.stringBytes(got)) != "pending-array-value" {
			t.Fatalf("pending array value = %#v %q", got, c.stringBytes(got))
		}
	})
}

func TestTableRejectsMismatchedHandleKind(t *testing.T) {
	t.Parallel()
	c := new(Context)
	tab, err := c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	str, err := c.NewString([]byte("abc"))
	if err != HeapOK {
		t.Fatal(err)
	}
	if _, err := c.tableData(Value{Kind: KindTable, Ref: str.Ref}); err != HeapCorruptRef {
		t.Fatalf("table view of string handle = %d, want HeapCorruptRef", err)
	}
	if c.stringBytes(Value{Kind: KindString, Ref: tab.Ref}) != nil {
		t.Fatal("string view of table handle returned bytes")
	}
}

func TestStableTableFieldCacheReadsAndWrites(t *testing.T) {
	t.Parallel()
	c := new(Context)
	tab, err := c.NewTableWithCaps(1, 8)
	if err != HeapOK {
		t.Fatal(err)
	}
	key := c.InternStatic("x")
	if err := c.TableSet(tab, key, IntVal(7)); err != HeapOK {
		t.Fatal(err)
	}

	var readCache TableFieldCache
	if got := c.TableGetStable(tab, key, &readCache); got.Int() != 7 || readCache == 0 {
		t.Fatalf("first stable read = %d cache=%d", got.Int(), readCache)
	}
	if got := c.TableGetStable(tab, key, &readCache); got.Int() != 7 {
		t.Fatalf("cached stable read = %d", got.Int())
	}

	var writeCache TableFieldCache
	if err := c.TableSetStable(tab, key, IntVal(11), &writeCache); err != HeapOK || writeCache == 0 {
		t.Fatalf("first stable write err=%d cache=%d", err, writeCache)
	}
	if err := c.TableSetStable(tab, key, IntVal(13), &writeCache); err != HeapOK {
		t.Fatal(err)
	}
	if got := c.TableGet(tab, key); got.Int() != 13 {
		t.Fatalf("stable write result = %d", got.Int())
	}
}

func TestStableTableFieldCacheSurvivesCollection(t *testing.T) {
	t.Parallel()
	f := new(gcFixture)
	tab, err := f.c.NewTableWithCaps(1, 8)
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(0, tab)
	key := f.c.InternStatic("x")
	if err := f.c.TableSet(tab, key, IntVal(21)); err != HeapOK {
		t.Fatal(err)
	}
	var cache TableFieldCache
	if got := f.c.TableGetStable(tab, key, &cache); got.Int() != 21 || cache == 0 {
		t.Fatalf("first read = %d cache=%d", got.Int(), cache)
	}
	if err := f.c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	if got := f.c.TableGetStable(f.slots[0], key, &cache); got.Int() != 21 {
		t.Fatalf("post-collection cached read = %d", got.Int())
	}
}

func TestStableNumericTableFieldCache(t *testing.T) {
	t.Parallel()
	c := new(Context)
	tab, err := c.NewTableWithCaps(1, 8)
	if err != HeapOK {
		t.Fatal(err)
	}
	key := c.InternStatic("x")
	if err := c.TableSet(tab, key, NumVal(FromInt(3)+One/2)); err != HeapOK {
		t.Fatal(err)
	}
	var readCache TableFieldCache
	if got := c.TableGetStableNumber(tab, key, &readCache); got != FromInt(3)+One/2 || readCache == 0 {
		t.Fatalf("first numeric read = %d cache=%d", got, readCache)
	}
	var writeCache TableFieldCache
	if err := c.TableSetStableNumber(tab, key, FromInt(7), &writeCache); err != HeapOK || writeCache == 0 {
		t.Fatalf("numeric write err=%d cache=%d", err, writeCache)
	}
	if got := c.TableGetStableNumber(tab, key, &readCache); got != FromInt(7) {
		t.Fatalf("cached numeric read after write = %d", got)
	}
}

func TestStableNumericTableFieldRejectsNonNumberFirstRead(t *testing.T) {
	t.Parallel()
	c := new(Context)
	tab, err := c.NewTableWithCaps(1, 8)
	if err != HeapOK {
		t.Fatal(err)
	}
	key := c.InternStatic("x")
	if err := c.TableSet(tab, key, BoolVal(true)); err != HeapOK {
		t.Fatal(err)
	}
	var cache TableFieldCache
	if got := c.TableGetStableNumber(tab, key, &cache); got != 0 || cache != 0 {
		t.Fatalf("nonnumeric first read = %d cache=%d", got, cache)
	}
}
