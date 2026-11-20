package lint

import (
	"fmt"

	p "github.com/drpaneas/mtile/internal/luaparser"
)

type framePath struct {
	functions []string
	guard     *p.Position
}

// Prefer a path without syntactic guards whenever one exists. This fixed point
// prevents a conditional first caller from hiding an unguarded second caller.
// Guards are evidence of control flow, never evidence that an event is rare.
func (a *analyzer) frameFunctions() map[string]framePath {
	paths := map[string]framePath{}
	queue := []string{}
	for _, name := range []string{"_update", "_draw"} {
		if a.funcs[name] != nil {
			paths[name] = framePath{functions: []string{name}}
			queue = append(queue, name)
		}
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		path := paths[name]
		v := visitor{expr: func(expr p.Expr, c context) {
			call, ok := expr.(*p.CallExpr)
			if !ok || call.Method != "" {
				return
			}
			target := variable(call.Callee)
			if a.funcs[target] == nil {
				return
			}
			guard := path.guard
			if guard == nil {
				guard = c.guard
			}
			old, seen := paths[target]
			if seen && (old.guard == nil || guard != nil) {
				return
			}
			names := append(append([]string{}, path.functions...), target)
			paths[target] = framePath{functions: names, guard: guard}
			queue = append(queue, target)
		}}
		v.statements(a.funcs[name].Body.Body, context{})
	}
	return paths
}

func (a *analyzer) location(pos p.Position) Location {
	path, line, column, ok := a.bundle.Position(pos.Line, pos.Column)
	if !ok {
		path, line, column = pos.Source, pos.Line, pos.Column
	}
	return Location{path, line, column}
}

func (a *analyzer) tableAllocation(table *p.TableExpr, c context, path framePath) {
	d := a.emit("MTL001", table, "A new table can be allocated on a frame-reachable path.", "Reuse persistent scratch slots only when all needed values are overwritten and no consumer needs an older table. Separate objects are correct when identity or lifetime requires them.")
	if d == nil {
		return
	}
	d.CallPath = append([]string{}, path.functions...)
	guard := path.guard
	if guard == nil {
		guard = c.guard
	}
	if guard != nil {
		location := a.location(*guard)
		d.ConditionalAt = &location
		d.Severity = "note"
		d.Message = "Table allocation on a conditional frame path; execution frequency is unknown."
		d.Label = "creates a fresh table when this path runs"
		d.Notes = []string{"Reachability does not establish an allocation on every frame. This may be transition setup or ordinary frequent gameplay; measure its actual frequency."}
		d.Suggestion = "Review measured transition/spawn or GC costs before changing this allocation. Preserve separate live objects and snapshots; do not replace them with one shared table."
	}
	if pool := a.poolFallbacks[table]; pool != "" {
		d.Severity = "note"
		d.Message = "Allocation fallback after a reuse attempt returned nil."
		d.Label = "fallback constructor; a reuse path already exists"
		d.Notes = append(d.Notes, fmt.Sprintf("The preceding deli(%s) result is tested for nil before constructing this table.", pool))
		d.Suggestion = "Keep a fallback when no reusable object is available. Profile pool growth before reserving more memory; every simultaneously live object still needs its own identity."
	}
}

// Recognize one exact lexical idiom, not a function/variable-name heuristic:
// local p = deli(pool); if p == nil then p = {...} ... end.
// Adjacency and the first assignment rule out intervening writes/shadowing.
func (a *analyzer) findPoolFallbacks(body []p.Stmt) {
	v := visitor{block: a.scanPoolBlock}
	v.statements(body, context{})
}

func (a *analyzer) scanPoolBlock(body []p.Stmt) {
	for i := 1; i < len(body); i++ {
		local, ok := body[i-1].(*p.LocalAssignStmt)
		if !ok || len(local.Names) != 1 || len(local.Values) != 1 {
			continue
		}
		call, ok := unparen(local.Values[0]).(*p.CallExpr)
		if !ok || !a.builtin(call, "deli") || len(call.Args) != 1 {
			continue
		}
		pool := variable(call.Args[0])
		if pool == "" {
			continue
		}
		branch, ok := body[i].(*p.IfStmt)
		if !ok || len(branch.Then.Body) == 0 {
			continue
		}
		condition, ok := unparen(branch.Condition).(*p.BinaryExpr)
		if !ok || condition.Op != p.OpEqual {
			continue
		}
		isNil := func(e p.Expr) bool { _, ok := unparen(e).(*p.NilExpr); return ok }
		name := local.Names[0]
		comparesWithNil := variable(condition.Left) == name && isNil(condition.Right) || variable(condition.Right) == name && isNil(condition.Left)
		if !comparesWithNil {
			continue
		}
		assign, ok := branch.Then.Body[0].(*p.AssignStmt)
		if !ok || len(assign.Targets) != 1 || len(assign.Values) != 1 || variable(assign.Targets[0]) != name {
			continue
		}
		if table, ok := unparen(assign.Values[0]).(*p.TableExpr); ok {
			a.poolFallbacks[table] = pool
		}
	}
}

func diagnosticLabel(id string) string {
	switch id {
	case "MTL001":
		return "fresh table constructor"
	case "MTL002":
		return "whole-table identity is copied here"
	case "MTL003":
		return "resets a complete palette mapping layer"
	case "MTL004":
		return "map write with literal arguments"
	case "MTL005":
		return "one substring and print call per character"
	case "MTL006":
		return "repeated pixel calls for a solid rectangle"
	case "MTL007":
		return "this slot receives numbers and nonnumeric values"
	case "MTL008":
		return "fresh construction replaced before observation"
	}
	return "source pattern"
}
