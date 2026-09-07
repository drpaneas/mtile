package codeeditor

import "testing"

func TestWordRange(t *testing.T) {
	tests := []struct {
		name, text string
		offset     int
		start, end int
	}{
		{"empty", "", 0, 0, 0},
		{"negative offset", "player", -5, 0, 6},
		{"identifier", "local player_x2 = 4", 9, 6, 15},
		{"Unicode rune offsets", "πλάγιο_2 = 4", 3, 0, 8},
		{"Unicode after spaces", "  ταχύτητα_２", 7, 2, 12},
		{"space run", "x \t  y", 2, 1, 5},
		{"Unicode whitespace", "x\u2003\t y", 2, 1, 4},
		{"leading spaces", "x\n  y", 2, 2, 4},
		{"punctuation", "call(x, y)", 6, 6, 7},
		{"adjacent punctuation", "x ~= (y)", 2, 2, 3},
		{"end of line", "player\nnext", 6, 0, 6},
		{"end of line whitespace", "player  \nnext", 8, 6, 8},
		{"end of line punctuation", "player()\nnext", 8, 7, 8},
		{"EOF", "player", 6, 0, 6},
		{"past EOF", "player", 50, 0, 6},
		{"EOF whitespace", "player  ", 8, 6, 8},
		{"blank first line", "\nnext", 0, 0, 0},
		{"blank middle line", "player\n\nnext", 7, 7, 7},
		{"trailing empty line", "player\n", 7, 7, 7},
		{"next line start", "first\nsecond", 6, 6, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(tt.text)
			start, end := e.WordRange(tt.offset)
			if start != tt.start || end != tt.end {
				t.Fatalf("WordRange(%d) = [%d,%d), want [%d,%d)", tt.offset, start, end, tt.start, tt.end)
			}
			if e.Cursor() != 0 || e.Anchor() != 0 || e.Dirty() {
				t.Fatal("query changed the document or selection")
			}
		})
	}
}

func TestLineRange(t *testing.T) {
	tests := []struct {
		name, text string
		offset     int
		start, end int
	}{
		{"empty", "", 0, 0, 0},
		{"first line", "first\nsecond\nthird", 2, 0, 6},
		{"newline", "first\nsecond\nthird", 5, 0, 6},
		{"next line start", "first\nsecond\nthird", 6, 6, 13},
		{"last line EOF", "first\nsecond", 12, 6, 12},
		{"blank line", "first\n\nthird", 6, 6, 7},
		{"trailing empty line", "first\n", 6, 6, 6},
		{"Unicode offsets", "πρώτη\nάλλη\n", 8, 6, 11},
		{"negative offset", "x\ny", -3, 0, 2},
		{"past EOF", "x\ny", 99, 2, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(tt.text)
			start, end := e.LineRange(tt.offset)
			if start != tt.start || end != tt.end {
				t.Fatalf("LineRange(%d) = [%d,%d), want [%d,%d)", tt.offset, start, end, tt.start, tt.end)
			}
		})
	}
}

func TestMouseSelectionsCanBeReplacedAndUndone(t *testing.T) {
	e := New("local player_x = 1\nlocal y = 2\n")
	e.SelectWord(9)
	if e.SelectedText() != "player_x" || e.Anchor() != 6 || e.Cursor() != 14 {
		t.Fatalf("word selection: %q, anchor=%d, cursor=%d", e.SelectedText(), e.Anchor(), e.Cursor())
	}
	if e.Dirty() || e.Undo() {
		t.Fatal("selection created a text edit")
	}
	e.Insert("ball_x")
	if e.Text() != "local ball_x = 1\nlocal y = 2\n" {
		t.Fatalf("word replacement: %q", e.Text())
	}
	if !e.Undo() || e.SelectedText() != "player_x" {
		t.Fatalf("undo did not restore selected word: %q", e.SelectedText())
	}
	e.SelectLine(20)
	if e.SelectedText() != "local y = 2\n" {
		t.Fatalf("line selection: %q", e.SelectedText())
	}
	e.DeleteSelection()
	if e.Text() != "local player_x = 1\n" {
		t.Fatalf("line deletion left a newline: %q", e.Text())
	}
	if !e.Undo() || e.SelectedText() != "local y = 2\n" {
		t.Fatal("undo did not restore selected whole line")
	}
}

func TestWordAndLineRangesSupportReverseDrag(t *testing.T) {
	e := New("first second\nthird fourth")
	start, end := e.WordRange(8)
	e.SetCursor(end, false)
	left, _ := e.WordRange(1)
	e.SetCursor(left, true)
	if start != 6 || e.Anchor() != 12 || e.Cursor() != 0 || e.SelectedText() != "first second" {
		t.Fatalf("reverse word drag: %q, anchor=%d, cursor=%d", e.SelectedText(), e.Anchor(), e.Cursor())
	}
	_, end = e.LineRange(17)
	e.SetCursor(end, false)
	start, _ = e.LineRange(2)
	e.SetCursor(start, true)
	if e.SelectedText() != e.Text() || e.Cursor() != 0 || e.Anchor() != len([]rune(e.Text())) {
		t.Fatal("reverse line drag did not cover the whole document")
	}
}
