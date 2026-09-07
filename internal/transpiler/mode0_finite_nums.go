package transpiler

import (
	"math"
	"sort"

	"github.com/drpaneas/mtile/internal/luaparser"
)

const (
	mode0MaxFiniteNums = 256
	mode0NumEpsilon    = 1e-9
)

type mode0NumEnv map[string][]float64

func cloneMode0NumEnv(env mode0NumEnv) mode0NumEnv {
	if len(env) == 0 {
		return mode0NumEnv{}
	}
	out := make(mode0NumEnv, len(env))
	for name, values := range env {
		out[name] = append([]float64(nil), values...)
	}
	return out
}

func unionMode0NumEnv(dst, src mode0NumEnv) mode0NumEnv {
	if dst == nil {
		dst = mode0NumEnv{}
	}
	for name, values := range src {
		merged, ok := mergeMode0Nums(dst[name], values)
		if !ok {
			delete(dst, name)
			continue
		}
		dst[name] = merged
	}
	return dst
}

func mergeMode0Nums(sets ...[]float64) ([]float64, bool) {
	seen := map[float64]bool{}
	out := make([]float64, 0)
	for _, set := range sets {
		for _, value := range set {
			key := mode0NumKey(value)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, value)
			if len(out) > mode0MaxFiniteNums {
				return nil, false
			}
		}
	}
	sort.Float64s(out)
	return out, true
}

func mode0NumKey(value float64) float64 {
	return math.Round(value/mode0NumEpsilon) * mode0NumEpsilon
}

func mode0IntsFromNums(values []float64) ([]int, bool) {
	if len(values) == 0 {
		return nil, false
	}
	seen := map[int]bool{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value-math.Trunc(value)) > 1e-6 {
			return nil, false
		}
		id := int(math.Trunc(value))
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Ints(out)
	return out, true
}

type mode0NumEval struct {
	chunk          *luaparser.Chunk
	graph          *mode0CallGraph
	analysis       *mode0SpriteIDAnalysis
	fn             *luaparser.FunctionDeclStmt
	visiting       map[*luaparser.FunctionDeclStmt]bool
	fieldsVisiting map[string]bool
}

func newMode0NumEval(chunk *luaparser.Chunk, analysis *mode0SpriteIDAnalysis) *mode0NumEval {
	return &mode0NumEval{
		chunk:          chunk,
		graph:          newMode0CallGraph(chunk),
		analysis:       analysis,
		visiting:       map[*luaparser.FunctionDeclStmt]bool{},
		fieldsVisiting: map[string]bool{},
	}
}

func (e *mode0NumEval) nums(expr luaparser.Expr, env mode0NumEnv) ([]float64, bool) {
	if expr == nil {
		return nil, false
	}
	expr = unparenExpr(expr)
	if value, ok := staticNumberLiteral(expr); ok {
		return []float64{value}, true
	}
	switch n := expr.(type) {
	case *luaparser.VarExpr:
		if values, ok := env[n.Name]; ok && len(values) > 0 {
			return values, true
		}
		if e.analysis != nil {
			if ids, proven, _ := e.analysis.finiteIDs(n); proven {
				out := make([]float64, len(ids))
				for i, id := range ids {
					out[i] = float64(id)
				}
				return out, true
			}
		}
		return nil, false
	case *luaparser.UnaryExpr:
		values, ok := e.nums(n.Expr, env)
		if !ok {
			return nil, false
		}
		if n.Op != luaparser.OpNegate {
			return nil, false
		}
		out := make([]float64, len(values))
		for i, value := range values {
			out[i] = -value
		}
		return out, true
	case *luaparser.BinaryExpr:
		left, ok := e.nums(n.Left, env)
		if !ok {
			if n.Op == luaparser.OpModulo {
				return e.modUnbounded(n.Right, env)
			}
			return nil, false
		}
		right, ok := e.nums(n.Right, env)
		if !ok {
			return nil, false
		}
		return e.binary(n.Op, left, right)
	case *luaparser.CallExpr:
		return e.call(n, env)
	case *luaparser.FieldExpr:
		return e.field(n.Name)
	case *luaparser.IndexExpr:
		return e.index(n, env)
	default:
		return nil, false
	}
}

func (e *mode0NumEval) modUnbounded(rhs luaparser.Expr, env mode0NumEnv) ([]float64, bool) {
	right, ok := e.nums(rhs, env)
	if !ok || len(right) != 1 || right[0] <= 0 {
		return nil, false
	}
	return e.intervalInts(0, right[0], false)
}

func (e *mode0NumEval) binary(op luaparser.Operator, left, right []float64) ([]float64, bool) {
	if len(left)*len(right) > mode0MaxFiniteNums {
		return nil, false
	}
	out := make([]float64, 0, len(left)*len(right))
	for _, a := range left {
		for _, b := range right {
			value, ok := mode0ApplyOp(op, a, b)
			if !ok {
				return nil, false
			}
			out = append(out, value)
		}
	}
	return mergeMode0Nums(out)
}

func mode0ApplyOp(op luaparser.Operator, a, b float64) (float64, bool) {
	switch op {
	case luaparser.OpAdd:
		return a + b, true
	case luaparser.OpSubtract:
		return a - b, true
	case luaparser.OpMultiply:
		return a * b, true
	case luaparser.OpDivide:
		if b == 0 {
			return 0, false
		}
		return a / b, true
	case luaparser.OpIntegerDivide:
		if b == 0 {
			return 0, false
		}
		return math.Floor(a / b), true
	case luaparser.OpModulo:
		if b == 0 {
			return 0, false
		}
		return a - math.Floor(a/b)*b, true
	default:
		return 0, false
	}
}

func (e *mode0NumEval) call(call *luaparser.CallExpr, env mode0NumEnv) ([]float64, bool) {
	name, ok := callKey(call)
	if !ok {
		return nil, false
	}
	switch name {
	case "flr", "ceil":
		if len(call.Args) == 0 {
			return nil, false
		}
		values, ok := e.nums(call.Args[0], env)
		if !ok {
			if lo, hi, bounded := e.interval(call.Args[0], env); bounded {
				return e.intervalInts(lo, hi, true)
			}
			return nil, false
		}
		out := make([]float64, len(values))
		for i, value := range values {
			if name == "ceil" {
				out[i] = math.Ceil(value)
			} else {
				out[i] = math.Floor(value)
			}
		}
		return mergeMode0Nums(out)
	case "mid":
		if len(call.Args) != 3 {
			return nil, false
		}
		a, okA := e.nums(call.Args[0], env)
		b, okB := e.nums(call.Args[1], env)
		c, okC := e.nums(call.Args[2], env)
		if !okA || !okB || !okC || len(a)*len(b)*len(c) > mode0MaxFiniteNums {
			return nil, false
		}
		out := make([]float64, 0, len(a)*len(b)*len(c))
		for _, x := range a {
			for _, y := range b {
				for _, z := range c {
					triple := [3]float64{x, y, z}
					sort.Float64s(triple[:])
					out = append(out, triple[1])
				}
			}
		}
		return mergeMode0Nums(out)
	case "abs":
		if len(call.Args) == 0 {
			return nil, false
		}
		values, ok := e.nums(call.Args[0], env)
		if !ok {
			return nil, false
		}
		out := make([]float64, len(values))
		for i, value := range values {
			out[i] = math.Abs(value)
		}
		return mergeMode0Nums(out)
	case "min", "max":
		if len(call.Args) != 2 {
			return nil, false
		}
		a, okA := e.nums(call.Args[0], env)
		b, okB := e.nums(call.Args[1], env)
		if !okA || !okB || len(a)*len(b) > mode0MaxFiniteNums {
			return nil, false
		}
		out := make([]float64, 0, len(a)*len(b))
		for _, x := range a {
			for _, y := range b {
				if name == "max" {
					out = append(out, math.Max(x, y))
				} else {
					out = append(out, math.Min(x, y))
				}
			}
		}
		return mergeMode0Nums(out)
	case "sin", "cos":
		return nil, false
	}
	if _, builtin := Builtins[name]; builtin {
		return nil, false
	}
	return e.userCall(name, call.Args, env)
}

func (e *mode0NumEval) userCall(name string, args []luaparser.Expr, env mode0NumEnv) ([]float64, bool) {
	if e.graph == nil {
		return nil, false
	}
	fns := e.graph.functions[name]
	if len(fns) == 0 {
		return nil, false
	}
	argSets := make([][]float64, len(args))
	for i, arg := range args {
		if values, ok := e.nums(arg, env); ok {
			argSets[i] = values
		}
	}
	var results []float64
	for _, fn := range fns {
		values, ok := e.execFunction(fn, argSets)
		if !ok {
			return nil, false
		}
		merged, ok := mergeMode0Nums(results, values)
		if !ok {
			return nil, false
		}
		results = merged
	}
	return results, len(results) > 0
}

func (e *mode0NumEval) index(expr *luaparser.IndexExpr, env mode0NumEnv) ([]float64, bool) {
	if expr == nil {
		return nil, false
	}
	slots, ok := e.arrayValues(expr.Base, env)
	if !ok {
		return nil, false
	}
	keys, ok := e.nums(expr.Key, env)
	if !ok {
		return nil, false
	}
	out := make([]float64, 0, len(keys))
	for _, key := range keys {
		if !isMode0Int(key) {
			return nil, false
		}
		index := int(key)
		if index < 1 || index > len(slots) {
			return nil, false
		}
		out = append(out, slots[index-1])
	}
	return mergeMode0Nums(out)
}

func (e *mode0NumEval) arrayValues(expr luaparser.Expr, env mode0NumEnv) ([]float64, bool) {
	switch n := unparenExpr(expr).(type) {
	case *luaparser.TableExpr:
		return mode0ArrayNums(n, e, env)
	case *luaparser.VarExpr:
		return e.namedArray(n.Name)
	default:
		return nil, false
	}
}

func mode0ArrayNums(table *luaparser.TableExpr, eval *mode0NumEval, env mode0NumEnv) ([]float64, bool) {
	if table == nil || eval == nil {
		return nil, false
	}
	out := make([]float64, 0, len(table.Fields))
	for _, field := range table.Fields {
		if field == nil || !field.Array {
			return nil, false
		}
		values, ok := eval.nums(field.Value, env)
		if !ok || len(values) != 1 {
			return nil, false
		}
		out = append(out, values[0])
	}
	return out, len(out) > 0
}

func (e *mode0NumEval) namedArray(name string) ([]float64, bool) {
	if e.chunk == nil || name == "" {
		return nil, false
	}
	var found []float64
	seen := false
	ok := true
	var visit func(luaparser.Stmt)
	visit = func(stmt luaparser.Stmt) {
		if stmt == nil || !ok {
			return
		}
		switch s := stmt.(type) {
		case *luaparser.LocalAssignStmt:
			for i, local := range s.Names {
				if local != name || i >= len(s.Values) {
					continue
				}
				table, isTable := unparenExpr(s.Values[i]).(*luaparser.TableExpr)
				if !isTable {
					ok = false
					return
				}
				values, proven := mode0ArrayNums(table, e, nil)
				if !proven || seen {
					ok = false
					return
				}
				found = values
				seen = true
			}
		case *luaparser.AssignStmt:
			for i, target := range s.Targets {
				if i >= len(s.Values) {
					continue
				}
				if index, isIndex := unparenExpr(target).(*luaparser.IndexExpr); isIndex {
					if variable, isVar := unparenExpr(index.Base).(*luaparser.VarExpr); isVar && variable.Name == name {
						ok = false
						return
					}
				}
				variable, isVar := unparenExpr(target).(*luaparser.VarExpr)
				if !isVar || variable.Name != name {
					continue
				}
				table, isTable := unparenExpr(s.Values[i]).(*luaparser.TableExpr)
				if !isTable {
					ok = false
					return
				}
				values, proven := mode0ArrayNums(table, e, nil)
				if !proven || seen {
					ok = false
					return
				}
				found = values
				seen = true
			}
		case *luaparser.FunctionDeclStmt:
			if s.Body == nil {
				return
			}
			for _, body := range s.Body.Body {
				visit(body)
			}
		case *luaparser.DoStmt:
			if s.Body == nil {
				return
			}
			for _, body := range s.Body.Body {
				visit(body)
			}
		case *luaparser.IfStmt:
			if s.Then != nil {
				for _, body := range s.Then.Body {
					visit(body)
				}
			}
			for _, branch := range s.ElseIf {
				if branch.Body == nil {
					continue
				}
				for _, body := range branch.Body.Body {
					visit(body)
				}
			}
			if s.Else != nil {
				for _, body := range s.Else.Body {
					visit(body)
				}
			}
		case *luaparser.NumericForStmt:
			if s.Body != nil {
				for _, body := range s.Body.Body {
					visit(body)
				}
			}
		case *luaparser.GenericForStmt:
			if s.Body != nil {
				for _, body := range s.Body.Body {
					visit(body)
				}
			}
		case *luaparser.WhileStmt:
			if s.Body != nil {
				for _, body := range s.Body.Body {
					visit(body)
				}
			}
		case *luaparser.RepeatStmt:
			if s.Body != nil {
				for _, body := range s.Body.Body {
					visit(body)
				}
			}
		}
	}
	for _, stmt := range e.chunk.Body {
		visit(stmt)
	}
	return found, ok && seen
}

func (e *mode0NumEval) execFunction(fn *luaparser.FunctionDeclStmt, args [][]float64) ([]float64, bool) {
	if fn == nil || e.visiting[fn] {
		return nil, false
	}
	e.visiting[fn] = true
	prev := e.fn
	e.fn = fn
	defer func() {
		delete(e.visiting, fn)
		e.fn = prev
	}()
	bindings := []mode0NumEnv{{}}
	for i, name := range fn.Params {
		if i >= len(args) || len(args[i]) == 0 {
			continue
		}
		next := make([]mode0NumEnv, 0, len(bindings)*len(args[i]))
		for _, env := range bindings {
			for _, value := range args[i] {
				cloned := cloneMode0NumEnv(env)
				cloned[name] = []float64{value}
				next = append(next, cloned)
				if len(next) > mode0MaxFiniteNums {
					return nil, false
				}
			}
		}
		bindings = next
	}
	var results []float64
	for _, env := range bindings {
		returns, _, ok := e.execBlock(fn.Body, env)
		if !ok {
			return nil, false
		}
		merged, mergedOK := mergeMode0Nums(results, returns)
		if !mergedOK {
			return nil, false
		}
		results = merged
	}
	return results, len(results) > 0
}

func (e *mode0NumEval) execBlock(block *luaparser.Block, env mode0NumEnv) ([]float64, mode0NumEnv, bool) {
	if block == nil {
		return nil, env, true
	}
	var returns []float64
	for _, stmt := range block.Body {
		ret, next, ok := e.execStmt(stmt, env)
		if !ok {
			return nil, env, false
		}
		env = next
		if len(ret) > 0 {
			merged, ok := mergeMode0Nums(returns, ret)
			if !ok {
				return nil, env, false
			}
			returns = merged
		}
	}
	return returns, env, true
}

func (e *mode0NumEval) execStmt(stmt luaparser.Stmt, env mode0NumEnv) ([]float64, mode0NumEnv, bool) {
	switch s := stmt.(type) {
	case *luaparser.LocalAssignStmt:
		next := cloneMode0NumEnv(env)
		for i, name := range s.Names {
			if i >= len(s.Values) {
				delete(next, name)
				continue
			}
			values, ok := e.nums(s.Values[i], env)
			if !ok {
				delete(next, name)
				continue
			}
			next[name] = values
		}
		return nil, next, true
	case *luaparser.AssignStmt:
		next := cloneMode0NumEnv(env)
		for i, target := range s.Targets {
			variable, ok := target.(*luaparser.VarExpr)
			if !ok || i >= len(s.Values) {
				continue
			}
			values, ok := e.nums(s.Values[i], env)
			if !ok {
				delete(next, variable.Name)
				continue
			}
			next[variable.Name] = values
		}
		return nil, next, true
	case *luaparser.IfStmt:
		if s.Else == nil && len(s.ElseIf) == 0 && blockAlwaysReturns(s.Then) {
			ret, _, ok := e.execBlock(s.Then, cloneMode0NumEnv(env))
			if !ok {
				return nil, env, false
			}
			return ret, e.applyGuardComplement(env, s.Condition), true
		}
		var returns []float64
		joined := mode0NumEnv{}
		if s.Else == nil {
			joined = unionMode0NumEnv(joined, env)
		}
		branches := []*luaparser.Block{s.Then}
		for _, branch := range s.ElseIf {
			branches = append(branches, branch.Body)
		}
		if s.Else != nil {
			branches = append(branches, s.Else)
		}
		for _, branch := range branches {
			ret, next, ok := e.execBlock(branch, cloneMode0NumEnv(env))
			if !ok {
				return nil, env, false
			}
			merged, ok := mergeMode0Nums(returns, ret)
			if !ok {
				return nil, env, false
			}
			returns = merged
			if blockAlwaysReturns(branch) {
				continue
			}
			joined = unionMode0NumEnv(joined, next)
		}
		return returns, joined, true
	case *luaparser.DoStmt:
		return e.execBlock(s.Body, env)
	case *luaparser.ReturnStmt:
		if len(s.Values) == 0 {
			return nil, env, true
		}
		values, ok := e.nums(s.Values[0], env)
		if !ok {
			if variable, isVar := unparenExpr(s.Values[0]).(*luaparser.VarExpr); isVar {
				if literals := mode0EqualityLiterals(e.fn, variable.Name); len(literals) > 0 {
					return literals, env, true
				}
			}
		}
		return values, env, ok
	case *luaparser.CallStmt, *luaparser.BreakStmt:
		return nil, env, true
	case *luaparser.NumericForStmt, *luaparser.GenericForStmt, *luaparser.WhileStmt, *luaparser.RepeatStmt:
		return nil, env, true
	default:
		return nil, env, true
	}
}

func (e *mode0NumEval) field(name string) ([]float64, bool) {
	if e.chunk == nil || name == "" || e.fieldsVisiting[name] {
		return nil, false
	}
	e.fieldsVisiting[name] = true
	defer delete(e.fieldsVisiting, name)
	var values []float64
	ok := true
	increment := false
	aliases := map[string]bool{}
	var uppers []float64
	var currentFn *luaparser.FunctionDeclStmt
	absorb := func(nums []float64, proven bool) {
		if !ok {
			return
		}
		if !proven {
			ok = false
			return
		}
		merged, mergedOK := mergeMode0Nums(values, nums)
		if !mergedOK {
			ok = false
			return
		}
		values = merged
	}
	var visit func(luaparser.Stmt)
	visit = func(stmt luaparser.Stmt) {
		if stmt == nil || !ok {
			return
		}
		switch s := stmt.(type) {
		case *luaparser.LocalAssignStmt:
			for i, local := range s.Names {
				if i >= len(s.Values) {
					continue
				}
				if _, _, self := mode0FieldIncrement(name, s.Values[i]); self {
					aliases[local] = true
					increment = true
				}
			}
		case *luaparser.AssignStmt:
			for i, target := range s.Targets {
				if i >= len(s.Values) {
					continue
				}
				field, isField := unparenExpr(target).(*luaparser.FieldExpr)
				if !isField || field.Name != name {
					continue
				}
				if _, _, self := mode0FieldIncrement(name, s.Values[i]); self {
					increment = true
					continue
				}
				if variable, isVar := unparenExpr(s.Values[i]).(*luaparser.VarExpr); isVar && aliases[variable.Name] {
					increment = true
					continue
				}
				nums, proven := e.exprNums(s.Values[i], currentFn)
				absorb(nums, proven)
			}
		case *luaparser.FunctionDeclStmt:
			prev := currentFn
			currentFn = s
			for _, body := range s.Body.Body {
				visit(body)
			}
			currentFn = prev
			return
		case *luaparser.DoStmt:
			for _, body := range s.Body.Body {
				visit(body)
			}
		case *luaparser.IfStmt:
			uppers = append(uppers, mode0BoundLiterals(s.Condition, name, aliases)...)
			for _, body := range s.Then.Body {
				visit(body)
			}
			for _, branch := range s.ElseIf {
				uppers = append(uppers, mode0BoundLiterals(branch.Condition, name, aliases)...)
				for _, body := range branch.Body.Body {
					visit(body)
				}
			}
			if s.Else != nil {
				for _, body := range s.Else.Body {
					visit(body)
				}
			}
		case *luaparser.NumericForStmt:
			for _, body := range s.Body.Body {
				visit(body)
			}
		case *luaparser.GenericForStmt:
			for _, body := range s.Body.Body {
				visit(body)
			}
		case *luaparser.WhileStmt:
			uppers = append(uppers, mode0BoundLiterals(s.Condition, name, aliases)...)
			for _, body := range s.Body.Body {
				visit(body)
			}
		case *luaparser.RepeatStmt:
			uppers = append(uppers, mode0BoundLiterals(s.Condition, name, aliases)...)
			for _, body := range s.Body.Body {
				visit(body)
			}
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			if !ok {
				return
			}
			table, isTable := unparenExpr(expr).(*luaparser.TableExpr)
			if !isTable {
				return
			}
			for _, field := range table.Fields {
				key, isString := unparenExpr(field.Key).(*luaparser.StringExpr)
				if !isString || key.Value != name {
					continue
				}
				nums, proven := e.exprNums(field.Value, currentFn)
				absorb(nums, proven)
			}
		})
	}
	for _, stmt := range e.chunk.Body {
		visit(stmt)
	}
	if !ok {
		return nil, false
	}
	if increment {
		if len(values) == 0 || len(uppers) == 0 {
			return nil, false
		}
		hi := uppers[0]
		for _, upper := range uppers[1:] {
			if upper > hi {
				hi = upper
			}
		}
		return e.intervalInts(values[0], hi, true)
	}
	return values, len(values) > 0
}

func (e *mode0NumEval) exprNums(expr luaparser.Expr, fn *luaparser.FunctionDeclStmt) ([]float64, bool) {
	if values, ok := e.nums(expr, nil); ok {
		return values, true
	}
	if lo, hi, bounded := e.interval(expr, nil); bounded {
		return e.intervalInts(lo, hi, true)
	}
	variable, ok := unparenExpr(expr).(*luaparser.VarExpr)
	if !ok || fn == nil {
		return nil, false
	}
	return e.nameInFunction(fn, variable.Name)
}

func (e *mode0NumEval) nameInFunction(fn *luaparser.FunctionDeclStmt, name string) ([]float64, bool) {
	if fn == nil || name == "" {
		return nil, false
	}
	for i, param := range fn.Params {
		if param == name {
			return e.callSiteArg(fn, i)
		}
	}
	return e.assignedInFunction(fn, name)
}

func (e *mode0NumEval) callSiteArg(fn *luaparser.FunctionDeclStmt, index int) ([]float64, bool) {
	if e.chunk == nil || e.graph == nil || fn == nil || index < 0 {
		return nil, false
	}
	key := functionKey(fn)
	var values []float64
	found := false
	ok := true
	var visit func(luaparser.Stmt)
	visit = func(stmt luaparser.Stmt) {
		if stmt == nil || !ok {
			return
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			if !ok {
				return
			}
			call, isCall := expr.(*luaparser.CallExpr)
			if !isCall {
				return
			}
			name, named := callKey(call)
			if !named || name != key || index >= len(call.Args) {
				return
			}
			nums, proven := e.nums(call.Args[index], nil)
			if !proven {
				if lo, hi, bounded := e.interval(call.Args[index], nil); bounded {
					nums, proven = e.intervalInts(lo, hi, true)
				}
			}
			if !proven {
				if field, isField := unparenExpr(call.Args[index]).(*luaparser.FieldExpr); isField && e.fieldsVisiting[field.Name] {
					return
				}
				ok = false
				return
			}
			merged, mergedOK := mergeMode0Nums(values, nums)
			if !mergedOK {
				ok = false
				return
			}
			values = merged
			found = true
		})
		switch s := stmt.(type) {
		case *luaparser.FunctionDeclStmt:
			if s == fn {
				return
			}
			for _, body := range s.Body.Body {
				visit(body)
			}
		case *luaparser.DoStmt:
			for _, body := range s.Body.Body {
				visit(body)
			}
		case *luaparser.IfStmt:
			for _, body := range s.Then.Body {
				visit(body)
			}
			for _, branch := range s.ElseIf {
				for _, body := range branch.Body.Body {
					visit(body)
				}
			}
			if s.Else != nil {
				for _, body := range s.Else.Body {
					visit(body)
				}
			}
		case *luaparser.NumericForStmt, *luaparser.GenericForStmt, *luaparser.WhileStmt, *luaparser.RepeatStmt:
			var body *luaparser.Block
			switch loop := s.(type) {
			case *luaparser.NumericForStmt:
				body = loop.Body
			case *luaparser.GenericForStmt:
				body = loop.Body
			case *luaparser.WhileStmt:
				body = loop.Body
			case *luaparser.RepeatStmt:
				body = loop.Body
			}
			if body != nil {
				for _, nested := range body.Body {
					visit(nested)
				}
			}
		}
	}
	for _, stmt := range e.chunk.Body {
		visit(stmt)
	}
	return values, ok && found
}

func (e *mode0NumEval) assignedInFunction(fn *luaparser.FunctionDeclStmt, name string) ([]float64, bool) {
	if fn == nil || fn.Body == nil {
		return nil, false
	}
	var values []float64
	found := false
	ok := true
	var visit func(*luaparser.Block)
	visit = func(block *luaparser.Block) {
		if block == nil || !ok {
			return
		}
		for _, stmt := range block.Body {
			switch s := stmt.(type) {
			case *luaparser.LocalAssignStmt:
				for i, local := range s.Names {
					if local != name || i >= len(s.Values) {
						continue
					}
					nums, proven := e.nums(s.Values[i], nil)
					if !proven {
						ok = false
						return
					}
					merged, mergedOK := mergeMode0Nums(values, nums)
					if !mergedOK {
						ok = false
						return
					}
					values = merged
					found = true
				}
			case *luaparser.AssignStmt:
				for i, target := range s.Targets {
					variable, isVar := unparenExpr(target).(*luaparser.VarExpr)
					if !isVar || variable.Name != name || i >= len(s.Values) {
						continue
					}
					nums, proven := e.nums(s.Values[i], nil)
					if !proven {
						ok = false
						return
					}
					merged, mergedOK := mergeMode0Nums(values, nums)
					if !mergedOK {
						ok = false
						return
					}
					values = merged
					found = true
				}
			case *luaparser.IfStmt:
				visit(s.Then)
				for _, branch := range s.ElseIf {
					visit(branch.Body)
				}
				visit(s.Else)
			case *luaparser.DoStmt:
				visit(s.Body)
			case *luaparser.NumericForStmt:
				visit(s.Body)
			case *luaparser.GenericForStmt:
				visit(s.Body)
			case *luaparser.WhileStmt:
				visit(s.Body)
			case *luaparser.RepeatStmt:
				visit(s.Body)
			}
		}
	}
	visit(fn.Body)
	return values, ok && found
}

func mode0FieldIncrement(field string, rhs luaparser.Expr) (float64, luaparser.Operator, bool) {
	bin, ok := unparenExpr(rhs).(*luaparser.BinaryExpr)
	if !ok || bin.Op != luaparser.OpAdd && bin.Op != luaparser.OpSubtract {
		return 0, "", false
	}
	match := func(side, other luaparser.Expr) (float64, bool) {
		fieldExpr, isField := unparenExpr(side).(*luaparser.FieldExpr)
		if !isField || fieldExpr.Name != field {
			return 0, false
		}
		value, literal := staticNumberLiteral(other)
		return value, literal
	}
	if value, ok := match(bin.Left, bin.Right); ok {
		return value, bin.Op, true
	}
	if value, ok := match(bin.Right, bin.Left); ok && bin.Op == luaparser.OpAdd {
		return value, bin.Op, true
	}
	return 0, "", false
}

func mode0BoundLiterals(expr luaparser.Expr, field string, aliases map[string]bool) []float64 {
	var values []float64
	var walk func(luaparser.Expr)
	walk = func(node luaparser.Expr) {
		bin, ok := unparenExpr(node).(*luaparser.BinaryExpr)
		if !ok {
			return
		}
		if bin.Op == luaparser.OpOr || bin.Op == luaparser.OpAnd {
			walk(bin.Left)
			walk(bin.Right)
			return
		}
		if bin.Op != luaparser.OpGreater && bin.Op != luaparser.OpGreaterEq &&
			bin.Op != luaparser.OpLess && bin.Op != luaparser.OpLessEq {
			return
		}
		named := func(side luaparser.Expr) bool {
			if fieldExpr, ok := unparenExpr(side).(*luaparser.FieldExpr); ok && fieldExpr.Name == field {
				return true
			}
			variable, ok := unparenExpr(side).(*luaparser.VarExpr)
			return ok && aliases[variable.Name]
		}
		if named(bin.Left) {
			if value, literal := staticNumberLiteral(bin.Right); literal {
				values = append(values, value)
			}
			return
		}
		if named(bin.Right) {
			if value, literal := staticNumberLiteral(bin.Left); literal {
				values = append(values, value)
			}
		}
	}
	walk(expr)
	return values
}

func (e *mode0NumEval) interval(expr luaparser.Expr, env mode0NumEnv) (float64, float64, bool) {
	if values, ok := e.nums(expr, env); ok && len(values) > 0 {
		return values[0], values[len(values)-1], true
	}
	expr = unparenExpr(expr)
	switch n := expr.(type) {
	case *luaparser.CallExpr:
		name, ok := callKey(n)
		if !ok {
			return 0, 0, false
		}
		switch name {
		case "sin", "cos":
			return -1, 1, true
		case "rnd":
			if len(n.Args) == 0 {
				return 0, 1, true
			}
			hi, ok := e.nums(n.Args[0], env)
			if !ok || len(hi) != 1 || hi[0] < 0 {
				return 0, 0, false
			}
			return 0, hi[0], true
		case "flr":
			lo, hi, ok := e.interval(n.Args[0], env)
			if !ok {
				return 0, 0, false
			}
			return math.Floor(lo), math.Floor(hi), true
		case "ceil":
			lo, hi, ok := e.interval(n.Args[0], env)
			if !ok {
				return 0, 0, false
			}
			return math.Ceil(lo), math.Ceil(hi), true
		case "min", "max":
			if len(n.Args) != 2 {
				return 0, 0, false
			}
			loA, hiA, okA := e.interval(n.Args[0], env)
			loB, hiB, okB := e.interval(n.Args[1], env)
			if !okA || !okB {
				return 0, 0, false
			}
			if name == "max" {
				return math.Max(loA, loB), math.Max(hiA, hiB), true
			}
			return math.Min(loA, loB), math.Min(hiA, hiB), true
		}
		if _, builtin := Builtins[name]; !builtin {
			if lo, hi, ok := e.userInterval(name); ok {
				return lo, hi, true
			}
		}
	case *luaparser.BinaryExpr:
		if n.Op == luaparser.OpModulo {
			right, ok := e.nums(n.Right, env)
			if ok && len(right) == 1 && right[0] > 0 {
				return 0, right[0], true
			}
		}
		loL, hiL, okL := e.interval(n.Left, env)
		loR, hiR, okR := e.interval(n.Right, env)
		if !okL || !okR {
			return 0, 0, false
		}
		switch n.Op {
		case luaparser.OpAdd:
			return loL + loR, hiL + hiR, true
		case luaparser.OpSubtract:
			return loL - hiR, hiL - loR, true
		case luaparser.OpMultiply:
			cands := []float64{loL * loR, loL * hiR, hiL * loR, hiL * hiR}
			sort.Float64s(cands)
			return cands[0], cands[3], true
		}
	case *luaparser.UnaryExpr:
		if n.Op == luaparser.OpNegate {
			lo, hi, ok := e.interval(n.Expr, env)
			if !ok {
				return 0, 0, false
			}
			return -hi, -lo, true
		}
	}
	return 0, 0, false
}

func (e *mode0NumEval) intervalInts(lo, hi float64, closedHigh bool) ([]float64, bool) {
	if math.IsNaN(lo) || math.IsNaN(hi) || math.IsInf(lo, 0) || math.IsInf(hi, 0) || hi < lo {
		return nil, false
	}
	start := math.Floor(lo)
	end := math.Floor(hi)
	if !closedHigh && hi == math.Trunc(hi) {
		end--
	}
	if end < start || end-start+1 > float64(mode0MaxFiniteNums) {
		return nil, false
	}
	out := make([]float64, 0, int(end-start)+1)
	for value := start; value <= end; value++ {
		out = append(out, value)
	}
	return out, true
}

func (e *mode0NumEval) spriteIDs(expr luaparser.Expr, env mode0NumEnv) ([]int, bool, string) {
	if e.analysis != nil {
		if ids, proven, _ := e.analysis.finiteIDs(expr); proven {
			return ids, true, ""
		}
	}
	values, ok := e.nums(expr, env)
	if !ok {
		if lo, hi, bounded := e.interval(expr, env); bounded {
			values, ok = e.intervalInts(lo, hi, true)
		}
	}
	if !ok {
		return nil, false, "the id expression is not a finite integer set"
	}
	ids, ok := mode0IntsFromNums(values)
	if !ok {
		return nil, false, "the id expression is not a finite integer set"
	}
	legal := make([]int, 0, len(ids))
	for _, id := range ids {
		if id >= 0 {
			legal = append(legal, id)
		}
	}
	if len(legal) == 0 {
		return nil, false, "the id expression is not a finite integer set"
	}
	return legal, true, ""
}

func (e *mode0NumEval) userInterval(name string) (float64, float64, bool) {
	if e.graph == nil {
		return 0, 0, false
	}
	fns := e.graph.functions[name]
	if len(fns) == 0 {
		return 0, 0, false
	}
	for _, fn := range fns {
		if !mode0FunctionReturnsUnitTrig(fn) {
			return 0, 0, false
		}
	}
	return -1, 1, true
}

func mode0FunctionReturnsUnitTrig(fn *luaparser.FunctionDeclStmt) bool {
	if fn == nil {
		return false
	}
	found, only := false, true
	var walk func(*luaparser.Block)
	walk = func(block *luaparser.Block) {
		if block == nil || !only {
			return
		}
		for _, stmt := range block.Body {
			switch s := stmt.(type) {
			case *luaparser.ReturnStmt:
				if len(s.Values) == 0 {
					only = false
					return
				}
				call, ok := unparenExpr(s.Values[0]).(*luaparser.CallExpr)
				if !ok {
					only = false
					return
				}
				name, named := callKey(call)
				if !named || name != "sin" && name != "cos" {
					only = false
					return
				}
				found = true
			case *luaparser.IfStmt:
				walk(s.Then)
				for _, branch := range s.ElseIf {
					walk(branch.Body)
				}
				walk(s.Else)
			case *luaparser.DoStmt:
				walk(s.Body)
			}
		}
	}
	walk(fn.Body)
	return found && only
}

func mode0EqualityLiterals(fn *luaparser.FunctionDeclStmt, name string) []float64 {
	if fn == nil || name == "" {
		return nil
	}
	var values []float64
	var walkExpr func(luaparser.Expr)
	walkExpr = func(expr luaparser.Expr) {
		bin, ok := unparenExpr(expr).(*luaparser.BinaryExpr)
		if !ok {
			return
		}
		if bin.Op == luaparser.OpOr || bin.Op == luaparser.OpAnd {
			walkExpr(bin.Left)
			walkExpr(bin.Right)
			return
		}
		if bin.Op != luaparser.OpEqual {
			return
		}
		match := func(side, other luaparser.Expr) {
			variable, ok := unparenExpr(side).(*luaparser.VarExpr)
			if !ok || variable.Name != name {
				return
			}
			if value, literal := staticNumberLiteral(other); literal {
				values = append(values, value)
			}
		}
		match(bin.Left, bin.Right)
		match(bin.Right, bin.Left)
	}
	var walk func(*luaparser.Block)
	walk = func(block *luaparser.Block) {
		if block == nil {
			return
		}
		for _, stmt := range block.Body {
			switch s := stmt.(type) {
			case *luaparser.IfStmt:
				walkExpr(s.Condition)
				walk(s.Then)
				for _, branch := range s.ElseIf {
					walkExpr(branch.Condition)
					walk(branch.Body)
				}
				walk(s.Else)
			case *luaparser.DoStmt:
				walk(s.Body)
			}
		}
	}
	walk(fn.Body)
	merged, ok := mergeMode0Nums(values)
	if !ok {
		return nil
	}
	return merged
}

type mode0NameBound struct {
	hasLo  bool
	hasHi  bool
	lo     float64
	hi     float64
	equals []float64
}

func (e *mode0NumEval) applyGuardComplement(env mode0NumEnv, cond luaparser.Expr) mode0NumEnv {
	next := cloneMode0NumEnv(env)
	for name, bound := range mode0GuardComplement(cond) {
		if bound == nil {
			continue
		}
		if len(bound.equals) > 0 {
			if existing, ok := next[name]; ok {
				filtered, ok := mode0IntersectNums(existing, bound.equals)
				if !ok {
					delete(next, name)
					continue
				}
				next[name] = filtered
				continue
			}
			next[name] = append([]float64(nil), bound.equals...)
			continue
		}
		if bound.hasLo && bound.hasHi {
			filled, ok := e.intervalInts(bound.lo, bound.hi, true)
			if !ok {
				delete(next, name)
				continue
			}
			if existing, ok := next[name]; ok {
				filtered, ok := mode0IntersectNums(existing, filled)
				if !ok {
					delete(next, name)
					continue
				}
				next[name] = filtered
				continue
			}
			next[name] = filled
			continue
		}
		if existing, ok := next[name]; ok {
			filtered := make([]float64, 0, len(existing))
			for _, value := range existing {
				if bound.hasLo && value < bound.lo {
					continue
				}
				if bound.hasHi && value > bound.hi {
					continue
				}
				filtered = append(filtered, value)
			}
			if len(filtered) == 0 {
				delete(next, name)
				continue
			}
			next[name] = filtered
		}
	}
	return next
}

func mode0IntersectNums(left, right []float64) ([]float64, bool) {
	seen := map[float64]bool{}
	for _, value := range right {
		seen[mode0NumKey(value)] = true
	}
	out := make([]float64, 0, len(left))
	for _, value := range left {
		if seen[mode0NumKey(value)] {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return mergeMode0Nums(out)
}

func mode0GuardComplement(cond luaparser.Expr) map[string]*mode0NameBound {
	if name, values, ok := mode0AndNotEqualSet(cond); ok {
		return map[string]*mode0NameBound{name: {equals: values}}
	}
	bounds := map[string]*mode0NameBound{}
	found := false
	for _, atom := range mode0FlattenOp(cond, luaparser.OpOr) {
		name, lo, hi, hasLo, hasHi, ok := mode0NotComparison(atom)
		if !ok {
			continue
		}
		found = true
		bound := bounds[name]
		if bound == nil {
			bound = &mode0NameBound{}
			bounds[name] = bound
		}
		if hasLo && (!bound.hasLo || lo > bound.lo) {
			bound.hasLo = true
			bound.lo = lo
		}
		if hasHi && (!bound.hasHi || hi < bound.hi) {
			bound.hasHi = true
			bound.hi = hi
		}
	}
	if !found {
		return nil
	}
	return bounds
}

func mode0AndNotEqualSet(cond luaparser.Expr) (string, []float64, bool) {
	var name string
	var values []float64
	for _, atom := range mode0FlattenOp(cond, luaparser.OpAnd) {
		n, lit, ok := mode0NotEqualVarLit(atom)
		if !ok {
			return "", nil, false
		}
		if name == "" {
			name = n
		} else if name != n {
			return "", nil, false
		}
		values = append(values, lit)
	}
	if name == "" {
		return "", nil, false
	}
	merged, ok := mergeMode0Nums(values)
	return name, merged, ok && len(merged) > 0
}

func mode0FlattenOp(expr luaparser.Expr, op luaparser.Operator) []luaparser.Expr {
	expr = unparenExpr(expr)
	bin, ok := expr.(*luaparser.BinaryExpr)
	if !ok || bin.Op != op {
		if expr == nil {
			return nil
		}
		return []luaparser.Expr{expr}
	}
	return append(mode0FlattenOp(bin.Left, op), mode0FlattenOp(bin.Right, op)...)
}

func mode0NotEqualVarLit(expr luaparser.Expr) (string, float64, bool) {
	bin, ok := unparenExpr(expr).(*luaparser.BinaryExpr)
	if !ok || bin.Op != luaparser.OpNotEqual {
		return "", 0, false
	}
	if name, lit, ok := mode0VarLit(bin.Left, bin.Right); ok {
		return name, lit, true
	}
	return mode0VarLit(bin.Right, bin.Left)
}

func mode0VarLit(nameExpr, litExpr luaparser.Expr) (string, float64, bool) {
	variable, ok := unparenExpr(nameExpr).(*luaparser.VarExpr)
	if !ok {
		return "", 0, false
	}
	lit, ok := staticNumberLiteral(litExpr)
	if !ok {
		return "", 0, false
	}
	return variable.Name, lit, true
}

func mode0NotComparison(expr luaparser.Expr) (string, float64, float64, bool, bool, bool) {
	bin, ok := unparenExpr(expr).(*luaparser.BinaryExpr)
	if !ok {
		return "", 0, 0, false, false, false
	}
	name, lit, op, ok := mode0VarCmpLit(bin)
	if !ok || !isMode0Int(lit) {
		return "", 0, 0, false, false, false
	}
	switch op {
	case luaparser.OpLess:
		return name, lit, 0, true, false, true
	case luaparser.OpLessEq:
		return name, lit + 1, 0, true, false, true
	case luaparser.OpGreater:
		return name, 0, lit, false, true, true
	case luaparser.OpGreaterEq:
		return name, 0, lit - 1, false, true, true
	default:
		return "", 0, 0, false, false, false
	}
}

func mode0VarCmpLit(bin *luaparser.BinaryExpr) (string, float64, luaparser.Operator, bool) {
	if bin == nil {
		return "", 0, "", false
	}
	if name, lit, ok := mode0VarLit(bin.Left, bin.Right); ok {
		return name, lit, bin.Op, true
	}
	name, lit, ok := mode0VarLit(bin.Right, bin.Left)
	if !ok {
		return "", 0, "", false
	}
	switch bin.Op {
	case luaparser.OpLess:
		return name, lit, luaparser.OpGreater, true
	case luaparser.OpLessEq:
		return name, lit, luaparser.OpGreaterEq, true
	case luaparser.OpGreater:
		return name, lit, luaparser.OpLess, true
	case luaparser.OpGreaterEq:
		return name, lit, luaparser.OpLessEq, true
	default:
		return "", 0, "", false
	}
}
