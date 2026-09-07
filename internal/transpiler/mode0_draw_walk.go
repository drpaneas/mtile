package transpiler

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/luaparser"
)

type mode0DrawOp struct {
	name      string
	call      *luaparser.CallExpr
	copies    int
	lastID    int
	sprW      int
	sprH      int
	printKind mode0PrintTextKind
	atlasOnly bool
}

func mode0WalkDraw(chunk *luaparser.Chunk, draw *luaparser.FunctionDeclStmt, analysis *mode0SpriteIDAnalysis, cols, rows int) ([]mode0DrawOp, bool, error) {
	w := &mode0DrawWalker{
		chunk:    chunk,
		graph:    newMode0CallGraph(chunk),
		analysis: analysis,
		eval:     newMode0NumEval(chunk, analysis),
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
	case *luaparser.DoStmt:
		return w.walkBlock(s.Body, cloneMode0NumEnv(env), copies, visiting, false, names)
	case *luaparser.IfStmt:
		return w.walkIf(s, env, copies, visiting, names)
	case *luaparser.NumericForStmt:
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
		if w.blockHasDrawOp(s.Body) {
			w.unbounded = true
			return w.walkBlock(s.Body, cloneMode0NumEnv(env), copies, visiting, false, names)
		}
		return nil
	case *luaparser.WhileStmt:
		if w.blockHasDrawOp(s.Body) {
			w.unbounded = true
			return w.walkBlock(s.Body, cloneMode0NumEnv(env), copies, visiting, false, names)
		}
		return nil
	case *luaparser.RepeatStmt:
		if w.blockHasDrawOp(s.Body) {
			w.unbounded = true
			return w.walkBlock(s.Body, cloneMode0NumEnv(env), copies, visiting, false, names)
		}
		return nil
	default:
		return nil
	}
}

func (w *mode0DrawWalker) walkIf(s *luaparser.IfStmt, env mode0NumEnv, copies int, visiting map[*luaparser.FunctionDeclStmt]bool, names map[string]bool) error {
	branches := []*luaparser.Block{s.Then}
	for _, branch := range s.ElseIf {
		branches = append(branches, branch.Body)
	}
	if s.Else != nil {
		branches = append(branches, s.Else)
	} else {
		branches = append(branches, nil)
	}
	start := len(w.ops)
	startUnbounded := w.unbounded
	type branchResult struct {
		ops []mode0DrawOp
	}
	results := make([]branchResult, 0, len(branches))
	best := -1
	bestCost := -1
	anyUnbounded := startUnbounded
	for _, body := range branches {
		w.ops = w.ops[:start]
		w.unbounded = startUnbounded
		if err := w.walkBlock(body, cloneMode0NumEnv(env), copies, visiting, false, names); err != nil {
			return err
		}
		branchOps := append([]mode0DrawOp(nil), w.ops[start:]...)
		if w.unbounded {
			anyUnbounded = true
		}
		cost := mode0OpsObjectCost(branchOps)
		results = append(results, branchResult{ops: branchOps})
		if cost > bestCost {
			bestCost = cost
			best = len(results) - 1
		}
	}
	w.unbounded = anyUnbounded
	var chosen []mode0DrawOp
	if best >= 0 {
		chosen = append(chosen, results[best].ops...)
	}
	for i, result := range results {
		if i == best {
			continue
		}
		for _, op := range result.ops {
			if op.name == "print" {
				op.atlasOnly = true
				chosen = append(chosen, op)
			}
		}
	}
	w.ops = append(w.ops[:start], chosen...)
	return nil
}

func mode0OpsObjectCost(ops []mode0DrawOp) int {
	total := 0
	for _, op := range ops {
		if op.atlasOnly {
			continue
		}
		copies := op.copies
		if copies < 1 {
			copies = 1
		}
		switch op.name {
		case "spr":
			width, height := op.sprW, op.sprH
			if width < 1 {
				width = 1
			}
			if height < 1 {
				height = 1
			}
			total += width * height * copies
		case "sspr":
			call := op.call
			if call == nil || len(call.Args) < 4 {
				total += copies
				continue
			}
			width := mode0LiteralInt(call.Args[2]) / 8
			height := mode0LiteralInt(call.Args[3]) / 8
			if width < 1 {
				width = 1
			}
			if height < 1 {
				height = 1
			}
			total += width * height * copies
		case "print":
			entries, ok := mode0PrintOAMEntries(op.call, op.printKind)
			if ok {
				total += entries * copies
			}
		}
	}
	return total
}

func (w *mode0DrawWalker) walkCall(call *luaparser.CallExpr, env mode0NumEnv, copies int, visiting map[*luaparser.FunctionDeclStmt]bool, topDraw bool, names map[string]bool) error {
	if call == nil {
		return nil
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
	case "sspr", "camera":
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
		if visiting[fn] {
			return fmt.Errorf("transpiler: %s: [%s] Mode 0 spr()/sspr()/print() cannot use a recursive helper",
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
