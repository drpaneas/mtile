package luaparser

import (
	"strconv"
	"strings"
)

var keywords = map[string]bool{
	"and": true, "break": true, "do": true, "else": true,
	"elseif": true, "end": true, "false": true, "for": true,
	"function": true, "goto": true, "if": true, "in": true,
	"local": true, "nil": true, "not": true, "or": true,
	"repeat": true, "return": true, "then": true, "true": true,
	"until": true, "while": true,
}

type lexer struct {
	source string
	name   string
	off    int
	line   int
	col    int
}

func newLexer(name, source string) *lexer {
	return &lexer{source: source, name: name, line: 1, col: 1}
}

func (l *lexer) pos() Position {
	return Position{Source: l.name, Line: l.line, Column: l.col}
}

func (l *lexer) peek(n int) byte {
	if l.off+n >= len(l.source) {
		return 0
	}
	return l.source[l.off+n]
}

func (l *lexer) take() byte {
	if l.off >= len(l.source) {
		return 0
	}
	b := l.source[l.off]
	l.off++
	if b == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return b
}

func (l *lexer) eof() bool { return l.off >= len(l.source) }

func (l *lexer) skipSpaceAndComments() *Error {
	for !l.eof() {
		switch l.peek(0) {
		case ' ', '\t', '\f', '\v', '\r', '\n':
			l.take()
			continue
		case '-':
			if l.peek(1) != '-' {
				return nil
			}
			start := l.pos()
			l.take()
			l.take()
			if l.peek(0) == '[' && (l.peek(1) == '[' || l.peek(1) == '=') {
				if err := l.skipLong(start); err != nil {
					return err
				}
				continue
			}
			for !l.eof() && l.peek(0) != '\n' && l.peek(0) != '\r' {
				l.take()
			}
			continue
		default:
			return nil
		}
	}
	return nil
}

// skipLong consumes a Lua [=*[ ... ]=*] comment. The opening '[' is still at
// the current offset when this method is called.
func (l *lexer) skipLong(start Position) *Error {
	level, ok := l.longOpen()
	if !ok {
		return lexError(start, "[", "invalid long comment")
	}
	for !l.eof() {
		if l.peek(0) == ']' {
			if l.longClose(level) {
				return nil
			}
		}
		l.take()
	}
	return lexError(start, "--[[", "unfinished long comment")
}

func (l *lexer) longOpen() (int, bool) {
	if l.peek(0) != '[' {
		return 0, false
	}
	l.take()
	level := 0
	for l.peek(0) == '=' {
		level++
		l.take()
	}
	if l.peek(0) != '[' {
		return 0, false
	}
	l.take()
	return level, true
}

func (l *lexer) longClose(level int) bool {
	if l.peek(0) != ']' {
		return false
	}
	i := 1
	for j := 0; j < level; j++ {
		if l.peek(i) != '=' {
			return false
		}
		i++
	}
	if l.peek(i) != ']' {
		return false
	}
	for j := 0; j <= level+1; j++ {
		l.take()
	}
	return true
}

func (l *lexer) scanIdentifier() Token {
	start := l.pos()
	begin := l.off
	for isIdentifierPart(l.peek(0), l.off == begin) {
		l.take()
	}
	text := l.source[begin:l.off]
	kind := TokenIdentifier
	if keywords[text] {
		kind = TokenKeyword
	}
	return Token{Kind: kind, Text: text, Pos: start}
}

func isIdentifierStart(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func isIdentifierPart(b byte, first bool) bool {
	if isIdentifierStart(b) {
		return true
	}
	return !first && b >= '0' && b <= '9'
}

func isBinaryDigit(b byte) bool { return b == '0' || b == '1' }

func (l *lexer) scanBasedFraction(isDigitFn func(byte) bool) int {
	if l.peek(0) != '.' || l.peek(1) == '.' || !isDigitFn(l.peek(1)) {
		return 0
	}
	l.take()
	digits := 0
	for isDigitFn(l.peek(0)) {
		l.take()
		digits++
	}
	return digits
}

func (l *lexer) scanNumber() (Token, *Error) {
	start := l.pos()
	begin := l.off
	if l.peek(0) == '0' && (l.peek(1) == 'b' || l.peek(1) == 'B') {
		l.take()
		l.take()
		digits := 0
		for isBinaryDigit(l.peek(0)) {
			l.take()
			digits++
		}
		digits += l.scanBasedFraction(isBinaryDigit)
		if digits == 0 {
			return Token{}, lexError(start, l.source[begin:l.off], "binary literal requires at least one digit")
		}
		if b := l.peek(0); b >= '0' && b <= '9' {
			return Token{}, lexError(start, l.source[begin:l.off+1], "invalid binary literal")
		}
		return Token{Kind: TokenNumber, Text: l.source[begin:l.off], Pos: start}, nil
	}
	if l.peek(0) == '0' && (l.peek(1) == 'x' || l.peek(1) == 'X') {
		l.take()
		l.take()
		digits := 0
		for isHexDigit(l.peek(0)) {
			l.take()
			digits++
		}
		digits += l.scanBasedFraction(isHexDigit)
		if digits == 0 {
			return Token{}, lexError(start, l.source[begin:l.off], "hexadecimal literal requires at least one digit")
		}
		return Token{Kind: TokenNumber, Text: l.source[begin:l.off], Pos: start}, nil
	}
	if l.peek(0) == '.' {
		l.take()
		if !isDigit(l.peek(0)) {
			return Token{}, lexError(start, l.source[begin:l.off], "number requires at least one digit")
		}
		for isDigit(l.peek(0)) {
			l.take()
		}
	} else {
		for isDigit(l.peek(0)) {
			l.take()
		}
		// Lua 5.1/5.2 accept a trailing decimal point (`4.`). Do not treat
		// `..` as a fraction; concat must stay a separate token.
		if l.peek(0) == '.' && l.peek(1) != '.' {
			l.take()
			for isDigit(l.peek(0)) {
				l.take()
			}
		}
	}
	if l.peek(0) == 'e' || l.peek(0) == 'E' {
		l.take()
		if l.peek(0) == '+' || l.peek(0) == '-' {
			l.take()
		}
		if !isDigit(l.peek(0)) {
			return Token{}, lexError(start, l.source[begin:l.off], "exponent requires at least one digit")
		}
		for isDigit(l.peek(0)) {
			l.take()
		}
	}
	return Token{Kind: TokenNumber, Text: l.source[begin:l.off], Pos: start}, nil
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isHexDigit(b byte) bool {
	return isDigit(b) || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}

func (l *lexer) scanString() (Token, *Error) {
	start := l.pos()
	quote := l.take()
	var value strings.Builder
	for !l.eof() {
		b := l.take()
		if b == quote {
			return Token{Kind: TokenString, Text: value.String(), Value: value.String(), Pos: start}, nil
		}
		if b == '\n' || b == '\r' {
			return Token{}, lexError(start, value.String(), "unfinished string")
		}
		if b != '\\' {
			value.WriteByte(b)
			continue
		}
		if l.eof() {
			return Token{}, lexError(start, value.String(), "unfinished escape sequence")
		}
		escape := l.take()
		switch escape {
		case 'a':
			value.WriteByte('\a')
		case 'b':
			value.WriteByte('\b')
		case 'f':
			value.WriteByte('\f')
		case 'n':
			value.WriteByte('\n')
		case 'r':
			value.WriteByte('\r')
		case 't':
			value.WriteByte('\t')
		case 'v':
			value.WriteByte('\v')
		// Print() control-code escapes. These are not standard Lua escapes:
		// the punctuation spellings decode to single-byte print() control
		// codes 1-6, so they must not fall through to the literal-character
		// default below.
		case '*':
			value.WriteByte(1) // repeat next character
		case '#':
			value.WriteByte(2) // draw solid background
		case '-':
			value.WriteByte(3) // move cursor horizontally
		case '|':
			value.WriteByte(4) // move cursor vertically
		case '+':
			value.WriteByte(5) // move cursor
		case '^':
			value.WriteByte(6) // special command
		case '\\', '\'', '"':
			value.WriteByte(escape)
		case '\n':
			value.WriteByte('\n')
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			digits := []byte{escape}
			for len(digits) < 3 && l.peek(0) >= '0' && l.peek(0) <= '9' {
				digits = append(digits, l.take())
			}
			v, err := strconv.ParseUint(string(digits), 10, 8)
			if err != nil {
				return Token{}, lexError(start, string(digits), "decimal escape is outside byte range")
			}
			value.WriteByte(byte(v))
		default:
			// Unknown escapes keep the escaped character (the backslash is dropped).
			value.WriteByte(escape)
		}
	}
	return Token{}, lexError(start, value.String(), "unfinished string")
}

func (l *lexer) scanLongString() (Token, *Error) {
	start := l.pos()
	level, ok := l.longOpen()
	if !ok {
		return Token{}, lexError(start, "[", "invalid long string")
	}
	// Lua ignores a newline immediately after the opening long bracket.
	if l.peek(0) == '\n' {
		l.take()
	} else if l.peek(0) == '\r' {
		l.take()
		if l.peek(0) == '\n' {
			l.take()
		}
	}
	var value strings.Builder
	for !l.eof() {
		if l.peek(0) == ']' && l.longClose(level) {
			return Token{Kind: TokenString, Text: value.String(), Value: value.String(), Pos: start}, nil
		}
		value.WriteByte(l.take())
	}
	return Token{}, lexError(start, value.String(), "unfinished long string")
}

// multiSymbols must stay sorted longest-first: scanSymbol matches the first
// entry that is a prefix of the remaining source, so a shorter entry listed
// before one of its own extensions (e.g. ">>=" before ">>>=") would shadow
// it and mis-tokenize. Longest-first makes this correct by construction
// instead of relying on the specific set of operators not colliding.
var multiSymbols = []string{
	">>>=", "<<>=", ">><=",
	"...", "<<=", ">>=", "^^=", "..=", ">>>", "<<>", ">><",
	"+=", "-=", "*=", "/=", "%=", "\\=", "&=", "|=", "^=",
	"<<", ">>", "^^", "..", "!=", "~=", "==", "<=", ">=", "::",
}

func (l *lexer) scanSymbol() (Token, *Error) {
	start := l.pos()
	for _, symbol := range multiSymbols {
		if strings.HasPrefix(l.source[l.off:], symbol) {
			for range symbol {
				l.take()
			}
			return Token{Kind: TokenSymbol, Text: symbol, Pos: start}, nil
		}
	}
	b := l.peek(0)
	if strings.ContainsRune("{}()[];,.:?+-*/%^&|~<>=#\\", rune(b)) {
		l.take()
		return Token{Kind: TokenSymbol, Text: string(b), Pos: start}, nil
	}
	if b == '!' {
		l.take()
		return Token{}, lexError(start, "!", "expected !=; use `not` for logical negation")
	}
	l.take()
	return Token{}, lexError(start, string(b), "unexpected character")
}

func (l *lexer) next() (Token, *Error) {
	if err := l.skipSpaceAndComments(); err != nil {
		return Token{}, err
	}
	if l.eof() {
		return Token{Kind: TokenEOF, Pos: l.pos()}, nil
	}
	b := l.peek(0)
	if isIdentifierStart(b) {
		return l.scanIdentifier(), nil
	}
	if isDigit(b) || b == '.' && isDigit(l.peek(1)) {
		return l.scanNumber()
	}
	if b == '\'' || b == '"' {
		return l.scanString()
	}
	if b == '[' && (l.peek(1) == '[' || l.peek(1) == '=') {
		return l.scanLongString()
	}
	return l.scanSymbol()
}

// Lex returns all tokens, including a final TokenEOF.
func Lex(sourceName, source string) ([]Token, error) {
	l := newLexer(sourceName, source)
	var tokens []Token
	for {
		tok, err := l.next()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.Kind == TokenEOF {
			return tokens, nil
		}
	}
}
