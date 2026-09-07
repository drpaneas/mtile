package runtime

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"strings"
	"testing"
)

func fnvStringReference(text string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(text))
	return h.Sum32()
}

func TestStaticHashMatchesFNVAndInvalidRefBehavior(t *testing.T) {
	var c Context
	texts := []string{"", "a", "x", "speed", "same-prefix-a", "same-prefix-b", "a\x00b", "σύννεφο", strings.Repeat("x", MaxStringBytes)}
	if len(texts) > len(c.staticTab) {
		texts = texts[:len(c.staticTab)]
	}
	for _, text := range texts {
		first := c.InternStatic(text)
		second := c.InternStatic(string(append([]byte(nil), text...)))
		if first.Kind != KindStaticString || first.SRef != second.SRef {
			t.Fatal("static registry did not deduplicate")
		}
		if got, want := c.hashValue(first), fnvStringReference(text); got != want {
			t.Fatalf("hash(%q)=%x want%x", text, got, want)
		}
	}
	for _, ref := range []uint16{c.staticN, ^uint16(0)} {
		value := Value{Kind: KindStaticString, SRef: ref}
		if got, want := c.hashValue(value), fnvStringReference(""); got != want {
			t.Fatalf("invalid static ref%d hash%x want%x", ref, got, want)
		}
	}
}

func TestStaticHashRepeatedProbesDoNotAllocate(t *testing.T) {
	var c Context
	key := c.InternStatic("dash_acceleration")
	want := fnvStringReference("dash_acceleration")
	if got := testing.AllocsPerRun(100, func() {
		if c.hashValue(key) != want {
			panic("static hash changed")
		}
	}); got != 0 {
		t.Fatalf("static hash probes allocated %g objects", got)
	}
}

func TestStaticHashPreservesMixedStringSlotsAndPairsAfterGC(t *testing.T) {
	for _, capacity := range []int{1, 2, 3, 5, 8, 16, 31, 64, 128} {
		if capacity > staticStringSlots {
			continue
		}
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			var c Context
			cached, err := c.NewTableWithCaps(0, capacity)
			if err != HeapOK {
				t.Fatal(err)
			}
			reference, err := c.NewTableWithCaps(0, capacity)
			if err != HeapOK {
				t.Fatal(err)
			}
			c.RegisterGlobal(&cached)
			c.RegisterGlobal(&reference)
			statics := make([]Value, capacity)
			dynamic := make([]Value, capacity)
			for i := 0; i < capacity; i++ {
				text := fmt.Sprintf("field-%03d", i)
				statics[i] = c.InternStatic(text)
				dynamic[i], err = c.NewString([]byte(text))
				if err != HeapOK {
					t.Fatal(err)
				}
				if err = c.TableSet(cached, statics[i], IntVal(i+1)); err != HeapOK {
					t.Fatal(err)
				}
				if err = c.TableSet(reference, dynamic[i], IntVal(i+1)); err != HeapOK {
					t.Fatal(err)
				}
			}
			compare := func() {
				for i := 0; i < c.TableSlotCount(cached); i++ {
					a, b := c.TableKeyAt(cached, i), c.TableKeyAt(reference, i)
					if (a.Kind == KindNil) != (b.Kind == KindNil) || !bytes.Equal(c.stringBytes(a), c.stringBytes(b)) {
						t.Fatalf("pairs slot%d changed", i)
					}
					if !c.ValuesEqual(c.TableValueAt(cached, i), c.TableValueAt(reference, i)) {
						t.Fatalf("value slot%d changed", i)
					}
				}
			}
			compare()
			// Use the opposite storage kind for deletes and reinsertions. Identical
			// byte strings must keep the same probe/tombstone behavior and pairs order.
			for i := 0; i < capacity; i += 3 {
				if err = c.TableSet(cached, dynamic[i], NilVal()); err != HeapOK {
					t.Fatal(err)
				}
				if err = c.TableSet(reference, statics[i], NilVal()); err != HeapOK {
					t.Fatal(err)
				}
			}
			compare()
			for i := capacity - 1; i >= 0; i-- {
				if err = c.TableSet(cached, dynamic[i], IntVal(i+7)); err != HeapOK {
					t.Fatal(err)
				}
				if err = c.TableSet(reference, statics[i], IntVal(i+7)); err != HeapOK {
					t.Fatal(err)
				}
			}
			compare()
			for pass := 0; pass < 3; pass++ {
				if err = c.Collect(); err != HeapOK {
					t.Fatal(err)
				}
				compare()
				for i, key := range statics {
					if got := c.TableGet(cached, key).Int(); got != i+7 {
						t.Fatalf("cached lookup after GC=%d want%d", got, i+7)
					}
					if got := c.TableGet(reference, key).Int(); got != i+7 {
						t.Fatalf("mixed lookup after GC=%d want%d", got, i+7)
					}
				}
			}
		})
	}
}

func BenchmarkStaticStringHash(b *testing.B) {
	for _, length := range []int{0, 4, 16, 64, 4096} {
		b.Run(fmt.Sprint(length), func(b *testing.B) {
			var c Context
			key := c.InternStatic(strings.Repeat("x", length))
			var sum uint32
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sum ^= c.hashValue(key)
			}
			b.StopTimer()
			if b.N&1 != 0 && sum != fnvStringReference(strings.Repeat("x", length)) {
				b.Fatal(sum)
			}
		})
	}
}

func BenchmarkColdStaticFieldLookup(b *testing.B) {
	var c Context
	table, err := c.NewTableWithCaps(0, 8)
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
