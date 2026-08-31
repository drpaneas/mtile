package runtime

import "testing"

func TestSubChrOrdTostrTonum(t *testing.T) {
	t.Parallel()
	c := new(Context)
	s := c.InternStatic("hello")
	sub, err := c.Sub(s, IntVal(2), IntVal(4))
	if err != HeapOK {
		t.Fatal(err)
	}
	if !c.ValuesEqual(sub, c.InternStatic("ell")) {
		t.Fatal("sub(2,4)")
	}
	ch, err := c.Chr(IntVal(65))
	if err != HeapOK {
		t.Fatal(err)
	}
	if c.Ord(ch, IntVal(1)).Int() != 65 {
		t.Fatal("chr/ord")
	}
	if c.Ord(s, IntVal(0)).Kind != KindNil {
		t.Fatal("ord out of range")
	}
	n, err := c.Tostr(IntVal(12), false)
	if err != HeapOK {
		t.Fatal(err)
	}
	if !c.ValuesEqual(n, c.InternStatic("12")) {
		t.Fatalf("tostr 12 = %q", c.stringBytes(n))
	}
	hex, err := c.Tostr(IntVal(15), true)
	if err != HeapOK {
		t.Fatal(err)
	}
	if !c.ValuesEqual(hex, c.InternStatic("0x000f.0000")) {
		t.Fatalf("tostr hex = %q", c.stringBytes(hex))
	}
	if c.Tonum(c.InternStatic("1.5")).Num != One+One/2 {
		t.Fatal("tonum 1.5")
	}
	if c.Tonum(c.InternStatic("nope")).Kind != KindNil {
		t.Fatal("tonum invalid")
	}
}

func TestPrintVDispatchesStringKinds(t *testing.T) {
	requireSoftwareFramebuffer(t)
	t.Parallel()
	c := new(Context)
	c.Cls(3)
	hello := c.InternStatic("A")
	c.PrintV(hello, IntVal(0), IntVal(0), IntVal(7))
	if c.Pget(2, 0) != 7 {
		t.Fatal("PrintV static string")
	}
	heap, err := c.NewString([]byte("A"))
	if err != HeapOK {
		t.Fatal(err)
	}
	c.PrintV(heap, IntVal(8), IntVal(0), IntVal(8))
	if c.Pget(10, 0) != 8 {
		t.Fatal("PrintV heap string")
	}
}

func TestTostrRootsTableAcrossAllocation(t *testing.T) {
	t.Parallel()
	c := new(Context)
	tab, err := c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := c.NewTable(); err != HeapOK {
			t.Fatalf("garbage allocation %d: %d", i, err)
		}
	}
	// Leave all existing handles valid but force the next small allocation
	// through the collection path.
	c.heap.cursor = uint32(heapSize) - alignUp4(uint32(2+len("[table]"))) + 1
	if _, err := c.Tostr(tab, false); err != HeapOK {
		t.Fatal(err)
	}
	if err := c.TableSet(tab, IntVal(1), IntVal(7)); err != HeapOK {
		t.Fatalf("table was reclaimed during tostr: %d", err)
	}
	if got := c.TableGetInt(tab, 1).Int(); got != 7 {
		t.Fatalf("table value after tostr = %d, want 7", got)
	}
}
