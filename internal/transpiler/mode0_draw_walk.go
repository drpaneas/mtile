package transpiler

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/luaparser"
)

type mode0DrawOp struct {
	name           string
	call           *luaparser.CallExpr
	copies         int
	lastID         int
	sprW           int
	sprH           int
	printKind      mode0PrintTextKind
	atlasOnly      bool
	allocationOnly bool
	psprUnique     int
	psprParts      int
	psprIDs        []int
	// tileLinkMap is the map() call whose linked objects this synthetic pose
	// site draws; see mode0_tile_links.go.
	tileLinkMap *luaparser.CallExpr
}

func mode0WalkDraw(chunk *luaparser.Chunk, draw *luaparser.FunctionDeclStmt, analysis *mode0SpriteIDAnalysis, opts Options) ([]mode0DrawOp, bool, error) {
	cols, rows := opts.SheetW/8, opts.SheetH/8
	w := &mode0DrawWalker{
		chunk:    chunk,
		graph:    newMode0CallGraph(chunk),
		analysis: analysis,
		eval:     newMode0NumEval(chunk, analysis),
		opts:     opts,
		cols:     cols,
		rows:     rows,
		names:    mode0NumericNames(chunk),
	}
	if err := w.walkBlock(draw.Body, mode0NumEnv{}, 1, map[*luaparser.FunctionDeclStmt]bool{}, true, cloneMode0PrintNames(w.names)); err != nil {
		return nil, false, err
	}
	return w.ops, w.unbounded, nil
}

type mode0DrawWalker struct {
	chunk     *luaparser.Chunk
	graph     *mode0CallGraph
	analysis  *mode0SpriteIDAnalysis
	eval      *mode0NumEval
	opts      Options
	cols      int
	rows      int
	ops       []mode0DrawOp
	unbounded bool
	names     map[string]bool
}

func (w *mode0DrawWalker) walkBlock(block *luaparser.Block, env mode0NumEnv, copies int, visiting map[*luaparser.FunctionDeclStmt]bool, topDraw bool, names map[string]bool) error {
	if block == nil {
		return nil
	}
	if copies < 1 {
		return nil
	}
	for _, stmt := range block.Body {
		if err := w.walkStmt(stmt, env, copies, visiting, topDraw, names); err != nil {
			return err
		}
		env = w.bindStmt(stmt, env)
	}
	return nil
}

func (w *mode0DrawWalker) bindStmt(stmt luaparser.Stmt, env mode0NumEnv) mode0NumEnv {
	if w.eval == nil {
		return env
	}
	_, next, ok := w.eval.execStmt(stmt, env)
	if !ok {
		return env
	}
	return next
}

func (w *mode0DrawWalker) walkStmt(stmt luaparser.Stmt, env mode0NumEnv, copies int, visiting map[*luaparser.FunctionDeclStmt]bool, topDraw bool, names map[string]bool) error {
	switch s := stmt.(type) {
	case *luaparser.CallStmt:
		return w.walkCall(s.Call, env, copies, visiting, topDraw, names)
	case *luaparser.LocalAssignStmt:
		return w.walkExprs(s.Values, env, copies, visiting, names)
	case *luaparser.AssignStmt:
		if err := w.walkExprs(s.Targets, env, copies, visiting, names); err != nil {
			return err
		}
		return w.walkExprs(s.Values, env, copies, visiting, names)
	case *luaparser.ReturnStmt:
		return w.walkExprs(s.Values, env, copies, visiting, names)
	case *luaparser.DoStmt:
		return w.walkBlock(s.Body, cloneMode0NumEnv(env), copies, visiting, false, names)
	case *luaparser.IfStmt:
		return w.walkIf(s, env, copies, visiting, names)
	case *luaparser.NumericForStmt:
		if err := w.walkExprs([]luaparser.Expr{s.Init, s.Limit, s.Step}, env, copies, visiting, names); err != nil {
			return err
		}
		trips, ok := w.numericTrips(s, env)
		if !ok {
			if w.blockHasDrawOp(s.Body) {
				w.unbounded = true
				return w.walkBlock(s.Body, cloneMode0NumEnv(env), copies, visiting, false, names)
			}
			return nil
		}
		values, okVals := w.loopValues(s, env, trips)
		if !okVals {
			return w.walkBlock(s.Body, cloneMode0NumEnv(env), copies*trips, visiting, false, names)
		}
		for _, value := range values {
			bodyEnv := cloneMode0NumEnv(env)
			bodyEnv[s.Name] = []float64{value}
			if err := w.walkBlock(s.Body, bodyEnv, copies, visiting, false, names); err != nil {
				return err
			}
		}
		return nil
	case *luaparser.GenericForStmt:
		if err := w.walkExprs(s.Values, env, copies, visiting, names); err != nil {
			return err
		}
		if w.blockHasDrawOp(s.Body) {
			w.unbounded = true
			return w.walkBlock(s.Body, cloneMode0NumEnv(env), copies, visiting, false, names)
		}
		return nil
	case *luaparser.WhileStmt:
		start := len(w.ops)
		if err := w.walkExpr(s.Condition, env, copies, visiting, names); err != nil {
			return err
		}
		if mode0OpsHaveObjects(w.ops[start:]) {
			w.unbounded = true
		}
		if w.blockHasDrawOp(s.Body) {
			w.unbounded = true
			return w.walkBlock(s.Body, cloneMode0NumEnv(env), copies, visiting, false, names)
		}
		return nil
	case *luaparser.RepeatStmt:
		if w.blockHasDrawOp(s.Body) {
			w.unbounded = true
			if err := w.walkBlock(s.Body, cloneMode0NumEnv(env), copies, visiting, false, names); err != nil {
				return err
			}
		}
		start := len(w.ops)
		if err := w.walkExpr(s.Condition, env, copies, visiting, names); err != nil {
			return err
		}
		if mode0OpsHaveObjects(w.ops[start:]) {
			w.unbounded = true
		}
		return nil
	default:
		return nil
	}
}

func (w *mode0DrawWalker) walkIf(s *luaparser.IfStmt, env mode0NumEnv, copies int, visiting map[*luaparser.FunctionDeclStmt]bool, names map[string]bool) error {
	// Each condition runs at most once. Summing every elseif condition is a
	// conservative bound and preserves sites even when earlier branches win.
	if err := w.walkExpr(s.Condition, env, copies, visiting, names); err != nil {
		return err
	}
	for _, branch := range s.ElseIf {
		if err := w.walkExpr(branch.Condition, env, copies, visiting, names); err != nil {
			return err
		}
	}
	branches := []*luaparser.Block{s.Then}
	for _, branch := range s.ElseIf {
		branches = append(branches, branch.Body)
	}
	if s.Else != nil {
		branches = append(branches, s.Else)
	} else {
		branches = append(branches, nil)
	}
	// Retain every call site for allocation and text-atlas construction. Object
	// counts and scanline costs use independent branch envelopes downstream;
	// choosing a single "largest" branch is unsafe when those maxima differ.
	w.ops = append(w.ops, mode0DrawOp{name: mode0BranchStart})
	for index, body := range branches {
		if index > 0 {
			w.ops = append(w.ops, mode0DrawOp{name: mode0BranchNext})
		}
		if err := w.walkBlock(body, cloneMode0NumEnv(env), copies, visiting, false, cloneMode0PrintNames(names)); err != nil {
			return err
		}
	}
	w.ops = append(w.ops, mode0DrawOp{name: mode0BranchEnd})
	return nil
}

const (
	mode0BranchStart = "branch-start"
	mode0BranchNext  = "branch-next"
	mode0BranchEnd   = "branch-end"
)

func (w *mode0DrawWalker) walkCall(call *luaparser.CallExpr, env mode0NumEnv, copies int, visiting map[*luaparser.FunctionDeclStmt]bool, topDraw bool, names map[string]bool) error {
	if call == nil {
		return nil
	}
	// Lua evaluates the receiver and arguments before entering the callee.
	// Calls used as values can draw too, even when their return is discarded.
	if err := w.walkExpr(call.Callee, env, copies, visiting, names); err != nil {
		return err
	}
	if err := w.walkExprs(call.Args, env, copies, visiting, names); err != nil {
		return err
	}
	name, ok := callKey(call)
	if !ok {
		return nil
	}
	switch name {
	case "cls", "map":
		if !topDraw {
			return fmt.Errorf("transpiler: %s: [%s] cls() and map() must be unconditional direct statements in _draw",
				call.Pos(), reasonMode0DrawShape)
		}
		w.ops = append(w.ops, mode0DrawOp{name: name, call: call, copies: copies})
		if name == "map" {
			if op, ok := mode0TileLinkOp(call, copies, w.opts); ok {
				w.ops = append(w.ops, op)
			}
		}
		return nil
	case "spr":
		lastID, sprW, sprH := -1, 1, 1
		if w.cols > 0 {
			id, width, height, err := validateMode0SprCallWithEnv(call, w.cols, w.rows, w.analysis, w.eval, env)
			if err != nil {
				return err
			}
			lastID, sprW, sprH = id, width, height
		}
		w.ops = append(w.ops, mode0DrawOp{name: name, call: call, copies: copies, lastID: lastID, sprW: sprW, sprH: sprH})
		return nil
	case "sspr":
		lastID, sprW, sprH := -1, 1, 1
		if w.cols > 0 {
			_, last, width, height, err := validateMode0SSprCallWithEnv(call, w.cols*8, w.rows*8, w.cols, w.eval, env)
			if err != nil {
				return err
			}
			lastID, sprW, sprH = last, width, height
		}
		w.ops = append(w.ops, mode0DrawOp{name: name, call: call, copies: copies, lastID: lastID, sprW: sprW, sprH: sprH})
		return nil
	case "pspr", "anim":
		ids, unique, parts, err := validateMode0PsprCallWithEnv(call, w.opts, w.analysis, w.eval, env)
		if err != nil {
			return err
		}
		w.ops = append(w.ops, mode0DrawOp{
			name:       "pspr",
			call:       call,
			copies:     copies,
			psprUnique: unique,
			psprParts:  parts,
			psprIDs:    ids,
		})
		return nil
	case "camera":
		w.ops = append(w.ops, mode0DrawOp{name: name, call: call, copies: copies})
		return nil
	case "print":
		kind := mode0PrintTextInvalid
		if len(call.Args) > 0 {
			kind = classifyMode0PrintText(call.Args[0], w.eval, env, names)
		}
		if _, err := validateMode0PrintCall(call, kind); err != nil {
			return err
		}
		w.ops = append(w.ops, mode0DrawOp{name: name, call: call, copies: copies, printKind: kind})
		return nil
	}
	if _, builtin := Builtins[name]; builtin {
		return nil
	}
	if w.graph == nil {
		return nil
	}
	for _, fn := range w.graph.functions[name] {
		// A math/data helper can recurse without multiplying draw sites.
		// Only follow bodies that can affect the renderer's operation trace.
		if w.graph.firstBuiltinFromBlock(fn.Body, map[string]bool{
			"cls": true, "map": true, "spr": true, "sspr": true,
			"pspr": true, "anim": true, "print": true, "camera": true,
		}, make(map[*luaparser.FunctionDeclStmt]bool)) == nil {
			continue
		}
		if visiting[fn] {
			return fmt.Errorf("transpiler: %s: [%s] Mode 0 spr()/sspr()/pspr()/print() cannot use a recursive helper",
				call.Pos(), reasonMode0DrawShape)
		}
		visiting[fn] = true
		childEnv := w.callEnv(fn, call, env)
		childNames := cloneMode0PrintNames(names)
		for i, param := range fn.Params {
			if i < len(call.Args) && mode0PrintAlwaysNumber(call.Args[i], w.eval, env, names) {
				childNames[param] = true
			}
		}
		if err := w.walkBlock(fn.Body, childEnv, copies, visiting, false, childNames); err != nil {
			delete(visiting, fn)
			return err
		}
		delete(visiting, fn)
	}
	return nil
}

// Expressions contribute draw operations in evaluation order. Both operands
// of and/or are counted, which safely overestimates short-circuit execution.
func (w *mode0DrawWalker) walkExpr(expr luaparser.Expr, env mode0NumEnv, copies int, visiting map[*luaparser.FunctionDeclStmt]bool, names map[string]bool) error {
	switch e := expr.(type) {
	case *luaparser.CallExpr:
		return w.walkCall(e, env, copies, visiting, false, names)
	case *luaparser.ParenExpr:
		return w.walkExpr(e.Expr, env, copies, visiting, names)
	case *luaparser.UnaryExpr:
		return w.walkExpr(e.Expr, env, copies, visiting, names)
	case *luaparser.BinaryExpr:
		return w.walkExprs([]luaparser.Expr{e.Left, e.Right}, env, copies, visiting, names)
	case *luaparser.IndexExpr:
		return w.walkExprs([]luaparser.Expr{e.Base, e.Key}, env, copies, visiting, names)
	case *luaparser.FieldExpr:
		return w.walkExpr(e.Base, env, copies, visiting, names)
	case *luaparser.TableExpr:
		for _, field := range e.Fields {
			if err := w.walkExprs([]luaparser.Expr{field.Key, field.Value}, env, copies, visiting, names); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *mode0DrawWalker) walkExprs(exprs []luaparser.Expr, env mode0NumEnv, copies int, visiting map[*luaparser.FunctionDeclStmt]bool, names map[string]bool) error {
	for _, expr := range exprs {
		if err := w.walkExpr(expr, env, copies, visiting, names); err != nil {
			return err
		}
	}
	return nil
}

func mode0OpsHaveObjects(ops []mode0DrawOp) bool {
	for _, op := range ops {
		switch op.name {
		case "spr", "sspr", "pspr", "anim", "print":
			return true
		}
	}
	return false
}

func (w *mode0DrawWalker) callEnv(fn *luaparser.FunctionDeclStmt, call *luaparser.CallExpr, parent mode0NumEnv) mode0NumEnv {
	env := mode0NumEnv{}
	if w.eval == nil || fn == nil {
		return env
	}
	for i, name := range fn.Params {
		if i >= len(call.Args) {
			break
		}
		if values, ok := w.eval.nums(call.Args[i], parent); ok {
			env[name] = values
			continue
		}
		if lo, hi, bounded := w.eval.interval(call.Args[i], parent); bounded {
			if values, ok := w.eval.intervalInts(lo, hi, true); ok {
				env[name] = values
			}
		}
	}
	return env
}

func (w *mode0DrawWalker) numericTrips(loop *luaparser.NumericForStmt, env mode0NumEnv) (int, bool) {
	if count := fixedIntegerLoopCount(loop); count > 0 {
		return count, true
	}
	if w.eval == nil {
		return 0, false
	}
	init, okInit := w.eval.nums(loop.Init, env)
	limit, okLimit := w.eval.nums(loop.Limit, env)
	step := []float64{1}
	okStep := true
	if loop.Step != nil {
		step, okStep = w.eval.nums(loop.Step, env)
	}
	if !okInit || !okLimit || !okStep || len(init) == 0 || len(limit) == 0 || len(step) == 0 {
		return 0, false
	}
	maxTrips := 0
	for _, start := range init {
		for _, end := range limit {
			for _, delta := range step {
				if delta == 0 || !isMode0Int(start) || !isMode0Int(end) || !isMode0Int(delta) {
					return 0, false
				}
				count := mode0TripCount(int64(start), int64(end), int64(delta))
				if count <= 0 {
					continue
				}
				if count > maxTrips {
					maxTrips = count
				}
			}
		}
	}
	if maxTrips < 1 || maxTrips > 128 {
		return 0, false
	}
	return maxTrips, true
}

func isMode0Int(value float64) bool {
	return value == float64(int64(value))
}

func mode0TripCount(init, limit, step int64) int {
	if step == 0 || (step > 0 && init > limit) || (step < 0 && init < limit) {
		return 0
	}
	count := (limit-init)/step + 1
	if count < 1 || count > 128 {
		return 0
	}
	return int(count)
}

func (w *mode0DrawWalker) loopValues(loop *luaparser.NumericForStmt, env mode0NumEnv, trips int) ([]float64, bool) {
	if trips < 1 || trips > mode0MaxFiniteNums {
		return nil, false
	}
	if count := fixedIntegerLoopCount(loop); count == trips {
		init, _, step, ok := integerNumericForBounds(loop)
		if !ok {
			return nil, false
		}
		out := make([]float64, 0, trips)
		value := init
		for i := 0; i < trips; i++ {
			out = append(out, float64(value))
			value += step
		}
		return out, true
	}
	if w.eval == nil {
		return nil, false
	}
	init, ok := w.eval.nums(loop.Init, env)
	step := []float64{1}
	okStep := true
	if loop.Step != nil {
		step, okStep = w.eval.nums(loop.Step, env)
	}
	if !ok || !okStep || len(init) != 1 || len(step) != 1 {
		return nil, false
	}
	out := make([]float64, 0, trips)
	value := init[0]
	delta := step[0]
	for i := 0; i < trips; i++ {
		out = append(out, value)
		value += delta
	}
	return out, true
}

func (w *mode0DrawWalker) blockHasDrawOp(block *luaparser.Block) bool {
	if block == nil || w.graph == nil {
		return false
	}
	visiting := map[*luaparser.FunctionDeclStmt]bool{}
	var walk func(*luaparser.Block) bool
	walk = func(body *luaparser.Block) bool {
		if body == nil {
			return false
		}
		for _, stmt := range body.Body {
			if w.graph.firstBuiltinFromStmt(stmt, "spr") != nil ||
				w.graph.firstBuiltinFromStmt(stmt, "sspr") != nil ||
				(w.graph.firstBuiltinFromStmt(stmt, "pspr") != nil || w.graph.firstBuiltinFromStmt(stmt, "anim") != nil) ||
				w.graph.firstBuiltinFromStmt(stmt, "print") != nil {
				return true
			}
			switch s := stmt.(type) {
			case *luaparser.IfStmt:
				if walk(s.Then) {
					return true
				}
				for _, branch := range s.ElseIf {
					if walk(branch.Body) {
						return true
					}
				}
				if walk(s.Else) {
					return true
				}
			case *luaparser.DoStmt:
				if walk(s.Body) {
					return true
				}
			case *luaparser.NumericForStmt:
				if walk(s.Body) {
					return true
				}
			case *luaparser.GenericForStmt:
				if walk(s.Body) {
					return true
				}
			case *luaparser.WhileStmt:
				if walk(s.Body) {
					return true
				}
			case *luaparser.RepeatStmt:
				if walk(s.Body) {
					return true
				}
			}
		}
		return false
	}
	_ = visiting
	return walk(block)
}
