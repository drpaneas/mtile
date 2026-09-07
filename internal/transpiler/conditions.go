package transpiler

import "github.com/drpaneas/mtile/internal/luaparser"

func (g *generator) emitConditionFlush(b *sourceBuilder, indent int, expr luaparser.Expr) string {
	condition := g.emitCondition(expr)
	g.writePending(b, indent)
	return condition
}

func comparisonOperator(op luaparser.Operator) bool {
	switch op {
	case luaparser.OpEqual, luaparser.OpNotEqual, luaparser.OpLess,
		luaparser.OpLessEq, luaparser.OpGreater, luaparser.OpGreaterEq:
		return true
	}
	return false
}

// Conditions consume only truthiness. Keeping an entire Lua Value for the
// result of each and/or unnecessarily copies its kind, payload and reference
// through branch joins. A bool also needs no GC root while evaluating the
// right operand: the left operand's identity is no longer observable.
// Proven numeric comparisons stay Go comparisons: boxing them into BoolVal
// only to call Truthy() copies a Value through every if/while/and/or.
// Value-producing comparisons and and/or expressions still use emitBinary.
func (g *generator) emitCondition(expr luaparser.Expr) string {
	if g.opts.DisableConditionSpecialization {
		return "(" + g.emitExpr(expr) + ").Truthy()"
	}
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return g.emitCondition(x.Expr)
	case *luaparser.UnaryExpr:
		if x.Op == luaparser.OpNot {
			return "!(" + g.emitCondition(x.Expr) + ")"
		}
	case *luaparser.BinaryExpr:
		if x.Op == luaparser.OpAnd || x.Op == luaparser.OpOr {
			left := g.emitCondition(x.Left)
			result := g.next("condition")
			g.pending = append(g.pending, result+" := "+left)
			rightStmts, right := g.capturePending(func() string { return g.emitCondition(x.Right) })
			guard := result
			if x.Op == luaparser.OpOr {
				guard = "!" + guard
			}
			g.pending = append(g.pending, "if "+guard+" {")
			g.indentPending(rightStmts)
			g.pending = append(g.pending, "\t"+result+" = "+right, "}")
			return result
		}
		if comparisonOperator(x.Op) {
			return g.emitComparisonCondition(x)
		}
	}
	return "(" + g.emitExpr(expr) + ").Truthy()"
}

func (g *generator) emitComparisonCondition(x *luaparser.BinaryExpr) string {
	if g.exprNumber(x.Left) && g.exprNumber(x.Right) {
		parts := g.sequenceNumberExprs([]luaparser.Expr{x.Left, x.Right})
		return parts[0] + " " + string(x.Op) + " " + parts[1]
	}
	names := g.sequenceExprs([]luaparser.Expr{x.Left, x.Right})
	switch x.Op {
	case luaparser.OpEqual:
		return "c.ValuesEqual(" + names[0] + "," + names[1] + ")"
	case luaparser.OpNotEqual:
		return "!c.ValuesEqual(" + names[0] + "," + names[1] + ")"
	default:
		return "gluCmp(c," + names[0] + "," + names[1] + ",\"" + string(x.Op) + "\")"
	}
}
