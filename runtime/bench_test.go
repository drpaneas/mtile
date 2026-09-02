package runtime

import "testing"

// Host benchmarks for the runtime's per-frame hot paths.
//
// Absolute ns/op on a development machine does not translate to ARM7TDMI
// cycles, but the *relative* cost of operations does, and these benchmarks
// catch regressions (an accidental O(n^2), a lost fast path) long before a
// ROM misses its frame deadline. The on-target companion is
// runtime/cmd/benchspike, which measures the same workloads in real GBA
// cycles with the hardware timers.
//
// Run with:
//
//	go test -bench=. -benchmem ./...
//
// -benchmem must report 0 allocs/op for every benchmark here: the runtime
// is designed to never allocate on the Go heap after Init.

// benchSheetW/H build a 128x128 sheet (16x16 sprites), a realistic mid-size
// game sheet that exercises the same row-stride math as a full 240x160 one.
const (
	benchSheetW  = 128
	benchSheetH  = 128
	benchMapW    = 64
	benchMapH    = 64
	benchSpriteN = (benchSheetW / 8) * (benchSheetH / 8)
)

func newBenchContext(b *testing.B) *Context {
	b.Helper()
	sheet := make([]byte, benchSheetW*benchSheetH)
	for i := range sheet {
		// Roughly half transparent (color 0), half opaque, in 4px runs:
		// exercises both the packed word fast path and the per-byte
		// transparency fallback like real art does.
		if (i/4)%2 == 0 {
			sheet[i] = byte(1 + i%15)
		}
	}
	mapCells := make([]uint16, benchMapW*benchMapH)
	for i := range mapCells {
		mapCells[i] = uint16(1 + i%(benchSpriteN-1))
	}
	flags := make([]byte, benchSpriteN)
	image := &AssetImage{
		SheetW: benchSheetW, SheetH: benchSheetH,
		SpriteCols: benchSheetW / 8, SpriteRows: benchSheetH / 8,
		SpriteN: benchSpriteN,
		MapW:    benchMapW, MapH: benchMapH,
		Sheet: sheet, Map: mapCells, Flags: flags,
	}
	ram := &AssetMemory{
		SheetW: benchSheetW, SheetH: benchSheetH,
		SpriteCols: benchSheetW / 8, SpriteRows: benchSheetH / 8,
		SpriteN: benchSpriteN,
		MapW:    benchMapW, MapH: benchMapH,
		Sheet: make([]byte, len(sheet)), Map: make([]uint16, len(mapCells)),
		Flags: make([]byte, len(flags)),
	}
	c := &Context{}
	if !c.Init(image, ram) {
		b.Fatal("bench context failed to Init")
	}
	c.Cls(0)
	return c
}

func newBenchSparseMapContext(tb testing.TB) *Context {
	tb.Helper()
	sheet := make([]byte, benchSheetW*benchSheetH)
	for i := range sheet {
		if (i/4)%2 == 0 {
			sheet[i] = byte(1 + i%15)
		}
	}
	const mapW, mapH = 30, 20
	mapCells := make([]uint16, mapW*mapH)
	put := func(x, y, t int) {
		if x < 0 || x >= mapW || y < 0 || y >= mapH {
			return
		}
		mapCells[y*mapW+x] = uint16(t)
	}
	for x := 0; x < mapW; x++ {
		put(x, 18, 1)
		put(x, 19, 2)
	}
	for x := 22; x < mapW; x++ {
		put(x, 17, 1)
		if x > 25 {
			put(x, 16, 1)
		}
	}
	for x := 22; x < mapW; x++ {
		put(x, 18, 2)
	}
	for x := 26; x < mapW; x++ {
		put(x, 17, 2)
	}
	for x := 4; x < 9; x++ {
		put(x, 13, 3)
	}
	for x := 12; x < 16; x++ {
		put(x, 10, 3)
	}
	for x := 19; x < 23; x++ {
		put(x, 7, 3)
	}
	for y := 14; y < 18; y++ {
		put(2, y, 4)
	}
	for x := 9; x < 12; x++ {
		put(x, 15, 1)
	}
	for _, p := range [][2]int{{5, 12}, {7, 12}, {13, 9}, {14, 9}, {20, 6}, {3, 17}, {10, 17}, {15, 17}, {24, 16}, {27, 15}} {
		put(p[0], p[1], 16)
	}
	for _, p := range [][2]int{{6, 12}, {21, 6}, {12, 17}, {25, 16}} {
		put(p[0], p[1], 17)
	}
	flags := make([]byte, benchSpriteN)
	image := &AssetImage{
		SheetW: benchSheetW, SheetH: benchSheetH,
		SpriteCols: benchSheetW / 8, SpriteRows: benchSheetH / 8,
		SpriteN: benchSpriteN,
		MapW:    mapW, MapH: mapH,
		Sheet: sheet, Map: mapCells, Flags: flags,
	}
	ram := &AssetMemory{
		SheetW: benchSheetW, SheetH: benchSheetH,
		SpriteCols: benchSheetW / 8, SpriteRows: benchSheetH / 8,
		SpriteN: benchSpriteN,
		MapW:    mapW, MapH: mapH,
		Sheet: make([]byte, len(sheet)), Map: make([]uint16, len(mapCells)),
		Flags: make([]byte, len(flags)),
	}
	c := &Context{}
	if !c.Init(image, ram) {
		tb.Fatal("sparse map bench context failed to Init")
	}
	c.Cls(0)
	return c
}

func newBenchLayeredContext(b *testing.B, mask uint8) *Context {
	b.Helper()
	const mapW, mapH, logicalLayers = 30, 20, 4
	storedLayers := mapMaskPopulation(mask)
	sheet := make([]byte, benchSheetW*benchSheetH)
	for i := range sheet {
		if (i/4)%2 == 0 {
			sheet[i] = byte(1 + i%15)
		}
	}
	mapCells := make([]uint16, mapW*mapH*storedLayers)
	for i := range mapCells {
		mapCells[i] = uint16(1 + i%(benchSpriteN-1))
	}
	flags := make([]byte, benchSpriteN)
	image := &AssetImage{
		SheetW: benchSheetW, SheetH: benchSheetH,
		SpriteCols: benchSheetW / 8, SpriteRows: benchSheetH / 8, SpriteN: benchSpriteN,
		MapW: mapW, MapH: mapH, MapLayers: logicalLayers, MapPlaneMask: mask,
		Sheet: sheet, Map: mapCells, Flags: flags,
	}
	ram := &AssetMemory{
		SheetW: benchSheetW, SheetH: benchSheetH,
		SpriteCols: benchSheetW / 8, SpriteRows: benchSheetH / 8, SpriteN: benchSpriteN,
		MapW: mapW, MapH: mapH, MapLayers: logicalLayers, MapPlaneMask: mask,
		Sheet: make([]byte, len(sheet)), Map: make([]uint16, len(mapCells)), Flags: make([]byte, len(flags)),
	}
	c := &Context{}
	if !c.Init(image, ram) {
		b.Fatal("layered map bench context failed to Init")
	}
	return c
}

// newBenchStreamingContext builds a large map that exceeds
// MapStreamingThresholdCells, forcing the chunked ROM-streaming path.
func newBenchStreamingContext(b *testing.B) *Context {
	return newBenchStreamingContextWithDensity(b, false)
}

func newBenchStreamingSparseContext(b *testing.B) *Context {
	return newBenchStreamingContextWithDensity(b, true)
}

func newBenchStreamingLayeredContext(b *testing.B) *Context {
	b.Helper()
	const mapW, mapH, logicalLayers = 256, 256, 4
	const mask = uint8(0x03)
	storedLayers := mapMaskPopulation(mask)
	sheet := make([]byte, benchSheetW*benchSheetH)
	mapCells := make([]uint16, mapW*mapH*storedLayers)
	for i := range mapCells {
		mapCells[i] = uint16(1 + i%(benchSpriteN-1))
	}
	flags := make([]byte, benchSpriteN)
	image := &AssetImage{
		SheetW: benchSheetW, SheetH: benchSheetH,
		SpriteCols: benchSheetW / 8, SpriteRows: benchSheetH / 8, SpriteN: benchSpriteN,
		MapW: mapW, MapH: mapH, MapLayers: logicalLayers, MapPlaneMask: mask, MapStreaming: true,
		Sheet: sheet, Map: mapCells, Flags: flags,
	}
	ram := &AssetMemory{
		SheetW: benchSheetW, SheetH: benchSheetH,
		SpriteCols: benchSheetW / 8, SpriteRows: benchSheetH / 8, SpriteN: benchSpriteN,
		MapW: mapW, MapH: mapH, MapLayers: logicalLayers, MapPlaneMask: mask, MapStreaming: true,
		Sheet: make([]byte, len(sheet)), Map: make([]uint16, MapCacheCells), Flags: make([]byte, len(flags)),
		MapChunks: make([]MapChunk, MapCacheSlots), MapEdits: make([]MapEdit, MapEditCapacity),
	}
	c := &Context{}
	if !c.Init(image, ram) {
		b.Fatal("bench layered streaming context failed to Init")
	}
	return c
}

func newBenchStreamingContextWithDensity(b *testing.B, sparse bool) *Context {
	b.Helper()
	const mapW, mapH = 256, 256 // 65,536 cells > 16,384 threshold
	sheet := make([]byte, benchSheetW*benchSheetH)
	for i := range sheet {
		if (i/4)%2 == 0 {
			sheet[i] = byte(1 + i%15)
		}
	}
	mapCells := make([]uint16, mapW*mapH)
	if sparse {
		// Match mapdemo's approximately 18%-filled visible rooms.
		for y := 0; y < mapH; y++ {
			for x := 0; x < mapW; x++ {
				if y%20 >= 18 || (x+y*3)%37 == 0 {
					mapCells[y*mapW+x] = uint16(1 + (x+y)%(benchSpriteN-1))
				}
			}
		}
	} else {
		for i := range mapCells {
			mapCells[i] = uint16(1 + i%(benchSpriteN-1))
		}
	}
	flags := make([]byte, benchSpriteN)
	image := &AssetImage{
		SheetW: benchSheetW, SheetH: benchSheetH,
		SpriteCols: benchSheetW / 8, SpriteRows: benchSheetH / 8,
		SpriteN: benchSpriteN,
		MapW:    mapW, MapH: mapH, MapStreaming: true,
		Sheet: sheet, Map: mapCells, Flags: flags,
	}
	ram := &AssetMemory{
		SheetW: benchSheetW, SheetH: benchSheetH,
		SpriteCols: benchSheetW / 8, SpriteRows: benchSheetH / 8,
		SpriteN: benchSpriteN,
		MapW:    mapW, MapH: mapH, MapStreaming: true,
		Sheet: make([]byte, len(sheet)), Map: make([]uint16, MapCacheCells),
		Flags:     make([]byte, len(flags)),
		MapChunks: make([]MapChunk, MapCacheSlots),
		MapEdits:  make([]MapEdit, MapEditCapacity),
	}
	c := &Context{}
	if !c.Init(image, ram) {
		b.Fatal("bench streaming context failed to Init")
	}
	c.Cls(0)
	return c
}

// --- Map data access ---

var benchMapValue uint16

// MgetBG0Compact is the compatibility baseline for the legacy two-argument
// Lua mget(x,y) path. Keep this separate from layered access benchmarks so an
// API expansion cannot hide a regression in existing collision-heavy games.
func BenchmarkMgetBG0Compact(b *testing.B) {
	c := newBenchContext(b)
	var value uint16
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value = c.Mget(i&(benchMapW-1), (i>>6)&(benchMapH-1))
	}
	benchMapValue = value
}

// MgetBG0StreamingWarm measures the legacy read path after its four-slot chunk
// cache contains the queried chunk. Streaming overlays are still empty.
func BenchmarkMgetBG0StreamingWarm(b *testing.B) {
	c := newBenchStreamingContext(b)
	_ = c.Mget(7, 9)
	var value uint16
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value = c.Mget(7, 9)
	}
	benchMapValue = value
}

func BenchmarkMgetBG1Compact(b *testing.B) {
	c := newBenchLayeredContext(b, 0x03)
	var value uint16
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value = c.MgetLayer(1, i%30, (i/30)%20)
	}
	benchMapValue = value
}

func BenchmarkMgetBG1StreamingWarm(b *testing.B) {
	c := newBenchStreamingLayeredContext(b)
	_ = c.MgetLayer(1, 7, 9)
	var value uint16
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value = c.MgetLayer(1, 7, 9)
	}
	benchMapValue = value
}

func BenchmarkMsetBG0CompactChanged(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Mset(7, 9, 1+(i&1))
	}
}

func BenchmarkMsetBG0StreamingChanged(b *testing.B) {
	c := newBenchStreamingContext(b)
	c.Mset(7, 9, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Mset(7, 9, 1+(i&1))
	}
}

func BenchmarkMsetBG1CompactChanged(b *testing.B) {
	c := newBenchLayeredContext(b, 0x03)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.MsetLayer(1, 7, 9, 1+(i&1))
	}
}

func BenchmarkMsetBG1StreamingChanged(b *testing.B) {
	c := newBenchStreamingLayeredContext(b)
	c.MsetLayer(1, 7, 9, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.MsetLayer(1, 7, 9, 1+(i&1))
	}
}

// --- Frame-level primitives ---

func BenchmarkCls(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Cls(1)
	}
}

func BenchmarkRectfillFullScreen(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Rectfill(0, 0, ScreenWidth-1, ScreenHeight-1, 5)
	}
}

func BenchmarkRectfill32x32(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Rectfill(50, 50, 81, 81, 5)
	}
}

func BenchmarkLineDiagonal(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Line(0, 0, ScreenWidth-1, ScreenHeight-1, 7)
	}
}

func BenchmarkCircfillR16(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Circfill(120, 80, 16, 9)
	}
}

// --- Sprites ---

// Spr8x8Aligned hits the packed 32-bit fast path: identity palette, color-0
// key, and 4-byte-aligned source and destination.
func BenchmarkSpr8x8Aligned(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Spr(1, 64, 64, 1, 1, false, false)
	}
}

// Spr8x8Unaligned forces the per-byte path via an odd x position.
func BenchmarkSpr8x8Unaligned(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Spr(1, 65, 64, 1, 1, false, false)
	}
}

// Spr8x8Remapped forces the palette-remap path via pal().
func BenchmarkSpr8x8Remapped(b *testing.B) {
	c := newBenchContext(b)
	c.Pal(1, 9, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Spr(1, 64, 64, 1, 1, false, false)
	}
}

func BenchmarkSpr8x8Flipped(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Spr(1, 64, 64, 1, 1, true, false)
	}
}

func BenchmarkSpr16x16(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Spr(1, 64, 64, 2, 2, false, false)
	}
}

// Spr100Sprites approximates a busy scene: 100 8x8 sprites per frame.
func BenchmarkSpr100Sprites(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for s := 0; s < 100; s++ {
			c.Spr(1+s%32, (s*13)%232, (s*7)%152, 1, 1, false, false)
		}
	}
}

func BenchmarkSSpr2xStretch(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.SSpr(8, 0, 8, 8, 64, 64, 16, 16, false, false)
	}
}

// --- Map ---

// MapFullScreen draws the standard 30x20-cell background covering 240x160.
func BenchmarkMapFullScreen(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Map(0, 0, 0, 0, 30, 20, 0)
	}
}

func BenchmarkMapFullScreenLayerMask(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Map(0, 0, 0, 0, 30, 20, 1)
	}
}

func BenchmarkMapAllSparseTwoOfFour(b *testing.B) {
	c := newBenchLayeredContext(b, 0x0a)
	b.ReportAllocs()
	b.SetBytes(2 * 30 * 20 * 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.MapAll(0x0a)
	}
}

func BenchmarkMapAllFour(b *testing.B) {
	c := newBenchLayeredContext(b, 0x0f)
	b.ReportAllocs()
	b.SetBytes(4 * 30 * 20 * 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.MapAll(0x0f)
	}
}

// BenchmarkMapSparse17Screen draws a 17%-filled 30x20 room like mapdemo.
func BenchmarkMapSparse17Screen(b *testing.B) {
	c := newBenchSparseMapContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Map(0, 0, 0, 0, 30, 20, 0)
	}
}

// MapStreamingFullScreen is the same 30x20 draw against a 256x256 streaming
// map with a warm chunk cache and no edits: the common per-frame case.
func BenchmarkMapStreamingFullScreen(b *testing.B) {
	c := newBenchStreamingContext(b)
	c.Map(0, 0, 0, 0, 30, 20, 0) // warm the chunk cache
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Map(0, 0, 0, 0, 30, 20, 0)
	}
}

// MapStreamingSparse18Screen is the direct comparison for mapdemo: the same
// visible density as its old EWRAM map, through the warm streaming cache.
func BenchmarkMapStreamingSparse18Screen(b *testing.B) {
	c := newBenchStreamingSparseContext(b)
	c.Map(0, 0, 0, 0, 30, 20, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Map(0, 0, 0, 0, 30, 20, 0)
	}
}

// MapStreamingWithEdits measures the same draw after Mset writes force the
// per-cell edit-overlay probes to do real work.
func BenchmarkMapStreamingWithEdits(b *testing.B) {
	c := newBenchStreamingContext(b)
	for i := 0; i < 64; i++ {
		c.Mset(i%32, i/32, 3)
	}
	c.Map(0, 0, 0, 0, 30, 20, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Map(0, 0, 0, 0, 30, 20, 0)
	}
}

// --- Text ---

func BenchmarkPrint10Chars(b *testing.B) {
	c := newBenchContext(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Print("HELLO GBA!", 8, 8, 7)
	}
}

func BenchmarkPrint40CharRow(b *testing.B) {
	c := newBenchContext(b)
	s := "0123456789012345678901234567890123456789"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Print(s, 0, 8, 7)
	}
}

// --- Lua value and table costs (what generated code pays per statement) ---

func BenchmarkValueArithmetic(b *testing.B) {
	x := NumVal(FromInt(3))
	y := NumVal(FromInt(4))
	var v Value
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v = x.Add(y).Mul(x).Sub(y)
	}
	_ = v
}

func BenchmarkTableGetArray(b *testing.B) {
	c := newBenchContext(b)
	t, err := c.NewTableWithCaps(64, 8)
	if err != HeapOK {
		b.Fatal("table alloc failed")
	}
	for i := 1; i <= 64; i++ {
		c.TableSet(t, IntVal(i), IntVal(i*2))
	}
	var v Value
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v = c.TableGet(t, IntVal(1+i%64))
	}
	_ = v
}

func BenchmarkTableSetArray(b *testing.B) {
	c := newBenchContext(b)
	t, err := c.NewTableWithCaps(64, 8)
	if err != HeapOK {
		b.Fatal("table alloc failed")
	}
	for i := 1; i <= 64; i++ {
		c.TableSet(t, IntVal(i), IntVal(i))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.TableSet(t, IntVal(1+i%64), IntVal(i))
	}
}

func BenchmarkTableGetHashStringKey(b *testing.B) {
	c := newBenchContext(b)
	t, err := c.NewTableWithCaps(0, 16)
	if err != HeapOK {
		b.Fatal("table alloc failed")
	}
	key := c.InternStatic("player_x")
	c.TableSet(t, key, IntVal(64))
	var v Value
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v = c.TableGet(t, key)
	}
	_ = v
}

func BenchmarkTableGetHashStringKeyStable(b *testing.B) {
	c := newBenchContext(b)
	t, err := c.NewTableWithCaps(0, 16)
	if err != HeapOK {
		b.Fatal("table alloc failed")
	}
	key := c.InternStatic("player_x")
	c.TableSet(t, key, IntVal(64))
	var cache TableFieldCache
	if c.TableGetStable(t, key, &cache).Int() != 64 || cache == 0 {
		b.Fatal("stable field cache did not initialize")
	}
	var v Value
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v = c.TableGetStable(t, key, &cache)
	}
	_ = v
}

func BenchmarkTableGetHashStringKeyStableNumber(b *testing.B) {
	c := newBenchContext(b)
	t, err := c.NewTableWithCaps(0, 16)
	if err != HeapOK {
		b.Fatal("table alloc failed")
	}
	key := c.InternStatic("player_x")
	c.TableSet(t, key, IntVal(64))
	var cache TableFieldCache
	if c.TableGetStableNumber(t, key, &cache) != FromInt(64) || cache == 0 {
		b.Fatal("stable numeric field cache did not initialize")
	}
	var v Number
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v = c.TableGetStableNumber(t, key, &cache)
	}
	_ = v
}

// TableLen documents the cost generated code pays for #t and for every
// add(): length is a linear scan to the first nil, not a stored count.
func BenchmarkTableLen64(b *testing.B) {
	c := newBenchContext(b)
	t, err := c.NewTableWithCaps(64, 8)
	if err != HeapOK {
		b.Fatal("table alloc failed")
	}
	for i := 1; i <= 64; i++ {
		c.TableSet(t, IntVal(i), IntVal(i))
	}
	var n int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n = c.TableLen(t)
	}
	_ = n
}

// TableAddDel measures the add+del pair a bullet/particle pool pays per
// entity per frame when implemented naively with add() and deli().
func BenchmarkTableAddDeli(b *testing.B) {
	c := newBenchContext(b)
	t, err := c.NewTableWithCaps(64, 0)
	if err != HeapOK {
		b.Fatal("table alloc failed")
	}
	for i := 1; i <= 32; i++ {
		c.TableSet(t, IntVal(i), IntVal(i))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.TableAdd(t, IntVal(i), NilVal())
		c.TableDeli(t, IntVal(1))
	}
}

// --- Math builtins ---

func BenchmarkSinCos(b *testing.B) {
	var v Number
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n := Number(i)
		v = n.Sin() + n.Cos()
	}
	_ = v
}

func BenchmarkSqrt(b *testing.B) {
	var v Number
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v = (Number(i&0xFFFF) << 8).Sqrt()
	}
	_ = v
}
