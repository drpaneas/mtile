package transpiler

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

// Renderer is a compile-time cartridge backend. RendererMode4 is zero so old
// callers and projects retain the established software-framebuffer renderer.
type Renderer uint8

const (
	RendererMode4 Renderer = iota
	RendererMode0
	RendererAuto
)

func (r Renderer) String() string {
	switch r {
	case RendererMode0:
		return "mode0"
	case RendererAuto:
		return "auto"
	default:
		return "mode4"
	}
}

// ParseRenderer parses the CLI spelling without accepting ambiguous aliases.
func ParseRenderer(value string) (Renderer, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mode4":
		return RendererMode4, nil
	case "mode0":
		return RendererMode0, nil
	case "auto":
		return RendererAuto, nil
	default:
		return RendererMode4, fmt.Errorf("renderer must be mode4, mode0, or auto, got %q", value)
	}
}

// RendererReport records why a backend was selected. Reason is a stable code
// suitable for CLI output and benchmark manifests.
type RendererReport struct {
	Requested Renderer
	Selected  Renderer
	Reason    string
	// Profile versions the evidence-backed AUTO rule set. It is empty for an
	// explicitly forced renderer.
	Profile string
	// Diagnostic explains an AUTO fallback. It is empty for forced selections
	// and successful AUTO Mode 0 proofs. Unlike Reason, it is human-readable
	// and may include a source position.
	Diagnostic string
	// Warnings are nonfatal, source-positioned asset diagnostics. They do not
	// change renderer selection or numeric API behavior.
	Warnings []string
	// Mode0Layers is the exact number of regular BG screen shadows selected
	// by the proof. It is zero for Mode 4 and drives a link-time build tag so
	// one-layer cartridges do not reserve four layers of EWRAM.
	Mode0Layers  int
	Mode0Objects bool
	// Pose streaming stages pixels before VBlank. Its buffer is linked only
	// for carts that actually draw hardware poses.
	Mode0PoseStreaming bool
	// Resident libraries upload all reachable layouts once during force-blanked
	// initialization, sharing exact packed pixel runs between poses and sites.
	Mode0PoseResident bool
	Mode0Text         bool
	// Mode0ObjectPacking enables the internal 16x16/32x32 OBJ cache. Packed
	// shapes consume one OAM entry while retaining their true pixel-width cost
	// in the scanline-cycle proof.
	Mode0ObjectPacking bool
	// Mode0OAMEntries and Mode0OBJLineCycles are the exact post-packing OAM
	// allocation and conservative worst scanline proof for the linked renderer.
	Mode0OAMEntries    int
	Mode0OBJLineCycles int
	// Mode0TextKeys is the interned (glyph,color) atlas uploaded into OBJ tiles
	// 448-479. Build stamps authored sheet cells 480-511 from the same keys.
	Mode0TextKeys []uint16
	// MapPlaneMask identifies authored (or runtime-mutable) logical planes
	// that need map storage. MapDrawMask is the subset materialized by map()
	// calls. Both preserve logical BG numbers while allowing dense allocation.
	MapPlaneMask uint8
	MapDrawMask  uint8
	// MapStreaming is the compile-time storage strategy after unused logical
	// planes have been removed. Build uses it to select one specialized runtime
	// path instead of retaining a per-access compact/streaming branch.
	MapStreaming bool
	// MapRasterDisabled proves that generation emitted no small-map raster
	// storage. Build can omit that optional cache while retaining cell and
	// packed-map rendering. A zero-valued report preserves the generic path.
	MapRasterDisabled bool
	// NativeAudioLinked reports whether reachable source uses a native audio
	// builtin and the generated cartridge therefore contains the PSG runtime
	// and authored audio records. Merely authoring assets does not link them.
	NativeAudioLinked bool
	// HeapLinked reports whether generated code can allocate a Lua table/string
	// and therefore retains collection plus the two target semispaces.
	HeapLinked bool
	// SheetReadOnly reports that no source path calls sset. Build uses that proof
	// for the compact Mode 4 sheet representation or the narrower Mode 0
	// dirty-upload-scan elision.
	SheetReadOnly bool
	// StaticStrings is the exact number of entries initialized in Context's
	// static string table and selects the smallest safe target capacity tier.
	StaticStrings int
}

const (
	reasonMode4Forced          = "M4_FORCED"
	reasonMode0ForcedProven    = "M0_FORCED_PROVEN"
	reasonAutoMode0Eligible    = "M0_AUTO_ELIGIBLE"
	reasonAutoMode0Ineligible  = "M4_AUTO_MODE0_INELIGIBLE"
	reasonAutoMode0CostUnknown = "M4_AUTO_MODE0_COST_UNPROVEN"
	reasonMode0CostUnproven    = "M0_COST_UNPROVEN"
	reasonMode0Mode4Builtin    = "M0_MODE4_BUILTIN"
	reasonMode0UnsupportedCall = "M0_UNSUPPORTED_BUILTIN"
	reasonMode0MutableSheet    = "M0_MUTABLE_SHEET"
	reasonMode0MutableMap      = "M0_MUTABLE_MAP"
	reasonMode0DrawShape       = "M0_DRAW_SHAPE"
	reasonMode0ClsShape        = "M0_CLS_SHAPE"
	reasonMode0MapShape        = "M0_MAP_SHAPE"
	reasonMode0SpriteShape     = "M0_SPR_SHAPE"
	reasonMode0PsprShape       = "M0_PSPR_SHAPE"
	reasonMode0TextShape       = "M0_PRINT_SHAPE"
	reasonMode0ObjectBudget    = "M0_OBJ_BUDGET"
	reasonMode0ResourceLimit   = "M0_RESOURCE_LIMIT"
	reasonExplicitMapContract  = "MAP_EXPLICIT_CONTRACT"
	reasonMode0TileLinks       = "M0_TILE_LINKS"
	autoProfileTileV1          = "tile-v1"
)

// SelectRenderer applies the same conservative proof to forced Mode 0 and
// AUTO. Forced Mode 0 fails closed, while AUTO records the proof failure and
// falls back to the exact Mode 4 renderer.
func SelectRenderer(chunk *luaparser.Chunk, opts Options) (RendererReport, error) {
	report := RendererReport{Requested: opts.Renderer, Selected: RendererMode4}
	if chunk == nil {
		return report, errors.New("transpiler: nil chunk")
	}
	restorePause, _, pauseErr := lowerPauseAnalysisRoots(chunk)
	defer restorePause()
	if pauseErr != nil {
		return report, pauseErr
	}
	warnings, restore, err := lowerPoseNames(chunk, opts)
	defer restore()
	report.Warnings = warnings
	if err != nil {
		return report, err
	}
	plan, err := validateMapContract(chunk, opts)
	if err != nil {
		return report, err
	}
	report.MapPlaneMask = plan.storageMask
	report.MapDrawMask = plan.drawMask
	switch opts.Renderer {
	case RendererMode4:
		report.Reason = reasonMode4Forced
		return report, nil
	case RendererAuto:
		report.Profile = autoProfileTileV1
		layers, objects, text, err := validateMode0Compatible(chunk, opts, plan)
		if err != nil {
			report.Reason = reasonAutoMode0Ineligible
			report.Diagnostic = err.Error()
			return report, nil
		}
		if err := validateAutoMode0Cost(chunk, plan.analysis); err != nil {
			report.Reason = reasonAutoMode0CostUnknown
			report.Diagnostic = err.Error()
			return report, nil
		}
		report.Selected = RendererMode0
		report.Reason = reasonAutoMode0Eligible
		report.Mode0Layers = layers
		report.Mode0Objects = objects
		report.Mode0PoseStreaming = objects && (chunkCallsBuiltin(chunk, "pspr") || chunkCallsBuiltin(chunk, "anim"))
		report.Mode0PoseResident = report.Mode0PoseStreaming && mode0ResidentPosesForChunk(chunk, opts) != nil
		report.Mode0PoseStreaming = report.Mode0PoseStreaming && !report.Mode0PoseResident
		report.Mode0Text = text
		budget := mode0ObjectBudgetForChunk(chunk, opts)
		report.Mode0ObjectPacking = budget.PackingEnabled
		report.Mode0OAMEntries = budget.OAMEntries
		report.Mode0OBJLineCycles = budget.MaxLineCycles
		return report, nil
	case RendererMode0:
		layers, objects, text, err := validateMode0Compatible(chunk, opts, plan)
		if err != nil {
			return report, err
		}
		report.Selected = RendererMode0
		report.Reason = reasonMode0ForcedProven
		report.Mode0Layers = layers
		report.Mode0Objects = objects
		report.Mode0PoseStreaming = objects && (chunkCallsBuiltin(chunk, "pspr") || chunkCallsBuiltin(chunk, "anim"))
		report.Mode0PoseResident = report.Mode0PoseStreaming && mode0ResidentPosesForChunk(chunk, opts) != nil
		report.Mode0PoseStreaming = report.Mode0PoseStreaming && !report.Mode0PoseResident
		report.Mode0Text = text
		budget := mode0ObjectBudgetForChunk(chunk, opts)
		report.Mode0ObjectPacking = budget.PackingEnabled
		report.Mode0OAMEntries = budget.OAMEntries
		report.Mode0OBJLineCycles = budget.MaxLineCycles
		return report, nil
	default:
		return report, fmt.Errorf("transpiler: unknown renderer value %d", opts.Renderer)
	}
}

const mode0PackedObjectSourceLimit = 416

// mode0ObjectPackingEligible is deliberately an optimization proof, not a
// renderer eligibility proof. Tiles 416..447 are a private 32-cell cache;
// text keeps its 448..479 range. Mutable sheets retain the ordinary
// metasprite path so sset never aliases this cache in a generated cart.
func mode0ObjectPackingEligible(chunk *luaparser.Chunk, opts Options) bool {
	if chunk == nil || chunkCallsBuiltin(chunk, "sset") {
		return false
	}
	cols, rows := opts.SheetW/8, opts.SheetH/8
	if cols <= 0 || rows <= 0 {
		return false
	}
	foundPackedShape, dynamicSource, maxSourceID := false, false, -1
	var visitStmt func(luaparser.Stmt)
	visitStmt = func(stmt luaparser.Stmt) {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			for _, bodyStmt := range fn.Body.Body {
				visitStmt(bodyStmt)
			}
			return
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			call, ok := expr.(*luaparser.CallExpr)
			if !ok {
				return
			}
			name, ok := callKey(call)
			if !ok || name != "spr" && name != "sspr" {
				return
			}
			var lastID int
			var err error
			switch name {
			case "spr":
				if len(call.Args) == 0 {
					return
				}
				if _, literal := staticNumberLiteral(call.Args[0]); !literal {
					dynamicSource = true
					return
				}
				_, lastID, err = validateMode0SprCall(call, cols, rows)
				foundPackedShape = foundPackedShape || mode0SprCallPackedCells(call) != 0
			case "sspr":
				_, lastID, err = validateMode0SSprCall(call, opts.SheetW, opts.SheetH, cols)
				foundPackedShape = foundPackedShape || mode0SSprCallPackedCells(call) != 0
			}
			if err == nil && lastID > maxSourceID {
				maxSourceID = lastID
			}
		})
	}
	for _, stmt := range chunk.Body {
		visitStmt(stmt)
	}
	return !dynamicSource && foundPackedShape && maxSourceID >= 0 && maxSourceID < mode0PackedObjectSourceLimit
}

func mode0SprCallPackedCells(call *luaparser.CallExpr) int {
	if call == nil || len(call.Args) < 5 {
		return 0
	}
	w, wok := staticNumberLiteral(call.Args[3])
	h, hok := staticNumberLiteral(call.Args[4])
	if !wok || !hok || w != h || w != 2 && w != 4 {
		return 0
	}
	return int(w)
}

func mode0SSprCallPackedCells(call *luaparser.CallExpr) int {
	if call == nil || len(call.Args) < 6 {
		return 0
	}
	sw, swok := staticNumberLiteral(call.Args[2])
	sh, shok := staticNumberLiteral(call.Args[3])
	if !swok || !shok || sw != sh || sw != 16 && sw != 32 {
		return 0
	}
	if len(call.Args) > 6 {
		dw, ok := staticNumberLiteral(call.Args[6])
		if !ok || dw != sw {
			return 0
		}
	}
	if len(call.Args) > 7 {
		dh, ok := staticNumberLiteral(call.Args[7])
		if !ok || dh != sh {
			return 0
		}
	}
	return int(sw / 8)
}

type mapContractPlan struct {
	storageMask uint8
	drawMask    uint8
	analysis    *generator
}

// validateMapContract defines the deliberately small, unambiguous map API:
// map() draws every non-empty plane. Every non-empty argument list starts
// with bg; the remaining source/destination/size/mask arguments keep their
// established order and defaults. Migrating an old call therefore consists
// only of prefixing the desired logical BG number.
func validateMapContract(chunk *luaparser.Chunk, opts Options) (mapContractPlan, error) {
	layers := opts.MapLayers
	if layers == 0 {
		layers = 1
	}
	if layers < 1 || layers > assetsync.MapLayerCount {
		return mapContractPlan{}, fmt.Errorf("transpiler: map layer count %d is outside 1..%d", layers, assetsync.MapLayerCount)
	}
	if opts.MapW < 0 || opts.MapH < 0 {
		return mapContractPlan{}, errors.New("transpiler: invalid map dimensions")
	}
	if _, ok := checkedProduct(opts.MapW, opts.MapH); !ok {
		return mapContractPlan{}, errors.New("transpiler: map dimensions overflow")
	}
	plan := mapContractPlan{storageMask: authoredMapMask(opts)}
	analysis := newGenerator(chunk, Options{})
	if err := analysis.discoverFunctions(); err != nil {
		return plan, err
	}
	if err := analysis.validateSemantics(); err != nil {
		return plan, err
	}
	if err := analysis.resolveReturnArities(); err != nil {
		return plan, err
	}
	if err := analysis.validateBuiltinCallArities(); err != nil {
		return plan, err
	}
	plan.analysis = analysis
	mapLayers := opts.MapLayers
	if mapLayers == 0 {
		mapLayers = 1
	}
	var draw *luaparser.FunctionDeclStmt
	var calls []*luaparser.CallExpr
	var mutableMask uint8
	var visitStmt func(luaparser.Stmt)
	visitStmt = func(stmt luaparser.Stmt) {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			if functionKey(fn) == "_draw" {
				draw = fn
			}
			for _, bodyStmt := range fn.Body.Body {
				visitStmt(bodyStmt)
			}
			return
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			call, ok := expr.(*luaparser.CallExpr)
			if !ok {
				return
			}
			name, ok := callKey(call)
			if ok {
				switch name {
				case "map":
					calls = append(calls, call)
				case "mset":
					argCount := analysis.exprListWidth(call.Args)
					if argCount == 3 {
						mutableMask |= 1
						break
					}
					if argCount != 4 {
						break
					}
					value, literal := mode0StaticRuntimeInt(call.Args[0])
					if literal {
						if value >= 0 && value < mapLayers {
							mutableMask |= 1 << uint(value)
						}
						break
					}
					mutableMask |= uint8((1 << uint(mapLayers)) - 1)
				}
			}
		})
	}
	for _, stmt := range chunk.Body {
		visitStmt(stmt)
	}
	plan.storageMask |= mutableMask
	if len(calls) == 0 {
		return plan, nil
	}
	for _, call := range calls {
		if analysis.exprListWidth(call.Args) > 8 {
			return plan, explicitMapError(call, "map() accepts at most eight arguments")
		}
	}
	if draw == nil {
		return plan, explicitMapError(calls[0], "map() calls must be direct statements in _draw")
	}
	direct := make([]*luaparser.CallExpr, 0, len(calls))
	for _, stmt := range draw.Body.Body {
		callStmt, ok := stmt.(*luaparser.CallStmt)
		if !ok {
			continue
		}
		name, ok := callKey(callStmt.Call)
		if ok && name == "map" {
			direct = append(direct, callStmt.Call)
		}
	}
	if len(direct) != len(calls) {
		return plan, explicitMapError(calls[0], "map() calls must be unconditional direct statements in _draw")
	}
	seen := [assetsync.MapLayerCount]bool{}
	lastBG := -1
	autoLayers := false
	for _, call := range direct {
		if analysis.exprListWidth(call.Args) == 0 {
			if len(direct) != 1 {
				return plan, explicitMapError(call, "map() cannot be mixed with map(bg) calls")
			}
			autoLayers = true
			continue
		}
		value, literal := staticNumberLiteral(call.Args[0])
		if !literal || value != math.Trunc(value) || value < 0 || value >= assetsync.MapLayerCount {
			return plan, explicitMapError(call, "bg must be an integer literal from 0 to 3")
		}
		bg := int(value)
		if bg >= mapLayers {
			return plan, explicitMapError(call, fmt.Sprintf("BG%d is not present in this %d-layer map asset", bg, mapLayers))
		}
		if seen[bg] {
			return plan, explicitMapError(call, fmt.Sprintf("BG%d is drawn more than once", bg))
		}
		if bg < lastBG {
			return plan, explicitMapError(call, "calls must appear in back-to-front BG0..BG3 order")
		}
		seen[bg] = true
		lastBG = bg
		if plan.storageMask&(1<<uint(bg)) != 0 {
			plan.drawMask |= 1 << uint(bg)
		}
	}
	if autoLayers {
		plan.drawMask = plan.storageMask
	}
	return plan, nil
}

func authoredMapMask(opts Options) uint8 {
	layers := opts.MapLayers
	if layers == 0 {
		layers = 1
	}
	planeCells, ok := checkedProduct(opts.MapW, opts.MapH)
	if !ok || planeCells <= 0 {
		return 0
	}
	totalCells, ok := checkedProduct(planeCells, layers)
	if !ok || len(opts.Map) < totalCells {
		return 0
	}
	var mask uint8
	for bg := 0; bg < layers; bg++ {
		base := bg * planeCells
		for _, id := range opts.Map[base : base+planeCells] {
			if id != 0 {
				mask |= 1 << uint(bg)
				break
			}
		}
	}
	return mask
}

func mapMaskCount(mask uint8) int {
	count := 0
	for mask != 0 {
		count += int(mask & 1)
		mask >>= 1
	}
	return count
}

func explicitMapError(call *luaparser.CallExpr, detail string) error {
	if call == nil {
		return fmt.Errorf("transpiler: [%s] %s", reasonExplicitMapContract, detail)
	}
	return fmt.Errorf("transpiler: %s: [%s] %s", call.Pos(), reasonExplicitMapContract, detail)
}

// validateAutoMode0Cost is deliberately narrower than semantic Mode 0
// compatibility. Runtime asset mutations are exact in forced Mode 0, but their
// upload/reclassification cost depends on data and call frequency. Paired mGBA
// measurements prove the bounded sset/mset shapes below; runtime fset remains
// below the profile's material-win threshold, and unbounded shapes stay on
// Mode 4 until a later profile supplies evidence.
func validateAutoMode0Cost(chunk *luaparser.Chunk, analysis *generator) error {
	var initFunction, update *luaparser.FunctionDeclStmt
	var firstMset, firstDynamicLayerMset, firstFset *luaparser.CallExpr
	var msetCalls, fsetCalls int
	var visitStmt func(luaparser.Stmt)
	visitStmt = func(stmt luaparser.Stmt) {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			switch functionKey(fn) {
			case "_init":
				initFunction = fn
			case "_update":
				update = fn
			}
			for _, bodyStmt := range fn.Body.Body {
				visitStmt(bodyStmt)
			}
			return
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			call, ok := expr.(*luaparser.CallExpr)
			if !ok {
				return
			}
			name, ok := callKey(call)
			if !ok {
				return
			}
			switch name {
			case "mset":
				msetCalls++
				if firstMset == nil {
					firstMset = call
				}
				if analysis != nil && analysis.exprListWidth(call.Args) == 4 {
					if _, literal := staticNumberLiteral(call.Args[0]); !literal && firstDynamicLayerMset == nil {
						firstDynamicLayerMset = call
					}
				}
			case "fset":
				fsetCalls++
				if firstFset == nil {
					firstFset = call
				}
			}
		})
	}
	for _, stmt := range chunk.Body {
		visitStmt(stmt)
	}
	const maxAutoMsetPerUpdate = 16
	if firstDynamicLayerMset != nil {
		return mode0CallError(firstDynamicLayerMset, reasonMode0CostUnproven,
			"auto requires the layer argument of mset(layer,x,y,id) to be a numeric literal; dynamic-layer mutation is exact in forced mode0 but its retained-plane and upload cost is not yet benchmark-proven")
	}
	directInitMset := countDirectMode0Calls(initFunction, "mset")
	directUpdateMset := countDirectMode0Calls(update, "mset")
	if msetCalls != directInitMset+directUpdateMset || directUpdateMset > maxAutoMsetPerUpdate {
		return mode0CallError(firstMset, reasonMode0CostUnproven,
			"auto requires mset() to be an unconditional direct statement in _init or _update, with at most 16 calls per update; use forced mode0 to opt in")
	}
	if fsetCalls != countDirectMode0Calls(initFunction, "fset") {
		return mode0CallError(firstFset, reasonMode0CostUnproven,
			"auto keeps runtime fset() workloads on Mode 4 because layer reclassification cost is data-dependent; init-only fset() is supported, or use forced mode0 to opt in")
	}
	return nil
}

func validateMode0Compatible(chunk *luaparser.Chunk, opts Options, plan mapContractPlan) (int, bool, bool, error) {
	if err := validateMode0TileLinkMsets(chunk, opts, plan.analysis); err != nil {
		return 0, false, false, err
	}
	cols, rows := opts.SheetW/8, opts.SheetH/8
	if opts.SheetW <= 0 || opts.SheetH <= 0 || opts.SheetW%8 != 0 || opts.SheetH%8 != 0 {
		return 0, false, false, fmt.Errorf("transpiler: [%s] Mode 0 requires an 8x8-aligned sheet", reasonMode0ResourceLimit)
	}
	var initFunction, update, draw *luaparser.FunctionDeclStmt
	spriteIDs := newMode0SpriteIDAnalysis(chunk)
	var firstErr error
	var clsCalls, mapCalls, sprCalls, ssprCalls, printCalls, ssetCalls, psprCalls int
	var visitStmt func(luaparser.Stmt)
	visitStmt = func(stmt luaparser.Stmt) {
		if firstErr != nil {
			return
		}
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			switch functionKey(fn) {
			case "_init":
				initFunction = fn
			case "_update":
				update = fn
			case "_draw":
				draw = fn
			}
			for _, bodyStmt := range fn.Body.Body {
				visitStmt(bodyStmt)
			}
			return
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			if firstErr != nil {
				return
			}
			if variable, ok := expr.(*luaparser.VarExpr); ok &&
				(variable.Name == "_init" || variable.Name == "_update" || variable.Name == "_draw") {
				firstErr = fmt.Errorf("transpiler: %s: [%s] Mode 0 lifecycle callbacks cannot be called or referenced by user code", variable.Pos(), reasonMode0DrawShape)
				return
			}
			call, ok := expr.(*luaparser.CallExpr)
			if !ok {
				return
			}
			name, ok := callKey(call)
			if !ok {
				return
			}
			// mset's linked-object proof checks expanded arguments itself;
			// ordinary map writes retain their unrestricted argument semantics.
			if name != "mset" && plan.analysis != nil && BuiltinRendererCompatibility(name) == RendererMode0Conditional &&
				plan.analysis.exprListWidth(call.Args) != len(call.Args) {
				firstErr = mode0CallError(call, reasonMode0DrawShape,
					name+"() receives expanded multi-results; Mode 0 requires explicit arguments so its static shape and object budget can be proven")
				return
			}
			switch name {
			case "cls":
				clsCalls++
			case "map":
				mapCalls++
			case "spr":
				sprCalls++
			case "sspr":
				ssprCalls++
			case "print":
				printCalls++
			case "sset":
				ssetCalls++
			case "pspr", "anim":
				psprCalls++
			case "clip":
				firstErr = validateMode0ClipCall(call)
			case "pal":
				firstErr = validateMode0PalCall(call)
			case "palt":
				firstErr = validateMode0PaltCall(call)
			}
			switch BuiltinRendererCompatibility(name) {
			case RendererMode4Only:
				firstErr = mode0CallError(call, reasonMode0Mode4Builtin, name+"() requires the Mode 4 framebuffer")
			case RendererMode0Conditional:
				if name != "pvariant" && name != "cls" && name != "map" && name != "mset" && name != "spr" && name != "sspr" && name != "pspr" && name != "anim" && name != "print" && name != "clip" && name != "pal" && name != "palt" {
					firstErr = mode0CallError(call, reasonMode0UnsupportedCall, name+"() is not in the Mode 0 tile-v1 proof")
				}
			}
		})
	}
	for _, stmt := range chunk.Body {
		visitStmt(stmt)
	}
	if firstErr != nil {
		return 0, false, false, firstErr
	}
	callGraph := newMode0CallGraph(chunk)
	mode0Layers := mapMaskCount(plan.drawMask)
	// Four dirty BG maps already consume a substantial part of VBlank. Native
	// dense-audio captures exceed the complete window when per-frame sheet or
	// palette work is added. Waiting longer cannot make that commit atomic.
	// Match native-audio linkage, rather than rejecting unused authored assets;
	// initialization-only mutations are committed while the LCD is blank.
	if mode0Layers == 4 && opts.NativeAudio != nil && chunkUsesNativeAudio(chunk) {
		mutation := callGraph.firstBuiltinFromFunction("_update", "sset", "pal")
		if mutation == nil {
			mutation = callGraph.firstBuiltinFromFunction("_draw", "sset", "pal")
		}
		if mutation != nil {
			return 0, false, false, mode0CallError(mutation, reasonMode0ResourceLimit,
				"Mode 0 with native audio and four materialized BGs cannot prove per-frame sset()/pal() commits fit VBlank; use Mode 4")
		}
	}
	if mapCalls != 0 && (opts.MapW <= 0 || opts.MapH <= 0) {
		return 0, false, false, fmt.Errorf("transpiler: [%s] Mode 0 map() requires positive map dimensions", reasonMode0ResourceLimit)
	}
	const maxMode0SsetPerUpdate = 16
	directInitSset := countDirectMode0Calls(initFunction, "sset")
	directUpdateSset := countDirectMode0Calls(update, "sset")
	if ssetCalls != directInitSset+directUpdateSset {
		return 0, false, false, fmt.Errorf("transpiler: [%s] Mode 0 sset() must be an unconditional direct statement in _init or _update", reasonMode0MutableSheet)
	}
	if directUpdateSset > maxMode0SsetPerUpdate {
		return 0, false, false, fmt.Errorf("transpiler: [%s] Mode 0 permits at most %d direct sset() calls per update so dirty tile uploads fit VBlank", reasonMode0ResourceLimit, maxMode0SsetPerUpdate)
	}
	if draw == nil || clsCalls != 1 || mapCalls > 4 {
		return 0, false, false, fmt.Errorf("transpiler: [%s] Mode 0 requires one direct cls() and at most four direct map() calls in _draw", reasonMode0DrawShape)
	}
	if hidden := callGraph.firstBuiltinFromFunction("_init", "spr", "sspr", "pspr", "anim", "print", "cls", "map"); hidden != nil {
		return 0, false, false, mode0CallError(hidden, reasonMode0DrawShape, "Mode 0 cls()/map()/spr()/sspr()/pspr()/print() belong to _draw")
	}
	if hidden := callGraph.firstBuiltinFromFunction("_update", "spr", "sspr", "pspr", "anim", "print", "cls", "map"); hidden != nil {
		return 0, false, false, mode0CallError(hidden, reasonMode0DrawShape, "Mode 0 cls()/map()/spr()/sspr()/pspr()/print() belong to _draw")
	}

	ops, unbounded, err := mode0WalkDraw(chunk, draw, spriteIDs, opts)
	if err != nil {
		return 0, false, false, err
	}
	if unbounded {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] Mode 0 object loops need a proven finite trip count; unproven object count", draw.Pos(), reasonMode0ObjectBudget)
	}
	directCls, directMap, textEntryCount := 0, 0, 0
	clsIndex, mapIndex, lastMapIndex, firstSpriteIndex, firstPrintIndex := -1, -1, -1, -1, -1
	maxSpriteID := -1
	materializedMaps := 0
	for opIndex, op := range ops {
		switch op.name {
		case "cls":
			directCls++
			if clsIndex < 0 {
				clsIndex = opIndex
			}
			if err := validateMode0ClsCall(op.call); err != nil {
				return 0, false, false, err
			}
		case "map":
			directMap++
			if mapIndex < 0 {
				mapIndex = opIndex
			}
			lastMapIndex = opIndex
			if err := validateMode0MapCall(op.call); err != nil {
				return 0, false, false, err
			}
			if len(op.call.Args) == 0 {
				materializedMaps = mode0Layers
			} else if bgValue, ok := staticNumberLiteral(op.call.Args[0]); ok {
				if bg := int(bgValue); plan.drawMask&(1<<uint(bg)) != 0 {
					materializedMaps++
				}
			}
		case "spr":
			if firstSpriteIndex < 0 {
				firstSpriteIndex = opIndex
			}
			if op.lastID > maxSpriteID {
				maxSpriteID = op.lastID
			}
			if mode0Layers > 0 && materializedMaps == 0 {
				return 0, false, false, mode0CallError(op.call, reasonMode0SpriteShape,
					"Mode 0 cannot place an object behind every materialized background; draw at least one non-empty map before spr()/sspr()")
			}
		case "sspr":
			if firstSpriteIndex < 0 {
				firstSpriteIndex = opIndex
			}
			if op.lastID > maxSpriteID {
				maxSpriteID = op.lastID
			}
			if mode0Layers > 0 && materializedMaps == 0 {
				return 0, false, false, mode0CallError(op.call, reasonMode0SpriteShape,
					"Mode 0 cannot place an object behind every materialized background; draw at least one non-empty map before spr()/sspr()")
			}
		case "pspr":
			if firstSpriteIndex < 0 {
				firstSpriteIndex = opIndex
			}
			if mode0Layers > 0 && materializedMaps == 0 {
				return 0, false, false, mode0CallError(op.call, reasonMode0PsprShape,
					"Mode 0 cannot place an object behind every materialized background; draw at least one non-empty map before pspr()")
			}
		case "print":
			if firstPrintIndex < 0 {
				firstPrintIndex = opIndex
			}
			objects, err := validateMode0PrintCall(op.call, op.printKind)
			if err != nil {
				return 0, false, false, err
			}
			textEntryCount += objects * op.copies
		}
	}
	if directCls != 1 || directMap != mapCalls {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] cls() and map() must be unconditional direct statements in _draw", draw.Pos(), reasonMode0DrawShape)
	}
	if firstSpriteIndex < 0 && firstPrintIndex < 0 && (sprCalls > 0 || ssprCalls > 0 || psprCalls > 0 || printCalls > 0) {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] Mode 0 spr()/sspr()/pspr()/print() belong to _draw", draw.Pos(), reasonMode0DrawShape)
	}
	clsStmt := indexOfDirectMode0Call(draw, "cls")
	if clsStmt < 0 {
		return 0, false, false, fmt.Errorf("transpiler: [%s] Mode 0 requires one direct cls() and at most four direct map() calls in _draw", reasonMode0DrawShape)
	}
	if returned := firstMode0Return(draw.Body.Body[:clsStmt]); returned != nil {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] cls() must execute before every return from _draw", returned.Pos(), reasonMode0ClsShape)
	}
	if paletteCall := callGraph.firstBuiltinFromFunction("_draw", "pal", "palt"); paletteCall != nil {
		return 0, false, false, mode0CallError(paletteCall, reasonMode0DrawShape, "Mode 0 pal()/palt() must run before _draw (for example in _init or _update)")
	}
	if mapIndex >= 0 && clsIndex > mapIndex {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] cls() must precede map() in _draw", draw.Pos(), reasonMode0ClsShape)
	}
	if firstSpriteIndex >= 0 && firstSpriteIndex < clsIndex {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] Mode 0 spr()/sspr() calls must follow cls()", draw.Pos(), reasonMode0SpriteShape)
	}
	if firstPrintIndex >= 0 && firstPrintIndex < clsIndex {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] Mode 0 spr()/print() calls must follow cls()", draw.Pos(), reasonMode0SpriteShape)
	}
	if firstPrintIndex >= 0 && lastMapIndex >= 0 && firstPrintIndex < lastMapIndex {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] Mode 0 print() calls must follow cls() and every map() call", draw.Pos(), reasonMode0TextShape)
	}
	if err := validateMode0PsprProof(chunk, ops, opts, sprCalls, ssprCalls); err != nil {
		return 0, false, false, err
	}
	if err := validateMode0PoseCommit(chunk, ops, opts, mode0Layers, directUpdateSset); err != nil {
		return 0, false, false, err
	}
	if err := validateMode0PausePalette(chunk, ops, opts); err != nil {
		return 0, false, false, err
	}
	budget := mode0ObjectBudgetFromOps(chunk, draw, ops, unbounded, opts)
	if budget.TextTiles > mode0TextTileLimit {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] Mode 0 text needs %d physical tiles; the reserved capacity is %d (up to 32 compact or 16 tall glyph/color pairs)", draw.Pos(), reasonMode0ObjectBudget, budget.TextTiles, mode0TextTileLimit)
	}
	if budget.OAMEntries > mode0OAMEntryLimit {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] Mode 0 draw needs %d OAM entries after packing; the game limit is %d (one OAM entry is reserved for the system pause overlay)", draw.Pos(), reasonMode0ObjectBudget, budget.OAMEntries, mode0OAMEntryLimit)
	}
	if budget.MaxLineCycles > mode0OBJLineCycleLimit {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] Mode 0 worst OBJ scanline needs %d rendering cycles; the game limit is %d after reserving 64 cycles for the system pause overlay, with H-Blank Interval Free disabled", draw.Pos(), reasonMode0ObjectBudget, budget.MaxLineCycles, mode0OBJLineCycleLimit)
	}
	if textEntryCount != 0 && maxSpriteID >= runtimeapi.Mode0TextTileBase {
		return 0, false, false, fmt.Errorf("transpiler: %s: [%s] Mode 0 text reserves OBJ tiles %d-%d; spr() source IDs must stay below %d",
			draw.Pos(), reasonMode0ResourceLimit, runtimeapi.Mode0TextTileBase, runtimeapi.Mode0TextTileBase+runtimeapi.Mode0TextTileCount-1, runtimeapi.Mode0TextTileBase)
	}
	tileLimit := (30 - 2*(mode0Layers-1)) * 32
	if cols*rows > tileLimit {
		return 0, false, false, fmt.Errorf("transpiler: [%s] %d Mode 0 map layers leave %d sprite tiles, but the sheet declares %d",
			reasonMode0ResourceLimit, mode0Layers, tileLimit, cols*rows)
	}
	return mode0Layers, budget.OAMEntries != 0, textEntryCount != 0, nil
}

func validateMode0ClsCall(call *luaparser.CallExpr) error {
	if call == nil || len(call.Args) == 0 {
		return nil
	}
	if len(call.Args) != 1 {
		return mode0CallError(call, reasonMode0ClsShape, "Mode 0 cls() accepts zero or one argument")
	}
	color, literal := mode0StaticRuntimeInt(call.Args[0])
	if !literal || color < 0 || color > 255 {
		return mode0CallError(call, reasonMode0ClsShape, "Mode 0 cls() color must be a static runtime integer from 0 to 255")
	}
	return nil
}

func countDirectMode0Calls(function *luaparser.FunctionDeclStmt, name string) int {
	if function == nil {
		return 0
	}
	count := 0
	for _, stmt := range function.Body.Body {
		callStmt, ok := stmt.(*luaparser.CallStmt)
		if !ok {
			continue
		}
		key, ok := callKey(callStmt.Call)
		if ok && key == name {
			count++
		}
	}
	return count
}

func indexOfDirectMode0Call(function *luaparser.FunctionDeclStmt, name string) int {
	if function == nil {
		return -1
	}
	for index, stmt := range function.Body.Body {
		callStmt, ok := stmt.(*luaparser.CallStmt)
		if !ok {
			continue
		}
		key, ok := callKey(callStmt.Call)
		if ok && key == name {
			return index
		}
	}
	return -1
}

func validateMode0ClipCall(call *luaparser.CallExpr) error {
	if call == nil || len(call.Args) == 0 {
		return nil
	}
	if len(call.Args) != 4 {
		return mode0CallError(call, reasonMode0UnsupportedCall, "Mode 0 clip() supports only full-screen reset")
	}
	want := []float64{0, 0, 240, 160}
	for index, expected := range want {
		value, literal := staticNumberLiteral(call.Args[index])
		if !literal || value != expected {
			return mode0CallError(call, reasonMode0UnsupportedCall, "Mode 0 clip() supports only clip() or clip(0,0,240,160)")
		}
	}
	return nil
}

func validateMode0PalCall(call *luaparser.CallExpr) error {
	if call == nil || len(call.Args) == 0 || len(call.Args) >= 2 && len(call.Args) <= 3 {
		return nil
	}
	if len(call.Args) == 1 {
		layer, literal := staticNumberLiteral(call.Args[0])
		if literal && (layer == 0 || layer == 1) {
			return nil
		}
	}
	return mode0CallError(call, reasonMode0UnsupportedCall, "Mode 0 pal() supports reset, layer reset 0/1, and 2-3 argument remaps")
}

func validateMode0PaltCall(call *luaparser.CallExpr) error {
	if call == nil || len(call.Args) == 0 {
		return nil
	}
	if len(call.Args) != 2 {
		return mode0CallError(call, reasonMode0UnsupportedCall, "Mode 0 palt() supports only default-transparency no-op forms")
	}
	color, literal := staticNumberLiteral(call.Args[0])
	transparent, boolean := unparenExpr(call.Args[1]).(*luaparser.BoolExpr)
	if !literal || color != math.Trunc(color) || color < 0 || color > 255 || !boolean ||
		(color == 0) != transparent.Value {
		return mode0CallError(call, reasonMode0UnsupportedCall, "Mode 0 palt() requires palt(0,true) or palt(nonzero,false)")
	}
	return nil
}

func validateMode0SprCall(call *luaparser.CallExpr, cols, rows int) (int, int, error) {
	if call == nil || len(call.Args) < 3 || len(call.Args) > 7 {
		return 0, 0, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 spr() requires 3 to 7 arguments")
	}
	readPositive := func(index int, fallback int, name string) (int, error) {
		if len(call.Args) <= index {
			return fallback, nil
		}
		value, literal := staticNumberLiteral(call.Args[index])
		if !literal || value != math.Trunc(value) || value < 1 || value > 32 {
			return 0, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 spr() "+name+" must be an integer literal from 1 to 32")
		}
		return int(value), nil
	}
	idValue, literal := staticNumberLiteral(call.Args[0])
	if !literal || idValue != math.Trunc(idValue) || idValue < 0 || idValue >= float64(cols*rows) {
		return 0, 0, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 spr() id must be a valid integer literal")
	}
	w, err := readPositive(3, 1, "width")
	if err != nil {
		return 0, 0, err
	}
	h, err := readPositive(4, 1, "height")
	if err != nil {
		return 0, 0, err
	}
	id := int(idValue)
	cellX, cellY := id%cols, id/cols
	lastID := id + (h-1)*cols + w - 1
	if cellX+w > cols || cellY+h > rows || lastID >= runtimeapi.Mode0ObjectTileLimit {
		return 0, 0, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 spr() source rectangle must stay inside the sheet and OBJ tile IDs 0-479 (480-511 are reserved for the system pause overlay)")
	}
	return w * h, lastID, nil
}

func validateMode0SprCallWithEnv(call *luaparser.CallExpr, cols, rows int, analysis *mode0SpriteIDAnalysis, eval *mode0NumEval, env mode0NumEnv) (int, int, int, error) {
	if call == nil || len(call.Args) < 3 || len(call.Args) > 7 {
		return 0, 0, 0, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 spr() requires 3 to 7 arguments")
	}
	if _, literal := staticNumberLiteral(call.Args[0]); literal {
		_, lastID, err := validateMode0SprCall(call, cols, rows)
		if err != nil {
			return 0, 0, 0, err
		}
		w, h := mode0SprCallCells(call)
		return lastID, w, h, nil
	}
	var ids []int
	var proven bool
	var detail string
	if eval != nil {
		ids, proven, detail = eval.spriteIDs(call.Args[0], env)
	}
	if !proven && analysis != nil {
		ids, proven, detail = analysis.finiteIDs(call.Args[0])
	}
	if !proven || len(ids) == 0 {
		message := "Mode 0 spr() id must be an integer literal or a finite integer set"
		if detail != "" {
			message += ": " + detail
		}
		return 0, 0, 0, mode0CallError(call, reasonMode0SpriteShape, message)
	}
	readPositive := func(index int, fallback int, name string) ([]int, error) {
		if len(call.Args) <= index {
			return []int{fallback}, nil
		}
		if value, literal := staticNumberLiteral(call.Args[index]); literal {
			if value != math.Trunc(value) || value < 1 || value > 32 {
				return nil, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 spr() "+name+" must be an integer literal from 1 to 32")
			}
			return []int{int(value)}, nil
		}
		if eval != nil {
			if values, ok := eval.nums(call.Args[index], env); ok {
				ints, ok := mode0IntsFromNums(values)
				if ok && len(ints) > 0 {
					for _, size := range ints {
						if size < 1 || size > 32 {
							return nil, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 spr() "+name+" must be an integer from 1 to 32")
						}
					}
					return ints, nil
				}
			}
		}
		return nil, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 spr() "+name+" must be an integer literal from 1 to 32")
	}
	widths, err := readPositive(3, 1, "width")
	if err != nil {
		return 0, 0, 0, err
	}
	heights, err := readPositive(4, 1, "height")
	if err != nil {
		return 0, 0, 0, err
	}
	maxLastID := -1
	maxW, maxH := 1, 1
	for _, id := range ids {
		for _, w := range widths {
			if w > maxW {
				maxW = w
			}
			for _, h := range heights {
				if h > maxH {
					maxH = h
				}
				if id < 0 || id >= cols*rows {
					return 0, 0, 0, mode0CallError(call, reasonMode0SpriteShape,
						fmt.Sprintf("Mode 0 spr() finite id set contains %d, outside the declared sheet", id))
				}
				cellX, cellY := id%cols, id/cols
				lastID := id + (h-1)*cols + w - 1
				if cellX+w > cols || cellY+h > rows || lastID >= runtimeapi.Mode0ObjectTileLimit {
					return 0, 0, 0, mode0CallError(call, reasonMode0SpriteShape,
						fmt.Sprintf("Mode 0 spr() finite id %d has a source rectangle outside the sheet or OBJ tile IDs 0-479 (480-511 are reserved for the system pause overlay)", id))
				}
				if lastID > maxLastID {
					maxLastID = lastID
				}
			}
		}
	}
	return maxLastID, maxW, maxH, nil
}

func mode0ProvenInt(expr luaparser.Expr, eval *mode0NumEval, env mode0NumEnv) (int, bool) {
	if value, literal := staticNumberLiteral(expr); literal {
		if value != math.Trunc(value) {
			return 0, false
		}
		return int(value), true
	}
	if eval == nil {
		return 0, false
	}
	values, ok := eval.nums(expr, env)
	if !ok {
		return 0, false
	}
	ints, ok := mode0IntsFromNums(values)
	if !ok || len(ints) != 1 {
		return 0, false
	}
	return ints[0], true
}

func validateMode0SSprCall(call *luaparser.CallExpr, sheetW, sheetH, cols int) (int, int, error) {
	cells, lastID, _, _, err := validateMode0SSprCallWithEnv(call, sheetW, sheetH, cols, nil, nil)
	return cells, lastID, err
}

func validateMode0SSprCallWithEnv(call *luaparser.CallExpr, sheetW, sheetH, cols int, eval *mode0NumEval, env mode0NumEnv) (int, int, int, int, error) {
	if call == nil || len(call.Args) < 6 || len(call.Args) > 10 {
		return 0, 0, 0, 0, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 sspr() requires 6 to 10 arguments")
	}
	readInt := func(index int, name string) (int, error) {
		value, ok := mode0ProvenInt(call.Args[index], eval, env)
		if !ok {
			return 0, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 sspr() "+name+" must be a proven integer")
		}
		return value, nil
	}
	sx, err := readInt(0, "sx")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	sy, err := readInt(1, "sy")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	sw, err := readInt(2, "sw")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	sh, err := readInt(3, "sh")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if sx < 0 || sy < 0 || sx&7 != 0 || sy&7 != 0 || sw <= 0 || sh <= 0 || sw&7 != 0 || sh&7 != 0 || sx+sw > sheetW || sy+sh > sheetH {
		return 0, 0, 0, 0, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 sspr() source must be an in-bounds, 8x8-aligned rectangle")
	}
	dw, dh := sw, sh
	if len(call.Args) > 6 {
		dw, err = readInt(6, "dw")
		if err != nil {
			return 0, 0, 0, 0, err
		}
	}
	if len(call.Args) > 7 {
		dh, err = readInt(7, "dh")
		if err != nil {
			return 0, 0, 0, 0, err
		}
	}
	if dw != sw || dh != sh {
		return 0, 0, 0, 0, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 sspr() does not scale; dw/dh must equal sw/sh")
	}
	w, h := sw/8, sh/8
	id := (sy/8)*cols + sx/8
	lastID := id + (h-1)*cols + w - 1
	if lastID >= runtimeapi.Mode0ObjectTileLimit {
		return 0, 0, 0, 0, mode0CallError(call, reasonMode0SpriteShape, "Mode 0 sspr() source must stay within OBJ tile IDs 0-479 (480-511 are reserved for the system pause overlay)")
	}
	return w * h, lastID, w, h, nil
}

func validateMode0PrintCall(call *luaparser.CallExpr, kind mode0PrintTextKind) (int, error) {
	if call == nil || len(call.Args) != 4 {
		return 0, mode0CallError(call, reasonMode0TextShape, "Mode 0 print() requires explicit text, x, y, and color arguments")
	}
	if kind == mode0PrintTextInvalid {
		return 0, mode0CallError(call, reasonMode0TextShape, "Mode 0 print() text must be a static printable-ASCII string or a proven decimal number; concat, tostr, chr, sub, and control codes are not eligible")
	}
	color, literal := staticNumberLiteral(call.Args[3])
	if !literal || color != math.Trunc(color) || color < 0 || color > 255 {
		return 0, mode0CallError(call, reasonMode0TextShape, "Mode 0 print() color must be an integer literal from 0 to 255")
	}
	entries, ok := mode0PrintOAMEntries(call, kind)
	if !ok {
		return 0, mode0CallError(call, reasonMode0TextShape, "Mode 0 print() text must be a static printable-ASCII string or a proven decimal number; concat, tostr, chr, sub, and control codes are not eligible")
	}
	if entries != 0 && color == 0 {
		return 0, mode0CallError(call, reasonMode0TextShape, "Mode 0 non-empty print() color must be an integer literal from 1 to 255 because OBJ palette index 0 is transparent")
	}
	return entries, nil
}

func firstMode0Return(statements []luaparser.Stmt) *luaparser.ReturnStmt {
	for _, statement := range statements {
		switch stmt := statement.(type) {
		case *luaparser.ReturnStmt:
			return stmt
		case *luaparser.IfStmt:
			if returned := firstMode0Return(stmt.Then.Body); returned != nil {
				return returned
			}
			for _, branch := range stmt.ElseIf {
				if returned := firstMode0Return(branch.Body.Body); returned != nil {
					return returned
				}
			}
			if stmt.Else != nil {
				if returned := firstMode0Return(stmt.Else.Body); returned != nil {
					return returned
				}
			}
		case *luaparser.WhileStmt:
			if returned := firstMode0Return(stmt.Body.Body); returned != nil {
				return returned
			}
		case *luaparser.RepeatStmt:
			if returned := firstMode0Return(stmt.Body.Body); returned != nil {
				return returned
			}
		case *luaparser.DoStmt:
			if returned := firstMode0Return(stmt.Body.Body); returned != nil {
				return returned
			}
		case *luaparser.NumericForStmt:
			if returned := firstMode0Return(stmt.Body.Body); returned != nil {
				return returned
			}
		case *luaparser.GenericForStmt:
			if returned := firstMode0Return(stmt.Body.Body); returned != nil {
				return returned
			}
		}
	}
	return nil
}

func validateMode0MapCall(call *luaparser.CallExpr) error {
	if call == nil || len(call.Args) > 8 {
		return mode0CallError(call, reasonMode0MapShape, "Mode 0 map() accepts at most eight arguments")
	}
	if len(call.Args) == 0 {
		return nil
	}
	limits := []struct {
		index int
		name  string
		max   float64
	}{{5, "cells_w", 64}, {6, "cells_h", 32}}
	for _, limit := range limits {
		if len(call.Args) <= limit.index {
			continue
		}
		value, literal := staticNumberLiteral(call.Args[limit.index])
		if !literal || value != math.Trunc(value) || value > limit.max {
			return mode0CallError(call, reasonMode0MapShape,
				fmt.Sprintf("Mode 0 map() %s must be an integer literal no greater than %.0f", limit.name, limit.max))
		}
	}
	return nil
}

func mode0CallError(call *luaparser.CallExpr, reason, detail string) error {
	if call == nil {
		return fmt.Errorf("transpiler: [%s] %s", reason, detail)
	}
	return fmt.Errorf("transpiler: %s: [%s] %s", call.Pos(), reason, detail)
}
