package codeeditor

import (
	"strings"
	"unicode"
)

// This is deliberately a lexer, not a Lua formatter: indentation must work
// while code is incomplete, and must never rewrite literal values or comments.
type luaLexState struct {
	longEquals int // -1 when outside a long string/comment
	quote      rune
}

type luaIndentLine struct {
	tokens    []string
	protected bool // line begins inside a literal or a multiline comment
	after     []string
	level     int
	state     luaLexState
}

func newLuaLexState() luaLexState { return luaLexState{longEquals: -1} }

func luaLongBracket(text []rune, at int, open bool) (equals, size int) {
	bracket := '['
	if !open {
		bracket = ']'
	}
	if at >= len(text) || text[at] != bracket {
		return 0, 0
	}
	end := at + 1
	for end < len(text) && text[end] == '=' {
		end++
	}
	if end < len(text) && text[end] == bracket {
		return end - at - 1, end - at + 1
	}
	return 0, 0
}

func lexLuaIndentLine(line string, state *luaLexState) []string {
	text := []rune(line)
	var tokens []string
	for at := 0; at < len(text); {
		if state.longEquals >= 0 {
			equals, size := luaLongBracket(text, at, false)
			if size > 0 && equals == state.longEquals {
				state.longEquals = -1
				at += size
			} else {
				at++
			}
			continue
		}
		if state.quote != 0 {
			switch text[at] {
			case '\\':
				at += 2
			case state.quote:
				state.quote = 0
				at++
			default:
				at++
			}
			continue
		}
		if text[at] == '-' && at+1 < len(text) && text[at+1] == '-' {
			equals, size := luaLongBracket(text, at+2, true)
			if size == 0 {
				break
			}
			state.longEquals = equals
			at += 2 + size
			continue
		}
		if equals, size := luaLongBracket(text, at, true); size > 0 {
			state.longEquals = equals
			at += size
			continue
		}
		if text[at] == '\'' || text[at] == '"' {
			state.quote = text[at]
			at++
			continue
		}
		if unicode.IsLetter(text[at]) || text[at] == '_' {
			end := at + 1
			for end < len(text) && isWord(text[end]) {
				end++
			}
			tokens = append(tokens, string(text[at:end]))
			at = end
			continue
		}
		if !unicode.IsSpace(text[at]) {
			tokens = append(tokens, string(text[at]))
		}
		at++
	}
	return tokens
}

func closeLuaIndent(stack []string, token string) []string {
	for i := len(stack) - 1; i >= 0; i-- {
		matches := false
		switch token {
		case "end":
			matches = stack[i] == "function" || stack[i] == "then" || stack[i] == "do"
		case "else", "elseif":
			matches = stack[i] == "then"
		case "until":
			matches = stack[i] == "repeat"
		case ")":
			matches = stack[i] == "("
		case "]":
			matches = stack[i] == "["
		case "}":
			matches = stack[i] == "{"
		}
		if matches {
			return stack[:i]
		}
	}
	return stack
}

func luaLineIndentLevel(stack []string, tokens []string) int {
	// Multiple closing delimiters (e.g. "})") all align with their openers.
	for _, token := range tokens {
		switch token {
		case "end", "until", "else", "elseif", ")", "]", "}":
			stack = closeLuaIndent(stack, token)
		default:
			return len(stack)
		}
		if token == "else" || token == "elseif" || token == "until" {
			break
		}
	}
	return len(stack)
}

func luaIndentLines(source string) []luaIndentLine {
	state := newLuaLexState()
	var stack []string
	lines := strings.Split(source, "\n")
	result := make([]luaIndentLine, len(lines))
	for i, line := range lines {
		item := &result[i]
		item.protected = state.longEquals >= 0 || state.quote != 0
		item.tokens = lexLuaIndentLine(line, &state)
		item.level = luaLineIndentLevel(stack, item.tokens)
		for _, token := range item.tokens {
			switch token {
			case "function", "then", "do", "repeat", "(", "[", "{":
				stack = append(stack, token)
			case "end", "until", "elseif", ")", "]", "}":
				stack = closeLuaIndent(stack, token)
				// "else" reuses its existing "then" block. "elseif" closes it
				// above and its following "then" opens the replacement branch.
			}
		}
		item.after = append([]string(nil), stack...)
		item.state = state
	}
	return result
}

func leadingLuaWhitespace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// indentationUnit prefers the document's existing indentation. Consistently
// indented block lines outweigh a stray badly indented line; two and four
// spaces win ambiguous ties. Blank lines and multiline text cannot influence
// the result, and deliberately three-space projects retain their convention.
func (e *Editor) indentationUnit() string {
	lines := strings.Split(e.Text(), "\n")
	info := luaIndentLines(e.Text())
	tabs, spaces, smallest := 0, 0, 0
	var widths [5]int
	for i, line := range lines {
		if info[i].protected || strings.TrimSpace(line) == "" {
			continue
		}
		indent := leadingLuaWhitespace(line)
		if strings.ContainsRune(indent, '\t') {
			tabs++
		} else if len(indent) > 0 {
			spaces++
			if level := info[i].level; level > 0 && len(indent)%level == 0 {
				width := len(indent) / level
				if width < len(widths) {
					widths[width]++
				}
			}
			if smallest == 0 || len(indent) < smallest {
				smallest = len(indent)
			}
		}
	}
	if tabs > spaces {
		return "\t"
	}
	best := 0
	for _, width := range []int{4, 2, 3, 1} {
		if widths[width] > widths[best] {
			best = width
		}
	}
	if best > 0 {
		return strings.Repeat(" ", best)
	}
	if smallest > 0 {
		if smallest%4 == 0 {
			return "    "
		}
		if smallest%2 == 0 {
			return "  "
		}
		if smallest <= 4 {
			return strings.Repeat(" ", smallest)
		}
	}
	return "    "
}

type luaIndentEdit struct {
	start, end int
	text       []rune
}

func mapLuaIndentOffset(offset int, edits []luaIndentEdit) int {
	delta := 0
	for _, edit := range edits {
		if offset < edit.start {
			break
		}
		if offset <= edit.end {
			if offset == edit.end {
				return edit.start + delta + len(edit.text)
			}
			return edit.start + delta + min(offset-edit.start, len(edit.text))
		}
		delta += len(edit.text) - (edit.end - edit.start)
	}
	return offset + delta
}

func (e *Editor) applyLuaIndentEdits(edits []luaIndentEdit) {
	cursor, anchor := mapLuaIndentOffset(e.cursor, edits), mapLuaIndentOffset(e.anchor, edits)
	size := len(e.text)
	for _, edit := range edits {
		size += len(edit.text) - (edit.end - edit.start)
	}
	text, at := make([]rune, 0, size), 0
	for _, edit := range edits {
		text = append(text, e.text[at:edit.start]...)
		text = append(text, edit.text...)
		at = edit.end
	}
	e.text = append(text, e.text[at:]...)
	e.starts = nil
	e.cursor, e.anchor = cursor, anchor
	e.preferredX = -1
}

// FormatIndentation fixes Lua block/delimiter indentation as one undoable edit.
// It leaves spacing inside a line, numeric spelling, and multiline strings and
// comments untouched. Cursor and selection remain attached to the same text.
func (e *Editor) FormatIndentation() bool {
	unit := e.indentationUnit()
	lines := strings.Split(e.Text(), "\n")
	info := luaIndentLines(e.Text())
	var edits []luaIndentEdit
	start := 0
	for i, line := range lines {
		indent := leadingLuaWhitespace(line)
		if !info[i].protected {
			wanted := ""
			if strings.TrimSpace(line) != "" {
				wanted = strings.Repeat(unit, info[i].level)
			}
			if wanted != indent {
				edits = append(edits, luaIndentEdit{start, start + len([]rune(indent)), []rune(wanted)})
			}
		}
		start += len([]rune(line)) + 1
	}
	if len(edits) == 0 {
		return false
	}
	e.pushUndo()
	e.applyLuaIndentEdits(edits)
	return true
}

func startsLuaCloser(tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	switch tokens[0] {
	case "end", "else", "elseif", "until", ")", "]", "}":
		return true
	}
	return false
}

// InsertTyped adds keystrokes, then aligns a completed leading closer. Pasting
// uses Insert instead: pasted text is never silently reformatted.
func (e *Editor) InsertTyped(value string) {
	if value == "" {
		return
	}
	e.Insert(value)
	if strings.ContainsAny(value, "\r\n") {
		return
	}
	line, column := e.LineColumn(e.cursor)
	lineText := e.Line(line)
	prefix := strings.TrimLeft(string([]rune(lineText)[:column]), " \t")
	if prefix == "" {
		return
	}
	if prefix[0] == '}' || prefix[0] == ')' || prefix[0] == ']' {
		if strings.Trim(prefix, "})] \t") != "" {
			return
		}
	} else {
		// Wait for a word boundary: the first three letters of "endless"
		// must not be mistaken for the closing keyword "end".
		keyword := ""
		for _, candidate := range []string{"end", "else", "elseif", "until"} {
			if strings.HasPrefix(prefix, candidate) && len(prefix) > len(candidate) &&
				strings.Trim(prefix[len(candidate):], " \t,;})]") == "" {
				keyword = candidate
				break
			}
		}
		if keyword == "" {
			return
		}
	}
	info := luaIndentLines(string(e.text[:e.cursor]))
	item := info[len(info)-1]
	if item.protected || !startsLuaCloser(item.tokens) {
		return
	}
	indent := leadingLuaWhitespace(lineText)
	wanted := strings.Repeat(e.indentationUnit(), item.level)
	if indent != wanted {
		start := e.cursor - column
		e.applyLuaIndentEdits([]luaIndentEdit{{start, start + len([]rune(indent)), []rune(wanted)}})
	}
}

func (e *Editor) smartNewline() {
	unit := e.indentationUnit()
	start, end, _ := e.Selection()
	text := append([]rune(nil), e.text[:start]...)
	text = append(text, e.text[end:]...)
	prefix := string(text[:start])
	info := luaIndentLines(prefix)
	item := info[len(info)-1]
	if item.state.longEquals >= 0 || item.state.quote != 0 {
		e.Insert("\n")
		return
	}
	lineStart := strings.LastIndex(prefix, "\n") + 1
	line := prefix[lineStart:]
	oldIndent := leadingLuaWhitespace(line)
	base := oldIndent
	if !item.protected && startsLuaCloser(item.tokens) {
		base = strings.Repeat(unit, item.level)
	}
	indent := base
	for level := item.level; level < len(item.after); level++ {
		indent += unit
	}
	value, trailing, cursorBack := "\n"+indent, 0, 0
	// Splitting just before a closer puts it on its own correctly aligned
	// line, leaving the caret on the indented blank line inside the block.
	suffix := string(text[start:])
	if endLine := strings.IndexByte(suffix, '\n'); endLine >= 0 {
		suffix = suffix[:endLine]
	}
	state := item.state
	closers := lexLuaIndentLine(suffix, &state)
	if strings.TrimSpace(line) != "" && startsLuaCloser(closers) && len(item.after) > item.level {
		closingLevel := luaLineIndentLevel(item.after, closers)
		closingIndent := base
		if closingLevel > item.level {
			closingIndent += strings.Repeat(unit, closingLevel-item.level)
		}
		trailing = len([]rune(leadingLuaWhitespace(suffix)))
		value += "\n" + closingIndent
		cursorBack = 1 + len([]rune(closingIndent))
	}
	e.pushUndo()
	e.cursor, e.anchor = start, end+trailing
	e.replaceSelectionNoHistory(value)
	e.cursor -= cursorBack
	e.anchor = e.cursor
	if base != oldIndent {
		at := start - len([]rune(line))
		e.applyLuaIndentEdits([]luaIndentEdit{{at, at + len([]rune(oldIndent)), []rune(base)}})
	}
}

func (e *Editor) smartIndent(outdent bool) {
	start, end, selected := e.Selection()
	unit := e.indentationUnit()
	if !selected && !outdent {
		if unit != "\t" {
			_, column := e.LineColumn(e.cursor)
			unit = strings.Repeat(" ", len(unit)-column%len(unit))
		}
		e.Insert(unit)
		return
	}
	startLine, _ := e.LineColumn(start)
	endLine, endColumn := e.LineColumn(end)
	if selected && endColumn == 0 && endLine > startLine {
		endLine--
	}
	starts := e.lineStarts()
	var edits []luaIndentEdit
	for line := startLine; line <= endLine; line++ {
		at := starts[line]
		if !outdent {
			edits = append(edits, luaIndentEdit{at, at, []rune(unit)})
			continue
		}
		remove := 0
		if at < len(e.text) && e.text[at] == '\t' {
			remove = 1
		} else {
			for remove < len(unit) && at+remove < len(e.text) && e.text[at+remove] == ' ' {
				remove++
			}
		}
		if remove > 0 {
			edits = append(edits, luaIndentEdit{at, at + remove, nil})
		}
	}
	if len(edits) != 0 {
		e.pushUndo()
		e.applyLuaIndentEdits(edits)
	}
}
