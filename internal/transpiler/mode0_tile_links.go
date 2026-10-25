package transpiler

import (
	"fmt"
	"slices"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

// Mode 0 draws linked objects through the ordinary pose machinery. Each map()
// call becomes one synthetic pose call site, repeated once for every linked
// cell that can be visible at the same time. The synthetic call has no
// arguments, so the object budget treats positions as unknown (coincident
// scanlines) and flips as absent, exactly as the runtime draws them.

// mode0TileLinkOp models the linked objects one map() call can draw.
func mode0TileLinkOp(mapCall *luaparser.CallExpr, copies int, opts Options) (mode0DrawOp, bool) {
	visible := mode0TileLinkCopies(mapCall, opts)
	if visible == 0 {
		return mode0DrawOp{}, false
	}
	var inRange []int
	unique, parts := 0, 0
	for _, id := range expandedPoseIDs(opts, mode0TileLinkPoseIDs(opts)) {
		u, p, ok := mode0PoseUniqueAndParts(opts, id)
		if !ok {
			continue
		}
		inRange = append(inRange, id)
		unique, parts = max(unique, u), max(parts, p)
	}
	synthetic := &luaparser.CallExpr{
		NodeBase: luaparser.NodeBase{At: mapCall.Pos()},
		Callee:   &luaparser.VarExpr{NodeBase: luaparser.NodeBase{At: mapCall.Pos()}, Name: "pspr"},
	}
	return mode0DrawOp{
		name: "pspr", call: synthetic, copies: copies * visible,
		psprUnique: unique, psprParts: parts, psprIDs: inRange, tileLinkMap: mapCall,
	}, true
}

// mode0TileLinkPoseIDs lists every pose a link can draw: linked poses and all
// frames of linked clips.
func mode0TileLinkPoseIDs(opts Options) []int {
	var ids []int
	for _, link := range opts.TileLinks {
		if !link.Clip {
			ids = append(ids, int(link.ID))
			continue
		}
		if int(link.ID) >= len(opts.AnimationClips) {
			continue
		}
		clip := opts.AnimationClips[link.ID]
		if uint64(clip.Start)+uint64(clip.Count) > uint64(len(opts.AnimationFrames)) {
			continue
		}
		for _, frame := range opts.AnimationFrames[clip.Start : clip.Start+clip.Count] {
			ids = append(ids, int(frame))
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// mode0TileLinkCopies bounds the linked cells one map() call can draw in a
// frame: the most linked cells in any window the call can make visible. The
// window covers the screen widened by the objects' reach, and is narrowed
// to literal cell-count arguments when present.
func mode0TileLinkCopies(call *luaparser.CallExpr, opts Options) int {
	table := tileLinkTable(opts.TileLinks, opts.SheetW/8*(opts.SheetH/8))
	if table == nil {
		return 0
	}
	reach := tileLinkReach(opts)
	windowW := (runtimeapi.ScreenWidth+int(reach.Left)+int(reach.Right)+7)/8 + 1
	windowH := (runtimeapi.ScreenHeight+int(reach.Top)+int(reach.Bottom)+7)/8 + 1
	layers := []int{}
	mapLayers := max(1, opts.MapLayers)
	if call != nil && len(call.Args) > 0 {
		if bg, ok := mode0StaticRuntimeInt(call.Args[0]); ok {
			layers = append(layers, bg)
		}
		if len(call.Args) > 5 {
			if w, ok := mode0StaticRuntimeInt(call.Args[5]); ok && w > 0 {
				windowW = min(windowW, w)
			}
		}
		if len(call.Args) > 6 {
			if h, ok := mode0StaticRuntimeInt(call.Args[6]); ok && h > 0 {
				windowH = min(windowH, h)
			}
		}
	}
	if len(layers) == 0 {
		for bg := 0; bg < mapLayers; bg++ {
			layers = append(layers, bg)
		}
	}
	total := 0
	for _, bg := range layers {
		total += mode0TileLinkWindowMax(opts, table, bg, windowW, windowH)
	}
	return total
}

// mode0TileLinkWindowMax is the most linked cells of one plane inside any
// windowW x windowH cell window, using a 2D prefix sum.
func mode0TileLinkWindowMax(opts Options, table []uint8, bg, windowW, windowH int) int {
	w, h := opts.MapW, opts.MapH
	if w <= 0 || h <= 0 || bg < 0 {
		return 0
	}
	base := bg * w * h
	if base+w*h > len(opts.Map) {
		return 0
	}
	sum := make([]int, (w+1)*(h+1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := 0
			if id := opts.Map[base+y*w+x]; int(id) < len(table) && table[id] != 0 {
				cell = 1
			}
			sum[(y+1)*(w+1)+x+1] = cell + sum[y*(w+1)+x+1] + sum[(y+1)*(w+1)+x] - sum[y*(w+1)+x]
		}
	}
	windowW, windowH = min(windowW, w), min(windowH, h)
	best := 0
	for y := 0; y+windowH <= h; y++ {
		for x := 0; x+windowW <= w; x++ {
			x1, y1 := x+windowW, y+windowH
			count := sum[y1*(w+1)+x1] - sum[y*(w+1)+x1] - sum[y1*(w+1)+x] + sum[y*(w+1)+x]
			best = max(best, count)
		}
	}
	return best
}

// validateMode0TileLinkMsets keeps the Mode 0 object bound sound: the bound
// counts linked cells placed in the map, so no mset may write a linked tile.
// Values must be literals or finite sets that the ID analysis enumerates.
func validateMode0TileLinkMsets(chunk *luaparser.Chunk, opts Options, arity *generator) error {
	if len(opts.TileLinks) == 0 {
		return nil
	}
	linked := map[int]bool{}
	for _, link := range opts.TileLinks {
		linked[int(link.Tile)] = true
	}
	analysis := newMode0SpriteIDAnalysis(chunk)
	var firstErr error
	var visit func(luaparser.Stmt)
	visit = func(stmt luaparser.Stmt) {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			for _, body := range fn.Body.Body {
				visit(body)
			}
			return
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			call, ok := expr.(*luaparser.CallExpr)
			if !ok || firstErr != nil {
				return
			}
			if name, _ := callKey(call); name != "mset" {
				return
			}
			if arity.exprListWidth(call.Args) != len(call.Args) {
				firstErr = mode0CallError(call, reasonMode0TileLinks, "Mode 0 linked maps require explicit mset() arguments so the written tile ID can be proven unlinked")
				return
			}
			if len(call.Args) < 3 {
				return
			}
			value := call.Args[len(call.Args)-1]
			var ids []int
			if id, literal := mode0StaticRuntimeInt(value); literal {
				ids = []int{id}
			} else if enumerated, proven, _ := analysis.finiteIDs(value); proven {
				ids = enumerated
			} else {
				firstErr = mode0CallError(call, reasonMode0TileLinks, "mset() can write a linked tile; Mode 0 sizes linked objects from the tiles placed in the map, so mset values must be provably unlinked literals")
				return
			}
			for _, id := range ids {
				if linked[id] {
					firstErr = mode0CallError(call, reasonMode0TileLinks, fmt.Sprintf("mset() writes linked tile %d; Mode 0 sizes linked objects from the tiles placed in the map", id))
					return
				}
			}
		})
	}
	for _, stmt := range chunk.Body {
		visit(stmt)
	}
	return firstErr
}
