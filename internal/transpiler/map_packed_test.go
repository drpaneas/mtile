package transpiler

import (
	"strings"
	"testing"
)

func TestPackedMapPlanCountsAlignedSourceRunsAndSparsePlanes(t *testing.T) {
	g := generator{opts: Options{MapW: 40, MapH: 2, MapPlaneMask: 10, MapDrawMask: 8, MapLayers: 4}}
	g.sheetIndex = []uint16{0, 1, 2}
	g.sheetCompact = make([]byte, 128)
	for i := 0; i < 64; i++ {
		g.sheetCompact[i] = 255
		if i%8 < 5 {
			g.sheetCompact[64+i] = 7
		}
	}
	g.mapCompact = make([]uint16, 160)
	g.mapCompact[0] = 1 // BG1 exists but is not drawn.
	for _, start := range []int{80, 86} {
		for i := 0; i < 4; i++ {
			g.mapCompact[start+i] = 1
		}
		g.mapCompact[start+4] = 2
	}
	plan := g.planPackedMap()
	// BG3 has 8 rows of two separate 37-pixel runs:
	// 2*align4(37)=80 bytes/row, plus bounded initialization allowance.
	if got, want := plan[3], (packedMapPlane{640 + 256, 16 + 16}); got != want {
		t.Fatalf("BG3 capacity %+v want %+v", got, want)
	}
	for _, bg := range []int{0, 1, 2} {
		if plan[bg] != (packedMapPlane{}) {
			t.Fatalf("undrawn plane %d retained storage: %+v", bg, plan[bg])
		}
	}
	g.opts.DisableMapRasterCache = true
	if g.planPackedMap() != [4]packedMapPlane{} {
		t.Fatal("disabled raster cache retained packed storage")
	}
}

func TestPackedMapPlanRejectsUnrepresentableStorage(t *testing.T) {
	g := generator{opts: Options{MapW: 64, MapH: 32, MapPlaneMask: 1, MapDrawMask: 1, MapLayers: 1}}
	g.sheetIndex = []uint16{0, 1}
	g.sheetCompact = make([]byte, 64)
	for i := range g.sheetCompact {
		g.sheetCompact[i] = byte(i%8 + 1)
	}
	g.mapCompact = make([]uint16, 64*32)
	for i := range g.mapCompact {
		g.mapCompact[i] = 1
	}
	if got := g.planPackedMap(); got != [4]packedMapPlane{} {
		t.Fatalf("uint16 pixel offsets would overflow: %+v", got)
	}
}

func TestPackedMapUniformRunsNeedNoPixelPayload(t *testing.T) {
	g := generator{opts: Options{MapW: 64, MapH: 32, MapPlaneMask: 1, MapDrawMask: 1, MapLayers: 1}}
	g.sheetIndex = []uint16{0, 1}
	g.sheetCompact = make([]byte, 64)
	for i := range g.sheetCompact {
		g.sheetCompact[i] = 255
	}
	g.mapCompact = make([]uint16, 64*32)
	for i := range g.mapCompact {
		g.mapCompact[i] = 1
	}
	if got, want := g.planPackedMap()[0], (packedMapPlane{256, 256 + 16}); got != want {
		t.Fatalf("uniform source retained pixel storage: %+v want %+v", got, want)
	}
	// A mismatch at the end of a tile makes each eighth source row mixed.
	// Those complete runs need payload, even after a long uniform prefix.
	g.sheetCompact[63] = 1
	if got, want := g.planPackedMap()[0], (packedMapPlane{32*512 + 256, 256 + 16}); got != want {
		t.Fatalf("late mixed pixel lost payload: %+v want %+v", got, want)
	}
}

func packedMapOptions() Options {
	opts := rasterOptions()
	opts.MapW, opts.MapH, opts.MapLayers, opts.MapPlaneMask = 40, 2, 4, 10
	opts.Map = make([]uint16, 320)
	// Input planes are dense; generation compacts BG1/BG3. Four adjacent
	// opaque tiles make one 32-pixel run per row, meeting the cost gate.
	for i := 240; i < 244; i++ {
		opts.Map[i] = 1
	}
	return opts
}

func TestPackedMapGeneratedStorageAndTargetBudget(t *testing.T) {
	got, err := Transpile("packed.lua", `function _draw() map(3) end`, packedMapOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`const mapPackedBytes = 256 + 24*int(unsafe.Sizeof(rt.MapPackedSpan{}))`,
		`const mapPackedEnabled = max(0, min(1, mapPackedBudget/mapPackedBytes))`,
		`var mapPackedWordsRAM [mapPackedEnabled*64]uint32`,
		`3: {PixelStart: uint16(mapPackedEnabled*0), PixelEnd: uint16(mapPackedEnabled*256)`,
		`unsafe.Pointer(&mapPackedWordsRAM)`,
		`ctx.PrepareMapPacked()`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing packed storage property %q", want)
		}
	}
	compileGenerated(t, got)
	// Force the compile-time budget below zero. Zero storage must still
	// compile; taking &words[0] would fail for this valid fallback case.
	limited := strings.Replace(got, "const mapPackedBudget = 256*1024", "const mapPackedBudget = 0", 1)
	limited += "\nvar _ [0-mapPackedEnabled]byte\n"
	compileGenerated(t, limited)
}

func TestPackedMapPreservesOtherStorageContracts(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		modify       func(*Options)
	}{
		{"disabled", `function _draw() map(3) end`, func(o *Options) { o.DisableMapRasterCache = true }},
		{"mutable", `function _draw() map(3) sset(0,0,1) end`, func(*Options) {}},
		{"mode0", `function _draw() cls(0) map(3) end`, func(o *Options) { o.Renderer = RendererMode0 }},
		{"no-map", `function _draw() spr(1,0,0) end`, func(*Options) {}},
		{"fragmented", `function _draw() map(3) end`, func(o *Options) {
			for i := range o.Map {
				o.Map[i] = 0
			}
			o.Map[240], o.Map[242], o.Map[244], o.Map[246] = 1, 1, 1, 1
		}},
		{"streaming", `function _draw() map(3) end`, func(o *Options) {
			o.MapW, o.MapH = 65, 33
			o.Map = make([]uint16, 65*33*4)
			o.Map[65*33*3] = 1
		}},
		{"small-raster", `function _draw() map(0) end`, func(o *Options) { *o = rasterOptions() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := packedMapOptions()
			tc.modify(&opts)
			got, err := Transpile("packed.lua", tc.source, opts)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got, "mapPackedWordsRAM") {
				t.Fatal("ineligible cartridge allocates packed cache")
			}
		})
	}
}
