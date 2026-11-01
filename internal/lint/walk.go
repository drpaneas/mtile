package lint

import p "github.com/drpaneas/mtile/internal/luaparser"

// The linter walks the parsed dialect, including compound-assignment lowering.
// A loop context is lexical evidence, not a claim that a branch executes.
type context struct {
	loops []*p.NumericForStmt
	guard *p.Position
}

func (c context) conditional(node p.Node) context {
	if c.guard == nil {
		pos := node.Pos()
		c.guard = &pos
	}
	return c
}

type visitor struct {
	block func([]p.Stmt)
	stmt  func(p.Stmt, context)
	expr  func(p.Expr, context)
}

func (v visitor) statements(stmts []p.Stmt, c context) {
	if v.block != nil {
		v.block(stmts)
	}
	for _, stmt := range stmts {
		if v.stmt != nil {
			v.stmt(stmt, c)
		}
		switch s := stmt.(type) {
		case *p.AssignStmt:
			for _, e := range s.Targets {
				v.expression(e, c)
			}
			for _, e := range s.Values {
				v.expression(e, c)
			}
		case *p.LocalAssignStmt:
			for _, e := range s.Values {
				v.expression(e, c)
			}
		case *p.CallStmt:
			v.expression(s.Call, c)
		case *p.FunctionDeclStmt:
			// Function bodies are analyzed separately with their call-graph context.
		case *p.IfStmt:
			v.expression(s.Condition, c)
			branch := c.conditional(s)
			v.statements(s.Then.Body, branch)
			for _, branch := range s.ElseIf {
				v.expression(branch.Condition, c.conditional(s))
				v.statements(branch.Body.Body, c.conditional(s))
			}
			if s.Else != nil {
				v.statements(s.Else.Body, branch)
			}
		case *p.DoStmt:
			v.statements(s.Body.Body, c)
		case *p.WhileStmt:
			v.expression(s.Condition, c)
			v.statements(s.Body.Body, c.conditional(s))
		case *p.RepeatStmt:
			v.statements(s.Body.Body, c)
			v.expression(s.Condition, c)
		case *p.NumericForStmt:
			v.expression(s.Init, c)
			v.expression(s.Limit, c)
			v.expression(s.Step, c)
			// Copy so sibling traversals cannot overwrite a retained loop context.
			body := c
			body.loops = append(append([]*p.NumericForStmt(nil), c.loops...), s)
			if !literalRange(s) {
				body = body.conditional(s)
			}
			v.statements(s.Body.Body, body)
		case *p.GenericForStmt:
			for _, e := range s.Values {
				v.expression(e, c)
			}
			v.statements(s.Body.Body, c.conditional(s))
		case *p.ReturnStmt:
			for _, e := range s.Values {
				v.expression(e, c)
			}
		}
	}
}

func (v visitor) expression(expr p.Expr, c context) {
	if expr == nil {
		return
	}
	if v.expr != nil {
		v.expr(expr, c)
	}
	switch e := expr.(type) {
	case *p.ParenExpr:
		v.expression(e.Expr, c)
	case *p.UnaryExpr:
		v.expression(e.Expr, c)
	case *p.BinaryExpr:
		v.expression(e.Left, c)
		if e.Op == p.OpAnd || e.Op == p.OpOr {
			c = c.conditional(e)
		}
		v.expression(e.Right, c)
	case *p.IndexExpr:
		v.expression(e.Base, c)
		v.expression(e.Key, c)
	case *p.FieldExpr:
		v.expression(e.Base, c)
	case *p.CallExpr:
		v.expression(e.Callee, c)
		for _, arg := range e.Args {
			v.expression(arg, c)
		}
	case *p.TableExpr:
		for _, field := range e.Fields {
			v.expression(field.Key, c)
			v.expression(field.Value, c)
		}
	}
}
