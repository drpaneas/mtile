package transpiler

import (
	"slices"

	"github.com/drpaneas/mtile/internal/luaparser"
)

func (fn *functionInfo) coercesNumberParam(index int) bool {
	return index < len(fn.coercedNumberParams) && fn.coercedNumberParams[index]
}

// A no-allocation function can take NumberOr(0) at its call boundary when
// every use of a parameter performs that exact coercion anyway. Keep the
// original argument sequencer: an arbitrary heap value remains rooted while
// later arguments evaluate. The callee cannot allocate, so narrowing its ABI
// does not shorten any root lifetime across a collection or change OOM paths.
//
// This is deliberately demand-based, not arithmetic type guessing. Raw
// arithmetic, truth tests, aliases, writes and shadow declarations reject the
// candidate. Forwarding only to already proved coercing parameters permits
// the ordinary signature fixed point to propagate through helper chains.
func (g *generator) parameterAlwaysCoerced(fn *functionInfo, index int) bool {
	if g.opts.DisableCoercedNumberParams || !fn.noAlloc {
		return false
	}
	if fn.coercesNumberParam(index) {
		return true
	}
	name := fn.params[index]
	seen := false
	var expression func(luaparser.Expr, bool) bool
	expression = func(expr luaparser.Expr, demand bool) bool {
		switch x := expr.(type) {
		case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.NumberExpr, *luaparser.StringExpr:
			return true
		case *luaparser.VarExpr:
			if x.Name != name {
				return true
			}
			seen = true
			return demand
		case *luaparser.ParenExpr:
			return expression(x.Expr, demand)
		case *luaparser.UnaryExpr:
			return expression(x.Expr, false)
		case *luaparser.BinaryExpr:
			return expression(x.Left, false) && expression(x.Right, false)
		case *luaparser.FieldExpr:
			return expression(x.Base, false)
		case *luaparser.IndexExpr:
			return expression(x.Base, false) && expression(x.Key, false)
		case *luaparser.TableExpr:
			for _, field := range x.Fields {
				if !expression(field.Key, false) || !expression(field.Value, false) {
					return false
				}
			}
			return true
		case *luaparser.CallExpr:
			callee := g.calledFunction(x)
			key, known := callKey(x)
			coercingBuiltin := callee == nil && known && (key == "min" || key == "max" || key == "mid")
			for position, arg := range operandExprs(x) {
				coerces := coercingBuiltin || callee != nil && callee.coercesNumberParam(position)
				if !expression(arg, coerces) {
					return false
				}
			}
			return true
		}
		return false
	}
	var block func(*luaparser.Block) bool
	block = func(body *luaparser.Block) bool {
		if body == nil {
			return true
		}
		for _, stmt := range body.Body {
			switch s := stmt.(type) {
			case *luaparser.AssignStmt:
				for _, target := range s.Targets {
					if !expression(target, false) {
						return false
					}
				}
				for _, value := range s.Values {
					if !expression(value, false) {
						return false
					}
				}
			case *luaparser.LocalAssignStmt:
				if slices.Contains(s.Names, name) {
					return false
				}
				for _, value := range s.Values {
					if !expression(value, false) {
						return false
					}
				}
			case *luaparser.CallStmt:
				if !expression(s.Call, false) {
					return false
				}
			case *luaparser.IfStmt:
				if !expression(s.Condition, false) || !block(s.Then) || !block(s.Else) {
					return false
				}
				for _, branch := range s.ElseIf {
					if !expression(branch.Condition, false) || !block(branch.Body) {
						return false
					}
				}
			case *luaparser.DoStmt:
				if !block(s.Body) {
					return false
				}
			case *luaparser.WhileStmt:
				if !expression(s.Condition, false) || !block(s.Body) {
					return false
				}
			case *luaparser.RepeatStmt:
				if !block(s.Body) || !expression(s.Condition, false) {
					return false
				}
			case *luaparser.NumericForStmt:
				if s.Name == name || !expression(s.Init, false) || !expression(s.Limit, false) || !expression(s.Step, false) || !block(s.Body) {
					return false
				}
			case *luaparser.GenericForStmt:
				if slices.Contains(s.Names, name) || !block(s.Body) {
					return false
				}
				for _, value := range s.Values {
					if !expression(value, false) {
						return false
					}
				}
			case *luaparser.ReturnStmt:
				for _, value := range s.Values {
					if !expression(value, false) {
						return false
					}
				}
			case *luaparser.BreakStmt:
			default:
				return false
			}
		}
		return true
	}
	return block(fn.decl.Body) && seen
}
