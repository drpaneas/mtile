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
	// Palette is the expanded 256-entry BGR555 palette for native projects.
	// An omitted palette preserves the runtime's built-in palette behavior for
	// the earlier transpiler tests and small hand-written cartridges.
	Palette   []uint16
	SFX       []assetsync.TargetSFX
	SFXStep   []assetsync.TargetSFXStep
	Music     []assetsync.TargetMusicPattern
	Samples   []assetsync.TargetSample
	SamplePCM []int8
	// Audio settings are explicit only on native project builds. Tests and
	// hand-written Options values retain unity through the zero-value fallback.
	AudioMasterVolume    uint16
	AudioSettingsPresent bool
	// NativeAudio selects the strict PSG-only v2 contract. It is mutually
	// exclusive with every legacy SFX/music/sample field above.
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
// CLI/editor validation layer. MinArgs and MaxArgs describe the Lua call surface; omitted arguments
// are emitted as zero/nil Value defaults.
type BuiltinSpec struct {
	GoName  string
	MinArgs int
	MaxArgs int
}

// Builtins is the Lua-to-runtime mapping table. Functions that are iterator
// syntax (all/pairs) are included so callers can validate them consistently;
// their ordinary expression form is rejected by the generator. Renderer
// compatibility is an independent, exhaustive contract in
// BuiltinRendererCompatibility; adding a builtin requires classifying it there
// and documenting its Mode 0/Mode 4 semantics in docs/render-modes.md.
var Builtins = map[string]BuiltinSpec{
	"cls": {"ClsV", 0, 1}, "pset": {"PsetV", 2, 3}, "pget": {"PgetV", 2, 2},
	"line": {"LineV", 4, 5}, "rect": {"RectV", 4, 5}, "rectfill": {"RectfillV", 4, 5},
	"circ": {"CircV", 3, 4}, "circfill": {"CircfillV", 3, 4},
	"spr": {"SprV", 3, 7}, "sspr": {"SSprV", 6, 10}, "map": {"MapBGAtV", 0, 8},
	"camera": {"CameraV", 0, 2}, "clip": {"ClipV", 0, 4}, "pal": {"PalV", 0, 3},
	"palt": {"PaltV", 0, 2}, "color": {"ColorV", 0, 1}, "cursor": {"CursorV", 0, 3},
	"print": {"PrintV", 1, 4},
	"sget":  {"Sget", 2, 2}, "sset": {"Sset", 3, 3}, "fget": {"Fget", 1, 2},
	"fset": {"Fset", 2, 3}, "mget": {"Mget", 2, 2}, "mset": {"Mset", 3, 3},
	"btn": {"BtnV", 1, 2}, "btnp": {"BtnpV", 1, 2},
	"flr": {"Flr", 1, 1}, "ceil": {"Ceil", 1, 1}, "abs": {"Abs", 1, 1},
	"sgn": {"Sgn", 1, 1}, "min": {"Min", 2, 2}, "max": {"Max", 2, 2},
	"mid": {"Mid", 3, 3}, "sin": {"Sin", 1, 1}, "cos": {"Cos", 1, 1},
	"atan2": {"Atan2", 2, 2}, "sqrt": {"Sqrt", 1, 1}, "rnd": {"RndV", 0, 1},
	"srand": {"SrandV", 1, 1}, "band": {"Band", 2, 2}, "bor": {"Bor", 2, 2},
	"bxor": {"Bxor", 2, 2}, "bnot": {"Bnot", 1, 1}, "shl": {"Shl", 2, 2},
	"shr": {"Shr", 2, 2}, "lshr": {"Lshr", 2, 2}, "rotl": {"Rotl", 2, 2},
	"rotr": {"Rotr", 2, 2},
	"add":  {"TableAdd", 2, 3}, "del": {"TableDel", 2, 2}, "deli": {"TableDeli", 1, 2},
	"count": {"TableCount", 1, 2}, "all": {"all", 1, 1}, "pairs": {"pairs", 1, 1}, "foreach": {"foreach", 2, 2},
	"sub": {"Sub", 2, 3}, "tostr": {"Tostr", 1, 2}, "tonum": {"Tonum", 1, 1},
	"chr": {"Chr", 1, 1}, "ord": {"Ord", 1, 2},
	"t": {"T", 0, 0}, "time": {"Time", 0, 0}, "stat": {"StatV", 1, 1}, "sfx": {"SFXV", 1, 4},
	"music": {"MusicV", 0, 2}, "sample": {"SampleV", 1, 3},
	"gba_note": {"GBANoteV", 3, 4}, "gba_stop": {"GBAStopV", 1, 1}, "printh": {"noop", 1, 2},
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
	generated, err := Generate(chunk, opts)
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
	g.discoverGlobals()
	report := g.globalReport()
	if report.Count > report.Limit {
		return report, &GlobalLimitError{Report: report}
	}
	return report, nil
}

// Generate emits a complete package main from a parsed chunk.
func Generate(chunk *luaparser.Chunk, opts Options) (string, error) {
	if chunk == nil {
		return "", errors.New("transpiler: nil chunk")
	}
	if opts.MapLayers == 0 {
		opts.MapLayers = 1
	}
	if opts.MapLayers < 1 || opts.MapLayers > assetsync.MapLayerCount {
		return "", fmt.Errorf("transpiler: map layer count %d is outside 1..%d", opts.MapLayers, assetsync.MapLayerCount)
	}
	// Do not let direct AST callers bypass the forced Mode 0 proof. Mode 4 is
	// the zero/default and needs no eligibility scan; every other preference is
	// normalized through the same conservative selector used by Build.
	if opts.Renderer != RendererMode4 {
		report, err := SelectRenderer(chunk, opts)
		if err != nil {
			return "", err
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
			return "", err
		}
		opts.MapPlaneMask = plan.storageMask
		opts.MapDrawMask = plan.drawMask
	}
	g := newGenerator(chunk, opts)
	if err := g.prepare(); err != nil {
		return "", err
	}
	source := g.render()
	if g.err != nil {
		return "", g.err
	}
	return source, nil
}

func newGenerator(chunk *luaparser.Chunk, opts Options) *generator {
	return &generator{
		chunk:         chunk,
		opts:          opts,
		funcs:         make(map[string]*functionInfo),
		globalGo:      make(map[string]string),
		globalKinds:   make(map[string]monoKind),
		globalOrigins: make(map[string]luaparser.Position),
		staticGo:      make(map[string]string),
		fieldCaches:   make(map[*luaparser.FieldExpr]int),
		numericFields: make(map[string]bool),
		numericArrays: make(map[string]*numericArrayInfo),
	}
}

type functionInfo struct {
	decl        *luaparser.FunctionDeclStmt
	key         string
	goName      string
	returnN     int
	returnKinds []monoKind
	params      []string
	paramKinds  []monoKind
	noAlloc     bool
}

type numericArrayInfo struct {
	declaration *luaparser.LocalAssignStmt
	literal     *luaparser.TableExpr
	length      int
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

	fieldCaches   map[*luaparser.FieldExpr]int
	fieldCacheN   int
	numericFields map[string]bool
	numericArrays map[string]*numericArrayInfo

	current            *functionInfo
	seq                int
	localSeq           int
	scopes             []*localScope
	err                error
	extraRoots         []string
	pending            []string
	spriteN            int
	sheetMutable       bool
	sheetCompact       []byte
	sheetIndex         []uint16
	mapCompact         []uint16
	audioEnabled       bool
	legacyAudioEnabled bool
	nativeAudioEnabled bool
	nativeAudioDecl    string
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

// bindParam records a Lua parameter name as an alias for the Go function
// parameter directly (a0, a1, ...). Go parameters are already mutable
// locals, so no extra copy is needed even though Lua allows reassigning a
// parameter like any other local.
func (g *generator) bindParam(name, goVar string) {
	g.bindParamKind(name, goVar, monoValue)
}

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
	if err := validateTargetSFX(g.opts.SFX, g.opts.SFXStep); err != nil {
		return err
	}
	if err := validateTargetMusic(g.opts.Music, g.opts.SFX); err != nil {
		return err
	}
	if err := validateTargetSamples(g.opts.Samples, g.opts.SamplePCM); err != nil {
		return err
	}
	g.legacyAudioEnabled = len(g.opts.SFX) != 0 || len(g.opts.Music) != 0 || len(g.opts.Samples) != 0
	if g.opts.NativeAudio != nil && g.legacyAudioEnabled {
		return errors.New("transpiler: audio v2 cannot be combined with legacy SFX/music/sample records")
	}
	if g.opts.NativeAudio != nil {
		library, err := assetsync.CompileNativeAudio(g.opts.NativeAudio.Settings, g.opts.NativeAudio.Sounds, g.opts.NativeAudio.Music)
		if err != nil {
			return fmt.Errorf("transpiler: native audio: %w", err)
		}
		g.opts.NativeAudio.Library = library
		nativeCall := chunkCallsBuiltin(g.chunk, "sfx") || chunkCallsBuiltin(g.chunk, "music") ||
			chunkCallsBuiltin(g.chunk, "gba_note") || chunkCallsBuiltin(g.chunk, "gba_stop")
		g.nativeAudioEnabled = nativeCall || len(library.Sounds) != 0 || len(library.Clips) != 0 || len(library.Parts) != 0 || len(library.Songs) != 0
		if g.nativeAudioEnabled {
			g.nativeAudioDecl, err = assetsync.NativeAudioGoDeclarations(library)
			if err != nil {
				return fmt.Errorf("transpiler: native audio declarations: %w", err)
			}
		}
	}
	g.audioEnabled = g.legacyAudioEnabled || g.nativeAudioEnabled
	if err := g.validateStaticAudioCalls(); err != nil {
		return err
	}
	g.opts.MapStreaming = len(g.mapCompact) > assetsync.MapStreamingThresholdCells
	if err := g.discoverFunctions(); err != nil {
		return err
	}
	g.resolveReturnArities()
	for _, info := range g.funcList {
		info.returnKinds = make([]monoKind, info.returnN)
	}
	g.sheetMutable = chunkCallsBuiltin(g.chunk, "sset")
	if !g.sheetMutable {
		g.compactSheet()
	}
	for _, info := range g.funcList {
		if (info.key == "_init" || info.key == "_update" || info.key == "_draw") && len(info.params) != 0 {
			return fmt.Errorf("transpiler: %s callback must not have parameters", info.key)
		}
	}
	g.discoverGlobals()
	g.analyzeNumericArrays()
	g.analyzeMonomorphicNumbers()
	g.analyzeMonomorphicFunctions()
	g.analyzeNoAllocFunctions()
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
	obviouslyNonNumeric := func(expr luaparser.Expr) bool {
		switch x := expr.(type) {
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

func (g *generator) analyzeMonomorphicNumbers() {
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
				if i < len(s.Values) && numericIntent(s.Values[i]) {
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
				if i < len(s.Values) && numericIntent(s.Values[i]) {
					g.globalKinds[v.Name] = monoNumber
				}
			}
		}
	}

	// Record fields retain Value storage, but a field name whose observed table
	// literal initializers are all numeric can use native Number reads/writes.
	seenField := make(map[string]bool)
	observe := func(expr luaparser.Expr) {
		table, ok := expr.(*luaparser.TableExpr)
		if !ok {
			return
		}
		for _, field := range table.Fields {
			if field.Array {
				continue
			}
			key, ok := field.Key.(*luaparser.StringExpr)
			if !ok {
				continue
			}
			isNumber := numericIntent(field.Value)
			if !seenField[key.Value] {
				seenField[key.Value] = true
				g.numericFields[key.Value] = isNumber
			} else if !isNumber {
				g.numericFields[key.Value] = false
			}
		}
	}
	for _, stmt := range g.chunk.Body {
		if fn, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			for _, bodyStmt := range fn.Body.Body {
				collectStmtExprs(bodyStmt, observe)
			}
			continue
		}
		collectStmtExprs(stmt, observe)
	}
}

func numericIntent(expr luaparser.Expr) bool {
	switch x := expr.(type) {
	case *luaparser.NumberExpr:
		return true
	case *luaparser.UnaryExpr:
		return x.Op == luaparser.OpNegate || x.Op == luaparser.OpBitNot
	case *luaparser.BinaryExpr:
		switch x.Op {
		case luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
			luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo,
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
			"band", "bor", "bxor", "bnot", "shl", "shr", "lshr", "rotl", "rotr", "t", "time", "stat":
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
	g      *generator
	scopes []map[string]monoKind
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
				if fn.paramKinds[i] == monoValue && observation.seen && observation.allNumber {
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
		for _, value := range s.Values {
			g.analyzeMonomorphicExpr(value, env, params)
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
		if initKind == monoNumber && limitKind == monoNumber && stepKind == monoNumber {
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
// retaining the inferred kind of each expanded result. Only a trailing known
// user call expands; missing positions are nil and therefore remain Value.
func (g *generator) analyzeMonomorphicValues(values []luaparser.Expr, nWanted int, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation) []monoKind {
	out := make([]monoKind, nWanted)
	slot := 0
	last := len(values) - 1
	for i, value := range values {
		kind := g.analyzeMonomorphicExpr(value, env, params)
		if i == last {
			if call, ok := value.(*luaparser.CallExpr); ok {
				if fn := g.calledFunction(call); fn != nil {
					if fn.returnN == 0 {
						continue
					}
					if fn.returnN > 1 {
						for j := 0; j < fn.returnN && slot < nWanted; j++ {
							out[slot] = fn.returnKind(j)
							slot++
						}
						continue
					}
				}
			}
		}
		if slot < nWanted {
			out[slot] = kind
			slot++
		}
	}
	return out
}

func (g *generator) analyzeMonomorphicExpr(expr luaparser.Expr, env *monomorphicAnalysisEnv, params map[*functionInfo][]functionParamObservation) monoKind {
	switch x := expr.(type) {
	case *luaparser.NumberExpr:
		return monoNumber
	case *luaparser.VarExpr:
		return env.lookup(x.Name)
	case *luaparser.FieldExpr:
		g.analyzeMonomorphicExpr(x.Base, env, params)
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
		if kind == monoNumber && (x.Op == luaparser.OpNegate || x.Op == luaparser.OpBitNot) {
			return monoNumber
		}
	case *luaparser.BinaryExpr:
		left := g.analyzeMonomorphicExpr(x.Left, env, params)
		right := g.analyzeMonomorphicExpr(x.Right, env, params)
		if left == monoNumber && right == monoNumber {
			switch x.Op {
			case luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
				luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo,
				luaparser.OpBitOr, luaparser.OpBitXor, luaparser.OpBitAnd,
				luaparser.OpShiftL, luaparser.OpShiftR, luaparser.OpShiftRLogical,
				luaparser.OpRotateL, luaparser.OpRotateR:
				return monoNumber
			}
		}
	case *luaparser.CallExpr:
		expressions := operandExprs(x)
		kinds := make([]monoKind, len(expressions))
		for i, argument := range expressions {
			kinds[i] = g.analyzeMonomorphicExpr(argument, env, params)
		}
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
			switch target.(type) {
			case *luaparser.VarExpr:
			case *luaparser.FieldExpr:
				// A first field write may insert/grow a table even when later
				// executions hit a stable cached slot.
				return false
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
		return ok && builtinNoAlloc(name)
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
		"sget", "sset", "fget", "fset", "mget", "mset",
		"cls", "pset", "pget", "line", "rect", "rectfill", "circ", "circfill", "spr", "sspr", "map",
		"camera", "clip", "pal", "palt", "color", "cursor":
		return true
	}
	return false
}

// validateStaticAudioCalls catches only IDs that are unambiguously numeric
// literals in a project that links audio assets. Dynamic expressions retain
// the runtime's bounded fail-closed behavior, and audio-less ports may keep
// compatibility calls without pulling in the audio runtime.
func (g *generator) validateStaticAudioCalls() error {
	if !g.audioEnabled && g.opts.NativeAudio == nil &&
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
			if g.opts.NativeAudio != nil {
				switch name {
				case "sample":
					firstErr = fmt.Errorf("transpiler: %s: sample() is not part of four-channel audio v2", call.Pos())
					return
				case "sfx":
					if len(call.Args) < 1 || len(call.Args) > 2 {
						firstErr = fmt.Errorf("transpiler: %s: audio v2 sfx() takes id and optional priority", call.Pos())
						return
					}
				case "music":
					if len(call.Args) != 1 {
						firstErr = fmt.Errorf("transpiler: %s: audio v2 music() takes exactly one id", call.Pos())
						return
					}
				case "gba_note":
					if len(call.Args) < 3 || len(call.Args) > 4 {
						firstErr = fmt.Errorf("transpiler: %s: gba_note() takes channel, note, sound id, and optional duration", call.Pos())
						return
					}
					g.validateStaticNativeNote(call, &firstErr)
					return
				case "gba_stop":
					if len(call.Args) != 1 {
						firstErr = fmt.Errorf("transpiler: %s: gba_stop() takes one channel", call.Pos())
						return
					}
					if channel, literal := staticNumberLiteral(call.Args[0]); literal && (channel != math.Trunc(channel) || channel < 1 || channel > 4) {
						firstErr = fmt.Errorf("transpiler: %s: GBA channel must be an integer from 1 through 4", call.Pos())
					}
					return
				}
			} else if name == "gba_note" || name == "gba_stop" {
				firstErr = fmt.Errorf("transpiler: %s: %s() requires four-channel audio v2 assets", call.Pos(), name)
				return
			}
			if (name != "sfx" && name != "music" && name != "sample") || len(call.Args) == 0 {
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
	case *luaparser.NumberExpr:
		return value.Value, true
	case *luaparser.UnaryExpr:
		if value.Op != luaparser.OpNegate {
			return 0, false
		}
		if number, ok := value.Expr.(*luaparser.NumberExpr); ok {
			return -number.Value, true
		}
	}
	return 0, false
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
	switch name {
	case "sfx":
		for _, definition := range g.opts.SFX {
			if int(definition.ID) == id {
				return true
			}
		}
	case "music":
		for _, pattern := range g.opts.Music {
			if int(pattern.ID) == id {
				return true
			}
		}
	case "sample":
		for _, sample := range g.opts.Samples {
			if int(sample.ID) == id {
				return true
			}
		}
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
	switch name {
	case "sfx":
		return "assets/audio/sfx.json"
	case "music":
		return "assets/audio/music.json"
	default:
		return "assets/audio/samples/manifest.json"
	}
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

func (g *generator) compactSheet() {
	g.sheetIndex = make([]uint16, g.spriteN)
	if len(g.opts.Sheet) == 0 {
		return
	}
	cols := g.opts.SheetW / 8
	for id := 0; id < g.spriteN; id++ {
		cellX, cellY := id%cols, id/cols
		nonzero := false
		for row := 0; row < 8 && !nonzero; row++ {
			start := (cellY*8+row)*g.opts.SheetW + cellX*8
			for _, pixel := range g.opts.Sheet[start : start+8] {
				if pixel != 0 {
					nonzero = true
					break
				}
			}
		}
		if !nonzero {
			continue
		}
		slot := len(g.sheetCompact) / 64
		g.sheetIndex[id] = uint16(slot + 1)
		for row := 0; row < 8; row++ {
			start := (cellY*8+row)*g.opts.SheetW + cellX*8
			g.sheetCompact = append(g.sheetCompact, g.opts.Sheet[start:start+8]...)
		}
	}
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

func (g *generator) discoverGlobals() {
	for _, stmt := range g.chunk.Body {
		if _, ok := stmt.(*luaparser.FunctionDeclStmt); ok {
			continue
		}
		g.collectTopLevel(stmt)
	}
	for _, info := range g.funcList {
		g.current = info
		g.collectBlockGlobals(info.decl.Body)
	}
	g.current = nil
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
func (g *generator) resolveReturnArities() {
	for {
		changed := false
		for _, info := range g.funcList {
			n := g.adjustedReturnN(info.decl.Body)
			if n != info.returnN {
				info.returnN = n
				changed = true
			}
		}
		if !changed {
			return
		}
	}
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
	fn := g.calledFunction(call)
	if fn == nil {
		return n
	}
	if fn.returnN == 0 {
		return n - 1
	}
	return n - 1 + fn.returnN
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

func (g *generator) collectTopLevel(stmt luaparser.Stmt) {
	if local, ok := stmt.(*luaparser.LocalAssignStmt); ok {
		for _, name := range local.Names {
			g.addGlobal(name, local.Pos())
		}
	}
	collectStmt(stmt, g.addGlobal, func(_ string) bool { return false }, g.funcs, Builtins)
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
	collectStmtExprs(stmt, func(e luaparser.Expr) { g.collectExprStatic(e) })
}

func (g *generator) collectBlockStatics(block *luaparser.Block) {
	for _, stmt := range block.Body {
		g.collectStmtStatics(stmt)
	}
}

func (g *generator) collectExprStatic(e luaparser.Expr) {
	switch x := e.(type) {
	case *luaparser.StringExpr:
		g.staticValue(x.Value)
	case *luaparser.FieldExpr:
		if !g.opts.DisableStableTableFields {
			if _, exists := g.fieldCaches[x]; !exists {
				g.fieldCaches[x] = g.fieldCacheN
				g.fieldCacheN++
			}
		}
		g.staticValue(x.Name)
		g.collectExprStatic(x.Base)
	case *luaparser.IndexExpr:
		g.collectExprStatic(x.Base)
		g.collectExprStatic(x.Key)
	case *luaparser.UnaryExpr:
		g.collectExprStatic(x.Expr)
	case *luaparser.BinaryExpr:
		g.collectExprStatic(x.Left)
		g.collectExprStatic(x.Right)
	case *luaparser.CallExpr:
		g.collectExprStatic(x.Callee)
		for _, a := range x.Args {
			g.collectExprStatic(a)
		}
	case *luaparser.TableExpr:
		for _, f := range x.Fields {
			if f.Key != nil {
				g.collectExprStatic(f.Key)
			}
			g.collectExprStatic(f.Value)
		}
	}
}

func (g *generator) staticValue(s string) string {
	if name, ok := g.staticGo[s]; ok {
		return name
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
	collectStmt(stmt, func(string, luaparser.Position) {}, func(string) bool { return true }, nil, nil)
	var expr func(luaparser.Expr)
	expr = func(e luaparser.Expr) {
		if e == nil {
			return
		}
		add(e)
		switch x := e.(type) {
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

func (g *generator) render() string {
	var b sourceBuilder
	b.line("package " + g.opts.PackageName)
	b.line("")
	b.line("import (")
	b.line("\trt \"github.com/drpaneas/mtile/runtime\"")
	if g.audioEnabled {
		b.line("\t\"github.com/drpaneas/mtile/runtime/audio\"")
		b.line("\t\"github.com/drpaneas/mtile/runtime/hw\"")
	}
	b.line("\t_ \"unsafe\"")
	b.line(")")
	b.line("")
	g.renderHelpers(&b)
	b.line("")
	g.renderGame(&b)
	b.line("")
	g.renderFunctions(&b)
	b.line("")
	g.renderMain(&b)
	return b.String()
}

func (g *generator) renderHelpers(b *sourceBuilder) {
	b.line("func gluPushRoots(c *rt.Context, roots ...rt.Value) int {")
	b.line("\tn := 0")
	b.line("\tfor _, root := range roots {")
	b.line("\t\tif !c.PushLocal(root) { c.PopLocals(n); return -1 }")
	b.line("\t\tn++")
	b.line("\t}")
	b.line("\treturn n")
	b.line("}")
	b.line("func gluPopRoots(c *rt.Context, n int) { if n > 0 { c.PopLocals(n) } }")
	b.line("func gluMust(v rt.Value, err rt.HeapError) rt.Value { if err != rt.HeapOK { return rt.NilVal() }; return v }")
	b.line("func gluCmp(a, z rt.Value, op string) bool {")
	b.line("\tif a.Kind != rt.KindNumber || z.Kind != rt.KindNumber { return false }")
	b.line("\tswitch op { case \"<\": return a.Num < z.Num; case \"<=\": return a.Num <= z.Num; case \">\": return a.Num > z.Num; case \">=\": return a.Num >= z.Num }; return false")
	b.line("}")
	b.line("func gluLen(c *rt.Context, v rt.Value) rt.Value { if v.Kind == rt.KindString || v.Kind == rt.KindStaticString { return rt.IntVal(c.StrLen(v)) }; if v.Kind == rt.KindTable { return rt.IntVal(c.TableLen(v)) }; return rt.IntVal(0) }")
	b.line("func gluMod(a, z rt.Value) rt.Value { if a.Kind != rt.KindNumber || z.Kind != rt.KindNumber || z.Num == 0 { return rt.IntVal(0) }; return rt.NumVal(a.Num % z.Num) }")
	b.line("func gluNumberMod(a, z rt.Number) rt.Number { if z == 0 { return 0 }; return a % z }")
	b.line("func gluPow(a, z rt.Value) rt.Value { if a.Kind != rt.KindNumber || z.Kind != rt.KindNumber { return rt.NilVal() }; n := z.Int(); if n == 0 { return rt.IntVal(1) }; neg := n < 0; if neg { n = -n }; out := rt.IntVal(1); for n > 0 { if n&1 != 0 { out = out.Mul(a) }; a = a.Mul(a); n >>= 1 }; if neg { out = rt.IntVal(1).Div(out) }; return out }")
	b.line("func gluConcat(c *rt.Context, a, z rt.Value, roots ...rt.Value) rt.Value { n := gluPushRoots(c, a, z); if n < 0 { return rt.NilVal() }; m := gluPushRoots(c, roots...); if m < 0 { gluPopRoots(c,n); return rt.NilVal() }; v, err := c.Concat(a,z); gluPopRoots(c,n+m); return gluMust(v,err) }")
	b.line("func gluSub(c *rt.Context, s, i, j rt.Value, roots ...rt.Value) rt.Value { n := gluPushRoots(c,s,i,j); if n < 0 { return rt.NilVal() }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,n); return rt.NilVal() }; v,err := c.Sub(s,i,j); gluPopRoots(c,n+m); return gluMust(v,err) }")
	b.line("func gluChr(c *rt.Context, n rt.Value, roots ...rt.Value) rt.Value { k := gluPushRoots(c,n); if k < 0 { return rt.NilVal() }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,k); return rt.NilVal() }; v,err := c.Chr(n); gluPopRoots(c,k+m); return gluMust(v,err) }")
	b.line("func gluTostr(c *rt.Context, v rt.Value, hex bool, roots ...rt.Value) rt.Value { k := gluPushRoots(c,v); if k < 0 { return rt.NilVal() }; m := gluPushRoots(c,roots...); if m < 0 { gluPopRoots(c,k); return rt.NilVal() }; out,err := c.Tostr(v,hex); gluPopRoots(c,k+m); return gluMust(out,err) }")
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
	}
	b.line("//go:section .rodata")
	mapCells := len(g.mapCompact)
	b.line(fmt.Sprintf("var mapROM = [ %d ]uint16{%s}", mapCells, u16Literal(g.mapCompact, mapCells)))
	b.line("//go:section .rodata")
	b.line(fmt.Sprintf("var flagROM = [ %d ]byte{%s}", g.spriteN, byteLiteral(g.opts.Flags, g.spriteN)))
	if len(g.opts.Palette) == 256 {
		b.line("//go:section .rodata")
		b.line(fmt.Sprintf("var paletteROM = [256]uint16{%s}", u16Literal(g.opts.Palette, 256)))
	}
	if g.legacyAudioEnabled {
		b.line("//go:section .rodata")
		b.line(fmt.Sprintf("var sfxROM = [%d]audio.SFXDefinition{%s}", len(g.opts.SFX), sfxDefinitionLiteral(g.opts.SFX)))
		b.line("//go:section .rodata")
		b.line(fmt.Sprintf("var sfxStepROM = [%d]audio.SFXStep{%s}", len(g.opts.SFXStep), sfxStepLiteral(g.opts.SFXStep)))
		if len(g.opts.Music) != 0 {
			b.line("//go:section .rodata")
			b.line(fmt.Sprintf("var musicROM = [%d]audio.MusicPattern{%s}", len(g.opts.Music), musicPatternLiteral(g.opts.Music)))
		}
		if len(g.opts.Samples) != 0 {
			b.line("//go:section .rodata")
			b.line(fmt.Sprintf("var samplePCMROM = [%d]int8{%s}", len(g.opts.SamplePCM), int8Literal(g.opts.SamplePCM)))
			b.line("//go:section .rodata")
			b.line(fmt.Sprintf("var sampleROM = [%d]audio.SampleDefinition{%s}", len(g.opts.Samples), sampleDefinitionLiteral(g.opts.Samples)))
		}
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
	b.line("//go:section .ewram\nvar cartData rt.AssetMemory")
	b.line("//go:section .ewram\nvar ctx rt.Context")
	b.line("//go:section .ewram\nvar game Game")
	if g.opts.Debug {
		b.line("//go:section .debug_mailbox\nvar debugMailbox rt.DebugMailbox")
	}
	if g.legacyAudioEnabled {
		b.line("//go:section .ewram\nvar sfxStream audio.MonoStream")
		b.line("//go:section .ewram\nvar sfxPlayer audio.SFXPlayer")
		b.line("//go:section .iwram\nvar sfxMixer audio.Mixer")
	}
	if g.nativeAudioEnabled {
		b.line("//go:section .ewram\nvar nativeAudioController audio.Controller")
	}
	b.line("")
	b.line("type Game struct {")
	for _, name := range g.globals {
		if g.globalKinds[name] == monoNumber {
			b.line("\t" + g.globalGo[name] + " rt.Number")
		} else {
			b.line("\t" + g.globalGo[name] + " rt.Value")
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
		if g.globalKinds[name] == monoValue {
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

func (g *generator) renderMain(b *sourceBuilder) {
	b.line("func main() {")
	b.line("\tcartImage := rt.AssetImage{")
	b.line("\t\tSheetW: SheetW, SheetH: SheetH, SpriteCols: SpriteCols, SpriteRows: SpriteRows, SpriteN: SpriteN,")
	b.line("\t\tMapW: MapW, MapH: MapH, MapLayers: MapLayers, MapPlaneMask: MapPlaneMask, MapStreaming: MapStreaming, SheetReadOnly: SheetReadOnly, Sheet: sheetROM[:], Map: mapROM[:], Flags: flagROM[:],")
	if !g.sheetMutable {
		b.line("\t\tSheetIndex: sheetIndexROM[:],")
	}
	b.line("\t}")
	b.line("\tcartData.SheetW, cartData.SheetH = SheetW, SheetH")
	b.line("\tcartData.SpriteCols, cartData.SpriteRows, cartData.SpriteN = SpriteCols, SpriteRows, SpriteN")
	b.line("\tcartData.MapW, cartData.MapH, cartData.MapLayers, cartData.MapPlaneMask = MapW, MapH, MapLayers, MapPlaneMask")
	b.line("\tcartData.MapStreaming, cartData.SheetReadOnly = MapStreaming, SheetReadOnly")
	b.line("\tcartData.Sheet = sheetRAM[:]")
	if !g.sheetMutable {
		b.line("\tcartData.SheetIndex = sheetIndexRAM[:]")
	}
	b.line("\tcartData.Map, cartData.Flags = mapRAM[:], flagRAM[:]")
	if g.opts.MapStreaming {
		b.line("\tcartData.MapChunks, cartData.MapEdits = mapChunksRAM[:], mapEditsRAM[:]")
	}
	b.line("\tif !ctx.Init(&cartImage, &cartData) { for {} }")
	if len(g.opts.Palette) == 256 {
		b.line("\tctx.LoadPalette(&paletteROM)")
	}
	b.line("\tctx.InitHardware()")
	b.line("\tctx.SetTickRate(30)")
	if g.opts.Debug {
		b.line("\tctx.SetProfiling(true)")
	}
	if g.legacyAudioEnabled {
		voiceCount := audio.SFXMixerVoices
		if len(g.opts.Samples) != 0 {
			voiceCount = audio.SFXSampleMixerVoices
		}
		b.line(fmt.Sprintf("\tif !sfxMixer.Init(%d) || !sfxPlayer.Init(&sfxMixer, sfxROM[:], sfxStepROM[:]) { for {} }", voiceCount))
		masterVolume := uint16(assetsync.AudioMasterVolumeMax)
		if g.opts.AudioSettingsPresent {
			masterVolume = g.opts.AudioMasterVolume
		}
		b.line(fmt.Sprintf("\tif !sfxPlayer.SetMasterVolume(%d) { for {} }", masterVolume))
		if len(g.opts.Music) != 0 {
			b.line("\tif !sfxPlayer.InitMusic(musicROM[:]) { for {} }")
		}
		if len(g.opts.Samples) != 0 {
			b.line("\tif !sfxPlayer.InitSamples(sampleROM[:], samplePCMROM[:]) { for {} }")
		}
		b.line("\tctx.AttachSFX(&sfxPlayer)")
		b.line("\tctx.AttachAudioStream(&sfxStream)")
	}
	if g.nativeAudioEnabled {
		settings := g.opts.NativeAudio.Settings
		b.line("\tif !nativeAudioController.Init(audio.ProfilePSG) { for {} }")
		b.line(fmt.Sprintf("\tif !nativeAudioController.ConfigureOutput(%d, %d, %s) { for {} }", settings.LeftLevel, settings.RightLevel, transpilerPSGRatioGoName(settings.PSGRatio)))
		b.line("\tif !nativeAudioController.Bind(audio.Library{Sounds: nativeSoundROM[:], Clips: nativeClipROM[:], Parts: nativePartROM[:], Songs: nativeSongROM[:]}) { for {} }")
		b.line("\tctx.AttachNativeAudio(&nativeAudioController)")
	}
	b.line("\tgame.Init(&ctx)")
	if g.opts.Renderer == RendererMode0 {
		b.line("\t// _init runs while Mode 0 is force-blanked; commit its")
		b.line("\t// spritesheet mutations before the first visible frame.")
		b.line("\tctx.FinalizeInitHardware()")
	}
	if g.legacyAudioEnabled {
		b.line("\tif !sfxStream.StartSFX(&sfxPlayer, hw.DirectSoundRate16384) { for {} }")
	}
	if g.nativeAudioEnabled {
		b.line("\tif !nativeAudioController.Start() { for {} }")
	}
	b.line("\tfor {")
	b.line("\t\t// One iteration = exactly two display frames (30 Hz), the same")
	b.line("\t\t// cadence PICO-8 uses natively. The LCD always scans at 59.73 Hz;")
	b.line("\t\t// each rendered image is shown for two whole scans. PaceFrame30")
	b.line("\t\t// burns whatever time update/draw left over in the first frame, so")
	b.line("\t\t// the flip always lands in the second frame's VBlank - a fast")
	b.line("\t\t// iteration can never flip one frame early and alternate cadence.")
	b.line("\t\tctx.BeginFramePace()")
	if g.legacyAudioEnabled {
		b.line("\t\tsfxStream.Service(audio.StreamPageCount)")
	}
	b.line("\t\tctx.BeginProfilePhase(rt.ProfileUpdate)")
	b.line("\t\tctx.PollInput()")
	b.line("\t\tctx.AdvanceTick()")
	b.line("\t\tgame.Update(&ctx)")
	b.line("\t\tctx.EndProfilePhase(rt.ProfileUpdate)")
	if g.legacyAudioEnabled {
		b.line("\t\tsfxStream.Service(audio.StreamPageCount)")
	}
	b.line("\t\tctx.BeginProfilePhase(rt.ProfileDraw)")
	b.line("\t\tctx.BeginRenderFrame()")
	b.line("\t\tgame.Draw(&ctx)")
	b.line("\t\tctx.EndProfilePhase(rt.ProfileDraw)")
	if g.legacyAudioEnabled {
		b.line("\t\tsfxStream.Service(audio.StreamPageCount)")
	}
	b.line("\t\t// Copy to the hidden page immediately - it is never on screen, so")
	b.line("\t\t// this cannot tear, and update+draw+copy own the full two-frame")
	b.line("\t\t// window (~562k cycles) minus only the flip itself.")
	b.line("\t\tctx.PrepareRender()")
	if g.legacyAudioEnabled {
		b.line("\t\tsfxStream.Service(audio.StreamPageCount)")
	}
	b.line("\t\tctx.CollectIfNeeded()")
	b.line("\t\tctx.PaceFrame30()")
	b.line("\t\tctx.WaitForPresentWindow()")
	b.line("\t\tctx.PresentRender()")
	if g.opts.Debug {
		b.line("\t\tdebugMailbox.Begin(ctx.PerformanceStats())")
		for index, name := range g.globals {
			if index >= runtimeapi.DebugWatchSlots {
				break
			}
			value := "game." + g.globalGo[name]
			if g.globalKinds[name] == monoNumber {
				value = "rt.NumVal(" + value + ")"
			}
			b.line("\t\tdebugMailbox.Add(" + strconv.Quote(name) + ", " + value + ")")
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
		cond := g.emitExprFlush(b, indent+1, s.Condition)
		b.line(pad + "\tif !(" + cond + ").Truthy() { break }")
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
		cond := g.emitExprFlush(b, indent+1, s.Condition)
		g.popScope()
		b.line(pad + "\tif (" + cond + ").Truthy() { break }")
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
		b.line(pad + initRoots + " := gluPushRoots(c, " + id + "Init)")
		b.line(pad + "if " + initRoots + " >= 0 {")
		oldRoots := g.extraRoots
		g.extraRoots = append(append([]string(nil), oldRoots...), id+"Init")
		limitExpr := g.emitExprFlush(b, indent+1, s.Limit)
		g.extraRoots = append(append([]string(nil), oldRoots...), id+"Init", id+"Limit")
		stepExpr := g.emitExprFlush(b, indent+1, s.Step)
		g.extraRoots = oldRoots
		b.line(pad + "\t" + id + "Limit := " + limitExpr)
		limitRoots := id + "LimitRoots"
		b.line(pad + "\t" + limitRoots + " := gluPushRoots(c, " + id + "Limit)")
		b.line(pad + "\tif " + limitRoots + " >= 0 {")
		b.line(pad + "\t\t" + id + "Step := " + stepExpr)
		stepRoots := id + "StepRoots"
		b.line(pad + "\t\t" + stepRoots + " := gluPushRoots(c, " + id + "Step)")
		b.line(pad + "\t\tif " + stepRoots + " >= 0 {")
		g.pushScope()
		loopVar := g.declareLocal(s.Name)
		b.line(pad + "\t\t\tvar " + loopVar + " rt.Value")
		b.line(pad + "\t\t\t_ = " + loopVar)
		b.line(pad + "\t\t\tfor " + id + "Init.Kind == rt.KindNumber && " + id + "Limit.Kind == rt.KindNumber && ((" + id + "Step.Num >= 0 && " + id + "Init.Num <= " + id + "Limit.Num) || (" + id + "Step.Num < 0 && " + id + "Init.Num >= " + id + "Limit.Num)) {")
		b.line(pad + "\t\t\t\t" + loopVar + " = " + id + "Init")
		g.pushScope()
		for _, q := range s.Body.Body {
			g.emitStmt(b, q, indent+4)
		}
		g.popScope()
		b.line(pad + "\t\t\t\t" + id + "Init = " + id + "Init.Add(" + id + "Step)")
		b.line(pad + "\t\t\t}")
		g.popScope()
		b.line(pad + "\t\t\tgluPopRoots(c, " + stepRoots + ")")
		b.line(pad + "\t\t}")
		b.line(pad + "\t\tgluPopRoots(c, " + limitRoots + ")")
		b.line(pad + "\t}")
		b.line(pad + "\tgluPopRoots(c, " + initRoots + ")")
		b.line(pad + "}")
	case *luaparser.GenericForStmt:
		g.emitGenericFor(b, s, indent)
	case *luaparser.ReturnStmt:
		g.emitReturn(b, s, indent)
	case *luaparser.BreakStmt:
		b.line(pad + "break")
	}
}

func (g *generator) emitMonomorphicNumericFor(b *sourceBuilder, s *luaparser.NumericForStmt, indent int) bool {
	if !g.exprNumber(s.Init) || !g.exprNumber(s.Limit) || !g.exprNumber(s.Step) {
		return false
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
	b.line(pad + "\t" + id + "Init += " + id + "Step")
	b.line(pad + "}")
	g.popScope()
	return true
}

func (g *generator) emitIfChain(b *sourceBuilder, indent int, cond luaparser.Expr, thenBody []luaparser.Stmt, elseIfs []*luaparser.IfBranch, elseBody []luaparser.Stmt) {
	pad := strings.Repeat("\t", indent)
	condExpr := g.emitExprFlush(b, indent, cond)
	b.line(pad + "if (" + condExpr + ").Truthy() {")
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
	}
	const (
		lhsVar = iota
		lhsIndex
		lhsField
		lhsNumberArray
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
				slots[i] = lhs{kind: lhsNumberArray, mono: monoNumber, base: g.lookup(name), key: key}
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
	for i, slot := range slots {
		switch slot.kind {
		case lhsVar:
			if slot.mono == monoNumber {
				b.line(pad + slot.dest + " = " + adjustedNumber(values[i]))
			} else {
				b.line(pad + slot.dest + " = " + adjustedLuaValue(values[i]))
			}
		case lhsIndex:
			b.line(pad + "_ = c.TableSet(" + slot.base + ", " + slot.key + ", " + adjustedLuaValue(values[i]) + ")")
		case lhsNumberArray:
			b.line(pad + "_ = c.NumberArraySet(" + slot.base + ", " + slot.key + ".Int(), " + adjustedNumber(values[i]) + ")")
		case lhsField:
			if slot.mono == monoNumber && slot.cache != "" {
				b.line(pad + "_ = c.TableSetStableNumber(" + slot.base + ", " + g.staticExpr(slot.field) + ", " + adjustedNumber(values[i]) + ", " + slot.cache + ")")
			} else if slot.cache != "" {
				b.line(pad + "_ = c.TableSetStable(" + slot.base + ", " + g.staticExpr(slot.field) + ", " + adjustedLuaValue(values[i]) + ", " + slot.cache + ")")
			} else {
				b.line(pad + "_ = c.TableSet(" + slot.base + ", " + g.staticExpr(slot.field) + ", " + adjustedLuaValue(values[i]) + ")")
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
		b.line(pad + "return")
		return
	}
	if g.current.returnN == 0 {
		// Even a zero-result trailing call must execute for side effects:
		// `return bump()` is not equivalent to a bare return.
		g.emitAdjustedValues(b, s.Values, 0, indent)
		b.line(pad + "return")
		return
	}
	if g.current.returnN == 1 && g.current.returnKind(0) == monoNumber {
		if len(s.Values) != 1 || !g.exprNumber(s.Values[0]) {
			g.setErr(fmt.Errorf("transpiler: internal numeric return mismatch in %s at %s", g.current.key, s.Pos()))
			b.line(pad + "return 0")
			return
		}
		value := g.emitNumberExpr(s.Values[0])
		g.writePending(b, indent)
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
	b.line(pad + "return " + strings.Join(parts, ", "))
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
// (extra results still execute for side effects), a trailing user-function
// call expands to all of its results, and the list is then truncated or
// nil-padded to nWanted slots.
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
				fn := g.calledFunction(call)
				if fn != nil && fn.returnN == 0 {
					// A known zero-result user call expands to no value slots,
					// but it still executes when it is the trailing expression.
					if callExpr := g.emitCall(call, false); callExpr != "" {
						g.writePending(b, indent)
						b.line(pad + callExpr)
					} else {
						g.writePending(b, indent)
					}
					continue
				}
				if fn != nil && fn.returnN > 1 {
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
	if (name != "all" && name != "pairs") || len(call.Args) != 1 {
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
	b.line(pad + root + " := gluPushRoots(c, " + table + ")")
	b.line(pad + "if " + root + " >= 0 {")
	g.pushScope()
	loopVars := make([]string, len(s.Names))
	for i, n := range s.Names {
		loopVars[i] = g.declareLocal(n)
	}
	switch name {
	case "all":
		idx := g.next("i")
		b.line(pad + "\tfor " + idx + " := 1; " + idx + " <= c.TableLen(" + table + "); " + idx + "++ {")
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
	b.line(pad + "\tgluPopRoots(c, " + root + ")")
	b.line(pad + "}")
}

func firstArg(c *luaparser.CallExpr) luaparser.Expr {
	if len(c.Args) == 0 {
		return &luaparser.NilExpr{}
	}
	return c.Args[0]
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
			return fmt.Sprintf("rt.IntVal(%d)", int64(x.Value))
		}
		return fmt.Sprintf("rt.NumVal(rt.Number(%d))", int64(math.Round(x.Value*65536)))
	case *luaparser.StringExpr:
		return g.staticExpr(x.Value)
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
	case *luaparser.NumberExpr:
		return true
	case *luaparser.VarExpr:
		return g.lookupKind(x.Name) == monoNumber
	case *luaparser.FieldExpr:
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
		}
		return (x.Op == luaparser.OpNegate || x.Op == luaparser.OpBitNot) && g.exprNumber(x.Expr)
	case *luaparser.BinaryExpr:
		switch x.Op {
		case luaparser.OpAdd, luaparser.OpSubtract, luaparser.OpMultiply,
			luaparser.OpDivide, luaparser.OpIntegerDivide, luaparser.OpModulo,
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
		requireNumbers := func() bool {
			for _, arg := range x.Args {
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
	case *luaparser.NumberExpr:
		if x.Integer {
			return fmt.Sprintf("rt.FromInt(%d)", int64(x.Value))
		}
		return fmt.Sprintf("rt.Number(%d)", int64(math.Round(x.Value*65536)))
	case *luaparser.VarExpr:
		return g.lookup(x.Name)
	case *luaparser.FieldExpr:
		return "c.TableGetStableNumber(" + g.emitExpr(x.Base) + ", " + g.staticExpr(x.Name) + ", " + g.fieldCacheRef(x) + ")"
	case *luaparser.IndexExpr:
		if name, _, ok := g.numericArrayIndex(x); ok {
			key := g.emitExpr(x.Key)
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
		}
		value := g.emitNumberExpr(x.Expr)
		if x.Op == luaparser.OpNegate {
			return "-(" + value + ")"
		}
		return "(" + value + ").Bnot()"
	case *luaparser.BinaryExpr:
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
		args := g.sequenceNumberExprs(x.Args)
		arg := func(i int, fallback string) string {
			if i >= len(args) {
				return fallback
			}
			return args[i]
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
		return "gluConcat(c," + names[0] + "," + names[1] + g.rootArgs() + ")"
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
		return "rt.BoolVal(gluCmp(" + left + "," + right + ",\"" + string(op) + "\"))"
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
	if n := len(x.Fields); n > 0 && x.Fields[n-1].Array {
		if call, ok := x.Fields[n-1].Value.(*luaparser.CallExpr); ok {
			if fn := g.calledFunction(call); fn != nil && fn.returnN > 1 {
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
	if trailingFn != nil {
		arrayN += trailingFn.returnN - 1
	}
	tbl := g.next("tbl")
	fail := g.next("tblFail")
	pushed := g.next("tblPush")
	errName := g.next("tblErr")
	g.pending = append(g.pending, "var "+tbl+" rt.Value")
	g.pending = append(g.pending, fail+" := false")
	g.pending = append(g.pending, pushed+" := 0")
	for _, root := range g.rootVars() {
		g.pending = append(g.pending, "if !"+fail+" {")
		g.pending = append(g.pending, "\tif c.PushLocal("+root+") { "+pushed+"++ } else { "+fail+" = true }")
		g.pending = append(g.pending, "}")
	}
	g.pending = append(g.pending, "if !"+fail+" {")
	g.pending = append(g.pending, "\tvar "+errName+" rt.HeapError")
	g.pending = append(g.pending, "\t"+tbl+", "+errName+" = c.NewTableWithCaps("+strconv.Itoa(max(arrayN, 1))+", "+strconv.Itoa(max(hashN, 1))+")")
	g.pending = append(g.pending, "\tif "+errName+" != rt.HeapOK { "+fail+" = true } else if !c.PushLocal("+tbl+") { "+fail+" = true } else { "+pushed+"++ }")
	g.pending = append(g.pending, "}")
	oldRoots := g.extraRoots
	g.extraRoots = append(append([]string(nil), oldRoots...), tbl)
	for i, f := range x.Fields {
		last := i == len(x.Fields)-1
		g.pending = append(g.pending, "if !"+fail+" {")
		inner, _ := g.capturePending(func() string {
			if f.Array {
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
						g.pending = append(g.pending, "\tif c.TableAdd("+tbl+","+value+",rt.NilVal()) != rt.HeapOK { "+fail+" = true }")
						g.pending = append(g.pending, "}")
					}
					return ""
				}
				v := g.emitExpr(f.Value)
				g.pending = append(g.pending, "if c.TableAdd("+tbl+","+v+",rt.NilVal()) != rt.HeapOK { "+fail+" = true }")
				return ""
			}
			names := g.sequenceExprs([]luaparser.Expr{f.Key, f.Value})
			g.pending = append(g.pending, "if c.TableSet("+tbl+","+names[0]+","+names[1]+") != rt.HeapOK { "+fail+" = true }")
			return ""
		})
		g.indentPending(inner)
		g.pending = append(g.pending, "}")
	}
	g.extraRoots = oldRoots
	g.pending = append(g.pending, "c.PopLocals("+pushed+")")
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
		if len(call.Args) < spec.MinArgs || len(call.Args) > spec.MaxArgs {
			g.setErr(fmt.Errorf("transpiler: %s expects %d..%d arguments, got %d", name, spec.MinArgs, spec.MaxArgs, len(call.Args)))
			return "rt.NilVal()"
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

func (g *generator) emitBuiltin(name string, argExprs []luaparser.Expr, value bool) string {
	names := g.sequenceExprs(argExprs)
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
	case "cls", "pset", "line", "rect", "rectfill", "circ", "circfill", "spr", "sspr", "camera", "clip", "color", "cursor":
		return void("c." + Builtins[name].GoName + "(" + argsUpTo(Builtins[name].MaxArgs) + ")")
	case "map":
		if len(argExprs) == 0 {
			return void(fmt.Sprintf("c.MapAll(%d)", g.opts.MapDrawMask))
		}
		bgValue, _ := staticNumberLiteral(argExprs[0])
		bg := int(bgValue)
		slot := mapMaskSlot(g.opts.MapDrawMask, bg)
		return void(fmt.Sprintf("c.MapBGAtV(%d,%s)", slot, argsUpTo(8)))
	case "pal":
		switch len(argExprs) {
		case 0:
			return void("c.PalReset()")
		case 1:
			if n, ok := argExprs[0].(*luaparser.NumberExpr); ok && n.Value == 2 {
				g.setErr(fmt.Errorf("transpiler: pal(2) is unsupported while fillp is deferred"))
				return "rt.NilVal()"
			}
			return void("c.PalLayerReset(" + arg(0) + ".Int())")
		default:
			return void("c.PalV(" + argsUpTo(3) + ")")
		}
	case "palt":
		if len(argExprs) == 0 {
			return void("c.PaltReset()")
		}
		return void("c.PaltV(" + argsUpTo(2) + ")")
	case "pget":
		return wrap("c.PgetV(" + argsUpTo(Builtins[name].MaxArgs) + ")")
	case "print":
		call := "c.PrintV(" + arg(0) + "," + arg(1) + "," + arg(2) + "," + arg(3) + ")"
		if value {
			return wrap("rt.IntVal(" + call + ")")
		}
		return call
	case "sget":
		return wrap("rt.IntVal(int(c.Sget(" + arg(0) + ".Int()," + arg(1) + ".Int())))")
	case "sset":
		return void("c.Sset(" + arg(0) + ".Int()," + arg(1) + ".Int()," + arg(2) + ".Int())")
	case "fget":
		if len(argExprs) < 2 {
			return wrap("rt.IntVal(c.FgetByte(" + arg(0) + ".Int()))")
		}
		return wrap("rt.BoolVal(c.Fget(" + arg(0) + ".Int()," + arg(1) + ".Int()))")
	case "fset":
		if len(argExprs) < 3 {
			return void("c.FsetByte(" + arg(0) + ".Int()," + arg(1) + ".Int())")
		}
		return void("c.Fset(" + arg(0) + ".Int()," + arg(1) + ".Int()," + arg(2) + ".Truthy())")
	case "mget":
		return wrap("rt.IntVal(int(c.Mget(" + arg(0) + ".Int()," + arg(1) + ".Int())))")
	case "mset":
		return void("c.Mset(" + arg(0) + ".Int()," + arg(1) + ".Int()," + arg(2) + ".Int())")
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
			g.pending = append(g.pending, "_ = c.TableAdd("+table+", "+v+", "+idx+")")
			return v
		}
		return "_ = c.TableAdd(" + table + "," + val + "," + idx + ")"
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
		return wrap("gluSub(c," + arg(0) + "," + arg(1) + "," + arg(2) + g.rootArgs() + ")")
	case "tostr":
		return wrap("gluTostr(c," + arg(0) + "," + arg(1) + ".Truthy()" + g.rootArgs() + ")")
	case "tonum":
		return wrap("c.Tonum(" + arg(0) + ")")
	case "chr":
		return wrap("gluChr(c," + arg(0) + g.rootArgs() + ")")
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

func validateTargetSFX(definitions []assetsync.TargetSFX, steps []assetsync.TargetSFXStep) error {
	if len(definitions) == 0 {
		if len(steps) != 0 {
			return errors.New("transpiler: SFX steps require definitions")
		}
		return nil
	}
	if len(definitions) > assetsync.MaxSFXAssets {
		return fmt.Errorf("transpiler: %d SFX definitions exceed %d", len(definitions), assetsync.MaxSFXAssets)
	}
	previousID := -1
	for _, definition := range definitions {
		if definition.ID >= assetsync.MaxSFXAssets || int(definition.ID) <= previousID || definition.Speed == 0 || definition.StepCount == 0 || definition.StepCount > assetsync.MaxSFXSteps ||
			int(definition.StepOffset)+int(definition.StepCount) > len(steps) || definition.Filters&1 != 0 || definition.Filters > 214 {
			return errors.New("transpiler: malformed SFX definition data")
		}
		if definition.LoopStart == assetsync.SFXNoLoop || definition.LoopEnd == assetsync.SFXNoLoop {
			if definition.LoopStart != assetsync.SFXNoLoop || definition.LoopEnd != assetsync.SFXNoLoop {
				return errors.New("transpiler: malformed SFX loop data")
			}
		} else if definition.LoopStart >= definition.LoopEnd || definition.LoopEnd > definition.StepCount {
			return errors.New("transpiler: malformed SFX loop data")
		}
		previousID = int(definition.ID)
	}
	for _, step := range steps {
		if step.PhaseStep == 0 || step.Volume > 256 || waveformGoName(step.Waveform) == "" || effectGoName(step.Effect) == "" {
			return errors.New("transpiler: malformed SFX step data")
		}
	}
	return nil
}

func validateTargetMusic(patterns []assetsync.TargetMusicPattern, definitions []assetsync.TargetSFX) error {
	if len(patterns) == 0 {
		return nil
	}
	if len(patterns) > assetsync.MaxMusicPatterns || len(definitions) == 0 {
		return errors.New("transpiler: music patterns require valid SFX definitions")
	}
	patternIDs := make(map[uint8]struct{}, len(patterns))
	sfxIDs := make(map[uint8]struct{}, len(definitions))
	for _, definition := range definitions {
		sfxIDs[definition.ID] = struct{}{}
	}
	previousID := -1
	for _, pattern := range patterns {
		if pattern.DurationSamples == 0 || pattern.ID >= assetsync.MaxMusicPatterns || int(pattern.ID) <= previousID {
			return errors.New("transpiler: malformed music pattern data")
		}
		patternIDs[pattern.ID] = struct{}{}
		previousID = int(pattern.ID)
		for _, id := range pattern.Channels {
			if id < -1 {
				return errors.New("transpiler: malformed music channel data")
			}
			if id >= 0 {
				if _, exists := sfxIDs[uint8(id)]; !exists {
					return errors.New("transpiler: music channel references missing SFX")
				}
			}
		}
	}
	for _, pattern := range patterns {
		if pattern.Next < -1 {
			return errors.New("transpiler: malformed music transition data")
		}
		if pattern.Next >= 0 {
			if _, exists := patternIDs[uint8(pattern.Next)]; !exists {
				return errors.New("transpiler: music transition references missing pattern")
			}
		}
	}
	return nil
}

func validateTargetSamples(definitions []assetsync.TargetSample, pcm []int8) error {
	if len(definitions) == 0 {
		if len(pcm) != 0 {
			return errors.New("transpiler: sample PCM requires definitions")
		}
		return nil
	}
	if len(definitions) > assetsync.MaxSampleAssets || len(pcm) > assetsync.MaxSampleROMBytes {
		return errors.New("transpiler: sample asset data exceeds target limits")
	}
	previousID := -1
	for _, definition := range definitions {
		end := uint64(definition.DataOffset) + uint64(definition.DataLength)
		if definition.ID >= assetsync.MaxSampleAssets || int(definition.ID) <= previousID || definition.DataLength == 0 ||
			end > uint64(len(pcm)) || definition.Volume > 256 {
			return errors.New("transpiler: malformed sample definition data")
		}
		if definition.Loop {
			if definition.LoopStart >= definition.LoopEnd || definition.LoopEnd > definition.DataLength ||
				definition.LoopEnd-definition.LoopStart < 2 {
				return errors.New("transpiler: malformed sample loop data")
			}
		} else if definition.LoopStart != 0 || definition.LoopEnd != 0 {
			return errors.New("transpiler: malformed sample loop data")
		}
		previousID = int(definition.ID)
	}
	return nil
}

func waveformGoName(waveform string) string {
	switch waveform {
	case "triangle":
		return "audio.SourceTriangle"
	case "tilted-saw":
		return "audio.SourceTiltedSaw"
	case "saw":
		return "audio.SourceSaw"
	case "square":
		return "audio.SourceSquare"
	case "pulse":
		return "audio.SourcePulse"
	case "organ":
		return "audio.SourceOrgan"
	case "noise":
		return "audio.SourceNoise"
	case "phaser":
		return "audio.SourcePhaser"
	case "custom-0":
		return "audio.SourceCustom0"
	case "custom-1":
		return "audio.SourceCustom1"
	case "custom-2":
		return "audio.SourceCustom2"
	case "custom-3":
		return "audio.SourceCustom3"
	case "custom-4":
		return "audio.SourceCustom4"
	case "custom-5":
		return "audio.SourceCustom5"
	case "custom-6":
		return "audio.SourceCustom6"
	case "custom-7":
		return "audio.SourceCustom7"
	default:
		return ""
	}
}

func effectGoName(effect string) string {
	switch effect {
	case "none":
		return "audio.SFXEffectNone"
	case "slide":
		return "audio.SFXEffectSlide"
	case "vibrato":
		return "audio.SFXEffectVibrato"
	case "drop":
		return "audio.SFXEffectDrop"
	case "fade-in":
		return "audio.SFXEffectFadeIn"
	case "fade-out":
		return "audio.SFXEffectFadeOut"
	case "arpeggio-fast":
		return "audio.SFXEffectArpeggioFast"
	case "arpeggio-slow":
		return "audio.SFXEffectArpeggioSlow"
	default:
		return ""
	}
}

func sfxDefinitionLiteral(definitions []assetsync.TargetSFX) string {
	var builder strings.Builder
	for _, definition := range definitions {
		fmt.Fprintf(&builder, "{ID:%d, Speed:%d, StepOffset:%d, StepCount:%d, LoopStart:%d, LoopEnd:%d, Filters:%d},", definition.ID, definition.Speed, definition.StepOffset, definition.StepCount, definition.LoopStart, definition.LoopEnd, definition.Filters)
	}
	return builder.String()
}

func sfxStepLiteral(steps []assetsync.TargetSFXStep) string {
	var builder strings.Builder
	for _, step := range steps {
		fmt.Fprintf(&builder, "{PhaseStep:%d, Volume:%d, Kind:%s, Effect:%s},", step.PhaseStep, step.Volume, waveformGoName(step.Waveform), effectGoName(step.Effect))
	}
	return builder.String()
}

func musicPatternLiteral(patterns []assetsync.TargetMusicPattern) string {
	var builder strings.Builder
	for _, pattern := range patterns {
		fmt.Fprintf(&builder, "{ID:%d,Channels:[audio.SFXChannels]int8{%d,%d,%d,%d},Next:%d,DurationSamples:%d},",
			pattern.ID, pattern.Channels[0], pattern.Channels[1], pattern.Channels[2], pattern.Channels[3], pattern.Next, pattern.DurationSamples)
	}
	return builder.String()
}

func sampleDefinitionLiteral(definitions []assetsync.TargetSample) string {
	var builder strings.Builder
	for _, definition := range definitions {
		fmt.Fprintf(&builder, "{ID:%d,DataOffset:%d,DataLength:%d,LoopStart:%d,LoopEnd:%d,Volume:%d,Priority:%d,Loop:%t},",
			definition.ID, definition.DataOffset, definition.DataLength, definition.LoopStart, definition.LoopEnd, definition.Volume, definition.Priority, definition.Loop)
	}
	return builder.String()
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

func callKey(call *luaparser.CallExpr) (string, bool) {
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
	return g.funcs[key]
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
	names := make([]string, len(exprs))
	if len(exprs) == 0 {
		return names
	}
	savedRoots := g.extraRoots
	g.extraRoots = append([]string(nil), savedRoots...)
	for i, expr := range exprs {
		kind := monoValue
		if i < len(fn.paramKinds) {
			kind = fn.paramKinds[i]
		}
		var code string
		if kind == monoNumber {
			if !g.exprNumber(expr) {
				g.setErr(fmt.Errorf("transpiler: internal numeric argument mismatch for %s at %s", fn.key, expr.Pos()))
				code = "0"
			} else {
				code = g.emitNumberExpr(expr)
			}
		} else {
			code = g.emitExpr(expr)
		}
		if len(exprs) == 1 {
			names[i] = code
			continue
		}
		tmp := g.next("arg")
		g.pending = append(g.pending, tmp+" := "+code)
		names[i] = tmp
		if kind == monoValue {
			g.extraRoots = append(g.extraRoots, tmp)
		}
	}
	g.extraRoots = savedRoots
	return names
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
	if fn.noAlloc {
		return "g." + fn.goName + "(c" + namesToArgs(args) + ")"
	}
	return "g.__glu_call_" + fn.goName + "(c" + namesToArgs(args) + g.rootArgs() + ")"
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
	if fn.noAlloc {
		return "g." + fn.goName + "(c" + namesToArgs(args) + ")"
	}
	return "g.__glu_multi_" + fn.goName + "(c" + namesToArgs(args) + g.rootArgs() + ")"
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
	return "g.__glu_first_" + fn.goName + "(c" + namesToArgs(args) + g.rootArgs() + ")"
}

// rootVars lists every Go variable currently reachable in the lexical scope
// stack (including any outer bindings shadowed by an inner `local` of the
// same name; a shadowed variable's Go slot can still hold a live heap value
// that must survive a GC triggered by an allocating call), plus any
// caller-supplied extraRoots.
func (g *generator) rootVars() []string {
	var p []string
	for _, scope := range g.scopes {
		for _, e := range scope.entries {
			if e.kind == monoValue {
				p = append(p, e.goVar)
			}
		}
	}
	p = append(p, g.extraRoots...)
	return p
}

// rootArgs formats the current root variables for variadic generated runtime
// calls.
func (g *generator) rootArgs() string {
	p := g.rootVars()
	if len(p) == 0 {
		return ""
	}
	return "," + strings.Join(p, ",")
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

func int8Literal(data []int8) string {
	parts := make([]string, len(data))
	for i, value := range data {
		parts[i] = strconv.Itoa(int(value))
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
