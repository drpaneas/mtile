// Package codeeditor contains the display-independent editing model used by
// Mtile's Lua workspace. Keeping text mutation out of the Ebitengine view
// makes keyboard behavior testable and prevents rendering concerns from
// corrupting project source.
package codeeditor

import (
	"sort"
	"strings"
	"unicode"
)

const historyLimit = 256

type snapshot struct {
	text   []rune
	cursor int
	anchor int
}

// Editor is a rune-indexed, single-document text editor. Cursor and Anchor are
// insertion offsets in [0,len(Text)]. A non-empty range between them is the
// current selection.
type Editor struct {
	text       []rune
	starts     []int // cached line starts; nil after any text mutation
	cursor     int
	anchor     int
	preferredX int
	undo       []snapshot
	redo       []snapshot
	saved      string
}

func New(source string) *Editor {
	e := &Editor{text: []rune(normalize(source)), saved: normalize(source), preferredX: -1}
	return e
}

func normalize(source string) string {
	return strings.ReplaceAll(strings.ReplaceAll(source, "\r\n", "\n"), "\r", "\n")
}

func (e *Editor) Text() string { return string(e.text) }
func (e *Editor) Cursor() int  { return e.cursor }
func (e *Editor) Anchor() int  { return e.anchor }
func (e *Editor) Dirty() bool  { return e.Text() != e.saved }

func (e *Editor) MarkSaved() { e.saved = e.Text() }

func (e *Editor) SetText(source string) {
	e.text = []rune(normalize(source))
	e.starts = nil
	e.cursor = min(e.cursor, len(e.text))
	e.anchor = e.cursor
	e.preferredX = -1
	e.undo = nil
	e.redo = nil
	e.saved = string(e.text)
}

func (e *Editor) SetCursor(offset int, selecting bool) {
	offset = clamp(offset, 0, len(e.text))
	e.cursor = offset
	if !selecting {
		e.anchor = offset
	}
	e.preferredX = -1
}

func (e *Editor) Selection() (start, end int, ok bool) {
	if e.cursor == e.anchor {
		return e.cursor, e.cursor, false
	}
	return min(e.cursor, e.anchor), max(e.cursor, e.anchor), true
}

func (e *Editor) SelectedText() string {
	start, end, ok := e.Selection()
	if !ok {
		return ""
	}
	return string(e.text[start:end])
}

func (e *Editor) SelectAll() {
	e.anchor = 0
	e.cursor = len(e.text)
	e.preferredX = -1
}

func (e *Editor) pushUndo() {
	s := snapshot{text: append([]rune(nil), e.text...), cursor: e.cursor, anchor: e.anchor}
	e.undo = append(e.undo, s)
	if len(e.undo) > historyLimit {
		e.undo = append([]snapshot(nil), e.undo[len(e.undo)-historyLimit:]...)
	}
	e.redo = nil
}

func (e *Editor) restore(s snapshot) {
	e.text = append(e.text[:0], s.text...)
	e.starts = nil
	e.cursor = clamp(s.cursor, 0, len(e.text))
	e.anchor = clamp(s.anchor, 0, len(e.text))
	e.preferredX = -1
}

func (e *Editor) Undo() bool {
	if len(e.undo) == 0 {
		return false
	}
	e.redo = append(e.redo, snapshot{text: append([]rune(nil), e.text...), cursor: e.cursor, anchor: e.anchor})
	s := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.restore(s)
	return true
}

func (e *Editor) Redo() bool {
	if len(e.redo) == 0 {
		return false
	}
	e.undo = append(e.undo, snapshot{text: append([]rune(nil), e.text...), cursor: e.cursor, anchor: e.anchor})
	s := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.restore(s)
	return true
}

func (e *Editor) deleteSelectionNoHistory() bool {
	start, end, ok := e.Selection()
	if !ok {
		return false
	}
	e.text = append(e.text[:start], e.text[end:]...)
	e.starts = nil
	e.cursor = start
	e.anchor = start
	return true
}

func (e *Editor) ReplaceSelection(value string) {
	value = normalize(value)
	if value == "" {
		e.DeleteSelection()
		return
	}
	e.pushUndo()
	e.replaceSelectionNoHistory(value)
}

func (e *Editor) replaceSelectionNoHistory(value string) {
	e.deleteSelectionNoHistory()
	runes := []rune(value)
	at := e.cursor
	e.text = append(e.text, make([]rune, len(runes))...)
	copy(e.text[at+len(runes):], e.text[at:len(e.text)-len(runes)])
	copy(e.text[at:], runes)
	e.starts = nil
	e.cursor = at + len(runes)
	e.anchor = e.cursor
	e.preferredX = -1
}

func (e *Editor) Insert(value string) { e.ReplaceSelection(value) }

func (e *Editor) DeleteSelection() bool {
	if _, _, ok := e.Selection(); !ok {
		return false
	}
	e.pushUndo()
	e.deleteSelectionNoHistory()
	e.preferredX = -1
	return true
}

func (e *Editor) DeleteBackward(word bool) bool {
	if _, _, ok := e.Selection(); ok {
		return e.DeleteSelection()
	}
	if e.cursor == 0 {
		return false
	}
	start := e.cursor - 1
	if word {
		start = wordBoundaryLeft(e.text, e.cursor)
	} else {
		_, column := e.LineColumn(e.cursor)
		lineStart := e.cursor - column
		prefix := string(e.text[lineStart:e.cursor])
		if column > 0 && strings.Trim(prefix, " ") == "" {
			unit := e.indentationUnit()
			if unit != "\t" {
				start = e.cursor - (column-1)%len(unit) - 1
			}
		}
	}
	e.pushUndo()
	e.anchor = start
	e.deleteSelectionNoHistory()
	e.preferredX = -1
	return true
}

func (e *Editor) DeleteForward(word bool) bool {
	if _, _, ok := e.Selection(); ok {
		return e.DeleteSelection()
	}
	if e.cursor == len(e.text) {
		return false
	}
	end := e.cursor + 1
	if word {
		end = wordBoundaryRight(e.text, e.cursor)
	}
	e.pushUndo()
	e.anchor = end
	e.deleteSelectionNoHistory()
	e.preferredX = -1
	return true
}

func (e *Editor) Newline() { e.smartNewline() }

// Indent shifts every selected line, or inserts one indentation unit when no
// range is selected. Both operations use the document's indentation style.
func (e *Editor) Indent(outdent bool) { e.smartIndent(outdent) }

func (e *Editor) MoveHorizontal(delta int, selecting, word bool) {
	if !selecting {
		if start, end, ok := e.Selection(); ok {
			if delta < 0 {
				e.cursor = start
			} else {
				e.cursor = end
			}
			e.anchor = e.cursor
			e.preferredX = -1
			return
		}
	}
	if delta < 0 {
		if word {
			e.cursor = wordBoundaryLeft(e.text, e.cursor)
		} else {
			e.cursor = max(0, e.cursor-1)
		}
	} else if delta > 0 {
		if word {
			e.cursor = wordBoundaryRight(e.text, e.cursor)
		} else {
			e.cursor = min(len(e.text), e.cursor+1)
		}
	}
	if !selecting {
		e.anchor = e.cursor
	}
	e.preferredX = -1
}

func (e *Editor) MoveVertical(delta int, selecting bool) {
	line, column := e.LineColumn(e.cursor)
	if e.preferredX < 0 {
		e.preferredX = column
	}
	target := clamp(line+delta, 0, e.LineCount()-1)
	e.cursor = e.Offset(target, e.preferredX)
	if !selecting {
		e.anchor = e.cursor
	}
}

func (e *Editor) MoveLineBoundary(end, selecting bool) {
	line, _ := e.LineColumn(e.cursor)
	if end {
		e.cursor = e.Offset(line, len([]rune(e.Line(line))))
	} else {
		e.cursor = e.Offset(line, 0)
	}
	if !selecting {
		e.anchor = e.cursor
	}
	e.preferredX = -1
}

func (e *Editor) LineCount() int { return len(e.lineStarts()) }

func (e *Editor) lineStarts() []int {
	if e.starts != nil {
		return e.starts
	}
	starts := []int{0}
	for i, r := range e.text {
		if r == '\n' {
			starts = append(starts, i+1)
		}
	}
	e.starts = starts
	return starts
}

func (e *Editor) Line(line int) string {
	starts := e.lineStarts()
	if line < 0 || line >= len(starts) {
		return ""
	}
	end := len(e.text)
	if line+1 < len(starts) {
		end = starts[line+1] - 1
	}
	return string(e.text[starts[line]:end])
}

func (e *Editor) LineColumn(offset int) (line, column int) {
	offset = clamp(offset, 0, len(e.text))
	starts := e.lineStarts()
	line = sort.Search(len(starts), func(i int) bool { return starts[i] > offset }) - 1
	if line < 0 {
		line = 0
	}
	return line, offset - starts[line]
}

func (e *Editor) Offset(line, column int) int {
	starts := e.lineStarts()
	line = clamp(line, 0, len(starts)-1)
	end := len(e.text)
	if line+1 < len(starts) {
		end = starts[line+1] - 1
	}
	return starts[line] + clamp(column, 0, end-starts[line])
}

func wordBoundaryLeft(text []rune, at int) int {
	at = clamp(at, 0, len(text))
	for at > 0 && unicode.IsSpace(text[at-1]) {
		at--
	}
	if at > 0 {
		word := isWord(text[at-1])
		for at > 0 && !unicode.IsSpace(text[at-1]) && isWord(text[at-1]) == word {
			at--
		}
	}
	return at
}

func wordBoundaryRight(text []rune, at int) int {
	at = clamp(at, 0, len(text))
	for at < len(text) && unicode.IsSpace(text[at]) {
		at++
	}
	if at < len(text) {
		word := isWord(text[at])
		for at < len(text) && !unicode.IsSpace(text[at]) && isWord(text[at]) == word {
			at++
		}
	}
	return at
}

func isWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

func clamp(value, low, high int) int { return min(max(value, low), high) }
