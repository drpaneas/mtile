package transpiler

import (
	"strings"
	"testing"
)

func rasterOptions() Options {
	sheet := make([]byte, 128)
	for i := range sheet {
		sheet[i] = 7
	}
	cells := make([]uint16, 600)
	for i := range cells {
		cells[i] = 1
	}
	return Options{SheetW: 16, SheetH: 8, MapW: 30, MapH: 20, MapLayers: 1, Sheet: sheet, Map: cells, Flags: []byte{0, 1}}
}

func TestMapRasterGeneratedStorageHasTargetMemoryGuard(t *testing.T) {
	source := `function _draw() cls(0) map(0) end`
	options := rasterOptions()
	got, err := Transpile("raster.lua", source, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`const mapRasterBudget = 256*1024 - 512 - 8192 - rt.MapRasterHeapReserve`,
		`int(unsafe.Sizeof(ctx))`, `int(unsafe.Sizeof(game))`,
		`const mapRasterEnabled = max(0, min(1, mapRasterBudget/mapRasterBytes))`,
		`[mapRasterEnabled*MapW*8*MapH*8]byte`,
		`cartData.MapRasterPixels, cartData.MapRasterSpans = mapRasterPixelsRAM[:], mapRasterSpansRAM[:]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing budget/storage gate %q", want)
		}
	}
	compileGenerated(t, got)
	options.DisableMapRasterCache = true
	plain, err := Transpile("raster.lua", source, options)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "mapRasterPixelsRAM") {
		t.Fatal("disabled cache still allocates storage")
	}
	compileGenerated(t, plain)
}

func TestMapRasterExcludesOtherStorageAndRendererContracts(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		modify       func(*Options)
	}{
		{"mutable-sheet", `function _draw() map(0) sset(0,0,1) end`, func(*Options) {}},
		{"mode0", `function _draw() cls(0) map(0) end`, func(o *Options) { o.Renderer = RendererMode0 }},
		{"no-map", `function _draw() cls(0) spr(1,0,0) end`, func(*Options) {}},
		{"wide-map", `function _draw() map(0) end`, func(o *Options) { o.MapW = 31; o.Map = make([]uint16, 31*20); o.Map[0] = 1 }},
		{"two-planes", `function _draw() map(0) map(1) end`, func(o *Options) { o.MapLayers = 2; o.Map = append(o.Map, make([]uint16, 600)...); o.Map[600] = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := rasterOptions()
			tc.modify(&opts)
			got, err := Transpile("raster.lua", tc.source, opts)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got, "mapRasterPixelsRAM") {
				t.Fatal("ineligible cartridge allocates cache")
			}
		})
	}
}

func TestMapRasterInsufficientBudgetCompilesZeroStorage(t *testing.T) {
	options := rasterOptions()
	options.SheetW, options.SheetH = 256, 256
	options.Sheet = make([]byte, 256*256)
	options.Flags = make([]byte, 1024)
	for i := range options.Sheet {
		options.Sheet[i] = 7
	}
	got, err := Transpile("raster-large-assets.lua", `function _draw() map(0) end`, options)
	if err != nil {
		t.Fatal(err)
	}
	// A negative budget must become zero-sized arrays, without uintptr
	// underflow or an otherwise valid cartridge acquiring a cache allocation.
	got += "\nvar _ [0-mapRasterEnabled]byte\n"
	compileGenerated(t, got)
}
