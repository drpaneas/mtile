package transpiler

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

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
	font := g.opts.Font
	if font == nil {
		font = runtimeapi.DefaultFont()
	}
	b.line(assetsync.FontGoDeclaration(font))
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
	g.renderTileLinkTables(b)
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
	if g.saveEnabled() {
		b.line("//go:section .ewram\nvar saveMemory rt.SaveMemory")
	}
	if g.mapRasterEligible() {
		// Use target sizes: host pointer widths and target capacity tags differ.
		// Reserving both Lua semispaces plus 8 KiB makes the optimization fail
		// closed to zero-length arrays when a small map belongs to a large cart.
		budget := "256*1024 - 512 - 8192 - rt.MapRasterHeapReserve - int(unsafe.Sizeof(ctx)) - int(unsafe.Sizeof(cartData)) - int(unsafe.Sizeof(game)) - int(unsafe.Sizeof(sheetRAM)) - int(unsafe.Sizeof(sheetIndexRAM)) - int(unsafe.Sizeof(mapRAM)) - int(unsafe.Sizeof(flagRAM))"
		if g.saveEnabled() {
			budget += " - int(unsafe.Sizeof(saveMemory))"
		}
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
	b.line("\tif !c.SetFont(&projectFontROM) { panic(\"invalid project font\") }")
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
	g.renderTileLinkBindings(b)
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
		b.line("\tif !ctx.SetFont(&projectFontROM) { panic(\"invalid project font\") }")
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
	if g.saveEnabled() {
		b.line("\tctx.AttachSaveMemory(&saveMemory)")
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
	b.line("\t\tctx.PollInput()")
	b.line("\t\tctx.PollPauseMenu()")
	b.line("\t\tpaused := ctx.PauseActive()")
	b.line("\t\tif !paused {")

	b.line("\t\tctx.BeginProfilePhase(rt.ProfileUpdate)")
	if g.inputEnabled || g.timeEnabled {
		b.line("\t\tctx.AdvanceTick()")
	}
	b.line("\t\tgame.Update(&ctx)")
	b.line("\t\tctx.EndProfilePhase(rt.ProfileUpdate)")

	b.line("\t\tctx.BeginProfilePhase(rt.ProfileDraw)")
	b.line("\t\tctx.BeginRenderFrame()")
	b.line("\t\tgame.Draw(&ctx)")
	b.line("\t\tctx.EndProfilePhase(rt.ProfileDraw)")

	b.line("\t\t// Finish renderer work off screen: Mode 4 already drew on the hidden")
	b.line("\t\t// page, so nothing tears, and update+draw own the full two-frame")
	b.line("\t\t// window (~562k cycles) minus only the flip itself.")
	b.line("\t\tctx.PrepareRender()")

	if g.heapEnabled {
		b.line("\t\tctx.CollectIfNeeded()")
	}
	b.line("\t\t}")
	b.line("\t\tif paused { ctx.PreparePauseMenu() }")
	b.line("\t\tctx.PaceFrame30()")
	b.line("\t\tctx.WaitForPresentWindow()")
	b.line("\t\tif paused { ctx.PresentPauseMenu() } else { ctx.PresentRender() }")
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
