package runtime

import (
	"strings"
	"testing"
)

func TestStaticStringEqualityMatchesBytewiseReference(t *testing.T) {
	t.Parallel()
	c := new(Context)
	texts := []string{"", "a", "b", "same-prefix-a", "same-prefix-b", "a\x00b", "a\x00c", "σύννεφο", strings.Repeat("x", MaxStringBytes)}
	if len(texts) > len(c.staticTab) {
		texts = texts[:len(c.staticTab)]
	}
	values := make([]Value, 0, len(texts)+2)
	for _, text := range texts {
		first := c.InternStatic(text)
		// Equal bytes from a separate allocation must reuse the ID too.
		second := c.InternStatic(string(append([]byte(nil), text...)))
		if first.Kind != KindStaticString || first != second {
			t.Fatalf("InternStatic did not deduplicate %q: %+v / %+v", text, first, second)
		}
		values = append(values, first)
	}
	if int(c.staticN) != len(texts) {
		t.Fatalf("registry has %d entries for %d distinct strings", c.staticN, len(texts))
	}
	// Malformed refs historically decode to empty bytes. Keep that behavior,
	// including comparison against a valid empty string, outside the fast path.
	values = append(values, Value{Kind: KindStaticString, SRef: c.staticN}, Value{Kind: KindStaticString, SRef: ^uint16(0)})
	for _, left := range values {
		for _, right := range values {
			want := bytesEqual(c.stringBytes(left), c.stringBytes(right))
			if got := c.ValuesEqual(left, right); got != want {
				t.Fatalf("refs %d/%d: equality=%v, bytewise=%v", left.SRef, right.SRef, got, want)
			}
		}
		for _, other := range []Value{NilVal(), BoolVal(false), IntVal(0)} {
			if c.ValuesEqual(left, other) || c.ValuesEqual(other, left) {
				t.Fatalf("string %+v compared equal to non-string %+v", left, other)
			}
		}
	}
}

func TestStaticStringEqualityWithDynamicKeysAfterCollection(t *testing.T) {
	t.Parallel()
	f := new(gcFixture)
	c := &f.c
	if len(c.staticTab) < 5 {
		t.Skip("mixed-storage fixture needs five distinct static strings")
	}
	key := c.InternStatic("particle")
	concat, err := c.Concat(c.InternStatic("part"), c.InternStatic("icle"))
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(0, concat)
	sub, err := c.Sub(c.InternStatic("[particle]"), IntVal(2), IntVal(9))
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(1, sub)
	empty, err := c.NewString(nil)
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(2, empty)
	table, err := c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(3, table)
	if err := c.TableSet(table, key, IntVal(17)); err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableSet(table, concat, IntVal(23)); err != HeapOK {
		t.Fatal(err)
	}
	if err := c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	for _, heapKey := range []Value{f.slots[0], f.slots[1]} {
		if !c.ValuesEqual(key, heapKey) || !c.ValuesEqual(heapKey, key) {
			t.Fatal("equal static/heap bytes differ after collection")
		}
		if got := c.TableGet(f.slots[3], heapKey).Int(); got != 23 {
			t.Fatalf("dynamic key lookup after collection=%d, want 23", got)
		}
	}
	if got := c.TableGet(f.slots[3], key).Int(); got != 23 {
		t.Fatalf("static key lookup after dynamic overwrite=%d, want 23", got)
	}
	staticEmpty := c.InternStatic("")
	if !c.ValuesEqual(staticEmpty, f.slots[2]) || !c.ValuesEqual(f.slots[2], staticEmpty) {
		t.Fatal("empty heap/static strings differ")
	}
	if c.ValuesEqual(key, f.slots[2]) || c.ValuesEqual(f.slots[2], key) {
		t.Fatal("nonempty static and empty heap strings compare equal")
	}
}
