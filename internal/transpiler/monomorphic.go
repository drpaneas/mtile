package transpiler

import "github.com/drpaneas/mtile/internal/luaparser"

func (g *generator) analyzeMonomorphicNumbers(poisonUnknown bool) {
	if g.opts.DisableMonomorphicNumbers {
		return
	}
	// Top-level declarations own Game fields. Their first source assignment is
	// the type observation used by the monomorphic contract.
	seenGlobal := make(map[string]bool)
	for _, stmt := range g.chunk.Body {
		switch s := stmt.(type) {
		case *luaparser.LocalAssignStmt:
			for i, name := range s.Names {
				if seenGlobal[name] {
					continue
				}
				seenGlobal[name] = true
				if i < len(s.Values) && g.numericInitializerIntent(s.Values[i]) {
					g.globalKinds[name] = monoNumber
				}
			}
		case *luaparser.AssignStmt:
			for i, target := range s.Targets {
				v, ok := target.(*luaparser.VarExpr)
				if !ok || seenGlobal[v.Name] {
					continue
				}
				seenGlobal[v.Name] = true
				if i < len(s.Values) && g.numericInitializerIntent(s.Values[i]) {
					g.globalKinds[v.Name] = monoNumber
				}
			}
		}
	}

	// Record fields retain Value storage, but a field name whose observed table
	// literal initializers and named-field writes are all numeric can use native
	// Number reads/writes. Resolve initializer aliases in their lexical scope.
	// local or parameter must hide a same-named numeric global. Keep provisional
	// field observations out of expression inference until every constructor
	// has been visited: a later mixed initializer may invalidate an early one.
	fields := make(map[string]bool)
	observe := func(name string, isNumber bool) {
		if previous, seen := fields[name]; !seen || previous {
			fields[name] = isNumber
		}
	}
	params := make(map[*functionInfo][]functionParamObservation)
	top := &monomorphicAnalysisEnv{g: g, observeField: observe, poisonUnknownFields: poisonUnknown}
	top.push()
	for _, stmt := range g.chunk.Body {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			env := &monomorphicAnalysisEnv{g: g, observeField: observe, poisonUnknownFields: poisonUnknown}
			env.push()
			info := g.funcs[functionKey(fn)]
			for i, name := range fn.Params {
				kind := monoValue
				if info != nil && i < len(info.paramKinds) {
					kind = info.paramKinds[i]
				}
				env.declare(name, kind)
			}
			for _, bodyStmt := range fn.Body.Body {
				g.analyzeMonomorphicStmt(bodyStmt, env, params, nil, false)
			}
			continue
		}
		g.analyzeMonomorphicStmt(stmt, top, params, nil, true)
	}
	g.numericFields = fields
}

func numericFieldsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for name, isNumber := range a {
		if b[name] != isNumber {
			return false
		}
	}
	return true
}

func cloneNumericFields(fields map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(fields))
	for name, isNumber := range fields {
		cloned[name] = isNumber
	}
	return cloned
}

func (g *generator) resetMonomorphicSignatures() {
	for _, fn := range g.funcList {
		for i := range fn.paramKinds {
			fn.paramKinds[i] = monoValue
		}
		for i := range fn.returnKinds {
			fn.returnKinds[i] = monoValue
		}
		fn.coercedNumberParams = nil
	}
}

// refineMonomorphicFieldSignatures closes the loop between function signatures
// and named-field observations. Untyped aliases are skipped until a later
// confirmation pass poisons any initializer that is still not a number, so a
// numeric parameter constructor can keep native fields without letting a
// string parameter keep a provisional numeric type.
func (g *generator) refineMonomorphicFieldSignatures() {
	if g.opts.DisableMonomorphicNumbers {
		return
	}
	for pass := 0; pass < 32; pass++ {
		before := cloneNumericFields(g.numericFields)
		g.analyzeMonomorphicNumbers(false)
		if !g.opts.DisableMonomorphicFunctions {
			g.analyzeMonomorphicFunctions()
		}
		if numericFieldsEqual(before, g.numericFields) {
			break
		}
	}
	// Confirm the provisional facts without discarding numeric read/write
	// aliases. Confirmation can only remove candidates: each retry removes at
	// least one numeric field, so N candidates require at most N+1 passes.
	// Recompute signatures after each removal so a function cannot carry a
	// stale numeric field type into another field through a parameter or return.
	candidates := cloneNumericFields(g.numericFields)
	for {
		g.analyzeMonomorphicNumbers(true)
		changed := false
		for name, isNumber := range candidates {
			if isNumber && !g.numericFields[name] {
				candidates[name] = false
				changed = true
			}
		}
		g.numericFields = cloneNumericFields(candidates)
		if !changed {
			return
		}
		g.resetMonomorphicSignatures()
		g.analyzeMonomorphicFunctions()
	}
}

func (g *generator) numericInitializerIntent(expr luaparser.Expr) bool {
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return g.numericInitializerIntent(x.Expr)
	case *luaparser.VarExpr:
		// The first pass observes chunk-scope declarations in source order, so
		// direct aliases of an already-proven numeric constant can keep native
		// Number storage. Forward references conservatively remain Values.
		return g.globalKinds[x.Name] == monoNumber
	default:
		return numericIntent(expr)
	}
}

// observeNamedField records constructor-style evidence from `t.name = value`.
// It uses the previous analysis pass for field reads, never the in-progress
// map, so a later mixed store can still poison a provisional numeric name.
func (e *monomorphicAnalysisEnv) observeNamedField(name string, value luaparser.Expr, kind monoKind) {
	if e.observeField == nil || name == "" {
		return
	}
	if kind == monoNumber || (value != nil && e.numericFieldIntent(value)) {
		e.observeField(name, true)
		return
	}
	if value != nil && (provenNonNumericFieldValue(value) || e.poisonUnknownFields && poisonUnknownFieldValue(value)) {
		e.observeField(name, false)
		return
	}
	if value == nil && e.poisonUnknownFields {
		e.observeField(name, false)
	}
}

func (e *monomorphicAnalysisEnv) numericFieldIntent(expr luaparser.Expr) bool {
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return e.numericFieldIntent(x.Expr)
	case *luaparser.VarExpr:
		return e.lookup(x.Name) == monoNumber
	case *luaparser.FieldExpr:
		return !e.g.opts.DisableStableTableFields && e.g.numericFields[x.Name]
	case *luaparser.CallExpr:
		if fn := e.g.calledFunction(x); fn != nil {
			return fn.returnN > 0 && fn.returnKind(0) == monoNumber
		}
		return numericIntent(expr)
	default:
		return numericIntent(expr)
	}
}

func provenNonNumericFieldValue(expr luaparser.Expr) bool {
	switch x := unparenExpr(expr).(type) {
	case *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.StringExpr, *luaparser.TableExpr:
		return true
	case *luaparser.UnaryExpr:
		return x.Op == luaparser.OpNot
	case *luaparser.BinaryExpr:
		switch x.Op {
		case luaparser.OpEqual, luaparser.OpNotEqual, luaparser.OpLess, luaparser.OpLessEq,
			luaparser.OpGreater, luaparser.OpGreaterEq, luaparser.OpConcat:
			return true
		}
	}
	return false
}

func poisonUnknownFieldValue(expr luaparser.Expr) bool {
	switch unparenExpr(expr).(type) {
	case *luaparser.FieldExpr, *luaparser.IndexExpr:
		// A read of another table's field is not proof this name is non-numeric.
		// Confirmation still poisons untyped aliases and non-numeric literals.
		return false
	default:
		return true
	}
}

func numericIntent(expr luaparser.Expr) bool {
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return numericIntent(x.Expr)
	case *luaparser.NumberExpr:
		return true
	case *luaparser.UnaryExpr:
		return x.Op == luaparser.OpNegate || x.Op == luaparser.OpBitNot
	case *luaparser.BinaryExpr:
		switch x.Op {
		case luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
			luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo, luaparser.OpPower,
			luaparser.OpBitOr, luaparser.OpBitXor, luaparser.OpBitAnd,
			luaparser.OpShiftL, luaparser.OpShiftR, luaparser.OpShiftRLogical,
			luaparser.OpRotateL, luaparser.OpRotateR:
			return true
		}
	case *luaparser.CallExpr:
		name, ok := callKey(x)
		if !ok {
			return false
		}
		switch name {
		case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd",
			"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr", "t", "time", "stat", "mget":
			return true
		}
	}
	return false
}

type functionParamObservation struct {
	seen      bool
	allNumber bool
}

type functionReturnObservation struct {
	seen      bool
	allNumber []bool
}

type monomorphicAnalysisEnv struct {
	g                   *generator
	scopes              []map[string]monoKind
	observeField        func(name string, isNumber bool)
	poisonUnknownFields bool
}

func (e *monomorphicAnalysisEnv) push() {
	e.scopes = append(e.scopes, make(map[string]monoKind))
}

func (e *monomorphicAnalysisEnv) pop() {
	e.scopes = e.scopes[:len(e.scopes)-1]
}

func (e *monomorphicAnalysisEnv) declare(name string, kind monoKind) {
	e.scopes[len(e.scopes)-1][name] = kind
}

func (e *monomorphicAnalysisEnv) lookup(name string) monoKind {
	for i := len(e.scopes) - 1; i >= 0; i-- {
		if kind, ok := e.scopes[i][name]; ok {
			return kind
		}
	}
	return e.g.globalKinds[name]
}

// analyzeMonomorphicFunctions computes a closed-world numeric signature for
// direct user functions. Mtile rejects closures and first-class function
// values, so every supported call target is statically known. A parameter is
// promoted only when every source call site supplies a proven Number. Each
// return position is promoted only when every path supplies a number in
// that position and the function cannot fall through. Unknown or mixed slots
// retain rt.Value independently.
func (g *generator) analyzeMonomorphicFunctions() {
	if g.opts.DisableMonomorphicNumbers || g.opts.DisableMonomorphicFunctions {
		return
	}
	for {
		params := make(map[*functionInfo][]functionParamObservation, len(g.funcList))
		returns := make(map[*functionInfo]*functionReturnObservation, len(g.funcList))
		for _, fn := range g.funcList {
			params[fn] = make([]functionParamObservation, len(fn.params))
			for i := range params[fn] {
				params[fn][i].allNumber = true
			}
			returns[fn] = &functionReturnObservation{allNumber: make([]bool, fn.returnN)}
			for i := range returns[fn].allNumber {
				returns[fn].allNumber[i] = true
			}
		}

		top := &monomorphicAnalysisEnv{g: g}
		top.push()
		for _, stmt := range g.chunk.Body {
			if _, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
				continue
			}
			g.analyzeMonomorphicStmt(stmt, top, params, nil, true)
		}

		for _, fn := range g.funcList {
			env := &monomorphicAnalysisEnv{g: g}
			env.push()
			for i, name := range fn.params {
				env.declare(name, fn.paramKinds[i])
			}
			for _, stmt := range fn.decl.Body.Body {
				g.analyzeMonomorphicStmt(stmt, env, params, returns[fn], false)
			}
		}

		changed := false
		for _, fn := range g.funcList {
			for i, observation := range params[fn] {
				demanded := observation.seen && g.parameterAlwaysCoerced(fn, i)
				if demanded && !fn.coercesNumberParam(i) {
					if fn.coercedNumberParams == nil {
						fn.coercedNumberParams = make([]bool, len(fn.params))
					}
					fn.coercedNumberParams[i] = true
					changed = true
				}
				if fn.paramKinds[i] == monoValue && observation.seen && (observation.allNumber || demanded) {
					fn.paramKinds[i] = monoNumber
					changed = true
				}
			}
			observation := returns[fn]
			if !observation.seen || !blockAlwaysReturns(fn.decl.Body) || (fn.returnN > 1 && g.opts.DisableMonomorphicMultiReturns) {
				continue
			}
			for i, allNumber := range observation.allNumber {
				if fn.returnKinds[i] == monoValue && allNumber {
					fn.returnKinds[i] = monoNumber
					changed = true
				}
			}
		}
		if !changed {
			return
		}
	}
}

func (g *generator) analyzeMonomorphicStmt(stmt luaparser.Stmt, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation, returns *functionReturnObservation, topLevel bool) {
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		for _, target := range s.Targets {
			switch target := target.(type) {
			case *luaparser.FieldExpr:
				g.analyzeMonomorphicExpr(target.Base, env, params)
			case *luaparser.IndexExpr:
				g.analyzeMonomorphicExpr(target.Base, env, params)
				g.analyzeMonomorphicExpr(target.Key, env, params)
			}
		}
		kinds := g.analyzeMonomorphicExprList(s.Values, env, params)
		for i, target := range s.Targets {
			field, ok := target.(*luaparser.FieldExpr)
			if !ok {
				continue
			}
			var value luaparser.Expr
			if i < len(s.Values) {
				value = s.Values[i]
			}
			kind := monoValue
			if i < len(kinds) {
				kind = kinds[i]
			}
			env.observeNamedField(field.Name, value, kind)
		}
	case *luaparser.LocalAssignStmt:
		kinds := g.analyzeMonomorphicValues(s.Values, len(s.Names), env, params)
		if !topLevel {
			for i, name := range s.Names {
				kind := monoValue
				if i < len(kinds) {
					kind = kinds[i]
				}
				env.declare(name, kind)
			}
		}
	case *luaparser.CallStmt:
		g.analyzeMonomorphicExpr(s.Call, env, params)
	case *luaparser.IfStmt:
		g.analyzeMonomorphicExpr(s.Condition, env, params)
		g.analyzeMonomorphicBlock(s.Then, env, params, returns)
		for _, branch := range s.ElseIf {
			g.analyzeMonomorphicExpr(branch.Condition, env, params)
			g.analyzeMonomorphicBlock(branch.Body, env, params, returns)
		}
		if s.Else != nil {
			g.analyzeMonomorphicBlock(s.Else, env, params, returns)
		}
	case *luaparser.WhileStmt:
		g.analyzeMonomorphicExpr(s.Condition, env, params)
		g.analyzeMonomorphicBlock(s.Body, env, params, returns)
	case *luaparser.RepeatStmt:
		env.push()
		for _, bodyStmt := range s.Body.Body {
			g.analyzeMonomorphicStmt(bodyStmt, env, params, returns, false)
		}
		g.analyzeMonomorphicExpr(s.Condition, env, params)
		env.pop()
	case *luaparser.DoStmt:
		g.analyzeMonomorphicBlock(s.Body, env, params, returns)
	case *luaparser.NumericForStmt:
		initKind := g.analyzeMonomorphicExpr(s.Init, env, params)
		limitKind := g.analyzeMonomorphicExpr(s.Limit, env, params)
		stepKind := g.analyzeMonomorphicExpr(s.Step, env, params)
		env.push()
		kind := monoValue
		if initKind == monoNumber && limitKind == monoNumber && stepKind == monoNumber && !numericForVariableReassigned(s) {
			kind = monoNumber
		}
		env.declare(s.Name, kind)
		g.analyzeMonomorphicBlock(s.Body, env, params, returns)
		env.pop()
	case *luaparser.GenericForStmt:
		for _, value := range s.Values {
			g.analyzeMonomorphicExpr(value, env, params)
		}
		env.push()
		for _, name := range s.Names {
			env.declare(name, monoValue)
		}
		g.analyzeMonomorphicBlock(s.Body, env, params, returns)
		env.pop()
	case *luaparser.ReturnStmt:
		n := 0
		if returns != nil {
			n = len(returns.allNumber)
		}
		kinds := g.analyzeMonomorphicValues(s.Values, n, env, params)
		if returns != nil {
			returns.seen = true
			for i, kind := range kinds {
				if kind != monoNumber {
					returns.allNumber[i] = false
				}
			}
		}
	}
}

func (g *generator) analyzeMonomorphicBlock(block *luaparser.Block, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation, returns *functionReturnObservation) {
	env.push()
	for _, stmt := range block.Body {
		g.analyzeMonomorphicStmt(stmt, env, params, returns, false)
	}
	env.pop()
}

// analyzeMonomorphicValues applies Lua's expression-list width rules while
// retaining the inferred kind of each expanded result. A trailing call expands
// to its declared result arity; missing positions are nil and therefore remain
// Value.
func (g *generator) analyzeMonomorphicValues(values []luaparser.Expr, nWanted int, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation) []monoKind {
	out := make([]monoKind, nWanted)
	kinds := g.analyzeMonomorphicExprList(values, env, params)
	for i := 0; i < len(kinds) && i < len(out); i++ {
		out[i] = kinds[i]
	}
	return out
}

func (g *generator) analyzeMonomorphicExprList(values []luaparser.Expr, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation) []monoKind {
	out := make([]monoKind, 0, len(values))
	last := len(values) - 1
	for i, value := range values {
		kind := g.analyzeMonomorphicExpr(value, env, params)
		if i == last {
			if call, ok := value.(*luaparser.CallExpr); ok {
				if arity, known := g.callResultArity(call); known {
					if arity == 0 {
						continue
					}
					if fn := g.calledFunction(call); fn != nil && arity > 1 {
						for j := 0; j < fn.returnN; j++ {
							out = append(out, fn.returnKind(j))
						}
						continue
					}
				}
			}
		}
		out = append(out, kind)
	}
	return out
}

func (g *generator) analyzeMonomorphicExpr(expr luaparser.Expr, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation) monoKind {
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return g.analyzeMonomorphicExpr(x.Expr, env, params)
	case *luaparser.NumberExpr:
		return monoNumber
	case *luaparser.VarExpr:
		return env.lookup(x.Name)
	case *luaparser.FieldExpr:
		g.analyzeMonomorphicExpr(x.Base, env, params)
		if _, _, ok := g.numericRecordField(x); ok {
			return monoNumber
		}
		if !g.opts.DisableStableTableFields && g.numericFields[x.Name] {
			return monoNumber
		}
	case *luaparser.IndexExpr:
		g.analyzeMonomorphicExpr(x.Key, env, params)
		if base, ok := x.Base.(*luaparser.VarExpr); ok && g.numericArrays[base.Name] != nil {
			return monoNumber
		}
		g.analyzeMonomorphicExpr(x.Base, env, params)
	case *luaparser.UnaryExpr:
		if x.Op == luaparser.OpLength {
			if base, ok := x.Expr.(*luaparser.VarExpr); ok && g.numericArrays[base.Name] != nil {
				return monoNumber
			}
		}
		kind := g.analyzeMonomorphicExpr(x.Expr, env, params)
		if x.Op == luaparser.OpLength && !g.opts.DisableNumericLengths {
			return monoNumber
		}
		if kind == monoNumber && (x.Op == luaparser.OpNegate || x.Op == luaparser.OpBitNot) {
			return monoNumber
		}
	case *luaparser.BinaryExpr:
		left := g.analyzeMonomorphicExpr(x.Left, env, params)
		right := g.analyzeMonomorphicExpr(x.Right, env, params)
		if left == monoNumber && right == monoNumber {
			switch x.Op {
			case luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
				luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo, luaparser.OpPower,
				luaparser.OpBitOr, luaparser.OpBitXor, luaparser.OpBitAnd,
				luaparser.OpShiftL, luaparser.OpShiftR, luaparser.OpShiftRLogical,
				luaparser.OpRotateL, luaparser.OpRotateR:
				return monoNumber
			}
		}
	case *luaparser.CallExpr:
		kinds := make([]monoKind, 0, len(x.Args)+1)
		if x.Method != "" {
			kinds = append(kinds, g.analyzeMonomorphicExpr(x.Callee, env, params))
		}
		kinds = append(kinds, g.analyzeMonomorphicExprList(x.Args, env, params)...)
		if fn := g.calledFunction(x); fn != nil {
			observations := params[fn]
			for i := range observations {
				observations[i].seen = true
				if i >= len(kinds) || kinds[i] != monoNumber {
					observations[i].allNumber = false
				}
			}
			params[fn] = observations
			if fn.returnN > 0 {
				return fn.returnKind(0)
			}
			return monoValue
		}
		name, ok := callKey(x)
		if !ok {
			return monoValue
		}
		// These builtins produce a number even when an argument is a generic
		// Lua Value: min/max/mid coerce with NumberOr(0), and mget always
		// returns a tile id (0 for invalid layers/coordinates). Preserve
		// argument observations above: only the result narrows.
		if name == "min" || name == "max" || name == "mid" || name == "mget" {
			return monoNumber
		}
		allNumber := true
		for _, kind := range kinds {
			if kind != monoNumber {
				allNumber = false
				break
			}
		}
		if allNumber {
			switch name {
			case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd",
				"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr", "t", "time", "stat":
				return monoNumber
			}
		}
	case *luaparser.TableExpr:
		for _, field := range x.Fields {
			if field.Key != nil {
				g.analyzeMonomorphicExpr(field.Key, env, params)
			}
			g.analyzeMonomorphicExpr(field.Value, env, params)
			if env.observeField != nil && !field.Array {
				if key, ok := field.Key.(*luaparser.StringExpr); ok {
					if env.numericFieldIntent(field.Value) {
						env.observeField(key.Value, true)
					} else if provenNonNumericFieldValue(field.Value) ||
						env.poisonUnknownFields && poisonUnknownFieldValue(field.Value) {
						env.observeField(key.Value, false)
					}
				}
			}
		}
	}
	return monoValue
}

func blockAlwaysReturns(block *luaparser.Block) bool {
	for _, stmt := range block.Body {
		if stmtAlwaysReturns(stmt) {
			return true
		}
	}
	return false
}

func stmtAlwaysReturns(stmt luaparser.Stmt) bool {
	switch s := stmt.(type) {
	case *luaparser.ReturnStmt:
		return true
	case *luaparser.DoStmt:
		return blockAlwaysReturns(s.Body)
	case *luaparser.IfStmt:
		if s.Else == nil || !blockAlwaysReturns(s.Then) || !blockAlwaysReturns(s.Else) {
			return false
		}
		for _, branch := range s.ElseIf {
			if !blockAlwaysReturns(branch.Body) {
				return false
			}
		}
		return true
	}
	return false
}
