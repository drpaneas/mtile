package codeeditor

import "testing"

func TestEditSelectionUndoRedo(t *testing.T) {
	e := New("hello world")
	e.SetCursor(5, false)
	e.SetCursor(11, true)
	e.Insert(" Lua")
	if got := e.Text(); got != "hello Lua" {
		t.Fatalf("insert = %q", got)
	}
	if !e.Undo() || e.Text() != "hello world" {
		t.Fatalf("undo = %q", e.Text())
	}
	if !e.Redo() || e.Text() != "hello Lua" {
		t.Fatalf("redo = %q", e.Text())
	}
}

func TestVerticalMotionKeepsPreferredColumn(t *testing.T) {
	e := New("abcdef\nx\nabcdef")
	e.SetCursor(e.Offset(0, 5), false)
	e.MoveVertical(1, false)
	if line, column := e.LineColumn(e.Cursor()); line != 1 || column != 1 {
		t.Fatalf("short line = %d:%d", line, column)
	}
	e.MoveVertical(1, false)
	if line, column := e.LineColumn(e.Cursor()); line != 2 || column != 5 {
		t.Fatalf("restored column = %d:%d", line, column)
	}
}

func TestNewlineAutoIndentAndSelectedIndent(t *testing.T) {
	e := New("function _draw()")
	e.SetCursor(len([]rune(e.Text())), false)
	e.Newline()
	e.Insert("cls()")
	if got := e.Text(); got != "function _draw()\n    cls()" {
		t.Fatalf("auto indent = %q", got)
	}
	e.SelectAll()
	e.Indent(false)
	if got := e.Text(); got != "    function _draw()\n        cls()" {
		t.Fatalf("indent = %q", got)
	}
	e.Indent(true)
	if got := e.Text(); got != "function _draw()\n    cls()" {
		t.Fatalf("outdent = %q", got)
	}
}

func TestWordDeleteAndDirty(t *testing.T) {
	e := New("local player")
	e.SetCursor(len([]rune(e.Text())), false)
	if !e.DeleteBackward(true) || e.Text() != "local " {
		t.Fatalf("word delete = %q", e.Text())
	}
	if !e.Dirty() {
		t.Fatal("edited document should be dirty")
	}
	e.MarkSaved()
	if e.Dirty() {
		t.Fatal("saved document should be clean")
	}
}
