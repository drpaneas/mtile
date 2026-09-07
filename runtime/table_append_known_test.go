package runtime

import (
	"bytes"
	"fmt"
	"testing"
)

func TestTableAppendKnownPreservesLayoutAcrossNilAttempts(t *testing.T) {
	for mask := 0; mask < 256; mask++ {
		var c Context
		got, _ := c.NewTableWithCaps(8, 1)
		want, _ := c.NewTableWithCaps(8, 1)
		for at := 1; at <= 8; at++ {
			value := IntVal(at * 3)
			if at%3 == 0 {
				value = BoolVal(false)
			}
			if mask&(1<<(at-1)) != 0 {
				value = NilVal()
			}
			ge := c.TableAppendKnown(got, value, at)
			we := c.TableAdd(want, value, NilVal())
			if ge != we {
				t.Fatalf("mask %d attempt %d errors %v/%v", mask, at, ge, we)
			}
			assertLiteralTablesEqual(t, &c, got, want)
		}
	}
}

func TestTableAppendKnownFallbacksKeepOrdinaryAppendSemantics(t *testing.T) {
	for _, at := range []int{-int(^uint(0)>>1) - 1, -1, 0, 1, 2, 3, 4, 17, int(^uint(0) >> 1)} {
		t.Run(fmt.Sprint(at), func(t *testing.T) {
			var c Context
			got, _ := c.NewTableWithCaps(2, 1)
			want, _ := c.NewTableWithCaps(2, 1)
			for _, list := range []Value{got, want} {
				_ = c.TableAdd(list, IntVal(10), NilVal())
			}
			if ge, we := c.TableAppendKnown(got, IntVal(20), at), c.TableAdd(want, IntVal(20), NilVal()); ge != we {
				t.Fatalf("errors %v/%v", ge, we)
			}
			assertLiteralTablesEqual(t, &c, got, want)
			// A full-capacity or occupied-slot mismatch must use normal growth.
			if ge, we := c.TableAppendKnown(got, IntVal(30), at), c.TableAdd(want, IntVal(30), NilVal()); ge != we {
				t.Fatalf("growth errors %v/%v", ge, we)
			}
			assertLiteralTablesEqual(t, &c, got, want)
		})
	}
}

func TestTableAppendKnownResolvesAfterCollectionAndPreservesCanaries(t *testing.T) {
	var c Context
	_, _ = c.NewTableWithCaps(10, 1) // Dead leading allocation forces movement.
	list, _ := c.NewTableWithCaps(8, 1)
	left, _ := c.NewTableWithCaps(3, 2)
	right, _ := c.NewTableWithCaps(3, 2)
	for _, root := range []Value{list, left, right} {
		if !c.PushLocal(root) {
			t.Fatal("cannot root fixture")
		}
	}
	defer c.PopLocals(3)
	leftData, _ := c.tableData(left)
	rightData, _ := c.tableData(right)
	leftBefore, rightBefore := bytes.Clone(leftData), bytes.Clone(rightData)
	before := c.heap.handles[list.Ref].Offset
	for at := 1; at <= 8; at++ {
		value, _ := c.NewTableWithCaps(1, 1)
		if !c.PushLocal(value) {
			t.Fatal("cannot root value")
		}
		_ = c.TableSet(value, IntVal(1), IntVal(at))
		if err := c.Collect(); err != HeapOK {
			t.Fatal(err)
		}
		if err := c.TableAppendKnown(list, value, at); err != HeapOK {
			t.Fatal(err)
		}
		c.PopLocals(1)
	}
	if before == c.heap.handles[list.Ref].Offset {
		t.Fatal("fixture did not move list")
	}
	for at := 1; at <= 8; at++ {
		if got := c.TableGetInt(c.TableGetInt(list, at), 1); got != IntVal(at) {
			t.Fatalf("lost value %d after collection: %#v", at, got)
		}
	}
	leftData, _ = c.tableData(left)
	rightData, _ = c.tableData(right)
	if !bytes.Equal(leftData, leftBefore) || !bytes.Equal(rightData, rightBefore) {
		t.Fatal("modified adjacent allocation")
	}
}

func TestTableAppendKnownRetainsValidationErrors(t *testing.T) {
	var c Context
	valid, _ := c.NewTableWithCaps(2, 1)
	numeric, _ := c.NewNumberArray(2)
	for _, list := range []Value{NilVal(), IntVal(0), {Kind: KindTable, Ref: invalidRef}, {Kind: KindTable, Ref: Ref(maxHandles - 1)}, numeric} {
		if ge, we := c.TableAppendKnown(list, IntVal(1), 1), c.TableAdd(list, IntVal(1), NilVal()); ge != we {
			t.Fatalf("invalid %#v errors %v/%v", list, ge, we)
		}
	}
	data, _ := c.tableData(valid)
	writeU16(data, 2, 256)
	before := bytes.Clone(c.activeSpace())
	if ge, we := c.TableAppendKnown(valid, IntVal(1), 1), c.TableAdd(valid, IntVal(1), NilVal()); ge != we || ge == HeapOK {
		t.Fatalf("corrupt header errors %v/%v", ge, we)
	}
	if !bytes.Equal(before, c.activeSpace()) {
		t.Fatal("corrupt header changed heap bytes")
	}
}
