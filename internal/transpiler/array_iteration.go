package transpiler

import "github.com/drpaneas/mtile/internal/luaparser"

// iteratorLengthStable proves that the loop and all its callees leave every
// Lua array unchanged. Rejecting all indexed stores avoids an alias analysis:
// even a store through another name cannot change the captured iterator's
// dense prefix. Named-field writes only change string keys; rebinding a local
// or global does not replace the already captured iterator table. TableGetInt
// remains in the loop and the iterator stays rooted, so GC may move its storage.
func (g *generator) iteratorLengthStable(body *luaparser.Block) bool {
	if g.opts.DisableIteratorLengthHoisting {
		return false
	}
	// Unknown/recursive calls fail closed. A fresh map per loop makes the
	// proof independent of the order in which functions are emitted.
	state := make(map[*functionInfo]uint8)
	var stable func(*luaparser.Block) bool
	stable = func(block *luaparser.Block) bool {
		if block == nil {
			return true
		}
		if blockHasIndexedStore(block) {
			return false
		}
		ok := true
		for _, stmt := range block.Body {
			collectStmtExprs(stmt, func(expr luaparser.Expr) {
				call, isCall := expr.(*luaparser.CallExpr)
				if !ok || !isCall {
					return
				}
				if fn := g.calledFunction(call); fn != nil {
					switch state[fn] {
					case 1, 3:
						ok = false
					case 0:
						state[fn] = 1
						ok = stable(fn.decl.Body)
						state[fn] = 3
						if ok {
							state[fn] = 2
						}
					}
					return
				}
				name, known := callKey(call)
				ok = known && builtinKeepsArrayLengths(name)
			})
			if !ok {
				return false
			}
		}
		return true
	}
	return stable(body)
}

func blockHasIndexedStore(block *luaparser.Block) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.Body {
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			for _, target := range s.Targets {
				if _, indexed := target.(*luaparser.IndexExpr); indexed {
					return true
				}
			}
		case *luaparser.IfStmt:
			if blockHasIndexedStore(s.Then) || blockHasIndexedStore(s.Else) {
				return true
			}
			for _, branch := range s.ElseIf {
				if blockHasIndexedStore(branch.Body) {
					return true
				}
			}
		case *luaparser.DoStmt:
			if blockHasIndexedStore(s.Body) {
				return true
			}
		case *luaparser.WhileStmt:
			if blockHasIndexedStore(s.Body) {
				return true
			}
		case *luaparser.RepeatStmt:
			if blockHasIndexedStore(s.Body) {
				return true
			}
		case *luaparser.NumericForStmt:
			if blockHasIndexedStore(s.Body) {
				return true
			}
		case *luaparser.GenericForStmt:
			if blockHasIndexedStore(s.Body) {
				return true
			}
		case *luaparser.LocalAssignStmt, *luaparser.CallStmt, *luaparser.ReturnStmt, *luaparser.BreakStmt:
		default:
			return true
		}
	}
	return false
}

// This is deliberately an allowlist. A future builtin must prove this effect
// before it can participate; add/del/deli and foreach retain live length reads.
func builtinKeepsArrayLengths(name string) bool {
	switch name {
	case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd", "srand",
		"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr",
		"btn", "btnp", "t", "time", "stat", "tonum", "ord", "printh", "sub", "tostr", "chr",
		"sfx", "music", "sample", "gba_note", "gba_stop", "count", "all", "pairs",
		"sget", "sset", "fget", "fset", "mget", "mset",
		"cls", "pset", "pget", "line", "rect", "rectfill", "circ", "circfill", "spr", "sspr", "pspr", "anim", "map", "print",
		"camera", "clip", "pal", "palt", "color", "cursor":
		return true
	}
	return false
}
