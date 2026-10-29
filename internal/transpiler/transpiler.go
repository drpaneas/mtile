// Package transpiler turns the Mtile Lua AST into a self-contained Go
// main package for the mtile runtime.
//
// User state lives in Game and the generated main owns the GBA presentation
// loop. Proven monomorphic numbers use runtime.Number; all other Lua values
// retain runtime.Value. Keeping those decisions here makes the runtime
// independent of the generated package (and avoids an import cycle).
package transpiler

import (
	"errors"
	"fmt"
	"slices"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

// GlobalLimit is the maximum number of distinct persistent Lua names kept in
// the generated Game state. The runtime root set has the same fixed capacity.
const GlobalLimit = 256

// GlobalSymbol records the first source occurrence which caused a persistent
// Lua name to be classified as global.
type GlobalSymbol struct {
	Name     string
	Position luaparser.Position
}

// GlobalReport is the project-wide persistent-state budget seen by the
// transpiler. Symbols are deterministic and ordered by first occurrence.
type GlobalReport struct {
	Count   int
	Limit   int
	Symbols []GlobalSymbol
}

// GlobalLimitError preserves the full report when source exceeds GlobalLimit.
type GlobalLimitError struct {
	Report GlobalReport
}

func (e *GlobalLimitError) Error() string {
	detail := fmt.Sprintf("%d global Value fields exceed runtime maxGlobals (%d)", e.Report.Count, e.Report.Limit)
	if e.Report.Limit >= 0 && e.Report.Limit < len(e.Report.Symbols) {
		symbol := e.Report.Symbols[e.Report.Limit]
		if symbol.Position.Line != 0 {
			return fmt.Sprintf("transpiler: %s: %s; first excess global %q", symbol.Position, detail, symbol.Name)
		}
	}
	return "transpiler: " + detail
}

// Options controls the generated cartridge shell. When assets are omitted, a
// valid 8x8 empty sheet is emitted. The build pipeline supplies the same
// fields with real ROM data.
type Options struct {
	// Font is immutable build-selected 6x10 artwork; nil uses the ASCII-only Arcade B preset.
	Font        *runtimeapi.Font
	PackageName string
	// IWRAMFunctions selects plain top-level Lua helpers for fast internal RAM.
	// Empty preserves normal code placement. The target linker enforces capacity.
	IWRAMFunctions []string
	// DisableStableTableFields retains the fully dynamic table.field lowering.
	// By default each static field access checks and resolves its table slot on
	// first execution, then assumes that the table type/layout stays stable.
	DisableStableTableFields bool
	// DisableMonomorphicNumbers keeps every Lua number in rt.Value storage.
	// The default lowers first-observed numeric globals, locals, loop counters,
	// arithmetic, and stable table fields to native rt.Number operations.
	DisableMonomorphicNumbers bool
	// DisableMonomorphicFunctions retains rt.Value parameters and returns for
	// every user function. It is the rollback/A-B path for numeric signatures.
	DisableMonomorphicFunctions bool
	// DisableMonomorphicMultiReturns keeps every result of a multi-return user
	// function as rt.Value while retaining parameter and single-return
	// specialization. It is the narrow rollback/A-B path for tuple signatures.
	DisableMonomorphicMultiReturns bool
	// DisableNoAllocFunctionCalls retains GC-root wrappers around every user
	// call. By default transitively proven non-allocating callees are direct.
	DisableNoAllocFunctionCalls bool
	// DisableNumericArrays retains generic rt.Value table slots for fixed,
	// non-escaping global numeric arrays. The default stores their elements as
	// compact native Numbers; this flag is the rollback/A-B path.
	DisableNumericArrays bool
	// DisableHeapElision retains the periodic collector even when the complete
	// program is proven unable to allocate. It is the rollback/A-B path.
	DisableHeapElision bool
	// DisableNumericConstants retains storage and initialization for otherwise
	// immutable chunk-scope numeric bindings. It is the rollback/A-B path for
	// compile-time constant propagation.
	DisableNumericConstants bool
	// DisableLiteralPrintSpecialization retains the generic root-protected
	// print path even when the first argument is a static string literal.
	DisableLiteralPrintSpecialization bool
	// DisableFixedMapLayerSpecialization retains logical-layer lookup for
	// compact maps even when the layer is a proven compile-time value.
	DisableFixedMapLayerSpecialization bool
	// DisableMapRasterCache keeps the per-cell Mode 4 map path for A/B checks.
	DisableMapRasterCache bool
	// DisableConditionSpecialization retains value-producing logical temporaries
	// in truth-only branches for paired code-generation and target checks.
	DisableConditionSpecialization bool
	// DisableStableWriteSpecialization retains caller-side variadic root
	// construction for already-cached table stores in paired target checks.
	DisableStableWriteSpecialization bool
	// DisableStableReadSpecialization retains full table/key Values on warm
	// numeric-field reads for paired target and correctness checks.
	DisableStableReadSpecialization bool
	// DisableConstantDivision retains generic fixed-point division by +/-1.5
	// for paired code-generation and target checks.
	DisableConstantDivision bool
	// DisableTypedDrawCalls retains the Value ABI for numeric spr, camera,
	// and map calls. It is the narrow rollback/A-B path for typed draw calls.
	DisableTypedDrawCalls bool
	// DisableNumericLengths retains boxed lengths of ordinary tables/strings.
	// The runtime's # result is always numeric, even for unsupported operands.
	DisableNumericLengths bool
	// DisableIteratorLengthHoisting rescans the array prefix on every all()
	// iteration, including loops proven not to mutate any array slots.
	DisableIteratorLengthHoisting bool
	// DisableStaticTableLiterals retains generic hash insertion for all
	// constructor fields. It is the rollback/A-B path for literal slot plans.
	DisableStaticTableLiterals bool
	// DisableTableLiteralViews retains per-field validation even when a fresh
	// static constructor cannot allocate while evaluating any of its values.
	DisableTableLiteralViews bool
	// DisableTrustedTableLiteralWrites retains the guarded per-field view
	// method even for a fresh complete non-nil static constructor.
	DisableTrustedTableLiteralWrites bool
	// DisableTableLiteralImages retains per-field insertion instead of copying
	// a ROM image of a trusted constructor's keys and constant values.
	DisableTableLiteralImages bool
	// DisableLoopTableReservations retains incremental array growth for fresh
	// tables immediately filled by a proved fixed-count append loop.
	DisableLoopTableReservations bool
	// DisableKnownTableAppends retains dense-prefix scans in the exact fresh
	// append loops whose final array capacities have already been reserved.
	DisableKnownTableAppends bool
	// DisableCoercedNumberParams retains Value parameters even when every
	// use in a non-allocating function explicitly coerces them to numbers.
	DisableCoercedNumberParams bool
	// DisableNumberArrayViews retains validation for every numeric-array
	// element access in proven non-allocating numeric loops.
	DisableNumberArrayViews bool
	// DisableMapLayerViews retains per-cell cart/plane validation in proven
	// non-allocating loops over literal resident map layers.
	DisableMapLayerViews bool
	// DisableIntegerNumericFor retains 16.16 induction for compile-time
	// integer numeric loops. The default emits a native Go integer for.
	DisableIntegerNumericFor bool
	// Renderer is selected at build time. Its zero value deliberately preserves
	// the shipping Mode 4 backend for every existing caller.
	Renderer  Renderer
	SheetW    int
	SheetH    int
	MapW      int
	MapH      int
	MapLayers int
	// MapPlaneMask and MapDrawMask are derived from the validated Lua/map
	// contract. They keep logical BG IDs stable while generated storage and
	// Mode 0 screen shadows are packed densely. Callers should leave them zero.
	MapPlaneMask uint8
	MapDrawMask  uint8
	// MapStreaming is normally derived from MapW*MapH. It is exposed so the
	// build path can make the selected storage mode visible to callers/tests.
	MapStreaming bool
	Sheet        []byte
	Map          []uint16
	Flags        []byte
	// PoseIndex/PoseParts supply pspr's multi-part sprite tables. Poses are
	// read-only cart metadata with no per-cart mutation path, so both stay
	// in .rodata: there is no matching EWRAM budget entry for them.
	PoseVariantBaseCount int
	PoseVariantNames     []string
	// PoseNames maps literal pspr names to canonical indices at build time.
	// Names never enter the cartridge's runtime pose records.
	PoseNames       []string
	PoseIndex       []runtimeapi.PoseRange
	AnimationClips  []runtimeapi.AnimationClip
	AnimationFrames []uint16
	PoseParts       []runtimeapi.PosePart
	// TileLinks draw linked shared tiles as poses or animation clips inside
	// map(); see runtime/tilelink.go.
	TileLinks []runtimeapi.TileLink
	// ObjSheet is the optional dense object warehouse. ObjSheetW == 0 means
	// poses index the spritesheet. Generated carts always compact this into
	// read-only ROM; there is no EWRAM copy.
	ObjSheetW int
	ObjSheetH int
	ObjSheet  []byte
	// Palette is the expanded 256-entry BGR555 palette for native projects.
	// An omitted palette preserves the runtime's built-in palette behavior for
	// the earlier transpiler tests and small hand-written cartridges.
	Palette []uint16

	// NativeAudio supplies the four-channel PSG asset contract.
	NativeAudio *assetsync.NativeAudioAssets
	// Debug emits the fixed preview mailbox and runtime instrumentation. It is
	// off for normal CLI/release cartridge builds.
	Debug bool
	// Mode0Layers is populated by SelectRenderer and is consumed by the build
	// service as a link-time allocation decision. Callers should leave it zero.
	Mode0Layers  int
	Mode0Objects bool
	Mode0Text    bool
}

// BuiltinSpec is the public mapping table used by the generator and by the
// CLI/editor validation layer. MinArgs and MaxArgs describe the effective Lua
// argument list after a bare trailing call expands. Results is the number of
// values this builtin contributes when it is that trailing call. Omitted
// arguments are emitted as zero/nil Value defaults.
type BuiltinSpec struct {
	GoName  string
	MinArgs int
	MaxArgs int
	Results int
}

// Builtins is the Lua-to-runtime mapping table. Functions that are iterator
// syntax (all/pairs) are included so callers can validate them consistently;
// their ordinary expression form is rejected by the generator. Renderer
// compatibility is an independent, exhaustive contract in
// BuiltinRendererCompatibility; adding a builtin requires classifying it there
// and documenting its Mode 0/Mode 4 semantics in docs/render-modes.md.
var Builtins = map[string]BuiltinSpec{
	// Progress callbacks only: the mandatory system overlay cannot be customized.
	"pause_menu": {"ConfigurePauseMenu", 2, 2, 0},
	"save_get":   {"SaveGetV", 1, 1, 1}, "save_set": {"SaveSetV", 2, 2, 1},
	"save_read": {"SaveRead", 1, 1, 1}, "save_write": {"SaveWrite", 1, 1, 1},
	"cls": {"ClsV", 0, 1, 0}, "pset": {"PsetV", 2, 3, 0}, "pget": {"PgetV", 2, 2, 1},
	"line": {"LineV", 4, 5, 0}, "rect": {"RectV", 4, 5, 0}, "rectfill": {"RectfillV", 4, 5, 0},
	"circ": {"CircV", 3, 4, 0}, "circfill": {"CircfillV", 3, 4, 0},
	"spr": {"SprV", 3, 7, 0}, "sspr": {"SSprV", 6, 10, 0},
	// Flips automatically preserve the pose bounds, or the union bounds of a
	// named clip; callers keep the same x/y anchor in either facing direction.
	"pvariant": {"PVariantV", 0, 1, 0},
	// pspr accepts a numeric ID or a literal pose name resolved before renderer
	// selection; the runtime ABI and numeric no-op semantics stay unchanged.
	"pspr": {"PSprV", 3, 5, 0}, "anim": {"AnimV", 3, 6, 0},
	"map":    {"MapBGAtV", 0, 8, 0},
	"camera": {"CameraV", 0, 2, 0}, "clip": {"ClipV", 0, 4, 0}, "pal": {"PalV", 0, 3, 0},
	"palt": {"PaltV", 0, 2, 0}, "color": {"ColorV", 0, 1, 0}, "cursor": {"CursorV", 0, 3, 0},
	// UTF-8 print uses one selected 6x10 font: x advances 6, newline/wrap 10.
	// Arcade ASCII is the default; extended language glyphs require selection.
	// Cursor/return values remain pixel coordinates; off-screen ink is clipped.
	"print": {"PrintV", 1, 4, 1},
	"sget":  {"Sget", 2, 2, 1}, "sset": {"Sset", 3, 3, 0}, "fget": {"Fget", 1, 2, 1},
	"fset": {"Fset", 2, 3, 0}, "mget": {"Mget", 2, 3, 1},
	// Linked maps require provably unlinked writes in Mode 0; expanded Lua
	// arguments retain their normal semantics in Mode 4 and unlinked maps.
	"mset": {"Mset", 3, 4, 0},
	"btn":  {"BtnV", 1, 2, 1}, "btnp": {"BtnpV", 1, 2, 1},
	"flr": {"Flr", 1, 1, 1}, "ceil": {"Ceil", 1, 1, 1}, "abs": {"Abs", 1, 1, 1},
	"sgn": {"Sgn", 1, 1, 1}, "min": {"Min", 2, 2, 1}, "max": {"Max", 2, 2, 1},
	"mid": {"Mid", 3, 3, 1}, "sin": {"Sin", 1, 1, 1}, "cos": {"Cos", 1, 1, 1},
	"atan2": {"Atan2", 2, 2, 1}, "sqrt": {"Sqrt", 1, 1, 1}, "rnd": {"RndV", 0, 1, 1},
	"srand": {"SrandV", 1, 1, 0}, "band": {"Band", 2, 2, 1}, "bor": {"Bor", 2, 2, 1},
	"bxor": {"Bxor", 2, 2, 1}, "bnot": {"Bnot", 1, 1, 1}, "shl": {"Shl", 2, 2, 1},
	"shr": {"Shr", 2, 2, 1}, "lshr": {"Lshr", 2, 2, 1}, "rotl": {"Rotl", 2, 2, 1},
	"rotr": {"Rotr", 2, 2, 1},
	"add":  {"TableAdd", 2, 3, 1}, "del": {"TableDel", 2, 2, 1}, "deli": {"TableDeli", 1, 2, 1},
	"count": {"TableCount", 1, 2, 1}, "all": {"all", 1, 1, 1}, "pairs": {"pairs", 1, 1, 1}, "foreach": {"foreach", 2, 2, 0},
	"sub": {"Sub", 2, 3, 1}, "tostr": {"Tostr", 1, 2, 1}, "tonum": {"Tonum", 1, 1, 1},
	"chr": {"Chr", 1, 1, 1}, "ord": {"Ord", 1, 2, 1},
	"t": {"T", 0, 0, 1}, "time": {"Time", 0, 0, 1}, "stat": {"StatV", 1, 1, 1}, "sfx": {"SFXV", 1, 4, 0},
	"music": {"MusicV", 0, 2, 0}, "sample": {"SampleV", 1, 3, 0},
	"gba_note": {"GBANoteV", 3, 4, 0}, "gba_stop": {"GBAStopV", 1, 1, 0}, "printh": {"noop", 1, 2, 0},
}

// Transpile parses source and returns generated Go source. Existing callers
// keep the Mode 4 default; callers that need selection diagnostics use
// TranspileWithReport.
func Transpile(sourceName, source string, opts Options) (string, error) {
	generated, _, err := TranspileWithReport(sourceName, source, opts)
	return generated, err
}

// TranspileWithReport parses source, performs conservative renderer selection,
// and returns the stable decision report alongside generated Go.
func TranspileWithReport(sourceName, source string, opts Options) (string, RendererReport, error) {
	chunk, err := luaparser.ParseString(sourceName, source)
	if err != nil {
		return "", RendererReport{}, err
	}
	if opts.MapLayers == 0 {
		opts.MapLayers = 1
	}
	report, err := SelectRenderer(chunk, opts)
	if err != nil {
		return "", report, err
	}
	opts.Renderer = report.Selected
	opts.Mode0Layers = report.Mode0Layers
	opts.Mode0Objects = report.Mode0Objects
	opts.Mode0Text = report.Mode0Text
	opts.MapPlaneMask = report.MapPlaneMask
	opts.MapDrawMask = report.MapDrawMask
	report.NativeAudioLinked = opts.NativeAudio != nil && chunkUsesNativeAudio(chunk)
	if planeCells, ok := checkedProduct(opts.MapW, opts.MapH); ok {
		if retainedCells, ok := checkedProduct(planeCells, mapMaskCount(report.MapPlaneMask)); ok {
			report.MapStreaming = retainedCells > assetsync.MapStreamingThresholdCells
		}
	}
	generated, metadata, err := generateWithMetadata(chunk, opts)
	report.HeapLinked = metadata.heapLinked
	report.SheetReadOnly = metadata.sheetReadOnly
	report.MapRasterDisabled = metadata.mapRasterDisabled
	report.StaticStrings = metadata.staticStrings
	report.Mode0TextKeys = metadata.mode0TextKeys
	return generated, report, err
}

// AnalyzeGlobals parses source and reports the exact distinct persistent names
// used by code generation without requiring assets, renderer selection, or a
// TinyGo toolchain. It is used by the editor for project-wide source feedback.
func AnalyzeGlobals(sourceName, source string) (GlobalReport, error) {
	chunk, err := luaparser.ParseString(sourceName, source)
	if err != nil {
		return GlobalReport{Limit: GlobalLimit}, err
	}
	return AnalyzeChunkGlobals(chunk)
}

// AnalyzeChunkGlobals is the parsed-AST form of AnalyzeGlobals.
func AnalyzeChunkGlobals(chunk *luaparser.Chunk) (GlobalReport, error) {
	if chunk == nil {
		return GlobalReport{Limit: GlobalLimit}, errors.New("transpiler: nil chunk")
	}
	g := newGenerator(chunk, Options{})
	if err := g.discoverFunctions(); err != nil {
		return g.globalReport(), err
	}
	if err := g.validateSemantics(); err != nil {
		return g.globalReport(), err
	}
	if err := g.resolveReturnArities(); err != nil {
		return g.globalReport(), err
	}
	if err := g.validateBuiltinCallArities(); err != nil {
		return g.globalReport(), err
	}
	g.discoverGlobals()
	report := g.globalReport()
	if report.Count > report.Limit {
		return report, &GlobalLimitError{Report: report}
	}
	return report, nil
}

// Generate emits a complete package main from a parsed chunk.
func Generate(chunk *luaparser.Chunk, opts Options) (string, error) {
	source, _, err := generateWithMetadata(chunk, opts)
	return source, err
}

type generationMetadata struct {
	heapLinked        bool
	sheetReadOnly     bool
	mapRasterDisabled bool
	staticStrings     int
	mode0TextKeys     []uint16
}

func generateWithMetadata(chunk *luaparser.Chunk, opts Options) (string, generationMetadata, error) {
	if opts.Font != nil {
		var c runtimeapi.Context
		if !c.SetFont(opts.Font) {
			return "", generationMetadata{}, errors.New("transpiler: invalid project font")
		}
	}
	if chunk == nil {
		return "", generationMetadata{}, errors.New("transpiler: nil chunk")
	}
	_, restore, err := lowerPoseNames(chunk, opts)
	defer restore()
	if err != nil {
		return "", generationMetadata{}, err
	}
	if opts.MapLayers == 0 {
		opts.MapLayers = 1
	}
	if opts.MapLayers < 1 || opts.MapLayers > assetsync.MapLayerCount {
		return "", generationMetadata{}, fmt.Errorf("transpiler: map layer count %d is outside 1..%d", opts.MapLayers, assetsync.MapLayerCount)
	}
	// Do not let direct AST callers bypass the forced Mode 0 proof. Mode 4 is
	// the zero/default and needs no eligibility scan; every other preference is
	// normalized through the same conservative selector used by Build.
	if opts.Renderer != RendererMode4 {
		report, err := SelectRenderer(chunk, opts)
		if err != nil {
			return "", generationMetadata{}, err
		}
		opts.Renderer = report.Selected
		opts.Mode0Layers = report.Mode0Layers
		opts.Mode0Objects = report.Mode0Objects
		opts.Mode0Text = report.Mode0Text
		opts.MapPlaneMask = report.MapPlaneMask
		opts.MapDrawMask = report.MapDrawMask
	} else {
		plan, err := validateMapContract(chunk, opts)
		if err != nil {
			return "", generationMetadata{}, err
		}
		opts.MapPlaneMask = plan.storageMask
		opts.MapDrawMask = plan.drawMask
	}
	restorePause, pauseCalls, pauseErr := lowerPauseAnalysisRoots(chunk)
	defer restorePause()
	if pauseErr != nil {
		return "", generationMetadata{}, pauseErr
	}
	g := newGenerator(chunk, opts)
	g.pauseAnalysisCalls = pauseCalls
	if err := g.prepare(); err != nil {
		return "", generationMetadata{}, err
	}
	source := g.render()
	if g.err != nil {
		return "", generationMetadata{}, g.err
	}
	if err := g.validateTransitiveRootBudget(); err != nil {
		return "", generationMetadata{}, err
	}
	return source, generationMetadata{heapLinked: g.heapEnabled, sheetReadOnly: !g.sheetMutable, mapRasterDisabled: !g.mapRasterEligible(), staticStrings: len(g.statics), mode0TextKeys: g.mode0TextKeys}, nil
}

func newGenerator(chunk *luaparser.Chunk, opts Options) *generator {
	return &generator{
		chunk:             chunk,
		opts:              opts,
		funcs:             make(map[string]*functionInfo),
		globalGo:          make(map[string]string),
		globalKinds:       make(map[string]monoKind),
		globalOrigins:     make(map[string]luaparser.Position),
		staticGo:          make(map[string]string),
		fieldCaches:       make(map[*luaparser.FieldExpr]int),
		numericFields:     make(map[string]bool),
		numericArrays:     make(map[string]*numericArrayInfo),
		numericRecords:    make(map[string]*numericRecordInfo),
		numericRecordKeys: make(map[*luaparser.StringExpr]bool),
		numericConstants:  make(map[string]*numericConstantInfo),
	}
}

type functionInfo struct {
	decl                *luaparser.FunctionDeclStmt
	key                 string
	goName              string
	returnN             int
	returnKinds         []monoKind
	params              []string
	paramKinds          []monoKind
	coercedNumberParams []bool
	noAlloc             bool
	rootPeak            int
}

type rootCallEdge struct {
	caller *functionInfo
	callee *functionInfo
	held   int
	pos    luaparser.Position
}

type numericArrayInfo struct {
	declaration *luaparser.LocalAssignStmt
	literal     *luaparser.TableExpr
	length      int
}

type numericRecordFieldInfo struct {
	name  string
	value luaparser.Expr
	index int
}

type numericRecordInfo struct {
	declaration luaparser.Stmt
	literal     *luaparser.TableExpr
	index       int
	fields      map[string]*numericRecordFieldInfo
	fieldOrder  []*numericRecordFieldInfo
}

type numericConstantInfo struct {
	declaration luaparser.Stmt
	value       int32
	order       int
}

type monoKind uint8

const (
	monoValue monoKind = iota
	monoNumber
)

func (fn *functionInfo) returnKind(index int) monoKind {
	if fn == nil || index < 0 || index >= len(fn.returnKinds) {
		return monoValue
	}
	return fn.returnKinds[index]
}

// localEntry binds one Lua name to the specific Go variable emitted for one
// `local` declaration, for-loop variable, or parameter occurrence.
type localEntry struct {
	name  string
	goVar string
	kind  monoKind
}

// localScope is one lexical block's bindings (function body, if/while/for/do
// body, ...). Lookups scan scopes innermost-first and, within a scope,
// most-recently-declared-first, so re-declaring `local x` (even within the
// same block, which Lua allows) shadows the earlier one without discarding
// its Go variable, which must stay reachable for GC-root purposes.
type localScope struct {
	entries []localEntry
}

type generator struct {
	pauseAnalysisCalls map[luaparser.Stmt]bool
	chunk              *luaparser.Chunk
	opts               Options

	funcs    map[string]*functionInfo
	funcList []*functionInfo

	globals       []string
	globalGo      map[string]string
	globalKinds   map[string]monoKind
	globalOrigins map[string]luaparser.Position

	statics  []string
	staticGo map[string]string

	fieldCaches          map[*luaparser.FieldExpr]int
	fieldCacheN          int
	numericFields        map[string]bool
	numericArrays        map[string]*numericArrayInfo
	borrowedNumberArrays map[string]string
	borrowedMapLayers    map[int]string
	literalArrayCaps     map[*luaparser.TableExpr]int
	reservedAppendLoops  map[*luaparser.NumericForStmt]*luaparser.CallExpr
	knownAppendPositions map[*luaparser.CallExpr]string
	numericRecords       map[string]*numericRecordInfo
	numericRecordOrder   []*numericRecordInfo
	numericRecordKeys    map[*luaparser.StringExpr]bool
	numericConstants     map[string]*numericConstantInfo

	current          *functionInfo
	seq              int
	localSeq         int
	scopes           []*localScope
	err              error
	extraRoots       []string
	activeRootCounts []string
	activeRootSlots  int
	topRootPeak      int
	rootCalls        []rootCallEdge
	pending          []string
	spriteN          int
	sheetMutable     bool
	sheetCompact     []byte
	sheetIndex       []uint16
	sheetSolid       []uint16
	objSpriteN       int
	objSheetCompact  []byte
	objSheetIndex    []uint32
	psprPlan         map[*luaparser.CallExpr]mode0PsprAllocation
	// tileLinkPlan holds each map() call's Mode 0 linked-object allocation.
	tileLinkPlan    map[*luaparser.CallExpr]mode0PsprAllocation
	poseOBJIndex    []runtimeapi.PoseOBJRange
	poseOBJParts    []runtimeapi.PoseOBJ
	poseOBJTiles    []uint16
	poseOBJResident *mode0ResidentPosePlan
	packedMap       [4]packedMapPlane
	mapCompact      []uint16
	mode0TextKeys   []uint16
	mode0TextTiles  []uint32

	nativeAudioEnabled bool
	nativeAudioDecl    string
	tableImages        []generatedTableImage
	heapEnabled        bool
	inputEnabled       bool
	timeEnabled        bool
	paletteIsDefault   bool
}

func (g *generator) pushScope() { g.scopes = append(g.scopes, &localScope{}) }
func (g *generator) popScope()  { g.scopes = g.scopes[:len(g.scopes)-1] }

// declareLocal introduces a fresh Go variable for a `local` declaration or
// for-loop variable at the current position and records it in the innermost
// scope. Each call allocates a globally-unique Go identifier (never reused,
// even for repeated declarations of the same Lua name) so that an outer
// binding shadowed by an inner one keeps its own live Go variable instead of
// having its value silently overwritten.
func (g *generator) declareLocal(name string) string {
	return g.declareLocalKind(name, monoValue)
}

func (g *generator) declareLocalKind(name string, kind monoKind) string {
	g.localSeq++
	goVar := fmt.Sprintf("%s_%d", goIdent("l_"+name), g.localSeq)
	top := g.scopes[len(g.scopes)-1]
	top.entries = append(top.entries, localEntry{name: name, goVar: goVar, kind: kind})
	return goVar
}

// bindParamKind records a Lua parameter name as an alias for the Go function
// parameter directly (a0, a1, ...). Go parameters are already mutable locals,
// so no extra copy is needed even though Lua allows reassigning a parameter
// like any other local.
func (g *generator) bindParamKind(name, goVar string, kind monoKind) {
	top := g.scopes[len(g.scopes)-1]
	top.entries = append(top.entries, localEntry{name: name, goVar: goVar, kind: kind})
}

// lookupLocal resolves a Lua name against the active scope stack, innermost
// scope and most-recent declaration first, matching Lua's block-scoping and
// same-block re-declaration/shadowing rules.
func (g *generator) lookupLocal(name string) (string, bool) {
	for i := len(g.scopes) - 1; i >= 0; i-- {
		entries := g.scopes[i].entries
		for j := len(entries) - 1; j >= 0; j-- {
			if entries[j].name == name {
				return entries[j].goVar, true
			}
		}
	}
	return "", false
}

func (g *generator) lookupKind(name string) monoKind {
	for i := len(g.scopes) - 1; i >= 0; i-- {
		entries := g.scopes[i].entries
		for j := len(entries) - 1; j >= 0; j-- {
			if entries[j].name == name {
				return entries[j].kind
			}
		}
	}
	return g.globalKinds[name]
}

func (g *generator) prepare() error {
	g.opts = withPoseFlipOffsets(g.opts)
	if g.opts.PackageName == "" {
		g.opts.PackageName = "main"
	}
	if !isGoIdent(g.opts.PackageName) {
		return fmt.Errorf("transpiler: invalid package name %q", g.opts.PackageName)
	}
	if g.opts.SheetW == 0 {
		g.opts.SheetW, g.opts.SheetH = 8, 8
	}
	if g.opts.SheetW < 1 || g.opts.SheetH < 1 || g.opts.MapW < 0 || g.opts.MapH < 0 {
		return errors.New("transpiler: invalid asset dimensions")
	}
	cols, rows := g.opts.SheetW/8, g.opts.SheetH/8
	spriteN, ok := checkedProduct(cols, rows)
	if !ok {
		return errors.New("transpiler: sprite dimensions overflow")
	}
	g.spriteN = spriteN
	if spriteN > 65536 {
		return errors.New("transpiler: sprite count exceeds uint16 map IDs")
	}
	sheetN, ok := checkedProduct(g.opts.SheetW, g.opts.SheetH)
	if !ok {
		return errors.New("transpiler: sheet dimensions overflow")
	}
	mapN, ok := checkedProduct(g.opts.MapW, g.opts.MapH)
	if !ok {
		return errors.New("transpiler: map dimensions overflow")
	}
	mapN, ok = checkedProduct(mapN, g.opts.MapLayers)
	if !ok {
		return errors.New("transpiler: map layer dimensions overflow")
	}
	if len(g.opts.Sheet) != 0 && len(g.opts.Sheet) != sheetN {
		return fmt.Errorf("transpiler: sheet has %d bytes, want %d", len(g.opts.Sheet), sheetN)
	}
	if len(g.opts.Map) != 0 && len(g.opts.Map) != mapN {
		return fmt.Errorf("transpiler: map has %d cells, want %d", len(g.opts.Map), mapN)
	}
	g.opts.MapPlaneMask &= uint8((1 << uint(g.opts.MapLayers)) - 1)
	g.mapCompact = compactMapPlanes(g.opts.Map, g.opts.MapW, g.opts.MapH, g.opts.MapLayers, g.opts.MapPlaneMask)
	if len(g.opts.Flags) != 0 && len(g.opts.Flags) != spriteN {
		return fmt.Errorf("transpiler: flags has %d bytes, want %d", len(g.opts.Flags), spriteN)
	}
	if len(g.opts.Palette) != 0 && len(g.opts.Palette) != 256 {
		return fmt.Errorf("transpiler: palette has %d entries, want 256", len(g.opts.Palette))
	}
	if err := g.discoverFunctions(); err != nil {
		return err
	}
	if err := g.validateIWRAMFunctions(); err != nil {
		return err
	}
	if err := g.validateSemantics(); err != nil {
		return err
	}
	if err := g.resolveReturnArities(); err != nil {
		return err
	}
	if err := g.validateBuiltinCallArities(); err != nil {
		return err
	}
	if err := g.validateTableLiteralLimits(); err != nil {
		return err
	}

	if g.opts.NativeAudio != nil {
		library, err := assetsync.CompileNativeAudio(g.opts.NativeAudio.Settings, g.opts.NativeAudio.Sounds, g.opts.NativeAudio.Music)
		if err != nil {
			return fmt.Errorf("transpiler: native audio: %w", err)
		}
		g.opts.NativeAudio.Library = library
		g.nativeAudioEnabled = chunkUsesNativeAudio(g.chunk)
		if g.nativeAudioEnabled {
			g.nativeAudioDecl, err = assetsync.NativeAudioGoDeclarations(library)
			if err != nil {
				return fmt.Errorf("transpiler: native audio declarations: %w", err)
			}
		}
	}
	g.inputEnabled = chunkCallsBuiltin(g.chunk, "btn") || chunkCallsBuiltin(g.chunk, "btnp")
	g.timeEnabled = chunkCallsBuiltin(g.chunk, "t") || chunkCallsBuiltin(g.chunk, "time") || chunkCallsBuiltin(g.chunk, "anim") ||
		chunkCallsBuiltin(g.chunk, "map") && tileLinksUseClips(g.opts.TileLinks)
	if len(g.opts.Palette) == 256 {
		canonical := assetsync.CanonicalPaletteROM()
		g.paletteIsDefault = slices.Equal(g.opts.Palette, canonical[:])
	}
	if err := g.validateStaticAudioCalls(); err != nil {
		return err
	}
	g.opts.MapStreaming = len(g.mapCompact) > assetsync.MapStreamingThresholdCells
	for _, info := range g.funcList {
		info.returnKinds = make([]monoKind, info.returnN)
	}
	g.sheetMutable = chunkCallsBuiltin(g.chunk, "sset")
	if g.opts.Renderer == RendererMode0 && g.opts.Mode0Text {
		draw := g.funcs["_draw"]
		if draw == nil || draw.decl == nil {
			return errors.New("transpiler: Mode 0 text selected without a _draw callback")
		}
		atlas := buildMode0TextAtlasForSource(g.chunk, draw.decl, g.opts)
		g.mode0TextKeys, g.mode0TextTiles = atlas.keys, atlas.tiles
		runtimeapi.OverlayMode0TextSheetWithFont(g.opts.Sheet, g.opts.SheetW, g.opts.SheetH, atlas.keys, g.opts.Font)
	}
	if !g.sheetMutable {
		g.compactSheet()
		if g.opts.Renderer != RendererMode0 && g.opts.MapDrawMask != 0 {
			g.sheetSolid = solidSheetColors(g.sheetCompact)
		}
	}
	g.compactObjSheet()
	if g.opts.Renderer == RendererMode0 {
		if info := g.funcs["_draw"]; info != nil && info.decl != nil {
			if ops, _, err := mode0WalkDraw(g.chunk, info.decl, newMode0SpriteIDAnalysis(g.chunk), g.opts); err == nil {
				g.psprPlan = mode0PsprEmitPlan(ops)
				for _, op := range ops {
					if op.tileLinkMap != nil {
						if g.tileLinkPlan == nil {
							g.tileLinkPlan = make(map[*luaparser.CallExpr]mode0PsprAllocation)
						}
						g.tileLinkPlan[op.tileLinkMap] = g.psprPlan[op.call]
					}
				}
				g.poseOBJResident = planMode0ResidentPoses(g.chunk, ops, g.opts)
				if g.poseOBJResident != nil {
					g.poseOBJIndex, g.poseOBJParts = g.poseOBJResident.index, g.poseOBJResident.parts
					if len(g.poseOBJResident.palettes) != 0 {
						// The immutable packed bank supplies every reachable pose.
						// Do not ship the build-time recolored source warehouse too.
						g.objSpriteN = 0
						g.objSheetCompact, g.objSheetIndex = nil, nil
					}
				} else {
					g.preparePoseOBJPlans()
				}
			}
		}
	}
	for _, info := range g.funcList {
		if (info.key == "_init" || info.key == "_update" || info.key == "_draw") && len(info.params) != 0 {
			return fmt.Errorf("transpiler: %s callback must not have parameters", info.key)
		}
	}
	g.discoverGlobals()
	g.analyzeNumericArrays()
	g.analyzeMonomorphicNumbers(false)
	g.analyzeNumericConstants()
	g.analyzeNumericRecords()
	g.analyzeNoAllocFunctions()
	g.analyzeMonomorphicFunctions()
	g.refineMonomorphicFieldSignatures()
	g.analyzeLoopTableReservations()
	g.heapEnabled = g.opts.DisableHeapElision || g.requiresHeap()
	if len(g.globals) > GlobalLimit {
		return &GlobalLimitError{Report: g.globalReport()}
	}
	// Collect strings after globals/functions have been discovered. Field
	// names are static strings too, so table field access never allocates.
	for _, stmt := range g.chunk.Body {
		g.collectStmtStatics(stmt)
	}
	for _, info := range g.funcList {
		g.collectBlockStatics(info.decl.Body)
	}
	if g.err != nil {
		return g.err
	}
	if err := g.validateGeneratedMemberNames(); err != nil {
		return err
	}
	return nil
}

func chunkUsesNativeAudio(chunk *luaparser.Chunk) bool {
	return chunkCallsBuiltin(chunk, "sfx") || chunkCallsBuiltin(chunk, "music") ||
		chunkCallsBuiltin(chunk, "gba_note") || chunkCallsBuiltin(chunk, "gba_stop")
}

// validateGeneratedMemberNames catches Lua names which are distinct in the
// source but collapse onto the same Go selector, or collide with lifecycle,
// wrapper, static-string, or field-cache members generated by Mtile.
func (g *generator) validateGeneratedMemberNames() error {
	type owner struct {
		description string
	}
	members := make(map[string]owner)
	claim := func(name, description string, position luaparser.Position) error {
		if previous, exists := members[name]; exists {
			where := ""
			if position.Line != 0 {
				where = " at " + position.String()
			}
			return fmt.Errorf("transpiler: generated Game member %q for %s collides with %s%s", name, description, previous.description, where)
		}
		members[name] = owner{description: description}
		return nil
	}
	for _, name := range []string{"Init", "Update", "Draw"} {
		if err := claim(name, "the generated lifecycle method", luaparser.Position{}); err != nil {
			return err
		}
	}
	for i := range g.statics {
		if err := claim(fmt.Sprintf("_gluStatic%d", i), "a generated static-string field", luaparser.Position{}); err != nil {
			return err
		}
	}
	for i := 0; i < g.fieldCacheN; i++ {
		if err := claim(fmt.Sprintf("_gluField%d", i), "a generated table-field cache", luaparser.Position{}); err != nil {
			return err
		}
	}
	for _, record := range g.numericRecordOrder {
		for _, field := range record.fieldOrder {
			if err := claim(numericRecordMember(record, field), "a generated numeric-record field", luaparser.Position{}); err != nil {
				return err
			}
		}
	}
	for _, fn := range g.funcList {
		position := fn.decl.Pos()
		names := []struct {
			name, kind string
		}{
			{fn.goName, fmt.Sprintf("Lua function %q", fn.key)},
			{"__glu_call_" + fn.goName, fmt.Sprintf("the call wrapper for Lua function %q", fn.key)},
		}
		if fn.returnN > 1 {
			names = append(names,
				struct{ name, kind string }{"__glu_multi_" + fn.goName, fmt.Sprintf("the multi-return wrapper for Lua function %q", fn.key)},
				struct{ name, kind string }{"__glu_first_" + fn.goName, fmt.Sprintf("the first-result wrapper for Lua function %q", fn.key)},
			)
		}
		for _, member := range names {
			if err := claim(member.name, member.kind, position); err != nil {
				return err
			}
		}
	}
	for _, name := range g.globals {
		if err := claim(g.globalGo[name], fmt.Sprintf("Lua global %q", name), g.globalOrigins[name]); err != nil {
			return err
		}
	}
	return nil
}
