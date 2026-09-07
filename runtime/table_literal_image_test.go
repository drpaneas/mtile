package runtime

import (
	"bytes"
	"fmt"
	"testing"
)

func TestTableLiteralImageMatchesOrdinaryInsertion(t *testing.T) {
	for capacity := 1; capacity <= maxHashCap; capacity++ {
		var c Context
		arrayCap := capacity % 17
		fields := make([]TableLiteralImageField, 0, capacity)
		occupied := make([]bool, capacity)
		want, _ := c.NewTableWithCaps(arrayCap, capacity)
		for i := 0; i < capacity; i++ {
			key := c.InternStatic(fmt.Sprintf("img_%d", i))
			slot := hashBucket(c.hashValue(key), uint16(capacity))
			for occupied[slot] {
				slot = (slot + 1) % capacity
			}
			occupied[slot] = true
			value := IntVal(i - 40)
			switch i % 3 {
			case 1:
				value = BoolVal(i%2 == 0)
			case 2:
				value = key
			}
			fields = append(fields, TableLiteralImageField{Slot: slot, KeyID: key.SRef, Value: value, HasValue: true})
			if err := c.TableSet(want, key, value); err != HeapOK {
				t.Fatal(err)
			}
		}
		image := BuildTableLiteralImage(arrayCap, capacity, fields)
		got, err := c.NewTableFromImage(image)
		if err != HeapOK {
			t.Fatal(err)
		}
		assertLiteralTablesEqual(t, &c, got, want)
		if !c.PushLocal(got) || !c.PushLocal(want) {
			t.Fatal("root")
		}
		if err := c.Collect(); err != HeapOK {
			t.Fatal(err)
		}
		assertLiteralTablesEqual(t, &c, got, want)
		c.PopLocals(2)
	}
}

func TestNewTableFromImageOverwritesUnclearedPayload(t *testing.T) {
	for _, shift := range []uint32{0, 1, 2, 3} {
		var c Context
		key := c.InternStatic("filled")
		c.ensureHeap()
		slot := hashBucket(c.hashValue(key), 4)
		image := BuildTableLiteralImage(2, 4, []TableLiteralImageField{
			{Slot: slot, KeyID: key.SRef, Value: IntVal(9), HasValue: true},
		})
		if image == nil {
			t.Fatal("image")
		}
		space := c.activeSpace()
		live := c.heap.cursor
		start := live + shift
		for i := live; i < uint32(len(space)); i++ {
			space[i] = 0xa5
		}
		c.heap.cursor = start
		got, err := c.NewTableFromImage(image)
		if err != HeapOK {
			t.Fatalf("shift%d: %v", shift, err)
		}
		header := c.heap.handles[got.Ref]
		if header.Offset != start || int(header.Size) != len(image) {
			t.Fatalf("shift%d header %+v", shift, header)
		}
		payload := space[header.Offset : header.Offset+uint32(header.Size)]
		if !bytes.Equal(payload, image) {
			t.Fatalf("shift%d payload does not match image", shift)
		}
		rounded := alignUp4(uint32(header.Size))
		for at := uint32(header.Size); at < rounded; at++ {
			if space[header.Offset+at] != 0 {
				t.Fatalf("shift%d padding byte%d=%02x", shift, at, space[header.Offset+at])
			}
		}
		if shift > 0 && space[start-1] != 0xa5 {
			t.Fatalf("shift%d wrote before the payload", shift)
		}
		if header.Offset+rounded < uint32(len(space)) && space[header.Offset+rounded] != 0xa5 {
			t.Fatalf("shift%d wrote after rounded padding", shift)
		}
		want, _ := c.NewTableWithCaps(2, 4)
		if err := c.TableSet(want, key, IntVal(9)); err != HeapOK {
			t.Fatal(err)
		}
		assertLiteralTablesEqual(t, &c, got, want)
	}
}

func TestTableLiteralImageOverlayPreservesLayoutAndEvaluation(t *testing.T) {
	var c Context
	keys := []Value{c.InternStatic("x"), c.InternStatic("y"), c.InternStatic("spd"), c.InternStatic("w")}
	occupied := make([]bool, 4)
	fields := make([]TableLiteralImageField, len(keys))
	want, _ := c.NewTableWithCaps(1, 4)
	for i, key := range keys {
		slot := hashBucket(c.hashValue(key), 4)
		for occupied[slot] {
			slot = (slot + 1) % 4
		}
		occupied[slot] = true
		fields[i] = TableLiteralImageField{Slot: slot, KeyID: key.SRef}
	}
	got, err := c.NewTableFromImage(BuildTableLiteralImage(1, 4, fields))
	if err != HeapOK {
		t.Fatal(err)
	}
	view := c.BorrowTableLiteral(got)
	for i, key := range keys {
		value := IntVal((i + 1) * 9)
		plan := literalPlanForTest(1, 4, fields[i].Slot, i)
		if ge, we := view.StorePlanned(key.SRef, value, plan), c.TableSet(want, key, value); ge != we {
			t.Fatalf("overlay %d errors %v/%v", i, ge, we)
		}
	}
	assertLiteralTablesEqual(t, &c, got, want)
}

func TestTableLiteralImageOverlayFallbackInvalidatesBorrow(t *testing.T) {
	for _, reason := range []string{"nil", "missing-key", "caps", "nil-table", "terminal"} {
		t.Run(reason, func(t *testing.T) {
			var c Context
			key := c.InternStatic("key")
			slot := hashBucket(c.hashValue(key), 1)
			image := BuildTableLiteralImage(1, 1, []TableLiteralImageField{{Slot: slot, KeyID: key.SRef}})
			got, want := Value{}, Value{}
			if reason != "missing-key" && reason != "nil-table" {
				got, _ = c.NewTableFromImage(image)
				want, _ = c.NewTableFromImage(image)
			} else if reason == "missing-key" {
				got, _ = c.NewTableWithCaps(1, 1)
				want, _ = c.NewTableWithCaps(1, 1)
			}
			view := c.BorrowTableLiteral(got)
			value := IntVal(7)
			arrayCap := 1
			switch reason {
			case "nil":
				value = NilVal()
			case "caps":
				arrayCap = 2
			case "nil-table":
				got, want = NilVal(), NilVal()
				view = c.BorrowTableLiteral(got)
			case "terminal":
				c.heap.terminal = HeapOOM
			}
			plan := literalPlanForTest(arrayCap, 1, slot, 0)
			if ge, we := view.StorePlanned(key.SRef, value, plan), c.TableSet(want, key, value); ge != we {
				t.Fatalf("errors %v/%v", ge, we)
			}
			if view.data != nil {
				t.Fatal("fallback retained a borrowed heap pointer")
			}
			if reason == "nil-table" || reason == "terminal" {
				return
			}
			assertLiteralTablesEqual(t, &c, got, want)
		})
	}
}

func TestNewTableFromImageRejectsMalformedBytes(t *testing.T) {
	var c Context
	table, _ := c.NewTableWithCaps(1, 2)
	before := append([]byte(nil), c.activeSpace()...)
	for _, image := range [][]byte{nil, {1}, BuildTableLiteralImage(1, 2, nil)[:8]} {
		got, err := c.NewTableFromImage(image)
		if got.Kind != KindNil || err != HeapCorrupt || c.heap.terminal != HeapOK {
			t.Fatalf("malformed image %d: %#v %v terminal %v", len(image), got, err, c.heap.terminal)
		}
	}
	if !bytes.Equal(before, c.activeSpace()) || c.heap.handles[table.Ref].State() != HandleAllocated {
		t.Fatal("malformed image mutated heap state")
	}
}

func TestStorePlannedNumberMatchesStorePlanned(t *testing.T) {
	var planned, numbered Context
	key := planned.InternStatic("x")
	_ = numbered.InternStatic("x")
	slot := hashBucket(planned.hashValue(key), 1)
	image := BuildTableLiteralImage(1, 1, []TableLiteralImageField{{Slot: slot, KeyID: key.SRef}})
	left, err := planned.NewTableFromImage(image)
	if err != HeapOK {
		t.Fatal(err)
	}
	right, err := numbered.NewTableFromImage(image)
	if err != HeapOK {
		t.Fatal(err)
	}
	plan := literalPlanForTest(1, 1, slot, 0)
	for _, n := range []Number{0, FromInt(-3), FromInt(9), Number(1)} {
		leftView := planned.BorrowTableLiteral(left)
		rightView := numbered.BorrowTableLiteral(right)
		if ge, ne := leftView.StorePlanned(key.SRef, NumVal(n), plan), rightView.StorePlannedNumber(key.SRef, n, plan); ge != ne {
			t.Fatalf("n=%v errors %v/%v", n, ge, ne)
		}
		if planned.TableGet(left, key) != numbered.TableGet(right, key) {
			t.Fatalf("n=%v stored %#v vs %#v", n, planned.TableGet(left, key), numbered.TableGet(right, key))
		}
		if numbered.TableGet(right, key) != NumVal(n) {
			t.Fatalf("n=%v did not store a number", n)
		}
	}
}

func TestStorePlannedNumberFallbackInvalidatesBorrow(t *testing.T) {
	var c Context
	key := c.InternStatic("key")
	slot := hashBucket(c.hashValue(key), 1)
	got, _ := c.NewTableWithCaps(1, 1)
	view := c.BorrowTableLiteral(got)
	if err := view.StorePlannedNumber(key.SRef, FromInt(7), literalPlanForTest(1, 1, slot, 0)); err != HeapOK {
		t.Fatal(err)
	}
	if view.data != nil {
		t.Fatal("missing image key retained a borrowed heap pointer")
	}
	if c.TableGet(got, key) != IntVal(7) {
		t.Fatal("fallback did not store the number")
	}
}

func BenchmarkStorePlannedOverlay(b *testing.B) {
	benchmarkStorePlannedOverlay(b, false)
}

func BenchmarkStorePlannedNumberOverlay(b *testing.B) {
	benchmarkStorePlannedOverlay(b, true)
}

func benchmarkStorePlannedOverlay(b *testing.B, numeric bool) {
	var c Context
	key := c.InternStatic("x")
	slot := hashBucket(c.hashValue(key), 1)
	image := BuildTableLiteralImage(1, 1, []TableLiteralImageField{{Slot: slot, KeyID: key.SRef}})
	table, err := c.NewTableFromImage(image)
	if err != HeapOK {
		b.Fatal(err)
	}
	plan := literalPlanForTest(1, 1, slot, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		view := c.BorrowTableLiteral(table)
		var storeErr HeapError
		if numeric {
			storeErr = view.StorePlannedNumber(key.SRef, FromInt(i&255), plan)
		} else {
			storeErr = view.StorePlanned(key.SRef, IntVal(i&255), plan)
		}
		if storeErr != HeapOK {
			b.Fatal(storeErr)
		}
	}
}
