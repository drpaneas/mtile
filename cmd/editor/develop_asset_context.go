package main

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/drpaneas/mtile/internal/luaparser"
)

type developAssetContext struct {
	Kind         string // anim or pspr
	Start, End   int    // rune offsets for the content, excluding the quotes
	Prefix, Name string // decoded Lua text before the caret and in the full argument
	Quote        rune
	Closed       bool
}

// Lexing through the caret leaves a string open only when the caret is inside
// it. Use the lexer's position to recover its opening quote without guessing
// whether it is escaped or belongs to a comment or long string.
func developAssetContextAt(source string, caret int) (developAssetContext, bool) {
	runes := []rune(source)
	if caret <= 0 || caret > len(runes) {
		return developAssetContext{}, false
	}
	beforeCaret := string(runes[:caret])
	_, err := luaparser.Lex("", beforeCaret)
	var lexErr *luaparser.Error
	if !errors.As(err, &lexErr) {
		return developAssetContext{}, false
	}
	openingByte := developAssetByteOffset(beforeCaret, lexErr.Pos)
	opening := utf8.RuneCountInString(beforeCaret[:openingByte])
	start := opening + 1
	quote := runes[opening]
	if quote != '\'' && quote != '"' {
		return developAssetContext{}, false
	}
	end, closed := developAssetStringEnd(runes, start, quote)
	if caret > end {
		return developAssetContext{}, false
	}
	kind, groups := developAssetCallPrefix(beforeCaret[:openingByte])
	if kind == "" || (closed && !developAssetLiteralTail(string(runes[end+1:]), groups)) {
		return developAssetContext{}, false
	}
	prefix := decodeDevelopAssetName(string(runes[start:caret]), quote, false)
	name := decodeDevelopAssetName(string(runes[start:end]), quote, closed)
	return developAssetContext{Kind: kind, Start: start, End: end, Prefix: prefix, Name: name, Quote: quote, Closed: closed}, true
}

func developAssetByteOffset(source string, pos luaparser.Position) int {
	start := 0
	for line := 1; line < pos.Line; line++ {
		start += strings.IndexByte(source[start:], '\n') + 1
	}
	return start + pos.Column - 1
}

func developAssetStringEnd(source []rune, start int, quote rune) (end int, closed bool) {
	for at := start; at < len(source); at++ {
		switch source[at] {
		case quote:
			return at, true
		case '\n', '\r':
			return at, false
		case '\\':
			at++
		}
	}
	return len(source), false
}

func developAssetCallPrefix(source string) (kind string, groups int) {
	tokens, err := luaparser.Lex("", source)
	if err != nil {
		return "", 0
	}
	// The last token is EOF. A literal may be wrapped in additional grouping
	// parentheses because the transpiler unwraps them before resolving names.
	at := len(tokens) - 2
	for at >= 0 && tokens[at].Kind == luaparser.TokenSymbol && tokens[at].Text == "(" {
		groups++
		at--
	}
	if groups == 0 || at < 0 || tokens[at].Kind != luaparser.TokenIdentifier {
		return "", 0
	}
	if at > 0 {
		before := tokens[at-1]
		if (before.Kind == luaparser.TokenSymbol && (before.Text == "." || before.Text == ":")) || (before.Kind == luaparser.TokenKeyword && before.Text == "function") {
			return "", 0
		}
	}
	switch tokens[at].Text {
	case "anim", "pspr":
		return tokens[at].Text, groups
	default:
		return "", 0
	}
}

func developAssetLiteralTail(source string, groups int) bool {
	tokens, err := luaparser.Lex("", source)
	var lexErr *luaparser.Error
	if errors.As(err, &lexErr) {
		// A later broken token should not hide completion for this argument.
		tokens, err = luaparser.Lex("", source[:developAssetByteOffset(source, lexErr.Pos)])
	}
	if err != nil {
		return false
	}
	for _, token := range tokens {
		if token.Kind == luaparser.TokenEOF {
			return true // the call can still be incomplete
		}
		if token.Kind != luaparser.TokenSymbol {
			return false
		}
		if groups == 1 {
			return token.Text == "," || token.Text == ")"
		}
		if token.Text != ")" {
			return false
		}
		groups--
	}
	return false
}

func decodeDevelopAssetName(content string, quote rune, closed bool) string {
	if !strings.ContainsRune(content, '\\') {
		return content
	}
	delimiter := string(quote)
	tokens, err := luaparser.Lex("", delimiter+content+delimiter)
	if err != nil && !closed && strings.HasSuffix(content, "\\") {
		// A caret immediately after the escape opener has no escaped rune yet.
		tokens, err = luaparser.Lex("", delimiter+strings.TrimSuffix(content, "\\")+delimiter)
	}
	if err != nil {
		return ""
	}
	return tokens[0].Value
}
