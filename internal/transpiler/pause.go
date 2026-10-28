package transpiler

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/luaparser"
)

type pauseCallbacks struct {
	save, load string
	call       *luaparser.CallExpr
}

func findPauseCallbacks(chunk *luaparser.Chunk) (pauseCallbacks, error) {
	var binding pauseCallbacks
	allowed := map[*luaparser.CallExpr]bool{}
	functions := map[string]*luaparser.FunctionDeclStmt{}
	for _, stmt := range chunk.Body {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			functions[functionKey(fn)] = fn
			if functionKey(fn) == "_init" {
				for _, s := range fn.Body.Body {
					if cs, ok := s.(*luaparser.CallStmt); ok {
						allowed[cs.Call] = true
					}
				}
			}
		}
	}
	var first error
	mode0EachStmt(chunk, func(stmt luaparser.Stmt) {
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			call, ok := expr.(*luaparser.CallExpr)
			if !ok || first != nil {
				return
			}
			name, ok := callKey(call)
			if !ok || name != "pause_menu" {
				return
			}
			if binding.call == call {
				return
			}
			fail := func(reason string) { first = fmt.Errorf("transpiler: %s: [PAUSE_MENU] %s", call.Pos(), reason) }
			if !allowed[call] || binding.call != nil || len(call.Args) != 2 {
				fail("pause_menu requires one direct _init registration with two literal function names")
				return
			}
			names := [2]string{}
			for i, arg := range call.Args {
				literal, ok := unparenExpr(arg).(*luaparser.StringExpr)
				if !ok {
					fail("pause_menu callback names must be literal strings")
					return
				}
				fn := functions[literal.Value]
				if fn == nil || len(fn.Params) != 0 || fn.Method != "" || fn.Vararg || literal.Value == "_init" || literal.Value == "_update" || literal.Value == "_draw" {
					fail("pause_menu callbacks must name zero-argument functions")
					return
				}
				names[i] = literal.Value
			}
			binding = pauseCallbacks{save: names[0], load: names[1], call: call}
		})
	})
	return binding, first
}

// During renderer analysis only, treat the callbacks as additional update
// roots. This makes all existing mutation, finite-ID, call-graph, layer and
// resource proofs see their effects; no hidden graphics path bypasses AUTO.
func lowerPauseAnalysisRoots(chunk *luaparser.Chunk) (func(), map[luaparser.Stmt]bool, error) {
	binding, err := findPauseCallbacks(chunk)
	if err != nil || binding.call == nil {
		return func() {}, nil, err
	}
	var update *luaparser.FunctionDeclStmt
	for _, stmt := range chunk.Body {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok && functionKey(fn) == "_update" {
			update = fn
		}
	}
	oldChunk := chunk.Body
	if update == nil {
		update = &luaparser.FunctionDeclStmt{Name: []string{"_update"}, Body: &luaparser.Block{}}
		chunk.Body = append(append([]luaparser.Stmt(nil), chunk.Body...), update)
	}
	oldBody := update.Body.Body
	synthetic := map[luaparser.Stmt]bool{}
	node := luaparser.NodeBase{At: binding.call.Pos()}
	callBlock := func(name string) *luaparser.Block {
		return &luaparser.Block{Body: []luaparser.Stmt{&luaparser.CallStmt{NodeBase: node, Call: &luaparser.CallExpr{NodeBase: node, Callee: &luaparser.VarExpr{NodeBase: node, Name: name}}}}}
	}
	// A tick runs EITHER the normal update OR a menu callback. A save may
	// happen before the first update, or after several ordinary updates.
	// Sequencing callbacks after _update would lose valid intermediate states.
	stmt := &luaparser.IfStmt{NodeBase: node, Condition: &luaparser.AnalysisChoiceExpr{NodeBase: node}, Then: callBlock(binding.save), ElseIf: []*luaparser.IfBranch{{NodeBase: node, Condition: &luaparser.AnalysisChoiceExpr{NodeBase: node}, Body: callBlock(binding.load)}}, Else: &luaparser.Block{Body: oldBody}}
	synthetic[stmt] = true
	update.Body.Body = []luaparser.Stmt{stmt}
	return func() { update.Body.Body = oldBody; chunk.Body = oldChunk }, synthetic, nil
}

func (g *generator) pauseCallbackMethod(index int) string {
	name := []string{"__glu_pause_save", "__glu_pause_load"}[index]
	for {
		used := false
		for _, fn := range g.funcList {
			if fn.goName == name {
				used = true
			}
		}
		for _, field := range g.globalGo {
			if field == name {
				used = true
			}
		}
		if !used {
			return name
		}
		name += "_"
	}
}

func (g *generator) renderPauseCallbacks(b *sourceBuilder) {
	binding, err := findPauseCallbacks(g.chunk)
	if err != nil {
		g.setErr(err)
		return
	}
	for i, name := range []string{binding.save, binding.load} {
		method := g.pauseCallbackMethod(i)
		b.line("func (g *Game) " + method + "(c *rt.Context) int {")
		if name == "" {
			b.line("\treturn rt.SaveIOError")
		} else {
			fn := g.funcs[name]
			if fn == nil || fn.returnN != 1 {
				g.setErr(fmt.Errorf("transpiler: %s: [PAUSE_MENU] callback %q must return one numeric save status", binding.call.Pos(), name))
				b.line("\treturn rt.SaveIOError")
			} else {
				call := "g." + fn.goName + "(c)"
				if fn.returnKind(0) == monoNumber {
					b.line("\tresult := " + call)
					b.line("\tif result&0xffff != 0 { return rt.SaveIOError }")
					b.line("\treturn result.Int()")
				} else {
					b.line("\tresult := " + call)
					b.line("\tif result.Kind != rt.KindNumber || result.Num&0xffff != 0 { return rt.SaveIOError }")
					b.line("\treturn result.Int()")
				}
			}
		}
		b.line("}")
	}
}
