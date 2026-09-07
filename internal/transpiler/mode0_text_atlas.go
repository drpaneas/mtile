package transpiler

import (
	"math"
	"sort"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

const (
	// mode0PrintNumberCharset is formatNumber's decimal alphabet. Minus, dot,
	// and digits cover every Tostr(number, false) layout the runtime emits.
	mode0PrintNumberCharset = "-.0123456789"
	// mode0PrintNumberOAMEntries is the conservative occurrence budget for one
	// print(number). formatNumber's widest decimal form is 11 characters
	// (-65535.9999); 12 leaves a one-glyph spare for the same 16.16 layout.
	mode0PrintNumberOAMEntries = 12
)

type mode0TextAtlas struct {
	keys  []uint16
	tiles []uint32
}

type mode0PrintTextKind uint8

const (
	mode0PrintTextInvalid mode0PrintTextKind = iota
	mode0PrintTextEmpty
	mode0PrintTextString
	mode0PrintTextNumber
)

func classifyMode0PrintText(expr luaparser.Expr, eval *mode0NumEval, env mode0NumEnv, names map[string]bool) mode0PrintTextKind {
	switch x := unparenExpr(expr).(type) {
	case *luaparser.StringExpr:
		for index := 0; index < len(x.Value); index++ {
			if x.Value[index] < 32 || x.Value[index] > 126 {
				return mode0PrintTextInvalid
			}
		}
		if len(x.Value) == 0 {
			return mode0PrintTextEmpty
		}
		return mode0PrintTextString
	default:
		if mode0PrintAlwaysNumber(expr, eval, env, names) {
			return mode0PrintTextNumber
		}
		return mode0PrintTextInvalid
	}
}

func (g *generator) mode0HeaplessPrintNumber(expr luaparser.Expr) bool {
	if g == nil || g.opts.Renderer != RendererMode0 {
		return false
	}
	if g.exprNumber(expr) {
		return true
	}
	return mode0PrintAlwaysNumber(expr, nil, nil, mode0NumericNames(g.chunk))
}

func mode0PrintAlwaysNumber(expr luaparser.Expr, eval *mode0NumEval, env mode0NumEnv, names map[string]bool) bool {
	if expr == nil {
		return false
	}
	if eval != nil {
		if _, ok := eval.nums(expr, env); ok {
			return true
		}
		if _, _, ok := eval.interval(expr, env); ok {
			return true
		}
	}
	switch x := unparenExpr(expr).(type) {
	case *luaparser.NumberExpr:
		return true
	case *luaparser.VarExpr:
		if env != nil && len(env[x.Name]) > 0 {
			return true
		}
		return names[x.Name]
	case *luaparser.UnaryExpr:
		if x.Op == luaparser.OpNot {
			return false
		}
		if x.Op == luaparser.OpLength {
			return true
		}
		return mode0PrintAlwaysNumber(x.Expr, eval, env, names)
	case *luaparser.BinaryExpr:
		if !mode0PrintArithmeticOp(x.Op) {
			return false
		}
		return mode0PrintAlwaysNumber(x.Left, eval, env, names) && mode0PrintAlwaysNumber(x.Right, eval, env, names)
	case *luaparser.CallExpr:
		name, ok := callKey(x)
		if !ok {
			return false
		}
		switch name {
		case "tostr", "chr", "sub":
			return false
		case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd",
			"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr", "t", "time", "stat", "tonum", "mget", "ord":
			for _, arg := range x.Args {
				if !mode0PrintAlwaysNumber(arg, eval, env, names) {
					return false
				}
			}
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func mode0NumericNames(chunk *luaparser.Chunk) map[string]bool {
	assigns := mode0VarAssigns(chunk)
	numeric := map[string]bool{}
	for {
		changed := false
		for name, exprs := range assigns {
			if numeric[name] || len(exprs) == 0 {
				continue
			}
			assumed := cloneMode0PrintNames(numeric)
			assumed[name] = true
			hasSeed := false
			ok := true
			for _, expr := range exprs {
				if mode0PrintAlwaysNumber(expr, nil, nil, numeric) {
					hasSeed = true
				}
				if !mode0PrintAlwaysNumber(expr, nil, nil, assumed) {
					ok = false
					break
				}
			}
			if ok && hasSeed {
				numeric[name] = true
				changed = true
			}
		}
		if !changed {
			return numeric
		}
	}
}

func mode0VarAssigns(chunk *luaparser.Chunk) map[string][]luaparser.Expr {
	assigns := map[string][]luaparser.Expr{}
	mode0EachStmt(chunk, func(stmt luaparser.Stmt) {
		assign, ok := stmt.(*luaparser.AssignStmt)
		if !ok {
			return
		}
		for i, target := range assign.Targets {
			variable, ok := target.(*luaparser.VarExpr)
			if !ok {
				continue
			}
			var value luaparser.Expr
			if i < len(assign.Values) {
				value = assign.Values[i]
			}
			assigns[variable.Name] = append(assigns[variable.Name], value)
		}
	})
	return assigns
}

func mode0EachStmt(chunk *luaparser.Chunk, visit func(luaparser.Stmt)) {
	if chunk == nil {
		return
	}
	var walk func(*luaparser.Block)
	walk = func(block *luaparser.Block) {
		if block == nil {
			return
		}
		for _, stmt := range block.Body {
			visit(stmt)
			switch s := stmt.(type) {
			case *luaparser.FunctionDeclStmt:
				walk(s.Body)
			case *luaparser.DoStmt:
				walk(s.Body)
			case *luaparser.IfStmt:
				walk(s.Then)
				for _, branch := range s.ElseIf {
					walk(branch.Body)
				}
				walk(s.Else)
			case *luaparser.NumericForStmt:
				walk(s.Body)
			case *luaparser.GenericForStmt:
				walk(s.Body)
			case *luaparser.WhileStmt:
				walk(s.Body)
			case *luaparser.RepeatStmt:
				walk(s.Body)
			}
		}
	}
	walk(&luaparser.Block{Body: chunk.Body})
}

func cloneMode0PrintNames(names map[string]bool) map[string]bool {
	out := make(map[string]bool, len(names))
	for name, ok := range names {
		if ok {
			out[name] = true
		}
	}
	return out
}

func mode0PrintArithmeticOp(op luaparser.Operator) bool {
	switch op {
	case luaparser.OpBitOr, luaparser.OpBitXor, luaparser.OpBitAnd,
		luaparser.OpShiftL, luaparser.OpShiftR, luaparser.OpShiftRLogical,
		luaparser.OpRotateL, luaparser.OpRotateR,
		luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
		luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo, luaparser.OpPower:
		return true
	default:
		return false
	}
}

func mode0PrintOAMEntries(call *luaparser.CallExpr, kind mode0PrintTextKind) (int, bool) {
	if call == nil || len(call.Args) != 4 {
		return 0, false
	}
	switch kind {
	case mode0PrintTextEmpty:
		return 0, true
	case mode0PrintTextString:
		text, _ := unparenExpr(call.Args[0]).(*luaparser.StringExpr)
		if text == nil {
			return 0, false
		}
		return len(text.Value), true
	case mode0PrintTextNumber:
		return mode0PrintNumberOAMEntries, true
	default:
		return 0, false
	}
}

func internMode0PrintCall(call *luaparser.CallExpr, kind mode0PrintTextKind, seen map[uint16]bool) {
	if call == nil || len(call.Args) != 4 || seen == nil {
		return
	}
	colorValue, literal := staticNumberLiteral(call.Args[3])
	if !literal || colorValue != math.Trunc(colorValue) || colorValue < 1 || colorValue > 255 {
		return
	}
	color := byte(colorValue)
	intern := func(ch byte) {
		if ch < 32 || ch > 126 {
			return
		}
		seen[runtimeapi.Mode0TextGlyphKey(ch, color)] = true
	}
	switch kind {
	case mode0PrintTextString:
		text, _ := unparenExpr(call.Args[0]).(*luaparser.StringExpr)
		if text == nil {
			return
		}
		for index := 0; index < len(text.Value); index++ {
			intern(text.Value[index])
		}
	case mode0PrintTextNumber:
		for index := 0; index < len(mode0PrintNumberCharset); index++ {
			intern(mode0PrintNumberCharset[index])
		}
	}
}

func sortedMode0TextKeys(seen map[uint16]bool) []uint16 {
	keys := make([]uint16, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func atlasFromMode0TextKeys(keys []uint16) mode0TextAtlas {
	atlas := mode0TextAtlas{keys: keys, tiles: make([]uint32, 0, len(keys)*16)}
	for _, key := range keys {
		tile := runtimeapi.BuildMode0GlyphTile(byte(key>>8), byte(key))
		for offset := 0; offset < len(tile); offset += 4 {
			atlas.tiles = append(atlas.tiles,
				uint32(tile[offset])|uint32(tile[offset+1])<<8|uint32(tile[offset+2])<<16|uint32(tile[offset+3])<<24)
		}
	}
	return atlas
}

// mode0TextAtlasKeysForDraw returns the unique glyph/color tiles reachable from
// direct _draw print statements. Renderer selection also walks helpers; generate
// uses mode0TextAtlasKeysForOps so helper print(number) intern the digit bank.
func mode0TextAtlasKeysForDraw(draw *luaparser.FunctionDeclStmt) []uint16 {
	if draw == nil {
		return nil
	}
	seen := make(map[uint16]bool)
	for _, stmt := range draw.Body.Body {
		callStmt, ok := stmt.(*luaparser.CallStmt)
		if !ok {
			continue
		}
		name, ok := callKey(callStmt.Call)
		if !ok || name != "print" || len(callStmt.Call.Args) == 0 {
			continue
		}
		internMode0PrintCall(callStmt.Call, classifyMode0PrintText(callStmt.Call.Args[0], nil, nil, nil), seen)
	}
	return sortedMode0TextKeys(seen)
}

func mode0TextAtlasKeysForOps(ops []mode0DrawOp) []uint16 {
	seen := make(map[uint16]bool)
	for _, op := range ops {
		if op.name != "print" {
			continue
		}
		internMode0PrintCall(op.call, op.printKind, seen)
	}
	return sortedMode0TextKeys(seen)
}

func buildMode0TextAtlas(draw *luaparser.FunctionDeclStmt) mode0TextAtlas {
	return atlasFromMode0TextKeys(mode0TextAtlasKeysForDraw(draw))
}

func buildMode0TextAtlasForSource(chunk *luaparser.Chunk, draw *luaparser.FunctionDeclStmt, opts Options) mode0TextAtlas {
	if chunk != nil && draw != nil {
		cols, rows := opts.SheetW/8, opts.SheetH/8
		if ops, _, err := mode0WalkDraw(chunk, draw, newMode0SpriteIDAnalysis(chunk), cols, rows); err == nil {
			return atlasFromMode0TextKeys(mode0TextAtlasKeysForOps(ops))
		}
	}
	return buildMode0TextAtlas(draw)
}
