package runtime

import (
	"testing"
	"unsafe"
)

func TestValueLayout(t *testing.T) {
	t.Parallel()
	if unsafe.Sizeof(Value{}) != valueBytes {
		t.Fatalf("Value size = %d, want %d", unsafe.Sizeof(Value{}), valueBytes)
	}
	if unsafe.Sizeof(ObjectHeader{}) != 8 {
		t.Fatalf("ObjectHeader size = %d, want 8", unsafe.Sizeof(ObjectHeader{}))
	}
}

func TestContextDoesNotEmbedSemispaces(t *testing.T) {
	t.Parallel()
	if got := unsafe.Sizeof(Context{}); got >= 128*1024 {
		t.Fatalf("Context size = %d; semispaces must remain outside Context", got)
	}
}

func TestTruthyAndIntOr(t *testing.T) {
	t.Parallel()
	if NilVal().Truthy() || BoolVal(false).Truthy() {
		t.Fatal("nil and false must be falsy")
	}
	if !IntVal(0).Truthy() || !BoolVal(true).Truthy() {
		t.Fatal("0 and true must be truthy")
	}
	if IntVal(7).IntOr(1) != 7 || NilVal().IntOr(1) != 1 {
		t.Fatal("IntOr")
	}
	if NilVal().Add(IntVal(1)).Kind != KindNil {
		t.Fatal("Add on nil should be nil")
	}
}

func TestValueArithmetic(t *testing.T) {
	t.Parallel()
	if IntVal(3).Add(IntVal(2)).Int() != 5 {
		t.Fatal("3+2")
	}
	if IntVal(3).Mul(IntVal(2)).Int() != 6 {
		t.Fatal("3*2")
	}
	if IntVal(0).Sgn().Int() != 1 {
		t.Fatal("sgn(0)")
	}
	got := IntVal(1).Div(IntVal(0))
	if got.Num != Number(0x7FFFFFFF) {
		t.Fatal("1/0")
	}
}

func TestStaticAndHeapStringEquality(t *testing.T) {
	t.Parallel()
	c := new(Context)
	hello := c.InternStatic("hello")
	hel := c.InternStatic("hel")
	lo := c.InternStatic("lo")
	concat, err := c.Concat(hel, lo)
	if err != HeapOK {
		t.Fatalf("concat: %d", err)
	}
	if !c.ValuesEqual(hello, concat) {
		t.Fatal("static hello != heap hel..lo")
	}
	tab, err := c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableSet(tab, hello, IntVal(9)); err != HeapOK {
		t.Fatal(err)
	}
	if c.TableGet(tab, concat).Int() != 9 {
		t.Fatal("table lookup did not ignore string representation")
	}
}

func TestNewTableUsesHintsAndRejectsOversizedHints(t *testing.T) {
	t.Parallel()
	c := new(Context)
	small, err := c.NewTableWithCaps(2, 1)
	if err != HeapOK {
		t.Fatal(err)
	}
	data, err := c.tableData(small)
	if err != HeapOK {
		t.Fatal(err)
	}
	if readU16(data, 2) != 2 || readU16(data, 6) != 1 {
		t.Fatal("table capacity hints were not stored")
	}
	if _, err := c.NewTableWithCaps(maxArrayCap+1, 1); err != HeapOOM {
		t.Fatalf("oversized array hint = %d, want HeapOOM", err)
	}
	if _, err := c.NewTableWithCaps(1, maxHashCap+1); err != HeapOOM {
		t.Fatalf("oversized hash hint = %d, want HeapOOM", err)
	}
}
