package runtime

import "testing"

type gcFixture struct {
	c     Context
	slots [16]Value
}

func (f *gcFixture) root(i int, v Value) {
	f.slots[i] = v
	f.c.RegisterGlobal(&f.slots[i])
}

func TestAllocCollectAndHandleReuse(t *testing.T) {
	t.Parallel()
	f := new(gcFixture)
	s, err := f.c.NewString([]byte("keep"))
	if err != HeapOK {
		t.Fatalf("alloc: %d", err)
	}
	f.root(0, s)
	if f.c.heap.handles[s.Ref].State() != HandleAllocated {
		t.Fatal("alloc did not set HandleAllocated")
	}
	if err := f.c.Collect(); err != HeapOK {
		t.Fatalf("collect: %d", err)
	}
	if f.c.heap.copiedN == 0 {
		t.Fatal("live string was not forwarded during collect")
	}
	if f.c.heap.handles[s.Ref].State() != HandleAllocated {
		t.Fatal("live handle did not return to Allocated")
	}
	if !bytesEqual(f.c.stringBytes(f.slots[0]), []byte("keep")) {
		t.Fatal("string bytes lost after collect")
	}

	dead, err := f.c.NewString([]byte("drop"))
	if err != HeapOK {
		t.Fatalf("dead alloc: %d", err)
	}
	deadRef := dead.Ref
	if err := f.c.Collect(); err != HeapOK {
		t.Fatalf("collect dead: %d", err)
	}
	if f.c.heap.handles[deadRef].State() != HandleFree {
		t.Fatal("unreachable handle was not freed")
	}
	again, err := f.c.NewString([]byte("new"))
	if err != HeapOK {
		t.Fatalf("reuse alloc: %d", err)
	}
	if f.c.heap.handles[again.Ref].State() != HandleAllocated {
		t.Fatal("reused handle is not Allocated")
	}
}

func TestTableChainAndCycleSurviveCollect(t *testing.T) {
	t.Parallel()
	f := new(gcFixture)
	a, err := f.c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(0, a)
	b, err := f.c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	if err := f.c.TableSet(a, IntVal(1), b); err != HeapOK {
		t.Fatal(err)
	}
	cc, err := f.c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	if err := f.c.TableSet(b, IntVal(1), cc); err != HeapOK {
		t.Fatal(err)
	}
	if err := f.c.TableSet(cc, IntVal(1), IntVal(42)); err != HeapOK {
		t.Fatal(err)
	}
	if err := f.c.Collect(); err != HeapOK {
		t.Fatalf("collect chain: %d", err)
	}
	got := f.c.TableGet(f.c.TableGet(f.c.TableGet(f.slots[0], IntVal(1)), IntVal(1)), IntVal(1))
	if got.Int() != 42 {
		t.Fatalf("chain = %d", got.Int())
	}

	cycle, err := f.c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(1, cycle)
	if err := f.c.TableSet(cycle, IntVal(1), cycle); err != HeapOK {
		t.Fatal(err)
	}
	if err := f.c.Collect(); err != HeapOK {
		t.Fatalf("collect cycle: %d", err)
	}
	self := f.c.TableGet(f.slots[1], IntVal(1))
	if self.Kind != KindTable || self.Ref != f.slots[1].Ref {
		t.Fatal("cyclic table did not survive collect")
	}
}

func TestPushLocalAndTempAcrossAlloc(t *testing.T) {
	t.Parallel()
	f := new(gcFixture)
	s, err := f.c.NewString([]byte("hel"))
	if err != HeapOK {
		t.Fatal(err)
	}
	if !f.c.PushLocal(s) {
		t.Fatal("PushLocal failed")
	}
	garbage := 0
	for i := 0; i < 32; i++ {
		if _, err := f.c.NewString([]byte("g")); err != HeapOK {
			t.Fatalf("garbage %d: %d", i, err)
		}
		garbage++
	}
	if err := f.c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	if !bytesEqual(f.c.stringBytes(f.c.heap.roots.locals[0]), []byte("hel")) {
		t.Fatal("PushLocal did not keep the string live")
	}
	f.c.PopLocals(1)
	if err := f.c.Collect(); err != HeapOK {
		t.Fatal(err)
	}

	left, err := f.c.NewString([]byte("hel"))
	if err != HeapOK {
		t.Fatal(err)
	}
	if !f.c.PushTemp(left) {
		t.Fatal("PushTemp failed")
	}
	right := f.c.InternStatic("lo")
	concat, err := f.c.Concat(f.c.heap.roots.temps[0], right)
	if err != HeapOK {
		t.Fatalf("concat during temp: %d", err)
	}
	f.c.PopTemp()
	if !f.c.ValuesEqual(concat, f.c.InternStatic("hello")) {
		t.Fatal("PushTemp across Concat lost a string")
	}
}

func TestThousandCollectionsNoDrift(t *testing.T) {
	t.Parallel()
	f := new(gcFixture)
	tab, err := f.c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(0, tab)
	if err := f.c.TableSet(tab, IntVal(1), IntVal(7)); err != HeapOK {
		t.Fatal(err)
	}
	if err := f.c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	live := f.c.heap.cursor
	for i := 0; i < 1000; i++ {
		if err := f.c.Collect(); err != HeapOK {
			t.Fatalf("collect %d: %d", i, err)
		}
		if f.c.heap.cursor != live {
			t.Fatalf("cursor drifted at %d: %d vs %d", i, f.c.heap.cursor, live)
		}
	}
	if f.c.TableGetInt(f.slots[0], 1).Int() != 7 {
		t.Fatal("value lost after 1000 collections")
	}
}

func TestHeapExhaustionLiveSet(t *testing.T) {
	t.Parallel()
	f := new(gcFixture)
	parent, err := f.c.NewTableWithCaps(maxArrayCap, maxHashCap)
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(0, parent)
	for i := 1; i <= 8; i++ {
		child, err := f.c.NewTableWithCaps(maxArrayCap, maxHashCap)
		if err != HeapOK {
			t.Fatalf("child %d: %d", i, err)
		}
		if err := f.c.TableSet(parent, IntVal(i), child); err != HeapOK {
			t.Fatal(err)
		}
	}
	_, err = f.c.NewTableWithCaps(maxArrayCap, maxHashCap)
	if err != HeapOOM {
		t.Fatalf("10th table err = %d, want HeapOOM", err)
	}
	if f.c.heap.terminal != HeapOOM {
		t.Fatal("live-set exhaustion did not put the heap in a terminal state")
	}
	stats := f.c.PerformanceStats()
	if stats.HeapError != HeapOOM || stats.HeapCapacity != heapSize || stats.HeapAllocated > heapSize {
		t.Fatalf("OOM heap telemetry = %+v", stats)
	}
}

func TestTransactionalPreCollectionFailures(t *testing.T) {
	t.Parallel()
	c := new(Context)
	c.ensureHeap()
	head := c.heap.freeHead
	cursor := c.heap.cursor
	c.heap.collecting = true
	_, err := c.heap.allocObject(ObjString, 8)
	if err != HeapBusy {
		t.Fatalf("during collect: %d", err)
	}
	if c.heap.freeHead != head || c.heap.cursor != cursor || !c.heap.collecting {
		t.Fatal("HeapBusy mutated heap state")
	}
	c.heap.collecting = false

	head = c.heap.freeHead
	cursor = c.heap.cursor
	_, err = c.heap.allocObject(ObjString, 65535)
	if err != HeapOOM {
		t.Fatalf("oversized: %d", err)
	}
	if c.heap.freeHead != head || c.heap.cursor != cursor || c.heap.collecting {
		t.Fatal("oversized alloc mutated heap state")
	}
}

func TestCollectionFailureIsTerminal(t *testing.T) {
	t.Parallel()
	f := new(gcFixture)
	tab, err := f.c.NewTable()
	if err != HeapOK {
		t.Fatal(err)
	}
	f.root(0, tab)
	data, err := f.c.tableData(tab)
	if err != HeapOK {
		t.Fatal(err)
	}
	// This used to exploit uint16 arithmetic in the collector's layout
	// check. It must now be rejected before any out-of-object scan.
	writeU16(data, 2, 1)
	writeU16(data, 6, 32768)
	if err := f.c.Collect(); err != HeapCorrupt {
		t.Fatalf("corrupt table collect = %d, want HeapCorrupt", err)
	}
	if f.c.heap.terminal != HeapCorrupt || f.c.heap.collecting {
		t.Fatal("collection failure did not put the heap in a terminal state")
	}
	if _, err := f.c.objectBytes(tab.Ref); err != HeapCorrupt {
		t.Fatalf("object access after terminal collect = %d, want HeapCorrupt", err)
	}
}

func TestPopNegativeRootCountsAreNoOp(t *testing.T) {
	t.Parallel()
	c := new(Context)
	if !c.PushLocal(IntVal(1)) || !c.PushTemp(IntVal(2)) {
		t.Fatal("root push failed")
	}
	c.PopLocals(-1)
	c.PopTemps(-1)
	if c.heap.roots.localSP != 1 || c.heap.roots.tempCount != 1 {
		t.Fatal("negative pop changed root stack pointers")
	}
}

func TestCorruptRefAndSentinelAndScanQueue(t *testing.T) {
	t.Parallel()
	c := new(Context)
	c.ensureHeap()
	if err := c.heap.copyObject(invalidRef); err != HeapCorruptRef {
		t.Fatalf("0xFFFF: %d", err)
	}
	if err := c.heap.copyObject(maxHandles); err != HeapCorruptRef {
		t.Fatalf("maxHandles: %d", err)
	}
	if err := c.heap.copyObject(0); err != HeapCorruptRef {
		t.Fatalf("free handle 0: %d", err)
	}

	s, err := c.NewString([]byte("q"))
	if err != HeapOK {
		t.Fatal(err)
	}
	c.heap.scanLen = maxHandles
	if err := c.heap.copyObject(s.Ref); err != HeapOOM {
		t.Fatalf("full scan queue: %d", err)
	}

	c.ensureHeap()
	c.heap.initHeap()
	c.heap.freeHead = invalidRef
	_, err = c.heap.allocObject(ObjString, 4)
	if err != HeapOOM {
		t.Fatalf("sentinel: %d", err)
	}
}

func TestStaticStringsAreNotCollected(t *testing.T) {
	t.Parallel()
	f := new(gcFixture)
	s := f.c.InternStatic("stay")
	f.root(0, s)
	if err := f.c.Collect(); err != HeapOK {
		t.Fatal(err)
	}
	if f.slots[0].Kind != KindStaticString {
		t.Fatal("static string kind changed")
	}
	if !bytesEqual(f.c.stringBytes(f.slots[0]), []byte("stay")) {
		t.Fatal("static string bytes moved or lost")
	}
}

func TestCollectIfNeededThreshold(t *testing.T) {
	t.Parallel()
	c := new(Context)
	c.CollectIfNeeded()
	if c.heapReady {
		t.Fatal("unused heap should stay uninitialized")
	}
	c.ensureHeap()
	c.heap.cursor = uint32(heapSize)*3/4 + 1
	c.CollectIfNeeded()
	if c.heap.cursor > uint32(heapSize)*3/4 {
		t.Fatal("CollectIfNeeded did not collect past 75%")
	}
}
