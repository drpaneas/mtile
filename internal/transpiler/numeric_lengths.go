package transpiler

import (
	"slices"

	"github.com/drpaneas/mtile/internal/luaparser"
)

// A Lua numeric loop has a private numeric induction value, but its exposed
// local can be assigned any Value without affecting the next iteration.
// Retain that local's Value storage when its binding is reassigned. Nested
// declarations shadow it and do not invalidate the outer numeric binding.
func numericForVariableReassigned(loop *luaparser.NumericForStmt) bool {
	name := loop.Name
	var blockAssigned func(*luaparser.Block) bool
	blockAssigned = func(block *luaparser.Block) bool {
		if block == nil {
			return false
		}
		for _, stmt := range block.Body {
			switch s := stmt.(type) {
			case *luaparser.AssignStmt:
				for _, target := range s.Targets {
					if variable, ok := target.(*luaparser.VarExpr); ok && variable.Name == name {
						return true
					}
				}
			case *luaparser.LocalAssignStmt:
				if slices.Contains(s.Names, name) {
					return false // The remainder of this lexical block is shadowed.
				}
			case *luaparser.IfStmt:
				if blockAssigned(s.Then) || blockAssigned(s.Else) {
					return true
				}
				for _, branch := range s.ElseIf {
					if blockAssigned(branch.Body) {
						return true
					}
				}
			case *luaparser.DoStmt:
				if blockAssigned(s.Body) {
					return true
				}
			case *luaparser.WhileStmt:
				if blockAssigned(s.Body) {
					return true
				}
			case *luaparser.RepeatStmt:
				if blockAssigned(s.Body) {
					return true
				}
			case *luaparser.NumericForStmt:
				if s.Name != name && blockAssigned(s.Body) {
					return true
				}
			case *luaparser.GenericForStmt:
				if !slices.Contains(s.Names, name) && blockAssigned(s.Body) {
					return true
				}
			}
		}
		return false
	}
	return blockAssigned(loop.Body)
}

func numericForVariableReferenced(loop *luaparser.NumericForStmt) bool {
	return stmtsReferenceName(loop.Body.Body, loop.Name)
}

func stmtsReferenceName(stmts []luaparser.Stmt, name string) bool {
	for _, stmt := range stmts {
		if stmtReferencesName(stmt, name) {
			return true
		}
	}
	return false
}

func stmtReferencesName(stmt luaparser.Stmt, name string) bool {
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		return exprsReferenceName(s.Targets, name) || exprsReferenceName(s.Values, name)
	case *luaparser.LocalAssignStmt:
		return exprsReferenceName(s.Values, name)
	case *luaparser.CallStmt:
		return exprReferencesName(s.Call, name)
	case *luaparser.FunctionDeclStmt:
		return blockReferencesName(s.Body, name)
	case *luaparser.IfStmt:
		if exprReferencesName(s.Condition, name) || blockReferencesName(s.Then, name) || blockReferencesName(s.Else, name) {
			return true
		}
		for _, branch := range s.ElseIf {
			if exprReferencesName(branch.Condition, name) || blockReferencesName(branch.Body, name) {
				return true
			}
		}
	case *luaparser.WhileStmt:
		return exprReferencesName(s.Condition, name) || blockReferencesName(s.Body, name)
	case *luaparser.RepeatStmt:
		return blockReferencesName(s.Body, name) || exprReferencesName(s.Condition, name)
	case *luaparser.DoStmt:
		return blockReferencesName(s.Body, name)
	case *luaparser.NumericForStmt:
		return exprReferencesName(s.Init, name) || exprReferencesName(s.Limit, name) ||
			exprReferencesName(s.Step, name) || blockReferencesName(s.Body, name)
	case *luaparser.GenericForStmt:
		return exprsReferenceName(s.Values, name) || blockReferencesName(s.Body, name)
	case *luaparser.ReturnStmt:
		return exprsReferenceName(s.Values, name)
	}
	return false
}

func blockReferencesName(block *luaparser.Block, name string) bool {
	return block != nil && stmtsReferenceName(block.Body, name)
}

func exprsReferenceName(exprs []luaparser.Expr, name string) bool {
	for _, expr := range exprs {
		if exprReferencesName(expr, name) {
			return true
		}
	}
	return false
}

func exprReferencesName(expr luaparser.Expr, name string) bool {
	switch x := expr.(type) {
	case *luaparser.VarExpr:
		return x.Name == name
	case *luaparser.ParenExpr:
		return exprReferencesName(x.Expr, name)
	case *luaparser.UnaryExpr:
		return exprReferencesName(x.Expr, name)
	case *luaparser.BinaryExpr:
		return exprReferencesName(x.Left, name) || exprReferencesName(x.Right, name)
	case *luaparser.IndexExpr:
		return exprReferencesName(x.Base, name) || exprReferencesName(x.Key, name)
	case *luaparser.FieldExpr:
		return exprReferencesName(x.Base, name)
	case *luaparser.CallExpr:
		return exprReferencesName(x.Callee, name) || exprsReferenceName(x.Args, name)
	case *luaparser.TableExpr:
		for _, field := range x.Fields {
			if exprReferencesName(field.Key, name) || exprReferencesName(field.Value, name) {
				return true
			}
		}
	}
	return false
}
