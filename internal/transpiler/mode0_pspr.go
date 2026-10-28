package transpiler

import (
	"fmt"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/audio"
)

const (
	mode0PsprSiteLimit   = mode0OAMEntryLimit
	mode0PsprWindowTiles = runtimeapi.Mode0ObjectTileLimit
)

type mode0PsprSite struct {
	call       *luaparser.CallExpr
	copies     int
	uniqueMax  int
	partsMax   int
	inRangeIDs []int
}

func validateMode0PsprCallWithEnv(
	call *luaparser.CallExpr,
	opts Options,
	analysis *mode0SpriteIDAnalysis,
	eval *mode0NumEval,
	env mode0NumEnv,
) (ids []int, uniqueMax, partsMax int, err error) {
	if name, _ := callKey(call); name == "anim" {
		return validateMode0AnimCall(call, opts)
	}
	if call == nil || len(call.Args) < 3 || len(call.Args) > 5 {
		return nil, 0, 0, mode0CallError(call, reasonMode0PsprShape, "Mode 0 pspr() requires 3 to 5 arguments")
	}
	var proven bool
	if _, literal := staticNumberLiteral(call.Args[0]); literal {
		id, ok := mode0StaticRuntimeInt(call.Args[0])
		ids, proven = []int{id}, ok
	}
	if !proven && eval != nil {
		ids, proven, _ = eval.spriteIDs(call.Args[0], env)
	}
	if !proven && analysis != nil {
		ids, proven, _ = analysis.finiteIDs(call.Args[0])
	}
	if !proven || len(ids) == 0 {
		// Every runtime ID either selects an authored pose or is a no-op.
		// Budgeting the whole library is a sound bound even when expression
		// analysis cannot enumerate a large animation counter or table lookup.
		ids = make([]int, canonicalPoseCount(opts))
		for id := range ids {
			ids[id] = id
		}
	}
	ids = expandedPoseIDs(opts, ids)
	inRange := make([]int, 0, len(ids))
	for _, id := range ids {
		// Scalar proofs may still supply source integer literals. The runtime
		// first converts these to signed 16.16 before indexing a pose.
		id = runtimeapi.FromInt(id).Int()
		unique, parts, ok := mode0PoseUniqueAndParts(opts, id)
		if !ok {
			continue
		}
		inRange = append(inRange, id)
		if unique > uniqueMax {
			uniqueMax = unique
		}
		if parts > partsMax {
			partsMax = parts
		}
	}
	return inRange, uniqueMax, partsMax, nil
}

func mode0PoseUniqueAndParts(opts Options, id int) (unique, parts int, inRange bool) {
	if id < 0 || id >= len(opts.PoseIndex) {
		return 0, 0, false
	}
	r := opts.PoseIndex[id]
	start := int(r.Start)
	count := int(r.Count)
	end := start + count
	if start < 0 || end > len(opts.PoseParts) || start > len(opts.PoseParts) {
		return 0, 0, false
	}
	objects, tiles := runtimeapi.BuildPoseOBJPlan(opts.PoseParts[start:end])
	return len(tiles), len(objects), true
}

func mode0PsprSitesFromOps(ops []mode0DrawOp) []mode0PsprSite {
	// A helper call may be reached repeatedly with different arguments. Give
	// every invocation the same stride, sized for the largest proven pose.
	var calls []mode0PsprSite
	byCall := make(map[*luaparser.CallExpr]int)
	for _, op := range ops {
		if op.name != "pspr" || op.call == nil {
			continue
		}
		copies := op.copies
		if copies < 1 {
			copies = 1
		}
		if index, exists := byCall[op.call]; exists {
			calls[index].copies += copies
			calls[index].uniqueMax = max(calls[index].uniqueMax, op.psprUnique)
			calls[index].partsMax = max(calls[index].partsMax, op.psprParts)
		} else {
			byCall[op.call] = len(calls)
			calls = append(calls, mode0PsprSite{
				call:       op.call,
				copies:     copies,
				uniqueMax:  op.psprUnique,
				partsMax:   op.psprParts,
				inRangeIDs: op.psprIDs,
			})
		}
	}
	var sites []mode0PsprSite
	for _, call := range calls {
		for i := 0; i < call.copies; i++ {
			sites = append(sites, call)
		}
	}
	return sites
}

// Ordinary sheet objects retain their identity-mapped tile IDs. Pose
// windows start after their highest reachable source cell.
func mode0PsprWindowStart(ops []mode0DrawOp) int {
	start := 0
	for _, op := range ops {
		if op.name == "spr" || op.name == "sspr" {
			start = max(start, op.lastID+1)
		}
	}
	return start
}

func mode0PsprTileLimit(chunk *luaparser.Chunk, ops []mode0DrawOp, opts Options) int {
	if mode0ObjectPackingEligible(chunk, opts) {
		return mode0PsprWindowTiles - mode0TextTileLimit - mode0PackedObjectTileCapacity
	}
	for _, op := range ops {
		if op.name == "print" {
			if entries, ok := mode0PrintOAMEntries(op.call, op.printKind); ok && entries > 0 {
				return runtimeapi.Mode0TextTileBase
			}
		}
	}
	return mode0PsprWindowTiles
}

func validateMode0PsprProof(chunk *luaparser.Chunk, ops []mode0DrawOp, opts Options, sprCalls, ssprCalls int) error {
	if planMode0ResidentPoses(chunk, ops, opts) != nil {
		return nil
	}
	sites := mode0PsprSitesFromOps(ops)
	if len(opts.PoseVariantNames) > 0 && len(sites) > 0 {
		return mode0CallError(sites[0].call, reasonMode0ResourceLimit, "Mode 0 color variants require a resident 4bpp pose bank: at most 15 visible colors per object, 15 compatible palette banks, no spr/sspr/print/pal/palt/sset, and sufficient OBJ VRAM")
	}
	if len(sites) == 0 {
		return nil
	}
	if len(sites) > mode0PsprSiteLimit {
		return mode0CallError(sites[mode0PsprSiteLimit].call, reasonMode0ResourceLimit,
			fmt.Sprintf("Mode 0 pspr() permits at most %d sites after copies", mode0PsprSiteLimit))
	}
	tiles := mode0PsprWindowStart(ops)
	for _, site := range sites {
		tiles += site.uniqueMax
	}
	limit := mode0PsprTileLimit(chunk, ops, opts)
	if tiles > limit {
		return mode0CallError(sites[0].call, reasonMode0ResourceLimit,
			fmt.Sprintf("Mode 0 sprites and pose windows need %d tiles, exceeding the %d available OBJ tiles (0-%d); reduce simultaneous graphics or use Mode 4", tiles, limit, limit-1))
	}
	return nil
}

type mode0PsprAllocation struct {
	site, count, window, stride int
}

func mode0PsprEmitPlan(ops []mode0DrawOp) map[*luaparser.CallExpr]mode0PsprAllocation {
	sites := mode0PsprSitesFromOps(ops)
	plan := make(map[*luaparser.CallExpr]mode0PsprAllocation)
	window := mode0PsprWindowStart(ops)
	for index, site := range sites {
		allocation, exists := plan[site.call]
		if !exists {
			allocation = mode0PsprAllocation{site: index, window: window, stride: site.uniqueMax}
		}
		allocation.count++
		plan[site.call] = allocation
		window += site.uniqueMax
	}
	return plan
}

// Streaming fits in VRAM only if its worst commit also fits VBlank. Pixels
// are prepared in EWRAM before the window and transferred in one DMA. Keep
// room for OAM/palettes/bookkeeping, dirty maps and direct sheet mutations;
// native audio may interrupt a long commit twice. Native evidence lives in
// docs/benchmarks/render-modes/pspr-stream-v1.md and the paired renderbench fixture.
func validateMode0PoseCommit(chunk *luaparser.Chunk, ops []mode0DrawOp, opts Options, layers, mutations int) error {
	if planMode0ResidentPoses(chunk, ops, opts) != nil {
		// All immutable pose pixels were uploaded before visible rendering.
		// The ordinary map/OAM/audio proof still applies without pose DMA.
		return nil
	}
	sites := mode0PsprSitesFromOps(ops)
	if len(opts.PoseVariantNames) > 0 && len(sites) > 0 {
		return mode0CallError(sites[0].call, reasonMode0ResourceLimit, "Mode 0 color variants require a resident 4bpp pose bank: at most 15 visible colors per object, 15 compatible palette banks, no spr/sspr/print/pal/palt/sset, and sufficient OBJ VRAM")
	}
	if len(sites) == 0 {
		return nil
	}
	tiles := 0
	for _, site := range sites {
		tiles += site.uniqueMax
	}
	if tiles == 0 {
		return nil
	}
	const window = 67 * 1232
	cycles := mode0PoseCommitCycles(chunk, opts, tiles, layers, mutations)
	if cycles > window {
		return mode0CallError(sites[0].call, reasonMode0ResourceLimit,
			fmt.Sprintf("Mode 0 pose streaming with %d BG layers and %d sheet mutations needs a conservative %d-cycle commit, exceeding the %d-cycle VBlank window; reduce simultaneous uploads or use Mode 4", layers, mutations, cycles, window))
	}
	return nil
}

func mode0PoseCommitCycles(chunk *luaparser.Chunk, opts Options, tiles, layers, mutations int) uint64 {
	cycles := uint64(16000 + 128*tiles + 9000*layers + 2048*mutations)
	if mode0ObjectPackingEligible(chunk, opts) {
		cycles += mode0PackedObjectTileCapacity * 128
	}
	graph := newMode0CallGraph(chunk)
	if graph.firstBuiltinFromFunction("_update", "pal", "palt") != nil {
		cycles += 8000
	}
	if opts.NativeAudio != nil && chunkUsesNativeAudio(chunk) {
		irqs := uint64(1)
		if cycles > audio.NativeServicePeriodCycles-audio.NativeServiceCycleBudget {
			irqs = 2
		}
		cycles += irqs * audio.NativeServiceCycleBudget
	}
	return cycles
}
