package main

import "testing"

func TestUndoThenImmediateEditClearsRedoHistory(t *testing.T) {
	var u undoStack
	u.reset(undoSnapshot{CurrentSprite: 0})
	u.forcePush(undoSnapshot{CurrentSprite: 1})
	if _, ok := u.undo(); !ok {
		t.Fatal("undo did not return the initial state")
	}

	// This happens within the normal 150 ms drag debounce window. It must
	// still create a new branch and invalidate the old redo state.
	u.push(undoSnapshot{CurrentSprite: 2})
	if len(u.future) != 0 {
		t.Fatal("new edit after undo left stale redo history")
	}
	if len(u.past) != 2 || u.past[1].CurrentSprite != 2 {
		t.Fatalf("new edit was not recorded immediately: past=%#v", u.past)
	}
}
