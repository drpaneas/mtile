//go:build !mtile_mode0

package runtime

import (
	"bytes"
	"fmt"
	"testing"
)

func TestMapBGAtVDefaultsEveryOptionalArgumentAfterBG(t *testing.T) {
	values := [8]Value{
		IntVal(2), IntVal(1), IntVal(1), IntVal(3),
		IntVal(5), IntVal(2), IntVal(2), IntVal(2),
	}
	for provided := 1; provided <= len(values); provided++ {
		t.Run(fmt.Sprintf("arity-%d", provided), func(t *testing.T) {
			makeContext := func() *Context {
				sheet := make([]byte, 16*8)
				for index := 8; index < 16; index++ {
					sheet[index] = 7
				}
				image := AssetImage{
					SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
					MapW: 3, MapH: 3, MapLayers: 4, MapPlaneMask: 0x04,
					Sheet: sheet, Map: []uint16{1, 1, 1, 1, 1, 1, 1, 1, 1}, Flags: []byte{0, 1},
				}
				ram := &AssetMemory{
					SheetW: 16, SheetH: 8, SpriteCols: 2, SpriteRows: 1, SpriteN: 2,
					MapW: 3, MapH: 3, MapLayers: 4, MapPlaneMask: 0x04,
					Sheet: make([]byte, len(sheet)), Map: make([]uint16, 9), Flags: make([]byte, 2),
				}
				var c Context
				if !c.Init(&image, ram) {
					t.Fatal("Init rejected compact BG2 arity fixture")
				}
				c.Cls(0)
				return &c
			}

			args := [8]Value{}
			copy(args[:provided], values[:provided])
			got := makeContext()
			got.MapBGAtV(0, args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7])

			expected := [8]int{}
			for index := 0; index < provided; index++ {
				expected[index] = values[index].Int()
			}
			want := makeContext()
			want.MapBGAt(0, expected[0], expected[1], expected[2], expected[3], expected[4], expected[5], expected[6], expected[7])
			if !bytes.Equal(got.fbForTest(), want.fbForTest()) {
				t.Fatalf("MapBGAtV arity %d did not match explicit defaults", provided)
			}
		})
	}
}
