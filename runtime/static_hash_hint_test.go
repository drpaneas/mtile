package runtime

import (
	"fmt"
	"testing"
	"unsafe"
)

func originalHashFind(c *Context, data []byte, key Value) (int, bool) {
	hashCap := int(readU16(data, 6))
	if hashCap == 0 {
		return 0, false
	}
	arrayCap := readU16(data, 2)
	start := hashBucket(c.hashValue(key), uint16(hashCap))
	for slot := start; ; {
		candidate := readValue(data, hashKeyOff(arrayCap, slot))
		if candidate.Kind == KindNil {
			value := readValue(data, hashValOff(arrayCap, uint16(hashCap), slot))
			if !isTombstone(candidate, value) {
				return 0, false
			}
		} else if c.ValuesEqual(candidate, key) {
			return slot, true
		}
		slot++
		if slot >= hashCap {
			slot = 0
		}
		if slot == start {
			return 0, false
		}
	}
}

func TestStaticHashHintDoesNotGrowMetadata(t *testing.T) {
	type previousStaticString struct {
		Ptr  unsafe.Pointer
		Len  uint16
		hash uint32
	}
	if unsafe.Sizeof(StaticString{}) != unsafe.Sizeof(previousStaticString{}) {
		t.Fatal("slot hint grew static string metadata")
	}
}

func BenchmarkColdStaticFieldLookupNonPowerOfTwo(b *testing.B) {
	var c Context
	table, err := c.NewTableWithCaps(0, 6)
	if err != HeapOK {
		b.Fatal(err)
	}
	key := c.InternStatic("dash_acceleration")
	if err = c.TableSet(table, key, IntVal(7)); err != HeapOK {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var cache TableFieldCache
		if c.TableGetStableNumber(table, key, &cache) != FromInt(7) {
			b.Fatal("field mismatch")
		}
	}
}

func TestStaticHashHintFixtureReference(t *testing.T) {
	for _, capacity := range []int{1, 2, 3, 5, 8, 16, 31, 64, 128} {
		if capacity+1 > staticStringSlots {
			continue
		}
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			var c Context
			table, err := c.NewTableWithCaps(3, capacity)
			if err != HeapOK {
				t.Fatal(err)
			}
			c.RegisterGlobal(&table)
			keys := make([]Value, capacity+1)
			for i := range keys {
				keys[i] = c.InternStatic(fmt.Sprintf("field-%03d", i))
				if i == capacity {
					continue // Absent static key exercises the miss path.
				}
				storedKey := keys[i]
				if i&1 != 0 {
					storedKey, err = c.NewString(c.stringBytes(storedKey))
					if err != HeapOK {
						t.Fatal(err)
					}
				}
				if err = c.TableSet(table, storedKey, IntVal(i+1)); err != HeapOK {
					t.Fatal(err)
				}
			}
			for pass := 0; pass < 3; pass++ {
				data, err := c.tableData(table)
				if err != HeapOK {
					t.Fatal(err)
				}
				for _, key := range append(keys, Value{Kind: KindStaticString, SRef: ^uint16(0)}) {
					wantSlot, wantFound := originalHashFind(&c, data, key)
					// Hints may come from a different table shape or a previously
					// deleted/reused object. Every byte value must be harmless.
					for slot := 0; slot < 256; slot++ {
						if key.SRef < c.staticN {
							c.staticTab[key.SRef].slotHint, c.staticTab[key.SRef].hintSet = uint8(slot), true
						}
						gotSlot, gotFound := c.hashFind(data, key)
						if gotSlot != wantSlot || gotFound != wantFound {
							t.Fatalf("hint%d lookup%+v got%d/%v want%d/%v", slot, key, gotSlot, gotFound, wantSlot, wantFound)
						}
					}
				}
				if pass == 0 {
					for i := 0; i < capacity; i += 3 {
						if err = c.TableSet(table, keys[i], NilVal()); err != HeapOK {
							t.Fatal(err)
						}
					}
				} else if pass == 1 {
					for i := capacity - 1; i >= 0; i-- {
						if err = c.TableSet(table, keys[i], IntVal(i+1)); err != HeapOK {
							t.Fatal(err)
						}
					}
				}
				if err = c.Collect(); err != HeapOK {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestStaticHashHintSurvivesGrowthAndHandleReuse(t *testing.T) {
	if staticStringSlots < 3 {
		t.Skip("requires three distinct static keys")
	}
	var c Context
	key, other, third := c.InternStatic("key"), c.InternStatic("other"), c.InternStatic("third")
	table, err := c.NewTableWithCaps(0, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	for i, value := range []Value{key, other, third} {
		if err = c.TableSet(table, value, IntVal(i+1)); err != HeapOK {
			t.Fatal(err)
		}
		if c.TableGet(table, key).Int() != 1 {
			t.Fatal("stale hint broke growing-table lookup")
		}
	}
	// Drop the only table reference and collect; the registry's hints remain.
	oldRef := table.Ref
	if err = c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	table, err = c.NewTableWithCaps(8, 1)
	if err != HeapOK || table.Ref != oldRef {
		t.Fatalf("expected reused handle%d got%d/%v", oldRef, table.Ref, err)
	}
	if err = c.TableSet(table, other, IntVal(9)); err != HeapOK {
		t.Fatal(err)
	}
	if c.TableGet(table, key) != NilVal() || c.TableGet(table, other).Int() != 9 {
		t.Fatal("stale hint accepted a different key in a reused handle")
	}
}
