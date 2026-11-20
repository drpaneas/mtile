package transpiler

import (
	"fmt"
	"sort"

	"github.com/drpaneas/mtile/internal/luaparser"
)

type boundedCounterResult struct {
	ids    []int
	ok     bool
	detail string
}

// boundedCounterIDs closes a scalar that is updated by arithmetic and then
// reset or reduced to a finite set inside complete lifecycle callbacks. The
// set is the values the variable can hold when _draw starts: after _init and
// after any number of complete _update/_draw callbacks. Intermediate values
// that exist only between statements inside _update are not drawn unless
// _draw is called from those other callbacks.
func (a *mode0SpriteIDAnalysis) boundedCounterIDs(name string) ([]int, bool, string) {
	if a == nil {
		return nil, false, "no finite-set analysis is available"
	}
	if result, cached := a.bounded[name]; cached {
		return result.ids, result.ok, result.detail
	}
	if a.boundedBusy[name] {
		return nil, false, fmt.Sprintf("%s has a cyclic bounded-counter proof", name)
	}
	a.boundedBusy[name] = true
	ids, ok, detail := a.proveBoundedCounter(name)
	delete(a.boundedBusy, name)
	result := boundedCounterResult{ids: ids, ok: ok, detail: detail}
	if a.bounded == nil {
		a.bounded = map[string]boundedCounterResult{}
	}
	a.bounded[name] = result
	return ids, ok, detail
}

func (a *mode0SpriteIDAnalysis) proveBoundedCounter(name string) ([]int, bool, string) {
	if a.chunk == nil {
		return nil, false, fmt.Sprintf("%s has a non-enumerated assignment", name)
	}
	if counterCallsDrawOutsideDraw(a.chunk) {
		return nil, false, fmt.Sprintf("%s is updated in a callback that also calls _draw", name)
	}
	interp := &counterInterp{
		analysis: a,
		name:     name,
		frozen:   a.frozenIntegers(name),
		graph:    newMode0CallGraph(a.chunk),
		visiting: map[*luaparser.FunctionDeclStmt]bool{},
	}
	if unreachable := interp.unreachableWriter(a.chunk); unreachable != "" {
		return nil, false, fmt.Sprintf("%s is assigned in %s, which is not a static lifecycle helper", name, unreachable)
	}
	initSet, ok := interp.initialSet()
	if !ok {
		return nil, false, fmt.Sprintf("%s has a non-enumerated assignment", name)
	}
	if len(initSet) == 0 {
		return nil, false, fmt.Sprintf("%s needs an unconditional top-level or _init assignment", name)
	}
	set := append([]int(nil), initSet...)
	for range mode0MaxFiniteNums + 2 {
		next, ok := unionIntSets(set)
		if !ok {
			return nil, false, fmt.Sprintf("%s has a non-enumerated assignment", name)
		}
		for _, key := range []string{"_update", "_draw"} {
			out, ok := interp.execFunctionKey(key, set)
			if !ok {
				return nil, false, fmt.Sprintf("%s has a non-enumerated assignment", name)
			}
			next, ok = unionIntSets(next, out)
			if !ok {
				return nil, false, fmt.Sprintf("%s has a non-enumerated assignment", name)
			}
		}
		if intSetsEqual(next, set) {
			return next, true, ""
		}
		set = next
	}
	return nil, false, fmt.Sprintf("%s has a non-enumerated assignment", name)
}

func (a *mode0SpriteIDAnalysis) frozenIntegers(except string) map[string]int {
	frozen := map[string]int{}
	for name, value := range a.constants {
		if name != except {
			frozen[name] = value
		}
	}
	for name, writes := range a.writes {
		if name == except || a.shadowed[name] || a.functionNames[name] {
			continue
		}
		if _, exists := frozen[name]; exists {
			continue
		}
		if len(writes) != 1 || !writes[0].valid {
			continue
		}
		write := writes[0]
		if !write.topLevel && !write.directInit {
			continue
		}
		value, ok := a.finiteWriteValue(write)
		if !ok {
			continue
		}
		frozen[name] = value
	}
	return frozen
}

type counterInterp struct {
	analysis *mode0SpriteIDAnalysis
	name     string
	frozen   map[string]int
	graph    *mode0CallGraph
	visiting map[*luaparser.FunctionDeclStmt]bool
}

func (c *counterInterp) initialSet() ([]int, bool) {
	set := []int{}
	for _, stmt := range c.analysis.chunk.Body {
		if _, isFn := stmt.(*luaparser.FunctionDeclStmt); isFn {
			continue
		}
		var ok bool
		set, ok = c.execStmt(stmt, set)
		if !ok {
			return nil, false
		}
	}
	return c.execFunctionKey("_init", set)
}

func (c *counterInterp) execFunctionKey(key string, in []int) ([]int, bool) {
	if c.graph == nil {
		return in, true
	}
	fns := c.graph.functions[key]
	if len(fns) == 0 {
		return in, true
	}
	var out []int
	for _, fn := range fns {
		next, ok := c.execFunction(fn, in)
		if !ok {
			return nil, false
		}
		merged, ok := unionIntSets(out, next)
		if !ok {
			return nil, false
		}
		out = merged
	}
	return out, true
}

func (c *counterInterp) execFunction(fn *luaparser.FunctionDeclStmt, in []int) ([]int, bool) {
	if fn == nil {
		return in, true
	}
	if c.visiting[fn] {
		return nil, false
	}
	c.visiting[fn] = true
	out, ok := c.execBlock(fn.Body, in)
	delete(c.visiting, fn)
	return out, ok
}

func (c *counterInterp) execBlock(block *luaparser.Block, in []int) ([]int, bool) {
	if block == nil {
		return in, true
	}
	set := in
	for _, stmt := range block.Body {
		var ok bool
		set, ok = c.execStmt(stmt, set)
		if !ok {
			return nil, false
		}
	}
	return set, true
}

func (c *counterInterp) execStmt(stmt luaparser.Stmt, in []int) ([]int, bool) {
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		return c.execAssign(s, in)
	case *luaparser.LocalAssignStmt:
		if localAssignsName(s, c.name) {
			return nil, false
		}
		return in, true
	case *luaparser.IfStmt:
		return c.execIf(s, in)
	case *luaparser.DoStmt:
		return c.execBlock(s.Body, in)
	case *luaparser.CallStmt:
		return c.execCall(s.Call, in)
	case *luaparser.WhileStmt:
		if blockAssignsName(s.Body, c.name) {
			return nil, false
		}
		return in, true
	case *luaparser.RepeatStmt:
		if blockAssignsName(s.Body, c.name) {
			return nil, false
		}
		return in, true
	case *luaparser.NumericForStmt:
		if s.Name == c.name || blockAssignsName(s.Body, c.name) {
			return nil, false
		}
		return in, true
	case *luaparser.GenericForStmt:
		for _, name := range s.Names {
			if name == c.name {
				return nil, false
			}
		}
		if blockAssignsName(s.Body, c.name) {
			return nil, false
		}
		return in, true
	case *luaparser.FunctionDeclStmt:
		return in, true
	case *luaparser.ReturnStmt, *luaparser.BreakStmt:
		return in, true
	default:
		return in, true
	}
}

func (c *counterInterp) execAssign(stmt *luaparser.AssignStmt, in []int) ([]int, bool) {
	if stmt == nil {
		return in, true
	}
	changed := false
	set := in
	for i, target := range stmt.Targets {
		variable, ok := unparenExpr(target).(*luaparser.VarExpr)
		if !ok || variable.Name != c.name {
			continue
		}
		if len(stmt.Targets) != 1 || len(stmt.Values) != 1 || i >= len(stmt.Values) {
			return nil, false
		}
		values, ok := c.eval(stmt.Values[i], in)
		if !ok {
			return nil, false
		}
		set = values
		changed = true
	}
	if !changed {
		return in, true
	}
	return set, true
}

func (c *counterInterp) execIf(stmt *luaparser.IfStmt, in []int) ([]int, bool) {
	remaining := in
	var joined []int
	apply := func(cond luaparser.Expr, body *luaparser.Block) bool {
		thenSet, elseSet, known := c.split(remaining, cond)
		if !known {
			thenSet = remaining
			elseSet = remaining
		}
		thenOut := thenSet
		if body != nil && (len(thenSet) > 0 || !known) {
			var ok bool
			thenOut, ok = c.execBlock(body, thenSet)
			if !ok {
				return false
			}
		}
		var ok bool
		joined, ok = unionIntSets(joined, thenOut)
		if !ok {
			return false
		}
		remaining = elseSet
		return true
	}
	if !apply(stmt.Condition, stmt.Then) {
		return nil, false
	}
	for _, branch := range stmt.ElseIf {
		if !apply(branch.Condition, branch.Body) {
			return nil, false
		}
	}
	if stmt.Else != nil {
		elseOut, ok := c.execBlock(stmt.Else, remaining)
		if !ok {
			return nil, false
		}
		return unionIntSets(joined, elseOut)
	}
	return unionIntSets(joined, remaining)
}

func (c *counterInterp) execCall(call *luaparser.CallExpr, in []int) ([]int, bool) {
	key, ok := callKey(call)
	if !ok {
		return nil, false
	}
	if _, builtin := Builtins[key]; builtin {
		return in, true
	}
	return c.execFunctionKey(key, in)
}

func (c *counterInterp) eval(expr luaparser.Expr, in []int) ([]int, bool) {
	if expr == nil {
		return nil, false
	}
	expr = unparenExpr(expr)
	if value, ok := staticNumberLiteral(expr); ok {
		if !isMode0Int(value) {
			return nil, false
		}
		return []int{int(value)}, true
	}
	switch n := expr.(type) {
	case *luaparser.VarExpr:
		if n.Name == c.name {
			if len(in) == 0 {
				return nil, false
			}
			return append([]int(nil), in...), true
		}
		value, ok := c.frozen[n.Name]
		if !ok {
			return nil, false
		}
		return []int{value}, true
	case *luaparser.UnaryExpr:
		if n.Op != luaparser.OpNegate {
			return nil, false
		}
		values, ok := c.eval(n.Expr, in)
		if !ok {
			return nil, false
		}
		out := make([]int, len(values))
		for i, value := range values {
			out[i] = -value
		}
		sort.Ints(out)
		return out, true
	case *luaparser.BinaryExpr:
		left, ok := c.eval(n.Left, in)
		if !ok {
			return nil, false
		}
		right, ok := c.eval(n.Right, in)
		if !ok {
			return nil, false
		}
		return counterBinary(n.Op, left, right)
	default:
		return nil, false
	}
}

func (c *counterInterp) split(in []int, cond luaparser.Expr) (thenSet, elseSet []int, known bool) {
	thenSet, elseSet, ok := c.constrain(in, cond, true)
	if !ok {
		return nil, nil, false
	}
	return thenSet, elseSet, true
}

func (c *counterInterp) constrain(in []int, cond luaparser.Expr, _ bool) (thenSet, elseSet []int, ok bool) {
	thenSet, elseSet, known := c.predicate(in, cond)
	if !known {
		return in, in, true
	}
	return thenSet, elseSet, true
}

func (c *counterInterp) predicate(in []int, cond luaparser.Expr) (thenSet, elseSet []int, known bool) {
	cond = unparenExpr(cond)
	bin, ok := cond.(*luaparser.BinaryExpr)
	if !ok {
		return nil, nil, false
	}
	switch bin.Op {
	case luaparser.OpAnd:
		leftThen, leftElse, leftKnown := c.predicate(in, bin.Left)
		if !leftKnown {
			return nil, nil, false
		}
		rightThen, _, rightKnown := c.predicate(leftThen, bin.Right)
		if !rightKnown {
			return nil, nil, false
		}
		thenSet = rightThen
		elseSet, ok = differenceIntSets(in, thenSet)
		if !ok {
			return nil, nil, false
		}
		_ = leftElse
		return thenSet, elseSet, true
	case luaparser.OpOr:
		leftThen, leftElse, leftKnown := c.predicate(in, bin.Left)
		if !leftKnown {
			return nil, nil, false
		}
		rightThen, _, rightKnown := c.predicate(leftElse, bin.Right)
		if !rightKnown {
			return nil, nil, false
		}
		thenSet, ok = unionIntSets(leftThen, rightThen)
		if !ok {
			return nil, nil, false
		}
		elseSet, ok = differenceIntSets(in, thenSet)
		if !ok {
			return nil, nil, false
		}
		return thenSet, elseSet, true
	}
	return c.comparePredicate(in, bin)
}

func (c *counterInterp) comparePredicate(in []int, bin *luaparser.BinaryExpr) (thenSet, elseSet []int, known bool) {
	if bin == nil {
		return nil, nil, false
	}
	match := func(side, other luaparser.Expr, op luaparser.Operator) bool {
		variable, ok := unparenExpr(side).(*luaparser.VarExpr)
		if !ok || variable.Name != c.name {
			return false
		}
		bounds, ok := c.eval(other, in)
		if !ok || len(bounds) != 1 {
			return false
		}
		bound := bounds[0]
		for _, value := range in {
			if counterCompare(op, value, bound) {
				thenSet = append(thenSet, value)
			} else {
				elseSet = append(elseSet, value)
			}
		}
		return true
	}
	if match(bin.Left, bin.Right, bin.Op) {
		return thenSet, elseSet, true
	}
	flipped, ok := flipCompare(bin.Op)
	if !ok {
		return nil, nil, false
	}
	if match(bin.Right, bin.Left, flipped) {
		return thenSet, elseSet, true
	}
	return nil, nil, false
}

func (c *counterInterp) unreachableWriter(chunk *luaparser.Chunk) string {
	reachable := lifecycleReachable(chunk)
	var found string
	var visit func(luaparser.Stmt, string)
	visit = func(stmt luaparser.Stmt, function string) {
		if stmt == nil || found != "" {
			return
		}
		switch s := stmt.(type) {
		case *luaparser.FunctionDeclStmt:
			key := functionKey(s)
			if s.Body == nil {
				return
			}
			for _, body := range s.Body.Body {
				visit(body, key)
			}
		case *luaparser.AssignStmt:
			for _, target := range s.Targets {
				variable, ok := unparenExpr(target).(*luaparser.VarExpr)
				if !ok || variable.Name != c.name {
					continue
				}
				if function == "" || reachable[function] {
					return
				}
				found = function
				return
			}
		case *luaparser.IfStmt:
			visitBlock(s.Then, function, visit)
			for _, branch := range s.ElseIf {
				visitBlock(branch.Body, function, visit)
			}
			visitBlock(s.Else, function, visit)
		case *luaparser.DoStmt:
			visitBlock(s.Body, function, visit)
		case *luaparser.WhileStmt:
			visitBlock(s.Body, function, visit)
		case *luaparser.RepeatStmt:
			visitBlock(s.Body, function, visit)
		case *luaparser.NumericForStmt:
			visitBlock(s.Body, function, visit)
		case *luaparser.GenericForStmt:
			visitBlock(s.Body, function, visit)
		}
	}
	for _, stmt := range chunk.Body {
		visit(stmt, "")
	}
	return found
}

func visitBlock(block *luaparser.Block, function string, visit func(luaparser.Stmt, string)) {
	if block == nil {
		return
	}
	for _, stmt := range block.Body {
		visit(stmt, function)
	}
}

func lifecycleReachable(chunk *luaparser.Chunk) map[string]bool {
	reachable := map[string]bool{"_init": true, "_update": true, "_draw": true}
	if chunk == nil {
		return reachable
	}
	graph := newMode0CallGraph(chunk)
	queue := []string{"_init", "_update", "_draw"}
	seen := map[*luaparser.FunctionDeclStmt]bool{}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		for _, fn := range graph.functions[key] {
			if seen[fn] || fn.Body == nil {
				continue
			}
			seen[fn] = true
			for _, stmt := range fn.Body.Body {
				collectStmtExprs(stmt, func(expr luaparser.Expr) {
					call, ok := expr.(*luaparser.CallExpr)
					if !ok {
						return
					}
					callee, ok := callKey(call)
					if !ok {
						return
					}
					if _, builtin := Builtins[callee]; builtin {
						return
					}
					if reachable[callee] {
						return
					}
					reachable[callee] = true
					queue = append(queue, callee)
				})
			}
		}
	}
	return reachable
}

func counterCallsDrawOutsideDraw(chunk *luaparser.Chunk) bool {
	if chunk == nil {
		return false
	}
	found := false
	var visit func(luaparser.Stmt, string)
	visit = func(stmt luaparser.Stmt, function string) {
		if stmt == nil || found {
			return
		}
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			key := functionKey(fn)
			if fn.Body == nil {
				return
			}
			for _, body := range fn.Body.Body {
				visit(body, key)
			}
			return
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			if found {
				return
			}
			call, ok := expr.(*luaparser.CallExpr)
			if !ok {
				return
			}
			name, ok := callKey(call)
			if ok && name == "_draw" && function != "_draw" {
				found = true
			}
		})
		switch s := stmt.(type) {
		case *luaparser.IfStmt:
			visitBlock(s.Then, function, visit)
			for _, branch := range s.ElseIf {
				visitBlock(branch.Body, function, visit)
			}
			visitBlock(s.Else, function, visit)
		case *luaparser.DoStmt:
			visitBlock(s.Body, function, visit)
		case *luaparser.WhileStmt:
			visitBlock(s.Body, function, visit)
		case *luaparser.RepeatStmt:
			visitBlock(s.Body, function, visit)
		case *luaparser.NumericForStmt:
			visitBlock(s.Body, function, visit)
		case *luaparser.GenericForStmt:
			visitBlock(s.Body, function, visit)
		}
	}
	for _, stmt := range chunk.Body {
		visit(stmt, "")
	}
	return found
}

func localAssignsName(stmt *luaparser.LocalAssignStmt, name string) bool {
	if stmt == nil {
		return false
	}
	for _, local := range stmt.Names {
		if local == name {
			return true
		}
	}
	return false
}

func blockAssignsName(block *luaparser.Block, name string) bool {
	if block == nil {
		return false
	}
	found := false
	var visit func(luaparser.Stmt)
	visit = func(stmt luaparser.Stmt) {
		if stmt == nil || found {
			return
		}
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			for _, target := range s.Targets {
				variable, ok := unparenExpr(target).(*luaparser.VarExpr)
				if ok && variable.Name == name {
					found = true
					return
				}
			}
		case *luaparser.LocalAssignStmt:
			if localAssignsName(s, name) {
				found = true
			}
		case *luaparser.IfStmt:
			visitBlock(s.Then, "", func(nested luaparser.Stmt, _ string) { visit(nested) })
			for _, branch := range s.ElseIf {
				visitBlock(branch.Body, "", func(nested luaparser.Stmt, _ string) { visit(nested) })
			}
			visitBlock(s.Else, "", func(nested luaparser.Stmt, _ string) { visit(nested) })
		case *luaparser.DoStmt:
			visitBlock(s.Body, "", func(nested luaparser.Stmt, _ string) { visit(nested) })
		case *luaparser.WhileStmt:
			visitBlock(s.Body, "", func(nested luaparser.Stmt, _ string) { visit(nested) })
		case *luaparser.RepeatStmt:
			visitBlock(s.Body, "", func(nested luaparser.Stmt, _ string) { visit(nested) })
		case *luaparser.NumericForStmt:
			visitBlock(s.Body, "", func(nested luaparser.Stmt, _ string) { visit(nested) })
		case *luaparser.GenericForStmt:
			visitBlock(s.Body, "", func(nested luaparser.Stmt, _ string) { visit(nested) })
		}
	}
	for _, stmt := range block.Body {
		visit(stmt)
	}
	return found
}

func counterBinary(op luaparser.Operator, left, right []int) ([]int, bool) {
	if len(left) == 0 || len(right) == 0 || len(left)*len(right) > mode0MaxFiniteNums {
		return nil, false
	}
	seen := map[int]bool{}
	out := make([]int, 0, len(left)*len(right))
	for _, a := range left {
		for _, b := range right {
			value, ok := mode0ApplyOp(op, float64(a), float64(b))
			if !ok || !isMode0Int(value) {
				return nil, false
			}
			id := int(value)
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
			if len(out) > mode0MaxFiniteNums {
				return nil, false
			}
		}
	}
	sort.Ints(out)
	return out, true
}

func counterCompare(op luaparser.Operator, left, right int) bool {
	switch op {
	case luaparser.OpEqual:
		return left == right
	case luaparser.OpNotEqual:
		return left != right
	case luaparser.OpLess:
		return left < right
	case luaparser.OpLessEq:
		return left <= right
	case luaparser.OpGreater:
		return left > right
	case luaparser.OpGreaterEq:
		return left >= right
	default:
		return false
	}
}

func flipCompare(op luaparser.Operator) (luaparser.Operator, bool) {
	switch op {
	case luaparser.OpEqual, luaparser.OpNotEqual:
		return op, true
	case luaparser.OpLess:
		return luaparser.OpGreater, true
	case luaparser.OpLessEq:
		return luaparser.OpGreaterEq, true
	case luaparser.OpGreater:
		return luaparser.OpLess, true
	case luaparser.OpGreaterEq:
		return luaparser.OpLessEq, true
	default:
		return "", false
	}
}

func unionIntSets(sets ...[]int) ([]int, bool) {
	seen := map[int]bool{}
	out := make([]int, 0)
	for _, set := range sets {
		for _, value := range set {
			if seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
			if len(out) > mode0MaxFiniteNums {
				return nil, false
			}
		}
	}
	sort.Ints(out)
	return out, true
}

func differenceIntSets(left, right []int) ([]int, bool) {
	drop := map[int]bool{}
	for _, value := range right {
		drop[value] = true
	}
	out := make([]int, 0, len(left))
	for _, value := range left {
		if drop[value] {
			continue
		}
		out = append(out, value)
	}
	sort.Ints(out)
	return out, true
}

func intSetsEqual(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i, value := range left {
		if value != right[i] {
			return false
		}
	}
	return true
}
