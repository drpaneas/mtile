package transpiler

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

func (g *generator) discoverGlobals() {
	g.collectChunkGlobals()
	for _, info := range g.funcList {
		g.current = info
		g.collectBlockGlobals(info.decl.Body)
	}
	g.current = nil
}

// collectChunkGlobals mirrors chunk code generation: only locals declared
// directly in the chunk become persistent Game fields. Locals inside a
// top-level do/if/loop are ordinary lexical stack values and must not inflate
// the global report or consume runtime global-root slots.
func (g *generator) collectChunkGlobals() {
	scopes := []map[string]bool{}
	isLocal := func(name string) bool {
		for i := len(scopes) - 1; i >= 0; i-- {
			if scopes[i][name] {
				return true
			}
		}
		return false
	}
	add := func(name string, position luaparser.Position) {
		if !isLocal(name) {
			g.addGlobal(name, position)
		}
	}
	var visitBlock func(*luaparser.Block, bool)
	var visitStmt func(luaparser.Stmt)
	visitBlock = func(block *luaparser.Block, newScope bool) {
		if newScope {
			scopes = append(scopes, map[string]bool{})
		}
		for _, stmt := range block.Body {
			visitStmt(stmt)
		}
		if newScope {
			scopes = scopes[:len(scopes)-1]
		}
	}
	visitStmt = func(stmt luaparser.Stmt) {
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			collectStmt(s, add, isLocal, g.funcs, Builtins)
		case *luaparser.LocalAssignStmt:
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
			if len(scopes) > 0 {
				for _, name := range s.Names {
					scopes[len(scopes)-1][name] = true
				}
			}
		case *luaparser.CallStmt:
			collectExpr(s.Call, add, isLocal, g.funcs, Builtins)
		case *luaparser.IfStmt:
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			visitBlock(s.Then, true)
			for _, branch := range s.ElseIf {
				collectExpr(branch.Condition, add, isLocal, g.funcs, Builtins)
				visitBlock(branch.Body, true)
			}
			if s.Else != nil {
				visitBlock(s.Else, true)
			}
		case *luaparser.WhileStmt:
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			visitBlock(s.Body, true)
		case *luaparser.RepeatStmt:
			scopes = append(scopes, map[string]bool{})
			visitBlock(s.Body, false)
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.DoStmt:
			visitBlock(s.Body, true)
		case *luaparser.NumericForStmt:
			collectExpr(s.Init, add, isLocal, g.funcs, Builtins)
			collectExpr(s.Limit, add, isLocal, g.funcs, Builtins)
			collectExpr(s.Step, add, isLocal, g.funcs, Builtins)
			scopes = append(scopes, map[string]bool{s.Name: true})
			visitBlock(s.Body, true)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.GenericForStmt:
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
			bindings := map[string]bool{}
			for _, name := range s.Names {
				bindings[name] = true
			}
			scopes = append(scopes, bindings)
			visitBlock(s.Body, true)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.ReturnStmt:
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
		}
	}
	for _, stmt := range g.chunk.Body {
		if _, function := stmt.(*luaparser.FunctionDeclStmt); function {
			continue
		}
		if local, ok := stmt.(*luaparser.LocalAssignStmt); ok {
			for _, value := range local.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
			for _, name := range local.Names {
				g.addGlobal(name, local.Pos())
			}
			continue
		}
		visitStmt(stmt)
	}
}

func (g *generator) globalReport() GlobalReport {
	report := GlobalReport{Count: len(g.globals), Limit: GlobalLimit, Symbols: make([]GlobalSymbol, 0, len(g.globals))}
	for _, name := range g.globals {
		report.Symbols = append(report.Symbols, GlobalSymbol{Name: name, Position: g.globalOrigins[name]})
	}
	return report
}

func (g *generator) addGlobal(name string, position luaparser.Position) {
	if name == "" {
		return
	}
	if _, ok := g.globalGo[name]; ok {
		return
	}
	g.globalGo[name] = goIdent(name)
	g.globalOrigins[name] = position
	g.globals = append(g.globals, name)
}

func (g *generator) collectBlockGlobals(block *luaparser.Block) {
	// Global discovery must use the same lexical scopes as code emission. A
	// name introduced in a nested block is not local before or after that
	// block, and a local declaration is not visible in its own RHS. The old
	// whole-function name set misclassified both cases and could emit an
	// assignment to rt.NilVal().
	scopes := []map[string]bool{{}}
	for _, name := range g.current.params {
		scopes[0][name] = true
	}
	isLocal := func(name string) bool {
		for i := len(scopes) - 1; i >= 0; i-- {
			if scopes[i][name] {
				return true
			}
		}
		return false
	}
	add := func(name string, position luaparser.Position) {
		if !isLocal(name) {
			g.addGlobal(name, position)
		}
	}
	var visitBlock func(*luaparser.Block, bool)
	var visitStmt func(luaparser.Stmt)
	visitBlock = func(b *luaparser.Block, newScope bool) {
		if newScope {
			scopes = append(scopes, map[string]bool{})
		}
		for _, stmt := range b.Body {
			visitStmt(stmt)
		}
		if newScope {
			scopes = scopes[:len(scopes)-1]
		}
	}
	visitStmt = func(stmt luaparser.Stmt) {
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			for _, target := range s.Targets {
				collectExpr(target, add, isLocal, g.funcs, Builtins)
			}
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
		case *luaparser.LocalAssignStmt:
			// The new names become visible only after all RHS expressions.
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
			for _, name := range s.Names {
				scopes[len(scopes)-1][name] = true
			}
		case *luaparser.CallStmt:
			collectExpr(s.Call, add, isLocal, g.funcs, Builtins)
		case *luaparser.IfStmt:
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			visitBlock(s.Then, true)
			for _, branch := range s.ElseIf {
				collectExpr(branch.Condition, add, isLocal, g.funcs, Builtins)
				visitBlock(branch.Body, true)
			}
			if s.Else != nil {
				visitBlock(s.Else, true)
			}
		case *luaparser.WhileStmt:
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			visitBlock(s.Body, true)
		case *luaparser.RepeatStmt:
			// Lua evaluates repeat's condition in the body scope.
			scopes = append(scopes, map[string]bool{})
			for _, bodyStmt := range s.Body.Body {
				visitStmt(bodyStmt)
			}
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.DoStmt:
			visitBlock(s.Body, true)
		case *luaparser.NumericForStmt:
			collectExpr(s.Init, add, isLocal, g.funcs, Builtins)
			collectExpr(s.Limit, add, isLocal, g.funcs, Builtins)
			collectExpr(s.Step, add, isLocal, g.funcs, Builtins)
			scopes = append(scopes, map[string]bool{s.Name: true})
			visitBlock(s.Body, true)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.GenericForStmt:
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
			loopScope := map[string]bool{}
			for _, name := range s.Names {
				loopScope[name] = true
			}
			scopes = append(scopes, loopScope)
			visitBlock(s.Body, true)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.ReturnStmt:
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
		}
	}
	visitBlock(block, false)
}

func (g *generator) collectStmtStatics(stmt luaparser.Stmt) {
	var clipNames map[*luaparser.StringExpr]bool
	collectStmtExprs(stmt, func(expr luaparser.Expr) {
		if call, ok := expr.(*luaparser.CallExpr); ok {
			if name, _ := callKey(call); name == "anim" {
				if _, literal := staticAnimationID(call.Args, g.opts); literal {
					if clipNames == nil {
						clipNames = make(map[*luaparser.StringExpr]bool)
					}
					clipNames[unparenExpr(call.Args[0]).(*luaparser.StringExpr)] = true
				}
			}
		}
		if str, ok := expr.(*luaparser.StringExpr); ok && clipNames[str] {
			return
		}
		g.collectExprStatic(expr)
	})
}

func (g *generator) collectBlockStatics(block *luaparser.Block) {
	for _, stmt := range block.Body {
		g.collectStmtStatics(stmt)
	}
}

func (g *generator) collectExprStatic(e luaparser.Expr) {
	switch x := e.(type) {
	case *luaparser.StringExpr:
		if g.numericRecordKeys[x] {
			return
		}
		g.staticValueAt(x.Value, x.Pos())
	case *luaparser.FieldExpr:
		if _, _, ok := g.numericRecordField(x); ok {
			return
		}
		if !g.opts.DisableStableTableFields {
			if _, exists := g.fieldCaches[x]; !exists {
				g.fieldCaches[x] = g.fieldCacheN
				g.fieldCacheN++
			}
		}
		g.staticValueAt(x.Name, x.Pos())
	}
}

func (g *generator) staticValue(s string) string {
	return g.staticValueAt(s, luaparser.Position{})
}

func (g *generator) staticValueAt(s string, position luaparser.Position) string {
	if name, ok := g.staticGo[s]; ok {
		return name
	}
	where := ""
	if position.Line != 0 {
		where = " at " + position.String()
	}
	if len(s) > runtimeapi.MaxStringBytes {
		g.setErr(fmt.Errorf("transpiler: static string has %d bytes, runtime limit is %d bytes%s", len(s), runtimeapi.MaxStringBytes, where))
		return ""
	}
	if len(g.statics) >= runtimeapi.MaxStaticStrings {
		g.setErr(fmt.Errorf("transpiler: cartridge requires more than %d static strings%s", runtimeapi.MaxStaticStrings, where))
		return ""
	}
	name := fmt.Sprintf("_gluStatic%d", len(g.statics))
	g.staticGo[s] = name
	g.statics = append(g.statics, s)
	return name
}

// collectStmt and collectBlock are deliberately separate from codegen. This
// means global discovery does not depend on traversal order or on whether a
// value is used as an l-value.
func collectBlock(block *luaparser.Block, add func(string, luaparser.Position), isLocal func(string) bool, funcs map[string]*functionInfo, builtins map[string]BuiltinSpec) {
	for _, stmt := range block.Body {
		collectStmt(stmt, add, isLocal, funcs, builtins)
	}
}

func collectStmt(stmt luaparser.Stmt, add func(string, luaparser.Position), isLocal func(string) bool, funcs map[string]*functionInfo, builtins map[string]BuiltinSpec) {
	addTarget := func(e luaparser.Expr) {
		switch x := e.(type) {
		case *luaparser.VarExpr:
			if !isLocal(x.Name) {
				add(x.Name, x.Pos())
			}
		case *luaparser.FieldExpr:
			collectExpr(x.Base, add, isLocal, funcs, builtins)
		case *luaparser.IndexExpr:
			collectExpr(x.Base, add, isLocal, funcs, builtins)
			collectExpr(x.Key, add, isLocal, funcs, builtins)
		}
	}
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		for _, t := range s.Targets {
			addTarget(t)
		}
		for _, v := range s.Values {
			collectExpr(v, add, isLocal, funcs, builtins)
		}
	case *luaparser.LocalAssignStmt:
		for _, v := range s.Values {
			collectExpr(v, add, isLocal, funcs, builtins)
		}
	case *luaparser.CallStmt:
		collectExpr(s.Call, add, isLocal, funcs, builtins)
	case *luaparser.IfStmt:
		collectExpr(s.Condition, add, isLocal, funcs, builtins)
		collectBlock(s.Then, add, isLocal, funcs, builtins)
		for _, b := range s.ElseIf {
			collectExpr(b.Condition, add, isLocal, funcs, builtins)
			collectBlock(b.Body, add, isLocal, funcs, builtins)
		}
		if s.Else != nil {
			collectBlock(s.Else, add, isLocal, funcs, builtins)
		}
	case *luaparser.WhileStmt:
		collectExpr(s.Condition, add, isLocal, funcs, builtins)
		collectBlock(s.Body, add, isLocal, funcs, builtins)
	case *luaparser.RepeatStmt:
		collectBlock(s.Body, add, isLocal, funcs, builtins)
		collectExpr(s.Condition, add, isLocal, funcs, builtins)
	case *luaparser.DoStmt:
		collectBlock(s.Body, add, isLocal, funcs, builtins)
	case *luaparser.NumericForStmt:
		collectExpr(s.Init, add, isLocal, funcs, builtins)
		collectExpr(s.Limit, add, isLocal, funcs, builtins)
		collectExpr(s.Step, add, isLocal, funcs, builtins)
		collectBlock(s.Body, add, isLocal, funcs, builtins)
	case *luaparser.GenericForStmt:
		for _, v := range s.Values {
			collectExpr(v, add, isLocal, funcs, builtins)
		}
		collectBlock(s.Body, add, isLocal, funcs, builtins)
	case *luaparser.ReturnStmt:
		for _, v := range s.Values {
			collectExpr(v, add, isLocal, funcs, builtins)
		}
	}
}

func collectExpr(e luaparser.Expr, add func(string, luaparser.Position), isLocal func(string) bool, funcs map[string]*functionInfo, builtins map[string]BuiltinSpec) {
	switch x := e.(type) {
	case *luaparser.ParenExpr:
		collectExpr(x.Expr, add, isLocal, funcs, builtins)
	case *luaparser.VarExpr:
		if !isLocal(x.Name) {
			if _, fn := funcs[x.Name]; !fn {
				if _, bi := builtins[x.Name]; !bi {
					add(x.Name, x.Pos())
				}
			}
		}
	case *luaparser.UnaryExpr:
		collectExpr(x.Expr, add, isLocal, funcs, builtins)
	case *luaparser.BinaryExpr:
		collectExpr(x.Left, add, isLocal, funcs, builtins)
		collectExpr(x.Right, add, isLocal, funcs, builtins)
	case *luaparser.IndexExpr:
		collectExpr(x.Base, add, isLocal, funcs, builtins)
		collectExpr(x.Key, add, isLocal, funcs, builtins)
	case *luaparser.FieldExpr:
		collectExpr(x.Base, add, isLocal, funcs, builtins)
	case *luaparser.CallExpr:
		if v, ok := x.Callee.(*luaparser.VarExpr); ok {
			if _, bi := builtins[v.Name]; !bi {
				if _, fn := funcs[v.Name]; !fn {
					if !isLocal(v.Name) {
						add(v.Name, v.Pos())
					}
				}
			}
		} else {
			collectExpr(x.Callee, add, isLocal, funcs, builtins)
		}
		for _, a := range x.Args {
			collectExpr(a, add, isLocal, funcs, builtins)
		}
	case *luaparser.TableExpr:
		for _, f := range x.Fields {
			if f.Key != nil {
				collectExpr(f.Key, add, isLocal, funcs, builtins)
			}
			collectExpr(f.Value, add, isLocal, funcs, builtins)
		}
	}
}

func collectStmtExprs(stmt luaparser.Stmt, add func(luaparser.Expr)) {
	var expr func(luaparser.Expr)
	expr = func(e luaparser.Expr) {
		if e == nil {
			return
		}
		add(e)
		switch x := e.(type) {
		case *luaparser.ParenExpr:
			expr(x.Expr)
		case *luaparser.UnaryExpr:
			expr(x.Expr)
		case *luaparser.BinaryExpr:
			expr(x.Left)
			expr(x.Right)
		case *luaparser.IndexExpr:
			expr(x.Base)
			expr(x.Key)
		case *luaparser.FieldExpr:
			expr(x.Base)
		case *luaparser.CallExpr:
			expr(x.Callee)
			for _, a := range x.Args {
				expr(a)
			}
		case *luaparser.TableExpr:
			for _, f := range x.Fields {
				expr(f.Key)
				expr(f.Value)
			}
		}
	}
	var stmtExpr func(luaparser.Stmt)
	stmtExpr = func(s luaparser.Stmt) {
		switch x := s.(type) {
		case *luaparser.AssignStmt:
			for _, t := range x.Targets {
				expr(t)
			}
			for _, v := range x.Values {
				expr(v)
			}
		case *luaparser.LocalAssignStmt:
			for _, v := range x.Values {
				expr(v)
			}
		case *luaparser.CallStmt:
			expr(x.Call)
		case *luaparser.IfStmt:
			expr(x.Condition)
			for _, b := range x.Then.Body {
				stmtExpr(b)
			}
			for _, b := range x.ElseIf {
				expr(b.Condition)
				for _, q := range b.Body.Body {
					stmtExpr(q)
				}
			}
			if x.Else != nil {
				for _, q := range x.Else.Body {
					stmtExpr(q)
				}
			}
		case *luaparser.WhileStmt:
			expr(x.Condition)
			for _, q := range x.Body.Body {
				stmtExpr(q)
			}
		case *luaparser.RepeatStmt:
			for _, q := range x.Body.Body {
				stmtExpr(q)
			}
			expr(x.Condition)
		case *luaparser.DoStmt:
			for _, q := range x.Body.Body {
				stmtExpr(q)
			}
		case *luaparser.NumericForStmt:
			expr(x.Init)
			expr(x.Limit)
			expr(x.Step)
			for _, q := range x.Body.Body {
				stmtExpr(q)
			}
		case *luaparser.GenericForStmt:
			for _, v := range x.Values {
				expr(v)
			}
			for _, q := range x.Body.Body {
				stmtExpr(q)
			}
		case *luaparser.ReturnStmt:
			for _, v := range x.Values {
				expr(v)
			}
		}
	}
	stmtExpr(stmt)
}
