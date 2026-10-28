package transpiler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/luaparser"
)

func (g *generator) emitExpr(e luaparser.Expr) string {
	if g.exprNumber(e) {
		return "rt.NumVal(" + g.emitNumberExpr(e) + ")"
	}
	switch x := e.(type) {
	case nil:
		return "rt.NilVal()"
	case *luaparser.NilExpr:
		return "rt.NilVal()"
	case *luaparser.BoolExpr:
		return fmt.Sprintf("rt.BoolVal(%t)", x.Value)
	case *luaparser.NumberExpr:
		if x.Integer {
			if value, ok := targetIntLiteral(x.Value); ok {
				return fmt.Sprintf("rt.IntVal(%d)", value)
			}
		}
		return fmt.Sprintf("rt.NumVal(rt.Number(%d))", fixedNumberLiteral(x.Value))
	case *luaparser.StringExpr:
		return g.staticExpr(x.Value)
	case *luaparser.ParenExpr:
		return g.emitExpr(x.Expr)
	case *luaparser.VarExpr:
		return g.lookup(x.Name)
	case *luaparser.UnaryExpr:
		if x.Op == luaparser.OpLength {
			if base, ok := x.Expr.(*luaparser.VarExpr); ok {
				if info := g.numericArrays[base.Name]; info != nil {
					if _, local := g.lookupLocal(base.Name); !local {
						return "rt.IntVal(" + strconv.Itoa(info.length) + ")"
					}
				}
			}
		}
		v := g.emitExpr(x.Expr)
		switch x.Op {
		case luaparser.OpNot:
			return "rt.BoolVal(!(" + v + ").Truthy())"
		case luaparser.OpNegate:
			return "rt.NumVal(-(" + v + ").NumberOr(0))"
		case luaparser.OpBitNot:
			return "rt.NumVal((" + v + ").NumberOr(0).Bnot())"
		case luaparser.OpLength:
			return "gluLen(c," + v + ")"
		}
	case *luaparser.BinaryExpr:
		return g.emitBinary(x)
	case *luaparser.IndexExpr:
		names := g.sequenceExprs([]luaparser.Expr{x.Base, x.Key})
		return "c.TableGet(" + names[0] + ", " + names[1] + ")"
	case *luaparser.FieldExpr:
		if record, field, ok := g.numericRecordField(x); ok {
			return "rt.NumVal(g." + numericRecordMember(record, field) + ")"
		}
		if cache := g.fieldCacheRef(x); cache != "" {
			return "c.TableGetStable(" + g.emitExpr(x.Base) + ", " + g.staticExpr(x.Name) + ", " + cache + ")"
		}
		return "c.TableGet(" + g.emitExpr(x.Base) + ", " + g.staticExpr(x.Name) + ")"
	case *luaparser.CallExpr:
		return g.emitCall(x, true)
	case *luaparser.TableExpr:
		return g.emitTableLiteral(x)
	}
	return "rt.NilVal()"
}

func (g *generator) exprNumber(expr luaparser.Expr) bool {
	if g.opts.DisableMonomorphicNumbers {
		return false
	}
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return g.exprNumber(x.Expr)
	case *luaparser.NumberExpr:
		return true
	case *luaparser.VarExpr:
		return g.lookupKind(x.Name) == monoNumber
	case *luaparser.FieldExpr:
		if _, _, ok := g.numericRecordField(x); ok {
			return true
		}
		return !g.opts.DisableStableTableFields && g.numericFields[x.Name]
	case *luaparser.IndexExpr:
		_, _, ok := g.numericArrayIndex(x)
		return ok
	case *luaparser.UnaryExpr:
		if x.Op == luaparser.OpLength {
			if base, ok := x.Expr.(*luaparser.VarExpr); ok {
				if info := g.numericArrays[base.Name]; info != nil {
					if _, local := g.lookupLocal(base.Name); !local {
						return true
					}
				}
			}
			return !g.opts.DisableNumericLengths
		}
		return (x.Op == luaparser.OpNegate || x.Op == luaparser.OpBitNot) && g.exprNumber(x.Expr)
	case *luaparser.BinaryExpr:
		switch x.Op {
		case luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
			luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo, luaparser.OpPower,
			luaparser.OpBitOr, luaparser.OpBitXor, luaparser.OpBitAnd,
			luaparser.OpShiftL, luaparser.OpShiftR, luaparser.OpShiftRLogical,
			luaparser.OpRotateL, luaparser.OpRotateR:
			return g.exprNumber(x.Left) && g.exprNumber(x.Right)
		}
	case *luaparser.CallExpr:
		name, ok := callKey(x)
		if !ok {
			return false
		}
		if fn := g.calledFunction(x); fn != nil {
			return fn.returnN > 0 && fn.returnKind(0) == monoNumber
		}
		if name == "min" || name == "max" || name == "mid" || name == "mget" || name == "save_get" || name == "save_read" || name == "save_write" {
			return true
		}
		requireNumbers := func() bool {
			for i, arg := range x.Args {
				if i == len(x.Args)-1 {
					if call, ok := arg.(*luaparser.CallExpr); ok {
						if fn := g.calledFunction(call); fn != nil && fn.returnN > 1 {
							for result := 0; result < fn.returnN; result++ {
								if fn.returnKind(result) != monoNumber {
									return false
								}
							}
							continue
						}
					}
				}
				if !g.exprNumber(arg) {
					return false
				}
			}
			return true
		}
		switch name {
		case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd",
			"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr", "t", "time", "stat":
			return requireNumbers()
		}
	}
	return false
}

func (g *generator) emitNumberExpr(expr luaparser.Expr) string {
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return g.emitNumberExpr(x.Expr)
	case *luaparser.NumberExpr:
		return fmt.Sprintf("rt.Number(%d)", fixedNumberLiteral(x.Value))
	case *luaparser.VarExpr:
		return g.lookup(x.Name)
	case *luaparser.FieldExpr:
		if record, field, ok := g.numericRecordField(x); ok {
			return "g." + numericRecordMember(record, field)
		}
		return g.emitStableNumberRead(x)
	case *luaparser.IndexExpr:
		if name, _, ok := g.numericArrayIndex(x); ok {
			key := g.emitExpr(x.Key)
			if view := g.borrowedNumberArrays[name]; view != "" {
				return view + ".Get((" + key + ").Int())"
			}
			return "c.NumberArrayGet(" + g.lookup(name) + ", (" + key + ").Int())"
		}
	case *luaparser.UnaryExpr:
		if x.Op == luaparser.OpLength {
			if base, ok := x.Expr.(*luaparser.VarExpr); ok {
				if info := g.numericArrays[base.Name]; info != nil {
					if _, local := g.lookupLocal(base.Name); !local {
						return "rt.FromInt(" + strconv.Itoa(info.length) + ")"
					}
				}
			}
			// gluLen returns IntVal for every operand kind, including zero
			// for unsupported types. Its input must still evaluate normally.
			return "(gluLen(c," + g.emitExpr(x.Expr) + ")).Num"
		}
		value := g.emitNumberExpr(x.Expr)
		if x.Op == luaparser.OpNegate {
			// A variable forces signed 16:16 wrapping instead of Go constant
			// overflow, including parenthesized literals and inlined globals.
			tmp := g.next("neg")
			g.pending = append(g.pending, tmp+" := "+value)
			return "-(" + tmp + ")"
		}
		return "(" + value + ").Bnot()"
	case *luaparser.BinaryExpr:
		if specialized, ok := g.emitConstantNumberDivision(x); ok {
			return specialized
		}
		if specialized, ok := g.emitConstantNumberModulo(x); ok {
			return specialized
		}
		parts := g.sequenceNumberExprs([]luaparser.Expr{x.Left, x.Right})
		left, right := parts[0], parts[1]
		switch x.Op {
		case luaparser.OpAdd:
			return "(" + left + " + " + right + ")"
		case luaparser.OpSubtract:
			return "(" + left + " - " + right + ")"
		case luaparser.OpMultiply:
			return "(" + left + ").Mul(" + right + ")"
		case luaparser.OpDivide:
			return "(" + left + ").Div(" + right + ")"
		case luaparser.OpIntegerDivide:
			return "(" + left + ").IDiv(" + right + ")"
		case luaparser.OpModulo:
			return "gluNumberMod(" + left + ", " + right + ")"
		case luaparser.OpPower:
			return "gluNumberPow(" + left + ", " + right + ")"
		case luaparser.OpBitOr:
			return "(" + left + ").Bor(" + right + ")"
		case luaparser.OpBitXor:
			return "(" + left + ").Bxor(" + right + ")"
		case luaparser.OpBitAnd:
			return "(" + left + ").Band(" + right + ")"
		case luaparser.OpShiftL:
			return "(" + left + ").Shl(" + right + ")"
		case luaparser.OpShiftR:
			return "(" + left + ").Shr(" + right + ")"
		case luaparser.OpShiftRLogical:
			return "(" + left + ").Lshr(" + right + ")"
		case luaparser.OpRotateL:
			return "(" + left + ").Rotl(" + right + ")"
		case luaparser.OpRotateR:
			return "(" + left + ").Rotr(" + right + ")"
		}
	case *luaparser.CallExpr:
		if fn := g.calledFunction(x); fn != nil && fn.returnN > 0 && fn.returnKind(0) == monoNumber {
			if fn.returnN == 1 {
				return g.emitUserCall(x, fn)
			}
			return g.emitFirstCall(x, fn)
		}
		name, _ := callKey(x)
		if name == "mget" {
			return "(" + g.emitBuiltin("mget", x.Args, true) + ").Num"
		}
		if name == "save_get" || name == "save_read" || name == "save_write" {
			// emitCall sequences the operands. Do not sequence them here too:
			// a dynamic field-index expression must execute exactly once.
			return "(" + g.emitCall(x, true) + ").NumberOr(0)"
		}
		var arg func(int, string) string
		if g.numberBuiltinArgs(x.Args) {
			args := g.sequenceNumberExprs(x.Args)
			arg = func(i int, fallback string) string {
				if i >= len(args) {
					return fallback
				}
				return args[i]
			}
		} else {
			values := g.sequenceAdjustedExprs(x.Args)
			args := make([]string, len(values))
			for i, value := range values {
				args[i] = adjustedNumber(value)
			}
			arg = func(i int, fallback string) string {
				if i >= len(args) {
					return fallback
				}
				return args[i]
			}
		}
		switch name {
		case "flr":
			return "(" + arg(0, "0") + ").Flr()"
		case "ceil":
			return "(" + arg(0, "0") + ").Ceil()"
		case "abs":
			return "(" + arg(0, "0") + ").Abs()"
		case "sgn":
			return "(" + arg(0, "0") + ").Sgn()"
		case "min":
			return "rt.Min(" + arg(0, "0") + ", " + arg(1, "0") + ")"
		case "max":
			return "rt.Max(" + arg(0, "0") + ", " + arg(1, "0") + ")"
		case "mid":
			return "rt.Mid(" + arg(0, "0") + ", " + arg(1, "0") + ", " + arg(2, "0") + ")"
		case "sin":
			return "(" + arg(0, "0") + ").Sin()"
		case "cos":
			return "(" + arg(0, "0") + ").Cos()"
		case "sqrt":
			return "(" + arg(0, "0") + ").Sqrt()"
		case "atan2":
			return "rt.Atan2(" + arg(0, "0") + ", " + arg(1, "0") + ")"
		case "rnd":
			return "c.Rnd(" + arg(0, "rt.One") + ")"
		case "band":
			return "(" + arg(0, "0") + ").Band(" + arg(1, "0") + ")"
		case "bor":
			return "(" + arg(0, "0") + ").Bor(" + arg(1, "0") + ")"
		case "bxor":
			return "(" + arg(0, "0") + ").Bxor(" + arg(1, "0") + ")"
		case "bnot":
			return "(" + arg(0, "0") + ").Bnot()"
		case "shl":
			return "(" + arg(0, "0") + ").Shl(" + arg(1, "0") + ")"
		case "shr":
			return "(" + arg(0, "0") + ").Shr(" + arg(1, "0") + ")"
		case "lshr":
			return "(" + arg(0, "0") + ").Lshr(" + arg(1, "0") + ")"
		case "rotl":
			return "(" + arg(0, "0") + ").Rotl(" + arg(1, "0") + ")"
		case "rotr":
			return "(" + arg(0, "0") + ").Rotr(" + arg(1, "0") + ")"
		case "t", "time":
			return "c.Time()"
		case "stat":
			return "c.Stat((" + arg(0, "0") + ").Int())"
		}
	}
	g.setErr(fmt.Errorf("transpiler: unsupported monomorphic number expression at %s", expr.Pos()))
	return "0"
}

func (g *generator) numberBuiltinArgs(args []luaparser.Expr) bool {
	for _, arg := range args {
		if !g.exprNumber(arg) {
			return false
		}
		call, ok := unparenExpr(arg).(*luaparser.CallExpr)
		if !ok {
			continue
		}
		if fn := g.calledFunction(call); fn != nil && fn.returnN > 1 {
			return false
		}
	}
	return true
}

func (g *generator) sequenceNumberExprs(exprs []luaparser.Expr) []string {
	names := make([]string, len(exprs))
	for i, expr := range exprs {
		code := g.emitNumberExpr(expr)
		if len(exprs) == 1 {
			names[i] = code
			continue
		}
		tmp := g.next("num")
		g.pending = append(g.pending, tmp+" := "+code)
		names[i] = tmp
	}
	return names
}

func (g *generator) emitBinary(x *luaparser.BinaryExpr) string {
	if x.Op == luaparser.OpAnd || x.Op == luaparser.OpOr {
		// `and`/`or` short-circuit, so the right operand cannot run unless
		// the left requires it. Capture the right side's pending statements
		// into the corresponding if-body instead of wrapping a closure.
		leftStmts, leftExpr := g.capturePending(func() string { return g.emitExpr(x.Left) })
		g.pending = append(g.pending, leftStmts...)
		leftName := g.next("logic")
		g.pending = append(g.pending, leftName+" := "+leftExpr)
		savedRoots := g.extraRoots
		g.extraRoots = append(append([]string(nil), savedRoots...), leftName)
		rightStmts, rightExpr := g.capturePending(func() string { return g.emitExpr(x.Right) })
		g.extraRoots = savedRoots
		result := g.next("logic")
		g.pending = append(g.pending, result+" := "+leftName)
		cond := leftName + ".Truthy()"
		if x.Op == luaparser.OpAnd {
			g.pending = append(g.pending, "if "+cond+" {")
		} else {
			g.pending = append(g.pending, "if !"+cond+" {")
		}
		g.indentPending(rightStmts)
		g.pending = append(g.pending, "\t"+result+" = "+rightExpr)
		g.pending = append(g.pending, "}")
		return result
	}
	if x.Op == luaparser.OpConcat {
		names := g.sequenceExprs([]luaparser.Expr{x.Left, x.Right})
		return "gluConcat(c," + names[0] + "," + names[1] + g.rootArgsFor(2, x.Pos()) + ")"
	}
	if specialized, ok := g.emitConstantValueArithmetic(x); ok {
		return specialized
	}
	if g.exprNumber(x.Left) && g.exprNumber(x.Right) {
		parts := g.sequenceNumberExprs([]luaparser.Expr{x.Left, x.Right})
		left, right := parts[0], parts[1]
		switch x.Op {
		case luaparser.OpEqual, luaparser.OpLess, luaparser.OpLessEq, luaparser.OpGreater, luaparser.OpGreaterEq:
			return "rt.BoolVal(" + left + " " + string(x.Op) + " " + right + ")"
		case luaparser.OpNotEqual:
			return "rt.BoolVal(" + left + " != " + right + ")"
		}
	}
	names := g.sequenceExprs([]luaparser.Expr{x.Left, x.Right})
	return g.emitEagerBinary(x.Op, names[0], names[1])
}

func (g *generator) emitEagerBinary(op luaparser.Operator, left, right string) string {
	switch op {
	case luaparser.OpEqual:
		return "rt.BoolVal(c.ValuesEqual(" + left + "," + right + "))"
	case luaparser.OpNotEqual:
		return "rt.BoolVal(!c.ValuesEqual(" + left + "," + right + "))"
	case luaparser.OpLess, luaparser.OpLessEq, luaparser.OpGreater, luaparser.OpGreaterEq:
		return "rt.BoolVal(gluCmp(c," + left + "," + right + ",\"" + string(op) + "\"))"
	case luaparser.OpAdd:
		return "(" + left + ").Add(" + right + ")"
	case luaparser.OpSubtract:
		return "(" + left + ").Sub(" + right + ")"
	case luaparser.OpMultiply:
		return "(" + left + ").Mul(" + right + ")"
	case luaparser.OpDivide:
		return "(" + left + ").Div(" + right + ")"
	case luaparser.OpIntegerDivide:
		return "(" + left + ").IDiv(" + right + ")"
	case luaparser.OpModulo:
		return "gluMod(" + left + "," + right + ")"
	case luaparser.OpPower:
		return "gluPow(" + left + "," + right + ")"
	case luaparser.OpBitOr:
		return "rt.NumVal((" + left + ").NumberOr(0).Bor((" + right + ").NumberOr(0)))"
	case luaparser.OpBitXor:
		return "rt.NumVal((" + left + ").NumberOr(0).Bxor((" + right + ").NumberOr(0)))"
	case luaparser.OpBitAnd:
		return "rt.NumVal((" + left + ").NumberOr(0).Band((" + right + ").NumberOr(0)))"
	case luaparser.OpShiftL:
		return "rt.NumVal((" + left + ").NumberOr(0).Shl((" + right + ").NumberOr(0)))"
	case luaparser.OpShiftR:
		return "rt.NumVal((" + left + ").NumberOr(0).Shr((" + right + ").NumberOr(0)))"
	case luaparser.OpShiftRLogical:
		return "rt.NumVal((" + left + ").NumberOr(0).Lshr((" + right + ").NumberOr(0)))"
	case luaparser.OpRotateL:
		return "rt.NumVal((" + left + ").NumberOr(0).Rotl((" + right + ").NumberOr(0)))"
	case luaparser.OpRotateR:
		return "rt.NumVal((" + left + ").NumberOr(0).Rotr((" + right + ").NumberOr(0)))"
	}
	return "rt.NilVal()"
}

func (g *generator) emitTableLiteral(x *luaparser.TableExpr) string {
	// Lua expands a multi-return call into all of its results only when it
	// is the constructor's last field and that field is a plain positional
	// (array) entry, not a [k]=v or name=v field; everywhere else - non-last
	// fields, hash fields - a multi-return call yields just its first value,
	// same as any other expression context.
	var trailingFn *functionInfo
	var trailingCall *luaparser.CallExpr
	trailingZero := false
	if n := len(x.Fields); n > 0 && x.Fields[n-1].Array {
		if call, ok := x.Fields[n-1].Value.(*luaparser.CallExpr); ok {
			if arity, known := g.callResultArity(call); known && arity == 0 {
				trailingZero, trailingCall = true, call
			} else if fn := g.calledFunction(call); fn != nil && fn.returnN > 1 {
				trailingFn, trailingCall = fn, call
			}
		}
	}
	arrayN := 0
	hashN := 0
	for _, f := range x.Fields {
		if f.Array {
			arrayN++
		} else {
			hashN++
		}
	}
	if trailingZero {
		arrayN--
	} else if trailingFn != nil {
		arrayN += trailingFn.returnN - 1
	}
	arrayN = max(arrayN, g.literalArrayCaps[x])
	arrayCap, hashCap := max(arrayN, 1), max(hashN, 1)
	literalPlans := g.staticTableLiteralPlans(x, arrayCap, hashCap)
	borrowLiteral := g.borrowableTableLiteral(x, literalPlans)
	trustedLiteral := borrowLiteral && g.trustedTableLiteralValues(x)
	image := g.tableLiteralImage(x, literalPlans, arrayCap, hashCap)
	keepTableRoot := g.tableLiteralNeedsLiveRoot(x)
	initialRoots := g.rootVars()
	skipRooting := len(initialRoots) == 0 && !keepTableRoot && trailingFn == nil && !trailingZero
	if skipRooting && image.name != "" {
		return g.emitTrustedImageConstructor(x, image)
	}
	if skipRooting && len(x.Fields) == 0 {
		return g.emitEmptyTableConstructor(arrayCap, hashCap)
	}
	tbl := g.next("tbl")
	fail := g.next("tblFail")
	pushed := ""
	errName := g.next("tblErr")
	g.pending = append(g.pending, "var "+tbl+" rt.Value")
	g.pending = append(g.pending, fail+" := false")
	// Existing Values only need to remain rooted across the allocation itself.
	// Once the table exists, release those temporary copies and keep just the
	// new table persistently rooted; nested field expressions will root the
	// caller's still-live Go Values only while they allocate.
	if skipRooting {
		g.pending = append(g.pending, "var "+errName+" rt.HeapError")
		if image.name != "" {
			g.pending = append(g.pending, tbl+", "+errName+" = c.NewTableFromImage("+image.name+"[:])")
		} else {
			g.pending = append(g.pending, tbl+", "+errName+" = c.NewTableWithCaps("+strconv.Itoa(arrayCap)+", "+strconv.Itoa(hashCap)+")")
		}
		g.pending = append(g.pending, "if "+errName+" != rt.HeapOK { "+fail+" = true }")
	} else {
		pushed = g.next("tblPush")
		g.pending = append(g.pending, pushed+" := 0")
		g.requireActiveRootSlots(len(initialRoots), x.Pos())
		for _, root := range initialRoots {
			g.pending = append(g.pending, "if !"+fail+" {")
			g.pending = append(g.pending, "\tif c.PushLocal("+root+") { "+pushed+"++ } else { "+fail+" = true }")
			g.pending = append(g.pending, "}")
		}
		g.pending = append(g.pending, "if !"+fail+" {")
		g.pending = append(g.pending, "\tvar "+errName+" rt.HeapError")
		if image.name != "" {
			g.pending = append(g.pending, "\t"+tbl+", "+errName+" = c.NewTableFromImage("+image.name+"[:])")
		} else {
			g.pending = append(g.pending, "\t"+tbl+", "+errName+" = c.NewTableWithCaps("+strconv.Itoa(arrayCap)+", "+strconv.Itoa(hashCap)+")")
		}
		g.pending = append(g.pending, "\tif "+errName+" != rt.HeapOK { "+fail+" = true }")
		g.pending = append(g.pending, "}")
		g.pending = append(g.pending, "c.PopLocals("+pushed+")")
		g.pending = append(g.pending, pushed+" = 0")
		if keepTableRoot {
			g.requireActiveRootSlots(1, x.Pos())
			g.pending = append(g.pending, "if !"+fail+" {")
			g.pending = append(g.pending, "\tif c.PushLocal("+tbl+") { "+pushed+" = 1 } else { "+fail+" = true }")
			g.pending = append(g.pending, "}")
		}
	}
	view := ""
	if image.name != "" && len(image.dynamic) != 0 || (borrowLiteral && image.name == "") {
		view = g.next("tblView")
		g.pending = append(g.pending, "var "+view+" rt.TableLiteralView")
		borrow := "c.BorrowTableLiteral(" + tbl + ")"
		if trustedLiteral && image.name == "" {
			borrow = "c.BorrowFreshTableLiteral(" + tbl + "," + strconv.Itoa(arrayCap) + "," + strconv.Itoa(hashCap) + ")"
		}
		g.pending = append(g.pending, "if !"+fail+" { "+view+" = "+borrow+" }")
	}
	oldRoots := g.extraRoots
	g.extraRoots = append([]string(nil), oldRoots...)
	rootSlots := g.activeRootSlots
	g.activeRootSlots++
	if trailingFn != nil {
		resultRoots := 0
		for i := 0; i < trailingFn.returnN; i++ {
			if trailingFn.returnKind(i) == monoValue {
				resultRoots++
			}
		}
		g.requireActiveRootSlots(resultRoots, x.Pos())
	}
	arrayIndex := 0
	for i, f := range x.Fields {
		last := i == len(x.Fields)-1
		firstArrayIndex := arrayIndex + 1
		if f.Array {
			if last && trailingZero {
				// A trailing zero-result call contributes no list field.
			} else if last && trailingFn != nil {
				arrayIndex += trailingFn.returnN
			} else {
				arrayIndex++
			}
		}
		if image.name != "" {
			if _, overlay := image.dynamic[i]; !overlay && !f.Array {
				continue
			}
		}
		g.pending = append(g.pending, "if !"+fail+" {")
		inner, _ := g.capturePending(func() string {
			if f.Array {
				if last && trailingZero {
					statement := g.emitCall(trailingCall, false)
					if statement != "" {
						g.pending = append(g.pending, statement)
					}
					return ""
				}
				if last && trailingFn != nil {
					results := make([]string, trailingFn.returnN)
					for j := range results {
						results[j] = g.next("ret")
					}
					rhs := g.emitMultiCall(trailingCall, trailingFn)
					g.pending = append(g.pending, strings.Join(results, ", ")+" := "+rhs)
					for j, r := range results {
						if trailingFn.returnKind(j) == monoValue {
							g.pending = append(g.pending, "if !c.PushLocal("+r+") { "+fail+" = true } else { "+pushed+"++ }")
						}
					}
					for j, r := range results {
						value := adjustedLuaValue(adjustedValue{code: r, kind: trailingFn.returnKind(j)})
						g.pending = append(g.pending, "if !"+fail+" {")
						g.pending = append(g.pending, "\tif c.TableSet("+tbl+",rt.IntVal("+strconv.Itoa(firstArrayIndex+j)+"),"+value+") != rt.HeapOK { "+fail+" = true }")
						g.pending = append(g.pending, "}")
					}
					return ""
				}
				v := g.emitExpr(f.Value)
				g.pending = append(g.pending, "if c.TableSet("+tbl+",rt.IntVal("+strconv.Itoa(firstArrayIndex)+"),"+v+") != rt.HeapOK { "+fail+" = true }")
				return ""
			}
			if image.name != "" {
				plan, overlay := image.dynamic[i]
				if !overlay {
					return ""
				}
				key, _ := unparenExpr(f.Key).(*luaparser.StringExpr)
				keyID, _ := g.staticID(key.Value)
				if _, nested := unparenExpr(f.Value).(*luaparser.TableExpr); nested {
					value := g.emitExpr(f.Value)
					g.pending = append(g.pending, "if !"+fail+" { "+view+" = c.BorrowTableLiteral("+tbl+") }")
					g.pending = append(g.pending, "if "+view+".StorePlanned("+strconv.Itoa(int(keyID))+","+value+","+strconv.FormatUint(uint64(plan), 10)+") != rt.HeapOK { "+fail+" = true }")
					return ""
				}
				if g.exprNumber(f.Value) {
					value := g.emitNumberExpr(f.Value)
					g.pending = append(g.pending, "if "+view+".StorePlannedNumber("+strconv.Itoa(int(keyID))+","+value+","+strconv.FormatUint(uint64(plan), 10)+") != rt.HeapOK { "+fail+" = true }")
				} else {
					value := g.emitExpr(f.Value)
					g.pending = append(g.pending, "if "+view+".StorePlanned("+strconv.Itoa(int(keyID))+","+value+","+strconv.FormatUint(uint64(plan), 10)+") != rt.HeapOK { "+fail+" = true }")
				}
				return ""
			}
			names := g.sequenceExprs([]luaparser.Expr{f.Key, f.Value})
			if plan, ok := literalPlans[i]; ok {
				callee := "c.TableInitStatic(" + tbl + ","
				if view != "" {
					callee = view + ".InitStatic("
					if trustedLiteral {
						callee = view + ".InitKnown("
					}
				}
				g.pending = append(g.pending, "if "+callee+"("+names[0]+").SRef,"+names[1]+","+strconv.FormatUint(uint64(plan), 10)+") != rt.HeapOK { "+fail+" = true }")
			} else {
				g.pending = append(g.pending, "if c.TableSet("+tbl+","+names[0]+","+names[1]+") != rt.HeapOK { "+fail+" = true }")
			}
			return ""
		})
		g.indentPending(inner)
		g.pending = append(g.pending, "}")
	}
	g.extraRoots = oldRoots
	g.activeRootSlots = rootSlots
	if !skipRooting {
		g.pending = append(g.pending, "c.PopLocals("+pushed+")")
	}
	g.pending = append(g.pending, "if "+fail+" { "+tbl+" = rt.NilVal() }")
	return tbl
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func checkedProduct(a, b int) (int, bool) {
	if a < 0 || b < 0 {
		return 0, false
	}
	if a == 0 || b == 0 {
		return 0, true
	}
	maxInt := int(^uint(0) >> 1)
	if a > maxInt/b {
		return 0, false
	}
	product := a * b
	if uint64(product) > uint64(^uint32(0)) {
		return 0, false
	}
	return product, true
}

func compactMapPlanes(data []uint16, width, height, layers int, mask uint8) []uint16 {
	planeCells := width * height
	if planeCells <= 0 || layers <= 0 || mask == 0 {
		return nil
	}
	out := make([]uint16, 0, planeCells*mapMaskCount(mask))
	for bg := 0; bg < layers; bg++ {
		if mask&(1<<uint(bg)) == 0 {
			continue
		}
		base := bg * planeCells
		if base+planeCells <= len(data) {
			out = append(out, data[base:base+planeCells]...)
		} else {
			out = append(out, make([]uint16, planeCells)...)
		}
	}
	return out
}

// mapMaskSlot maps a stable logical BG number to its dense allocation slot.
// A missing/empty plane returns -1 and is a runtime no-op.
func mapMaskSlot(mask uint8, bg int) int {
	if bg < 0 || bg >= assetsync.MapLayerCount || mask&(1<<uint(bg)) == 0 {
		return -1
	}
	return mapMaskCount(mask & uint8((1<<uint(bg))-1))
}
