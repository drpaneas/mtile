package luaparser

import (
	"strings"
	"testing"
)

func TestLexMtileExtensions(t *testing.T) {
	source := `a = 0b1010; b != c; d += 1; e \= 2; f ^^= 3; g >>>= 1; h <<> i; j >>< k`
	tokens, err := Lex("extensions.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "=", "0b1010", ";", "b", "!=", "c", ";", "d", "+=", "1", ";", "e", "\\=", "2", ";", "f", "^^=", "3", ";", "g", ">>>=", "1", ";", "h", "<<>", "i", ";", "j", ">><", "k"}
	var got []string
	for _, token := range tokens {
		if token.Kind != TokenEOF {
			got = append(got, token.Text)
		}
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
}

func TestParseMtileExtensions(t *testing.T) {
	source := `
function _update()
  if score != 0 and (mask & 0b1010) == 2 then
    score += 1
    mask |= 1
    mask ^^= 3
    mask <<= 1
    mask >>= 1
    mask >>>= 1
    mask <<>= 1
    mask >><= 1
    score \= 2
  end
end
?score
`
	chunk, err := ParseString("extensions.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 2 {
		t.Fatalf("chunk statements = %d, want function plus print", len(chunk.Body))
	}
	function, ok := chunk.Body[0].(*FunctionDeclStmt)
	if !ok || len(function.Body.Body) != 1 {
		t.Fatalf("first statement = %#v, want function containing if", chunk.Body[0])
	}
	printCall, ok := chunk.Body[1].(*CallStmt)
	if !ok {
		t.Fatalf("second statement = %T, want *CallStmt print", chunk.Body[1])
	}
	callee, ok := printCall.Call.Callee.(*VarExpr)
	if !ok || callee.Name != "print" || len(printCall.Call.Args) != 1 {
		t.Fatalf("print shorthand = %#v, want print(score)", printCall.Call)
	}
	ifBranch, ok := function.Body.Body[0].(*IfStmt)
	if !ok || len(ifBranch.Then.Body) != 9 {
		t.Fatalf("if body = %#v, want nine compound assignments", function.Body.Body[0])
	}
	comparison, ok := ifBranch.Condition.(*BinaryExpr)
	if !ok || comparison.Op != OpAnd {
		t.Fatalf("condition = %#v, want logical and", ifBranch.Condition)
	}
}

func TestParseSingleLineIf(t *testing.T) {
	chunk, err := ParseString("single-line-if.lua", "if (ready != 0) score += 1\nprint(score)")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 2 {
		t.Fatalf("statements = %d, want if plus print", len(chunk.Body))
	}
	branch, ok := chunk.Body[0].(*IfStmt)
	if !ok || len(branch.Then.Body) != 1 {
		t.Fatalf("first statement = %#v, want single-line if", chunk.Body[0])
	}
	if _, ok := branch.Then.Body[0].(*AssignStmt); !ok {
		t.Fatalf("single-line body = %T, want lowered assignment", branch.Then.Body[0])
	}
}

func TestCompoundIndexedAssignmentLowersWithTemps(t *testing.T) {
	chunk, err := ParseString("compound.lua", `items[next_key()] += amount()`)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 3 {
		t.Fatalf("lowered statements = %d, want 3", len(chunk.Body))
	}
	baseLocal, ok := chunk.Body[0].(*LocalAssignStmt)
	if !ok || !baseLocal.Synthetic {
		t.Fatalf("first lowered statement = %#v, want synthetic base local", chunk.Body[0])
	}
	keyLocal, ok := chunk.Body[1].(*LocalAssignStmt)
	if !ok || !keyLocal.Synthetic {
		t.Fatalf("second lowered statement = %#v, want synthetic key local", chunk.Body[1])
	}
	assign, ok := chunk.Body[2].(*AssignStmt)
	if !ok || !assign.Synthetic {
		t.Fatalf("third lowered statement = %#v, want synthetic assignment", chunk.Body[2])
	}
	target, ok := assign.Targets[0].(*IndexExpr)
	if !ok {
		t.Fatalf("target = %#v, want index expression", assign.Targets[0])
	}
	base, baseOK := target.Base.(*VarExpr)
	key, keyOK := target.Key.(*VarExpr)
	if !baseOK || !keyOK || base.Name != baseLocal.Names[0] || key.Name != keyLocal.Names[0] {
		t.Fatalf("target = %#v, want generated base/key locals", target)
	}
	rhs, ok := assign.Values[0].(*BinaryExpr)
	if !ok || rhs.Op != OpAdd {
		t.Fatalf("rhs = %#v, want additive read/modify/write", assign.Values[0])
	}
	if assign.Targets[0] == rhs.Left {
		t.Fatal("compound assignment aliased the l-value node as the read")
	}
	if baseValue, ok := baseLocal.Values[0].(*VarExpr); !ok || baseValue.Name != "items" {
		t.Fatalf("base local value = %#v, want original table variable", baseLocal.Values[0])
	}
}

func TestParseLuaControlFlowAndTables(t *testing.T) {
	source := `
local total, step = 0, 1
local data = {1, 2, name = "hero", [4] = true}
for i = 1, 4, step do total += data[i] end
for key, value in pairs(data) do print(key, value) end
while total < 10 do total += 1 end
repeat total -= 1 until total == 0
do local x = #data end
return total
`
	chunk, err := ParseString("control.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 8 {
		t.Fatalf("statements = %d, want 8", len(chunk.Body))
	}
	if _, ok := chunk.Body[1].(*LocalAssignStmt); !ok {
		t.Fatalf("table declaration = %T, want local assignment", chunk.Body[1])
	}
	if _, ok := chunk.Body[2].(*NumericForStmt); !ok {
		t.Fatalf("numeric loop = %T, want numeric for", chunk.Body[2])
	}
	if _, ok := chunk.Body[3].(*GenericForStmt); !ok {
		t.Fatalf("generic loop = %T, want generic for", chunk.Body[3])
	}
}

// TestTextControlCodeEscapes guards against a regression where these six
// punctuation escapes fell through to the generic "unknown escape preserves
// the literal character" default, silently writing the punctuation byte
// (e.g. '*' = 0x2A) instead of print() control-code bytes 1-6. That default
// is correct for undefined escapes (see TestUnknownEscapePreservesCharacter),
// but these six are documented control codes and must not fall through.
func TestTextControlCodeEscapes(t *testing.T) {
	tests := []struct {
		escape string
		code   byte
		name   string
	}{
		{`\*`, 1, "repeat next character"},
		{`\#`, 2, "draw solid background"},
		{`\-`, 3, "move cursor horizontally"},
		{`\|`, 4, "move cursor vertically"},
		{`\+`, 5, "move cursor"},
		{`\^`, 6, "special command"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tokens, err := Lex("textctrl.lua", `"`+test.escape+`"`)
			if err != nil {
				t.Fatalf("lex %q: %v", test.escape, err)
			}
			if len(tokens) < 1 || tokens[0].Kind != TokenString {
				t.Fatalf("tokens = %#v, want a single string token", tokens)
			}
			if got := tokens[0].Value; got != string(test.code) {
				t.Fatalf("%s decoded to %q (0x%02x), want byte 0x%02x", test.escape, got, got, test.code)
			}
		})
	}
}

// TestUnknownEscapePreservesCharacter documents the deliberate fallback for
// punctuation with no assigned escape meaning: the backslash is dropped
// and the literal character is kept, matching the existing lexer comment.
func TestUnknownEscapePreservesCharacter(t *testing.T) {
	tokens, err := Lex("unknown-escape.lua", `"\$"`)
	if err != nil {
		t.Fatal(err)
	}
	if got := tokens[0].Value; got != "$" {
		t.Fatalf("value = %q, want literal $", got)
	}
}

func TestLongStringsAndComments(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"level0", "x = [[hello]]", "hello"},
		{"level0-multiline", "x = [[a\nb]]", "a\nb"},
		{"level0-not-closed-by-inner-level1", "x = [[a]=]b]]", "a]=]b"},
		{"level1-not-closed-by-plain-brackets", "x = [=[a]]b]=]", "a]]b"},
		{"level2", "x = [==[a]=]b]==]", "a]=]b"},
		{"empty", "x = [[]]", ""},
		{"skip-first-newline", "x = [[\nhello]]", "hello"},
		{"skip-crlf", "x = [[\r\nhello]]", "hello"},
		{"skip-cr-only", "x = [[\rhello]]", "hello"},
		{"keep-second-newline", "x = [[\n\nhello]]", "\nhello"},
		{"level2-payload-is-close-of-level1", "x = [==[]=]==]", "]="},
		{"nested-junk-inside-level2", "x = [==[[===[[=[]]=][====[]]===]===]==]", "[===[[=[]]=][====[]]===]==="},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunk, err := ParseString(test.name+".lua", test.source)
			if err != nil {
				t.Fatal(err)
			}
			assign, ok := chunk.Body[0].(*AssignStmt)
			if !ok {
				t.Fatalf("statement = %#v, want assignment", chunk.Body[0])
			}
			str, ok := assign.Values[0].(*StringExpr)
			if !ok || str.Value != test.want {
				t.Fatalf("value = %#v, want %q", assign.Values[0], test.want)
			}
		})
	}
}

func TestLongCommentSkipsToMatchingClose(t *testing.T) {
	chunk, err := ParseString("long-comment.lua", "--[[ a=b\nstill a comment ]]\nx = 1\n--[==[ level ]] inside ]==]\ny = 2")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 2 {
		t.Fatalf("statements = %d, want 2 (comments fully skipped)", len(chunk.Body))
	}
}

func TestLongCommentIgnoresInnerShorterBrackets(t *testing.T) {
	chunk, err := ParseString("nested-long-comment.lua", "--[===[ inner ]==] still comment ]===]\nx = 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 1 {
		t.Fatalf("statements = %d, want 1 after nested long comment", len(chunk.Body))
	}
}

func TestUnterminatedLongStringErrors(t *testing.T) {
	if _, err := ParseString("unterminated.lua", "x = [==[unterminated]=]"); err == nil {
		t.Fatal("expected error for long string closed at the wrong bracket level")
	}
}

func TestMethodCalls(t *testing.T) {
	chunk, err := ParseString("test.lua", "obj:method(arg)\nrect:draw(x, y, 10)")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(chunk.Body))
	}

	call1, ok := chunk.Body[0].(*CallStmt)
	if !ok {
		t.Fatalf("stmt 0 = %T, want CallStmt", chunk.Body[0])
	}
	if call1.Call.Method != "method" {
		t.Fatalf("method = %q, want method", call1.Call.Method)
	}
	if len(call1.Call.Args) != 1 {
		t.Fatalf("args count = %d, want 1", len(call1.Call.Args))
	}

	call2, ok := chunk.Body[1].(*CallStmt)
	if !ok {
		t.Fatalf("stmt 1 = %T, want CallStmt", chunk.Body[1])
	}
	if call2.Call.Method != "draw" {
		t.Fatalf("method = %q, want draw", call2.Call.Method)
	}
	if len(call2.Call.Args) != 3 {
		t.Fatalf("args count = %d, want 3", len(call2.Call.Args))
	}
}

func TestMultipleAssignment(t *testing.T) {
	chunk, err := ParseString("test.lua", `a, b = 1, 2
x, y, z = f(), g(), h()
local p, q = 10, 20`)
	if err != nil {
		t.Fatal(err)
	}

	if len(chunk.Body) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(chunk.Body))
	}

	assign1, ok := chunk.Body[0].(*AssignStmt)
	if !ok {
		t.Fatalf("stmt 0 = %T, want AssignStmt", chunk.Body[0])
	}
	if len(assign1.Targets) != 2 || len(assign1.Values) != 2 {
		t.Fatalf("targets=%d values=%d, want 2,2", len(assign1.Targets), len(assign1.Values))
	}

	assign2, ok := chunk.Body[1].(*AssignStmt)
	if !ok {
		t.Fatalf("stmt 1 = %T, want AssignStmt", chunk.Body[1])
	}
	if len(assign2.Targets) != 3 || len(assign2.Values) != 3 {
		t.Fatalf("targets=%d values=%d, want 3,3", len(assign2.Targets), len(assign2.Values))
	}

	local, ok := chunk.Body[2].(*LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt 2 = %T, want LocalAssignStmt", chunk.Body[2])
	}
	if len(local.Names) != 2 || len(local.Values) != 2 {
		t.Fatalf("names=%d values=%d, want 2,2", len(local.Names), len(local.Values))
	}
}

func TestRejectUnsupportedPhase4Constructs(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"update60", "function _update60() end", "_update60"},
		{"closure", "function outer() local f = function() end end", "closures"},
		{"variadic function", "function f(...) end", "variadic functions"},
		{"string format", `print(string.format("%d", 1))`, "string.format"},
		{"dotted update60", "function game._update60() end", "_update60"},
		{"method update60", "function game:_update60() end", "_update60"},
		{"nested in if", "if true then function f() end end", "closures"},
		{"local dotted function", "local function a.b() end", "local function"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseString(test.name+".lua", test.src)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestForeachLowersToAllGenericFor(t *testing.T) {
	chunk, err := ParseString("foreach.lua", `foreach(make_scores(), show_score)`)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 1 {
		t.Fatalf("body has %d statements, want 1", len(chunk.Body))
	}
	loop, ok := chunk.Body[0].(*GenericForStmt)
	if !ok {
		t.Fatalf("statement = %T, want GenericForStmt", chunk.Body[0])
	}
	if len(loop.Names) != 1 || len(loop.Values) != 1 || len(loop.Body.Body) != 1 {
		t.Fatalf("lowered foreach shape = names:%d values:%d body:%d", len(loop.Names), len(loop.Values), len(loop.Body.Body))
	}
	allCall, ok := loop.Values[0].(*CallExpr)
	if !ok {
		t.Fatalf("iterator = %T, want CallExpr", loop.Values[0])
	}
	allName, ok := allCall.Callee.(*VarExpr)
	if !ok || allName.Name != "all" || len(allCall.Args) != 1 {
		t.Fatalf("iterator is not all(table): %#v", allCall)
	}
	bodyCall, ok := loop.Body.Body[0].(*CallStmt)
	if !ok {
		t.Fatalf("loop body = %T, want CallStmt", loop.Body.Body[0])
	}
	callback, ok := bodyCall.Call.Callee.(*VarExpr)
	if !ok || callback.Name != "show_score" || len(bodyCall.Call.Args) != 1 {
		t.Fatalf("callback call shape = %#v", bodyCall.Call)
	}
}

func TestForeachRejectsDynamicOrMalformedCallbacks(t *testing.T) {
	for _, test := range []struct {
		src  string
		want string
	}{
		{`foreach(items)`, "exactly two arguments"},
		{`foreach(items, draw())`, "named function"},
		{`foreach(items, callbacks.draw)`, "named function"},
	} {
		if _, err := ParseString("foreach.lua", test.src); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("ParseString(%q) error = %v, want %q", test.src, err, test.want)
		}
	}
}

func TestParseMtileOperatorsAndCompounds(t *testing.T) {
	tests := []struct {
		name string
		src  string
		op   Operator
	}{
		{"not-equal-bang", "x = a != b", OpNotEqual},
		{"not-equal-lua", "x = a ~= b", OpNotEqual},
		{"bit-and", "x = a & b", OpBitAnd},
		{"bit-or", "x = a | b", OpBitOr},
		{"bit-xor", "x = a ^^ b", OpBitXor},
		{"shift-left", "x = a << b", OpShiftL},
		{"shift-right", "x = a >> b", OpShiftR},
		{"shift-logical", "x = a >>> b", OpShiftRLogical},
		{"rotate-left", "x = a <<> b", OpRotateL},
		{"rotate-right", "x = a >>< b", OpRotateR},
		{"integer-divide", `x = a \ b`, OpIntegerDivide},
		{"add-assign", "x += 1", OpAdd},
		{"sub-assign", "x -= 1", OpSubtract},
		{"mul-assign", "x *= 2", OpMultiply},
		{"div-assign", "x /= 2", OpDivide},
		{"mod-assign", "x %= 2", OpModulo},
		{"idiv-assign", `x \= 2`, OpIntegerDivide},
		{"band-assign", "x &= 1", OpBitAnd},
		{"bor-assign", "x |= 1", OpBitOr},
		{"bxor-assign", "x ^^= 1", OpBitXor},
		{"shl-assign", "x <<= 1", OpShiftL},
		{"shr-assign", "x >>= 1", OpShiftR},
		{"lshr-assign", "x >>>= 1", OpShiftRLogical},
		{"rotl-assign", "x <<>= 1", OpRotateL},
		{"rotr-assign", "x >><= 1", OpRotateR},
		{"pow-assign", "x ^= 2", OpPower},
		{"concat-assign", `x ..= "a"`, OpConcat},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunk, err := ParseString(test.name+".lua", test.src)
			if err != nil {
				t.Fatal(err)
			}
			assign, ok := chunk.Body[0].(*AssignStmt)
			if !ok || len(assign.Values) != 1 {
				t.Fatalf("statement = %#v, want assignment", chunk.Body[0])
			}
			binary, ok := assign.Values[0].(*BinaryExpr)
			if !ok || binary.Op != test.op {
				t.Fatalf("rhs = %#v, want operator %q", assign.Values[0], test.op)
			}
		})
	}
}

func TestParseUnaryBitwiseNot(t *testing.T) {
	chunk, err := ParseString("bnot.lua", "x = ~mask")
	if err != nil {
		t.Fatal(err)
	}
	assign, ok := chunk.Body[0].(*AssignStmt)
	if !ok {
		t.Fatalf("statement = %T, want assignment", chunk.Body[0])
	}
	unary, ok := assign.Values[0].(*UnaryExpr)
	if !ok || unary.Op != OpBitNot {
		t.Fatalf("rhs = %#v, want unary ~", assign.Values[0])
	}
}

func TestParseNumberLiterals(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		value   float64
		integer bool
	}{
		{"binary", "0b1010", 10, true},
		{"hex-fraction", "0x0.8", 0.5, false},
		{"hex-leading-dot", "0x.8", 0.5, false},
		{"binary-fraction", "0b0.1", 0.5, false},
		{"leading-dot-decimal", ".5", 0.5, false},
		{"trailing-dot-decimal", "4.", 4, false},
		{"leading-zeros", "010", 10, true},
		{"fixed16-max", "0x7fff.ffff", 32767 + 15.0/16 + 15.0/256 + 15.0/4096 + 15.0/65536, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunk, err := ParseString(test.name+".lua", "x = "+test.src)
			if err != nil {
				t.Fatal(err)
			}
			assign, ok := chunk.Body[0].(*AssignStmt)
			if !ok {
				t.Fatalf("statement = %T, want assignment", chunk.Body[0])
			}
			num, ok := assign.Values[0].(*NumberExpr)
			if !ok || num.Value != test.value || num.Integer != test.integer {
				t.Fatalf("number = %#v, want value %v integer %v", assign.Values[0], test.value, test.integer)
			}
		})
	}
}

func TestLexDoesNotGlueNumberForms(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"two-decimals", "1.2.3", []string{"1.2", ".3"}},
		{"trailing-dot-then-dot", "4.5.", []string{"4.5", "."}},
		{"concat-after-int", "1..2", []string{"1", "..", "2"}},
		{"hex-range", "0x10..20", []string{"0x10", "..", "20"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tokens, err := Lex(test.name+".lua", test.src)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, token := range tokens {
				if token.Kind != TokenEOF {
					got = append(got, token.Text)
				}
			}
			if strings.Join(got, "|") != strings.Join(test.want, "|") {
				t.Fatalf("tokens = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseSingleLineIfRestOfLine(t *testing.T) {
	chunk, err := ParseString("if-rest.lua", "if (x) i=1 j=2\nk=3")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 2 {
		t.Fatalf("statements = %d, want if plus following assignment", len(chunk.Body))
	}
	branch, ok := chunk.Body[0].(*IfStmt)
	if !ok || len(branch.Then.Body) != 2 || branch.Else != nil {
		t.Fatalf("if = %#v, want two then-assignments and no else", chunk.Body[0])
	}
	if _, ok := chunk.Body[1].(*AssignStmt); !ok {
		t.Fatalf("trailing statement = %T, want assignment", chunk.Body[1])
	}
}

func TestParseSingleLineIfElse(t *testing.T) {
	chunk, err := ParseString("if-else.lua", "if (x) a=1 else a=2")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 1 {
		t.Fatalf("statements = %d, want one if", len(chunk.Body))
	}
	branch, ok := chunk.Body[0].(*IfStmt)
	if !ok || len(branch.Then.Body) != 1 || branch.Else == nil || len(branch.Else.Body) != 1 {
		t.Fatalf("if = %#v, want one then and one else assignment", chunk.Body[0])
	}
}

func TestParseParenthesizedIfWithThenEnd(t *testing.T) {
	chunk, err := ParseString("if-then.lua", "if (x) then y=1 end")
	if err != nil {
		t.Fatal(err)
	}
	branch, ok := chunk.Body[0].(*IfStmt)
	if !ok || len(branch.Then.Body) != 1 || branch.Else != nil {
		t.Fatalf("if = %#v, want then/end form", chunk.Body[0])
	}
}

func TestPrintShorthandIsCall(t *testing.T) {
	chunk, err := ParseString("print-shorthand.lua", "?score")
	if err != nil {
		t.Fatal(err)
	}
	stmt, ok := chunk.Body[0].(*CallStmt)
	if !ok {
		t.Fatalf("statement = %T, want print call", chunk.Body[0])
	}
	callee, ok := stmt.Call.Callee.(*VarExpr)
	arg, argOK := stmt.Call.Args[0].(*VarExpr)
	if !ok || callee.Name != "print" || !argOK || arg.Name != "score" {
		t.Fatalf("call = %#v, want print(score)", stmt.Call)
	}
}

func TestPrintShorthandMustStartLine(t *testing.T) {
	if _, err := ParseString("midline-print.lua", "x=1 ? y"); err == nil || !strings.Contains(err.Error(), "line") {
		t.Fatalf("error = %v, want line-start diagnostic", err)
	}
	chunk, err := ParseString("newline-print.lua", "print(1)\n  ?x")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 2 {
		t.Fatalf("statements = %d, want call plus print shorthand", len(chunk.Body))
	}
	if _, ok := chunk.Body[1].(*CallStmt); !ok {
		t.Fatalf("second statement = %T, want print call", chunk.Body[1])
	}
}

func TestStringAndTableCallShorthands(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		method string
		arg    string
	}{
		{"print-string", `print"string"`, "", "string"},
		{"print-bracket", `print"["`, "", "["},
		{"method-string", `obj:method"arg"`, "method", "arg"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunk, err := ParseString(test.name+".lua", test.src)
			if err != nil {
				t.Fatal(err)
			}
			stmt, ok := chunk.Body[0].(*CallStmt)
			if !ok {
				t.Fatalf("statement = %T, want call", chunk.Body[0])
			}
			if stmt.Call.Method != test.method || len(stmt.Call.Args) != 1 {
				t.Fatalf("call = %#v, want method %q and one arg", stmt.Call, test.method)
			}
			arg, ok := stmt.Call.Args[0].(*StringExpr)
			if !ok || arg.Value != test.arg {
				t.Fatalf("arg = %#v, want %q", stmt.Call.Args[0], test.arg)
			}
		})
	}
}

func TestCompoundAssignmentDoesNotAliasLValue(t *testing.T) {
	chunk, err := ParseString("compound-var.lua", "score += 1")
	if err != nil {
		t.Fatal(err)
	}
	assign, ok := chunk.Body[0].(*AssignStmt)
	if !ok {
		t.Fatalf("statement = %T, want assignment", chunk.Body[0])
	}
	rhs, ok := assign.Values[0].(*BinaryExpr)
	if !ok {
		t.Fatalf("rhs = %#v, want binary expression", assign.Values[0])
	}
	if assign.Targets[0] == rhs.Left {
		t.Fatal("compound assignment aliased the l-value node as the read")
	}
}

func TestMethodDeclarationInsertsSelf(t *testing.T) {
	chunk, err := ParseString("method.lua", "function obj:draw(x)\nend")
	if err != nil {
		t.Fatal(err)
	}
	decl, ok := chunk.Body[0].(*FunctionDeclStmt)
	if !ok {
		t.Fatalf("statement = %T, want function", chunk.Body[0])
	}
	if decl.Method != "draw" || len(decl.Params) != 2 || decl.Params[0] != "self" || decl.Params[1] != "x" {
		t.Fatalf("declaration = %#v, want method draw(self, x)", decl)
	}
}

// Lua 5.1 scanner/syntax fragments from the suite GopherLua runs
// (_lua5.1-tests/literals.lua, constructs.lua) and mixed l-values from
// _glua-tests/issues.lua #315. Runtime asserts are dropped; we only keep
// what the Mtile Lua parser can check.

func TestLuaDecimalEscapesAndNullBytes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"three-digit-then-digits", `"\09912"`, "c12"},
		{"two-digit-then-letters", `"\99ab"`, "cab"},
		{"three-digit-alone", `"\099"`, "c"},
		{"embedded-nul", "'a\\0a'", "a\x00a"},
		{"leading-nuls", `"\0\0\0alo"`, "\x00\x00\x00alo"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tokens, err := Lex(test.name+".lua", test.src)
			if err != nil {
				t.Fatal(err)
			}
			if len(tokens) < 1 || tokens[0].Kind != TokenString {
				t.Fatalf("tokens = %#v, want a string", tokens)
			}
			if got := tokens[0].Value; got != test.want {
				t.Fatalf("value = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLuaOperatorPrecedence(t *testing.T) {
	t.Run("power-right-associative", func(t *testing.T) {
		root := mustBinaryNode(t, parseAssignValue(t, "x = 2^3^2"), OpPower)
		mustNumber(t, root.Left, 2)
		inner := mustBinaryNode(t, root.Right, OpPower)
		mustNumber(t, inner.Left, 3)
		mustNumber(t, inner.Right, 2)
	})
	t.Run("unary-minus-binds-less-than-power", func(t *testing.T) {
		unary, ok := parseAssignValue(t, "x = -2^2").(*UnaryExpr)
		if !ok || unary.Op != OpNegate {
			t.Fatalf("expr = %#v, want -(2^2)", parseAssignValue(t, "x = -2^2"))
		}
		pow := mustBinaryNode(t, unary.Expr, OpPower)
		mustNumber(t, pow.Left, 2)
		mustNumber(t, pow.Right, 2)
	})
	t.Run("concat-between-add-and-mul", func(t *testing.T) {
		concat := mustBinaryNode(t, parseAssignValue(t, "x = 1+2 .. 3*1"), OpConcat)
		add := mustBinaryNode(t, concat.Left, OpAdd)
		mustNumber(t, add.Left, 1)
		mustNumber(t, add.Right, 2)
		mul := mustBinaryNode(t, concat.Right, OpMultiply)
		mustNumber(t, mul.Left, 3)
		mustNumber(t, mul.Right, 1)
	})
	t.Run("and-binds-tighter-than-or", func(t *testing.T) {
		or := mustBinaryNode(t, parseAssignValue(t, "x = true or false and nil"), OpOr)
		if _, ok := or.Left.(*BoolExpr); !ok {
			t.Fatalf("or left = %#v, want true", or.Left)
		}
		and := mustBinaryNode(t, or.Right, OpAnd)
		if _, ok := and.Left.(*BoolExpr); !ok {
			t.Fatalf("and left = %#v, want false", and.Left)
		}
		if _, ok := and.Right.(*NilExpr); !ok {
			t.Fatalf("and right = %#v, want nil", and.Right)
		}
	})
}

func TestLuaEmptyControlBodies(t *testing.T) {
	chunk, err := ParseString("empty-control.lua", "while false do end\nrepeat until true")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 2 {
		t.Fatalf("statements = %d, want while plus repeat", len(chunk.Body))
	}
	loop, ok := chunk.Body[0].(*WhileStmt)
	if !ok || len(loop.Body.Body) != 0 {
		t.Fatalf("while = %#v, want empty body", chunk.Body[0])
	}
	rep, ok := chunk.Body[1].(*RepeatStmt)
	if !ok || len(rep.Body.Body) != 0 {
		t.Fatalf("repeat = %#v, want empty body", chunk.Body[1])
	}
}

func TestLuaTableTrailingComma(t *testing.T) {
	assign := mustAssign(t, "x = { a = 1, b = 2 >= 1, }")
	table, ok := assign.Values[0].(*TableExpr)
	if !ok || len(table.Fields) != 2 {
		t.Fatalf("value = %#v, want two table fields", assign.Values[0])
	}
	rel, ok := table.Fields[1].Value.(*BinaryExpr)
	if !ok || rel.Op != OpGreaterEq {
		t.Fatalf("second field = %#v, want 2 >= 1", table.Fields[1].Value)
	}
}

func TestLuaMixedAssignmentTargets(t *testing.T) {
	assign := mustAssign(t, "f, a.d = f, d")
	if len(assign.Targets) != 2 || len(assign.Values) != 2 {
		t.Fatalf("assign = %#v, want two targets and two values", assign)
	}
	if _, ok := assign.Targets[0].(*VarExpr); !ok {
		t.Fatalf("first target = %#v, want variable", assign.Targets[0])
	}
	field, ok := assign.Targets[1].(*FieldExpr)
	if !ok || field.Name != "d" {
		t.Fatalf("second target = %#v, want field d", assign.Targets[1])
	}
}

func TestLuaTrailingDotAddition(t *testing.T) {
	add := mustBinaryNode(t, parseAssignValue(t, "x = 4.+.5"), OpAdd)
	mustNumber(t, add.Left, 4)
	mustNumber(t, add.Right, 0.5)
}

func TestLuaTrailingDotThenExtraDotErrors(t *testing.T) {
	if _, err := ParseString("extra-dot.lua", "x = 4.5."); err == nil {
		t.Fatal("expected error for 4.5. as a number-like spelling")
	}
}

func TestLuaCarriageReturnLineEnds(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"lf", "x=1\ny=2"},
		{"cr", "x=1\ry=2"},
		{"crlf", "x=1\r\ny=2"},
		{"comment-cr", "x=1 -- c\ry=2"},
		{"comment-crlf", "x=1 -- c\r\ny=2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunk, err := ParseString(test.name+".lua", test.src)
			if err != nil {
				t.Fatal(err)
			}
			if len(chunk.Body) != 2 {
				t.Fatalf("statements = %d, want 2", len(chunk.Body))
			}
		})
	}
}

func mustAssign(t *testing.T, src string) *AssignStmt {
	t.Helper()
	chunk, err := ParseString("lua51.lua", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 1 {
		t.Fatalf("statements = %d, want 1 in %q", len(chunk.Body), src)
	}
	assign, ok := chunk.Body[0].(*AssignStmt)
	if !ok {
		t.Fatalf("statement = %T, want assignment in %q", chunk.Body[0], src)
	}
	return assign
}

func parseAssignValue(t *testing.T, src string) Expr {
	t.Helper()
	assign := mustAssign(t, src)
	if len(assign.Values) != 1 {
		t.Fatalf("values = %d, want 1 in %q", len(assign.Values), src)
	}
	return assign.Values[0]
}

func mustBinaryNode(t *testing.T, expr Expr, op Operator) *BinaryExpr {
	t.Helper()
	binary, ok := expr.(*BinaryExpr)
	if !ok || binary.Op != op {
		t.Fatalf("expr = %#v, want binary %q", expr, op)
	}
	return binary
}

func mustNumber(t *testing.T, expr Expr, want float64) {
	t.Helper()
	num, ok := expr.(*NumberExpr)
	if !ok || num.Value != want {
		t.Fatalf("expr = %#v, want number %v", expr, want)
	}
}
