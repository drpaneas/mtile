package transpiler

import (
	"errors"
	"fmt"
	"strings"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

func (g *generator) emitCall(call *luaparser.CallExpr, value bool) string {
	key, ok := callKey(call)
	name := key
	if !ok {
		name = call.Method
	}
	if spec, ok := Builtins[name]; ok {
		argCount := g.exprListWidth(call.Args)
		if argCount < spec.MinArgs || argCount > spec.MaxArgs {
			g.setErr(fmt.Errorf("transpiler: %s expects %d..%d arguments, got %d", name, spec.MinArgs, spec.MaxArgs, argCount))
			return "rt.NilVal()"
		}
		if at := g.knownAppendPositions[call]; at != "" {
			return g.emitKnownTableAppend(call, at)
		}
		if name == "anim" {
			if err := validateAnimationCall(call, g.opts); err != nil {
				g.setErr(err)
				return "rt.NilVal()"
			}
		}
		if (name == "pspr" || name == "anim") && g.opts.Renderer == RendererMode0 {
			return g.emitMode0Pspr(call, value)
		}
		if name == "map" && g.opts.Renderer == RendererMode0 && len(g.opts.TileLinks) != 0 {
			if allocation, ok := g.tileLinkPlan[call]; ok {
				g.pending = append(g.pending, fmt.Sprintf("c.SetTileLinkSites(%d, %d, %d)", allocation.site, allocation.window, allocation.stride))
			} else {
				g.pending = append(g.pending, "c.SetTileLinkSites(-1, 0, 0)")
			}
		}
		return g.emitBuiltin(name, call.Args, value)
	}
	var fn *functionInfo
	if ok {
		fn = g.funcs[key]
	}
	if fn == nil {
		g.setErr(fmt.Errorf("transpiler: unsupported call %q", name))
		return "rt.NilVal()"
	}
	if fn.returnN == 0 {
		callExpr := g.emitUserCall(call, fn)
		if value {
			if callExpr != "" {
				g.pending = append(g.pending, callExpr)
			}
			return "rt.NilVal()"
		}
		return callExpr
	}
	if fn.returnN > 1 {
		if value {
			first := g.emitFirstCall(call, fn)
			if fn.returnKind(0) == monoNumber {
				return "rt.NumVal(" + first + ")"
			}
			return first
		}
		return g.emitMultiCall(call, fn)
	}
	return g.emitUserCall(call, fn)
}

func (g *generator) emitMode0Pspr(call *luaparser.CallExpr, value bool) string {
	width, method, clipID := 5, "PSpr", ""
	if name, _ := callKey(call); name == "anim" {
		width = 6
		method, clipID = g.animationMethod(call.Args)
	}
	sequenced := call.Args
	if clipID != "" {
		sequenced = sequenced[1:]
	}
	adjusted := g.sequenceAdjustedExprs(sequenced)
	names := make([]string, len(adjusted))
	for i, item := range adjusted {
		names[i] = adjustedLuaValue(item)
	}
	if clipID != "" {
		names = append([]string{clipID}, names...)
	}
	arg := func(i int) string {
		if i >= len(names) {
			return "rt.Value{}"
		}
		return names[i]
	}
	parts := make([]string, width)
	for i := range parts {
		parts[i] = arg(i)
	}
	args := strings.Join(parts, ",")
	allocation, allocated := g.psprPlan[call]
	void := func(expr string) string {
		if !value {
			return expr
		}
		g.pending = append(g.pending, expr)
		return "rt.NilVal()"
	}
	if g.poseOBJResident != nil {
		return void("c." + method + "V(" + args + ")")
	}
	if !allocated {
		if g.current != nil && !lifecycleReachable(g.chunk)[g.current.key] {
			// Unreachable helpers are emitted too, but consume no draw slots.
			return void("c." + method + "V(" + args + ")")
		}
		g.setErr(mode0CallError(call, reasonMode0PsprShape, "Mode 0 pspr() has no proven draw allocation"))
		return "rt.NilVal()"
	}
	if allocation.count > 1 {
		return void(fmt.Sprintf("c.%sSiteFromBaseV(rt.IntVal(%d), rt.IntVal(%d), rt.IntVal(%d), %s)", method, allocation.site, allocation.window, allocation.stride, args))
	}
	return void(fmt.Sprintf("c.%sSiteV(rt.IntVal(%d), rt.IntVal(%d), %s)", method, allocation.site, allocation.window, args))
}

func (g *generator) emitBuiltin(name string, argExprs []luaparser.Expr, value bool) string {
	// A proven map-layer operand has already served its purpose by selecting a
	// fixed compact-plane path. Literals and immutable numeric constants are
	// pure, so do not materialize a dead Go temporary. Keep a placeholder below
	// so effective mget/mset positions remain unchanged, including trailing Lua
	// calls that expand to multiple results.
	skipMapLayerOperand := false
	effectiveArity := g.exprListWidth(argExprs)
	layeredMapCall := (name == "mget" && effectiveArity == 3 || name == "mset" && effectiveArity == 4) && len(argExprs) != 0
	if layeredMapCall {
		if !g.opts.DisableFixedMapLayerSpecialization && !g.opts.MapStreaming {
			_, skipMapLayerOperand = g.staticRuntimeInt(argExprs[0])
		}
		// Streaming maps and the specialization rollback still preserve the
		// original BG0 shortcut below. It also ignores a pure literal layer,
		// so omit that operand instead of declaring an unused Go temporary.
		if !skipMapLayerOperand {
			bg, literal := mode0StaticRuntimeInt(argExprs[0])
			skipMapLayerOperand = literal && bg == 0
		}
	}
	sequencedExprs := argExprs
	if name == "pvariant" && len(argExprs) == 1 {
		if _, literal := unparenExpr(argExprs[0]).(*luaparser.StringExpr); literal {
			sequencedExprs = nil
		}
	}
	animationMethod, animationID := "", ""
	if name == "anim" {
		animationMethod, animationID = g.animationMethod(argExprs)
		if animationID != "" {
			sequencedExprs = argExprs[1:]
		}
	}
	if skipMapLayerOperand {
		sequencedExprs = argExprs[1:]
	}
	adjusted := g.sequenceAdjustedExprs(sequencedExprs)
	names := make([]string, len(adjusted))
	for i, value := range adjusted {
		names[i] = adjustedLuaValue(value)
	}
	if skipMapLayerOperand {
		names = append([]string{""}, names...)
	}
	if animationID != "" {
		names = append([]string{animationID}, names...)
	}
	callPos := luaparser.Position{}
	if len(argExprs) > 0 {
		callPos = argExprs[0].Pos()
	}
	arg := func(i int) string {
		if i >= len(names) {
			return "rt.Value{}"
		}
		return names[i]
	}
	argsUpTo := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = arg(i)
		}
		return strings.Join(parts, ",")
	}
	wrap := func(call string) string { return call }
	void := func(call string) string {
		if !value {
			return call
		}
		g.pending = append(g.pending, call)
		return "rt.NilVal()"
	}
	switch name {
	case "spr", "camera":
		if call, ok := g.typedDrawCall(name, adjusted); ok {
			return void(call)
		}
		return void("c." + Builtins[name].GoName + "(" + argsUpTo(Builtins[name].MaxArgs) + ")")
	case "anim":
		return void("c." + animationMethod + "V(" + argsUpTo(Builtins[name].MaxArgs) + ")")
	case "cls", "pset", "line", "rect", "rectfill", "circ", "circfill", "sspr", "pspr", "clip", "color", "cursor":
		return void("c." + Builtins[name].GoName + "(" + argsUpTo(Builtins[name].MaxArgs) + ")")
	case "map":
		if len(names) == 0 {
			return void(fmt.Sprintf("c.MapAll(%d)", g.opts.MapDrawMask))
		}
		bg, _ := g.staticRuntimeInt(argExprs[0])
		slot := mapMaskSlot(g.opts.MapDrawMask, bg)
		if args, ok := g.typedDrawIntegers(adjusted, []int{0, 0, 0, 0, 0, 0, 0, 0}); ok {
			return void(fmt.Sprintf("c.MapBGAt(%d,%s)", slot, strings.Join(args, ",")))
		}
		return void(fmt.Sprintf("c.MapBGAtV(%d,%s)", slot, argsUpTo(8)))
	case "pvariant":
		if len(argExprs) == 0 {
			return void("c.PVariantID(0)")
		}
		if name, literal := unparenExpr(argExprs[0]).(*luaparser.StringExpr); literal {
			for i, v := range g.opts.PoseVariantNames {
				if v == name.Value {
					return void(fmt.Sprintf("c.PVariantID(%d)", i+1))
				}
			}
			g.setErr(fmt.Errorf("transpiler: %s: [PVARIANT_UNKNOWN] no pose color variant %q", argExprs[0].Pos(), name.Value))
			return "rt.NilVal()"
		}
		if !g.heapEnabled {
			return void("c.PVariantStaticV(" + arg(0) + ")")
		}
		return void("c.PVariantV(" + arg(0) + ")")
	case "pal":
		switch len(names) {
		case 0:
			return void("c.PalReset()")
		case 1:
			if n, ok := staticNumberLiteral(argExprs[0]); ok && n == 2 {
				g.setErr(fmt.Errorf("transpiler: pal(2) is unsupported while fillp is deferred"))
				return "rt.NilVal()"
			}
			return void("c.PalLayerReset(" + arg(0) + ".Int())")
		default:
			return void("c.PalV(" + argsUpTo(3) + ")")
		}
	case "palt":
		if len(names) == 0 {
			return void("c.PaltReset()")
		}
		return void("c.PaltV(" + argsUpTo(2) + ")")
	case "pget":
		return wrap("c.PgetV(" + argsUpTo(Builtins[name].MaxArgs) + ")")
	case "print":
		call := "gluPrintV(c," + arg(0) + "," + arg(1) + "," + arg(2) + "," + arg(3) + g.rootArgsFor(4, callPos) + ")"
		if !g.opts.DisableLiteralPrintSpecialization && len(argExprs) != 0 {
			if _, literal := unparenExpr(argExprs[0]).(*luaparser.StringExpr); literal {
				// A static string cannot enter PrintV's Tostr allocation path, so
				// no argument/root wrapper is required. This is also essential for
				// a heapless build, whose root arrays are compile-only sentinels.
				call = "c.PrintStaticV(" + arg(0) + "," + arg(1) + "," + arg(2) + "," + arg(3) + ")"
			} else if g.mode0HeaplessPrintNumber(argExprs[0]) {
				call = "c.PrintV(" + arg(0) + "," + arg(1) + "," + arg(2) + "," + arg(3) + ")"
			}
		}
		if value {
			return wrap("rt.IntVal(" + call + ")")
		}
		return call
	case "sget":
		return wrap("rt.IntVal(int(c.Sget(" + arg(0) + ".Int()," + arg(1) + ".Int())))")
	case "sset":
		return void("c.Sset(" + arg(0) + ".Int()," + arg(1) + ".Int()," + arg(2) + ".Int())")
	case "fget":
		if len(names) < 2 {
			return wrap("rt.IntVal(c.FgetByte(" + arg(0) + ".Int()))")
		}
		return wrap("rt.BoolVal(c.Fget(" + arg(0) + ".Int()," + arg(1) + ".Int()))")
	case "fset":
		if len(names) < 3 {
			return void("c.FsetByte(" + arg(0) + ".Int()," + arg(1) + ".Int())")
		}
		return void("c.Fset(" + arg(0) + ".Int()," + arg(1) + ".Int()," + arg(2) + ".Truthy())")
	case "mget":
		if view, first := g.borrowedMapCall(name, argExprs, len(names)); view != "" {
			return wrap("rt.IntVal(int(" + view + ".Get(" + arg(first) + ".Int()," + arg(first+1) + ".Int())))")
		}
		if len(names) == 2 {
			if !g.opts.DisableFixedMapLayerSpecialization && !g.opts.MapStreaming {
				slot := mapMaskSlot(g.opts.MapPlaneMask, 0)
				return wrap(fmt.Sprintf("rt.IntVal(int(c.MgetCompact(0,%d,%s.Int(),%s.Int())))", slot, arg(0), arg(1)))
			}
			return wrap("rt.IntVal(int(c.Mget(" + arg(0) + ".Int()," + arg(1) + ".Int())))")
		}
		if !g.opts.DisableFixedMapLayerSpecialization && !g.opts.MapStreaming {
			if bg, fixed := g.staticRuntimeInt(argExprs[0]); fixed {
				slot := mapMaskSlot(g.opts.MapPlaneMask, bg)
				return wrap(fmt.Sprintf("rt.IntVal(int(c.MgetCompact(%d,%d,%s.Int(),%s.Int())))", bg, slot, arg(1), arg(2)))
			}
		}
		if bg, literal := mode0StaticRuntimeInt(argExprs[0]); literal && bg == 0 {
			return wrap("rt.IntVal(int(c.Mget(" + arg(1) + ".Int()," + arg(2) + ".Int())))")
		}
		return wrap("rt.IntVal(int(c.MgetLayer(" + arg(0) + ".Int()," + arg(1) + ".Int()," + arg(2) + ".Int())))")
	case "mset":
		if view, first := g.borrowedMapCall(name, argExprs, len(names)); view != "" {
			return void(view + ".Set(" + arg(first) + ".Int()," + arg(first+1) + ".Int()," + arg(first+2) + ".Int())")
		}
		if len(names) == 3 {
			if !g.opts.DisableFixedMapLayerSpecialization && !g.opts.MapStreaming {
				slot := mapMaskSlot(g.opts.MapPlaneMask, 0)
				return void(fmt.Sprintf("c.MsetCompact(0,%d,%s.Int(),%s.Int(),%s.Int())", slot, arg(0), arg(1), arg(2)))
			}
			return void("c.Mset(" + arg(0) + ".Int()," + arg(1) + ".Int()," + arg(2) + ".Int())")
		}
		if !g.opts.DisableFixedMapLayerSpecialization && !g.opts.MapStreaming {
			if bg, fixed := g.staticRuntimeInt(argExprs[0]); fixed {
				slot := mapMaskSlot(g.opts.MapPlaneMask, bg)
				return void(fmt.Sprintf("c.MsetCompact(%d,%d,%s.Int(),%s.Int(),%s.Int())", bg, slot, arg(1), arg(2), arg(3)))
			}
		}
		if bg, literal := mode0StaticRuntimeInt(argExprs[0]); literal && bg == 0 {
			return void("c.Mset(" + arg(1) + ".Int()," + arg(2) + ".Int()," + arg(3) + ".Int())")
		}
		return void("c.MsetLayer(" + arg(0) + ".Int()," + arg(1) + ".Int()," + arg(2) + ".Int()," + arg(3) + ".Int())")
	case "btn":
		if len(names) > 1 {
			g.pending = append(g.pending, "_ = "+names[1])
		}
		return wrap("c.BtnV(" + arg(0) + ")")
	case "btnp":
		if len(names) > 1 {
			g.pending = append(g.pending, "_ = "+names[1])
		}
		return wrap("c.BtnpV(" + arg(0) + ")")
	case "flr", "ceil", "abs", "sgn":
		return wrap("(" + arg(0) + ")." + Builtins[name].GoName + "()")
	case "min", "max", "mid":
		return wrap("rt.NumVal(rt." + Builtins[name].GoName + ("(" + arg(0) + ".NumberOr(0), " + arg(1) + ".NumberOr(0)" + func() string {
			if name == "mid" {
				return ", " + arg(2) + ".NumberOr(0)"
			}
			return ""
		}() + "))"))
	case "sin", "cos", "sqrt":
		return wrap("rt.NumVal(" + arg(0) + ".NumberOr(0)." + capitalize(name) + "())")
	case "atan2":
		return wrap("rt.NumVal(rt.Atan2(" + arg(0) + ".NumberOr(0)," + arg(1) + ".NumberOr(0)))")
	case "rnd":
		return wrap("c.RndV(" + arg(0) + ")")
	case "srand":
		return void("c.SrandV(" + arg(0) + ")")
	case "band", "bor", "bxor", "shl", "shr", "lshr", "rotl", "rotr":
		return wrap("rt.NumVal(" + arg(0) + ".NumberOr(0)." + Builtins[name].GoName + "(" + arg(1) + ".NumberOr(0)))")
	case "bnot":
		return wrap("rt.NumVal(" + arg(0) + ".NumberOr(0).Bnot())")
	case "add":
		table, val, idx := arg(0), arg(1), arg(2)
		if value {
			v := g.next("add")
			g.pending = append(g.pending, v+" := "+val)
			g.pending = append(g.pending, "_ = gluTableAdd(c,"+table+", "+v+", "+idx+g.rootArgsFor(5, callPos)+")")
			return v
		}
		return "_ = gluTableAdd(c," + table + "," + val + "," + idx + g.rootArgsFor(5, callPos) + ")"
	case "del":
		return wrap("c.TableDel(" + arg(0) + "," + arg(1) + ")")
	case "deli":
		return wrap("c.TableDeli(" + arg(0) + "," + arg(1) + ")")
	case "count":
		return wrap("rt.IntVal(c.TableCount(" + arg(0) + "," + arg(1) + "))")
	case "all", "pairs":
		g.setErr(fmt.Errorf("transpiler: %s() is only valid as a generic-for iterator", name))
		return "rt.NilVal()"
	case "foreach":
		g.setErr(errors.New("transpiler: foreach is only valid as a standalone statement"))
		return "rt.NilVal()"
	case "sub":
		return wrap("gluSub(c," + arg(0) + "," + arg(1) + "," + arg(2) + g.rootArgsFor(3, callPos) + ")")
	case "tostr":
		return wrap("gluTostr(c," + arg(0) + "," + arg(1) + ".Truthy()" + g.rootArgsFor(1, callPos) + ")")
	case "tonum":
		return wrap("c.Tonum(" + arg(0) + ")")
	case "chr":
		return wrap("gluChr(c," + arg(0) + g.rootArgsFor(1, callPos) + ")")
	case "ord":
		return wrap("c.Ord(" + arg(0) + "," + arg(1) + ")")
	case "t":
		return wrap("c.TimeV()")
	case "time":
		return wrap("c.TimeV()")
	case "stat":
		return wrap("c.StatV(" + arg(0) + ")")
	case "sfx":
		return void("c.SFXV(" + argsUpTo(4) + ")")
	case "music":
		return void("c.MusicV(" + argsUpTo(2) + ")")
	case "sample":
		return void("c.SampleV(" + argsUpTo(3) + ")")
	case "gba_note":
		return void("c.GBANoteV(" + argsUpTo(4) + ")")
	case "gba_stop":
		return void("c.GBAStopV(" + arg(0) + ")")
	case "printh":
		for _, n := range names {
			g.pending = append(g.pending, "_ = "+n)
		}
		if value {
			return "rt.NilVal()"
		}
		return ""
	}
	return "rt.NilVal()"
}

func (g *generator) staticRuntimeInt(expr luaparser.Expr) (int, bool) {
	if value, literal := mode0StaticRuntimeInt(expr); literal {
		return value, true
	}
	variable, ok := unparenExpr(expr).(*luaparser.VarExpr)
	if !ok {
		return 0, false
	}
	if _, local := g.lookupLocal(variable.Name); local {
		return 0, false
	}
	constant := g.numericConstants[variable.Name]
	if constant == nil {
		return 0, false
	}
	return int(constant.value >> 16), true
}

func calleePath(e luaparser.Expr) (string, bool) {
	switch x := e.(type) {
	case *luaparser.VarExpr:
		return x.Name, true
	case *luaparser.FieldExpr:
		base, ok := calleePath(x.Base)
		if !ok {
			return "", false
		}
		return base + "." + x.Name, true
	default:
		return "", false
	}
}

// staticMemberPath is deliberately validation-only. It recognizes literal
// bracket members as aliases of dot members so writes such as
// actor["body"]["draw"] cannot mutate a statically lowered function behind
// the generator's back. callKey remains narrower; this does not introduce
// bracket-call syntax or dynamic dispatch into the public dialect.
func staticMemberPath(e luaparser.Expr) (string, bool) {
	switch x := e.(type) {
	case *luaparser.ParenExpr:
		return staticMemberPath(x.Expr)
	case *luaparser.VarExpr:
		return x.Name, true
	case *luaparser.FieldExpr:
		base, ok := staticMemberPath(x.Base)
		if !ok {
			return "", false
		}
		return base + "." + x.Name, true
	case *luaparser.IndexExpr:
		base, ok := staticMemberPath(x.Base)
		if !ok {
			return "", false
		}
		key, ok := unparenExpr(x.Key).(*luaparser.StringExpr)
		if !ok {
			return "", false
		}
		return base + "." + key.Value, true
	default:
		return "", false
	}
}

func callKey(call *luaparser.CallExpr) (string, bool) {
	if call == nil {
		return "", false
	}
	path, ok := calleePath(call.Callee)
	if !ok {
		return "", false
	}
	if call.Method != "" {
		return path + ":" + call.Method, true
	}
	return path, true
}

func (g *generator) calledFunction(call *luaparser.CallExpr) *functionInfo {
	key, ok := callKey(call)
	if !ok {
		return nil
	}
	// Builtins have precedence in emitCall. Keep every analysis and optimized
	// lowering path on that same resolution rule; otherwise a user declaration
	// named `abs`, for example, changes meaning when monomorphic numbers are
	// enabled even though the generic path still calls the builtin.
	if _, builtin := Builtins[key]; builtin {
		return nil
	}
	return g.funcs[key]
}

// callResultArity resolves the exact number of values contributed by a bare
// trailing call in a Lua expression list. Builtins and user functions share
// this path so zero-result builtins cannot accidentally manufacture a nil
// element or argument.
func (g *generator) callResultArity(call *luaparser.CallExpr) (int, bool) {
	key, ok := callKey(call)
	if !ok {
		return 0, false
	}
	if spec, builtin := Builtins[key]; builtin {
		return spec.Results, true
	}
	fn := g.funcs[key]
	if fn == nil {
		return 0, false
	}
	return fn.returnN, true
}

// operandExprs returns a method call's receiver (bound to the callee's
// implicit self parameter) followed by its ordinary arguments, in the
// exact left-to-right order Lua evaluates them.
func operandExprs(call *luaparser.CallExpr) []luaparser.Expr {
	exprs := make([]luaparser.Expr, 0, len(call.Args)+1)
	if call.Method != "" {
		exprs = append(exprs, call.Callee)
	}
	exprs = append(exprs, call.Args...)
	return exprs
}

func namesToArgs(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return ", " + strings.Join(names, ",")
}

// sequenceFunctionOperands preserves Lua's left-to-right argument evaluation
// while keeping proven numeric arguments unboxed. Value temporaries remain GC
// roots while later arguments execute; Number temporaries never enter the
// root stack.
func (g *generator) sequenceFunctionOperands(exprs []luaparser.Expr, fn *functionInfo) []string {
	values := g.sequenceAdjustedExprs(exprs)
	names := make([]string, len(values))
	for i, value := range values {
		want := monoValue
		if i < len(fn.paramKinds) {
			want = fn.paramKinds[i]
		}
		if want == monoNumber {
			if value.kind != monoNumber {
				if fn.coercesNumberParam(i) {
					names[i] = adjustedNumber(value)
				} else {
					g.setErr(fmt.Errorf("transpiler: internal numeric argument mismatch for %s", fn.key))
					names[i] = "0"
				}
			} else {
				names[i] = value.code
			}
		} else {
			names[i] = adjustedLuaValue(value)
		}
	}
	return names
}

// sequenceAdjustedExprs evaluates a Lua expression list into stable generated
// temporaries. A bare trailing call contributes its declared result count;
// parentheses keep that call in a single-value context. Value temporaries
// remain roots while later expressions execute, matching Lua's left-to-right
// evaluation rule.
func (g *generator) sequenceAdjustedExprs(exprs []luaparser.Expr) []adjustedValue {
	if len(exprs) == 0 {
		return nil
	}
	savedRoots := g.extraRoots
	g.extraRoots = append([]string(nil), savedRoots...)
	defer func() { g.extraRoots = savedRoots }()
	out := make([]adjustedValue, 0, g.exprListWidth(exprs))
	last := len(exprs) - 1
	for i, expr := range exprs {
		if i == last {
			if call, ok := expr.(*luaparser.CallExpr); ok {
				if arity, known := g.callResultArity(call); known && arity == 0 {
					if statement := g.emitCall(call, false); statement != "" {
						g.pending = append(g.pending, statement)
					}
					continue
				}
				if fn := g.calledFunction(call); fn != nil && fn.returnN > 1 {
					results := make([]string, fn.returnN)
					for j := range results {
						results[j] = g.next("arg")
					}
					rhs := g.emitMultiCall(call, fn)
					g.pending = append(g.pending, strings.Join(results, ", ")+" := "+rhs)
					for j, result := range results {
						kind := fn.returnKind(j)
						out = append(out, adjustedValue{code: result, kind: kind})
						if kind == monoValue {
							g.extraRoots = append(g.extraRoots, result)
						}
					}
					continue
				}
			}
		}
		kind := monoValue
		code := ""
		if g.exprNumber(expr) {
			kind = monoNumber
			code = g.emitNumberExpr(expr)
		} else {
			code = g.emitExpr(expr)
		}
		tmp := g.next("arg")
		g.pending = append(g.pending, tmp+" := "+code)
		out = append(out, adjustedValue{code: tmp, kind: kind})
		if kind == monoValue {
			g.extraRoots = append(g.extraRoots, tmp)
		}
	}
	return out
}

func (g *generator) emitUserCall(call *luaparser.CallExpr, fn *functionInfo) string {
	names := g.sequenceFunctionOperands(operandExprs(call), fn)
	args := make([]string, len(fn.params))
	for i := range fn.params {
		if i < len(names) {
			args[i] = names[i]
		} else if fn.paramKinds[i] == monoNumber {
			args[i] = "0"
		} else {
			args[i] = "rt.NilVal()"
		}
	}
	for i := len(fn.params); i < len(names); i++ {
		g.pending = append(g.pending, "_ = "+names[i])
	}
	rootArgs := g.userCallRootArgs(fn, call.Pos())
	if fn.noAlloc {
		return "g." + fn.goName + "(c" + namesToArgs(args) + ")"
	}
	return "g.__glu_call_" + fn.goName + "(c" + namesToArgs(args) + rootArgs + ")"
}

func (g *generator) emitMultiCall(call *luaparser.CallExpr, fn *functionInfo) string {
	names := g.sequenceFunctionOperands(operandExprs(call), fn)
	args := make([]string, len(fn.params))
	for i := range fn.params {
		if i < len(names) {
			args[i] = names[i]
		} else if fn.paramKinds[i] == monoNumber {
			args[i] = "0"
		} else {
			args[i] = "rt.NilVal()"
		}
	}
	for i := len(fn.params); i < len(names); i++ {
		g.pending = append(g.pending, "_ = "+names[i])
	}
	rootArgs := g.userCallRootArgs(fn, call.Pos())
	if fn.noAlloc {
		return "g." + fn.goName + "(c" + namesToArgs(args) + ")"
	}
	return "g.__glu_multi_" + fn.goName + "(c" + namesToArgs(args) + rootArgs + ")"
}

func (g *generator) emitFirstCall(call *luaparser.CallExpr, fn *functionInfo) string {
	names := g.sequenceFunctionOperands(operandExprs(call), fn)
	args := make([]string, len(fn.params))
	for i := range fn.params {
		if i < len(names) {
			args[i] = names[i]
		} else if fn.paramKinds[i] == monoNumber {
			args[i] = "0"
		} else {
			args[i] = "rt.NilVal()"
		}
	}
	for i := len(fn.params); i < len(names); i++ {
		g.pending = append(g.pending, "_ = "+names[i])
	}
	return "g.__glu_first_" + fn.goName + "(c" + namesToArgs(args) + g.userCallRootArgs(fn, call.Pos()) + ")"
}

// rootVars lists every Go variable currently reachable in the lexical scope
// stack (including any outer bindings shadowed by an inner `local` of the
// same name; a shadowed variable's Go slot can still hold a live heap value
// that must survive a GC triggered by an allocating call), plus any
// caller-supplied extraRoots.
func (g *generator) rootVars() []string {
	var p []string
	seen := make(map[string]bool)
	appendRoot := func(root string) {
		if root == "" || seen[root] {
			return
		}
		seen[root] = true
		p = append(p, root)
	}
	for _, scope := range g.scopes {
		for _, e := range scope.entries {
			if e.kind == monoValue {
				appendRoot(e.goVar)
			}
		}
	}
	for _, root := range g.extraRoots {
		appendRoot(root)
	}
	return p
}

// userCallRootArgs records the Context root-stack occupancy that remains in
// place while a statically known callee executes. Allocating callees receive
// every live caller Value through their wrapper; proven no-allocation calls
// are direct and retain only roots already owned by surrounding loops.
func (g *generator) userCallRootArgs(fn *functionInfo, pos luaparser.Position) string {
	p := g.rootVars()
	held := g.activeRootSlots
	if !fn.noAlloc {
		held += len(p)
	}
	g.noteRootPeak(held)
	g.rootCalls = append(g.rootCalls, rootCallEdge{caller: g.current, callee: fn, held: held, pos: pos})
	if held > runtimeapi.MaxLocalRoots {
		g.rootBudgetError(held, pos, "user-call GC root budget")
	}
	if fn.noAlloc || len(p) == 0 {
		return ""
	}
	return "," + strings.Join(p, ",")
}

func (g *generator) noteRootPeak(total int) {
	if g.current == nil {
		if total > g.topRootPeak {
			g.topRootPeak = total
		}
		return
	}
	if total > g.current.rootPeak {
		g.current.rootPeak = total
	}
}

func (g *generator) rootBudgetError(total int, pos luaparser.Position, label string) {
	where := ""
	if pos.Line != 0 {
		where = " at " + pos.String()
	}
	g.setErr(fmt.Errorf("transpiler: %s requires %d slots, runtime limit is %d%s", label, total, runtimeapi.MaxLocalRoots, where))
}

// requireActiveRootSlots validates roots which remain on Context's stack
// across generated control flow, rather than roots pushed only for one helper
// call. Without this check deeply nested loops could make gluPushRoots fail and
// silently skip a loop even when its body performs no allocating operation.
func (g *generator) requireActiveRootSlots(additional int, pos luaparser.Position) {
	total := g.activeRootSlots + additional
	g.noteRootPeak(total)
	if total > runtimeapi.MaxLocalRoots {
		g.rootBudgetError(total, pos, "active GC root budget")
	}
}

// rootArgsFor also proves that the roots already held by surrounding loops,
// the helper/runtime operation's peak operand roots, and the currently live
// lexical/temporary Values fit in the runtime's fixed local-root stack. This
// turns an otherwise silent nil/HeapOOM result into a deterministic,
// source-positioned compile error.
func (g *generator) rootArgsFor(helperRoots int, pos luaparser.Position) string {
	p := g.rootVars()
	total := g.activeRootSlots + helperRoots + len(p)
	g.noteRootPeak(total)
	if total > runtimeapi.MaxLocalRoots {
		g.rootBudgetError(total, pos, "local GC root budget")
	}
	if len(p) == 0 {
		return ""
	}
	return "," + strings.Join(p, ",")
}

// validateTransitiveRootBudget solves the maximum additional Context-root
// demand of each statically known function. A call contributes roots already
// held by its caller plus the callee's peak. Any positive recursive cycle is
// unbounded because Mtile has no static recursion-depth limit, so accepting it
// would merely postpone a deterministic root-stack failure to runtime.
func (g *generator) validateTransitiveRootBudget() error {
	for pass := 0; ; pass++ {
		changed := false
		var changedEdge rootCallEdge
		var changedTotal int
		for _, edge := range g.rootCalls {
			if edge.callee == nil {
				continue
			}
			total := edge.held + edge.callee.rootPeak
			if total > runtimeapi.MaxLocalRoots {
				where := ""
				if edge.pos.Line != 0 {
					where = " at " + edge.pos.String()
				}
				return fmt.Errorf("transpiler: transitive GC root budget calling %q requires %d slots, runtime limit is %d%s", edge.callee.key, total, runtimeapi.MaxLocalRoots, where)
			}
			if edge.caller == nil {
				if total > g.topRootPeak {
					g.topRootPeak = total
					changed, changedEdge, changedTotal = true, edge, total
				}
			} else if total > edge.caller.rootPeak {
				edge.caller.rootPeak = total
				changed, changedEdge, changedTotal = true, edge, total
			}
		}
		if !changed {
			return nil
		}
		if pass >= len(g.funcList) {
			where := ""
			if changedEdge.pos.Line != 0 {
				where = " at " + changedEdge.pos.String()
			}
			return fmt.Errorf("transpiler: recursive call cycle can grow the GC root stack without bound while calling %q (reached %d slots)%s", changedEdge.callee.key, changedTotal, where)
		}
	}
}
