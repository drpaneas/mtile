package runtime

import "testing"

func TestNumericHashKeysPreserveValuesAcrossGrowthAndCollection(t *testing.T) {
	var c Context
	table, err := c.NewTableWithCaps(0, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	c.RegisterGlobal(&table)
	keys := []Number{0, -1, 1, -One, One, -2147483648, 2147483647, One + 1, -One - 1}
	for i := 1; i <= 64; i++ {
		keys = append(keys, FromInt(-i-1))
	}
	for i, key := range keys {
		if err := c.TableSet(table, NumVal(key), IntVal(i+1)); err != HeapOK {
			t.Fatalf("insert key %d: %v", key, err)
		}
	}
	if err := c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	for i, key := range keys {
		if got := c.TableGet(table, NumVal(key)); got != IntVal(i+1) {
			t.Fatalf("key %d after growth/GC = %+v, want %d", key, got, i+1)
		}
		if err := c.TableSet(table, NumVal(key), NilVal()); err != HeapOK {
			t.Fatal(err)
		}
		if got := c.TableGet(table, NumVal(key)); got.Kind != KindNil {
			t.Fatalf("deleted key %d = %+v", key, got)
		}
	}
}

// Integer keys outside the array part must use the integer bits of 16:16
// Numbers when selecting a hash bucket. Otherwise every key probes bucket 0.
func TestIntegerHashUsesIntegerBits(t *testing.T) {
	var c Context
	for _, start := range []int{-128, 0, 256} {
		seen := [128]bool{}
		for i := start; i < start+len(seen); i++ {
			bucket := c.hashValue(IntVal(i)) % uint32(len(seen))
			if seen[bucket] {
				t.Fatalf("consecutive integer %d reuses bucket %d", i, bucket)
			}
			seen[bucket] = true
		}
	}
}

var numericHashSink Value

func BenchmarkTableNumericHash(b *testing.B) {
	for _, test := range []struct {
		name string
		base int
	}{{"Negative", -64}, {"SparsePositive", 1024}} {
		b.Run(test.name, func(b *testing.B) {
			var c Context
			table, err := c.NewTableWithCaps(0, 128)
			if err != HeapOK {
				b.Fatal(err)
			}
			var keys [64]Value
			for i := range keys {
				keys[i] = IntVal(test.base + i)
				if err := c.TableSet(table, keys[i], IntVal(i+1)); err != HeapOK {
					b.Fatal(err)
				}
			}
			b.Run("Get", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					numericHashSink = c.TableGet(table, keys[i%len(keys)])
				}
			})
			b.Run("Set", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					c.TableSet(table, keys[i%len(keys)], IntVal(i))
				}
			})
		})
	}
}
