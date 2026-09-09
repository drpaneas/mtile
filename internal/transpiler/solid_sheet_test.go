package transpiler

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSolidSheetColorsPreserveCompactSlotAndColorRange(t *testing.T) {
	pixels := make([]byte, 4*64)
	for i := 64; i < 128; i++ {
		pixels[i] = 255
	}
	pixels[2*64+63] = 7 // A late mismatch must not be classified as solid.
	for i := 3 * 64; i < len(pixels); i++ {
		pixels[i] = 18
	}
	if got, want := solidSheetColors(pixels), []uint16{1, 256, 0, 19}; !reflect.DeepEqual(got, want) {
		t.Fatalf("solid metadata %v want %v", got, want)
	}
	if got := solidSheetColors(pixels[2*64 : 3*64]); got != nil {
		t.Fatalf("mixed-only sheet retained useless metadata: %v", got)
	}
}

func TestSolidSheetMetadataOnlyForImmutableSoftwareMaps(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		renderer     Renderer
		solid, want  bool
	}{
		{"map", `function _draw() map(0) end`, RendererMode4, true, true},
		{"mutable", `function _draw() sset(8,0,7) map(0) end`, RendererMode4, true, false},
		{"mixed", `function _draw() map(0) end`, RendererMode4, false, false},
		{"hardware", `function _draw() cls(0) map(0) end`, RendererMode0, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := rasterOptions()
			options.Renderer = tc.renderer
			for i := range options.Sheet {
				options.Sheet[i] = 255
				if !tc.solid && i%8 == 7 {
					options.Sheet[i] = 0
				}
			}
			generated, err := Transpile("solid-map.lua", tc.source, options)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(generated, "cartData.SheetSolid = sheetSolidROM[:]"); got != tc.want {
				t.Fatalf("solid metadata emitted=%v want=%v", got, tc.want)
			}
			if tc.renderer == RendererMode0 {
				t.Setenv("GOFLAGS", os.Getenv("GOFLAGS")+" -tags=mtile_mode0")
			}
			compileGenerated(t, generated)
		})
	}
}
