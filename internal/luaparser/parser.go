package luaparser

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Parse parses a source reader using sourceName for diagnostics.
func Parse(r io.Reader, sourceName string) (*Chunk, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseString(sourceName, string(data))
}

// ParseString parses one Mtile Lua source unit.
func ParseString(sourceName, source string) (*Chunk, error) {
	tokens, err := Lex(sourceName, source)
	if err != nil {
		return nil, err
	}
	p := &Parser{tokens: tokens, usedNames: make(map[string]bool)}
	for _, token := range tokens {
		if token.Kind == TokenIdentifier || token.Kind == TokenKeyword {
			p.usedNames[token.Text] = true
		}
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	if token := p.peek(0); token.Kind != TokenEOF {
		return nil, p.errorAt(token, "unexpected token after chunk")
	}
	chunk := &Chunk{
		NodeBase:   NodeBase{At: Position{Source: sourceName, Line: 1, Column: 1}},
		SourceName: sourceName,
		Body:       body.Body,
	}
	if len(body.Body) > 0 {
		chunk.Last = body.Last
	}
	return chunk, nil
}

// ParseFile is a convenience for the CLI and later build pipeline.
func ParseFile(filename string) (*Chunk, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return Parse(file, filename)
}

// Parser is intentionally small and allocation-light. The lexer produces a
// token slice once, allowing the parser to look ahead for compound assignment
// and for the `name = value` versus array-field ambiguity in constructors.
type Parser struct {
	tokens        []Token
	index         int
	usedNames     map[string]bool
	tempNumber    int
	functionDepth int
	controlDepth  int
}

func (p *Parser) peek(n int) Token {
	index := p.index + n
	if index >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[index]
}

func (p *Parser) take() Token {
	token := p.peek(0)
	if p.index < len(p.tokens) {
		p.index++
	}
	return token
}

func (p *Parser) match(text string) bool {
	if p.peek(0).Text == text {
		p.take()
		return true
	}
	return false
}

func (p *Parser) expect(text string) (Token, error) {
	token := p.peek(0)
	if token.Text != text {
		return Token{}, p.errorAt(token, "expected "+text)
	}
	p.take()
	return token, nil
}

func (p *Parser) expectIdentifier() (Token, error) {
	token := p.peek(0)
	if token.Kind != TokenIdentifier {
		return Token{}, p.errorAt(token, "expected identifier")
	}
	p.take()
	return token, nil
}

func (p *Parser) errorAt(token Token, message string) error {
	return &Error{Pos: token.Pos, Token: token.Text, Message: message}
}

func (p *Parser) parseBlock(stops ...string) (*Block, error) {
	stop := make(map[string]bool, len(stops))
	for _, text := range stops {
		stop[text] = true
	}
	start := p.peek(0).Pos
	block := &Block{NodeBase: NodeBase{At: start}}
	for {
		token := p.peek(0)
		if token.Kind == TokenEOF || stop[token.Text] {
			block.Last = token.Pos
			return block, nil
		}
		if token.Text == ";" {
			p.take()
			continue
		}
		statements, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		block.Body = append(block.Body, statements...)
	}
}

func (p *Parser) parseStatement() ([]Stmt, error) {
	token := p.peek(0)
	switch token.Text {
	case "?":
		if p.index > 0 && p.tokens[p.index-1].Pos.Line == token.Pos.Line {
			return nil, p.errorAt(token, "? print shorthand must start a line")
		}
		p.take()
		values, err := p.parseExpressionList()
		if err != nil {
			return nil, err
		}
		call := &CallExpr{
			NodeBase: NodeBase{At: token.Pos},
			Callee:   &VarExpr{NodeBase: NodeBase{At: token.Pos}, Name: "print"},
			Args:     values,
		}
		if err := p.rejectUnsupportedCall(call); err != nil {
			return nil, err
		}
		return []Stmt{&CallStmt{NodeBase: NodeBase{At: token.Pos}, Call: call}}, nil
	case "local":
		p.take()
		if p.match("function") {
			return p.parseFunctionDeclaration(true, token.Pos)
		}
		return p.parseLocalAssignment(token.Pos)
	case "function":
		p.take()
		return p.parseFunctionDeclaration(false, token.Pos)
	case "if":
		return p.parseIf()
	case "while":
		return p.parseWhile()
	case "repeat":
		return p.parseRepeat()
	case "do":
		return p.parseDo()
	case "for":
		return p.parseFor()
	case "return":
		return p.parseReturn()
	case "break":
		p.take()
		return []Stmt{&BreakStmt{NodeBase: NodeBase{At: token.Pos}}}, nil
	case "goto", "::":
		return nil, p.errorAt(token, "goto and labels are unsupported in Mtile")
	default:
		return p.parseAssignmentOrCall()
	}
}

func (p *Parser) parseLocalAssignment(pos Position) ([]Stmt, error) {
	first, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}
	names := []string{first.Text}
	for p.match(",") {
		name, err := p.expectIdentifier()
		if err != nil {
			return nil, err
		}
		names = append(names, name.Text)
	}
	var values []Expr
	if p.match("=") {
		values, err = p.parseExpressionList()
		if err != nil {
			return nil, err
		}
	}
	return []Stmt{&LocalAssignStmt{NodeBase: NodeBase{At: pos}, Names: names, Values: values}}, nil
}

func (p *Parser) parseAssignmentOrCall() ([]Stmt, error) {
	first, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	if isCompoundOperator(p.peek(0).Text) {
		operatorToken := p.take()
		if !isAssignable(first) {
			return nil, p.errorAt(operatorToken, "compound assignment requires a variable, field, or indexed l-value")
		}
		right, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		return p.lowerCompound(first, operatorToken.Text, right)
	}
	if p.peek(0).Text == "," || p.peek(0).Text == "=" {
		if !isAssignable(first) {
			return nil, p.errorAt(p.peek(0), "assignment requires a variable, field, or indexed l-value")
		}
		targets := []Expr{first}
		for p.match(",") {
			target, err := p.parseExpression(0)
			if err != nil {
				return nil, err
			}
			if !isAssignable(target) {
				return nil, p.errorAt(p.previous(), "assignment target is not assignable")
			}
			targets = append(targets, target)
		}
		if _, err := p.expect("="); err != nil {
			return nil, err
		}
		values, err := p.parseExpressionList()
		if err != nil {
			return nil, err
		}
		return []Stmt{&AssignStmt{NodeBase: NodeBase{At: first.Pos()}, Targets: targets, Values: values}}, nil
	}
	if call, ok := first.(*CallExpr); ok {
		if name, ok := call.Callee.(*VarExpr); ok && name.Name == "foreach" {
			return p.lowerForeach(call)
		}
		return []Stmt{&CallStmt{NodeBase: NodeBase{At: first.Pos()}, Call: call}}, nil
	}
	return nil, p.errorAt(p.peek(0), "expected assignment or function call")
}

// lowerForeach turns PICO-8's foreach(table, callback) statement into the
// existing all(table) generic-for AST. The callback must be a static name:
// Mtile deliberately has no first-class function values or closures. The
// generic-for generator evaluates the table expression once and roots it for
// the whole iteration, matching foreach's call semantics without introducing
// another runtime iterator implementation.
func (p *Parser) lowerForeach(call *CallExpr) ([]Stmt, error) {
	if len(call.Args) != 2 {
		return nil, p.errorAt(p.previous(), "foreach expects exactly two arguments: table and function name")
	}
	callback, ok := call.Args[1].(*VarExpr)
	if !ok {
		return nil, p.errorAt(p.previous(), "foreach callback must be a named function without parentheses")
	}
	valueName := p.newTemp()
	value := &VarExpr{NodeBase: NodeBase{At: call.Pos()}, Name: valueName}
	allCall := &CallExpr{
		NodeBase: NodeBase{At: call.Pos(), Last: call.Args[0].Pos()},
		Callee:   &VarExpr{NodeBase: NodeBase{At: call.Pos()}, Name: "all"},
		Args:     []Expr{call.Args[0]},
	}
	callbackCall := &CallExpr{
		NodeBase: NodeBase{At: callback.Pos(), Last: call.LastPos()},
		Callee:   callback,
		Args:     []Expr{value},
	}
	loop := &GenericForStmt{
		NodeBase: NodeBase{At: call.Pos(), Last: call.LastPos()},
		Names:    []string{valueName},
		Values:   []Expr{allCall},
		Body: &Block{
			NodeBase: NodeBase{At: call.Pos(), Last: call.LastPos()},
			Body:     []Stmt{&CallStmt{NodeBase: NodeBase{At: callback.Pos(), Last: call.LastPos()}, Call: callbackCall}},
		},
	}
	return []Stmt{loop}, nil
}

func (p *Parser) previous() Token {
	if p.index == 0 {
		return p.peek(0)
	}
	return p.tokens[p.index-1]
}

func (p *Parser) parseFunctionDeclaration(local bool, pos Position) ([]Stmt, error) {
	if p.functionDepth > 0 || p.controlDepth > 0 {
		return nil, p.errorAt(p.peek(0), "closures are unsupported; functions must be top-level")
	}
	name, method, err := p.parseFunctionName()
	if err != nil {
		return nil, err
	}
	if local && (len(name) > 1 || method != "") {
		return nil, p.errorAt(p.previous(), "local function cannot be a field or method")
	}
	if hasUpdate60(name, method) {
		return nil, p.errorAt(p.previous(), "_update60 is unsupported; Mtile runs _update at 30 Hz (PICO-8 default rate)")
	}
	if _, err := p.expect("("); err != nil {
		return nil, err
	}
	params := []string{}
	vararg := false
	if !p.match(")") {
		for {
			if p.match("...") {
				return nil, p.errorAt(p.previous(), "variadic functions are unsupported in Mtile")
			}
			param, err := p.expectIdentifier()
			if err != nil {
				return nil, err
			}
			params = append(params, param.Text)
			if p.match(")") {
				break
			}
			if _, err := p.expect(","); err != nil {
				return nil, err
			}
		}
	}
	if method != "" {
		params = append([]string{"self"}, params...)
	}
	p.functionDepth++
	body, err := p.parseBlock("end")
	p.functionDepth--
	if err != nil {
		return nil, err
	}
	end, err := p.expect("end")
	if err != nil {
		return nil, err
	}
	stmt := &FunctionDeclStmt{
		NodeBase: NodeBase{At: pos, Last: end.Pos},
		Name:     name, Method: method, Params: params, Vararg: vararg,
		Body: body, Local: local,
	}
	return []Stmt{stmt}, nil
}

func hasUpdate60(name []string, method string) bool {
	if method == "_update60" {
		return true
	}
	for _, part := range name {
		if part == "_update60" {
			return true
		}
	}
	return false
}

func (p *Parser) parseFunctionName() ([]string, string, error) {
	first, err := p.expectIdentifier()
	if err != nil {
		return nil, "", err
	}
	name := []string{first.Text}
	for p.match(".") {
		part, err := p.expectIdentifier()
		if err != nil {
			return nil, "", err
		}
		name = append(name, part.Text)
	}
	method := ""
	if p.match(":") {
		part, err := p.expectIdentifier()
		if err != nil {
			return nil, "", err
		}
		method = part.Text
	}
	return name, method, nil
}

func (p *Parser) parseIf() ([]Stmt, error) {
	start := p.take()
	parenthesized := p.peek(0).Text == "("
	condition, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	if p.peek(0).Text != "then" && parenthesized {
		if p.peek(0).Pos.Line != start.Pos.Line {
			return nil, p.errorAt(p.peek(0), "single-line if condition and statement must share a line")
		}
		p.controlDepth++
		bodyStatements, err := p.parseSameLineStatements(start.Pos.Line)
		p.controlDepth--
		if err != nil {
			return nil, err
		}
		body := &Block{NodeBase: NodeBase{At: bodyStatements[0].Pos()}, Body: bodyStatements}
		body.Last = bodyStatements[len(bodyStatements)-1].Pos()
		stmt := &IfStmt{NodeBase: NodeBase{At: start.Pos, Last: body.Last}, Condition: condition, Then: body}
		if p.peek(0).Text == "else" && p.peek(0).Pos.Line == start.Pos.Line {
			p.take()
			p.controlDepth++
			elseStatements, err := p.parseSameLineStatements(start.Pos.Line)
			p.controlDepth--
			if err != nil {
				return nil, err
			}
			elseBody := &Block{NodeBase: NodeBase{At: elseStatements[0].Pos()}, Body: elseStatements}
			elseBody.Last = elseStatements[len(elseStatements)-1].Pos()
			stmt.Else = elseBody
			stmt.Last = elseBody.Last
		}
		return []Stmt{stmt}, nil
	}
	if _, err := p.expect("then"); err != nil {
		return nil, err
	}
	then, err := p.parseControlBlock("elseif", "else", "end")
	if err != nil {
		return nil, err
	}
	stmt := &IfStmt{NodeBase: NodeBase{At: start.Pos}, Condition: condition, Then: then}
	for p.match("elseif") {
		branchPos := p.previous().Pos
		branchCondition, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect("then"); err != nil {
			return nil, err
		}
		branchBody, err := p.parseControlBlock("elseif", "else", "end")
		if err != nil {
			return nil, err
		}
		stmt.ElseIf = append(stmt.ElseIf, &IfBranch{NodeBase: NodeBase{At: branchPos}, Condition: branchCondition, Body: branchBody})
	}
	if p.match("else") {
		elseBody, err := p.parseControlBlock("end")
		if err != nil {
			return nil, err
		}
		stmt.Else = elseBody
	}
	end, err := p.expect("end")
	if err != nil {
		return nil, err
	}
	stmt.Last = end.Pos
	return []Stmt{stmt}, nil
}

func (p *Parser) parseControlBlock(stops ...string) (*Block, error) {
	p.controlDepth++
	defer func() { p.controlDepth-- }()
	return p.parseBlock(stops...)
}

func (p *Parser) parseSameLineStatements(line int) ([]Stmt, error) {
	var statements []Stmt
	for {
		token := p.peek(0)
		if token.Kind == TokenEOF || token.Pos.Line != line {
			break
		}
		if token.Text == "else" || token.Text == "elseif" || token.Text == "end" || token.Text == "until" {
			break
		}
		if token.Text == ";" {
			p.take()
			continue
		}
		more, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		statements = append(statements, more...)
	}
	if len(statements) == 0 {
		return nil, p.errorAt(p.peek(0), "single-line if requires a statement")
	}
	return statements, nil
}

func (p *Parser) parseWhile() ([]Stmt, error) {
	start := p.take()
	condition, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect("do"); err != nil {
		return nil, err
	}
	body, err := p.parseControlBlock("end")
	if err != nil {
		return nil, err
	}
	end, err := p.expect("end")
	if err != nil {
		return nil, err
	}
	return []Stmt{&WhileStmt{NodeBase: NodeBase{At: start.Pos, Last: end.Pos}, Condition: condition, Body: body}}, nil
}

func (p *Parser) parseRepeat() ([]Stmt, error) {
	start := p.take()
	body, err := p.parseControlBlock("until")
	if err != nil {
		return nil, err
	}
	p.take()
	condition, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	return []Stmt{&RepeatStmt{NodeBase: NodeBase{At: start.Pos, Last: condition.Pos()}, Body: body, Condition: condition}}, nil
}

func (p *Parser) parseDo() ([]Stmt, error) {
	start := p.take()
	body, err := p.parseControlBlock("end")
	if err != nil {
		return nil, err
	}
	end, err := p.expect("end")
	if err != nil {
		return nil, err
	}
	return []Stmt{&DoStmt{NodeBase: NodeBase{At: start.Pos, Last: end.Pos}, Body: body}}, nil
}

func (p *Parser) parseFor() ([]Stmt, error) {
	start := p.take()
	first, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}
	if p.match("=") {
		init, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(","); err != nil {
			return nil, err
		}
		limit, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		step := Expr(&NumberExpr{NodeBase: NodeBase{At: start.Pos}, Raw: "1", Value: 1, Integer: true})
		if p.match(",") {
			step, err = p.parseExpression(0)
			if err != nil {
				return nil, err
			}
		}
		if _, err := p.expect("do"); err != nil {
			return nil, err
		}
		body, err := p.parseControlBlock("end")
		if err != nil {
			return nil, err
		}
		end, err := p.expect("end")
		if err != nil {
			return nil, err
		}
		return []Stmt{&NumericForStmt{NodeBase: NodeBase{At: start.Pos, Last: end.Pos}, Name: first.Text, Init: init, Limit: limit, Step: step, Body: body}}, nil
	}
	names := []string{first.Text}
	for p.match(",") {
		name, err := p.expectIdentifier()
		if err != nil {
			return nil, err
		}
		names = append(names, name.Text)
	}
	if _, err := p.expect("in"); err != nil {
		return nil, err
	}
	values, err := p.parseExpressionList()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect("do"); err != nil {
		return nil, err
	}
	body, err := p.parseControlBlock("end")
	if err != nil {
		return nil, err
	}
	end, err := p.expect("end")
	if err != nil {
		return nil, err
	}
	return []Stmt{&GenericForStmt{NodeBase: NodeBase{At: start.Pos, Last: end.Pos}, Names: names, Values: values, Body: body}}, nil
}

func (p *Parser) parseReturn() ([]Stmt, error) {
	start := p.take()
	var values []Expr
	next := p.peek(0)
	if next.Pos.Line == start.Pos.Line && next.Text != ";" && next.Text != "end" && next.Text != "else" && next.Text != "elseif" && next.Text != "until" && next.Kind != TokenEOF {
		var err error
		values, err = p.parseExpressionList()
		if err != nil {
			return nil, err
		}
	}
	return []Stmt{&ReturnStmt{NodeBase: NodeBase{At: start.Pos}, Values: values}}, nil
}

func (p *Parser) parseExpressionList() ([]Expr, error) {
	first, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	values := []Expr{first}
	for p.match(",") {
		value, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (p *Parser) parseExpression(minPrecedence int) (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		token := p.peek(0)
		op, precedence, rightAssociative := binaryOperator(token.Text)
		if op == "" || precedence < minPrecedence {
			break
		}
		p.take()
		nextPrecedence := precedence + 1
		if rightAssociative {
			nextPrecedence = precedence
		}
		right, err := p.parseExpression(nextPrecedence)
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{NodeBase: NodeBase{At: left.Pos(), Last: right.Pos()}, Left: left, Op: op, Right: right}
	}
	return left, nil
}

func (p *Parser) parseUnary() (Expr, error) {
	token := p.peek(0)
	var op Operator
	switch token.Text {
	case "not":
		op = OpNot
	case "-":
		op = OpNegate
	case "~":
		op = OpBitNot
	case "#":
		op = OpLength
	default:
		return p.parsePostfix()
	}
	p.take()
	operand, err := p.parseExpression(11)
	if err != nil {
		return nil, err
	}
	return &UnaryExpr{NodeBase: NodeBase{At: token.Pos, Last: operand.Pos()}, Op: op, Expr: operand}, nil
}

func binaryOperator(text string) (Operator, int, bool) {
	switch text {
	case "or":
		return OpOr, 1, false
	case "and":
		return OpAnd, 2, false
	case "==":
		return OpEqual, 3, false
	case "!=", "~=":
		return OpNotEqual, 3, false
	case "<":
		return OpLess, 3, false
	case "<=":
		return OpLessEq, 3, false
	case ">":
		return OpGreater, 3, false
	case ">=":
		return OpGreaterEq, 3, false
	case "|":
		return OpBitOr, 4, false
	case "^^":
		return OpBitXor, 5, false
	case "&":
		return OpBitAnd, 6, false
	case "<<":
		return OpShiftL, 7, false
	case ">>":
		return OpShiftR, 7, false
	case ">>>":
		return OpShiftRLogical, 7, false
	case "<<>":
		return OpRotateL, 7, false
	case ">><":
		return OpRotateR, 7, false
	case "..":
		return OpConcat, 8, true
	case "+":
		return OpAdd, 9, false
	case "-":
		return OpSubtract, 9, false
	case "*":
		return OpMultiply, 10, false
	case "/":
		return OpDivide, 10, false
	case "\\":
		return OpIntegerDivide, 10, false
	case "%":
		return OpModulo, 10, false
	case "^":
		return OpPower, 12, true
	default:
		return "", 0, false
	}
}

func (p *Parser) parsePostfix() (Expr, error) {
	expr, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		token := p.peek(0)
		switch {
		case token.Kind == TokenSymbol && token.Text == "[":
			start := p.take()
			key, err := p.parseExpression(0)
			if err != nil {
				return nil, err
			}
			end, err := p.expect("]")
			if err != nil {
				return nil, err
			}
			expr = &IndexExpr{NodeBase: NodeBase{At: start.Pos, Last: end.Pos}, Base: expr, Key: key}
		case token.Kind == TokenSymbol && token.Text == ".":
			p.take()
			name, err := p.expectIdentifier()
			if err != nil {
				return nil, err
			}
			if name.Text == "_update60" {
				return nil, p.errorAt(name, "_update60 is unsupported; Mtile runs _update at 30 Hz (PICO-8 default rate)")
			}
			expr = &FieldExpr{NodeBase: NodeBase{At: expr.Pos(), Last: name.Pos}, Base: expr, Name: name.Text}
		case token.Kind == TokenSymbol && token.Text == ":":
			colon := p.take()
			method, err := p.expectIdentifier()
			if err != nil {
				return nil, err
			}
			if method.Text == "_update60" {
				return nil, p.errorAt(method, "_update60 is unsupported; Mtile runs _update at 30 Hz (PICO-8 default rate)")
			}
			args, err := p.parseCallArguments()
			if err != nil {
				return nil, err
			}
			call := &CallExpr{NodeBase: NodeBase{At: colon.Pos, Last: p.previous().Pos}, Callee: expr, Args: args, Method: method.Text}
			if err := p.rejectUnsupportedCall(call); err != nil {
				return nil, err
			}
			expr = call
		case token.Kind == TokenString || (token.Kind == TokenSymbol && (token.Text == "(" || token.Text == "{")):
			args, err := p.parseCallArguments()
			if err != nil {
				return nil, err
			}
			call := &CallExpr{NodeBase: NodeBase{At: expr.Pos(), Last: p.previous().Pos}, Callee: expr, Args: args}
			if err := p.rejectUnsupportedCall(call); err != nil {
				return nil, err
			}
			expr = call
		default:
			return expr, nil
		}
	}
}

func (p *Parser) parseCallArguments() ([]Expr, error) {
	token := p.peek(0)
	switch {
	case token.Kind == TokenSymbol && token.Text == "(":
		return p.parseCallArgs()
	case token.Kind == TokenSymbol && token.Text == "{":
		table, err := p.parseTable()
		if err != nil {
			return nil, err
		}
		return []Expr{table}, nil
	case token.Kind == TokenString:
		stringToken := p.take()
		arg := &StringExpr{NodeBase: NodeBase{At: stringToken.Pos}, Raw: stringToken.Text, Value: stringToken.Value}
		return []Expr{arg}, nil
	default:
		return nil, p.errorAt(token, "expected function arguments")
	}
}

func (p *Parser) parseCallArgs() ([]Expr, error) {
	if _, err := p.expect("("); err != nil {
		return nil, err
	}
	if p.match(")") {
		return nil, nil
	}
	args, err := p.parseExpressionList()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(")"); err != nil {
		return nil, err
	}
	return args, nil
}

func (p *Parser) rejectUnsupportedCall(call *CallExpr) error {
	if field, ok := call.Callee.(*FieldExpr); ok {
		if base, ok := field.Base.(*VarExpr); ok && base.Name == "string" && field.Name == "format" {
			return p.errorAt(p.previous(), "string.format is unsupported in Mtile")
		}
	}
	return nil
}

func (p *Parser) parsePrimary() (Expr, error) {
	token := p.peek(0)
	switch token.Kind {
	case TokenNumber:
		p.take()
		value, integer, err := parseNumber(token.Text)
		if err != nil {
			return nil, p.errorAt(token, err.Error())
		}
		return &NumberExpr{NodeBase: NodeBase{At: token.Pos}, Raw: token.Text, Value: value, Integer: integer}, nil
	case TokenString:
		p.take()
		return &StringExpr{NodeBase: NodeBase{At: token.Pos}, Raw: token.Text, Value: token.Value}, nil
	case TokenIdentifier:
		p.take()
		if token.Text == "_update60" {
			return nil, p.errorAt(token, "_update60 is unsupported; Mtile runs _update at 30 Hz (PICO-8 default rate)")
		}
		return &VarExpr{NodeBase: NodeBase{At: token.Pos}, Name: token.Text}, nil
	case TokenKeyword:
		switch token.Text {
		case "nil":
			p.take()
			return &NilExpr{NodeBase: NodeBase{At: token.Pos}}, nil
		case "true", "false":
			p.take()
			return &BoolExpr{NodeBase: NodeBase{At: token.Pos}, Value: token.Text == "true"}, nil
		case "function":
			return nil, p.errorAt(token, "closures are unsupported; functions must be top-level declarations")
		default:
			return nil, p.errorAt(token, "expected expression")
		}
	case TokenSymbol:
		switch token.Text {
		case "(":
			p.take()
			expr, err := p.parseExpression(0)
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(")"); err != nil {
				return nil, err
			}
			return expr, nil
		case "{":
			return p.parseTable()
		case "...":
			return nil, p.errorAt(token, "variadic arguments are unsupported in Mtile")
		default:
			return nil, p.errorAt(token, "expected expression")
		}
	default:
		return nil, p.errorAt(token, "expected expression")
	}
}

func parseNumber(raw string) (float64, bool, error) {
	switch {
	case strings.HasPrefix(raw, "0b"), strings.HasPrefix(raw, "0B"):
		return parseBasedNumber(raw[2:], 2)
	case strings.HasPrefix(raw, "0x"), strings.HasPrefix(raw, "0X"):
		return parseBasedNumber(raw[2:], 16)
	case !strings.ContainsAny(raw, ".eE"):
		value, err := strconv.ParseInt(raw, 10, 64)
		return float64(value), true, err
	default:
		value, err := strconv.ParseFloat(raw, 64)
		return value, false, err
	}
}

func parseBasedNumber(digits string, base int) (float64, bool, error) {
	intPart, fracPart, hasDot := strings.Cut(digits, ".")
	var value float64
	if intPart != "" {
		n, err := strconv.ParseUint(intPart, base, 64)
		if err != nil {
			return 0, false, err
		}
		value = float64(n)
	}
	if !hasDot {
		return value, true, nil
	}
	scale := 1.0
	baseValue := float64(base)
	for i := 0; i < len(fracPart); i++ {
		n, err := strconv.ParseUint(fracPart[i:i+1], base, 8)
		if err != nil {
			return 0, false, err
		}
		scale /= baseValue
		value += float64(n) * scale
	}
	return value, false, nil
}

func (p *Parser) parseTable() (*TableExpr, error) {
	start, err := p.expect("{")
	if err != nil {
		return nil, err
	}
	table := &TableExpr{NodeBase: NodeBase{At: start.Pos}}
	for !p.match("}") {
		fieldStart := p.peek(0).Pos
		var key Expr
		array := false
		if p.match("[") {
			key, err = p.parseExpression(0)
			if err != nil {
				return nil, err
			}
			if _, err := p.expect("]"); err != nil {
				return nil, err
			}
			if _, err := p.expect("="); err != nil {
				return nil, err
			}
		} else if p.peek(0).Kind == TokenIdentifier && p.peek(1).Text == "=" {
			name := p.take()
			p.take()
			key = &StringExpr{NodeBase: NodeBase{At: name.Pos}, Raw: name.Text, Value: name.Text}
		} else {
			array = true
		}
		value, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		table.Fields = append(table.Fields, &TableField{NodeBase: NodeBase{At: fieldStart, Last: value.Pos()}, Key: key, Value: value, Array: array})
		if p.match("}") {
			table.Last = p.previous().Pos
			return table, nil
		}
		if _, err := p.expect(","); err != nil {
			if _, semiErr := p.expect(";"); semiErr != nil {
				return nil, err
			}
		}
		if p.match("}") {
			table.Last = p.previous().Pos
			return table, nil
		}
	}
	table.Last = p.previous().Pos
	return table, nil
}

func isAssignable(expr Expr) bool {
	switch expr.(type) {
	case *VarExpr, *IndexExpr, *FieldExpr:
		return true
	default:
		return false
	}
}

var compoundOperators = map[string]Operator{
	"+=": OpAdd, "-=": OpSubtract, "*=": OpMultiply, "/=": OpDivide,
	"%=": OpModulo, "\\=": OpIntegerDivide, "&=": OpBitAnd, "|=": OpBitOr,
	"^^=": OpBitXor, "<<=": OpShiftL, ">>=": OpShiftR, ">>>=": OpShiftRLogical,
	"<<>=": OpRotateL, ">><=": OpRotateR, "^=": OpPower, "..=": OpConcat,
}

func isCompoundOperator(text string) bool {
	_, ok := compoundOperators[text]
	return ok
}

func (p *Parser) lowerCompound(target Expr, spelling string, value Expr) ([]Stmt, error) {
	op := compoundOperators[spelling]
	switch target := target.(type) {
	case *VarExpr:
		read := &VarExpr{NodeBase: target.NodeBase, Name: target.Name}
		return []Stmt{&AssignStmt{
			NodeBase: NodeBase{At: target.Pos()}, Targets: []Expr{target},
			Values: []Expr{&BinaryExpr{NodeBase: NodeBase{At: target.Pos(), Last: value.Pos()}, Left: read, Op: op, Right: value}},
		}}, nil
	case *FieldExpr:
		baseName := p.newTemp()
		readBase := &VarExpr{NodeBase: NodeBase{At: target.Base.Pos()}, Name: baseName}
		writeBase := &VarExpr{NodeBase: NodeBase{At: target.Base.Pos()}, Name: baseName}
		read := &FieldExpr{NodeBase: NodeBase{At: target.Pos()}, Base: readBase, Name: target.Name}
		write := &FieldExpr{NodeBase: NodeBase{At: target.Pos()}, Base: writeBase, Name: target.Name}
		local := &LocalAssignStmt{NodeBase: NodeBase{At: target.Pos()}, Names: []string{baseName}, Values: []Expr{target.Base}, Synthetic: true}
		assign := &AssignStmt{NodeBase: NodeBase{At: target.Pos()}, Targets: []Expr{write}, Values: []Expr{&BinaryExpr{NodeBase: NodeBase{At: target.Pos(), Last: value.Pos()}, Left: read, Op: op, Right: value}}, Synthetic: true}
		return []Stmt{local, assign}, nil
	default:
		indexed := target.(*IndexExpr)
		baseName := p.newTemp()
		keyName := p.newTemp()
		readBase := &VarExpr{NodeBase: NodeBase{At: indexed.Base.Pos()}, Name: baseName}
		writeBase := &VarExpr{NodeBase: NodeBase{At: indexed.Base.Pos()}, Name: baseName}
		readKey := &VarExpr{NodeBase: NodeBase{At: indexed.Key.Pos()}, Name: keyName}
		writeKey := &VarExpr{NodeBase: NodeBase{At: indexed.Key.Pos()}, Name: keyName}
		read := &IndexExpr{NodeBase: NodeBase{At: indexed.Pos()}, Base: readBase, Key: readKey}
		write := &IndexExpr{NodeBase: NodeBase{At: indexed.Pos()}, Base: writeBase, Key: writeKey}
		baseLocal := &LocalAssignStmt{NodeBase: NodeBase{At: target.Pos()}, Names: []string{baseName}, Values: []Expr{indexed.Base}, Synthetic: true}
		keyLocal := &LocalAssignStmt{NodeBase: NodeBase{At: target.Pos()}, Names: []string{keyName}, Values: []Expr{indexed.Key}, Synthetic: true}
		assign := &AssignStmt{NodeBase: NodeBase{At: target.Pos()}, Targets: []Expr{write}, Values: []Expr{&BinaryExpr{NodeBase: NodeBase{At: target.Pos(), Last: value.Pos()}, Left: read, Op: op, Right: value}}, Synthetic: true}
		return []Stmt{baseLocal, keyLocal, assign}, nil
	}
}

func (p *Parser) newTemp() string {
	for {
		name := fmt.Sprintf("__glu_tmp%d", p.tempNumber)
		p.tempNumber++
		if !p.usedNames[name] {
			p.usedNames[name] = true
			return name
		}
	}
}
