package transpiler

import (
	"github.com/drpaneas/mtile/internal/luaparser"
	rt "github.com/drpaneas/mtile/runtime"
)

// The fixed system OBJ must not recolor any game object while it is visible.
// Background colors remain unrestricted. Fail closed when sprite mutations
// cannot prove that the five borrowed physical palette indices stay unused.
func validateMode0PausePalette(chunk *luaparser.Chunk, ops []mode0DrawOp, opts Options) error {
	var sheet, poses *luaparser.CallExpr
	reserved := func(c byte) bool {
		return int(c) >= rt.PauseOBJPaletteStart && int(c) < rt.PauseOBJPaletteStart+rt.PauseOBJPaletteColors
	}
	fail := func(call *luaparser.CallExpr) error {
		return mode0CallError(call, "M0_PAUSE_PALETTE", "the fixed pause overlay reserves OBJ palette indices 241-245; use other sprite/text indices or Mode 4 (background colors remain unrestricted)")
	}
	for _, op := range ops {
		switch op.name {
		case "spr", "sspr":
			if sheet == nil {
				sheet = op.call
			}
		case "pspr":
			if poses == nil {
				poses = op.call
			}
		case "print":
			for _, key := range mode0TextAtlasKeysForOps([]mode0DrawOp{op}) {
				if reserved(byte(key)) {
					return fail(op.call)
				}
			}
		}
	}
	if sheet != nil || poses != nil && opts.ObjSheetW == 0 {
		call := sheet
		if call == nil {
			call = poses
		}
		for _, c := range opts.Sheet {
			if reserved(c) {
				return fail(call)
			}
		}
	}
	if poses != nil {
		plan := planMode0ResidentPoses(chunk, ops, opts)
		// Packed 4bpp variants have at most fifteen private game palette banks.
		if plan == nil || len(plan.palettes) == 0 {
			for _, c := range opts.ObjSheet {
				if reserved(c) {
					return fail(poses)
				}
			}
		}
	}
	if sheet != nil || poses != nil {
		var err error
		mode0EachStmt(chunk, func(stmt luaparser.Stmt) {
			collectStmtExprs(stmt, func(expr luaparser.Expr) {
				call, ok := expr.(*luaparser.CallExpr)
				if !ok || err != nil {
					return
				}
				name, _ := callKey(call)
				if name != "sset" || len(call.Args) < 3 {
					return
				}
				n, known := mode0StaticRuntimeInt(call.Args[2])
				if !known || n >= rt.PauseOBJPaletteStart && n < rt.PauseOBJPaletteStart+rt.PauseOBJPaletteColors {
					err = fail(call)
				}
			})
		})
		return err
	}
	return nil
}
