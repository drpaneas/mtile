package codeeditor

import "unicode"

// WordRange returns the rune range under offset. Identifiers include Unicode
// letters and digits and underscores. Whitespace is grouped within its line;
// punctuation is selected one character at a time. An insertion offset at the
// end of a line targets the preceding character, without crossing a newline.
func (e *Editor) WordRange(offset int) (start, end int) {
	offset = clamp(offset, 0, len(e.text))
	if offset == len(e.text) || e.text[offset] == '\n' {
		if offset == 0 || e.text[offset-1] == '\n' {
			return offset, offset
		}
		offset--
	}
	start, end = offset, offset+1
	match := isWord
	if !isWord(e.text[offset]) {
		if !unicode.IsSpace(e.text[offset]) {
			return start, end
		}
		match = func(r rune) bool { return r != '\n' && unicode.IsSpace(r) }
	}
	for start > 0 && match(e.text[start-1]) {
		start--
	}
	for end < len(e.text) && match(e.text[end]) {
		end++
	}
	return start, end
}

// LineRange returns the whole line containing offset, including its terminating
// newline when present. The final empty line after a newline has an empty range.
func (e *Editor) LineRange(offset int) (start, end int) {
	start = clamp(offset, 0, len(e.text))
	end = start
	for start > 0 && e.text[start-1] != '\n' {
		start--
	}
	for end < len(e.text) && e.text[end] != '\n' {
		end++
	}
	if end < len(e.text) {
		end++
	}
	return start, end
}

// SelectWord selects the identifier, whitespace, or punctuation under offset.
func (e *Editor) SelectWord(offset int) {
	e.anchor, e.cursor = e.WordRange(offset)
	e.preferredX = -1
}

// SelectLine selects the whole line under offset, including its newline.
func (e *Editor) SelectLine(offset int) {
	e.anchor, e.cursor = e.LineRange(offset)
	e.preferredX = -1
}
