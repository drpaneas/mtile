package runtime

import "testing"

func TestPoppedRootTailDoesNotRetainObjectsOrReusedHandles(t *testing.T) {
	if maxHandles < 4 || maxLocals < 3 || maxTemps < 3 {
		t.Skip("requires three simultaneous roots and handles")
	}
	for _, local := range []bool{true, false} {
		c := new(Context)
		push, pop := c.PushTemp, c.PopTemps
		if local {
			push, pop = c.PushLocal, c.PopLocals
		}
		keep, err := c.NewString([]byte("keep"))
		if err != HeapOK || !push(keep) {
			t.Fatal("live root setup failed")
		}
		child, err := c.NewString([]byte("child"))
		if err != HeapOK || !push(child) {
			t.Fatal("child root setup failed")
		}
		dead, err := c.NewTable()
		if err != HeapOK || !push(dead) || c.TableSet(dead, IntVal(1), child) != HeapOK {
			t.Fatal("table root setup failed")
		}
		pop(2)
		if err := c.Collect(); err != HeapOK {
			t.Fatalf("local%v collect: %v", local, err)
		}
		if c.heap.handles[child.Ref].State() != HandleFree || c.heap.handles[dead.Ref].State() != HandleFree ||
			string(c.stringBytes(keep)) != "keep" {
			t.Fatalf("local%v popped subtree survived or live prefix was lost", local)
		}
		reused, err := c.NewString([]byte("replacement"))
		if err != HeapOK || reused.Ref != child.Ref && reused.Ref != dead.Ref {
			t.Fatal("reclaimed handle was not reused")
		}
		if err := c.Collect(); err != HeapOK || c.heap.handles[reused.Ref].State() != HandleFree {
			t.Fatalf("local%v stale popped handle retained a replacement object", local)
		}
		// An inactive tail must be ignored even when it contains invalid refs.
		// A later successful push replaces one whole slot before increasing SP.
		if local {
			for i := c.heap.roots.localSP; i < maxLocals; i++ {
				c.heap.roots.locals[i] = Value{Kind: KindTable, Ref: invalidRef}
			}
		} else {
			for i := c.heap.roots.tempCount; i < maxTemps; i++ {
				c.heap.roots.temps[i] = Value{Kind: KindTable, Ref: invalidRef}
			}
		}
		if !push(IntVal(42)) || c.Collect() != HeapOK {
			t.Fatalf("local%v reused root slot exposed stale data", local)
		}
		pop(int(^uint(0) >> 1))
		if c.Collect() != HeapOK || c.heap.handleN != 0 {
			t.Fatalf("local%v excess pop did not remove every root", local)
		}
	}
}

func TestRootPopPreservesFullStackFailureAndCapacityReuse(t *testing.T) {
	for _, local := range []bool{true, false} {
		c := new(Context)
		push, pop, limit := c.PushTemp, c.PopTemps, maxTemps
		if local {
			push, pop, limit = c.PushLocal, c.PopLocals, maxLocals
		}
		unrooted, err := c.NewString([]byte("unrooted"))
		if err != HeapOK {
			t.Fatal(err)
		}
		for i := 0; i < limit; i++ {
			if !push(IntVal(i)) {
				t.Fatalf("local%v root%d rejected below capacity", local, i)
			}
		}
		if push(unrooted) || c.Collect() != HeapOK || c.heap.handles[unrooted.Ref].State() != HandleFree {
			t.Fatalf("local%v failed push changed active roots", local)
		}
		pop(0)
		pop(-1)
		if push(IntVal(0)) {
			t.Fatal("nonpositive pop changed full root stack")
		}
		reused, err := c.NewString([]byte("rooted"))
		if err != HeapOK {
			t.Fatal(err)
		}
		pop(1)
		if !push(reused) || push(IntVal(0)) || c.Collect() != HeapOK || string(c.stringBytes(reused)) != "rooted" {
			t.Fatalf("local%v freed root capacity was not safely reusable", local)
		}
		pop(limit + 1)
		if c.Collect() != HeapOK || c.heap.handleN != 0 {
			t.Fatal("full stack pop retained an object")
		}
	}
}

func TestRootPopDoesNotInitializeHeap(t *testing.T) {
	var c Context
	c.PopLocals(1)
	c.PopTemps(1)
	if c.heapReady {
		t.Fatal("popping an uninitialized stack initialized the heap")
	}
}
