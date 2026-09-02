package transpiler

import (
	"math"

	"github.com/drpaneas/mtile/internal/luaparser"
)

const (
	mode0OAMEntryLimit            = 128
	mode0OBJLineCycleLimit        = 1226
	mode0TextTileLimit            = 32
	mode0PackedObjectEntryLimit   = 8
	mode0PackedObjectTileCapacity = 32
	mode0OBJCoordinatePeriod      = 256
	mode0TextGlyphWidth           = 6
	mode0TextGlyphHeight          = 8
	mode0ScreenWidth              = 240
)

// mode0ObjectBudget mirrors the linked normal-OBJ renderer. OAMEntries is the
// actual post-packing allocation, while MaxLineCycles is the worst normal-OBJ
// rendering cost on the GBA's circular 8-bit Y coordinate. DISPCNT keeps the
// H-Blank Interval Free bit clear, which leaves 308*4-6 = 1226 rendering cycles.
type mode0ObjectBudget struct {
	OAMEntries     int
	MaxLineCycles  int
	TextEntries    int
	TextTiles      int
	PackingEnabled bool
}

type mode0PackedObjectKey struct {
	source int
	cells  int
}

type mode0PackingBudgetPlanner struct {
	enabled bool
	keys    [mode0PackedObjectEntryLimit]mode0PackedObjectKey
	count   int
	tiles   int
}

func (p *mode0PackingBudgetPlanner) use(source, cells int) bool {
	if p == nil || !p.enabled || cells != 2 && cells != 4 {
		return false
	}
	for index := 0; index < p.count; index++ {
		if p.keys[index] == (mode0PackedObjectKey{source: source, cells: cells}) {
			return true
		}
	}
	tiles := cells * cells
	if p.count >= len(p.keys) || p.tiles+tiles > mode0PackedObjectTileCapacity {
		return false
	}
	p.keys[p.count] = mode0PackedObjectKey{source: source, cells: cells}
	p.count++
	p.tiles += tiles
	return true
}

func mode0ObjectBudgetForChunk(chunk *luaparser.Chunk, opts Options) mode0ObjectBudget {
	if chunk == nil {
		return mode0ObjectBudget{}
	}
	for _, stmt := range chunk.Body {
		fn, ok := stmt.(*luaparser.FunctionDeclStmt)
		if ok && functionKey(fn) == "_draw" {
			return mode0ObjectBudgetForDraw(chunk, fn, opts)
		}
	}
	return mode0ObjectBudget{}
}

func mode0ObjectBudgetForDraw(chunk *luaparser.Chunk, draw *luaparser.FunctionDeclStmt, opts Options) mode0ObjectBudget {
	packing := mode0ObjectPackingEligible(chunk, opts)
	budget := mode0ObjectBudget{PackingEnabled: packing}
	planner := mode0PackingBudgetPlanner{enabled: packing}
	var staticCycles [mode0OBJCoordinatePeriod]int
	dynamicCycles := 0
	cols := opts.SheetW / 8
	cameraRebase := mode0DrawRebasesCameraBetweenObjects(draw, newMode0CallGraph(chunk))
	textXDynamic := chunkCallsBuiltin(chunk, "camera")

	for _, stmt := range draw.Body.Body {
		callStmt, ok := stmt.(*luaparser.CallStmt)
		if !ok {
			continue
		}
		call := callStmt.Call
		name, ok := callKey(call)
		if !ok {
			continue
		}
		switch name {
		case "spr":
			id := mode0LiteralInt(call.Args[0])
			w, h := 1, 1
			if len(call.Args) > 3 {
				w = mode0LiteralInt(call.Args[3])
			}
			if len(call.Args) > 4 {
				h = mode0LiteralInt(call.Args[4])
			}
			packed := planner.use(id, mode0SprCallPackedCells(call))
			if packed {
				budget.OAMEntries++
			} else {
				budget.OAMEntries += w * h
			}
			yExpr := call.Args[2]
			if cameraRebase {
				yExpr = nil
			}
			addMode0SpriteLineProfile(&staticCycles, &dynamicCycles, yExpr, w, h, packed)
		case "sspr":
			sx := mode0LiteralInt(call.Args[0])
			sy := mode0LiteralInt(call.Args[1])
			w := mode0LiteralInt(call.Args[2]) / 8
			h := mode0LiteralInt(call.Args[3]) / 8
			id := (sy/8)*cols + sx/8
			packed := planner.use(id, mode0SSprCallPackedCells(call))
			if packed {
				budget.OAMEntries++
			} else {
				budget.OAMEntries += w * h
			}
			yExpr := call.Args[5]
			if cameraRebase {
				yExpr = nil
			}
			addMode0SpriteLineProfile(&staticCycles, &dynamicCycles, yExpr, w, h, packed)
		case "print":
			text, ok := call.Args[0].(*luaparser.StringExpr)
			if !ok {
				continue
			}
			entries := len(text.Value)
			budget.OAMEntries += entries
			budget.TextEntries += entries
			addMode0TextLineProfile(&staticCycles, &dynamicCycles, call.Args[1], call.Args[2], entries, textXDynamic || cameraRebase)
		}
	}

	for _, cycles := range staticCycles {
		if total := cycles + dynamicCycles; total > budget.MaxLineCycles {
			budget.MaxLineCycles = total
		}
	}
	budget.TextTiles = len(mode0TextAtlasKeysForDraw(draw))
	return budget
}

// mode0DrawRebasesCameraBetweenObjects reports the draw shape that breaks the
// shared-camera translation proof. A camera change before every object is a
// common translation and preserves relative scanline bands; a change between
// objects lets previously separated bands coincide and needs a conservative
// independent-placement proof.
func mode0DrawRebasesCameraBetweenObjects(draw *luaparser.FunctionDeclStmt, graph *mode0CallGraph) bool {
	if draw == nil {
		return false
	}
	seenObject := false
	cameraAfterObject := false
	for _, stmt := range draw.Body.Body {
		containsCamera := graph.firstBuiltinFromStmt(stmt, "camera") != nil
		if containsCamera && seenObject {
			cameraAfterObject = true
		}
		callStmt, ok := stmt.(*luaparser.CallStmt)
		if !ok {
			continue
		}
		name, ok := callKey(callStmt.Call)
		if !ok || name != "spr" && name != "sspr" && name != "print" {
			continue
		}
		if cameraAfterObject {
			return true
		}
		seenObject = true
	}
	return false
}

func addMode0TextLineProfile(staticCycles *[mode0OBJCoordinatePeriod]int, dynamicCycles *int,
	xExpr, yExpr luaparser.Expr, entries int, dynamicX bool,
) {
	if entries <= 0 {
		return
	}
	x, xStatic := mode0StaticRuntimeInt(xExpr)
	if dynamicX || !xStatic {
		// An unknown horizontal phase can make every glyph occupy one scanline.
		*dynamicCycles += entries * 8
		return
	}
	var profile [mode0OBJCoordinatePeriod]int
	sx, sy := x, 0
	left := sx
	if left < 0 {
		left = 0
	}
	for index := 0; index < entries; index++ {
		if sx+mode0TextGlyphWidth > mode0ScreenWidth && sx > left {
			sx = left
			sy += mode0TextGlyphHeight
		}
		addMode0ProfileRun(&profile, sy, mode0TextGlyphHeight, 8)
		sx += mode0TextGlyphWidth
	}
	mergeMode0LineProfile(staticCycles, dynamicCycles, yExpr, &profile)
}

func mode0LiteralInt(expr luaparser.Expr) int {
	value, _ := staticNumberLiteral(expr)
	return int(value)
}

func addMode0SpriteLineProfile(staticCycles *[mode0OBJCoordinatePeriod]int, dynamicCycles *int,
	yExpr luaparser.Expr, w, h int, packed bool,
) {
	if packed {
		addMode0LineRun(staticCycles, dynamicCycles, yExpr, 0, h*8, w*8)
		return
	}
	var profile [mode0OBJCoordinatePeriod]int
	for row := 0; row < h; row++ {
		addMode0ProfileRun(&profile, row*8, 8, w*8)
	}
	mergeMode0LineProfile(staticCycles, dynamicCycles, yExpr, &profile)
}

func addMode0LineRun(staticCycles *[mode0OBJCoordinatePeriod]int, dynamicCycles *int,
	yExpr luaparser.Expr, offset, height, cycles int,
) {
	var profile [mode0OBJCoordinatePeriod]int
	addMode0ProfileRun(&profile, offset, height, cycles)
	mergeMode0LineProfile(staticCycles, dynamicCycles, yExpr, &profile)
}

func addMode0ProfileRun(profile *[mode0OBJCoordinatePeriod]int, offset, height, cycles int) {
	if profile == nil || height <= 0 || cycles <= 0 {
		return
	}
	for line := 0; line < height; line++ {
		profile[(offset+line)&(mode0OBJCoordinatePeriod-1)] += cycles
	}
}

func mergeMode0LineProfile(staticCycles *[mode0OBJCoordinatePeriod]int, dynamicCycles *int,
	yExpr luaparser.Expr, profile *[mode0OBJCoordinatePeriod]int,
) {
	if y, ok := mode0StaticY(yExpr); ok {
		for line, cycles := range profile {
			staticCycles[(line+y)&(mode0OBJCoordinatePeriod-1)] += cycles
		}
		return
	}
	peak := 0
	for _, cycles := range profile {
		if cycles > peak {
			peak = cycles
		}
	}
	*dynamicCycles += peak
}

func mode0StaticY(expr luaparser.Expr) (int, bool) {
	value, literal := mode0StaticRuntimeInt(expr)
	if !literal {
		return 0, false
	}
	value %= mode0OBJCoordinatePeriod
	if value < 0 {
		value += mode0OBJCoordinatePeriod
	}
	return value, true
}

// mode0StaticRuntimeInt mirrors literal lowering to the runtime's signed
// 16:16 Number before Int() performs its arithmetic shift. Flooring the source
// float first is wrong near fixed-point quantization boundaries.
func mode0StaticRuntimeInt(expr luaparser.Expr) (int, bool) {
	value, literal := staticNumberLiteral(expr)
	if !literal || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	fixed := fixedNumberLiteral(value)
	return int(fixed >> 16), true
}
