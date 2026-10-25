// Package transpiler turns the Mtile Lua AST into a self-contained Go
// main package for the mtile runtime.
//
// User state lives in Game and the generated main owns the GBA presentation
// loop. Proven monomorphic numbers use runtime.Number; all other Lua values
// retain runtime.Value. Keeping those decisions here makes the runtime
// independent of the generated package (and avoids an import cycle).
package transpiler

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
	"github.com/drpaneas/mtile/runtime/audio"
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
	PoseIndex            []runtimeapi.PoseRange
	AnimationClips       []runtimeapi.AnimationClip
	AnimationFrames      []uint16
	PoseParts            []runtimeapi.PosePart
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
	"cls": {"ClsV", 0, 1, 0}, "pset": {"PsetV", 2, 3, 0}, "pget": {"PgetV", 2, 2, 1},
	"line": {"LineV", 4, 5, 0}, "rect": {"RectV", 4, 5, 0}, "rectfill": {"RectfillV", 4, 5, 0},
	"circ": {"CircV", 3, 4, 0}, "circfill": {"CircfillV", 3, 4, 0},
	"spr": {"SprV", 3, 7, 0}, "sspr": {"SSprV", 6, 10, 0},
	// Flips automatically preserve the pose bounds, or the union bounds of a
	// named clip; callers keep the same x/y anchor in either facing direction.
	"pvariant": {"PVariantV", 0, 1, 0},
	"pspr":     {"PSprV", 3, 5, 0}, "anim": {"AnimV", 3, 6, 0},
	"map":    {"MapBGAtV", 0, 8, 0},
	"camera": {"CameraV", 0, 2, 0}, "clip": {"ClipV", 0, 4, 0}, "pal": {"PalV", 0, 3, 0},
	"palt": {"PaltV", 0, 2, 0}, "color": {"ColorV", 0, 1, 0}, "cursor": {"CursorV", 0, 3, 0},
	"print": {"PrintV", 1, 4, 1},
	"sget":  {"Sget", 2, 2, 1}, "sset": {"Sset", 3, 3, 0}, "fget": {"Fget", 1, 2, 1},
	"fset": {"Fset", 2, 3, 0}, "mget": {"Mget", 2, 3, 1}, "mset": {"Mset", 3, 4, 0},
	"btn": {"BtnV", 1, 2, 1}, "btnp": {"BtnpV", 1, 2, 1},
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
	if chunk == nil {
		return "", generationMetadata{}, errors.New("transpiler: nil chunk")
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
	g := newGenerator(chunk, opts)
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
	chunk *luaparser.Chunk
	opts  Options

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
	poseOBJIndex     []runtimeapi.PoseOBJRange
	poseOBJParts     []runtimeapi.PoseOBJ
	poseOBJTiles     []uint16
	poseOBJResident  *mode0ResidentPosePlan
	packedMap        [4]packedMapPlane
	mapCompact       []uint16
	mode0TextKeys    []uint16
	mode0TextTiles   []uint32

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
	g.timeEnabled = chunkCallsBuiltin(g.chunk, "t") || chunkCallsBuiltin(g.chunk, "time") || chunkCallsBuiltin(g.chunk, "anim")
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
		runtimeapi.OverlayMode0TextSheet(g.opts.Sheet, g.opts.SheetW, g.opts.SheetH, atlas.keys)
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

const maxNumericArrayLength = 256

// analyzeNumericArrays finds a deliberately narrow compact-storage shape:
// fixed-length numeric literals declared at chunk scope and used only through
// direct integer indexing (or #array). Any escape, rebinding, or shadowing
// keeps the ordinary Lua table representation. Element writes are converted
// to Number once; enabling the experiment opts into the monomorphic contract
// that their type will not change later.
func (g *generator) analyzeNumericArrays() {
	if g.opts.DisableNumericArrays || g.opts.DisableMonomorphicNumbers {
		return
	}
	for _, stmt := range g.chunk.Body {
		declaration, ok := stmt.(*luaparser.LocalAssignStmt)
		if !ok || len(declaration.Names) != 1 || len(declaration.Values) != 1 {
			continue
		}
		literal, ok := declaration.Values[0].(*luaparser.TableExpr)
		if !ok || len(literal.Fields) == 0 || len(literal.Fields) > maxNumericArrayLength {
			continue
		}
		numeric := true
		for _, field := range literal.Fields {
			if !field.Array {
				numeric = false
				break
			}
			if _, ok := field.Value.(*luaparser.NumberExpr); !ok {
				numeric = false
				break
			}
		}
		if numeric {
			name := declaration.Names[0]
			g.numericArrays[name] = &numericArrayInfo{declaration: declaration, literal: literal, length: len(literal.Fields)}
		}
	}
	for name, info := range g.numericArrays {
		if !g.numericArrayUsesAreSafe(name, info.declaration) {
			delete(g.numericArrays, name)
		}
	}
}

func (g *generator) numericArrayUsesAreSafe(name string, declaration *luaparser.LocalAssignStmt) bool {
	var obviouslyNonNumeric func(luaparser.Expr) bool
	obviouslyNonNumeric = func(expr luaparser.Expr) bool {
		switch x := expr.(type) {
		case *luaparser.ParenExpr:
			return obviouslyNonNumeric(x.Expr)
		case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.StringExpr, *luaparser.TableExpr:
			return true
		case *luaparser.UnaryExpr:
			return x.Op == luaparser.OpNot
		case *luaparser.BinaryExpr:
			switch x.Op {
			case luaparser.OpEqual, luaparser.OpNotEqual, luaparser.OpLess,
				luaparser.OpLessEq, luaparser.OpGreater, luaparser.OpGreaterEq,
				luaparser.OpConcat:
				return true
			}
		}
		return false
	}
	var safeExpr func(luaparser.Expr) bool
	safeExpr = func(expr luaparser.Expr) bool {
		switch x := expr.(type) {
		case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.NumberExpr, *luaparser.StringExpr:
			return true
		case *luaparser.ParenExpr:
			return safeExpr(x.Expr)
		case *luaparser.VarExpr:
			return x.Name != name
		case *luaparser.UnaryExpr:
			if x.Op == luaparser.OpLength {
				if base, ok := x.Expr.(*luaparser.VarExpr); ok && base.Name == name {
					return true
				}
			}
			return safeExpr(x.Expr)
		case *luaparser.BinaryExpr:
			return safeExpr(x.Left) && safeExpr(x.Right)
		case *luaparser.IndexExpr:
			if base, ok := x.Base.(*luaparser.VarExpr); ok && base.Name == name {
				return safeExpr(x.Key)
			}
			return safeExpr(x.Base) && safeExpr(x.Key)
		case *luaparser.FieldExpr:
			return safeExpr(x.Base)
		case *luaparser.CallExpr:
			if !safeExpr(x.Callee) {
				return false
			}
			for _, argument := range x.Args {
				if !safeExpr(argument) {
					return false
				}
			}
			return true
		case *luaparser.TableExpr:
			for _, field := range x.Fields {
				if !safeExpr(field.Key) || !safeExpr(field.Value) {
					return false
				}
			}
			return true
		}
		return true
	}

	var safeBlock func(*luaparser.Block) bool
	var safeStmt func(luaparser.Stmt) bool
	safeBlock = func(block *luaparser.Block) bool {
		for _, stmt := range block.Body {
			if !safeStmt(stmt) {
				return false
			}
		}
		return true
	}
	safeStmt = func(stmt luaparser.Stmt) bool {
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			for index, target := range s.Targets {
				if variable, ok := target.(*luaparser.VarExpr); ok && variable.Name == name {
					return false
				}
				if indexed, ok := target.(*luaparser.IndexExpr); ok {
					if base, ok := indexed.Base.(*luaparser.VarExpr); ok && base.Name == name {
						if index >= len(s.Values) || obviouslyNonNumeric(s.Values[index]) {
							return false
						}
					}
				}
				if !safeExpr(target) {
					return false
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.LocalAssignStmt:
			if s != declaration {
				for _, local := range s.Names {
					if local == name {
						return false
					}
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.CallStmt:
			return safeExpr(s.Call)
		case *luaparser.IfStmt:
			if !safeExpr(s.Condition) || !safeBlock(s.Then) {
				return false
			}
			for _, branch := range s.ElseIf {
				if !safeExpr(branch.Condition) || !safeBlock(branch.Body) {
					return false
				}
			}
			return s.Else == nil || safeBlock(s.Else)
		case *luaparser.WhileStmt:
			return safeExpr(s.Condition) && safeBlock(s.Body)
		case *luaparser.RepeatStmt:
			return safeBlock(s.Body) && safeExpr(s.Condition)
		case *luaparser.DoStmt:
			return safeBlock(s.Body)
		case *luaparser.NumericForStmt:
			return s.Name != name && safeExpr(s.Init) && safeExpr(s.Limit) && safeExpr(s.Step) && safeBlock(s.Body)
		case *luaparser.GenericForStmt:
			for _, local := range s.Names {
				if local == name {
					return false
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
			return safeBlock(s.Body)
		case *luaparser.ReturnStmt:
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.FunctionDeclStmt:
			for _, parameter := range s.Params {
				if parameter == name {
					return false
				}
			}
			return safeBlock(s.Body)
		}
		return true
	}

	for _, stmt := range g.chunk.Body {
		if !safeStmt(stmt) {
			return false
		}
	}
	return true
}

const maxNumericRecordFields = 32

// analyzeNumericRecords performs scalar replacement for one deliberately
// narrow Lua record shape: a chunk-scope local table with unique named fields,
// numeric initializers, and no identity-observing use. Any alias, dynamic key,
// rebinding, shadowing, shape change, or obviously nonnumeric write keeps the
// ordinary heap-table representation.
func (g *generator) analyzeNumericRecords() {
	if g.opts.DisableMonomorphicNumbers || g.opts.DisableStableTableFields {
		return
	}
	for _, stmt := range g.chunk.Body {
		name, literal, ok := numericRecordDeclaration(stmt)
		if !ok || len(literal.Fields) == 0 || len(literal.Fields) > maxNumericRecordFields {
			continue
		}
		info := &numericRecordInfo{
			declaration: stmt,
			literal:     literal,
			fields:      make(map[string]*numericRecordFieldInfo, len(literal.Fields)),
		}
		valid := true
		for index, field := range literal.Fields {
			key, named := field.Key.(*luaparser.StringExpr)
			if field.Array || !named || key.Value == "" || info.fields[key.Value] != nil ||
				!g.numericRecordInitializerSafe(field.Value, stmt) {
				valid = false
				break
			}
			recordField := &numericRecordFieldInfo{name: key.Value, value: field.Value, index: index}
			info.fields[key.Value] = recordField
			info.fieldOrder = append(info.fieldOrder, recordField)
		}
		if valid {
			g.numericRecords[name] = info
			g.numericRecordOrder = append(g.numericRecordOrder, info)
		}
	}

	kept := g.numericRecordOrder[:0]
	for name, info := range g.numericRecords {
		if !g.numericRecordUsesAreSafe(name, info) {
			delete(g.numericRecords, name)
		}
	}
	// Restore source order after the map-based validation pass and assign
	// deterministic generated member indices.
	for _, stmt := range g.chunk.Body {
		name, _, ok := numericRecordDeclaration(stmt)
		info := g.numericRecords[name]
		if !ok || info == nil || info.declaration != stmt {
			continue
		}
		info.index = len(kept)
		kept = append(kept, info)
		for _, field := range info.literal.Fields {
			g.numericRecordKeys[field.Key.(*luaparser.StringExpr)] = true
		}
	}
	g.numericRecordOrder = kept
}

func (g *generator) numericRecordInitializerSafe(expr luaparser.Expr, declaration luaparser.Stmt) bool {
	switch x := expr.(type) {
	case *luaparser.NumberExpr:
		return true
	case *luaparser.ParenExpr:
		return g.numericRecordInitializerSafe(x.Expr, declaration)
	case *luaparser.VarExpr:
		if g.globalKinds[x.Name] != monoNumber {
			return false
		}
		for _, stmt := range g.chunk.Body {
			if stmt == declaration {
				return false
			}
			switch s := stmt.(type) {
			case *luaparser.LocalAssignStmt:
				for _, name := range s.Names {
					if name == x.Name {
						return true
					}
				}
			case *luaparser.AssignStmt:
				for _, target := range s.Targets {
					if variable, ok := target.(*luaparser.VarExpr); ok && variable.Name == x.Name {
						return true
					}
				}
			}
		}
		return false
	case *luaparser.UnaryExpr:
		return (x.Op == luaparser.OpNegate || x.Op == luaparser.OpBitNot) && g.numericRecordInitializerSafe(x.Expr, declaration)
	case *luaparser.BinaryExpr:
		switch x.Op {
		case luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
			luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo, luaparser.OpPower,
			luaparser.OpBitOr, luaparser.OpBitXor, luaparser.OpBitAnd,
			luaparser.OpShiftL, luaparser.OpShiftR, luaparser.OpShiftRLogical,
			luaparser.OpRotateL, luaparser.OpRotateR:
			return g.numericRecordInitializerSafe(x.Left, declaration) && g.numericRecordInitializerSafe(x.Right, declaration)
		}
	case *luaparser.CallExpr:
		name, ok := callKey(x)
		if !ok {
			return false
		}
		switch name {
		case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd",
			"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr", "t", "time", "stat", "mget":
			for _, argument := range x.Args {
				if !g.numericRecordInitializerSafe(argument, declaration) {
					return false
				}
			}
			return true
		}
	}
	return false
}

func numericRecordDeclaration(stmt luaparser.Stmt) (string, *luaparser.TableExpr, bool) {
	switch declaration := stmt.(type) {
	case *luaparser.LocalAssignStmt:
		if len(declaration.Names) != 1 || len(declaration.Values) != 1 {
			return "", nil, false
		}
		literal, ok := declaration.Values[0].(*luaparser.TableExpr)
		return declaration.Names[0], literal, ok
	case *luaparser.AssignStmt:
		if len(declaration.Targets) != 1 || len(declaration.Values) != 1 {
			return "", nil, false
		}
		variable, ok := declaration.Targets[0].(*luaparser.VarExpr)
		if !ok {
			return "", nil, false
		}
		literal, ok := declaration.Values[0].(*luaparser.TableExpr)
		return variable.Name, literal, ok
	default:
		return "", nil, false
	}
}

func (g *generator) numericRecordUsesAreSafe(name string, info *numericRecordInfo) bool {
	for _, stmt := range g.chunk.Body {
		if stmt == info.declaration {
			break
		}
		referenced := false
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			if variable, ok := expr.(*luaparser.VarExpr); ok && variable.Name == name {
				referenced = true
			}
		})
		if referenced {
			return false
		}
	}
	obviouslyNonNumeric := func(expr luaparser.Expr) bool {
		for {
			paren, ok := expr.(*luaparser.ParenExpr)
			if !ok {
				break
			}
			expr = paren.Expr
		}
		switch x := expr.(type) {
		case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.StringExpr, *luaparser.TableExpr:
			return true
		case *luaparser.UnaryExpr:
			return x.Op == luaparser.OpNot
		case *luaparser.BinaryExpr:
			switch x.Op {
			case luaparser.OpEqual, luaparser.OpNotEqual, luaparser.OpLess,
				luaparser.OpLessEq, luaparser.OpGreater, luaparser.OpGreaterEq,
				luaparser.OpConcat, luaparser.OpAnd, luaparser.OpOr:
				return true
			}
		}
		return false
	}
	directField := func(expr luaparser.Expr) (*luaparser.FieldExpr, bool) {
		field, ok := expr.(*luaparser.FieldExpr)
		if !ok {
			return nil, false
		}
		base, ok := field.Base.(*luaparser.VarExpr)
		return field, ok && base.Name == name
	}
	var safeExpr func(luaparser.Expr) bool
	safeExpr = func(expr luaparser.Expr) bool {
		switch x := expr.(type) {
		case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.NumberExpr, *luaparser.StringExpr:
			return true
		case *luaparser.ParenExpr:
			return safeExpr(x.Expr)
		case *luaparser.VarExpr:
			return x.Name != name
		case *luaparser.UnaryExpr:
			return safeExpr(x.Expr)
		case *luaparser.BinaryExpr:
			return safeExpr(x.Left) && safeExpr(x.Right)
		case *luaparser.IndexExpr:
			if base, ok := x.Base.(*luaparser.VarExpr); ok && base.Name == name {
				return false
			}
			return safeExpr(x.Base) && safeExpr(x.Key)
		case *luaparser.FieldExpr:
			if field, ok := directField(x); ok {
				return info.fields[field.Name] != nil
			}
			return safeExpr(x.Base)
		case *luaparser.CallExpr:
			if !safeExpr(x.Callee) {
				return false
			}
			for _, argument := range x.Args {
				if !safeExpr(argument) {
					return false
				}
			}
			return true
		case *luaparser.TableExpr:
			for _, field := range x.Fields {
				if !safeExpr(field.Key) || !safeExpr(field.Value) {
					return false
				}
			}
			return true
		}
		return true
	}

	var safeBlock func(*luaparser.Block) bool
	var safeStmt func(luaparser.Stmt) bool
	safeBlock = func(block *luaparser.Block) bool {
		for _, stmt := range block.Body {
			if !safeStmt(stmt) {
				return false
			}
		}
		return true
	}
	safeStmt = func(stmt luaparser.Stmt) bool {
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			for index, target := range s.Targets {
				if variable, ok := target.(*luaparser.VarExpr); ok && variable.Name == name {
					if s != info.declaration {
						return false
					}
					continue
				}
				if field, ok := directField(target); ok {
					if info.fields[field.Name] == nil || len(s.Targets) != len(s.Values) || index >= len(s.Values) || obviouslyNonNumeric(s.Values[index]) {
						return false
					}
				}
				if !safeExpr(target) {
					return false
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.LocalAssignStmt:
			if luaparser.Stmt(s) != info.declaration {
				for _, local := range s.Names {
					if local == name {
						return false
					}
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.CallStmt:
			return safeExpr(s.Call)
		case *luaparser.IfStmt:
			if !safeExpr(s.Condition) || !safeBlock(s.Then) {
				return false
			}
			for _, branch := range s.ElseIf {
				if !safeExpr(branch.Condition) || !safeBlock(branch.Body) {
					return false
				}
			}
			return s.Else == nil || safeBlock(s.Else)
		case *luaparser.WhileStmt:
			return safeExpr(s.Condition) && safeBlock(s.Body)
		case *luaparser.RepeatStmt:
			return safeBlock(s.Body) && safeExpr(s.Condition)
		case *luaparser.DoStmt:
			return safeBlock(s.Body)
		case *luaparser.NumericForStmt:
			return s.Name != name && safeExpr(s.Init) && safeExpr(s.Limit) && safeExpr(s.Step) && safeBlock(s.Body)
		case *luaparser.GenericForStmt:
			for _, local := range s.Names {
				if local == name {
					return false
				}
			}
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
			return safeBlock(s.Body)
		case *luaparser.ReturnStmt:
			for _, value := range s.Values {
				if !safeExpr(value) {
					return false
				}
			}
		case *luaparser.FunctionDeclStmt:
			if len(s.Name) != 0 && s.Name[0] == name {
				return false
			}
			for _, parameter := range s.Params {
				if parameter == name {
					return false
				}
			}
			return safeBlock(s.Body)
		}
		return true
	}
	for _, stmt := range g.chunk.Body {
		if !safeStmt(stmt) {
			return false
		}
	}
	return true
}

func (g *generator) numericRecordField(expr *luaparser.FieldExpr) (*numericRecordInfo, *numericRecordFieldInfo, bool) {
	if expr == nil {
		return nil, nil, false
	}
	base, ok := expr.Base.(*luaparser.VarExpr)
	if !ok {
		return nil, nil, false
	}
	if _, local := g.lookupLocal(base.Name); local {
		return nil, nil, false
	}
	info := g.numericRecords[base.Name]
	if info == nil || info.fields[expr.Name] == nil {
		return nil, nil, false
	}
	return info, info.fields[expr.Name], true
}

func numericRecordMember(info *numericRecordInfo, field *numericRecordFieldInfo) string {
	return fmt.Sprintf("_gluRecord%dField%d", info.index, field.index)
}

func (g *generator) numericArrayIndex(expr *luaparser.IndexExpr) (string, *numericArrayInfo, bool) {
	if expr == nil {
		return "", nil, false
	}
	base, ok := expr.Base.(*luaparser.VarExpr)
	if !ok {
		return "", nil, false
	}
	if _, local := g.lookupLocal(base.Name); local {
		return "", nil, false
	}
	info := g.numericArrays[base.Name]
	return base.Name, info, info != nil
}

func (g *generator) analyzeMonomorphicNumbers(poisonUnknown bool) {
	if g.opts.DisableMonomorphicNumbers {
		return
	}
	// Top-level declarations own Game fields. Their first source assignment is
	// the type observation used by the monomorphic contract.
	seenGlobal := make(map[string]bool)
	for _, stmt := range g.chunk.Body {
		switch s := stmt.(type) {
		case *luaparser.LocalAssignStmt:
			for i, name := range s.Names {
				if seenGlobal[name] {
					continue
				}
				seenGlobal[name] = true
				if i < len(s.Values) && g.numericInitializerIntent(s.Values[i]) {
					g.globalKinds[name] = monoNumber
				}
			}
		case *luaparser.AssignStmt:
			for i, target := range s.Targets {
				v, ok := target.(*luaparser.VarExpr)
				if !ok || seenGlobal[v.Name] {
					continue
				}
				seenGlobal[v.Name] = true
				if i < len(s.Values) && g.numericInitializerIntent(s.Values[i]) {
					g.globalKinds[v.Name] = monoNumber
				}
			}
		}
	}

	// Record fields retain Value storage, but a field name whose observed table
	// literal initializers and named-field writes are all numeric can use native
	// Number reads/writes. Resolve initializer aliases in their lexical scope.
	// local or parameter must hide a same-named numeric global. Keep provisional
	// field observations out of expression inference until every constructor
	// has been visited: a later mixed initializer may invalidate an early one.
	fields := make(map[string]bool)
	observe := func(name string, isNumber bool) {
		if previous, seen := fields[name]; !seen || previous {
			fields[name] = isNumber
		}
	}
	params := make(map[*functionInfo][]functionParamObservation)
	top := &monomorphicAnalysisEnv{g: g, observeField: observe, poisonUnknownFields: poisonUnknown}
	top.push()
	for _, stmt := range g.chunk.Body {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			env := &monomorphicAnalysisEnv{g: g, observeField: observe, poisonUnknownFields: poisonUnknown}
			env.push()
			info := g.funcs[functionKey(fn)]
			for i, name := range fn.Params {
				kind := monoValue
				if info != nil && i < len(info.paramKinds) {
					kind = info.paramKinds[i]
				}
				env.declare(name, kind)
			}
			for _, bodyStmt := range fn.Body.Body {
				g.analyzeMonomorphicStmt(bodyStmt, env, params, nil, false)
			}
			continue
		}
		g.analyzeMonomorphicStmt(stmt, top, params, nil, true)
	}
	g.numericFields = fields
}

func numericFieldsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for name, isNumber := range a {
		if b[name] != isNumber {
			return false
		}
	}
	return true
}

func cloneNumericFields(fields map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(fields))
	for name, isNumber := range fields {
		cloned[name] = isNumber
	}
	return cloned
}

func (g *generator) resetMonomorphicSignatures() {
	for _, fn := range g.funcList {
		for i := range fn.paramKinds {
			fn.paramKinds[i] = monoValue
		}
		for i := range fn.returnKinds {
			fn.returnKinds[i] = monoValue
		}
		fn.coercedNumberParams = nil
	}
}

// refineMonomorphicFieldSignatures closes the loop between function signatures
// and named-field observations. Untyped aliases are skipped until a later
// confirmation pass poisons any initializer that is still not a number, so a
// numeric parameter constructor can keep native fields without letting a
// string parameter keep a provisional numeric type.
func (g *generator) refineMonomorphicFieldSignatures() {
	if g.opts.DisableMonomorphicNumbers {
		return
	}
	for pass := 0; pass < 32; pass++ {
		before := cloneNumericFields(g.numericFields)
		g.analyzeMonomorphicNumbers(false)
		if !g.opts.DisableMonomorphicFunctions {
			g.analyzeMonomorphicFunctions()
		}
		if numericFieldsEqual(before, g.numericFields) {
			break
		}
	}
	// Confirm the provisional facts without discarding numeric read/write
	// aliases. Confirmation can only remove candidates: each retry removes at
	// least one numeric field, so N candidates require at most N+1 passes.
	// Recompute signatures after each removal so a function cannot carry a
	// stale numeric field type into another field through a parameter or return.
	candidates := cloneNumericFields(g.numericFields)
	for {
		g.analyzeMonomorphicNumbers(true)
		changed := false
		for name, isNumber := range candidates {
			if isNumber && !g.numericFields[name] {
				candidates[name] = false
				changed = true
			}
		}
		g.numericFields = cloneNumericFields(candidates)
		if !changed {
			return
		}
		g.resetMonomorphicSignatures()
		g.analyzeMonomorphicFunctions()
	}
}

func (g *generator) numericInitializerIntent(expr luaparser.Expr) bool {
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return g.numericInitializerIntent(x.Expr)
	case *luaparser.VarExpr:
		// The first pass observes chunk-scope declarations in source order, so
		// direct aliases of an already-proven numeric constant can keep native
		// Number storage. Forward references conservatively remain Values.
		return g.globalKinds[x.Name] == monoNumber
	default:
		return numericIntent(expr)
	}
}

// observeNamedField records constructor-style evidence from `t.name = value`.
// It uses the previous analysis pass for field reads, never the in-progress
// map, so a later mixed store can still poison a provisional numeric name.
func (e *monomorphicAnalysisEnv) observeNamedField(name string, value luaparser.Expr, kind monoKind) {
	if e.observeField == nil || name == "" {
		return
	}
	if kind == monoNumber || (value != nil && e.numericFieldIntent(value)) {
		e.observeField(name, true)
		return
	}
	if value != nil && (provenNonNumericFieldValue(value) || e.poisonUnknownFields && poisonUnknownFieldValue(value)) {
		e.observeField(name, false)
		return
	}
	if value == nil && e.poisonUnknownFields {
		e.observeField(name, false)
	}
}

func (e *monomorphicAnalysisEnv) numericFieldIntent(expr luaparser.Expr) bool {
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return e.numericFieldIntent(x.Expr)
	case *luaparser.VarExpr:
		return e.lookup(x.Name) == monoNumber
	case *luaparser.FieldExpr:
		return !e.g.opts.DisableStableTableFields && e.g.numericFields[x.Name]
	case *luaparser.CallExpr:
		if fn := e.g.calledFunction(x); fn != nil {
			return fn.returnN > 0 && fn.returnKind(0) == monoNumber
		}
		return numericIntent(expr)
	default:
		return numericIntent(expr)
	}
}

func provenNonNumericFieldValue(expr luaparser.Expr) bool {
	switch x := unparenExpr(expr).(type) {
	case *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.StringExpr, *luaparser.TableExpr:
		return true
	case *luaparser.UnaryExpr:
		return x.Op == luaparser.OpNot
	case *luaparser.BinaryExpr:
		switch x.Op {
		case luaparser.OpEqual, luaparser.OpNotEqual, luaparser.OpLess, luaparser.OpLessEq,
			luaparser.OpGreater, luaparser.OpGreaterEq, luaparser.OpConcat:
			return true
		}
	}
	return false
}

func poisonUnknownFieldValue(expr luaparser.Expr) bool {
	switch unparenExpr(expr).(type) {
	case *luaparser.FieldExpr, *luaparser.IndexExpr:
		// A read of another table's field is not proof this name is non-numeric.
		// Confirmation still poisons untyped aliases and non-numeric literals.
		return false
	default:
		return true
	}
}

func numericIntent(expr luaparser.Expr) bool {
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return numericIntent(x.Expr)
	case *luaparser.NumberExpr:
		return true
	case *luaparser.UnaryExpr:
		return x.Op == luaparser.OpNegate || x.Op == luaparser.OpBitNot
	case *luaparser.BinaryExpr:
		switch x.Op {
		case luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
			luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo, luaparser.OpPower,
			luaparser.OpBitOr, luaparser.OpBitXor, luaparser.OpBitAnd,
			luaparser.OpShiftL, luaparser.OpShiftR, luaparser.OpShiftRLogical,
			luaparser.OpRotateL, luaparser.OpRotateR:
			return true
		}
	case *luaparser.CallExpr:
		name, ok := callKey(x)
		if !ok {
			return false
		}
		switch name {
		case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd",
			"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr", "t", "time", "stat", "mget":
			return true
		}
	}
	return false
}

type functionParamObservation struct {
	seen      bool
	allNumber bool
}

type functionReturnObservation struct {
	seen      bool
	allNumber []bool
}

type monomorphicAnalysisEnv struct {
	g                   *generator
	scopes              []map[string]monoKind
	observeField        func(name string, isNumber bool)
	poisonUnknownFields bool
}

func (e *monomorphicAnalysisEnv) push() {
	e.scopes = append(e.scopes, make(map[string]monoKind))
}

func (e *monomorphicAnalysisEnv) pop() {
	e.scopes = e.scopes[:len(e.scopes)-1]
}

func (e *monomorphicAnalysisEnv) declare(name string, kind monoKind) {
	e.scopes[len(e.scopes)-1][name] = kind
}

func (e *monomorphicAnalysisEnv) lookup(name string) monoKind {
	for i := len(e.scopes) - 1; i >= 0; i-- {
		if kind, ok := e.scopes[i][name]; ok {
			return kind
		}
	}
	return e.g.globalKinds[name]
}

// analyzeMonomorphicFunctions computes a closed-world numeric signature for
// direct user functions. Mtile rejects closures and first-class function
// values, so every supported call target is statically known. A parameter is
// promoted only when every source call site supplies a proven Number. Each
// return position is promoted only when every path supplies a number in
// that position and the function cannot fall through. Unknown or mixed slots
// retain rt.Value independently.
func (g *generator) analyzeMonomorphicFunctions() {
	if g.opts.DisableMonomorphicNumbers || g.opts.DisableMonomorphicFunctions {
		return
	}
	for {
		params := make(map[*functionInfo][]functionParamObservation, len(g.funcList))
		returns := make(map[*functionInfo]*functionReturnObservation, len(g.funcList))
		for _, fn := range g.funcList {
			params[fn] = make([]functionParamObservation, len(fn.params))
			for i := range params[fn] {
				params[fn][i].allNumber = true
			}
			returns[fn] = &functionReturnObservation{allNumber: make([]bool, fn.returnN)}
			for i := range returns[fn].allNumber {
				returns[fn].allNumber[i] = true
			}
		}

		top := &monomorphicAnalysisEnv{g: g}
		top.push()
		for _, stmt := range g.chunk.Body {
			if _, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
				continue
			}
			g.analyzeMonomorphicStmt(stmt, top, params, nil, true)
		}

		for _, fn := range g.funcList {
			env := &monomorphicAnalysisEnv{g: g}
			env.push()
			for i, name := range fn.params {
				env.declare(name, fn.paramKinds[i])
			}
			for _, stmt := range fn.decl.Body.Body {
				g.analyzeMonomorphicStmt(stmt, env, params, returns[fn], false)
			}
		}

		changed := false
		for _, fn := range g.funcList {
			for i, observation := range params[fn] {
				demanded := observation.seen && g.parameterAlwaysCoerced(fn, i)
				if demanded && !fn.coercesNumberParam(i) {
					if fn.coercedNumberParams == nil {
						fn.coercedNumberParams = make([]bool, len(fn.params))
					}
					fn.coercedNumberParams[i] = true
					changed = true
				}
				if fn.paramKinds[i] == monoValue && observation.seen && (observation.allNumber || demanded) {
					fn.paramKinds[i] = monoNumber
					changed = true
				}
			}
			observation := returns[fn]
			if !observation.seen || !blockAlwaysReturns(fn.decl.Body) || (fn.returnN > 1 && g.opts.DisableMonomorphicMultiReturns) {
				continue
			}
			for i, allNumber := range observation.allNumber {
				if fn.returnKinds[i] == monoValue && allNumber {
					fn.returnKinds[i] = monoNumber
					changed = true
				}
			}
		}
		if !changed {
			return
		}
	}
}

func (g *generator) analyzeMonomorphicStmt(stmt luaparser.Stmt, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation, returns *functionReturnObservation, topLevel bool) {
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		for _, target := range s.Targets {
			switch target := target.(type) {
			case *luaparser.FieldExpr:
				g.analyzeMonomorphicExpr(target.Base, env, params)
			case *luaparser.IndexExpr:
				g.analyzeMonomorphicExpr(target.Base, env, params)
				g.analyzeMonomorphicExpr(target.Key, env, params)
			}
		}
		kinds := g.analyzeMonomorphicExprList(s.Values, env, params)
		for i, target := range s.Targets {
			field, ok := target.(*luaparser.FieldExpr)
			if !ok {
				continue
			}
			var value luaparser.Expr
			if i < len(s.Values) {
				value = s.Values[i]
			}
			kind := monoValue
			if i < len(kinds) {
				kind = kinds[i]
			}
			env.observeNamedField(field.Name, value, kind)
		}
	case *luaparser.LocalAssignStmt:
		kinds := g.analyzeMonomorphicValues(s.Values, len(s.Names), env, params)
		if !topLevel {
			for i, name := range s.Names {
				kind := monoValue
				if i < len(kinds) {
					kind = kinds[i]
				}
				env.declare(name, kind)
			}
		}
	case *luaparser.CallStmt:
		g.analyzeMonomorphicExpr(s.Call, env, params)
	case *luaparser.IfStmt:
		g.analyzeMonomorphicExpr(s.Condition, env, params)
		g.analyzeMonomorphicBlock(s.Then, env, params, returns)
		for _, branch := range s.ElseIf {
			g.analyzeMonomorphicExpr(branch.Condition, env, params)
			g.analyzeMonomorphicBlock(branch.Body, env, params, returns)
		}
		if s.Else != nil {
			g.analyzeMonomorphicBlock(s.Else, env, params, returns)
		}
	case *luaparser.WhileStmt:
		g.analyzeMonomorphicExpr(s.Condition, env, params)
		g.analyzeMonomorphicBlock(s.Body, env, params, returns)
	case *luaparser.RepeatStmt:
		env.push()
		for _, bodyStmt := range s.Body.Body {
			g.analyzeMonomorphicStmt(bodyStmt, env, params, returns, false)
		}
		g.analyzeMonomorphicExpr(s.Condition, env, params)
		env.pop()
	case *luaparser.DoStmt:
		g.analyzeMonomorphicBlock(s.Body, env, params, returns)
	case *luaparser.NumericForStmt:
		initKind := g.analyzeMonomorphicExpr(s.Init, env, params)
		limitKind := g.analyzeMonomorphicExpr(s.Limit, env, params)
		stepKind := g.analyzeMonomorphicExpr(s.Step, env, params)
		env.push()
		kind := monoValue
		if initKind == monoNumber && limitKind == monoNumber && stepKind == monoNumber && !numericForVariableReassigned(s) {
			kind = monoNumber
		}
		env.declare(s.Name, kind)
		g.analyzeMonomorphicBlock(s.Body, env, params, returns)
		env.pop()
	case *luaparser.GenericForStmt:
		for _, value := range s.Values {
			g.analyzeMonomorphicExpr(value, env, params)
		}
		env.push()
		for _, name := range s.Names {
			env.declare(name, monoValue)
		}
		g.analyzeMonomorphicBlock(s.Body, env, params, returns)
		env.pop()
	case *luaparser.ReturnStmt:
		n := 0
		if returns != nil {
			n = len(returns.allNumber)
		}
		kinds := g.analyzeMonomorphicValues(s.Values, n, env, params)
		if returns != nil {
			returns.seen = true
			for i, kind := range kinds {
				if kind != monoNumber {
					returns.allNumber[i] = false
				}
			}
		}
	}
}

func (g *generator) analyzeMonomorphicBlock(block *luaparser.Block, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation, returns *functionReturnObservation) {
	env.push()
	for _, stmt := range block.Body {
		g.analyzeMonomorphicStmt(stmt, env, params, returns, false)
	}
	env.pop()
}

// analyzeMonomorphicValues applies Lua's expression-list width rules while
// retaining the inferred kind of each expanded result. A trailing call expands
// to its declared result arity; missing positions are nil and therefore remain
// Value.
func (g *generator) analyzeMonomorphicValues(values []luaparser.Expr, nWanted int, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation) []monoKind {
	out := make([]monoKind, nWanted)
	kinds := g.analyzeMonomorphicExprList(values, env, params)
	for i := 0; i < len(kinds) && i < len(out); i++ {
		out[i] = kinds[i]
	}
	return out
}

func (g *generator) analyzeMonomorphicExprList(values []luaparser.Expr, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation) []monoKind {
	out := make([]monoKind, 0, len(values))
	last := len(values) - 1
	for i, value := range values {
		kind := g.analyzeMonomorphicExpr(value, env, params)
		if i == last {
			if call, ok := value.(*luaparser.CallExpr); ok {
				if arity, known := g.callResultArity(call); known {
					if arity == 0 {
						continue
					}
					if fn := g.calledFunction(call); fn != nil && arity > 1 {
						for j := 0; j < fn.returnN; j++ {
							out = append(out, fn.returnKind(j))
						}
						continue
					}
				}
			}
		}
		out = append(out, kind)
	}
	return out
}

func (g *generator) analyzeMonomorphicExpr(expr luaparser.Expr, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation) monoKind {
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return g.analyzeMonomorphicExpr(x.Expr, env, params)
	case *luaparser.NumberExpr:
		return monoNumber
	case *luaparser.VarExpr:
		return env.lookup(x.Name)
	case *luaparser.FieldExpr:
		g.analyzeMonomorphicExpr(x.Base, env, params)
		if _, _, ok := g.numericRecordField(x); ok {
			return monoNumber
		}
		if !g.opts.DisableStableTableFields && g.numericFields[x.Name] {
			return monoNumber
		}
	case *luaparser.IndexExpr:
		g.analyzeMonomorphicExpr(x.Key, env, params)
		if base, ok := x.Base.(*luaparser.VarExpr); ok && g.numericArrays[base.Name] != nil {
			return monoNumber
		}
		g.analyzeMonomorphicExpr(x.Base, env, params)
	case *luaparser.UnaryExpr:
		if x.Op == luaparser.OpLength {
			if base, ok := x.Expr.(*luaparser.VarExpr); ok && g.numericArrays[base.Name] != nil {
				return monoNumber
			}
		}
		kind := g.analyzeMonomorphicExpr(x.Expr, env, params)
		if x.Op == luaparser.OpLength && !g.opts.DisableNumericLengths {
			return monoNumber
		}
		if kind == monoNumber && (x.Op == luaparser.OpNegate || x.Op == luaparser.OpBitNot) {
			return monoNumber
		}
	case *luaparser.BinaryExpr:
		left := g.analyzeMonomorphicExpr(x.Left, env, params)
		right := g.analyzeMonomorphicExpr(x.Right, env, params)
		if left == monoNumber && right == monoNumber {
			switch x.Op {
			case luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
				luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo, luaparser.OpPower,
				luaparser.OpBitOr, luaparser.OpBitXor, luaparser.OpBitAnd,
				luaparser.OpShiftL, luaparser.OpShiftR, luaparser.OpShiftRLogical,
				luaparser.OpRotateL, luaparser.OpRotateR:
				return monoNumber
			}
		}
	case *luaparser.CallExpr:
		kinds := make([]monoKind, 0, len(x.Args)+1)
		if x.Method != "" {
			kinds = append(kinds, g.analyzeMonomorphicExpr(x.Callee, env, params))
		}
		kinds = append(kinds, g.analyzeMonomorphicExprList(x.Args, env, params)...)
		if fn := g.calledFunction(x); fn != nil {
			observations := params[fn]
			for i := range observations {
				observations[i].seen = true
				if i >= len(kinds) || kinds[i] != monoNumber {
					observations[i].allNumber = false
				}
			}
			params[fn] = observations
			if fn.returnN > 0 {
				return fn.returnKind(0)
			}
			return monoValue
		}
		name, ok := callKey(x)
		if !ok {
			return monoValue
		}
		// These builtins produce a number even when an argument is a generic
		// Lua Value: min/max/mid coerce with NumberOr(0), and mget always
		// returns a tile id (0 for invalid layers/coordinates). Preserve
		// argument observations above: only the result narrows.
		if name == "min" || name == "max" || name == "mid" || name == "mget" {
			return monoNumber
		}
		allNumber := true
		for _, kind := range kinds {
			if kind != monoNumber {
				allNumber = false
				break
			}
		}
		if allNumber {
			switch name {
			case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd",
				"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr", "t", "time", "stat":
				return monoNumber
			}
		}
	case *luaparser.TableExpr:
		for _, field := range x.Fields {
			if field.Key != nil {
				g.analyzeMonomorphicExpr(field.Key, env, params)
			}
			g.analyzeMonomorphicExpr(field.Value, env, params)
			if env.observeField != nil && !field.Array {
				if key, ok := field.Key.(*luaparser.StringExpr); ok {
					if env.numericFieldIntent(field.Value) {
						env.observeField(key.Value, true)
					} else if provenNonNumericFieldValue(field.Value) ||
						env.poisonUnknownFields && poisonUnknownFieldValue(field.Value) {
						env.observeField(key.Value, false)
					}
				}
			}
		}
	}
	return monoValue
}

func blockAlwaysReturns(block *luaparser.Block) bool {
	for _, stmt := range block.Body {
		if stmtAlwaysReturns(stmt) {
			return true
		}
	}
	return false
}

func stmtAlwaysReturns(stmt luaparser.Stmt) bool {
	switch s := stmt.(type) {
	case *luaparser.ReturnStmt:
		return true
	case *luaparser.DoStmt:
		return blockAlwaysReturns(s.Body)
	case *luaparser.IfStmt:
		if s.Else == nil || !blockAlwaysReturns(s.Then) || !blockAlwaysReturns(s.Else) {
			return false
		}
		for _, branch := range s.ElseIf {
			if !blockAlwaysReturns(branch.Body) {
				return false
			}
		}
		return true
	}
	return false
}

// analyzeNoAllocFunctions proves a deliberately narrow transitive effect:
// the function and every user callee execute without allocating from the Lua
// heap. Such calls cannot trigger collection, so the caller's live Values do
// not need to be copied to the temporary root stack around the call. Starting
// from proven leaves means recursive/unknown call cycles remain protected.
func (g *generator) analyzeNoAllocFunctions() {
	if g.opts.DisableNoAllocFunctionCalls {
		return
	}
	for {
		changed := false
		for _, fn := range g.funcList {
			if fn.noAlloc || !g.blockNoAlloc(fn.decl.Body) {
				continue
			}
			fn.noAlloc = true
			changed = true
		}
		if !changed {
			return
		}
	}
}

func (g *generator) blockNoAlloc(block *luaparser.Block) bool {
	for _, stmt := range block.Body {
		if !g.stmtNoAlloc(stmt) {
			return false
		}
	}
	return true
}

func (g *generator) stmtNoAlloc(stmt luaparser.Stmt) bool {
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		for _, target := range s.Targets {
			switch target := target.(type) {
			case *luaparser.VarExpr:
			case *luaparser.FieldExpr:
				// A first field write may insert/grow a table even when later
				// executions hit a stable cached slot. Scalar-replaced records
				// have no table identity or allocation left to observe.
				if _, _, scalar := g.numericRecordField(target); !scalar {
					return false
				}
			case *luaparser.IndexExpr:
				return false
			default:
				return false
			}
		}
		return g.exprsNoAlloc(s.Values)
	case *luaparser.LocalAssignStmt:
		return g.exprsNoAlloc(s.Values)
	case *luaparser.CallStmt:
		return g.exprNoAlloc(s.Call)
	case *luaparser.IfStmt:
		if !g.exprNoAlloc(s.Condition) || !g.blockNoAlloc(s.Then) {
			return false
		}
		for _, branch := range s.ElseIf {
			if !g.exprNoAlloc(branch.Condition) || !g.blockNoAlloc(branch.Body) {
				return false
			}
		}
		return s.Else == nil || g.blockNoAlloc(s.Else)
	case *luaparser.WhileStmt:
		return g.exprNoAlloc(s.Condition) && g.blockNoAlloc(s.Body)
	case *luaparser.RepeatStmt:
		return g.blockNoAlloc(s.Body) && g.exprNoAlloc(s.Condition)
	case *luaparser.DoStmt:
		return g.blockNoAlloc(s.Body)
	case *luaparser.NumericForStmt:
		return g.exprNoAlloc(s.Init) && g.exprNoAlloc(s.Limit) && g.exprNoAlloc(s.Step) && g.blockNoAlloc(s.Body)
	case *luaparser.GenericForStmt:
		return g.exprsNoAlloc(s.Values) && g.blockNoAlloc(s.Body)
	case *luaparser.ReturnStmt:
		return g.exprsNoAlloc(s.Values)
	case *luaparser.BreakStmt:
		return true
	}
	return false
}

// requiresHeap is intentionally stricter than merely finding a table literal.
// It reuses the transitive no-allocation proof and accounts only for the table
// forms that earlier passes completely scalar-replaced. Boxed globals can hold
// static strings, booleans and nil without a heap; they only need GC roots when
// some part of this closed program can allocate a heap value.
func (g *generator) requiresHeap() bool {
	for _, fn := range g.funcList {
		if !fn.noAlloc {
			return true
		}
	}
	for _, stmt := range g.chunk.Body {
		if _, declaration := stmt.(*luaparser.FunctionDeclStmt); declaration {
			continue
		}
		name, _, recordDeclaration := numericRecordDeclaration(stmt)
		if recordDeclaration {
			if info := g.numericRecords[name]; info != nil && info.declaration == stmt {
				continue
			}
		}
		if !g.stmtNoAlloc(stmt) {
			return true
		}
	}
	return false
}

func (g *generator) exprsNoAlloc(exprs []luaparser.Expr) bool {
	for _, expr := range exprs {
		if !g.exprNoAlloc(expr) {
			return false
		}
	}
	return true
}

func (g *generator) exprNoAlloc(expr luaparser.Expr) bool {
	switch x := expr.(type) {
	case nil, *luaparser.NilExpr, *luaparser.BoolExpr, *luaparser.NumberExpr,
		*luaparser.StringExpr, *luaparser.VarExpr:
		return true
	case *luaparser.ParenExpr:
		return g.exprNoAlloc(x.Expr)
	case *luaparser.UnaryExpr:
		return g.exprNoAlloc(x.Expr)
	case *luaparser.BinaryExpr:
		return x.Op != luaparser.OpConcat && g.exprNoAlloc(x.Left) && g.exprNoAlloc(x.Right)
	case *luaparser.IndexExpr:
		return g.exprNoAlloc(x.Base) && g.exprNoAlloc(x.Key)
	case *luaparser.FieldExpr:
		return g.exprNoAlloc(x.Base)
	case *luaparser.CallExpr:
		if !g.exprsNoAlloc(operandExprs(x)) {
			return false
		}
		if fn := g.calledFunction(x); fn != nil {
			return fn.noAlloc
		}
		name, ok := callKey(x)
		if !ok {
			return false
		}
		if name == "print" && !g.opts.DisableLiteralPrintSpecialization {
			// PrintV converts non-string values through Tostr, which allocates
			// from the Lua heap. A literal is interned as KindStaticString at
			// initialization and follows the allocation-free drawing path.
			// Mode 0 numbers use formatNumber on a stack buffer instead.
			if len(x.Args) == 0 {
				return false
			}
			if _, literal := unparenExpr(x.Args[0]).(*luaparser.StringExpr); literal {
				return true
			}
			return g.mode0HeaplessPrintNumber(x.Args[0])
		}
		return builtinNoAlloc(name)
	case *luaparser.TableExpr:
		return false
	}
	return false
}

// This allowlist is about custom Lua-heap allocation, not general side
// effects. Calls omitted here simply retain root protection.
func builtinNoAlloc(name string) bool {
	switch name {
	case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd", "srand",
		"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr",
		"btn", "btnp", "t", "time", "stat", "tonum", "ord", "printh",
		"sfx", "music", "sample", "gba_note", "gba_stop",
		"del", "deli", "count", "all", "pairs",
		"sget", "sset", "fget", "fset", "mget", "mset",
		"cls", "pset", "pget", "line", "rect", "rectfill", "circ", "circfill", "spr", "sspr", "pspr", "anim", "map",
		"camera", "clip", "pal", "palt", "pvariant", "color", "cursor":
		return true
	}
	return false
}

// validateStaticAudioCalls catches only IDs that are unambiguously numeric
// literals in a project that links audio assets. Dynamic expressions retain
// the runtime's bounded fail-closed behavior, and audio-less ports may keep
// compatibility calls without pulling in the audio runtime.
func (g *generator) validateStaticAudioCalls() error {
	if !g.nativeAudioEnabled && g.opts.NativeAudio == nil &&
		!chunkCallsBuiltin(g.chunk, "gba_note") && !chunkCallsBuiltin(g.chunk, "gba_stop") {
		return nil
	}
	var firstErr error
	var visitStmt func(luaparser.Stmt)
	visitStmt = func(stmt luaparser.Stmt) {
		if firstErr != nil {
			return
		}
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			for _, bodyStmt := range fn.Body.Body {
				visitStmt(bodyStmt)
			}
			return
		}
		collectStmtExprs(stmt, func(expr luaparser.Expr) {
			if firstErr != nil {
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
			argCount := g.exprListWidth(call.Args)
			expanded := argCount != len(call.Args)
			if g.opts.NativeAudio != nil {
				switch name {
				case "sample":
					firstErr = fmt.Errorf("transpiler: %s: sample() is not part of four-channel audio v2", call.Pos())
					return
				case "sfx":
					if argCount < 1 || argCount > 2 {
						firstErr = fmt.Errorf("transpiler: %s: audio v2 sfx() takes id and optional priority", call.Pos())
						return
					}
				case "music":
					if argCount != 1 {
						firstErr = fmt.Errorf("transpiler: %s: audio v2 music() takes exactly one id", call.Pos())
						return
					}
				case "gba_note":
					if argCount < 3 || argCount > 4 {
						firstErr = fmt.Errorf("transpiler: %s: gba_note() takes channel, note, sound id, and optional duration", call.Pos())
						return
					}
					if !expanded {
						g.validateStaticNativeNote(call, &firstErr)
					}
					return
				case "gba_stop":
					if argCount != 1 {
						firstErr = fmt.Errorf("transpiler: %s: gba_stop() takes one channel", call.Pos())
						return
					}
					if !expanded {
						if channel, literal := staticNumberLiteral(call.Args[0]); literal && (channel != math.Trunc(channel) || channel < 1 || channel > 4) {
							firstErr = fmt.Errorf("transpiler: %s: GBA channel must be an integer from 1 through 4", call.Pos())
						}
					}
					return
				}
			} else if name == "gba_note" || name == "gba_stop" {
				firstErr = fmt.Errorf("transpiler: %s: %s() requires four-channel audio v2 assets", call.Pos(), name)
				return
			}
			if (name != "sfx" && name != "music" && name != "sample") || argCount == 0 || expanded {
				return
			}
			value, literal := staticNumberLiteral(call.Args[0])
			if !literal {
				return
			}
			if value != math.Trunc(value) {
				firstErr = fmt.Errorf("transpiler: %s: %s ID must be an integer, got %g", call.Pos(), name, value)
				return
			}
			if value == -1 {
				return
			}
			if value < 0 || value > math.MaxInt32 || !g.hasAudioID(name, int(value)) {
				firstErr = fmt.Errorf("transpiler: %s: %s ID %g is not present in %s", call.Pos(), name, value, g.audioAssetPath(name))
			}
		})
	}
	for _, stmt := range g.chunk.Body {
		visitStmt(stmt)
	}
	return firstErr
}

func (g *generator) validateStaticNativeNote(call *luaparser.CallExpr, firstErr *error) {
	channelValue, channelStatic := staticNumberLiteral(call.Args[0])
	if channelStatic && (channelValue != math.Trunc(channelValue) || channelValue < 1 || channelValue > 4) {
		*firstErr = fmt.Errorf("transpiler: %s: GBA channel must be an integer from 1 through 4", call.Pos())
		return
	}
	noteValue, noteStatic := staticNumberLiteral(call.Args[1])
	if noteStatic && (noteValue != math.Trunc(noteValue) || noteValue < 0 || noteValue > 127) {
		*firstErr = fmt.Errorf("transpiler: %s: GBA note must be an integer from 0 through 127", call.Pos())
		return
	}
	soundValue, soundStatic := staticNumberLiteral(call.Args[2])
	if soundStatic && (soundValue != math.Trunc(soundValue) || soundValue < 0 || soundValue > 255) {
		*firstErr = fmt.Errorf("transpiler: %s: GBA sound ID must be an integer from 0 through 255", call.Pos())
		return
	}
	if soundStatic {
		var sound *audio.Sound
		for index := range g.opts.NativeAudio.Library.Sounds {
			candidate := &g.opts.NativeAudio.Library.Sounds[index]
			if int(candidate.ID) == int(soundValue) {
				sound = candidate
				break
			}
		}
		if sound == nil {
			*firstErr = fmt.Errorf("transpiler: %s: sound ID %g is not present in assets/audio/sounds.json", call.Pos(), soundValue)
			return
		}
		if channelStatic && int(sound.Kind)+1 != int(channelValue) {
			*firstErr = fmt.Errorf("transpiler: %s: sound ID %g belongs to CH%d, not CH%d", call.Pos(), soundValue, int(sound.Kind)+1, int(channelValue))
			return
		}
		if channelStatic && noteStatic && sound.Kind != audio.SoundCH4 {
			if _, ok := audio.NativeSoundPeriod(*sound, uint8(noteValue)); !ok {
				*firstErr = fmt.Errorf("transpiler: %s: note %g is outside CH%d hardware range", call.Pos(), noteValue, int(channelValue))
				return
			}
		}
	}
	if len(call.Args) == 4 {
		if duration, literal := staticNumberLiteral(call.Args[3]); literal && (duration != math.Trunc(duration) || duration < 0 || duration > 65535) {
			*firstErr = fmt.Errorf("transpiler: %s: duration must be an integer from 0 through 65535", call.Pos())
		}
	}
}

func staticNumberLiteral(expr luaparser.Expr) (float64, bool) {
	switch value := expr.(type) {
	case *luaparser.ParenExpr:
		return staticNumberLiteral(value.Expr)
	case *luaparser.NumberExpr:
		return value.Value, true
	case *luaparser.UnaryExpr:
		if value.Op != luaparser.OpNegate {
			return 0, false
		}
		if number, ok := staticNumberLiteral(value.Expr); ok {
			return -number, true
		}
	}
	return 0, false
}

func unparenExpr(expr luaparser.Expr) luaparser.Expr {
	for {
		paren, ok := expr.(*luaparser.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.Expr
	}
}

func (g *generator) hasAudioID(name string, id int) bool {
	if g.opts.NativeAudio != nil {
		switch name {
		case "sfx":
			for _, clip := range g.opts.NativeAudio.Library.Clips {
				if int(clip.ID) == id {
					return true
				}
			}
		case "music":
			for _, song := range g.opts.NativeAudio.Library.Songs {
				if int(song.ID) == id {
					return true
				}
			}
		}
		return false
	}
	return false
}

func (g *generator) audioAssetPath(name string) string {
	if g.opts.NativeAudio != nil {
		if name == "music" || name == "sfx" {
			return "assets/audio/music.json"
		}
		return "four-channel audio v2"
	}
	return "four-channel audio v2"
}

// chunkCallsBuiltin reports whether the source directly calls a builtin.
// Mtile has no first-class functions or closures, so every sset mutation is
// syntactically visible in the AST and immutable-sheet selection is exact.
func chunkCallsBuiltin(chunk *luaparser.Chunk, name string) bool {
	found := false
	var visitStmt func(luaparser.Stmt)
	visitStmt = func(stmt luaparser.Stmt) {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			for _, bodyStmt := range fn.Body.Body {
				visitStmt(bodyStmt)
			}
			return
		}
		collectStmtExprs(stmt, func(e luaparser.Expr) {
			call, ok := e.(*luaparser.CallExpr)
			if !ok {
				return
			}
			if key, ok := callKey(call); ok && key == name {
				found = true
			}
		})
	}
	for _, stmt := range chunk.Body {
		visitStmt(stmt)
	}
	return found
}

func functionCallsBuiltin(fn *functionInfo, name string) bool {
	if fn == nil || fn.decl == nil {
		return false
	}
	found := false
	for _, stmt := range fn.decl.Body.Body {
		collectStmtExprs(stmt, func(e luaparser.Expr) {
			call, ok := e.(*luaparser.CallExpr)
			if !ok {
				return
			}
			if key, ok := callKey(call); ok && key == name {
				found = true
			}
		})
	}
	return found
}

func (g *generator) compactSheet() {
	g.sheetCompact, g.sheetIndex = assetsync.CompactSheetCells(g.opts.Sheet, g.opts.SheetW, g.spriteN)
}

func (g *generator) compactObjSheet() {
	if g.opts.ObjSheetW < 8 || g.opts.ObjSheetH < 8 {
		return
	}
	cols := g.opts.ObjSheetW / 8
	rows := g.opts.ObjSheetH / 8
	g.objSpriteN = cols * rows
	g.objSheetCompact, g.objSheetIndex = assetsync.CompactObjectSheetCells(g.opts.ObjSheet, g.opts.ObjSheetW, g.objSpriteN)
}

func functionKey(fn *luaparser.FunctionDeclStmt) string {
	key := strings.Join(fn.Name, ".")
	if fn.Method != "" {
		key += ":" + fn.Method
	}
	return key
}

func functionGoName(key string) string {
	switch key {
	case "_init":
		return "gluInit"
	case "_update":
		return "gluUpdate"
	case "_draw":
		return "gluDraw"
	default:
		return goIdent(strings.NewReplacer(".", "_", ":", "_").Replace(key))
	}
}

func (g *generator) discoverFunctions() error {
	for _, stmt := range g.chunk.Body {
		fn, ok := stmt.(*luaparser.FunctionDeclStmt)
		if !ok {
			continue
		}
		key := functionKey(fn)
		if _, exists := g.funcs[key]; exists {
			return fmt.Errorf("transpiler: duplicate function %q", key)
		}
		if fn.Vararg {
			return fmt.Errorf("transpiler: variadic function %q is unsupported", key)
		}
		info := &functionInfo{decl: fn, key: key, goName: functionGoName(key)}
		info.params = append([]string(nil), fn.Params...)
		info.paramKinds = make([]monoKind, len(fn.Params))
		// Start all known user functions at zero results. Return arity is
		// resolved later from syntax and the arities of trailing user calls.
		info.returnN = 0
		g.funcs[key] = info
		g.funcList = append(g.funcList, info)
	}
	return nil
}

// validateSemantics rejects source shapes which the static Mtile dialect
// cannot represent without silently changing Lua name binding or numeric
// behavior. Calls are intentionally static: a lexical binding may not shadow
// a user function/builtin at a call site, and callable names cannot be used as
// first-class values. Top-level locals remain Game fields, but only functions
// declared after them may capture them.
func (g *generator) validateSemantics() error {
	allTopLocals := make(map[string]luaparser.Position)
	namespaceDeclOrder := make(map[string]int)
	namespaceChildren := make(map[string]map[string]bool)
	callableMembers := make(map[string]map[string]bool)
	for order, stmt := range g.chunk.Body {
		if local, ok := stmt.(*luaparser.LocalAssignStmt); ok {
			for _, name := range local.Names {
				if previous, exists := allTopLocals[name]; exists {
					return fmt.Errorf("transpiler: %s: top-level local %q redeclaration is unsupported because the generated Game state has one slot (first declared at %s)", local.Pos(), name, previous)
				}
				allTopLocals[name] = local.Pos()
			}
		}
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok && (len(fn.Name) > 1 || fn.Method != "") {
			key := functionKey(fn)
			separator := strings.LastIndexAny(key, ".:")
			if separator > 0 && separator+1 < len(key) {
				namespace, member := key[:separator], key[separator+1:]
				// Every prefix is part of the statically lowered callable
				// namespace. Replacing a nested prefix such as actor.body after
				// declaring actor.body:draw() would change Lua dispatch while the
				// generated direct call would keep invoking the old function.
				for prefix := namespace; prefix != ""; {
					if _, exists := namespaceDeclOrder[prefix]; !exists {
						namespaceDeclOrder[prefix] = order
					}
					dot := strings.LastIndexByte(prefix, '.')
					if dot < 0 {
						break
					}
					prefix = prefix[:dot]
				}
				if callableMembers[namespace] == nil {
					callableMembers[namespace] = make(map[string]bool)
				}
				callableMembers[namespace][member] = true
			}
		}
	}
	for path := range namespaceDeclOrder {
		if dot := strings.LastIndexByte(path, '.'); dot >= 0 {
			parent, child := path[:dot], path[dot+1:]
			if namespaceChildren[parent] == nil {
				namespaceChildren[parent] = make(map[string]bool)
			}
			namespaceChildren[parent][child] = true
		}
	}

	// Static lowering does not materialize Lua function values in their tables.
	// Consequently, a table backing a function namespace must not be shared
	// through an older alias: that alias could later replace a callable field in
	// Lua while generated direct calls kept invoking the original Go function.
	// Requiring a fresh constructor at every explicitly initialized namespace
	// boundary makes that invariant syntactically provable.
	var validateNamespaceInitializer func(string, luaparser.Expr, luaparser.Position) error
	validateNamespaceInitializer = func(path string, value luaparser.Expr, position luaparser.Position) error {
		if _, namespace := namespaceDeclOrder[path]; !namespace {
			return nil
		}
		table, fresh := unparenExpr(value).(*luaparser.TableExpr)
		if !fresh {
			return fmt.Errorf("transpiler: %s: static function namespace %q must be initialized with a fresh table constructor", position, path)
		}
		children := namespaceChildren[path]
		if len(children) == 0 {
			return nil
		}
		for _, field := range table.Fields {
			if field.Array {
				continue
			}
			switch key := unparenExpr(field.Key).(type) {
			case *luaparser.StringExpr:
				if children[key.Value] {
					if err := validateNamespaceInitializer(path+"."+key.Value, field.Value, field.Value.Pos()); err != nil {
						return err
					}
				}
			case *luaparser.NumberExpr, *luaparser.BoolExpr, *luaparser.NilExpr:
				// Literal non-string keys cannot name a dotted namespace child.
			default:
				return fmt.Errorf("transpiler: %s: dynamic initializer field may alias a nested static function namespace below %q", field.Pos(), path)
			}
		}
		return nil
	}

	type lexicalScopes []map[string]bool
	currentSelfNamespace := ""
	isLocal := func(scopes lexicalScopes, name string) bool {
		for i := len(scopes) - 1; i >= 0; i-- {
			if scopes[i][name] {
				return true
			}
		}
		return false
	}
	resolveStaticPath := func(scopes lexicalScopes, path string) (string, bool) {
		if currentSelfNamespace != "" && (path == "self" || strings.HasPrefix(path, "self.")) {
			return currentSelfNamespace + strings.TrimPrefix(path, "self"), true
		}
		root := path
		if dot := strings.IndexByte(root, '.'); dot >= 0 {
			root = root[:dot]
		}
		return path, !isLocal(scopes, root)
	}
	copyScope := func(source map[string]bool) map[string]bool {
		out := make(map[string]bool, len(source))
		for name := range source {
			out[name] = true
		}
		return out
	}
	lateLocalError := func(name string, use luaparser.Position) error {
		declaration := allTopLocals[name]
		return fmt.Errorf("transpiler: %s: top-level local %q is declared after this use/function and cannot share the generated global slot (declared at %s)", use, name, declaration)
	}

	var walkExpr func(luaparser.Expr, lexicalScopes, bool, bool) error
	walkExpr = func(expr luaparser.Expr, scopes lexicalScopes, directCallee, memberBase bool) error {
		if expr == nil {
			return nil
		}
		switch x := expr.(type) {
		case *luaparser.ParenExpr:
			// Parenthesizing a callable first evaluates it as a value, which the
			// static-call dialect deliberately does not support.
			return walkExpr(x.Expr, scopes, false, false)
		case *luaparser.VarExpr:
			if x.Name == "self" && currentSelfNamespace != "" && !memberBase && !directCallee {
				return fmt.Errorf("transpiler: %s: first-class function namespace %q is unsupported", x.Pos(), currentSelfNamespace)
			}
			if isLocal(scopes, x.Name) {
				if directCallee {
					return fmt.Errorf("transpiler: %s: dynamic call through local %q is unsupported", x.Pos(), x.Name)
				}
				return nil
			}
			if _, later := allTopLocals[x.Name]; later {
				return lateLocalError(x.Name, x.Pos())
			}
			if !directCallee {
				if _, callable := g.funcs[x.Name]; callable {
					return fmt.Errorf("transpiler: %s: first-class function value %q is unsupported", x.Pos(), x.Name)
				}
				if _, callable := Builtins[x.Name]; callable {
					return fmt.Errorf("transpiler: %s: first-class builtin value %q is unsupported", x.Pos(), x.Name)
				}
				if !memberBase {
					if _, namespace := namespaceDeclOrder[x.Name]; namespace {
						return fmt.Errorf("transpiler: %s: first-class function namespace %q is unsupported", x.Pos(), x.Name)
					}
				}
			}
		case *luaparser.UnaryExpr:
			return walkExpr(x.Expr, scopes, false, false)
		case *luaparser.BinaryExpr:
			if x.Op == luaparser.OpPower {
				exponent, literal := staticNumberLiteral(x.Right)
				if !literal || exponent != math.Trunc(exponent) {
					return fmt.Errorf("transpiler: %s: power requires an integer literal exponent", x.Right.Pos())
				}
			}
			if err := walkExpr(x.Left, scopes, false, false); err != nil {
				return err
			}
			return walkExpr(x.Right, scopes, false, false)
		case *luaparser.IndexExpr:
			if path, ok := staticMemberPath(x); ok && !memberBase {
				if path, static := resolveStaticPath(scopes, path); static {
					if _, namespace := namespaceDeclOrder[path]; namespace {
						return fmt.Errorf("transpiler: %s: first-class function namespace %q is unsupported", x.Pos(), path)
					}
				}
			}
			if base, ok := staticMemberPath(x.Base); ok {
				base, static := resolveStaticPath(scopes, base)
				if static {
					if key, literal := unparenExpr(x.Key).(*luaparser.StringExpr); literal {
						path := base + "." + key.Value
						if callableMembers[base][key.Value] {
							return fmt.Errorf("transpiler: %s: first-class function value %q is unsupported", x.Pos(), path)
						}
					} else if _, namespace := namespaceDeclOrder[base]; namespace {
						return fmt.Errorf("transpiler: %s: dynamic indexed access to static function namespace %q is unsupported", x.Pos(), base)
					}
				}
			}
			if err := walkExpr(x.Base, scopes, false, true); err != nil {
				return err
			}
			return walkExpr(x.Key, scopes, false, false)
		case *luaparser.FieldExpr:
			if directCallee {
				if path, ok := calleePath(x); ok {
					root := strings.SplitN(path, ".", 2)[0]
					if isLocal(scopes, root) {
						return fmt.Errorf("transpiler: %s: dynamic call through local %q is unsupported", x.Pos(), root)
					}
					if _, later := allTopLocals[root]; later {
						return lateLocalError(root, x.Pos())
					}
					return nil
				}
			}
			if path, ok := staticMemberPath(x); ok {
				path, static := resolveStaticPath(scopes, path)
				if !static {
					return walkExpr(x.Base, scopes, false, true)
				}
				if _, callable := g.funcs[path]; callable {
					return fmt.Errorf("transpiler: %s: first-class function value %q is unsupported", x.Pos(), path)
				}
				if dot := strings.LastIndexByte(path, '.'); dot >= 0 {
					method := path[:dot] + ":" + path[dot+1:]
					if _, callable := g.funcs[method]; callable {
						return fmt.Errorf("transpiler: %s: first-class function value %q is unsupported", x.Pos(), path)
					}
				}
				if !memberBase {
					if _, namespace := namespaceDeclOrder[path]; namespace {
						return fmt.Errorf("transpiler: %s: first-class function namespace %q is unsupported", x.Pos(), path)
					}
				}
			}
			return walkExpr(x.Base, scopes, false, true)
		case *luaparser.CallExpr:
			if err := walkExpr(x.Callee, scopes, true, false); err != nil {
				return err
			}
			for _, argument := range x.Args {
				if err := walkExpr(argument, scopes, false, false); err != nil {
					return err
				}
			}
		case *luaparser.TableExpr:
			for _, field := range x.Fields {
				if err := walkExpr(field.Key, scopes, false, false); err != nil {
					return err
				}
				if err := walkExpr(field.Value, scopes, false, false); err != nil {
					return err
				}
			}
		}
		return nil
	}

	var walkBlock func(*luaparser.Block, lexicalScopes, bool) error
	var walkStmt func(luaparser.Stmt, lexicalScopes) error
	currentTopOrder := -1
	insideFunction := false
	walkBlock = func(block *luaparser.Block, scopes lexicalScopes, newScope bool) error {
		if newScope {
			scopes = append(scopes, map[string]bool{})
		}
		for _, stmt := range block.Body {
			if err := walkStmt(stmt, scopes); err != nil {
				return err
			}
		}
		return nil
	}
	walkStmt = func(stmt luaparser.Stmt, scopes lexicalScopes) error {
		walkValues := func(values []luaparser.Expr) error {
			for _, value := range values {
				if err := walkExpr(value, scopes, false, false); err != nil {
					return err
				}
			}
			return nil
		}
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			targets := s.Targets
			if s.CompoundTarget != nil {
				targets = []luaparser.Expr{s.CompoundTarget}
			}
			for _, target := range targets {
				switch target := target.(type) {
				case *luaparser.VarExpr:
					if target.Name == "self" && currentSelfNamespace != "" {
						return fmt.Errorf("transpiler: %s: reassignment of method receiver self is unsupported", target.Pos())
					}
					if !isLocal(scopes, target.Name) {
						if _, later := allTopLocals[target.Name]; later {
							return lateLocalError(target.Name, target.Pos())
						}
						if _, callable := g.funcs[target.Name]; callable {
							return fmt.Errorf("transpiler: %s: assignment to static function name %q is unsupported", target.Pos(), target.Name)
						}
						if _, callable := Builtins[target.Name]; callable {
							return fmt.Errorf("transpiler: %s: assignment to static builtin name %q is unsupported", target.Pos(), target.Name)
						}
						if declarationOrder, namespace := namespaceDeclOrder[target.Name]; namespace &&
							(insideFunction || currentTopOrder > declarationOrder) {
							return fmt.Errorf("transpiler: %s: assignment to static function namespace %q after its declaration is unsupported", target.Pos(), target.Name)
						}
					}
				case *luaparser.FieldExpr:
					if path, ok := staticMemberPath(target); ok {
						path, static := resolveStaticPath(scopes, path)
						if !static {
							if err := walkExpr(target.Base, scopes, false, true); err != nil {
								return err
							}
							continue
						}
						if _, callable := g.funcs[path]; callable {
							return fmt.Errorf("transpiler: %s: assignment to static function field %q is unsupported", target.Pos(), path)
						}
						if dot := strings.LastIndexByte(path, '.'); dot >= 0 {
							method := path[:dot] + ":" + path[dot+1:]
							if _, callable := g.funcs[method]; callable {
								return fmt.Errorf("transpiler: %s: assignment to static method field %q is unsupported", target.Pos(), path)
							}
						}
						if declarationOrder, namespace := namespaceDeclOrder[path]; namespace &&
							(insideFunction || currentTopOrder > declarationOrder) {
							return fmt.Errorf("transpiler: %s: assignment to static function namespace %q after its declaration is unsupported", target.Pos(), path)
						}
					}
					if err := walkExpr(target.Base, scopes, false, true); err != nil {
						return err
					}
				case *luaparser.IndexExpr:
					if base, ok := staticMemberPath(target.Base); ok {
						base, static := resolveStaticPath(scopes, base)
						if !static {
							if err := walkExpr(target.Base, scopes, false, true); err != nil {
								return err
							}
							if err := walkExpr(target.Key, scopes, false, false); err != nil {
								return err
							}
							continue
						}
						if key, literal := unparenExpr(target.Key).(*luaparser.StringExpr); literal {
							path := base + "." + key.Value
							if _, callable := g.funcs[path]; callable {
								return fmt.Errorf("transpiler: %s: assignment to static function field %q is unsupported", target.Pos(), path)
							}
							method := base + ":" + key.Value
							if _, callable := g.funcs[method]; callable {
								return fmt.Errorf("transpiler: %s: assignment to static method field %q is unsupported", target.Pos(), path)
							}
							if declarationOrder, namespace := namespaceDeclOrder[path]; namespace &&
								(insideFunction || currentTopOrder > declarationOrder) {
								return fmt.Errorf("transpiler: %s: assignment to static function namespace %q after its declaration is unsupported", target.Pos(), path)
							}
						} else if declarationOrder, namespace := namespaceDeclOrder[base]; namespace &&
							(insideFunction || currentTopOrder > declarationOrder) {
							return fmt.Errorf("transpiler: %s: dynamic indexed assignment to static function namespace %q is unsupported", target.Pos(), base)
						}
					}
					if err := walkExpr(target.Base, scopes, false, true); err != nil {
						return err
					}
					if err := walkExpr(target.Key, scopes, false, false); err != nil {
						return err
					}
				}
			}
			if s.CompoundTarget == nil && !insideFunction {
				for index, target := range targets {
					path, static := staticMemberPath(target)
					if !static {
						continue
					}
					path, static = resolveStaticPath(scopes, path)
					declarationOrder, namespace := namespaceDeclOrder[path]
					if !static || !namespace || currentTopOrder > declarationOrder {
						continue
					}
					var value luaparser.Expr
					if index < len(s.Values) {
						value = s.Values[index]
					}
					if err := validateNamespaceInitializer(path, value, target.Pos()); err != nil {
						return err
					}
				}
			}
			return walkValues(s.Values)
		case *luaparser.LocalAssignStmt:
			if s.Synthetic {
				// Compound lowering creates block-local aliases solely to evaluate
				// an l-value base once. They cannot escape; the companion synthetic
				// assignment validates CompoundTarget, the original source l-value.
				for _, value := range s.Values {
					if err := walkExpr(value, scopes, false, true); err != nil {
						return err
					}
				}
			} else if err := walkValues(s.Values); err != nil {
				return err
			}
			for _, name := range s.Names {
				if name == "self" && currentSelfNamespace != "" {
					return fmt.Errorf("transpiler: %s: shadowing method receiver self is unsupported", s.Pos())
				}
				scopes[len(scopes)-1][name] = true
			}
		case *luaparser.CallStmt:
			return walkExpr(s.Call, scopes, false, false)
		case *luaparser.IfStmt:
			if err := walkExpr(s.Condition, scopes, false, false); err != nil {
				return err
			}
			if err := walkBlock(s.Then, scopes, true); err != nil {
				return err
			}
			for _, branch := range s.ElseIf {
				if err := walkExpr(branch.Condition, scopes, false, false); err != nil {
					return err
				}
				if err := walkBlock(branch.Body, scopes, true); err != nil {
					return err
				}
			}
			if s.Else != nil {
				return walkBlock(s.Else, scopes, true)
			}
		case *luaparser.WhileStmt:
			if err := walkExpr(s.Condition, scopes, false, false); err != nil {
				return err
			}
			return walkBlock(s.Body, scopes, true)
		case *luaparser.RepeatStmt:
			repeatScopes := append(scopes, map[string]bool{})
			if err := walkBlock(s.Body, repeatScopes, false); err != nil {
				return err
			}
			return walkExpr(s.Condition, repeatScopes, false, false)
		case *luaparser.DoStmt:
			return walkBlock(s.Body, scopes, true)
		case *luaparser.NumericForStmt:
			if err := walkExpr(s.Init, scopes, false, false); err != nil {
				return err
			}
			if err := walkExpr(s.Limit, scopes, false, false); err != nil {
				return err
			}
			if err := walkExpr(s.Step, scopes, false, false); err != nil {
				return err
			}
			if s.Name == "self" && currentSelfNamespace != "" {
				return fmt.Errorf("transpiler: %s: shadowing method receiver self is unsupported", s.Pos())
			}
			loopScopes := append(scopes, map[string]bool{s.Name: true})
			return walkBlock(s.Body, loopScopes, true)
		case *luaparser.GenericForStmt:
			if err := walkValues(s.Values); err != nil {
				return err
			}
			bindings := map[string]bool{}
			for _, name := range s.Names {
				if name == "self" && currentSelfNamespace != "" {
					return fmt.Errorf("transpiler: %s: shadowing method receiver self is unsupported", s.Pos())
				}
				bindings[name] = true
			}
			return walkBlock(s.Body, append(scopes, bindings), true)
		case *luaparser.ReturnStmt:
			return walkValues(s.Values)
		}
		return nil
	}

	topVisible := map[string]bool{}
	for order, stmt := range g.chunk.Body {
		currentTopOrder = order
		if local, ok := stmt.(*luaparser.LocalAssignStmt); ok {
			for index, name := range local.Names {
				declarationOrder, namespace := namespaceDeclOrder[name]
				if !namespace || order > declarationOrder {
					continue
				}
				var value luaparser.Expr
				if index < len(local.Values) {
					value = local.Values[index]
				}
				if err := validateNamespaceInitializer(name, value, local.Pos()); err != nil {
					return err
				}
			}
		}
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			if fn.Local {
				return fmt.Errorf("transpiler: %s: local function declarations are unsupported; use a top-level function declaration", fn.Pos())
			}
			functionScope := copyScope(topVisible)
			for _, parameter := range fn.Params {
				functionScope[parameter] = true
			}
			currentSelfNamespace = ""
			if fn.Method != "" {
				currentSelfNamespace = strings.Join(fn.Name, ".")
				for _, parameter := range fn.Params[1:] {
					if parameter == "self" {
						return fmt.Errorf("transpiler: %s: shadowing method receiver self is unsupported", fn.Pos())
					}
				}
			}
			insideFunction = true
			err := walkBlock(fn.Body, lexicalScopes{functionScope}, false)
			insideFunction = false
			currentSelfNamespace = ""
			if err != nil {
				return err
			}
			continue
		}
		if err := walkStmt(stmt, lexicalScopes{topVisible}); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) discoverGlobals() {
	g.collectChunkGlobals()
	for _, info := range g.funcList {
		g.current = info
		g.collectBlockGlobals(info.decl.Body)
	}
	g.current = nil
}

// collectChunkGlobals mirrors chunk code generation: only locals declared
// directly in the chunk become persistent Game fields. Locals inside a
// top-level do/if/loop are ordinary lexical stack values and must not inflate
// the global report or consume runtime global-root slots.
func (g *generator) collectChunkGlobals() {
	scopes := []map[string]bool{}
	isLocal := func(name string) bool {
		for i := len(scopes) - 1; i >= 0; i-- {
			if scopes[i][name] {
				return true
			}
		}
		return false
	}
	add := func(name string, position luaparser.Position) {
		if !isLocal(name) {
			g.addGlobal(name, position)
		}
	}
	var visitBlock func(*luaparser.Block, bool)
	var visitStmt func(luaparser.Stmt)
	visitBlock = func(block *luaparser.Block, newScope bool) {
		if newScope {
			scopes = append(scopes, map[string]bool{})
		}
		for _, stmt := range block.Body {
			visitStmt(stmt)
		}
		if newScope {
			scopes = scopes[:len(scopes)-1]
		}
	}
	visitStmt = func(stmt luaparser.Stmt) {
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			collectStmt(s, add, isLocal, g.funcs, Builtins)
		case *luaparser.LocalAssignStmt:
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
			if len(scopes) > 0 {
				for _, name := range s.Names {
					scopes[len(scopes)-1][name] = true
				}
			}
		case *luaparser.CallStmt:
			collectExpr(s.Call, add, isLocal, g.funcs, Builtins)
		case *luaparser.IfStmt:
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			visitBlock(s.Then, true)
			for _, branch := range s.ElseIf {
				collectExpr(branch.Condition, add, isLocal, g.funcs, Builtins)
				visitBlock(branch.Body, true)
			}
			if s.Else != nil {
				visitBlock(s.Else, true)
			}
		case *luaparser.WhileStmt:
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			visitBlock(s.Body, true)
		case *luaparser.RepeatStmt:
			scopes = append(scopes, map[string]bool{})
			visitBlock(s.Body, false)
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.DoStmt:
			visitBlock(s.Body, true)
		case *luaparser.NumericForStmt:
			collectExpr(s.Init, add, isLocal, g.funcs, Builtins)
			collectExpr(s.Limit, add, isLocal, g.funcs, Builtins)
			collectExpr(s.Step, add, isLocal, g.funcs, Builtins)
			scopes = append(scopes, map[string]bool{s.Name: true})
			visitBlock(s.Body, true)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.GenericForStmt:
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
			bindings := map[string]bool{}
			for _, name := range s.Names {
				bindings[name] = true
			}
			scopes = append(scopes, bindings)
			visitBlock(s.Body, true)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.ReturnStmt:
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
		}
	}
	for _, stmt := range g.chunk.Body {
		if _, function := stmt.(*luaparser.FunctionDeclStmt); function {
			continue
		}
		if local, ok := stmt.(*luaparser.LocalAssignStmt); ok {
			for _, value := range local.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
			for _, name := range local.Names {
				g.addGlobal(name, local.Pos())
			}
			continue
		}
		visitStmt(stmt)
	}
}

func (g *generator) globalReport() GlobalReport {
	report := GlobalReport{Count: len(g.globals), Limit: GlobalLimit, Symbols: make([]GlobalSymbol, 0, len(g.globals))}
	for _, name := range g.globals {
		report.Symbols = append(report.Symbols, GlobalSymbol{Name: name, Position: g.globalOrigins[name]})
	}
	return report
}

// resolveReturnArities expands `return f()` / `return 1, f()` using each
// callee's current arity until the set of function arities is stable. A
// function that only `return pair()` must have pair's full result count, not
// 1, or `local a, b = wrap()` would nil-fill b. Starting every known function
// at zero also lets a trailing zero-result call remove its syntactic call slot.
func (g *generator) resolveReturnArities() error {
	for pass := 0; ; pass++ {
		changed := false
		var changedInfo *functionInfo
		for _, info := range g.funcList {
			n := g.adjustedReturnN(info.decl.Body)
			if n != info.returnN {
				info.returnN = n
				changed = true
				changedInfo = info
			}
		}
		if !changed {
			return nil
		}
		// With N functions, every finite return-width dependency reaches its
		// fixed point after at most N changing passes. A later change proves a
		// positive recursive cycle such as `return n, expand(n-1)`, whose
		// statically generated Go tuple would grow without bound.
		if pass >= len(g.funcList) {
			return fmt.Errorf("transpiler: recursive return arity expands without bound in function %q at %s", changedInfo.key, changedInfo.decl.Pos())
		}
	}
}

func (g *generator) validateTableLiteralLimits() error {
	var firstErr error
	check := func(expr luaparser.Expr) {
		if firstErr != nil {
			return
		}
		table, ok := expr.(*luaparser.TableExpr)
		if !ok {
			return
		}
		arrayN, hashN := 0, 0
		for _, field := range table.Fields {
			if field.Array {
				arrayN++
			} else {
				hashN++
			}
		}
		if n := len(table.Fields); n > 0 && table.Fields[n-1].Array {
			if call, ok := table.Fields[n-1].Value.(*luaparser.CallExpr); ok {
				if arity, known := g.callResultArity(call); known {
					arrayN += arity - 1
				}
			}
		}
		if arrayN > runtimeapi.MaxTableArrayCapacity {
			firstErr = fmt.Errorf("transpiler: %s: table literal requires %d array slots, runtime limit is %d", table.Pos(), arrayN, runtimeapi.MaxTableArrayCapacity)
			return
		}
		if hashN > runtimeapi.MaxTableHashCapacity {
			firstErr = fmt.Errorf("transpiler: %s: table literal requires %d hash slots, runtime limit is %d", table.Pos(), hashN, runtimeapi.MaxTableHashCapacity)
		}
	}
	for _, stmt := range g.chunk.Body {
		collectStmtExprs(stmt, check)
	}
	for _, fn := range g.funcList {
		for _, stmt := range fn.decl.Body.Body {
			collectStmtExprs(stmt, check)
		}
	}
	return firstErr
}

// validateBuiltinCallArities runs after user-function return widths have
// reached their fixed point. That timing matters because a trailing call may
// contribute zero or multiple arguments to the surrounding builtin call.
// Keeping this out of lowering also prevents numeric optimized calls from
// bypassing public API validation.
func (g *generator) validateBuiltinCallArities() error {
	var firstErr error
	check := func(expr luaparser.Expr) {
		if firstErr != nil {
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
		spec, builtin := Builtins[name]
		if !builtin {
			return
		}
		got := g.exprListWidth(call.Args)
		if got < spec.MinArgs || got > spec.MaxArgs {
			firstErr = fmt.Errorf("transpiler: %s: %s expects %d..%d arguments, got %d", call.Pos(), name, spec.MinArgs, spec.MaxArgs, got)
		}
	}
	for _, stmt := range g.chunk.Body {
		collectStmtExprs(stmt, check)
	}
	for _, fn := range g.funcList {
		for _, stmt := range fn.decl.Body.Body {
			collectStmtExprs(stmt, check)
		}
	}
	return firstErr
}

func (g *generator) adjustedReturnN(block *luaparser.Block) int {
	maxN := 0
	var visit func([]luaparser.Stmt)
	visit = func(stmts []luaparser.Stmt) {
		for _, stmt := range stmts {
			switch s := stmt.(type) {
			case *luaparser.ReturnStmt:
				n := g.exprListWidth(s.Values)
				if n > maxN {
					maxN = n
				}
			case *luaparser.IfStmt:
				visit(s.Then.Body)
				for _, branch := range s.ElseIf {
					visit(branch.Body.Body)
				}
				if s.Else != nil {
					visit(s.Else.Body)
				}
			case *luaparser.WhileStmt:
				visit(s.Body.Body)
			case *luaparser.RepeatStmt:
				visit(s.Body.Body)
			case *luaparser.DoStmt:
				visit(s.Body.Body)
			case *luaparser.NumericForStmt:
				visit(s.Body.Body)
			case *luaparser.GenericForStmt:
				visit(s.Body.Body)
			}
		}
	}
	visit(block.Body)
	return maxN
}

func (g *generator) exprListWidth(values []luaparser.Expr) int {
	n := len(values)
	if n == 0 {
		return 0
	}
	call, ok := values[n-1].(*luaparser.CallExpr)
	if !ok {
		return n
	}
	arity, known := g.callResultArity(call)
	if !known {
		return n
	}
	return n - 1 + arity
}

func (g *generator) addGlobal(name string, position luaparser.Position) {
	if name == "" {
		return
	}
	if _, ok := g.globalGo[name]; ok {
		return
	}
	g.globalGo[name] = goIdent(name)
	g.globalOrigins[name] = position
	g.globals = append(g.globals, name)
}

func (g *generator) collectBlockGlobals(block *luaparser.Block) {
	// Global discovery must use the same lexical scopes as code emission. A
	// name introduced in a nested block is not local before or after that
	// block, and a local declaration is not visible in its own RHS. The old
	// whole-function name set misclassified both cases and could emit an
	// assignment to rt.NilVal().
	scopes := []map[string]bool{{}}
	for _, name := range g.current.params {
		scopes[0][name] = true
	}
	isLocal := func(name string) bool {
		for i := len(scopes) - 1; i >= 0; i-- {
			if scopes[i][name] {
				return true
			}
		}
		return false
	}
	add := func(name string, position luaparser.Position) {
		if !isLocal(name) {
			g.addGlobal(name, position)
		}
	}
	var visitBlock func(*luaparser.Block, bool)
	var visitStmt func(luaparser.Stmt)
	visitBlock = func(b *luaparser.Block, newScope bool) {
		if newScope {
			scopes = append(scopes, map[string]bool{})
		}
		for _, stmt := range b.Body {
			visitStmt(stmt)
		}
		if newScope {
			scopes = scopes[:len(scopes)-1]
		}
	}
	visitStmt = func(stmt luaparser.Stmt) {
		switch s := stmt.(type) {
		case *luaparser.AssignStmt:
			for _, target := range s.Targets {
				collectExpr(target, add, isLocal, g.funcs, Builtins)
			}
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
		case *luaparser.LocalAssignStmt:
			// The new names become visible only after all RHS expressions.
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
			for _, name := range s.Names {
				scopes[len(scopes)-1][name] = true
			}
		case *luaparser.CallStmt:
			collectExpr(s.Call, add, isLocal, g.funcs, Builtins)
		case *luaparser.IfStmt:
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			visitBlock(s.Then, true)
			for _, branch := range s.ElseIf {
				collectExpr(branch.Condition, add, isLocal, g.funcs, Builtins)
				visitBlock(branch.Body, true)
			}
			if s.Else != nil {
				visitBlock(s.Else, true)
			}
		case *luaparser.WhileStmt:
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			visitBlock(s.Body, true)
		case *luaparser.RepeatStmt:
			// Lua evaluates repeat's condition in the body scope.
			scopes = append(scopes, map[string]bool{})
			for _, bodyStmt := range s.Body.Body {
				visitStmt(bodyStmt)
			}
			collectExpr(s.Condition, add, isLocal, g.funcs, Builtins)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.DoStmt:
			visitBlock(s.Body, true)
		case *luaparser.NumericForStmt:
			collectExpr(s.Init, add, isLocal, g.funcs, Builtins)
			collectExpr(s.Limit, add, isLocal, g.funcs, Builtins)
			collectExpr(s.Step, add, isLocal, g.funcs, Builtins)
			scopes = append(scopes, map[string]bool{s.Name: true})
			visitBlock(s.Body, true)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.GenericForStmt:
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
			loopScope := map[string]bool{}
			for _, name := range s.Names {
				loopScope[name] = true
			}
			scopes = append(scopes, loopScope)
			visitBlock(s.Body, true)
			scopes = scopes[:len(scopes)-1]
		case *luaparser.ReturnStmt:
			for _, value := range s.Values {
				collectExpr(value, add, isLocal, g.funcs, Builtins)
			}
		}
	}
	visitBlock(block, false)
}

func (g *generator) collectStmtStatics(stmt luaparser.Stmt) {
	var clipNames map[*luaparser.StringExpr]bool
	collectStmtExprs(stmt, func(expr luaparser.Expr) {
		if call, ok := expr.(*luaparser.CallExpr); ok {
			if name, _ := callKey(call); name == "anim" {
				if _, literal := staticAnimationID(call.Args, g.opts); literal {
					if clipNames == nil {
						clipNames = make(map[*luaparser.StringExpr]bool)
					}
					clipNames[unparenExpr(call.Args[0]).(*luaparser.StringExpr)] = true
				}
			}
		}
		if str, ok := expr.(*luaparser.StringExpr); ok && clipNames[str] {
			return
		}
		g.collectExprStatic(expr)
	})
}

func (g *generator) collectBlockStatics(block *luaparser.Block) {
	for _, stmt := range block.Body {
		g.collectStmtStatics(stmt)
	}
}

func (g *generator) collectExprStatic(e luaparser.Expr) {
	switch x := e.(type) {
	case *luaparser.StringExpr:
		if g.numericRecordKeys[x] {
			return
		}
		g.staticValueAt(x.Value, x.Pos())
	case *luaparser.FieldExpr:
		if _, _, ok := g.numericRecordField(x); ok {
			return
		}
		if !g.opts.DisableStableTableFields {
			if _, exists := g.fieldCaches[x]; !exists {
				g.fieldCaches[x] = g.fieldCacheN
				g.fieldCacheN++
			}
		}
		g.staticValueAt(x.Name, x.Pos())
	}
}

func (g *generator) staticValue(s string) string {
	return g.staticValueAt(s, luaparser.Position{})
}

func (g *generator) staticValueAt(s string, position luaparser.Position) string {
	if name, ok := g.staticGo[s]; ok {
		return name
	}
	where := ""
	if position.Line != 0 {
		where = " at " + position.String()
	}
	if len(s) > runtimeapi.MaxStringBytes {
		g.setErr(fmt.Errorf("transpiler: static string has %d bytes, runtime limit is %d bytes%s", len(s), runtimeapi.MaxStringBytes, where))
		return ""
	}
	if len(g.statics) >= runtimeapi.MaxStaticStrings {
		g.setErr(fmt.Errorf("transpiler: cartridge requires more than %d static strings%s", runtimeapi.MaxStaticStrings, where))
		return ""
	}
	name := fmt.Sprintf("_gluStatic%d", len(g.statics))
	g.staticGo[s] = name
	g.statics = append(g.statics, s)
	return name
}

// collectStmt and collectBlock are deliberately separate from codegen. This
// means global discovery does not depend on traversal order or on whether a
// value is used as an l-value.
func collectBlock(block *luaparser.Block, add func(string, luaparser.Position), isLocal func(string) bool, funcs map[string]*functionInfo, builtins map[string]BuiltinSpec) {
	for _, stmt := range block.Body {
		collectStmt(stmt, add, isLocal, funcs, builtins)
	}
}

func collectStmt(stmt luaparser.Stmt, add func(string, luaparser.Position), isLocal func(string) bool, funcs map[string]*functionInfo, builtins map[string]BuiltinSpec) {
	addTarget := func(e luaparser.Expr) {
		switch x := e.(type) {
		case *luaparser.VarExpr:
			if !isLocal(x.Name) {
				add(x.Name, x.Pos())
			}
		case *luaparser.FieldExpr:
			collectExpr(x.Base, add, isLocal, funcs, builtins)
		case *luaparser.IndexExpr:
			collectExpr(x.Base, add, isLocal, funcs, builtins)
			collectExpr(x.Key, add, isLocal, funcs, builtins)
		}
	}
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		for _, t := range s.Targets {
			addTarget(t)
		}
		for _, v := range s.Values {
			collectExpr(v, add, isLocal, funcs, builtins)
		}
	case *luaparser.LocalAssignStmt:
		for _, v := range s.Values {
			collectExpr(v, add, isLocal, funcs, builtins)
		}
	case *luaparser.CallStmt:
		collectExpr(s.Call, add, isLocal, funcs, builtins)
	case *luaparser.IfStmt:
		collectExpr(s.Condition, add, isLocal, funcs, builtins)
		collectBlock(s.Then, add, isLocal, funcs, builtins)
		for _, b := range s.ElseIf {
			collectExpr(b.Condition, add, isLocal, funcs, builtins)
			collectBlock(b.Body, add, isLocal, funcs, builtins)
		}
		if s.Else != nil {
			collectBlock(s.Else, add, isLocal, funcs, builtins)
		}
	case *luaparser.WhileStmt:
		collectExpr(s.Condition, add, isLocal, funcs, builtins)
		collectBlock(s.Body, add, isLocal, funcs, builtins)
	case *luaparser.RepeatStmt:
		collectBlock(s.Body, add, isLocal, funcs, builtins)
		collectExpr(s.Condition, add, isLocal, funcs, builtins)
	case *luaparser.DoStmt:
		collectBlock(s.Body, add, isLocal, funcs, builtins)
	case *luaparser.NumericForStmt:
		collectExpr(s.Init, add, isLocal, funcs, builtins)
		collectExpr(s.Limit, add, isLocal, funcs, builtins)
		collectExpr(s.Step, add, isLocal, funcs, builtins)
		collectBlock(s.Body, add, isLocal, funcs, builtins)
	case *luaparser.GenericForStmt:
		for _, v := range s.Values {
			collectExpr(v, add, isLocal, funcs, builtins)
		}
		collectBlock(s.Body, add, isLocal, funcs, builtins)
	case *luaparser.ReturnStmt:
		for _, v := range s.Values {
			collectExpr(v, add, isLocal, funcs, builtins)
		}
	}
}

func collectExpr(e luaparser.Expr, add func(string, luaparser.Position), isLocal func(string) bool, funcs map[string]*functionInfo, builtins map[string]BuiltinSpec) {
	switch x := e.(type) {
	case *luaparser.ParenExpr:
		collectExpr(x.Expr, add, isLocal, funcs, builtins)
	case *luaparser.VarExpr:
		if !isLocal(x.Name) {
			if _, fn := funcs[x.Name]; !fn {
				if _, bi := builtins[x.Name]; !bi {
					add(x.Name, x.Pos())
				}
			}
		}
	case *luaparser.UnaryExpr:
		collectExpr(x.Expr, add, isLocal, funcs, builtins)
	case *luaparser.BinaryExpr:
		collectExpr(x.Left, add, isLocal, funcs, builtins)
		collectExpr(x.Right, add, isLocal, funcs, builtins)
	case *luaparser.IndexExpr:
		collectExpr(x.Base, add, isLocal, funcs, builtins)
		collectExpr(x.Key, add, isLocal, funcs, builtins)
	case *luaparser.FieldExpr:
		collectExpr(x.Base, add, isLocal, funcs, builtins)
	case *luaparser.CallExpr:
		if v, ok := x.Callee.(*luaparser.VarExpr); ok {
			if _, bi := builtins[v.Name]; !bi {
				if _, fn := funcs[v.Name]; !fn {
					if !isLocal(v.Name) {
						add(v.Name, v.Pos())
					}
				}
			}
		} else {
			collectExpr(x.Callee, add, isLocal, funcs, builtins)
		}
		for _, a := range x.Args {
			collectExpr(a, add, isLocal, funcs, builtins)
		}
	case *luaparser.TableExpr:
		for _, f := range x.Fields {
			if f.Key != nil {
				collectExpr(f.Key, add, isLocal, funcs, builtins)
			}
			collectExpr(f.Value, add, isLocal, funcs, builtins)
		}
	}
}

func collectStmtExprs(stmt luaparser.Stmt, add func(luaparser.Expr)) {
	var expr func(luaparser.Expr)
	expr = func(e luaparser.Expr) {
		if e == nil {
			return
		}
		add(e)
		switch x := e.(type) {
		case *luaparser.ParenExpr:
			expr(x.Expr)
		case *luaparser.UnaryExpr:
			expr(x.Expr)
		case *luaparser.BinaryExpr:
			expr(x.Left)
			expr(x.Right)
		case *luaparser.IndexExpr:
			expr(x.Base)
			expr(x.Key)
		case *luaparser.FieldExpr:
			expr(x.Base)
		case *luaparser.CallExpr:
			expr(x.Callee)
			for _, a := range x.Args {
				expr(a)
			}
		case *luaparser.TableExpr:
			for _, f := range x.Fields {
				expr(f.Key)
				expr(f.Value)
			}
		}
	}
	var stmtExpr func(luaparser.Stmt)
	stmtExpr = func(s luaparser.Stmt) {
		switch x := s.(type) {
		case *luaparser.AssignStmt:
			for _, t := range x.Targets {
				expr(t)
			}
			for _, v := range x.Values {
				expr(v)
			}
		case *luaparser.LocalAssignStmt:
			for _, v := range x.Values {
				expr(v)
			}
		case *luaparser.CallStmt:
			expr(x.Call)
		case *luaparser.IfStmt:
			expr(x.Condition)
			for _, b := range x.Then.Body {
				stmtExpr(b)
			}
			for _, b := range x.ElseIf {
				expr(b.Condition)
				for _, q := range b.Body.Body {
					stmtExpr(q)
				}
			}
			if x.Else != nil {
				for _, q := range x.Else.Body {
					stmtExpr(q)
				}
			}
		case *luaparser.WhileStmt:
			expr(x.Condition)
			for _, q := range x.Body.Body {
				stmtExpr(q)
			}
		case *luaparser.RepeatStmt:
			for _, q := range x.Body.Body {
				stmtExpr(q)
			}
			expr(x.Condition)
		case *luaparser.DoStmt:
			for _, q := range x.Body.Body {
				stmtExpr(q)
			}
		case *luaparser.NumericForStmt:
			expr(x.Init)
			expr(x.Limit)
			expr(x.Step)
			for _, q := range x.Body.Body {
				stmtExpr(q)
			}
		case *luaparser.GenericForStmt:
			for _, v := range x.Values {
				expr(v)
			}
			for _, q := range x.Body.Body {
				stmtExpr(q)
			}
		case *luaparser.ReturnStmt:
			for _, v := range x.Values {
				expr(v)
			}
		}
	}
	stmtExpr(stmt)
}

func (g *generator) mapRasterEligible() bool {
	return !g.opts.DisableMapRasterCache && g.opts.Renderer != RendererMode0 && !g.sheetMutable &&
		!g.opts.MapStreaming && g.opts.MapDrawMask != 0 && g.opts.MapW > 0 && g.opts.MapW <= 30 &&
		g.opts.MapH > 0 && g.opts.MapH <= 20 && len(g.mapCompact) == g.opts.MapW*g.opts.MapH
}

func (g *generator) render() string {
	g.packedMap = g.planPackedMap()
	var b sourceBuilder
	b.line("package " + g.opts.PackageName)
	b.line("")
	b.line("import (")
	b.line("\trt \"github.com/drpaneas/mtile/runtime\"")
	if g.nativeAudioEnabled {
		b.line("\t\"github.com/drpaneas/mtile/runtime/audio\"")
		b.line("\t\"github.com/drpaneas/mtile/runtime/hw\"")
	}
	if g.mapRasterEligible() || g.hasPackedMap() {
		b.line("\t\"unsafe\"")
	} else {
		b.line("\t_ \"unsafe\"")
	}
	b.line(")")
	b.line("")
	g.renderHelpers(&b)
	b.line("")
	g.renderGame(&b)
	b.line("")
	g.renderFunctions(&b)
	b.line("")
	g.renderTableImages(&b)
	g.renderMain(&b)
	return b.String()
}

func (g *generator) renderHelpers(b *sourceBuilder) {
	renderConstantDivisionHelpers(b)
	// This variadic root walker includes heap initialization and failure
	// cleanup. Keep it shared instead of duplicating those cold paths into
	// every IWRAM loop and generated allocation helper after LLVM inlining.
	b.line("//go:noinline")
	b.line("func gluPushRoots(c *rt.Context, roots ...rt.Value) int {")
	b.line("\tn := 0")
	b.line("\tfor _, root := range roots {")
	// Only these two kinds refer to moving heap objects. Scalar values and
	// ROM-backed static strings need no root-stack writes or later clearing.
	b.line("\t\tif root.Kind != rt.KindTable && root.Kind != rt.KindString { continue }")
	b.line("\t\tif !c.PushLocal(root) { c.PopLocals(n); return -1 }")
	b.line("\t\tn++")
	b.line("\t}")
	b.line("\treturn n")
	b.line("}")
	b.line("func gluPopRoots(c *rt.Context, n int) { if n > 0 { c.PopLocals(n) } }")
	b.line("func gluMust(v rt.Value, err rt.HeapError) rt.Value { if err != rt.HeapOK { return rt.NilVal() }; return v }")
	b.line("func gluCmp(c *rt.Context, a, z rt.Value, op string) bool {")
	b.line("\tcmp := 0")
	b.line("\tif a.Kind == rt.KindNumber && z.Kind == rt.KindNumber { if a.Num < z.Num { cmp = -1 } else if a.Num > z.Num { cmp = 1 } } else { var ok bool; cmp, ok = c.CompareStrings(a,z); if !ok { return false } }")
	b.line("\tswitch op { case \"<\": return cmp < 0; case \"<=\": return cmp <= 0; case \">\": return cmp > 0; case \">=\": return cmp >= 0 }; return false")
	b.line("}")
	b.line("func gluLen(c *rt.Context, v rt.Value) rt.Value { if v.Kind == rt.KindString || v.Kind == rt.KindStaticString { return rt.IntVal(c.StrLen(v)) }; if v.Kind == rt.KindTable { return rt.IntVal(c.TableLen(v)) }; return rt.IntVal(0) }")
	b.line("func gluNumberMod(a, z rt.Number) rt.Number { return a.Mod(z) }")
	b.line("func gluMod(a, z rt.Value) rt.Value { if a.Kind != rt.KindNumber || z.Kind != rt.KindNumber { return rt.NilVal() }; return rt.NumVal(gluNumberMod(a.Num,z.Num)) }")
	b.line("func gluNumberPow(a, z rt.Number) rt.Number { n := z.Int(); if n == 0 { return rt.One }; neg := n < 0; if neg { n = -n }; out := rt.One; for n > 0 { if n&1 != 0 { out = out.Mul(a) }; a = a.Mul(a); n >>= 1 }; if neg { out = rt.One.Div(out) }; return out }")
	b.line("func gluPow(a, z rt.Value) rt.Value { if a.Kind != rt.KindNumber || z.Kind != rt.KindNumber { return rt.NilVal() }; return rt.NumVal(gluNumberPow(a.Num,z.Num)) }")
	b.line("func gluConcat(c *rt.Context, a, z rt.Value, roots ...rt.Value) rt.Value { n := gluPushRoots(c, a, z); if n < 0 { return rt.NilVal() }; m := gluPushRoots(c, roots...); if m < 0 { gluPopRoots(c,n); return rt.NilVal() }; v, err := c.Concat(a,z); gluPopRoots(c,n+m); return gluMust(v,err) }")
	b.line("func gluSub(c *rt.Context, s, i, j rt.Value, roots ...rt.Value) rt.Value { n := gluPushRoots(c,s,i,j); if n < 0 { return rt.NilVal() }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,n); return rt.NilVal() }; v,err := c.Sub(s,i,j); gluPopRoots(c,n+m); return gluMust(v,err) }")
	b.line("func gluChr(c *rt.Context, n rt.Value, roots ...rt.Value) rt.Value { k := gluPushRoots(c,n); if k < 0 { return rt.NilVal() }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,k); return rt.NilVal() }; v,err := c.Chr(n); gluPopRoots(c,k+m); return gluMust(v,err) }")
	b.line("func gluTostr(c *rt.Context, v rt.Value, hex bool, roots ...rt.Value) rt.Value { k := gluPushRoots(c,v); if k < 0 { return rt.NilVal() }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,k); return rt.NilVal() }; out,err := c.Tostr(v,hex); gluPopRoots(c,k+m); return gluMust(out,err) }")
	b.line("func gluTableAdd(c *rt.Context, t, v, i rt.Value, roots ...rt.Value) rt.HeapError { n := gluPushRoots(c,t,v,i); if n < 0 { return rt.HeapOOM }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,n); return rt.HeapOOM }; err := c.TableAdd(t,v,i); gluPopRoots(c,n+m); return err }")
	b.line("func gluTableAppendKnown(c *rt.Context, t, v rt.Value, at int, roots ...rt.Value) rt.HeapError { n := gluPushRoots(c,t,v); if n < 0 { return rt.HeapOOM }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,n); return rt.HeapOOM }; err := c.TableAppendKnown(t,v,at); gluPopRoots(c,n+m); return err }")
	b.line("func gluTableAppendKnownHeld(c *rt.Context, t, v rt.Value, at int, roots ...rt.Value) rt.HeapError { n := gluPushRoots(c,v); if n < 0 { return rt.HeapOOM }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,n); return rt.HeapOOM }; err := c.TableAppendKnown(t,v,at); gluPopRoots(c,n+m); return err }")
	b.line("func gluTableSet(c *rt.Context, t, k, v rt.Value, roots ...rt.Value) rt.HeapError { n := gluPushRoots(c,t,k,v); if n < 0 { return rt.HeapOOM }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,n); return rt.HeapOOM }; err := c.TableSet(t,k,v); gluPopRoots(c,n+m); return err }")
	// A cached field write resolves its handle and stores without allocating.
	// RHS evaluation has already rooted allocations; only the cold lookup/
	// insertion path can collect and needs the temporary root scopes below.
	b.line("func gluTableSetStable(c *rt.Context, t, k, v rt.Value, cache *rt.TableFieldCache, roots ...rt.Value) rt.HeapError { if *cache != 0 { return c.TableSetStable(t,k,v,cache) }; n := gluPushRoots(c,t,k,v); if n < 0 { return rt.HeapOOM }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,n); return rt.HeapOOM }; err := c.TableSetStable(t,k,v,cache); gluPopRoots(c,n+m); return err }")
	b.line("func gluTableSetStableNumber(c *rt.Context, t, k rt.Value, v rt.Number, cache *rt.TableFieldCache, roots ...rt.Value) rt.HeapError { if *cache != 0 { return c.TableSetStableNumber(t,k,v,cache) }; n := gluPushRoots(c,t,k); if n < 0 { return rt.HeapOOM }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,n); return rt.HeapOOM }; err := c.TableSetStableNumber(t,k,v,cache); gluPopRoots(c,n+m); return err }")
	b.line("func gluPrintV(c *rt.Context, s, x, y, color rt.Value, roots ...rt.Value) int { n := gluPushRoots(c,s,x,y,color); if n < 0 { return x.Int() }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,n); return x.Int() }; out := c.PrintV(s,x,y,color); gluPopRoots(c,n+m); return out }")
}

func (g *generator) renderGame(b *sourceBuilder) {
	b.line("const (")
	b.line(fmt.Sprintf("\tSheetW = %d; SheetH = %d; MapW = %d; MapH = %d; MapLayers = %d; MapPlaneMask = %d; MapStreaming = %t; SheetReadOnly = %t", g.opts.SheetW, g.opts.SheetH, g.opts.MapW, g.opts.MapH, g.opts.MapLayers, g.opts.MapPlaneMask, g.opts.MapStreaming, !g.sheetMutable))
	b.line("\tSpriteCols = SheetW / 8; SpriteRows = SheetH / 8; SpriteN = SpriteCols * SpriteRows")
	b.line(")")
	b.line("")
	b.line("//go:section .rodata")
	if g.sheetMutable {
		b.line(fmt.Sprintf("var sheetROM = [ %d ]byte{%s}", g.opts.SheetW*g.opts.SheetH, byteLiteral(g.opts.Sheet, g.opts.SheetW*g.opts.SheetH)))
	} else {
		b.line(fmt.Sprintf("var sheetROM = [ %d ]byte{%s}", len(g.sheetCompact), byteLiteral(g.sheetCompact, len(g.sheetCompact))))
		b.line("//go:section .rodata")
		b.line(fmt.Sprintf("var sheetIndexROM = [ %d ]uint16{%s}", g.spriteN, u16Literal(g.sheetIndex, g.spriteN)))
		if len(g.sheetSolid) != 0 {
			b.line("//go:section .rodata")
			b.line(fmt.Sprintf("var sheetSolidROM = [ %d ]uint16{%s}", len(g.sheetSolid), u16Literal(g.sheetSolid, len(g.sheetSolid))))
		}
	}
	b.line("//go:section .rodata")
	mapCells := len(g.mapCompact)
	b.line(fmt.Sprintf("var mapROM = [ %d ]uint16{%s}", mapCells, u16Literal(g.mapCompact, mapCells)))
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var flagROM = [ %d ]byte{%s}", g.spriteN, byteLiteral(g.opts.Flags, g.spriteN)))
	poseIndex, poseParts := g.opts.PoseIndex, g.opts.PoseParts
	if g.poseOBJResident != nil && len(g.poseOBJResident.palettes) != 0 {
		// Hardware variants share canonical geometry as well as pixels.
		poseIndex = append([]runtimeapi.PoseRange(nil), poseIndex...)
		count, end := g.opts.PoseVariantBaseCount, 0
		for i := range poseIndex {
			poseIndex[i] = poseIndex[i%count]
			if n := int(poseIndex[i].Start) + int(poseIndex[i].Count); n > end {
				end = n
			}
		}
		poseParts = poseParts[:end]
	}
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var poseIndexROM = [ %d ]rt.PoseRange{%s}", len(poseIndex), poseRangeLiteral(poseIndex)))
	b.line(fmt.Sprintf("const PoseVariantBaseCount = %d", g.opts.PoseVariantBaseCount))
	b.line(fmt.Sprintf("var poseVariantNamesROM = %#v", g.opts.PoseVariantNames))
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var posePartROM = [ %d ]rt.PosePart{%s}", len(poseParts), posePartLiteral(poseParts)))
	g.renderAnimationTables(b)
	if len(g.poseOBJIndex) > 0 {
		g.renderPoseOBJPlans(b)
	}
	if g.objSpriteN > 0 {
		b.line(fmt.Sprintf("const (\n\tObjSheetW = %d; ObjSheetH = %d; ObjSpriteCols = ObjSheetW / 8; ObjSpriteRows = ObjSheetH / 8; ObjSpriteN = ObjSpriteCols * ObjSpriteRows\n)", g.opts.ObjSheetW, g.opts.ObjSheetH))
		b.line(assetsync.ObjectSheetROMDeclaration(g.objSheetCompact))
		b.line("//go:section .rodata")
		b.line(fmt.Sprintf("var objSheetIndexROM = [ %d ]uint32{%s}", g.objSpriteN, u32Literal(g.objSheetIndex, g.objSpriteN)))
	}
	if len(g.mode0TextKeys) != 0 {
		b.line("//go:section .rodata")
		b.line(fmt.Sprintf("var mode0TextKeysROM = [%d]uint16{%s}", len(g.mode0TextKeys), u16Literal(g.mode0TextKeys, len(g.mode0TextKeys))))
		b.line("//go:section .rodata")
		b.line(fmt.Sprintf("var mode0TextTilesROM = [%d]uint32{%s}", len(g.mode0TextTiles), u32Literal(g.mode0TextTiles, len(g.mode0TextTiles))))
	}
	if len(g.opts.Palette) == 256 && !g.paletteIsDefault {
		b.line("//go:section .rodata")
		b.line(fmt.Sprintf("var paletteROM = [256]uint16{%s}", u16Literal(g.opts.Palette, 256)))
	}

	if g.nativeAudioEnabled {
		b.line(g.nativeAudioDecl)
	}
	b.line("")
	if g.sheetMutable {
		b.line(fmt.Sprintf("//go:section .ewram\nvar sheetRAM [%d]byte", g.opts.SheetW*g.opts.SheetH))
	} else {
		b.line(fmt.Sprintf("//go:section .ewram\nvar sheetRAM [%d]byte", len(g.sheetCompact)))
		b.line(fmt.Sprintf("//go:section .ewram\nvar sheetIndexRAM [%d]uint16", g.spriteN))
	}
	if g.opts.MapStreaming {
		b.line("//go:section .ewram\nvar mapRAM [rt.MapCacheCells]uint16")
		b.line("//go:section .ewram\nvar mapChunksRAM [rt.MapCacheSlots]rt.MapChunk")
		b.line("//go:section .ewram\nvar mapEditsRAM [rt.MapEditCapacity]rt.MapEdit")
	} else {
		b.line(fmt.Sprintf("//go:section .ewram\nvar mapRAM [%d]uint16", mapCells))
	}
	b.line(fmt.Sprintf("//go:section .ewram\nvar flagRAM [%d]byte", g.spriteN))
	if _, tiles := g.poseOBJStageWindow(); tiles > 0 {
		b.line(fmt.Sprintf("//go:section .ewram\nvar poseOBJStageRAM [%d]uint32", tiles*16))
	}
	b.line("//go:section .ewram\nvar cartData rt.AssetMemory")
	b.line("//go:section .ewram\nvar ctx rt.Context")
	b.line("//go:section .ewram\nvar game Game")
	if g.mapRasterEligible() {
		// Use target sizes: host pointer widths and target capacity tags differ.
		// Reserving both Lua semispaces plus 8 KiB makes the optimization fail
		// closed to zero-length arrays when a small map belongs to a large cart.
		budget := "256*1024 - 512 - 8192 - rt.MapRasterHeapReserve - int(unsafe.Sizeof(ctx)) - int(unsafe.Sizeof(cartData)) - int(unsafe.Sizeof(game)) - int(unsafe.Sizeof(sheetRAM)) - int(unsafe.Sizeof(sheetIndexRAM)) - int(unsafe.Sizeof(mapRAM)) - int(unsafe.Sizeof(flagRAM))"
		if g.nativeAudioEnabled {
			budget += " - int(unsafe.Sizeof(nativeAudioController))"
		}
		b.line("const mapRasterBudget = " + budget)
		b.line("const mapRasterBytes = MapW*8*MapH*8 + rt.MapRasterMaxSpans*int(unsafe.Sizeof(rt.MapRasterSpan{}))")
		b.line("const mapRasterEnabled = max(0, min(1, mapRasterBudget/mapRasterBytes))")
		b.line("//go:section .ewram\nvar mapRasterPixelsRAM [mapRasterEnabled*MapW*8*MapH*8]byte")
		b.line("//go:section .ewram\nvar mapRasterSpansRAM [mapRasterEnabled*rt.MapRasterMaxSpans]rt.MapRasterSpan")
	}
	g.renderPackedMapStorage(b)
	if g.opts.Debug {
		b.line("//go:section .debug_mailbox\nvar debugMailbox rt.DebugMailbox")
	}

	if g.nativeAudioEnabled {
		b.line("//go:section .ewram\nvar nativeAudioController audio.Controller")
	}
	b.line("")
	b.line("type Game struct {")
	for _, name := range g.globals {
		if g.numericRecords[name] != nil || g.numericConstants[name] != nil {
			continue
		}
		if g.globalKinds[name] == monoNumber {
			b.line("\t" + g.globalGo[name] + " rt.Number")
		} else {
			b.line("\t" + g.globalGo[name] + " rt.Value")
		}
	}
	for _, record := range g.numericRecordOrder {
		for _, field := range record.fieldOrder {
			b.line("\t" + numericRecordMember(record, field) + " rt.Number")
		}
	}
	for _, name := range g.statics {
		b.line("\t" + g.staticGo[name] + " rt.Value")
	}
	for i := 0; i < g.fieldCacheN; i++ {
		b.line(fmt.Sprintf("\t_gluField%d rt.TableFieldCache", i))
	}
	b.line("}")
	b.line("")
	b.line("type Runnable interface { Init(*rt.Context); Update(*rt.Context); Draw(*rt.Context) }")
	b.line("var _ Runnable = (*Game)(nil)")
	b.line("")
	b.line("func (g *Game) Init(c *rt.Context) {")
	for _, s := range g.statics {
		b.line("\tg." + g.staticGo[s] + " = c.InternStatic(" + strconv.Quote(s) + ")")
	}
	for _, name := range g.globals {
		if g.numericRecords[name] != nil || g.numericConstants[name] != nil {
			continue
		}
		if g.heapEnabled && g.globalKinds[name] == monoValue {
			b.line("\tc.RegisterGlobal(&g." + g.globalGo[name] + ")")
		}
	}
	g.current = nil
	for _, stmt := range g.chunk.Body {
		if _, ok := stmt.(*luaparser.FunctionDeclStmt); !ok {
			g.emitStmt(b, stmt, 1)
		}
	}
	if fn := g.funcs["_init"]; fn != nil {
		b.line("\tg." + fn.goName + "(c)")
	}
	b.line("}")
	b.line("")
	for _, key := range []string{"_update", "_draw"} {
		name := map[string]string{"_update": "Update", "_draw": "Draw"}[key]
		b.line("func (g *Game) " + name + "(c *rt.Context) {")
		if fn := g.funcs[key]; fn != nil {
			b.line("\tg." + fn.goName + "(c)")
		}
		b.line("}")
		b.line("")
	}
	g.current = nil
}

func (g *generator) renderFunctions(b *sourceBuilder) {
	for _, fn := range g.funcList {
		g.current = fn
		g.emitFunction(b, fn)
		b.line("")
		g.emitCallWrapper(b, fn)
		b.line("")
	}
	g.current = nil
}

func (g *generator) haltStatement(reason string) string {
	if g.opts.Debug {
		return "rt.DebugLog(" + strconv.Quote(reason) + "); for {}"
	}
	return "for {}"
}

func (g *generator) renderMain(b *sourceBuilder) {
	b.line("var cartImage = rt.AssetImage{")
	b.line("\t\tSheetW: SheetW, SheetH: SheetH, SpriteCols: SpriteCols, SpriteRows: SpriteRows, SpriteN: SpriteN,")
	b.line("\t\tMapW: MapW, MapH: MapH, MapLayers: MapLayers, MapPlaneMask: MapPlaneMask, MapStreaming: MapStreaming, SheetReadOnly: SheetReadOnly, Sheet: sheetROM[:], Map: mapROM[:], Flags: flagROM[:],")
	b.line("\t\tPoseIndex: poseIndexROM[:], PoseParts: posePartROM[:],")
	b.line(fmt.Sprintf("\t\tPoseVariantBaseCount: %d, PoseVariantNames: poseVariantNamesROM,", g.opts.PoseVariantBaseCount))
	b.line("\t\tAnimationClips: animationClipROM[:], AnimationFrames: animationFrameROM[:],")
	if len(g.poseOBJIndex) > 0 {
		b.line("\t\tPoseOBJIndex: poseOBJIndexROM[:], PoseOBJParts: poseOBJPartsROM[:], PoseOBJTiles: poseOBJTilesROM[:],")
		if g.poseOBJResident != nil {
			b.line(fmt.Sprintf("\t\tPoseOBJResident: poseOBJResidentROM[:], PoseOBJResidentBase: %d, PoseOBJPalettes: poseOBJPalettesROM[:],", g.poseOBJResident.base))
		}
	}
	if g.objSpriteN > 0 {
		b.line("\t\tObjSheetW: ObjSheetW, ObjSheetH: ObjSheetH, ObjSpriteCols: ObjSpriteCols, ObjSpriteRows: ObjSpriteRows, ObjSpriteN: ObjSpriteN,")
		b.line("\t\tObjSheet: objSheetROM.Pixels[:], ObjSheetIndex: objSheetIndexROM[:],")
	}
	if !g.sheetMutable {
		b.line("\t\tSheetIndex: sheetIndexROM[:],")
	}
	b.line("}")
	b.line("func main() {")
	if g.opts.Debug {
		b.line("\trt.DebugLog(\"Cartridge preview started\")")
	}
	b.line("\tcartData.SheetW, cartData.SheetH = SheetW, SheetH")
	b.line("\tcartData.SpriteCols, cartData.SpriteRows, cartData.SpriteN = SpriteCols, SpriteRows, SpriteN")
	b.line("\tcartData.MapW, cartData.MapH, cartData.MapLayers, cartData.MapPlaneMask = MapW, MapH, MapLayers, MapPlaneMask")
	b.line("\tcartData.MapStreaming, cartData.SheetReadOnly = MapStreaming, SheetReadOnly")
	b.line("\tcartData.Sheet = sheetRAM[:]")
	if !g.sheetMutable {
		b.line("\tcartData.SheetIndex = sheetIndexRAM[:]")
	}
	if len(g.sheetSolid) != 0 {
		b.line("\tcartData.SheetSolid = sheetSolidROM[:]")
	}
	b.line("\tcartData.Map, cartData.Flags = mapRAM[:], flagRAM[:]")
	b.line("\tcartData.PoseIndex, cartData.PoseParts = poseIndexROM[:], posePartROM[:]")
	b.line(fmt.Sprintf("\tcartData.PoseVariantBaseCount, cartData.PoseVariantNames = %d, poseVariantNamesROM", g.opts.PoseVariantBaseCount))
	b.line("\tcartData.AnimationClips, cartData.AnimationFrames = animationClipROM[:], animationFrameROM[:]")
	if base, tiles := g.poseOBJStageWindow(); tiles > 0 {
		b.line(fmt.Sprintf("\tcartData.PoseOBJStage, cartData.PoseOBJStageBase = poseOBJStageRAM[:], %d", base))
	}
	if g.objSpriteN > 0 {
		b.line("\tcartData.ObjSheetW, cartData.ObjSheetH = ObjSheetW, ObjSheetH")
		b.line("\tcartData.ObjSpriteCols, cartData.ObjSpriteRows, cartData.ObjSpriteN = ObjSpriteCols, ObjSpriteRows, ObjSpriteN")
		b.line("\tcartData.ObjSheet, cartData.ObjSheetIndex = objSheetROM.Pixels[:], objSheetIndexROM[:]")
	}
	if g.mapRasterEligible() {
		b.line("\tcartData.MapRasterPixels, cartData.MapRasterSpans = mapRasterPixelsRAM[:], mapRasterSpansRAM[:]")
	}
	g.renderPackedMapBindings(b)
	if g.opts.MapStreaming {
		b.line("\tcartData.MapChunks, cartData.MapEdits = mapChunksRAM[:], mapEditsRAM[:]")
	}
	b.line("\tif !ctx.Init(&cartImage, &cartData) { " + g.haltStatement("Cartridge assets could not be initialized. Rebuild the project.") + " }")
	if len(g.opts.Palette) == 256 && !g.paletteIsDefault {
		b.line("\tctx.LoadPalette(&paletteROM)")
	}
	b.line("\tctx.InitHardware()")
	if len(g.mode0TextKeys) != 0 {
		b.line("\tif !ctx.LoadMode0TextAtlas(mode0TextKeysROM[:], mode0TextTilesROM[:]) { " + g.haltStatement("Text tiles could not be initialized. Rebuild the project or select Mode 4.") + " }")
	}
	if g.timeEnabled {
		b.line("\tctx.SetTickRate(30)")
	}
	if g.opts.Debug {
		b.line("\tctx.SetProfiling(true)")
	}

	if g.nativeAudioEnabled {
		settings := g.opts.NativeAudio.Settings
		b.line("\tif !nativeAudioController.Init(audio.ProfilePSG) { " + g.haltStatement("Sound could not be initialized. Stop and reload the preview.") + " }")
		b.line(fmt.Sprintf("\tif !nativeAudioController.ConfigureOutput(%d, %d, %s) { %s }", settings.LeftLevel, settings.RightLevel, transpilerPSGRatioGoName(settings.PSGRatio), g.haltStatement("Sound output settings were rejected. Check the project's audio settings.")))
		b.line("\tif !nativeAudioController.Bind(audio.Library{Sounds: nativeSoundROM[:], Clips: nativeClipROM[:], Parts: nativePartROM[:], Songs: nativeSongROM[:]}) { " + g.haltStatement("Sound assets were rejected. Check sounds.json and music.json, then rebuild.") + " }")
		b.line("\tctx.AttachNativeAudio(&nativeAudioController)")
	}
	b.line("\tgame.Init(&ctx)")
	if g.mapRasterEligible() {
		b.line("\tctx.PrepareMapRaster()")
	}
	if g.hasPackedMap() {
		b.line("\tctx.PrepareMapPacked()")
	}
	if g.opts.Renderer == RendererMode0 {
		b.line("\t// _init runs while Mode 0 is force-blanked; commit its")
		b.line("\t// spritesheet mutations before the first visible frame.")
		b.line("\tctx.FinalizeInitHardware()")
	}

	if g.nativeAudioEnabled {
		b.line("\tif !nativeAudioController.Start() { " + g.haltStatement("The sound timer could not start. Stop and reload the preview.") + " }")
	}
	b.line("\tfor {")
	b.line("\t\t// One iteration = exactly two display frames (about 30 Hz).")
	b.line("\t\t// The LCD always scans at 59.73 Hz;")
	b.line("\t\t// each rendered image is shown for two whole scans. PaceFrame30")
	b.line("\t\t// burns whatever time update/draw left over in the first frame, so")
	b.line("\t\t// the flip always lands in the second frame's VBlank - a fast")
	b.line("\t\t// iteration can never flip one frame early and alternate cadence.")
	b.line("\t\tctx.BeginFramePace()")

	b.line("\t\tctx.BeginProfilePhase(rt.ProfileUpdate)")
	if g.inputEnabled {
		b.line("\t\tctx.PollInput()")
	}
	if g.inputEnabled || g.timeEnabled {
		b.line("\t\tctx.AdvanceTick()")
	}
	b.line("\t\tgame.Update(&ctx)")
	b.line("\t\tctx.EndProfilePhase(rt.ProfileUpdate)")

	b.line("\t\tctx.BeginProfilePhase(rt.ProfileDraw)")
	b.line("\t\tctx.BeginRenderFrame()")
	b.line("\t\tgame.Draw(&ctx)")
	b.line("\t\tctx.EndProfilePhase(rt.ProfileDraw)")

	b.line("\t\t// Copy to the hidden page immediately - it is never on screen, so")
	b.line("\t\t// this cannot tear, and update+draw+copy own the full two-frame")
	b.line("\t\t// window (~562k cycles) minus only the flip itself.")
	b.line("\t\tctx.PrepareRender()")

	if g.heapEnabled {
		b.line("\t\tctx.CollectIfNeeded()")
	}
	b.line("\t\tctx.PaceFrame30()")
	b.line("\t\tctx.WaitForPresentWindow()")
	b.line("\t\tctx.PresentRender()")
	if g.opts.Debug {
		if g.heapEnabled {
			b.line("\t\tdebugMailbox.Begin(ctx.DebugSnapshot())")
		} else {
			b.line("\t\theaplessStats := ctx.DebugSnapshot()")
			b.line("\t\theaplessStats.HeapCapacity, heaplessStats.HeapHandleLimit = 0, 0")
			b.line("\t\tdebugMailbox.Begin(heaplessStats)")
		}
		watchIndex := 0
		for _, name := range g.globals {
			if watchIndex >= runtimeapi.DebugWatchSlots {
				break
			}
			if record := g.numericRecords[name]; record != nil {
				for _, field := range record.fieldOrder {
					if watchIndex >= runtimeapi.DebugWatchSlots {
						break
					}
					value := "rt.NumVal(game." + numericRecordMember(record, field) + ")"
					b.line("\t\tdebugMailbox.Add(" + strconv.Quote(name+"."+field.name) + ", " + value + ")")
					watchIndex++
				}
				continue
			}
			if constant := g.numericConstants[name]; constant != nil {
				value := fmt.Sprintf("rt.NumVal(rt.Number(%d))", constant.value)
				b.line("\t\tdebugMailbox.Add(" + strconv.Quote(name) + ", " + value + ")")
				watchIndex++
				continue
			}
			value := "game." + g.globalGo[name]
			if g.globalKinds[name] == monoNumber {
				value = "rt.NumVal(" + value + ")"
			}
			b.line("\t\tdebugMailbox.Add(" + strconv.Quote(name) + ", " + value + ")")
			watchIndex++
		}
		b.line("\t\tdebugMailbox.End()")
	}
	b.line("\t\tctx.EndFramePace30()")
	b.line("\t\tctx.WaitVDraw()")
	b.line("\t}")
	b.line("}")
}

func transpilerPSGRatioGoName(ratio int) string {
	switch ratio {
	case 25:
		return "hw.PSGMix25"
	case 50:
		return "hw.PSGMix50"
	default:
		return "hw.PSGMix100"
	}
}

func (g *generator) emitFunction(b *sourceBuilder, fn *functionInfo) {
	if slices.Contains(g.opts.IWRAMFunctions, fn.key) {
		b.line("//go:noinline")
		b.line("//go:section .iwram")
	} else if functionCallsBuiltin(fn, "print") {
		// Keep print() helpers out of the caller's native frame. TinyGo
		// 0.42 moves locals above 256 bytes onto runtime.alloc, which
		// -gc=none does not link; inlining print into gluDraw blanks the
		// Mode 4 screen at InitHardware's 0xFF fill.
		b.line("//go:noinline")
	}
	b.line("func (g *Game) " + fn.goName + "(c *rt.Context" + functionParams(fn) + ")" + functionReturnSignature(fn) + " {")
	g.pushScope()
	for i, p := range fn.params {
		g.bindParamKind(p, fmt.Sprintf("a%d", i), fn.paramKinds[i])
	}
	for _, stmt := range fn.decl.Body.Body {
		g.emitStmt(b, stmt, 1)
	}
	g.popScope()
	if fn.returnN > 0 {
		b.line("\treturn " + functionZeroReturns(fn))
	}
	b.line("}")
}

func functionParams(fn *functionInfo) string {
	var b strings.Builder
	for i, kind := range fn.paramKinds {
		fmt.Fprintf(&b, ", a%d %s", i, goTypeForKind(kind))
	}
	return b.String()
}

func goTypeForKind(kind monoKind) string {
	if kind == monoNumber {
		return "rt.Number"
	}
	return "rt.Value"
}

func functionReturnSignature(fn *functionInfo) string {
	if fn.returnN == 0 {
		return ""
	}
	parts := make([]string, fn.returnN)
	for i := range parts {
		parts[i] = goTypeForKind(fn.returnKind(i))
	}
	if fn.returnN == 1 {
		return " " + parts[0]
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func functionZeroReturns(fn *functionInfo) string {
	parts := make([]string, fn.returnN)
	for i := range parts {
		if fn.returnKind(i) == monoNumber {
			parts[i] = "0"
		} else {
			parts[i] = "rt.NilVal()"
		}
	}
	return strings.Join(parts, ", ")
}

func (g *generator) emitCallWrapper(b *sourceBuilder, fn *functionInfo) {
	name := "__glu_call_" + fn.goName
	b.line("func (g *Game) " + name + "(c *rt.Context" + functionParams(fn) + ", roots ...rt.Value)" + functionReturnSignature(fn) + " {")
	b.line("\tn := gluPushRoots(c, roots...)")
	switch fn.returnN {
	case 0:
		b.line("\tif n < 0 { return }")
		b.line("\tg." + fn.goName + "(c" + valueArgs(fn.params) + ")")
		b.line("\tgluPopRoots(c,n)")
	case 1:
		b.line("\tif n < 0 { return " + functionZeroReturns(fn) + " }")
		b.line("\tv := g." + fn.goName + "(c" + valueArgs(fn.params) + ")")
		b.line("\tgluPopRoots(c,n)")
		b.line("\treturn v")
	default:
		b.line("\tif n < 0 { return " + functionZeroReturns(fn) + " }")
		parts := make([]string, fn.returnN)
		for i := range parts {
			parts[i] = fmt.Sprintf("v%d", i)
		}
		b.line("\t" + strings.Join(parts, ", ") + " := g." + fn.goName + "(c" + valueArgs(fn.params) + ")")
		b.line("\tgluPopRoots(c,n)")
		b.line("\treturn " + strings.Join(parts, ", "))
	}
	b.line("}")
	if fn.returnN > 1 {
		g.emitMultiWrapper(b, fn)
	}
}

func (g *generator) emitMultiWrapper(b *sourceBuilder, fn *functionInfo) {
	// __glu_multi_X is the root-protected call used when all of a
	// multi-return function's results are wanted (e.g. `local a, b =
	// f(...)`); __glu_first_X is used where only the first result is wanted
	// (f(...) used as a single-value expression). Extra results there must
	// be discarded with "_", not bound to names like "v1" that are never
	// read: Go's compiler rejects an unused declared variable, so an
	// unconditional v0, v1 := ... paired with only "return v0" would make
	// every generated program containing a multi-return function fail to
	// build.
	name := "__glu_multi_" + fn.goName
	b.line("func (g *Game) " + name + "(c *rt.Context" + functionParams(fn) + ", roots ...rt.Value)" + functionReturnSignature(fn) + " {")
	b.line("\tn := gluPushRoots(c, roots...)")
	b.line("\tif n < 0 { return " + functionZeroReturns(fn) + " }")
	parts := make([]string, fn.returnN)
	for i := range parts {
		parts[i] = fmt.Sprintf("v%d", i)
	}
	b.line("\t" + strings.Join(parts, ", ") + " := g." + fn.goName + "(c" + valueArgs(fn.params) + ")")
	b.line("\tgluPopRoots(c,n)")
	b.line("\treturn " + strings.Join(parts, ", "))
	b.line("}")
	b.line("func (g *Game) __glu_first_" + fn.goName + "(c *rt.Context" + functionParams(fn) + ", roots ...rt.Value) " + goTypeForKind(fn.returnKind(0)) + " {")
	first := make([]string, fn.returnN)
	first[0] = "v0"
	for i := 1; i < fn.returnN; i++ {
		first[i] = "_"
	}
	b.line("\t" + strings.Join(first, ", ") + " := g.__glu_multi_" + fn.goName + "(c" + valueArgs(fn.params) + ", roots...)")
	b.line("\treturn v0")
	b.line("}")
}

func valueArgs(params []string) string {
	var b strings.Builder
	for i := range params {
		fmt.Fprintf(&b, ", a%d", i)
	}
	return b.String()
}

func (g *generator) emitStmt(b *sourceBuilder, stmt luaparser.Stmt, indent int) {
	pad := strings.Repeat("\t", indent)
	switch s := stmt.(type) {
	case *luaparser.AssignStmt:
		g.emitAssign(b, s, indent)
	case *luaparser.LocalAssignStmt:
		g.emitLocalAssign(b, s, indent)
	case *luaparser.CallStmt:
		call := g.emitCall(s.Call, false)
		g.writePending(b, indent)
		if call != "" {
			b.line(pad + call)
		}
	case *luaparser.IfStmt:
		var elseBody []luaparser.Stmt
		if s.Else != nil {
			elseBody = s.Else.Body
		}
		g.emitIfChain(b, indent, s.Condition, s.Then.Body, s.ElseIf, elseBody)
	case *luaparser.WhileStmt:
		b.line(pad + "for {")
		cond := g.emitConditionFlush(b, indent+1, s.Condition)
		b.line(pad + "\tif !(" + cond + ") { break }")
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+1)
		}
		g.popScope()
		b.line(pad + "}")
	case *luaparser.RepeatStmt:
		b.line(pad + "for {")
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+1)
		}
		cond := g.emitConditionFlush(b, indent+1, s.Condition)
		g.popScope()
		b.line(pad + "\tif (" + cond + ") { break }")
		b.line(pad + "}")
	case *luaparser.DoStmt:
		b.line(pad + "{")
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+1)
		}
		g.popScope()
		b.line(pad + "}")
	case *luaparser.NumericForStmt:
		if g.emitMonomorphicNumericFor(b, s, indent) {
			return
		}
		id := g.next("for")
		initExpr := g.emitExprFlush(b, indent, s.Init)
		b.line(pad + id + "Init := " + initExpr)
		initRoots := id + "Roots"
		rootSlots := g.activeRootSlots
		g.requireActiveRootSlots(1, s.Init.Pos())
		b.line(pad + initRoots + " := gluPushRoots(c, " + id + "Init)")
		b.line(pad + "if " + initRoots + " >= 0 {")
		// Init is already on Context's root stack while limit is evaluated.
		// Track that occupied slot instead of passing the same value again as
		// an extra helper root.
		g.activeRootSlots = rootSlots + 1
		limitExpr := g.emitExprFlush(b, indent+1, s.Limit)
		b.line(pad + "\t" + id + "Limit := " + limitExpr)
		limitRoots := id + "LimitRoots"
		g.requireActiveRootSlots(1, s.Limit.Pos())
		b.line(pad + "\t" + limitRoots + " := gluPushRoots(c, " + id + "Limit)")
		b.line(pad + "\tif " + limitRoots + " >= 0 {")
		// The step may allocate and therefore needs the already evaluated limit
		// as a root. Generate and flush it only after Limit has been declared;
		// otherwise root arguments inside the step can reference a Go name before
		// its declaration.
		g.activeRootSlots = rootSlots + 2
		stepExpr := g.emitExprFlush(b, indent+2, s.Step)
		b.line(pad + "\t\t" + id + "Step := " + stepExpr)
		stepRoots := id + "StepRoots"
		g.requireActiveRootSlots(1, s.Step.Pos())
		b.line(pad + "\t\t" + stepRoots + " := gluPushRoots(c, " + id + "Step)")
		b.line(pad + "\t\tif " + stepRoots + " >= 0 {")
		rootDepth := len(g.activeRootCounts)
		g.activeRootCounts = append(g.activeRootCounts, initRoots, limitRoots, stepRoots)
		g.activeRootSlots = rootSlots + 3
		g.pushScope()
		loopVar := g.declareLocal(s.Name)
		b.line(pad + "\t\t\tvar " + loopVar + " rt.Value")
		b.line(pad + "\t\t\t_ = " + loopVar)
		b.line(pad + "\t\t\tfor " + id + "Init.Kind == rt.KindNumber && " + id + "Limit.Kind == rt.KindNumber && " + id + "Step.Kind == rt.KindNumber && ((" + id + "Step.Num >= 0 && " + id + "Init.Num <= " + id + "Limit.Num) || (" + id + "Step.Num < 0 && " + id + "Init.Num >= " + id + "Limit.Num)) {")
		b.line(pad + "\t\t\t\t" + loopVar + " = " + id + "Init")
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+4)
		}
		g.popScope()
		g.activeRootCounts = g.activeRootCounts[:rootDepth]
		g.activeRootSlots = rootSlots
		b.line(pad + "\t\t\t\t" + id + "Init = " + id + "Init.Add(" + id + "Step)")
		b.line(pad + "\t\t\t}")
		g.popScope()
		b.line(pad + "\t\t\tgluPopRoots(c, " + stepRoots + ")")
		b.line(pad + "\t\t}")
		b.line(pad + "\t\tgluPopRoots(c, " + limitRoots + ")")
		b.line(pad + "\t}")
		b.line(pad + "\tgluPopRoots(c, " + initRoots + ")")
		b.line(pad + "}")
		g.activeRootSlots = rootSlots
	case *luaparser.GenericForStmt:
		g.emitGenericFor(b, s, indent)
	case *luaparser.ReturnStmt:
		g.emitReturn(b, s, indent)
	case *luaparser.BreakStmt:
		b.line(pad + "break")
	}
}

func (g *generator) emitMonomorphicNumericFor(b *sourceBuilder, s *luaparser.NumericForStmt, indent int) bool {
	if !g.exprNumber(s.Init) || !g.exprNumber(s.Limit) || !g.exprNumber(s.Step) || numericForVariableReassigned(s) {
		return false
	}
	if g.emitIntegerNumericFor(b, s, indent) {
		return true
	}
	pad := strings.Repeat("\t", indent)
	id := g.next("numFor")
	initExpr := g.emitNumberExpr(s.Init)
	g.writePending(b, indent)
	b.line(pad + id + "Init := " + initExpr)
	limitExpr := g.emitNumberExpr(s.Limit)
	g.writePending(b, indent)
	b.line(pad + id + "Limit := " + limitExpr)
	stepExpr := g.emitNumberExpr(s.Step)
	g.writePending(b, indent)
	b.line(pad + id + "Step := " + stepExpr)
	restoreViews := g.borrowLoopViews(b, s.Body, indent)
	defer restoreViews()
	appendPosition, restoreAppend := g.beginKnownAppendLoop(b, s, indent)
	defer restoreAppend()
	g.pushScope()
	loopVar := g.declareLocalKind(s.Name, monoNumber)
	b.line(pad + "var " + loopVar + " rt.Number")
	b.line(pad + "_ = " + loopVar)
	b.line(pad + "for (" + id + "Step >= 0 && " + id + "Init <= " + id + "Limit) || (" + id + "Step < 0 && " + id + "Init >= " + id + "Limit) {")
	b.line(pad + "\t" + loopVar + " = " + id + "Init")
	g.pushScope()
	for _, stmt := range s.Body.Body {
		g.emitStmt(b, stmt, indent+1)
	}
	g.popScope()
	if appendPosition != "" {
		b.line(pad + "\t" + appendPosition + "++")
	}
	b.line(pad + "\t" + id + "Init += " + id + "Step")
	b.line(pad + "}")
	g.popScope()
	return true
}

func (g *generator) emitIntegerNumericFor(b *sourceBuilder, s *luaparser.NumericForStmt, indent int) bool {
	if g.opts.DisableIntegerNumericFor {
		return false
	}
	init, limit, step, ok := integerNumericForBounds(s)
	if !ok {
		return false
	}
	count := (limit-init)/step + 1
	next := init + count*step
	if count < 1 || next < -32768 || next > 32767 {
		return false
	}
	pad := strings.Repeat("\t", indent)
	id := g.next("intFor")
	restoreViews := g.borrowLoopViews(b, s.Body, indent)
	defer restoreViews()
	appendPosition, restoreAppend := g.beginKnownAppendLoop(b, s, indent)
	defer restoreAppend()
	g.pushScope()
	used := numericForVariableReferenced(s)
	loopVar := ""
	if used {
		loopVar = g.declareLocalKind(s.Name, monoNumber)
		b.line(pad + "var " + loopVar + " rt.Number")
		b.line(pad + "_ = " + loopVar)
	}
	b.line(pad + integerNumericForHeader(id, init, limit, step))
	if used {
		b.line(pad + "\t" + loopVar + " = rt.FromInt(" + id + ")")
	}
	g.pushScope()
	for _, stmt := range s.Body.Body {
		g.emitStmt(b, stmt, indent+1)
	}
	g.popScope()
	if appendPosition != "" {
		b.line(pad + "\t" + appendPosition + "++")
	}
	b.line(pad + "}")
	g.popScope()
	return true
}

func (g *generator) emitIfChain(b *sourceBuilder, indent int, cond luaparser.Expr, thenBody []luaparser.Stmt, elseIfs []*luaparser.IfBranch, elseBody []luaparser.Stmt) {
	pad := strings.Repeat("\t", indent)
	condExpr := g.emitConditionFlush(b, indent, cond)
	b.line(pad + "if (" + condExpr + ") {")
	g.pushScope()
	for _, q := range thenBody {
		g.emitStmt(b, q, indent+1)
	}
	g.popScope()
	if len(elseIfs) == 0 && elseBody == nil {
		b.line(pad + "}")
		return
	}
	b.line(pad + "} else {")
	if len(elseIfs) > 0 {
		next := elseIfs[0]
		var nextThen []luaparser.Stmt
		if next.Body != nil {
			nextThen = next.Body.Body
		}
		g.emitIfChain(b, indent+1, next.Condition, nextThen, elseIfs[1:], elseBody)
	} else {
		g.pushScope()
		for _, q := range elseBody {
			g.emitStmt(b, q, indent+1)
		}
		g.popScope()
	}
	b.line(pad + "}")
}

func (g *generator) emitAssign(b *sourceBuilder, s *luaparser.AssignStmt, indent int) {
	pad := strings.Repeat("\t", indent)
	if len(g.scopes) == 0 && len(s.Targets) == 1 {
		if variable, ok := s.Targets[0].(*luaparser.VarExpr); ok {
			if info := g.numericConstants[variable.Name]; info != nil && info.declaration == s {
				return
			}
			if info := g.numericRecords[variable.Name]; info != nil && info.declaration == s {
				for _, field := range info.fieldOrder {
					value := g.emitNumberExpr(field.value)
					g.writePending(b, indent)
					b.line(pad + "g." + numericRecordMember(info, field) + " = " + value)
				}
				return
			}
		}
	}
	// Lua evaluates every l-value table/key, then every r-value, then stores.
	// Materializing both sides into temps is what makes `a, b = b, a` and
	// `i, t[i] = 2, 99` match Lua instead of Go's left-to-right stores.
	oldRoots := g.extraRoots
	g.extraRoots = append([]string(nil), oldRoots...)
	type lhs struct {
		kind  int
		mono  monoKind
		dest  string
		base  string
		key   string
		field string
		cache string
		view  string
	}
	const (
		lhsVar = iota
		lhsIndex
		lhsField
		lhsNumberArray
		lhsNumericRecord
	)
	slots := make([]lhs, len(s.Targets))
	for i, target := range s.Targets {
		switch t := target.(type) {
		case *luaparser.VarExpr:
			slots[i] = lhs{kind: lhsVar, mono: g.lookupKind(t.Name), dest: g.lookup(t.Name)}
		case *luaparser.IndexExpr:
			if name, _, ok := g.numericArrayIndex(t); ok {
				key := g.next("lhs")
				b.line(pad + key + " := " + g.emitExprFlush(b, indent, t.Key))
				g.extraRoots = append(g.extraRoots, key)
				slots[i] = lhs{kind: lhsNumberArray, mono: monoNumber, base: g.lookup(name), key: key, view: g.borrowedNumberArrays[name]}
				continue
			}
			base := g.next("lhs")
			key := g.next("lhs")
			b.line(pad + base + " := " + g.emitExprFlush(b, indent, t.Base))
			g.extraRoots = append(g.extraRoots, base)
			b.line(pad + key + " := " + g.emitExprFlush(b, indent, t.Key))
			g.extraRoots = append(g.extraRoots, key)
			slots[i] = lhs{kind: lhsIndex, base: base, key: key}
		case *luaparser.FieldExpr:
			if record, field, ok := g.numericRecordField(t); ok {
				slots[i] = lhs{kind: lhsNumericRecord, mono: monoNumber, dest: "g." + numericRecordMember(record, field)}
				continue
			}
			base := g.next("lhs")
			b.line(pad + base + " := " + g.emitExprFlush(b, indent, t.Base))
			g.extraRoots = append(g.extraRoots, base)
			mono := monoValue
			if g.exprNumber(t) {
				mono = monoNumber
			}
			slots[i] = lhs{kind: lhsField, mono: mono, base: base, field: t.Name, cache: g.fieldCacheRef(t)}
		}
	}
	values := g.emitAdjustedValues(b, s.Values, len(s.Targets), indent)
	// Every table store may grow its table and collect. Keep all RHS results
	// alive until all stores have completed, because Lua evaluates the whole
	// assignment before performing its first write.
	for _, value := range values {
		if value.kind == monoValue && value.code != "rt.NilVal()" {
			g.extraRoots = append(g.extraRoots, value.code)
		}
	}
	for i, slot := range slots {
		switch slot.kind {
		case lhsVar:
			if slot.mono == monoNumber {
				b.line(pad + slot.dest + " = " + adjustedNumber(values[i]))
			} else {
				b.line(pad + slot.dest + " = " + adjustedLuaValue(values[i]))
			}
		case lhsIndex:
			b.line(pad + "_ = gluTableSet(c, " + slot.base + ", " + slot.key + ", " + adjustedLuaValue(values[i]) + g.rootArgsFor(6, s.Pos()) + ")")
		case lhsNumberArray:
			if slot.view != "" {
				b.line(pad + "_ = " + slot.view + ".Set(" + slot.key + ".Int(), " + adjustedNumber(values[i]) + ")")
			} else {
				b.line(pad + "_ = c.NumberArraySet(" + slot.base + ", " + slot.key + ".Int(), " + adjustedNumber(values[i]) + ")")
			}
		case lhsNumericRecord:
			b.line(pad + slot.dest + " = " + adjustedNumber(values[i]))
		case lhsField:
			if slot.mono == monoNumber && slot.cache != "" {
				g.emitStableWrite(b, indent, true, slot.base, g.staticExpr(slot.field), adjustedNumber(values[i]), slot.cache, g.rootArgsFor(5, s.Pos()))
			} else if slot.cache != "" {
				g.emitStableWrite(b, indent, false, slot.base, g.staticExpr(slot.field), adjustedLuaValue(values[i]), slot.cache, g.rootArgsFor(6, s.Pos()))
			} else {
				b.line(pad + "_ = gluTableSet(c, " + slot.base + ", " + g.staticExpr(slot.field) + ", " + adjustedLuaValue(values[i]) + g.rootArgsFor(6, s.Pos()) + ")")
			}
		}
	}
	g.extraRoots = oldRoots
}

// emitLocalAssign lowers `local a, b, ... = values...`. RHS is fully
// evaluated (and last-call-expanded) before any new name is declared, so
// `local x, y = y, x` and `local x = x + 1` read the old bindings.
func (g *generator) emitLocalAssign(b *sourceBuilder, s *luaparser.LocalAssignStmt, indent int) {
	pad := strings.Repeat("\t", indent)
	// A `local` at chunk top level (outside every function, hence no active
	// scope) names a Game struct field per collectTopLevel/addGlobal, not a
	// stack local: it must stay reachable from every function, which only a
	// struct field (not a scope-stack entry that dies at chunk-code's end)
	// can provide. Route it through the same global assignment path as a
	// plain top-level `x = ...`.
	topLevel := len(g.scopes) == 0
	if topLevel && len(s.Names) == 1 {
		if info := g.numericConstants[s.Names[0]]; info != nil && info.declaration == s {
			return
		}
		if info := g.numericRecords[s.Names[0]]; info != nil && info.declaration == s {
			for _, field := range info.fieldOrder {
				value := g.emitNumberExpr(field.value)
				g.writePending(b, indent)
				b.line(pad + "g." + numericRecordMember(info, field) + " = " + value)
			}
			return
		}
		if info := g.numericArrays[s.Names[0]]; info != nil && info.declaration == s {
			array := g.lookup(s.Names[0])
			errName := g.next("numArrayErr")
			b.line(pad + "var " + errName + " rt.HeapError")
			b.line(pad + array + ", " + errName + " = c.NewNumberArray(" + strconv.Itoa(info.length) + ")")
			b.line(pad + "if " + errName + " != rt.HeapOK { " + array + " = rt.NilVal() }")
			for index, field := range info.literal.Fields {
				b.line(pad + "_ = c.NumberArraySet(" + array + ", " + strconv.Itoa(index+1) + ", " + g.emitNumberExpr(field.Value) + ")")
			}
			return
		}
	}
	values := g.emitAdjustedValues(b, s.Values, len(s.Names), indent)
	kinds := make([]monoKind, len(s.Names))
	for i, name := range s.Names {
		if topLevel {
			kinds[i] = g.globalKinds[name]
		} else {
			kinds[i] = values[i].kind
		}
	}
	declare := func(name string, value adjustedValue, kind monoKind) {
		if topLevel {
			if kind == monoNumber {
				b.line(pad + g.lookup(name) + " = " + adjustedNumber(value))
			} else {
				b.line(pad + g.lookup(name) + " = " + adjustedLuaValue(value))
			}
			return
		}
		govar := g.declareLocalKind(name, kind)
		if kind == monoNumber {
			b.line(pad + "var " + govar + " rt.Number = " + adjustedNumber(value))
		} else {
			b.line(pad + "var " + govar + " rt.Value = " + adjustedLuaValue(value))
		}
		b.line(pad + "_ = " + govar)
	}
	for i, name := range s.Names {
		declare(name, values[i], kinds[i])
	}
}

func (g *generator) emitReturn(b *sourceBuilder, s *luaparser.ReturnStmt, indent int) {
	pad := strings.Repeat("\t", indent)
	if g.current == nil {
		// A Lua chunk may return values even though Game.Init has no Go return
		// slots. The values are discarded, but their expressions must still run
		// left-to-right for side effects before the chunk exits.
		g.emitAdjustedValues(b, s.Values, 0, indent)
		g.emitActiveRootCleanup(b, indent)
		b.line(pad + "return")
		return
	}
	if g.current.returnN == 0 {
		// Even a zero-result trailing call must execute for side effects:
		// `return bump()` is not equivalent to a bare return.
		g.emitAdjustedValues(b, s.Values, 0, indent)
		g.emitActiveRootCleanup(b, indent)
		b.line(pad + "return")
		return
	}
	if g.current.returnN == 1 && g.current.returnKind(0) == monoNumber &&
		len(s.Values) == 1 && g.exprNumber(s.Values[0]) {
		value := g.emitNumberExpr(s.Values[0])
		g.writePending(b, indent)
		if len(g.activeRootCounts) > 0 {
			result := g.next("return")
			b.line(pad + result + " := " + value)
			g.emitActiveRootCleanup(b, indent)
			value = result
		}
		b.line(pad + "return " + value)
		return
	}
	values := g.emitAdjustedValues(b, s.Values, g.current.returnN, indent)
	parts := make([]string, len(values))
	for i, value := range values {
		if g.current.returnKind(i) == monoNumber {
			if value.kind != monoNumber {
				g.setErr(fmt.Errorf("transpiler: internal numeric return mismatch in %s result %d at %s", g.current.key, i+1, s.Pos()))
			}
			parts[i] = adjustedNumber(value)
		} else {
			parts[i] = adjustedLuaValue(value)
		}
	}
	if len(g.activeRootCounts) > 0 {
		for i, part := range parts {
			result := g.next("return")
			b.line(pad + result + " := " + part)
			parts[i] = result
		}
		g.emitActiveRootCleanup(b, indent)
	}
	b.line(pad + "return " + strings.Join(parts, ", "))
}

// emitActiveRootCleanup unwinds roots owned by every generated control-flow
// scope that a Lua return exits. Counts are popped in reverse push order so
// nested numeric and generic loops obey Context's local-root stack protocol.
func (g *generator) emitActiveRootCleanup(b *sourceBuilder, indent int) {
	pad := strings.Repeat("\t", indent)
	for i := len(g.activeRootCounts) - 1; i >= 0; i-- {
		b.line(pad + "gluPopRoots(c, " + g.activeRootCounts[i] + ")")
	}
}

type adjustedValue struct {
	code string
	kind monoKind
}

func adjustedLuaValue(value adjustedValue) string {
	if value.kind == monoNumber {
		return "rt.NumVal(" + value.code + ")"
	}
	return value.code
}

func adjustedNumber(value adjustedValue) string {
	if value.kind == monoNumber {
		return value.code
	}
	return "(" + value.code + ").NumberOr(0)"
}

// emitAdjustedValues evaluates a Lua expression list: every expression runs
// (extra results still execute for side effects), a trailing call expands to
// its declared results, and the list is then truncated or nil-padded to
// nWanted slots.
func (g *generator) emitAdjustedValues(b *sourceBuilder, values []luaparser.Expr, nWanted, indent int) []adjustedValue {
	pad := strings.Repeat("\t", indent)
	out := make([]adjustedValue, nWanted)
	for i := range out {
		out[i] = adjustedValue{code: "rt.NilVal()", kind: monoValue}
	}
	if len(values) == 0 {
		return out
	}
	savedRoots := g.extraRoots
	g.extraRoots = append([]string(nil), savedRoots...)
	slot := 0
	last := len(values) - 1
	for i, v := range values {
		if i == last {
			if call, ok := v.(*luaparser.CallExpr); ok {
				arity, known := g.callResultArity(call)
				if known && arity == 0 {
					// A known zero-result call expands to no value slots,
					// but it still executes when it is the trailing expression.
					if callExpr := g.emitCall(call, false); callExpr != "" {
						g.writePending(b, indent)
						b.line(pad + callExpr)
					} else {
						g.writePending(b, indent)
					}
					continue
				}
				if fn := g.calledFunction(call); fn != nil && fn.returnN > 1 {
					parts := make([]string, fn.returnN)
					used := 0
					for j := range parts {
						if slot+used < nWanted {
							parts[j] = g.next("ret")
							used++
						} else {
							parts[j] = "_"
						}
					}
					rhs := g.emitMultiCall(call, fn)
					g.writePending(b, indent)
					if used == 0 {
						// A trailing multi-return call can be beyond the
						// assignment's target list (`local a = 1, pair()`).
						// A short declaration containing only blank identifiers
						// is invalid Go, but the call itself still has to run.
						b.line(pad + rhs)
					} else {
						b.line(pad + strings.Join(parts, ", ") + " := " + rhs)
					}
					for j := 0; j < used; j++ {
						kind := fn.returnKind(j)
						out[slot] = adjustedValue{code: parts[j], kind: kind}
						if kind == monoValue {
							g.extraRoots = append(g.extraRoots, parts[j])
						}
						slot++
					}
					continue
				}
			}
		}
		kind := monoValue
		var expr string
		if g.exprNumber(v) {
			kind = monoNumber
			expr = g.emitNumberExpr(v)
			g.writePending(b, indent)
		} else {
			expr = g.emitExprFlush(b, indent, v)
		}
		tmp := g.next("rhs")
		b.line(pad + tmp + " := " + expr)
		if slot < nWanted {
			out[slot] = adjustedValue{code: tmp, kind: kind}
			if kind == monoValue {
				g.extraRoots = append(g.extraRoots, tmp)
			}
			slot++
		} else {
			b.line(pad + "_ = " + tmp)
		}
	}
	g.extraRoots = savedRoots
	return out
}

func (g *generator) writePending(b *sourceBuilder, indent int) {
	if len(g.pending) == 0 {
		return
	}
	pad := strings.Repeat("\t", indent)
	for _, s := range g.pending {
		for _, line := range strings.Split(s, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			b.line(pad + line)
		}
	}
	g.pending = g.pending[:0]
}

func (g *generator) emitExprFlush(b *sourceBuilder, indent int, e luaparser.Expr) string {
	expr := g.emitExpr(e)
	g.writePending(b, indent)
	return expr
}

func (g *generator) capturePending(fn func() string) (stmts []string, expr string) {
	saved := g.pending
	g.pending = nil
	expr = fn()
	stmts = g.pending
	g.pending = saved
	return stmts, expr
}

func (g *generator) indentPending(stmts []string) {
	for _, s := range stmts {
		for _, line := range strings.Split(s, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			g.pending = append(g.pending, "\t"+line)
		}
	}
}

// sequenceExprs evaluates a list of Lua expressions in the same order Lua
// would, one Go statement at a time, each bound to its own temp variable
// and immediately added to g.extraRoots. Statements are appended to
// g.pending so the enclosing statement can flush them without wrapping an
// immediately-invoked function (TinyGo -gc=none cannot link runtime.alloc).
// With 0 or 1 expressions there is no sibling to race against, so no extra
// temp is introduced (nested pending from that one expression still runs).
func (g *generator) sequenceExprs(exprs []luaparser.Expr) []string {
	names := make([]string, len(exprs))
	if len(exprs) == 0 {
		return names
	}
	savedRoots := g.extraRoots
	g.extraRoots = append([]string(nil), savedRoots...)
	for i, e := range exprs {
		expr := g.emitExpr(e)
		if len(exprs) == 1 {
			names[i] = expr
			continue
		}
		tmp := g.next("op")
		g.pending = append(g.pending, tmp+" := "+expr)
		names[i] = tmp
		g.extraRoots = append(g.extraRoots, tmp)
	}
	g.extraRoots = savedRoots
	return names
}

func (g *generator) emitGenericFor(b *sourceBuilder, s *luaparser.GenericForStmt, indent int) {
	pad := strings.Repeat("\t", indent)
	if len(s.Values) != 1 {
		g.setErr(errors.New("transpiler: generic for requires one all(t) or pairs(t) iterator"))
		return
	}
	call, ok := s.Values[0].(*luaparser.CallExpr)
	if !ok {
		g.setErr(errors.New("transpiler: generic for requires all(t) or pairs(t)"))
		return
	}
	name := ""
	if v, ok := call.Callee.(*luaparser.VarExpr); ok {
		name = v.Name
	}
	if (name != "all" && name != "pairs") || g.exprListWidth(call.Args) != 1 {
		g.setErr(errors.New("transpiler: generic for requires all(t) or pairs(t) with one table argument"))
		return
	}
	expected := 1
	if name == "pairs" {
		expected = 2
	}
	if len(s.Names) != expected {
		g.setErr(fmt.Errorf("transpiler: %s iterator requires %d loop variables", name, expected))
		return
	}
	table := "gluIter" + g.next("t")
	tableExpr := g.emitExprFlush(b, indent, firstArg(call))
	b.line(pad + table + " := " + tableExpr)
	root := g.next("iterRoot")
	g.requireActiveRootSlots(1, s.Pos())
	b.line(pad + root + " := gluPushRoots(c, " + table + ")")
	b.line(pad + "if " + root + " >= 0 {")
	rootDepth := len(g.activeRootCounts)
	g.activeRootCounts = append(g.activeRootCounts, root)
	rootSlots := g.activeRootSlots
	g.activeRootSlots++
	g.pushScope()
	loopVars := make([]string, len(s.Names))
	for i, n := range s.Names {
		loopVars[i] = g.declareLocal(n)
	}
	switch name {
	case "all":
		idx := g.next("i")
		limit := "c.TableLen(" + table + ")"
		if g.iteratorLengthStable(s.Body) {
			limit = g.next("iterLen")
			b.line(pad + "\t" + limit + " := c.TableLen(" + table + ")")
		}
		b.line(pad + "\tfor " + idx + " := 1; " + idx + " <= " + limit + "; " + idx + "++ {")
		if len(loopVars) > 0 {
			b.line(pad + "\t\t" + loopVars[0] + " := c.TableGetInt(" + table + ", " + idx + ")")
			b.line(pad + "\t\t_ = " + loopVars[0])
		}
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+2)
		}
		g.popScope()
		b.line(pad + "\t}")
	case "pairs":
		idx := g.next("i")
		b.line(pad + "\tfor " + idx + " := 0; " + idx + " < c.TableSlotCount(" + table + "); " + idx + "++ {")
		b.line(pad + "\t\t" + g.next("key") + " := c.TableKeyAt(" + table + ", " + idx + ")")
		key := g.lastName("key")
		b.line(pad + "\t\tif " + key + ".Kind == rt.KindNil { continue }")
		if len(loopVars) > 0 {
			b.line(pad + "\t\t" + loopVars[0] + " := " + key)
			b.line(pad + "\t\t_ = " + loopVars[0])
		}
		if len(loopVars) > 1 {
			b.line(pad + "\t\t" + loopVars[1] + " := c.TableValueAt(" + table + ", " + idx + ")")
			b.line(pad + "\t\t_ = " + loopVars[1])
		}
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+2)
		}
		g.popScope()
		b.line(pad + "\t}")
	default:
		g.setErr(fmt.Errorf("transpiler: unsupported iterator %q; use all or pairs", name))
	}
	g.popScope()
	g.activeRootCounts = g.activeRootCounts[:rootDepth]
	g.activeRootSlots = rootSlots
	b.line(pad + "\tgluPopRoots(c, " + root + ")")
	b.line(pad + "}")
}

func firstArg(c *luaparser.CallExpr) luaparser.Expr {
	if len(c.Args) == 0 {
		return &luaparser.NilExpr{}
	}
	return c.Args[0]
}

// fixedNumberLiteral mirrors runtime.Number's signed 16:16 conversion. The
// explicit int32 step preserves the runtime's two's-complement wrapping while
// keeping the emitted Go constant within the Number type's representable range.
func fixedNumberLiteral(value float64) int32 {
	return int32(int64(math.Round(value * 65536)))
}

// targetIntLiteral reports whether a source integer can be passed through a
// generated int parameter on the 32-bit GBA target. Larger integers are still
// valid 16:16 values, but must be emitted as their already-wrapped Number bits.
func targetIntLiteral(value float64) (int64, bool) {
	if value < -2147483648 || value > 2147483647 {
		return 0, false
	}
	return int64(value), true
}

func (g *generator) emitExpr(e luaparser.Expr) string {
	if g.exprNumber(e) {
		return "rt.NumVal(" + g.emitNumberExpr(e) + ")"
	}
	switch x := e.(type) {
	case nil:
		return "rt.NilVal()"
	case *luaparser.NilExpr:
		return "rt.NilVal()"
	case *luaparser.BoolExpr:
		return fmt.Sprintf("rt.BoolVal(%t)", x.Value)
	case *luaparser.NumberExpr:
		if x.Integer {
			if value, ok := targetIntLiteral(x.Value); ok {
				return fmt.Sprintf("rt.IntVal(%d)", value)
			}
		}
		return fmt.Sprintf("rt.NumVal(rt.Number(%d))", fixedNumberLiteral(x.Value))
	case *luaparser.StringExpr:
		return g.staticExpr(x.Value)
	case *luaparser.ParenExpr:
		return g.emitExpr(x.Expr)
	case *luaparser.VarExpr:
		return g.lookup(x.Name)
	case *luaparser.UnaryExpr:
		if x.Op == luaparser.OpLength {
			if base, ok := x.Expr.(*luaparser.VarExpr); ok {
				if info := g.numericArrays[base.Name]; info != nil {
					if _, local := g.lookupLocal(base.Name); !local {
						return "rt.IntVal(" + strconv.Itoa(info.length) + ")"
					}
				}
			}
		}
		v := g.emitExpr(x.Expr)
		switch x.Op {
		case luaparser.OpNot:
			return "rt.BoolVal(!(" + v + ").Truthy())"
		case luaparser.OpNegate:
			return "rt.NumVal(-(" + v + ").NumberOr(0))"
		case luaparser.OpBitNot:
			return "rt.NumVal((" + v + ").NumberOr(0).Bnot())"
		case luaparser.OpLength:
			return "gluLen(c," + v + ")"
		}
	case *luaparser.BinaryExpr:
		return g.emitBinary(x)
	case *luaparser.IndexExpr:
		names := g.sequenceExprs([]luaparser.Expr{x.Base, x.Key})
		return "c.TableGet(" + names[0] + ", " + names[1] + ")"
	case *luaparser.FieldExpr:
		if record, field, ok := g.numericRecordField(x); ok {
			return "rt.NumVal(g." + numericRecordMember(record, field) + ")"
		}
		if cache := g.fieldCacheRef(x); cache != "" {
			return "c.TableGetStable(" + g.emitExpr(x.Base) + ", " + g.staticExpr(x.Name) + ", " + cache + ")"
		}
		return "c.TableGet(" + g.emitExpr(x.Base) + ", " + g.staticExpr(x.Name) + ")"
	case *luaparser.CallExpr:
		return g.emitCall(x, true)
	case *luaparser.TableExpr:
		return g.emitTableLiteral(x)
	}
	return "rt.NilVal()"
}

func (g *generator) exprNumber(expr luaparser.Expr) bool {
	if g.opts.DisableMonomorphicNumbers {
		return false
	}
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return g.exprNumber(x.Expr)
	case *luaparser.NumberExpr:
		return true
	case *luaparser.VarExpr:
		return g.lookupKind(x.Name) == monoNumber
	case *luaparser.FieldExpr:
		if _, _, ok := g.numericRecordField(x); ok {
			return true
		}
		return !g.opts.DisableStableTableFields && g.numericFields[x.Name]
	case *luaparser.IndexExpr:
		_, _, ok := g.numericArrayIndex(x)
		return ok
	case *luaparser.UnaryExpr:
		if x.Op == luaparser.OpLength {
			if base, ok := x.Expr.(*luaparser.VarExpr); ok {
				if info := g.numericArrays[base.Name]; info != nil {
					if _, local := g.lookupLocal(base.Name); !local {
						return true
					}
				}
			}
			return !g.opts.DisableNumericLengths
		}
		return (x.Op == luaparser.OpNegate || x.Op == luaparser.OpBitNot) && g.exprNumber(x.Expr)
	case *luaparser.BinaryExpr:
		switch x.Op {
		case luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
			luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo, luaparser.OpPower,
			luaparser.OpBitOr, luaparser.OpBitXor, luaparser.OpBitAnd,
			luaparser.OpShiftL, luaparser.OpShiftR, luaparser.OpShiftRLogical,
			luaparser.OpRotateL, luaparser.OpRotateR:
			return g.exprNumber(x.Left) && g.exprNumber(x.Right)
		}
	case *luaparser.CallExpr:
		name, ok := callKey(x)
		if !ok {
			return false
		}
		if fn := g.calledFunction(x); fn != nil {
			return fn.returnN > 0 && fn.returnKind(0) == monoNumber
		}
		if name == "min" || name == "max" || name == "mid" || name == "mget" {
			return true
		}
		requireNumbers := func() bool {
			for i, arg := range x.Args {
				if i == len(x.Args)-1 {
					if call, ok := arg.(*luaparser.CallExpr); ok {
						if fn := g.calledFunction(call); fn != nil && fn.returnN > 1 {
							for result := 0; result < fn.returnN; result++ {
								if fn.returnKind(result) != monoNumber {
									return false
								}
							}
							continue
						}
					}
				}
				if !g.exprNumber(arg) {
					return false
				}
			}
			return true
		}
		switch name {
		case "flr", "ceil", "abs", "sgn", "min", "max", "mid", "sin", "cos", "atan2", "sqrt", "rnd",
			"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr", "t", "time", "stat":
			return requireNumbers()
		}
	}
	return false
}

func (g *generator) emitNumberExpr(expr luaparser.Expr) string {
	switch x := expr.(type) {
	case *luaparser.ParenExpr:
		return g.emitNumberExpr(x.Expr)
	case *luaparser.NumberExpr:
		return fmt.Sprintf("rt.Number(%d)", fixedNumberLiteral(x.Value))
	case *luaparser.VarExpr:
		return g.lookup(x.Name)
	case *luaparser.FieldExpr:
		if record, field, ok := g.numericRecordField(x); ok {
			return "g." + numericRecordMember(record, field)
		}
		return g.emitStableNumberRead(x)
	case *luaparser.IndexExpr:
		if name, _, ok := g.numericArrayIndex(x); ok {
			key := g.emitExpr(x.Key)
			if view := g.borrowedNumberArrays[name]; view != "" {
				return view + ".Get((" + key + ").Int())"
			}
			return "c.NumberArrayGet(" + g.lookup(name) + ", (" + key + ").Int())"
		}
	case *luaparser.UnaryExpr:
		if x.Op == luaparser.OpLength {
			if base, ok := x.Expr.(*luaparser.VarExpr); ok {
				if info := g.numericArrays[base.Name]; info != nil {
					if _, local := g.lookupLocal(base.Name); !local {
						return "rt.FromInt(" + strconv.Itoa(info.length) + ")"
					}
				}
			}
			// gluLen returns IntVal for every operand kind, including zero
			// for unsupported types. Its input must still evaluate normally.
			return "(gluLen(c," + g.emitExpr(x.Expr) + ")).Num"
		}
		value := g.emitNumberExpr(x.Expr)
		if x.Op == luaparser.OpNegate {
			// A variable forces signed 16:16 wrapping instead of Go constant
			// overflow, including parenthesized literals and inlined globals.
			tmp := g.next("neg")
			g.pending = append(g.pending, tmp+" := "+value)
			return "-(" + tmp + ")"
		}
		return "(" + value + ").Bnot()"
	case *luaparser.BinaryExpr:
		if specialized, ok := g.emitConstantNumberDivision(x); ok {
			return specialized
		}
		if specialized, ok := g.emitConstantNumberModulo(x); ok {
			return specialized
		}
		parts := g.sequenceNumberExprs([]luaparser.Expr{x.Left, x.Right})
		left, right := parts[0], parts[1]
		switch x.Op {
		case luaparser.OpAdd:
			return "(" + left + " + " + right + ")"
		case luaparser.OpSubtract:
			return "(" + left + " - " + right + ")"
		case luaparser.OpMultiply:
			return "(" + left + ").Mul(" + right + ")"
		case luaparser.OpDivide:
			return "(" + left + ").Div(" + right + ")"
		case luaparser.OpIntegerDivide:
			return "(" + left + ").IDiv(" + right + ")"
		case luaparser.OpModulo:
			return "gluNumberMod(" + left + ", " + right + ")"
		case luaparser.OpPower:
			return "gluNumberPow(" + left + ", " + right + ")"
		case luaparser.OpBitOr:
			return "(" + left + ").Bor(" + right + ")"
		case luaparser.OpBitXor:
			return "(" + left + ").Bxor(" + right + ")"
		case luaparser.OpBitAnd:
			return "(" + left + ").Band(" + right + ")"
		case luaparser.OpShiftL:
			return "(" + left + ").Shl(" + right + ")"
		case luaparser.OpShiftR:
			return "(" + left + ").Shr(" + right + ")"
		case luaparser.OpShiftRLogical:
			return "(" + left + ").Lshr(" + right + ")"
		case luaparser.OpRotateL:
			return "(" + left + ").Rotl(" + right + ")"
		case luaparser.OpRotateR:
			return "(" + left + ").Rotr(" + right + ")"
		}
	case *luaparser.CallExpr:
		if fn := g.calledFunction(x); fn != nil && fn.returnN > 0 && fn.returnKind(0) == monoNumber {
			if fn.returnN == 1 {
				return g.emitUserCall(x, fn)
			}
			return g.emitFirstCall(x, fn)
		}
		name, _ := callKey(x)
		if name == "mget" {
			return "(" + g.emitBuiltin("mget", x.Args, true) + ").Num"
		}
		var arg func(int, string) string
		if g.numberBuiltinArgs(x.Args) {
			args := g.sequenceNumberExprs(x.Args)
			arg = func(i int, fallback string) string {
				if i >= len(args) {
					return fallback
				}
				return args[i]
			}
		} else {
			values := g.sequenceAdjustedExprs(x.Args)
			args := make([]string, len(values))
			for i, value := range values {
				args[i] = adjustedNumber(value)
			}
			arg = func(i int, fallback string) string {
				if i >= len(args) {
					return fallback
				}
				return args[i]
			}
		}
		switch name {
		case "flr":
			return "(" + arg(0, "0") + ").Flr()"
		case "ceil":
			return "(" + arg(0, "0") + ").Ceil()"
		case "abs":
			return "(" + arg(0, "0") + ").Abs()"
		case "sgn":
			return "(" + arg(0, "0") + ").Sgn()"
		case "min":
			return "rt.Min(" + arg(0, "0") + ", " + arg(1, "0") + ")"
		case "max":
			return "rt.Max(" + arg(0, "0") + ", " + arg(1, "0") + ")"
		case "mid":
			return "rt.Mid(" + arg(0, "0") + ", " + arg(1, "0") + ", " + arg(2, "0") + ")"
		case "sin":
			return "(" + arg(0, "0") + ").Sin()"
		case "cos":
			return "(" + arg(0, "0") + ").Cos()"
		case "sqrt":
			return "(" + arg(0, "0") + ").Sqrt()"
		case "atan2":
			return "rt.Atan2(" + arg(0, "0") + ", " + arg(1, "0") + ")"
		case "rnd":
			return "c.Rnd(" + arg(0, "rt.One") + ")"
		case "band":
			return "(" + arg(0, "0") + ").Band(" + arg(1, "0") + ")"
		case "bor":
			return "(" + arg(0, "0") + ").Bor(" + arg(1, "0") + ")"
		case "bxor":
			return "(" + arg(0, "0") + ").Bxor(" + arg(1, "0") + ")"
		case "bnot":
			return "(" + arg(0, "0") + ").Bnot()"
		case "shl":
			return "(" + arg(0, "0") + ").Shl(" + arg(1, "0") + ")"
		case "shr":
			return "(" + arg(0, "0") + ").Shr(" + arg(1, "0") + ")"
		case "lshr":
			return "(" + arg(0, "0") + ").Lshr(" + arg(1, "0") + ")"
		case "rotl":
			return "(" + arg(0, "0") + ").Rotl(" + arg(1, "0") + ")"
		case "rotr":
			return "(" + arg(0, "0") + ").Rotr(" + arg(1, "0") + ")"
		case "t", "time":
			return "c.Time()"
		case "stat":
			return "c.Stat((" + arg(0, "0") + ").Int())"
		}
	}
	g.setErr(fmt.Errorf("transpiler: unsupported monomorphic number expression at %s", expr.Pos()))
	return "0"
}

func (g *generator) numberBuiltinArgs(args []luaparser.Expr) bool {
	for _, arg := range args {
		if !g.exprNumber(arg) {
			return false
		}
		call, ok := unparenExpr(arg).(*luaparser.CallExpr)
		if !ok {
			continue
		}
		if fn := g.calledFunction(call); fn != nil && fn.returnN > 1 {
			return false
		}
	}
	return true
}

func (g *generator) sequenceNumberExprs(exprs []luaparser.Expr) []string {
	names := make([]string, len(exprs))
	for i, expr := range exprs {
		code := g.emitNumberExpr(expr)
		if len(exprs) == 1 {
			names[i] = code
			continue
		}
		tmp := g.next("num")
		g.pending = append(g.pending, tmp+" := "+code)
		names[i] = tmp
	}
	return names
}

func (g *generator) emitBinary(x *luaparser.BinaryExpr) string {
	if x.Op == luaparser.OpAnd || x.Op == luaparser.OpOr {
		// `and`/`or` short-circuit, so the right operand cannot run unless
		// the left requires it. Capture the right side's pending statements
		// into the corresponding if-body instead of wrapping a closure.
		leftStmts, leftExpr := g.capturePending(func() string { return g.emitExpr(x.Left) })
		g.pending = append(g.pending, leftStmts...)
		leftName := g.next("logic")
		g.pending = append(g.pending, leftName+" := "+leftExpr)
		savedRoots := g.extraRoots
		g.extraRoots = append(append([]string(nil), savedRoots...), leftName)
		rightStmts, rightExpr := g.capturePending(func() string { return g.emitExpr(x.Right) })
		g.extraRoots = savedRoots
		result := g.next("logic")
		g.pending = append(g.pending, result+" := "+leftName)
		cond := leftName + ".Truthy()"
		if x.Op == luaparser.OpAnd {
			g.pending = append(g.pending, "if "+cond+" {")
		} else {
			g.pending = append(g.pending, "if !"+cond+" {")
		}
		g.indentPending(rightStmts)
		g.pending = append(g.pending, "\t"+result+" = "+rightExpr)
		g.pending = append(g.pending, "}")
		return result
	}
	if x.Op == luaparser.OpConcat {
		names := g.sequenceExprs([]luaparser.Expr{x.Left, x.Right})
		return "gluConcat(c," + names[0] + "," + names[1] + g.rootArgsFor(2, x.Pos()) + ")"
	}
	if specialized, ok := g.emitConstantValueArithmetic(x); ok {
		return specialized
	}
	if g.exprNumber(x.Left) && g.exprNumber(x.Right) {
		parts := g.sequenceNumberExprs([]luaparser.Expr{x.Left, x.Right})
		left, right := parts[0], parts[1]
		switch x.Op {
		case luaparser.OpEqual, luaparser.OpLess, luaparser.OpLessEq, luaparser.OpGreater, luaparser.OpGreaterEq:
			return "rt.BoolVal(" + left + " " + string(x.Op) + " " + right + ")"
		case luaparser.OpNotEqual:
			return "rt.BoolVal(" + left + " != " + right + ")"
		}
	}
	names := g.sequenceExprs([]luaparser.Expr{x.Left, x.Right})
	return g.emitEagerBinary(x.Op, names[0], names[1])
}

func (g *generator) emitEagerBinary(op luaparser.Operator, left, right string) string {
	switch op {
	case luaparser.OpEqual:
		return "rt.BoolVal(c.ValuesEqual(" + left + "," + right + "))"
	case luaparser.OpNotEqual:
		return "rt.BoolVal(!c.ValuesEqual(" + left + "," + right + "))"
	case luaparser.OpLess, luaparser.OpLessEq, luaparser.OpGreater, luaparser.OpGreaterEq:
		return "rt.BoolVal(gluCmp(c," + left + "," + right + ",\"" + string(op) + "\"))"
	case luaparser.OpAdd:
		return "(" + left + ").Add(" + right + ")"
	case luaparser.OpSubtract:
		return "(" + left + ").Sub(" + right + ")"
	case luaparser.OpMultiply:
		return "(" + left + ").Mul(" + right + ")"
	case luaparser.OpDivide:
		return "(" + left + ").Div(" + right + ")"
	case luaparser.OpIntegerDivide:
		return "(" + left + ").IDiv(" + right + ")"
	case luaparser.OpModulo:
		return "gluMod(" + left + "," + right + ")"
	case luaparser.OpPower:
		return "gluPow(" + left + "," + right + ")"
	case luaparser.OpBitOr:
		return "rt.NumVal((" + left + ").NumberOr(0).Bor((" + right + ").NumberOr(0)))"
	case luaparser.OpBitXor:
		return "rt.NumVal((" + left + ").NumberOr(0).Bxor((" + right + ").NumberOr(0)))"
	case luaparser.OpBitAnd:
		return "rt.NumVal((" + left + ").NumberOr(0).Band((" + right + ").NumberOr(0)))"
	case luaparser.OpShiftL:
		return "rt.NumVal((" + left + ").NumberOr(0).Shl((" + right + ").NumberOr(0)))"
	case luaparser.OpShiftR:
		return "rt.NumVal((" + left + ").NumberOr(0).Shr((" + right + ").NumberOr(0)))"
	case luaparser.OpShiftRLogical:
		return "rt.NumVal((" + left + ").NumberOr(0).Lshr((" + right + ").NumberOr(0)))"
	case luaparser.OpRotateL:
		return "rt.NumVal((" + left + ").NumberOr(0).Rotl((" + right + ").NumberOr(0)))"
	case luaparser.OpRotateR:
		return "rt.NumVal((" + left + ").NumberOr(0).Rotr((" + right + ").NumberOr(0)))"
	}
	return "rt.NilVal()"
}

func (g *generator) emitTableLiteral(x *luaparser.TableExpr) string {
	// Lua expands a multi-return call into all of its results only when it
	// is the constructor's last field and that field is a plain positional
	// (array) entry, not a [k]=v or name=v field; everywhere else - non-last
	// fields, hash fields - a multi-return call yields just its first value,
	// same as any other expression context.
	var trailingFn *functionInfo
	var trailingCall *luaparser.CallExpr
	trailingZero := false
	if n := len(x.Fields); n > 0 && x.Fields[n-1].Array {
		if call, ok := x.Fields[n-1].Value.(*luaparser.CallExpr); ok {
			if arity, known := g.callResultArity(call); known && arity == 0 {
				trailingZero, trailingCall = true, call
			} else if fn := g.calledFunction(call); fn != nil && fn.returnN > 1 {
				trailingFn, trailingCall = fn, call
			}
		}
	}
	arrayN := 0
	hashN := 0
	for _, f := range x.Fields {
		if f.Array {
			arrayN++
		} else {
			hashN++
		}
	}
	if trailingZero {
		arrayN--
	} else if trailingFn != nil {
		arrayN += trailingFn.returnN - 1
	}
	arrayN = max(arrayN, g.literalArrayCaps[x])
	arrayCap, hashCap := max(arrayN, 1), max(hashN, 1)
	literalPlans := g.staticTableLiteralPlans(x, arrayCap, hashCap)
	borrowLiteral := g.borrowableTableLiteral(x, literalPlans)
	trustedLiteral := borrowLiteral && g.trustedTableLiteralValues(x)
	image := g.tableLiteralImage(x, literalPlans, arrayCap, hashCap)
	keepTableRoot := g.tableLiteralNeedsLiveRoot(x)
	initialRoots := g.rootVars()
	skipRooting := len(initialRoots) == 0 && !keepTableRoot && trailingFn == nil && !trailingZero
	if skipRooting && image.name != "" {
		return g.emitTrustedImageConstructor(x, image)
	}
	if skipRooting && len(x.Fields) == 0 {
		return g.emitEmptyTableConstructor(arrayCap, hashCap)
	}
	tbl := g.next("tbl")
	fail := g.next("tblFail")
	pushed := ""
	errName := g.next("tblErr")
	g.pending = append(g.pending, "var "+tbl+" rt.Value")
	g.pending = append(g.pending, fail+" := false")
	// Existing Values only need to remain rooted across the allocation itself.
	// Once the table exists, release those temporary copies and keep just the
	// new table persistently rooted; nested field expressions will root the
	// caller's still-live Go Values only while they allocate.
	if skipRooting {
		g.pending = append(g.pending, "var "+errName+" rt.HeapError")
		if image.name != "" {
			g.pending = append(g.pending, tbl+", "+errName+" = c.NewTableFromImage("+image.name+"[:])")
		} else {
			g.pending = append(g.pending, tbl+", "+errName+" = c.NewTableWithCaps("+strconv.Itoa(arrayCap)+", "+strconv.Itoa(hashCap)+")")
		}
		g.pending = append(g.pending, "if "+errName+" != rt.HeapOK { "+fail+" = true }")
	} else {
		pushed = g.next("tblPush")
		g.pending = append(g.pending, pushed+" := 0")
		g.requireActiveRootSlots(len(initialRoots), x.Pos())
		for _, root := range initialRoots {
			g.pending = append(g.pending, "if !"+fail+" {")
			g.pending = append(g.pending, "\tif c.PushLocal("+root+") { "+pushed+"++ } else { "+fail+" = true }")
			g.pending = append(g.pending, "}")
		}
		g.pending = append(g.pending, "if !"+fail+" {")
		g.pending = append(g.pending, "\tvar "+errName+" rt.HeapError")
		if image.name != "" {
			g.pending = append(g.pending, "\t"+tbl+", "+errName+" = c.NewTableFromImage("+image.name+"[:])")
		} else {
			g.pending = append(g.pending, "\t"+tbl+", "+errName+" = c.NewTableWithCaps("+strconv.Itoa(arrayCap)+", "+strconv.Itoa(hashCap)+")")
		}
		g.pending = append(g.pending, "\tif "+errName+" != rt.HeapOK { "+fail+" = true }")
		g.pending = append(g.pending, "}")
		g.pending = append(g.pending, "c.PopLocals("+pushed+")")
		g.pending = append(g.pending, pushed+" = 0")
		if keepTableRoot {
			g.requireActiveRootSlots(1, x.Pos())
			g.pending = append(g.pending, "if !"+fail+" {")
			g.pending = append(g.pending, "\tif c.PushLocal("+tbl+") { "+pushed+" = 1 } else { "+fail+" = true }")
			g.pending = append(g.pending, "}")
		}
	}
	view := ""
	if image.name != "" && len(image.dynamic) != 0 || (borrowLiteral && image.name == "") {
		view = g.next("tblView")
		g.pending = append(g.pending, "var "+view+" rt.TableLiteralView")
		borrow := "c.BorrowTableLiteral(" + tbl + ")"
		if trustedLiteral && image.name == "" {
			borrow = "c.BorrowFreshTableLiteral(" + tbl + "," + strconv.Itoa(arrayCap) + "," + strconv.Itoa(hashCap) + ")"
		}
		g.pending = append(g.pending, "if !"+fail+" { "+view+" = "+borrow+" }")
	}
	oldRoots := g.extraRoots
	g.extraRoots = append([]string(nil), oldRoots...)
	rootSlots := g.activeRootSlots
	g.activeRootSlots++
	if trailingFn != nil {
		resultRoots := 0
		for i := 0; i < trailingFn.returnN; i++ {
			if trailingFn.returnKind(i) == monoValue {
				resultRoots++
			}
		}
		g.requireActiveRootSlots(resultRoots, x.Pos())
	}
	arrayIndex := 0
	for i, f := range x.Fields {
		last := i == len(x.Fields)-1
		firstArrayIndex := arrayIndex + 1
		if f.Array {
			if last && trailingZero {
				// A trailing zero-result call contributes no list field.
			} else if last && trailingFn != nil {
				arrayIndex += trailingFn.returnN
			} else {
				arrayIndex++
			}
		}
		if image.name != "" {
			if _, overlay := image.dynamic[i]; !overlay && !f.Array {
				continue
			}
		}
		g.pending = append(g.pending, "if !"+fail+" {")
		inner, _ := g.capturePending(func() string {
			if f.Array {
				if last && trailingZero {
					statement := g.emitCall(trailingCall, false)
					if statement != "" {
						g.pending = append(g.pending, statement)
					}
					return ""
				}
				if last && trailingFn != nil {
					results := make([]string, trailingFn.returnN)
					for j := range results {
						results[j] = g.next("ret")
					}
					rhs := g.emitMultiCall(trailingCall, trailingFn)
					g.pending = append(g.pending, strings.Join(results, ", ")+" := "+rhs)
					for j, r := range results {
						if trailingFn.returnKind(j) == monoValue {
							g.pending = append(g.pending, "if !c.PushLocal("+r+") { "+fail+" = true } else { "+pushed+"++ }")
						}
					}
					for j, r := range results {
						value := adjustedLuaValue(adjustedValue{code: r, kind: trailingFn.returnKind(j)})
						g.pending = append(g.pending, "if !"+fail+" {")
						g.pending = append(g.pending, "\tif c.TableSet("+tbl+",rt.IntVal("+strconv.Itoa(firstArrayIndex+j)+"),"+value+") != rt.HeapOK { "+fail+" = true }")
						g.pending = append(g.pending, "}")
					}
					return ""
				}
				v := g.emitExpr(f.Value)
				g.pending = append(g.pending, "if c.TableSet("+tbl+",rt.IntVal("+strconv.Itoa(firstArrayIndex)+"),"+v+") != rt.HeapOK { "+fail+" = true }")
				return ""
			}
			if image.name != "" {
				plan, overlay := image.dynamic[i]
				if !overlay {
					return ""
				}
				key, _ := unparenExpr(f.Key).(*luaparser.StringExpr)
				keyID, _ := g.staticID(key.Value)
				if _, nested := unparenExpr(f.Value).(*luaparser.TableExpr); nested {
					value := g.emitExpr(f.Value)
					g.pending = append(g.pending, "if !"+fail+" { "+view+" = c.BorrowTableLiteral("+tbl+") }")
					g.pending = append(g.pending, "if "+view+".StorePlanned("+strconv.Itoa(int(keyID))+","+value+","+strconv.FormatUint(uint64(plan), 10)+") != rt.HeapOK { "+fail+" = true }")
					return ""
				}
				if g.exprNumber(f.Value) {
					value := g.emitNumberExpr(f.Value)
					g.pending = append(g.pending, "if "+view+".StorePlannedNumber("+strconv.Itoa(int(keyID))+","+value+","+strconv.FormatUint(uint64(plan), 10)+") != rt.HeapOK { "+fail+" = true }")
				} else {
					value := g.emitExpr(f.Value)
					g.pending = append(g.pending, "if "+view+".StorePlanned("+strconv.Itoa(int(keyID))+","+value+","+strconv.FormatUint(uint64(plan), 10)+") != rt.HeapOK { "+fail+" = true }")
				}
				return ""
			}
			names := g.sequenceExprs([]luaparser.Expr{f.Key, f.Value})
			if plan, ok := literalPlans[i]; ok {
				callee := "c.TableInitStatic(" + tbl + ","
				if view != "" {
					callee = view + ".InitStatic("
					if trustedLiteral {
						callee = view + ".InitKnown("
					}
				}
				g.pending = append(g.pending, "if "+callee+"("+names[0]+").SRef,"+names[1]+","+strconv.FormatUint(uint64(plan), 10)+") != rt.HeapOK { "+fail+" = true }")
			} else {
				g.pending = append(g.pending, "if c.TableSet("+tbl+","+names[0]+","+names[1]+") != rt.HeapOK { "+fail+" = true }")
			}
			return ""
		})
		g.indentPending(inner)
		g.pending = append(g.pending, "}")
	}
	g.extraRoots = oldRoots
	g.activeRootSlots = rootSlots
	if !skipRooting {
		g.pending = append(g.pending, "c.PopLocals("+pushed+")")
	}
	g.pending = append(g.pending, "if "+fail+" { "+tbl+" = rt.NilVal() }")
	return tbl
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func checkedProduct(a, b int) (int, bool) {
	if a < 0 || b < 0 {
		return 0, false
	}
	if a == 0 || b == 0 {
		return 0, true
	}
	maxInt := int(^uint(0) >> 1)
	if a > maxInt/b {
		return 0, false
	}
	product := a * b
	if uint64(product) > uint64(^uint32(0)) {
		return 0, false
	}
	return product, true
}

func compactMapPlanes(data []uint16, width, height, layers int, mask uint8) []uint16 {
	planeCells := width * height
	if planeCells <= 0 || layers <= 0 || mask == 0 {
		return nil
	}
	out := make([]uint16, 0, planeCells*mapMaskCount(mask))
	for bg := 0; bg < layers; bg++ {
		if mask&(1<<uint(bg)) == 0 {
			continue
		}
		base := bg * planeCells
		if base+planeCells <= len(data) {
			out = append(out, data[base:base+planeCells]...)
		} else {
			out = append(out, make([]uint16, planeCells)...)
		}
	}
	return out
}

// mapMaskSlot maps a stable logical BG number to its dense allocation slot.
// A missing/empty plane returns -1 and is a runtime no-op.
func mapMaskSlot(mask uint8, bg int) int {
	if bg < 0 || bg >= assetsync.MapLayerCount || mask&(1<<uint(bg)) == 0 {
		return -1
	}
	return mapMaskCount(mask & uint8((1<<uint(bg))-1))
}

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
func (g *generator) staticExpr(s string) string {
	if _, ok := g.staticGo[s]; !ok {
		g.staticValue(s)
	}
	if g.current == nil {
		return "c.InternStatic(" + strconv.Quote(s) + ")"
	}
	return "g." + g.staticGo[s]
}
func (g *generator) fieldCacheRef(e *luaparser.FieldExpr) string {
	if g.opts.DisableStableTableFields {
		return ""
	}
	if _, _, ok := g.numericRecordField(e); ok {
		return ""
	}
	id, ok := g.fieldCaches[e]
	if !ok {
		g.setErr(fmt.Errorf("transpiler: missing stable field cache at %s", e.Pos()))
		return ""
	}
	return fmt.Sprintf("&g._gluField%d", id)
}
func (g *generator) lookup(name string) string {
	if goVar, ok := g.lookupLocal(name); ok {
		return goVar
	}
	if constant := g.numericConstants[name]; constant != nil {
		return fmt.Sprintf("rt.Number(%d)", constant.value)
	}
	if v, ok := g.globalGo[name]; ok {
		return "g." + v
	}
	return "rt.NilVal()"
}
func (g *generator) next(prefix string) string {
	g.seq++
	return fmt.Sprintf("glu_%s_%d", prefix, g.seq)
}
func (g *generator) lastName(prefix string) string { return fmt.Sprintf("glu_%s_%d", prefix, g.seq) }
func (g *generator) setErr(err error) {
	if g.err == nil {
		g.err = err
	}
}

type sourceBuilder struct {
	buf bytes.Buffer
}

func (b *sourceBuilder) line(s string)  { b.buf.WriteString(s); b.buf.WriteByte('\n') }
func (b *sourceBuilder) String() string { return b.buf.String() }

func goIdent(s string) string {
	if s == "" {
		return "glu_"
	}
	var b strings.Builder
	for i, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "glu_"
	}
	switch out {
	case "break", "default", "func", "interface", "select", "case", "defer", "go", "map", "struct", "chan", "else", "goto", "package", "switch", "const", "fallthrough", "if", "range", "type", "continue", "for", "import", "return", "var":
		out = "glu_" + out
	}
	return out
}
func isGoIdent(s string) bool { return goIdent(s) == s }

// capitalize upper-cases the first byte of an ASCII builtin name (e.g. "sin"
// -> "Sin") to match the corresponding exported rt.Number method name.
// strings.Title is deprecated (and Unicode-aware in ways this call site does
// not need) since every caller passes a fixed, lowercase ASCII identifier.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}

func byteLiteral(data []byte, n int) string {
	if len(data) == 0 {
		data = make([]byte, n)
	}
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		v := byte(0)
		if i < len(data) {
			v = data[i]
		}
		parts[i] = fmt.Sprintf("0x%02x", v)
	}
	return strings.Join(parts, ",")
}

func u16Literal(data []uint16, n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		v := uint16(0)
		if i < len(data) {
			v = data[i]
		}
		parts[i] = fmt.Sprintf("0x%04x", v)
	}
	return strings.Join(parts, ",")
}

func poseRangeLiteral(data []runtimeapi.PoseRange) string {
	parts := make([]string, len(data))
	for i, r := range data {
		parts[i] = fmt.Sprintf("{Start: %d, Count: %d, FlipOffsetX: %d, FlipOffsetY: %d}", r.Start, r.Count, r.FlipOffsetX, r.FlipOffsetY)
	}
	return strings.Join(parts, ",")
}

func posePartLiteral(data []runtimeapi.PosePart) string {
	parts := make([]string, len(data))
	for i, p := range data {
		parts[i] = fmt.Sprintf("{Tile: %d, X: %d, Y: %d, Flags: %d}", p.Tile, p.X, p.Y, p.Flags)
	}
	return strings.Join(parts, ",")
}

func u32Literal(data []uint32, n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		v := uint32(0)
		if i < len(data) {
			v = data[i]
		}
		parts[i] = fmt.Sprintf("0x%08x", v)
	}
	return strings.Join(parts, ",")
}
