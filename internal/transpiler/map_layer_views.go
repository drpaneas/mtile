package transpiler

import "github.com/drpaneas/mtile/internal/luaparser"

// Only syntax-level numeric literals are accepted here. A local declaration
// inside a not-yet-emitted loop can shadow a chunk constant of the same name.
// Literal conversion still matches the runtime's fixed-point quantization.
func literalMapCallLayer(name string, args []luaparser.Expr) (layer, first int, ok bool) {
	implicit := 0
	switch name {
	case "mget":
		implicit = 2
	case "mset":
		implicit = 3
	default:
		return 0, 0, false
	}
	if len(args) == implicit {
		return 0, 0, true
	}
	if len(args) != implicit+1 {
		return 0, 0, false
	}
	layer, fixed := mode0StaticRuntimeInt(args[0])
	return layer, 1, fixed && layer >= 0 && layer < 4
}

func (g *generator) borrowedMapCall(name string, args []luaparser.Expr, width int) (string, int) {
	if len(args) != width {
		return "", 0
	}
	layer, first, fixed := literalMapCallLayer(name, args)
	if !fixed {
		return "", 0
	}
	return g.borrowedMapLayers[layer], first
}
