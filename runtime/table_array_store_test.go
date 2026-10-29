package runtime

import (
	"bytes"
	"testing"
)

func TestTryTableSetArrayPreservesTaggedValuesAndTableLength(t *testing.T) {
	var c Context
	table, err := c.NewTableWithCaps(4, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	child, _ := c.NewTableWithCaps(1, 1)
	text, _ := c.NewString([]byte("retained reference"))
	values := []Value{NumVal(-One / 2), BoolVal(true), text, child, NilVal()}
	for _, value := range values {
		beforeCursor, beforeHandles := c.heap.cursor, c.heap.handleN
		if !c.TryTableSetArray(table, IntVal(1), value) {
			t.Fatal("existing slot was not handled")
		}
		if got := c.TableGetInt(table, 1); got != value {
			t.Fatalf("stored %+v, want %+v", got, value)
		}
		if c.heap.cursor != beforeCursor || c.heap.handleN != beforeHandles {
			t.Fatal("existing-slot store allocated")
		}
		wantLen := 1
		if value.Kind == KindNil {
			wantLen = 0
		}
		if got := c.TableLen(table); got != wantLen {
			t.Fatalf("length=%d want%d", got, wantLen)
		}
	}
	// A reference stored without temporary roots must be traced through its
	// owner table when a later, unrelated allocation collects the heap.
	if !c.TryTableSetArray(table, IntVal(1), text) || !c.TryTableSetArray(table, IntVal(2), child) {
		t.Fatal("reference store failed")
	}
	if !c.PushLocal(table) {
		t.Fatal("root owner")
	}
	defer c.PopLocals(1)
	if err := c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	if got := c.TableGetInt(table, 1); c.StrLen(got) != len("retained reference") {
		t.Fatalf("string did not survive collection: %+v", got)
	}
	if got := c.TableSet(c.TableGetInt(table, 2), IntVal(1), IntVal(9)); got != HeapOK {
		t.Fatalf("child did not survive collection: %v", got)
	}
}

func TestTryTableSetArrayFallbackDoesNotMutateOrAllocate(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  Value
	}{
		{"nil", NilVal()}, {"boolean", BoolVal(true)}, {"fraction", NumVal(One + One/2)},
		{"zero", IntVal(0)}, {"negative", IntVal(-1)}, {"growth", IntVal(5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c Context
			table, _ := c.NewTableWithCaps(4, 1)
			before := append([]byte(nil), c.activeSpace()...)
			cursor, handles := c.heap.cursor, c.heap.handleN
			if c.TryTableSetArray(table, tc.key, IntVal(99)) {
				t.Fatal("unsupported key claimed")
			}
			if !bytes.Equal(before, c.activeSpace()) || c.heap.cursor != cursor || c.heap.handleN != handles {
				t.Fatal("fallback changed the table or allocated")
			}
			if err := c.TableSet(table, tc.key, IntVal(99)); err != HeapOK {
				t.Fatal(err)
			}
			if tc.key.Kind != KindNil && c.TableGet(table, tc.key) != IntVal(99) {
				t.Fatal("normal fallback lost the store")
			}
		})
	}
	var c Context
	for _, invalid := range []Value{NilVal(), IntVal(3), {Kind: KindTable, Ref: invalidRef}} {
		if c.TryTableSetArray(invalid, IntVal(1), IntVal(9)) {
			t.Fatal("invalid owner claimed")
		}
	}
	array, _ := c.NewNumberArray(4)
	if c.TryTableSetArray(array, IntVal(1), IntVal(9)) {
		t.Fatal("compact array treated as a generic table")
	}
}
