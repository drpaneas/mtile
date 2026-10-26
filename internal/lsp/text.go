package lsp

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"

	"github.com/drpaneas/mtile/internal/luaparser"
)

func filePath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Host != "" && u.Host != "localhost" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("expected a local file URI")
	}
	path := u.Path
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	path = filepath.FromSlash(path)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("file URI must be absolute")
	}
	return filepath.Clean(path), nil
}

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func relativePath(root, path string) (string, bool) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func lineStart(text string, line int) (int, error) {
	if line < 0 {
		return 0, fmt.Errorf("negative line")
	}
	start := 0
	for i := 0; i < line; i++ {
		next := strings.IndexByte(text[start:], '\n')
		if next < 0 {
			return 0, fmt.Errorf("line outside document")
		}
		start += next + 1
	}
	return start, nil
}

func lineText(text string, start int) string {
	line, _, _ := strings.Cut(text[start:], "\n")
	return strings.TrimSuffix(line, "\r")
}

// LSP defaults to UTF-16 code units; the compiler uses byte columns.
func offsetAt(text string, at position) (int, error) {
	start, err := lineStart(text, at.Line)
	if err != nil || at.Character < 0 {
		return 0, fmt.Errorf("invalid document position")
	}
	units := 0
	line := lineText(text, start)
	for index, r := range line {
		if units == at.Character {
			return start + index, nil
		}
		units += utf16.RuneLen(r)
		if units > at.Character {
			return 0, fmt.Errorf("position splits a UTF-16 surrogate pair")
		}
	}
	return start + len(line), nil
}

func positionAt(text string, offset int) position {
	offset = min(max(0, offset), len(text))
	start := strings.LastIndexByte(text[:offset], '\n') + 1
	result := position{Line: strings.Count(text[:offset], "\n")}
	for _, r := range strings.TrimSuffix(text[start:offset], "\r") {
		result.Character += utf16.RuneLen(r)
	}
	return result
}

func compilerPosition(text string, line, column int) position {
	start, err := lineStart(text, max(0, line-1))
	if err != nil {
		return positionAt(text, len(text))
	}
	return positionAt(text, start+min(max(0, column-1), len(lineText(text, start))))
}

func identifierByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func wordAt(text string, offset int) (string, textRange) {
	start, end := offset, offset
	for start > 0 && identifierByte(text[start-1]) {
		start--
	}
	for end < len(text) && identifierByte(text[end]) {
		end++
	}
	return text[start:end], textRange{Start: positionAt(text, start), End: positionAt(text, end)}
}

// Lex only through the identifier, so a later unfinished edit cannot disable
// help. Comments, strings, and table-member accesses do not name builtins.
func identifierInCode(text string, start, end int) bool {
	prefix := text[:end]
	if start == end {
		prefix += "_"
	}
	tokens, err := luaparser.Lex("buffer", prefix)
	if err != nil || len(tokens) < 2 {
		return false
	}
	last := tokens[len(tokens)-2]
	line := strings.Count(text[:start], "\n") + 1
	column := start - strings.LastIndexByte(text[:start], '\n')
	if last.Kind != luaparser.TokenIdentifier || last.Pos.Line != line || last.Pos.Column != column {
		return false
	}
	if len(tokens) > 2 {
		before := tokens[len(tokens)-3].Text
		if before == "." || before == ":" {
			return false
		}
	}
	return true
}
