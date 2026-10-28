package transpiler

import "github.com/drpaneas/mtile/internal/luaparser"

// analyzeNoAllocFunctions proves a deliberately narrow transitive effect:
// the function and every user callee execute without allocating from the Lua
// heap. Such calls cannot trigger collection, so the caller's live Values do
// not need to be copied to the temporary root stack around the call. Starting
// from proven leaves means recursive/unknown call cycles remain protected.
func (g *generator) analyzeNoAllocFunctions() {
	if g.opts.DisableNoAllocFunctionCalls {
		return
	}
	for {
		changed := false
		for _, fn := range g.funcList {
			if fn.noAlloc || !g.blockNoAlloc(fn.decl.Body) {
				continue
			}
			fn.noAlloc = true
			changed = true
		}
		if !changed {
			return
		}
	}
}

func (g *generator) blockNoAlloc(block *luaparser.Block) bool {
	for _, stmt := range block.Body {
		if !g.stmtNoAlloc(stmt) {
			return false
		}
	}
	return true
}

func (g *generator) stmtNoAlloc(stmt luaparser.Stmt) bool {
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		for _, target := range s.Targets {
			switch target := target.(type) {
			case *luaparser.VarExpr:
			case *luaparser.FieldExpr:
				// A first field write may insert/grow a table even when later
				// executions hit a stable cached slot. Scalar-replaced records
				// have no table identity or allocation left to observe.
				if _, _, scalar := g.numericRecordField(target); !scalar {
					return false
				}
			case *luaparser.IndexExpr:
				return false
			default:
				return false
			}
		}
		return g.exprsNoAlloc(s.Values)
	case *luaparser.LocalAssignStmt:
		return g.exprsNoAlloc(s.Values)
	case *luaparser.CallStmt:
		return g.exprNoAlloc(s.Call)
	case *luaparser.IfStmt:
		if !g.exprNoAlloc(s.Condition) || !g.blockNoAlloc(s.Then) {
			return false
		}
		for _, branch := range s.ElseIf {
			if !g.exprNoAlloc(branch.Condition) || !g.blockNoAlloc(branch.Body) {
				return false
			}
		}
		return s.Else == nil || g.blockNoAlloc(s.Else)
	case *luaparser.WhileStmt:
		return g.exprNoAlloc(s.Condition) && g.blockNoAlloc(s.Body)
	case *luaparser.RepeatStmt:
		return g.blockNoAlloc(s.Body) && g.exprNoAlloc(s.Condition)
	case *luaparser.DoStmt:
		return g.blockNoAlloc(s.Body)
	case *luaparser.NumericForStmt:
		return g.exprNoAlloc(s.Init) && g.exprNoAlloc(s.Limit) && g.exprNoAlloc(s.Step) && g.blockNoAlloc(s.Body)
	case *luaparser.GenericForStmt:
		return g.exprsNoAlloc(s.Values) && g.blockNoAlloc(s.Body)
	case *luaparser.ReturnStmt:
		return g.exprsNoAlloc(s.Values)
	case *luaparser.BreakStmt:
		return true
	}
	return false
}

// requiresHeap is intentionally stricter than merely finding a table literal.
// It reuses the transitive no-allocation proof and accounts only for the table
// forms that earlier passes completely scalar-replaced. Boxed globals can hold
// static strings, booleans and nil without a heap; they only need GC roots when
// some part of this closed program can allocate a heap value.
func (g *generator) requiresHeap() bool {
	for _, fn := range g.funcList {
		if !fn.noAlloc {
			return true
		}
	}
	for _, stmt := range g.chunk.Body {
		if _, declaration := stmt.(*luaparser.FunctionDeclStmt); declaration {
			continue
		}
		name, _, recordDeclaration := numericRecordDeclaration(stmt)
		if recordDeclaration {
			if info := g.numericRecords[name]; info != nil && info.declaration == stmt {
				continue
			}
		}
		if !g.stmtNoAlloc(stmt) {
			return true
		}
	}
	return false
}

func (g *generator) exprsNoAlloc(exprs []luaparser.Expr) bool {
	for _, expr := range exprs {
		if !g.exprNoAlloc(expr) {
			return false
		}
	}
	return true
}

func (g *generator) exprNoAlloc(expr luaparser.Expr) bool {
	switch x := expr.(type) {
	case nil, *luaparser.AnalysisChoiceExpr, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.NumberExpr,
		*luaparser.StringExpr, *luaparser.VarExpr:
		return true
	case *luaparser.ParenExpr:
		return g.exprNoAlloc(x.Expr)
	case *luaparser.UnaryExpr:
		return g.exprNoAlloc(x.Expr)
	case *luaparser.BinaryExpr:
		return x.Op != luaparser.OpConcat && g.exprNoAlloc(x.Left) && g.exprNoAlloc(x.Right)
	case *luaparser.IndexExpr:
		return g.exprNoAlloc(x.Base) && g.exprNoAlloc(x.Key)
	case *luaparser.FieldExpr:
		return g.exprNoAlloc(x.Base)
	case *luaparser.CallExpr:
		if !g.exprsNoAlloc(operandExprs(x)) {
			return false
		}
		if fn := g.calledFunction(x); fn != nil {
			return fn.noAlloc
		}
		name, ok := callKey(x)
		if !ok {
			return false
		}
		if name == "print" && !g.opts.DisableLiteralPrintSpecialization {
			// PrintV converts non-string values through Tostr, which allocates
			// from the Lua heap. A literal is interned as KindStaticString at
			// initialization and follows the allocation-free drawing path.
			// Mode 0 numbers use formatNumber on a stack buffer instead.
			if len(x.Args) == 0 {
				return false
			}
			if _, literal := unparenExpr(x.Args[0]).(*luaparser.StringExpr); literal {
				return true
			}
			return g.mode0HeaplessPrintNumber(x.Args[0])
		}
		return builtinNoAlloc(name)
	case *luaparser.TableExpr:
		return false
	}
	return false
}

// This allowlist is about custom Lua-heap allocation, not general side
// effects. Calls omitted here simply retain root protection.
func builtinNoAlloc(name string) bool {
	switch name {
	case "pause_menu", "save_get", "save_set", "save_read", "save_write", "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd", "srand",
		"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr",
		"btn", "btnp", "t", "time", "stat", "tonum", "ord", "printh",
		"sfx", "music", "sample", "gba_note", "gba_stop",
		"del", "deli", "count", "all", "pairs",
		"sget", "sset", "fget", "fset", "mget", "mset",
		"cls", "pset", "pget", "line", "rect", "rectfill", "circ", "circfill", "spr", "sspr", "pspr", "anim", "map",
		"camera", "clip", "pal", "palt", "pvariant", "color", "cursor":
		return true
	}
	return false
}
