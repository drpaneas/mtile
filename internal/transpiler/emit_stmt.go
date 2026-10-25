package transpiler

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/drpaneas/mtile/internal/luaparser"
)

func (g *generator) emitStmt(b *sourceBuilder, stmt luaparser.Stmt, indent int) {
	pad := strings.Repeat("\t", indent)
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		g.emitAssign(b, s, indent)
	case *luaparser.LocalAssignStmt:
		g.emitLocalAssign(b, s, indent)
	case *luaparser.CallStmt:
		call := g.emitCall(s.Call, false)
		g.writePending(b, indent)
		if call != "" {
			b.line(pad + call)
		}
	case *luaparser.IfStmt:
		var elseBody []luaparser.Stmt
		if s.Else != nil {
			elseBody = s.Else.Body
		}
		g.emitIfChain(b, indent, s.Condition, s.Then.Body, s.ElseIf, elseBody)
	case *luaparser.WhileStmt:
		b.line(pad + "for {")
		cond := g.emitConditionFlush(b, indent+1, s.Condition)
		b.line(pad + "\tif !(" + cond + ") { break }")
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+1)
		}
		g.popScope()
		b.line(pad + "}")
	case *luaparser.RepeatStmt:
		b.line(pad + "for {")
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+1)
		}
		cond := g.emitConditionFlush(b, indent+1, s.Condition)
		g.popScope()
		b.line(pad + "\tif (" + cond + ") { break }")
		b.line(pad + "}")
	case *luaparser.DoStmt:
		b.line(pad + "{")
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+1)
		}
		g.popScope()
		b.line(pad + "}")
	case *luaparser.NumericForStmt:
		if g.emitMonomorphicNumericFor(b, s, indent) {
			return
		}
		id := g.next("for")
		initExpr := g.emitExprFlush(b, indent, s.Init)
		b.line(pad + id + "Init := " + initExpr)
		initRoots := id + "Roots"
		rootSlots := g.activeRootSlots
		g.requireActiveRootSlots(1, s.Init.Pos())
		b.line(pad + initRoots + " := gluPushRoots(c, " + id + "Init)")
		b.line(pad + "if " + initRoots + " >= 0 {")
		// Init is already on Context's root stack while limit is evaluated.
		// Track that occupied slot instead of passing the same value again as
		// an extra helper root.
		g.activeRootSlots = rootSlots + 1
		limitExpr := g.emitExprFlush(b, indent+1, s.Limit)
		b.line(pad + "\t" + id + "Limit := " + limitExpr)
		limitRoots := id + "LimitRoots"
		g.requireActiveRootSlots(1, s.Limit.Pos())
		b.line(pad + "\t" + limitRoots + " := gluPushRoots(c, " + id + "Limit)")
		b.line(pad + "\tif " + limitRoots + " >= 0 {")
		// The step may allocate and therefore needs the already evaluated limit
		// as a root. Generate and flush it only after Limit has been declared;
		// otherwise root arguments inside the step can reference a Go name before
		// its declaration.
		g.activeRootSlots = rootSlots + 2
		stepExpr := g.emitExprFlush(b, indent+2, s.Step)
		b.line(pad + "\t\t" + id + "Step := " + stepExpr)
		stepRoots := id + "StepRoots"
		g.requireActiveRootSlots(1, s.Step.Pos())
		b.line(pad + "\t\t" + stepRoots + " := gluPushRoots(c, " + id + "Step)")
		b.line(pad + "\t\tif " + stepRoots + " >= 0 {")
		rootDepth := len(g.activeRootCounts)
		g.activeRootCounts = append(g.activeRootCounts, initRoots, limitRoots, stepRoots)
		g.activeRootSlots = rootSlots + 3
		g.pushScope()
		loopVar := g.declareLocal(s.Name)
		b.line(pad + "\t\t\tvar " + loopVar + " rt.Value")
		b.line(pad + "\t\t\t_ = " + loopVar)
		b.line(pad + "\t\t\tfor " + id + "Init.Kind == rt.KindNumber && " + id + "Limit.Kind == rt.KindNumber && " + id + "Step.Kind == rt.KindNumber && ((" + id + "Step.Num >= 0 && " + id + "Init.Num <= " + id + "Limit.Num) || (" + id + "Step.Num < 0 && " + id + "Init.Num >= " + id + "Limit.Num)) {")
		b.line(pad + "\t\t\t\t" + loopVar + " = " + id + "Init")
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+4)
		}
		g.popScope()
		g.activeRootCounts = g.activeRootCounts[:rootDepth]
		g.activeRootSlots = rootSlots
		b.line(pad + "\t\t\t\t" + id + "Init = " + id + "Init.Add(" + id + "Step)")
		b.line(pad + "\t\t\t}")
		g.popScope()
		b.line(pad + "\t\t\tgluPopRoots(c, " + stepRoots + ")")
		b.line(pad + "\t\t}")
		b.line(pad + "\t\tgluPopRoots(c, " + limitRoots + ")")
		b.line(pad + "\t}")
		b.line(pad + "\tgluPopRoots(c, " + initRoots + ")")
		b.line(pad + "}")
		g.activeRootSlots = rootSlots
	case *luaparser.GenericForStmt:
		g.emitGenericFor(b, s, indent)
	case *luaparser.ReturnStmt:
		g.emitReturn(b, s, indent)
	case *luaparser.BreakStmt:
		b.line(pad + "break")
	}
}

func (g *generator) emitMonomorphicNumericFor(b *sourceBuilder, s *luaparser.NumericForStmt, indent int) bool {
	if !g.exprNumber(s.Init) || !g.exprNumber(s.Limit) || !g.exprNumber(s.Step) || numericForVariableReassigned(s) {
		return false
	}
	if g.emitIntegerNumericFor(b, s, indent) {
		return true
	}
	pad := strings.Repeat("\t", indent)
	id := g.next("numFor")
	initExpr := g.emitNumberExpr(s.Init)
	g.writePending(b, indent)
	b.line(pad + id + "Init := " + initExpr)
	limitExpr := g.emitNumberExpr(s.Limit)
	g.writePending(b, indent)
	b.line(pad + id + "Limit := " + limitExpr)
	stepExpr := g.emitNumberExpr(s.Step)
	g.writePending(b, indent)
	b.line(pad + id + "Step := " + stepExpr)
	restoreViews := g.borrowLoopViews(b, s.Body, indent)
	defer restoreViews()
	appendPosition, restoreAppend := g.beginKnownAppendLoop(b, s, indent)
	defer restoreAppend()
	g.pushScope()
	loopVar := g.declareLocalKind(s.Name, monoNumber)
	b.line(pad + "var " + loopVar + " rt.Number")
	b.line(pad + "_ = " + loopVar)
	b.line(pad + "for (" + id + "Step >= 0 && " + id + "Init <= " + id + "Limit) || (" + id + "Step < 0 && " + id + "Init >= " + id + "Limit) {")
	b.line(pad + "\t" + loopVar + " = " + id + "Init")
	g.pushScope()
	for _, stmt := range s.Body.Body {
		g.emitStmt(b, stmt, indent+1)
	}
	g.popScope()
	if appendPosition != "" {
		b.line(pad + "\t" + appendPosition + "++")
	}
	b.line(pad + "\t" + id + "Init += " + id + "Step")
	b.line(pad + "}")
	g.popScope()
	return true
}

func (g *generator) emitIntegerNumericFor(b *sourceBuilder, s *luaparser.NumericForStmt, indent int) bool {
	if g.opts.DisableIntegerNumericFor {
		return false
	}
	init, limit, step, ok := integerNumericForBounds(s)
	if !ok {
		return false
	}
	count := (limit-init)/step + 1
	next := init + count*step
	if count < 1 || next < -32768 || next > 32767 {
		return false
	}
	pad := strings.Repeat("\t", indent)
	id := g.next("intFor")
	restoreViews := g.borrowLoopViews(b, s.Body, indent)
	defer restoreViews()
	appendPosition, restoreAppend := g.beginKnownAppendLoop(b, s, indent)
	defer restoreAppend()
	g.pushScope()
	used := numericForVariableReferenced(s)
	loopVar := ""
	if used {
		loopVar = g.declareLocalKind(s.Name, monoNumber)
		b.line(pad + "var " + loopVar + " rt.Number")
		b.line(pad + "_ = " + loopVar)
	}
	b.line(pad + integerNumericForHeader(id, init, limit, step))
	if used {
		b.line(pad + "\t" + loopVar + " = rt.FromInt(" + id + ")")
	}
	g.pushScope()
	for _, stmt := range s.Body.Body {
		g.emitStmt(b, stmt, indent+1)
	}
	g.popScope()
	if appendPosition != "" {
		b.line(pad + "\t" + appendPosition + "++")
	}
	b.line(pad + "}")
	g.popScope()
	return true
}

func (g *generator) emitIfChain(b *sourceBuilder, indent int, cond luaparser.Expr, thenBody []luaparser.Stmt, elseIfs []*luaparser.IfBranch, elseBody []luaparser.Stmt) {
	pad := strings.Repeat("\t", indent)
	condExpr := g.emitConditionFlush(b, indent, cond)
	b.line(pad + "if (" + condExpr + ") {")
	g.pushScope()
	for _, q := range thenBody {
		g.emitStmt(b, q, indent+1)
	}
	g.popScope()
	if len(elseIfs) == 0 && elseBody == nil {
		b.line(pad + "}")
		return
	}
	b.line(pad + "} else {")
	if len(elseIfs) > 0 {
		next := elseIfs[0]
		var nextThen []luaparser.Stmt
		if next.Body != nil {
			nextThen = next.Body.Body
		}
		g.emitIfChain(b, indent+1, next.Condition, nextThen, elseIfs[1:], elseBody)
	} else {
		g.pushScope()
		for _, q := range elseBody {
			g.emitStmt(b, q, indent+1)
		}
		g.popScope()
	}
	b.line(pad + "}")
}

func (g *generator) emitAssign(b *sourceBuilder, s *luaparser.AssignStmt, indent int) {
	pad := strings.Repeat("\t", indent)
	if len(g.scopes) == 0 && len(s.Targets) == 1 {
		if variable, ok := s.Targets[0].(*luaparser.VarExpr); ok {
			if info := g.numericConstants[variable.Name]; info != nil && info.declaration == s {
				return
			}
			if info := g.numericRecords[variable.Name]; info != nil && info.declaration == s {
				for _, field := range info.fieldOrder {
					value := g.emitNumberExpr(field.value)
					g.writePending(b, indent)
					b.line(pad + "g." + numericRecordMember(info, field) + " = " + value)
				}
				return
			}
		}
	}
	// Lua evaluates every l-value table/key, then every r-value, then stores.
	// Materializing both sides into temps is what makes `a, b = b, a` and
	// `i, t[i] = 2, 99` match Lua instead of Go's left-to-right stores.
	oldRoots := g.extraRoots
	g.extraRoots = append([]string(nil), oldRoots...)
	type lhs struct {
		kind  int
		mono  monoKind
		dest  string
		base  string
		key   string
		field string
		cache string
		view  string
	}
	const (
		lhsVar = iota
		lhsIndex
		lhsField
		lhsNumberArray
		lhsNumericRecord
	)
	slots := make([]lhs, len(s.Targets))
	for i, target := range s.Targets {
		switch t := target.(type) {
		case *luaparser.VarExpr:
			slots[i] = lhs{kind: lhsVar, mono: g.lookupKind(t.Name), dest: g.lookup(t.Name)}
		case *luaparser.IndexExpr:
			if name, _, ok := g.numericArrayIndex(t); ok {
				key := g.next("lhs")
				b.line(pad + key + " := " + g.emitExprFlush(b, indent, t.Key))
				g.extraRoots = append(g.extraRoots, key)
				slots[i] = lhs{kind: lhsNumberArray, mono: monoNumber, base: g.lookup(name), key: key, view: g.borrowedNumberArrays[name]}
				continue
			}
			base := g.next("lhs")
			key := g.next("lhs")
			b.line(pad + base + " := " + g.emitExprFlush(b, indent, t.Base))
			g.extraRoots = append(g.extraRoots, base)
			b.line(pad + key + " := " + g.emitExprFlush(b, indent, t.Key))
			g.extraRoots = append(g.extraRoots, key)
			slots[i] = lhs{kind: lhsIndex, base: base, key: key}
		case *luaparser.FieldExpr:
			if record, field, ok := g.numericRecordField(t); ok {
				slots[i] = lhs{kind: lhsNumericRecord, mono: monoNumber, dest: "g." + numericRecordMember(record, field)}
				continue
			}
			base := g.next("lhs")
			b.line(pad + base + " := " + g.emitExprFlush(b, indent, t.Base))
			g.extraRoots = append(g.extraRoots, base)
			mono := monoValue
			if g.exprNumber(t) {
				mono = monoNumber
			}
			slots[i] = lhs{kind: lhsField, mono: mono, base: base, field: t.Name, cache: g.fieldCacheRef(t)}
		}
	}
	values := g.emitAdjustedValues(b, s.Values, len(s.Targets), indent)
	// Every table store may grow its table and collect. Keep all RHS results
	// alive until all stores have completed, because Lua evaluates the whole
	// assignment before performing its first write.
	for _, value := range values {
		if value.kind == monoValue && value.code != "rt.NilVal()" {
			g.extraRoots = append(g.extraRoots, value.code)
		}
	}
	for i, slot := range slots {
		switch slot.kind {
		case lhsVar:
			if slot.mono == monoNumber {
				b.line(pad + slot.dest + " = " + adjustedNumber(values[i]))
			} else {
				b.line(pad + slot.dest + " = " + adjustedLuaValue(values[i]))
			}
		case lhsIndex:
			b.line(pad + "_ = gluTableSet(c, " + slot.base + ", " + slot.key + ", " + adjustedLuaValue(values[i]) + g.rootArgsFor(6, s.Pos()) + ")")
		case lhsNumberArray:
			if slot.view != "" {
				b.line(pad + "_ = " + slot.view + ".Set(" + slot.key + ".Int(), " + adjustedNumber(values[i]) + ")")
			} else {
				b.line(pad + "_ = c.NumberArraySet(" + slot.base + ", " + slot.key + ".Int(), " + adjustedNumber(values[i]) + ")")
			}
		case lhsNumericRecord:
			b.line(pad + slot.dest + " = " + adjustedNumber(values[i]))
		case lhsField:
			if slot.mono == monoNumber && slot.cache != "" {
				g.emitStableWrite(b, indent, true, slot.base, g.staticExpr(slot.field), adjustedNumber(values[i]), slot.cache, g.rootArgsFor(5, s.Pos()))
			} else if slot.cache != "" {
				g.emitStableWrite(b, indent, false, slot.base, g.staticExpr(slot.field), adjustedLuaValue(values[i]), slot.cache, g.rootArgsFor(6, s.Pos()))
			} else {
				b.line(pad + "_ = gluTableSet(c, " + slot.base + ", " + g.staticExpr(slot.field) + ", " + adjustedLuaValue(values[i]) + g.rootArgsFor(6, s.Pos()) + ")")
			}
		}
	}
	g.extraRoots = oldRoots
}

// emitLocalAssign lowers `local a, b, ... = values...`. RHS is fully
// evaluated (and last-call-expanded) before any new name is declared, so
// `local x, y = y, x` and `local x = x + 1` read the old bindings.
func (g *generator) emitLocalAssign(b *sourceBuilder, s *luaparser.LocalAssignStmt, indent int) {
	pad := strings.Repeat("\t", indent)
	// A `local` at chunk top level (outside every function, hence no active
	// scope) names a Game struct field per collectTopLevel/addGlobal, not a
	// stack local: it must stay reachable from every function, which only a
	// struct field (not a scope-stack entry that dies at chunk-code's end)
	// can provide. Route it through the same global assignment path as a
	// plain top-level `x = ...`.
	topLevel := len(g.scopes) == 0
	if topLevel && len(s.Names) == 1 {
		if info := g.numericConstants[s.Names[0]]; info != nil && info.declaration == s {
			return
		}
		if info := g.numericRecords[s.Names[0]]; info != nil && info.declaration == s {
			for _, field := range info.fieldOrder {
				value := g.emitNumberExpr(field.value)
				g.writePending(b, indent)
				b.line(pad + "g." + numericRecordMember(info, field) + " = " + value)
			}
			return
		}
		if info := g.numericArrays[s.Names[0]]; info != nil && info.declaration == s {
			array := g.lookup(s.Names[0])
			errName := g.next("numArrayErr")
			b.line(pad + "var " + errName + " rt.HeapError")
			b.line(pad + array + ", " + errName + " = c.NewNumberArray(" + strconv.Itoa(info.length) + ")")
			b.line(pad + "if " + errName + " != rt.HeapOK { " + array + " = rt.NilVal() }")
			for index, field := range info.literal.Fields {
				b.line(pad + "_ = c.NumberArraySet(" + array + ", " + strconv.Itoa(index+1) + ", " + g.emitNumberExpr(field.Value) + ")")
			}
			return
		}
	}
	values := g.emitAdjustedValues(b, s.Values, len(s.Names), indent)
	kinds := make([]monoKind, len(s.Names))
	for i, name := range s.Names {
		if topLevel {
			kinds[i] = g.globalKinds[name]
		} else {
			kinds[i] = values[i].kind
		}
	}
	declare := func(name string, value adjustedValue, kind monoKind) {
		if topLevel {
			if kind == monoNumber {
				b.line(pad + g.lookup(name) + " = " + adjustedNumber(value))
			} else {
				b.line(pad + g.lookup(name) + " = " + adjustedLuaValue(value))
			}
			return
		}
		govar := g.declareLocalKind(name, kind)
		if kind == monoNumber {
			b.line(pad + "var " + govar + " rt.Number = " + adjustedNumber(value))
		} else {
			b.line(pad + "var " + govar + " rt.Value = " + adjustedLuaValue(value))
		}
		b.line(pad + "_ = " + govar)
	}
	for i, name := range s.Names {
		declare(name, values[i], kinds[i])
	}
}

func (g *generator) emitReturn(b *sourceBuilder, s *luaparser.ReturnStmt, indent int) {
	pad := strings.Repeat("\t", indent)
	if g.current == nil {
		// A Lua chunk may return values even though Game.Init has no Go return
		// slots. The values are discarded, but their expressions must still run
		// left-to-right for side effects before the chunk exits.
		g.emitAdjustedValues(b, s.Values, 0, indent)
		g.emitActiveRootCleanup(b, indent)
		b.line(pad + "return")
		return
	}
	if g.current.returnN == 0 {
		// Even a zero-result trailing call must execute for side effects:
		// `return bump()` is not equivalent to a bare return.
		g.emitAdjustedValues(b, s.Values, 0, indent)
		g.emitActiveRootCleanup(b, indent)
		b.line(pad + "return")
		return
	}
	if g.current.returnN == 1 && g.current.returnKind(0) == monoNumber &&
		len(s.Values) == 1 && g.exprNumber(s.Values[0]) {
		value := g.emitNumberExpr(s.Values[0])
		g.writePending(b, indent)
		if len(g.activeRootCounts) > 0 {
			result := g.next("return")
			b.line(pad + result + " := " + value)
			g.emitActiveRootCleanup(b, indent)
			value = result
		}
		b.line(pad + "return " + value)
		return
	}
	values := g.emitAdjustedValues(b, s.Values, g.current.returnN, indent)
	parts := make([]string, len(values))
	for i, value := range values {
		if g.current.returnKind(i) == monoNumber {
			if value.kind != monoNumber {
				g.setErr(fmt.Errorf("transpiler: internal numeric return mismatch in %s result %d at %s", g.current.key, i+1, s.Pos()))
			}
			parts[i] = adjustedNumber(value)
		} else {
			parts[i] = adjustedLuaValue(value)
		}
	}
	if len(g.activeRootCounts) > 0 {
		for i, part := range parts {
			result := g.next("return")
			b.line(pad + result + " := " + part)
			parts[i] = result
		}
		g.emitActiveRootCleanup(b, indent)
	}
	b.line(pad + "return " + strings.Join(parts, ", "))
}

// emitActiveRootCleanup unwinds roots owned by every generated control-flow
// scope that a Lua return exits. Counts are popped in reverse push order so
// nested numeric and generic loops obey Context's local-root stack protocol.
func (g *generator) emitActiveRootCleanup(b *sourceBuilder, indent int) {
	pad := strings.Repeat("\t", indent)
	for i := len(g.activeRootCounts) - 1; i >= 0; i-- {
		b.line(pad + "gluPopRoots(c, " + g.activeRootCounts[i] + ")")
	}
}

type adjustedValue struct {
	code string
	kind monoKind
}

func adjustedLuaValue(value adjustedValue) string {
	if value.kind == monoNumber {
		return "rt.NumVal(" + value.code + ")"
	}
	return value.code
}

func adjustedNumber(value adjustedValue) string {
	if value.kind == monoNumber {
		return value.code
	}
	return "(" + value.code + ").NumberOr(0)"
}

// emitAdjustedValues evaluates a Lua expression list: every expression runs
// (extra results still execute for side effects), a trailing call expands to
// its declared results, and the list is then truncated or nil-padded to
// nWanted slots.
func (g *generator) emitAdjustedValues(b *sourceBuilder, values []luaparser.Expr, nWanted, indent int) []adjustedValue {
	pad := strings.Repeat("\t", indent)
	out := make([]adjustedValue, nWanted)
	for i := range out {
		out[i] = adjustedValue{code: "rt.NilVal()", kind: monoValue}
	}
	if len(values) == 0 {
		return out
	}
	savedRoots := g.extraRoots
	g.extraRoots = append([]string(nil), savedRoots...)
	slot := 0
	last := len(values) - 1
	for i, v := range values {
		if i == last {
			if call, ok := v.(*luaparser.CallExpr); ok {
				arity, known := g.callResultArity(call)
				if known && arity == 0 {
					// A known zero-result call expands to no value slots,
					// but it still executes when it is the trailing expression.
					if callExpr := g.emitCall(call, false); callExpr != "" {
						g.writePending(b, indent)
						b.line(pad + callExpr)
					} else {
						g.writePending(b, indent)
					}
					continue
				}
				if fn := g.calledFunction(call); fn != nil && fn.returnN > 1 {
					parts := make([]string, fn.returnN)
					used := 0
					for j := range parts {
						if slot+used < nWanted {
							parts[j] = g.next("ret")
							used++
						} else {
							parts[j] = "_"
						}
					}
					rhs := g.emitMultiCall(call, fn)
					g.writePending(b, indent)
					if used == 0 {
						// A trailing multi-return call can be beyond the
						// assignment's target list (`local a = 1, pair()`).
						// A short declaration containing only blank identifiers
						// is invalid Go, but the call itself still has to run.
						b.line(pad + rhs)
					} else {
						b.line(pad + strings.Join(parts, ", ") + " := " + rhs)
					}
					for j := 0; j < used; j++ {
						kind := fn.returnKind(j)
						out[slot] = adjustedValue{code: parts[j], kind: kind}
						if kind == monoValue {
							g.extraRoots = append(g.extraRoots, parts[j])
						}
						slot++
					}
					continue
				}
			}
		}
		kind := monoValue
		var expr string
		if g.exprNumber(v) {
			kind = monoNumber
			expr = g.emitNumberExpr(v)
			g.writePending(b, indent)
		} else {
			expr = g.emitExprFlush(b, indent, v)
		}
		tmp := g.next("rhs")
		b.line(pad + tmp + " := " + expr)
		if slot < nWanted {
			out[slot] = adjustedValue{code: tmp, kind: kind}
			if kind == monoValue {
				g.extraRoots = append(g.extraRoots, tmp)
			}
			slot++
		} else {
			b.line(pad + "_ = " + tmp)
		}
	}
	g.extraRoots = savedRoots
	return out
}

func (g *generator) writePending(b *sourceBuilder, indent int) {
	if len(g.pending) == 0 {
		return
	}
	pad := strings.Repeat("\t", indent)
	for _, s := range g.pending {
		for _, line := range strings.Split(s, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			b.line(pad + line)
		}
	}
	g.pending = g.pending[:0]
}

func (g *generator) emitExprFlush(b *sourceBuilder, indent int, e luaparser.Expr) string {
	expr := g.emitExpr(e)
	g.writePending(b, indent)
	return expr
}

func (g *generator) capturePending(fn func() string) (stmts []string, expr string) {
	saved := g.pending
	g.pending = nil
	expr = fn()
	stmts = g.pending
	g.pending = saved
	return stmts, expr
}

func (g *generator) indentPending(stmts []string) {
	for _, s := range stmts {
		for _, line := range strings.Split(s, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			g.pending = append(g.pending, "\t"+line)
		}
	}
}

// sequenceExprs evaluates a list of Lua expressions in the same order Lua
// would, one Go statement at a time, each bound to its own temp variable
// and immediately added to g.extraRoots. Statements are appended to
// g.pending so the enclosing statement can flush them without wrapping an
// immediately-invoked function (TinyGo -gc=none cannot link runtime.alloc).
// With 0 or 1 expressions there is no sibling to race against, so no extra
// temp is introduced (nested pending from that one expression still runs).
func (g *generator) sequenceExprs(exprs []luaparser.Expr) []string {
	names := make([]string, len(exprs))
	if len(exprs) == 0 {
		return names
	}
	savedRoots := g.extraRoots
	g.extraRoots = append([]string(nil), savedRoots...)
	for i, e := range exprs {
		expr := g.emitExpr(e)
		if len(exprs) == 1 {
			names[i] = expr
			continue
		}
		tmp := g.next("op")
		g.pending = append(g.pending, tmp+" := "+expr)
		names[i] = tmp
		g.extraRoots = append(g.extraRoots, tmp)
	}
	g.extraRoots = savedRoots
	return names
}

func (g *generator) emitGenericFor(b *sourceBuilder, s *luaparser.GenericForStmt, indent int) {
	pad := strings.Repeat("\t", indent)
	if len(s.Values) != 1 {
		g.setErr(errors.New("transpiler: generic for requires one all(t) or pairs(t) iterator"))
		return
	}
	call, ok := s.Values[0].(*luaparser.CallExpr)
	if !ok {
		g.setErr(errors.New("transpiler: generic for requires all(t) or pairs(t)"))
		return
	}
	name := ""
	if v, ok := call.Callee.(*luaparser.VarExpr); ok {
		name = v.Name
	}
	if (name != "all" && name != "pairs") || g.exprListWidth(call.Args) != 1 {
		g.setErr(errors.New("transpiler: generic for requires all(t) or pairs(t) with one table argument"))
		return
	}
	expected := 1
	if name == "pairs" {
		expected = 2
	}
	if len(s.Names) != expected {
		g.setErr(fmt.Errorf("transpiler: %s iterator requires %d loop variables", name, expected))
		return
	}
	table := "gluIter" + g.next("t")
	tableExpr := g.emitExprFlush(b, indent, firstArg(call))
	b.line(pad + table + " := " + tableExpr)
	root := g.next("iterRoot")
	g.requireActiveRootSlots(1, s.Pos())
	b.line(pad + root + " := gluPushRoots(c, " + table + ")")
	b.line(pad + "if " + root + " >= 0 {")
	rootDepth := len(g.activeRootCounts)
	g.activeRootCounts = append(g.activeRootCounts, root)
	rootSlots := g.activeRootSlots
	g.activeRootSlots++
	g.pushScope()
	loopVars := make([]string, len(s.Names))
	for i, n := range s.Names {
		loopVars[i] = g.declareLocal(n)
	}
	switch name {
	case "all":
		idx := g.next("i")
		limit := "c.TableLen(" + table + ")"
		if g.iteratorLengthStable(s.Body) {
			limit = g.next("iterLen")
			b.line(pad + "\t" + limit + " := c.TableLen(" + table + ")")
		}
		b.line(pad + "\tfor " + idx + " := 1; " + idx + " <= " + limit + "; " + idx + "++ {")
		if len(loopVars) > 0 {
			b.line(pad + "\t\t" + loopVars[0] + " := c.TableGetInt(" + table + ", " + idx + ")")
			b.line(pad + "\t\t_ = " + loopVars[0])
		}
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+2)
		}
		g.popScope()
		b.line(pad + "\t}")
	case "pairs":
		idx := g.next("i")
		b.line(pad + "\tfor " + idx + " := 0; " + idx + " < c.TableSlotCount(" + table + "); " + idx + "++ {")
		b.line(pad + "\t\t" + g.next("key") + " := c.TableKeyAt(" + table + ", " + idx + ")")
		key := g.lastName("key")
		b.line(pad + "\t\tif " + key + ".Kind == rt.KindNil { continue }")
		if len(loopVars) > 0 {
			b.line(pad + "\t\t" + loopVars[0] + " := " + key)
			b.line(pad + "\t\t_ = " + loopVars[0])
		}
		if len(loopVars) > 1 {
			b.line(pad + "\t\t" + loopVars[1] + " := c.TableValueAt(" + table + ", " + idx + ")")
			b.line(pad + "\t\t_ = " + loopVars[1])
		}
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+2)
		}
		g.popScope()
		b.line(pad + "\t}")
	default:
		g.setErr(fmt.Errorf("transpiler: unsupported iterator %q; use all or pairs", name))
	}
	g.popScope()
	g.activeRootCounts = g.activeRootCounts[:rootDepth]
	g.activeRootSlots = rootSlots
	b.line(pad + "\tgluPopRoots(c, " + root + ")")
	b.line(pad + "}")
}

func firstArg(c *luaparser.CallExpr) luaparser.Expr {
	if len(c.Args) == 0 {
		return &luaparser.NilExpr{}
	}
	return c.Args[0]
}

// fixedNumberLiteral mirrors runtime.Number's signed 16:16 conversion. The
// explicit int32 step preserves the runtime's two's-complement wrapping while
// keeping the emitted Go constant within the Number type's representable range.
func fixedNumberLiteral(value float64) int32 {
	return int32(int64(math.Round(value * 65536)))
}

// targetIntLiteral reports whether a source integer can be passed through a
// generated int parameter on the 32-bit GBA target. Larger integers are still
// valid 16:16 values, but must be emitted as their already-wrapped Number bits.
func targetIntLiteral(value float64) (int64, bool) {
	if value < -2147483648 || value > 2147483647 {
		return 0, false
	}
	return int64(value), true
}
