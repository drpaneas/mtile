package transpiler

import (
	"fmt"
	"math"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

func (g *generator) compactSheet() {
	g.sheetCompact, g.sheetIndex = assetsync.CompactSheetCells(g.opts.Sheet, g.opts.SheetW, g.spriteN)
}

func (g *generator) compactObjSheet() {
	if g.opts.ObjSheetW < 8 || g.opts.ObjSheetH < 8 {
		return
	}
	cols := g.opts.ObjSheetW / 8
	rows := g.opts.ObjSheetH / 8
	g.objSpriteN = cols * rows
	g.objSheetCompact, g.objSheetIndex = assetsync.CompactObjectSheetCells(g.opts.ObjSheet, g.opts.ObjSheetW, g.objSpriteN)
}

func functionKey(fn *luaparser.FunctionDeclStmt) string {
	key := strings.Join(fn.Name, ".")
	if fn.Method != "" {
		key += ":" + fn.Method
	}
	return key
}

func functionGoName(key string) string {
	switch key {
	case "_init":
		return "gluInit"
	case "_update":
		return "gluUpdate"
	case "_draw":
		return "gluDraw"
	default:
		return goIdent(strings.NewReplacer(".", "_", ":", "_").Replace(key))
	}
}

func (g *generator) discoverFunctions() error {
	for _, stmt := range g.chunk.Body {
		fn, ok := stmt.(*luaparser.FunctionDeclStmt)
		if !ok {
			continue
		}
		key := functionKey(fn)
		if _, exists := g.funcs[key]; exists {
			return fmt.Errorf("transpiler: duplicate function %q", key)
		}
		if fn.Vararg {
			return fmt.Errorf("transpiler: variadic function %q is unsupported", key)
		}
		info := &functionInfo{decl: fn, key: key, goName: functionGoName(key)}
		info.params = append([]string(nil), fn.Params...)
		info.paramKinds = make([]monoKind, len(fn.Params))
		// Start all known user functions at zero results. Return arity is
		// resolved later from syntax and the arities of trailing user calls.
		info.returnN = 0
		g.funcs[key] = info
		g.funcList = append(g.funcList, info)
	}
	return nil
}

// validateSemantics rejects source shapes which the static Mtile dialect
// cannot represent without silently changing Lua name binding or numeric
// behavior. Calls are intentionally static: a lexical binding may not shadow
// a user function/builtin at a call site, and callable names cannot be used as
// first-class values. Top-level locals remain Game fields, but only functions
// declared after them may capture them.
func (g *generator) validateSemantics() error {
	allTopLocals := make(map[string]luaparser.Position)
	namespaceDeclOrder := make(map[string]int)
	namespaceChildren := make(map[string]map[string]bool)
	callableMembers := make(map[string]map[string]bool)
	for order, stmt := range g.chunk.Body {
		if local, ok := stmt.(*luaparser.LocalAssignStmt); ok {
			for _, name := range local.Names {
				if previous, exists := allTopLocals[name]; exists {
					return fmt.Errorf("transpiler: %s: top-level local %q redeclaration is unsupported because the generated Game state has one slot (first declared at %s)", local.Pos(), name, previous)
				}
				allTopLocals[name] = local.Pos()
			}
		}
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok && (len(fn.Name) > 1 || fn.Method != "") {
			key := functionKey(fn)
			separator := strings.LastIndexAny(key, ".:")
			if separator > 0 && separator+1 < len(key) {
				namespace, member := key[:separator], key[separator+1:]
				// Every prefix is part of the statically lowered callable
				// namespace. Replacing a nested prefix such as actor.body after
				// declaring actor.body:draw() would change Lua dispatch while the
				// generated direct call would keep invoking the old function.
				for prefix := namespace; prefix != ""; {
					if _, exists := namespaceDeclOrder[prefix]; !exists {
						namespaceDeclOrder[prefix] = order
					}
					dot := strings.LastIndexByte(prefix, '.')
					if dot < 0 {
						break
					}
					prefix = prefix[:dot]
				}
				if callableMembers[namespace] == nil {
					callableMembers[namespace] = make(map[string]bool)
				}
				callableMembers[namespace][member] = true
			}
		}
	}
	for path := range namespaceDeclOrder {
		if dot := strings.LastIndexByte(path, '.'); dot >= 0 {
			parent, child := path[:dot], path[dot+1:]
			if namespaceChildren[parent] == nil {
				namespaceChildren[parent] = make(map[string]bool)
			}
			namespaceChildren[parent][child] = true
		}
	}

	// Static lowering does not materialize Lua function values in their tables.
	// Consequently, a table backing a function namespace must not be shared
	// through an older alias: that alias could later replace a callable field in
	// Lua while generated direct calls kept invoking the original Go function.
	// Requiring a fresh constructor at every explicitly initialized namespace
	// boundary makes that invariant syntactically provable.
	var validateNamespaceInitializer func(string, luaparser.Expr, luaparser.Position) error
	validateNamespaceInitializer = func(path string, value luaparser.Expr, position luaparser.Position) error {
		if _, namespace := namespaceDeclOrder[path]; !namespace {
			return nil
		}
		table, fresh := unparenExpr(value).(*luaparser.TableExpr)
		if !fresh {
			return fmt.Errorf("transpiler: %s: static function namespace %q must be initialized with a fresh table constructor", position, path)
		}
		children := namespaceChildren[path]
		if len(children) == 0 {
			return nil
		}
		for _, field := range table.Fields {
			if field.Array {
				continue
			}
			switch key := unparenExpr(field.Key).(type) {
			case *luaparser.StringExpr:
				if children[key.Value] {
					if err := validateNamespaceInitializer(path+"."+key.Value, field.Value, field.Value.Pos()); err != nil {
						return err
					}
				}
			case *luaparser.NumberExpr, *luaparser.BoolExpr, *luaparser.NilExpr:
				// Literal non-string keys cannot name a dotted namespace child.
			default:
				return fmt.Errorf("transpiler: %s: dynamic initializer field may alias a nested static function namespace below %q", field.Pos(), path)
			}
		}
		return nil
	}

	type lexicalScopes []map[string]bool
	currentSelfNamespace := ""
	isLocal := func(scopes lexicalScopes, name string) bool {
		for i := len(scopes) - 1; i >= 0; i-- {
			if scopes[i][name] {
				return true
			}
		}
		return false
	}
	resolveStaticPath := func(scopes lexicalScopes, path string) (string, bool) {
		if currentSelfNamespace != "" && (path == "self" || strings.HasPrefix(path, "self.")) {
			return currentSelfNamespace + strings.TrimPrefix(path, "self"), true
		}
		root := path
		if dot := strings.IndexByte(root, '.'); dot >= 0 {
			root = root[:dot]
		}
		return path, !isLocal(scopes, root)
	}
	copyScope := func(source map[string]bool) map[string]bool {
		out := make(map[string]bool, len(source))
		for name := range source {
			out[name] = true
		}
		return out
	}
	lateLocalError := func(name string, use luaparser.Position) error {
		declaration := allTopLocals[name]
		return fmt.Errorf("transpiler: %s: top-level local %q is declared after this use/function and cannot share the generated global slot (declared at %s)", use, name, declaration)
	}

	var walkExpr func(luaparser.Expr, lexicalScopes, bool, bool) error
	walkExpr = func(expr luaparser.Expr, scopes lexicalScopes, directCallee, memberBase bool) error {
		if expr == nil {
			return nil
		}
		switch x := expr.(type) {
		case *luaparser.ParenExpr:
			// Parenthesizing a callable first evaluates it as a value, which the
			// static-call dialect deliberately does not support.
			return walkExpr(x.Expr, scopes, false, false)
		case *luaparser.VarExpr:
			if x.Name == "self" && currentSelfNamespace != "" && !memberBase && !directCallee {
				return fmt.Errorf("transpiler: %s: first-class function namespace %q is unsupported", x.Pos(), currentSelfNamespace)
			}
			if isLocal(scopes, x.Name) {
				if directCallee {
					return fmt.Errorf("transpiler: %s: dynamic call through local %q is unsupported", x.Pos(), x.Name)
				}
				return nil
			}
			if _, later := allTopLocals[x.Name]; later {
				return lateLocalError(x.Name, x.Pos())
			}
			if !directCallee {
				if _, callable := g.funcs[x.Name]; callable {
					return fmt.Errorf("transpiler: %s: first-class function value %q is unsupported", x.Pos(), x.Name)
				}
				if _, callable := Builtins[x.Name]; callable {
					return fmt.Errorf("transpiler: %s: first-class builtin value %q is unsupported", x.Pos(), x.Name)
				}
				if !memberBase {
					if _, namespace := namespaceDeclOrder[x.Name]; namespace {
						return fmt.Errorf("transpiler: %s: first-class function namespace %q is unsupported", x.Pos(), x.Name)
					}
				}
			}
		case *luaparser.UnaryExpr:
			return walkExpr(x.Expr, scopes, false, false)
		case *luaparser.BinaryExpr:
			if x.Op == luaparser.OpPower {
				exponent, literal := staticNumberLiteral(x.Right)
				if !literal || exponent != math.Trunc(exponent) {
					return fmt.Errorf("transpiler: %s: power requires an integer literal exponent", x.Right.Pos())
				}
			}
			if err := walkExpr(x.Left, scopes, false, false); err != nil {
				return err
			}
			return walkExpr(x.Right, scopes, false, false)
		case *luaparser.IndexExpr:
			if path, ok := staticMemberPath(x); ok && !memberBase {
				if path, static := resolveStaticPath(scopes, path); static {
					if _, namespace := namespaceDeclOrder[path]; namespace {
						return fmt.Errorf("transpiler: %s: first-class function namespace %q is unsupported", x.Pos(), path)
					}
				}
			}
			if base, ok := staticMemberPath(x.Base); ok {
				base, static := resolveStaticPath(scopes, base)
				if static {
					if key, literal := unparenExpr(x.Key).(*luaparser.StringExpr); literal {
						path := base + "." + key.Value
						if callableMembers[base][key.Value] {
							return fmt.Errorf("transpiler: %s: first-class function value %q is unsupported", x.Pos(), path)
						}
					} else if _, namespace := namespaceDeclOrder[base]; namespace {
						return fmt.Errorf("transpiler: %s: dynamic indexed access to static function namespace %q is unsupported", x.Pos(), base)
					}
				}
			}
			if err := walkExpr(x.Base, scopes, false, true); err != nil {
				return err
			}
			return walkExpr(x.Key, scopes, false, false)
		case *luaparser.FieldExpr:
			if directCallee {
				if path, ok := calleePath(x); ok {
					root := strings.SplitN(path, ".", 2)[0]
					if isLocal(scopes, root) {
						return fmt.Errorf("transpiler: %s: dynamic call through local %q is unsupported", x.Pos(), root)
					}
					if _, later := allTopLocals[root]; later {
						return lateLocalError(root, x.Pos())
					}
					return nil
				}
			}
			if path, ok := staticMemberPath(x); ok {
				path, static := resolveStaticPath(scopes, path)
				if !static {
					return walkExpr(x.Base, scopes, false, true)
				}
				if _, callable := g.funcs[path]; callable {
					return fmt.Errorf("transpiler: %s: first-class function value %q is unsupported", x.Pos(), path)
				}
				if dot := strings.LastIndexByte(path, '.'); dot >= 0 {
					method := path[:dot] + ":" + path[dot+1:]
					if _, callable := g.funcs[method]; callable {
						return fmt.Errorf("transpiler: %s: first-class function value %q is unsupported", x.Pos(), path)
					}
				}
				if !memberBase {
					if _, namespace := namespaceDeclOrder[path]; namespace {
						return fmt.Errorf("transpiler: %s: first-class function namespace %q is unsupported", x.Pos(), path)
					}
				}
			}
			return walkExpr(x.Base, scopes, false, true)
		case *luaparser.CallExpr:
			if err := walkExpr(x.Callee, scopes, true, false); err != nil {
				return err
			}
			for _, argument := range x.Args {
				if err := walkExpr(argument, scopes, false, false); err != nil {
					return err
				}
			}
		case *luaparser.TableExpr:
			for _, field := range x.Fields {
				if err := walkExpr(field.Key, scopes, false, false); err != nil {
					return err
				}
				if err := walkExpr(field.Value, scopes, false, false); err != nil {
					return err
				}
			}
		}
		return nil
	}

	var walkBlock func(*luaparser.Block, lexicalScopes, bool) error
	var walkStmt func(luaparser.Stmt, lexicalScopes) error
	currentTopOrder := -1
	insideFunction := false
	walkBlock = func(block *luaparser.Block, scopes lexicalScopes, newScope bool) error {
		if newScope {
			scopes = append(scopes, map[string]bool{})
		}
		for _, stmt := range block.Body {
			if err := walkStmt(stmt, scopes); err != nil {
				return err
			}
		}
		return nil
	}
	walkStmt = func(stmt luaparser.Stmt, scopes lexicalScopes) error {
		walkValues := func(values []luaparser.Expr) error {
			for _, value := range values {
				if err := walkExpr(value, scopes, false, false); err != nil {
					return err
				}
			}
			return nil
		}
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			targets := s.Targets
			if s.CompoundTarget != nil {
				targets = []luaparser.Expr{s.CompoundTarget}
			}
			for _, target := range targets {
				switch target := target.(type) {
				case *luaparser.VarExpr:
					if target.Name == "self" && currentSelfNamespace != "" {
						return fmt.Errorf("transpiler: %s: reassignment of method receiver self is unsupported", target.Pos())
					}
					if !isLocal(scopes, target.Name) {
						if _, later := allTopLocals[target.Name]; later {
							return lateLocalError(target.Name, target.Pos())
						}
						if _, callable := g.funcs[target.Name]; callable {
							return fmt.Errorf("transpiler: %s: assignment to static function name %q is unsupported", target.Pos(), target.Name)
						}
						if _, callable := Builtins[target.Name]; callable {
							return fmt.Errorf("transpiler: %s: assignment to static builtin name %q is unsupported", target.Pos(), target.Name)
						}
						if declarationOrder, namespace := namespaceDeclOrder[target.Name]; namespace &&
							(insideFunction || currentTopOrder > declarationOrder) {
							return fmt.Errorf("transpiler: %s: assignment to static function namespace %q after its declaration is unsupported", target.Pos(), target.Name)
						}
					}
				case *luaparser.FieldExpr:
					if path, ok := staticMemberPath(target); ok {
						path, static := resolveStaticPath(scopes, path)
						if !static {
							if err := walkExpr(target.Base, scopes, false, true); err != nil {
								return err
							}
							continue
						}
						if _, callable := g.funcs[path]; callable {
							return fmt.Errorf("transpiler: %s: assignment to static function field %q is unsupported", target.Pos(), path)
						}
						if dot := strings.LastIndexByte(path, '.'); dot >= 0 {
							method := path[:dot] + ":" + path[dot+1:]
							if _, callable := g.funcs[method]; callable {
								return fmt.Errorf("transpiler: %s: assignment to static method field %q is unsupported", target.Pos(), path)
							}
						}
						if declarationOrder, namespace := namespaceDeclOrder[path]; namespace &&
							(insideFunction || currentTopOrder > declarationOrder) {
							return fmt.Errorf("transpiler: %s: assignment to static function namespace %q after its declaration is unsupported", target.Pos(), path)
						}
					}
					if err := walkExpr(target.Base, scopes, false, true); err != nil {
						return err
					}
				case *luaparser.IndexExpr:
					if base, ok := staticMemberPath(target.Base); ok {
						base, static := resolveStaticPath(scopes, base)
						if !static {
							if err := walkExpr(target.Base, scopes, false, true); err != nil {
								return err
							}
							if err := walkExpr(target.Key, scopes, false, false); err != nil {
								return err
							}
							continue
						}
						if key, literal := unparenExpr(target.Key).(*luaparser.StringExpr); literal {
							path := base + "." + key.Value
							if _, callable := g.funcs[path]; callable {
								return fmt.Errorf("transpiler: %s: assignment to static function field %q is unsupported", target.Pos(), path)
							}
							method := base + ":" + key.Value
							if _, callable := g.funcs[method]; callable {
								return fmt.Errorf("transpiler: %s: assignment to static method field %q is unsupported", target.Pos(), path)
							}
							if declarationOrder, namespace := namespaceDeclOrder[path]; namespace &&
								(insideFunction || currentTopOrder > declarationOrder) {
								return fmt.Errorf("transpiler: %s: assignment to static function namespace %q after its declaration is unsupported", target.Pos(), path)
							}
						} else if declarationOrder, namespace := namespaceDeclOrder[base]; namespace &&
							(insideFunction || currentTopOrder > declarationOrder) {
							return fmt.Errorf("transpiler: %s: dynamic indexed assignment to static function namespace %q is unsupported", target.Pos(), base)
						}
					}
					if err := walkExpr(target.Base, scopes, false, true); err != nil {
						return err
					}
					if err := walkExpr(target.Key, scopes, false, false); err != nil {
						return err
					}
				}
			}
			if s.CompoundTarget == nil && !insideFunction {
				for index, target := range targets {
					path, static := staticMemberPath(target)
					if !static {
						continue
					}
					path, static = resolveStaticPath(scopes, path)
					declarationOrder, namespace := namespaceDeclOrder[path]
					if !static || !namespace || currentTopOrder > declarationOrder {
						continue
					}
					var value luaparser.Expr
					if index < len(s.Values) {
						value = s.Values[index]
					}
					if err := validateNamespaceInitializer(path, value, target.Pos()); err != nil {
						return err
					}
				}
			}
			return walkValues(s.Values)
		case *luaparser.LocalAssignStmt:
			if s.Synthetic {
				// Compound lowering creates block-local aliases solely to evaluate
				// an l-value base once. They cannot escape; the companion synthetic
				// assignment validates CompoundTarget, the original source l-value.
				for _, value := range s.Values {
					if err := walkExpr(value, scopes, false, true); err != nil {
						return err
					}
				}
			} else if err := walkValues(s.Values); err != nil {
				return err
			}
			for _, name := range s.Names {
				if name == "self" && currentSelfNamespace != "" {
					return fmt.Errorf("transpiler: %s: shadowing method receiver self is unsupported", s.Pos())
				}
				scopes[len(scopes)-1][name] = true
			}
		case *luaparser.CallStmt:
			return walkExpr(s.Call, scopes, false, false)
		case *luaparser.IfStmt:
			if err := walkExpr(s.Condition, scopes, false, false); err != nil {
				return err
			}
			if err := walkBlock(s.Then, scopes, true); err != nil {
				return err
			}
			for _, branch := range s.ElseIf {
				if err := walkExpr(branch.Condition, scopes, false, false); err != nil {
					return err
				}
				if err := walkBlock(branch.Body, scopes, true); err != nil {
					return err
				}
			}
			if s.Else != nil {
				return walkBlock(s.Else, scopes, true)
			}
		case *luaparser.WhileStmt:
			if err := walkExpr(s.Condition, scopes, false, false); err != nil {
				return err
			}
			return walkBlock(s.Body, scopes, true)
		case *luaparser.RepeatStmt:
			repeatScopes := append(scopes, map[string]bool{})
			if err := walkBlock(s.Body, repeatScopes, false); err != nil {
				return err
			}
			return walkExpr(s.Condition, repeatScopes, false, false)
		case *luaparser.DoStmt:
			return walkBlock(s.Body, scopes, true)
		case *luaparser.NumericForStmt:
			if err := walkExpr(s.Init, scopes, false, false); err != nil {
				return err
			}
			if err := walkExpr(s.Limit, scopes, false, false); err != nil {
				return err
			}
			if err := walkExpr(s.Step, scopes, false, false); err != nil {
				return err
			}
			if s.Name == "self" && currentSelfNamespace != "" {
				return fmt.Errorf("transpiler: %s: shadowing method receiver self is unsupported", s.Pos())
			}
			loopScopes := append(scopes, map[string]bool{s.Name: true})
			return walkBlock(s.Body, loopScopes, true)
		case *luaparser.GenericForStmt:
			if err := walkValues(s.Values); err != nil {
				return err
			}
			bindings := map[string]bool{}
			for _, name := range s.Names {
				if name == "self" && currentSelfNamespace != "" {
					return fmt.Errorf("transpiler: %s: shadowing method receiver self is unsupported", s.Pos())
				}
				bindings[name] = true
			}
			return walkBlock(s.Body, append(scopes, bindings), true)
		case *luaparser.ReturnStmt:
			return walkValues(s.Values)
		}
		return nil
	}

	topVisible := map[string]bool{}
	for order, stmt := range g.chunk.Body {
		currentTopOrder = order
		if local, ok := stmt.(*luaparser.LocalAssignStmt); ok {
			for index, name := range local.Names {
				declarationOrder, namespace := namespaceDeclOrder[name]
				if !namespace || order > declarationOrder {
					continue
				}
				var value luaparser.Expr
				if index < len(local.Values) {
					value = local.Values[index]
				}
				if err := validateNamespaceInitializer(name, value, local.Pos()); err != nil {
					return err
				}
			}
		}
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			if fn.Local {
				return fmt.Errorf("transpiler: %s: local function declarations are unsupported; use a top-level function declaration", fn.Pos())
			}
			functionScope := copyScope(topVisible)
			for _, parameter := range fn.Params {
				functionScope[parameter] = true
			}
			currentSelfNamespace = ""
			if fn.Method != "" {
				currentSelfNamespace = strings.Join(fn.Name, ".")
				for _, parameter := range fn.Params[1:] {
					if parameter == "self" {
						return fmt.Errorf("transpiler: %s: shadowing method receiver self is unsupported", fn.Pos())
					}
				}
			}
			insideFunction = true
			err := walkBlock(fn.Body, lexicalScopes{functionScope}, false)
			insideFunction = false
			currentSelfNamespace = ""
			if err != nil {
				return err
			}
			continue
		}
		if err := walkStmt(stmt, lexicalScopes{topVisible}); err != nil {
			return err
		}
	}
	return nil
}

// resolveReturnArities expands `return f()` / `return 1, f()` using each
// callee's current arity until the set of function arities is stable. A
// function that only `return pair()` must have pair's full result count, not
// 1, or `local a, b = wrap()` would nil-fill b. Starting every known function
// at zero also lets a trailing zero-result call remove its syntactic call slot.
func (g *generator) resolveReturnArities() error {
	for pass := 0; ; pass++ {
		changed := false
		var changedInfo *functionInfo
		for _, info := range g.funcList {
			n := g.adjustedReturnN(info.decl.Body)
			if n != info.returnN {
				info.returnN = n
				changed = true
				changedInfo = info
			}
		}
		if !changed {
			return nil
		}
		// With N functions, every finite return-width dependency reaches its
		// fixed point after at most N changing passes. A later change proves a
		// positive recursive cycle such as `return n, expand(n-1)`, whose
		// statically generated Go tuple would grow without bound.
		if pass >= len(g.funcList) {
			return fmt.Errorf("transpiler: recursive return arity expands without bound in function %q at %s", changedInfo.key, changedInfo.decl.Pos())
		}
	}
}

func (g *generator) validateTableLiteralLimits() error {
	var firstErr error
	check := func(expr luaparser.Expr) {
		if firstErr != nil {
			return
		}
		table, ok := expr.(*luaparser.TableExpr)
		if !ok {
			return
		}
		arrayN, hashN := 0, 0
		for _, field := range table.Fields {
			if field.Array {
				arrayN++
			} else {
				hashN++
			}
		}
		if n := len(table.Fields); n > 0 && table.Fields[n-1].Array {
			if call, ok := table.Fields[n-1].Value.(*luaparser.CallExpr); ok {
				if arity, known := g.callResultArity(call); known {
					arrayN += arity - 1
				}
			}
		}
		if arrayN > runtimeapi.MaxTableArrayCapacity {
			firstErr = fmt.Errorf("transpiler: %s: table literal requires %d array slots, runtime limit is %d", table.Pos(), arrayN, runtimeapi.MaxTableArrayCapacity)
			return
		}
		if hashN > runtimeapi.MaxTableHashCapacity {
			firstErr = fmt.Errorf("transpiler: %s: table literal requires %d hash slots, runtime limit is %d", table.Pos(), hashN, runtimeapi.MaxTableHashCapacity)
		}
	}
	for _, stmt := range g.chunk.Body {
		collectStmtExprs(stmt, check)
	}
	for _, fn := range g.funcList {
		for _, stmt := range fn.decl.Body.Body {
			collectStmtExprs(stmt, check)
		}
	}
	return firstErr
}

// validateBuiltinCallArities runs after user-function return widths have
// reached their fixed point. That timing matters because a trailing call may
// contribute zero or multiple arguments to the surrounding builtin call.
// Keeping this out of lowering also prevents numeric optimized calls from
// bypassing public API validation.
func (g *generator) validateBuiltinCallArities() error {
	var firstErr error
	check := func(expr luaparser.Expr) {
		if firstErr != nil {
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
		spec, builtin := Builtins[name]
		if !builtin {
			return
		}
		got := g.exprListWidth(call.Args)
		if got < spec.MinArgs || got > spec.MaxArgs {
			firstErr = fmt.Errorf("transpiler: %s: %s expects %d..%d arguments, got %d", call.Pos(), name, spec.MinArgs, spec.MaxArgs, got)
		}
	}
	for _, stmt := range g.chunk.Body {
		collectStmtExprs(stmt, check)
	}
	for _, fn := range g.funcList {
		for _, stmt := range fn.decl.Body.Body {
			collectStmtExprs(stmt, check)
		}
	}
	return firstErr
}

func (g *generator) adjustedReturnN(block *luaparser.Block) int {
	maxN := 0
	var visit func([]luaparser.Stmt)
	visit = func(stmts []luaparser.Stmt) {
		for _, stmt := range stmts {
			switch s := stmt.(type) {
			case *luaparser.ReturnStmt:
				n := g.exprListWidth(s.Values)
				if n > maxN {
					maxN = n
				}
			case *luaparser.IfStmt:
				visit(s.Then.Body)
				for _, branch := range s.ElseIf {
					visit(branch.Body.Body)
				}
				if s.Else != nil {
					visit(s.Else.Body)
				}
			case *luaparser.WhileStmt:
				visit(s.Body.Body)
			case *luaparser.RepeatStmt:
				visit(s.Body.Body)
			case *luaparser.DoStmt:
				visit(s.Body.Body)
			case *luaparser.NumericForStmt:
				visit(s.Body.Body)
			case *luaparser.GenericForStmt:
				visit(s.Body.Body)
			}
		}
	}
	visit(block.Body)
	return maxN
}

func (g *generator) exprListWidth(values []luaparser.Expr) int {
	n := len(values)
	if n == 0 {
		return 0
	}
	call, ok := values[n-1].(*luaparser.CallExpr)
	if !ok {
		return n
	}
	arity, known := g.callResultArity(call)
	if !known {
		return n
	}
	return n - 1 + arity
}
