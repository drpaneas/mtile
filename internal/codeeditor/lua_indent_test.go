package codeeditor

import (
	"strings"
	"testing"
)

func editorWithCaret(source string) *Editor {
	at := strings.Index(source, "|")
	e := New(strings.Replace(source, "|", "", 1))
	e.SetCursor(len([]rune(source[:at])), false)
	return e
}

func textWithCaret(e *Editor) string {
	text := []rune(e.Text())
	return string(text[:e.Cursor()]) + "|" + string(text[e.Cursor():])
}

func TestLuaNewline(t *testing.T) {
	cases := []struct{ source, want string }{
		{"if ready then|", "if ready then\n    |"},
		{"for i=1,3 do -- repeat is a comment|", "for i=1,3 do -- repeat is a comment\n    |"},
		{"repeat|", "repeat\n    |"},
		{"local update = function()|", "local update = function()\n    |"},
		{"if ready then\n  run()\nelse|", "if ready then\n  run()\nelse\n  |"},
		{"if ready then\n  run()\nelseif other then|", "if ready then\n  run()\nelseif other then\n  |"},
		{"if ready then\n  run()\n  end|", "if ready then\n  run()\nend\n|"},
		{"if ready then\n  run()\nend|", "if ready then\n  run()\nend\n|"},
		{"if ready then run() end|", "if ready then run() end\n|"},
		{"local word = 'then' -- do|", "local word = 'then' -- do\n|"},
		{"-- function fake()|", "-- function fake()\n|"},
		{"local tiles = {|}", "local tiles = {\n    |\n}"},
		{"function f()\n  local tiles = {|}", "function f()\n  local tiles = {\n    |\n  }"},
		{"call(|)", "call(\n    |\n)"},
		{"if x then| end", "if x then\n    |\nend"},
		{"local s = [=[ literal then| end ]=]", "local s = [=[ literal then\n| end ]=]"},
		{"local s = 'literal| end'", "local s = 'literal\n| end'"},
		{"function f()\n\tcall()|", "function f()\n\tcall()\n\t|"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			e := editorWithCaret(tc.source)
			e.Newline()
			if got := textWithCaret(e); got != tc.want {
				t.Fatalf("got %q; want %q", got, tc.want)
			}
			if !e.Undo() || textWithCaret(e) != tc.source {
				t.Fatalf("undo = %q; want %q", textWithCaret(e), tc.source)
			}
		})
	}
}

func TestLuaTypedClosersAndLiteralPaste(t *testing.T) {
	cases := []struct{ source, typed, want string }{
		{"function f()\n    |", "end ", "function f()\nend |"},
		{"if x then\n  run()\n  |", "else ", "if x then\n  run()\nelse |"},
		{"if x then\n  run()\n  |", "elseif ", "if x then\n  run()\nelseif |"},
		{"repeat\n    |", "until ", "repeat\nuntil |"},
		{"local t = {\n    |", "}", "local t = {\n}|"},
		{"call({\n    |", "})", "call({\n})|"},
		{"local s = [[\n    |", "end", "local s = [[\n    end|"},
		{"function f()\n    -- |", "end", "function f()\n    -- end|"},
		{"function f()\n    |", "endless", "function f()\n    endless|"},
		{"function f()\n    label = '|", "end", "function f()\n    label = 'end|"},
	}
	for _, tc := range cases {
		t.Run(tc.source+tc.typed, func(t *testing.T) {
			e := editorWithCaret(tc.source)
			e.InsertTyped(tc.typed)
			if got := textWithCaret(e); got != tc.want {
				t.Fatalf("got %q; want %q", got, tc.want)
			}
			if !e.Undo() || textWithCaret(e) != tc.source {
				t.Fatalf("undo = %q", textWithCaret(e))
			}
		})
	}
	e := editorWithCaret("function f()\n    |")
	e.Insert("end")
	if got := textWithCaret(e); got != "function f()\n    end|" {
		t.Fatalf("paste was changed: %q", got)
	}
}

func TestFormatLuaIndentationPreservesContentAndSelection(t *testing.T) {
	source := "function f()\n  local x = 3.55553 -- do not round\nif x then\nlocal μήλο = {\n1,\n2,\n}\nelse\nprint('end', \"then\")\nend\nend\n"
	want := "function f()\n  local x = 3.55553 -- do not round\n  if x then\n    local μήλο = {\n      1,\n      2,\n    }\n  else\n    print('end', \"then\")\n  end\nend\n"
	e := New(source)
	start := len([]rune(source[:strings.Index(source, "μήλο")]))
	e.SetCursor(start+len([]rune("μήλο")), false)
	e.SetCursor(start, true) // backwards selection must remain backwards
	oldCursor, oldAnchor := e.Cursor(), e.Anchor()
	if !e.FormatIndentation() || e.Text() != want {
		t.Fatalf("formatted = %q; want %q", e.Text(), want)
	}
	if e.SelectedText() != "μήλο" || e.Cursor() >= e.Anchor() {
		t.Fatalf("selection moved: %q (%d,%d)", e.SelectedText(), e.Cursor(), e.Anchor())
	}
	if e.FormatIndentation() {
		t.Fatal("format must be idempotent")
	}
	if !e.Undo() || e.Text() != source || e.Cursor() != oldCursor || e.Anchor() != oldAnchor {
		t.Fatalf("single undo failed: %q", e.Text())
	}
	if e.Undo() {
		t.Fatal("format should create one undo entry")
	}
	if !e.Redo() || e.Text() != want || e.SelectedText() != "μήλο" {
		t.Fatal("redo did not restore formatted selection")
	}
}

func TestFormatLuaPreservesMultilineLiteralsAndComments(t *testing.T) {
	source := "function f()\nlocal value = [==[\n  end then {\n     exact spaces\n]==]\n--[=[\n if not code then\n    ]=]\nlocal escaped = 'a\\\n  end then'\nif x then y() end\nrepeat\ncall(\n{\nx=1\n})\nuntil done\nend"
	want := "function f()\n    local value = [==[\n  end then {\n     exact spaces\n]==]\n    --[=[\n if not code then\n    ]=]\n    local escaped = 'a\\\n  end then'\n    if x then y() end\n    repeat\n        call(\n            {\n                x=1\n        })\n    until done\nend"
	e := New(source)
	e.FormatIndentation()
	if e.Text() != want {
		t.Fatalf("formatted = %q; want %q", e.Text(), want)
	}
}

func TestLuaIndentAndBackspaceUseFileStyle(t *testing.T) {
	e := editorWithCaret("function f()\n  x()\n    |")
	e.DeleteBackward(false)
	if got := textWithCaret(e); got != "function f()\n  x()\n  |" {
		t.Fatalf("backspace = %q", got)
	}
	e.Indent(false)
	if got := textWithCaret(e); got != "function f()\n  x()\n    |" {
		t.Fatalf("tab = %q", got)
	}
	e = editorWithCaret("a\n|")
	e.DeleteBackward(false)
	if got := textWithCaret(e); got != "a|" {
		t.Fatalf("backspace at line start = %q", got)
	}
	e = editorWithCaret("abc|")
	e.Indent(false)
	if got := textWithCaret(e); got != "abc |" {
		t.Fatalf("tab stop = %q", got)
	}
}

func TestWordNavigationAndDeletionCrossPunctuation(t *testing.T) {
	e := editorWithCaret("player|.x += 1")
	e.MoveHorizontal(1, false, true)
	if got := textWithCaret(e); got != "player.|x += 1" {
		t.Fatalf("word right = %q", got)
	}
	e.MoveHorizontal(-1, false, true)
	if got := textWithCaret(e); got != "player|.x += 1" {
		t.Fatalf("word left = %q", got)
	}
	e.DeleteForward(true)
	if got := textWithCaret(e); got != "player|x += 1" {
		t.Fatalf("word delete = %q", got)
	}
	e = editorWithCaret("value +=| 1")
	e.DeleteBackward(true)
	if got := textWithCaret(e); got != "value | 1" {
		t.Fatalf("word backspace = %q", got)
	}
}

func TestLuaTypingKeywordPrefixDoesNotOutdentIdentifier(t *testing.T) {
	e := editorWithCaret("function f()\n    |")
	for _, r := range "endless" {
		e.InsertTyped(string(r))
	}
	if got := textWithCaret(e); got != "function f()\n    endless|" {
		t.Fatalf("identifier was dedented: %q", got)
	}
	e = editorWithCaret("function f()\n    |")
	for _, r := range "end" {
		e.InsertTyped(string(r))
	}
	e.Newline()
	if got := textWithCaret(e); got != "function f()\nend\n|" {
		t.Fatalf("Enter did not align end: %q", got)
	}
}

func TestLuaSplitCloserUndoRestoresSelection(t *testing.T) {
	e := New("if x then replace end")
	e.SetCursor(len("if x then replace"), false)
	e.SetCursor(len("if x then"), true)
	cursor, anchor := e.Cursor(), e.Anchor()
	e.Newline()
	if got := textWithCaret(e); got != "if x then\n    |\nend" {
		t.Fatalf("split = %q", got)
	}
	if !e.Undo() || e.Text() != "if x then replace end" || e.Cursor() != cursor || e.Anchor() != anchor {
		t.Fatalf("undo did not restore original selection: %q (%d,%d)", e.Text(), e.Cursor(), e.Anchor())
	}
}

func TestOutdentClampsCaretWithinIndent(t *testing.T) {
	e := editorWithCaret("function f()\n |   x()")
	e.Indent(true)
	if got := textWithCaret(e); got != "function f()\n|x()" {
		t.Fatalf("caret escaped its line: %q", got)
	}
}

func TestDeleteUndoRestoresUnselectedCaret(t *testing.T) {
	for _, backwards := range []bool{false, true} {
		e := editorWithCaret("a|b")
		if backwards {
			e.DeleteBackward(false)
		} else {
			e.DeleteForward(false)
		}
		if !e.Undo() || textWithCaret(e) != "a|b" || e.Anchor() != e.Cursor() {
			t.Fatalf("undo selected deleted text: %q (%d,%d)", e.Text(), e.Cursor(), e.Anchor())
		}
	}
}

func TestLineCacheSurvivesEveryEditAndHistory(t *testing.T) {
	e := New("function f()\n  x()\nend")
	check := func() {
		t.Helper()
		lines := strings.Split(e.Text(), "\n")
		if e.LineCount() != len(lines) {
			t.Fatalf("cached line count %d; want %d", e.LineCount(), len(lines))
		}
		at := 0
		for line, text := range lines {
			if e.Line(line) != text {
				t.Fatalf("cached line %d = %q; want %q", line, e.Line(line), text)
			}
			for col := 0; col <= len([]rune(text)); col++ {
				if e.Offset(line, col) != at+col {
					t.Fatalf("cached offset %d:%d = %d; want %d", line, col, e.Offset(line, col), at+col)
				}
				gotLine, gotCol := e.LineColumn(at + col)
				if gotLine != line || gotCol != col {
					t.Fatalf("cached position %d = %d:%d; want %d:%d", at+col, gotLine, gotCol, line, col)
				}
			}
			at += len([]rune(text)) + 1
		}
	}
	check()
	e.SetCursor(e.Offset(1, 5), false)
	e.Newline()
	check()
	e.Insert("local μήλο = {\nx=1\n}")
	check()
	e.FormatIndentation()
	check()
	e.SelectAll()
	e.Indent(false)
	check()
	e.Indent(true)
	check()
	e.SetCursor(0, false)
	e.DeleteForward(false)
	check()
	e.SetCursor(len([]rune(e.Text())), false)
	e.DeleteBackward(false)
	check()
	for e.Undo() {
		check()
	}
	for e.Redo() {
		check()
	}
	e.SelectAll()
	e.DeleteSelection()
	check()
	e.SetText("νέο\nαρχείο\n")
	check()
	allocations := testing.AllocsPerRun(100, func() {
		e.LineCount()
		e.LineColumn(2)
		e.Offset(1, 2)
	})
	if allocations != 0 {
		t.Fatalf("cached layout lookup allocated %g times", allocations)
	}
}

func TestIndentStyleIgnoresOneMisindentedLine(t *testing.T) {
	e := New("function f()\n  first()\n wrong()\n  last()\nend")
	if unit := e.indentationUnit(); unit != "  " {
		t.Fatalf("style = %q; want two spaces", unit)
	}
	e.FormatIndentation()
	if want := "function f()\n  first()\n  wrong()\n  last()\nend"; e.Text() != want {
		t.Fatalf("format = %q; want %q", e.Text(), want)
	}
}
