package transpiler

// RendererCompatibility is the build-time compatibility contract for one
// public Lua builtin. It describes what the compile-time renderer selector may
// emit. Keep this table synchronized with
// docs/render-modes.md whenever an API is added or its exact semantics change.
type RendererCompatibility uint8

const (
	RendererUnknown RendererCompatibility = iota
	// RendererIndependent does not read or mutate graphics state and therefore
	// cannot influence a Mode 0 versus Mode 4 decision.
	RendererIndependent
	// RendererBoth has exact semantics that can be implemented in either backend
	// without restricting the Lua call shape.
	RendererBoth
	// RendererMode0Conditional is representable in Mode 0 only when the static
	// call/use restrictions documented in docs/render-modes.md are satisfied.
	RendererMode0Conditional
	// RendererMode4Only requires exact software-framebuffer raster or readback
	// semantics. Any reachable call forces an AUTO build to Mode 4.
	RendererMode4Only
)

// BuiltinRendererCompatibility is deliberately exhaustive. Returning
// RendererUnknown for a newly added builtin makes the coverage test fail, so a
// public API cannot silently enter AUTO mode without an explicit decision.
func BuiltinRendererCompatibility(name string) RendererCompatibility {
	switch name {
	case "pset", "pget", "line", "rect", "rectfill", "circ", "circfill":
		return RendererMode4Only
	case "pvariant":
		// Exact independent color variants require proven resident 4bpp
		// objects and compatible hardware palette-bank allocation.
		return RendererMode0Conditional
	case "pspr", "anim":
		// Literal pspr names lower to the identical numeric call before any
		// renderer proof. Unknown names fail in both modes; names never relax
		// the pose working-set or resource restrictions.
		// Both backends use build-derived bounds for automatic flip anchoring
		// (a pose's bounds or a whole clip's union). Mode 0 still requires the
		// documented bounded draw sites, OAM budget, and tile streaming proof.
		return RendererMode0Conditional
	case "cls", "spr", "sspr", "map", "clip", "pal", "palt":
		return RendererMode0Conditional
	case "print":
		// Arcade ASCII is the default; optional language fonts use the same metrics.
		// Selected-font 6x10 ASCII has exact pixel parity through 8x8/8x16 OBJs.
		// Only proven blank rows 8/9 are omitted. The 32-tile bank packs one/two
		// tiles per pair, and scanline proof uses the actual 8/16-row object span.
		// UTF-8 text and control sequences require Mode 4 (M0_PRINT_SHAPE).
		return RendererMode0Conditional
	case "cursor":
		// Pixel coordinates and six-pixel tabs are shared; implicit print lines
		// advance ten pixels in both renderers. No renderer-specific conversion.
		return RendererBoth
	case "camera", "color", "sget", "sset", "fget", "fset":
		return RendererBoth
	case "mset":
		// Linked maps size Mode 0 objects from authored cells. Writes must
		// prove an unlinked tile ID, with no expanded argument list.
		return RendererMode0Conditional
	case "mget":
		// Both the legacy BG0 arities and the layer-first arities access the
		// shared logical map exactly, independently of linked objects.
		return RendererBoth
	case "pause", "save_get", "save_set", "save_read", "save_write", "save_pending", "btn", "btnp",
		"flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd", "srand",
		"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr",
		"add", "del", "deli", "count", "all", "pairs", "foreach",
		"sub", "tostr", "tonum", "chr", "ord",
		"t", "time", "stat",
		"sfx", "music", "sample", "pcm_music", "gba_note", "gba_stop", "printh":
		return RendererIndependent
	default:
		return RendererUnknown
	}
}
