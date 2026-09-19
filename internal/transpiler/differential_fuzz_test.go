package transpiler

import (
	"fmt"
	"math/rand"
	"slices"
	"strconv"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
)

// This deliberately small oracle models nil, booleans, and signed 16:16
// arithmetic without importing the runtime or the parser's precedence table.
// It is not a general Lua interpreter. The generated tree is the specification;
// evaluating the parsed tree checks that source preserves that specification.
type differentialValue struct {
	kind string
	raw  int32
}

func differentialNumber(raw int32) differentialValue { return differentialValue{"number", raw} }
func differentialBool(b bool) differentialValue {
	if b {
		return differentialValue{"bool", 1}
	}
	return differentialValue{"bool", 0}
}
func (v differentialValue) truth() bool    { return v.kind != "nil" && (v.kind != "bool" || v.raw != 0) }
func (v differentialValue) String() string { return fmt.Sprintf("%s:%d", v.kind, v.raw) }

type differentialExpr struct {
	op          string
	value       differentialValue
	left, right *differentialExpr
	tag         int
}

func differentialLiteral(v differentialValue) *differentialExpr {
	return &differentialExpr{value: v}
}
func differentialBinary(op string, a, b *differentialExpr) *differentialExpr {
	return &differentialExpr{op: op, left: a, right: b}
}
func differentialMark(tag int, e *differentialExpr) *differentialExpr {
	return &differentialExpr{op: "mark", left: e, tag: tag}
}

func differentialPrecedence(op string) int {
	switch op {
	case "or":
		return 1
	case "and":
		return 2
	case "==", "!=", "<", "<=":
		return 3
	case "+", "-":
		return 4
	case "*":
		return 5
	case "not", "neg":
		return 6
	default:
		return 7
	}
}

func (e *differentialExpr) source(parent int) string {
	precedence := differentialPrecedence(e.op)
	var s string
	switch e.op {
	case "":
		switch e.value.kind {
		case "nil":
			s = "nil"
		case "bool":
			s = strconv.FormatBool(e.value.raw != 0)
		default:
			s = strconv.FormatFloat(float64(e.value.raw)/65536, 'f', -1, 64)
			if e.value.raw < 0 {
				s = "(" + s + ")"
			}
		}
	case "mark":
		s = fmt.Sprintf("mark(%d, %s)", e.tag, e.left.source(0))
	case "not":
		s = "not " + e.left.source(precedence)
	case "neg":
		s = "- " + e.left.source(precedence)
	default:
		s = e.left.source(precedence) + " " + e.op + " " + e.right.source(precedence+1)
	}
	if precedence < parent {
		return "(" + s + ")"
	}
	return s
}

func differentialApply(op string, a, b differentialValue) differentialValue {
	switch op {
	case "+":
		return differentialNumber(int32(int64(a.raw) + int64(b.raw)))
	case "-":
		return differentialNumber(int32(int64(a.raw) - int64(b.raw)))
	case "*":
		return differentialNumber(int32((int64(a.raw) * int64(b.raw)) >> 16))
	case "==":
		return differentialBool(a == b)
	case "!=":
		return differentialBool(a != b)
	case "<":
		return differentialBool(a.raw < b.raw)
	case "<=":
		return differentialBool(a.raw <= b.raw)
	default:
		panic("unsupported oracle operator: " + op)
	}
}

func (e *differentialExpr) eval(trace *[]int) differentialValue {
	if e.op == "" {
		return e.value
	}
	a := e.left.eval(trace)
	switch e.op {
	case "mark":
		*trace = append(*trace, e.tag)
		return a
	case "not":
		return differentialBool(!a.truth())
	case "neg":
		return differentialNumber(int32(-int64(a.raw)))
	case "and":
		if !a.truth() {
			return a
		}
		return e.right.eval(trace)
	case "or":
		if a.truth() {
			return a
		}
		return e.right.eval(trace)
	}
	return differentialApply(e.op, a, e.right.eval(trace))
}

// At most 64 input bytes, depth three, and fifteen tree nodes. Exhausted input
// produces zeroes, so shrinking always produces a valid, deterministic case.
func differentialGenerate(data []byte) *differentialExpr {
	if len(data) > 64 {
		data = data[:64]
	}
	position, tag := 0, 0
	next := func() byte {
		if position >= len(data) {
			return 0
		}
		b := data[position]
		position++
		return b
	}
	var generate func(int, bool) *differentialExpr
	generate = func(depth int, numberOnly bool) *differentialExpr {
		choice := next()
		if depth == 0 || choice%8 == 0 {
			if !numberOnly {
				switch next() % 5 {
				case 0:
					return differentialLiteral(differentialValue{kind: "nil"})
				case 1, 2:
					return differentialLiteral(differentialBool(choice%2 == 0))
				}
			}
			raw := []int32{0, 16384, -16384, 65536, -65536, 131072, 2147418112, -2147483648}[int(next())%8]
			return differentialLiteral(differentialNumber(raw))
		}
		switch choice % 8 {
		case 1:
			tag++
			id := tag
			return differentialMark(id, generate(depth-1, numberOnly))
		case 2:
			op := "neg"
			if !numberOnly {
				op = "not"
			}
			return &differentialExpr{op: op, left: generate(depth-1, numberOnly)}
		case 3, 4:
			if !numberOnly {
				op := "and"
				if choice%8 == 4 {
					op = "or"
				}
				return differentialBinary(op, generate(depth-1, false), generate(depth-1, false))
			}
		case 5:
			if !numberOnly {
				op := []string{"==", "!=", "<", "<="}[next()%4]
				return differentialBinary(op, generate(depth-1, true), generate(depth-1, true))
			}
		}
		op := []string{"+", "-", "*"}[next()%3]
		return differentialBinary(op, generate(depth-1, true), generate(depth-1, true))
	}
	return generate(3, false)
}

func differentialCorpus() []*differentialExpr {
	n := func(v int32) *differentialExpr { return differentialLiteral(differentialNumber(v * 65536)) }
	b := func(v bool) *differentialExpr { return differentialLiteral(differentialBool(v)) }
	m := func(tag int, v *differentialExpr) *differentialExpr { return differentialMark(tag, v) }
	corpus := []*differentialExpr{
		differentialBinary("+", n(1), differentialBinary("*", n(2), n(3))),
		differentialBinary("-", n(9), differentialBinary("-", n(4), n(1))),
		differentialBinary("and", b(false), m(1, n(9))),
		differentialBinary("or", b(true), m(1, n(9))),
		differentialBinary("or", n(0), m(1, n(9))),
		differentialBinary("and", n(0), m(1, n(9))),
		differentialBinary("or", differentialLiteral(differentialValue{kind: "nil"}), m(1, n(9))),
		differentialBinary("+", m(1, n(1)), m(2, n(2))),
		differentialBinary("<", m(1, n(1)), m(2, n(2))),
		differentialBinary("and", m(1, b(true)), differentialBinary("or", m(2, b(false)), m(3, n(4)))),
		differentialBinary("+", n(32767), n(1)),
		differentialBinary("*", differentialLiteral(differentialNumber(-16384)), differentialLiteral(differentialNumber(16384))),
		differentialLiteral(differentialNumber(-2147483648)),
		{op: "neg", left: differentialLiteral(differentialNumber(-2147483648))},
	}
	rng := rand.New(rand.NewSource(0x6d74696c65))
	for range 128 {
		data := make([]byte, 64)
		_, _ = rng.Read(data)
		corpus = append(corpus, differentialGenerate(data))
	}
	return corpus
}

// Parsing is the system under test here; this AST evaluator does not call any
// transpiler, runtime arithmetic, or runtime truthiness helpers.
func differentialEvalParsed(e luaparser.Expr, trace *[]int) differentialValue {
	switch x := e.(type) {
	case *luaparser.NilExpr:
		return differentialValue{kind: "nil"}
	case *luaparser.BoolExpr:
		return differentialBool(x.Value)
	case *luaparser.NumberExpr:
		return differentialNumber(int32(int64(x.Value * 65536)))
	case *luaparser.ParenExpr:
		return differentialEvalParsed(x.Expr, trace)
	case *luaparser.UnaryExpr:
		a := differentialEvalParsed(x.Expr, trace)
		if x.Op == "not" {
			return differentialBool(!a.truth())
		}
		if x.Op == "-" {
			return differentialNumber(int32(-int64(a.raw)))
		}
	case *luaparser.CallExpr:
		tag := differentialEvalParsed(x.Args[0], trace)
		value := differentialEvalParsed(x.Args[1], trace)
		*trace = append(*trace, int(tag.raw/65536))
		return value
	case *luaparser.BinaryExpr:
		a := differentialEvalParsed(x.Left, trace)
		if x.Op == "and" {
			if !a.truth() {
				return a
			}
			return differentialEvalParsed(x.Right, trace)
		}
		if x.Op == "or" {
			if a.truth() {
				return a
			}
			return differentialEvalParsed(x.Right, trace)
		}
		return differentialApply(string(x.Op), a, differentialEvalParsed(x.Right, trace))
	}
	panic(fmt.Sprintf("unsupported oracle AST: %T", e))
}

func differentialCheckParser(t testing.TB, e *differentialExpr) {
	t.Helper()
	source := e.source(0)
	chunk, err := luaparser.ParseString("differential.lua", "local result = "+source)
	if err != nil {
		t.Fatalf("parse %q: %v", source, err)
	}
	var wantTrace, gotTrace []int
	want := e.eval(&wantTrace)
	got := differentialEvalParsed(chunk.Body[0].(*luaparser.LocalAssignStmt).Values[0], &gotTrace)
	if got != want || !slices.Equal(gotTrace, wantTrace) {
		t.Fatalf("semantic mismatch: %s: got %s trace %v, want %s trace %v", source, got, gotTrace, want, wantTrace)
	}
}

func TestDifferentialParserCorpus(t *testing.T) {
	for _, e := range differentialCorpus() {
		differentialCheckParser(t, e)
	}
}

func FuzzDifferentialParser(f *testing.F) {
	f.Add([]byte{})
	rng := rand.New(rand.NewSource(0x6d74696c65))
	for range 32 {
		data := make([]byte, 64)
		_, _ = rng.Read(data)
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		differentialCheckParser(t, differentialGenerate(data))
	})
}
