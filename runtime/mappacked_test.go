//go:build !mtile_mode0 && !mtile_tilecanvas

package runtime

import (
	"bytes"
	"math/rand"
	"testing"
	"unsafe"
)

func packedAssets(t testing.TB, cache bool) (*AssetImage, *AssetMemory) {
	t.Helper()
	source := solidMapFixture(t, true, false).cart
	image := &AssetImage{SheetW: 64, SheetH: 8, SpriteCols: 8, SpriteRows: 1, SpriteN: 8, MapW: 40, MapH: 4, MapLayers: 4, MapPlaneMask: 15, SheetReadOnly: true, Sheet: source.Sheet, SheetIndex: source.SheetIndex, Map: source.Map[:40*4*4], Flags: source.Flags}
	memory := &AssetMemory{SheetW: 64, SheetH: 8, SpriteCols: 8, SpriteRows: 1, SpriteN: 8, MapW: 40, MapH: 4, MapLayers: 4, MapPlaneMask: 15, SheetReadOnly: true, Sheet: make([]byte, len(source.Sheet)), SheetIndex: make([]uint16, 8), Map: make([]uint16, len(image.Map)), Flags: make([]byte, 8), SheetSolid: source.SheetSolid}
	if cache {
		memory.MapPackedPixels = make([]byte, 60004)
		memory.MapPackedSpans = make([]MapPackedSpan, 4000)
		for bg := 0; bg < 4; bg++ {
			memory.MapPackedLayers[bg] = MapPackedLayer{uint16(bg * 15000), uint16((bg + 1) * 15000), uint16(bg * 1000), uint16((bg + 1) * 1000)}
		}
	}
	return image, memory
}

func packedFixture(t testing.TB, cache bool) *Context {
	t.Helper()
	image, memory := packedAssets(t, cache)
	c := new(Context)
	if !c.Init(image, memory) {
		t.Fatal("Init")
	}
	return c
}

func TestPackedMapParityAndInvalidation(t *testing.T) {
	fast, plain := packedFixture(t, true), packedFixture(t, false)
	fast.PrepareMapPacked()
	for bg := 0; bg < 4; bg++ {
		if !fast.mapPackedReady[bg] {
			t.Fatalf("layer%d not built", bg)
		}
	}
	rng := rand.New(rand.NewSource(3517))
	for trial := 0; trial < 300; trial++ {
		fast.PrepareMapPacked()
		x, y, w, h := rng.Intn(7)-2, rng.Intn(5)-2, rng.Intn(45), rng.Intn(10)
		camX, camY := rng.Intn(90)-12, rng.Intn(16)-8
		for _, c := range []*Context{fast, plain} {
			c.CameraReset()
			c.ClipReset()
			c.PalReset()
			c.PaltReset()
			c.Cls(199)
			c.Line(0, 0, 239, 159, 42)
			c.Camera(camX, camY)
			if trial%7 == 1 {
				c.Clip(3, 5, 229, 151)
			}
			if trial%7 == 2 {
				c.Pal(7, 0, 0)
			}
			if trial%7 == 3 {
				c.PaltColor(0, false)
				c.Pal(0, 88, 0)
			}
			if trial%7 == 4 {
				c.PaltColor(7, true)
			}
			mask := 0
			if trial%7 == 5 {
				mask = 1
			}
			for bg := 0; bg < 4; bg++ {
				c.MapBGAt(bg, bg, x, y, 0, 0, w, h, mask)
			}
		}
		assertFramesMatch(t, fast, plain, "packed map")
		if trial%11 == 0 {
			bg := trial % 4
			for _, c := range []*Context{fast, plain} {
				c.MsetLayer(bg, 3, 2, (trial/11)%8)
			}
			for other := 0; other < 4; other++ {
				if other != bg && !fast.mapPackedReady[other] {
					t.Fatalf("edit%d invalidated%d", bg, other)
				}
			}
		}
	}
	// A bounded partition can fail; drawing must still use the live cell map.
	fast.cart.MapPackedLayers[1].PixelEnd = fast.cart.MapPackedLayers[1].PixelStart + 4
	fast.invalidateMapPacked(1)
	fast.PrepareMapPacked()
	for _, c := range []*Context{fast, plain} {
		c.CameraReset()
		c.ClipReset()
		c.PalReset()
		c.PaltReset()
		c.Cls(99)
		c.MapBGAt(1, 1, 0, 0, 0, 0, 40, 4, 0)
	}
	if !fast.mapPackedFailed[1] {
		t.Fatal("overflow not detected")
	}
	assertFramesMatch(t, fast, plain, "packed overflow fallback")
}

func TestPackedMapStorageValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*AssetImage, *AssetMemory)
	}{
		{"missing pixels", func(_ *AssetImage, m *AssetMemory) { m.MapPackedPixels = nil }},
		{"missing spans", func(_ *AssetImage, m *AssetMemory) { m.MapPackedSpans = nil }},
		{"missing partitions", func(_ *AssetImage, m *AssetMemory) { m.MapPackedLayers = [4]MapPackedLayer{} }},
		{"pixel offset overflow", func(_ *AssetImage, m *AssetMemory) { m.MapPackedPixels = make([]byte, 65536) }},
		{"span offset overflow", func(_ *AssetImage, m *AssetMemory) { m.MapPackedSpans = make([]MapPackedSpan, 65536) }},
		{"pixel overlap", func(_ *AssetImage, m *AssetMemory) { m.MapPackedLayers[1].PixelStart = 0 }},
		{"span overlap", func(_ *AssetImage, m *AssetMemory) { m.MapPackedLayers[1].SpanStart = 0 }},
		{"unaligned start", func(_ *AssetImage, m *AssetMemory) { m.MapPackedLayers[0].PixelStart = 1 }},
		{"unaligned end", func(_ *AssetImage, m *AssetMemory) { m.MapPackedLayers[0].PixelEnd-- }},
		{"pixel end beyond storage", func(_ *AssetImage, m *AssetMemory) { m.MapPackedLayers[3].PixelEnd = 64000 }},
		{"span end beyond storage", func(_ *AssetImage, m *AssetMemory) { m.MapPackedLayers[3].SpanEnd = 5000 }},
		{"reversed pixels", func(_ *AssetImage, m *AssetMemory) { m.MapPackedLayers[0].PixelStart = m.MapPackedLayers[0].PixelEnd }},
		{"reversed spans", func(_ *AssetImage, m *AssetMemory) { m.MapPackedLayers[0].SpanStart = m.MapPackedLayers[0].SpanEnd }},
		{"streamed storage", func(i *AssetImage, m *AssetMemory) {
			i.MapStreaming = true
			m.MapStreaming = true
			m.Map = make([]uint16, MapCacheCells)
			m.MapChunks = make([]MapChunk, MapCacheSlots)
			m.MapEdits = make([]MapEdit, MapEditCapacity)
		}},
		{"mutable sheet", func(i *AssetImage, m *AssetMemory) {
			i.SheetReadOnly = false
			m.SheetReadOnly = false
			i.Sheet = make([]byte, 64*8)
			m.Sheet = make([]byte, 64*8)
			i.SheetIndex = nil
			m.SheetIndex = nil
			m.SheetSolid = nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			image, memory := packedAssets(t, true)
			test.mutate(image, memory)
			if new(Context).Init(image, memory) {
				t.Fatal("invalid packed storage accepted")
			}
		})
	}
}

func TestPackedMapSparsePlaneAndReinitialization(t *testing.T) {
	image, memory := packedAssets(t, true)
	image.MapPlaneMask, memory.MapPlaneMask = 8, 8
	image.Map = image.Map[:40*4]
	memory.Map = memory.Map[:40*4]
	memory.MapPackedLayers = [4]MapPackedLayer{{}, {}, {}, {0, 15000, 0, 1000}}
	c := new(Context)
	if !c.Init(image, memory) {
		t.Fatal("sparse Init")
	}
	c.Cls(91)
	c.Camera(7, 3)
	c.Clip(5, 5, 220, 150)
	before, cursor := bytes.Clone(c.fbForTest()), c.cursor
	c.PrepareMapPacked()
	if !bytes.Equal(c.fbForTest(), before) || c.cursor != cursor {
		t.Fatal("prepare changed draw state")
	}
	if !c.mapPackedReady[3] || c.mapPackedReady[0] {
		t.Fatal("cache used storage slot as logical plane")
	}
	c.MsetLayer(3, 0, 0, 6)
	if c.mapPackedReady[3] {
		t.Fatal("sparse Mset retained stale cache")
	}
	c.PrepareMapPacked()
	if !c.mapPackedReady[3] {
		t.Fatal("sparse cache did not rebuild")
	}
	if !c.Init(image, memory) {
		t.Fatal("reinit")
	}
	if c.mapPackedReady[3] || c.mapPackedFailed[3] {
		t.Fatal("reinit retained cache state")
	}

	// Removing the only resident plane makes its nonempty partition invalid.
	image.MapPlaneMask, memory.MapPlaneMask = 1, 1
	if c.Init(image, memory) {
		t.Fatal("partition for absent logical plane accepted")
	}
}

func TestPackedMapEmptyAndFragmentationFallback(t *testing.T) {
	image, memory := packedAssets(t, true)
	clear(image.Map)
	memory.MapPackedLayers[0].SpanEnd = 1
	c := new(Context)
	if !c.Init(image, memory) {
		t.Fatal("Init")
	}
	c.PrepareMapPacked()
	if !c.mapPackedReady[0] || c.mapPackedSpanCount[0] != 0 {
		t.Fatal("empty map was not cached")
	}
	c.Cls(91)
	c.MapBGAt(0, 0, 0, 0, 0, 0, 30, 4, 0)
	for _, pixel := range c.fbForTest() {
		if pixel != 91 {
			t.Fatal("empty cache changed pixels")
		}
	}
	c.MsetLayer(0, 0, 0, 3)
	c.MapBGAt(0, 0, 0, 0, 0, 0, 30, 4, 0)
	if c.mapPackedReady[0] || c.mapPackedFailed[0] {
		t.Fatal("drawing rebuilt invalidated cache")
	}
	c.PrepareMapPacked()
	if !c.mapPackedFailed[0] {
		t.Fatal("fragmentation did not exhaust span capacity")
	}
	want := new(Context)
	plainImage, plainMemory := packedAssets(t, false)
	clear(plainImage.Map)
	plainImage.Map[0] = 3
	if !want.Init(plainImage, plainMemory) {
		t.Fatal("plain Init")
	}
	want.Cls(91)
	want.MapBGAt(0, 0, 0, 0, 0, 0, 30, 4, 0)
	assertFramesMatch(t, c, want, "fragmented cache fallback")
}

func TestPackedMapMutationFallsBackWithoutRebuilding(t *testing.T) {
	fast, plain := packedFixture(t, true), packedFixture(t, false)
	fast.PrepareMapPacked()
	for _, c := range []*Context{fast, plain} {
		c.MsetLayer(2, 1, 1, 6)
	}
	if fast.mapPackedReady[2] {
		t.Fatal("mutation retained cache")
	}
	for i := range fast.cart.MapPackedPixels {
		fast.cart.MapPackedPixels[i] = 201
	}
	before := append([]byte(nil), fast.cart.MapPackedPixels...)
	for _, c := range []*Context{fast, plain} {
		c.Cls(43)
		c.MapBGAt(2, 2, 0, 0, 0, 0, 30, 4, 0)
	}
	assertFramesMatch(t, fast, plain, "mutated map live fallback")
	for i, pixel := range before {
		if fast.cart.MapPackedPixels[i] != pixel {
			t.Fatal("drawing rebuilt cache")
		}
	}
	if !fast.mapPackedReady[0] || !fast.mapPackedReady[1] || !fast.mapPackedReady[3] {
		t.Fatal("mutation discarded other planes")
	}
	fast.PrepareMapPacked()
	if !fast.mapPackedReady[2] {
		t.Fatal("explicit preparation did not rebuild")
	}
}

func TestPackedMapUnalignedBackingFallsBack(t *testing.T) {
	image, memory := packedAssets(t, true)
	storage := make([]byte, 60008)
	offset := int((-uintptr(unsafe.Pointer(&storage[0])))&3) + 1
	memory.MapPackedPixels = storage[offset : offset+60004]
	c := new(Context)
	if !c.Init(image, memory) {
		t.Fatal("Init")
	}
	c.PrepareMapPacked()
	if c.drawMapPacked(0, 0, 0, 0, 0, 30, 0, 4, 0, 0) {
		t.Fatal("unaligned backing reached native copy")
	}
	want := packedFixture(t, false)
	for _, ctx := range []*Context{c, want} {
		ctx.Cls(91)
		ctx.MapBGAt(0, 0, 0, 0, 0, 0, 30, 4, 0)
	}
	assertFramesMatch(t, c, want, "unaligned cache fallback")
}

func TestPackedMapUniformRunsNeedNoPixelPayload(t *testing.T) {
	image, memory := packedAssets(t, true)
	clear(image.Map)
	for i := 0; i < 40*4; i++ {
		image.Map[i] = 1
	}
	memory.MapPackedPixels = []byte{11, 12, 13, 14}
	memory.MapPackedSpans = make([]MapPackedSpan, 32)
	memory.MapPackedLayers = [4]MapPackedLayer{{0, 4, 0, 32}}
	c := new(Context)
	if !c.Init(image, memory) {
		t.Fatal("Init")
	}
	c.PrepareMapPacked()
	if !c.mapPackedReady[0] || c.mapPackedSpanCount[0] != 32 {
		t.Fatal("uniform runs exhausted pixel capacity")
	}
	for i, value := range []byte{11, 12, 13, 14} {
		if memory.MapPackedPixels[i] != value {
			t.Fatal("uniform run wrote a pixel payload")
		}
	}
	for _, span := range memory.MapPackedSpans {
		if span.Length != mapPackedUniformFlag|320 || span.Offset != 7 {
			t.Fatalf("uniform span=%+v", span)
		}
	}
	plainImage, plainMemory := packedAssets(t, false)
	copy(plainImage.Map, image.Map)
	want := new(Context)
	if !want.Init(plainImage, plainMemory) {
		t.Fatal("plain Init")
	}
	for _, ctx := range []*Context{c, want} {
		ctx.Cls(91)
		ctx.Camera(7, 3)
		ctx.Clip(5, 1, 221, 159)
		ctx.MapBGAt(0, 0, 0, 0, 0, 0, 40, 4, 0)
	}
	assertFramesMatch(t, c, want, "payload-free uniform runs")
}

func TestPackedMapUniformLengthFlagBoundary(t *testing.T) {
	for _, cells := range []int{4095, 4096} {
		image, memory := packedAssets(t, true)
		image.MapW, memory.MapW = cells, cells
		image.MapH, memory.MapH = 1, 1
		image.MapLayers, memory.MapLayers = 1, 1
		image.MapPlaneMask, memory.MapPlaneMask = 1, 1
		image.Map = make([]uint16, cells)
		memory.Map = make([]uint16, cells)
		for i := range image.Map {
			image.Map[i] = 1
		}
		memory.MapPackedPixels = make([]byte, 4)
		memory.MapPackedSpans = make([]MapPackedSpan, 8)
		memory.MapPackedLayers = [4]MapPackedLayer{{0, 4, 0, 8}}
		c := new(Context)
		if !c.Init(image, memory) {
			t.Fatal("Init")
		}
		c.PrepareMapPacked()
		if cells == 4096 {
			if c.mapPackedReady[0] {
				t.Fatal("32768-pixel width can alias the uniform flag")
			}
			continue
		}
		if !c.mapPackedReady[0] {
			t.Fatal("long uniform run incorrectly needs pixel capacity")
		}
		for _, span := range memory.MapPackedSpans {
			if span.Length != mapPackedUniformFlag|32760 {
				t.Fatalf("length=%d", span.Length)
			}
		}
	}
}

func TestPackedMapLateColorChangeNeedsFullPayload(t *testing.T) {
	image, memory := packedAssets(t, true)
	clear(image.Map)
	for i := 0; i < 40*4; i++ {
		image.Map[i] = 1
	}
	// Every source row stays color7 until its final pixel. A partial scan
	// must not classify that long run as uniform or charge only its suffix.
	for i := 0; i < 64; i++ {
		image.Sheet[2*64+i] = 7
	}
	for y := 0; y < 8; y++ {
		image.Sheet[2*64+y*8+7] = 8
	}
	for y := 0; y < 4; y++ {
		image.Map[y*40+39] = 3
	}
	memory.MapPackedPixels = []byte{11, 12, 13, 14}
	memory.MapPackedSpans = make([]MapPackedSpan, 32)
	memory.MapPackedLayers = [4]MapPackedLayer{{0, 4, 0, 32}}
	c := new(Context)
	if !c.Init(image, memory) {
		t.Fatal("Init")
	}
	c.PrepareMapPacked()
	if c.mapPackedReady[0] || !c.mapPackedFailed[0] {
		t.Fatal("late color change escaped mixed-run capacity check")
	}
	for i, value := range []byte{11, 12, 13, 14} {
		if memory.MapPackedPixels[i] != value {
			t.Fatal("failed run wrote beyond its reservation")
		}
	}
}
