package luaparser

import "fmt"

// TokenKind identifies the lexical class of a token. Keywords and operators
// retain their spelling in Token.Text so the parser can give useful errors.
type TokenKind uint8

const (
	TokenEOF TokenKind = iota
	TokenIdentifier
	TokenNumber
	TokenString
	TokenKeyword
	TokenSymbol
)

// Position is one-based for both line and column. Column is the byte column,
// which matches the source offsets used by the lexer.
type Position struct {
	Source string
	Line   int
	Column int
}

func (p Position) String() string {
	if p.Source == "" {
		return fmt.Sprintf("line %d, column %d", p.Line, p.Column)
	}
	return fmt.Sprintf("%s:%d:%d", p.Source, p.Line, p.Column)
}

// Token is the lossless-enough lexical representation consumed by Parser.
// Text is the source spelling for identifiers, numbers, strings, keywords,
// and operators. String tokens additionally carry their decoded Value.
type Token struct {
	Kind  TokenKind
	Text  string
	Value string
	Pos   Position
}

func (t Token) String() string {
	if t.Kind == TokenEOF {
		return "<eof>"
	}
	if t.Kind == TokenString {
		return fmt.Sprintf("%q", t.Value)
	}
	return t.Text
}

// Error is returned for lexical and syntactic failures. Token contains the
// nearby source spelling when one exists.
type Error struct {
	Pos     Position
	Token   string
	Message string
}

func (e *Error) Error() string {
	if e.Token == "" {
		return fmt.Sprintf("%s: %s", e.Pos, e.Message)
	}
	return fmt.Sprintf("%s near %q: %s", e.Pos, e.Token, e.Message)
}

func lexError(pos Position, token, message string) *Error {
	return &Error{Pos: pos, Token: token, Message: message}
}
