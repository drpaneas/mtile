package transpiler

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/drpaneas/mtile/internal/luaparser"
)

// Borrow only after all three loop bounds have evaluated: those expressions
// may allocate and move the Lua heap. The complete body (including nested
// bounds) must be covered by the strict non-allocating allowlist below.
func (g *generator) borrowLoopViews(b *sourceBuilder, body *luaparser.Block, indent int) func() {
	names, layers, writtenLayers, ok := g.loopBorrowedViews(body)
	if g.opts.DisableNumberArrayViews {
		names = nil
	}
	if g.opts.DisableMapLayerViews || g.opts.MapStreaming {
		layers = nil
	}
	if !ok || len(names) == 0 && len(layers) == 0 {
		return func() {}
	}
	previous := g.borrowedNumberArrays
	previousLayers := g.borrowedMapLayers
	g.borrowedNumberArrays = maps.Clone(previous)
	if g.borrowedNumberArrays == nil {
		g.borrowedNumberArrays = make(map[string]string)
	}
	for _, name := range names {
		if g.borrowedNumberArrays[name] != "" {
			continue
		}
		view := g.next("arrayView")
		b.line(strings.Repeat("\t", indent) + view + " := c.BorrowNumberArray(" + g.lookup(name) + ")")
		g.borrowedNumberArrays[name] = view
	}
	g.borrowedMapLayers = maps.Clone(previousLayers)
	if g.borrowedMapLayers == nil {
		g.borrowedMapLayers = make(map[int]string)
	}
	for _, layer := range layers {
		if g.borrowedMapLayers[layer] != "" {
			continue
		}
		view := g.next("mapView")
		borrow := "BorrowMapLayerRead"
		if writtenLayers[layer] {
			borrow = "BorrowMapLayer"
		}
		b.line(strings.Repeat("\t", indent) + view + " := c." + borrow + "(" + fmt.Sprint(layer) + ")")
		g.borrowedMapLayers[layer] = view
	}
	return func() {
		g.borrowedNumberArrays = previous
		g.borrowedMapLayers = previousLayers
	}
}

// Numeric-array discovery proves fixed identity/length and excludes shadowing.
// The region excludes Lua heap mutation except numeric elements, allocation,
// and unknown calls. Its map/flag accessors can edit cart cells but cannot
// replace cart metadata/storage or draw/reconfigure the renderer. Both sorts
// of borrowed storage consequently remain valid until loop exit.
func (g *generator) loopBorrowedViews(body *luaparser.Block) ([]string, []int, map[int]bool, bool) {
	arrays := make(map[string]bool)
	layers := make(map[int]bool)
	writtenLayers := make(map[int]bool)
	var exprSafe func(luaparser.Expr) bool
	exprSafe = func(expr luaparser.Expr) bool {
		switch x := expr.(type) {
		case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.NumberExpr,
			*luaparser.StringExpr, *luaparser.VarExpr:
			return true
		case *luaparser.ParenExpr:
			return exprSafe(x.Expr)
		case *luaparser.UnaryExpr:
			return exprSafe(x.Expr)
		case *luaparser.BinaryExpr:
			return x.Op != luaparser.OpConcat && exprSafe(x.Left) && exprSafe(x.Right)
		case *luaparser.FieldExpr:
			_, _, ok := g.numericRecordField(x)
			return ok
		case *luaparser.IndexExpr:
			name, _, ok := g.numericArrayIndex(x)
			if !ok || !exprSafe(x.Key) {
				return false
			}
			arrays[name] = true
			return true
		case *luaparser.CallExpr:
			if g.calledFunction(x) != nil {
				return false
			}
			name, ok := callKey(x)
			if !ok || !numberArrayViewBuiltin(name) {
				return false
			}
			for _, argument := range x.Args {
				if !exprSafe(argument) {
					return false
				}
			}
			if layer, _, fixed := literalMapCallLayer(name, x.Args); fixed {
				layers[layer] = true
				if name == "mset" {
					writtenLayers[layer] = true
				}
			}
			return true
		}
		return false
	}
	exprsSafe := func(exprs []luaparser.Expr) bool {
		for _, expr := range exprs {
			if !exprSafe(expr) {
				return false
			}
		}
		return true
	}
	var blockSafe func(*luaparser.Block) bool
	blockSafe = func(block *luaparser.Block) bool {
		if block == nil {
			return true
		}
		for _, stmt := range block.Body {
			switch s := stmt.(type) {
			case *luaparser.AssignStmt:
				for _, target := range s.Targets {
					if !exprSafe(target) {
						return false
					}
				}
				if !exprsSafe(s.Values) {
					return false
				}
			case *luaparser.LocalAssignStmt:
				if !exprsSafe(s.Values) {
					return false
				}
			case *luaparser.CallStmt:
				if !exprSafe(s.Call) {
					return false
				}
			case *luaparser.IfStmt:
				if !exprSafe(s.Condition) || !blockSafe(s.Then) || !blockSafe(s.Else) {
					return false
				}
				for _, branch := range s.ElseIf {
					if !exprSafe(branch.Condition) || !blockSafe(branch.Body) {
						return false
					}
				}
			case *luaparser.DoStmt:
				if !blockSafe(s.Body) {
					return false
				}
			case *luaparser.NumericForStmt:
				if !exprSafe(s.Init) || !exprSafe(s.Limit) || !exprSafe(s.Step) || !blockSafe(s.Body) {
					return false
				}
			case *luaparser.ReturnStmt:
				if !exprsSafe(s.Values) {
					return false
				}
			case *luaparser.BreakStmt:
			default:
				return false
			}
		}
		return true
	}
	if !blockSafe(body) {
		return nil, nil, nil, false
	}
	names := slices.Collect(maps.Keys(arrays))
	slices.Sort(names)
	mapLayers := slices.Collect(maps.Keys(layers))
	slices.Sort(mapLayers)
	return names, mapLayers, writtenLayers, true
}

func numberArrayViewBuiltin(name string) bool {
	switch name {
	case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd", "srand",
		"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr", "mget", "mset", "fget", "fset":
		return true
	}
	return false
}
