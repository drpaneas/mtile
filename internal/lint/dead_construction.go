package lint

import (
	"fmt"
	"strconv"
	"strings"

	p "github.com/drpaneas/mtile/internal/luaparser"
)

// This is a bounded forward ownership trace, not an instruction to delete a
// whole initializer. It follows exact lexical bindings and fresh table paths.
// Observations, escapes, unknown effects and uncertain control flow end proofs.
type constructionPath struct {
	key  string
	text string
}

type constructionCall struct {
	name string
	node *p.CallExpr
}

type pendingConstruction struct {
	path  constructionPath
	node  *p.TableExpr
	calls []constructionCall
}

type constructionTrace struct {
	a       *analyzer
	visible map[string]map[string]constructionPath
	pending map[string]pendingConstruction
	owned   map[string]bool
	active  map[string]bool
	serial  int
	budget  int
	epoch   int
	frame   []string
	failed  bool
}

func (a *analyzer) checkDeadConstructions(chunk *p.Chunk, functions []*p.FunctionDeclStmt, hot map[string]framePath) {
	t := &constructionTrace{a: a, visible: map[string]map[string]constructionPath{}}
	// Top-level functions see the chunk locals in scope at their declaration.
	// They do not capture arbitrary caller locals when we follow a helper call.
	visible := map[string]constructionPath{}
	for _, stmt := range chunk.Body {
		switch s := stmt.(type) {
		case *p.LocalAssignStmt:
			for _, name := range s.Names {
				visible[name] = t.local(name)
			}
		case *p.FunctionDeclStmt:
			if len(s.Name) == 1 && a.funcs[s.Name[0]] == s {
				t.visible[s.Name[0]] = copyConstructionEnv(visible)
			}
		}
	}
	for _, fn := range functions {
		name := fn.Name[0]
		if _, ok := hot[name]; !ok {
			continue
		}
		t.pending = map[string]pendingConstruction{}
		t.owned = map[string]bool{}
		t.active = map[string]bool{name: true}
		t.budget = 2048
		t.frame = append([]string{}, hot[name].functions...)
		t.failed = false
		env := copyConstructionEnv(t.visible[name])
		for _, param := range fn.Params {
			env[param] = t.local(param)
		}
		t.block(fn.Body.Body, env, nil)
	}
}

func copyConstructionEnv(env map[string]constructionPath) map[string]constructionPath {
	copy := make(map[string]constructionPath, len(env))
	for key, value := range env {
		copy[key] = value
	}
	return copy
}

func (t *constructionTrace) local(name string) constructionPath {
	t.serial++
	return constructionPath{fmt.Sprintf("local:%d:%s", t.serial, name), name}
}

func (t *constructionTrace) path(expr p.Expr, env map[string]constructionPath) (constructionPath, bool) {
	switch e := unparen(expr).(type) {
	case *p.VarExpr:
		if path, ok := env[e.Name]; ok {
			return path, true
		}
		return constructionPath{"global:" + e.Name, e.Name}, true
	case *p.FieldExpr:
		base, ok := t.path(e.Base, env)
		return constructionField(base, e.Name), ok
	case *p.IndexExpr:
		base, ok := t.path(e.Base, env)
		if !ok {
			return constructionPath{}, false
		}
		if key, ok := unparen(e.Key).(*p.StringExpr); ok {
			return constructionField(base, key.Value), true
		}
		if key, ok := integer(e.Key); ok {
			return constructionPath{base.key + "\x00#" + strconv.Itoa(key), fmt.Sprintf("%s[%d]", base.text, key)}, true
		}
	}
	return constructionPath{}, false
}

func constructionField(base constructionPath, key string) constructionPath {
	text := base.text + "[" + strconv.Quote(key) + "]"
	if tokens, err := p.Lex("", key); err == nil && len(tokens) == 2 && tokens[0].Kind == p.TokenIdentifier {
		text = base.text + "." + key
	}
	return constructionPath{base.key + "\x00" + strconv.Quote(key), text}
}

func pathContains(parent, child string) bool {
	return parent == child || strings.HasPrefix(child, parent+"\x00")
}

func constructionParent(key string) string {
	if n := strings.LastIndexByte(key, 0); n >= 0 {
		return key[:n]
	}
	return ""
}

func (t *constructionTrace) barrier() {
	clear(t.pending)
	clear(t.owned)
	t.epoch++
}

func (t *constructionTrace) forget(key string) {
	for candidate := range t.pending {
		if pathContains(key, candidate) {
			delete(t.pending, candidate)
		}
	}
	for owned := range t.owned {
		if pathContains(key, owned) {
			delete(t.owned, owned)
		}
	}
}

func (t *constructionTrace) observe(path constructionPath) {
	// Reading one sibling can observe its parent record, but not an unrelated
	// child. A whole-table read may alias that object and all of its descendants.
	for key := range t.pending {
		if pathContains(key, path.key) || pathContains(path.key, key) {
			delete(t.pending, key)
		}
	}
	for key := range t.owned {
		if pathContains(path.key, key) {
			delete(t.owned, key)
		}
	}
}

func (t *constructionTrace) expression(expr p.Expr, env map[string]constructionPath) {
	if expr == nil {
		return
	}
	if path, ok := t.path(expr, env); ok {
		t.observe(path)
		return
	}
	switch e := unparen(expr).(type) {
	case *p.UnaryExpr:
		t.expression(e.Expr, env)
	case *p.BinaryExpr:
		t.expression(e.Left, env)
		if e.Op == p.OpAnd || e.Op == p.OpOr {
			t.barrier()
		}
		t.expression(e.Right, env)
	case *p.TableExpr:
		for _, field := range e.Fields {
			if !field.Array {
				t.expression(field.Key, env)
			}
			t.expression(field.Value, env)
		}
	case *p.CallExpr, *p.IndexExpr, *p.FieldExpr:
		// We do not guess effects or aliases of computed receivers, calls used
		// as values, dynamic indexing, or functions passed through parameters.
		t.barrier()
	}
}

func (t *constructionTrace) register(path constructionPath, table *p.TableExpr, calls []constructionCall, safe bool) {
	parent := constructionParent(path.key)
	if parent != "" && !t.owned[parent] {
		return
	}
	t.owned[path.key] = true
	if safe {
		t.pending[path.key] = pendingConstruction{path, table, append([]constructionCall{}, calls...)}
	}
	arrayIndex := 0
	for _, field := range table.Fields {
		value, ok := unparen(field.Value).(*p.TableExpr)
		if !ok {
			if field.Array {
				arrayIndex++
			}
			continue
		}
		var child constructionPath
		if field.Array {
			arrayIndex++
			child = constructionPath{path.key + "\x00#" + strconv.Itoa(arrayIndex), fmt.Sprintf("%s[%d]", path.text, arrayIndex)}
		} else if key, ok := unparen(field.Key).(*p.StringExpr); ok {
			child = constructionField(path, key.Value)
		} else if key, ok := integer(field.Key); ok {
			child = constructionPath{path.key + "\x00#" + strconv.Itoa(key), fmt.Sprintf("%s[%d]", path.text, key)}
		} else {
			continue
		}
		t.register(child, value, calls, safe)
	}
}

func (t *constructionTrace) assign(path constructionPath, value p.Expr, site p.Node, env map[string]constructionPath, calls []constructionCall) {
	epoch := t.epoch
	t.expression(value, env)
	// Report only the outermost discarded fresh result if its nested tables
	// disappear with it. Reading a parent earlier may leave a child-only proof.
	for key, old := range t.pending {
		if !pathContains(path.key, key) {
			continue
		}
		covered := false
		for other := range t.pending {
			if other != key && pathContains(path.key, other) && pathContains(other, key) {
				covered = true
				break
			}
		}
		if !covered {
			t.report(old, site, calls)
		}
	}
	t.forget(path.key)
	if table, ok := unparen(value).(*p.TableExpr); ok {
		t.register(path, table, calls, epoch == t.epoch)
	}
}

// Helpers that contain uncertain control flow are barriers. A small literal
// unit-step loop can be traced exactly; unknown or large bounds cannot.
func flatConstructionBlock(body []p.Stmt) bool {
	for _, stmt := range body {
		switch s := stmt.(type) {
		case *p.AssignStmt, *p.LocalAssignStmt, *p.CallStmt:
		case *p.DoStmt:
			if !flatConstructionBlock(s.Body.Body) {
				return false
			}
		case *p.NumericForStmt:
			if !constructionLoop(s) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func constructionLoop(loop *p.NumericForStmt) bool {
	if !literalRange(loop) || !flatConstructionBlock(loop.Body.Body) {
		return false
	}
	start, _ := integer(loop.Init)
	end, _ := integer(loop.Limit)
	if end-start+1 > 64 {
		return false
	}
	stable := true
	v := visitor{stmt: func(stmt p.Stmt, _ context) {
		if assign, ok := stmt.(*p.AssignStmt); ok {
			for _, target := range assign.Targets {
				if variable(target) == loop.Name {
					stable = false
				}
			}
		}
	}}
	v.statements(loop.Body.Body, context{})
	return stable
}

func (t *constructionTrace) call(call *p.CallExpr, env map[string]constructionPath, calls []constructionCall) {
	// Appending a fresh element to an owned, unescaped construction is part of
	// building that result, not an external observation. No other mutator is
	// assumed safe. Arguments are still evaluated for reads/effects/escapes.
	if t.a.builtin(call, "add") && len(call.Args) == 2 {
		if target, ok := t.path(call.Args[0], env); ok && t.owned[target.key] {
			t.expression(call.Args[1], env)
			return
		}
	}
	name := variable(call.Callee)
	fn := t.a.funcs[name]
	if fn != nil && t.active[name] {
		t.barrier()
		t.failed = true
		return
	}
	if call.Method != "" || fn == nil || len(call.Args) != 0 || len(fn.Params) != 0 || !flatConstructionBlock(fn.Body.Body) {
		t.barrier()
		return
	}
	t.active[name] = true
	next := append(append([]constructionCall{}, calls...), constructionCall{name, call})
	t.block(fn.Body.Body, t.visible[name], next)
	delete(t.active, name)
}

func (t *constructionTrace) block(body []p.Stmt, parent map[string]constructionPath, calls []constructionCall) bool {
	env := copyConstructionEnv(parent)
	locals := []string{}
	defer func() {
		for _, key := range locals {
			t.forget(key)
		}
	}()
	for _, stmt := range body {
		t.budget--
		if t.budget < 0 || t.failed {
			t.barrier()
			t.failed = true
			return false
		}
		switch s := stmt.(type) {
		case *p.LocalAssignStmt:
			if len(s.Names) == 1 && len(s.Values) == 1 && !s.Synthetic {
				// The RHS sees the old scope, before the new local is introduced.
				epoch := t.epoch
				t.expression(s.Values[0], env)
				path := t.local(s.Names[0])
				env[s.Names[0]] = path
				locals = append(locals, path.key)
				if table, ok := unparen(s.Values[0]).(*p.TableExpr); ok {
					t.register(path, table, calls, epoch == t.epoch)
				}
			} else {
				t.barrier()
				for _, name := range s.Names {
					path := t.local(name)
					env[name] = path
					locals = append(locals, path.key)
				}
			}
		case *p.AssignStmt:
			if len(s.Targets) != 1 || len(s.Values) != 1 || s.Synthetic {
				t.barrier()
				continue
			}
			path, ok := t.path(s.Targets[0], env)
			if !ok {
				t.barrier()
				continue
			}
			t.assign(path, s.Values[0], s.Targets[0], env, calls)
		case *p.CallStmt:
			t.call(s.Call, env, calls)
			if t.failed {
				return false
			}
		case *p.DoStmt:
			if !t.block(s.Body.Body, env, calls) {
				return false
			}
		case *p.ReturnStmt:
			for _, value := range s.Values {
				t.expression(value, env)
			}
			return false
		case *p.NumericForStmt:
			if constructionLoop(s) {
				start, _ := integer(s.Init)
				end, _ := integer(s.Limit)
				if end-start+1 <= 64 {
					loopEnv := copyConstructionEnv(env)
					loopEnv[s.Name] = t.local(s.Name)
					for i := start; i <= end && t.budget >= 0; i++ {
						if !t.block(s.Body.Body, loopEnv, calls) {
							return false
						}
					}
					continue
				}
			}
			t.barrier()
		case *p.IfStmt:
			// No dominance or frequency claim crosses a branch boundary. A proof
			// contained entirely in one branch still describes that entered path.
			t.barrier()
			allReturn, canFallThrough := true, true
			branches := []*p.IfBranch{{Condition: s.Condition, Body: s.Then}}
			branches = append(branches, s.ElseIf...)
			for _, branch := range branches {
				if !canFallThrough {
					break
				}
				if literal, ok := unparen(branch.Condition).(*p.BoolExpr); ok {
					if !literal.Value {
						continue
					}
					canFallThrough = false
				}
				if t.block(branch.Body.Body, env, calls) {
					allReturn = false
				}
				t.barrier()
			}
			if canFallThrough && s.Else != nil {
				if t.block(s.Else.Body, env, calls) {
					allReturn = false
				}
				canFallThrough = false
				t.barrier()
			}
			if !canFallThrough && allReturn {
				return false
			}
		default:
			t.barrier()
		}
	}
	return true
}

func (t *constructionTrace) report(old pendingConstruction, replacement p.Node, calls []constructionCall) {
	d := t.a.emit("MTL008", old.node, fmt.Sprintf("Fresh construction of %s is replaced before its result is observed.", old.path.text), "Avoid the first construction on the shown caller path while preserving other state changes. Other callers may need this construction. Keep results that are read, retained or shared; do not delete an entire helper call solely from this finding.")
	if d == nil {
		return
	}
	d.Label = "this fresh result is discarded by a later write"
	d.Guide = "docs/lint.md#mtl008-discarded-construction"
	d.CallPath = append([]string{}, t.frame...)
	for _, call := range old.calls {
		d.CallPath = append(d.CallPath, call.name)
	}
	d.Notes = []string{"The bounded trace found a fresh, unescaped object and a later replacement without an intervening observation. This is a data-flow finding, not a claim that the path runs every frame or a measured speedup."}
	add := func(node p.Node, label string) {
		location := t.a.location(node.Pos())
		if location.Path == d.Path && location.Line == d.Line && location.Column == d.Column {
			return
		}
		for _, related := range d.Related {
			if related.Location == location {
				return
			}
		}
		d.Related = append(d.Related, RelatedLocation{Location: location, Label: label, Snippet: t.a.snippet(node.Pos())})
	}
	if len(old.calls) > 0 {
		call := old.calls[len(old.calls)-1]
		add(call.node, "first construction reached through "+call.name+"()")
	}
	if len(calls) > 0 {
		call := calls[len(calls)-1]
		add(call.node, "replacement reached through "+call.name+"()")
	}
	add(replacement, "this write replaces "+old.path.text)
}
