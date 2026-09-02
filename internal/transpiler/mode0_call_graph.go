package transpiler

import "github.com/drpaneas/mtile/internal/luaparser"

// mode0CallGraph provides the small amount of transitive call information
// needed by the Mode 0 proof. It is deliberately conservative: every
// declaration with a matching function key is considered reachable.
type mode0CallGraph struct {
	functions map[string][]*luaparser.FunctionDeclStmt
}

func newMode0CallGraph(chunk *luaparser.Chunk) *mode0CallGraph {
	graph := &mode0CallGraph{functions: make(map[string][]*luaparser.FunctionDeclStmt)}
	if chunk == nil {
		return graph
	}
	var visit func(luaparser.Stmt)
	visit = func(stmt luaparser.Stmt) {
		fn, ok := stmt.(*luaparser.FunctionDeclStmt)
		if !ok {
			return
		}
		key := functionKey(fn)
		graph.functions[key] = append(graph.functions[key], fn)
		for _, bodyStmt := range fn.Body.Body {
			visit(bodyStmt)
		}
	}
	for _, stmt := range chunk.Body {
		visit(stmt)
	}
	return graph
}

func (g *mode0CallGraph) firstBuiltinFromFunction(function string, targets ...string) *luaparser.CallExpr {
	if g == nil {
		return nil
	}
	targetSet := make(map[string]bool, len(targets))
	for _, target := range targets {
		targetSet[target] = true
	}
	visiting := make(map[*luaparser.FunctionDeclStmt]bool)
	for _, fn := range g.functions[function] {
		if call := g.firstBuiltinFromBlock(fn.Body, targetSet, visiting); call != nil {
			return call
		}
	}
	return nil
}

func (g *mode0CallGraph) firstBuiltinFromStmt(stmt luaparser.Stmt, target string) *luaparser.CallExpr {
	if g == nil || stmt == nil {
		return nil
	}
	targets := map[string]bool{target: true}
	visiting := make(map[*luaparser.FunctionDeclStmt]bool)
	return g.firstBuiltinInStmt(stmt, targets, visiting)
}

func (g *mode0CallGraph) firstBuiltinFromBlock(block *luaparser.Block, targets map[string]bool,
	visiting map[*luaparser.FunctionDeclStmt]bool,
) *luaparser.CallExpr {
	if block == nil {
		return nil
	}
	for _, stmt := range block.Body {
		if call := g.firstBuiltinInStmt(stmt, targets, visiting); call != nil {
			return call
		}
	}
	return nil
}

func (g *mode0CallGraph) firstBuiltinInStmt(stmt luaparser.Stmt, targets map[string]bool,
	visiting map[*luaparser.FunctionDeclStmt]bool,
) *luaparser.CallExpr {
	var found *luaparser.CallExpr
	collectStmtExprs(stmt, func(expr luaparser.Expr) {
		if found != nil {
			return
		}
		call, ok := expr.(*luaparser.CallExpr)
		if !ok {
			return
		}
		name, ok := callKey(call)
		if !ok {
			return
		}
		if targets[name] {
			found = call
			return
		}
		// Code generation gives public builtins precedence over a user
		// declaration with the same key. The proof must resolve calls the same
		// way; following the unreachable user body can otherwise invent a
		// camera/palette side effect and reject an exact Mode 0 program.
		if _, builtin := Builtins[name]; builtin {
			return
		}
		for _, fn := range g.functions[name] {
			if visiting[fn] {
				continue
			}
			visiting[fn] = true
			found = g.firstBuiltinFromBlock(fn.Body, targets, visiting)
			delete(visiting, fn)
			if found != nil {
				return
			}
		}
	})
	return found
}
