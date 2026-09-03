package transpiler

import (
	"math"

	"github.com/drpaneas/mtile/internal/luaparser"
)

// analyzeNumericConstants removes storage for the deliberately narrow class
// of chunk-scope numeric bindings whose value cannot change. The existing
// sprite-ID write analysis gives us a conservative whole-program inventory of
// writes and shadowing. We additionally require the declaration to precede
// every function declaration and every direct top-level read, which preserves
// Lua's declaration/capture order without needing interprocedural execution
// analysis for chunk initialization.
func (g *generator) analyzeNumericConstants() {
	if g.opts.DisableMonomorphicNumbers || g.opts.DisableNumericConstants {
		return
	}
	analysis := newMode0SpriteIDAnalysis(g.chunk)
	for order, stmt := range g.chunk.Body {
		name, initializer, declaration := numericConstantDeclaration(stmt)
		if !declaration || g.globalKinds[name] != monoNumber {
			continue
		}
		writes := analysis.writes[name]
		if len(writes) != 1 || analysis.shadowed[name] || analysis.functionNames[name] {
			continue
		}
		write := writes[0]
		if !write.valid || !write.topLevel || write.order != order {
			continue
		}
		// A chunk-scope local declared after a function can change what that
		// earlier function captures. Applying the same boundary to globals also
		// excludes calls made during partially initialized chunk execution.
		if analysis.firstFunctionOrder >= 0 && order >= analysis.firstFunctionOrder {
			continue
		}
		if localOrder, local := analysis.topLocals[name]; local && localOrder != order {
			continue
		}
		if numericConstantReadBefore(g.chunk, name, order) {
			continue
		}
		value, ok := g.numericConstantValue(initializer, order)
		if !ok {
			continue
		}
		g.numericConstants[name] = &numericConstantInfo{
			declaration: stmt,
			value:       value,
			order:       order,
		}
	}
}

func numericConstantDeclaration(stmt luaparser.Stmt) (string, luaparser.Expr, bool) {
	switch declaration := stmt.(type) {
	case *luaparser.LocalAssignStmt:
		if len(declaration.Names) != 1 || len(declaration.Values) != 1 {
			return "", nil, false
		}
		return declaration.Names[0], declaration.Values[0], true
	case *luaparser.AssignStmt:
		if len(declaration.Targets) != 1 || len(declaration.Values) != 1 {
			return "", nil, false
		}
		variable, ok := declaration.Targets[0].(*luaparser.VarExpr)
		if !ok {
			return "", nil, false
		}
		return variable.Name, declaration.Values[0], true
	default:
		return "", nil, false
	}
}

func (g *generator) numericConstantValue(expr luaparser.Expr, declarationOrder int) (int32, bool) {
	if number, literal := staticNumberLiteral(expr); literal {
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return 0, false
		}
		return fixedNumberLiteral(number), true
	}
	variable, ok := unparenExpr(expr).(*luaparser.VarExpr)
	if !ok {
		return 0, false
	}
	constant := g.numericConstants[variable.Name]
	if constant == nil || constant.order >= declarationOrder {
		return 0, false
	}
	return constant.value, true
}

func numericConstantReadBefore(chunk *luaparser.Chunk, name string, declarationOrder int) bool {
	for order, stmt := range chunk.Body {
		if order >= declarationOrder {
			return false
		}
		// Function bodies execute later and all candidates are required to be
		// declared before the first function anyway.
		if _, function := stmt.(*luaparser.FunctionDeclStmt); function {
			continue
		}
		found := false
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			if variable, ok := expr.(*luaparser.VarExpr); ok && variable.Name == name {
				found = true
			}
		})
		if found {
			return true
		}
	}
	return false
}
