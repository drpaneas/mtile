package runtime

import "testing"

func TestHashBucketMatchesModuloAtEveryCapacity(t *testing.T) {
	for capacity := uint16(1); capacity <= MaxTableHashCapacity; capacity++ {
		check := func(hash uint32) {
			t.Helper()
			if got, want := hashBucket(hash, capacity), int(hash%uint32(capacity)); got != want {
				t.Fatalf("hash %#x capacity %d: bucket %d, want %d", hash, capacity, got, want)
			}
		}
		for hash := uint32(0); hash < 1024; hash++ {
			check(hash)
			check(^hash)
			check(hash << 16)
		}
		state := uint32(0x12345678)
		for i := 0; i < 1024; i++ {
			state = state*1664525 + 1013904223
			check(state)
		}
	}
}

func TestHashBucketNonPowerOfTwoGrowthAndTombstones(t *testing.T) {
	for _, capacity := range []int{3, 5, 7, 63, 65, 127} {
		var c Context
		table, err := c.NewTableWithCaps(0, capacity)
		if err != HeapOK {
			t.Fatal(err)
		}
		c.RegisterGlobal(&table)
		// Multiples of capacity force collisions under the modulo fallback.
		keys := make([]Value, capacity)
		for i := range keys {
			keys[i] = NumVal(Number((i + 1) * capacity))
			if err := c.TableSet(table, keys[i], IntVal(i+1)); err != HeapOK {
				t.Fatal(err)
			}
		}
		if err := c.TableSet(table, keys[0], NilVal()); err != HeapOK {
			t.Fatal(err)
		}
		for i := 1; i < len(keys); i++ {
			if got := c.TableGet(table, keys[i]); got != IntVal(i+1) {
				t.Fatalf("capacity %d: collision chain lost key %d after deletion", capacity, i)
			}
		}
		if err := c.TableSet(table, keys[0], IntVal(1)); err != HeapOK {
			t.Fatal(err)
		}
		// A full hash part grows through arbitrary capacities, then to 128.
		if err := c.TableSet(table, IntVal(-1), IntVal(99)); err != HeapOK {
			t.Fatal(err)
		}
		if err := c.Collect(); err != HeapOK {
			t.Fatal(err)
		}
		for i := range keys {
			if got := c.TableGet(table, keys[i]); got != IntVal(i+1) {
				t.Fatalf("capacity %d: key %d after growth/GC = %+v", capacity, i, got)
			}
		}
		if got := c.TableGet(table, IntVal(-1)); got != IntVal(99) {
			t.Fatalf("capacity %d: new key after growth/GC = %+v", capacity, got)
		}
	}
}
